import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import ts from "typescript";
const source=readFileSync(new URL("../motion-presentation.ts",import.meta.url),"utf8");
const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replace('"../../utils/display-number"',JSON.stringify(new URL("../../../utils/display-number.ts",import.meta.url).href)).replace('"./assembly-engineering-state"',JSON.stringify(new URL("../assembly-engineering-state.ts",import.meta.url).href));
const {motionPresentations,translationLabel,displayNumber,displayLength}=await import(`data:text/javascript;base64,${Buffer.from(code).toString("base64")}`);
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
const {makeMotionMarkers,disposeMotionMarkers}=await server.ssrLoadModule("/src/cad/assembly/motion-markers.ts");
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
assert.equal(displayLength(-7.10543e-14),"0 mm");assert.equal(displayLength(1.42109e-14),"0 mm");assert.equal(displayLength(1e-10),"0 mm");
assert.equal(displayNumber(-1e-10),"0");assert.equal(displayNumber(0.000012),"0");
assert.equal(displayLength(0.000012,"mm"),"0 mm");assert.equal(displayLength(0.000012,"m"),"0 m");
assert.notEqual(motionPresentations(view,evidence,"first")[0].id,motionPresentations(view,evidence,"second")[0].id,"shared subProduct occurrences have distinct presentation identities");
const marker=makeMotionMarkers(rows[1],10);assert.equal(marker.children.length,6);assert.deepEqual(marker.children[4].position.toArray(),[10,0,0],"offset rotation axis not world Z");
disposeMotionMarkers(marker);assert.equal(JSON.stringify(evidence),raw);
for(const count of [0,1,100,500,1000]){const large={...view,product:{instances:Array.from({length:count},(_,i)=>({id:`b${i}`,name:"同名"})),constraints:[]}};const items=motionPresentations(large);assert.equal(items.length,count);assert.equal(new Set(items.map(i=>i.id)).size,count);assert.ok(items.every(i=>i.status==="待计算"));}
try{
 const THREE=await import("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const engine=Object.create(CadViewportEngine.prototype);let frames=0;
 const scene=new THREE.Scene(),overlay=new THREE.Scene(),camera=new THREE.PerspectiveCamera();camera.position.z=100;
 const body=new THREE.Mesh(new THREE.BoxGeometry(1,1,1),new THREE.MeshBasicMaterial());scene.add(body);const geometry=body.geometry;
 Object.assign(engine,{scene,analysisScene:overlay,camera,view,navigation:{target:new THREE.Vector3()},invalidate:()=>frames++});
 engine.showRemainingMotion(rows[1]);assert.equal(overlay.children.length,1);assert.equal(engine.motionMarkers.children.length,6);
 engine.view={...view,document:{...view.document,versionId:"new"}};
 engine.showRemainingMotion(rows[1]);assert.equal(overlay.children.length,0,"stale evidence cannot create markers");
 engine.view=view;engine.editContext={view,occurrencePath:"nested",translation:[20,0,0],rotation:[0,0,Math.SQRT1_2,Math.SQRT1_2]};
 engine.showRemainingMotion(rows[1]);assert.equal(overlay.children.length,0,"same child definition in wrong occurrence rejected");
 const nested=motionPresentations(view,evidence,"nested")[1];engine.showRemainingMotion(nested);
 const point=new THREE.Vector3(10,0,0).applyMatrix4((engine.motionMarkers.updateMatrixWorld(true),engine.motionMarkers.matrixWorld));assert.ok(point.distanceTo(new THREE.Vector3(20,10,0))<1e-10,"one owning Product to scene transform");
 let projected=0;Object.assign(engine,{renderGeometrySignature:"same",geometrySignature:()=>"same",visualGeneration:0,moveInteraction:{invalidate(){}},updateDisplayProjection(){projected++;},visuals:{hydrate(){throw new Error("motion guides must not hydrate geometry");}}});
 engine.render(view,{...engine.editContext,translation:[30,0,0]});
 const movedPoint=new THREE.Vector3(10,0,0).applyMatrix4((engine.motionMarkers.updateMatrixWorld(true),engine.motionMarkers.matrixWorld));assert.ok(movedPoint.distanceTo(new THREE.Vector3(30,10,0))<1e-10,"retained owning evidence follows outer rigid occurrence without double transform");assert.equal(projected,1);assert.equal(body.geometry,geometry);
 Object.assign(engine,{selected:[],preselected:null,cancelMovePreviewGesture(){},updateSketchContextVisibility(){},applyTreeVisibility(){},moveManipulator:{detach(){}},refreshInteractionHighlights(){},emitDebugState(){}});
 engine.selectMany([{kind:"instance",id:"b",instanceId:"b",occurrencePath:"nested/b"}],false);assert.equal(overlay.children.length,1,"locating the same motion unit must not erase its new marker");
 engine.selectMany([{kind:"instance",id:"a",instanceId:"a",occurrencePath:"nested/a"}],false);assert.equal(overlay.children.length,0,"switching component clears markers");
 engine.showRemainingMotion(nested);
 engine.showRemainingMotion();assert.equal(overlay.children.length,0);assert.equal(body.geometry,geometry);assert.ok(frames>=5);assert.equal(JSON.stringify(evidence),raw);
 // CSS-pixel analysis primitives: perspective/orthographic, DPR and zoom.
 const {analysisArrow,updateAnalysisGuides,disposeAnalysisGuides}=await server.ssrLoadModule("/src/cad/rendering/analysis-guides.ts");
 for(const projection of [new THREE.PerspectiveCamera(50,2,.1,1000),new THREE.OrthographicCamera(-100,100,50,-50,.1,1000)]){
  projection.position.z=100;projection.updateMatrixWorld();
  const guides=new THREE.Scene(),arrow=analysisArrow(new THREE.Vector3(1,0,0),new THREE.Vector3(),"translation",10);guides.add(arrow);
  const shaft=arrow.children.find(c=>c.isLine2);const owned=[shaft.geometry,shaft.material,arrow.cone.geometry,arrow.cone.material];
  for(const resource of owned)resource.userData.disposals=0;
  for(const resource of owned)resource.addEventListener("dispose",()=>resource.userData.disposals++);
  for(const [height,dpr,zoom] of [[500,1,1],[500,2,1],[800,2,2],[400,1,4]]){
   projection.zoom=zoom;projection.updateProjectionMatrix();
   updateAnalysisGuides(guides,projection,{cssWidth:height*2,cssHeight:height,devicePixelRatio:dpr});
   const origin=arrow.getWorldPosition(new THREE.Vector3());const tip=arrow.cone.getWorldPosition(new THREE.Vector3());
   const cssLength=origin.clone().project(projection).sub(tip.clone().project(projection)).length()*height;
   assert.ok(Math.abs(cssLength-64)<1e-8,`64 CSS pixels: ${cssLength}`);
   assert.equal(shaft.material.linewidth,2);assert.deepEqual(shaft.material.resolution.toArray(),[height*2,height]);
   for(const material of [shaft.material,arrow.cone.material]){assert.equal(material.depthTest,false);assert.equal(material.depthWrite,false);assert.equal(material.toneMapped,false);}
   assert.equal(shaft.geometry,owned[0]);assert.equal(arrow.cone.geometry,owned[2]);
  }
  arrow.position.z=101;updateAnalysisGuides(guides,projection,{cssWidth:800,cssHeight:400,devicePixelRatio:1});assert.equal(arrow.visible,false);
  disposeAnalysisGuides(guides);for(const resource of owned)assert.equal(resource.userData.disposals,1);
 }
 // Actual viewport lifecycle: selecting blank after pointerup must neither
 // invalidate the frozen operation nor retarget it to the new UI selection.
 let resolveFinal,finishes=0,commits=0,cancels=0;
 const final=new Promise(resolve=>resolveFinal=resolve);
 const targetGroup=new THREE.Group();targetGroup.position.set(9,2,3);
 const operation=Object.create(CadViewportEngine.prototype);
 const interaction={state:"finalizing",hasUncommittedFinal:false,finish(){finishes++;return final;},committing(){this.state="committing";},committed(){this.state="committed";},cancel(){cancels++;}};
 Object.assign(operation,{view,selected:[{kind:"instance",id:"b",instanceId:"b"}],preselected:null,moveInteraction:interaction,moveSessionDocumentId:"p",moveGestureGeneration:7,
  moveTarget:{group:targetGroup,startPosition:new THREE.Vector3(),startQuaternion:new THREE.Quaternion(),startPivot:new THREE.Vector3(),localPivot:new THREE.Vector3()},
  moveManipulator:{detach(){},isDragging:()=>false,commitPreviewFrame(){},setAuthoritativePose(){}},
  callbacks:{instanceMoved:async(document,candidate)=>{commits++;assert.equal(document,"p");assert.equal(candidate.previewId,"final-current");assert.equal(candidate.target.targetPose.translation[0],9);}},
  updateSketchContextVisibility(){},applyTreeVisibility(){},refreshInteractionHighlights(){},emitDebugState(){},invalidate(){},cancelMovePreviewGesture(){cancels++;}});
 operation.finishMovePreviewGesture();operation.finishMovePreviewGesture();
 operation.selectMany([],false);assert.equal(cancels,0);assert.equal(operation.moveGestureGeneration,7);
 operation.selectMany([{kind:"instance",id:"a",instanceId:"a"}],false);
 const activated=[];Object.assign(operation,{toolActivationGeneration:0,tools:{activate:id=>activated.push(id)}});
 operation.setActiveTool("assembly.fix");operation.setActiveTool("select");assert.deepEqual(activated,[],"tool changes wait behind final solve/commit; only latest activation survives");
 let settled=false;const barrier=operation.settleAssemblyInteraction().then(()=>settled=true);
 await Promise.resolve();assert.equal(settled,false);assert.equal(finishes,1);
 resolveFinal({previewId:"final-current",target:{targetPose:{translation:[9,2,3]}}});await barrier;
 assert.equal(commits,1);assert.equal(interaction.state,"committed");assert.equal(operation.selected[0].id,"a","success does not steal selection");
 assert.deepEqual(activated,["select"]);
 operation.moveCommitPending={};await assert.rejects(operation.settleAssemblyInteraction(),/尚未保存/);
 operation.moveCommitPending=undefined;interaction.hasUncommittedFinal=true;await assert.rejects(operation.settleAssemblyInteraction(),/尚未保存/);
 interaction.hasUncommittedFinal=false;interaction.state="finalizing";
 const canceled=new Promise(resolve=>resolveFinal=resolve);interaction.finish=()=>canceled;
 delete operation.cancelMovePreviewGesture;
 operation.finishMovePreviewGesture();operation.cancelMovePreviewGesture("explicit Esc");
 assert.equal(operation.moveGestureGeneration,8);resolveFinal({previewId:"late",target:{targetPose:{translation:[99,2,3]}}});
 await operation.moveFinalization;assert.equal(commits,1,"explicit cancellation still invalidates late final result");
}finally{await server.close();}
