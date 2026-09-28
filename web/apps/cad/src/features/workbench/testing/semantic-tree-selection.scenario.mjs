import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";

const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const tree = await server.ssrLoadModule("/src/features/workbench/workbench-tree-model.tsx");
  const { selectionKey, sameSelection } = await server.ssrLoadModule("/src/cad/interaction/selection-identity.ts");
  const { selectionModeForTool } = await server.ssrLoadModule("/src/cad/interaction/selection-mode.ts");
  const { Properties } = await server.ssrLoadModule("/src/features/workbench/workbench-inspector.tsx");
  const path = (id, revisionId) => ({ rootDocumentId: "product", canonical: id, display: id,
    segments: [{ ownerDocumentId: "product", ownerVersionId: "product-r1", instanceId: id,
      instanceName: id, referencedDocumentId: "part", resolvedVersionId: revisionId }] });
  const feature = (prefix, instancePath, revisionId, geometryKey) => ({
    id: `${prefix}/pad:feature`, kind: "PAD", name: "Pad.1", entityId: "feature", bodyId: "body-a",
    documentId: "part", versionId: revisionId, geometryKey, instancePath,
  });
  const body = (prefix, instancePath, revisionId, geometryKey) => ({
    id: `${prefix}/body:body-a`, kind: "BODY", name: "Body.1", entityId: "body-a",
    bodyId: "body-a", documentId: "part", versionId: revisionId, geometryKey, instancePath,
    children: [feature(`${prefix}/body:body-a`, instancePath, revisionId, geometryKey)],
  });
  const firstPath = path("instance-a", "revision-1"), secondPath = path("instance-b", "revision-2");
  const view = { document: { id: "product", type: "PRODUCT", versionId: "product-r1" },
    structureTree: { id: "document:product", kind: "PRODUCT", name: "Product", documentId: "product",
      children: [
        { id: "document:product/instance:instance-a", kind: "INSTANCE", name: "Part(A)",
          entityId: "instance-a", documentId: "part", versionId: "revision-1", instancePath: firstPath,
          children: [{ id: "document:product/instance:instance-a/reference", kind: "PART", name: "Part",
            documentId: "part", versionId: "revision-1", instancePath: firstPath,
            children: [body("document:product/instance:instance-a/reference", firstPath, "revision-1", "geometry-1")] }] },
        { id: "document:product/instance:instance-b", kind: "INSTANCE", name: "Part(B)",
          entityId: "instance-b", documentId: "part", versionId: "revision-2", instancePath: secondPath,
          referenceMode: "PINNED", children: [{ id: "document:product/instance:instance-b/reference", kind: "PART",
            name: "Part", documentId: "part", versionId: "revision-2", instancePath: secondPath,
            children: [body("document:product/instance:instance-b/reference", secondPath,
              "revision-2", "geometry-2")] }] },
      ] },
    resolvedInstances: [
      { bodyId: "body-a", documentId: "part", occurrencePath: "instance-a", geometryKey: "geometry-1",
        instancePath: firstPath, bodyVisible: true },
      { bodyId: "body-a", documentId: "part", occurrencePath: "instance-b", geometryKey: "geometry-2",
        instancePath: secondPath, bodyVisible: true },
    ], artifacts: { "geometry-2": { geometryKey: "geometry-2", representations: {
      VISUAL: { objectId: "mesh-object", size: 128, schemaVersion: 1 } } } } };
  const nodes = tree.treeData(view);
  const instances = view.structureTree.children;
  const first = tree.structureSelection(instances[0].children[0].children[0].children[0], view);
  const second = tree.structureSelection(instances[1].children[0].children[0].children[0], view);
  assert.equal(first.kind, "pad");
  assert.notEqual(selectionKey(first), selectionKey(second), "same Feature in different resolved occurrences must differ");
  assert.deepEqual(tree.treeKeysForSelections(nodes, [first]), [instances[0].children[0].children[0].children[0].id]);
  assert.deepEqual(tree.treeKeysForSelections(nodes, [second]), [instances[1].children[0].children[0].children[0].id]);
  const occurrencePart = tree.structureSelection(instances[0].children[0], view);
  assert.equal(occurrencePart.kind, "part");
  assert.notEqual(selectionKey(occurrencePart), selectionKey(tree.structureSelection(instances[0], view)),
    "Part definition and occurrence have different command identities");
  const part = tree.structureSelection({ id: "document:part", kind: "PART", name: "Part", documentId: "part" }, view);
  assert.equal(part.kind, "part", "selecting a Part cannot become Body selection");
  const face = (bodyId, geometryKey, revisionId) => ({ kind: "face", id: "FACE 1", topologyId: 1,
    documentId: "part", bodyId, geometryKey, versionId: revisionId });
  assert.notEqual(selectionKey(face("body-a", "geometry-1", "revision-1")),
    selectionKey(face("body-b", "geometry-2", "revision-1")));
  assert.equal(sameSelection(face("body-a", "geometry-1", "revision-1"),
    face("body-a", "geometry-1", "revision-2")), false);
  const projectedInstance = selectionModeForTool("assembly.move").project({ ...first, instanceId: "instance-a" });
  assert.equal(projectedInstance.instancePath.canonical, "instance-a");
  assert.notEqual(selectionKey(projectedInstance), selectionKey(selectionModeForTool("assembly.move")
    .project({ ...second, instanceId: "instance-b" })), "assembly instance projection keeps occurrence identity");
  const picked = { ...face("body-a", "geometry-1", "revision-1"), occurrencePath: "instance-a",
    instancePath: firstPath, treeNodeId: instances[0].children[0].children[0].id };
  assert.deepEqual(tree.treeKeysForSelections(nodes, [picked]), [], "topology has no exact Body selection");
  assert(tree.ancestorHintKeysForSelections(nodes, [picked]).includes(instances[0].children[0].children[0].id),
    "collapsed owning Body gets a descendant hint");
  assert.equal(tree.treeKeyForSelection(nodes, picked), undefined, "ancestor hint never becomes command target");
  const pinnedBody = tree.structureSelection(instances[1].children[0].children[0], view);
  const properties = renderToStaticMarkup(createElement(Properties, { view, selection: pinnedBody,
    workbench: "part-design", activeTool: "select", navigationProfile: "cad" }));
  assert(properties.includes("/api/documents/part/representations/mesh-object?versionId=revision-2"),
    "Product Body file link must use the pinned Part Revision");
  assert(properties.includes("bodyId=body-a"), "file link keeps the selected Body scope");
  const publicationNode = { id: "document:part/publications/publication:pub", kind: "PUBLICATION",
    name: "Published Face", entityId: "pub", documentId: "part", versionId: "revision-1",
    publication: { id: "pub", name: "Published Face", type: "SURFACE", target: { kind: "TOPOLOGY",
      persistentSelection: { sourceBodyId: "body-a" } },
      resolution: { status: "CONNECTED", geometryKey: "geometry-1", resolvedVersionId: "revision-1",
        topologyKind: "FACE", localId: 1 } } };
  const pub = tree.structureSelection(publicationNode, { ...view, document: { id: "part" } });
  assert.equal(pub.kind, "publication");
  assert.equal(pub.highlightTarget.kind, "face");
  assert.notEqual(selectionKey(pub), selectionKey(picked));
  console.log("Semantic tree selection, Body/occurrence/revision identity and ancestor hints passed");
} finally { await server.close(); }
