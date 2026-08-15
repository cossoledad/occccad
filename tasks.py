"""
occccad Development CLI — powered by Invoke

Provides:
    invoke bootstrap      — Install all toolchain dependencies
    invoke configure      — Run Conan install + CMake configure
    invoke build          — Build all C++ targets
    invoke test           — Run C++ tests
    invoke clean          — Remove build artifacts
    invoke run.geometry   — Run geometry worker smoke test
    invoke run.worker     — Start the Geometry Worker gRPC server
    invoke run.server     — Start the Go API and Web server
    invoke run.jobs       — Start the durable background job worker
    invoke run.app        — Build and start the complete local application
    invoke data.reset     — Clear all server-side development data
    invoke web.build      — Build the current web application
    invoke info           — Print toolchain versions and paths

All commands respect OCCCCAD_BUILD_TYPE from environment (default: Debug).
"""

import os
import platform
import shutil
import sys
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
    """Run C++ tests via CTest."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure build' first.")

    print("[test] Building Geometry Worker smoke target...")
    c.run(f"cmake --build {build_dir} --target occccad_geometry_worker --parallel", pty=True)

    worker_smoke = build_dir / "workers" / "geometry" / "occccad_geometry_worker"
    print("[test] Running Geometry Worker smoke test...")
    c.run(f"{worker_smoke} --smoke", pty=True)

    print("[test] Running tests...")
    ctest_cmd = f"ctest --test-dir {build_dir} --output-on-failure"
    if filter:
        ctest_cmd += f" -R {filter}"

    c.run(ctest_cmd, pty=True)
    print("[test] Done.")


# ---------------------------------------------------------------------------
# run
# ---------------------------------------------------------------------------


@task(help={"build_type": "Debug or Release"})
def run_geometry(c, build_type=None):
    """Build and run the geometry worker (smoke test)."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure build' first.")

    worker_bin = build_dir / "workers" / "geometry" / "occccad_geometry_worker"
    print("[run] Building geometry worker incrementally...")
    c.run(f"cmake --build {build_dir} --target occccad_geometry_worker --parallel", pty=True)

    print(f"[run] Starting geometry worker at {worker_bin}")
    c.run(f"{worker_bin} --smoke", pty=True)


@task(help={"build_type": "Debug or Release"})
def run_worker(c, build_type=None):
    """Build and start the Geometry Worker gRPC server."""
    bt = build_type or _get_build_type()
    build_dir = _get_build_dir(bt)
    worker_bin = build_dir / "workers" / "geometry" / "occccad_geometry_worker"
    c.run(f"cmake --build {build_dir} --target occccad_geometry_worker --parallel", pty=True)
    c.run(str(worker_bin), pty=True)


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
ns.add_task(clean)
ns.add_collection(run_collection)
ns.add_collection(web_collection)
ns.add_collection(data_collection)
