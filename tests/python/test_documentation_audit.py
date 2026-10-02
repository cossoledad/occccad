import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from tools.documentation_audit import anchors, audit


class DocumentationAuditTest(unittest.TestCase):
    def test_heading_duplicates_unicode_and_fences(self):
        self.assertEqual(anchors("# 显示与选择\n# Heading `code`\n# Heading `code`\n```md\n# Not a heading\n```\n"), {"显示与选择", "heading-code", "heading-code-1"})

    def test_all_owned_readmes_paths_and_anchors(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # Test input only, not repository editing.
            (root / "module").mkdir()
            (root / "module/README.md").write_text("[topic](../topic.md#known)\n[broken](../topic.md#missing)\n[absent](gone.md)\n[remote](https://example.test)\n")
            (root / "topic.md").write_text("# Known\n")
            with patch("tools.documentation_audit.subprocess.check_output", return_value=b"module/README.md\0topic.md\0"):
                errors = audit(root)
            self.assertEqual(len(errors), 2)
            self.assertTrue(any("broken anchor" in e for e in errors))
            self.assertTrue(any("broken local link" in e for e in errors))

    def test_reference_links_and_fenced_examples(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            (root/"README.md").write_text("```md\n[example](not-real.md)\n```\n[ref]: absent.md\n")
            with patch("tools.documentation_audit.subprocess.check_output", return_value=b"README.md\0"):
                self.assertEqual(len(audit(root)),1)

    def test_unclosed_fenced_block(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            (root/"README.md").write_text("# Broken\n```sh\ncommand\n")
            with patch("tools.documentation_audit.subprocess.check_output", return_value=b"README.md\0"):
                self.assertTrue(any("unclosed fenced block" in e for e in audit(root)))
