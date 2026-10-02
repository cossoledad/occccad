import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {snapFrame,exactSnapCandidates,ManipulatorSnapCache}=await server.ssrLoadModule("/src/cad/interaction/manipulator-snap.ts");
 const prior=new THREE.Quaternion(),direction=new THREE.Vector3(0,1,0);
 const frame=snapFrame(direction,true,prior,new THREE.Vector3(1,0,0));
 assert.ok(new THREE.Vector3(0,0,1).applyQuaternion(frame).distanceTo(direction)<1e-12);
 assert.ok(new THREE.Matrix4().makeRotationFromQuaternion(frame).determinant()>0.999999);
 assert.equal(snapFrame(new THREE.Vector3(),true,prior),undefined);
 const stable=snapFrame(direction,true,frame,new THREE.Vector3(-1,0,0));assert.ok(stable.angleTo(frame)<1e-12);
 const rotation=new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0,0,1),Math.PI/2);
 const world=new THREE.Matrix4().compose(new THREE.Vector3(10,20,30),rotation,new THREE.Vector3(1,1,1));
 const descriptor={Kind:"AXIS",Origin:[1,2,3],Direction:[1,0,0],LengthUnit:"mm"};
 const values=exactSnapCandidates(descriptor,{center:[5,2,3],endFirst:[3,2,3],endLast:[7,2,3]},world,new THREE.Vector3(8,24,33),prior);
 assert.ok(values.find(c=>c.role==="center").position.distanceTo(new THREE.Vector3(8,25,33))<1e-12);
 assert.ok(new THREE.Vector3(1,0,0).applyQuaternion(values[0].orientation).distanceTo(new THREE.Vector3(0,1,0))<1e-12);
 for(const kind of ["POINT","FRAME","CIRCLE","PLANE","CYLINDER"]){
   const c=exactSnapCandidates({...descriptor,Kind:kind,Rotation:[0,0,0,1]},undefined,world,new THREE.Vector3(8,24,33),prior);
   assert.ok(c.length>0,kind);assert.ok(c.every(v=>v.exact&&v.position.toArray().every(Number.isFinite)));
 }
 assert.deepEqual(exactSnapCandidates({...descriptor,LengthUnit:"m"},undefined,world,new THREE.Vector3(),prior),[],"unknown units cannot be certified");
 const cache=new ManipulatorSnapCache();let resolveA,resolveB,accepted=[],calls=0;
 cache.request("a",()=>{calls++;return new Promise(resolve=>resolveA=resolve)},v=>accepted.push(v));
 cache.request("a",()=>{throw Error("duplicate pointermove query")},v=>accepted.push(v));assert.equal(calls,1);
 cache.request("b",()=>new Promise(resolve=>resolveB=resolve),v=>accepted.push(v));
 resolveA({versionId:"old"});resolveB({versionId:"fresh"});await new Promise(resolve=>setImmediate(resolve));
 assert.deepEqual(accepted,[{versionId:"fresh"}]);assert.equal(cache.get("a"),undefined);
 cache.clear();assert.equal(cache.get("b"),undefined);
 // Exercise the actual viewport boundary: a precise source on another rigid
 // unit must orient/reposition only the original unit's temporary handle.
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const engine=Object.create(CadViewportEngine.prototype),controlled=new THREE.Group(),source=new THREE.Group();
 controlled.userData.id="controlled";source.position.set(10,20,30);source.quaternion.copy(rotation);source.updateMatrixWorld(true);
 const path={rootDocumentId:"product",canonical:"source",segments:[{instanceId:"source",ownerVersionId:"r",resolvedVersionId:"part"}]};
 let resolveExact,attached=[],moving=false,invalidations=0;
 const camera=new THREE.OrthographicCamera(-50,50,50,-50,.1,1000);camera.position.z=200;camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
 Object.assign(engine,{activeToolID:"assembly.move",view:{document:{id:"product",versionId:"r"},resolvedInstances:[{occurrencePath:"source",instancePath:path}]},
   instanceGroups:new Map([["controlled",controlled],["source",source]]),manipulatorPivots:new Map(),manipulatorFrames:new Map(),
   moveTarget:{group:controlled,startPivot:new THREE.Vector3(),localPivot:new THREE.Vector3()},
   moveManipulator:{frameQuaternion:()=>new THREE.Quaternion(),isDragging:()=>moving,isPivotDragging:()=>false,attach:(p,q)=>attached.push({p:p.clone(),q:q.clone()})},
   manipulatorSnapCache:new ManipulatorSnapCache(),camera,renderer:{domElement:{clientWidth:1000,clientHeight:1000},getPixelRatio:()=>1},
   callbacks:{inspectAssemblySupports:()=>new Promise(resolve=>resolveExact=resolve)},invalidate:()=>invalidations++});
 const pick={kind:"edge",id:"edge",instanceId:"source",occurrencePath:"source",geometryKey:"g",topologyId:1};
 const response={documentId:"product",versionId:"r",supports:[{status:"RESOLVED",descriptor:{...descriptor,Kind:"CIRCLE"},reference:{},constraintEligible:true}]};
 engine.manipulatorAnchorFromIntersection({point:new THREE.Vector3(8,21,33)},pick);
 resolveExact(response);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(attached.length,1);assert.equal(engine.moveTarget.group,controlled);assert.deepEqual(controlled.position.toArray(),[0,0,0]);assert.ok(invalidations>0);
 assert.ok(attached[0].p.distanceTo(new THREE.Vector3(8,21,33))<1e-12,"exact circle center transformed once across components");
 engine.manipulatorAnchorFromIntersection({point:new THREE.Vector3(8,21,33)},{...pick,topologyId:2});
 moving=true;resolveExact(response);await new Promise(resolve=>setImmediate(resolve));assert.equal(attached.length,1,"late precision cannot rotate a started motion frame");
 moving=false;engine.manipulatorAnchorFromIntersection({point:new THREE.Vector3(8,21,33)},{...pick,topologyId:3});
 engine.view={...engine.view,document:{...engine.view.document,versionId:"new"}};
 resolveExact(response);await new Promise(resolve=>setImmediate(resolve));assert.equal(attached.length,1,"old snapshot cannot install a new pivot");
}finally{await server.close();}
