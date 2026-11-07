import assert from "node:assert/strict";
import {createServer} from "vite";

const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
const activity=await server.ssrLoadModule('/src/features/activity/activity-model.ts');

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
assert.match(failed.description,/assembly.step.*操作未完成/);assert.doesNotMatch(failed.description,/worker unavailable/);
assert.deepEqual(failed.actions, ["RETRY"]);

const feed = activity.buildActivityFeed([
  baseJob,
  { ...baseJob, id: "job-2", type: "FUTURE_TASK", createdAt: "2026-08-20T11:00:00Z" },
]);
assert.equal(feed[0].sourceType, "FUTURE_TASK");
assert.equal(feed[0].title, "后台任务");


const splitImport = activity.projectJobActivity({ ...baseJob, type: "EXCHANGE_IMPORT", progress: 45,
  payload: { fileName: "assembly.step", progressDetail: { phase: "EVALUATING", completed: 60, total: 114 } } });
assert.match(splitImport.description, /生成零件几何（60\/114）/);
const retryWaiting = activity.projectJobActivity({ ...baseJob, state: "RETRY_WAIT", progress: 70 });
assert.equal(retryWaiting.progress, 70);
assert.match(retryWaiting.description, /等待重试，保留已完成进度/);
const retrying = activity.projectJobActivity({ ...baseJob, type: "EXCHANGE_IMPORT", attemptCount: 2, progress: 70,
  payload: { progressDetail: { phase: "CREATING_PARTS", completed: 0, total: 114 } } });
assert.equal(retrying.progress, 70);
assert.match(retrying.description, /第 2 次尝试.*生成零件（0\/114）/);

console.log("Activity center model tests passed.");
} finally {await server.close();}
