#!/usr/bin/env python3
"""Measure real workers in disposable Docker resources; never connect to the VDS."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import shutil
import time
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', required=True, help='Already built backend image')
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--ca-file', type=Path, help='Optional runtime proxy CA bundle')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    prefix = 'catalog-budget-' + uuid.uuid4().hex[:10]
    network, database = prefix, prefix + '-db'
    created = []
    staging = tempfile.TemporaryDirectory(prefix=prefix + '-')
    stage = Path(staging.name)
    stage.chmod(0o755)
    for filename in ('updater-sources.json', 'real-catalog.json'):
        shutil.copyfile(root / 'data' / filename, stage / filename)
        (stage / filename).chmod(0o644)
    results = {'scope': 'disposable live-source verification; not production coverage',
               'configured_feeds': len(json.loads((root / 'data/updater-sources.json').read_text())['discovery']),
               'worker_limit_bytes': 128 * 1024 * 1024, 'workers': {}}

    def docker(*parts, check=True):
        return subprocess.run(['docker', *parts], text=True, capture_output=True,
                              check=check, timeout=60)

    def inspect(name):
        return json.loads(docker('inspect', name).stdout)[0]

    def run_worker(role, command, name):
        options = ['run', '-d', '--name', name, '--network', network, '--memory', '128m',
                   '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
                   '-v', str(stage) + ':/data:ro', '-e', 'GOMEMLIMIT=96MiB',
                   '-e', 'CATALOG_WORKER=' + role, '-e', 'PGHOST=' + database,
                   '-e', 'PGDATABASE=devcourse_test', '-e', 'PGUSER=devcourse',
                   '-e', 'PGPASSWORD=disposable-budget-check', '-e', 'PGSSLMODE=disable']
        for key in ('HTTPS_PROXY', 'HTTP_PROXY', 'NO_PROXY'):
            if key in os.environ:
                options += ['-e', key]
        if args.ca_file:
            options += ['-v', str(args.ca_file.resolve()) + ':/run/proxy-ca.pem:ro',
                        '-e', 'SSL_CERT_FILE=/run/proxy-ca.pem']
        docker(*options, args.image, *command)
        created.append(name)

    try:
        docker('network', 'create', network)
        docker('run', '-d', '--name', database, '--network', network,
               '--memory', '256m', '-e', 'POSTGRES_DB=devcourse_test',
               '-e', 'POSTGRES_USER=devcourse', '-e', 'POSTGRES_PASSWORD=disposable-budget-check',
               'postgres:17-alpine')
        created.append(database)
        ready = time.monotonic() + 60
        while docker('exec', database, 'pg_isready', '-U', 'devcourse', check=False).returncode:
            if time.monotonic() > ready:
                raise RuntimeError('Disposable database did not start')
            time.sleep(.5)
        migrate = prefix + '-migrate'
        run_worker('publisher', ['migrate'], migrate)
        if docker('wait', migrate).stdout.strip() != '0':
            raise RuntimeError('Disposable migration failed')
        names = {role: prefix + '-' + role for role in ('api', 'pages', 'publisher')}
        run_worker('publisher', ['catalog-update', 'serve'], names['publisher'])
        for role in ('api', 'pages'):
            run_worker(role, ['catalog-update', 'once'], names[role])
        started = time.monotonic()
        deadline = started + 660
        peaks = {role: 0 for role in names}
        while True:
            running = {role: inspect(name)['State']['Running'] for role, name in names.items()}
            for role, name in names.items():
                if running[role]:
                    usage = docker('stats', '--no-stream', '--format', '{{.MemUsage}}', name).stdout.split('/')[0].strip()
                    match = re.fullmatch(r'([\d.]+)([A-Za-z]+)', usage)
                    if match:
                        units = {'B': 1, 'KiB': 1024, 'MiB': 1024**2, 'GiB': 1024**3,
                                 'kB': 1000, 'MB': 1000**2, 'GB': 1000**3}
                        peaks[role] = max(peaks[role], int(float(match[1]) * units[match[2]]))
            print(json.dumps({'elapsed_seconds': round(time.monotonic() - started),
                              'running': running, 'sampled_peak_bytes': peaks}), flush=True)
            if not running['publisher']:
                raise RuntimeError('Publisher exited during collection')
            if not running['api'] and not running['pages']:
                break
            if time.monotonic() > deadline:
                raise RuntimeError('Workers exceeded the bounded run deadline')
            time.sleep(10)
        # Publisher ticks every 10 seconds; wait until all collector observations drain.
        for _ in range(15):
            pending = docker('exec', database, 'psql', '-U', 'devcourse', '-d', 'devcourse_test',
                             '-Atc', 'SELECT count(*) FROM catalog_observations WHERE processed_at IS NULL').stdout.strip()
            if pending == '0':
                break
            time.sleep(5)
        else:
            raise RuntimeError('Publisher did not drain the collected observations')
        report = docker('exec', names['publisher'], 'devcourse-finder', 'catalog-update', 'status').stdout
        results['status'] = [json.loads(line) for line in report.splitlines()]
        results['duration_seconds'] = round(time.monotonic() - started, 2)
        for role, name in names.items():
            state = inspect(name)['State']
            results['workers'][role] = {'sampled_peak_bytes': peaks[role],
                                       'oom_killed': state['OOMKilled'], 'exit_code': state['ExitCode'],
                                       'running': state['Running']}
            # Fixed diagnostics contain provider IDs and rejection codes, no secrets.
            results['workers'][role]['log'] = docker('logs', name).stderr.splitlines()[-80:]
            if state['OOMKilled'] or (role != 'publisher' and state['ExitCode'] != 0):
                raise RuntimeError(role + ' failed the resource/collection check')
        results['passed'] = True
    except Exception as error:
        results['passed'] = False
        results['error'] = str(error)
        for name in created:
            if name != database:
                result = docker('logs', name, check=False)
                results['workers'][name] = {'log': result.stderr.splitlines()[-80:]}
        raise
    finally:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(results, indent=2) + '\n')
        for name in reversed(created):
            docker('rm', '-f', name, check=False)
        docker('network', 'rm', network, check=False)
        staging.cleanup()


if __name__ == '__main__':
    main()
