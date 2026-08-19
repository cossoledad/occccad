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
    LineSketchTool,
    PointSketchTool,
    RectangleSketchTool,
  } = await server.ssrLoadModule("/src/cad/tool/cad-tool.ts");
  const { buildSketchRenderModel } = await server.ssrLoadModule("/src/cad/rendering/sketch-render-model.ts");
  const { visualSelection, visualType } = await server.ssrLoadModule("/src/cad/rendering/visualization-render-model.ts");
  const operations = [];
  const prompts = [];
  const previews = [];
  const references = [
    { target: "ENTITY", entityId: "line-a", subElement: "END" },
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
    showReferencePreview: (value) => previews.push(value),
    clearReferencePreview: () => previews.push("clear-reference"),
    setToolPrompt: (value) => prompts.push(value),
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
  assert.equal(previews.some((value) => value?.closed === true), true);

  const line = new LineSketchTool();
  line.activate(context);
  line.pointerDown(pointer(1, 1, "down"), context);
  line.pointerUp(pointer(1, 1, "up"), context);
  line.pointerMove(pointer(5, 5), context);
  line.pointerDown(pointer(5, 5, "down"), context);
  line.pointerUp(pointer(5, 5, "up"), context);
  assert.equal(operations[1][0].entity.kind, "LINE");

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
  assert.equal(previews.some((value) => value?.point?.[0] === 4), true);

  const coincident = new ConstraintSketchTool("sketch.constraint.coincident");
  coincident.activate(context);
  coincident.pointerDown(pointer(0, 0, "down"), context);
  coincident.pointerUp(pointer(0, 0, "up"), context);
  coincident.pointerDown(pointer(0, 0, "down"), context);
  coincident.pointerUp(pointer(0, 0, "up"), context);
  assert.equal(operations[3][0].constraint.kind, "COINCIDENT");
  assert.equal(prompts.some((value) => value.includes("第二个端点")), true);

  const renderModel = buildSketchRenderModel({
    id: "sketch-1", type: "SKETCH", sketch: { entities: [
      { id: "point-1", kind: "POINT", role: "PROFILE", point: { x: 4, y: 5 } },
      { id: "line-1", kind: "LINE", role: "PROFILE", start: { x: 0, y: 0 }, end: { x: 2, y: 0 } },
    ] },
  });
  assert.deepEqual(renderModel.profilePoints, [[4, 5]]);
  assert.deepEqual(renderModel.profileLines, [[0, 0], [2, 0]]);
  assert.deepEqual(renderModel.endpoints, [[0, 0], [2, 0]]);

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
