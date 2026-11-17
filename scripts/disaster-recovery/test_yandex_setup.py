import configparser
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch, Mock

spec=importlib.util.spec_from_file_location('setup_yandex',Path(__file__).with_name('configure-yandex-api.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class YandexSetupTests(unittest.TestCase):
    def test_rejected_api_never_changes_active_credentials_and_saves_candidate(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            active=b'[yandex]\ntype = webdav\n'
            (root/'rclone.conf').write_bytes(active)
            (root/'config.json').write_text(json.dumps({'repository':'rclone:yandex:backups/devcourse-finder'}))
            previous=(root/'config.json').read_bytes()
            before=os.umask(0o077)
            try:
                with patch.object(m,'C',root),patch.object(m.os,'geteuid',return_value=0),patch.object(m.sys,'argv',['setup']),patch.object(m.getpass,'getpass',return_value='example-token'),patch.object(m,'call',return_value=b'{"id":"expected-repo"}'),patch.object(m,'check_api',side_effect=RuntimeError('API metadata: HTTP 403')),patch.object(m.sp,'run') as process:
                    with self.assertRaisesRegex(RuntimeError,'403'):m.main()
                self.assertEqual((root/'rclone.conf').read_bytes(),active)
                self.assertEqual((root/'config.json').read_bytes(),previous)
                candidate=root/'rclone-api-candidate.conf'
                self.assertEqual(candidate.stat().st_mode & 0o777,0o600)
                p=configparser.ConfigParser(interpolation=None);p.read(candidate)
                self.assertEqual(json.loads(p['yandex-api']['token'])['access_token'],'example-token')
                process.assert_not_called()
            finally:os.umask(before)

    def test_pinned_identity_allows_renewal_without_expired_old_auth(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            (root/'rclone.conf').write_text('[yandex-api]\ntype = yandex\n')
            (root/'config.json').write_text('{"repository":"rclone:yandex-api:backups/devcourse-finder"}')
            (root/'repository-id').write_text('expected-repository-id\n')
            before=os.umask(0o077)
            try:
                with patch.object(m,'C',root),patch.object(m.os,'geteuid',return_value=0),patch.object(m.sys,'argv',['setup']),patch.object(m.getpass,'getpass',return_value='example-token'),patch.object(m,'call') as call,patch.object(m,'check_api',side_effect=RuntimeError('stop after new credential check')):
                    with self.assertRaisesRegex(RuntimeError,'new credential check'):m.main()
                call.assert_not_called()
            finally:os.umask(before)

    def test_compact_refresh_token_error_explained_without_raw_stderr(self):
        result=Mock(returncode=1,stderr=b'unable to get RefreshToken PRIVATE_DETAILS',stdout=b'')
        with patch.object(m.sp,'run',return_value=result):
            with self.assertRaises(RuntimeError) as caught:m.call(['restic','cat','config'],phase='API repository')
        self.assertIn('refresh token',str(caught.exception))
        self.assertNotIn('PRIVATE_DETAILS',str(caught.exception))

    def test_redirect_fragment_and_json_use_same_access_token(self):
        for raw in ['example-token','OAuth example-token','https://oauth.yandex.ru/verification_code#access_token=example-token','{"access_token":"example-token","token_type":"OAuth"}']:
            self.assertEqual(m.parse_token(raw)['access_token'],'example-token')


if __name__=='__main__':unittest.main()
