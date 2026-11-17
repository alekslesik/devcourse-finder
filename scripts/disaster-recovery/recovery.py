#!/usr/bin/env python3
"""Encrypted deployment snapshots and fail-closed recovery. Run as root."""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
import re
from pathlib import Path
import secrets
import stat
import shutil
import subprocess as sp
import sys
import tarfile
import tempfile
import time
import zipfile

BASE = Path('/srv/devcourse-finder')
STATE = Path('/var/lib/devcourse-backup')
CONFIG = Path('/etc/devcourse-backup')
TAG = 'devcourse-full-v1'


def run(args, *, data=None, output=None, env=None):
    result = sp.run([str(a) for a in args], input=data, stdout=output or sp.PIPE,
                    stderr=sp.PIPE, env=env)
    if result.returncode:
        # Do not echo commands, rendered Compose, environment, or subprocess output.
        raise RuntimeError(f'{Path(str(args[0])).name} failed (exit {result.returncode})')
    return result.stdout


def js(args):
    return json.loads(run(args))


def write(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + '.partial')
    tmp.write_text(json.dumps(value, indent=2) + '\n')
    os.chmod(tmp, 0o600)
    tmp.replace(path)


def redact_diagnostics(payload):
    text = payload.decode(errors='replace')
    text = re.sub(r'https?://[^\s]+', '<URL omitted>', text)
    return '\n'.join('<credential detail omitted>' if any(word in line.lower() for word in
                     ['authorization', 'access_token', 'refresh_token', 'access token', 'refresh token', 'oauth token', 'password']) else line
                     for line in text.splitlines()).encode()


def restic(*args, output=None, progress=False):
    cfg = json.loads((CONFIG / 'config.json').read_text())
    env = dict(os.environ, RESTIC_REPOSITORY=cfg['repository'],
               RESTIC_PASSWORD_FILE=str(CONFIG / 'password'),
               RCLONE_CONFIG=str(CONFIG / 'rclone.conf'),
               RESTIC_CACHE_DIR=str(STATE / 'cache'))
    env['PATH']=str(Path(__file__).parent/'bin')+os.pathsep+env.get('PATH','/usr/bin:/bin')
    command = ['restic', '--limit-upload', '8192', '--limit-download', '16384',
               '--option', 'rclone.connections=32',
               '--option', 'rclone.args=serve restic --stdio --streaming-upload-cutoff=64M --disable-http2 --timeout=20m --contimeout=20s --low-level-retries=2 --tpslimit=4',
               *args]
    if not progress:
        return run(command, env=env, output=output)
    summaries = []
    last_print = 0
    with tempfile.TemporaryFile() as errors:
        with sp.Popen([str(x) for x in command], stdout=sp.PIPE, stderr=errors, env=env) as child:
            for line in child.stdout:
                record = json.loads(line)
                if record.get('message_type') == 'summary':
                    summaries.append(line)
                elif record.get('message_type') == 'status':
                    counters = {k: record[k] for k in ['percent_done', 'bytes_done', 'total_bytes', 'files_done', 'total_files'] if k in record}
                    write(STATE / 'upload-progress.json', {'at': dt.datetime.now(dt.timezone.utc).isoformat(), **counters})
                    if time.monotonic() - last_print > 60:
                        print('Archive processing: ' + str(counters.get('bytes_done', 0)) + '/' + str(counters.get('total_bytes', 0)) + ' bytes', flush=True)
                        last_print = time.monotonic()
            code = child.wait()
        if code:
            errors.seek(0)
            path = STATE / 'last-upload-error.log'
            path.write_bytes(redact_diagnostics(errors.read()))
            path.chmod(0o600)
            raise RuntimeError(f'restic upload failed (exit {code}); root-only diagnostics saved')
    return b''.join(summaries)



def dbexec(name, command, *, data=None, output=None):
    return run(['docker', 'exec', '-i', name, 'sh', '-ec', command], data=data, output=output)


