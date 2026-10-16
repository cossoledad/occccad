"""Quiet validation with complete logs, explicit verdicts and reusable metrics."""
from __future__ import annotations

import json
from pathlib import Path
import re
import subprocess
import shlex
import time
from uuid import uuid4

from invoke import Exit
from tools.validation import ctest_summary, go_summary
from tools.development_environment import environment_identity
from tools.build_identity import source_identity


def compact_output(output: str, limit: int = 200, byte_limit: int = 16384) -> str:
    rendered = _compact_lines(output, limit).encode()
    if len(rendered) <= byte_limit:
        return rendered.decode()
    marker = b"\n... output truncated; see full log ...\n"
    budget = max(0, byte_limit - len(marker))
    front = budget // 2
    return (rendered[:front].decode(errors="ignore") + marker.decode()
            + rendered[-(budget - front):].decode(errors="ignore"))


def _compact_lines(output: str, limit: int) -> str:
    lines = output.splitlines()
    if len(lines) <= limit:
        return output.rstrip()
    signal = re.compile(r"error|fail|fatal|panic|expected|actual|warning|undefined|unimplemented", re.I)
    selected = [*range(min(20, len(lines))),
                *(i for i, line in enumerate(lines) if signal.search(line)),
                *range(max(0, len(lines) - 80), len(lines))]
    indices = sorted(set(selected))
    if len(indices) > limit:
        indices = indices[:limit // 2] + indices[-(limit - limit // 2):]
    rendered, previous = [], -1
    for index in indices:
        if previous >= 0 and index > previous + 1:
            rendered.append(f"... {index - previous - 1} lines omitted; see full log ...")
        rendered.append(lines[index])
        previous = index
    return "\n".join(rendered)


def run_steps(c, steps: list, root: Path, environment: dict, verbose: bool = False,
              required_selection: bool = False, metadata: dict | None = None) -> dict:
    directory = root / "build/agent-check" / (time.strftime("%Y%m%dT%H%M%S") + "-" + uuid4().hex[:8])
    directory.mkdir(parents=True)
    git = subprocess.run(["git", "rev-parse", "HEAD"], cwd=root, capture_output=True, text=True)
    report = {"schema": "occccad.validation-run.v1", "commit": git.stdout.strip(),
              "source": source_identity(root),
              "environment": environment_identity(environment), "selection": metadata or {},
              "status": "RUNNING", "steps": [], "durationSeconds": 0}
    started = time.monotonic()
    failure_code = 0
    for index, (name, command, cwd) in enumerate(steps):
        record = {"name": name, "command": command, "cwd": str(cwd), "status": "NOT_RUN"}
        report["steps"].append(record)
        if failure_code:
            record["reason"] = "previous step failed or was blocked"
            continue
        slug = re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")
        if command.startswith("ctest "):
            xml = directory / f"{index:02d}-{slug}.xml"
            command += " --output-junit " + shlex.quote(str(xml))
            record["command"] = command
        before = time.monotonic()
        with c.cd(str(cwd)):
            result = c.run(command, env=environment, hide=not verbose, warn=True, pty=verbose)
        record["durationSeconds"] = round(time.monotonic() - before, 6)
        log = directory / f"{index:02d}-{slug}.log"
        log.write_text(f"cwd: {cwd}\ncommand: {command}\n\n{result.stdout}\n{result.stderr}")
        record.update(log=str(log.relative_to(root)), outputBytes=len(result.stdout.encode()) + len(result.stderr.encode()))
        readable = result.stdout
        if command.startswith("go test -json "):
            verdict = go_summary(result.stdout, result.exited or 0, required_selection)
            verdict.pop("output")
            readable = verdict.pop("failureOutput")
            record.update(verdict)
        elif command.startswith("ctest "):
            verdict = ctest_summary(xml, result.exited or 0)
            readable = verdict.pop("failureOutput", "") or readable
            record.update(verdict)
        else:
            record["status"] = "PASS" if result.ok else "FAIL"
        counts = record.get("counts")
        detail = f"; pass={counts['passed']} skip={counts['skipped']} fail={counts['failed']}" if counts else ""
        print(f"[check] {record['status']} {name} ({record['durationSeconds']:.1f}s{detail})")
        if record["status"] not in {"PASS", "PASS_WITH_SKIPS"}:
            failure_code = 2 if record["status"] == "ENVIRONMENT_BLOCKED" else result.exited or 1
            if not verbose:
                print(compact_output(readable + "\n" + result.stderr))
            print(f"[check] {record.get('reason', 'command failed')}")
            print(f"[check] Full log: {record['log']}")
            print(f"[check] Reproduce: cd {shlex.quote(str(cwd))} && {command}")
    report["durationSeconds"] = round(time.monotonic() - started, 6)
    statuses = {step["status"] for step in report["steps"]}
    report["status"] = ("FAIL" if "FAIL" in statuses else "ENVIRONMENT_BLOCKED" if "ENVIRONMENT_BLOCKED" in statuses
                        else "PASS_WITH_SKIPS" if "PASS_WITH_SKIPS" in statuses else "PASS")
    report["outputBytes"] = sum(step.get("outputBytes", 0) for step in report["steps"])
    report["sourceAfter"] = source_identity(root)
    if report["sourceAfter"] != report["source"]:
        report["status"] = "SOURCE_CHANGED"
        failure_code = failure_code or 2
    path = directory / "report.json"
    report["artifact"] = str(path.relative_to(root))
    path.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    print(f"[check] {report['status']} {len(steps)} steps ({report['durationSeconds']:.1f}s); report: {path.relative_to(root)}")
    if failure_code:
        raise Exit(code=failure_code)
    return report
