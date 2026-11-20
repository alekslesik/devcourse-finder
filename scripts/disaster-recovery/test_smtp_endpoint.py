import importlib.util
import json
import os
from pathlib import Path
import secrets
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))
from smtp_endpoint import endpoint_from_environment
spec = importlib.util.spec_from_file_location('configure_email', Path(__file__).with_name('configure-email.py'))
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)


class SMTPConfigurationTests(unittest.TestCase):
    def test_existing_default_and_explicit_domain_endpoint(self):
        with patch.dict(os.environ, {}, clear=True):
            self.assertEqual(endpoint_from_environment('AUTH_SMTP'), ('smtp.yandex.ru', 465))
        for port in ('465', '587'):
            with patch.dict(os.environ, {'AUTH_SMTP_HOST': 'smtp.hosting.example', 'AUTH_SMTP_PORT': port}, clear=True):
                self.assertEqual(endpoint_from_environment('AUTH_SMTP'), ('smtp.hosting.example', int(port)))

    def test_plaintext_and_noncanonical_endpoints_fail_without_echoing_values(self):
        for host in ['', 'localhost', '127.0.0.1', '::1', 'https://mail.example', 'mail.example:465', 'MAIL.example', 'mail.example.', 'mail..example', '-mail.example', 'mail-.example', 'mail.example\r\n', 'a'*64+'.example']:
            with self.subTest(host=host), patch.dict(os.environ, {'AUTH_SMTP_HOST': host}, clear=True):
                with self.assertRaises(RuntimeError):
                    endpoint_from_environment('AUTH_SMTP')
        for port in ('25', '2525', '0465', '465\n', '587 extra', ''):
            with self.subTest(port=port), patch.dict(os.environ, {'AUTH_SMTP_PORT': port}, clear=True):
                with self.assertRaises(RuntimeError):
                    endpoint_from_environment('AUTH_SMTP')

    def test_backup_configuration_enforces_tls_and_rolls_back_failed_delivery(self):
        for port, success in [('465', True), ('587', True), ('465', False)]:
            with self.subTest(port=port, success=success), tempfile.TemporaryDirectory() as directory:
                config = Path(directory) / 'email.json'
                previous = b'{"previous":"synthetic"}'
                config.write_bytes(previous); config.chmod(0o600)
                value = secrets.token_urlsafe(24)
                env = {'BACKUP_SMTP_HOST': 'smtp.hosting.example', 'BACKUP_SMTP_PORT': port, 'BACKUP_SMTP_USER': 'noreply@example.test', 'BACKUP_SMTP_PASSWORD': value}
                def notify(command):
                    candidate = json.loads(config.read_text())
                    self.assertEqual(candidate['host'], 'smtp.hosting.example')
                    self.assertEqual(candidate['port'], int(port))
                    self.assertEqual(candidate['ssl'], port == '465')
                    self.assertEqual(candidate['password'], value)
                    self.assertEqual(config.stat().st_mode & 0o777, 0o600)
                    self.assertNotIn(value, ' '.join(map(str, command)))
                    return type('Result', (), {'returncode': 0 if success else 1})()
                with patch.object(p, 'CONFIG_FILE', config), patch.object(p.os, 'geteuid', return_value=0), patch.object(p.os, 'umask'), patch.dict(os.environ, env, clear=True), patch.object(p.subprocess, 'run', side_effect=notify), patch.object(sys, 'argv', ['configure-email', '--to', 'operator@example.test']):
                    if success:
                        p.main()
                        self.assertEqual(json.loads(config.read_text())['to'], 'operator@example.test')
                    else:
                        with self.assertRaisesRegex(RuntimeError, 'previous configuration preserved'):
                            p.main()
                        self.assertEqual(config.read_bytes(), previous)
