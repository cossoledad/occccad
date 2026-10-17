import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const T=await server.ssrLoadModule("three"),{InfiniteGroundGrid}=await server.ssrLoadModule("/src/cad/rendering/infinite-ground-grid.ts");
 const frame={origin:new T.Vector3(10,20,30),normal:new T.Vector3(1,0,0),u:new T.Vector3(0,1,0),v:new T.Vector3(0,0,1)};
 for(const camera of [new T.OrthographicCamera(-200,200,150,-150,.1,1e12),new T.PerspectiveCamera(35,4/3,.0001,1e12)]){
  camera.position.set(120,30,80);camera.up.set(0,0,1);camera.lookAt(frame.origin);
  const sketch=new InfiniteGroundGrid(0xffffff,"sketch"),ground=new InfiniteGroundGrid(0xffffff);
  for(const zoom of [.001,1,10000]){
   camera.zoom=zoom;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
   for(const grid of [sketch,ground]){
    grid.update(camera,600,frame);assert.equal(grid.object.visible,true);
    const u=grid.object.material.uniforms,period=u.uSpacing.value*10;
    if(grid===ground){assert.equal(u.uSpacing.value,10);assert.equal(grid.object.material.depthTest,true)}
    const local=v=>new T.Vector3(v.dot(frame.u),v.dot(frame.v),v.dot(frame.normal));
    const anchor=camera.position.clone().sub(frame.u.clone().multiplyScalar(u.uOrigin.value.x)).sub(frame.v.clone().multiplyScalar(u.uOrigin.value.y)).sub(frame.normal.clone().multiplyScalar(u.uOrigin.value.z));
    for(const [x,y] of [[0,0],[-.8,.7],[.9,-.6]]){
     const ray=new T.Raycaster();ray.setFromCamera(new T.Vector2(x,y),camera);
     const t=frame.origin.clone().sub(ray.ray.origin).dot(frame.normal)/ray.ray.direction.dot(frame.normal);
     const actual=local(ray.ray.at(t,new T.Vector3()).sub(frame.origin));
     const offset=u.uRight.value.clone().multiplyScalar(x).addScaledVector(u.uUp.value,y);
     const origin=u.uOrigin.value.clone().addScaledVector(offset,1-u.uPerspective.value),direction=u.uForward.value.clone().addScaledVector(offset,u.uPerspective.value);
     const hit=origin.addScaledVector(direction,-origin.z/direction.z),world=anchor.clone().addScaledVector(frame.u,hit.x).addScaledVector(frame.v,hit.y);
     const phase=actual.sub(hit).sub(new T.Vector3(u.uBasePhase.value.x,u.uBasePhase.value.y,0)).divideScalar(period);
     assert.ok(Math.abs(phase.x-Math.round(phase.x))<1e-6&&Math.abs(phase.y-Math.round(phase.y))<1e-6,"ray-plane grid phase matches picking");
     if(grid===ground)for(const [step,offset] of [[u.uLevels.value.x,u.uLevelPhaseA.value],[u.uLevels.value.y,u.uLevelPhaseB.value]]){
      const delta=local(world.clone().sub(frame.origin)).sub(hit).sub(new T.Vector3(offset.x,offset.y,0)).divideScalar(step);
      assert.ok(Math.abs(delta.x-Math.round(delta.x))<1e-5&&Math.abs(delta.y-Math.round(delta.y))<1e-5,"all levels remain anchored to the document origin");
     }
     const clip=new T.Vector4(hit.x,hit.y,0,1).applyMatrix4(u.uClip.value),projected=world.project(camera);
     assert.ok(Math.abs(clip.z/clip.w-projected.z)<1e-7,"grid depth matches scene depth in either projection");
    }
   }
  }
  sketch.dispose();ground.dispose();
 }
}finally{await server.close()}
