#!/usr/bin/env python3
"""Execute catalog-selected existing implementations; never infer PASS from coverage."""
from __future__ import annotations

import argparse
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent))
from catalog import DATA, load_catalog
import xml.etree.ElementTree as ET
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
LAYERS = {"ui", "domain", "resolution", "workerSolver", "lifecycle", "historyReplay"}
ADAPTERS = {"cpp", "go", "go-flags", "web", "web-catalog", "integration"}
STATES = {"implemented", "partial", "missing", "unknown"}
TASK_TOPICS = {
    "CONSTRAINT-ACTIVATION": "激活模式连接和求值状态正交",
    "CONSTRAINT-GEOMETRY": "支持几何与参数门",
    "CONSTRAINT-COINCIDENCE": "coincidence",
    "CONSTRAINT-OFFSET": "offset-与-measure",
    "CONSTRAINT-ANGLE": "angle空间角指定轴角与方向关系",
    "CONSTRAINT-FIX": "fix-与-fix-together",
    "CONSTRAINT-FIX-TOGETHER": "fix-与-fix-together",
    "CONSTRAINT-CONTACT": "contact",
    "CONSTRAINT-COMPOSITION": "自由度控制与组合能力",
}


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest()


def integration_environment():
    worker = os.getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
    worker_digest = None
    if worker and Path(worker).is_file():
        with Path(worker).open("rb") as stream:
            worker_digest = hashlib.file_digest(stream, "sha256").hexdigest()
    database = unquote(urlsplit(os.getenv("OCCCCAD_TEST_DATABASE_URL", "")).path).lstrip("/") or None
    # Record reproducible build/database identity, never URL credentials.
    return {"workerExecutable": worker, "workerExecutableDigest": worker_digest, "databaseName": database,
            "integrationVariablesPresent": {k: bool(os.getenv(k)) for k in ("OCCCCAD_TEST_DATABASE_URL", "OCCCCAD_TEST_GEOMETRY_WORKER")}}


def frozen_contract(catalog):
    # A reviewable regression lock, not another coverage matrix. New IDs may be added;
    # deleting IDs, changing target expectations or lowering a coverage floor fails.
    return {
        "schemaVersion": catalog["schemaVersion"],
        "contractVersion": catalog["contractVersion"],
        "semantics": digest({k: catalog[k] for k in ("families", "layers", "descriptors", "derivedSupports", "policies", "failurePolicies", "tolerances")}),
        "capabilities": {c["capabilityId"]: digest({k: v for k, v in c.items() if k not in ("caseIds", "implementationProfile", "implementationOverrides")}) for c in catalog["capabilities"]},
        "cases": {c["caseId"]: digest(c) for c in catalog["cases"]},
        "assertions": {c["caseId"]: digest(assertion_source(c)) for c in catalog["cases"]},
        "coverageFloor": {c["capabilityId"]: {layer: value["state"] for layer, value in implementation(c, catalog).items()} for c in catalog["capabilities"]},
    }


def assertion_source(case):
    source = (ROOT / case["fixture"]["source"]).read_text()
    extra = ""
    for ref in case["fixture"].get("assertionSources", []):
        text = (ROOT / ref["source"]).read_text()
        match = re.search(r"\b(?:func|void)\s+" + re.escape(ref["symbol"]) + r"\s*\(", text)
        if match is None:
            raise ValueError("missing additional assertion source " + ref["symbol"])
        extra += "\n" + re.split(r"\n(?:func |TEST\(|}\s*(?:\n|$))", text[match.start():], maxsplit=1)[0]
    if case["adapter"] == "cpp":
        suite, test = case["selector"].split(".")
        start = re.search(r"TEST\(\s*" + re.escape(suite) + r"\s*,\s*" + re.escape(test) + r"\s*\)", source)
        body = re.split(r"\nTEST\(", source[start.start():], maxsplit=1)[0]
        # Explicit shared numeric fixtures are legitimate assertions, not empty
        # wrappers. Include their source in the regression lock and validate it.
        helper = case["fixture"].get("assertionHelperSymbol")
        if helper:
            match = re.search(r"\bvoid\s+" + re.escape(helper) + r"\s*\(", source)
            if match is None:
                raise ValueError("missing C++ assertion helper " + helper)
            body += "\n" + re.split(r"\n}\s*(?:\n|$)", source[match.start():], maxsplit=1)[0]
        return body + extra
    if case["adapter"] in {"go", "go-flags", "integration"}:
        start = source.index("func " + case["selector"].split("/")[0] + "(")
        body = re.split(r"\nfunc ", source[start:], maxsplit=1)[0]
        helper = case["fixture"].get("assertionHelperSymbol")
        if helper:
            if not re.search(r"\b" + re.escape(helper) + r"\s*\(", body):
                raise ValueError("Go assertion helper is not called: " + helper)
            start = source.index("func " + helper + "(")
            body += "\n" + re.split(r"\nfunc ", source[start:], maxsplit=1)[0]
        return body + extra
    return source + extra


