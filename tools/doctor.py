"""Readiness for the selected build and the existing development resources."""
from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import socket
import subprocess

from tools.development_environment import environment_identity


def run(root: Path, environment: dict, scopes: set[str], browser=False, live=False) -> dict:
    checks = []
    env = dict(os.environ, **environment)

    def add(name, ok, detail, prepare=None):
        checks.append({"name": name, "status": "PASS" if ok else "ENVIRONMENT_BLOCKED",
                       "detail": detail, "prepare": prepare})

    native = bool(scopes & {"all", "assembly", "geometry", "sketch", "workspace", "services"})
    web = bool(scopes & {"all", "web"})
    for tool in (["go", "cmake", "ninja"] if native else []) + (["node", "pnpm"] if web else []):
        add(tool, bool(shutil.which(tool)), shutil.which(tool) or f"{tool} unavailable")
    if native:
        worker = Path(environment["OCCCCAD_GEOMETRY_WORKER_BIN"])
        build = root / "build/cmake" / environment["OCCCCAD_BUILD_TYPE"].lower()
        add("worker", worker.is_file() and os.access(worker, os.X_OK), str(worker),
            f"invoke build --build-type {environment['OCCCCAD_BUILD_TYPE']} --target occccad_geometry_worker")
        cache = build / "CMakeCache.txt"
        selected = ""
        if cache.exists():
            selected = next((line.split("=", 1)[1] for line in cache.read_text().splitlines()
                             if line.startswith("CMAKE_BUILD_TYPE:STRING=")), "")
        add("native-build-type", selected == environment["OCCCCAD_BUILD_TYPE"],
            f"requested={environment['OCCCCAD_BUILD_TYPE']} configured={selected or 'missing'}",
            f"invoke configure --build-type {environment['OCCCCAD_BUILD_TYPE']}")
        expected = build / "workers/geometry/occccad_geometry_worker"
        if worker.resolve() == expected.resolve() and (build / "build.ninja").exists() and shutil.which("ninja"):
            pending = subprocess.run(["ninja", "-C", str(build), "-n", "occccad_geometry_worker",
                                      "occcad_geometry_scenarios", "occccad_naming_artifact_tests"],
                                     capture_output=True, text=True, env=env)
            clean = pending.returncode == 0 and "no work to do" in pending.stdout
            add("native-freshness", clean, "production Worker and fixture producers are current" if clean
                else "native inputs changed or targets are missing; rebuild before integration",
                f"invoke build --build-type {environment['OCCCCAD_BUILD_TYPE']}")
        else:
            add("native-freshness", False, "configured external/missing Worker has no verifiable build graph")
        if shutil.which("go"):
            resources = subprocess.run(["go", "run", "./cmd/occccad-dev", "doctor"], cwd=root / "services",
                                       capture_output=True, text=True, env=env)
            try:
                checks.extend(json.loads(resources.stdout))
            except (ValueError, TypeError):
                add("development-resources", False, "resource checker could not run; build services/cmd/occccad-dev")
    if web:
        add("web-dependencies", (root / "web/node_modules/.pnpm").is_dir(), "existing pnpm workspace",
            "pnpm --dir web install --frozen-lockfile")
    if browser and shutil.which("pnpm"):
        script = ("const {chromium}=require('@playwright/test');"
                  "chromium.launch({headless:true,args:['--use-gl=angle','--use-angle=swiftshader',"
                  "'--enable-unsafe-swiftshader']}).then(async b=>{await b.close();process.stdout.write('ready')})"
                  ".catch(()=>process.exit(2))")
        try:
            result = subprocess.run(["pnpm", "exec", "node", "-e", script], cwd=root / "web/apps/cad",
                                    capture_output=True, text=True, env=env, timeout=30)
            add("browser", result.returncode == 0, "existing Playwright Chromium launch",
                "see docs/development/environment.md#chromium-与浏览器验证")
        except subprocess.TimeoutExpired:
            add("browser", False, "existing Chromium did not become ready")
    if live:
        from tools.build_identity import running_identity
        checks.append(running_identity(root, environment))
        # A TCP listener proves reachability only; protocol acceptance is owned
        # by the recipe's real Router/API tests and browser scenarios.
        for name, value in (("router-listener", environment.get("OCCCCAD_GEOMETRY_WORKER_ADDRESS", "127.0.0.1:51001")),
                            ("api-listener", environment.get("OCCCCAD_APP_LISTEN", "127.0.0.1:8080"))):
            host, port = value.rsplit(":", 1)
            try:
                with socket.create_connection(("127.0.0.1" if host in {"", "0.0.0.0"} else host, int(port)), timeout=2):
                    add(name, True, "configured listener reachable; protocol behavior requires integration validation")
            except OSError:
                add(name, False, "configured listener unavailable", "invoke run")
    return {"schema": "occccad.development-doctor.v1", "environment": environment_identity(environment),
            "checks": checks, "status": "PASS" if all(c["status"] == "PASS" for c in checks) else "ENVIRONMENT_BLOCKED"}
