import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule("three");
 const {NavigationController}=await server.ssrLoadModule("/src/cad/navigation/navigation-controller.ts");
 const {NavigationPicker,markNavigationPickable}=await server.ssrLoadModule("/src/cad/navigation/navigation-picker.ts");
 const {updatePerspectiveClipping}=await server.ssrLoadModule("/src/cad/navigation/projection-camera.ts");
 const wheel=deltaY=>({x:300,y:300,deltaY,deltaX:0,state:{modifiers:{},buttons:{},keys:new Set()},originalEvent:{deltaMode:0}});
 for(const scale of [.001,1,1000]){
  const box=new T.Mesh(new T.BoxGeometry(20*scale,20*scale,20*scale),new T.MeshBasicMaterial());markNavigationPickable(box);box.updateMatrixWorld(true);
  const hiddenPoint=new T.Vector3(0,0,9.995*scale);
  const hidden=new T.Points(new T.BufferGeometry().setFromPoints([hiddenPoint]),new T.PointsMaterial());markNavigationPickable(hidden);hidden.updateMatrixWorld(true);
  const camera=new T.PerspectiveCamera(35,1,1e-7,1e8);camera.position.set(0,0,10.01*scale);camera.updateMatrixWorld(true);
  const picker=new NavigationPicker(camera,{clientWidth:600,clientHeight:600},()=>[box,hidden],()=>hiddenPoint);
  const bounds=new T.Box3().setFromObject(box);
  const nav=new NavigationController(camera,()=>({width:600,height:600}),()=>updatePerspectiveClipping(camera,nav.target,bounds),picker,()=>new T.Vector3(),"default",false,()=>bounds);
  assert.equal(picker.pickNearest(300,300)?.object,box,"hidden/hovered point must not outrank a very close surface");
  for(let i=0;i<50;i++){
   nav.wheel(wheel(i===20?-1e6:-120));
   assert.ok(!bounds.containsPoint(camera.position),"even a large wheel delta stays outside the closed solid");
   const clearance=camera.position.z-10*scale;
   assert.ok(clearance>camera.near,"near plane stays ahead of the front surface");
   assert.ok(new T.Vector3(0,0,10*scale).project(camera).z>=-1,"the visible surface is not near-clipped");
   assert.ok(Number.isFinite(camera.far)&&camera.far>camera.near);
  }
  const close=camera.position.z;nav.wheel(wheel(120));assert.ok(camera.position.z>close,"zooming out remains responsive at the limit");
  camera.position.set(0,0,30*scale);camera.updateMatrixWorld(true);nav.target.set(0,0,0);
  const anchor=picker.pickSurface(320,280).point.clone(),screen=anchor.clone().project(camera);
  for(let i=0;i<12;i++){
   nav.wheel({...wheel(-120),x:320,y:280});const after=anchor.clone().project(camera);
   assert.ok(Math.hypot(after.x-screen.x,after.y-screen.y)<1e-8,"off-centre surface stays beneath the cursor");
  }
  hidden.visible=false;assert.equal(picker.pickSurface(300,300)?.object,box);
  box.visible=false;hidden.visible=true;assert.equal(picker.pickSurface(300,300),undefined,"hidden surfaces and visible points do not anchor surface zoom");
  box.geometry.dispose();box.material.dispose();hidden.geometry.dispose();hidden.material.dispose();
 }
 // Clipping independently protects a close face when the rotation pivot is deep in the part.
 const camera=new T.PerspectiveCamera(35,1,.1,1e8);camera.position.set(0,0,10.0001);camera.updateMatrixWorld(true);
 const bounds=new T.Box3(new T.Vector3(-10,-10,-10),new T.Vector3(10,10,10)),eye=camera.position.clone();
 updatePerspectiveClipping(camera,new T.Vector3(),bounds);assert.deepEqual(camera.position,eye);assert.ok(camera.near<.0001);
 camera.position.z=5;camera.updateMatrixWorld(true);updatePerspectiveClipping(camera,new T.Vector3(),bounds);assert.ok(camera.near>1e-7,"a bounding envelope crossing the eye must not collapse depth precision");
 camera.position.set(3,-3,13);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);updatePerspectiveClipping(camera,new T.Vector3(),bounds);
 assert.ok(camera.near>.001,"ordinary oblique views retain depth precision even inside the bounding envelope");
 let queries=0;updatePerspectiveClipping(camera,new T.Vector3(),bounds,()=>{queries++;return .0001});
 assert.equal(queries,1);assert.ok(camera.near<.0001,"a real very close surface constrains near independently of the bounding envelope");
 camera.position.set(0,0,100);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);updatePerspectiveClipping(camera,new T.Vector3(),bounds,()=>{throw Error("fit views must not need a surface query")});
 // Perspective feature thresholds include projection zoom, rather than using the part centre distance.
 camera.position.set(0,0,20);camera.zoom=100;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
 const point=new T.Points(new T.BufferGeometry().setFromPoints([new T.Vector3(.001,0,0)]),new T.PointsMaterial());markNavigationPickable(point);point.updateMatrixWorld(true);
 const picker=new NavigationPicker(camera,{clientWidth:600,clientHeight:600},()=>[point]);
 assert.ok(picker.pickNearest(300,300),"a point within 6 CSS pixels remains pickable at high projection zoom");
 point.position.x=.1;point.updateMatrixWorld(true);assert.equal(picker.pickNearest(300,300),undefined,"a distant screen point is not promoted by a world-sized tolerance");
 point.geometry.dispose();point.material.dispose();
}finally{await server.close()}