def implementation(capability, catalog):
    return {**catalog["implementationProfiles"][capability["implementationProfile"]], **capability.get("implementationOverrides", {})}


def progress(capability, catalog, results):
    """Coverage is a declaration; acceptance requires specific, actually run evidence."""
    required = capability.get("requiredLayers", catalog["layers"])
    coverage = implementation(capability, catalog)
    states = [coverage[layer]["state"] for layer in required]
    implemented = all(state == "implemented" for state in states)
    target = "IMPLEMENTED" if implemented else "TARGET_NOT_IMPLEMENTED" if all(state in {"missing", "unknown"} for state in states) else "PARTIAL"
    applicable = [t for t in catalog["cases"] if capability["capabilityId"] in t["capabilityIds"]
                  and t.get("purpose") != "unsupported-rejection" and t["evidenceScope"] == "specific-combination"]
    missing = [layer for layer in required if not any(t["layer"] == layer for t in applicable)]
    required_ids = set(capability.get("requiredCaseIds", [])) | {t["caseId"] for t in applicable if t["layer"] in required}
    verdicts = {ident: results.get(ident, {"status": "NOT_RUN"})["status"] for ident in sorted(required_ids)}
    outstanding = {ident: status for ident, status in verdicts.items() if status != "PASS"}
    verified = not missing and bool(required_ids) and not outstanding
    return {"targetStatus": target, "requiredLayers": required, "missingSpecificTestLayers": missing,
            "requiredEvidence": verdicts, "outstandingEvidence": outstanding,
            "verificationStatus": "VERIFIED" if verified else "INCOMPLETE",
            "acceptanceStatus": "ACCEPTED" if implemented and verified else "NOT_ACCEPTED"}


