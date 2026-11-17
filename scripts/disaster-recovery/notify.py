#!/usr/bin/env python3
"""SMTP alerts. Credentials live in /etc/devcourse-backup/email.json, never Git."""
from email.message import EmailMessage
from pathlib import Path
import datetime as dt
import json
import os
import smtplib
import ssl
import sys


def send(mode):
    os.umask(0o077)
    state = Path('/var/lib/devcourse-backup')
    config = Path('/etc/devcourse-backup/email.json')
    if not config.exists():
        raise RuntimeError('Email delivery is not configured')
    cfg = json.loads(config.read_text())
    marker = state / 'email-alert.json'
    now = dt.datetime.now(dt.timezone.utc)
    if mode == 'recovered' and not marker.exists():
        return
    if mode == 'failure' and marker.exists():
        old = dt.datetime.fromisoformat(json.loads(marker.read_text())['at'])
        if (now - old).total_seconds() < 12 * 3600:
            return
    message = EmailMessage()
    message['From'] = cfg['from']
    message['To'] = cfg['to']
    message['Subject'] = {'test': '[devcourse-finder] Backup email test',
                          'failure': '[devcourse-finder] BACKUP FAILURE: action required',
                          'recovered': '[devcourse-finder] External backups recovered'}[mode]
    text = ['Backup notification at ' + now.isoformat(), 'Event: ' + mode]
    path = state / 'last-success.json'
    if path.exists():
        result = json.loads(path.read_text())
        text += ['Last successful external backup: ' + result['at'], 'Snapshot: ' + result['snapshot']]
    text += ['Check systemctl --failed and journalctl -u devcourse-full-backup.service',
             'Repository: Yandex Disk backups/devcourse-finder. No credentials are included.']
    message.set_content('\n'.join(text))
    cls = smtplib.SMTP_SSL if cfg.get('ssl', True) else smtplib.SMTP
    kwargs = {'timeout': 30}
    if cfg.get('ssl', True): kwargs['context'] = ssl.create_default_context()
    with cls(cfg['host'], int(cfg.get('port', 465)), **kwargs) as client:
        if not cfg.get('ssl', True): client.starttls(context=ssl.create_default_context())
        if cfg.get('user'): client.login(cfg['user'], cfg['password'])
        client.send_message(message)
    if mode == 'failure':
        marker.write_text(json.dumps({'at': now.isoformat()}))
        marker.chmod(0o600)
    if mode == 'recovered': marker.unlink(missing_ok=True)
    print('Backup email accepted by SMTP server')


if __name__ == '__main__':
    try:
        send(sys.argv[1])
    except Exception as e:
        # SMTP error text can contain addresses or authentication details.
        print('Backup email failed: ' + type(e).__name__, file=sys.stderr)
        sys.exit(1)
