import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

// Exercise mise's real argument boundary in a checkout fixture. No CAD build,
// database or external storage is available even if the wrapper regresses.
test("mise preserves missing/empty values for strict CLI validation and pure JSON", (t) => {
  const repo = path.resolve(import.meta.dirname, "../.."),
    root = fs.mkdtempSync(path.join(os.tmpdir(), "occccad-cli-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const d of ["dev/tasks/cpp", "build/tools", "services", "config"])
    fs.mkdirSync(path.join(root, d), { recursive: true });
  fs.writeFileSync(path.join(root, "services/go.mod"), "module fixture\n");
  fs.writeFileSync(
    path.join(root, "mise.toml"),
    '[settings.task]\nrun_auto_install=false\n[task_config]\nincludes=["dev/tasks"]\n',
  );
  for (const f of [
    "dev/entry",
    "dev/tasks/cpp/build",
    "dev/tasks/plan",
    "build/tools/occccad-dev",
  ]) {
    fs.copyFileSync(path.join(repo, f), path.join(root, f));
    fs.chmodSync(path.join(root, f), 0o755);
  }
  const env = {
    ...process.env,
    MISE_CONFIG_DIR: path.join(root, "config"),
    MISE_TRUSTED_CONFIG_PATHS: root,
  };
  for (const args of [
    ["cpp:build", "--target"],
    ["cpp:build", "--target", ""],
    ["plan", "--jobs"],
    ["plan", "--jobs", "0"],
    ["plan", "--match", ""],
    ["plan", "--jobs", "--json"],
  ]) {
    const result = spawnSync("mise", ["run", "--quiet", ...args], {
      cwd: root,
      env,
      encoding: "utf8",
      timeout: 15000,
    });
    assert.ifError(result.error);
    assert.notEqual(result.status, 0, args.join(" "));
    assert.match(
      result.stderr,
      /needs an argument|requires a non-empty value|positive decimal integer/,
    );
    assert.ok(
      !result.stderr.includes("OCCCCAD_NATIVE_COMMAND"),
      "validation reached native build",
    );
    if (args.includes("--json"))
      assert.equal(JSON.parse(result.stdout).status, "FAIL");
  }
  const help = spawnSync("mise", ["run", "--quiet", "plan", "--help"], {
    cwd: root,
    env,
    encoding: "utf8",
    timeout: 15000,
  });
  assert.equal(help.status, 0, help.stderr);
  assert.match(help.stderr, /-scope/);
  assert.ok(!fs.existsSync(path.join(root, "build/cmake")));
});
