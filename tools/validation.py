"""Structured execution evidence shared by local checks and CAD contracts."""
from __future__ import annotations

import json
import xml.etree.ElementTree as ET
from pathlib import Path


def go_events(output: str) -> tuple[list[dict], str]:
    events, readable = [], []
    for line in output.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            readable.append(line)
            continue
        if not isinstance(event, dict):
            readable.append(line)
            continue
        events.append(event)
        if event.get("Action") in {"output", "build-output"} and event.get("Output"):
            readable.append(str(event["Output"]).rstrip("\n"))
    return events, "\n".join(line for line in readable if line)


def go_summary(output: str, code: int, required_selection: bool = False) -> dict:
    events, readable = go_events(output)
    tests: dict[tuple[str, str], dict] = {}
    for event in events:
        if not event.get("Test"):
            continue
        key = event.get("Package", ""), event["Test"]
        record = tests.setdefault(key, {"package": key[0], "test": key[1], "output": []})
        action = event.get("Action")
        if action == "run":
            record["started"] = True
        elif action in {"pass", "fail", "skip"}:
            record["verdict"] = action
            record["elapsedSeconds"] = event.get("Elapsed")
        elif action == "output":
            record["output"].append(event.get("Output", ""))
    skipped = [dict(package=t["package"], test=t["test"], reason="".join(t["output"]).strip(),
                    applicable="NOT_APPLICABLE:" not in "".join(t["output"]))
               for t in tests.values() if t.get("verdict") == "skip"]
    counts = {"started": sum(bool(t.get("started")) for t in tests.values()),
              "passed": sum(t.get("verdict") == "pass" for t in tests.values()),
              "failed": sum(t.get("verdict") == "fail" for t in tests.values()),
              "skipped": len(skipped)}
    if code or counts["failed"]:
        status, reason = "FAIL", "Go assertion, build or process failed"
    elif not counts["started"]:
        status, reason = "FAIL", "selected zero Go tests"
    elif any(t.get("started") and "verdict" not in t for t in tests.values()):
        status, reason = "FAIL", "started Go test has no terminal verdict"
    elif skipped and (required_selection or any(t["applicable"] for t in skipped) or not counts["passed"]):
        status, reason = "ENVIRONMENT_BLOCKED", "selected required tests skipped; not complete evidence"
    elif skipped:
        status, reason = "PASS_WITH_SKIPS", "applicable assertions passed; optional/backend-specific tests excluded"
    else:
        status, reason = "PASS", "actual Go assertions passed"
    failed_tests = {(t["package"], t["test"]) for t in tests.values() if t.get("verdict") == "fail"}
    diagnostics = []
    for event in events:
        if not event.get("Output"):
            continue
        key = event.get("Package", ""), event.get("Test", "")
        related = any(key[0] == pkg and (key[1] == name or key[1].startswith(name + "/"))
                      for pkg, name in failed_tests)
        if event.get("Action") == "build-output" or related:
            diagnostics.append(event["Output"])
    if status == "ENVIRONMENT_BLOCKED":
        diagnostics.extend(item["reason"] for item in skipped)
    cases = [{key: t.get(key) for key in ("package", "test", "verdict", "elapsedSeconds")}
             for t in tests.values() if "verdict" in t]
    return {"status": status, "reason": reason, "counts": counts, "skipped": skipped,
            "cases": cases, "output": readable, "failureOutput": "".join(diagnostics) or readable}


def go_selector_verdict(events: list[dict], selector: str, code: int) -> tuple[str, str]:
    matching = [e for e in events if e.get("Test") == selector]
    ancestor_names = {selector.rsplit("/", depth)[0] for depth in range(1, selector.count("/") + 1)}
    ancestors = [e for e in events if e.get("Test") in ancestor_names]
    if any(e["Action"] == "fail" for e in ancestors):
        return "FAIL", "mapped Go fixture ancestor failed; child PASS cannot certify the complete fixture"
    if any(e["Action"] == "skip" for e in ancestors):
        return "ENVIRONMENT_BLOCKED", "mapped Go fixture ancestor skipped; child PASS is not complete evidence"
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


def ctest_summary(path: Path, code: int) -> dict:
    if not path.exists():
        return {"status": "FAIL", "reason": "CTest produced no execution report"}
    try:
        tests = ET.parse(path).getroot().findall(".//testcase")
    except ET.ParseError:
        return {"status": "FAIL", "reason": "invalid CTest execution report"}
    skipped = [t.get("name") for t in tests if t.find("skipped") is not None or t.get("status") == "notrun"]
    failed = sum(t.find("failure") is not None or t.find("error") is not None for t in tests)
    status = "FAIL" if code or failed or not tests else "ENVIRONMENT_BLOCKED" if skipped else "PASS"
    diagnostics = []
    for test in tests:
        if test.find("failure") is not None or test.find("error") is not None:
            diagnostics.append(test.get("name", "native failure") + "\n" + test.findtext("system-out", ""))
    return {"status": status, "reason": "CTest execution evidence", "counts": {
        "started": len(tests) - len(skipped), "passed": len(tests) - len(skipped) - failed,
        "failed": failed, "skipped": len(skipped)}, "skipped": skipped,
        "failureOutput": "\n".join(diagnostics)}
