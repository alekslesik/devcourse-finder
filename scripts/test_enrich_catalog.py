import fcntl
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest

SCRIPT = Path(__file__).with_name('enrich-catalog.py')
# Freeze only the subprocess test clock, without a production clock override.
TEST_CLOCK_PREFIX = """import datetime as _dt, os as _os
_real_datetime = _dt.datetime
class _FixedDatetime(_real_datetime):
 @classmethod
 def now(cls, tz=None):
  value = _real_datetime.fromisoformat(_os.environ.get('TEST_CLOCK','2026-10-16T05:00:00+00:00'))
  return value.astimezone(tz) if tz else value.replace(tzinfo=None)
_dt.datetime = _FixedDatetime
"""



class EnrichCatalogTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.base = self.root / 'production'
        self.release = self.base / 'releases' / 'v0.28.0-test'
        self.release.mkdir(parents=True)
        (self.base / 'current').symlink_to(self.release)
        (self.base / '.env').write_text('SECRET_TEST_KEY=never-print-me\n')
        for name in ('compose.yaml', 'compose.production.yaml'):
            (self.release / name).write_text('services: {}\n')
        (self.release / 'VERSION').write_text('0.28.0\n')
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.calls = self.root / 'calls'
        docker = self.bin / 'docker'
        docker.write_text('''#!/usr/bin/env python3
import os,sys,json,time
from pathlib import Path
a=sys.argv[1:]
with open(os.environ['TEST_CALLS'],'a') as f:f.write(json.dumps(a)+'\\n')
if a[0]=='rm':raise SystemExit(0)
if 'ps' in a:
 print('' if a[-1]==os.environ.get('TEST_STOPPED') else 'running-id')
elif 'exec' in a:
 assert not sys.stdin.read(), 'consumed SSH stdin'
 print(json.dumps({'ai_collector':True,'pending':1,'requests_reserved_today':4,'tokens_today':100,'provider_coverage':[]}))
elif 'run' in a:
 assert not sys.stdin.read(), 'consumed SSH stdin'
 if os.environ.get('TEST_SLEEP'):
  Path(os.environ['TEST_READY']).touch();time.sleep(30)
 if os.environ.get('TEST_EXIT'):raise SystemExit(7)
 counter=Path(os.environ['TEST_COUNTER']);i=int(counter.read_text()) if counter.exists() else 0;counter.write_text(str(i+1))
 batches=json.loads(os.environ['TEST_BATCHES']);b=batches[min(i,len(batches)-1)]
 for code in b.get('codes',[]):print(json.dumps({'msg':'AI course checked','code':code}),file=sys.stderr)
 if not os.environ.get('TEST_MALFORMED'):
  print(json.dumps(dict(run_id=i+1,attempted=b.get('attempted',1),queued=b.get('queued',1),cached=0,failed=b.get('failed',0),tokens=10,status='partial' if b.get('failed') else 'completed')))
else:raise SystemExit('Unexpected Docker operation')
''')
        docker.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + ':' + os.environ['PATH'],
                        TEST_CALLS=str(self.calls), TEST_COUNTER=str(self.root / 'counter'),
                        TEST_READY=str(self.root / 'ready'), TEST_BATCHES=json.dumps([
                            dict(attempted=2, queued=1, failed=1, codes=['source_unavailable','queued']),
                            dict(attempted=0, queued=0, codes=[])]))

    def invoke(self, mode='enrich', budget='200', batches='10', **env):
        # SSH sends source on stdin, just as in the real workflow.
        p = subprocess.run([sys.executable, '-', str(self.base), mode, budget, batches],
                           input=TEST_CLOCK_PREFIX + SCRIPT.read_text(), capture_output=True, text=True, timeout=10,
                           env=dict(self.env, **env))
        self.assertNotIn('never-print-me', p.stdout + p.stderr)
        return p

    def recorded(self):
        return [json.loads(v) for v in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def report(self, p):
        lines = [v for v in p.stdout.splitlines() if v.startswith('AI_ENRICHMENT_SUMMARY ')]
        self.assertEqual(len(lines), 1, p.stdout + p.stderr)
        return json.loads(lines[0].split(' ', 1)[1])

    def test_due_batches_use_deployed_image_and_transient_budget(self):
        original = (self.base / '.env').read_bytes()
        p = self.invoke()
        self.assertEqual(p.returncode, 0, p.stderr)
        r = self.report(p)
        self.assertEqual((r['batches'], r['attempted'], r['queued'], r['failed']), (2, 2, 1, 1))
        self.assertEqual(r['stop_reason'], 'no_due_courses')
        self.assertEqual(r['course_codes'], {'source_unavailable': 1, 'queued': 1})
        calls = self.recorded()
        runs = [v for v in calls if 'run' in v]
        self.assertEqual(len(runs), 2)
        for a in runs:
            self.assertIn(str(self.release / 'compose.production.yaml'), a)
            for flag in ('--no-deps', '--rm', '--pull', 'never', 'CATALOG_AI_BATCH_SIZE=40', 'CATALOG_AI_DAILY_REQUEST_LIMIT=200'):
                self.assertIn(flag, a)
            self.assertEqual(a[-3:], ['catalog-ai', 'catalog-ai', 'once'])
            self.assertFalse(any(v in a for v in ('up', 'build', 'restart')))
        self.assertEqual(calls[-1][:2], ['rm', '-f'])
        self.assertTrue(calls[-1][-1].startswith('devcourse-ai-manual-'))
        self.assertEqual((self.base / '.env').read_bytes(), original)
        self.assertEqual((self.base / 'current').resolve(), self.release)

    def test_budget_day_boundary_does_not_start_a_paid_batch(self):
        p = self.invoke(TEST_CLOCK='2026-10-16T23:45:00+00:00')
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(self.report(p)['stop_reason'], 'budget_day_boundary')
        self.assertFalse(any('run' in v for v in self.recorded()))

    def test_budget_stops_before_another_batch(self):
        p = self.invoke(TEST_BATCHES=json.dumps([dict(codes=['daily_budget_exhausted'],queued=0,failed=1)]))
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(self.report(p)['stop_reason'], 'daily_budget_exhausted')
        self.assertEqual(len([v for v in self.recorded() if 'run' in v]), 1)

    def test_status_never_runs_paid_worker_or_removes_containers(self):
        p = self.invoke(mode='status')
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(self.report(p)['stop_reason'], 'status_only')
        self.assertFalse(any('run' in v or v[0]=='rm' for v in self.recorded()))

    def test_batch_cap_does_not_promise_whole_catalog_completion(self):
        p = self.invoke(batches='1')
        self.assertEqual(self.report(p)['stop_reason'], 'batch_limit')
        self.assertEqual(self.report(p)['batches'], 1)

    def test_provider_failure_is_not_masked_as_success_and_stops(self):
        p = self.invoke(TEST_BATCHES=json.dumps([dict(codes=['deepseek_authentication_failed'],queued=0,failed=1)]))
        self.assertNotEqual(p.returncode, 0)
        self.assertEqual(self.report(p)['stop_reason'], 'provider_unavailable')
        self.assertIn('after', self.report(p))
        self.assertEqual(len([v for v in self.recorded() if 'run' in v]), 1)

    def test_command_or_result_failure_stops_and_cleans_owned_container(self):
        for env in ({'TEST_EXIT': '1'}, {'TEST_MALFORMED': '1'}):
            with self.subTest(env=env):
                self.calls.unlink(missing_ok=True)
                p = self.invoke(**env)
                self.assertNotEqual(p.returncode, 0)
                self.assertEqual(self.report(p)['stop_reason'], 'error')
                self.assertEqual(self.recorded()[-1][:2], ['rm', '-f'])
                self.assertEqual(len([v for v in self.recorded() if 'run' in v]), 1)

    def test_host_lock_prevents_overlap(self):
        with (self.base / '.deploy.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            p = self.invoke()
        self.assertNotEqual(p.returncode, 0)
        self.assertIn('running', p.stderr)
        self.assertFalse(self.recorded())

    def test_missing_publisher_blocks_paid_work(self):
        p = self.invoke(TEST_STOPPED='catalog-publisher')
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse(any('run' in v for v in self.recorded()))

    def test_rejects_unsafe_or_out_of_range_inputs_before_docker(self):
        for mode, budget, batches in [('enrich; echo bad','200','10'),('enrich','201','10'),('enrich','200','26')]:
            p = self.invoke(mode=mode,budget=budget,batches=batches)
            self.assertNotEqual(p.returncode, 0)
            self.assertFalse(self.recorded())

    def test_signal_cleans_only_owned_oneoff(self):
        p = subprocess.Popen([sys.executable,'-',str(self.base),'enrich','200','10'],
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             text=True,env=dict(self.env,TEST_SLEEP='1'))
        p.stdin.write(TEST_CLOCK_PREFIX + SCRIPT.read_text());p.stdin.close();p.stdin=None
        try:
            for _ in range(300):
                if (self.root / 'ready').exists():break
                time.sleep(.01)
            self.assertTrue((self.root / 'ready').exists())
            p.send_signal(signal.SIGTERM)
            out,err=p.communicate(timeout=10)
            self.assertNotEqual(p.returncode,0)
            self.assertIn('"stop_reason":"cancelled"',out)
            self.assertEqual(self.recorded()[-1][:2],['rm','-f'])
        finally:
            if p.poll() is None:p.kill();p.wait()

    def test_runner_summary_marks_partial_work_and_source_failures(self):
        p=self.invoke()
        log=self.root/'log';log.write_text(p.stdout)
        report=self.root/'summary'
        subprocess.run([sys.executable,str(SCRIPT),'--summary',str(log),str(report)],check=True)
        text=report.read_text()
        self.assertIn('source_unavailable',text)
        self.assertIn('not necessarily a changed card',text)
        self.assertIn('03:00 Moscow',text)


if __name__=='__main__':unittest.main()