def validate(catalog, root=ROOT, lock=None):
    def require(condition, message):
        if not condition:
            raise ValueError(message)

    require(catalog["schemaVersion"] == 1, "unknown schemaVersion")
    require(bool(catalog["contractVersion"]), "missing contractVersion")
    require(set(catalog["families"]) == {"Coincidence", "Contact", "Offset", "Angle", "Fix", "FixTogether"}, "six families required")
    require(set(catalog["layers"]) == LAYERS, "coverage layers incomplete")
    require((root / catalog["target"]).is_file(), "missing target contract")
    target = (root / catalog["target"]).read_text()
    slug = lambda s: re.sub(r"[^\w\s-]", "", s.lower()).replace(" ", "-")
    anchors = {slug(h) for h in re.findall(r"^#{1,6}\s+(.+)$", target, re.M)}
    caps, cases = catalog["capabilities"], catalog["cases"]
    require(bool(caps) and bool(cases), "empty catalog")
    cap_ids = [c["capabilityId"] for c in caps]
    case_ids = [c["caseId"] for c in cases]
    require(len(set(cap_ids)) == len(cap_ids), "duplicate capabilityId")
    require(len(set(case_ids)) == len(case_ids), "duplicate caseId")
    require(all(re.fullmatch(r"[a-z][a-z0-9.-]+", i) for i in cap_ids + case_ids), "invalid stable ID")
    for name, descriptor in catalog["descriptors"].items():
        require(descriptor["fields"] and descriptor["preconditions"], f"descriptor {name} incomplete")
        require(set(descriptor["fields"]) == set(descriptor["units"]), f"descriptor {name} unit fields missing")
    # Stable catalog task identities outlive completed planning documents.
    # Navigation follows topic ownership; no numerical expectation or ID changes.
    tasks = TASK_TOPICS
    require(set(tasks.values()).issubset(anchors), "broken task topic navigation")
    require((root / "docs/architecture/current/assembly-constraints.md").is_file(), "missing current constraint contract")
    for policy in catalog["failurePolicies"].values():
        require(len({f["category"] for f in policy}) == len(policy), "duplicate failure category")
        require(all(all(k in f for k in ("category", "phase", "saveDefinition", "advanceHead", "adoptCandidatePose", "evaluation", "recovery")) for f in policy), "incomplete failure semantics")
    # Stable derived roles must not silently shadow another source contract.
    require(len({s["supportId"] for s in catalog["derivedSupports"]}) == len(catalog["derivedSupports"]), "duplicate derived supportId")
    for support in catalog["derivedSupports"]:
        require(support["sourceDescriptor"] in catalog["descriptors"] and support["resultDescriptor"] in catalog["descriptors"] and support["precondition"] and support["task"] in tasks, "invalid derived support")
    for c in caps:
        ident = c["capabilityId"]
        require(c["family"] in catalog["families"] and c["subtype"], f"{ident}: invalid family/subtype")
        require(c["targetAnchor"] in anchors, f"{ident}: invalid target anchor")
        require(c["arity"]["kind"] in {"unary", "binary", "group"}, f"{ident}: invalid arity")
        require(c["arity"]["min"] >= 1 and c["roles"], f"{ident}: missing roles")
        require(all(r["descriptor"] in catalog["descriptors"] and r["role"] for r in c["roles"]), f"{ident}: invalid descriptor")
        require(all(not ("supportedDescriptors" in role) or (role["supportedDescriptors"] and set(role["supportedDescriptors"]).issubset(catalog["descriptors"])) for role in c["roles"]), f"{ident}: invalid supported descriptor")
        policy = catalog["policies"][c["policy"]]
        for key in ("direction", "sign", "exchange", "branch", "modes", "otherModes", "activation", "failurePolicy"):
            require(bool(policy[key]), f"{ident}: missing {key}")
        for p in policy["parameters"]:
            require(all(p.get(k) for k in ("name", "type", "unit", "domain", "dependsOn")), f"{ident}: missing parameter semantics")
        require(policy["failurePolicy"] in catalog["failurePolicies"], f"{ident}: failure policy missing")
        require(c["rank"]["configuration"] and (c["rank"]["normal"] is None or 0 <= c["rank"]["normal"] <= 6), f"{ident}: rank missing")
        require(c["rank"]["normal"] is not None or c.get("openQuestions") or c["rank"]["special"], f"{ident}: unknown rank has no boundary")
        require(c["followupTasks"] and all(t in tasks for t in c["followupTasks"]), f"{ident}: invalid followup task")
        require(bool(c.get("requiredLayers", catalog["layers"])) and set(c.get("requiredLayers", catalog["layers"])).issubset(LAYERS), f"{ident}: invalid required layers")
        require(set(c.get("requiredCaseIds", [])).issubset(c["caseIds"]), f"{ident}: invalid required evidence")
        require(all(i in case_ids for i in c["caseIds"]), f"{ident}: invalid test mapping")
        coverage = implementation(c, catalog)
        require(set(coverage) == LAYERS, f"{ident}: incomplete coverage")
        for layer, state in coverage.items():
            require(state["state"] in STATES and state["reason"], f"{ident}: invalid coverage")
            if state.get("source"):
                require((root / state["source"]).is_file(), f"{ident}: missing implementation source")
            if state["state"] == "implemented":
                require(any(t["caseId"] in c["caseIds"] and t["layer"] == layer and t.get("purpose") != "unsupported-rejection" for t in cases), f"{ident}: implemented {layer} has no executable test")
    for t in cases:
        ident = t["caseId"]
        for ref in t["fixture"].get("assertionSources", []):
            require(bool(ref.get("symbol")) and (root / ref.get("source", "")).is_file(), f"{ident}: missing additional assertion fixture")
        require(t["adapter"] in ADAPTERS and t["layer"] in LAYERS, f"{ident}: invalid adapter/layer")
        require(t["capabilityIds"] and all(i in cap_ids for i in t["capabilityIds"]), f"{ident}: invalid capability reference")
        require(t["expectation"] and t["selector"], f"{ident}: empty assertion/selector")
        require(t["fixture"]["symbol"] == t["selector"], f"{ident}: invalid fixture symbol")
        require(t["evidenceScope"] in {"specific-combination", "representative-shared-foundation"} and t["evidenceKind"], f"{ident}: missing evidence level")
        source = root / t["fixture"]["source"]
        require(source.is_file(), f"{ident}: missing fixture")
        contents = source.read_text()
        if t["adapter"] == "cpp":
            suite, test = t["selector"].split(".")
            require(re.search(r"TEST\(\s*" + re.escape(suite) + r"\s*,\s*" + re.escape(test) + r"\s*\)", contents), f"{ident}: missing C++ test")
            body = assertion_source(t)
            require(re.search(r"(?:ASSERT|EXPECT)_", body), f"{ident}: empty C++ assertion body")
        elif t["adapter"] in {"go", "integration", "go-flags"}:
            require(re.search(r"func " + re.escape(t["selector"].split("/")[0]) + r"\(", contents), f"{ident}: missing Go test")
            require(t["package"].startswith("./internal/"), f"{ident}: invalid Go package")
            body = assertion_source(t)
            require(re.search(r"t\.(Fatal|Error|Fail)", body), f"{ident}: empty Go assertion body")
            if t["adapter"] == "go-flags":
                require(set(t["expected"]) == {"direction", "distanceSide", "directedAngle"} and all(isinstance(v, bool) for v in t["expected"].values()), f"{ident}: invalid expected flags")
                require(set(t["input"]) == {"kind", "first", "second"}, f"{ident}: invalid capability inputs")
        elif t["adapter"] == "web-catalog":
            require(t["selector"] in {"entry", "clear-axis", "offset-normal", "production-capability"} and t["input"] and t["expected"], f"{ident}: invalid web adapter")
        else:
            require(t["selector"] == t["fixture"]["source"], f"{ident}: invalid web path")
        for c in caps:
            require((ident in c["caseIds"]) == (c["capabilityId"] in t["capabilityIds"]), f"{ident}: nonreciprocal mapping")
    if lock is not None:
        now = frozen_contract(catalog)
        for field in ("schemaVersion", "contractVersion", "semantics"):
            require(now[field] == lock[field], f"regression lock changed: {field}")
        for field in ("capabilities", "cases", "assertions"):
            for ident, expected in lock[field].items():
                require(now[field].get(ident) == expected, f"regression lock: removed/changed {ident}")
        weight = {"missing": 0, "unknown": 0, "partial": 1, "implemented": 2}
        for ident, layers in lock["coverageFloor"].items():
            for layer, state in layers.items():
                require(weight[now["coverageFloor"][ident][layer]] >= weight[state], f"coverage lowered: {ident}/{layer}")


