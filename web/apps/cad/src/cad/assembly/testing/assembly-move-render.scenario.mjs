import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {AssemblyManipulator}=await server.ssrLoadModule("/src/cad/interaction/assembly-manipulator.ts");
 const {TransformTransitionSystem}=await server.ssrLoadModule("/src/cad/animation/transform-transition.ts");
 const {SelectionIndex}=await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
 const shaders={createMaterial:(_program,uniforms)=>new THREE.ShaderMaterial({uniforms:{uActive:{value:0},...Object.fromEntries(Object.entries(uniforms).map(([k,v])=>[k,{value:v}]))}})};
 const manipulator=new AssemblyManipulator(shaders,{poseChanged(){},visualChanged(){},pivotChanged(){},snapPivot(){},dragStarted(){},dragFinished(){}});
 const engine=Object.create(CadViewportEngine.prototype),a=new THREE.Group(),b=new THREE.Group(),content=new THREE.Group();
 a.userData.id="a";b.userData.id="b";a.add(new THREE.Mesh(new THREE.BoxGeometry(2,2,2),new THREE.MeshBasicMaterial()));b.add(a.children[0].clone());content.add(a,b);
 b.position.x=10;content.updateMatrixWorld(true);manipulator.attach(new THREE.Vector3(1,0,0));
 let invalidates=0,hydrates=0;
 Object.assign(engine,{content,helpers:new THREE.Group(),instanceGroups:new Map([["a",a],["b",b]]),
  moveManipulator:manipulator,moveTarget:{group:a,startPosition:new THREE.Vector3(),startQuaternion:new THREE.Quaternion(),startPivot:new THREE.Vector3(1,0,0),localPivot:new THREE.Vector3(1,0,0)},
  selectable:new Map([["instance:a",a],["instance:b",b]]),manipulatorFrames:new Map(),manipulatorPivots:new Map(),moveInteraction:{hasUncommittedFinal:false},
  selected:[],preselected:null,selectionIndex:new SelectionIndex(),assemblyConstraintReferences:new Map(),assemblyConstraintMarkers:new Map(),solidBindings:new Map(),
  highlightedRoots:new Set(),selectedOverlays:[],preselectedOverlays:[],
  materials:{constraintGlyph:(_glyph,color)=>new THREE.PointsMaterial({color}),setInteractionState(){}},
  transforms:new TransformTransitionSystem(()=>invalidates++,false,()=>{throw Error("accepted assembly frame must never schedule interpolation");}),
  visuals:{hydrate(){hydrates++;throw Error("accepted frame must not hydrate GLB");}},refreshContentBounds(){},invalidate:()=>invalidates++});
 const constraint={id:"group",kind:"FIX_TOGETHER",first:{instanceId:"a",kind:"BODY"},second:{instanceId:"b",kind:"BODY"},groupMembers:[{instanceId:"a"},{instanceId:"b"}],evaluationStatus:"VERIFIED"};
 engine.view={document:{id:"assembly",versionId:"frozen",type:"PRODUCT"},product:{instances:[{id:"a",translation:[0,0,0]},{id:"b",translation:[10,0,0]}],constraints:[constraint]},resolvedInstances:[]};
 const rotation=new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0,0,1),Math.PI/2);
 const geometry=a.children[0].geometry;
 engine.applyAcceptedMoveFrame({previewId:"",instancePoses:[{instanceId:"a",translation:[2,3,0],rotation:rotation.toArray()},{instanceId:"b",translation:[2,13,0],rotation:rotation.toArray()}]});
 assert.ok(a.position.distanceTo(new THREE.Vector3(2,3,0))<1e-12);
 assert.ok(b.position.distanceTo(new THREE.Vector3(2,13,0))<1e-12);
 assert.ok(manipulator.object.position.distanceTo(new THREE.Vector3(2,4,0))<1e-12,"grabbed point follows accepted rigid motion");
 assert.ok(manipulator.object.quaternion.angleTo(rotation)<1e-12);
 assert.equal(engine.assemblyConstraintMarkers.size,1);assert.equal(hydrates,0);assert.equal(a.children[0].geometry,geometry);
 assert.ok(invalidates>0);
 // The authoritative cached Product stays frozen; accepted visual frames do
 // not mutate Query state or independent definition poses.
 assert.deepEqual(engine.view.product.instances[0].translation,[0,0,0]);
 const groupMarker=[...engine.assemblyConstraintMarkers.values()][0];assert.ok(groupMarker.group.parent===engine.helpers);
 // Inspection returns body-local exact directions (nested descriptor transforms
 // have already happened server-side). Only the root rigid-unit rotation is
 // applied here: local X under +90deg Z must become world Y, not -X.
 engine.select=selection=>{engine.selected=[selection];};engine.setActiveTool=()=>{};
 assert.equal(engine.setAssemblyMoveDirection("a",[1,0,0],"line"),true);
 const frameX=new THREE.Vector3(1,0,0).applyQuaternion(manipulator.frameQuaternion());
 assert.ok(frameX.distanceTo(new THREE.Vector3(0,1,0))<1e-12,"exact local direction transformed once");
 assert.equal(engine.setAssemblyMoveDirection("a",[0,0,0],"line"),false);
 // A metadata-only authoritative revision must not pair its new Head with
 // stale render-object paths. No selection change or mesh hydration occurs.
 const path=revision=>({rootDocumentId:"assembly",canonical:"a",segments:[{instanceId:"a",ownerDocumentId:"assembly",ownerVersionId:revision,referencedDocumentId:"part",resolvedVersionId:"part-frozen",referenceMode:"PINNED"}]});
 a.userData={kind:"instance",id:"a",instanceId:"a",occurrencePath:"a",instancePath:path("old")};
 let begun;
 engine.moveInteraction={begin(input){begun=input;},cancel(){}};
 engine.updateSketchContextVisibility=()=>{};engine.applyTreeVisibility=()=>{};
 engine.moveGestureGeneration=0;
 const fresh={...engine.view,document:{...engine.view.document,versionId:"new"},resolvedInstances:[{occurrencePath:"a",instancePath:path("new")}]};
 engine.updateDisplayProjection(fresh);
 assert.deepEqual(a.userData.instancePath,path("new"));
 // Even if an obsolete UI object is supplied, begin resolves the current
 // path from the authority, not by surgically changing the old version field.
 a.userData.instancePath=path("old");
 engine.beginMovePreviewGesture();
 assert.equal(begun.baseRevisionId,"new");assert.deepEqual(begun.occurrencePath,path("new"));
 assert.equal(begun.occurrencePath.segments[0].referenceMode,"PINNED");
 assert.equal(hydrates,0);assert.equal(a.children[0].geometry,geometry);
 engine.moveNominalScene=undefined;
 // A receipt failure must not write an old nominal over a newer Head.
 engine.moveNominalBase={documentId:"assembly",revisionId:"old-head"};
 engine.moveNominalScene=new Map([["a",{position:new THREE.Vector3(100,100,100),rotation:new THREE.Quaternion(),scale:new THREE.Vector3(1,1,1)}]]);
 engine.restoreMoveNominalScene();assert.deepEqual(a.position.toArray(),[2,3,0]);assert.equal(engine.moveNominalScene,undefined);
 manipulator.dispose();engine.transforms.stopAll();
}finally{await server.close();}
