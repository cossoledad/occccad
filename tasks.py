"""
occccad Development CLI — powered by Invoke

Provides:
    invoke bootstrap      — Install all toolchain dependencies
    invoke configure      — Run Conan install + CMake configure
    invoke build          — Build all C++ targets
    invoke test           — Run the full C++, Go, and Web test suite
    invoke check          — Run quiet scoped/changed-file validation
    invoke clean          — Remove build artifacts
    invoke run.geometry   — Run geometry worker smoke test
    invoke run.worker     — Start the Geometry Worker gRPC server
    invoke run.server     — Start the Go API and Web server
    invoke run.jobs       — Start the durable background job worker
    invoke run.app        — Build and start the complete local application
    invoke run.monitor    — Start the local TUI monitoring dashboard
    invoke data.reset     — Clear all server-side development data
    invoke web.build      — Build the current web application
    invoke info           — Print toolchain versions and paths

All commands respect OCCCCAD_BUILD_TYPE from environment (default: Debug).
"""

import json
import os
import platform
import shlex
import shutil
import subprocess
import sys
import time
from pathlib import Path

from invoke import Collection, Exit, task

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

PROJECT_ROOT = Path(__file__).resolve().parent
BUILD_DIR = PROJECT_ROOT / "build" / "cmake"
CONAN_DIR = PROJECT_ROOT / "build-support" / "conan"
PROFILES_DIR = CONAN_DIR / "profiles"
LOCKS_DIR = CONAN_DIR / "locks"


def _load_project_env() -> None:
    """Load simple KEY=VALUE entries from .env; exported variables take precedence."""
    env_file = PROJECT_ROOT / ".env"
    if not env_file.exists():
        return
    for raw_line in env_file.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line.removeprefix("export ").strip()
        key, separator, value = line.partition("=")
        key = key.strip()
        value = value.strip()
        if not separator or not key.replace("_", "").isalnum():
            continue
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        os.environ.setdefault(key, value)


_load_project_env()

# Default profile depending on detected compiler
_CC = os.environ.get("CC", "gcc").split("/")[-1]
_IS_CLANG = "clang" in _CC


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _get_build_type() -> str:
    """Return Debug or Release from env, defaulting to Debug."""
    return os.environ.get("OCCCCAD_BUILD_TYPE", "Debug")


def _get_profile(build_type: str | None = None) -> str:
    """Select a Conan profile based on compiler and build type."""
    bt = build_type or _get_build_type()
    bt_lower = bt.lower()
    if _IS_CLANG:
        return f"linux-clang-{bt_lower}"
    return f"linux-gcc15-{bt_lower}"


def _get_build_dir(build_type: str | None = None) -> Path:
    bt = (build_type or _get_build_type()).lower()
    return BUILD_DIR / bt


def _get_conan_toolchain(build_type: str | None = None) -> Path:
    return _get_build_dir(build_type) / "build" / "generators" / "conan_toolchain.cmake"


# ---------------------------------------------------------------------------
# info
# ---------------------------------------------------------------------------


@task
def info(c):
    """Print toolchain version information."""
    print("occccad Development Environment")
    print("================================")
    print(f"  Project root:  {PROJECT_ROOT}")
    print(f"  OS:            {platform.system()} {platform.release()}")
    print(f"  Architecture:  {platform.machine()}")
    print()

    tools = [
        ("g++", "g++ --version | head -1"),
        ("cmake", "cmake --version | head -1"),
        ("ninja", "ninja --version"),
        ("conan", "conan --version"),
        ("python", "python3 --version"),
        ("go", "go version"),
        ("node", "node --version"),
        ("pnpm", "pnpm --version"),
        ("invoke", "invoke --version"),
    ]

    for name, cmd in tools:
        result = c.run(cmd, warn=True, hide=True)
        version = result.stdout.strip() if result and result.ok else "NOT FOUND"
        print(f"  {name:12s} {version}")

    print()
    print(f"  Build type:     {_get_build_type()}")
    print(f"  Conan profile:  {_get_profile()}")
    print(f"  Build dir:      {_get_build_dir()}")


