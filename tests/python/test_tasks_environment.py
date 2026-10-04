"""Exercise Invoke's environment loading without starting any resource commands."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class TaskEnvironmentTests(unittest.TestCase):
    def test_explicit_file_and_export_precedence(self):
        with tempfile.TemporaryDirectory() as directory:
            env_file = Path(directory) / '.env'
            env_file.write_text('OCCCCAD_ARTIFACT_BACKEND=LOCAL\nOCCCCAD_DATA_DIR=/isolated/data\n')
            env = os.environ.copy()
            for key in ('OCCCCAD_ARTIFACT_BACKEND', 'OCCCCAD_DATA_DIR'):
                env.pop(key, None)
            env['OCCCCAD_ENV_FILE'] = os.path.relpath(env_file, ROOT)
            script = '''import os, tasks
assert os.environ['OCCCCAD_ARTIFACT_BACKEND'] == 'LOCAL'
assert os.environ['OCCCCAD_DATA_DIR'] == '/isolated/data'
assert os.path.isabs(os.environ['OCCCCAD_ENV_FILE'])
'''
            subprocess.run([sys.executable, '-c', script], cwd=ROOT, env=env, check=True)
            env['OCCCCAD_ARTIFACT_BACKEND'] = 'EXPORTED'
            subprocess.run([sys.executable, '-c', "import os, tasks; assert os.environ['OCCCCAD_ARTIFACT_BACKEND'] == 'EXPORTED'"], cwd=ROOT, env=env, check=True)

    def test_missing_explicit_file_fails_without_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            env = dict(os.environ, OCCCCAD_ENV_FILE=str(Path(directory) / 'missing'))
            result = subprocess.run([sys.executable, '-c', 'import tasks'], cwd=ROOT, env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'FileNotFoundError', result.stderr)


if __name__ == '__main__':
    unittest.main()
