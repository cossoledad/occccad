"""Serial assembly performance measurements from existing optimized artifacts."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
import platform
import re
import statistics
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT))
import tasks  # noqa: E402; reuse the repository's non-shell env-file loader


def native_summary(static: str, interaction: str) -> list[dict]:
    rows = []
    rotations = []
    for line in static.splitlines():
        match = re.match(r"AssemblyPlaneChain(\d+) (\d+) (\d+) ns/op (.*)", line)
        if match:
            fields = dict(token.split("=", 1) for token in match[4].split() if "=" in token)
            if fields.get("valid") != "true":
                raise ValueError("Invalid static solve")
            rows.append({"scene": "plane" + match[1], "statistic": "mean", "samples": int(match[2]),
                         "total_ms": int(match[3]) / 1e6, "last_sample": fields})
        elif line.startswith("AssemblyWarmRotation30 "):
            fields = dict(token.split("=", 1) for token in line.split()[1:] if "=" in token)
            if fields.get("valid") != "true":
                raise ValueError("Invalid BFGS solve")
            rotations.append(float(fields["ms"]))
    if rotations:
        rows.append({"scene": "warm-rotation30", "statistic": "median", "samples": len(rotations),
                     "total_ms": statistics.median(rotations), "samples_ms": rotations})
    scenes = {}
    for line in interaction.splitlines():
        if not line.startswith("{"):
            continue
        sample = json.loads(line)
        if sample.get("kind") != "sample":
            continue
        if not sample["valid"]:
            raise ValueError("Invalid interaction: " + sample["scene"])
        scenes.setdefault(sample["scene"], []).append(sample)
    for scene, samples in scenes.items():
        times = [sample["kernel_ms"] for sample in samples]
        fields = [key for key in samples[0] if key.endswith("_ms") and key != "kernel_ms"]
        rows.append({"scene": scene, "statistic": "median", "samples": len(samples),
                     "total_ms": statistics.median(times), "samples_ms": times,
                     "eligible": all(sample["eligible"] for sample in samples),
                     "phases_ms": {key: statistics.median(sample[key] for sample in samples) for key in fields}})
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--build-dir", default=str(tasks._get_build_dir()))
    parser.add_argument("--label", default=datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ"))
    parser.add_argument("--samples", type=int, default=5)
    parser.add_argument("--stages", nargs="+", choices=["static", "interaction", "service", "allocations"], default=["static", "interaction"])
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]{0,79}", args.label) or not 2 <= args.samples <= 100:
        parser.error("label must be a safe directory name; samples must be 2..100")
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
    native = build / "kernel/assembly/tests"
    binaries = {"static": native / "occcad_assembly_solver_benchmark",
                "interaction": native / "occcad_assembly_interaction_benchmark",
                "service": build / "workers/geometry/occccad_geometry_worker"}
    for stage in args.stages:
        if stage in binaries and not os.access(binaries[stage], os.X_OK):
            parser.error(f"missing executable {binaries[stage]}; build the selected targets first")
    out = ROOT / "build/performance/assembly" / args.label
    if out.exists():
        parser.error(f"output already exists: {out}; choose a new label")
    out.mkdir(parents=True)
    metadata = {"configuration": config, "flags": flags, "compiler": cache["CMAKE_CXX_COMPILER"],
                "toolchain": cache.get("CMAKE_TOOLCHAIN_FILE"), "samples": args.samples,
                "system": platform.platform(), "cpu_count": os.cpu_count(),
                "cpu": next((l.split(":", 1)[1].strip() for l in Path("/proc/cpuinfo").read_text().splitlines() if l.startswith("model name")), "unknown"),
                "head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
                "go": subprocess.check_output(["go", "version"], text=True).strip() if {"service", "allocations"} & set(args.stages) else None,
                "compiler_version": subprocess.check_output([cache["CMAKE_CXX_COMPILER"], "--version"], text=True).splitlines()[0],
                "stages": args.stages, "started_at": datetime.now(timezone.utc).isoformat(),
                "worktree": subprocess.check_output(["git", "status", "--short"], cwd=ROOT, text=True),
                "binaries": {stage: {"path": str(binary), "sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
                                     "mtime_ns": binary.stat().st_mtime_ns}
                             for stage, binary in binaries.items() if stage in args.stages},
                "solver_sha256": hashlib.sha256((ROOT / "kernel/assembly/src/solver.cpp").read_bytes()).hexdigest(),
                "measurement": "serial wall time; inclusive phases; no browser/rendering"}
    (out / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")

    def run(name, command, cwd=ROOT, env=None):
        if name not in args.stages:
            return
        print(f"[assembly-performance] {args.label}: {name}", flush=True)
        with (out / (name + ".txt")).open("w") as log:
            result = subprocess.run(command, cwd=cwd, env=env, stdout=log, stderr=subprocess.STDOUT)
        if result.returncode:
            raise SystemExit(f"{name} failed (exit {result.returncode}); inspect {out / (name + '.txt')}")

    run("static", [str(native / "occcad_assembly_solver_benchmark"), str(args.samples)])
    run("interaction", [str(native / "occcad_assembly_interaction_benchmark"), str(args.samples)])
    env = dict(os.environ, OCCCCAD_RUN_ASSEMBLY_PERFORMANCE="1", OCCCCAD_PERFORMANCE_SAMPLES=str(args.samples),
               OCCCCAD_TEST_GEOMETRY_WORKER=str(build / "workers/geometry/occccad_geometry_worker"))
    run("service", ["go", "test", "./internal/control", "-run", "^TestAssemblyPerformance$", "-count=1", "-v", "-timeout=15m"], ROOT / "services", env)
    if "service" in args.stages and "ENVIRONMENT_BLOCKED" in (out / "service.txt").read_text():
        raise SystemExit("Service measurements blocked: inspect service.txt")
    run("allocations", ["go", "test", "./internal/workspace", "./internal/geometry", "-run", "^$", "-bench", "BenchmarkAssembly", "-benchmem", "-benchtime=200ms", "-count=3"], ROOT / "services", env)
    rows = native_summary((out / "static.txt").read_text() if "static" in args.stages else "",
                          (out / "interaction.txt").read_text() if "interaction" in args.stages else "")
    expected = {"static": {"plane5", "plane15", "plane30", "warm-rotation30"},
                "interaction": {"single", "connected50-200", "independent50", "group-contact",
                                "grounded-failure", "cancelled", "budget", "invalid-input"}}
    scenes = set().union(*(expected.get(stage, set()) for stage in args.stages))
    if {row["scene"] for row in rows} != scenes or len(rows) != len(scenes) or any(row["samples"] != args.samples for row in rows):
        raise SystemExit(f"Missing/incomplete native measurements; inspect {out}")
    (out / "summary.json").write_text(json.dumps({"native": rows, "stages": args.stages}, indent=2) + "\n")
    for row in rows:
        print(f"[assembly-performance] {row['scene']}: {row['total_ms']:.3f} ms ({row['statistic']}, n={row['samples']})", flush=True)
    print(f"[assembly-performance] saved {out}", flush=True)


if __name__ == "__main__":
    main()
