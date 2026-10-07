import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { DocumentSessions, registerDocumentState } = await server.ssrLoadModule("/src/cad/document/document-session.ts");
  const { restoreDocumentEditingState, documentEditingState } = await server.ssrLoadModule("/src/features/workbench/document-editing-state.ts");
  const { rootEditSession, EditActivationGate } = await server.ssrLoadModule("/src/features/workbench/edit-session.ts");
  const THREE = await server.ssrLoadModule("three");
  const { CadViewportEngine } = await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const { documentCameraState, captureDocumentCamera } = await server.ssrLoadModule("/src/cad/navigation/document-camera-state.ts");
  const { saveView } = await server.ssrLoadModule("/src/cad/navigation/orthographic-view.ts");
  const sessions = new DocumentSessions(), slot = registerDocumentState("test.document-state");
  assert.throws(() => registerDocumentState(slot.id), /duplicate/);
  assert.throws(() => registerDocumentState(" "), /Invalid/);
  assert.equal(sessions.write("root", slot, {}), false, "state writes do not implicitly open a document");
  sessions.open("root"); sessions.open("other");
  let updates = 0; const unsubscribe = sessions.subscribe(() => updates++);
  const state = { values: [1] }; sessions.write("root", slot, state); state.values.push(2);
  assert.deepEqual(sessions.read("root", slot), { values: [1] });
  assert.equal(sessions.read("other", slot), undefined); assert.equal(updates, 1);
  sessions.close("root");
  assert.equal(sessions.write("root", slot, state), false, "late cleanup cannot recreate a closed document's snapshot");
  sessions.open("root"); assert.equal(sessions.read("root", slot), undefined); unsubscribe();

  const path = { rootDocumentId: "root", canonical: "nested/part", segments: [
    { ownerDocumentId: "root", instanceId: "nested", referencedDocumentId: "assembly" },
    { ownerDocumentId: "assembly", instanceId: "part", referencedDocumentId: "part" },
  ] };
  const host = { document: { id: "root", type: "PRODUCT", versionId: "root-new" }, product: { instances: [] },
    structureTree: { kind: "PRODUCT", documentId: "root", children: [
      { kind: "INSTANCE", referenceMode: "FOLLOW_HEAD", instancePath: { ...path, canonical: "nested" } },
    ] } };
  const part = { document: { id: "part", type: "PART", versionId: "part-new", permission: "OWNER" }, part: {
    activeBodyId: "first", bodies: [{ id: "first" }, { id: "working" }], features: [{ id: "sketch", type: "SKETCH" }],
  } };
  const resolve = async () => ({ rootProductDocumentId: "root", rootProductRevisionId: "root-new",
    activeDocumentId: "part", activeRevisionId: "part-new", activeInstancePath: path });
  const saved = { session: { ...rootEditSession(host, 1), editTarget: { documentId: "part", documentType: "PART", instancePath: path },
    workingBodyId: "working", snapshot: { hostRevisionId: "old", targetRevisionId: "old" } }, sketchId: "sketch",
    newSketchSession: { documentId: "part", sketchId: "sketch", edited: false } };
  sessions.write("root", documentEditingState, saved);
  const restored = await restoreDocumentEditingState(host, sessions.read("root", documentEditingState), 2, resolve, async () => part);
  assert.equal(restored.session.editTarget.instancePath.canonical, "nested/part");
  assert.equal(restored.session.snapshot.targetRevisionId, "part-new", "restore validates fresh versions instead of replaying a stale baseline");
  assert.equal(restored.session.workingBodyId, "working"); assert.equal(restored.sketchId, "sketch");
  assert.deepEqual(restored.newSketchSession, saved.newSketchSession);
  const independent = { ...saved, session: { ...rootEditSession(part, 1), workingBodyId: "working" } };
  const independentRestore = await restoreDocumentEditingState(part, independent, 2,
    async () => { throw Error("Part restore must not resolve a Product occurrence"); }, async () => part);
  assert.equal(independentRestore.session.workingBodyId, "working"); assert.equal(independentRestore.sketchId, "sketch");
  const readonlyRestore = await restoreDocumentEditingState({ ...part, document: { ...part.document, permission: "VIEWER" } },
    independent, 2, resolve, async () => part);
  assert.equal(readonlyRestore.sketchId, undefined, "lost edit permission cannot reactivate sketch editing");
  const removed = await restoreDocumentEditingState(host, saved, 3, resolve, async () => ({ ...part,
    part: { ...part.part, bodies: [{ id: "first" }], features: [] } }));
  assert.equal(removed.session.workingBodyId, "first"); assert.equal(removed.sketchId, undefined);
  assert.equal(removed.newSketchSession, undefined);
  const wrongHost = await restoreDocumentEditingState({ ...part, document: { ...part.document, id: "other" } }, saved, 4,
    async () => { throw Error("foreign host state must not resolve"); }, async () => part);
  assert.equal(wrongHost.session.editTarget.documentId, "other"); assert.equal(wrongHost.sketchId, undefined);
  const pinned = structuredClone(host); pinned.structureTree.children[0].referenceMode = "PINNED";
  await assert.rejects(() => restoreDocumentEditingState(pinned, saved, 5, resolve, async () => part), /已固定版本/);
  await assert.rejects(() => restoreDocumentEditingState(host, saved, 5, resolve,
    async () => ({ ...part, document: { ...part.document, versionId: "unresolved" } })), /尚未解析/);
  const gate = new EditActivationGate(); let finish;
  const generation = gate.begin();
  const pending = restoreDocumentEditingState(host, saved, generation, () => new Promise(resolve => { finish = resolve; }), async () => part);
  gate.invalidate(); finish(await resolve());
  assert.equal(gate.commit(generation, (await pending).session, () => { throw Error("late restore applied"); }), false);

  // Exercise the engine's real document-entry and sketch-entry path without a
  // WebGL renderer. Scene construction is irrelevant to camera/session state.
  const camera = new THREE.OrthographicCamera(-80, 80, 60, -60, .01, 10000), target = new THREE.Vector3(10, 20, 30);
  camera.position.set(100, -100, 80); camera.up.set(0, 0, 1); camera.lookAt(target); camera.zoom = 2.5;
  const beforeSketch = saveView(camera, target);
  camera.position.add(new THREE.Vector3(12, 13, 14)); camera.zoom = 7;
  camera.rotateZ(.2); camera.updateProjectionMatrix(); camera.updateMatrixWorld(true);
  const snapshot = captureDocumentCamera(camera, target, beforeSketch);
  sessions.write("root", documentCameraState, snapshot);
  const engine = Object.create(CadViewportEngine.prototype), noop = () => {};
  let cameraSyncs = 0, ready = 0, animations = 0;
  Object.assign(engine, { documentSessions: sessions, camera, navigation: { target, cancel: noop, syncCamera: () => cameraSyncs++, setEnabled: noop },
    renderer: { domElement: { clientWidth: 1200, clientHeight: 600 } }, viewTransition: { cancel: noop, start: () => animations++ },
    content: new THREE.Group(), helpers: new THREE.Group(), sketchContext: new THREE.Group(), instanceGroups: new Map(),
    solidBindings: new Map(), selectable: new Set(), assemblyConstraintReferences: new Map(), assemblyConstraintMarkers: new Map(),
    screenStableReferences: new Map(), selectionIndex: { clear: noop }, selected: [], activeToolID: "select",
    transforms: { stopAll: noop }, moveManipulator: { detach: noop },
    clearInsertPatternPreview: noop, clearCommandPreview: noop, clearInteractionState: noop, disposeGroup: noop,
    renderPart: noop, renderProduct: noop, updateSketchContextVisibility: noop, applyTreeVisibility: noop,
    setDatumPreview: noop, refreshContentBounds: noop, selectMany: noop, select: noop, buildSketchContext: noop,
    emitDebugState: noop, invalidate: noop,
    frameContent: () => { throw Error("restored camera must not be fitted automatically"); },
    callbacks: { toolPromptChanged: noop, documentViewReady: (id, restoredCamera) => {
      ready++; assert.equal(id, "root"); assert.equal(restoredCamera, true);
      engine.beginSketch("sketch", {}, restoredCamera);
    } },
  });
  camera.position.set(0, 0, 1); camera.top = 500; camera.bottom = -500; target.set(0, 0, 0);
  engine.renderReady(host);
  assert.deepEqual(camera.position.toArray(), snapshot.position); assert.deepEqual(camera.quaternion.toArray(), snapshot.rotation);
  assert.deepEqual(target.toArray(), snapshot.target); assert.equal(camera.zoom, snapshot.zoom);
  assert.equal(camera.top, 60); assert.equal(camera.left, -120, "restore adapts saved vertical span to current viewport aspect");
  assert.equal(animations, 0, "resuming sketch does not normalize the restored camera"); assert.equal(ready, 1); assert.equal(cameraSyncs, 1);
  assert.deepEqual(engine.sketchReturnView.position.toArray(), beforeSketch.position.toArray());
  // Capture is keyed to the physical scene's host, regardless of its edit target.
  engine.captureDocumentView(); assert.equal(sessions.read("other", documentCameraState), undefined);
  sessions.close("root"); engine.captureDocumentView(); assert.equal(sessions.read("root", documentCameraState), undefined);
  console.log("Document snapshots, close lifecycle, validated nested edit/sketch restore and engine camera restoration passed.");
} finally { await server.close(); }
