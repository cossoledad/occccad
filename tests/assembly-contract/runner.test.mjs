import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { loadCatalog, composeCatalog, read, DATA, ROOT } from "./catalog.mjs";
import * as r from "./runner.mjs";
const catalog = loadCatalog(),
  lock = read(path.join(DATA, "baseline.json")),
  clone = structuredClone;
const bad = (mutate, pattern, useLock = false) => {
  const c = clone(catalog);
  mutate(c);
  assert.throws(() => r.validate(c, { lock: useLock ? lock : null }), pattern);
};
test("split catalog binding and semantic ownership", async () => {
  const p = read(
      path.join(ROOT, "services/internal/assemblycontract/catalog.json"),
    ),
    e = read(path.join(DATA, "evidence.json"));
  assert.equal(composeCatalog(p, e).cases.length, e.cases.length);
  assert.equal(p.cases, undefined);
  assert.equal(p.capabilities[0].caseIds, undefined);
  assert.ok(Object.keys(e.bindings).length < e.cases.length);
  let b = clone(e);
  b.cases[0].bindingId = "absent";
  assert.throws(() => composeCatalog(p, b), /missing binding/);
  b = clone(e);
  b.bindings.orphan = {};
  assert.throws(() => composeCatalog(p, b), /unused test binding/);
  b = clone(e);
  b.capabilityEvidence[p.capabilities[0].capabilityId].roles = [];
  assert.throws(() => composeCatalog(p, b), /overwrites production/);
});
test("catalog and all existing regression hashes remain enforced", async () => {
  r.validate(catalog, { lock });
  assert.ok(catalog.capabilities.length >= 58);
  assert.ok(catalog.cases.length >= 677);
  assert.ok(Object.keys(lock.cases).length >= 677);
});
test("duplicates and broken references", async () => {
  bad((c) => c.capabilities.push(c.capabilities[0]), /duplicate capabilityId/);
  bad((c) => c.cases.push(c.cases[0]), /duplicate caseId/);
  bad((c) => c.capabilities[0].caseIds.push("typo"), /invalid test mapping/);
  bad(
    (c) => c.cases[0].capabilityIds.push("typo"),
    /invalid capability reference/,
  );
  bad(
    (c) => c.derivedSupports.push(c.derivedSupports[0]),
    /duplicate derived supportId/,
  );
  bad(
    (c) => (c.capabilities[0].roles[0].supportedDescriptors = ["MESH_FACE"]),
    /invalid supported descriptor/,
  );
});
test("semantics fixture and selector required", async () => {
  bad((c) => delete c.policies.spatial.parameters, /parameters/);
  bad(
    (c) => delete c.policies.spatial.parameters[0].unit,
    /parameter semantics/,
  );
  bad((c) => (c.descriptors.POINT.units = {}), /unit fields/);
  bad((c) => (c.cases[0].fixture.source = "missing"), /missing fixture/);
  bad(
    (c) =>
      (c.cases[0].fixture.assertionSources = [
        { source: "missing", symbol: "missing" },
      ]),
    /missing additional assertion fixture/,
  );
  bad((c) => {
    const t = c.cases.find((t) => t.adapter === "cpp");
    t.selector = t.fixture.symbol = "AssemblySolver.Typo";
  }, /missing C\+\+ test/);
});
test("implemented layer requires executable test", async () =>
  bad((c) => (c.capabilities[0].caseIds = []), /has no executable test/));
