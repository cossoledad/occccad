"""Build, run and validate occccad with Invoke."""

import json as jsonlib
import os
import platform
import re
import shlex
import shutil
import subprocess
import sys
import time
from pathlib import Path

from invoke import Collection, Exit, task
from tools.validation_run import run_steps, compact_output as _compact_failure_output

PROJECT_ROOT = Path(__file__).resolve().parent
BUILD_DIR = PROJECT_ROOT / "build" / "cmake"
PROFILES_DIR = PROJECT_ROOT / "build-support" / "conan" / "profiles"


from tools.development_environment import load as _load_environment, command_environment, build_type as _selected_build_type

_load_environment(PROJECT_ROOT)


def _development_environment(build_type=None):
    return command_environment(PROJECT_ROOT, build_type)


@task(help={"scope": "Comma-separated domains, default all", "build_type": "Debug or Release",
            "browser": "Launch the existing Chromium", "live": "Check configured application listeners",
            "json": "Emit structured readiness evidence"})
def doctor(c, scope="all", build_type=None, browser=False, live=False, json=False):
    """Check the existing shared development environment without resetting data."""
    from tools.doctor import run
    scopes = set(scope.split(","))
    if not scopes or scopes - {"assembly", "geometry", "sketch", "workspace", "services", "web", "all"}:
        raise Exit("Unknown doctor scope")
    result = run(PROJECT_ROOT, _development_environment(build_type), scopes, browser, live)
    if json:
        print(jsonlib.dumps(result, indent=2, ensure_ascii=False))
    else:
        for item in result["checks"]:
            print(f"[doctor] {item['status']} {item['name']}: {item['detail']}")
            if item["status"] != "PASS" and item.get("prepare"):
                print(f"[doctor] Prepare: {item['prepare']}")
        print(f"[doctor] {result['status']}; environment: {result['environment']['digest'][:12]}")
    if result["status"] != "PASS":
        raise Exit(code=2)
    return result


@task(help={"recipe": "property, rpc, selection, feature or diagnostic", "symbol": "Literal symbol to locate",
            "read": "Include at most two bounded source excerpts", "json": "Emit a machine-readable task packet"})
def task_context(c, recipe, symbol=None, read=False, json=False):
    """Locate a task's code, contracts, completion obligations and verification."""
    from tools.task_context import packet
    try:
        result = packet(PROJECT_ROOT, recipe, symbol, read)
    except ValueError as error:
        raise Exit(str(error)) from error
    if json:
        print(jsonlib.dumps(result, indent=2, ensure_ascii=False))
    else:
        print(f"[task] {result['recipe']}: {result['goal']}")
        for source in result["sources"]:
            print(f"[task] {source['path']}:{','.join(map(str, source['lines'])) or '?'} ({source['query']})")
        for obligation in result["obligations"]:
            print(f"[task] Required: {obligation}")
        for name, command in result["validation"].items():
            print(f"[task] {name}: {command}")
        for excerpt in result["snippets"]:
            print(f"\n{excerpt['path']}\n{excerpt['text']}")
        print(f"[task] Evidence: {result['artifact']}")
    return result