def select(catalog, capability=None, family=None, layer=None, case=None):
    caps = catalog["capabilities"]
    for value, choices, label in ((capability, [c["capabilityId"] for c in caps], "capabilityId"), (family, catalog["families"], "family"), (layer, catalog["layers"], "layer"), (case, [t["caseId"] for t in catalog["cases"]], "caseId")):
        if value and value not in choices:
            raise ValueError(f"unknown {label}: {value}")
    picked = [c for c in caps if (not capability or c["capabilityId"] == capability) and (not family or c["family"] == family)]
    ids = {c["capabilityId"] for c in picked}
    tests = [t for t in catalog["cases"] if ids.intersection(t["capabilityIds"]) and (not layer or t["layer"] == layer) and (not case or t["caseId"] == case)]
    if not picked or ((case or layer) and not tests):
        raise ValueError("execution selection is empty")
    return picked, tests


def command(argv, cwd, env, log):
    try:
        process = subprocess.run(argv, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=300)
        output, code = process.stdout, process.returncode
    except (OSError, subprocess.TimeoutExpired) as exc:
        output, code = str(exc), -1
    log.parent.mkdir(parents=True, exist_ok=True)
    log.write_text(output)
    return code, output


def go_verdict(events, selector, code):
    matching = [e for e in events if e.get("Test") == selector]
    ancestor_names = {selector.rsplit("/", depth)[0] for depth in range(1, selector.count("/") + 1)}
    ancestors = [e for e in events if e.get("Test") in ancestor_names]
    # A child PASS is not complete fixture evidence if its containing test fails
    # or skips final validation. Preserve the child event in the raw log, but
    # conservatively reject acceptance of that fixture scope.
    if any(e["Action"] == "fail" for e in ancestors):
        return "FAIL", "mapped Go fixture ancestor failed; child PASS cannot certify the complete fixture"
    if any(e["Action"] == "skip" for e in ancestors):
        return "ENVIRONMENT_BLOCKED", "mapped Go fixture ancestor skipped; child PASS is not complete evidence"
    # Parent PASS cannot hide a skipped descendant (including integration subtests).
    descendants = [e for e in events if e.get("Test", "").startswith(selector + "/")]
    if any(e["Action"] == "fail" for e in matching + descendants):
        return "FAIL", "Go assertion/build/process failed"
    if any(e["Action"] == "skip" for e in matching + descendants):
        return "ENVIRONMENT_BLOCKED", "Go test or descendant skipped; not PASS"
    if not any(e["Action"] == "run" for e in matching) or not any(e["Action"] == "pass" for e in matching):
        return "FAIL", "mapped Go test did not execute (zero tests or stale mapping)"
    if code and not (code == 1 and any(e.get("Test") and e["Action"] == "fail" and e not in matching + descendants + ancestors for e in events)):
        return "FAIL", "Go process failed without an independently identified failing test"
    return "PASS", "actual Go assertions passed"


