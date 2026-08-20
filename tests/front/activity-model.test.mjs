import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const require = createRequire(new URL("../../web/apps/cad/package.json", import.meta.url));
const ts = require("typescript");
const sourceURL = new URL("../../web/apps/cad/src/features/activity/activity-model.ts", import.meta.url);
const source = await readFile(sourceURL, "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const moduleURL = `data:text/javascript;base64,${Buffer.from(`${output}\n//# sourceURL=${pathToFileURL(sourceURL.pathname)}`).toString("base64")}`;
const activity = await import(moduleURL);

const baseJob = {
  id: "job-1", type: "EXCHANGE_EXPORT", state: "RUNNING", progress: 45,
  payload: { fileName: "assembly.step" }, attemptCount: 1, maxAttempts: 3,
  createdAt: "2026-08-20T10:00:00Z", canCancel: true, canRetry: false, userVisible: true,
};

const running = activity.projectJobActivity(baseJob);
assert.equal(running.status, "RUNNING");
assert.equal(running.progress, 45);
assert.match(running.description, /assembly\.step/);
assert.deepEqual(running.actions, ["CANCEL"]);

const succeeded = activity.projectJobActivity({ ...baseJob, state: "SUCCEEDED", progress: 100,
  resultObjectId: "object-1", canCancel: false, completedAt: "2026-08-20T10:01:00Z" });
assert.deepEqual(succeeded.actions, ["DOWNLOAD"]);
assert.equal(succeeded.progress, undefined);

const failed = activity.projectJobActivity({ ...baseJob, state: "FAILED", canCancel: false, canRetry: true,
  errorMessage: "worker unavailable" });
assert.equal(failed.description, "assembly.step · worker unavailable");
assert.deepEqual(failed.actions, ["RETRY"]);

const feed = activity.buildActivityFeed([
  baseJob,
  { ...baseJob, id: "job-2", type: "FUTURE_TASK", createdAt: "2026-08-20T11:00:00Z" },
]);
assert.equal(feed[0].sourceType, "FUTURE_TASK");
assert.equal(feed[0].title, "后台任务");

console.log("Activity center model tests passed.");