@task(help={"build_type": "Debug or Release"})
def workflow_baseline(c, build_type=None):
    """Measure the five task packets and their real focused validations serially."""
    from tools.task_context import packet
    from tools.task_recipes import RECIPES
    from tools.build_identity import source_identity
    from tools.development_environment import environment_identity
    from uuid import uuid4

    directory = PROJECT_ROOT / "build/agent-workflow" / (time.strftime("%Y%m%dT%H%M%S") + "-" + uuid4().hex[:8])
    directory.mkdir(parents=True)
    source = source_identity(PROJECT_ROOT)
    result = {"schema": "occccad.workflow-baseline.v1", "source": source,
              "environment": environment_identity(_development_environment(build_type)),
              "platform": platform.platform(), "tasks": [], "modelTokens": None,
              "boundary": "one serial sample per recipe; packet discovery and focused validation only; excludes implementation, reasoning and subsequent reads; no before/after speedup"}
    for name in RECIPES:
        context = packet(PROJECT_ROOT, name, read=True, output=directory / f"{name}.json")
        tokens = shlex.split(context["validation"]["focused"])
        if tokens[:2] != ["invoke", "check"]:
            raise Exit(f"Unsupported focused validation for {name}")
        scope = tokens[tokens.index("--scope") + 1]
        match = tokens[tokens.index("--match") + 1]
        print(f"[workflow] {name}: {context['validation']['focused']}")
        validation = check.body(c, scope=scope, match=match, build_type=build_type)
        if validation["source"] != source:
            raise Exit("Workflow sources changed; rerun the baseline")
        result["tasks"].append({"recipe": name, "packet": str((directory / f"{name}.json").relative_to(PROJECT_ROOT)),
                                "discovery": context["metrics"], "validation": validation["artifact"],
                                "status": validation["status"], "durationSeconds": validation["durationSeconds"],
                                "outputBytes": validation["outputBytes"],
                                "counts": [step.get("counts") for step in validation["steps"] if step.get("counts")]})
    destination = directory / "baseline.json"
    destination.write_text(jsonlib.dumps(result, indent=2, ensure_ascii=False) + "\n")
    print(f"[workflow] Baseline: {destination.relative_to(PROJECT_ROOT)}")
    return result


@task(help={"reference": "CAD_DIAGNOSTIC document/id or document/id", "file": "Retained diagnostic/replay file",
            "document": "List the latest five retained diagnostics for a document", "job": "List retained job failures",
            "attempt": "Recorded numerical attempt index", "replay": "Execute that immutable attempt offline",
            "build_type": "Debug or Release", "json": "Emit structured inspection"})
def diagnose(c, reference=None, file=None, attempt=0, replay=False, build_type=None, json=False, document=None, job=None):
    """Inspect a retained failure and reproduce available production numerical input."""
    from tools.diagnostic import inspect, recent
    try:
        environment = _development_environment(build_type)
        if document or job:
            if reference or file or replay or (document and job):
                raise ValueError("choose a document/job list or a single diagnostic")
            result = recent(Path(environment["OCCCCAD_LOG_DIR"]), document or job, bool(job))
        else:
            result = inspect(PROJECT_ROOT, environment, reference, file, int(attempt), replay)
    except (ValueError, OSError, subprocess.TimeoutExpired) as error:
        raise Exit(str(error)) from error
    if json:
        print(jsonlib.dumps(result, indent=2, ensure_ascii=False))
    else:
        for item in result.get("entries", []):
            print(f"[diagnose] {item['reference']} {item['mode'] or 'JOB'}/{item['stage']} {item['failure']}")
        for key in ("source", "sha256", "documentId", "baseRevisionId", "mode", "stage", "failure", "replayStatus", "command", "observedOutcome", "boundary"):
            if result.get(key) is not None:
                print(f"[diagnose] {key}: {result[key]}")
    if result.get("replayStatus") == "EXECUTION_FAILED":
        raise Exit(code=1)
    return result


def _get_build_type() -> str:
    """Return Debug or Release from env, defaulting to Release."""
    return _selected_build_type()


def _get_profile(build_type: str | None = None) -> str:
    bt = _selected_build_type(build_type)
    return f"linux-gcc15-{bt.lower()}"


def _get_build_dir(build_type: str | None = None) -> Path:
    bt = (build_type or _get_build_type()).lower()
    return BUILD_DIR / bt


def _get_conan_toolchain(build_type: str | None = None) -> Path:
    bt = _selected_build_type(build_type)
    # cmake_layout with Ninja nests generators under the case-sensitive type.
    return _get_build_dir(bt) / "build" / bt / "generators" / "conan_toolchain.cmake"


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


