import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule("three");
 const {NavigationController}=await server.ssrLoadModule("/src/cad/navigation/navigation-controller.ts");
 const {ThreeCameraRig}=await server.ssrLoadModule("/src/cad/navigation/camera-rig.ts");
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
 // Drag zoom uses an interior pivot in CATIA/SOLIDWORKS: its distance is not
 // a surface clearance. Exercise the complete controller -> picker -> rig path.
 for(const scale of [.001,1,1000])for(const profile of ["catia","solidworks"]){
  const box=new T.Mesh(new T.BoxGeometry(20,20,20),new T.MeshBasicMaterial());box.scale.setScalar(scale);box.updateMatrixWorld(true);markNavigationPickable(box);
  const bounds=new T.Box3().setFromObject(box),camera=new T.PerspectiveCamera(35,1,.01,1e8);camera.position.z=30*scale;camera.updateMatrixWorld(true);
  const picker=new NavigationPicker(camera,{clientWidth:600,clientHeight:600},()=>[box]);
  const nav=new NavigationController(camera,()=>({width:600,height:600}),()=>updatePerspectiveClipping(camera,nav.target,bounds),picker,()=>new T.Vector3(),profile,false,()=>bounds);
  const margin=picker.pickZoomBarrier(camera.position,new T.Vector3(0,0,-1)).clearance;
  assert.ok(Math.abs(margin/scale-Math.sqrt(1200)*.001)<1e-8,"clearance follows world model size, including occurrence scale");
  let y=300;
  const event=(phase,deltaY=0)=>({phase,button:phase==="down"?1:-1,pointerId:1,pointerType:"mouse",x:300,y:y+=deltaY,deltaX:0,deltaY,
   state:{buttons:{middle:true},modifiers:{ctrl:profile==="catia",shift:profile==="solidworks"},keys:new Set()},originalEvent:{timeStamp:0}});
  nav.pointerDown(event("down"));let lastTravel=Infinity;
  for(let i=0;i<50;i++){
   const before=camera.position.z;nav.pointerMove(event("move",i===40?-1e6:-120));const travel=before-camera.position.z,gap=camera.position.z-10*scale;
   assert.ok(gap>=margin-1e-10*scale,"eye remains before the surface even for one huge input");
   assert.ok(gap>camera.near,"near does not cut the inspected exterior");
   if(i<40)assert.ok(travel<=lastTravel+1e-10*scale,"equal input smoothly decreases eye travel");
   assert.ok(travel>=-1e-10*scale,"zoom in never pushes the eye backwards");lastTravel=travel;
  }
  const near=camera.position.z;nav.pointerMove(event("move",120));assert.ok(camera.position.z>near,"zoom out responds immediately at the limit");
  assert.ok(camera.position.z-near<margin,"zoom out from the limit must not jump back by the interior pivot distance");nav.cancel();
  // An already closer view is not pushed back to the new safety distance.
  camera.position.z=10*scale+margin/2;camera.updateMatrixWorld(true);nav.target.set(0,0,0);
  nav.pointerDown(event("down"));const close=camera.position.clone();nav.pointerMove(event("move",-120));assert.ok(camera.position.distanceTo(close)<1e-12*scale);
  box.geometry.dispose();box.material.dispose();
 }
 // Off-axis movement may cross a solid even when its endpoint is outside it.
 const obstacle=new T.Mesh(new T.BoxGeometry(20,20,20),new T.MeshBasicMaterial());markNavigationPickable(obstacle);obstacle.updateMatrixWorld(true);
 const sweptCamera=new T.PerspectiveCamera(35,1,.01,1000);sweptCamera.position.set(0,0,30);sweptCamera.updateMatrixWorld(true);
 const sweepPicker=new NavigationPicker(sweptCamera,{clientWidth:600,clientHeight:600},()=>[obstacle]);
 const sweepRig=new ThreeCameraRig(sweptCamera,{zoomBarrier:(origin,direction)=>sweepPicker.pickZoomBarrier(origin,direction)});sweepRig.setPivot(new T.Vector3(12,0,0));
 const anchor=sweepRig.pivot.clone(),projected=anchor.clone().project(sweptCamera);
 sweepRig.dollyPixels(-1e6);assert.ok(sweptCamera.position.z>10,"large off-axis zoom cannot tunnel through a solid to an empty endpoint");
 const after=anchor.clone().project(sweptCamera);assert.ok(Math.hypot(after.x-projected.x,after.y-projected.y)<1e-10,"guarded off-axis zoom preserves the image anchor");
 const run=steps=>{sweptCamera.position.set(0,0,30);sweptCamera.updateMatrixWorld(true);sweepRig.setPivot(new T.Vector3());for(const step of steps)sweepRig.dollyPixels(step);return sweptCamera.position.clone()};
 assert.ok(run([-600]).distanceTo(run(Array(10).fill(-60)))<1e-10,"zoom curve is independent of event subdivision");
 assert.ok(run([-120,120]).distanceTo(run([]))<1e-10,"away from the buffer, reverse input restores the previous magnification");
 const translatedPivot=new T.Vector3(250,-180,75);sweepRig.setPivot(translatedPivot);sweptCamera.position.copy(translatedPivot).add(new T.Vector3(0,0,30));sweptCamera.updateMatrixWorld(true);
 sweepRig.dollyPixels(-120);assert.ok(sweepRig.pivot.equals(translatedPivot),"default anchor must not be zeroed by an aliased pivot transform");
 const fixedScreen=translatedPivot.clone().project(sweptCamera);sweepRig.dollyPixels(-120);assert.ok(sweepRig.pivot.equals(translatedPivot));
 const translatedScreen=translatedPivot.clone().project(sweptCamera);assert.ok(Math.hypot(translatedScreen.x-fixedScreen.x,translatedScreen.y-fixedScreen.y)<1e-9,"repeated zoom does not drift towards the world origin");
 const parent=new T.Group();parent.add(obstacle);parent.visible=false;assert.equal(sweepPicker.pickZoomBarrier(sweptCamera.position,new T.Vector3(0,0,-1)),undefined,"hidden ancestry cannot block zoom");
 parent.visible=true;markNavigationPickable(obstacle,false);assert.equal(sweepPicker.pickZoomBarrier(sweptCamera.position,new T.Vector3(0,0,-1)),undefined,"unpickable helpers do not block zoom");
 const parallel=new T.OrthographicCamera(-10,10,10,-10,0,100),parallelRig=new ThreeCameraRig(parallel,{zoomBarrier:()=>{throw Error("parallel projection never moves towards a surface")}});
 parallel.position.z=30;parallelRig.dollyPixels(-120);assert.ok(parallel.zoom>1);assert.equal(parallel.position.z,30);
 obstacle.geometry.dispose();obstacle.material.dispose();
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
