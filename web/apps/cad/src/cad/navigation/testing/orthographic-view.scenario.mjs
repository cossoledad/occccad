import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const THREE = await server.ssrLoadModule("three");
  const { ThreeCameraRig } = await server.ssrLoadModule("/src/cad/navigation/camera-rig.ts");
  const { standardView, fitOrthographicView, updateOrthographicClipping, saveView, restoreView, orientView, viewFocus } = await server.ssrLoadModule("/src/cad/navigation/orthographic-view.ts");
  const camera = new THREE.OrthographicCamera(-200, 200, 150, -150, .001, 10000);
  camera.position.set(300, -300, 300); camera.up.set(0, 0, 1);
  const rig = new ThreeCameraRig(camera); rig.lookAtPivot();
  const screen = point => {
    const p = point.clone().project(camera); return new THREE.Vector2(p.x * 400, -p.y * 300);
  };
  // At each scale, a 17px drag must produce exactly 17 screen pixels, even far from world zero.
  for (const zoom of [.01, 1, 100, 100000]) {
    camera.zoom = zoom; camera.updateProjectionMatrix();
    const point = rig.pivot.clone(); const before = screen(point);
    rig.panPixels(17, -11, 800, 600);
    const movement = screen(point).sub(before);
    assert.ok(Math.abs(movement.x - 17) < 1e-5 && Math.abs(movement.y + 11) < 1e-5, `pan scale ${zoom}: ${movement.toArray()}`);
  }
  // Off-centre wheel zoom keeps the hit point fixed and the view centre/pivot coherent.
  camera.zoom = 100; camera.updateProjectionMatrix();
  const hit = rig.pivot.clone().add(new THREE.Vector3(.1, .2, .1));
  const anchor = screen(hit);
  for (let i = 0; i < 10; i++) rig.dollyPixels(-100, hit);
  assert.ok(screen(hit).distanceTo(anchor) < 1e-5);
  assert.ok(screen(rig.pivot).length() < 1e-5, "zoom translates the pivot with the view centre");
  // The orientation primitive retains scale/focus; the viewport ISO command follows it with Fit.
  rig.orbitPixels(120, -80);
  const zoom = camera.zoom, focus = viewFocus(camera, rig.pivot);
  for (let i = 0; i < 5; i++) {
    standardView(camera, rig.pivot, "ISO");
    assert.equal(camera.zoom, zoom);
    assert.ok(screen(focus).length() < 1e-5);
    const direction = camera.getWorldDirection(new THREE.Vector3());
    assert.ok(direction.distanceTo(new THREE.Vector3(-1, 1, -1).normalize()) < 1e-10);
  }
  // Camera-aligned fitting works for wide and tall viewports, off-origin content and rolled views.
  const box = new THREE.Box3(new THREE.Vector3(10000, -300, 200), new THREE.Vector3(10120, -260, 235));
  for (const aspect of [.4, 2]) {
    camera.left = -150 * aspect; camera.right = 150 * aspect;
    rig.orbitPixels(33, 40);
    fitOrthographicView(camera, rig.pivot, box);
    for (let i = 0; i < 8; i++) {
      const p = new THREE.Vector3(i & 1 ? box.max.x : box.min.x, i & 2 ? box.max.y : box.min.y, i & 4 ? box.max.z : box.min.z).project(camera);
      assert.ok(Math.abs(p.x) < 1 && Math.abs(p.y) < 1, "fit must include every projected corner");
    }
  }
  // Clipping accommodates both very distant content and content behind the old eye,
  // without changing orthographic XY projection or sacrificing precision to an arbitrary infinity.
  for (const offset of [0, 1e9]) {
    const clipBounds = new THREE.Box3(new THREE.Vector3(offset-100, -100, -100), new THREE.Vector3(offset+100, 100, 100));
    camera.position.set(offset, 0, 0); camera.quaternion.identity(); camera.zoom = 100;
    camera.updateProjectionMatrix(); camera.updateMatrixWorld(true);
    const point = new THREE.Vector3(offset+20, 10, 80), before = point.clone().project(camera);
    updateOrthographicClipping(camera, clipBounds);
    const after = point.clone().project(camera);
    assert.ok(Math.hypot(after.x-before.x,after.y-before.y)<1e-8);
    for (const z of [-100,100]) assert.ok(Math.abs(new THREE.Vector3(offset,0,z).project(camera).z)<1);
    assert.equal(camera.near,0); assert.ok(Number.isFinite(camera.far));
    const position = camera.position.clone(); updateOrthographicClipping(camera, clipBounds);
    assert.deepEqual(camera.position,position,"clipping must not drift the eye on every frame");
    const closeRange = camera.far;
    camera.zoom = 1e-5; updateOrthographicClipping(camera, clipBounds);
    assert.ok(camera.far > closeRange * 1000);
    camera.zoom = 100; updateOrthographicClipping(camera, clipBounds);
    assert.ok(Math.abs(camera.far-closeRange)<1e-6,"zooming back in must recover depth precision");
  }
  const saved = saveView(camera, rig.pivot);
  const planeOrigin = new THREE.Vector3(0, 0, 7), normal = new THREE.Vector3(0, 0, 1);
  const planeFocus = viewFocus(camera, rig.pivot); planeFocus.z = planeOrigin.z;
  orientView(camera, rig.pivot, planeFocus, normal, new THREE.Vector3(0, 1, 0));
  assert.equal(camera.zoom, saved.zoom);
  assert.ok(screen(planeFocus).length() < 1e-6);
  assert.ok(camera.getWorldDirection(new THREE.Vector3()).distanceTo(normal.clone().negate()) < 1e-10);
  rig.dollyPixels(-500); rig.panPixels(50, 40, 800, 600);
  restoreView(camera, rig.pivot, saved);
  assert.ok(camera.position.distanceTo(saved.position) < 1e-10);
  assert.ok(camera.quaternion.angleTo(saved.rotation) < 1e-7);
  assert.equal(camera.zoom, saved.zoom); assert.deepEqual(rig.pivot, saved.target);
  // At detail scale, CATIA orbit anchors to the visible surface, not the body centre behind it.
  const { NavigationController } = await server.ssrLoadModule("/src/cad/navigation/navigation-controller.ts");
  const detailCamera = new THREE.OrthographicCamera(-20, 20, 15, -15, .001, 1000);
  detailCamera.position.set(0, 0, 100); detailCamera.zoom = 100; detailCamera.updateProjectionMatrix();
  const surface = new THREE.Vector3(0, 0, 10);
  const detailNav = new NavigationController(detailCamera, () => ({width:800,height:600}), () => {},
    { pickNearest: () => ({ point: surface.clone(), source:"raycast" }), pickViewPlane: () => undefined },
    () => new THREE.Vector3(), "catia", false,
    () => new THREE.Box3(new THREE.Vector3(-10,-10,-10), new THREE.Vector3(10,10,10)));
  const input = (button, left, x=400, y=300) => ({pointerId:1,button,x,y,deltaX:x-400,deltaY:y-300,
    state:{buttons:{left,middle:true,right:false},modifiers:{ctrl:false,alt:false,shift:false,meta:false}}});
  detailNav.pointerDown(input(1,false)); detailNav.pointerDown(input(0,true));
  detailNav.pointerMove(input(-1,true,420,310));
  const anchored = surface.clone().project(detailCamera);
  assert.ok(Math.hypot(anchored.x,anchored.y)<1e-8, "visible detail stays centred during rotation");
  console.log("Orthographic precision, zoom anchor, standard views, fitting and sketch view round-trip passed.");
} finally { await server.close(); }
