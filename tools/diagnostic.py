"""Locate immutable diagnostics and replay only recorded numerical inputs."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import re
import shlex
import subprocess
import os


MAX_BYTES = 8 * 1024 * 1024
SAFE_ID = re.compile(r"^[A-Za-z0-9_.-]+$")


def read(path: Path) -> tuple[dict, str]:
    with path.open("rb") as stream:
        data = stream.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise ValueError("diagnostic exceeds 8 MiB")
    parsed = json.loads(data)
    if not isinstance(parsed, dict):
        raise ValueError("diagnostic must be an object")
    return parsed, hashlib.sha256(data).hexdigest()


def resolve(reference: str, logs: Path) -> Path:
    match = re.search(r"\b(CAD_(?:JOB_)?DIAGNOSTIC)\s+([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)(?:\s|$|\])", reference)
    job = bool(match and match[1] == "CAD_JOB_DIAGNOSTIC")
    if match:
        reference = match[2]
    parts = reference.split("/")
    if len(parts) != 2 or any(not SAFE_ID.fullmatch(p) or p in {".", ".."} for p in parts):
        raise ValueError("expected documentId/diagnosticId")
    base = (logs / ("debug/job-diagnostics" if job else "debug/cad-diagnostics")).resolve()
    # Record IDs contain fractional seconds; appending preserves their identity.
    path = Path(str(base.joinpath(*parts)) + ".json")
    if not path.resolve().is_relative_to(base):
        raise ValueError("diagnostic path escapes the configured archive")
    if not path.is_file():
        raise ValueError("diagnostic missing or expired in the configured environment")
    return path


def recent(logs: Path, owner: str, job=False) -> dict:
    if not SAFE_ID.fullmatch(owner) or owner in {".", ".."}:
        raise ValueError("invalid archive owner identity")
    base = (logs / ("debug/job-diagnostics" if job else "debug/cad-diagnostics")).resolve()
    directory = base / owner
    if not directory.resolve().is_relative_to(base):
        raise ValueError("archive path escapes configured logs")
    entries = []
    for meta in sorted(directory.glob("*.meta.json"), reverse=True)[:5]:
        identity = meta.name.removesuffix(".meta.json")
        prefix = "CAD_JOB_DIAGNOSTIC" if job else "CAD_DIAGNOSTIC"
        reference = f"{prefix} {owner}/{identity}"
        record, digest = read(resolve(reference, logs))
        entries.append({"reference": reference, "stage": record.get("stage"), "mode": record.get("mode"),
                        "baseRevisionId": record.get("baseRevisionId"), "resultRevisionId": record.get("resultRevisionId"),
                        "sha256": digest, "failure": str(record.get("failure", ""))[:400]})
    return {"schema": "occccad.diagnostic-list.v1", "owner": owner, "entries": entries}


def inspect(root: Path, environment: dict, reference=None, file=None, attempt=0, replay=False) -> dict:
    if bool(reference) == bool(file):
        raise ValueError("choose exactly one diagnostic reference or file")
    path = Path(file).resolve() if file else resolve(reference, Path(environment["OCCCCAD_LOG_DIR"]))
    diagnostic, digest = read(path)
    if diagnostic.get("schema") not in {"occccad.operation-diagnostic.v1", "occccad.job-diagnostic.v1", "occccad.3dreplay.v1"}:
        raise ValueError("unsupported diagnostic schema")
    attempts = diagnostic.get("assemblyAttempts", [])
    if diagnostic.get("schema") == "occccad.3dreplay.v1":
        attempts = [diagnostic]
    if not isinstance(attempts, list):
        raise ValueError("invalid recorded attempt list")
    missing = []
    for field in ("assemblyMissing", "metadataMissing", "missing"):
        values = diagnostic.get(field, [])
        if not isinstance(values, list):
            raise ValueError("invalid missing-input evidence")
        missing.extend(values)
    result = {"schema": "occccad.diagnostic-inspection.v1", "source": str(path), "sha256": digest,
              "documentId": diagnostic.get("documentId"), "baseRevisionId": diagnostic.get("baseRevisionId"),
              "resultRevisionId": diagnostic.get("resultRevisionId"), "outcome": diagnostic.get("outcome"),
              "jobId": diagnostic.get("jobId"), "operationDiagnosticId": diagnostic.get("operationDiagnosticId"),
              "jobType": diagnostic.get("jobType"), "attempt": diagnostic.get("attempt"),
              "inputObjectId": diagnostic.get("inputObjectId"), "versionId": diagnostic.get("versionId"),
              "inputDigest": diagnostic.get("inputDigest"),
              "mode": diagnostic.get("mode"), "stage": diagnostic.get("stage"),
              "failure": str(diagnostic.get("failure", ""))[:2000],
              "recordedAttempts": len(attempts),
              "missing": missing,
              "replayStatus": "AVAILABLE" if attempts else "NO_RECORDED_NUMERICAL_ATTEMPT",
              "provenance": {key: diagnostic.get(key) for key in ("goVersion", "evaluator", "namingPolicy")},
              "boundary": "recorded numerical attempt only; does not replay the full domain transaction or advance Head"}
    if not attempts:
        if replay:
            raise ValueError("no recorded numerical attempt; retained operation input can be inspected, full operation replay is unavailable")
        return result
    if attempt < 0 or attempt >= len(attempts):
        raise ValueError("recorded attempt index out of range")
    recorded = attempts[attempt]
    if not isinstance(recorded, dict) or recorded.get("schema") != "occccad.3dreplay.v1":
        raise ValueError("recorded attempt is not a supported production replay")
    if not recorded.get("request") and not recorded.get("snapshot"):
        raise ValueError("recorded numerical input is missing")
    # Derive a deterministic new workspace from immutable source content. No
    # editable business state or archive file is ever substituted or overwritten.
    directory = root / "build/diagnostics" / digest[:20] / str(attempt)
    directory.mkdir(parents=True, exist_ok=True)
    exported = directory / "input.3dreplay"
    payload = json.dumps(recorded, ensure_ascii=False, separators=(",", ":")).encode()
    if exported.exists():
        if exported.read_bytes() != payload:
            raise ValueError("derived replay input differs from the immutable diagnostic")
    else:
        with exported.open("xb") as stream:
            stream.write(payload)
    from uuid import uuid4
    output = directory / ("result-" + uuid4().hex[:8] + ".3dreplay")
    argv = ["go", "run", "./cmd/occccad-3dreplay", "-worker-binary", environment["OCCCCAD_GEOMETRY_WORKER_BIN"],
            "-mode", "original", "-out", str(output), str(exported)]
    result.update(attempt=attempt, input=str(exported), command="cd " + shlex.quote(str(root / "services")) + " && " + shlex.join(argv))
    if replay:
        executed = subprocess.run(argv, cwd=root / "services", env=dict(os.environ, **environment),
                                  capture_output=True, text=True, timeout=60)
        log = directory / (output.stem + ".log")
        log.write_text(executed.stdout + executed.stderr)
        result.update(replayStatus="EXECUTED" if executed.returncode == 0 else "EXECUTION_FAILED",
                      output=str(output) if output.exists() else None, log=str(log))
        # A successful replay process can reproduce a numerical failure. Keep
        # its observed outcome independent from process execution success.
        if output.exists():
            observed, _ = read(output)
            result["observedOutcome"] = observed.get("outcome")
    return result