@task
def bootstrap(c):
    """Install Python build dependencies."""
    print("[bootstrap] Installing Python build dependencies...")

    c.run(shlex.join([sys.executable, "-m", "pip", "install", "-r", str(PROJECT_ROOT / "requirements-build.txt")]))

    print("[bootstrap] Done.")
    print(f"[bootstrap] Run 'invoke configure' next.")


@task(help={
    "build_type": "Debug or Release (OCCCCAD_BUILD_TYPE, otherwise Release)",
    "profile": "Conan profile name or path (default: repository GCC 15 profile)",
})
def configure(c, build_type=None, profile=None):
    """Install Conan dependencies and configure CMake with a fresh cache."""
    bt = _selected_build_type(build_type)
    prof = profile or str(PROFILES_DIR / _get_profile(bt))
    build_dir = _get_build_dir(bt)

    print(f"[configure] Build type: {bt}")
    print(f"[configure] Profile:    {prof}")
    print(f"[configure] Build dir:  {build_dir}")

    print("\n[configure] Step 1/2: conan install")
    build_dir.mkdir(parents=True, exist_ok=True)

    conan_cmd = shlex.join([
        "conan", "install", str(PROJECT_ROOT), "-of", str(build_dir),
        "-pr:b", prof, "-pr:h", prof, "--build=missing", "-s", f"build_type={bt}",
    ])
    c.run(conan_cmd, pty=True)

    print("\n[configure] Step 2/2: cmake configure")
    toolchain = _get_conan_toolchain(bt)
    if not toolchain.is_file():
        raise Exit(f"Conan toolchain not found at {toolchain}; cannot configure without Conan dependencies.")

    # Reset toolchain discovery after a failed configure.
    cmake_cmd = shlex.join([
        "cmake", "--fresh", "-S", str(PROJECT_ROOT), "-B", str(build_dir), "-G", "Ninja",
        f"-DCMAKE_BUILD_TYPE={bt}", f"-DCMAKE_TOOLCHAIN_FILE={toolchain}",
        f"-DOCCCCAD_ENABLE_ASAN={os.environ.get('OCCCCAD_ENABLE_ASAN') or 'OFF'}",
        f"-DOCCCCAD_ENABLE_UBSAN={os.environ.get('OCCCCAD_ENABLE_UBSAN') or 'OFF'}",
    ])
    c.run(cmake_cmd, pty=True)

    compdb = build_dir / "compile_commands.json"
    if compdb.exists():
        link = PROJECT_ROOT / "compile_commands.json"
        link.unlink(missing_ok=True)
        link.symlink_to(compdb)

    print(f"\n[configure] Done. Run 'invoke build' to compile.")


@task(help={
    "build_type": "Debug or Release",
    "target": "Specific CMake target to build",
    "jobs": "Parallel jobs (default: all cores)",
})
def build(c, build_type=None, target=None, jobs=0):
    """Build all C++ targets (or a specific target)."""
    bt = _selected_build_type(build_type)
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure' first.")

    print(f"[build] Build type: {bt}")
    print(f"[build] Build dir:  {build_dir}")

    cmake_cmd = f"cmake --build {shlex.quote(str(build_dir))} --parallel"
    if jobs > 0:
        cmake_cmd += f" {jobs}"
    if target:
        cmake_cmd += f" --target {shlex.quote(target)}"

    c.run(cmake_cmd, pty=True)
    print("[build] Done.")


@task(help={
    "build_type": "Debug or Release",
    "filter": "CTest filter regex",
    "verbose": "Stream complete command output",
})
def test(c, build_type=None, filter=None, verbose=False):
    """Run the repository C++, Go, and Front test suites."""
    bt = _selected_build_type(build_type)
    build_dir = _get_build_dir(bt)

    if not build_dir.exists():
        raise Exit(f"Build dir {build_dir} not found. Run 'invoke configure build' first.")

    steps = _check_steps({"all"}, bt)
    if filter:
        steps = [(name, command + (f" -R {shlex.quote(filter)}" if name == "CTest" else ""), cwd)
                 for name, command, cwd in steps]
    run_steps(c, steps, PROJECT_ROOT, _development_environment(bt), verbose=verbose,
              metadata={"scopes": ["all"], "ctestFilter": filter})



