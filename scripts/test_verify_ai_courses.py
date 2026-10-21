from datetime import datetime, timezone
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from contextlib import redirect_stdout
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('verification', Path(__file__).with_name('verify-ai-courses.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class VerificationTests(unittest.TestCase):
    def setUp(self):
        clock = patch.object(module, 'datetime')
        self.clock = clock.start()
        self.addCleanup(clock.stop)
        self.clock.now.return_value = datetime(2026, 10, 20, 12, tzinfo=timezone.utc)

    def test_utc_budget_boundary_stops_before_a_paid_course(self):
        self.clock.now.return_value = datetime(2026, 10, 20, 23, 55, tzinfo=timezone.utc)
        with tempfile.TemporaryDirectory() as root:
            calls = []
            with patch.object(module.subprocess, 'run', self.fake(calls)), redirect_stdout(io.StringIO()):
                with self.assertRaisesRegex(ValueError, 'UTC budget rollover'):
                    module.verify(self.base(root), ['netology-go'])
            self.assertFalse(any('verify' in call for call in calls))

    def test_bounded_explicit_ids_reject_injection_duplicates_and_bulk(self):
        self.assertEqual(module.targets('netology-go,hexlet-java'), ['netology-go', 'hexlet-java'])
        for raw in ['', 'a,a', 'a,b,c,d,e', 'a;echo-x', '$(id)', '../a', 'a, b']:
            with self.assertRaises(ValueError):
                module.targets(raw)

    def base(self, root):
        base = Path(root) / 'production'
        release = base / 'releases' / 'v-test'
        release.mkdir(parents=True)
        (base / 'current').symlink_to(release)
        (base / '.env').write_text('API_KEY=never-print-this\n')
        for file in ('compose.yaml', 'compose.production.yaml'):
            (release / file).write_text('services: {}')
        (release / 'VERSION').write_text('0.31.0')
        return base

    def fake(self, calls, fail=False, timeout=False):
        def run(args, **kwargs):
            calls.append(args)
            if 'ps' in args:
                return subprocess.CompletedProcess(args, 0, 'running\n', '')
            if 'verify' in args:
                if timeout:
                    raise subprocess.TimeoutExpired(args, 1)
                payload = dict(course_id=args[-1], financial_unchanged=not fail,
                               verified_claims=[] if fail else [dict(field='duration')])
                return subprocess.CompletedProcess(args, int(fail), json.dumps(payload), '')
            return subprocess.CompletedProcess(args, 0, '', '')
        return run

    def test_uses_only_named_deployed_targets_without_budget_override(self):
        with tempfile.TemporaryDirectory() as root:
            base = self.base(root)
            before = (base / '.env').read_bytes()
            calls, output = [], io.StringIO()
            with patch.object(module.subprocess, 'run', self.fake(calls)), redirect_stdout(output):
                self.assertEqual(module.verify(base, ['netology-go', 'hexlet-java']), 0)
            paid = [a for a in calls if 'verify' in a]
            self.assertEqual([a[-1] for a in paid], ['netology-go', 'hexlet-java'])
            for args in paid:
                self.assertEqual(args[-4:-1], ['catalog-ai', 'catalog-ai', 'verify'])
                self.assertIn('--no-deps', args)
                self.assertIn('--pull', args)
                self.assertNotIn('CATALOG_AI_DAILY_REQUEST_LIMIT', ' '.join(args))
                self.assertFalse(any(a in args for a in ['up', 'build', 'restart', 'once']))
            self.assertEqual((base / '.env').read_bytes(), before)
            self.assertNotIn('never-print-this', output.getvalue())
            self.assertEqual(output.getvalue().count('VERIFY_AI_REPORT '), 2)
            self.assertEqual(len([a for a in calls if a[:2] == ['docker', 'rm']]), 2)

    def test_failed_first_report_stops_spending_and_cleans_owned_container(self):
        with tempfile.TemporaryDirectory() as root:
            calls = []
            with patch.object(module.subprocess, 'run', self.fake(calls, fail=True)), redirect_stdout(io.StringIO()):
                with self.assertRaises(ValueError):
                    module.verify(self.base(root), ['netology-go', 'hexlet-java'])
            self.assertEqual(len([a for a in calls if 'verify' in a]), 1)
            self.assertEqual(calls[-1][:3], ['docker', 'rm', '-f'])
            self.assertTrue(calls[-1][-1].startswith('devcourse-ai-verify-'))

    def test_timeout_cleans_only_owned_container_and_does_not_retry(self):
        with tempfile.TemporaryDirectory() as root:
            calls = []
            with patch.object(module.subprocess, 'run', self.fake(calls, timeout=True)), redirect_stdout(io.StringIO()):
                with self.assertRaises(subprocess.TimeoutExpired):
                    module.verify(self.base(root), ['netology-go'])
            self.assertEqual(len([a for a in calls if 'verify' in a]), 1)
            self.assertEqual(calls[-1][:3], ['docker', 'rm', '-f'])
