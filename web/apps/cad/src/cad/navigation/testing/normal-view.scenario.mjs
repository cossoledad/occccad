import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule("three");
 const {normalViewFrame,exactNormalViewPlane}=await server.ssrLoadModule("/src/cad/navigation/normal-view.ts");
 const {orientView,orientPlaneView}=await server.ssrLoadModule("/src/cad/navigation/orthographic-view.ts");
 const plane={origin:[2,3,5],normal:[1,2,3],uDirection:[2,-1,0]};
 const parent=new THREE.Matrix4().makeRotationX(.7).setPosition(8,9,10);
 const child=new THREE.Matrix4().makeRotationY(-.3).setPosition(-3,4,7);
 const placement=parent.clone().multiply(child);
 const frame=normalViewFrame(plane,placement);
 assert.ok(frame.origin.distanceTo(new THREE.Vector3(...plane.origin).applyMatrix4(child).applyMatrix4(parent))<1e-10);
 assert.ok(frame.normal.distanceTo(new THREE.Vector3(...plane.normal).transformDirection(placement))<1e-10);
 assert.ok(Math.abs(frame.normal.dot(frame.up))<1e-10);
 const camera=new THREE.OrthographicCamera(-100,100,80,-80,.01,10000);
 camera.position.set(20,-30,50);camera.zoom=7;
 const target=new THREE.Vector3(4,5,6);
 orientView(camera,target,frame.origin,frame.normal,frame.up);
 assert.equal(camera.zoom,7);
 assert.ok(camera.getWorldDirection(new THREE.Vector3()).dot(frame.normal)<-1+1e-10);
 assert.equal(normalViewFrame({...plane,normal:[0,0,0]},placement),undefined);
 assert.equal(normalViewFrame({...plane,uDirection:plane.normal},placement),undefined);
 for (const side of [-1,1]) {
   const n = frame.normal.clone().multiplyScalar(side);
   camera.position.copy(frame.origin).addScaledVector(n,80).addScaledVector(frame.up,15);
   camera.up.copy(frame.up);camera.lookAt(frame.origin);camera.rotateZ(.6);camera.updateMatrixWorld(true);
   orientPlaneView(camera,target,frame.origin,frame.normal,frame.up);
   assert.ok(camera.getWorldDirection(new THREE.Vector3()).dot(n)<-1+1e-10);
   const screenUp=new THREE.Vector3(0,1,0).applyQuaternion(camera.quaternion);
   assert.ok(Math.max(Math.abs(screenUp.dot(frame.up)),Math.abs(screenUp.dot(frame.normal.clone().cross(frame.up))))>1-1e-10,
     "plane axes must be horizontal/vertical, including rotated occurrences");
   assert.equal(camera.zoom,7);
   const aligned=camera.quaternion.clone();
   orientPlaneView(camera,target,frame.origin,frame.normal.clone().negate(),frame.up);
   assert.ok(aligned.angleTo(camera.quaternion)<1e-7,"reversed support normal must not flip the view");
 }
 // An XY sketch should remove arbitrary roll but keep the closest quarter turn.
 for (const roll of [.4,1.4,2.9,-1.2]) {
   camera.position.set(0,0,80);camera.up.set(0,1,0);camera.lookAt(0,0,0);camera.rotateZ(roll);camera.updateMatrixWorld(true);
   const before=camera.quaternion.clone();
   orientPlaneView(camera,target,new THREE.Vector3(),new THREE.Vector3(0,0,1),new THREE.Vector3(0,1,0));
   const up=new THREE.Vector3(0,1,0).applyQuaternion(camera.quaternion);
   assert.ok(Math.max(Math.abs(up.x),Math.abs(up.y))>1-1e-10);
   assert.ok(before.angleTo(camera.quaternion)<=Math.PI/4+1e-10);
 }
 const {ViewTransition}=await server.ssrLoadModule("/src/cad/navigation/view-transition.ts");
 const {saveView}=await server.ssrLoadModule("/src/cad/navigation/orthographic-view.ts");
 camera.position.set(40,-60,70);camera.up.set(0,0,1);target.set(1,2,3);camera.lookAt(target);camera.zoom=7;camera.updateMatrixWorld(true);
 const original=saveView(camera,target);
 const destination=camera.clone(), endTarget=target.clone();
 orientPlaneView(destination,endTarget,new THREE.Vector3(1,2,0),new THREE.Vector3(0,0,1),new THREE.Vector3(0,1,0));
 const end=saveView(destination,endTarget);
 const transition=new ViewTransition(camera,target);
 transition.start(end,0,280);
 assert.ok(camera.position.equals(original.position),"starting animation must not jump to the destination");
 transition.update(140);
 assert.ok(transition.active);
 assert.ok(camera.quaternion.angleTo(original.rotation)>1e-3);
 assert.ok(camera.quaternion.angleTo(end.rotation)>1e-3);
 assert.ok(Math.abs(camera.zoom-7)<1e-10);
 assert.ok(camera.position.distanceTo(target)>50,"orbit must not cut through the model focus");
 // A new command starts exactly at the current rendered frame.
 const halfway=saveView(camera,target);
 transition.start(original,140,280);transition.update(140);
 assert.ok(camera.position.distanceTo(halfway.position)<1e-10);
 assert.ok(camera.quaternion.angleTo(halfway.rotation)<1e-7);
 transition.update(420);
 assert.equal(transition.active,false);
 assert.ok(camera.position.equals(original.position));
 assert.ok(target.equals(original.target));
 transition.start(end,500);transition.update(600);transition.cancel();
 const interrupted=saveView(camera,target);
 assert.equal(transition.update(1000),false);
 assert.ok(camera.position.equals(interrupted.position),"cancelled animation cannot overwrite user navigation");
 transition.start(end,1000,0);
 assert.equal(transition.active,false);
 assert.ok(camera.position.equals(end.position));
 assert.ok(camera.quaternion.angleTo(end.rotation)<1e-7);
 const properties={geometryType:"PLANE",properties:{origin:plane.origin,normal:plane.normal,xDirection:plane.uDirection}};
 assert.deepEqual(exactNormalViewPlane(properties),plane);
 assert.equal(exactNormalViewPlane({...properties,geometryType:"CYLINDER"}),undefined);
 assert.equal(exactNormalViewPlane({...properties,properties:{normal:[0,0,1]}}),undefined);
} finally {await server.close();}