def _changed_files(base=None, head=None, staged=False) -> list[str]:
    """Return tracked and untracked workspace paths without reading their contents."""
    if head and not base:
        raise Exit("--head requires --base")
    if staged and head:
        raise Exit("--staged and --head cannot be combined")
    reference = base or "HEAD"
    for revision in (reference, head):
        if revision:
            result = subprocess.run(["git", "rev-parse", "--verify", revision + "^{commit}"],
                                    cwd=PROJECT_ROOT, capture_output=True, text=True)
            if result.returncode:
                raise Exit(f"Unknown commit reference: {revision}")
    comparison = [f"{reference}...{head}"] if head else (["--cached", reference] if staged else [reference])
    commands = [["git", "diff", "--name-only", "--no-renames", "-z", "--relative", *comparison, "--"]]
    if not head and not staged:
        commands.append(["git", "ls-files", "--others", "--exclude-standard", "-z"])
    paths: set[str] = set()
    for command in commands:
        result = subprocess.run(
            command,
            cwd=PROJECT_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        paths.update(path for path in result.stdout.split("\0") if path)
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
        elif path.startswith(("services/internal/database/postgres_migrations/", "services/internal/database/sqlite_migrations/")):
            scopes.add("all")
            reasons.append(f"{path}: database schema")
        elif path.startswith("kernel/assembly/") or path.startswith("tests/assembly-corpus/") or (path.startswith("tests/test.data/") and path.endswith(".3dreplay")):
            scopes.add("assembly")
        elif path.startswith("workers/geometry/sketch/"):
            scopes.add("sketch")
        elif path.startswith("kernel/occt/") or (path.startswith("tests/test.data/") and Path(path).suffix.lower() in {".step", ".stp", ".brep", ".iges", ".igs", ".stl"}):
            scopes.add("geometry")
        elif path.startswith("workers/geometry/") or path.startswith("kernel/api/"):
            scopes.add("all")
            reasons.append(f"{path}: shared Geometry Worker boundary")
        elif path.startswith("services/internal/workspace/") or path.startswith("services/internal/modelcore/") or path.startswith("tests/test.data/sketch/"):
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
            "Development tooling",
            shlex.join([sys.executable, "-m", "unittest", "discover", "-s", "tests/python", "-p", "test_*.py"]),
        )
        add("C++ build", f"cmake --build {build_dir} --parallel")
        add("CTest", f"ctest --test-dir {build_dir} --output-on-failure --no-tests=error")
        add("Go packages", "go test -json -count=1 -p 1 ./...", PROJECT_ROOT / "services")
        add("Go conformance", "go test -json -count=1 -p 1 ./...", PROJECT_ROOT / "tests" / "go")
        add("Web scenarios", "pnpm test", PROJECT_ROOT / "web" / "apps" / "cad")
        add("Web production build", "pnpm build", PROJECT_ROOT / "web")
        return steps

    if not match and scopes & {"assembly", "geometry", "sketch", "workspace", "services"}:
        add("Integration native build", f"cmake --build {build_dir} --target occccad_geometry_worker "
            "occcad_geometry_scenarios occccad_naming_artifact_tests --parallel")

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
                "go test -json -count=1 -p 1 ./internal/geometry ./internal/workspace ./internal/control",
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
                "go test -json -count=1 -p 1 ./internal/geometry ./internal/control",
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
            add("Sketch workspace", "go test -json -count=1 -p 1 ./internal/workspace", PROJECT_ROOT / "services")
            add("Sketch Web scenarios", "pnpm test -- sketch", PROJECT_ROOT / "web" / "apps" / "cad")
    if "workspace" in scopes:
        add(
            "Workspace and model core",
            "go test -json -count=1 -p 1 ./internal/workspace ./internal/modelcore"
            + (f" -run {shlex.quote(match)}" if match else ""),
            PROJECT_ROOT / "services",
        )
    if "services" in scopes:
        command = "go test -json -count=1 -p 1 ./..."
        add("Go packages", command + (f" -run {shlex.quote(match)}" if match else ""), PROJECT_ROOT / "services")
        if not match:
            add("Go conformance", "go test -json -count=1 -p 1 ./...", PROJECT_ROOT / "tests" / "go")
    if "web" in scopes:
        add(
            "Web scenarios",
            f"pnpm test{f' -- {shlex.quote(match)}' if match else ''}",
            PROJECT_ROOT / "web" / "apps" / "cad",
        )
        if not match:
            add("Web production build", "pnpm build", PROJECT_ROOT / "web")
    return steps


