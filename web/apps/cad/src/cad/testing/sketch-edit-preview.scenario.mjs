import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { buildSketchEditPreview, mirrorSketchPreviewEntity } = await server.ssrLoadModule("/src/cad/sketch/sketch-edit-preview.ts");
  const { evaluateCanonicalSpline } = await server.ssrLoadModule("/src/cad/sketch/sketch-geometry.ts");
  const close = (a, b, tolerance = 1e-9) => assert(Math.abs(a - b) < tolerance, `${a} != ${b}`);
  const pointClose = (a, b) => { close(a.x, b.x); close(a.y, b.y); };
  const ref = (id, subElement) => ({ target: "ENTITY", entityId: id, subElement });
  const line = (id, start, end) => ({ id, kind: "LINE", role: "PROFILE", start: { x: start[0], y: start[1] }, end: { x: end[0], y: end[1] } });
  const arc = (id, center, radius, startAngle, endAngle) => ({ id, kind: "ARC", role: "PROFILE", center: { x: center[0], y: center[1] }, radius, startAngle, endAngle });
  const curveEnd = (e, end) => e.kind === "LINE" ? e[end ? "end" : "start"] : { x: e.center.x + e.radius * Math.cos(e[end ? "endAngle" : "startAngle"]), y: e.center.y + e.radius * Math.sin(e[end ? "endAngle" : "startAngle"]) };
  const polar = (e, parameter) => {
    if (e.kind === "ARC") return { x: e.center.x + e.radius * Math.cos(parameter), y: e.center.y + e.radius * Math.sin(parameter) };
    const x = e.majorRadius * Math.cos(parameter), y = e.minorRadius * Math.sin(parameter), c = Math.cos(e.rotation), s = Math.sin(e.rotation);
    return { x: e.center.x + x * c - y * s, y: e.center.y + x * s + y * c };
  };
  // Independent Householder reflection oracle, rather than repeating projection.
  const axis = { start: [2, 3], end: [6, 6] }, normal = [-0.6, 0.8];
  const reflectedPoint = p => { const d = (p.x - 2) * normal[0] + (p.y - 3) * normal[1]; return { x: p.x - 2 * d * normal[0], y: p.y - 2 * d * normal[1] }; };
  const originals = [
    { id: "point", kind: "POINT", role: "PROFILE", point: { x: 3, y: 7 } },
    line("line", [-3, 1], [8, -2]),
    { id: "circle", kind: "CIRCLE", role: "CONSTRUCTION", center: { x: 8, y: 2 }, radius: 4 },
    arc("arc", [4, 2], 3, 5.9, 7.1),
    { id: "ellipse", kind: "ELLIPSE", role: "PROFILE", center: { x: 2, y: 7 }, majorRadius: 5, minorRadius: 2, rotation: 0.4 },
    { id: "earc", kind: "ELLIPTICAL_ARC", role: "CONSTRUCTION", center: { x: 2, y: 7 }, majorRadius: 5, minorRadius: 2, rotation: 0.4, startAngle: 0.3, endAngle: -1.8 },
    { id: "fit", kind: "SPLINE", role: "PROFILE", mode: "FIT", controlPoints: [{ x: 1, y: 2 }, { x: 3, y: 8 }, { x: 5, y: 2 }], closed: true },
    { id: "control", kind: "SPLINE", role: "PROFILE", mode: "CONTROL", poles: [{ x: 1, y: 2 }, { x: 3, y: 8 }, { x: 5, y: 2 }], degree: 2, knots: [0, 1], multiplicities: [3, 3], weights: [1, 0.6, 1], parameterStart: 0, parameterEnd: 1 }
  ];
  const baseline = JSON.stringify(originals), operation = { type: "MIRROR_ENTITIES", operationId: "mirror", entityIds: originals.map(e => e.id), axis: ref("axis", "DIRECTION"), mirrorMode: "LINKED" };
  const mirror = buildSketchEditPreview({ entities: originals, operation, axisPoints: axis });
  assert.equal(mirror.status, "APPROXIMATE"); assert.equal(mirror.entities.length, originals.length); assert.deepEqual(mirror.operation, operation); assert.equal(JSON.stringify(originals), baseline);
  for (let i = 0; i < originals.length; ++i) {
    const source = originals[i], target = mirror.entities[i]; assert.equal(target.role, source.role);
    for (const key of ["point", "start", "end", "center"]) if (source[key]) pointClose(target[key], reflectedPoint(source[key]));
    for (const key of ["controlPoints", "poles"]) if (source[key]) for (let j = 0; j < source[key].length; ++j) pointClose(target[key][j], reflectedPoint(source[key][j]));
    if (source.startAngle !== undefined) close(target.endAngle - target.startAngle, source.startAngle - source.endAngle);
    if (source.kind === "ARC" || source.kind === "ELLIPTICAL_ARC") for (const t of [0, .23, .7, 1]) pointClose(polar(target, target.startAngle + (target.endAngle - target.startAngle) * t), reflectedPoint(polar(source, source.startAngle + (source.endAngle - source.startAngle) * t)));
  }
  const targetSpline = mirror.entities.at(-1);
  for (const t of [0, .2, .5, .9, 1]) { const p = evaluateCanonicalSpline(originals.at(-1), t), q = evaluateCanonicalSpline(targetSpline, t); pointClose({ x: q[0], y: q[1] }, reflectedPoint({ x: p[0], y: p[1] })); }
  assert.throws(() => mirrorSketchPreviewEntity(originals[0], { start: [0, 0], end: [0, 0] }));

  const first = line("first", [0, 0], [10, 0]), second = line("second", [0, 0], [0, 8]);
  const corner = { type: "FILLET_ENTITIES", operationId: "corner", entityIds: ["first", "second"], firstReference: ref("first", "START"), secondReference: ref("second", "START"), point: { x: 0, y: 0 }, value: 1, trimMode: "TRIM" };
  const fillet = buildSketchEditPreview({ entities: [first, second], operation: corner });
  assert.equal(fillet.status, "APPROXIMATE"); assert.deepEqual(fillet.replacedEntityIds, ["first", "second"]); assert.equal(fillet.entities[2].kind, "ARC");
  assert.equal(fillet.dimensions[0].kind,'RADIUS');assert.equal(fillet.dimensions[0].references[0].entityId,fillet.entities[2].id);close(fillet.dimensions[0].value,1);
  pointClose(fillet.entities[2].center, { x: 1, y: 1 }); pointClose(fillet.entities[0].start, { x: 1, y: 0 }); pointClose(fillet.entities[1].start, { x: 0, y: 1 });
  pointClose(curveEnd(fillet.entities[2], false), fillet.entities[0].start); pointClose(curveEnd(fillet.entities[2], true), fillet.entities[1].start);
  close(Math.abs(fillet.entities[2].endAngle - fillet.entities[2].startAngle), Math.PI / 2);
  const distantClick = buildSketchEditPreview({ entities: [first, second], operation: { ...corner, point: { x: 2, y: 1 } } });
  close(Math.abs(distantClick.entities[2].endAngle - distantClick.entities[2].startAngle), Math.PI / 2);
  pointClose(distantClick.entities[2].center, { x: 1, y: 1 }); // click chooses the support side, not a complementary 270 degree arc
  assert.equal(buildSketchEditPreview({ entities: [first, second], operation: { ...corner, value: 9 } }).status, "UNAVAILABLE");
  const kept = buildSketchEditPreview({ entities: [first, second], operation: { ...corner, trimMode: "KEEP" } }); assert(kept.entities.every(e => e.role === "CONSTRUCTION")); assert.deepEqual(kept.replacedEntityIds, []);

  const diameter = line("diameter", [-5, 0], [5, 0]), semicircle = arc("semicircle", [0, 0], 5, 0, Math.PI);
  const la = buildSketchEditPreview({ entities: [diameter, semicircle], operation: { ...corner, entityIds: ["diameter", "semicircle"], firstReference: ref("diameter", "END"), secondReference: ref("semicircle", "START"), point: { x: 5, y: 0 } } });
  assert.equal(la.status, "APPROXIMATE"); const lc = la.entities[2]; close(lc.center.x, Math.sqrt(15)); close(lc.center.y, 1);
  pointClose(curveEnd(lc, false), la.entities[0].end); pointClose(curveEnd(lc, true), curveEnd(la.entities[1], false));
  close(Math.hypot(lc.center.x, lc.center.y), 4); close(Math.hypot(curveEnd(lc, true).x, curveEnd(lc, true).y), 5);
  const alpha = Math.acos(.6), left = arc("left", [-3, 0], 5, -alpha, alpha), right = arc("right", [3, 0], 5, Math.PI - alpha, Math.PI + alpha);
  const aa = buildSketchEditPreview({ entities: [left, right], operation: { ...corner, entityIds: ["left", "right"], firstReference: ref("left", "END"), secondReference: ref("right", "START"), point: { x: 0, y: 4 }, value: .5 } });
  assert.equal(aa.status, "APPROXIMATE"); close(aa.entities[2].center.x, 0); close(aa.entities[2].center.y, Math.sqrt(4.5 ** 2 - 9));
  pointClose(curveEnd(aa.entities[2], false), curveEnd(aa.entities[0], true)); pointClose(curveEnd(aa.entities[2], true), curveEnd(aa.entities[1], false));

  for (const [mode, secondLength, angle] of [["EQUAL", 1, undefined], ["TWO_LENGTHS", 2, undefined], ["LENGTH_ANGLE", undefined, 30]]) {
    const op = { ...corner, type: "CHAMFER_ENTITIES", chamferMode: mode, chamferFirst: 1, chamferSecond: secondLength, chamferAngle: angle };
    const preview = buildSketchEditPreview({ entities: [first, second], operation: op }); assert.equal(preview.status, "APPROXIMATE");
    assert.equal(preview.dimensions[0].kind,'LENGTH');assert.equal(preview.dimensions[0].references[0].entityId,preview.dimensionEntities[0].id);close(preview.dimensions[0].value,1);
    assert.equal(preview.dimensions.length,mode==='EQUAL'?1:2);if(mode==='LENGTH_ANGLE')assert.equal(preview.dimensions[1].kind,'ANGLE');
    const bridge = preview.entities[2]; assert.equal(bridge.kind, "LINE");
    const firstContact = mode === "LENGTH_ANGLE" ? bridge.end : bridge.start, secondContact = mode === "LENGTH_ANGLE" ? bridge.start : bridge.end;
    pointClose(firstContact, { x: 1, y: 0 }); close(secondContact.x, 0); close(secondContact.y, mode === "LENGTH_ANGLE" ? Math.tan(Math.PI / 6) : secondLength);
  }
  assert.equal(buildSketchEditPreview({ entities: [first, second], operation: { ...corner, type: "CHAMFER_ENTITIES", chamferMode: "LENGTH_ANGLE", chamferFirst: 1, chamferAngle: 120 } }).status, "UNAVAILABLE");

  const boundaries = [line("b2", [2, -1], [2, 1]), line("b8", [8, -1], [8, 1])];
  const quick = { type: "QUICK_TRIM", operationId: "quick", entityIds: ["first"], boundaryIds: ["b2", "b8"], hitParameter: .5, trimMode: "DELETE_HIT" };
  const deletion = buildSketchEditPreview({ entities: [first, ...boundaries], operation: quick }); assert.equal(deletion.status, "APPROXIMATE"); assert.deepEqual(deletion.hitIntervals, [[.2, .8]]); assert.deepEqual(deletion.retainedIntervals, [[0, .2], [.8, 1]]); pointClose(deletion.hitEntities[0].start, { x: 2, y: 0 }); pointClose(deletion.hitEntities[0].end, { x: 8, y: 0 });
  const retention = buildSketchEditPreview({ entities: [first, ...boundaries], operation: { ...quick, trimMode: "KEEP_HIT" } }); assert.deepEqual(retention.retainedIntervals, [[.2, .8]]);
  const broken = buildSketchEditPreview({ entities: [first, ...boundaries], operation: { ...quick, trimMode: "BREAK" } }); assert.deepEqual(broken.retainedIntervals, [[0, .2], [.2, .8], [.8, 1]]);
  const ca = { id: "ca", kind: "CIRCLE", role: "PROFILE", center: { x: 0, y: 0 }, radius: 5 }, cb = { ...ca, id: "cb", center: { x: 6, y: 0 } };
  const cyclic = buildSketchEditPreview({ entities: [ca, cb], operation: { ...quick, entityIds: ["ca"], boundaryIds: ["cb"], hitParameter: 0 } });
  assert.equal(cyclic.status, "APPROXIMATE"); assert.equal(cyclic.hitIntervals.length, 2); assert.equal(cyclic.retainedIntervals.length, 1); close(cyclic.retainedIntervals[0][0], alpha / (2 * Math.PI)); close(cyclic.retainedIntervals[0][1], 1 - alpha / (2 * Math.PI));
  const unsupported = buildSketchEditPreview({ entities: [originals[4], first], operation: { ...quick, entityIds: ["ellipse"], boundaryIds: ["first"] } }); assert.equal(unsupported.status, "UNAVAILABLE"); assert.equal(unsupported.entities.length, 0);
  const exactEllipse = buildSketchEditPreview({ entities: [originals[4]], operation: { ...quick, entityIds: ["ellipse"], boundaryIds: ["external"] }, intersectionParameters: [.2, .8] }); assert.equal(exactEllipse.status, "APPROXIMATE"); assert(exactEllipse.entities.every(e => e.kind === "ELLIPTICAL_ARC"));
  const spline = originals.at(-1), trim = buildSketchEditPreview({ entities: [spline], operation: { type: "TRIM_ENTITY", operationId: "spline-trim", entityIds: [spline.id], parameters: [.2, .8] } }); assert.equal(trim.status, "APPROXIMATE"); assert.deepEqual(trim.entities[0].poles, spline.poles); close(trim.entities[0].parameterStart, .2); close(trim.entities[0].parameterEnd, .8);
  assert.equal(JSON.stringify(originals), baseline); assert.deepEqual(mirror.operation, operation); assert.notEqual(mirror.operation, operation, "candidate definitions are frozen copies");
  console.log("sketch edit preview: true-curve mirror, fillet/chamfer branches, trim intent and immutable display candidates passed");
  // Mock cannot turn a numerical approximation into a production candidate.
  const previousWindow = globalThis.window;
  globalThis.window = { setTimeout };
  try {
    const { mockApi } = await server.ssrLoadModule("/src/api/mock-api.ts");
    const document = await mockApi.createDocument("PART", "preview-readonly");
    const created = await mockApi.createSketch(document.document.id, { plane: "XY" });
    const sketchId = created.part.features.find(feature => feature.sketch)?.id;
    const beforeMock = await mockApi.getDocument(document.document.id);
    const beforeMockHistory = await mockApi.getHistory(document.document.id);
    await assert.rejects(mockApi.previewCommand(document.document.id, { type: "EDIT_SKETCH", sketchId, operations: [] }), /Mock 不支持权威草图编辑候选/);
    assert.deepEqual(await mockApi.getDocument(document.document.id), beforeMock);
    assert.deepEqual(await mockApi.getHistory(document.document.id), beforeMockHistory);
  } finally { globalThis.window = previousWindow; }

} finally { await server.close(); }