def run_cases(cases, output, build_type, adapters):
    results = {}
    env = os.environ.copy()
    env["OCCCCAD_ASSEMBLY_CONTRACT_CASES"] = json.dumps([t["caseId"] for t in cases])
    commands = []
    def record(t, status, reason, argv=None, log=None):
        results[t["caseId"]] = {"caseId": t["caseId"], "status": status, "observed": reason, "command": argv, "log": str(log.relative_to(ROOT)) if log and log.is_relative_to(ROOT) else str(log) if log else None}
    pending = []
    for t in cases:
        if t["adapter"] not in adapters:
            record(t, "NOT_RUN", "adapter excluded by execution scope")
        elif any(not env.get(key) for key in t.get("requires", [])):
            record(t, "ENVIRONMENT_BLOCKED", "required environment missing: " + ", ".join(key for key in t["requires"] if not env.get(key)))
        else:
            pending.append(t)
    cpp = [t for t in pending if t["adapter"] == "cpp"]
    if cpp:
        build_dir = ROOT / "build/cmake" / build_type.lower()
        argv = ["cmake", "--build", str(build_dir), "--target", "occcad_assembly_solver_scenarios", "--parallel", "2"]
        commands.append(argv)
        log = output / "cpp-build.log"
        code, _ = command(argv, ROOT, env, log)
        if code:
            for t in cpp:
                record(t, "FAIL", "C++ build failed; inspect build log", argv, log)
        else:
            xml = output / "cpp.xml"
            if xml.exists():
                xml.unlink()  # Only this runner-owned previous result, never accept stale XML.
            argv = [str(build_dir / "kernel/assembly/tests/occcad_assembly_solver_scenarios"), "--gtest_filter=" + ":".join(sorted({t["selector"] for t in cpp})), "--gtest_output=xml:" + str(xml)]
            commands.append(argv)
            log = output / "cpp.log"
            code, _ = command(argv, ROOT, env, log)
            actual = {e.attrib["classname"] + "." + e.attrib["name"]: e for e in ET.parse(xml).iter("testcase")} if xml.is_file() else {}
            for t in cpp:
                e = actual.get(t["selector"])
                if e is None:
                    record(t, "FAIL", "mapped C++ test did not execute", argv, log)
                elif e.find("failure") is not None:
                    record(t, "FAIL", e.find("failure").attrib.get("message", "assertion failed"), argv, log)
                elif e.attrib.get("status") != "run" or e.find("skipped") is not None:
                    record(t, "NOT_RUN", "C++ test skipped", argv, log)
                elif code:
                    record(t, "FAIL", "C++ process failed (another assertion or crash)", argv, log)
                else:
                    record(t, "PASS", "actual C++ geometry/rank assertions passed", argv, log)
    # Integration cases run once per native test, not in one package process:
    # an unrelated historical failure must not relabel a passing Router fixture.
    packages = sorted({(t["package"], t["selector"].split("/")[0] if t["adapter"] == "integration" else "") for t in pending if t["adapter"] in {"go", "go-flags", "integration"}})
    for index, (package, integration_selector) in enumerate(packages):
        batch = [t for t in pending if t.get("package") == package and (t["selector"].split("/")[0] == integration_selector if integration_selector else t["adapter"] != "integration")]
        names = sorted({t["selector"].split("/")[0] for t in batch})
        argv = ["go", "test", "-json", "-count=1", "-timeout=240s", package, "-run", "^(" + "|".join(map(re.escape, names)) + ")$"]
        commands.append(argv)
        log = output / (package.rsplit("/", 1)[1] + f"-{index}.log")
        code, raw = command(argv, ROOT / "services", env, log)
        events = []
        for line in raw.splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        for t in batch:
            verdict, reason = go_verdict(events, t["selector"], code)
            evidence = "".join(e.get("Output", "") for e in events if e.get("Test", "").startswith(t["selector"]))
            record(t, verdict, reason + (": " + evidence[-2500:] if verdict != "PASS" else ""), argv, log)
            results[t["caseId"]]["evidence"] = evidence[-6000:]
    paths = sorted({t["fixture"]["source"] for t in pending if t["adapter"] in {"web", "web-catalog"}})
    for index, source in enumerate(paths):
        batch = [t for t in pending if t["fixture"]["source"] == source]
        relative = str((ROOT / source).relative_to(ROOT / "web/apps/cad"))
        argv = ["pnpm", "test", "--", "--verbose", relative]
        commands.append(argv)
        log = output / f"web-{index}.log"
        batch_env = {**env, "OCCCCAD_ASSEMBLY_CONTRACT_CASES": json.dumps([t["caseId"] for t in batch])}
        code, raw = command(argv, ROOT / "web/apps/cad", batch_env, log)
        # The normal scenario runner rejects empty selection. The catalog adapter also
        # emits a per-case marker; success exit without that marker cannot certify it.
        for t in batch:
            marker = re.search(r"(?<!\S)CONTRACT_PASS\s+" + re.escape(t["caseId"]) + r"(?![a-z0-9.-])", raw)
            if code:
                record(t, "FAIL", "TypeScript scenario process failed", argv, log)
            elif t["adapter"] == "web-catalog" and marker:
                record(t, "PASS", "actual per-case TypeScript assertions passed", argv, log)
            elif t["adapter"] == "web-catalog" and not marker:
                record(t, "FAIL", "missing actual per-case web assertion evidence", argv, log)
            elif not re.search(r"PASS|passed", raw, re.I):
                record(t, "FAIL", "scenario runner returned no execution evidence", argv, log)
            else:
                record(t, "PASS", "actual TypeScript interaction/model assertions passed", argv, log)
    return results, commands


