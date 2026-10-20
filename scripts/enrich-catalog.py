#!/usr/bin/env python3
"""Manual AI backfill of the deployed release; model keys stay on the VDS."""
import argparse
import collections
import fcntl
import json
import os
import re
from pathlib import Path
import signal
import subprocess
import sys
import uuid

MARKER = 'AI_ENRICHMENT_SUMMARY '


class EnrichmentError(Exception):
    pass


class Cancelled(EnrichmentError):
    pass


def objects(lines):
    for line in lines:
        try:
            value = json.loads(line)
        except (ValueError, TypeError):
            continue
        if isinstance(value, dict):
            yield value


def run(command):
    values = []
    process = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, text=True)
    try:
        for line in process.stdout:
            print(line, end='', flush=True)
            values.extend(objects([line]))
        if process.wait() != 0:
            raise EnrichmentError('Deployed command failed; see step logs.')
    finally:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        process.stdout.close()
    return values


def header(values):
    matches = [v for v in values if v.get('ai_collector') is True]
    if len(matches) != 1:
        raise EnrichmentError('Missing AI status response; verify the deployed release.')
    return matches[0]


def batch_result(values):
    matches = [v for v in values if 'run_id' in v and all(k in v for k in
               ('attempted', 'queued', 'cached', 'failed', 'tokens', 'status'))]
    if len(matches) != 1:
        raise EnrichmentError('Missing batch result; refusing to start another batch.')
    value = matches[0]
    for key in ('attempted', 'queued', 'cached', 'failed', 'tokens'):
        if type(value[key]) is not int or value[key] < 0:
            raise EnrichmentError('Invalid batch result.')
    return value


def provider_failure(code):
    return any(code == provider + '_' + reason for provider in ('openai', 'deepseek')
               for reason in ('unavailable', 'authentication_failed', 'insufficient_quota',
                              'rate_limited', 'model_unavailable', 'protocol_rejected'))


