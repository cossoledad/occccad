import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../edit-session.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const sessions = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);

const path = (canonical, documentId, revisionId = "part-r1") => ({ rootDocumentId: "root", canonical,
  display: canonical, segments: canonical.split("/").map((instanceId, index, all) => ({
    ownerDocumentId: index ? "nested" : "root", ownerVersionId: "root-r1", instanceId,
    instanceName: instanceId, referencedDocumentId: index === all.length - 1 ? documentId : "nested",
    resolvedVersionId: index === all.length - 1 ? revisionId : "nested-r1",
  })) });
const firstPath = path("part-a", "part"), secondPath = path("part-b", "part");
const nestedPath = path("nested-a", "nested", "nested-r1");
const emptyPartView = { document: { id: "part", name: "Empty", type: "PART", versionId: "part-r1" },
  part: { bodies: [], activeBodyId: "", features: [], units: "mm", datumPlanes: [], axisSystems: [] } };
const host = { document: { id: "root", name: "Root", type: "PRODUCT", versionId: "root-r1" },
  structureTree: { id: "document:root", kind: "PRODUCT", name: "Root", documentId: "root", documentType: "PRODUCT", children: [
    { id: "instance:a", kind: "INSTANCE", name: "A", documentId: "part", documentType: "PART", referenceMode: "FOLLOW_HEAD", instancePath: firstPath,
      children: [{ id: "instance:a/part", kind: "PART", name: "Part", documentId: "part", documentType: "PART", instancePath: firstPath }] },
    { id: "instance:b", kind: "INSTANCE", name: "B", documentId: "part", documentType: "PART", referenceMode: "FOLLOW_HEAD", instancePath: secondPath,
      children: [{ id: "instance:b/part", kind: "PART", name: "Part", documentId: "part", documentType: "PART", instancePath: secondPath }] },
    { id: "instance:nested", kind: "INSTANCE", name: "Nested", documentId: "nested", documentType: "PRODUCT", referenceMode: "FOLLOW_HEAD", instancePath: nestedPath,
      children: [{ id: "instance:nested/product", kind: "PRODUCT", name: "Nested", documentId: "nested", documentType: "PRODUCT", instancePath: nestedPath }] },
  ] } };
const resolve = async (_root, canonical) => ({ rootProductDocumentId: "root", rootProductRevisionId: "root-r1",
  rootSnapshotDigest: "snapshot-1", activeInstancePath: canonical === "part-b" ? secondPath : canonical === "nested-a" ? nestedPath : firstPath,
  activeDocumentId: canonical === "nested-a" ? "nested" : "part",
  activeRevisionId: canonical === "nested-a" ? "nested-r1" : "part-r1", contextCatalogDigest: "catalog-1" });
const load = async (id) => id === "part" ? emptyPartView : { document: { id: "nested", name: "Nested", type: "PRODUCT", versionId: "nested-r1" } };

const instanceTarget = await sessions.prepareOccurrenceEditSession({ host, node: host.structureTree.children[0], activationGeneration: 1,
  getDesignSession: resolve, getDocument: load });
const definitionTarget = await sessions.prepareOccurrenceEditSession({ host, node: host.structureTree.children[0].children[0], activationGeneration: 2,
  getDesignSession: resolve, getDocument: load });
assert.deepEqual(instanceTarget.session.editTarget, definitionTarget.session.editTarget,
  "double-clicking an Instance and its definition root must resolve the same target");
assert.equal(instanceTarget.session.workingBodyId, undefined, "empty Part activation cannot depend on a resolved Body");
assert(sessions.isEditTargetNode(instanceTarget.session, host.structureTree.children[0].children[0]));
assert(!sessions.isEditTargetNode(instanceTarget.session, host.structureTree.children[0]), "Instance is only an activation shortcut");
assert(!sessions.isEditTargetNode(instanceTarget.session, host.structureTree.children[1].children[0]),
  "another occurrence of the same Part must not share edit state");

const nested = await sessions.prepareOccurrenceEditSession({ host, node: host.structureTree.children[2], activationGeneration: 3,
  getDesignSession: resolve, getDocument: load });
assert.equal(nested.session.editTarget.documentType, "PRODUCT");
assert(sessions.isEditTargetNode(nested.session, host.structureTree.children[2].children[0]));
const root = sessions.rootEditSession(host, 4);
assert(sessions.isEditTargetNode(root, host.structureTree), "root Product activation returns to assembly editing");

const pinnedHost = structuredClone(host);
pinnedHost.structureTree.children[2].referenceMode = "PINNED";
pinnedHost.structureTree.children[2].children[0].children = [{ id: "nested-part", kind: "PART", name: "Part", documentId: "part",
  documentType: "PART", instancePath: path("nested-a/part-c", "part") }];
await assert.rejects(() => sessions.prepareOccurrenceEditSession({ host: pinnedHost,
  node: pinnedHost.structureTree.children[2].children[0].children[0], activationGeneration: 5,
  getDesignSession: resolve, getDocument: load }), /已固定版本/);

const gate = new sessions.EditActivationGate();
const oldGeneration = gate.begin(), newGeneration = gate.begin();
let committed = "";
assert.equal(gate.commit(oldGeneration, instanceTarget.session, () => { committed = "old"; }), false);
assert.equal(gate.commit(newGeneration, definitionTarget.session, () => { committed = "new"; }), true);
assert.equal(committed, "new", "a stale activation callback cannot replace the newer session");

const workbenchSource = await readFile(new URL("../workbench.tsx", import.meta.url), "utf8");
assert(!workbenchSource.includes("definitionContextPath"));
assert(!workbenchSource.includes("打开定义"));
assert(!workbenchSource.includes("在此上下文打开"));
assert(!workbenchSource.includes("定义编辑 ·"));
console.log("Product edit-session identity, snapshot validation, empty Part, pinned path and stale activation passed");
