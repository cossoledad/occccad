import contextlib
import io
import json
from types import SimpleNamespace
from unittest.mock import Mock
from invoke import Exit
from tools.validation_run import run_steps
from pathlib import Path
import tempfile
import unittest

from tools.build_identity import record_runtime, running_identity
from test_validation_selection import git


class RunningBuildTests(unittest.TestCase):
    def test_same_files_do_not_hide_an_old_running_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            git(root, 'init', '-q')
            source = root / 'services/internal/config/environment.go'
            source.parent.mkdir(parents=True)
            source.write_text('package config\n')
            git(root, 'add', '.')
            git(root, 'commit', '-qm', 'source')
            paths = [root / 'build/services' / name for name in ('occccad-control','occccad-server','occccad-jobs')]
            paths.append(root / 'build/cmake/release/workers/geometry/occccad_geometry_worker')
            for path in paths:
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(b'compiled fixture')
            environment = {'OCCCCAD_BUILD_TYPE':'Release','OCCCCAD_GEOMETRY_WORKER_BIN':str(paths[-1]),
                           'OCCCCAD_DATABASE_URL':'sqlite:' + str(root / 'services/data/app.db'),
                           'OCCCCAD_DATA_DIR':str(root / 'services/data'),'OCCCCAD_LOG_DIR':str(root / 'services/logs'),
                           'OCCCCAD_RUNTIME_ID':'current-start'}
            record_runtime(root, environment)
            proc = root / 'proc'
            for index, path in enumerate(paths):
                process = proc / str(index + 100)
                process.mkdir(parents=True)
                (process / 'exe').symlink_to(path)
                (process / 'environ').write_bytes(b'OCCCCAD_RUNTIME_ID=current-start\0')
            self.assertEqual(running_identity(root, environment, proc)['status'], 'PASS')
            paths[0].write_bytes(b'new build at same path')
            self.assertEqual(running_identity(root, environment, proc)['status'], 'ENVIRONMENT_BLOCKED')
            paths[0].write_bytes(b'compiled fixture')
            # A private integration Worker is not part of the managed app tree.
            other = proc / '999'
            other.mkdir()
            (other / 'exe').symlink_to(paths[-1])
            (other / 'environ').write_bytes(b'OCCCCAD_RUNTIME_ID=another-process\0')
            self.assertEqual(running_identity(root, environment, proc)['status'], 'PASS')
            (proc / '100/environ').write_bytes(b'OCCCCAD_RUNTIME_ID=old-start\0')
            self.assertEqual(running_identity(root, environment, proc)['status'], 'ENVIRONMENT_BLOCKED')
            (proc / '100/environ').write_bytes(b'OCCCCAD_RUNTIME_ID=current-start\0')
            environment['OCCCCAD_DATA_DIR'] = str(root / 'another-directory')
            self.assertEqual(running_identity(root, environment, proc)['status'], 'ENVIRONMENT_BLOCKED')


class ValidationSourceTests(unittest.TestCase):
    def test_changes_during_validation_cannot_report_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            git(root, 'init', '-q')
            source = root / 'code.go'
            source.write_text('package before')
            environment = {'OCCCCAD_BUILD_TYPE':'Release', 'OCCCCAD_GEOMETRY_WORKER_BIN':str(root/'worker'),
                           'OCCCCAD_DATABASE_URL':'sqlite:' + str(root/'app.db'),
                           'OCCCCAD_DATA_DIR':str(root/'data'), 'OCCCCAD_LOG_DIR':str(root/'logs')}
            context = Mock()
            context.cd.return_value = contextlib.nullcontext()
            def change(*args, **kwargs):
                source.write_text('package after')
                return SimpleNamespace(stdout='success', stderr='', ok=True, exited=0)
            context.run.side_effect = change
            with contextlib.redirect_stdout(io.StringIO()), self.assertRaises(Exit):
                run_steps(context, [('fixture','fixture',root)], root, environment)
            report = json.loads(next((root/'build/agent-check').glob('*/report.json')).read_text())
            self.assertEqual(report['status'], 'SOURCE_CHANGED')
            self.assertNotEqual(report['source'], report['sourceAfter'])
