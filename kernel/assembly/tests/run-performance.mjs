// Serial native/service measurements. Phases are inclusive; no browser timing.
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { parseArgs } from "node:util";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import {
  ROOT,
  fileDigest,
  command,
} from "../../../tests/assembly-contract/runner.mjs";
const require = (p, m) => {
  if (!p) throw Error(m);
};
export const median = (vs) => {
  vs = [...vs].sort((a, b) => a - b);
  return (
    (vs[Math.floor((vs.length - 1) / 2)] + vs[Math.floor(vs.length / 2)]) / 2
  );
};
const fields = (s) =>
  Object.fromEntries(
    s
      .trim()
      .split(/\s+/)
      .filter((s) => s.includes("="))
      .map((s) => s.split("=")),
  );
export function nativeSummary(staticText, interaction) {
  const rows = [],
    rotations = [];
  for (const l of staticText.split("\n")) {
    const m = l.match(/^AssemblyPlaneChain(\d+) (\d+) (\d+) ns\/op (.*)/);
    if (m) {
      const f = fields(m[4]);
      require(f.valid === "true", "Invalid static solve");
      rows.push({
        scene: "plane" + m[1],
        statistic: "mean",
        samples: Number(m[2]),
        total_ms: Number(m[3]) / 1e6,
        last_sample: f,
      });
    } else if (l.startsWith("AssemblyWarmRotation30 ")) {
      const f = fields(l.slice("AssemblyWarmRotation30 ".length));
      require(f.valid === "true", "Invalid BFGS solve");
      rotations.push(Number(f.ms));
    }
  }
  if (rotations.length)
    rows.push({
      scene: "warm-rotation30",
      statistic: "median",
      samples: rotations.length,
      total_ms: median(rotations),
      samples_ms: rotations,
    });
  const scenes = {};
  for (const l of interaction.split("\n")) {
    if (!l.startsWith("{")) continue;
    const sample = JSON.parse(l);
    if (sample.kind !== "sample") continue;
    require(sample.valid, "Invalid interaction: " + sample.scene);
    (scenes[sample.scene] ??= []).push(sample);
  }
  for (const [scene, samples] of Object.entries(scenes)) {
    const times = samples.map((s) => s.kernel_ms),
      phases = Object.keys(samples[0]).filter(
        (k) => k.endsWith("_ms") && k !== "kernel_ms",
      );
    rows.push({
      scene,
      statistic: "median",
      samples: samples.length,
      total_ms: median(times),
      samples_ms: times,
      eligible: samples.every((s) => s.eligible),
      phases_ms: Object.fromEntries(
        phases.map((k) => [k, median(samples.map((s) => s[k]))]),
      ),
    });
  }
  return rows;
}
export function serviceSamples(raw, count) {
  const events = raw.split("\n").flatMap((l) => {
    try {
      return [JSON.parse(l)];
    } catch {
      return [];
    }
  });
  require(events.some((e) => e.Action === "run") &&
    events.some((e) => e.Action === "pass") &&
    !events.some((e) =>
      ["skip", "fail"].includes(e.Action),
    ), "Service execution evidence missing/skipped/failed");
  const rows = events
    .filter((e) => e.Output?.includes("ASSEMBLY_PERFORMANCE "))
    .map((e) => JSON.parse(e.Output.split("ASSEMBLY_PERFORMANCE ")[1]));
  const expected = {};
  for (const scene of [
    "plane5",
    "plane15",
    "plane30",
    "single",
    "connected50-200",
    "independent50",
    "group-contact",
    "grounded-failure",
    "budget",
  ])
    expected["router/" + scene] = count;
  for (const kind of ["session-begin", "session-update"])
    for (const nested of [false, true])
      expected[kind + "/" + nested] = kind === "session-begin" ? 1 : count;
  const actual = {};
  for (const row of rows) {
    const key = row.kind + "/" + (row.scene ?? row.nested);
    actual[key] = (actual[key] ?? 0) + 1;
  }
  require(Object.keys(actual).length === Object.keys(expected).length &&
    Object.entries(expected).every(([k, v]) => actual[k] === v) &&
    rows.every(
      (r) => r.valid !== false,
    ), "Missing/incomplete service measurements");
  return rows;
}
export async function main(args = process.argv.slice(2)) {
  const { values: v } = parseArgs({
    args,
    options: Object.fromEntries(
      ["profile", "samples", "stages", "label", "jobs"].map((k) => [
        k,
        { type: "string" },
      ]),
    ),
  });
  const profile = v.profile ?? "release",
    count = Number(v.samples ?? 5),
    jobs = Number(v.jobs ?? 4),
    stages = (v.stages ?? "static,interaction").split(","),
    label = v.label ?? new Date().toISOString().replace(/[^0-9TZ]/g, "");
  require(/^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$/.test(label) &&
    Number.isInteger(count) &&
    count >= 2 &&
    count <= 100, "safe label and samples 2..100 required");
  require(Number.isInteger(jobs) &&
    jobs > 0 &&
    stages.length &&
    stages.every((s) =>
      ["static", "interaction", "service", "allocations"].includes(s),
    ), "invalid jobs/stages");
  const build = path.join(ROOT, "build/cmake/linux-gcc15-x86_64", profile),
    cache = Object.fromEntries(
      fs
        .readFileSync(path.join(build, "CMakeCache.txt"), "utf8")
        .split("\n")
        .filter(
          (l) =>
            l && !l.startsWith("#") && !l.startsWith("//") && l.includes("="),
        )
        .map((l) => [l.slice(0, l.indexOf(":")), l.slice(l.indexOf("=") + 1)]),
    ),
    config = cache.CMAKE_BUILD_TYPE,
    flags =
      (cache.CMAKE_CXX_FLAGS ?? "") +
      " " +
      (cache["CMAKE_CXX_FLAGS_" + config.toUpperCase()] ?? "");
  require(/-O[23]/.test(flags), "performance requires optimized build");
  require(!["ASAN", "UBSAN", "TSAN"].some(
    (s) => cache["OCCCCAD_ENABLE_" + s] === "ON",
  ), "performance requires sanitizers off");
  const native = path.join(build, "kernel/assembly/tests"),
    binaries = {
      static: path.join(native, "occcad_assembly_solver_benchmark"),
      interaction: path.join(native, "occcad_assembly_interaction_benchmark"),
      service: process.env.OCCCCAD_GEOMETRY_WORKER_BIN,
    };
  for (const s of stages)
    if (s in binaries) {
      require(binaries[s] &&
        fs.existsSync(binaries[s]), "missing executable for " +
        s +
        "; build selected targets first");
      fs.accessSync(binaries[s], fs.constants.X_OK);
    }
  const out = path.join(ROOT, "build/performance/assembly", label);
  require(!fs.existsSync(out), "output already exists; choose a new label");
  fs.mkdirSync(out, { recursive: true });
  const metadata = {
    configuration: config,
    flags,
    compiler: cache.CMAKE_CXX_COMPILER,
    toolchain: cache.CMAKE_TOOLCHAIN_FILE,
    samples: count,
    system: os.platform(),
    cpu_count: os.cpus().length,
    cpu: os.cpus()[0]?.model,
    head: execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: ROOT,
      encoding: "utf8",
    }).trim(),
    compiler_version: execFileSync(cache.CMAKE_CXX_COMPILER, ["--version"], {
      encoding: "utf8",
    }).split("\n")[0],
    stages,
    started_at: new Date().toISOString(),
    binaries: Object.fromEntries(
      Object.entries(binaries)
        .filter(([s]) => stages.includes(s))
        .map(([s, p]) => [
          s,
          {
            path: p,
            sha256: fileDigest(p),
            mtime_ns: fs.statSync(p, { bigint: true }).mtimeNs.toString(),
          },
        ]),
    ),
    solver_sha256: fileDigest(
      path.join(ROOT, "kernel/assembly/src/solver.cpp"),
    ),
    measurement: "serial wall time; inclusive phases; no browser/rendering",
  };
  fs.writeFileSync(
    path.join(out, "environment.json"),
    JSON.stringify(metadata, null, 2) + "\n",
  );
  const env = {
    ...process.env,
    OCCCCAD_RUN_ASSEMBLY_PERFORMANCE: "1",
    OCCCCAD_PERFORMANCE_SAMPLES: String(count),
  };
  const run = async (s, argv, cwd = ROOT) => {
    if (!stages.includes(s)) return;
    const [code] = await command(argv, cwd, env, path.join(out, s + ".txt"));
    require(code === 0, s + " process failed; inspect " + out);
  };
  await run("static", [binaries.static, String(count)]);
  await run("interaction", [binaries.interaction, String(count)]);
  await run(
    "service",
    [
      "go",
      "test",
      "-json",
      "-p",
      String(jobs),
      "-parallel",
      "1",
      "-tags=integration",
      "./internal/control",
      "-run",
      "^TestAssemblyPerformance$",
      "-count=1",
      "-timeout=15m",
    ],
    path.join(ROOT, "services"),
  );
  if (stages.includes("service"))
    fs.writeFileSync(
      path.join(out, "service-samples.json"),
      JSON.stringify(
        serviceSamples(
          fs.readFileSync(path.join(out, "service.txt"), "utf8"),
          count,
        ),
        null,
        2,
      ),
    );
  await run(
    "allocations",
    [
      "go",
      "test",
      "-p",
      String(jobs),
      "./internal/workspace",
      "./internal/geometry",
      "-run",
      "^$",
      "-bench",
      "BenchmarkAssembly",
      "-benchmem",
      "-benchtime=200ms",
      "-count=3",
    ],
    path.join(ROOT, "services"),
  );
  const rows = nativeSummary(
    stages.includes("static")
      ? fs.readFileSync(path.join(out, "static.txt"), "utf8")
      : "",
    stages.includes("interaction")
      ? fs.readFileSync(path.join(out, "interaction.txt"), "utf8")
      : "",
  );
  const expected = {
      static: ["plane5", "plane15", "plane30", "warm-rotation30"],
      interaction: [
        "single",
        "connected50-200",
        "independent50",
        "group-contact",
        "grounded-failure",
        "cancelled",
        "budget",
        "invalid-input",
      ],
    },
    scenes = stages.flatMap((s) => expected[s] ?? []);
  require(rows.length === scenes.length &&
    scenes.every((s) => rows.some((r) => r.scene === s)) &&
    rows.every(
      (r) => r.samples === count,
    ), "Missing/incomplete native measurements");
  for (const [s, b] of Object.entries(metadata.binaries))
    require(fileDigest(b.path) ===
      b.sha256, "SOURCE_CHANGED: measured binary changed: " + s);
  require(fileDigest(path.join(ROOT, "kernel/assembly/src/solver.cpp")) ===
    metadata.solver_sha256, "SOURCE_CHANGED: solver changed");
  fs.writeFileSync(
    path.join(out, "summary.json"),
    JSON.stringify({ native: rows, stages }, null, 2) + "\n",
  );
  console.log("Assembly measurements saved: " + out);
  return 0;
}
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = await main();
  } catch (e) {
    console.error(e.message);
    process.exitCode = 1;
  }
}
