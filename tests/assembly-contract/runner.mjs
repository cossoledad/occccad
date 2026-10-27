// Actual execution evidence. Declarations, Mock results and process success alone
// cannot certify target acceptance. Baseline has no automatic update command.
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import os from "node:os";
import { spawn, execFileSync } from "node:child_process";
import { parseArgs } from "node:util";
import { fileURLToPath } from "node:url";
import { ROOT, DATA, loadCatalog, read } from "./catalog.mjs";
import { slug, anchors } from "../../tools/documentation-audit.mjs";
export { loadCatalog, ROOT };
export const LAYERS = [
  "ui",
  "domain",
  "resolution",
  "workerSolver",
  "lifecycle",
  "historyReplay",
];
const ADAPTERS = ["cpp", "go", "go-flags", "web", "web-catalog", "integration"];
const STATES = ["implemented", "partial", "missing", "unknown"];
export const TASK_TOPICS = {
  "CONSTRAINT-ACTIVATION": "激活模式连接和求值状态正交",
  "CONSTRAINT-GEOMETRY": "支持几何与参数门",
  "CONSTRAINT-COINCIDENCE": "coincidence",
  "CONSTRAINT-OFFSET": "offset-与-measure",
  "CONSTRAINT-ANGLE": "angle空间角指定轴角与方向关系",
  "CONSTRAINT-FIX": "fix-与-fix-together",
  "CONSTRAINT-FIX-TOGETHER": "fix-与-fix-together",
  "CONSTRAINT-CONTACT": "contact",
  "CONSTRAINT-COMPOSITION": "自由度控制与组合能力",
};
const require = (p, m) => {
    if (!p) throw Error(m);
  },
  keys = Object.keys,
  eq = (a, b) =>
    JSON.stringify([...a].sort()) === JSON.stringify([...b].sort()),
  subset = (a, b) => a.every((x) => b.includes(x));
const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
// Retain the existing baseline encoding, including Python's two-digit exponent.
export function canonical(v) {
  if (Array.isArray(v)) return "[" + v.map(canonical).join(",") + "]";
  if (v && typeof v === "object")
    return (
      "{" +
      keys(v)
        .sort()
        .map((k) => JSON.stringify(k) + ":" + canonical(v[k]))
        .join(",") +
      "}"
    );
  if (typeof v === "number")
    return JSON.stringify(v).replace(
      /e([+-]?)(\d)$/,
      (_, s, n) => "e" + (s || "+") + "0" + n,
    );
  return JSON.stringify(v);
}
export const digest = (v) =>
  crypto.createHash("sha256").update(canonical(v)).digest("hex");
export const fileDigest = (p) =>
  crypto.createHash("sha256").update(fs.readFileSync(p)).digest("hex");
