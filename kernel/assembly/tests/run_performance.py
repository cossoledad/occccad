"""Serial optimized kernel + real Router/Session performance evidence."""
import argparse
import hashlib
import json
import os
import platform
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT))
import tasks  # noqa: E402; reuse the repository's non-shell env-file loader


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--build-dir", default="build/cmake/performance")
    parser.add_argument("--label", required=True)
    parser.add_argument("--samples", type=int, default=5)
    parser.add_argument("--stages", nargs="+", choices=["static", "interaction", "service", "allocations"], default=["static", "interaction", "service", "allocations"])
    args = parser.parse_args()
    if args.label not in {"before", "after"} or not 2 <= args.samples <= 100:
        parser.error("label must be before/after; samples must be 2..100")
    build = (ROOT / args.build_dir).resolve()
    cache = {}
    for line in (build / "CMakeCache.txt").read_text().splitlines():
        if line and not line.startswith(("#", "//")) and "=" in line:
            key, value = line.split("=", 1)
            cache[key.split(":")[0]] = value
    config = cache["CMAKE_BUILD_TYPE"]
    flags = cache.get("CMAKE_CXX_FLAGS", "") + " " + cache.get("CMAKE_CXX_FLAGS_" + config.upper(), "")
    if "-O2" not in flags and "-O3" not in flags:
        parser.error("performance measurements require an optimized build")
    for sanitizer in ("ASAN", "UBSAN", "TSAN"):
        if cache.get("OCCCCAD_ENABLE_" + sanitizer) == "ON":
            parser.error("performance measurements require sanitizers off")
    out = ROOT / "build/performance/assembly-round1" / args.label
    out.mkdir(parents=True, exist_ok=True)
    metadata = {"configuration": config, "flags": flags, "compiler": cache["CMAKE_CXX_COMPILER"],
                "toolchain": cache.get("CMAKE_TOOLCHAIN_FILE"), "samples": args.samples,
                "system": platform.platform(), "cpu_count": os.cpu_count(),
                "cpu": next((l.split(":", 1)[1].strip() for l in Path("/proc/cpuinfo").read_text().splitlines() if l.startswith("model name")), "unknown"),
                "head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
                "go": subprocess.check_output(["go", "version"], text=True).strip(),
                "solver_sha256": hashlib.sha256((ROOT / "kernel/assembly/src/solver.cpp").read_bytes()).hexdigest(),
                "measurement": "serial wall time; inclusive phases; no browser/rendering"}
    (out / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")

    def run(name, command, cwd=ROOT, env=None):
        if name not in args.stages:
            return
        print(f"[assembly-performance] {args.label}: {name}", flush=True)
        with (out / (name + ".txt")).open("w") as log:
            subprocess.run(command, cwd=cwd, env=env, stdout=log, stderr=subprocess.STDOUT, check=True)

    native = build / "kernel/assembly/tests"
    run("static", [str(native / "occcad_assembly_solver_benchmark"), str(args.samples)])
    run("interaction", [str(native / "occcad_assembly_interaction_benchmark"), str(args.samples)])
    env = dict(os.environ, OCCCCAD_RUN_ASSEMBLY_PERFORMANCE="1", OCCCCAD_PERFORMANCE_SAMPLES=str(args.samples),
               OCCCCAD_TEST_GEOMETRY_WORKER=str(build / "workers/geometry/occccad_geometry_worker"))
    run("service", ["go", "test", "./internal/control", "-run", "^TestAssemblyPerformance$", "-count=1", "-v", "-timeout=15m"], ROOT / "services", env)
    if "service" in args.stages and "ENVIRONMENT_BLOCKED" in (out / "service.txt").read_text():
        raise SystemExit("Service measurements blocked: inspect service.txt")
    run("allocations", ["go", "test", "./internal/workspace", "./internal/geometry", "-run", "^$", "-bench", "BenchmarkAssembly", "-benchmem", "-benchtime=200ms", "-count=3"], ROOT / "services", env)
    print(f"[assembly-performance] saved {out}", flush=True)


if __name__ == "__main__":
    main()
