#!/usr/bin/env python3
"""Configure verified TLS SMTP without exposing a password in argv/history/output."""
import argparse
import getpass
import json
import os
from pathlib import Path
import subprocess
import sys
from smtp_endpoint import endpoint_from_environment

CONFIG_FILE = Path('/etc/devcourse-backup/email.json')


def main():
    if os.geteuid() != 0:
        raise SystemExit('Run as root')
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--to', required=True)
    args = parser.parse_args()
    host, port = endpoint_from_environment('BACKUP_SMTP')
    login_address = os.environ.get('BACKUP_SMTP_USER') or input('SMTP sender email: ').strip()
    if '@' not in login_address or '@' not in args.to or any(c in login_address + args.to for c in '\r\n'):
        raise SystemExit('Provide complete sender and recipient email addresses')
    mail_secret = os.environ.get('BACKUP_SMTP_PASSWORD') or getpass.getpass()
    if not mail_secret:
        raise SystemExit('Empty SMTP credential')
    config = CONFIG_FILE
    config.parent.mkdir(mode=0o700, exist_ok=True)
    previous = config.read_bytes() if config.exists() else None
    try:
        config.write_text(json.dumps({'host': host, 'port': port, 'ssl': port == 465,
                                      'user': login_address, 'password': mail_secret, 'from': login_address, 'to': args.to}))
        config.chmod(0o600)
        result = subprocess.run(['python3', Path(__file__).with_name('notify.py'), 'test'])
        if result.returncode:
            raise RuntimeError('SMTP test failed; previous configuration preserved')
    except BaseException:
        if previous is None: config.unlink(missing_ok=True)
        else: config.write_bytes(previous)
        raise
    print('SMTP test accepted; check delivery in recipient inbox/spam.')


if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        print('Email configuration failed: ' + type(e).__name__, file=sys.stderr)
        sys.exit(1)