def milestone_evidence(catalog, results):
    """Derived execution evidence, not a second implementation matrix."""
    evidence = {}
    for milestone in ("m4", "m5"):
        required = sorted(c["caseId"] for c in catalog["cases"] if c["caseId"].startswith(milestone + "."))
        verdicts = {ident: results.get(ident, {"status": "NOT_RUN"})["status"] for ident in required}
        evidence[milestone.upper()] = {"scope": "FIRST_STAGE_AUTOMATED_CASES_ONLY", "requiredEvidence": verdicts,
            "status": "ALL_REQUIRED_PASS" if required and all(v == "PASS" for v in verdicts.values()) else "INCOMPLETE",
            "manualAcceptance": "PENDING_MAINTAINER"}
    return evidence


def make_report(catalog, caps, cases, results, commands, purpose, argv):
    git = lambda *args: subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()
    rows = []
    for c in caps:
        selected = [t for t in cases if c["capabilityId"] in t["capabilityIds"]]
        evidence = [{**t, "result": results[t["caseId"]]} for t in selected]
        coverage = implementation(c, catalog)
        unresolved = [layer for layer, state in coverage.items() if state["state"] != "implemented"]
        advancement = progress(c, catalog, results)
        layer_evidence = {layer: [t["caseId"] for t in selected if t["layer"] == layer and results[t["caseId"]]["status"] == "PASS" and t.get("purpose") != "unsupported-rejection"] for layer in sorted(LAYERS)}
        rows.append({"capabilityId": c["capabilityId"], "family": c["family"], "target": {**c, "policy": catalog["policies"][c["policy"]]}, "implementation": coverage, **advancement, "unresolvedLayers": unresolved, "verifiedCaseIdsByLayer": layer_evidence, "verification": evidence, "missingTestLayers": [layer for layer in LAYERS if not any(t["layer"] == layer and t.get("purpose") != "unsupported-rejection" for t in catalog["cases"] if c["capabilityId"] in t["capabilityIds"])], "testStatus": "MISSING_TEST" if not any(t.get("purpose") != "unsupported-rejection" for t in catalog["cases"] if c["capabilityId"] in t["capabilityIds"]) else "MAPPED", "followupTasks": c["followupTasks"], "followupLinks": {t: catalog["target"] + "#" + TASK_TOPICS[t] for t in c["followupTasks"]}})
    native = {t["fixture"]["source"] + "::" + (t["caseId"] if t["adapter"] == "web-catalog" else t["selector"]) for t in cases if results[t["caseId"]]["status"] == "PASS"}
    sources = {"tests/test.data/assembly-contract/catalog.json", "tests/assembly-contract/catalog.py", "tests/test.data/assembly-contract/evidence.json", "tests/assembly-contract/runner.py", "tests/test.data/assembly-contract/baseline.json"} | {t["fixture"]["source"] for t in cases}
    sources |= {ref["source"] for t in cases for ref in t["fixture"].get("assertionSources", [])}
    sources |= {state["source"] for c in caps for state in implementation(c, catalog).values() if state.get("source")}
    sources |= {"services/internal/assemblycontract/catalog.json", "proto/occccad/worker/v1/geometry_worker.proto",
                "kernel/assembly/src/contact.cpp", "kernel/assembly/src/contact.hpp",
                "services/internal/workspace/assembly_groups.go", "services/internal/workspace/assembly_manifest_digest.go",
                "services/internal/workspace/assembly_offset.go", "workers/geometry/src/main.cpp",
                "services/internal/workspace/assembly_geometry.go", "services/internal/workspace/assembly_contact.go",
                "services/internal/control/assembly_composition_quality_test.go",
                "services/internal/workspace/interaction_candidate.go", "services/internal/workspace/model_core.go",
                "services/internal/workspace/model.go", "services/internal/workspace/service.go",
                "web/apps/cad/src/cad/assembly/assembly-edit-intent.ts", "web/apps/cad/src/cad/interaction/selection-index.ts",
                "web/apps/cad/src/features/workbench/workbench.tsx", "web/apps/cad/src/viewport/cad-viewport-engine.ts",
                "web/apps/cad/src/viewport/cad-viewport.tsx", "web/apps/cad/src/types.ts"}
    sources |= {"kernel/assembly/src/solver.cpp", "services/internal/geometry/client.go",
                "services/internal/workspace/assembly_interaction.go", "services/internal/workspace/assembly_interaction_continuation.go", "services/internal/workspace/assembly_conflict.go",
                "services/internal/workspace/assembly_conflict_geometry.go", "services/internal/api/realtime_assembly.go",
                "web/apps/cad/src/cad/assembly/assembly-interaction.ts", "web/apps/cad/src/cad/assembly/assembly-conflict.ts"}
    return {"schemaVersion": catalog["schemaVersion"], "contractVersion": catalog["contractVersion"], "catalogDigest": digest(catalog), "purpose": purpose, "acceptanceScope": "AUTOMATED_CONTRACT_ONLY", "milestoneEvidence": milestone_evidence(catalog, results), "manualAcceptance": {"status": "PENDING_MAINTAINER", "scope": "Definition-only confirmation, finalizing selection barrier and analysis overlay await usage acceptance", "maintainerFeedback": "Maintainer reports assembly solve/drag generally stable in current usage; not an industrial, concurrent, platform or performance acceptance."}, "baselineMeaning": "selected executed existing abilities only; NOT six-family product acceptance", "commit": git("rev-parse", "HEAD"), "worktree": git("status", "--short"), "worktreeDiffDigest": hashlib.sha256(subprocess.check_output(["git", "diff", "HEAD"], cwd=ROOT)).hexdigest(), "inputDigests": {source: hashlib.sha256((ROOT / source).read_bytes()).hexdigest() for source in sorted(sources)}, "invocation": argv, "selectedCaseIds": [t["caseId"] for t in cases], "environment": {"platform": platform.platform(), "python": platform.python_version(), **integration_environment()}, "commands": commands, "results": list(results.values()), "counts": dict(Counter(r["status"] for r in results.values())), "uniquePassedTestMappings": len(native), "targetCounts": dict(Counter(r["targetStatus"] for r in rows)), "capabilities": rows}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("purpose", choices=["baseline", "gaps", "composition", "validate"])
    parser.add_argument("--capability")
    parser.add_argument("--family")
    parser.add_argument("--layer")
    parser.add_argument("--case")
    parser.add_argument("--adapters", default="cpp,go,go-flags,web,web-catalog,integration")
    parser.add_argument("--build-type", choices=["Debug", "Release"], default="Release")
    parser.add_argument("--output", type=Path, default=ROOT / "build/assembly-contract")
    args = parser.parse_args(argv)
    catalog = load_catalog()
    validate(catalog, lock=json.loads((DATA / "baseline.json").read_text()))
    caps, cases = select(catalog, args.capability, args.family, args.layer, args.case)
    if args.purpose == "validate":
        print(f"Catalog/lock PASS: {len(catalog['capabilities'])} capabilities, {len(catalog['cases'])} cases; no implementation tests executed")
        return 0
    adapters = set(args.adapters.split(","))
    if not adapters or adapters - ADAPTERS:
        raise ValueError("unknown/empty adapter selection")
    output = args.output.resolve()
    scheduled = cases if args.purpose in {"gaps", "composition"} else [t for t in cases if t["baseline"]]
    results, commands = run_cases(scheduled, output, args.build_type, adapters)
    for t in cases:
        if t["caseId"] not in results:
            results[t["caseId"]] = {"caseId": t["caseId"], "status": "NOT_RUN", "observed": "target-only assertion excluded from the verified existing baseline; run gaps", "command": None, "log": None}
    report = make_report(catalog, caps, cases, results, commands, args.purpose, sys.argv if argv is None else argv)
    output.mkdir(parents=True, exist_ok=True)
    (output / "report.json").write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    counts = report["counts"]
    summary = ["# Assembly constraint contract report", "", f"Commit: `{report['commit']}`; contract `{catalog['contractVersion']}`", "", "Execution: " + json.dumps(counts, sort_keys=True), "", "Target coverage: " + json.dumps(report["targetCounts"], sort_keys=True), "", "Baseline checks only selected executed abilities; target gaps are not PASS.", "", "| Case requiring attention | Execution verdict | Evidence |", "|---|---|---|"]
    for case in cases:
        result = results[case["caseId"]]
        if result["status"] != "PASS":
            summary.append(f"| {case['caseId']} | {result['status']} | {result['log'] or result['observed']} |")
    summary += ["", "| Capability | Implementation | Verification | Acceptance | Missing/partial layers | Outstanding evidence | Followup |", "|---|---|---|---|---|---|---|"]
    for row in report["capabilities"]:
        summary.append(f"| {row['capabilityId']} | {row['targetStatus']} | {row['verificationStatus']} | {row['acceptanceStatus']} | {', '.join(row['unresolvedLayers'])} | {json.dumps(row['outstandingEvidence'], sort_keys=True)} | {', '.join(row['followupTasks'])} |")
    summary += ["", "See report.json for case expectations, actual verdicts, logs and per-layer evidence."]
    (output / "summary.md").write_text("\n".join(summary) + "\n")
    print("Executed contract cases: " + json.dumps(counts, sort_keys=True))
    print(f"Reports: {output / 'report.json'} and summary.md")
    if counts.get("FAIL"):
        return 1
    if args.purpose == "composition" and any(row["acceptanceStatus"] != "ACCEPTED" for row in report["capabilities"]):
        print("COMPOSITION incomplete: missing implementation or required actual evidence (including blocked integration)", file=sys.stderr)
        return 1
    if not counts.get("PASS"):
        print("No actual assertions passed; cannot report green baseline", file=sys.stderr)
        return 2
    if args.purpose == "baseline" and any(t["baseline"] and t["adapter"] != "integration" and results[t["caseId"]]["status"] != "PASS" for t in cases):
        print("Selected baseline contains unexecuted cases", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, KeyError, ET.ParseError) as exc:
        print(f"Contract error: {exc}", file=sys.stderr)
        sys.exit(2)
