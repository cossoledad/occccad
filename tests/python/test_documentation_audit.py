import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from tools.documentation_audit import (
    CODE_GUIDES, REQUIRED_ENTRIES, anchors, audit, entry_sizes,
    navigation_errors, structure_errors,
)


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

    def test_discovery_rejects_orphan_cycles_and_fenced_fake_links(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "docs").mkdir()
            (root / "README.md").write_text("[docs](docs/)\n```md\n[example](orphan.md)\n```\n")
            (root / "docs/README.md").write_text("[page]: topic.md\n")
            (root / "docs/topic.md").write_text("# Topic\n")
            (root / "orphan.md").write_text("[cycle](cycle.md)\n")
            (root / "cycle.md").write_text("[orphan](orphan.md)\n")
            paths = b"README.md\0docs/README.md\0docs/topic.md\0orphan.md\0cycle.md\0"
            with patch("tools.documentation_audit.subprocess.check_output", return_value=paths):
                self.assertEqual(navigation_errors(root), [
                    "unreachable Markdown: cycle.md", "unreachable Markdown: orphan.md",
                ])
                with (root / "README.md").open("a") as output:
                    output.write("[report](orphan.md)\n")
                self.assertEqual(navigation_errors(root), [])

    def test_moved_report_links_to_space_named_fixtures_and_encoded_anchors(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "docs/validation").mkdir(parents=True)
            (root / "tests/test.data").mkdir(parents=True)
            (root / "tests/test.data/part model.step").touch()
            (root / "docs/topic.md").write_text("# 中文标题\n")
            (root / "docs/validation/report.md").write_text(
                "[input](<../../tests/test.data/part model.step>)\n"
                "[topic](../topic.md#%E4%B8%AD%E6%96%87%E6%A0%87%E9%A2%98)\n"
                "[input-ref]: <../../tests/test.data/part model.step>\n")
            with patch("tools.documentation_audit.subprocess.check_output", return_value=b"docs/validation/report.md\0docs/topic.md\0"):
                self.assertEqual(audit(root), [])

    def test_knowledge_layout_requires_indexes_and_classifies_new_pages(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            paths = [*REQUIRED_ENTRIES, "docs/loose-topic.md", "docs/guides/Bad_Name.md"]
            for name in paths:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("# Page\n")
            (root / "docs/guides/README.md").unlink()
            with patch("tools.documentation_audit.subprocess.check_output", return_value="\0".join(paths).encode()):
                self.assertEqual(structure_errors(root), [
                    "missing knowledge entry: docs/guides/README.md",
                    "non-semantic documentation filename: docs/guides/Bad_Name.md",
                    "unclassified knowledge page: docs/loose-topic.md",
                ])

    def test_local_task_budget_does_not_count_optional_project_map(self):
        data = {"AGENTS.md": 100, "docs/README.md": 1000,
                **{guide: 200 for guide in CODE_GUIDES}}
        before = entry_sizes(data)
        data["docs/README.md"] = 2000
        after = entry_sizes(data)
        self.assertEqual(before["localTask"], after["localTask"])
        self.assertEqual(after["withProjectMap"][CODE_GUIDES[0]], 2300)
