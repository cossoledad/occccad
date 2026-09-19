import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { structureSelection, treeData, treeKeysForSelections } = await server.ssrLoadModule("/src/features/workbench/workbench-tree-model.tsx");
  const { bindPublicationSelection } = await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const { describeAssemblyReference } = await server.ssrLoadModule("/src/cad/assembly/assembly-reference-presentation.ts");
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

  const instancePath = { rootDocumentId: "product", canonical: "instance-a", display: "Part1.1",
    segments: [{ ownerDocumentId: "product", ownerVersionId: "product-r1", instanceId: "instance-a",
      instanceName: "Part1.1", referencedDocumentId: "part", resolvedVersionId: "revision-2" }] };
  view.structureTree = { id: "product", kind: "PRODUCT", name: "Product1", documentId: "product", documentType: "PRODUCT",
    children: [{ id: "product/instance:instance-a", kind: "INSTANCE", name: "Part1(Part1.1)", referenceName: "Part1",
      instanceName: "Part1.1", entityId: "instance-a", documentId: "part", documentType: "PART", instancePath,
      children: [{ id: "product/instance:instance-a/reference/body", kind: "BODY", name: "PartBody", documentId: "part", instancePath },
        { id: "product/instance:instance-a/reference/publications/publication:publication-face", kind: "PUBLICATION",
          name: "Mounting Surface", entityId: "publication-face", documentId: "part", instancePath,
          publication: view.part.publications[0] }] }] };
  view.resolvedInstances = [{ id: "instance-a", documentId: "part", geometryKey: "geometry-key", instancePath }];
  const rawFace = { kind: "face", id: "pick", topologyId: 7, geometryKey: "geometry-key", occurrencePath: "instance-a",
    instanceId: "instance-a", documentId: "part" };
  const promoted = bindPublicationSelection(rawFace, view.structureTree);
  assert.equal(promoted.publicationId, "publication-face", "viewport picks should acquire the unique Publication identity");
  const nodes = treeData(view, view);
  const selectedKeys = treeKeysForSelections(nodes, [promoted]);
  assert(selectedKeys.includes("product/instance:instance-a/reference/body"), "the owning PartBody should be highlighted");
  assert(selectedKeys.includes("product/instance:instance-a/reference/publications/publication:publication-face"), "the Publication node should be highlighted");
  const presentation = describeAssemblyReference({ instanceId: "instance-a", kind: "FACE", geometryKey: "geometry-key",
    topologyId: 7, publicationRef: { publicationId: "publication-face", expectedType: "SURFACE", compatibilityVersion: "1.0.0" } }, view);
  assert.equal(presentation.primary, "Part1(Part1.1) / Mounting Surface");
  assert.equal(presentation.publication, "Mounting Surface");
} finally {
  await server.close();
}
