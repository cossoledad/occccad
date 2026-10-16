import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from tools.diagnostic import inspect, resolve, recent
from tools.task_context import audit, packet

ROOT = Path(__file__).resolve().parents[2]


class TaskContextTests(unittest.TestCase):
    def test_owned_task_routes_have_real_sources_and_symbols(self):
        self.assertEqual(audit(ROOT), [])

    def test_packet_excerpts_are_bounded_and_metrics_are_not_token_claims(self):
        with tempfile.TemporaryDirectory() as directory:
            result = packet(ROOT, 'selection', read=True, output=Path(directory) / 'packet.json')
            self.assertLessEqual(len(result['snippets']), 2)
            self.assertLessEqual(result['metrics']['returnedSourceBytes'], 10000)
            self.assertIsNone(result['metrics']['modelTokens'])
            self.assertGreater(result['metrics']['searchCalls'], 0)

    def test_stale_symbol_routes_fail_audit(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'owner.go').write_text('func Current() {}')
            (root / 'contract.md').write_text('contract')
            with patch('tools.task_context.RECIPES', {'example': {'sources': [('owner.go', 'Removed')], 'documents':['contract.md']}}):
                self.assertTrue(any('missing symbol Removed' in e for e in audit(root)))


class DiagnosticToolsTests(unittest.TestCase):
    def test_resolver_preserves_fractional_ids_and_rejects_escape(self):
        with tempfile.TemporaryDirectory() as directory:
            logs = Path(directory)
            path = logs / 'debug/cad-diagnostics/document/20261016T090000.123Z-record.json'
            path.parent.mkdir(parents=True)
            path.write_text('{}')
            self.assertEqual(resolve('CAD_DIAGNOSTIC document/20261016T090000.123Z-record CODE', logs), path)
            for reference in ('../record', 'document/../../record', 'document/..', '/tmp/record'):
                with self.assertRaises(ValueError): resolve(reference, logs)

    def test_inspection_exports_only_recorded_input_and_never_changes_source(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'failure.json'
            data = {'schema':'occccad.operation-diagnostic.v1','documentId':'doc','baseRevisionId':'frozen',
                    'assemblyAttempts':[{'schema':'occccad.3dreplay.v1','request':{'requestId':'recorded'}}]}
            source.write_text(json.dumps(data))
            before = source.read_bytes()
            environment = {'OCCCCAD_GEOMETRY_WORKER_BIN':'/existing/worker'}
            result = inspect(root, environment, file=source)
            self.assertEqual(result['baseRevisionId'], 'frozen')
            self.assertEqual(json.loads(Path(result['input']).read_text())['request']['requestId'], 'recorded')
            self.assertEqual(source.read_bytes(), before)
            Path(result['input']).write_text('{}')
            with self.assertRaisesRegex(ValueError, 'differs'):
                inspect(root, environment, file=source)

    def test_retained_domain_input_does_not_claim_numerical_replay(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'failure.json'
            source.write_text(json.dumps({'schema':'occccad.operation-diagnostic.v1','baseModel':{'features':[]}}))
            self.assertEqual(inspect(root, {}, file=source)['replayStatus'], 'NO_RECORDED_NUMERICAL_ATTEMPT')
            with self.assertRaisesRegex(ValueError, 'full operation replay is unavailable'):
                inspect(root, {}, file=source, replay=True)

    def test_jobs_use_their_own_archive_without_a_second_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            logs = Path(directory)
            path = logs / 'debug/job-diagnostics/job/record.json'
            path.parent.mkdir(parents=True)
            path.write_text(json.dumps({'schema':'occccad.job-diagnostic.v1','jobId':'job','stage':'EXECUTE'}))
            path.with_name('record.meta.json').write_text('{}')
            self.assertEqual(resolve('[CAD_JOB_DIAGNOSTIC job/record]', logs), path)
            self.assertEqual(recent(logs, 'job', job=True)['entries'][0]['stage'], 'EXECUTE')