def enrich(base, mode, budget, max_batches, course_ids=''):
    ids = course_ids.split(',') if course_ids else []
    if (len(ids) > 8 or len(set(ids)) != len(ids) or
            any(not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,99}', value) for value in ids) or
            mode == 'sources' and not ids or mode == 'status' and ids):
        raise EnrichmentError('Select 1-8 unique published course IDs for source checks or targeted enrichment.')
    if ids and mode == 'enrich':
        max_batches = 1  # A bounded acceptance never turns into a backfill loop.
    base = Path(base)
    if not base.is_absolute() or not base.is_dir():
        raise EnrichmentError('Deployment directory is missing.')
    env = base / '.env'
    if not env.is_file() or env.is_symlink():
        raise EnrichmentError('Production .env is missing or is a symlink.')
    summary = dict(mode=mode, stop_reason='error', daily_request_limit=budget,
                   batches=0, attempted=0, queued=0, cached=0, failed=0, tokens=0,
                   course_codes={}, useful_queued=0, empty_queued=0, verified_claims=0, discarded_claims=0)
    summary['selected_course_ids'] = ids
    oneoff = 'devcourse-ai-manual-' + uuid.uuid4().hex[:16]
    codes = collections.Counter()
    # Same host lock as deploy/collection, before resolving the release symlink.
    with (base / '.deploy.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise EnrichmentError('Deployment or manual collection is running. Retry after it finishes.')
        release = (base / 'current').resolve(strict=True)
        for name in ('compose.yaml', 'compose.production.yaml', 'VERSION'):
            if not (release / name).is_file():
                raise EnrichmentError('The deployed release files are missing.')
        summary['version'] = (release / 'VERSION').read_text().strip()
        compose = ['docker', 'compose', '--project-name', 'devcourse-finder', '--env-file', str(env),
                   '-f', str(release / 'compose.yaml'), '-f', str(release / 'compose.production.yaml'),
                   '--profile', 'ai']
        try:
            # Both roles must already be running: do not build, migrate or restart.
            for role in ('catalog-ai', 'catalog-publisher'):
                value = subprocess.run(compose + ['ps', '--status', 'running', '-q', role],
                                       stdin=subprocess.DEVNULL, capture_output=True, text=True, check=True)
                if not value.stdout.strip():
                    raise EnrichmentError('The deployed ' + role + ' is not running.')
            status_command = compose + ['exec', '--interactive=false', '-T', 'catalog-ai',
                                        'devcourse-finder', 'catalog-ai', 'status']
            summary['before'] = header(run(status_command))
            if mode == 'status':
                summary['after'] = summary['before']
                summary['stop_reason'] = 'status_only'
                return 0
            if mode == 'sources':
                values = run(compose + ['exec', '--interactive=false', '-T', 'catalog-ai',
                                       'devcourse-finder', 'catalog-ai', 'sources', course_ids])
                reports = [v for v in values if v.get('source_check') is True]
                if len(reports) != 1:
                    raise EnrichmentError('Missing source check report.')
                summary['source_check'] = reports[0]
                summary['stop_reason'] = 'source_check_only'
                return 0
            # Leave room for the worker's 30-minute timeout before UTC budget rollover.
            from datetime import datetime, timedelta, timezone
            budget_day = datetime.now(timezone.utc).date()
            summary['stop_reason'] = 'batch_limit'
            for _ in range(max_batches):
                if (datetime.now(timezone.utc) + timedelta(minutes=31)).date() != budget_day:
                    summary['stop_reason'] = 'budget_day_boundary'
                    break
                selection = ['-e', 'CATALOG_AI_COURSE_IDS=' + course_ids] if ids else []
                values = run(compose + ['run', '--rm', '--no-deps', '--pull', 'never', '-T',
                             '--name', oneoff, '-e', 'CATALOG_AI_BATCH_SIZE=40',
                             '-e', 'CATALOG_AI_DAILY_REQUEST_LIMIT=' + str(budget),
                             *selection, 'catalog-ai', 'catalog-ai', 'once'])
                result = batch_result(values)
                summary['batches'] += 1
                for key in ('attempted', 'queued', 'cached', 'failed', 'tokens'):
                    summary[key] += result[key]
                for key in ('useful_queued', 'empty_queued', 'verified_claims', 'discarded_claims'):
                    value = result.get(key)
                    if value is None:
                        summary[key] = None  # Older deployed releases do not expose these counters.
                    elif type(value) is not int or value < 0:
                        raise EnrichmentError('Invalid enrichment counters.')
                    elif summary[key] is not None:
                        summary[key] += value
                course_codes = [v.get('code', '') for v in values if v.get('msg') == 'AI course checked']
                codes.update(course_codes)
                if 'daily_budget_exhausted' in course_codes:
                    summary['stop_reason'] = 'daily_budget_exhausted'
                    break
                if any(provider_failure(code) for code in course_codes):
                    summary['stop_reason'] = 'provider_unavailable'
                    summary['after'] = header(run(status_command))
                    return 1
                if result['attempted'] == 0:
                    summary['stop_reason'] = 'no_due_courses'
                    break
            summary['after'] = header(run(status_command))
            return 0
        except Cancelled:
            summary['stop_reason'] = 'cancelled'
            raise
        except (EnrichmentError, OSError, subprocess.SubprocessError):
            summary['stop_reason'] = 'error'
            raise
        finally:
            # A lost SSH connection/timeout must kill only this owned one-off
            # container, not the scheduled worker or its heartbeat.
            if mode == 'enrich':
                for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
                    signal.signal(sig, signal.SIG_IGN)
                try:
                    subprocess.run(['docker', 'rm', '-f', oneoff], stdin=subprocess.DEVNULL,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
                except (OSError, subprocess.TimeoutExpired):
                    pass
            summary['course_codes'] = dict(codes)
            print(MARKER + json.dumps(summary, separators=(',', ':')), flush=True)


def write_summary(log, destination):
    reports = []
    for line in Path(log).read_text(errors='replace').splitlines():
        if line.startswith(MARKER):
            reports.append(json.loads(line[len(MARKER):]))
    with Path(destination).open('a') as output:
        output.write('### AI catalog enrichment\n\n')
        if len(reports) != 1:
            output.write('No complete remote report. Check the SSH/enrichment step logs.\n')
            return
        r = reports[0]
        if r.get('selected_course_ids'):
            output.write('\nSelected courses: `' + ', '.join(r['selected_course_ids']) + '`.\n')
        if r.get('source_check'):
            output.write('\nSource check only: no model requests or database writes. Partial course text does not verify a tariff.\n\n')
            output.write('| Course | Result | Course documents | Offer documents | Bytes |\n|---|---|---|---|---|\n')
            for row in r['source_check']['results']:
                output.write('| ' + ' | '.join(str(row[k]) for k in ('course_id', 'code', 'course_documents', 'offer_documents', 'text_bytes')) + ' |\n')
        output.write(f"- Release: `{r['version']}`\n- Stop reason: `{r['stop_reason']}`\n")
        output.write(f"- Temporary shared daily cap: {r['daily_request_limit']} requests\n")
        output.write('\n| Batches | Attempted | Queued | Cached | Failed | Tokens this run |\n|---|---|---|---|---|---|\n')
        output.write('| ' + ' | '.join(str(r[k]) for k in ('batches', 'attempted', 'queued', 'cached', 'failed', 'tokens')) + ' |\n')
        output.write(f"\nQueued with verified facts: {r.get('useful_queued') if r.get('useful_queued') is not None else 'unknown'}; "
                     f"queued without verified facts: {r.get('empty_queued') if r.get('empty_queued') is not None else 'unknown'}; "
                     f"verified claims: {r.get('verified_claims') if r.get('verified_claims') is not None else 'unknown'}; "
                     f"discarded claims: {r.get('discarded_claims') if r.get('discarded_claims') is not None else 'unknown'}.\n")
        output.write('\n| Course result | Count |\n|---|---|\n')
        for code, count in sorted(r['course_codes'].items()):
            output.write(f'| `{code}` | {count} |\n')
        after = r.get('after', r.get('before', {}))
        output.write(f"\nRequests reserved today: {after.get('requests_reserved_today', 'unknown')}; "
                     f"pending AI observations: {after.get('pending', 'unknown')}.\n")
        for health in after.get('provider_health', []):
            output.write(f"\nSource `{health['provider']}`: `{health['code']}`; next probe after `{health['next_probe_at']}`.\n")
        output.write('\nFull catalog source states: `' + json.dumps(after.get('source_codes', {}), sort_keys=True) + '`.\n')
        output.write('\nQueued means independently checked and awaiting publication, not necessarily a changed card. '
                     'Only due published courses are selected; source cooldowns and cache remain active. '
                     'A successful workflow can stop at its budget/batch cap with courses remaining. '
                     'Repeat after the UTC daily reset (03:00 Moscow) to continue. '
                     'The permanent .env and Monday/Thursday schedule are unchanged.\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--summary', nargs=2, metavar=('LOG', 'OUTPUT'))
    parser.add_argument('base', nargs='?', default='/srv/devcourse-finder')
    parser.add_argument('mode', nargs='?', choices=('enrich', 'status', 'sources'), default='status')
    parser.add_argument('budget', nargs='?', type=int, choices=(40, 100, 200), default=200)
    parser.add_argument('batches', nargs='?', type=int, choices=range(1, 26), default=10)
    parser.add_argument('course_ids', nargs='?', default='')
    args = parser.parse_args()
    if args.summary:
        write_summary(*args.summary)
        return 0
    def cancel(_signum, _frame):
        raise Cancelled('Manual enrichment cancelled.')
    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(sig, cancel)
    try:
        return enrich(args.base, args.mode, args.budget, args.batches, args.course_ids)
    except (EnrichmentError, OSError, subprocess.SubprocessError) as error:
        # Fixed messages only: never dump captured Compose configuration or .env.
        print(str(error) if isinstance(error, EnrichmentError) else 'Enrichment operation failed; inspect service status.', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