@task(help={
    "scope": "Comma-separated: assembly, geometry, sketch, workspace, services, web, all",
    "changed": "Select scopes conservatively from Git changes",
    "base": "Compare this commit against the worktree (includes untracked files)",
    "head": "Compare committed branch changes from --base to this commit",
    "staged": "Check only staged changes against --base or HEAD",
    "verbose": "Stream subprocess output",
    "match": "Required test regex/substring within one explicit non-all scope",
    "plan": "Show the selection and commands without executing",
    "json": "Emit a machine-readable --plan",
    "build_type": "Debug or Release",
})
def check(c, scope=None, changed=False, verbose=False, match=None, plan=False, build_type=None,
          base=None, head=None, staged=False, json=False):
    """Run scoped validation against the shared development environment."""
    valid_scopes = {"assembly", "geometry", "sketch", "workspace", "services", "web", "all"}
    if scope and (changed or base or head or staged):
        raise Exit("Use an explicit --scope or Git change selection, not both.")
    if json and not plan:
        raise Exit("--json requires --plan")
    if match and not scope:
        raise Exit("--match requires one explicit --scope.")
    reasons, paths = [], []
    if scope:
        scopes = {item.strip() for item in scope.split(",") if item.strip()}
        if not scopes or scopes - valid_scopes:
            raise Exit(f"Unknown or empty validation scope: {scope}")
        if match and (len(scopes) != 1 or "all" in scopes):
            raise Exit("--match requires exactly one non-all scope.")
    else:
        paths = _changed_files(base, head, staged)
        scopes, reasons = _validation_scopes_for_paths(paths)
    bt = _selected_build_type(build_type)
    steps = _check_steps(scopes, bt, match)
    selection = {"schema": "occccad.validation-plan.v1", "scopes": sorted(scopes), "paths": paths,
                 "base": base or "HEAD", "head": head, "staged": staged, "match": match,
                 "buildType": bt, "reasons": reasons,
                 "steps": [{"name": name, "command": command, "cwd": str(cwd)} for name, command, cwd in steps]}
    if plan:
        if json:
            print(jsonlib.dumps(selection, indent=2, ensure_ascii=False))
        else:
            print(f"[check] PLAN scopes: {', '.join(sorted(scopes)) or ('docs-only' if paths else 'clean workspace')}")
            for reason in reasons:
                print(f"[check] Escalation: {reason}")
            for index, (name, command, cwd) in enumerate(steps, 1):
                print(f"[check] {index}. {name}: cd {cwd} && {command}")
            if not steps:
                print("[check] No executable validation selected")
        return selection
    if not steps:
        print("[check] NOT_RUN: no executable validation selected; knowledge changes require invoke context-audit")
        return selection
    if any("cmake" in command or "ctest" in command for _, command, _ in steps):
        if not _get_build_dir(bt).exists():
            raise Exit(f"Build dir {_get_build_dir(bt)} not found. Run 'invoke configure' first.")
    print(f"[check] Running scopes: {', '.join(sorted(scopes))}" + (f"; match: {match}" if match else ""))
    return run_steps(c, steps, PROJECT_ROOT, _development_environment(bt), verbose, bool(match), selection)



