import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";
import {
  anchors,
  links,
  audit,
  entrySizes,
  codeGuides,
} from "../../tools/documentation-audit.mjs";
import { readDevelopmentEnvironment } from "../../tools/development_environment.mjs";
import {
  nativeSummary,
  serviceSamples,
} from "../../kernel/assembly/tests/run-performance.mjs";
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "occccad-doc-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  execFileSync("git", ["init", "-q"], { cwd: root });
  return root;
}
function put(root, p, s) {
  fs.mkdirSync(path.dirname(path.join(root, p)), { recursive: true });
  fs.writeFileSync(path.join(root, p), s);
}
test("heading duplicates unicode and fences", () => {
  const a = anchors(
    '# 中文 Test\n# 中文 Test\n```md\n# fake\n```\n<a id="manual"/>',
  );
  assert.ok(a.has("中文-test"));
  assert.ok(a.has("中文-test-1"));
  assert.ok(a.has("manual"));
  assert.ok(!a.has("fake"));
});
test("all owned readme paths and anchors", (t) => {
  const root = fixture(t);
  put(root, "README.md", "[A](a.md#hello)\n[broken](missing.md)");
  put(root, "a.md", "# Hello\n");
  assert.ok(audit(root).errors.some((e) => e.includes("broken local link")));
  put(root, "README.md", "[A](a.md#absent)");
  assert.ok(audit(root).errors.some((e) => e.includes("broken anchor")));
});
test("reference links and fenced examples", () => {
  const l = links(
    "/tmp/root/a.md",
    "[text][id]\n[id]: <space file.md#hello>\n```\n[fake](missing.md)\n```",
  );
  assert.equal(l.length, 1);
  assert.equal(l[0].target, "/tmp/root/space file.md");
  assert.equal(l[0].anchor, "hello");
});
test("unclosed fenced block", (t) => {
  const root = fixture(t);
  put(root, "README.md", "```\nunterminated");
  assert.ok(
    audit(root).errors.some((e) => e.includes("unclosed fenced block")),
  );
});
test("orphan cycles and fenced fake links do not create discovery", (t) => {
  const root = fixture(t);
  put(root, "README.md", "```\n[x](a.md)\n```");
  put(root, "a.md", "[b](b.md)");
  put(root, "b.md", "[a](a.md)");
  assert.equal(
    audit(root).errors.filter((e) => e.includes("unreachable Markdown")).length,
    2,
  );
});
test("space named fixtures encoded anchors and directory readme", () => {
  const l = links(
    "/tmp/root/a.md",
    "[fixture](space%20file.md#%E4%B8%AD%E6%96%87)",
  );
  assert.equal(l[0].anchor, "中文");
  assert.equal(l[0].target, "/tmp/root/space file.md");
});
test("knowledge indexes and layout required", (t) => {
  const root = fixture(t);
  put(root, "README.md", "# root");
  put(root, "docs/unclassified/name.md", "# Name");
  const e = audit(root).errors;
  assert.ok(e.some((e) => e.includes("missing knowledge entry")));
  assert.ok(e.some((e) => e.includes("unclassified knowledge page")));
});
test("local task budget excludes optional project map", () => {
  const d = {
    "AGENTS.md": 100,
    "docs/README.md": 200,
    ...Object.fromEntries(codeGuides.map((g) => [g, 50])),
  };
  const s = entrySizes(d);
  assert.equal(s.localTask[codeGuides[0]], 150);
  assert.equal(s.withProjectMap[codeGuides[0]], 350);
});
test("literal Node env precedence missing explicit file and no expansion", (t) => {
  const root = fixture(t);
  put(
    root,
    ".env",
    "export OCCCCAD_A=\"$(false)\"\nOCCCCAD_A=duplicate\nOCCCCAD_B=first\nOCCCCAD_C=' spaced '",
  );
  let v = readDevelopmentEnvironment(root, { OCCCCAD_B: "override" });
  assert.equal(v.OCCCCAD_A, "$(false)");
  assert.equal(v.OCCCCAD_B, "override");
  assert.equal(v.OCCCCAD_C, " spaced ");
  assert.throws(() =>
    readDevelopmentEnvironment(root, {
      OCCCCAD_ENV_FILE: path.join(root, "missing"),
    }),
  );
  put(root, "selected.env", "OCCCCAD_B=selected");
  v = readDevelopmentEnvironment(root, {
    OCCCCAD_ENV_FILE: path.join(root, "selected.env"),
  });
  assert.equal(v.OCCCCAD_B, "selected");
});
test("service requires actual execution and all samples", () => {
  const s = [
    { Action: "run", Test: "TestMeasured" },
    { Action: "skip", Test: "TestMeasured" },
  ]
    .map(JSON.stringify)
    .join("\n");
  assert.throws(() => serviceSamples(s, 2), /Service execution evidence/);
  assert.throws(
    () => serviceSamples(s.replace("skip", "pass"), 2),
    /Missing\/incomplete/,
  );
});
test("native mean and interaction median have distinct units", () => {
  const input = [3, 1, 2]
      .map((value) =>
        JSON.stringify({
          kind: "sample",
          scene: "single",
          kernel_ms: value,
          valid: true,
          eligible: true,
          compile_ms: 0.1,
        }),
      )
      .join("\n"),
    rows = nativeSummary(
      "AssemblyPlaneChain5 3 1000000 ns/op valid=true rank=12\n",
      input,
    );
  assert.equal(rows[0].statistic, "mean");
  assert.equal(rows[0].total_ms, 1);
  assert.equal(rows[1].statistic, "median");
  assert.equal(rows[1].total_ms, 2);
  assert.deepEqual(rows[1].samples_ms, [3, 1, 2]);
  assert.equal(rows[1].phases_ms.compile_ms, 0.1);
});
test("expected failure is valid and ineligible; invalid cannot measure", () => {
  const s = {
    kind: "sample",
    scene: "budget",
    kernel_ms: 1,
    valid: true,
    eligible: false,
  };
  assert.equal(nativeSummary("", JSON.stringify(s))[0].eligible, false);
  assert.throws(
    () => nativeSummary("", JSON.stringify({ ...s, valid: false })),
    /Invalid interaction/,
  );
});