export function assertionSource(t, root = ROOT) {
  const source = fs.readFileSync(path.join(root, t.fixture.source), "utf8");
  let extra = "";
  for (const ref of t.fixture.assertionSources ?? []) {
    const text = fs.readFileSync(path.join(root, ref.source), "utf8"),
      m = new RegExp("\\b(?:func|void)\\s+" + esc(ref.symbol) + "\\s*\\(").exec(
        text,
      );
    require(m, "missing additional assertion source " + ref.symbol);
    extra +=
      "\n" + text.slice(m.index).split(/\n(?:func |TEST\(|}\s*(?:\n|$))/)[0];
  }
  if (t.adapter === "cpp") {
    const [suite, test] = t.selector.split("."),
      m = new RegExp(
        "TEST\\(\\s*" + esc(suite) + "\\s*,\\s*" + esc(test) + "\\s*\\)",
      ).exec(source);
    require(m, "missing C++ test");
    let body = source.slice(m.index).split(/\nTEST\(/)[0];
    const helper = t.fixture.assertionHelperSymbol;
    if (helper) {
      const h = new RegExp("\\bvoid\\s+" + esc(helper) + "\\s*\\(").exec(
        source,
      );
      require(h, "missing C++ assertion helper " + helper);
      body += "\n" + source.slice(h.index).split(/\n}\s*(?:\n|$)/)[0];
    }
    return body + extra;
  }
  if (["go", "go-flags", "integration"].includes(t.adapter)) {
    const start = source.indexOf("func " + t.selector.split("/")[0] + "(");
    require(start >= 0, "missing Go test");
    let body = source.slice(start).split(/\nfunc /)[0],
      helper = t.fixture.assertionHelperSymbol;
    if (helper) {
      require(new RegExp("\\b" + esc(helper) + "\\s*\\(").test(
        body,
      ), "Go assertion helper is not called: " + helper);
      const start = source.indexOf("func " + helper + "(");
      require(start >= 0, "missing Go assertion helper " + helper);
      body += "\n" + source.slice(start).split(/\nfunc /)[0];
    }
    return body + extra;
  }
  return source + extra;
}
export const implementation = (c, catalog) => ({
  ...catalog.implementationProfiles[c.implementationProfile],
  ...c.implementationOverrides,
});
export function frozenContract(c) {
  return {
    schemaVersion: c.schemaVersion,
    contractVersion: c.contractVersion,
    semantics: digest(
      Object.fromEntries(
        [
          "families",
          "layers",
          "descriptors",
          "derivedSupports",
          "policies",
          "failurePolicies",
          "tolerances",
        ].map((k) => [k, c[k]]),
      ),
    ),
    capabilities: Object.fromEntries(
      c.capabilities.map((t) => [
        t.capabilityId,
        digest(
          Object.fromEntries(
            Object.entries(t).filter(
              ([k]) =>
                ![
                  "caseIds",
                  "implementationProfile",
                  "implementationOverrides",
                ].includes(k),
            ),
          ),
        ),
      ]),
    ),
    cases: Object.fromEntries(c.cases.map((t) => [t.caseId, digest(t)])),
    assertions: Object.fromEntries(
      c.cases.map((t) => [t.caseId, digest(assertionSource(t))]),
    ),
    coverageFloor: Object.fromEntries(
      c.capabilities.map((t) => [
        t.capabilityId,
        Object.fromEntries(
          Object.entries(implementation(t, c)).map(([k, v]) => [k, v.state]),
        ),
      ]),
    ),
  };
}
export function validate(c, { root = ROOT, lock = null } = {}) {
  require(c.schemaVersion === 1, "unknown schemaVersion");
  require(c.contractVersion, "missing contractVersion");
  require(eq(c.families, [
    "Coincidence",
    "Contact",
    "Offset",
    "Angle",
    "Fix",
    "FixTogether",
  ]), "six families required");
  require(eq(c.layers, LAYERS), "coverage layers incomplete");
  require(fs.existsSync(path.join(root, c.target)), "missing target contract");
  const targetAnchors = anchors(
    fs.readFileSync(path.join(root, c.target), "utf8"),
  );
  require(c.capabilities.length && c.cases.length, "empty catalog");
  const capIds = c.capabilities.map((x) => x.capabilityId),
    caseIds = c.cases.map((x) => x.caseId);
  require(new Set(capIds).size === capIds.length, "duplicate capabilityId");
  require(new Set(caseIds).size === caseIds.length, "duplicate caseId");
  require([...capIds, ...caseIds].every((x) =>
    /^[a-z][a-z0-9.-]+$/.test(x),
  ), "invalid stable ID");
  for (const [name, d] of Object.entries(c.descriptors)) {
    require(d.fields.length &&
      d.preconditions.length, `descriptor ${name} incomplete`);
    require(eq(
      d.fields,
      keys(d.units),
    ), `descriptor ${name} unit fields missing`);
  }
  require(Object.values(TASK_TOPICS).every((x) =>
    targetAnchors.has(x),
  ), "broken task topic navigation");
  require(fs.existsSync(
    path.join(root, "docs/architecture/current/assembly-constraints.md"),
  ), "missing current constraint contract");
  for (const p of Object.values(c.failurePolicies)) {
    require(new Set(p.map((x) => x.category)).size ===
      p.length, "duplicate failure category");
    require(p.every((f) =>
      [
        "category",
        "phase",
        "saveDefinition",
        "advanceHead",
        "adoptCandidatePose",
        "evaluation",
        "recovery",
      ].every((k) => k in f),
    ), "incomplete failure semantics");
  }
  require(new Set(c.derivedSupports.map((x) => x.supportId)).size ===
    c.derivedSupports.length, "duplicate derived supportId");
  for (const s of c.derivedSupports)
    require(c.descriptors[s.sourceDescriptor] &&
      c.descriptors[s.resultDescriptor] &&
      s.precondition &&
      TASK_TOPICS[s.task], "invalid derived support");
  for (const cap of c.capabilities) {
    const id = cap.capabilityId;
    require(c.families.includes(cap.family) && cap.subtype, id +
      ": invalid family/subtype");
    require(targetAnchors.has(cap.targetAnchor), id +
      ": invalid target anchor");
    require(["unary", "binary", "group"].includes(cap.arity.kind), id +
      ": invalid arity");
    require(cap.arity.min >= 1 && cap.roles.length, id + ": missing roles");
    require(cap.roles.every((r) => c.descriptors[r.descriptor] && r.role), id +
      ": invalid descriptor");
    require(cap.roles.every(
      (r) =>
        !r.supportedDescriptors ||
        (r.supportedDescriptors.length &&
          subset(r.supportedDescriptors, keys(c.descriptors))),
    ), id + ": invalid supported descriptor");
    const p = c.policies[cap.policy];
    for (const k of [
      "direction",
      "sign",
      "exchange",
      "branch",
      "modes",
      "otherModes",
      "activation",
      "failurePolicy",
    ])
      require(p[k], id + ": missing " + k);
    require(Array.isArray(p.parameters), id + ": missing parameters");
    for (const q of p.parameters)
      require(["name", "type", "unit", "domain", "dependsOn"].every(
        (k) => q[k],
      ), id + ": missing parameter semantics");
    require(c.failurePolicies[p.failurePolicy], id +
      ": failure policy missing");
    require(cap.rank.configuration &&
      (cap.rank.normal === null ||
        (cap.rank.normal >= 0 && cap.rank.normal <= 6)), id + ": rank missing");
    require(cap.rank.normal !== null ||
      cap.openQuestions?.length ||
      cap.rank.special, id + ": unknown rank has no boundary");
    require(cap.followupTasks.length &&
      cap.followupTasks.every((t) => TASK_TOPICS[t]), id +
      ": invalid followup task");
    const required = cap.requiredLayers ?? c.layers;
    require(required.length && subset(required, LAYERS), id +
      ": invalid required layers");
    require(subset(cap.requiredCaseIds ?? [], cap.caseIds), id +
      ": invalid required evidence");
    require(subset(cap.caseIds, caseIds), id + ": invalid test mapping");
    const coverage = implementation(cap, c);
    require(eq(keys(coverage), LAYERS), id + ": incomplete coverage");
    for (const [layer, s] of Object.entries(coverage)) {
      require(STATES.includes(s.state) && s.reason, id + ": invalid coverage");
      if (s.source)
        require(fs.existsSync(path.join(root, s.source)), id +
          ": missing implementation source");
      if (s.state === "implemented")
        require(c.cases.some(
          (t) =>
            cap.caseIds.includes(t.caseId) &&
            t.layer === layer &&
            t.purpose !== "unsupported-rejection",
        ), id + ": implemented " + layer + " has no executable test");
    }
  }
  for (const t of c.cases) {
    const id = t.caseId;
    for (const ref of t.fixture.assertionSources ?? [])
      require(ref.symbol &&
        fs.existsSync(path.join(root, ref.source ?? "")), id +
        ": missing additional assertion fixture");
    require(ADAPTERS.includes(t.adapter) && LAYERS.includes(t.layer), id +
      ": invalid adapter/layer");
    require(t.capabilityIds.length && subset(t.capabilityIds, capIds), id +
      ": invalid capability reference");
    require(t.expectation && t.selector, id + ": empty assertion/selector");
    require(t.fixture.symbol === t.selector, id + ": invalid fixture symbol");
    require([
      "specific-combination",
      "representative-shared-foundation",
    ].includes(t.evidenceScope) && t.evidenceKind, id +
      ": missing evidence level");
    require(fs.existsSync(path.join(root, t.fixture.source)), id +
      ": missing fixture");
    if (t.adapter === "cpp")
      require(/(?:ASSERT|EXPECT)_/.test(assertionSource(t, root)), id +
        ": empty C++ assertion body");
    else if (["go", "go-flags", "integration"].includes(t.adapter)) {
      require(t.package.startsWith("./internal/"), id + ": invalid Go package");
      require(/t\.(Fatal|Error|Fail)/.test(assertionSource(t, root)), id +
        ": empty Go assertion body");
      if (t.adapter === "go-flags") {
        require(eq(keys(t.expected), [
          "direction",
          "distanceSide",
          "directedAngle",
        ]) &&
          Object.values(t.expected).every((v) => typeof v === "boolean"), id +
          ": invalid expected flags");
        require(eq(keys(t.input), ["kind", "first", "second"]), id +
          ": invalid capability inputs");
      }
    } else if (t.adapter === "web-catalog")
      require([
        "entry",
        "clear-axis",
        "offset-normal",
        "production-capability",
      ].includes(t.selector) &&
        t.input &&
        t.expected, id + ": invalid web adapter");
    else require(t.selector === t.fixture.source, id + ": invalid web path");
    for (const cap of c.capabilities)
      require(cap.caseIds.includes(id) ===
        t.capabilityIds.includes(cap.capabilityId), id +
        ": nonreciprocal mapping");
  }
  if (lock) {
    const now = frozenContract(c);
    for (const f of ["schemaVersion", "contractVersion", "semantics"])
      require(now[f] === lock[f], "regression lock changed: " + f);
    for (const f of ["capabilities", "cases", "assertions"])
      for (const [id, h] of Object.entries(lock[f]))
        require(now[f][id] === h, "regression lock: removed/changed " + id);
    const weights = { missing: 0, unknown: 0, partial: 1, implemented: 2 };
    for (const [id, layers] of Object.entries(lock.coverageFloor))
      for (const [layer, state] of Object.entries(layers))
        require(weights[now.coverageFloor[id][layer]] >=
          weights[state], `coverage lowered: ${id}/${layer}`);
  }
}
export function select(c, { capability, family, layer, case: caseId } = {}) {
  for (const [value, choices, label] of [
    [capability, c.capabilities.map((t) => t.capabilityId), "capabilityId"],
    [family, c.families, "family"],
    [layer, c.layers, "layer"],
    [caseId, c.cases.map((t) => t.caseId), "caseId"],
  ])
    require(!value || choices.includes(value), "unknown " +
      label +
      ": " +
      value);
  const caps = c.capabilities.filter(
      (t) =>
        (!capability || t.capabilityId === capability) &&
        (!family || t.family === family),
    ),
    ids = new Set(caps.map((t) => t.capabilityId)),
    cases = c.cases.filter(
      (t) =>
        t.capabilityIds.some((id) => ids.has(id)) &&
        (!layer || t.layer === layer) &&
        (!caseId || t.caseId === caseId),
    );
  require(caps.length &&
    (!(caseId || layer) || cases.length), "execution selection is empty");
  return [caps, cases];
}
export function progress(cap, c, results) {
  const required = cap.requiredLayers ?? c.layers,
    coverage = implementation(cap, c),
    states = required.map((l) => coverage[l].state),
    implemented = states.every((s) => s === "implemented"),
    target = implemented
      ? "IMPLEMENTED"
      : states.every((s) => ["missing", "unknown"].includes(s))
        ? "TARGET_NOT_IMPLEMENTED"
        : "PARTIAL";
  const applicable = c.cases.filter(
      (t) =>
        t.capabilityIds.includes(cap.capabilityId) &&
        t.purpose !== "unsupported-rejection" &&
        t.evidenceScope === "specific-combination",
    ),
    missing = required.filter((l) => !applicable.some((t) => t.layer === l)),
    ids = [
      ...new Set([
        ...(cap.requiredCaseIds ?? []),
        ...applicable
          .filter((t) => required.includes(t.layer))
          .map((t) => t.caseId),
      ]),
    ].sort(),
    verdicts = Object.fromEntries(
      ids.map((id) => [id, results[id]?.status ?? "NOT_RUN"]),
    ),
    outstanding = Object.fromEntries(
      Object.entries(verdicts).filter(([, s]) => s !== "PASS"),
    ),
    verified = !missing.length && ids.length > 0 && !keys(outstanding).length;
  return {
    targetStatus: target,
    requiredLayers: required,
    missingSpecificTestLayers: missing,
    requiredEvidence: verdicts,
    outstandingEvidence: outstanding,
    verificationStatus: verified ? "VERIFIED" : "INCOMPLETE",
    acceptanceStatus: implemented && verified ? "ACCEPTED" : "NOT_ACCEPTED",
  };
}
export function goVerdict(events, selector, code) {
  const matching = events.filter((e) => e.Test === selector),
    parents = events.filter((e) => selector.startsWith(e.Test + "/")),
    desc = events.filter((e) => e.Test?.startsWith(selector + "/"));
  if (parents.some((e) => e.Action === "fail"))
    return ["FAIL", "mapped Go fixture ancestor failed"];
  if (parents.some((e) => e.Action === "skip"))
    return ["ENVIRONMENT_BLOCKED", "mapped Go fixture ancestor skipped"];
  if ([...matching, ...desc].some((e) => e.Action === "fail"))
    return ["FAIL", "Go assertion/build/process failed"];
  if ([...matching, ...desc].some((e) => e.Action === "skip"))
    return ["ENVIRONMENT_BLOCKED", "Go test or descendant skipped"];
  if (
    !matching.some((e) => e.Action === "run") ||
    !matching.some((e) => e.Action === "pass")
  )
    return ["FAIL", "mapped Go test did not execute"];
  if (
    code &&
    !(
      code === 1 &&
      events.some(
        (e) =>
          e.Test &&
          e.Action === "fail" &&
          ![...matching, ...desc, ...parents].includes(e),
      )
    )
  )
    return ["FAIL", "Go process failed without independently identified test"];
  return ["PASS", "actual Go assertions passed"];
}
export function integrationEnvironment(env = process.env) {
  let databaseName = null;
  try {
    const u = new URL(env.OCCCCAD_DATABASE_URL);
    databaseName =
      u.protocol === "sqlite:"
        ? u.pathname
        : decodeURIComponent(u.pathname).replace(/^\//, "");
  } catch {}
  const worker = env.OCCCCAD_GEOMETRY_WORKER_BIN;
  return {
    workerExecutable: worker,
    workerExecutableDigest:
      worker && fs.existsSync(worker) ? fileDigest(worker) : null,
    databaseName,
    integrationVariablesPresent: Object.fromEntries(
      ["OCCCCAD_DATABASE_URL", "OCCCCAD_GEOMETRY_WORKER_BIN"].map((k) => [
        k,
        !!env[k],
      ]),
    ),
  };
}
let interrupted = false;
let signalsInstalled = false;
function guardedArgv(argv, env) {
  const integration =
    (argv[0] === "go" && argv.includes("-tags=integration")) ||
    argv[0].includes("/build/cmake/linux-gcc15-x86_64/");
  if (!integration) return argv;
  const profile = env.OCCCCAD_DEV_PROFILE ?? "release";
  return [
    "flock",
    "--shared",
    path.join(ROOT, "build/locks/resources.lock"),
    "flock",
    "--shared",
    path.join(ROOT, "build/locks/cpp-" + profile + ".lock"),
    ...argv,
  ];
}
const activeChildren = new Map();
function installSignals() {
  if (signalsInstalled) return;
  signalsInstalled = true;
  for (const signal of ["SIGINT", "SIGTERM"])
    process.on(signal, () => {
      interrupted = true;
      for (const stop of activeChildren.values()) stop();
    });
}
export async function command(
  argv,
  cwd,
  env,
  log,
  { timeoutMs = 300000 } = {},
) {
  installSignals();
  if (interrupted) return [130, "execution cancelled"];
  argv = guardedArgv(argv, env);
  fs.mkdirSync(path.join(ROOT, "build/locks"), { recursive: true });
  const [code, raw] = await new Promise((resolve) => {
    const child = spawn(argv[0], argv.slice(1), {
      cwd,
      env,
      detached: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const chunks = [];
    let bytes = 0,
      error = "",
      finished = false,
      killTimer;
    const stop = () => {
      if (finished) return;
      try {
        process.kill(-child.pid, "SIGTERM");
      } catch {}
      killTimer ??= setTimeout(() => {
        if (!finished)
          try {
            process.kill(-child.pid, "SIGKILL");
          } catch {}
      }, 2000);
    };
    activeChildren.set(child.pid, stop);
    const timer = setTimeout(() => {
      error = `execution exceeded ${timeoutMs} ms`;
      stop();
    }, timeoutMs);
    const collect = (data) => {
      bytes += data.length;
      if (bytes > 128 * 1024 * 1024) {
        error = "execution output exceeded 128 MiB";
        stop();
        return;
      }
      chunks.push(data);
    };
    child.stdout.on("data", collect);
    child.stderr.on("data", collect);
    child.on("error", (e) => {
      error = e.message;
    });
    child.on("close", (code) => {
      finished = true;
      clearTimeout(timer);
      clearTimeout(killTimer);
      activeChildren.delete(child.pid);
      resolve([
        interrupted ? 130 : error ? -1 : (code ?? -1),
        Buffer.concat(chunks).toString("utf8") + (error ? "\n" + error : ""),
      ]);
    });
  });
  let output = raw.replace(
    /([a-z][a-z0-9+.-]*:\/\/)[^\s/@]+(?::[^\s/@]*)?@/gi,
    "$1<redacted>@",
  );
  for (const [k, v] of Object.entries(env))
    if (/PASSWORD|TOKEN|SECRET|ACCESS_KEY/.test(k) && v)
      output = output.replaceAll(v, "<redacted>");
  fs.mkdirSync(path.dirname(log), { recursive: true });
  fs.writeFileSync(log, output, { mode: 0o600 });
  return [code, output];
}
export async function runCases(
  cases,
  output,
  profile,
  adapters,
  { execute = command, env = process.env, jobs = 4, testJobs = 1 } = {},
) {
  env = {
    ...env,
    OCCCCAD_DEV_PROFILE: profile,
    OCCCCAD_ASSEMBLY_CONTRACT_CASES: JSON.stringify(cases.map((t) => t.caseId)),
  };
  const results = {},
    commands = [],
    pending = [];
  const record = (t, status, observed, argv = null, log = null) =>
    (results[t.caseId] = {
      caseId: t.caseId,
      status,
      observed,
      command: argv,
      log,
    });
  for (const t of cases) {
    if (!adapters.includes(t.adapter))
      record(t, "NOT_RUN", "adapter excluded by execution scope");
    else if ((t.requires ?? []).some((k) => !env[k]))
      record(t, "ENVIRONMENT_BLOCKED", "required environment missing");
    else pending.push(t);
  }
  const cpp = pending.filter((t) => t.adapter === "cpp");
  if (cpp.length) {
    let argv = [
        "mise",
        "run",
        "--quiet",
        "cpp:build",
        "--target",
        "occcad_assembly_solver_scenarios",
        "--profile",
        profile,
        "--jobs",
        String(jobs),
      ],
      log = path.join(output, "cpp-build.log");
    commands.push(argv);
    let [code] = await execute(argv, ROOT, env, log);
    if (code)
      cpp.forEach((t) => record(t, "FAIL", "C++ build failed", argv, log));
    else {
      const xml = path.join(output, "cpp.xml");
      if (fs.existsSync(xml)) fs.unlinkSync(xml);
      argv = [
        path.join(
          ROOT,
          "build/cmake/linux-gcc15-x86_64",
          profile,
          "kernel/assembly/tests/occcad_assembly_solver_scenarios",
        ),
        "--gtest_filter=" +
          [...new Set(cpp.map((t) => t.selector))].sort().join(":"),
        "--gtest_output=xml:" + xml,
      ];
      log = path.join(output, "cpp.log");
      commands.push(argv);
      [code] = await execute(argv, ROOT, env, log);
      const actual = {};
      if (fs.existsSync(xml))
        for (const m of fs
          .readFileSync(xml, "utf8")
          .matchAll(/<testcase\s+([^>]+?)(?:\/>|>([\s\S]*?)<\/testcase>)/g)) {
          const attrs = Object.fromEntries(
            [...m[1].matchAll(/([\w-]+)="([^"]*)"/g)].map((x) => [x[1], x[2]]),
          );
          actual[attrs.classname + "." + attrs.name] = {
            ...attrs,
            body: m[2] ?? "",
          };
        }
      for (const t of cpp) {
        const test = actual[t.selector];
        record(
          t,
          !test
            ? "FAIL"
            : /<failure|<error/.test(test.body)
              ? "FAIL"
              : test.status !== "run" || /<skipped/.test(test.body)
                ? "ENVIRONMENT_BLOCKED"
                : code
                  ? "FAIL"
                  : "PASS",
          !test
            ? "mapped C++ test did not execute"
            : "actual C++ geometry/rank assertions",
          argv,
          log,
        );
      }
    }
  }
  const groups = new Map();
  for (const t of pending.filter((t) =>
    ["go", "go-flags", "integration"].includes(t.adapter),
  )) {
    const key =
      t.package +
      "|" +
      (t.adapter === "integration" ? t.selector.split("/")[0] : "");
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(t);
  }
  let index = 0;
  for (const batch of groups.values()) {
    const names = [
        ...new Set(batch.map((t) => t.selector.split("/")[0])),
      ].sort(),
      integration = batch.some((t) => t.adapter === "integration"),
      argv = [
        "go",
        "test",
        "-json",
        "-p",
        String(jobs),
        "-parallel",
        String(testJobs),
        "-count=1",
        "-timeout=240s",
        ...(integration ? ["-tags=integration"] : []),
        batch[0].package,
        "-run",
        "^(" + names.map(esc).join("|") + ")$",
      ],
      log = path.join(output, "go-" + index++ + ".log");
    commands.push(argv);
    const [code, raw] = await execute(
        argv,
        path.join(ROOT, "services"),
        env,
        log,
      ),
      events = raw.split("\n").flatMap((l) => {
        try {
          return [JSON.parse(l)];
        } catch {
          return [];
        }
      });
    for (const t of batch) {
      const [status, reason] = goVerdict(events, t.selector, code);
      record(t, status, reason, argv, log);
      results[t.caseId].evidence = events
        .filter((e) => e.Test?.startsWith(t.selector))
        .map((e) => e.Output ?? "")
        .join("")
        .slice(-6000);
    }
  }
  const sources = [
    ...new Set(
      pending
        .filter((t) => ["web", "web-catalog"].includes(t.adapter))
        .map((t) => t.fixture.source),
    ),
  ];
  for (const source of sources) {
    const batch = pending.filter((t) => t.fixture?.source === source),
      argv = [
        "pnpm",
        "test",
        "--",
        "--verbose",
        path.relative(path.join(ROOT, "web/apps/cad"), path.join(ROOT, source)),
      ],
      log = path.join(output, "web-" + index++ + ".log"),
      batchEnv = {
        ...env,
        OCCCCAD_ASSEMBLY_CONTRACT_CASES: JSON.stringify(
          batch.map((t) => t.caseId),
        ),
      };
    commands.push(argv);
    const [code, raw] = await execute(
      argv,
      path.join(ROOT, "web/apps/cad"),
      batchEnv,
      log,
    );
    for (const t of batch) {
      const marker = new RegExp(
        "(?<!\\S)CONTRACT_PASS\\s+" + esc(t.caseId) + "(?![a-z0-9.-])",
      ).test(raw);
      record(
        t,
        code
          ? "FAIL"
          : t.adapter === "web-catalog"
            ? marker
              ? "PASS"
              : "FAIL"
            : /PASS|passed/i.test(raw)
              ? "PASS"
              : "FAIL",
        "actual per-case TypeScript assertions required",
        argv,
        log,
      );
    }
  }
  return [results, commands];
}
export function milestoneEvidence(c, results) {
  return Object.fromEntries(
    ["m4", "m5"].map((m) => {
      const ids = c.cases
          .filter((t) => t.caseId.startsWith(m + "."))
          .map((t) => t.caseId)
          .sort(),
        verdicts = Object.fromEntries(
          ids.map((id) => [id, results[id]?.status ?? "NOT_RUN"]),
        );
      return [
        m.toUpperCase(),
        {
          scope: "FIRST_STAGE_AUTOMATED_CASES_ONLY",
          requiredEvidence: verdicts,
          status:
            ids.length && Object.values(verdicts).every((s) => s === "PASS")
              ? "ALL_REQUIRED_PASS"
              : "INCOMPLETE",
          manualAcceptance: "PENDING_MAINTAINER",
        },
      ];
    }),
  );
}
export function exitVerdict(purpose, cases, results, rows) {
  const selected = cases.map((t) => results[t.caseId]);
  if (selected.some((t) => t.status === "FAIL")) return 1;
  if (
    purpose === "composition" &&
    rows.some((r) => r.acceptanceStatus !== "ACCEPTED")
  )
    return 1;
  if (!selected.some((t) => t.status === "PASS")) return 2;
  if (selected.some((t) => t.status === "ENVIRONMENT_BLOCKED")) return 2;
  if (
    purpose === "baseline" &&
    cases.some((t) => t.baseline && results[t.caseId].status !== "PASS")
  )
    return 2;
  return 0;
}
export async function main(args = process.argv.slice(2)) {
  const [purpose, ...rest] = args;
  require(["baseline", "gaps", "composition", "validate"].includes(
    purpose,
  ), "unknown contract purpose");
  const options = Object.fromEntries(
    [
      "capability",
      "family",
      "layer",
      "case",
      "adapters",
      "profile",
      "output",
      "jobs",
      "test-jobs",
    ].map((k) => [k, { type: "string" }]),
  );
  const { values: v } = parseArgs({ args: rest, options });
  const profile = v.profile ?? "release",
    adapters = (v.adapters ?? ADAPTERS.join(",")).split(",");
  require(subset(adapters, ADAPTERS) &&
    adapters.length, "unknown/empty adapter selection");
  const jobs = Number(v.jobs ?? 4),
    testJobs = Number(v["test-jobs"] ?? 1);
  require(Number.isInteger(jobs) &&
    jobs > 0 &&
    testJobs === 1, "positive jobs and test-jobs=1 required");
  const c = loadCatalog();
  validate(c, { lock: read(path.join(DATA, "baseline.json")) });
  const [caps, cases] = select(c, v);
  if (purpose === "validate") {
    console.log(
      `Catalog/lock PASS: ${c.capabilities.length} capabilities, ${c.cases.length} cases; no implementation tests executed`,
    );
    return 0;
  }
  const output = path.resolve(
      v.output ??
        path.join(ROOT, "build/assembly-contract", Date.now().toString()),
    ),
    scheduled =
      purpose === "baseline" ? cases.filter((t) => t.baseline) : cases;
  const before = fileInputs(c, cases);
  const [results, commands] = await runCases(
    scheduled,
    output,
    profile,
    adapters,
    {
      jobs,
      testJobs,
    },
  );
  for (const t of cases)
    results[t.caseId] ??= {
      caseId: t.caseId,
      status: "NOT_RUN",
      observed: "target-only assertion excluded from baseline; run gaps",
    };
  const changed = digest(before) !== digest(fileInputs(c, cases));
  if (changed)
    for (const r of Object.values(results))
      if (r.status === "PASS") r.status = "SOURCE_CHANGED";
  const rows = caps.map((cap) => ({
    capabilityId: cap.capabilityId,
    family: cap.family,
    target: { ...cap, policy: c.policies[cap.policy] },
    implementation: implementation(cap, c),
    ...progress(cap, c, results),
    verification: cases
      .filter((t) => t.capabilityIds.includes(cap.capabilityId))
      .map((t) => ({ ...t, result: results[t.caseId] })),
    followupLinks: Object.fromEntries(
      cap.followupTasks.map((t) => [t, c.target + "#" + TASK_TOPICS[t]]),
    ),
  }));
  const count = (values) =>
    values.reduce((a, v) => ((a[v] = (a[v] ?? 0) + 1), a), {});
  const report = {
    schemaVersion: c.schemaVersion,
    contractVersion: c.contractVersion,
    catalogDigest: digest(c),
    purpose,
    acceptanceScope: "AUTOMATED_CONTRACT_ONLY",
    milestoneEvidence: milestoneEvidence(c, results),
    manualAcceptance: { status: "PENDING_MAINTAINER" },
    baselineMeaning:
      "selected executed existing abilities only; NOT six-family product acceptance",
    commit: execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: ROOT,
      encoding: "utf8",
    }).trim(),
    inputDigests: before,
    invocation: args,
    environment: {
      platform: os.platform(),
      node: process.version,
      ...integrationEnvironment(),
    },
    commands: commands.map((argv) => ({
      argv: guardedArgv(argv, { ...process.env, OCCCCAD_DEV_PROFILE: profile }),
      cwd:
        argv[0] === "go"
          ? path.join(ROOT, "services")
          : argv[0] === "pnpm"
            ? path.join(ROOT, "web/apps/cad")
            : ROOT,
    })),
    selectedCaseIds: cases.map((t) => t.caseId),
    results: Object.values(results),
    counts: count(Object.values(results).map((r) => r.status)),
    targetCounts: count(rows.map((r) => r.targetStatus)),
    capabilities: rows,
  };
  fs.mkdirSync(output, { recursive: true });
  fs.writeFileSync(
    path.join(output, "report.json"),
    JSON.stringify(report, null, 2) + "\n",
  );
  fs.writeFileSync(
    path.join(output, "summary.md"),
    "# Assembly constraint contract report\n\nAutomated selected evidence; manual acceptance remains pending.\n\n" +
      JSON.stringify(report.counts) +
      "\n\n" +
      rows
        .map(
          (r) =>
            `${r.capabilityId}: ${r.targetStatus}; ${r.verificationStatus}; ${r.acceptanceStatus}`,
        )
        .join("\n") +
      "\n",
  );
  console.log(
    "Executed contract cases: " +
      JSON.stringify(report.counts) +
      "\nReports: " +
      output,
  );
  return changed ? 2 : exitVerdict(purpose, cases, results, rows);
}
function fileInputs(c, cases) {
  const files = new Set([
    "tests/assembly-contract/catalog.mjs",
    "tests/assembly-contract/runner.mjs",
    "tests/test.data/assembly-contract/catalog.json",
    "tests/test.data/assembly-contract/evidence.json",
    "tests/test.data/assembly-contract/baseline.json",
    "services/internal/assemblycontract/catalog.json",
    "proto/occccad/worker/v1/geometry_worker.proto",
  ]);
  for (const t of cases) {
    files.add(t.fixture.source);
    for (const ref of t.fixture.assertionSources ?? []) files.add(ref.source);
  }
  for (const cap of c.capabilities)
    for (const state of Object.values(implementation(cap, c)))
      if (state.source) files.add(state.source);
  const worker = process.env.OCCCCAD_GEOMETRY_WORKER_BIN;
  if (worker) files.add(worker);
  return Object.fromEntries(
    [...files]
      .sort()
      .map((p) => [
        p,
        fs.existsSync(path.isAbsolute(p) ? p : path.join(ROOT, p))
          ? fileDigest(path.isAbsolute(p) ? p : path.join(ROOT, p))
          : "MISSING",
      ]),
  );
}
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = await main();
  } catch (e) {
    console.error("Contract error: " + e.message);
    process.exitCode = 2;
  }
}
