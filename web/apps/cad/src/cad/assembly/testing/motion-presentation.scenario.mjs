import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import ts from "typescript";
const source=readFileSync(new URL("../motion-presentation.ts",import.meta.url),"utf8");
const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replace('"./assembly-engineering-state"',JSON.stringify(new URL("../assembly-engineering-state.ts",import.meta.url).href));
const {motionPresentations,translationLabel,displayNumber,displayLength}=await import(`data:text/javascript;base64,${Buffer.from(code).toString("base64")}`);
import {makeMotionMarkers,disposeMotionMarkers} from "../motion-markers.ts";
import {createServer} from "vite";
const view={document:{id:"p",versionId:"r"},product:{instances:[{id:"a",name:"基体"},{id:"b",name:"滑块"},{id:"c",name:"未覆盖"}],constraints:[]}};
const f={bodyId:"b",kind:4,linearizationPose:{translation:[10,0,0],rotation:[0,0,0,1]},translationDirections:[[1,0,0],[0,1,0]],rotations:[{direction:[0,0,1],axisPoint:[10,0,5],pitch:0}],translationDof:2,rotationDof:1,allowedBasis:[],blockedBasis:[],rankThreshold:1e-8};
const evidence={documentId:"p",revisionId:"r",available:true,referenceSemantics:"STATIC_RELATIVE",coordinateFrame:"OWNING_PRODUCT",lengthUnit:"mm",components:[{bodyIds:["a","b"],gaugeDof:0,solved:true,relativeDof:3,freedoms:[{...f,bodyId:"a",kind:0},f]}]};
const raw=JSON.stringify(evidence),rows=motionPresentations(view,evidence);
assert.equal(rows[0].status,"已定位");assert.match(rows[1].description,/XY.*法向旋转/);assert.equal(rows[2].status,"待计算");
assert.equal(JSON.stringify(evidence),raw);
for(const [kind,description] of [[1,"绕图示轴旋转"],[2,"沿装配 X 方向平移"],[3,"沿轴平移并绕该轴旋转"],[5,"绕图示中心转动"],[6,"自由移动和旋转"],[7,"组合运动"]]){
 const e=structuredClone(evidence);e.components[0].freedoms[1]={...f,kind,translationDirections:[[1,0,0]]};assert.equal(motionPresentations(view,e)[1].description,description);
}
const ungrounded=structuredClone(evidence);ungrounded.components[0].gaugeDof=6;ungrounded.components[0].freedoms=[{...f,bodyId:"a",kind:0,relativeToBodyId:"a"},{...f,kind:0,relativeToBodyId:"a"}];
const rigid=motionPresentations(view,ungrounded);assert.equal(rigid[0].status,"可运动");assert.match(rigid[0].description,/整体/);assert.doesNotMatch(rigid[0].reference,/基体/);assert.match(rigid[1].description,/刚性.*整体可动/);
const whole=makeMotionMarkers(rigid[0],10);assert.equal(whole.children.length,12,"numeric anchor exposes component-wide rigid motion without inventing relative DOF");disposeMotionMarkers(whole);assert.equal(ungrounded.components[0].freedoms[0].kind,0);
const drag=structuredClone(ungrounded);drag.referenceSemantics="INTERACTION_PHYSICAL";drag.components[0].freedoms[0].kind=6;drag.components[0].freedoms[0].relativeToBodyId="";assert.equal(motionPresentations(view,drag)[0].reference,"当前装配框架");
assert.equal(motionPresentations(view,{...evidence,revisionId:"old"})[0].status,"结果已失效");
assert.equal(motionPresentations(view,{...evidence,referenceSemantics:undefined})[0].status,"待计算");
const unsolved=structuredClone(evidence);unsolved.components[0].solved=false;assert.equal(motionPresentations(view,unsolved)[0].status,"待计算");
const q=Math.SQRT1_2;for(const plane of [[[q,q,0],[-q,q,0]],[[0,-1,0],[-1,0,0]]])assert.match(translationLabel(plane),/XY/);
assert.equal(displayNumber(-1e-10),"0");assert.equal(displayNumber(0.000012),"0.000012");
assert.equal(displayLength(0.000012,"mm"),"0.000012 mm");assert.equal(displayLength(0.000012,"m"),"1.2e-8 m");
assert.notEqual(motionPresentations(view,evidence,"first")[0].id,motionPresentations(view,evidence,"second")[0].id,"shared subProduct occurrences have distinct presentation identities");
const marker=makeMotionMarkers(rows[1],10);assert.equal(marker.children.length,6);assert.deepEqual(marker.children[4].position.toArray(),[10,0,0],"offset rotation axis not world Z");
disposeMotionMarkers(marker);assert.equal(JSON.stringify(evidence),raw);
for(const count of [0,1,100,500,1000]){const large={...view,product:{instances:Array.from({length:count},(_,i)=>({id:`b${i}`,name:"同名"})),constraints:[]}};const items=motionPresentations(large);assert.equal(items.length,count);assert.equal(new Set(items.map(i=>i.id)).size,count);assert.ok(items.every(i=>i.status==="待计算"));}
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const engine=Object.create(CadViewportEngine.prototype);let frames=0;
 const scene=new THREE.Scene(),camera=new THREE.PerspectiveCamera();camera.position.z=100;
 const body=new THREE.Mesh(new THREE.BoxGeometry(1,1,1),new THREE.MeshBasicMaterial());scene.add(body);const geometry=body.geometry;
 Object.assign(engine,{scene,camera,view,navigation:{target:new THREE.Vector3()},invalidate:()=>frames++});
 engine.showRemainingMotion(rows[1]);assert.equal(scene.children.length,2);assert.equal(engine.motionMarkers.children.length,6);
 engine.view={...view,document:{...view.document,versionId:"new"}};
 engine.showRemainingMotion(rows[1]);assert.equal(scene.children.length,1,"stale evidence cannot create markers");
 engine.view=view;engine.editContext={view,occurrencePath:"nested",translation:[20,0,0],rotation:[0,0,Math.SQRT1_2,Math.SQRT1_2]};
 engine.showRemainingMotion(rows[1]);assert.equal(scene.children.length,1,"same child definition in wrong occurrence rejected");
 const nested=motionPresentations(view,evidence,"nested")[1];engine.showRemainingMotion(nested);
 const point=new THREE.Vector3(10,0,0).applyMatrix4((engine.motionMarkers.updateMatrixWorld(true),engine.motionMarkers.matrixWorld));assert.ok(point.distanceTo(new THREE.Vector3(20,10,0))<1e-10,"one owning Product to scene transform");
 let projected=0;Object.assign(engine,{renderGeometrySignature:"same",geometrySignature:()=>"same",visualGeneration:0,moveInteraction:{invalidate(){}},updateDisplayProjection(){projected++;},visuals:{hydrate(){throw new Error("motion guides must not hydrate geometry");}}});
 engine.render(view,{...engine.editContext,translation:[30,0,0]});
 const movedPoint=new THREE.Vector3(10,0,0).applyMatrix4((engine.motionMarkers.updateMatrixWorld(true),engine.motionMarkers.matrixWorld));assert.ok(movedPoint.distanceTo(new THREE.Vector3(30,10,0))<1e-10,"retained owning evidence follows outer rigid occurrence without double transform");assert.equal(projected,1);assert.equal(body.geometry,geometry);
 Object.assign(engine,{selected:[],preselected:null,cancelMovePreviewGesture(){},updateSketchContextVisibility(){},applyTreeVisibility(){},moveManipulator:{detach(){}},refreshInteractionHighlights(){},emitDebugState(){}});
 engine.selectMany([{kind:"instance",id:"b",instanceId:"b",occurrencePath:"nested/b"}],false);assert.equal(scene.children.length,2,"locating the same motion unit must not erase its new marker");
 engine.selectMany([{kind:"instance",id:"a",instanceId:"a",occurrencePath:"nested/a"}],false);assert.equal(scene.children.length,1,"switching component clears markers");
 engine.showRemainingMotion(nested);
 engine.showRemainingMotion();assert.equal(scene.children.length,1);assert.equal(body.geometry,geometry);assert.ok(frames>=5);assert.equal(JSON.stringify(evidence),raw);
}finally{await server.close();}
