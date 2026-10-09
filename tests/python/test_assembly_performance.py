"""Assembly performance command and statistics, without running benchmarks."""
import importlib.util
import json
from pathlib import Path
import shlex
import unittest
from unittest.mock import Mock

import tasks

spec = importlib.util.spec_from_file_location(
    "assembly_performance", Path(__file__).resolve().parents[2] / "kernel/assembly/tests/run_performance.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class AssemblyPerformanceTests(unittest.TestCase):
    def test_command_forwards_samples_and_build_without_building(self):
        context = Mock()
        tasks.performance_assembly.body(context, samples=7, build_type="Release")
        context.run.assert_called_once()
        command = shlex.split(context.run.call_args.args[0])
        self.assertEqual(command[command.index("--samples") + 1], "7")
        self.assertEqual(command[command.index("--build-dir") + 1], str(tasks._get_build_dir("Release")))
        self.assertEqual(command[-3:], ["--stages", "static", "interaction"])

    def test_static_mean_and_interaction_median_use_distinct_units(self):
        static = "AssemblyPlaneChain5 3 1000000 ns/op valid=true rank=12\n"
        samples = [dict(kind="sample", scene="single", kernel_ms=value, valid=True, eligible=True, compile_ms=0.1)
                   for value in [3, 1, 2]]
        rows = runner.native_summary(static, "\n".join(json.dumps(sample) for sample in samples))
        self.assertEqual(rows[0]["statistic"], "mean")
        self.assertEqual(rows[0]["total_ms"], 1)
        self.assertEqual(rows[1]["statistic"], "median")
        self.assertEqual(rows[1]["total_ms"], 2)
        self.assertEqual(rows[1]["samples_ms"], [3, 1, 2])
        self.assertEqual(rows[1]["phases_ms"]["compile_ms"], 0.1)

    def test_expected_failure_is_valid_but_ineligible(self):
        sample = json.dumps(dict(kind="sample", scene="budget", kernel_ms=1, valid=True, eligible=False))
        self.assertFalse(runner.native_summary("", sample)[0]["eligible"])
        invalid = json.dumps(dict(kind="sample", scene="single", kernel_ms=1, valid=False, eligible=False))
        with self.assertRaisesRegex(ValueError, "Invalid interaction"):
            runner.native_summary("", invalid)


if __name__ == "__main__":
    unittest.main()