test("regression lock prevents removal and weakening", async () => {
  bad(
    (c) => (c.policies.spatial.direction = "Converged is enough"),
    /regression lock changed/,
    true,
  );
  bad(
    (c) =>
      (c.cases.find((t) => t.adapter === "cpp").expectation =
        "Converged is enough"),
    /regression lock/,
    true,
  );
  bad(
    (c) => {
      (c.capabilities[0].implementationOverrides ??= {}).workerSolver = {
        state: "partial",
        reason: "weakened",
      };
    },
    /coverage lowered/,
    true,
  );
  bad(
    (c) => {
      const id = "coincidence.frame-frame";
      c.capabilities = c.capabilities.filter((t) => t.capabilityId !== id);
      for (const t of c.cases)
        t.capabilityIds = t.capabilityIds.filter((x) => x !== id);
      c.cases = c.cases.filter((t) => t.capabilityIds.length);
      const remaining = c.cases.map((t) => t.caseId);
      for (const t of c.capabilities) {
        t.caseIds = t.caseIds.filter((id) => remaining.includes(id));
        if (t.requiredCaseIds)
          t.requiredCaseIds = t.requiredCaseIds.filter((id) =>
            remaining.includes(id),
          );
      }
    },
    /removed\/changed/,
    true,
  );
});
test("unknown and empty selections fail", async () => {
  for (const options of [
    { case: "typo" },
    { family: "Concentric" },
    { capability: "typo" },
    { layer: "browser" },
  ])
    assert.throws(() => r.select(catalog, options));
  const c = clone(catalog);
  c.cases = [];
  assert.throws(
    () => r.select(c, { capability: "coincidence.frame-frame", layer: "ui" }),
    /empty/,
  );
});
const ev = (Test, Action) => ({ Test, Action });
test("Go requires actual run and no skip", async () => {
  const s = "TestReal",
    e = [ev(s, "run"), ev(s, "pass")];
  assert.equal(r.goVerdict([], s, 0)[0], "FAIL");
  assert.equal(r.goVerdict([ev(s, "pass")], s, 0)[0], "FAIL");
  assert.equal(r.goVerdict(e, s, 0)[0], "PASS");
  assert.equal(r.goVerdict(e, s, 1)[0], "FAIL");
  e.push(ev(s + "/child", "skip"));
  assert.equal(r.goVerdict(e, s, 0)[0], "ENVIRONMENT_BLOCKED");
  e.push(ev(s + "/child", "fail"));
  assert.equal(r.goVerdict(e, s, 0)[0], "FAIL");
});
test("child pass cannot hide failed fixture ancestor", async () => {
  const e = [
    ev("TestReal/good", "run"),
    ev("TestReal/good", "pass"),
    ev("TestReal/bad", "run"),
    ev("TestReal/bad", "fail"),
    ev("TestReal", "fail"),
  ];
  for (const s of ["TestReal/good", "TestReal/bad", "TestReal"])
    assert.equal(r.goVerdict(e, s, 1)[0], "FAIL");
});
test("ancestor skips and nested ancestor failures are not pass", async () => {
  const s = "TestReal/group/child",
    e = [ev(s, "run"), ev(s, "pass")];
  for (const p of ["TestReal", "TestReal/group"]) {
    assert.equal(
      r.goVerdict([...e, ev(p, "skip")], s, 0)[0],
      "ENVIRONMENT_BLOCKED",
    );
    assert.equal(r.goVerdict([...e, ev(p, "fail")], s, 1)[0], "FAIL");
  }
});
test("unrelated fixture failure does not relabel pass", async () => {
  const s = "TestReal/good",
    e = [
      ev(s, "run"),
      ev(s, "pass"),
      ev("TestReal", "pass"),
      ev("TestOther/bad", "fail"),
      ev("TestOther", "fail"),
    ];
  assert.equal(r.goVerdict(e, s, 1)[0], "PASS");
});
test("progress requires specific applicable actual evidence", async () => {
  const cap = clone(catalog.capabilities[0]),
    c = clone(catalog);
  for (const layer of r.LAYERS)
    (cap.implementationOverrides ??= {})[layer] = {
      state: "implemented",
      reason: "declaration",
    };
  cap.requiredLayers = ["workerSolver", "historyReplay"];
  c.cases = [
    {
      caseId: "math",
      capabilityIds: [cap.capabilityId],
      layer: "workerSolver",
      evidenceScope: "specific-combination",
    },
    {
      caseId: "db",
      capabilityIds: [cap.capabilityId],
      layer: "historyReplay",
      evidenceScope: "specific-combination",
    },
  ];
  const results = {
    math: { status: "PASS" },
    db: { status: "ENVIRONMENT_BLOCKED" },
  };
  let p = r.progress(cap, c, results);
  assert.equal(p.targetStatus, "IMPLEMENTED");
  assert.equal(p.acceptanceStatus, "NOT_ACCEPTED");
  assert.deepEqual(p.outstandingEvidence, { db: "ENVIRONMENT_BLOCKED" });
  results.db.status = "PASS";
  assert.equal(r.progress(cap, c, results).acceptanceStatus, "ACCEPTED");
  for (const change of [
    { evidenceScope: "representative-shared-foundation" },
    { purpose: "unsupported-rejection" },
  ]) {
    const altered = clone(c);
    Object.assign(altered.cases[1], change);
    assert.equal(
      r.progress(cap, altered, results).acceptanceStatus,
      "NOT_ACCEPTED",
    );
  }
  delete results.db;
  assert.deepEqual(r.progress(cap, c, results).outstandingEvidence, {
    db: "NOT_RUN",
  });
  cap.implementationOverrides.historyReplay.state = "missing";
  assert.equal(r.progress(cap, c, results).targetStatus, "PARTIAL");
});
test("missing environment is not Mock pass", async () => {
  const c = {
    caseId: "integration.example",
    adapter: "integration",
    requires: ["CONTRACT_TEST_MISSING_ENV"],
  };
  const [res, cmd] = await r.runCases([c], ".", "debug", ["integration"], {
    env: {},
  });
  assert.equal(res[c.caseId].status, "ENVIRONMENT_BLOCKED");
  assert.deepEqual(cmd, []);
});
test("environment identity never records credentials", async () => {
  const e = r.integrationEnvironment({
    OCCCCAD_DATABASE_URL:
      "postgresql://actor:secret@localhost/dedicated_contract_test?sslmode=disable",
    OCCCCAD_GEOMETRY_WORKER_BIN: "/missing-worker",
  });
  assert.equal(e.databaseName, "dedicated_contract_test");
  assert.equal(e.workerExecutableDigest, null);
  assert.ok(!JSON.stringify(e).includes("secret"));
});
test("integration failures do not relabel independent fixtures and force rerun", async () => {
  const cases = ["TestPassing", "TestFailing"].map((selector) => ({
    caseId: selector.toLowerCase(),
    adapter: "integration",
    requires: [],
    package: "./internal/control",
    selector,
  }));
  const [results, commands] = await r.runCases(
    cases,
    ".",
    "debug",
    ["integration"],
    {
      execute: (argv) => {
        const name = argv.at(-1).includes("TestFailing")
          ? "TestFailing"
          : "TestPassing";
        assert.ok(argv.includes("-count=1"));
        assert.ok(argv.includes("-tags=integration"));
        return [
          name === "TestFailing" ? 1 : 0,
          [ev(name, "run"), ev(name, name === "TestFailing" ? "fail" : "pass")]
            .map((e) => JSON.stringify(e))
            .join("\n"),
        ];
      },
    },
  );
  assert.equal(commands.length, 2);
  assert.equal(results.testpassing.status, "PASS");
  assert.equal(results.testfailing.status, "FAIL");
});
test("assertion source changes fail the lock", async () => {
  const b = clone(lock);
  b.assertions[catalog.cases[0].caseId] = "changed-assertion";
  assert.throws(() => r.validate(catalog, { lock: b }), /regression lock/);
});
test("web marker must be exact and process must succeed", async () => {
  const c = catalog.cases.find((t) => t.adapter === "web-catalog");
  for (const [code, raw, want] of [
    [0, "CONTRACT_PASS " + c.caseId + "-wrong", "FAIL"],
    [1, "CONTRACT_PASS " + c.caseId, "FAIL"],
    [0, "CONTRACT_PASS " + c.caseId, "PASS"],
  ]) {
    const [results] = await r.runCases([c], ".", "debug", ["web-catalog"], {
      execute: () => [code, raw],
    });
    assert.equal(results[c.caseId].status, want);
  }
});
test("web batches receive only their own cases", async () => {
  const cases = [
    catalog.cases.find((t) => t.caseId === "angle.free.web-entry"),
    catalog.cases.find((t) => t.selector === "production-capability"),
  ];
  const [results, commands] = await r.runCases(
    cases,
    ".",
    "debug",
    ["web-catalog"],
    {
      execute: (argv, cwd, env) => {
        const ids = JSON.parse(env.OCCCCAD_ASSEMBLY_CONTRACT_CASES);
        assert.equal(ids.length, 1);
        return [0, "CONTRACT_PASS " + ids[0]];
      },
    },
  );
  assert.equal(commands.length, 2);
  assert.ok(Object.values(results).every((r) => r.status === "PASS"));
});
test("composition rejects incomplete or blocked required evidence", async () => {
  const t = catalog.cases.find((t) => t.adapter === "web-catalog");
  for (const status of ["PASS", "ENVIRONMENT_BLOCKED"])
    assert.equal(
      r.exitVerdict("composition", [t], { [t.caseId]: { status } }, [
        { acceptanceStatus: "NOT_ACCEPTED" },
      ]),
      1,
    );
});
test("assertion helper must exist and be called", async () => {
  const c = clone(catalog.cases.find((t) => t.adapter === "integration"));
  c.fixture.assertionHelperSymbol = "missingContractAssertionHelper";
  assert.throws(() => r.assertionSource(c), /not called/);
  delete c.fixture.assertionHelperSymbol;
  c.fixture.assertionSources = [
    { source: c.fixture.source, symbol: "missingContractAssertionHelper" },
  ];
  assert.throws(() => r.assertionSource(c), /missing additional assertion/);
});
test("zero execution cannot be green", async () => {
  const t = catalog.cases.find(
    (t) => t.caseId === "offset.plane-plane.first-normal-editor",
  );
  assert.equal(
    r.exitVerdict("baseline", [t], { [t.caseId]: { status: "NOT_RUN" } }, []),
    2,
  );
});
test("milestones require all actual evidence and retain manual boundary", async () => {
  const c = {
    cases: [{ caseId: "m4.one" }, { caseId: "m4.two" }, { caseId: "m5.one" }],
  };
  for (const status of ["FAIL", "ENVIRONMENT_BLOCKED", "NOT_RUN"])
    assert.equal(
      r.milestoneEvidence(c, {
        "m4.one": { status: "PASS" },
        "m4.two": { status },
      }).M4.status,
      "INCOMPLETE",
    );
  const p = r.milestoneEvidence(c, { "m4.one": { status: "PASS" } });
  assert.equal(p.M4.requiredEvidence["m4.two"], "NOT_RUN");
  const done = r.milestoneEvidence(c, {
    "m4.one": { status: "PASS" },
    "m4.two": { status: "PASS" },
  });
  assert.equal(done.M4.status, "ALL_REQUIRED_PASS");
  assert.equal(done.M4.manualAcceptance, "PENDING_MAINTAINER");
  assert.equal(r.milestoneEvidence({ cases: [] }, {}).M4.status, "INCOMPLETE");
});

test("Go adapters receive the exact selected stable case IDs", async () => {
  const c = catalog.cases.find((t) => t.adapter === "go-flags");
  const [results] = await r.runCases([c], ".", "release", ["go-flags"], {
    execute: (argv, cwd, env) => {
      assert.deepEqual(JSON.parse(env.OCCCCAD_ASSEMBLY_CONTRACT_CASES), [
        c.caseId,
      ]);
      return [
        0,
        [ev(c.selector, "run"), ev(c.selector, "pass")]
          .map((e) => JSON.stringify(e))
          .join("\n"),
      ];
    },
  });
  assert.equal(results[c.caseId].status, "PASS");
});
