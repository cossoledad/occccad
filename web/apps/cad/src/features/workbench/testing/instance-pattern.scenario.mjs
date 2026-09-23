import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { instancePatternOffsets } = await server.ssrLoadModule("/src/features/workbench/instance-pattern.ts");
  assert.deepEqual(instancePatternOffsets({ axis: "Y", count: 4, spacing: 12.5, reversed: true }),
    [[0, -12.5, 0], [0, -25, 0], [0, -37.5, 0]]);
  assert.deepEqual(instancePatternOffsets({ axis: "Z", count: 2, spacing: 3, reversed: false }), [[0, 0, 3]]);
  for (const invalid of [{ axis: "X", count: 1, spacing: 1 }, { axis: "Q", count: 2, spacing: 1 },
    { axis: "X", count: 2, spacing: 0 }, { axis: "X", count: 129, spacing: 1 }]) {
    assert.deepEqual(instancePatternOffsets({ ...invalid, reversed: false }), []);
  }
  console.log("Instance pattern offsets passed.");
} finally { await server.close(); }