def dump(name, target):
    partial = Path(str(target) + '.partial')
    try:
        with partial.open('wb') as out:
            dbexec(name, 'PGCONNECT_TIMEOUT=10 pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc', output=out)
        if partial.stat().st_size < 100:
            raise RuntimeError('Database dump is empty')
        with partial.open('rb') as src:
            # Validate archive header without buffering the complete dump in RAM.
            if src.read(5) != b'PGDMP':
                raise RuntimeError('Invalid database archive')
        partial.replace(target)
    finally:
        partial.unlink(missing_ok=True)


def checksum(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda: f.read(1024 * 1024), b''):
            h.update(b)
    return h.hexdigest()


def check_bundle(bundle):
    manifest = json.loads((bundle / 'manifest.json').read_text())
    if manifest['format'] != 1:
        raise RuntimeError('Unsupported snapshot format')
    for relative, digest in manifest['sha256'].items():
        p = bundle / relative
        if not p.resolve().is_relative_to(bundle.resolve()) or not p.is_file() or checksum(p) != digest:
            raise RuntimeError('Snapshot checksum validation failed')
    for relative, target in manifest.get('symlinks', {}).items():
        path = bundle / relative
        if not path.parent.resolve().is_relative_to(bundle.resolve()) or not path.is_symlink() or os.readlink(path) != target:
            raise RuntimeError('Snapshot symlink validation failed')
    return manifest


def status(kind, **extra):
    write(STATE / f'{kind}.json', {'at': dt.datetime.now(dt.timezone.utc).isoformat(), **extra})


def copy_path(source, root):
    dest = root / source.relative_to('/')
    dest.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    if source.is_symlink():
        dest.symlink_to(os.readlink(source))
    elif source.is_dir():
        shutil.copytree(source, dest, symlinks=True, dirs_exist_ok=True)
    else:
        shutil.copy2(source, dest)


def source_paths(directory):
    if not directory.is_relative_to(BASE):
        raise RuntimeError('Unexpected deployment path')
    if directory != BASE:
        return {directory}
    # Legacy containers may have been created before immutable releases existed.
    # Never recurse into BASE: it includes dumps, releases and runtime files.
    names = ['backend', 'frontend', 'data', 'scripts', 'docs', 'compose.yaml',
             'compose.production.yaml', 'VERSION', 'CHANGELOG.md', 'README.md',
             'go.mod', 'go.sum', '.dockerignore', '.release-commit']
    return {BASE / name for name in names if (BASE / name).exists()}


def pin_services(config, captured):
    active = []
    for name, svc in list(config['services'].items()):
        if name not in captured:
            if svc.get('profiles'):
                del config['services'][name]
                continue
            raise RuntimeError('Required service has no captured image')
        c = captured[name]
        svc.pop('build', None)
        svc['image'] = c['Image']
        svc['pull_policy'] = 'never'
        svc['environment'] = dict(item.split('=', 1) for item in c['Config']['Env'])
        svc.pop('profiles', None)
        if c['State']['Running']:
            active.append(name)
    return active


def password_secret_paths(base=None):
    directory = (base if base is not None else BASE) / 'secrets'
    names = ['password-keyring.json', 'password-recovery.json']
    if not directory.exists() and not directory.is_symlink():
        return set()
    if directory.is_symlink() or not directory.is_dir() or directory.stat().st_mode & 0o077 or directory.stat().st_uid != os.geteuid():
        raise RuntimeError('Password secret directory must be private and regular')
    paths = {directory / name for name in names}
    for path in paths:
        try:
            info = path.lstat()
        except OSError:
            raise RuntimeError('Password secret pair is incomplete') from None
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or info.st_uid != os.geteuid() or info.st_size > 16384:
            raise RuntimeError('Password secret file is unsafe')
    return paths


