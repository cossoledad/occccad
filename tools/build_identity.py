"""Code/artifact identity for validation and the running development stack."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import subprocess
from tools.development_environment import environment_identity


def file_digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def source_identity(root: Path, runtime=False) -> dict:
    listed = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=root)
    inputs = {}
    extensions = {".go", ".cpp", ".hpp", ".h", ".c", ".inl", ".ts", ".tsx", ".mjs", ".py", ".proto", ".sql", ".cmake", ".json", ".css"}
    for name in sorted(set(listed.decode().split("\0"))):
        if not name or name.startswith(("build/", "docs/", "plans/", "tests/test.data/", "node_modules/", "third_party/", "external/")):
            continue
        path = root / name
        if not path.is_file() or (path.suffix not in extensions and path.name not in {"CMakeLists.txt", "go.mod", "go.sum", ".env.example"}):
            continue
        if runtime and (not name.startswith(("services/", "kernel/", "workers/", "proto/", "cmake/"))
                        and name not in {"CMakeLists.txt", "conanfile.py"}):
            continue
        if runtime and (name.endswith("_test.go") or "tests" in path.relative_to(root).parts
                        or name.startswith(("services/cmd/occccad-dev/", "services/cmd/occccad-monitor/", "services/cmd/occccad-3dreplay/"))):
            continue
        inputs[name] = file_digest(path)
    digest = hashlib.sha256(json.dumps(inputs, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    return {"sha256": digest, "files": len(inputs), "boundary": "owned code/build inputs; excludes data, corpus and secret configuration"}


def record_runtime(root: Path, environment: dict) -> Path:
    paths = [root / "build/services" / name for name in ("occccad-control", "occccad-server", "occccad-jobs")]
    paths.append(Path(environment["OCCCCAD_GEOMETRY_WORKER_BIN"]))
    artifacts = {str(path): file_digest(path) for path in paths if path.is_file()}
    result = {"schema": "occccad.runtime-build.v1", "source": source_identity(root, runtime=True),
              "buildType": environment["OCCCCAD_BUILD_TYPE"], "artifacts": artifacts,
              "runtimeId": environment["OCCCCAD_RUNTIME_ID"], "environment": environment_identity(environment)}
    destination = root / "build/runtime/identity.json"
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(result, indent=2) + "\n")
    return destination


def running_identity(root: Path, environment: dict, proc_root=Path("/proc")) -> dict:
    receipt = root / "build/runtime/identity.json"
    if not receipt.is_file():
        return {"name": "running-build", "status": "ENVIRONMENT_BLOCKED", "detail": "no invoke run build receipt", "prepare": "invoke run"}
    saved = json.loads(receipt.read_text())
    if saved.get("source") != source_identity(root, runtime=True) or saved.get("buildType") != environment["OCCCCAD_BUILD_TYPE"]:
        return {"name": "running-build", "status": "ENVIRONMENT_BLOCKED", "detail": "runtime sources or build type changed", "prepare": "restart invoke run"}
    if saved.get("environment") != environment_identity(environment):
        return {"name": "running-build", "status": "ENVIRONMENT_BLOCKED", "detail": "configured resources/listeners or Worker binary changed", "prepare": "restart invoke run"}
    known = saved.get("artifacts", {})
    if any(not Path(path).is_file() or file_digest(Path(path)) != digest for path, digest in known.items()):
        return {"name": "running-build", "status": "ENVIRONMENT_BLOCKED", "detail": "built artifacts changed since application startup", "prepare": "restart invoke run"}
    stale, observed = [], []
    if not proc_root.is_dir():
        return {"name": "running-build", "status": "ENVIRONMENT_BLOCKED", "detail": "running binary inspection requires Linux/WSL"}
    for process in proc_root.iterdir():
        if not process.name.isdigit():
            continue
        executable = process / "exe"
        try:
            target = os.readlink(executable).removesuffix(" (deleted)")
            if target not in known:
                continue
            entries = (process / "environ").read_bytes().split(b"\0")
            runtime_id = next((entry.split(b"=", 1)[1].decode() for entry in entries if entry.startswith(b"OCCCCAD_RUNTIME_ID=")), None)
            if runtime_id != saved.get("runtimeId"):
                continue
            observed.append(Path(target).name)
            if file_digest(executable) != known[target]:
                stale.append(process.name)
        except (FileNotFoundError, PermissionError, ProcessLookupError):
            continue
    required = {"occccad-control", "occccad-server", "occccad-jobs", "occccad_geometry_worker"}
    missing = sorted(required - set(observed))
    ok = not stale and not missing
    return {"name": "running-build", "status": "PASS" if ok else "ENVIRONMENT_BLOCKED",
            "detail": "running processes match the recorded source and binaries" if ok else f"missing={missing}; stale loaded process IDs={stale}",
            "prepare": None if ok else "restart invoke run"}