test("contract commands time out, redact logs and clean their owned process group", async (t) => {
  const { command } = await import("../assembly-contract/runner.mjs");
  const root = fixture(t),
    log = path.join(root, "execution.log");
  const secret = "owned-tooling-secret";
  const [code] = await command(
    [
      process.execPath,
      "-e",
      `console.log('postgres://actor:password@example.test/db ${secret}');setInterval(()=>{},1000)`,
    ],
    root,
    { ...process.env, OCCCCAD_ADMIN_PASSWORD: secret },
    log,
    { timeoutMs: 100 },
  );
  assert.notEqual(code, 0);
  const text = fs.readFileSync(log, "utf8");
  assert.ok(text.includes("execution exceeded 100 ms"));
  assert.ok(!text.includes(secret));
  assert.ok(!text.includes("actor:password"));
});

test("integration build tags and resource inventory stay explicit", () => {
  const root = path.resolve(import.meta.dirname, "../.."),
    rows = JSON.parse(
      fs.readFileSync(path.join(root, "dev/test-resources.json"), "utf8"),
    ),
    paths = new Set(rows.map((r) => r.path));
  const files = execFileSync(
    "git",
    ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
    { cwd: root, encoding: "utf8" },
  )
    .split("\0")
    .filter((p) => p.endsWith("_test.go") && fs.existsSync(path.join(root, p)));
  for (const p of files) {
    const source = fs.readFileSync(path.join(root, p), "utf8");
    if (source.startsWith("//go:build integration"))
      assert.ok(paths.has(p), "missing resource inventory: " + p);
    if (p.startsWith("services/") && !p.startsWith("services/internal/config/") && !p.startsWith("services/internal/devtool/")) {
      if (/LoadEnvironment\(|testsupport\.(Environment|OpenDevelopmentDatabase)|os\.Getenv\("OCCCCAD_(DATABASE_URL|GEOMETRY_WORKER_BIN|ARTIFACT_BACKEND)/.test(source))
        assert.ok(source.startsWith("//go:build integration"), "configured resources leaked into unit compilation: " + p);
    }
  }
});
