"""Regression tests for the contract facility (not product capability evidence)."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("contract_runner", HERE / "runner.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class CatalogTests(unittest.TestCase):
    def setUp(self):
        self.catalog = json.loads((HERE / "catalog.json").read_text())
        self.lock = json.loads((HERE / "baseline.json").read_text())

    def check_bad(self, mutate, message, lock=False):
        changed = copy.deepcopy(self.catalog)
        mutate(changed)
        with self.assertRaisesRegex((ValueError, KeyError), message):
            runner.validate(changed, lock=self.lock if lock else None)

    def test_catalog_and_regression_lock(self):
        runner.validate(self.catalog, lock=self.lock)

    def test_duplicates_and_broken_references(self):
        self.check_bad(lambda c: c["capabilities"].append(c["capabilities"][0]), "duplicate capabilityId")
        self.check_bad(lambda c: c["cases"].append(c["cases"][0]), "duplicate caseId")
        self.check_bad(lambda c: c["capabilities"][0]["caseIds"].append("typo"), "invalid test mapping")
        self.check_bad(lambda c: c["cases"][0]["capabilityIds"].append("typo"), "invalid capability reference")

    def test_semantics_fixture_and_selector_are_required(self):
        self.check_bad(lambda c: c["policies"]["spatial"].pop("parameters"), "parameters")
        self.check_bad(lambda c: c["policies"]["spatial"]["parameters"][0].pop("unit"), "parameter semantics")
        self.check_bad(lambda c: c["descriptors"]["POINT"]["units"].clear(), "unit fields")
        self.check_bad(lambda c: c["cases"][0]["fixture"].update(source="missing"), "missing fixture")
        def typo(c):
            c["cases"][0]["selector"] = "AssemblySolver.Typo"
            c["cases"][0]["fixture"]["symbol"] = "AssemblySolver.Typo"
        self.check_bad(typo, r"missing C\+\+ test")

    def test_implemented_layer_cannot_have_no_test(self):
        self.check_bad(lambda c: c["capabilities"][0].update(caseIds=[]), "has no executable test")

    def test_regression_lock_prevents_target_removal_and_weakening(self):
        self.check_bad(lambda c: c["policies"]["spatial"].update(direction="match current solver instead"), "regression lock changed", lock=True)
        self.check_bad(lambda c: c["cases"][0].update(expectation="Converged is enough"), "regression lock", lock=True)
        self.check_bad(lambda c: c["implementationProfiles"]["existing"]["workerSolver"].update(state="partial"), "coverage lowered", lock=True)
        # Remove a missing target without leaving broken references: still rejected.
        def delete_target(c):
            removed = "coincidence.frame-frame"
            c["capabilities"] = [r for r in c["capabilities"] if r["capabilityId"] != removed]
            for case in c["cases"]:
                case["capabilityIds"] = [ident for ident in case["capabilityIds"] if ident != removed]
        self.check_bad(delete_target, "removed/changed", lock=True)

    def test_unknown_or_empty_selection_fails(self):
        for options in ({"case": "typo"}, {"family": "Concentric"}, {"capability": "typo"}, {"layer": "browser"}, {"capability": "coincidence.frame-frame", "layer": "ui"}):
            with self.assertRaises(ValueError):
                runner.select(self.catalog, **options)

    def test_go_results_require_actual_run_and_no_skip(self):
        selector = "TestReal"
        self.assertEqual(runner.go_verdict([], selector, 0)[0], "FAIL")
        self.assertEqual(runner.go_verdict([{"Test": selector, "Action": "pass"}], selector, 0)[0], "FAIL")
        events = [{"Test": selector, "Action": "run"}, {"Test": selector, "Action": "pass"}]
        self.assertEqual(runner.go_verdict(events, selector, 0)[0], "PASS")
        self.assertEqual(runner.go_verdict(events, selector, 1)[0], "FAIL")
        events.append({"Test": selector + "/integration", "Action": "skip"})
        self.assertEqual(runner.go_verdict(events, selector, 0)[0], "ENVIRONMENT_BLOCKED")
        events.append({"Test": selector + "/integration", "Action": "fail"})
        self.assertEqual(runner.go_verdict(events, selector, 0)[0], "FAIL")

    def test_missing_environment_is_not_mock_pass(self):
        from unittest.mock import patch
        case = {"caseId": "integration.example", "adapter": "integration", "requires": ["CONTRACT_TEST_MISSING_ENV"]}
        with patch.dict("os.environ", {}, clear=True):
            result, commands = runner.run_cases([case], HERE, "Debug", {"integration"})
        self.assertEqual(result[case["caseId"]]["status"], "ENVIRONMENT_BLOCKED")
        self.assertEqual(commands, [])

    def test_assertion_source_changes_fail_the_lock(self):
        from unittest.mock import patch
        with patch.object(runner, "assertion_source", return_value="EXPECT_TRUE(true); // loosened assertion"):
            with self.assertRaisesRegex(ValueError, "regression lock"):
                runner.validate(self.catalog, lock=self.lock)

    def test_zero_execution_cannot_be_green(self):
        import contextlib
        import io
        import tempfile
        with tempfile.TemporaryDirectory() as output:
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                code = runner.main(["baseline", "--case", "offset.plane-plane.first-normal-editor", "--output", output])
            self.assertEqual(code, 2)


if __name__ == "__main__":
    unittest.main()