def verify_password_secrets(bundle, image):
    directory = bundle / 'rootfs/srv/devcourse-finder/secrets'
    if not directory.resolve().is_relative_to(bundle.resolve()) or not password_secret_paths(directory.parent):
        raise RuntimeError('Snapshot password secret pair is missing or unsafe')
    run(['docker', 'run', '--rm', '--network', 'none', '--read-only',
         '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges:true',
         '--memory', '384m', '--cpus', '1', '--user', '0:0',
         '--mount', f'type=bind,source={directory},target=/run/password-secrets,readonly',
         image, 'password-keyring', 'verify',
         '/run/password-secrets/password-keyring.json', '/run/password-secrets/password-recovery.json'])


def backup():
    STATE.mkdir(parents=True, exist_ok=True)
    with (BASE / '.deploy.lock').open('a') as deploy:
        fcntl.flock(deploy, fcntl.LOCK_EX)
        bundle = STATE / 'bundle'
        shutil.rmtree(bundle, ignore_errors=True)
        bundle.mkdir(mode=0o700)
        try:
            release = (BASE / 'current').resolve(strict=True) if (BASE / 'current').is_symlink() else BASE
            ids = run(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=devcourse-finder']).decode().split()
            ids += run(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=devcourse-ingress']).decode().split()
            records = js(['docker', 'inspect', *ids])
            by_project = {}
            for c in records:
                labels = c['Config']['Labels']
                by_project.setdefault(labels['com.docker.compose.project'], {})[labels['com.docker.compose.service']] = c
            app = by_project['devcourse-finder']
            ingress = by_project['devcourse-ingress']
            if not app['db']['State']['Running']:
                raise RuntimeError('Production database is not running')
            # Never recursively copy the base directory: it contains old dumps.
            password_paths = password_secret_paths()
            paths = source_paths(release) | {BASE / '.env', BASE / 'DEPLOYED_VERSION', BASE / 'ingress'} | password_paths
            for c in records:
                wd = Path(c['Config']['Labels']['com.docker.compose.project.working_dir'])
                paths |= source_paths(wd)
            binds = set()
            volumes = {}
            for c in records:
                for m in c['Mounts']:
                    if m['Type'] == 'bind':
                        source = Path(m['Source'])
                        if source == BASE or not source.is_relative_to(BASE) or source.is_relative_to(BASE / 'backups') or m['RW']:
                            raise RuntimeError('Unreviewed writable or external bind mount')
                        binds.add(source)
                    elif m['Type'] == 'volume':
                        if c['Id'] == app['db']['Id']:
                            continue  # A live PostgreSQL directory is never a database backup.
                        if c['Id'] != ingress['proxy']['Id']:
                            raise RuntimeError('Unreviewed application volume')
                        volumes[m['Name']] = m['Source']
                    else:
                        raise RuntimeError('Unreviewed mount type')
            paths |= binds
            roots = sorted(p for p in paths if not any(q != p and p.is_relative_to(q) for q in paths))
            rootfs = bundle / 'rootfs'
            for p in roots:
                copy_path(p, rootfs)
            if release != BASE:
                (rootfs / 'srv/devcourse-finder/current').symlink_to(release)
            if (CONFIG / 'email.json').exists():
                copy_path(CONFIG / 'email.json', rootfs)
            shutil.copytree(Path(__file__).parent, bundle / 'tools')
            if password_paths:
                verify_password_secrets(bundle, app['api']['Image'])
            print('Creating consistent PostgreSQL dump', flush=True)
            dump(app['db']['Name'].lstrip('/'), bundle / 'database.dump')
            dbversion = dbexec(app['db']['Name'].lstrip('/'), 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "show server_version"').decode().strip()
            with (bundle / 'globals.sql').open('wb') as out:
                dbexec(app['db']['Name'].lstrip('/'), 'pg_dumpall -U "$POSTGRES_USER" --globals-only', output=out)
            images = sorted({c['Image'] for c in records})
            with (bundle / 'images.tar').open('wb') as out:
                run(['docker', 'image', 'save', *images], output=out)
            for name, source in volumes.items():
                dest = bundle / 'volumes' / (name + '.tar')
                dest.parent.mkdir(exist_ok=True)
                # Caddy's durable certificate store; application files are read-only binds.
                run(['tar', '--numeric-owner', '-cf', dest, '-C', source, '.'])
            active = {}
            for project, services in by_project.items():
                # App Compose must be from the current release; containers may predate it.
                files = [release / 'compose.yaml', release / 'compose.production.yaml'] if project == 'devcourse-finder' else [BASE / 'ingress/compose.yaml']
                cmd = ['docker', 'compose', '--profile', '*', '--project-name', project]
                if project == 'devcourse-finder':
                    cmd += ['--env-file', BASE / '.env']
                for f in files:
                    cmd += ['-f', f]
                config = js([*cmd, 'config', '--format', 'json'])
                active[project] = pin_services(config, services)
                write(bundle / f'{project}.json', config)
            metadata = {'format': 1, 'created_at': dt.datetime.now(dt.timezone.utc).isoformat(),
                        'deployment': (BASE / 'DEPLOYED_VERSION').read_text(), 'postgres': dbversion,
                        'images': images, 'active_services': active, 'root_paths': [str(p) for p in roots],
                        'password_storage': {'format': 1, 'files': sorted(str(p.relative_to('/')) for p in password_paths)} if password_paths else None,
                        'volumes': list(volumes), 'consistency': 'pg_dump MVCC; deployment lock; application binds read-only; Caddy volumes copied separately',
                        'symlinks': {str(p.relative_to(bundle)): os.readlink(p) for p in bundle.rglob('*') if p.is_symlink()},
                        'sha256': {str(p.relative_to(bundle)): checksum(p) for p in sorted(bundle.rglob('*')) if p.is_file() and not p.is_symlink()}}
            write(bundle / 'manifest.json', metadata)
            upload_bundle(bundle)

        except Exception as e:
            status('last-failure', error=str(e))
            raise
        finally:
            shutil.rmtree(bundle, ignore_errors=True)


def upload_bundle(bundle):
    if bundle != STATE / 'bundle':
        raise RuntimeError('Upload staging path must be /var/lib/devcourse-backup/bundle')
    metadata = check_bundle(bundle)
    print('Uploading encrypted snapshot', flush=True)
    report = restic('backup', '--pack-size', '4', '--json', '--host', 'devcourse-finder-production', '--tag', TAG, bundle, progress=True)
    summaries = [json.loads(line) for line in report.splitlines() if line.strip()]
    summary = next(x for x in summaries if x.get('message_type') == 'summary')
    snapshot = summary['snapshot_id']
    restic('forget', '--host', 'devcourse-finder-production', '--tag', TAG, '--path', bundle,
           '--keep-daily', '7', '--keep-weekly', '4', '--keep-monthly', '6')
    status('last-success', snapshot=snapshot, data_created_at=metadata['created_at'], postgres=metadata['postgres'], deployment=metadata['deployment'])
    (STATE / 'last-failure.json').unlink(missing_ok=True)
    print('Backup successful: ' + snapshot, flush=True)
    if (CONFIG / 'email.json').exists():
        run(['python3', Path(__file__).with_name('notify.py'), 'recovered'])


def compose(bundle, project, *args, file=None):
    return run(['docker', 'compose', '--project-name', project, '-f', file or bundle / 'devcourse-finder.json', *args])


def restore_dump(name, source):
    # Stream the archive, do not hold gigabytes in memory.
    with source.open('rb') as f:
        result = sp.run(['docker', 'exec', '-i', name, 'sh', '-ec',
                         'pg_restore --single-transaction --exit-on-error --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],
                        stdin=f, stdout=sp.DEVNULL, stderr=sp.PIPE)
    if result.returncode:
        raise RuntimeError('pg_restore failed; inspect isolated database')


def wait_db(name):
    for _ in range(90):
        try:
            dbexec(name, 'PGPASSWORD="$POSTGRES_PASSWORD" PGCONNECT_TIMEOUT=2 psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -Atc "SELECT 1"')
            return
        except RuntimeError:
            time.sleep(2)
    raise RuntimeError('Database readiness timeout')


def verify(bundle=None, project='devcourse-finder', file=None):
    name = compose(bundle, project, 'ps', '-q', 'db', file=file).decode().strip() if bundle else 'devcourse-finder-db-1'
    wait_db(name)
    result = dbexec(name, 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT count(*) FROM courses"').decode().strip()
    api = compose(bundle, project, 'ps', '-q', 'api', file=file).decode().strip() if bundle else 'devcourse-finder-api-1'
    for _ in range(90):
        try:
            run(['docker', 'exec', api, 'wget', '-Y', 'off', '-qO-', 'http://127.0.0.1:8080/health/ready'])
            break
        except RuntimeError:
            time.sleep(2)
    else:
        raise RuntimeError('Restored API readiness timeout')
    front = compose(bundle, project, 'ps', '-q', 'frontend', file=file).decode().strip() if bundle else 'devcourse-finder-frontend-1'
    for _ in range(90):
        try:
            # Frontend container has Node; this probes both HTML and the real API proxy.
            script = "for(const p of ['/', '/api/v1/courses']){const r=await fetch('http://127.0.0.1:3000'+p);if(!r.ok)throw Error('HTTP');if(p.includes('api')){const j=await r.json();if(!Array.isArray(j.items)||typeof j.total!=='number')throw Error('catalog')}}"
            run(['docker', 'exec', front, 'node', '--input-type=module', '-e', script])
            break
        except RuntimeError:
            time.sleep(2)
    else:
        raise RuntimeError('Restored frontend readiness timeout')
    print('Database, API, frontend and catalog verified; course rows: ' + result, flush=True)
    return result


def verify_ingress(bundle):
    image = json.loads((bundle / 'devcourse-ingress.json').read_text())['services']['proxy']['image']
    ingress = bundle / 'rootfs/srv/devcourse-finder/ingress'
    run(['docker', 'run', '--rm', '--network', 'none', '--entrypoint', 'caddy',
         '--mount', f'type=bind,src={ingress}/Caddyfile,dst=/etc/caddy/Caddyfile,readonly',
         '--mount', f'type=bind,src={ingress}/tls,dst=/etc/caddy/tls,readonly', image,
         'validate', '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile'])
    print('Saved Caddy configuration and TLS files validated', flush=True)


def fetch_snapshot(snapshot, dest):
    if dest.exists():
        raise RuntimeError('Restore destination must not exist')
    dest.mkdir(parents=True, mode=0o700)
    restic('restore', snapshot, '--tag', TAG, '--host', 'devcourse-finder-production', '--target', dest)
    bundle = dest / str(STATE / 'bundle').lstrip('/')
    check_bundle(bundle)
    return bundle


def isolated_config(config):
    # Deep copy: callers may still need the original production config.
    config = json.loads(json.dumps(config))
    config.pop('name', None)
    config['networks'] = {'default': {'internal': True}}
    config['volumes'] = {'postgres-data': {}}
    selected = {}
    for name in ['db', 'api', 'frontend']:
        svc = config['services'][name]
        for key in ['ports', 'container_name', 'depends_on', 'networks', 'volumes', 'restart']:
            svc.pop(key, None)
        svc['restart'] = 'no'
        if name == 'db':
            svc['mem_limit'] = '512m'
            svc['volumes'] = [{'type': 'volume', 'source': 'postgres-data', 'target': '/var/lib/postgresql/data'}]
        selected[name] = svc
    config['services'] = selected
    return config


def drill(snapshot):
    dest = STATE / ('drill-' + secrets.token_hex(4))
    project = 'devcourse-drill-' + secrets.token_hex(4)
    bundle = None
    started = False
    try:
        print('Downloading snapshot for isolated recovery', flush=True)
        bundle = fetch_snapshot(snapshot, dest)
        manifest = check_bundle(bundle)
        run(['docker', 'image', 'load', '-i', bundle / 'images.tar'])
        saved_config = json.loads((bundle / 'devcourse-finder.json').read_text())
        if manifest.get('password_storage'):
            verify_password_secrets(bundle, saved_config['services']['api']['image'])
        config = isolated_config(saved_config)
        file = bundle / 'drill.json'
        write(file, config)
        started = True
        compose(bundle, project, 'up', '-d', '--no-build', '--pull', 'never', 'db', file=file)
        name = compose(bundle, project, 'ps', '-q', 'db', file=file).decode().strip()
        wait_db(name)
        print('Restoring database in an isolated volume', flush=True)
        restore_dump(name, bundle / 'database.dump')
        compose(bundle, project, 'up', '-d', '--no-build', '--pull', 'never', '--no-deps', 'api', 'frontend', file=file)
        rows = verify(bundle, project, file)
        verify_ingress(bundle)
        status('last-drill', snapshot=snapshot, course_rows=rows, deployment=manifest['deployment'])
        print('Isolated restore successful', flush=True)
    finally:
        if started:
            compose(bundle, project, 'down', '--volumes', '--remove-orphans', file=bundle / 'drill.json')
        shutil.rmtree(dest, ignore_errors=True)


def refresh_recovery_kit():
    # Contains repository access/encryption credentials: NEVER include this kit
    # in the restic snapshot. It is an independent password-manager export.
    paths = password_secret_paths()
    descriptor, temporary = tempfile.mkstemp(prefix='recovery-kit-', suffix='.zip', dir=STATE)
    temporary = Path(temporary)
    try:
        with os.fdopen(descriptor, 'wb') as output:
            with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as archive:
                for name in ['config.json', 'rclone.conf', 'password', 'email.json', 'repository-id']:
                    if (CONFIG / name).is_file():
                        archive.write(CONFIG / name, 'etc/devcourse-backup/' + name)
                for path in Path(__file__).parent.rglob('*'):
                    if path.is_file() and '__pycache__' not in path.parts:
                        archive.write(path, 'tools/' + str(path.relative_to(Path(__file__).parent)))
                for path in paths:
                    archive.write(path, str(path.relative_to('/')))
                archive.writestr('READ-ME-FIRST.txt', 'CONFIDENTIAL: backup encryption key, cloud credentials and password pepper keys. Store independently in your password manager. Never upload this archive into the backup repository. Follow tools/README.md and tools/PASSWORD-STORAGE.md.\n')
            output.flush()
            os.fsync(output.fileno())
        temporary.replace(STATE / 'recovery-kit.zip')
    finally:
        temporary.unlink(missing_ok=True)


def password_checkpoint():
    # Separate, small recovery proof. It never replaces the full database backup
    # or updates last-success/freshness. Keep its tag outside full-snapshot retention.
    with (BASE / '.deploy.lock').open('a') as deploy:
        fcntl.flock(deploy, fcntl.LOCK_EX)
        paths = password_secret_paths()
        if not paths:
            raise RuntimeError('Password secret pair is not configured')
        stage = STATE / 'password-checkpoint'
        shutil.rmtree(stage, ignore_errors=True)
        stage.mkdir(mode=0o700)
        dest = Path(tempfile.mkdtemp(prefix='password-restore-', dir=STATE))
        try:
            for path in paths:
                copy_path(path, stage / 'rootfs')
            api = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=devcourse-finder',
                       '--filter', 'label=com.docker.compose.service=api']).decode().strip()
            if not api or '\n' in api:
                raise RuntimeError('Exactly one production API is required')
            run(['docker', 'cp', f'{api}:/usr/local/bin/devcourse-finder', stage / 'verifier'])
            (stage / 'verifier').chmod(0o700)
            pair = stage / 'rootfs' / str(BASE).lstrip('/') / 'secrets'
            run([stage / 'verifier', 'password-keyring', 'verify', pair / 'password-keyring.json', pair / 'password-recovery.json'])
            metadata = {'format': 1, 'kind': 'password-keys-only-not-a-database-backup',
                        'deployment': (BASE / 'DEPLOYED_VERSION').read_text(),
                        'sha256': {str(p.relative_to(stage)): checksum(p) for p in stage.rglob('*') if p.is_file()},
                        'created_at': dt.datetime.now(dt.timezone.utc).isoformat()}
            write(stage / 'manifest.json', metadata)
            result = restic('backup', '--json', '--host', 'devcourse-finder-production',
                            '--tag', 'devcourse-password-keys-v1', stage)
            summaries = [json.loads(line) for line in result.splitlines() if line.strip()]
            snapshot = next((x['snapshot_id'] for x in summaries if x.get('message_type') == 'summary'), None)
            if not snapshot:
                raise RuntimeError('Password checkpoint snapshot ID unavailable')
            restic('restore', snapshot, '--target', dest, '--tag', 'devcourse-password-keys-v1', '--host', 'devcourse-finder-production')
            restored = dest / str(stage).lstrip('/')
            check_bundle(restored)
            pair = restored / 'rootfs' / str(BASE).lstrip('/') / 'secrets'
            if not password_secret_paths(pair.parent):
                raise RuntimeError('Restored password secret pair missing')
            run([restored / 'verifier', 'password-keyring', 'verify', pair / 'password-keyring.json', pair / 'password-recovery.json'])
            refresh_recovery_kit()
            status('last-password-checkpoint', snapshot=snapshot, deployment=metadata['deployment'])
            print('Encrypted password-key checkpoint restored and verified; full database backup remains separate', flush=True)
        finally:
            shutil.rmtree(stage, ignore_errors=True)
            shutil.rmtree(dest, ignore_errors=True)


def restore_db(source, confirmation):
    if confirmation != 'REPLACE devcourse-finder':
        raise RuntimeError('Explicit --confirm "REPLACE devcourse-finder" is required')
    ids = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=devcourse-finder']).decode().split()
    records = js(['docker', 'inspect', *ids])
    db = next(c for c in records if c['Config']['Labels']['com.docker.compose.service'] == 'db')
    others = [c['Id'] for c in records if c['Id'] != db['Id']]
    with source.open('rb') as archive:
        result = sp.run(['docker', 'exec', '-i', db['Id'], 'pg_restore', '--list'], stdin=archive, stdout=sp.DEVNULL, stderr=sp.PIPE)
    if result.returncode:
        raise RuntimeError('Chosen archive is invalid or incompatible')
    # Stop writers before taking the safety dump; keep them stopped if recovery fails.
    if others:
        run(['docker', 'stop', *others])
    safety = BASE / 'backups' / ('before-restore-' + dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '.dump')
    safety.parent.mkdir(exist_ok=True)
    dump(db['Id'], safety)
    dbexec(db['Id'], 'dropdb --force -U "$POSTGRES_USER" "$POSTGRES_DB"; createdb -U "$POSTGRES_USER" -O "$POSTGRES_USER" "$POSTGRES_DB"')
    restore_dump(db['Id'], source)
    if others:
        run(['docker', 'start', *others])
    print('Database restored; safety dump: ' + str(safety))


def restore(bundle):
    manifest = check_bundle(bundle)
    if BASE.exists() or run(['docker', 'ps', '-aq']).strip():
        raise RuntimeError('Full restore requires a clean server with no application directory or containers')
    existing_volumes = set(run(['docker', 'volume', 'ls', '-q']).decode().split())
    if existing_volumes.intersection(set(manifest['volumes']) | {'devcourse-finder_postgres-data'}):
        raise RuntimeError('Recovery volumes already exist; refusing to overwrite')
    # Install Docker with bootstrap.sh first. No migrations or builds during recovery.
    run(['docker', 'image', 'load', '-i', bundle / 'images.tar'])
    if manifest.get('password_storage'):
        config = json.loads((bundle / 'devcourse-finder.json').read_text())
        verify_password_secrets(bundle, config['services']['api']['image'])
    rootfs = bundle / 'rootfs'
    if not (rootfs / 'srv/devcourse-finder/.env').is_file():
        raise RuntimeError('Missing production configuration')
    shutil.copytree(rootfs / 'srv/devcourse-finder', BASE, symlinks=True)
    if (rootfs / 'etc/devcourse-backup/email.json').exists():
        CONFIG.mkdir(mode=0o700, exist_ok=True)
        shutil.copy2(rootfs / 'etc/devcourse-backup/email.json', CONFIG / 'email.json')
    for name in manifest['volumes']:
        run(['docker', 'volume', 'create', name])
        mount = js(['docker', 'volume', 'inspect', name])[0]['Mountpoint']
        with tarfile.open(bundle / 'volumes' / (name + '.tar')) as archive:
            archive.extractall(mount, filter='data')
    # Persist resolved, pinned Compose next to the recovered installation.
    recovery = BASE / 'recovery'
    recovery.mkdir(mode=0o700)
    for project in ['devcourse-finder', 'devcourse-ingress']:
        shutil.copy2(bundle / f'{project}.json', recovery / f'{project}.json')
    appfile = recovery / 'devcourse-finder.json'
    compose(bundle, 'devcourse-finder', 'up', '-d', '--no-build', '--pull', 'never', 'db', file=appfile)
    name = compose(bundle, 'devcourse-finder', 'ps', '-q', 'db', file=appfile).decode().strip()
    wait_db(name)
    restore_dump(name, bundle / 'database.dump')
    active = [n for n in manifest['active_services']['devcourse-finder'] if n != 'db']
    compose(bundle, 'devcourse-finder', 'up', '-d', '--no-build', '--pull', 'never', '--no-deps', *active, file=appfile)
    verify(bundle, file=appfile)
    compose(bundle, 'devcourse-ingress', 'up', '-d', '--no-build', '--pull', 'never', '--no-deps', *manifest['active_services']['devcourse-ingress'], file=recovery / 'devcourse-ingress.json')
    print('Full recovery complete. Verify public DNS/HTTPS before routing users here.')


def main():
    os.umask(0o077)
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['backup', 'upload', 'drill', 'restore', 'restore-db', 'verify', 'status', 'check', 'prune', 'password-checkpoint'])
    p.add_argument('source', nargs='?', default='latest')
    p.add_argument('--confirm', default='')
    a = p.parse_args()
    if os.geteuid() != 0:
        p.error('Run as root')
    STATE.mkdir(parents=True, exist_ok=True)
    if a.action == 'status':
        path = STATE / 'last-success.json'
        if not path.exists(): raise RuntimeError('No successful external backup')
        state = json.loads(path.read_text())
        print(json.dumps(state))
        age = dt.datetime.now(dt.timezone.utc) - dt.datetime.fromisoformat(state.get('data_created_at', state['at']))
        if age.total_seconds() > 30 * 3600 or (STATE / 'last-failure.json').exists():
            raise RuntimeError('Backup is stale or last operation failed')
        return
    if a.action == 'verify':
        verify()
        return
    with (STATE / 'operation.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print('Waiting for the active backup/recovery operation', flush=True)
            fcntl.flock(lock, fcntl.LOCK_EX)
        if a.action == 'backup': backup()
        elif a.action == 'password-checkpoint': password_checkpoint()
        elif a.action == 'upload': upload_bundle(Path(a.source).resolve(strict=True))
        elif a.action == 'drill': drill(a.source)
        elif a.action == 'restore': restore(Path(a.source).resolve())
        elif a.action == 'restore-db':
            with (BASE / '.deploy.lock').open('a') as deploy:
                fcntl.flock(deploy, fcntl.LOCK_EX)
                restore_db(Path(a.source).resolve(strict=True), a.confirm)
        elif a.action == 'check': restic('check', '--read-data-subset=10%'); print('Repository check passed')
        elif a.action == 'prune': restic('prune'); print('Repository prune passed')



if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('Recovery operation failed: ' + str(error), file=sys.stderr)
        sys.exit(1)
