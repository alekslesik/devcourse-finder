import fcntl
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('collect-catalog.sh')


class CollectCatalogTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.base = self.root / 'production'
        self.release = self.base / 'releases' / 'v0.6.0-test'
        self.release.mkdir(parents=True)
        for name in ['compose.yaml', 'compose.production.yaml']:
            (self.release / name).write_text('services: {}\n')
        (self.base / '.env').write_text('SENSITIVE_TEST_VALUE=do-not-print-this\n')
        (self.base / 'current').symlink_to(self.release)
        self.calls = self.root / 'calls.jsonl'
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        docker = self.bin / 'docker'
        docker.write_text('''#!/usr/bin/env python3
import json,os,sys
args=sys.argv[1:]
with open(os.environ['TEST_CALLS'],'a') as f:f.write(json.dumps(args)+'\\n')
if 'config' in args:print(os.environ.get('TEST_SERVICES','db\\napi\\nfrontend\\ncatalog-updater'))
elif 'ps' in args:print(os.environ.get('TEST_CONTAINER','container-id'))
elif 'exec' in args:
 if sys.stdin.read():raise SystemExit('exec consumed the SSH script stream')
 if args[-1]=='once':raise SystemExit(int(os.environ.get('TEST_ONCE_EXIT','0')))
 if args[-1]=='status':
  print('{"queue":true,"pending":1,"published":5}')
  raise SystemExit(int(os.environ.get('TEST_STATUS_EXIT','0')))
else:raise SystemExit('Unexpected Docker operation')
''')
        docker.chmod(0o755)
        self.env = dict(os.environ, PATH=f'{self.bin}:{os.environ["PATH"]}', TEST_CALLS=str(self.calls))

    def run_script(self, **overrides):
        # Match the production SSH invocation: bash reads the entire script on stdin.
        result = subprocess.run(['bash', '-s', '--', str(self.base)], input=SCRIPT.read_text(),
                                text=True, capture_output=True, timeout=5,
                                env=dict(self.env, **overrides))
        self.assertNotIn('do-not-print-this', result.stdout + result.stderr)
        return result

    def recorded(self):
        return [json.loads(line) for line in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def test_success_uses_current_release_and_prints_status_without_deploying(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.recorded()
        self.assertEqual([call[-1] for call in calls[-2:]], ['once', 'status'])
        for call in calls:
            self.assertIn(str(self.release / 'compose.yaml'), call)
            self.assertIn(str(self.base / '.env'), call)
            self.assertEqual(call[call.index('--project-name') + 1], 'devcourse-finder')
            self.assertFalse(any(word in call for word in ['up', 'build', 'pull', 'restart']))
        for call in calls[-2:]:
            self.assertIn('--interactive=false', call)
            self.assertIn('-T', call)
        self.assertIn('"published":5', result.stdout)
        self.assertEqual((self.base / 'current').resolve(), self.release)

    def test_collection_failure_still_prints_status_and_preserves_exit_code(self):
        result = self.run_script(TEST_ONCE_EXIT='7')
        self.assertEqual(result.returncode, 7)
        self.assertEqual([call[-1] for call in self.recorded()[-2:]], ['once', 'status'])
        self.assertIn('"queue":true', result.stdout)

    def test_diagnostics_failure_fails_the_action(self):
        self.assertEqual(self.run_script(TEST_STATUS_EXIT='8').returncode, 1)

    def test_missing_worker_and_stopped_worker_are_not_started(self):
        result = self.run_script(TEST_SERVICES='db\napi\nfrontend')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('does not include', result.stderr)
        self.assertFalse(any('exec' in call for call in self.recorded()))
        self.calls.unlink()
        result = self.run_script(TEST_CONTAINER='')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('not running', result.stderr)
        self.assertFalse(any('exec' in call for call in self.recorded()))

    def test_shared_deployment_lock_prevents_collection(self):
        with (self.base / '.deploy.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('already running', result.stderr)
        self.assertEqual(self.recorded(), [])

    def test_missing_or_symlinked_environment_is_rejected(self):
        env_file = self.base / '.env'
        env_file.unlink()
        self.assertNotEqual(self.run_script().returncode, 0)
        target = self.root / 'other.env'
        target.write_text('SENSITIVE_TEST_VALUE=do-not-print-this\n')
        env_file.symlink_to(target)
        self.assertNotEqual(self.run_script().returncode, 0)
        self.assertEqual(self.recorded(), [])


if __name__ == '__main__':
    unittest.main()