def _context_audit() -> tuple[list[str], list[tuple[int, str]]]:
    """Audit durable Agent entrypoints without indexing generated/runtime content."""
    errors: list[str] = []
    from tools.task_context import audit as task_routes_audit
    errors.extend(task_routes_audit(PROJECT_ROOT))
    root_guide = PROJECT_ROOT / "AGENTS.md"
    root_text = root_guide.read_text(encoding="utf-8")
    if len(root_text.splitlines()) > 100 or root_guide.stat().st_size > 7000:
        errors.append("root AGENTS.md exceeded 100 lines or 7 KB")

    local_guides = [
        PROJECT_ROOT / "kernel" / "assembly" / "AGENTS.md",
        PROJECT_ROOT / "kernel" / "occt" / "AGENTS.md",
        PROJECT_ROOT / "workers" / "geometry" / "AGENTS.md",
        PROJECT_ROOT / "services" / "AGENTS.md",
        PROJECT_ROOT / "web" / "apps" / "cad" / "AGENTS.md",
        PROJECT_ROOT / "docs" / "AGENTS.md",
    ]
    for guide in local_guides:
        if not guide.exists():
            errors.append(f"missing local guide: {guide.relative_to(PROJECT_ROOT)}")
        elif len(guide.read_text(encoding="utf-8").splitlines()) > 100 or guide.stat().st_size > 7000:
            errors.append(f"oversized local guide: {guide.relative_to(PROJECT_ROOT)}")

    project_map = PROJECT_ROOT / "docs" / "README.md"
    if project_map.exists() and project_map.stat().st_size > 5000:
        errors.append("docs/README.md exceeded the 5 KB project-map budget")
    if (PROJECT_ROOT / "save-token.md").exists() or (PROJECT_ROOT / "save-token-plan.md").exists():
        errors.append("superseded root token prompt is present; maintain focused docs instead")
    if Path(__file__).stat().st_size > 40000:
        errors.append("tasks.py exceeded the 40 KB agent-large threshold; split validation tooling")

    from tools.documentation_audit import audit, navigation_errors, structure_errors
    errors.extend(audit(PROJECT_ROOT))
    errors.extend(navigation_errors(PROJECT_ROOT))
    errors.extend(structure_errors(PROJECT_ROOT))

    listed = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        cwd=PROJECT_ROOT,
        check=True,
        capture_output=True,
    ).stdout.split(b"\0")
    hotspots: list[tuple[int, str]] = []
    binary_suffixes = {".brep", ".glb", ".ico", ".jpg", ".png", ".step", ".wasm"}
    for encoded in listed:
        if not encoded:
            continue
        relative = encoded.decode("utf-8")
        path = PROJECT_ROOT / relative
        if not path.is_file() or path.suffix.lower() in binary_suffixes:
            continue
        size = path.stat().st_size
        if size < 40000:
            continue
        if b"\0" not in path.read_bytes()[:8192]:
            hotspots.append((size, relative))
    hotspots.sort(reverse=True)
    return errors, hotspots


@task(help={"verbose": "List every workspace text file above the 40 KB context threshold"})
def context_audit(c, verbose=False):
    """Check Agent routing/doc health and report large workspace text hotspots."""
    del c
    errors, hotspots = _context_audit()
    if errors:
        print("[context-audit] FAIL")
        for error in errors:
            print(f"  - {error}")
        raise Exit(code=1)
    strong = sum(size >= 120000 for size, _ in hotspots)
    print(f"[context-audit] PASS guides/links/navigation/layout; large text: {len(hotspots)}, strong candidates: {strong}")
    from tools.documentation_audit import entry_sizes, sizes
    entries = entry_sizes(sizes(PROJECT_ROOT))
    local = list(entries["localTask"].values())
    print(f"[context-audit] entry bytes: local task {min(local)}..{max(local)} (root + one code guide); project map +{entries['projectMap']}")
    if verbose:
        for size, path in hotspots:
            print(f"  {size:>8}  {path}")


