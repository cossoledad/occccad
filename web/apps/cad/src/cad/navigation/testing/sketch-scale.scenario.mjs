import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ appType:"custom", logLevel:"silent", server:{middlewareMode:true} });
try {
  const T = await server.ssrLoadModule("three");
  const { InfiniteGroundGrid, adaptiveGridSpacing } = await server.ssrLoadModule("/src/cad/rendering/infinite-ground-grid.ts");
  const { sketchAxisEndpoints, makeSketchReferenceAxis } = await server.ssrLoadModule("/src/cad/rendering/sketch-reference-axis.ts");
  const { updateScreenLines } = await server.ssrLoadModule("/src/cad/rendering/screen-space-lines.ts");
  const { resolveSketchSnap } = await server.ssrLoadModule("/src/cad/interaction/sketch-snap.ts");
  const { resolveSketchReference } = await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
  const { CadViewportEngine } = await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const camera = new T.OrthographicCamera(-200,200,150,-150,.1,5000);
  camera.position.set(20,-30,500); camera.lookAt(0,0,0); camera.updateMatrixWorld(true);
  const origin = new T.Vector3(0,0,0), direction = new T.Vector3(1,0,0);
  const axes = new T.Group(); axes.add(makeSketchReferenceAxis(origin,direction,new T.Color(0xffffff)));
  const helpers = new T.Group();
  helpers.add(new T.Line(new T.BufferGeometry().setFromPoints([origin,new T.Vector3(.0001,.0001,0)]),new T.LineBasicMaterial()));
  const engine = { camera, contentBounds:new T.Box3(), helpers, sketchContext:axes, navigation:{target:origin} };
  const grid = new InfiniteGroundGrid(0xffffff,"sketch");
  const frame = { origin, normal:new T.Vector3(0,0,1), u:direction, v:new T.Vector3(0,1,0) };
  const screen = point => {
    const p = new T.Vector3(point[0],point[1],0).project(camera);
    return [p.x*400,p.y*300];
  };
  for (const zoom of [.001,1,1e4,1e6]) {
    camera.zoom=zoom; camera.updateProjectionMatrix();
    updateScreenLines(axes,camera,800,600);
    CadViewportEngine.prototype.updateCameraClipping.call(engine);
    for (const point of [origin,new T.Vector3(.0001,.0001,0)]) {
      assert.ok(Math.abs(point.clone().project(camera).z)<1, "standalone sketch survives clipping at detail scale");
    }
    const before = camera.position.clone();
    CadViewportEngine.prototype.updateCameraClipping.call(engine);
    assert.ok(before.distanceTo(camera.position)<1e-8,"repeated clipping must not drift");
    const ends=sketchAxisEndpoints(camera,origin,direction,800,600);
    const a=ends[0].clone().project(camera), b=ends[1].clone().project(camera);
    assert.ok(Math.abs(a.z)<1 && Math.abs(b.z)<1,"the full displayed axes survive clipping");
    assert.ok(Math.abs(Math.hypot((a.x-b.x)*400,(a.y-b.y)*300)-320)<1e-5);
    assert.equal(ends[0].z,0); assert.equal(ends[1].z,0,"axis stays on the sketch plane");
    const extent=ends[1].distanceTo(origin);
    const pickPoint=screen([extent*.8,0]);
    assert.equal(resolveSketchReference({x:pickPoint[0],y:pickPoint[1]},[],p=>{
      const q=screen(p);return {x:q[0],y:q[1]};
    },"LINE",12,[extent,extent]).target,"SKETCH_X_AXIS");
    grid.update(camera,600,frame);
    const spacing=adaptiveGridSpacing(camera,600);
    assert.equal(grid.object.material.uniforms.uSpacing.value,spacing);
    for (const index of [1,2,3,7]) {
      const node=[spacing*index,spacing*2];
      const snapped=resolveSketchSnap([node[0]+spacing*.01,node[1]],[],1,spacing,11,["GRID"],screen);
      assert.equal(snapped?.kind,"GRID","every visible subdivision participates in snapping");
      assert.ok(Math.abs(snapped.point[0]-node[0])<spacing*1e-10);
    }
  }
  grid.dispose();
  axes.traverse(o=>{o.geometry?.dispose();o.material?.dispose();});
  helpers.traverse(o=>{o.geometry?.dispose();o.material?.dispose();});
  console.log("Sketch clipping, fixed-pixel axis picking and adaptive grid snapping passed.");
} finally { await server.close(); }
