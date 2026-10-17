import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {ViewTransition}=await server.ssrLoadModule("/src/cad/navigation/view-transition.ts");
 const {saveView,restoreView,standardView,fitOrthographicView}=await server.ssrLoadModule("/src/cad/navigation/orthographic-view.ts");
 const {fitPerspectiveView}=await server.ssrLoadModule("/src/cad/navigation/projection-camera.ts");
 const bounds=new T.Box3(new T.Vector3(-10,-15,-8),new T.Vector3(25,30,20));
 for(const perspective of [true,false]){
  const camera=perspective?new T.PerspectiveCamera(35,4/3,.01,10000):new T.OrthographicCamera(-100,100,75,-75,0,10000);
  const target=new T.Vector3(3,4,5);camera.position.set(40,-50,70);camera.up.set(0,0,1);camera.lookAt(target);camera.zoom=3;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
  const original=saveView(camera,target),transition=new ViewTransition(camera,target),engine=Object.create(CadViewportEngine.prototype);
  let cancelled=0,invalidated=0;
  Object.assign(engine,{camera,navigation:{target,cancel:()=>cancelled++,syncCamera(){}},viewTransition:transition,
   visibleContentBounds:()=>bounds.clone(),emitDebugState(){},invalidate:()=>invalidated++});
  for(const command of ["FIT","TOP","FRONT","RIGHT","ISO"]){
   restoreView(camera,target,original);
   const destination=camera.clone(),endTarget=target.clone();
   if(command!=="FIT")standardView(destination,endTarget,command);
   if(command==="FIT"||command==="ISO")(perspective?fitPerspectiveView:fitOrthographicView)(destination,endTarget,bounds);
   if(command==="FIT")engine.fit();else engine.setStandardView(command);
   assert.ok(transition.active,`${command} must animate`);assert.ok(camera.position.equals(original.position),"command starts from the displayed frame");
   const started=transition.pending.started;transition.update(started+140);
   assert.ok(transition.active);assert.ok(camera.position.distanceTo(destination.position)>1e-7||camera.quaternion.angleTo(destination.quaternion)>1e-5||Math.abs(camera.zoom-destination.zoom)>1e-7,"there is a real intermediate frame");
   transition.update(started+280);
   assert.ok(camera.position.distanceTo(destination.position)<1e-9);assert.ok(camera.quaternion.angleTo(destination.quaternion)<1e-7);
   assert.ok(target.distanceTo(endTarget)<1e-9);assert.ok(Math.abs(camera.zoom-destination.zoom)<1e-9);
  }
  engine.fit();transition.update(transition.pending.started+70);const current=saveView(camera,target);
  engine.setStandardView("RIGHT");assert.ok(camera.position.equals(current.position),"retargeted commands do not snap");
  engine.frameContent();assert.equal(transition.active,false,"initial document framing remains immediate");
  assert.ok(cancelled>=7&&invalidated>=7);
 }
 const camera=new T.PerspectiveCamera(35,1,.001,10000),target=new T.Vector3(0,0,10);
 camera.position.set(0,0,10.001);camera.updateMatrixWorld(true);
 const destination=camera.clone();destination.position.z=100;
 const transition=new ViewTransition(camera,target);transition.start(saveView(destination,new T.Vector3()),0,280);
 transition.update(140);assert.ok(Math.abs(camera.position.distanceTo(target)-Math.sqrt(.001*100))<1e-9,"Fit magnification interpolates geometrically");
 assert.ok(camera.position.z>10,"recentring waits for zoom out instead of cutting into the inspected face");
 transition.update(280);assert.ok(camera.position.equals(destination.position));
}finally{await server.close()}