@task(help={"count": "Repeated benchmark samples for benchstat-compatible output"})
def performance_baseline(c, count=5):
    """Run deterministic CAD hot-path benchmarks and save a comparable baseline."""
    output = PROJECT_ROOT / "build" / "performance"
    output.mkdir(parents=True, exist_ok=True)
    target = output / "go-workspace.txt"
    if int(count) < 1:
        raise Exit("--count must be positive")
    environment = _development_environment()

    def capture(command, cwd, path):
        with c.cd(str(cwd)):
            result = c.run(command, hide=True, warn=True, env=environment)
        path.write_text(result.stdout + result.stderr)
        if not result.ok:
            print(_compact_failure_output(result.stdout + result.stderr))
            print(f"[performance] FAIL; full log: {path.relative_to(PROJECT_ROOT)}")
            raise Exit(code=result.exited or 1)

    capture(f"go test ./internal/workspace -run '^$' -bench 'Benchmark(ProfileBuilder|VisualizationManifest)' "
            f"-benchmem -count={int(count)}", PROJECT_ROOT / "services", target)
    measurements = target.read_text()
    for family in ("ProfileBuilder", "VisualizationManifest"):
        samples = re.findall(r"^Benchmark" + family + r"\S*\s+\d+\s+[0-9.eE+-]+\s+ns/op", measurements, re.M)
        if len(samples) < int(count):
            raise Exit(f"Benchmark{family}: expected {count} measured runs; got {len(samples)}. See {target}")
    assembly_target = "occcad_assembly_solver_benchmark"
    build.body(c, target=assembly_target)
    assembly_output = output / "assembly-solver.txt"
    executable = _get_build_dir() / "kernel" / "assembly" / "tests" / assembly_target
    capture(shlex.quote(str(executable)), PROJECT_ROOT, assembly_output)
    native = assembly_output.read_text()
    if "ns/op" not in native or "valid=false" in native:
        raise Exit(f"Native benchmark produced missing/invalid measurements. See {assembly_output}")
    from tools.development_environment import environment_identity
    from tools.build_identity import source_identity, file_digest
    (output / "hot-path-environment.json").write_text(jsonlib.dumps({
        "schema": "occccad.hot-path-measurement.v1", "environment": environment_identity(environment),
        "source": source_identity(PROJECT_ROOT), "goCount": int(count),
        "outputs": {path.name: file_digest(path) for path in (target, assembly_output)},
        "boundary": "ProfileBuilder/VisualizationManifest and native static/warm scenes; not the full performance catalog",
    }, indent=2) + "\n")
    print(f"[performance] Baseline written to {target}")
    print(f"[performance] Assembly baseline written to {assembly_output}")



@task(help={
    "samples": "Samples per scene (2..100, default 5)",
    "build_type": "Configured native build type (default OCCCCAD_BUILD_TYPE or Release)",
    "stages": "Comma-separated: static,interaction,service,allocations (default native stages)",
})
def performance_assembly(c, samples=5, build_type=None, stages="static,interaction"):
    """Measure the assembly solver serially using existing optimized artifacts."""
    c.run(shlex.join([
        sys.executable, str(PROJECT_ROOT / "kernel/assembly/tests/run_performance.py"),
        "--build-dir", str(_get_build_dir(build_type)), "--samples", str(samples),
        "--stages", *(stage.strip() for stage in stages.split(",")),
    ]), pty=True)


@task(help={"build_type": "Debug or Release"})
def run_geometry(c, build_type=None):
    """Build and run the C++ geometry tests."""
    bt = _selected_build_type(build_type)
    build_dir = _get_build_dir(bt)

    build.body(c, build_type=bt, target="occcad_geometry_scenarios")
    c.run(f"ctest --test-dir {build_dir} --output-on-failure -R '^geometry/'", pty=True)


