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
  const { resolveSketchReference } = await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
  const { CadShaderLibrary } = await server.ssrLoadModule("/src/cad/rendering/shader/cad-shader-library.ts");
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
    showPolylinePreview: (points, closed) => previews.push({ points, closed }),
    showPointPreview: (point) => previews.push({ point }),
    clearToolPreview: () => previews.push("clear"),
    commitSketchOperations: (value) => operations.push(value),
    hasActiveSketch: () => true,
    sketchReferenceAt: () => references.shift() ?? null,
    showReferencePreview: (value, retained) => previews.push({ reference: value, retained }),
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
    state: { buttons: { left: false, middle: false, right: false } },
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
  assert.equal(prompts.some((value) => value.includes("第二个端点")), true);

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
  length.keyDown({ key: "2" }, context); length.keyDown({ key: "5" }, context); length.keyDown({ key: "Enter" }, context);
  assert.equal(operations[dimensionIndex][0].constraint.value, 25);
  assert.equal(operations[dimensionIndex][0].constraint.unit, "mm");

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
  const project = ([x, y]) => ({ x: x * 10, y: y * 10 });
  assert.deepEqual(resolveSketchReference({ x: 3, y: 2 }, snapEntities, project, "COINCIDENT"),
    { target: "SKETCH_ORIGIN", subElement: "POINT" });
  assert.deepEqual(resolveSketchReference({ x: 198, y: 1 }, snapEntities, project, "COINCIDENT"),
    { target: "ENTITY", entityId: "line-a", subElement: "END" });
  assert.deepEqual(resolveSketchReference({ x: 400, y: 2 }, [], project, "PARALLEL"),
    { target: "SKETCH_X_AXIS", subElement: "DIRECTION" });
  assert.deepEqual(resolveSketchReference({ x: 2, y: 400 }, [], project, "PARALLEL"),
    { target: "SKETCH_Y_AXIS", subElement: "DIRECTION" });
  const pointMaterial = new CadShaderLibrary().createMaterial("cad.point");
  assert.match(pointMaterial.fragmentShader, /abs\(p\.x - p\.y\)/);
  pointMaterial.dispose();
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
    instanceId: "instance-a", treeNodeId: "product/instance-a/reference/body/sketch:sketch-1/constraints/constraint:parallel-1",
  }), {
    kind: "sketch-constraint", id: "instance-a:sketch-1:constraint:parallel-1",
    featureId: "sketch-1", constraintId: "parallel-1", constraintType: "PARALLEL",
    documentId: "part-1", geometryKey: "geometry-1", occurrencePath: "instance-a",
    instanceId: "instance-a", treeNodeId: "product/instance-a/reference/body/sketch:sketch-1/constraints/constraint:parallel-1",
  });

  console.log("Sketch tool interaction tests passed.");
} finally {
  await server.close();
}
