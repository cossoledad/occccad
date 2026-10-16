import json
from pathlib import Path
import tempfile
import unittest

from tools.validation import ctest_summary, go_summary
from tools.validation_run import compact_output


def output(*events):
    return '\n'.join(json.dumps(dict(Package='fixture', **event)) for event in events)


class ExecutionEvidenceTests(unittest.TestCase):
    def test_failure_output_bounds_a_single_large_utf8_line(self):
        result = compact_output('failure start ' + '数据' * 20000 + ' failure end')
        self.assertLessEqual(len(result.encode()), 16384)
        self.assertTrue(result.startswith('failure start'))
        self.assertTrue(result.endswith('failure end'))
        self.assertIn('see full log', result)

    def test_all_skipped_tests_are_blocked_even_with_success_exit(self):
        result = go_summary(output(dict(Action='run', Test='TestWorker'), dict(Action='skip', Test='TestWorker')), 0)
        self.assertEqual(result['status'], 'ENVIRONMENT_BLOCKED')
        self.assertEqual(result['counts']['passed'], 0)

    def test_parent_pass_does_not_hide_a_skipped_child(self):
        result = go_summary(output(dict(Action='run', Test='TestLifecycle'),
                                   dict(Action='run', Test='TestLifecycle/Commit'),
                                   dict(Action='skip', Test='TestLifecycle/Commit'),
                                   dict(Action='pass', Test='TestLifecycle')), 0)
        self.assertEqual(result['status'], 'ENVIRONMENT_BLOCKED')

    def test_optional_probe_is_explicit_and_selected_probe_is_required(self):
        events = output(dict(Action='run', Test='TestOrdinary'), dict(Action='pass', Test='TestOrdinary'),
                        dict(Action='run', Test='TestPostgres'),
                        dict(Action='output', Test='TestPostgres', Output='NOT_APPLICABLE: backend is SQLite'),
                        dict(Action='skip', Test='TestPostgres'))
        self.assertEqual(go_summary(events, 0)['status'], 'PASS_WITH_SKIPS')
        self.assertEqual(go_summary(events, 0, required_selection=True)['status'], 'ENVIRONMENT_BLOCKED')

    def test_zero_match_and_missing_terminal_verdict_fail(self):
        self.assertEqual(go_summary('', 0)['status'], 'FAIL')
        self.assertEqual(go_summary(output(dict(Action='run', Test='TestInterrupted')), 0)['status'], 'FAIL')

    def test_readable_failure_retains_process_output(self):
        events = output(dict(Action='run', Test='TestUndo'),
                        dict(Action='output', Test='TestUndo', Output='expected second Undo'),
                        dict(Action='fail', Test='TestUndo'))
        result = go_summary(events, 1)
        self.assertEqual(result['status'], 'FAIL')
        self.assertIn('expected second Undo', result['output'])

    def test_go_build_errors_remain_visible_without_unrelated_pass_logs(self):
        events = output(dict(Action='build-output', Output='undefined: missingSymbol'),
                        dict(Action='run', Test='TestOther'),
                        dict(Action='output', Test='TestOther', Output='unrelated geometry trace'),
                        dict(Action='pass', Test='TestOther'))
        result = go_summary(events, 1)
        self.assertIn('undefined: missingSymbol', result['failureOutput'])
        self.assertNotIn('unrelated geometry trace', result['failureOutput'])

    def test_ctest_requires_fresh_actual_execution(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.xml'
            self.assertEqual(ctest_summary(path, 0)['status'], 'FAIL')
            path.write_text('<testsuite><testcase name="native"><skipped/></testcase></testsuite>')
            self.assertEqual(ctest_summary(path, 0)['status'], 'ENVIRONMENT_BLOCKED')
            path.write_text('<testsuite><testcase name="native"/></testsuite>')
            self.assertEqual(ctest_summary(path, 0)['status'], 'PASS')
            path.write_text('<testsuite/>')
            self.assertEqual(ctest_summary(path, 0)['status'], 'FAIL')