@task(help={"build_type": "Debug or Release"})
def run_worker(c, build_type=None):
    """Build and start the Geometry Worker gRPC server."""
    bt = _selected_build_type(build_type)
    build_dir = _get_build_dir(bt)
    worker_bin = build_dir / "workers" / "geometry" / "occccad_geometry_worker"
    build.body(c, build_type=bt, target="occccad_geometry_worker")
    environment = _development_environment(bt)
    c.run(shlex.quote(environment["OCCCCAD_GEOMETRY_WORKER_BIN"]), env=environment, pty=True)



@task
def run_server(c):
    """Start the standalone Go API server (does not build or serve the frontend)."""
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run("go run ./cmd/occccad-server", env=_development_environment(), pty=True)


@task
def run_jobs(c):
    """Start the artifact and STEP job worker."""
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run("go run ./cmd/occccad-jobs", env=_development_environment(), pty=True)


def _reset_development_data(c):
    """Clear the configured database, ArtifactStore and staging, then migrate."""
    print("[data.reset] Clearing the configured database, ArtifactStore (including S3) and local staging; stop API/Jobs before resetting...")
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
    bt = _selected_build_type(build_type)
    if reset_data:
        _reset_development_data(c)
    build.body(c, build_type=bt, target="occccad_geometry_worker")
    service_build = PROJECT_ROOT / "build" / "services"
    service_build.mkdir(parents=True, exist_ok=True)
    with c.cd(str(PROJECT_ROOT / "services")):
        c.run(shlex.join([
            "go", "build", "-o", str(service_build),
            "./cmd/occccad-server", "./cmd/occccad-jobs", "./cmd/occccad-control",
        ]))
    from tools.build_identity import record_runtime
    from uuid import uuid4
    environment = _development_environment(bt)
    environment["OCCCCAD_RUNTIME_ID"] = uuid4().hex
    record_runtime(PROJECT_ROOT, environment)
    c.run(
        str(service_build / "occccad-control"),
        env=environment,
        pty=True,
    )


@task(help={"mode": "mock (no backend) or api (proxy /api to the backend)"})
def run_web(c, mode="api"):
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
    # Give Bubble Tea the real terminal rather than a nested Invoke PTY.
    os.execv(monitor_binary, [str(monitor_binary)])


@task
def build_web(c):
    """Type-check and build the current web application."""
    with c.cd(str(PROJECT_ROOT / "web")):
        c.run("pnpm build", pty=True)


@task
def clean(c):
    """Remove all build artifacts."""
    build_root = PROJECT_ROOT / "build"
    if build_root.exists():
        print(f"[clean] Removing {build_root}")
        shutil.rmtree(build_root)

    (PROJECT_ROOT / "compile_commands.json").unlink(missing_ok=True)
    (PROJECT_ROOT / "CMakeUserPresets.json").unlink(missing_ok=True)

    print("[clean] Done.")


run_collection = Collection("run")
run_collection.add_task(run_geometry, "geometry")
run_collection.add_task(run_worker, "worker")
run_collection.add_task(run_server, "server")
run_collection.add_task(run_jobs, "jobs")
run_collection.add_task(run_app, "app", default=True)
run_collection.add_task(run_web, "web")
run_collection.add_task(run_monitor, "monitor")

web_collection = Collection("web")
web_collection.add_task(build_web, "build")

data_collection = Collection("data")
data_collection.add_task(reset_data, "reset")

performance_collection = Collection("performance")
performance_collection.add_task(performance_assembly, "assembly")

ns = Collection()
ns.add_task(info)
ns.add_task(doctor)
ns.add_task(task_context, "task")
ns.add_task(workflow_baseline, "workflow-baseline")
ns.add_task(diagnose)
ns.add_task(bootstrap)
ns.add_task(configure)
ns.add_task(build)
ns.add_task(test)
ns.add_task(check)
ns.add_task(context_audit, "context-audit")
ns.add_task(performance_baseline, "performance-baseline")
ns.add_task(clean)
ns.add_collection(run_collection)
ns.add_collection(web_collection)
ns.add_collection(data_collection)
ns.add_collection(performance_collection)
