#!/usr/bin/env python3
"""Bounded paid source/model/review/publication acceptance; keys stay on the VDS."""
from datetime import datetime, timedelta, timezone
import fcntl
import json
from pathlib import Path
import re
import signal
import subprocess
import sys
import uuid


def targets(raw):
    ids = raw.split(',')
    if not 1 <= len(ids) <= 4 or len(set(ids)) != len(ids) or any(
            not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,149}', item) for item in ids):
        raise ValueError('Specify 1-4 unique published course IDs, comma-separated.')
    return ids


def verify(base, ids):
    base = Path(base)
    if not base.is_absolute() or not base.is_dir():
        raise ValueError('Deployment directory is missing.')
    env = base / '.env'
    if not env.is_file() or env.is_symlink():
        raise ValueError('Production environment file is missing or a symlink.')
    with (base / '.deploy.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        release = (base / 'current').resolve(strict=True)
        for name in ('compose.yaml', 'compose.production.yaml', 'VERSION'):
            if not (release / name).is_file():
                raise ValueError('Deployed release files are missing.')
        print('Deployed release:', (release / 'VERSION').read_text().strip(), flush=True)
        compose = ['docker', 'compose', '--project-name', 'devcourse-finder', '--env-file', str(env),
                   '-f', str(release / 'compose.yaml'), '-f', str(release / 'compose.production.yaml'),
                   '--profile', 'ai']
        for role in ('catalog-ai', 'catalog-publisher'):
            running = subprocess.run(compose + ['ps', '--status', 'running', '-q', role],
                                     capture_output=True, text=True, check=True)
            if not running.stdout.strip():
                raise ValueError('Required deployed role is not running: ' + role)
        for course in ids:
            now = datetime.now(timezone.utc)
            if (now + timedelta(minutes=11)).date() != now.date():
                raise ValueError('Too close to UTC budget rollover; no paid course was started.')
            owned = 'devcourse-ai-verify-' + uuid.uuid4().hex[:16]
            try:
                # No budget override, schedule edits, builds, migrations, source
                # postponements, forced cache bypass or general batch selection.
                result = subprocess.run(compose + ['run', '--rm', '--no-deps', '--pull', 'never', '-T',
                                        '--name', owned, 'catalog-ai', 'catalog-ai', 'verify', course],
                                        stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=620)
                print(result.stderr, end='', flush=True)
                reports = []
                for line in result.stdout.splitlines():
                    try:
                        value = json.loads(line)
                    except ValueError:
                        continue
                    if isinstance(value, dict) and value.get('course_id') == course and 'financial_unchanged' in value:
                        reports.append(value)
                if len(reports) != 1:
                    raise ValueError('No complete single-course report; do not retry blindly.')
                report = reports[0]
                print('VERIFY_AI_REPORT ' + json.dumps(report, separators=(',', ':')), flush=True)
                if result.returncode or report.get('financial_unchanged') is not True or not report.get('verified_claims'):
                    raise ValueError('Verification did not pass; stopping before another paid course.')
            finally:
                subprocess.run(['docker', 'rm', '-f', owned], stdin=subprocess.DEVNULL,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
    return 0


def interrupted(signum, frame):
    raise RuntimeError('Verification interrupted; the owned container will be removed.')


if __name__ == '__main__':
    try:
        for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
            signal.signal(sig, interrupted)
        if len(sys.argv) != 3:
            raise ValueError('Usage: verify-ai-courses.py DEPLOYMENT_DIRECTORY COURSE_IDS')
        raise SystemExit(verify(sys.argv[1], targets(sys.argv[2])))
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError) as error:
        print(type(error).__name__ + ': verification failed; inspect the bounded report.', file=sys.stderr)
        raise SystemExit(1)
