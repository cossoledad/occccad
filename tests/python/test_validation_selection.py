from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from invoke import Exit
import tasks


def git(root, *args):
    return subprocess.check_output(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@invalid',
                                    '-c', 'commit.gpgsign=false', '-c', 'core.hooksPath=/dev/null', *args],
                                   cwd=root, stderr=subprocess.DEVNULL, text=True).strip()


class GitSelectionTests(unittest.TestCase):
    def test_commit_range_worktree_and_index_have_distinct_selection(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            git(root, 'init', '-q')
            owner = root / 'services/internal/workspace/model_core.go'
            owner.parent.mkdir(parents=True)
            owner.write_text('package workspace\n')
            git(root, 'add', '.')
            git(root, 'commit', '-qm', 'base')
            base = git(root, 'rev-parse', 'HEAD')
            owner.write_text('package workspace\n// committed\n')
            git(root, 'add', '.')
            git(root, 'commit', '-qm', 'change')
            (root / 'proto').mkdir()
            (root / 'proto/new.proto').write_text('syntax = "proto3";')
            git(root, 'add', 'proto/new.proto')
            (root / 'web').mkdir()
            (root / 'web/untracked.ts').write_text('export {};')
            with patch.object(tasks, 'PROJECT_ROOT', root):
                self.assertEqual(tasks._changed_files(base, 'HEAD'), ['services/internal/workspace/model_core.go'])
                self.assertEqual(tasks._changed_files(staged=True), ['proto/new.proto'])
                self.assertEqual(set(tasks._changed_files(base)), {'services/internal/workspace/model_core.go','proto/new.proto','web/untracked.ts'})
                with self.assertRaises(Exit): tasks._changed_files('missing-commit')

    def test_cross_domain_rename_selects_both_owners(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            git(root, 'init', '-q')
            old = root / 'services/internal/workspace/old.go'
            old.parent.mkdir(parents=True)
            old.write_text('package workspace\n')
            git(root, 'add', '.')
            git(root, 'commit', '-qm', 'base')
            (root / 'web').mkdir()
            git(root, 'mv', 'services/internal/workspace/old.go', 'web/moved.go')
            with patch.object(tasks, 'PROJECT_ROOT', root):
                paths = tasks._changed_files(staged=True)
                self.assertEqual(tasks._validation_scopes_for_paths(paths)[0], {'workspace','web'})
