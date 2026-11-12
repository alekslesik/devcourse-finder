import importlib.util
from datetime import datetime, timedelta, timezone
from pathlib import Path
import ssl
import unittest
from unittest.mock import MagicMock, patch

spec = importlib.util.spec_from_file_location('tls_expiry', Path(__file__).with_name('check-tls-expiry.py'))
monitor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(monitor)


class FakeGitHub(monitor.GitHub):
    def __init__(self, issues=()):
        super().__init__('owner/repo', 'test-only')
        self.issues = list(issues)
        self.writes = []

    def request(self, method, path, data=None):
        if method == 'GET':
            return self.issues if 'page=1' in path else []
        self.writes.append((method, path, data))
        if method == 'POST' and path == '/issues':
            issue = dict(data, number=900, user={'login': monitor.BOT}, state='open')
            self.issues.append(issue)
            return issue
        if method == 'PATCH':
            issue = next(i for i in self.issues if str(i['number']) == path.split('/')[-1])
            issue.update(data)
            return issue
        return {}


class CertificateTests(unittest.TestCase):
    now = datetime(2027, 1, 1, tzinfo=timezone.utc)

    def inspect(self, days, connect_address=None):
        expires = self.now + timedelta(days=days)
        certificate = {'notAfter': expires.strftime('%b %d %H:%M:%S %Y GMT'),
                       'issuer': ((('organizationName', 'Test CA'),),)}
        secure = MagicMock()
        secure.getpeercert.side_effect = lambda binary_form=False: b'origin-certificate' if binary_form else certificate
        context = MagicMock()
        context.wrap_socket.return_value.__enter__.return_value = secure
        with patch.object(monitor.ssl, 'create_default_context', return_value=context), \
             patch.object(monitor.socket, 'create_connection') as connect:
            result = monitor.inspect_certificate('devcoursefinder.ru', connect_address=connect_address, now=self.now)
            self.assertEqual(connect.call_args.args[0], (connect_address or 'devcoursefinder.ru', 443))
            self.assertEqual(context.wrap_socket.call_args.kwargs['server_hostname'], 'devcoursefinder.ru')
            return result

    def test_renewal_boundaries_and_origin_fingerprint(self):
        for days, status in [(46, 'ok'), (45, 'warning'), (15, 'warning'), (14, 'critical'), (0, 'critical')]:
            with self.subTest(days=days):
                report = self.inspect(days)
                self.assertEqual(report['status'], status)
                self.assertEqual(report['days_remaining'], days)
                self.assertEqual(len(report['sha256']), 64)
                self.assertEqual(report['issuer'], 'Test CA')

    def test_loopback_keeps_public_hostname_verification(self):
        self.assertEqual(self.inspect(100, '127.0.0.1')['status'], 'ok')
        with self.assertRaises(ValueError):
            monitor.inspect_certificate('devcoursefinder.ru', connect_address='10.0.0.1')

    def test_untrusted_certificate_and_timeout_are_failures(self):
        for failure in [ssl.SSLCertVerificationError('untrusted'), OSError('timeout')]:
            with self.subTest(failure=type(failure).__name__), \
                 patch.object(monitor.socket, 'create_connection', side_effect=failure):
                report = monitor.inspect_certificate('devcoursefinder.ru')
                self.assertEqual(report['status'], 'error')
                self.assertNotIn('not_after', report)

    def test_invalid_host_and_thresholds_never_connect(self):
        for host, warning, critical in [('https://example.com', 45, 14), ('localhost', 45, 14),
                                        ('example.com', 0, 14), ('example.com', 14, 14)]:
            with self.subTest(host=host, warning=warning), self.assertRaises(ValueError):
                monitor.inspect_certificate(host, warning, critical)


class NotificationTests(unittest.TestCase):
    def report(self, status):
        return {'host': 'devcoursefinder.ru', 'status': status, 'not_after': '2027-05-25T13:34:52+00:00',
                'days_remaining': 200 if status == 'ok' else 12, 'warning_days': 45}

    def test_warning_is_deduplicated_escalated_and_closed_on_recovery(self):
        github = FakeGitHub()
        self.assertEqual(monitor.notify(self.report('warning'), github)['action'], 'created')
        writes = len(github.writes)
        self.assertEqual(monitor.notify(self.report('warning'), github)['action'], 'already_open')
        self.assertEqual(len(github.writes), writes)
        monitor.notify(self.report('critical'), github)
        self.assertEqual(len(github.issues), 1)
        self.assertIn('<!-- status:critical -->', github.issues[0]['body'])
        self.assertEqual(monitor.notify(self.report('ok'), github)['action'], 'recovered')
        self.assertEqual(github.issues[0]['state'], 'closed')

    def test_healthy_run_never_creates_issue_or_edits_unrelated_issue(self):
        report = self.report('ok')
        issue = {'number': 186, 'state': 'open', 'body': monitor.notification_body(self.report('warning')),
                 'user': {'login': 'owner'}}
        github = FakeGitHub([issue])
        self.assertEqual(monitor.notify(report, github)['action'], 'healthy')
        self.assertEqual(github.writes, [])
        self.assertEqual(issue['state'], 'open')

    def test_connection_failure_opens_an_attention_issue(self):
        report = self.report('error')
        report['error'] = 'certificate_validation_failed'
        github = FakeGitHub()
        monitor.notify(report, github)
        self.assertIn('certificate_validation_failed', github.issues[0]['body'])


if __name__ == '__main__':
    unittest.main()
