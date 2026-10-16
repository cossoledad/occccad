import contextlib
import io
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from invoke import Exit
import tasks


class BenchmarkEntryTests(unittest.TestCase):
    def test_upstream_failure_is_not_hidden_by_log_capture(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            context = Mock()
            context.cd.return_value = contextlib.nullcontext()
            context.run.return_value = SimpleNamespace(ok=False, stdout='compiler failed\n', stderr='', exited=7)
            with patch.object(tasks, 'PROJECT_ROOT', root), patch.object(tasks.build, 'body') as build, contextlib.redirect_stdout(io.StringIO()):
                with self.assertRaises(Exit) as error:
                    tasks.performance_baseline.body(context, count=1)
                self.assertEqual(error.exception.code, 7)
                build.assert_not_called()
            self.assertNotIn('| tee', context.run.call_args.args[0])
            self.assertIn('compiler failed', (root / 'build/performance/go-workspace.txt').read_text())

    def test_success_exit_with_no_benchmark_samples_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            context = Mock()
            context.cd.return_value = contextlib.nullcontext()
            context.run.return_value = SimpleNamespace(ok=True, stdout='PASS\n', stderr='', exited=0)
            with patch.object(tasks, 'PROJECT_ROOT', Path(directory)), patch.object(tasks.build, 'body') as build, contextlib.redirect_stdout(io.StringIO()):
                with self.assertRaisesRegex(Exit, 'expected 1 measured runs; got 0'):
                    tasks.performance_baseline.body(context, count=1)
                build.assert_not_called()
