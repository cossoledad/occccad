import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { topologyPropertyContext } = await server.ssrLoadModule("/src/features/workbench/topology-property-context.ts");
  assert.deepEqual(topologyPropertyContext({ kind: "face", id: "pick", topologyId: 6,
    documentId: "part-document", versionId: "accepted-part-v3", geometryKey: "part-geometry",
    occurrencePath: "product/instance-a" }, "product-document"),
  { documentId: "part-document", versionId: "accepted-part-v3" });
  assert.deepEqual(topologyPropertyContext(null, "product-document"), { documentId: "product-document" });
} finally { await server.close(); }
