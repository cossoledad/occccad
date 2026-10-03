import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { measureSketchDimension, buildSketchConstraintLayout } = await server.ssrLoadModule("/src/cad/sketch/sketch-constraint-layout.ts");
  const { constraintDefinition } = await server.ssrLoadModule("/src/cad/sketch/sketch-constraint-definition.ts");
  const { resolveSketchReference } = await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
  const ref = (entityId, subElement = "WHOLE") => ({ target: "ENTITY", entityId, subElement });
  const line = (id, start, end) => ({ id, kind: "LINE", role: "PROFILE", start: { x: start[0], y: start[1] }, end: { x: end[0], y: end[1] } });
  const first = line("a", [10, 20], [40, 60]);
  // The segments do not overlap. Their supporting lines are five mm apart.
  const second = line("b", [84, 127], [90, 135]);
  const refs = [ref("a"), ref("b")];
  const entities = [first, second];
  assert.equal(measureSketchDimension("DISTANCE", refs, entities), 5);
  assert.equal(measureSketchDimension("DISTANCE", [...refs].reverse(), entities), 5);
  const reversed = [first, { ...second, start: second.end, end: second.start }];
  assert.equal(measureSketchDimension("DISTANCE", refs, reversed), 5);
  const scale = 0.01;
  const scaled = entities.map(entity => ({ ...entity, start: { x: entity.start.x * scale, y: entity.start.y * scale }, end: { x: entity.end.x * scale, y: entity.end.y * scale } }));
  assert.ok(Math.abs(measureSketchDimension("DISTANCE", refs, scaled) - 0.05) < 1e-12);
  const coincident = [first, line("b", [100, 140], [106, 148])];
  assert.equal(measureSketchDimension("DISTANCE", refs, coincident), 0);
  assert.equal(measureSketchDimension("DISTANCE", refs, [first, line("b", [2, 0], [3, 0])]), undefined);
  assert.equal(measureSketchDimension("DISTANCE", refs, [first, line("b", [2, 0], [2, 0])]), undefined);
  assert.equal(measureSketchDimension("DISTANCE", [ref("a", "START"), ref("a", "END")], entities), 50);
  assert.equal(measureSketchDimension("DISTANCE", [ref("a", "START"), ref("b")], entities), 5);
  const fitEntities = [{ id: "fit", kind: "SPLINE", role: "PROFILE", controlPoints: [{ x: 3, y: 4 }, { x: 8, y: 1 }, { x: 10, y: 0 }] }];
  assert.equal(measureSketchDimension("DISTANCE", [{ ...ref("fit", "CONTROL"), controlPointIndex: 0 },
    { target: "SKETCH_ORIGIN", subElement: "POINT" }], fitEntities), 5);
  const layout = buildSketchConstraintLayout({ id: "spacing", kind: "DISTANCE", references: refs, value: 5, unit: "mm" }, entities);
  assert.equal(layout.segments.length, 7);
  const [firstWitness, secondWitness] = layout.segments;
  const delta = [secondWitness[0][0] - firstWitness[0][0], secondWitness[0][1] - firstWitness[0][1]];
  assert.ok(Math.abs(delta[0] * 30 + delta[1] * 40) < 1e-10, "witnesses join along the common normal");
  assert.ok(Math.abs(Math.hypot(...delta) - 5) < 1e-10);
  const zeroLayout = buildSketchConstraintLayout({ id: "zero", kind: "DISTANCE", references: refs, value: 0, unit: "mm" }, coincident);
  assert.equal(zeroLayout.label.text, "0");
  assert.ok(zeroLayout.segments.flat().every(point => point.every(Number.isFinite)));
  assert.deepEqual(constraintDefinition("DISTANCE").picks, ["LINEAR_DIMENSION", "LINEAR_DIMENSION"]);
  const picked = resolveSketchReference({ x: 25, y: 40 }, entities, point => ({ x: point[0], y: point[1] }), "LINEAR_DIMENSION", 2, 0);
  assert.deepEqual(picked, ref("a"));
  console.log("sketch line distance: supporting-line measurement, reversal, zero, layout and picking passed");
} finally { await server.close(); }
