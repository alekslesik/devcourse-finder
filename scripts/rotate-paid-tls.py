#!/usr/bin/env python3
"""Validate and optionally install a renewed paid GlobalSign pair on the Caddy VDS."""
import argparse
from datetime import datetime, timezone
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import ssl
import stat
import subprocess
import sys
import tempfile


def run(*args):
    result = subprocess.run(args, capture_output=True, timeout=45)
    if result.returncode:
        # OpenSSL/Caddy diagnostics may contain configuration paths. Never echo
        # subprocess output or any private key material to logs or artifacts.
        raise RuntimeError('Command failed: ' + args[0])
    return result.stdout


def protected_file(path, private=False):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_size > 262144 or info.st_mode & 0o022:
        raise ValueError('Inputs must be root-owned regular files below 256 KiB')
    if private and info.st_mode & 0o077:
        raise ValueError('The supplied private key must have mode 0600 or stricter')
    return path.read_bytes()


def leaf_metadata(path):
    binary = run('openssl', 'x509', '-in', str(path), '-outform', 'DER')
    expires = run('openssl', 'x509', '-in', str(path), '-noout', '-enddate').decode().strip().split('=', 1)[1]
    return {'sha256': hashlib.sha256(binary).hexdigest(),
            'not_after': datetime.fromtimestamp(ssl.cert_time_to_seconds(expires), timezone.utc).isoformat()}


def validate_pair(chain, key, host, directory, minimum_days=30):
    blocks = re.findall(rb'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', chain, re.S)
    if len(blocks) < 2 or b'PRIVATE KEY' in chain:
        raise ValueError('A leaf certificate and its intermediate chain are required')
    leaf = directory / 'leaf.pem'
    intermediates = directory / 'intermediates.pem'
    key_path = directory / 'key.pem'
    leaf.write_bytes(blocks[0] + b'\n')
    intermediates.write_bytes(b'\n'.join(blocks[1:]) + b'\n')
    key_path.write_bytes(key)
    key_path.chmod(0o600)
    run('openssl', 'verify', '-purpose', 'sslserver', '-verify_hostname', host,
        '-untrusted', str(intermediates), str(leaf))
    run('openssl', 'x509', '-in', str(leaf), '-noout', '-checkend', str(minimum_days * 86400))
    issuer = run('openssl', 'x509', '-in', str(leaf), '-noout', '-issuer').decode()
    if 'GlobalSign' not in issuer:
        raise ValueError('Expected the retained GlobalSign issuer')
    cert_public = run('openssl', 'x509', '-in', str(leaf), '-noout', '-pubkey')
    key_public = run('openssl', 'pkey', '-in', str(key_path), '-pubout', '-passin', 'pass:')
    if cert_public.strip() != key_public.strip():
        raise ValueError('Certificate and private key do not match')
    return leaf_metadata(leaf)


def atomic_write(path, data, mode=0o600):
    descriptor, temporary = tempfile.mkstemp(prefix='.tls-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'wb') as target:
            os.fchmod(target.fileno(), mode)
            target.write(data)
            target.flush()
            os.fsync(target.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def activate(directory):
    command = ['docker', 'compose', '--project-directory', str(directory), 'exec', '-T', 'proxy', 'caddy']
    for action in ('validate', 'reload'):
        run(*command, action, '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile')


def verify_served(host, expected):
    spec = importlib.util.spec_from_file_location('tls_expiry', Path(__file__).with_name('check-tls-expiry.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    result = module.inspect_certificate(host, connect_address='127.0.0.1')
    if result['status'] != 'ok' or result.get('sha256') != expected:
        raise RuntimeError('Caddy did not serve the expected trusted certificate')
    return result


def install_pair(ingress, tls, chain, key, metadata, host):
    current_chain, current_key = tls / 'fullchain.pem', tls / 'privkey.key'
    old_chain, old_key = protected_file(current_chain), protected_file(current_key, private=True)
    old = leaf_metadata(current_chain)
    if old['sha256'] == metadata['sha256']:
        return {'action': 'already_installed', 'certificate': verify_served(host, old['sha256'])}
    if metadata['not_after'] <= old['not_after']:
        raise ValueError('A renewal must extend the installed certificate validity')
    backup = Path(tempfile.mkdtemp(prefix='rollback-', dir=tls))
    backup.chmod(0o700)
    atomic_write(backup / 'fullchain.pem', old_chain)
    atomic_write(backup / 'privkey.key', old_key)
    try:
        atomic_write(current_chain, chain)
        atomic_write(current_key, key)
        activate(ingress)
        served = verify_served(host, metadata['sha256'])
    except Exception as failure:
        atomic_write(current_chain, old_chain)
        atomic_write(current_key, old_key)
        try:
            activate(ingress)
            verify_served(host, old['sha256'])
        except Exception as rollback:
            raise RuntimeError('Rotation failed and rollback requires operator attention; protected backup: ' + str(backup)) from rollback
        raise RuntimeError('Rotation failed; the previous trusted pair was restored') from failure
    return {'action': 'installed', 'certificate': served, 'rollback_directory': str(backup)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fullchain', required=True, type=Path)
    parser.add_argument('--key', required=True, type=Path)
    parser.add_argument('--ingress', type=Path, default=Path('/srv/devcourse-finder/ingress'))
    parser.add_argument('--host', default='devcoursefinder.ru')
    parser.add_argument('--apply', action='store_true', help='Install after validation; default is validation only')
    args = parser.parse_args()
    try:
        if os.geteuid() != 0:
            raise ValueError('Run on the VDS as root')
        if args.host != 'devcoursefinder.ru':
            raise ValueError('Only the approved production hostname is supported')
        ingress = args.ingress.resolve(strict=True)
        tls = ingress / 'tls' / args.host
        info = tls.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
            raise ValueError('TLS directory must be root-owned with mode 0700')
        chain, key = protected_file(args.fullchain), protected_file(args.key, private=True)
        # Same lock as release deployments; retain it through validation/reload.
        with (ingress.parent / '.deploy.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with tempfile.TemporaryDirectory(prefix='.verify-', dir=tls) as temporary:
                metadata = validate_pair(chain, key, args.host, Path(temporary))
            result = install_pair(ingress, tls, chain, key, metadata, args.host) if args.apply else {'action': 'validated_only', 'certificate': metadata}
        print(json.dumps(result, indent=2))
        return 0
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired):
        print(json.dumps({'status': 'error', 'message': 'Certificate validation or rotation failed; inspect protected local state. No key material logged.'}), file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
