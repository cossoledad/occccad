import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { submitImportBatch } = await server.ssrLoadModule("/src/features/documents/import-batch.ts");
  const files = Array.from({ length: 5 }, (_, index) => ({ uid: String(index), file: { name: `part-${index}.step` } }));
  let active = 0, peak = 0;
  const progress = [];
  const result = await submitImportBatch(files, async (file) => {
    active++; peak = Math.max(peak, active);
    await new Promise((resolve) => setTimeout(resolve, file.name === "part-0.step" ? 15 : 1));
    active--;
    if (file.name === "part-2.step") throw new Error("upload rejected");
    return { id: file.name };
  }, 2, (finished, total) => progress.push([finished, total]));
  assert.equal(peak, 2, "bounded uploads must overlap");
  assert.deepEqual(result.submitted.map((job) => job.id), ["part-0.step", "part-1.step", "part-3.step", "part-4.step"]);
  assert.deepEqual(result.failures, [{ uid: "2", name: "part-2.step", reason: "upload rejected" }]);
  assert.deepEqual(progress.map(([finished]) => finished), [1, 2, 3, 4, 5]);
  assert(progress.every(([, total]) => total === 5));
  console.log("Import batch concurrency and partial failure passed.");
} finally { await server.close(); }
