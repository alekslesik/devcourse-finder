import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('tls_rotation', Path(__file__).with_name('rotate-paid-tls.py'))
rotation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rotation)


class RotationTests(unittest.TestCase):
    def test_reload_forces_manual_certificate_reload_with_unchanged_caddyfile(self):
        with patch.object(rotation, 'run') as run:
            rotation.activate(Path('/srv/devcourse-finder/ingress'))
            self.assertEqual(run.call_count, 2)
            validate, reload = [call.args for call in run.call_args_list]
            self.assertIn('validate', validate)
            self.assertNotIn('--force', validate)
            self.assertIn('reload', reload)
            self.assertIn('--force', reload)

    def test_private_key_permissions_and_symlink_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            key = Path(temporary) / 'private.key'
            key.write_bytes(b'test-only-key')
            key.chmod(0o644)
            with patch.object(rotation.os, 'geteuid', return_value=0), self.assertRaises(ValueError):
                rotation.protected_file(key, private=True)
            linked = Path(temporary) / 'linked'
            linked.symlink_to(key)
            with self.assertRaises(ValueError):
                rotation.protected_file(linked)

    def test_payment_issuer_chain_and_matching_key_are_required(self):
        chain = b'-----BEGIN CERTIFICATE-----leaf-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----intermediate-----END CERTIFICATE-----'
        for issuer, key, ok in [(b'issuer=GlobalSign', b'PUBLIC', True),
                                 (b'issuer=Other CA', b'PUBLIC', False),
                                 (b'issuer=GlobalSign', b'FOREIGN', False)]:
            def fake_run(*args):
                if '-issuer' in args: return issuer
                if '-pubkey' in args: return b'PUBLIC'
                if args[:2] == ('openssl', 'pkey'): return key
                return b'OK'
            with self.subTest(issuer=issuer, key=key), tempfile.TemporaryDirectory() as temporary, \
                    patch.object(rotation, 'run', side_effect=fake_run), \
                    patch.object(rotation, 'leaf_metadata', return_value={'sha256': 'new'}):
                if ok:
                    self.assertEqual(rotation.validate_pair(chain, b'PRIVATE', 'devcoursefinder.ru', Path(temporary)), {'sha256': 'new'})
                else:
                    with self.assertRaises(ValueError):
                        rotation.validate_pair(chain, b'PRIVATE', 'devcoursefinder.ru', Path(temporary))
        with tempfile.TemporaryDirectory() as temporary, self.assertRaises(ValueError):
            rotation.validate_pair(b'-----BEGIN CERTIFICATE-----leaf-----END CERTIFICATE-----', b'key', 'devcoursefinder.ru', Path(temporary))

    def test_atomic_pair_rollback_and_extended_validity_guard(self):
        old = {'sha256': 'old', 'not_after': '2027-05-25T13:34:52+00:00'}
        new = {'sha256': 'new', 'not_after': '2028-05-25T13:34:52+00:00'}
        for failure in (False, True):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                tls = Path(temporary)
                chain, key = tls / 'fullchain.pem', tls / 'privkey.key'
                chain.write_bytes(b'OLD_CHAIN'); key.write_bytes(b'OLD_KEY'); key.chmod(0o600)
                with patch.object(rotation, 'protected_file', side_effect=lambda p, **kw: p.read_bytes()), \
                        patch.object(rotation, 'leaf_metadata', return_value=old), \
                        patch.object(rotation, 'activate') as activate, \
                        patch.object(rotation, 'verify_served', side_effect=([RuntimeError('wrong origin'), old] if failure else [new])):
                    if failure:
                        with self.assertRaisesRegex(RuntimeError, 'previous trusted pair was restored'):
                            rotation.install_pair(tls, tls, b'NEW_CHAIN', b'NEW_KEY', new, 'devcoursefinder.ru')
                        self.assertEqual(chain.read_bytes(), b'OLD_CHAIN')
                        self.assertEqual(key.read_bytes(), b'OLD_KEY')
                        self.assertEqual(activate.call_count, 2)
                    else:
                        result = rotation.install_pair(tls, tls, b'NEW_CHAIN', b'NEW_KEY', new, 'devcoursefinder.ru')
                        self.assertEqual(result['action'], 'installed')
                        self.assertEqual(chain.read_bytes(), b'NEW_CHAIN')
                        self.assertEqual(key.stat().st_mode & 0o777, 0o600)
                        self.assertEqual((Path(result['rollback_directory']) / 'privkey.key').read_bytes(), b'OLD_KEY')
                    with self.assertRaises(ValueError):
                        rotation.install_pair(tls, tls, b'NEW', b'NEW_KEY', dict(new, not_after='2027-01-01T00:00:00+00:00'), 'devcoursefinder.ru')

    def test_already_installed_pair_never_reloads(self):
        metadata = {'sha256': 'same', 'not_after': '2027-05-25T13:34:52+00:00'}
        with patch.object(rotation, 'protected_file', return_value=b'PAIR'), \
                patch.object(rotation, 'leaf_metadata', return_value=metadata), \
                patch.object(rotation, 'verify_served', return_value=metadata), \
                patch.object(rotation, 'activate') as activate:
            self.assertEqual(rotation.install_pair(Path('/unused'), Path('/unused'), b'PAIR', b'PAIR', metadata, 'devcoursefinder.ru')['action'], 'already_installed')
            activate.assert_not_called()


if __name__ == '__main__':
    unittest.main()
