"""Regression tests for the contract facility (not product capability evidence)."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest
from catalog import DATA, compose_catalog

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("contract_runner", HERE / "runner.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class CatalogTests(unittest.TestCase):
    def test_split_catalog_binding_and_semantic_ownership(self):
        production=json.loads((HERE / "../../services/internal/assemblycontract/catalog.json").read_text())
        evidence=json.loads((DATA / "evidence.json").read_text())
        combined=compose_catalog(production,evidence)
        self.assertEqual(len(combined["cases"]),len(evidence["cases"]))
        self.assertNotIn("cases",production)
        self.assertNotIn("caseIds",production["capabilities"][0])
        self.assertLess(len(evidence["bindings"]),len(evidence["cases"]))
        bad=copy.deepcopy(evidence);bad["cases"][0]["bindingId"]="absent"
        with self.assertRaisesRegex(ValueError,"missing binding"):compose_catalog(production,bad)
        bad=copy.deepcopy(evidence);bad["bindings"]["orphan"]={}
        with self.assertRaisesRegex(ValueError,"unused test binding"):compose_catalog(production,bad)
        bad=copy.deepcopy(evidence);bad["capabilityEvidence"][production["capabilities"][0]["capabilityId"]]["roles"]=[]
        with self.assertRaisesRegex(ValueError,"overwrites production"):compose_catalog(production,bad)
    def setUp(self):
        self.catalog = runner.load_catalog()
        self.lock = json.loads((DATA / "baseline.json").read_text())

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
        self.check_bad(lambda c: c["derivedSupports"].append(c["derivedSupports"][0]), "duplicate derived supportId")
        self.check_bad(lambda c: c["capabilities"][0]["roles"][0].update(supportedDescriptors=["MESH_FACE"]), "invalid supported descriptor")

    def test_semantics_fixture_and_selector_are_required(self):
        self.check_bad(lambda c: c["policies"]["spatial"].pop("parameters"), "parameters")
        self.check_bad(lambda c: c["policies"]["spatial"]["parameters"][0].pop("unit"), "parameter semantics")
        self.check_bad(lambda c: c["descriptors"]["POINT"]["units"].clear(), "unit fields")
        self.check_bad(lambda c: c["cases"][0]["fixture"].update(source="missing"), "missing fixture")
        self.check_bad(lambda c: c["cases"][0]["fixture"].update(assertionSources=[{"source": "missing", "symbol": "missing"}]), "missing additional assertion fixture")
        def typo(c):
            case = next(case for case in c["cases"] if case["adapter"] == "cpp")
            case["selector"] = "AssemblySolver.Typo"
            case["fixture"]["symbol"] = "AssemblySolver.Typo"
        self.check_bad(typo, r"missing C\+\+ test")

    def test_implemented_layer_cannot_have_no_test(self):
        self.check_bad(lambda c: c["capabilities"][0].update(caseIds=[]), "has no executable test")

    def test_regression_lock_prevents_target_removal_and_weakening(self):
        self.check_bad(lambda c: c["policies"]["spatial"].update(direction="match current solver instead"), "regression lock changed", lock=True)
        self.check_bad(lambda c: next(case for case in c["cases"] if case["adapter"] == "cpp").update(expectation="Converged is enough"), "regression lock", lock=True)
        self.check_bad(lambda c: c["capabilities"][0].setdefault("implementationOverrides", {}).update(workerSolver={"state": "partial", "reason": "weakened"}), "coverage lowered", lock=True)
        # Remove a missing target without leaving broken references: still rejected.
        def delete_target(c):
            removed = "coincidence.frame-frame"
            c["capabilities"] = [r for r in c["capabilities"] if r["capabilityId"] != removed]
            for case in c["cases"]:
                case["capabilityIds"] = [ident for ident in case["capabilityIds"] if ident != removed]
            c["cases"] = [case for case in c["cases"] if case["capabilityIds"]]
            remaining = {case["caseId"] for case in c["cases"]}
            for capability in c["capabilities"]:
                capability["caseIds"] = [ident for ident in capability["caseIds"] if ident in remaining]
                if "requiredCaseIds" in capability:
                    capability["requiredCaseIds"] = [ident for ident in capability["requiredCaseIds"] if ident in remaining]
        self.check_bad(delete_target, "removed/changed", lock=True)

    def test_unknown_or_empty_selection_fails(self):
        for options in ({"case": "typo"}, {"family": "Concentric"}, {"capability": "typo"}, {"layer": "browser"}):
            with self.assertRaises(ValueError):
                runner.select(self.catalog, **options)
        empty = copy.deepcopy(self.catalog)
        empty["cases"] = []
        with self.assertRaisesRegex(ValueError, "empty"):
            runner.select(empty, capability="coincidence.frame-frame", layer="ui")

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

    def test_child_pass_cannot_hide_failed_fixture_ancestor(self):
        events = [{"Test": "TestReal/good", "Action": "run"},
                  {"Test": "TestReal/good", "Action": "pass"},
                  {"Test": "TestReal/bad", "Action": "run"},
                  {"Test": "TestReal/bad", "Action": "fail"},
                  {"Test": "TestReal", "Action": "fail"}]
        self.assertEqual(runner.go_verdict(events, "TestReal/good", 1)[0], "FAIL")
        self.assertEqual(runner.go_verdict(events, "TestReal/bad", 1)[0], "FAIL")
        self.assertEqual(runner.go_verdict(events, "TestReal", 1)[0], "FAIL")

    def test_ancestor_skip_and_nested_ancestor_failure_are_not_pass(self):
        selector = "TestReal/group/child"
        events = [{"Test": selector, "Action": "run"}, {"Test": selector, "Action": "pass"}]
        for ancestor in ("TestReal", "TestReal/group"):
            self.assertEqual(runner.go_verdict(events + [{"Test": ancestor, "Action": "skip"}], selector, 0)[0], "ENVIRONMENT_BLOCKED")
            self.assertEqual(runner.go_verdict(events + [{"Test": ancestor, "Action": "fail"}], selector, 1)[0], "FAIL")

    def test_truly_unrelated_fixture_failure_does_not_relabel_pass(self):
        selector = "TestReal/good"
        events = [{"Test": selector, "Action": "run"}, {"Test": selector, "Action": "pass"},
                  {"Test": "TestReal", "Action": "pass"}, {"Test": "TestOther/bad", "Action": "fail"},
                  {"Test": "TestOther", "Action": "fail"}]
        self.assertEqual(runner.go_verdict(events, selector, 1)[0], "PASS")

    def test_progress_requires_specific_applicable_actual_evidence(self):
        c = copy.deepcopy(self.catalog["capabilities"][0])
        catalog = copy.deepcopy(self.catalog)
        for layer in runner.LAYERS:
            c.setdefault("implementationOverrides", {})[layer] = {"state": "implemented", "reason": "test declaration"}
        c["requiredLayers"] = ["workerSolver", "historyReplay"]
        catalog["cases"] = [dict(caseId="math", capabilityIds=[c["capabilityId"]], layer="workerSolver", evidenceScope="specific-combination"),
                            dict(caseId="db", capabilityIds=[c["capabilityId"]], layer="historyReplay", evidenceScope="specific-combination")]
        results = {"math": {"status": "PASS"}, "db": {"status": "ENVIRONMENT_BLOCKED"}}
        p = runner.progress(c, catalog, results)
        self.assertEqual(p["targetStatus"], "IMPLEMENTED")
        self.assertEqual(p["acceptanceStatus"], "NOT_ACCEPTED")
        self.assertEqual(p["outstandingEvidence"], {"db": "ENVIRONMENT_BLOCKED"})
        results["db"]["status"] = "PASS"
        self.assertEqual(runner.progress(c, catalog, results)["acceptanceStatus"], "ACCEPTED")
        for evidence in ({"evidenceScope": "representative-shared-foundation"}, {"purpose": "unsupported-rejection"}):
            altered = copy.deepcopy(catalog)
            altered["cases"][1].update(evidence)
            self.assertEqual(runner.progress(c, altered, results)["acceptanceStatus"], "NOT_ACCEPTED")
        results.pop("db")
        self.assertEqual(runner.progress(c, catalog, results)["outstandingEvidence"], {"db": "NOT_RUN"})
        c["implementationOverrides"]["historyReplay"]["state"] = "missing"
        self.assertEqual(runner.progress(c, catalog, results)["targetStatus"], "PARTIAL")

    def test_missing_environment_is_not_mock_pass(self):
        from unittest.mock import patch
        case = {"caseId": "integration.example", "adapter": "integration", "requires": ["CONTRACT_TEST_MISSING_ENV"]}
        with patch.dict("os.environ", {}, clear=True):
            result, commands = runner.run_cases([case], HERE, "Debug", {"integration"})
        self.assertEqual(result[case["caseId"]]["status"], "ENVIRONMENT_BLOCKED")
        self.assertEqual(commands, [])

    def test_environment_identity_never_records_credentials(self):
        from unittest.mock import patch
        with patch.dict("os.environ", {"OCCCCAD_TEST_DATABASE_URL": "postgresql://actor:secret@localhost/dedicated_contract_test?sslmode=disable", "OCCCCAD_TEST_GEOMETRY_WORKER": "/missing-worker"}, clear=True):
            environment = runner.integration_environment()
        self.assertEqual(environment["databaseName"], "dedicated_contract_test")
        self.assertIsNone(environment["workerExecutableDigest"])
        self.assertNotIn("secret", json.dumps(environment))

    def test_integration_failure_does_not_relabel_another_fixture(self):
        import tempfile
        from unittest.mock import patch
        cases = [dict(caseId=name.lower(), adapter="integration", requires=[], package="./internal/control", selector=name)
                 for name in ("TestPassing", "TestFailing")]
        def execute(argv, cwd, env, log):
            name = "TestFailing" if "TestFailing" in argv[-1] else "TestPassing"
            failed = name == "TestFailing"
            events = [{"Test": name, "Action": "run"}, {"Test": name, "Action": "fail" if failed else "pass"}]
            return int(failed), "\n".join(json.dumps(event) for event in events)
        with tempfile.TemporaryDirectory() as output, patch.object(runner, "command", side_effect=execute):
            results, commands = runner.run_cases(cases, Path(output), "Debug", {"integration"})
        self.assertEqual(len(commands), 2)
        self.assertEqual(results["testpassing"]["status"], "PASS")
        self.assertEqual(results["testfailing"]["status"], "FAIL")

    def test_assertion_source_changes_fail_the_lock(self):
        from unittest.mock import patch
        with patch.object(runner, "assertion_source", return_value='EXPECT_TRUE(true); t.Fatal("changed assertion");'):
            with self.assertRaisesRegex(ValueError, "regression lock"):
                runner.validate(self.catalog, lock=self.lock)

    def test_web_marker_is_exact_and_process_must_succeed(self):
        import tempfile
        from unittest.mock import patch
        case = next(c for c in self.catalog["cases"] if c["adapter"] == "web-catalog")
        for code, raw, want in ((0, "CONTRACT_PASS " + case["caseId"] + "-wrong", "FAIL"),
                                (1, "CONTRACT_PASS " + case["caseId"], "FAIL"),
                                (0, "CONTRACT_PASS " + case["caseId"], "PASS")):
            with tempfile.TemporaryDirectory() as output, patch.object(runner, "command", return_value=(code, raw)):
                results, _ = runner.run_cases([case], Path(output), "Debug", {"web-catalog"})
            self.assertEqual(results[case["caseId"]]["status"], want)

    def test_web_batches_receive_only_their_catalog_cases(self):
        import tempfile
        from unittest.mock import patch
        legacy = next(c for c in self.catalog["cases"] if c["caseId"] == "angle.free.web-entry")
        production = next(c for c in self.catalog["cases"] if c["selector"] == "production-capability")
        def execute(argv, cwd, env, log):
            ids = json.loads(env["OCCCCAD_ASSEMBLY_CONTRACT_CASES"])
            self.assertEqual(len(ids), 1)
            return 0, "CONTRACT_PASS " + ids[0]
        with tempfile.TemporaryDirectory() as output, patch.object(runner, "command", side_effect=execute):
            results, commands = runner.run_cases([legacy, production], Path(output), "Debug", {"web-catalog"})
        self.assertEqual(len(commands), 2)
        self.assertTrue(all(r["status"] == "PASS" for r in results.values()))

    def test_composition_rejects_blocked_or_unexecuted_required_evidence(self):
        import tempfile
        import contextlib
        import io
        from unittest.mock import patch
        case = next(c for c in self.catalog["cases"] if c["adapter"] == "web-catalog")
        for status in ("PASS", "ENVIRONMENT_BLOCKED"):
            results = {case["caseId"]: dict(caseId=case["caseId"], status=status, observed="facility fixture", command=None, log=None)}
            with tempfile.TemporaryDirectory() as output, patch.object(runner, "run_cases", return_value=(results, [])), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                code = runner.main(["composition", "--capability", case["capabilityIds"][0], "--output", output])
            self.assertEqual(code, 1)  # Even a real UI PASS cannot stand in for required DB/math evidence.

    def test_assertion_helper_must_exist_and_be_called(self):
        case = copy.deepcopy(next(c for c in self.catalog["cases"] if c["adapter"] == "integration"))
        case["fixture"]["assertionHelperSymbol"] = "missingContractAssertionHelper"
        with self.assertRaisesRegex(ValueError, "not called"):
            runner.assertion_source(case)
        case["fixture"].pop("assertionHelperSymbol")
        case["fixture"]["assertionSources"] = [{"source": case["fixture"]["source"], "symbol": "missingContractAssertionHelper"}]
        with self.assertRaisesRegex(ValueError, "missing additional assertion"):
            runner.assertion_source(case)

    def test_zero_execution_cannot_be_green(self):
        import contextlib
        import io
        import tempfile
        with tempfile.TemporaryDirectory() as output:
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                code = runner.main(["baseline", "--case", "offset.plane-plane.first-normal-editor", "--adapters", "go", "--output", output])
            self.assertEqual(code, 2)

    def test_milestone_requires_all_actual_evidence(self):
        catalog={"cases":[{"caseId":"m4.one"},{"caseId":"m4.two"},{"caseId":"m5.one"}]}
        evidence=runner.milestone_evidence(catalog,{"m4.one":{"status":"PASS"}})
        self.assertEqual(evidence["M4"]["status"],"INCOMPLETE")
        self.assertEqual(evidence["M4"]["requiredEvidence"]["m4.two"],"NOT_RUN")
        for verdict in ("FAIL","ENVIRONMENT_BLOCKED","NOT_RUN"):
            self.assertEqual(runner.milestone_evidence(catalog,{"m4.one":{"status":"PASS"},"m4.two":{"status":verdict}})["M4"]["status"],"INCOMPLETE")
        done=runner.milestone_evidence(catalog,{"m4.one":{"status":"PASS"},"m4.two":{"status":"PASS"}})
        self.assertEqual(done["M4"]["status"],"ALL_REQUIRED_PASS")
        self.assertEqual(done["M4"]["manualAcceptance"],"PENDING_MAINTAINER")
        self.assertEqual(runner.milestone_evidence({"cases":[]},{})["M4"]["status"],"INCOMPLETE")


if __name__ == "__main__":
    unittest.main()
