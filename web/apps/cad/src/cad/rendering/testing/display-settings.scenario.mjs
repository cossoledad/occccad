import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const THREE = await server.ssrLoadModule("three");
  const { applySolidRenderMode } = await server.ssrLoadModule("/src/cad/rendering/solid-render-mode.ts");
  const { normalizeReferenceVisibility, normalizeRenderMode, referenceCategory } =
    await server.ssrLoadModule("/src/cad/rendering/display-settings.ts");
  const { raycastDatumAxis } = await server.ssrLoadModule("/src/cad/interaction/datum-axis-picking.ts");
  const { SelectionIndex } = await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
  const { datumAxisHitAccepted } = await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const { makeSketchOverlayLine } = await server.ssrLoadModule("/src/cad/rendering/interaction-highlight.ts");
  const { updateScreenLines, screenSpaceEndpoint } = await server.ssrLoadModule("/src/cad/rendering/screen-space-lines.ts");
  const { CadMaterialFactory } = await server.ssrLoadModule("/src/cad/rendering/cad-material-factory.ts");

  assert.deepEqual(normalizeReferenceVisibility(undefined), { planes:true, axes:true, coordinateSystems:true });
  assert.deepEqual(normalizeReferenceVisibility({axes:false}), { planes:true, axes:false, coordinateSystems:true });
  assert.equal(normalizeRenderMode("obsolete"), "default");
  assert.equal(referenceCategory("axis", "DATUM"), "axes");
  assert.equal(referenceCategory("axis", "X"), "coordinateSystems");
  assert.equal(referenceCategory("plane"), "planes");

  const factory = new CadMaterialFactory(null);
  const mesh = new THREE.Mesh(new THREE.BoxGeometry(), factory.surface());
  const edges = new THREE.LineSegments(new THREE.EdgesGeometry(mesh.geometry), new THREE.LineBasicMaterial());
  const points = new THREE.Points(mesh.geometry, new THREE.PointsMaterial());
  const solid = new THREE.Group(); solid.add(mesh, edges, points);
  assert.ok(mesh.material.isMeshStandardMaterial);
  const originalColor = mesh.material.color.getHex();
  factory.setSelected(mesh, true); factory.setSelected(mesh, false);
  assert.equal(mesh.material.color.getHex(), originalColor);
  for (const [mode, expected] of [["smooth",[true,false,false]],["wireframe",[false,true,false]],["default",[true,true,true]]]) {
    solid.visible = false;
    applySolidRenderMode(solid, mode);
    assert.equal(solid.visible, false, "mode must preserve tree hiding");
    assert.deepEqual(solid.children.map(object => object.visible), expected);
    assert.equal(edges.material.depthTest, mode !== "wireframe");
  }

  solid.visible = true;
  const axis = new THREE.Line(new THREE.BufferGeometry().setFromPoints([
    new THREE.Vector3(-1,0,-2), new THREE.Vector3(1,0,-2),
  ]), new THREE.LineBasicMaterial());
  const ray = new THREE.Raycaster(new THREE.Vector3(0,0,5), new THREE.Vector3(0,0,-1));
  ray.params.Line.threshold = .1;
  const hit = ray.intersectObject(axis)[0];
  assert.ok(hit); assert.equal(hit.distanceToRay, undefined, "regression: ordinary Line omits distanceToRay");
  axis.raycast = raycastDatumAxis;
  const axisParent = new THREE.Group(); axisParent.add(axis);
  for (const scale of [.00001, 1, 100000]) {
    axisParent.scale.setScalar(scale); axisParent.updateMatrixWorld(true);
    const scaledRay = new THREE.Raycaster(new THREE.Vector3(0,.01*scale,5*scale),new THREE.Vector3(0,0,-1));
    scaledRay.params.Line.threshold = .02*scale;
    assert.equal(scaledRay.intersectObject(axis).length, 1, "parent scaling must preserve the pixel hit area");
    scaledRay.params.Line.threshold = .005*scale;
    assert.equal(scaledRay.intersectObject(axis).length, 0);
  }
  axisParent.scale.setScalar(1); axisParent.updateMatrixWorld(true);
  const index = new SelectionIndex();
  index.registerPick(mesh, () => ({kind:"solid",id:"body"}));
  index.registerPick(axis, intersection => datumAxisHitAccepted(ray.ray.distanceToPoint(intersection.point), .02)
    ? {kind:"axis",id:"axis"} : null, 200, 10);
  assert.equal(index.pick(ray).id, "axis", "axis behind a solid stays selectable");
  axis.visible = false;
  assert.equal(index.pick(ray).id, "body", "hidden axes do not intercept picking");

  const camera = new THREE.OrthographicCamera(-10,10,10,-10,.001,1000);
  camera.position.set(0,0,20); camera.lookAt(0,0,0); camera.updateMatrixWorld();
  const dashed = makeSketchOverlayLine([new THREE.Vector3(0,0,0),new THREE.Vector3(1,0,0)], 0xffffff, 2, true);
  const parent = new THREE.Group(); parent.add(dashed); parent.scale.setScalar(3); parent.rotation.z = .3;
  for (const zoom of [.001,1,10000]) {
    camera.zoom = zoom; camera.updateProjectionMatrix();
    updateScreenLines(parent, camera, 800, 600);
    const a = new THREE.Vector3().applyMatrix4(dashed.matrixWorld).project(camera);
    const b = new THREE.Vector3(1,0,0).applyMatrix4(dashed.matrixWorld).project(camera);
    const pixelLength = Math.hypot((b.x-a.x)*400,(b.y-a.y)*300);
    const distance = dashed.geometry.getAttribute("instanceDistanceEnd").getX(0);
    assert.ok(Math.abs(distance-pixelLength) < pixelLength*1e-6);
    assert.equal(dashed.material.dashSize, 7); assert.equal(dashed.material.gapSize, 4);
    const anchor = new THREE.Vector3(), toward = new THREE.Vector3(3,2,0);
    const end = screenSpaceEndpoint(anchor,toward,camera,800,600,9).project(camera);
    const start = anchor.project(camera);
    assert.ok(Math.abs(Math.hypot((end.x-start.x)*400,(end.y-start.y)*300)-9) < 1e-6);
  }
  console.log("Display modes, datum occlusion picking, PBR interaction, CSS pixel dashes and arrows passed.");
} finally { await server.close(); }
