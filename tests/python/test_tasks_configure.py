"""Verify Conan-to-CMake configuration commands without running either tool."""
import contextlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

from invoke import Exit
import tasks


class ConfigureTests(unittest.TestCase):
    def test_generated_toolchain_is_passed_with_fresh_configuration(self):
        for build_type in ("Release", "Debug"):
            with self.subTest(build_type=build_type), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                build_dir = root / build_type.lower()
                toolchain = build_dir / "build" / build_type / "generators" / "conan_toolchain.cmake"
                context = Mock()

                def generate(command, **kwargs):
                    if command.startswith("conan install "):
                        toolchain.parent.mkdir(parents=True)
                        toolchain.touch()

                context.run.side_effect = generate
                with patch.object(tasks, "BUILD_DIR", root), contextlib.redirect_stdout(io.StringIO()):
                    tasks.configure.body(context, build_type=build_type)

                self.assertEqual(context.run.call_count, 2)
                command = context.run.call_args.args[0]
                self.assertIn("cmake --fresh ", command)
                self.assertIn(f"-DCMAKE_TOOLCHAIN_FILE={toolchain}", command)
                self.assertIn(f"-DCMAKE_BUILD_TYPE={build_type}", command)

    def test_missing_toolchain_stops_before_cmake(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # A stale toolchain for another type must never be substituted.
            stale = root / "release" / "build" / "Debug" / "generators" / "conan_toolchain.cmake"
            stale.parent.mkdir(parents=True)
            stale.touch()
            context = Mock()
            with patch.object(tasks, "BUILD_DIR", root), contextlib.redirect_stdout(io.StringIO()):
                with self.assertRaisesRegex(Exit, "Conan toolchain not found"):
                    tasks.configure.body(context, build_type="Release")
            self.assertEqual(context.run.call_count, 1)
            self.assertTrue(context.run.call_args.args[0].startswith("conan install "))


if __name__ == "__main__":
    unittest.main()
