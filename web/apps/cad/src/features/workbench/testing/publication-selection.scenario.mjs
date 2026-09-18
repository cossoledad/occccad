import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { structureSelection } = await server.ssrLoadModule("/src/features/workbench/workbench-tree-model.tsx");
  const view = {
    document: { id: "part", type: "PART", versionId: "revision-2" },
    datumPlanes: [{ id: "datum-xy", plane: "XY", name: "XY", origin: [0, 0, 0], normal: [0, 0, 1], uDirection: [1, 0, 0], size: 100 }],
    part: { units: "mm", datumPlanes: [], axisSystems: [], features: [], publications: [{
      id: "publication-face", name: "Mounting Surface", type: "SURFACE", compatibilityVersion: "1.0.0",
      target: { kind: "TOPOLOGY", sourceVersionId: "revision-1" }, contract: { geometryKind: "SURFACE" },
      resolution: { status: "CONNECTED", resolvedVersionId: "revision-2", topologyKind: "FACE", localId: 7,
        geometryKey: "geometry-key", sourceDigest: "digest" },
    }, {
      id: "publication-plane", name: "Base Plane", type: "PLANE", compatibilityVersion: "1.0.0",
      target: { kind: "DATUM", datumId: "datum-xy" }, contract: { geometryKind: "PLANE" },
      resolution: { status: "CONNECTED", resolvedVersionId: "revision-2" },
    }] },
  };
  const face = structureSelection({ id: "tree/publication-face", kind: "PUBLICATION", name: "Mounting Surface",
    entityId: "publication-face", documentId: "part", versionId: "revision-2" }, view);
  assert.equal(face.kind, "face");
  assert.equal(face.topologyId, 7);
  assert.equal(face.geometryKey, "geometry-key");
  assert.equal(face.publicationId, "publication-face");

  const plane = structureSelection({ id: "tree/publication-plane", kind: "PUBLICATION", name: "Base Plane",
    entityId: "publication-plane", documentId: "part", versionId: "revision-2" }, view);
  assert.equal(plane.kind, "plane");
  assert.equal(plane.entityId, "datum-xy");
  assert.equal(plane.publicationId, "publication-plane");
} finally {
  await server.close();
}