# ---------------------------------------------------------------------------
# bootstrap
# ---------------------------------------------------------------------------


@task
def bootstrap(c):
    """Install all build-time dependencies (pip, conan, etc.)."""
    print("[bootstrap] Installing Python build dependencies...")

    req_file = PROJECT_ROOT / "requirements-build.txt"
    if req_file.exists():
        c.run(f"{sys.executable} -m pip install -r {req_file}")

    # Ensure Conan profile exists
    profile_path = PROFILES_DIR / _get_profile()
    if not profile_path.exists():
        print(f"[bootstrap] Creating default Conan profile: {_get_profile()}")
        c.run(f"conan profile detect --force")

    print("[bootstrap] Done.")
    print(f"[bootstrap] Run 'invoke configure' next.")


# ---------------------------------------------------------------------------
# configure
# ---------------------------------------------------------------------------


@task(help={
    "build_type": "Debug or Release (default from env OCCCCAD_BUILD_TYPE)",
    "profile": "Conan profile name override",
})
def configure(c, build_type=None, profile=None):
    """
    Run Conan install + CMake configure.

    Steps:
      1. conan install (resolves dependencies, generates CMake toolchain)
      2. cmake configure (with Conan toolchain file)
    """
    bt = build_type or _get_build_type()
    prof = profile or _get_profile(bt)
    build_dir = _get_build_dir(bt)
    profile_path = PROFILES_DIR / prof

    # Validate profile
    if not profile_path.exists() and not prof.startswith("default"):
        print(f"[configure] WARNING: Profile '{prof}' not found at {profile_path}")
        print(f"[configure] Available profiles:")
        for p in PROFILES_DIR.glob("*"):
            print(f"  - {p.name}")

    print(f"[configure] Build type: {bt}")
    print(f"[configure] Profile:    {prof}")
    print(f"[configure] Build dir:  {build_dir}")

    # Step 1: Conan install
    print("\n[configure] Step 1/2: conan install")
    build_dir.mkdir(parents=True, exist_ok=True)

    conan_cmd = (
        f"conan install {PROJECT_ROOT} "
        f"-of {build_dir} "
        f"-pr:b {PROFILES_DIR / prof} "
        f"-pr:h {PROFILES_DIR / prof} "
        f"--build=missing "
        f"-s build_type={bt}"
    )
    c.run(conan_cmd, pty=True)

    # Step 2: CMake configure
    print("\n[configure] Step 2/2: cmake configure")
    toolchain = _get_conan_toolchain(bt)
    if not toolchain.exists():
        alt_toolchain = build_dir / "conan_toolchain.cmake"
        if alt_toolchain.exists():
            toolchain = alt_toolchain
        else:
            print("[configure] WARNING: Conan toolchain not found, trying without...")
            for f in build_dir.rglob("conan_toolchain.cmake"):
                toolchain = f
                break

    cmake_args = (
        f"-S {PROJECT_ROOT} "
        f"-B {build_dir} "
        f"-G Ninja "
        f"-DCMAKE_BUILD_TYPE={bt} "
        f"-DCMAKE_EXPORT_COMPILE_COMMANDS=ON"
    )
    if toolchain.exists():
        cmake_args += f" -DCMAKE_TOOLCHAIN_FILE={toolchain}"

    c.run(f"cmake {cmake_args}", pty=True)

    # Symlink compile_commands.json to project root for clangd/IDE
    compdb = build_dir / "compile_commands.json"
    if compdb.exists():
        link = PROJECT_ROOT / "compile_commands.json"
        if link.is_symlink() or link.exists():
            link.unlink()
        link.symlink_to(compdb)

    print(f"\n[configure] Done. Run 'invoke build' to compile.")


