#!/usr/bin/env python3
"""Verify the origin certificate and report/track renewal warnings."""
import argparse
from datetime import datetime, timezone
import hashlib
import ipaddress
import json
import math
import os
from pathlib import Path
import re
import socket
import ssl
import sys
import urllib.error
import urllib.request

MARKER = '<!-- devcourse-tls-monitor:v1 -->'
BOT = 'github-actions[bot]'


def hostname(value):
    if not re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}', value):
        raise ValueError('Expected a lowercase DNS hostname')
    return value


def inspect_certificate(host, warning_days=45, critical_days=14, *, connect_address=None, now=None):
    hostname(host)
    if not 0 < critical_days < warning_days <= 365:
        raise ValueError('Expected 0 < critical days < warning days <= 365')
    if connect_address and not ipaddress.ip_address(connect_address).is_loopback:
        raise ValueError('An explicit connection address must be loopback')
    now = now or datetime.now(timezone.utc)
    result = {'host': host, 'checked_at': now.isoformat(), 'warning_days': warning_days,
              'critical_days': critical_days, 'status': 'error'}
    try:
        context = ssl.create_default_context()
        with socket.create_connection((connect_address or host, 443), timeout=15) as connection:
            with context.wrap_socket(connection, server_hostname=host) as secure:
                certificate = secure.getpeercert()
                binary = secure.getpeercert(binary_form=True)
        expires = datetime.fromtimestamp(ssl.cert_time_to_seconds(certificate['notAfter']), timezone.utc)
        remaining = (expires - now).total_seconds() / 86400
        status = 'critical' if remaining <= critical_days else 'warning' if remaining <= warning_days else 'ok'
        result.update(status=status, not_after=expires.isoformat(), days_remaining=math.floor(remaining),
                      sha256=hashlib.sha256(binary).hexdigest(),
                      issuer=', '.join(value for group in certificate.get('issuer', ()) for key, value in group if key in ('organizationName', 'commonName')))
    except ssl.SSLCertVerificationError:
        result['error'] = 'certificate_validation_failed'
    except (OSError, ssl.SSLError, KeyError, ValueError):
        result['error'] = 'tls_connection_or_certificate_failed'
    return result


class GitHub:
    def __init__(self, repository, token):
        if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repository) or not token:
            raise ValueError('Notification requires GITHUB_REPOSITORY and GITHUB_TOKEN')
        self.base = 'https://api.github.com/repos/' + repository
        self.token = token

    def request(self, method, path, data=None):
        body = None if data is None else json.dumps(data).encode()
        request = urllib.request.Request(self.base + path, data=body, method=method, headers={
            'Authorization': 'Bearer ' + self.token, 'Accept': 'application/vnd.github+json',
            'Content-Type': 'application/json', 'X-GitHub-Api-Version': '2022-11-28',
            'User-Agent': 'DevCourseFinder-TLS-Monitor'})
        with urllib.request.urlopen(request, timeout=25) as response:
            return json.load(response)

    def owned_issues(self, host):
        found = []
        for page in range(1, 11):
            rows = self.request('GET', f'/issues?state=all&per_page=100&sort=updated&direction=desc&page={page}')
            for issue in rows:
                body = issue.get('body') or ''
                if (issue.get('user', {}).get('login') == BOT and not issue.get('pull_request')
                        and MARKER in body and f'<!-- host:{host} -->' in body):
                    found.append(issue)
            if len(rows) < 100:
                return found
        raise RuntimeError('Issue search exceeded its bound; refusing a duplicate notification')


def notification_body(report):
    return (f'{MARKER}\n<!-- host:{report["host"]} -->\n<!-- status:{report["status"]} -->\n\n'
            f'Проверка TLS для **{report["host"]}** требует внимания.\n\n'
            f'- Статус: `{report["status"]}`.\n'
            f'- Сертификат действителен до: `{report.get("not_after", "не подтверждено")}`.\n'
            f'- Осталось полных дней: `{report.get("days_remaining", "не подтверждено")}`.\n'
            f'- Результат проверки: `{report.get("error", "достигнут порог обновления")}`.\n\n'
            'Проверить выпуск/автопродление сертификата и его установку на текущем сервере. '
            'Повторный успешный запуск монитора закроет это уведомление автоматически.\n')


def notify(report, github):
    issues = github.owned_issues(report['host'])
    opened = [issue for issue in issues if issue['state'] == 'open']
    if report['status'] == 'ok':
        for issue in opened:
            github.request('POST', f'/issues/{issue["number"]}/comments', {
                'body': f'Проверка восстановилась. TLS доверенный, срок сертификата до `{report["not_after"]}`; до истечения больше {report["warning_days"]} дней.'})
            github.request('PATCH', f'/issues/{issue["number"]}', {'state': 'closed', 'state_reason': 'completed'})
        return {'action': 'recovered' if opened else 'healthy', 'issues': [issue['number'] for issue in opened]}
    body = notification_body(report)
    if opened:
        issue = opened[0]
        if f'<!-- status:{report["status"]} -->' not in (issue.get('body') or ''):
            github.request('PATCH', f'/issues/{issue["number"]}', {'body': body})
            github.request('POST', f'/issues/{issue["number"]}/comments', {
                'body': f'Статус проверки изменился на `{report["status"]}`. См. актуальный отчёт в описании.'})
        return {'action': 'already_open', 'issues': [issue['number']]}
    issue = github.request('POST', '/issues', {'title': f'TLS: требуется внимание к сертификату {report["host"]}', 'body': body})
    return {'action': 'created', 'issues': [issue['number']]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', default='devcoursefinder.ru')
    parser.add_argument('--warning-days', type=int, default=45)
    parser.add_argument('--critical-days', type=int, default=14)
    parser.add_argument('--connect-address', help='Loopback only, for verifying the origin locally on its server')
    parser.add_argument('--output', type=Path)
    parser.add_argument('--notify', action='store_true', help='Track warnings in this GitHub repository using the Actions token')
    args = parser.parse_args()
    try:
        report = inspect_certificate(args.host, args.warning_days, args.critical_days, connect_address=args.connect_address)
        if args.notify:
            try:
                report['notification'] = notify(report, GitHub(os.environ.get('GITHUB_REPOSITORY', ''), os.environ.get('GITHUB_TOKEN', '')))
            except (OSError, ValueError, RuntimeError) as error:
                report['notification_error'] = type(error).__name__
        text = json.dumps(report, ensure_ascii=False, indent=2)
        print(text)
        if args.output:
            args.output.write_text(text + '\n')
        return 0 if report['status'] == 'ok' and 'notification_error' not in report else 1
    except ValueError as error:
        parser.error(str(error))


if __name__ == '__main__':
    sys.exit(main())
