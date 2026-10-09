"""Verify Conan-to-CMake configuration commands without running either tool."""
import contextlib
import io
import shlex
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

    def test_explicit_profile_name_and_paths_with_spaces(self):
        with tempfile.TemporaryDirectory(prefix="cad commands ") as directory:
            root = Path(directory)
            toolchain = root / "release/build/Release/generators/conan_toolchain.cmake"
            toolchain.parent.mkdir(parents=True)
            toolchain.touch()
            context = Mock()
            with patch.object(tasks, "BUILD_DIR", root), contextlib.redirect_stdout(io.StringIO()):
                tasks.configure.body(context, build_type="Release", profile="custom-profile")
            conan = shlex.split(context.run.call_args_list[0].args[0])
            cmake = shlex.split(context.run.call_args_list[1].args[0])
            self.assertEqual(conan[conan.index("-pr:h") + 1], "custom-profile")
            self.assertEqual(conan[conan.index("-of") + 1], str(root / "release"))
            self.assertIn(f"-DCMAKE_TOOLCHAIN_FILE={toolchain}", cmake)

    def test_build_parallelism_and_worker_target(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "release").mkdir()
            context = Mock()
            with patch.object(tasks, "BUILD_DIR", root), contextlib.redirect_stdout(io.StringIO()):
                tasks.build.body(context, build_type="Release")
                default = shlex.split(context.run.call_args.args[0])
                self.assertEqual(default[-1], "--parallel")
                tasks.build.body(context, build_type="Release", jobs=2)
                limited = shlex.split(context.run.call_args.args[0])
                self.assertEqual(limited[limited.index("--parallel") + 1], "2")
                tasks.run_worker.body(context, build_type="Release")
                worker_build = shlex.split(context.run.call_args_list[-2].args[0])
                self.assertIn("--parallel", worker_build)
                self.assertEqual(worker_build[worker_build.index("--target") + 1], "occccad_geometry_worker")

    def test_clean_removes_generated_references_and_preserves_env(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "build").mkdir()
            (root / "compile_commands.json").symlink_to(root / "build/missing.json")
            (root / "CMakeUserPresets.json").write_text("{}")
            (root / ".env").write_text("OCCCCAD_BUILD_TYPE=Release\n")
            with patch.object(tasks, "PROJECT_ROOT", root), contextlib.redirect_stdout(io.StringIO()):
                tasks.clean.body(None)
                tasks.clean.body(None)
            self.assertFalse((root / "build").exists())
            self.assertFalse((root / "compile_commands.json").is_symlink())
            self.assertFalse((root / "CMakeUserPresets.json").exists())
            self.assertEqual((root / ".env").read_text(), "OCCCCAD_BUILD_TYPE=Release\n")

    def test_app_builds_services_together_then_starts_selected_type(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "build/cmake/release").mkdir(parents=True)
            context = Mock()
            context.cd.return_value = contextlib.nullcontext()
            with patch.multiple(tasks, PROJECT_ROOT=root, BUILD_DIR=root / "build/cmake"), contextlib.redirect_stdout(io.StringIO()):
                tasks.run_app.body(context, build_type="Release")
            self.assertEqual(context.run.call_count, 3)
            go = shlex.split(context.run.call_args_list[1].args[0])
            self.assertEqual(go[:4], ["go", "build", "-o", str(root / "build/services")])
            self.assertEqual(go[4:], ["./cmd/occccad-server", "./cmd/occccad-jobs", "./cmd/occccad-control"])
            self.assertEqual(context.run.call_args.args[0], str(root / "build/services/occccad-control"))
            self.assertEqual(context.run.call_args.kwargs["env"]["OCCCCAD_BUILD_TYPE"], "Release")


if __name__ == "__main__":
    unittest.main()