# ---------------------------------------------------------------------------
# build
# ---------------------------------------------------------------------------


@task(help={
    "build_type": "Debug or Release",
    "target": "Specific CMake target to build",
    "jobs": "Parallel jobs (default: all cores)",
})
def build(c, build_type=None, target=None, jobs=0):
    """Build all C++ targets (or a specific target)."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure' first.")

    print(f"[build] Build type: {bt}")
    print(f"[build] Build dir:  {build_dir}")

    cmake_cmd = f"cmake --build {build_dir}"
    if jobs and jobs > 0:
        cmake_cmd += f" -j {jobs}"
    if target:
        cmake_cmd += f" --target {target}"

    c.run(cmake_cmd, pty=True)
    print("[build] Done.")


# ---------------------------------------------------------------------------
# test
# ---------------------------------------------------------------------------


@task(help={
    "build_type": "Debug or Release",
    "filter": "CTest filter regex",
})
def test(c, build_type=None, filter=None):
    """Run the repository C++, Go, and Front test suites."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure build' first.")

    print("[test] Running development tooling tests...")
    c.run(f"{sys.executable} -m unittest tests.python.test_validation_routing", pty=True)

    print("[test] Building C++ tests...")
    c.run(f"cmake --build {build_dir} --parallel", pty=True)

    print("[test] Running CTest...")
    ctest_cmd = f"ctest --test-dir {build_dir} --output-on-failure"
    if filter:
        ctest_cmd += f" -R {filter}"

    c.run(ctest_cmd, pty=True)

    print("[test] Running Go package tests...")
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run("go test ./...", pty=True)

    print("[test] Running standalone Go tests...")
    with c.cd(str(PROJECT_ROOT / "tests" / "go")):
        c.run("go test ./...", pty=True)

    print("[test] Running colocated Front scenarios...")
    with c.cd(str(PROJECT_ROOT / "web" / "apps" / "cad")):
        c.run("pnpm test", pty=True)
    print("[test] Done.")


# ---------------------------------------------------------------------------
# agent-facing validation
# ---------------------------------------------------------------------------


