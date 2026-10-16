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

    def test_node_uses_the_same_literal_file_and_export_precedence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / '.env').write_text('OCCCCAD_APP_LISTEN=127.0.0.1:9991\n')
            file = root / 'selected.env'
            file.write_text('export OCCCCAD_ADMIN_PASSWORD="literal$secret#text"\nOCCCCAD_APP_LISTEN=127.0.0.1:9992\n')
            env = {k:v for k,v in os.environ.items() if not k.startswith('OCCCCAD_')}
            env['OCCCCAD_ENV_FILE'] = str(file)
            script = """import assert from 'node:assert/strict';
import {readDevelopmentEnvironment} from './tools/development_environment.mjs';
const root=process.argv[1], values=readDevelopmentEnvironment(root);
assert.equal(values.OCCCCAD_ADMIN_PASSWORD, 'literal$secret#text');
assert.equal(values.OCCCCAD_APP_LISTEN, '127.0.0.1:9992');
assert.equal(readDevelopmentEnvironment(root,{OCCCCAD_ENV_FILE:process.env.OCCCCAD_ENV_FILE,OCCCCAD_APP_LISTEN:'exported'}).OCCCCAD_APP_LISTEN,'exported');
assert.throws(()=>readDevelopmentEnvironment(root,{OCCCCAD_ENV_FILE:root+'/missing'}));
"""
            subprocess.run(['node','--input-type=module','-e',script,str(root)], cwd=ROOT, env=env, check=True)

    def test_missing_explicit_file_fails_without_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            env = dict(os.environ, OCCCCAD_ENV_FILE=str(Path(directory) / 'missing'))
            result = subprocess.run([sys.executable, '-c', 'import tasks'], cwd=ROOT, env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'FileNotFoundError', result.stderr)

    def test_test_resource_defaults_and_relative_sqlite_are_anchored(self):
        with tempfile.TemporaryDirectory() as directory:
            file = Path(directory) / '.env'
            file.write_text('OCCCCAD_TEST_RESOURCES_DIR=build/test-resources\nOCCCCAD_DATABASE_URL=sqlite:build/test-resources/example_test.db\n')
            env = os.environ.copy()
            for key in list(env):
                if key.startswith('OCCCCAD_TEST_') or key in {'OCCCCAD_ASSEMBLY_FIXTURE_DIR', 'OCCCCAD_DATABASE_URL', 'OCCCCAD_GEOMETRY_WORKER_BIN'}:
                    env.pop(key)
            env['OCCCCAD_ENV_FILE'] = str(file)
            script = '''import os, tasks
from pathlib import Path
assert Path(os.environ['OCCCCAD_ASSEMBLY_FIXTURE_DIR']).is_absolute()
assert Path(tasks._development_environment()['OCCCCAD_GEOMETRY_WORKER_BIN']).is_absolute()
assert Path(os.environ['OCCCCAD_TEST_IMPORT_BREP']).is_absolute()
assert os.environ['OCCCCAD_DATABASE_URL'] == 'sqlite:' + str(tasks.PROJECT_ROOT / 'build/test-resources/example_test.db')
'''
            subprocess.run([sys.executable, '-c', script], cwd=ROOT, env=env, check=True)

    def test_explicit_build_selects_the_same_worker_for_run_and_check(self):
        with tempfile.TemporaryDirectory() as directory:
            file = Path(directory) / '.env'
            file.write_text('OCCCCAD_BUILD_TYPE=Release\nOCCCCAD_DATABASE_URL=sqlite:services/data/app.db\n')
            env = {k: v for k, v in os.environ.items() if not k.startswith('OCCCCAD_')}
            env['OCCCCAD_ENV_FILE'] = str(file)
            script = '''import tasks
from pathlib import Path
release = tasks._development_environment()
debug = tasks._development_environment('Debug')
assert '/release/' in release['OCCCCAD_GEOMETRY_WORKER_BIN']
assert '/debug/' in debug['OCCCCAD_GEOMETRY_WORKER_BIN']
assert release['OCCCCAD_DATABASE_URL'] == debug['OCCCCAD_DATABASE_URL']
assert release['OCCCCAD_DATA_DIR'] == debug['OCCCCAD_DATA_DIR']
assert debug['OCCCCAD_BUILD_TYPE'] == 'Debug'
'''
            subprocess.run([sys.executable, '-c', script], cwd=ROOT, env=env, check=True)


if __name__ == '__main__':
    unittest.main()
