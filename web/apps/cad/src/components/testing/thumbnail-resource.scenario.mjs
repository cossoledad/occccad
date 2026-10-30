import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { ThumbnailRequests } = await server.ssrLoadModule("/src/components/thumbnail-resource.ts");
  const started = [], bodies = [];
  const requests = new ThumbnailRequests(async (url, options) => {
    started.push(url);
    assert.equal(options.credentials, "include");
    assert.equal(options.cache, "no-cache");
    assert.equal(options.priority, "low");
    return { ok: true, headers: new Headers({ "X-OCCCCAD-Thumbnail-State": "ready" }),
      blob: () => new Promise((resolve, reject) => {
        bodies.push(() => resolve(new Blob([url])));
        options.signal.addEventListener("abort", () => reject(options.signal.reason), { once: true });
      }) };
  });
  const controllers = Array.from({ length: 7 }, () => new AbortController());
  const pending = controllers.map((controller, i) => requests.load(String(i), controller.signal));
  const canceled = assert.rejects(pending[4], { name: "AbortError" });
  controllers[4].abort();
  await Promise.resolve();
  assert.deepEqual(started, ["0", "1", "2", "3"], "body reads, not only headers, occupy the four slots");
  bodies[0]();
  assert.equal((await pending[0]).pending, false);
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(started, ["0", "1", "2", "3", "5"], "canceling a queued image cannot start it later");
  const aborted = assert.rejects(pending[1], { name: "AbortError" });
  controllers[1].abort(); await aborted;
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(started.at(-1), "6", "an aborted active body releases its slot");
  bodies.slice(2).forEach(resolve => resolve());
  await Promise.all([pending[2], pending[3], pending[5], pending[6], canceled]);
  const failed = new ThumbnailRequests(async () => ({ ok: false, status: 403 }));
  await assert.rejects(failed.load("private", new AbortController().signal), /403/);
  assert.equal((await requests.load("already-canceled", AbortSignal.abort()).catch(error => error)).name, "AbortError");
  console.log("Thumbnail queue: bounded body reads, queued/active cancellation, failure and credentials passed.");
} finally { await server.close(); }
