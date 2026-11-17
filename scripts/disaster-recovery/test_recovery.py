import copy
import datetime as dt
import importlib.util
import json
import shutil
import zipfile
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('recovery', Path(__file__).with_name('recovery.py'))
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)


class RecoverySafetyTests(unittest.TestCase):
    def test_password_secret_pair_is_explicit_private_and_complete(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, 'BASE', Path(d)):
            self.assertEqual(r.password_secret_paths(), set())
            directory = Path(d) / 'secrets'
            directory.mkdir(mode=0o700)
            key = directory / 'password-keyring.json'
            probe = directory / 'password-recovery.json'
            key.write_text('synthetic'); key.chmod(0o600)
            with self.assertRaisesRegex(RuntimeError, 'incomplete'):
                r.password_secret_paths()
            probe.write_text('synthetic'); probe.chmod(0o600)
            self.assertEqual(r.password_secret_paths(), {key, probe})
            probe.chmod(0o644)
            with self.assertRaisesRegex(RuntimeError, 'unsafe'):
                r.password_secret_paths()
            probe.unlink(); probe.symlink_to(key)
            with self.assertRaisesRegex(RuntimeError, 'unsafe'):
                r.password_secret_paths()
            directory.chmod(0o755)
            with self.assertRaisesRegex(RuntimeError, 'private'):
                r.password_secret_paths()

    def test_encrypted_password_checkpoint_restores_checksums_and_keeps_full_backup_status(self):
        for corrupt in [False, True]:
            with self.subTest(corrupt=corrupt), tempfile.TemporaryDirectory() as d:
                base = Path(d) / 'app'; state = Path(d) / 'state'
                base.mkdir(); state.mkdir()
                directory = base / 'secrets'; directory.mkdir(mode=0o700)
                for name in ['password-keyring.json', 'password-recovery.json']:
                    path = directory / name; path.write_text('synthetic'); path.chmod(0o600)
                (base / 'DEPLOYED_VERSION').write_text('version=v-test')
                def command(args, **kwargs):
                    if args[:3] == ['docker', 'ps', '-q']:
                        return b'fixture-container'
                    if args[:2] == ['docker', 'cp']:
                        Path(args[-1]).write_bytes(b'synthetic-verifier')
                    return b''
                def archive(*args, **kwargs):
                    if args[0] == 'backup':
                        self.assertIn('devcourse-password-keys-v1', args)
                        return b'{"message_type":"summary","snapshot_id":"fixture-snapshot"}\n'
                    self.assertEqual(args[:2], ('restore', 'fixture-snapshot'))
                    dest = Path(args[args.index('--target') + 1])
                    restored = dest / str(state / 'password-checkpoint').lstrip('/')
                    shutil.copytree(state / 'password-checkpoint', restored)
                    if corrupt:
                        (restored / 'verifier').write_bytes(b'corrupt')
                    return b''
                with patch.object(r, 'BASE', base), patch.object(r, 'STATE', state), patch.object(r, 'run', side_effect=command), patch.object(r, 'restic', side_effect=archive), patch.object(r, 'refresh_recovery_kit') as kit, patch.object(r, 'status') as status:
                    if corrupt:
                        with self.assertRaisesRegex(RuntimeError, 'checksum'):
                            r.password_checkpoint()
                        status.assert_not_called()
                        kit.assert_not_called()
                    else:
                        r.password_checkpoint()
                        kit.assert_called_once_with()
                        self.assertEqual(status.call_args.args, ('last-password-checkpoint',))
                        self.assertEqual(status.call_args.kwargs['snapshot'], 'fixture-snapshot')
                self.assertEqual(list(state.iterdir()), [])

    def test_independent_recovery_kit_includes_pepper_and_repository_access(self):
        with tempfile.TemporaryDirectory() as d:
            base = Path(d) / 'app'; config = Path(d) / 'config'; state = Path(d) / 'state'
            base.mkdir(); config.mkdir(); state.mkdir()
            directory = base / 'secrets'; directory.mkdir(mode=0o700)
            for name in ['password-keyring.json', 'password-recovery.json']:
                path = directory / name; path.write_text('synthetic-pepper'); path.chmod(0o600)
            (config / 'password').write_text('synthetic-backup-key')
            with patch.object(r, 'BASE', base), patch.object(r, 'STATE', state), patch.object(r, 'CONFIG', config):
                r.refresh_recovery_kit()
            kit = state / 'recovery-kit.zip'
            self.assertEqual(kit.stat().st_mode & 0o777, 0o600)
            with zipfile.ZipFile(kit) as archive:
                self.assertEqual(archive.read('etc/devcourse-backup/password'), b'synthetic-backup-key')
                self.assertEqual(archive.read(str((directory / 'password-keyring.json').relative_to('/'))), b'synthetic-pepper')
                self.assertNotIn('recovery-kit.zip', archive.namelist())

    @patch.object(r, 'password_secret_paths', return_value={Path('/fixture')})
    @patch.object(r, 'run')
    def test_password_recovery_verifies_only_snapshot_keys_in_isolated_container(self, run, paths):
        r.verify_password_secrets(Path('/isolated/snapshot'), 'sha256:pinned')
        args = run.call_args.args[0]
        self.assertIn('none', args)
        self.assertIn('--read-only', args)
        self.assertIn('384m', args)
        self.assertIn('sha256:pinned', args)
        self.assertIn('type=bind,source=/isolated/snapshot/rootfs/srv/devcourse-finder/secrets,target=/run/password-secrets,readonly', args)
        self.assertEqual(args[-4:], ['password-keyring', 'verify', '/run/password-secrets/password-keyring.json', '/run/password-secrets/password-recovery.json'])

    @patch.object(r, 'check_bundle', return_value={'postgres': '17.11', 'deployment': 'v-test'})
    @patch.object(r, 'restic', side_effect=RuntimeError('upload failed'))
    def test_failed_upload_never_applies_retention(self, restic, bundle):
        with self.assertRaisesRegex(RuntimeError, 'upload failed'):
            r.upload_bundle(r.STATE / 'bundle')
        self.assertEqual(restic.call_count, 1)
        self.assertEqual(restic.call_args.args[0], 'backup')
        self.assertNotIn('forget', restic.call_args.args)

    def test_recent_upload_of_stale_data_does_not_pass_freshness_monitor(self):
        with tempfile.TemporaryDirectory() as directory:
            state=Path(directory)
            now=dt.datetime.now(dt.timezone.utc)
            (state/'last-success.json').write_text(json.dumps({'at':now.isoformat(),'data_created_at':(now-dt.timedelta(hours=31)).isoformat(),'snapshot':'fixture'}))
            with patch.object(r,'STATE',state),patch.object(r.os,'geteuid',return_value=0),patch.object(r.sys,'argv',['recovery','status']):
                with self.assertRaisesRegex(RuntimeError,'stale'):r.main()

    def test_diagnostics_never_persist_signed_urls_or_auth_fields(self):
        result=r.redact_diagnostics(b'HTTP 500 at https://upload.example.invalid/file?capability=PRIVATE_VALUE\nAuthorization: PRIVATE_HEADER\naccess_token=PRIVATE_ACCESS\nnetwork timed out')
        self.assertIn(b'HTTP 500',result)
        self.assertIn(b'network timed out',result)
        self.assertNotIn(b'PRIVATE_',result)
        self.assertNotIn(b'https://',result)

    def test_legacy_source_selection_never_includes_backup_tree(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, 'BASE', Path(d)):
            base = Path(d)
            for name in ['backend', 'data', 'backups', 'releases', 'incoming']:
                (base / name).mkdir()
            (base / 'compose.yaml').write_text('services: {}')
            (base / 'current').symlink_to(base / 'releases')
            paths = r.source_paths(base)
            self.assertEqual(paths, {base / 'backend', base / 'data', base / 'compose.yaml'})
            self.assertNotIn(base, paths)

    def test_optional_uncaptured_service_removed_but_required_fails(self):
        config = {'services': {'db': {'build': '.'}, 'ai': {'profiles': ['ai']}}}
        captured = {'db': {'Image': 'sha256:actual', 'Config': {'Env': ['KEY=value=extra']}, 'State': {'Running': True}}}
        self.assertEqual(r.pin_services(config, captured), ['db'])
        self.assertNotIn('ai', config['services'])
        self.assertEqual(config['services']['db']['image'], 'sha256:actual')
        self.assertEqual(config['services']['db']['environment'], {'KEY': 'value=extra'})
        self.assertEqual(config['services']['db']['pull_policy'], 'never')
        self.assertNotIn('build', config['services']['db'])
        with self.assertRaisesRegex(RuntimeError, 'Required'):
            r.pin_services({'services': {'db': {}}}, {})

    def test_drill_cannot_publish_ports_mount_production_or_start_workers(self):
        original = {'name': 'devcourse-finder', 'networks': {'default': {'name': 'production'}},
                    'volumes': {'postgres-data': {'name': 'devcourse-finder_postgres-data'}},
                    'services': {n: {'image': 'sha256:abc', 'ports': ['3000:3000'],
                                     'networks': ['default'], 'volumes': ['/srv/production:/data'],
                                     'container_name': 'production-' + n,
                                     'depends_on': ['migrate'], 'restart': 'always'}
                                 for n in ['db', 'api', 'frontend', 'migrate', 'catalog-ai']}}
        before = copy.deepcopy(original)
        result = r.isolated_config(original)
        self.assertEqual(original, before)
        self.assertEqual(set(result['services']), {'db', 'api', 'frontend'})
        self.assertEqual(result['networks'], {'default': {'internal': True}})
        self.assertEqual(result['volumes'], {'postgres-data': {}})
        for name, svc in result['services'].items():
            for key in ['ports', 'container_name', 'depends_on', 'networks']:
                self.assertNotIn(key, svc)
            self.assertEqual(svc['restart'], 'no')
            if name != 'db': self.assertNotIn('volumes', svc)
        self.assertEqual(result['services']['db']['volumes'][0]['source'], 'postgres-data')

    def test_corruption_and_path_escape_refused(self):
        with tempfile.TemporaryDirectory() as d:
            bundle = Path(d)
            file = bundle / 'database.dump'
            file.write_bytes(b'PGDMP example')
            manifest = {'format': 1, 'sha256': {'database.dump': r.checksum(file)}}
            (bundle / 'manifest.json').write_text(json.dumps(manifest))
            r.check_bundle(bundle)
            file.write_bytes(b'corrupt')
            with self.assertRaisesRegex(RuntimeError, 'checksum'): r.check_bundle(bundle)
            manifest['sha256'] = {'../outside': 'bad'}
            (bundle / 'manifest.json').write_text(json.dumps(manifest))
            with self.assertRaisesRegex(RuntimeError, 'checksum'): r.check_bundle(bundle)

    @patch.object(r, 'run')
    def test_database_replacement_requires_confirmation_before_any_command(self, run):
        for confirm in ['', 'yes', 'REPLACE other-project']:
            with self.assertRaisesRegex(RuntimeError, 'Explicit'):
                r.restore_db(Path('/chosen.dump'), confirm)
        run.assert_not_called()

    @patch.object(r, 'dump', side_effect=RuntimeError('disk full'))
    @patch.object(r, 'js')
    @patch.object(r, 'run', return_value=b'db-id\napi-id\n')
    def test_failed_safety_dump_prevents_drop_and_keeps_writers_stopped(self, run, js, dump):
        js.return_value = [{'Id': 'db-id', 'Config': {'Labels': {'com.docker.compose.service': 'db'}}},
                           {'Id': 'api-id', 'Config': {'Labels': {'com.docker.compose.service': 'api'}}}]
        with tempfile.TemporaryDirectory() as d, patch.object(r, 'BASE', Path(d)), patch.object(r.sp, 'run') as process:
            process.return_value.returncode = 0
            source = Path(d) / 'chosen.dump'
            source.write_bytes(b'PGDMP example')
            with self.assertRaisesRegex(RuntimeError, 'disk full'):
                r.restore_db(source, 'REPLACE devcourse-finder')
        commands = [c.args[0] for c in run.call_args_list]
        self.assertIn(['docker', 'stop', 'api-id'], commands)
        self.assertFalse(any('start' in c or 'dropdb' in str(c) for c in commands))


if __name__ == '__main__': unittest.main()
