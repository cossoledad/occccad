import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../web/apps/cad/package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));

const server = await createServer({
  appType: "custom",
  logLevel: "silent",
  server: { middlewareMode: true },
});

try {
  const {
    ConstraintSketchTool,
    LinearDimensionSketchTool,
    CircleSketchTool,
    LineSketchTool,
    PointSketchTool,
    PolylineSketchTool,
    RectangleSketchTool,
    SlotSketchTool,
  } = await server.ssrLoadModule("/src/cad/tool/cad-tool.ts");
  const { buildSketchRenderModel } = await server.ssrLoadModule("/src/cad/rendering/sketch-render-model.ts");
  const { visualSelection, visualType } = await server.ssrLoadModule("/src/cad/rendering/visualization-render-model.ts");
  const { resolveSketchSnap } = await server.ssrLoadModule("/src/cad/interaction/sketch-snap.ts");
  const { allowsSelection, DEFAULT_CAPTURE_SETTINGS, selectionCaptureKind } = await server.ssrLoadModule("/src/cad/interaction/capture-settings.ts");
  const { ToolManager } = await server.ssrLoadModule("/src/cad/tool/tool-manager.ts");
  const { SelectionIndex } = await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
  const { resultBodyFeatureTreeNode } = await server.ssrLoadModule("/src/cad/interaction/selection-hierarchy.ts");
  const { SelectionController } = await server.ssrLoadModule("/src/cad/interaction/selection-controller.ts");
  const { closestTreeKey, resolveTreeSelection } = await server.ssrLoadModule("/src/features/workbench/tree-selection.ts");
  const { resolveSketchReference } = await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
  const { constraintDefinition, TOOLBAR_CONSTRAINT_KINDS } = await server.ssrLoadModule("/src/cad/sketch/sketch-constraint-definition.ts");
  const { buildSketchConstraintLayout } = await server.ssrLoadModule("/src/cad/sketch/sketch-constraint-layout.ts");
  const { CadShaderLibrary } = await server.ssrLoadModule("/src/cad/rendering/shader/cad-shader-library.ts");
  const { makeOcclusionVisibleHighlightLine } = await server.ssrLoadModule("/src/cad/rendering/interaction-highlight.ts");
  const { defaultDocumentName } = await server.ssrLoadModule("/src/features/documents/document-utils.ts");
  const operations = [];
  const prompts = [];
  const previews = [];
  let completions = 0;
  const references = [
    { target: "ENTITY", entityId: "line-a", subElement: "END" },
    { target: "ENTITY", entityId: "line-b", subElement: "START" },
    { target: "ENTITY", entityId: "line-b", subElement: "START" },
  ];
  const viewport = {
    sketchPoint: (x, y) => [x, y],
    sketchPlacementPoint: (x, y) => [x, y],
    showPolylinePreview: (points, closed) => previews.push({ points, closed }),
    showPointPreview: (point) => previews.push({ point }),
    clearToolPreview: () => previews.push("clear"),
    commitSketchOperations: (value) => operations.push(value),
    hasActiveSketch: () => true,
    sketchReferenceAt: () => references.shift() ?? null,
    showReferencePreview: (value, retained) => previews.push({ reference: value, retained }),
    showConstraintPreview: (kind, value, dimension, labelPosition) => previews.push({ constraintPreview: kind, value, dimension, labelPosition }),
    beginDimensionDrag: () => false, updateDimensionDrag: () => {}, finishDimensionDrag: () => {}, cancelDimensionDrag: () => {},
    editDimensionAt: () => false,
    clearReferencePreview: () => previews.push("clear-reference"),
    setToolPrompt: (value) => prompts.push(value),
    finishToolUse: () => { completions += 1; },
  };
  const context = { viewport };
  const pointer = (x, y, phase = "move") => ({
    phase,
    pointerId: 1,
    pointerType: "mouse",
    button: 0,
    x,
    y,
    deltaX: 0,
    deltaY: 0,
    originalEvent: { detail: 1 },
    state: { buttons: { left: false, middle: false, right: false },
      modifiers: { ctrl: false, shift: false, alt: false, meta: false }, keys: new Set(), pointer: { x, y, deltaX: 0, deltaY: 0 } },
  });

  const rectangle = new RectangleSketchTool();
  rectangle.activate(context);
  assert.equal(rectangle.pointerDown(pointer(2, 3, "down"), context), "capture");
  assert.equal(rectangle.pointerUp(pointer(2, 3, "up"), context), "release-capture");
  rectangle.pointerMove(pointer(8, 9), context);
  assert.equal(rectangle.pointerDown(pointer(8, 9, "down"), context), "capture");
  assert.equal(rectangle.pointerUp(pointer(8, 9, "up"), context), "release-capture");
  assert.equal(operations[0][0].type, "ADD_RECTANGLE");
  assert.equal(completions, 1);
  assert.equal(previews.some((value) => value?.closed === true), true);

  const line = new LineSketchTool();
  line.activate(context);
  assert.equal(line.pointerMove(pointer(0.2, 0.3), context), "consumed");
  line.pointerDown(pointer(1, 1, "down"), context);
  line.pointerUp(pointer(1, 1, "up"), context);
  line.pointerMove(pointer(5, 5), context);
  line.pointerDown(pointer(5, 5, "down"), context);
  line.pointerUp(pointer(5, 5, "up"), context);
  assert.equal(operations[1][0].entity.kind, "LINE");
  assert.equal(completions, 2);

  const cancelledLine = new LineSketchTool();
  cancelledLine.activate(context);
  cancelledLine.pointerDown(pointer(1, 1, "down"), context);
  cancelledLine.pointerUp(pointer(1, 1, "up"), context);
  assert.equal(cancelledLine.keyDown({ key: "Escape" }, context), "consumed");

  const point = new PointSketchTool();
  point.pointerMove(pointer(4, 5), context);
  assert.equal(point.pointerDown(pointer(4, 5, "down"), context), "capture");
  assert.equal(point.pointerUp(pointer(4, 5, "up"), context), "release-capture");
  assert.equal(operations[2][0].entity.role, "PROFILE");
  assert.equal(completions, 3);
  assert.equal(previews.some((value) => value?.point?.[0] === 4), true);

  const coincident = new ConstraintSketchTool("sketch.constraint.coincident");
  coincident.activate(context);
  coincident.pointerDown(pointer(0, 0, "down"), context);
  coincident.pointerUp(pointer(0, 0, "up"), context);
  assert.equal(coincident.pointerMove(pointer(4, 0), context), "consumed");
  assert.deepEqual(previews.at(-1), { reference: references[0], retained: { target: "ENTITY", entityId: "line-a", subElement: "END" } });
  coincident.pointerDown(pointer(0, 0, "down"), context);
  coincident.pointerUp(pointer(0, 0, "up"), context);
  assert.equal(operations[3][0].constraint.kind, "COINCIDENT");
  assert.equal(completions, 4);
  assert.equal(prompts.some((value) => value.includes("第二个点")), true);

  const circleIndex = operations.length;
  const circle = new CircleSketchTool();
  circle.pointerDown(pointer(0, 0, "down"), context); circle.pointerUp(pointer(0, 0, "up"), context);
  circle.pointerDown(pointer(0, 10, "down"), context); circle.pointerUp(pointer(0, 10, "up"), context);
  assert.equal(operations[circleIndex][0].entity.kind, "CIRCLE");
  assert.equal(operations[circleIndex][0].entity.radius, 10);

  const polylineIndex = operations.length;
  const polyline = new PolylineSketchTool();
  polyline.pointerDown(pointer(0, 0, "down"), context); polyline.pointerUp(pointer(0, 0, "up"), context);
  polyline.pointerDown(pointer(10, 0, "down"), context); polyline.pointerUp(pointer(10, 0, "up"), context);
  polyline.pointerDown(pointer(10, 10, "down"), context); polyline.pointerUp(pointer(10, 10, "up"), context);
  polyline.keyDown({ key: "Enter" }, context);
  assert.equal(operations[polylineIndex].filter((item) => item.type === "ADD_ENTITY").length, 2);
  assert.equal(operations[polylineIndex].filter((item) => item.type === "ADD_CONSTRAINT").length, 1);

  references.push({ target: "ENTITY", entityId: "line-dimension", subElement: "DIRECTION" });
  const dimensionIndex = operations.length;
  const length = new ConstraintSketchTool("LENGTH");
  length.pointerDown(pointer(0, 0, "down"), context); length.pointerUp(pointer(0, 0, "up"), context);
  length.pointerMove(pointer(10, 8), context);
  length.pointerDown(pointer(10, 8, "down"), context); length.pointerUp(pointer(10, 8, "up"), context);
  length.keyDown({ key: "2" }, context); length.keyDown({ key: "5" }, context); length.keyDown({ key: "Enter" }, context);
  assert.equal(operations[dimensionIndex][0].constraint.value, 25);
  assert.equal(operations[dimensionIndex][0].constraint.unit, "mm");
  assert.deepEqual(operations[dimensionIndex][0].constraint.labelPosition, { x: 10, y: 8 });

  references.push({ target: "ENTITY", entityId: "line-smart", subElement: "WHOLE" });
  const smartLengthIndex = operations.length;
  const smartLength = new LinearDimensionSketchTool();
  smartLength.pointerDown(pointer(2, 2, "down"), context); smartLength.pointerUp(pointer(2, 2, "up"), context);
  smartLength.pointerMove(pointer(12, 9), context);
  smartLength.pointerDown(pointer(12, 9, "down"), context); smartLength.pointerUp(pointer(12, 9, "up"), context);
  smartLength.keyDown({ key: "4" }, context); smartLength.keyDown({ key: "0" }, context); smartLength.keyDown({ key: "Enter" }, context);
  assert.equal(operations[smartLengthIndex][0].constraint.kind, "LENGTH");

  references.push({ target: "ENTITY", entityId: "line-smart", subElement: "START" },
    { target: "ENTITY", entityId: "line-smart", subElement: "END" });
  const smartDistanceIndex = operations.length;
  const smartDistance = new LinearDimensionSketchTool();
  for (const [x, y] of [[0, 0], [20, 0]]) {
    smartDistance.pointerDown(pointer(x, y, "down"), context); smartDistance.pointerUp(pointer(x, y, "up"), context);
  }
  smartDistance.pointerDown(pointer(10, 10, "down"), context); smartDistance.pointerUp(pointer(10, 10, "up"), context);
  smartDistance.keyDown({ key: "2" }, context); smartDistance.keyDown({ key: "0" }, context); smartDistance.keyDown({ key: "Enter" }, context);
  assert.equal(operations[smartDistanceIndex][0].constraint.kind, "DISTANCE");

  const slotIndex = operations.length;
  const slot = new SlotSketchTool();
  for (const [x, y] of [[0, 0], [20, 0], [10, 5]]) {
    slot.pointerDown(pointer(x, y, "down"), context); slot.pointerUp(pointer(x, y, "up"), context);
  }
  assert.equal(operations[slotIndex].filter((item) => item.entity?.kind === "LINE").length, 2);
  assert.equal(operations[slotIndex].filter((item) => item.entity?.kind === "ARC").length, 2);
  assert.equal(operations[slotIndex].filter((item) => item.constraint?.kind === "COINCIDENT").length, 4);

  const renderModel = buildSketchRenderModel({
    id: "sketch-1", type: "SKETCH", sketch: { entities: [
      { id: "point-1", kind: "POINT", role: "PROFILE", point: { x: 4, y: 5 } },
      { id: "line-1", kind: "LINE", role: "PROFILE", start: { x: 0, y: 0 }, end: { x: 2, y: 0 } },
    ] },
  });
  assert.deepEqual(renderModel.profilePoints, [[4, 5]]);
  assert.deepEqual(renderModel.profileLines, [[0, 0], [2, 0]]);
  assert.deepEqual(renderModel.endpoints, [[0, 0], [2, 0]]);

  const snapEntities = [{ id: "line-a", kind: "LINE", role: "PROFILE",
    start: { x: 0, y: 0 }, end: { x: 20, y: 0 } }];
  assert.deepEqual(resolveSketchSnap([0.7, 0.4], snapEntities, 10), {
    point: [0, 0], kind: "ORIGIN", distancePixels: Math.hypot(0.7, 0.4) * 10,
  });
  const endpointSnap = resolveSketchSnap([19.4, 0.2], snapEntities, 10);
  assert.deepEqual({ ...endpointSnap, distancePixels: undefined }, {
    point: [20, 0], kind: "ENDPOINT", distancePixels: undefined, entityId: "line-a", subElement: "END",
  });
  assert.ok(Math.abs(endpointSnap.distancePixels - Math.hypot(0.6, 0.2) * 10) < 1e-10);
  assert.equal(resolveSketchSnap([30.1, 24.9], [], 1)?.kind, "GRID");
  assert.equal(resolveSketchSnap([10.2, 0.6], snapEntities, 10)?.kind, "MIDPOINT");
  assert.equal(resolveSketchSnap([0.1, 0.1], snapEntities, 10, 10, 11, ["GRID"])?.kind, "GRID");
  assert.equal(resolveSketchSnap([19.9, 0.1], snapEntities, 10, 10, 11, ["MIDPOINT"]), undefined);
  const curvedSnapEntities = [
    { id: "circle-a", kind: "CIRCLE", role: "PROFILE", center: { x: 30, y: 30 }, radius: 10 },
    { id: "arc-a", kind: "ARC", role: "PROFILE", center: { x: 0, y: 30 }, radius: 10, startAngle: 0, endAngle: Math.PI },
    { id: "spline-a", kind: "SPLINE", role: "PROFILE", controlPoints: [{ x: 0, y: 50 }, { x: 10, y: 55 }, { x: 20, y: 50 }], degree: 2 },
  ];
  assert.equal(resolveSketchSnap([30.2, 29.9], curvedSnapEntities, 10)?.kind, "CENTER");
  assert.equal(resolveSketchSnap([39.8, 30.1], curvedSnapEntities, 10)?.kind, "CURVE");
  assert.equal(resolveSketchSnap([10.1, 30.1], curvedSnapEntities, 10)?.kind, "ENDPOINT");
  assert.equal(resolveSketchSnap([8, 52.4], curvedSnapEntities, 10)?.kind, "CURVE");
  assert.equal(selectionCaptureKind({ kind: "plane", id: "xy", plane: "XY" }), "DATUM_PLANE");
  assert.equal(selectionCaptureKind({ kind: "axis", id: "x", axis: "X" }), "DATUM_AXIS");
  assert.equal(selectionCaptureKind({ kind: "vertex", id: "v1", topologyId: 1 }), "POINT");
  assert.equal(allowsSelection({ ...DEFAULT_CAPTURE_SETTINGS, selection: ["POINT"] },
    { kind: "vertex", id: "v1", topologyId: 1 }), true);
  assert.equal(allowsSelection({ ...DEFAULT_CAPTURE_SETTINGS, selection: ["POINT"] },
    { kind: "face", id: "f1", topologyId: 1 }), false);

  const THREE = await import(require.resolve("three"));
  const index = new SelectionIndex();
  const near = new THREE.Mesh(new THREE.PlaneGeometry(10, 10)); near.position.z = -1; near.updateMatrixWorld();
  const far = new THREE.Mesh(new THREE.PlaneGeometry(10, 10)); far.position.z = -2; far.updateMatrixWorld();
  index.registerPick(near, () => ({ kind: "face", id: "near", topologyId: 1 }));
  index.registerPick(far, () => ({ kind: "vertex", id: "far", topologyId: 2 }));
  assert.equal(index.pick(new THREE.Raycaster(new THREE.Vector3(), new THREE.Vector3(0, 0, -1)),
    (selection) => selection.kind === "vertex")?.id, "far");
  const childObject = new THREE.Group();
  index.register({ kind: "sketch", id: "sketch-1", treeNodeId: "document/body/sketch:1" }, childObject);
  assert.equal(index.objectsFor({ kind: "face", id: "face-1", topologyId: 1,
    treeNodeId: "document/body" }).includes(childObject), false);
  assert.equal(index.objectsFor({ kind: "tree", id: "body", treeNodeId: "document/body",
    expandTreeDescendants: true }).includes(childObject), true);
  const relatedLine = new THREE.Group();
  const constraintSelection = { kind: "sketch-constraint", id: "constraint-1", featureId: "sketch-1",
    constraintId: "length-1", constraintType: "LENGTH" };
  index.register(constraintSelection, childObject);
  index.associate(constraintSelection, relatedLine);
  assert.equal(index.objectsFor(constraintSelection).includes(relatedLine), true);
  near.geometry.dispose(); far.geometry.dispose();

  assert.deepEqual(resolveTreeSelection(["a", "b", "c", "d"], [], "b", undefined, {}), ["b"]);
  assert.deepEqual(resolveTreeSelection(["a", "b", "c", "d"], ["b"], "d", "b", { ctrl: true }), ["b", "d"]);
  assert.deepEqual(resolveTreeSelection(["a", "b", "c", "d"], ["b", "d"], "b", "d", { ctrl: true }), ["d"]);
  assert.deepEqual(resolveTreeSelection(["a", "b", "c", "d"], ["a"], "d", "b", { shift: true }), ["b", "c", "d"]);
  assert.deepEqual(resolveTreeSelection(["a", "b", "c", "d"], ["a"], "d", "b", { ctrl: true, shift: true }), ["a", "b", "c", "d"]);
  assert.equal(closestTreeKey([{ key: "document", children: [{ key: "document/body" }] }],
    "document/body/imported-face:42"), "document/body");
  assert.equal(resultBodyFeatureTreeNode({ id: "document", kind: "PART", name: "Part", children: [
    { id: "document/body", kind: "BODY", name: "PartBody", children: [
      { id: "document/body/import:1", kind: "IMPORT", name: "Import" },
      { id: "document/body/pad:2", kind: "PAD", name: "Extrude" },
    ] },
  ] }, "document/body"), "document/body/pad:2");

  let additivePick = false;
  const selector = new SelectionController((_x, _y, additive) => { additivePick = additive; }, () => {}, () => {});
  const controlPointer = { ...pointer(5, 5, "down"), state: { ...pointer(5, 5, "down").state,
    modifiers: { ctrl: true, shift: false, alt: false, meta: false } } };
  selector.pointerDown(controlPointer);
  selector.pointerUp({ ...controlPointer, phase: "up" });
  assert.equal(additivePick, true);

  let dimensionDragUpdates=0,dimensionDragFinished=0,dimensionEdits=0;
  viewport.beginDimensionDrag=()=>true;
  viewport.updateDimensionDrag=()=>{dimensionDragUpdates+=1;};
  viewport.finishDimensionDrag=()=>{dimensionDragFinished+=1;};
  viewport.editDimensionAt=()=>{dimensionEdits+=1;return true;};
  const dimensionSelector=new (await server.ssrLoadModule("/src/cad/tool/cad-tool.ts")).SelectTool();
  const dragDown=pointer(8,8,"down");
  assert.equal(dimensionSelector.pointerDown(dragDown,context),"capture");
  dimensionSelector.pointerMove({...pointer(12,12),pointerId:dragDown.pointerId},context);
  assert.equal(dimensionSelector.pointerUp({...pointer(12,12,"up"),pointerId:dragDown.pointerId},context),"release-capture");
  assert.equal(dimensionDragUpdates,1);assert.equal(dimensionDragFinished,1);
  assert.equal(dimensionSelector.pointerDown({...pointer(8,8,"down"),originalEvent:{detail:2}},context),"consumed");
  assert.equal(dimensionEdits,1);
  viewport.beginDimensionDrag=()=>false;
  viewport.editDimensionAt=()=>false;

  const manager = new ToolManager(context);
  manager.register(new (await server.ssrLoadModule("/src/cad/tool/cad-tool.ts")).SelectTool());
  manager.register(new LineSketchTool());
  manager.activate("sketch.line");
  assert.equal(manager.keyDown({ key: "Escape" }), "consumed");
  assert.equal(manager.activeToolID, "select");
  const project = ([x, y]) => ({ x: x * 10, y: y * 10 });
  assert.deepEqual(resolveSketchReference({ x: 3, y: 2 }, snapEntities, project, "POINT"),
    { target: "SKETCH_ORIGIN", subElement: "POINT" });
  assert.deepEqual(resolveSketchReference({ x: 198, y: 1 }, snapEntities, project, "POINT"),
    { target: "ENTITY", entityId: "line-a", subElement: "END" });
  assert.deepEqual(resolveSketchReference({ x: 400, y: 2 }, [], project, "LINE"),
    { target: "SKETCH_X_AXIS", subElement: "DIRECTION" });
  assert.deepEqual(resolveSketchReference({ x: 2, y: 400 }, [], project, "LINE"),
    { target: "SKETCH_Y_AXIS", subElement: "DIRECTION" });
  const constraintEntities = [
    { id: "point-a", kind: "POINT", role: "PROFILE", point: { x: 0, y: 0 } },
    { id: "point-b", kind: "POINT", role: "PROFILE", point: { x: 20, y: 10 } },
    { id: "line-a", kind: "LINE", role: "PROFILE", start: { x: 0, y: 0 }, end: { x: 20, y: 0 } },
    { id: "line-b", kind: "LINE", role: "PROFILE", start: { x: 0, y: 0 }, end: { x: 0, y: 20 } },
    { id: "circle-a", kind: "CIRCLE", role: "PROFILE", center: { x: 30, y: 20 }, radius: 8 },
    { id: "arc-a", kind: "ARC", role: "PROFILE", center: { x: 50, y: 20 }, radius: 8, startAngle: 0, endAngle: Math.PI },
    { id: "spline-a", kind: "SPLINE", role: "PROFILE", controlPoints: [{ x: 0, y: 30 }, { x: 10, y: 35 }, { x: 20, y: 30 }], degree: 2 },
  ];
  const ref = (entityId, subElement = "WHOLE") => ({ target: "ENTITY", entityId, subElement });
  const fixtures = {
    COINCIDENT: [ref("point-a", "POINT"), ref("line-a", "START")],
    PARALLEL: [ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")], FIXED: [ref("line-a")],
    FIXED_POINT: [ref("point-a", "POINT")], HORIZONTAL: [ref("line-a", "DIRECTION")],
    VERTICAL: [ref("line-b", "DIRECTION")], PERPENDICULAR: [ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")],
    TANGENT: [ref("line-a"), ref("circle-a")], EQUAL: [ref("line-a"), ref("line-b")],
    DISTANCE: [ref("point-a", "POINT"), ref("point-b", "POINT")], LENGTH: [ref("line-a", "DIRECTION")],
    RADIUS: [ref("circle-a")], DIAMETER: [ref("arc-a")], ANGLE: [ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")],
    CONCENTRIC: [ref("circle-a"), ref("arc-a")], POINT_ON_OBJECT: [ref("point-a", "POINT"), ref("line-a")],
    MIDPOINT: [ref("point-a", "POINT"), ref("line-a", "DIRECTION")],
  };
  assert.equal(TOOLBAR_CONSTRAINT_KINDS.length, 16);
  for (const [kind, refs] of Object.entries(fixtures)) {
    const definition = constraintDefinition(kind);
    assert.equal(definition.picks.length, refs.length, `${kind} selection count`);
    const layout = buildSketchConstraintLayout({ id: `constraint-${kind}`, kind, references: refs,
      fixedPoint: kind === "FIXED_POINT" ? { x: 0, y: 0 } : undefined,
      value: definition.unit ? 12.5 : undefined, unit: definition.unit }, constraintEntities);
    assert.ok(layout.anchors.length > 0 || layout.segments.length > 0, `${kind} must have a viewport glyph`);
    if (definition.dimension !== "none") {
      assert.ok(layout.segments.length >= 3, `${kind} must have leaders and arrow geometry`);
      assert.ok(layout.label?.text, `${kind} must have a dimension label`);
    }
  }
  const placedLayout = buildSketchConstraintLayout({ id: "placed-length", kind: "LENGTH",
    references: [ref("line-a", "DIRECTION")], value: 20, unit: "mm", labelPosition: { x: 10, y: -12 } }, constraintEntities);
  assert.deepEqual(placedLayout.label?.position, [10, -12]);
  const identityProject = ([x, y]) => ({ x, y });
  assert.equal(resolveSketchReference({ x: 10, y: 32 }, constraintEntities, identityProject, "SOLVER_CURVE"),
    null, "tangent/point-on-object must not pick splines");
  assert.equal(resolveSketchReference({ x: 38, y: 20 }, constraintEntities, identityProject, "EQUAL_CURVE", 12, 110,
    ref("line-a")), null, "equal line must reject a circular second element");
  assert.equal(resolveSketchReference({ x: 38, y: 20 }, constraintEntities, identityProject, "EQUAL_CURVE", 12, 110,
    ref("circle-a"))?.entityId, "circle-a");
  assert.equal(resolveSketchReference({ x: 10, y: 0 }, constraintEntities, identityProject, "TANGENT_CURVE", 12, 110,
    ref("line-b")), null, "line-line tangent must be rejected while selecting");
  assert.equal(resolveSketchReference({ x: 38, y: 20 }, constraintEntities, identityProject, "TANGENT_CURVE", 12, 110,
    ref("line-a"))?.entityId, "circle-a");
  const pointMaterial = new CadShaderLibrary().createMaterial("cad.point");
  assert.match(pointMaterial.fragmentShader, /abs\(p\.x - p\.y\)/);
  pointMaterial.dispose();
  const constraintMaterial = new CadShaderLibrary().createMaterial("cad.constraint.glyph");
  assert.match(constraintMaterial.fragmentShader, /uniform float uGlyph/);
  assert.equal(constraintMaterial.depthTest, false);
  assert.equal(constraintMaterial.depthWrite, false);
  constraintMaterial.dispose();
  const highlightLine = makeOcclusionVisibleHighlightLine([
    new THREE.Vector3(0, 0, 0), new THREE.Vector3(10, 0, 0),
  ], 0xffa62b, 5);
  assert.equal(highlightLine.material.depthTest, false);
  assert.equal(highlightLine.material.depthWrite, false);
  assert.equal(highlightLine.material.linewidth, 5);
  highlightLine.geometry.dispose(); highlightLine.material.dispose();
  assert.equal(defaultDocumentName("PART", [{ name: "Part1" }, { name: "part3" }]), "Part2");
  assert.equal(defaultDocumentName("PRODUCT", [{ name: "Product1" }, { name: "Product2" }]), "Product3");
  const persistedLine = { id: "line-1", featureId: "sketch-1", kind: "POLYLINE",
    semantic: "SKETCH_CURVE", role: "PROFILE", positions: [[0, 0, 0], [2, 0, 0]], selectable: true };
  assert.equal(visualType(persistedLine), "CURVE");
  assert.deepEqual(visualSelection(persistedLine, {
    documentId: "part-1", geometryKey: "geometry-1", occurrencePath: "instance-a",
    instanceId: "instance-a", treeNodeId: "product/instance-a/reference/body/sketch:sketch-1",
  }), {
    kind: "visual", id: "instance-a:sketch-1:line-1", visualType: "CURVE",
    featureId: "sketch-1", entityId: "line-1", role: "PROFILE",
    documentId: "part-1", geometryKey: "geometry-1", occurrencePath: "instance-a",
    instanceId: "instance-a", treeNodeId: "product/instance-a/reference/body/sketch:sketch-1",
  });
  const persistedConstraint = { id: "parallel-1", featureId: "sketch-1", kind: "LINE_SEGMENTS",
    semantic: "SKETCH_CONSTRAINT", entityType: "PARALLEL", positions: [[0, 0, 0], [0, 2, 0]], selectable: true };
  assert.equal(visualType(persistedConstraint), "CURVE");
  assert.deepEqual(visualSelection(persistedConstraint, {
    documentId: "part-1", geometryKey: "geometry-1", occurrencePath: "instance-a",
    instanceId: "instance-a", treeNodeId: "product/instance-a/reference/body/sketch:sketch-1/constraints/logical/constraint:parallel-1",
  }), {
    kind: "sketch-constraint", id: "instance-a:sketch-1:constraint:parallel-1",
    featureId: "sketch-1", constraintId: "parallel-1", constraintType: "PARALLEL",
    documentId: "part-1", geometryKey: "geometry-1", occurrencePath: "instance-a",
    instanceId: "instance-a", treeNodeId: "product/instance-a/reference/body/sketch:sketch-1/constraints/logical/constraint:parallel-1",
  });

  console.log("Sketch tool interaction tests passed.");
} finally {
  await server.close();
}