def _changed_files() -> list[str]:
    """Return tracked and untracked workspace paths without reading their contents."""
    commands = (
        ["git", "diff", "--name-only", "--relative", "HEAD"],
        ["git", "ls-files", "--others", "--exclude-standard"],
    )
    paths: set[str] = set()
    for command in commands:
        result = subprocess.run(
            command,
            cwd=PROJECT_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        paths.update(line for line in result.stdout.splitlines() if line)
    return sorted(paths)


def _validation_scopes_for_paths(paths: list[str]) -> tuple[set[str], list[str]]:
    """Map changed paths conservatively to stable domain validation scopes."""
    scopes: set[str] = set()
    reasons: list[str] = []
    for path in paths:
        if path.endswith(".md") or path.startswith("docs/"):
            continue
        if path == "tasks.py" or path.startswith("cmake/") or path == "CMakeLists.txt":
            scopes.add("all")
            reasons.append(f"{path}: shared build or validation entry")
        elif path.startswith("proto/") or path.startswith("services/gen/"):
            scopes.add("all")
            reasons.append(f"{path}: cross-language contract")
        elif path.startswith("services/internal/database/migrations/"):
            scopes.add("all")
            reasons.append(f"{path}: database schema")
        elif path.startswith("kernel/assembly/") or path.startswith("tests/assembly-corpus/"):
            scopes.add("assembly")
        elif path.startswith("workers/geometry/sketch/"):
            scopes.add("sketch")
        elif path.startswith("kernel/occt/") or path.startswith("models/"):
            scopes.add("geometry")
        elif path.startswith("workers/geometry/") or path.startswith("kernel/api/"):
            scopes.add("all")
            reasons.append(f"{path}: shared Geometry Worker boundary")
        elif path.startswith("services/internal/workspace/") or path.startswith("services/internal/modelcore/"):
            scopes.add("workspace")
        elif path.startswith("services/") or path.startswith("tests/go/"):
            scopes.add("services")
        elif path.startswith("tests/python/"):
            scopes.add("all")
            reasons.append(f"{path}: validation tooling")
        elif path.startswith("web/"):
            scopes.add("web")
        else:
            scopes.add("all")
            reasons.append(f"{path}: no narrower ownership mapping")
    if "all" in scopes:
        return {"all"}, reasons
    return scopes, reasons


def _check_steps(scopes: set[str], build_type: str, match: str | None = None) -> list[tuple[str, str, Path]]:
    """Expand domain scopes into deduplicated build/test commands."""
    build_dir = _get_build_dir(build_type)
    steps: list[tuple[str, str, Path]] = []

    def add(name: str, command: str, cwd: Path = PROJECT_ROOT) -> None:
        step = (name, command, cwd)
        if step not in steps:
            steps.append(step)

    if "all" in scopes:
        add(
            "Validation routing",
            f"{sys.executable} -m unittest tests.python.test_validation_routing",
        )
        add("C++ build", f"cmake --build {build_dir} --parallel")
        add("CTest", f"ctest --test-dir {build_dir} --output-on-failure --no-tests=error")
        add("Go packages", "go test ./...", PROJECT_ROOT / "services")
        add("Go conformance", "go test ./...", PROJECT_ROOT / "tests" / "go")
        add("Web scenarios", "pnpm test", PROJECT_ROOT / "web" / "apps" / "cad")
        add("Web production build", "pnpm build", PROJECT_ROOT / "web")
        return steps

    if "assembly" in scopes:
        add(
            "Assembly C++ build",
            f"cmake --build {build_dir} --target occcad_assembly_solver_scenarios "
            "occcad_assembly_solver_corpus --parallel",
        )
        add(
            "Assembly CTest",
            f"ctest --test-dir {build_dir} --output-on-failure --no-tests=error -R "
            f"{shlex.quote(f'^(assembly|assembly-corpus)/.*{match}' if match else '^(assembly|assembly-corpus)/')}",
        )
        if not match:
            add(
                "Assembly Go integration",
                "go test ./internal/geometry ./internal/workspace ./internal/control",
                PROJECT_ROOT / "services",
            )
            add("Assembly Web scenarios", "pnpm test -- assembly", PROJECT_ROOT / "web" / "apps" / "cad")
    if "geometry" in scopes:
        add(
            "Geometry C++ build",
            f"cmake --build {build_dir} --target occcad_geometry_scenarios --parallel",
        )
        add(
            "Geometry CTest",
            f"ctest --test-dir {build_dir} --output-on-failure --no-tests=error -R "
            f"{shlex.quote(f'^geometry/.*{match}' if match else '^geometry/')}",
        )
        if not match:
            add(
                "Geometry Go integration",
                "go test ./internal/geometry ./internal/control",
                PROJECT_ROOT / "services",
            )
    if "sketch" in scopes:
        add(
            "Sketch C++ build",
            f"cmake --build {build_dir} --target occcad_sketch_solver_scenarios --parallel",
        )
        add(
            "Sketch CTest",
            f"ctest --test-dir {build_dir} --output-on-failure --no-tests=error -R "
            f"{shlex.quote(f'^sketch/.*{match}' if match else '^sketch/')}",
        )
        if not match:
            add("Sketch workspace", "go test ./internal/workspace", PROJECT_ROOT / "services")
            add("Sketch Web scenarios", "pnpm test -- sketch", PROJECT_ROOT / "web" / "apps" / "cad")
    if "workspace" in scopes:
        add(
            "Workspace and model core",
            ("go test -json ./internal/workspace ./internal/modelcore"
             if match else "go test ./internal/workspace ./internal/modelcore")
            + (f" -run {shlex.quote(match)}" if match else ""),
            PROJECT_ROOT / "services",
        )
    if "services" in scopes:
        command = "go test -json ./..." if match else "go test ./..."
        add("Go packages", command + (f" -run {shlex.quote(match)}" if match else ""), PROJECT_ROOT / "services")
        if not match:
            add("Go conformance", "go test ./...", PROJECT_ROOT / "tests" / "go")
    if "web" in scopes:
        add(
            "Web scenarios",
            f"pnpm test{f' -- {shlex.quote(match)}' if match else ''}",
            PROJECT_ROOT / "web" / "apps" / "cad",
        )
        if not match:
            add("Web production build", "pnpm build", PROJECT_ROOT / "web")
    return steps


def _go_test_events(output: str) -> tuple[int, str]:
    """Return the number of executed Go tests and readable output from go test -json."""
    runs = 0
    readable: list[str] = []
    for line in output.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            readable.append(line)
            continue
        if event.get("Action") == "run" and event.get("Test"):
            runs += 1
        if event.get("Action") == "output" and event.get("Output"):
            readable.append(str(event["Output"]).rstrip("\n"))
    return runs, "\n".join(line for line in readable if line)


def _run_quiet_step(c, name: str, command: str, cwd: Path, verbose: bool) -> None:
    started = time.monotonic()
    with c.cd(str(cwd)):
        result = c.run(command, hide=not verbose, warn=True, pty=verbose)
    elapsed = time.monotonic() - started
    go_match = command.startswith("go test -json ")
    go_runs, go_output = _go_test_events(result.stdout) if go_match else (0, "")
    if result.ok and (not go_match or go_runs > 0):
        print(f"[check] PASS {name} ({elapsed:.1f}s)")
        return

    if result.ok and go_match:
        print(f"\n[check] FAIL {name}: --match selected zero Go tests")
        print(f"[check] Reproduce: cd {cwd} && {command}")
        raise Exit(code=1)

    if not verbose:
        stdout = go_output if go_match else result.stdout.rstrip()
        stderr = result.stderr.rstrip()
        if stdout:
            print(f"\n--- {name} stdout ---\n{stdout}")
        if stderr:
            print(f"\n--- {name} stderr ---\n{stderr}")
    print(f"\n[check] FAIL {name} ({elapsed:.1f}s)")
    print(f"[check] Reproduce: cd {cwd} && {command}")
    raise Exit(code=result.exited or 1)


@task(help={
    "scope": "Comma-separated: assembly, geometry, sketch, workspace, services, web, all",
    "changed": "Select scopes conservatively from git changes (default when scope is omitted)",
    "verbose": "Stream normal subprocess output instead of success summaries",
    "match": "Run one test/scenario name regex or substring within one explicit scope",
    "build_type": "Debug or Release",
})
def check(c, scope=None, changed=False, verbose=False, match=None, build_type=None):
    """Run quiet, domain-scoped validation; expand diagnostics only on failure."""
    valid_scopes = {"assembly", "geometry", "sketch", "workspace", "services", "web", "all"}
    if scope and changed:
        raise Exit("Use either --scope or --changed, not both.")
    if match and not scope:
        raise Exit("--match requires one explicit --scope.")

    if scope:
        scopes = {item.strip() for item in scope.split(",") if item.strip()}
        unknown = scopes - valid_scopes
        if unknown:
            raise Exit(f"Unknown validation scope(s): {', '.join(sorted(unknown))}")
        if match and (len(scopes) != 1 or "all" in scopes):
            raise Exit("--match requires exactly one non-all scope.")
    else:
        paths = _changed_files()
        scopes, reasons = _validation_scopes_for_paths(paths)
        if not paths:
            print("[check] PASS clean workspace; no affected validation scope")
            return
        print(f"[check] Changed paths: {len(paths)}; scopes: {', '.join(sorted(scopes)) or 'docs-only'}")
        for reason in reasons:
            print(f"[check] Escalation: {reason}")
        if not scopes:
            print("[check] PASS documentation-only changes; no executable validation selected")
            return

    bt = build_type or _get_build_type()
    steps = _check_steps(scopes, bt, match)
    if any("cmake" in command or "ctest" in command for _, command, _ in steps):
        build_dir = _get_build_dir(bt)
        if not build_dir.exists():
            raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure' first.")

    selection = f"; match: {match}" if match else ""
    print(f"[check] Running scopes: {', '.join(sorted(scopes))}{selection}")
    started = time.monotonic()
    for name, command, cwd in steps:
        _run_quiet_step(c, name, command, cwd, verbose)
    print(f"[check] PASS {len(steps)} steps ({time.monotonic() - started:.1f}s)")


@task(help={"count": "Repeated benchmark samples for benchstat-compatible output"})
def performance_baseline(c, count=5):
    """Run deterministic CAD hot-path benchmarks and save a comparable baseline."""
    output = PROJECT_ROOT / "build" / "performance"
    output.mkdir(parents=True, exist_ok=True)
    target = output / "go-workspace.txt"
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run(
            f"go test ./internal/workspace -run '^$' -bench 'Benchmark(ProfileBuilder|VisualizationManifest)' "
            f"-benchmem -count={int(count)} | tee {target}",
            pty=True,
        )
    assembly_target = "occcad_assembly_solver_benchmark"
    c.run(f"cmake --build {_get_build_dir()} --target {assembly_target}", pty=True)
    assembly_output = output / "assembly-solver.txt"
    executable = _get_build_dir() / "kernel" / "assembly" / "tests" / assembly_target
    c.run(f"{executable} | tee {assembly_output}", pty=True)
    print(f"[performance] Baseline written to {target}")
    print(f"[performance] Assembly baseline written to {assembly_output}")


# ---------------------------------------------------------------------------
# run
# ---------------------------------------------------------------------------


@task(help={"build_type": "Debug or Release"})
def run_geometry(c, build_type=None):
    """Build and run the C++ geometry tests."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure build' first.")

    print("[run] Building geometry tests incrementally...")
    c.run(f"cmake --build {build_dir} --target occcad_geometry_scenarios --parallel", pty=True)
    c.run(f"ctest --test-dir {build_dir} --output-on-failure -R '^geometry/'", pty=True)


@task(help={"build_type": "Debug or Release"})
def run_worker(c, build_type=None):
    """Build and start the Geometry Worker gRPC server."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)
    worker_bin = build_dir / "workers" / "geometry" / "occccad_geometry_worker"
    c.run(f"cmake --build {build_dir} --target occccad_geometry_worker --parallel", pty=True)
    data_directory = Path(os.environ.get("OCCCCAD_DATA_DIR", PROJECT_ROOT / "services" / "data"))
    if not data_directory.is_absolute():
        data_directory = PROJECT_ROOT / "services" / data_directory
    c.run(str(worker_bin), env={"OCCCCAD_DATA_DIR": str(data_directory.resolve())}, pty=True)


@task
def run_server(c):
    """Start the standalone Go API server (does not build or serve the frontend)."""
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run("go run ./cmd/occccad-server", pty=True)


@task
def run_jobs(c):
    """Start the PostgreSQL-backed artifact and STEP job worker."""
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run("go run ./cmd/occccad-jobs", pty=True)


def _reset_development_data(c):
    """Clear the fixed PostgreSQL schema and local ArtifactStore, then migrate."""
    print("[data.reset] Clearing PostgreSQL schema 'occccad' and the local ArtifactStore...")
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run(
            "go run ./cmd/occccad-migrate --reset-development-data",
            env={"OCCCCAD_ALLOW_DEV_RESET": "1"},
            pty=True,
        )


@task(help={"yes": "Confirm deletion of all server-side development data"})
def reset_data(c, yes=False):
    """Clear all server-side development data without starting the application."""
    if not yes:
        raise Exit("data.reset is destructive; rerun with --yes")
    _reset_development_data(c)


@task(
    help={
        "build_type": "Debug or Release",
        "reset_data": "Clear all server-side development data before startup",
    }
)
def run_app(c, build_type=None, reset_data=False):
    """Build and start the backend control plane, API, jobs, and geometry workers."""
    bt = build_type or _get_build_type()
    if reset_data:
        _reset_development_data(c)
    worker_bin = _get_build_dir(bt) / "workers" / "geometry" / "occccad_geometry_worker"
    c.run(
        f"cmake --build {_get_build_dir(bt)} --target occccad_geometry_worker --parallel",
        pty=True,
    )
    service_build = PROJECT_ROOT / "build" / "services"
    service_build.mkdir(parents=True, exist_ok=True)
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run(f"go build -o {service_build / 'occccad-server'} ./cmd/occccad-server")
        c.run(f"go build -o {service_build / 'occccad-jobs'} ./cmd/occccad-jobs")
        c.run(f"go build -o {service_build / 'occccad-control'} ./cmd/occccad-control")
    c.run(
        str(service_build / "occccad-control"),
        env={"OCCCCAD_BUILD_TYPE": bt},
        pty=True,
    )


@task(help={"mode": "mock (no backend) or api (proxy /api to the backend)"})
def run_web(c, mode="mock"):
    """Start the independent Vite frontend development server."""
    if mode not in {"mock", "api"}:
        raise Exit("--mode must be 'mock' or 'api'")
    with c.cd(str(PROJECT_ROOT / "web")):
        c.run(f"pnpm dev:{mode}", pty=True)


@task
def run_monitor(c):
    """Build and start the read-only local monitoring TUI."""
    service_build = PROJECT_ROOT / "build" / "services"
    service_build.mkdir(parents=True, exist_ok=True)
    monitor_binary = service_build / "occccad-monitor"
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run(f"go build -o {monitor_binary} ./cmd/occccad-monitor")
    # A nested Invoke PTY leaves the developer's outer terminal in canonical
    # mode, so Bubble Tea never receives individual keys such as q or arrows.
    # Replacing Invoke gives the TUI direct ownership of the real terminal.
    os.execv(monitor_binary, [str(monitor_binary)])


@task
def build_web(c):
    """Type-check and build the current web application."""
    with c.cd(str(PROJECT_ROOT / "web")):
        c.run("pnpm build", pty=True)


# ---------------------------------------------------------------------------
# clean
# ---------------------------------------------------------------------------


@task
def clean(c):
    """Remove all build artifacts."""
    build_root = PROJECT_ROOT / "build"
    if build_root.exists():
        print(f"[clean] Removing {build_root}")
        shutil.rmtree(build_root)

    compdb = PROJECT_ROOT / "compile_commands.json"
    if compdb.is_symlink() or compdb.exists():
        compdb.unlink()

    print("[clean] Done.")


# ---------------------------------------------------------------------------
# Namespace collections
# ---------------------------------------------------------------------------

run_collection = Collection("run")
run_collection.add_task(run_geometry, "geometry")
run_collection.add_task(run_worker, "worker")
run_collection.add_task(run_server, "server")
run_collection.add_task(run_jobs, "jobs")
run_collection.add_task(run_app, "app")
run_collection.add_task(run_web, "web")
run_collection.add_task(run_monitor, "monitor")

web_collection = Collection("web")
web_collection.add_task(build_web, "build")

data_collection = Collection("data")
data_collection.add_task(reset_data, "reset")

# ---------------------------------------------------------------------------
# Root namespace
# ---------------------------------------------------------------------------

ns = Collection()
ns.add_task(info)
ns.add_task(bootstrap)
ns.add_task(configure)
ns.add_task(build)
ns.add_task(test)
ns.add_task(check)
ns.add_task(performance_baseline, "performance-baseline")
ns.add_task(clean)
ns.add_collection(run_collection)
ns.add_collection(web_collection)
ns.add_collection(data_collection)
