import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const T = await server.ssrLoadModule("three");
  const { InfiniteGroundGrid } = await server.ssrLoadModule("/src/cad/rendering/infinite-ground-grid.ts");
  const grid = new InfiniteGroundGrid(0xffffff);
  const frame = {origin:new T.Vector3(10,20,30),normal:new T.Vector3(1,0,0),u:new T.Vector3(0,1,0),v:new T.Vector3(0,0,1)};
  const camera = new T.OrthographicCamera(-200,200,150,-150,.1,1000);
  camera.position.set(120,30,80);camera.up.set(0,0,1);camera.lookAt(frame.origin);
  const plane = new T.Plane().setFromNormalAndCoplanarPoint(frame.normal,frame.origin);
  for (const zoom of [.001,1,10000]) {
    camera.zoom=zoom;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
    grid.update(camera,600,frame);
    assert.equal(grid.object.visible,true);
    const uniforms=grid.object.material.uniforms;
    const period=uniforms.uSpacing.value*10;
    const pixels=uniforms.uSpacing.value/((camera.top-camera.bottom)/zoom/600);
    assert.ok(pixels>=32-1e-8&&pixels<=80+1e-8,"grid spacing should stay readable across scales");
    for (const [x,y] of [[0,0],[-.8,.7],[.9,-.6]]) {
      const raycaster=new T.Raycaster();raycaster.setFromCamera(new T.Vector2(x,y),camera);
      // An infinite orthographic grid also covers plane points behind the eye;
      // use the signed line intersection instead of Ray's forward-only result.
      const distance=-plane.distanceToPoint(raycaster.ray.origin)/frame.normal.dot(raycaster.ray.direction);
      const point=raycaster.ray.at(distance,new T.Vector3()).sub(frame.origin);
      const actual=new T.Vector2(point.dot(frame.u),point.dot(frame.v));
      const shader=uniforms.uCenter.value.clone().addScaledVector(uniforms.uHorizontal.value,x).addScaledVector(uniforms.uVertical.value,y);
      const error=actual.sub(shader).divideScalar(period);
      assert.ok(Math.abs(error.x-Math.round(error.x))<1e-6&&Math.abs(error.y-Math.round(error.y))<1e-6,
        "screen grid must coincide with ray/plane intersection modulo rebased grid periods");
    }
    const before=uniforms.uHorizontal.value.clone();camera.near=0;camera.far=1e12;camera.updateProjectionMatrix();grid.update(camera,600,frame);
    assert.ok(before.distanceTo(uniforms.uHorizontal.value)<1e-10,"model clipping must not change grid projection");
  }
  camera.position.set(10,20,130);camera.up.set(0,1,0);camera.lookAt(frame.origin);grid.update(camera,600,frame);
  assert.equal(grid.object.visible,false,"edge-on planes have no area and must not produce NaNs");
  grid.dispose();
} finally { await server.close(); }
