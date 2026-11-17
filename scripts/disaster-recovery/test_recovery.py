import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('recovery', Path(__file__).with_name('recovery.py'))
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)


class RecoverySafetyTests(unittest.TestCase):
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
