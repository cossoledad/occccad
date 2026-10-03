import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
  const {SketchEditTool,sketchEditPreview,copySketchSelection,pasteSketchClipboard}=await server.ssrLoadModule("/src/cad/tool/sketch-edit-tool.ts");
  const {SelectTool,LinearDimensionSketchTool}=await server.ssrLoadModule("/src/cad/tool/cad-tool.ts");
  const entities=[{id:"a",kind:"LINE",role:"PROFILE",start:{x:0,y:0},end:{x:10,y:0}},
    {id:"b",kind:"ARC",role:"PROFILE",center:{x:10,y:5},radius:5,startAngle:-Math.PI/2,endAngle:Math.PI/2},
    {id:"ellipse",kind:"ELLIPTICAL_ARC",role:"CONSTRUCTION",center:{x:20,y:10},majorRadius:10,minorRadius:4,rotation:Math.PI/3,startAngle:0.2,endAngle:2.7},
    {id:"spline",kind:"SPLINE",role:"PROFILE",mode:"CONTROL",poles:[{x:0,y:0},{x:4,y:5},{x:10,y:0}],degree:2,knots:[0,1],multiplicities:[3,3],weights:[1,0.8,1],parameterStart:0,parameterEnd:1}];
  const constraints=[{id:"connection",kind:"COINCIDENT",references:[{target:"ENTITY",entityId:"a",subElement:"END"},{target:"ENTITY",entityId:"b",subElement:"START"}]},
    {id:"driver",kind:"LENGTH",value:10,unit:"mm",parameterId:"dimension",references:[{target:"ENTITY",entityId:"a",subElement:"WHOLE"}]},
    {id:"outside",kind:"PARALLEL",references:[{target:"ENTITY",entityId:"a",subElement:"WHOLE"},{target:"ENTITY",entityId:"unselected",subElement:"WHOLE"}]}];
  let scope={documentId:"part",sketchId:"sketch",versionId:"head",occurrencePath:"outer/part"};
  let selections=[{kind:"visual",id:"visual:a",featureId:"sketch",entityId:"a",ownerDocumentId:"part",occurrencePath:"outer/part"},
    {kind:"visual",id:"visual:b",featureId:"sketch",entityId:"b",ownerDocumentId:"part",occurrencePath:"outer/part"}];
  const operations=[],previews=[],prompts=[];
  const context={viewport:{currentSketchEntities:()=>entities,currentSketchConstraints:()=>constraints,currentSketchIdentity:()=>scope,currentSelections:()=>selections,
    hasActiveSketch:()=>true,showSketchEntityPreview:value=>previews.push(value),setToolPrompt:value=>prompts.push(value),
    showPointPreview:()=>{},showReferencePreview:()=>{},clearToolPreview:()=>{},clearReferencePreview:()=>{},commitSketchOperations:value=>operations.push(value),finishToolUse:()=>{},
    selectionAt:()=>selections[0],sketchPlacementPoint:(x,y)=>[x,y],sketchReferenceAt:()=>({target:"SKETCH_Y_AXIS",subElement:"WHOLE"})}};
  const key=(tool,key,modifiers={})=>tool.keyDown({key,state:{modifiers:{ctrl:false,meta:false,shift:false,...modifiers}}},context);
  const pointer=(x,y,phase="down",button=0)=>({x,y,phase,pointerId:1,button,state:{buttons:{left:phase==="down",middle:false,right:button===2}}});
  const click=(tool,x,y)=>{tool.pointerDown(pointer(x,y),context);tool.pointerUp(pointer(x,y,"up"),context);};
  const close=(actual,expected)=>assert(Math.abs(actual-expected)<1e-10,`${actual} != ${expected}`);

  const move=new SketchEditTool("move");move.selectionInput(selections,context);key(move,"Enter");
  for(const value of ["2","0","Tab","-","5"])key(move,value);
  assert.equal(operations.length,0,"selection, input and preview cannot mutate the model");
  key(move,"Enter");const moved=operations.at(-1)[0];
  assert.equal(moved.type,"TRANSFORM_ENTITIES");assert.deepEqual(moved.entityIds,["a","b"]);assert.deepEqual(moved.translation,{x:20,y:-5});
  assert(!moved.detachConstraintIds,"tool must never silently detach restrictions");assert(moved.operationId);key(move,"Enter");assert.equal(operations.length,1,"duplicate confirmation does not repeat commit");
  const copy=new SketchEditTool("copy");copy.selectionInput(selections,context);key(copy,"Enter");key(copy,"i");key(copy,"3");key(copy,"Enter");
  assert.equal(operations.at(-1)[0].type,"COPY_ENTITIES");assert.equal(operations.at(-1)[0].constraintPolicy,"INTERNAL");
  assert.notEqual(operations.at(-1)[0].operationId,moved.operationId);

  const rotate=new SketchEditTool("rotate");rotate.selectionInput(selections,context);key(rotate,"Enter");
  click(rotate,0,0);click(rotate,10,0);rotate.pointerMove(pointer(0,10,"move"),context);
  const rotationPreview=previews.at(-1);close(rotationPreview[0].end.x,0);close(rotationPreview[0].end.y,10);
  click(rotate,0,10);close(operations.at(-1)[0].angle,Math.PI/2);
  const scale=new SketchEditTool("scale");scale.selectionInput(selections,context);key(scale,"Enter");key(scale,"0");
  const scaleCount=operations.length;key(scale,"Enter");assert.equal(operations.length,scaleCount,"nonpositive scale is rejected atomically");
  key(scale,"Escape");assert.equal(operations.length,scaleCount);
  const mirror=new SketchEditTool("mirror");mirror.selectionInput(selections,context);key(mirror,"Enter");key(mirror,"y");key(mirror,"Enter");
  assert.deepEqual(operations.at(-1)[0].axis,{target:"SKETCH_Y_AXIS",subElement:"WHOLE"});
  assert.equal(operations.at(-1)[0].type,"MIRROR_ENTITIES");assert.equal(operations.at(-1)[0].mirrorMode,"LINKED");assert.equal(operations.at(-1)[0].constraintPolicy,undefined);
  const reflected=sketchEditPreview(entities,[0,0],[0,0],0,1,{start:[2,3],end:[6,6]});
  for(let index=0;index<entities.length;index++)assert.equal(reflected[index].role,entities[index].role);
  close(reflected[1].endAngle-reflected[1].startAngle,-Math.PI);
  close(reflected[2].endAngle-reflected[2].startAngle,-2.5);
  const axisPoint=sketchEditPreview([{id:"p",kind:"POINT",role:"PROFILE",point:{x:2,y:3}}],[0,0],[0,0],0,1,{start:[2,3],end:[6,6]})[0].point;
  close(axisPoint.x,2);close(axisPoint.y,3);

  const stale=new SketchEditTool("move");stale.selectionInput(selections,context);key(stale,"Enter");
  scope={...scope,occurrencePath:"other/part"};const staleCount=operations.length;key(stale,"Enter");assert.equal(operations.length,staleCount,"occurrence session change invalidates frozen edit");
  assert(prompts.at(-1).includes("版本"));scope={...scope,occurrencePath:"outer/part"};
  const split=new SketchEditTool("split");split.selectionInput([selections[0]],context);key(split,"Enter");key(split,"Enter");
  assert.equal(operations.at(-1)[0].type,"SPLIT_ENTITY");assert.deepEqual(operations.at(-1)[0].parameters,[0.5]);
  const trim=new SketchEditTool("trim");trim.selectionInput([selections[0]],context);key(trim,"Enter");
  for(const value of ["0",".","2","Tab","0",".","8"])key(trim,value);
  const trimPreview=previews.at(-1)[0];close(trimPreview.start.x,2);close(trimPreview.end.x,8);
  key(trim,"u");key(trim,"Enter");
  assert.equal(operations.at(-1)[0].type,"TRIM_ENTITY");assert.deepEqual(operations.at(-1)[0].parameters,[0.2,0.8]);
  assert(operations.at(-1)[0].detachConstraintIds.includes("driver"),"whole-curve relation release is an explicit user action");
  const quickTrim=new SketchEditTool("quick_trim");quickTrim.selectionInput([selections[0]],context);key(quickTrim,"Enter");
  context.viewport.sketchReferenceAt=()=>({target:"EXTERNAL",entityId:"external-boundary",subElement:"WHOLE"});click(quickTrim,5,0);
  key(quickTrim,"q");key(quickTrim,"Enter");
  assert.equal(operations.at(-1)[0].type,"QUICK_TRIM");assert.deepEqual(operations.at(-1)[0].boundaryIds,["external-boundary"]);
  assert.equal(operations.at(-1)[0].trimMode,"KEEP_HIT");assert.equal(operations.at(-1)[0].hitParameter,0.5);
  assert.equal(operations.at(-1)[0].parameters,undefined,"frontend supplies interval intent rather than sampled intersection geometry");
  const right={id:"corner-right",kind:"LINE",role:"PROFILE",start:{x:10,y:0},end:{x:10,y:10}},
    top={id:"corner-top",kind:"LINE",role:"PROFILE",start:{x:10,y:10},end:{x:0,y:10}};entities.push(right,top);
  const selectCorner=id=>({...selections[0],id:`visual:${id}`,entityId:id});
  const cornerBatch=new SketchEditTool("fillet");cornerBatch.selectionInput([selectCorner("a"),selectCorner(right.id)],context);key(cornerBatch,"Enter");
  let cornerPick="a";context.viewport.sketchReferenceAt=()=>({target:"ENTITY",entityId:cornerPick,subElement:"WHOLE"});
  const beforeCorners=operations.length;click(cornerBatch,9,0);cornerPick=right.id;click(cornerBatch,10,1);click(cornerBatch,8,2);key(cornerBatch,"2");key(cornerBatch,"b");
  assert.equal(operations.length,beforeCorners,"staging a corner batch has no model side effects");
  cornerBatch.selectionInput([selectCorner(right.id),selectCorner(top.id)],context);key(cornerBatch,"Enter");cornerPick=right.id;click(cornerBatch,10,9);
  cornerPick=top.id;click(cornerBatch,9,10);click(cornerBatch,8,8);key(cornerBatch,"Enter");
  const cornerOperations=operations.at(-1);assert.equal(cornerOperations.length,2);
  assert(cornerOperations.every(operation=>operation.type==="FILLET_ENTITIES"&&operation.value===2&&operation.trimMode==="TRIM"));
  assert.notEqual(cornerOperations[0].operationId,cornerOperations[1].operationId);
  assert.deepEqual(cornerOperations[0].firstReference,{target:"ENTITY",entityId:"a",subElement:"END"});
  assert.deepEqual(cornerOperations[0].secondReference,{target:"ENTITY",entityId:right.id,subElement:"START"});
  assert.deepEqual(cornerOperations[1].firstReference,{target:"ENTITY",entityId:right.id,subElement:"END"});
  assert.deepEqual(cornerOperations[1].point,{x:8,y:8});
  const chamfer=new SketchEditTool("chamfer");chamfer.selectionInput([selectCorner("a"),selectCorner(right.id)],context);key(chamfer,"Enter");
  cornerPick="a";click(chamfer,9,0);cornerPick=right.id;click(chamfer,10,1);click(chamfer,8,2);
  key(chamfer,"m");key(chamfer,"m");for(const value of ["3","Tab","3","0"])key(chamfer,value);key(chamfer,"k");key(chamfer,"Enter");
  const chamferOperation=operations.at(-1)[0];assert.equal(chamferOperation.type,"CHAMFER_ENTITIES");assert.equal(chamferOperation.chamferMode,"LENGTH_ANGLE");
  assert.equal(chamferOperation.chamferFirst,3);assert.equal(chamferOperation.chamferAngle,30);assert.equal(chamferOperation.trimMode,"KEEP");
  const extend=new SketchEditTool("extend");extend.selectionInput([selectCorner("a")],context);key(extend,"Enter");
  cornerPick="a";click(extend,9,0);cornerPick="external-limit";context.viewport.sketchReferenceAt=()=>({target:cornerPick==="a"?"ENTITY":"EXTERNAL",entityId:cornerPick,subElement:"WHOLE"});
  click(extend,20,0);click(extend,20,1);key(extend,"Enter");
  const extendOperation=operations.at(-1)[0];assert.equal(extendOperation.type,"EXTEND_ENTITY");
  assert.deepEqual(extendOperation.firstReference,{target:"ENTITY",entityId:"a",subElement:"END"});
  assert.deepEqual(extendOperation.boundaryIds,["external-limit"]);assert.deepEqual(extendOperation.point,{x:20,y:1});
  const offset=new SketchEditTool("offset");offset.selectionInput([selectCorner("a"),selectCorner("b")],context);key(offset,"Enter");
  key(offset,"-");key(offset,"2");key(offset,"m");key(offset,"Enter");
  assert.equal(operations.at(-1)[0].type,"OFFSET_ENTITIES");assert.equal(operations.at(-1)[0].value,-2);assert.equal(operations.at(-1)[0].mode,"ROUND");
  const invalidOffset=new SketchEditTool("offset");invalidOffset.selectionInput([selectCorner("ellipse")],context);
  const invalidOffsetCount=operations.length;key(invalidOffset,"Enter");key(invalidOffset,"Enter");assert.equal(operations.length,invalidOffsetCount,"ellipse exact equidistant is rejected rather than polyline approximated");
  const complement=new SketchEditTool("complement");complement.selectionInput([selectCorner("b")],context);key(complement,"Enter");key(complement,"u");key(complement,"Enter");
  assert.equal(operations.at(-1)[0].type,"ARC_COMPLEMENT");assert(operations.at(-1)[0].detachConstraintIds.includes("connection"),"changed endpoint semantics require explicit relation release");
  const closeCurve=new SketchEditTool("close");closeCurve.selectionInput([selectCorner("ellipse")],context);key(closeCurve,"Enter");key(closeCurve,"Enter");assert.equal(operations.at(-1)[0].type,"CLOSE_CURVE");
  const deleted=new SketchEditTool("delete");deleted.selectionInput(selections,context);key(deleted,"Enter");
  assert.deepEqual(operations.at(-1),[{type:"DELETE_ENTITIES",entityIds:["a","b"]}]);

  const fit={id:"fit-edit",kind:"SPLINE",role:"PROFILE",mode:"FIT",controlPoints:[{x:0,y:0},{x:5,y:4},{x:10,y:0}],controlPointIds:["fit-first","fit-middle","fit-last"],poles:[{x:0,y:0},{x:5,y:8},{x:10,y:0}],degree:2,knots:[0,1],multiplicities:[3,3],weights:[1,1,1]};entities.push(fit);
  const insertFit=new SketchEditTool("spline_insert");insertFit.selectionInput([selectCorner(fit.id)],context);key(insertFit,"Enter");key(insertFit,"1");click(insertFit,3,7);key(insertFit,"Enter");
  assert.deepEqual({...operations.at(-1)[0],operationId:undefined},{type:"EDIT_SPLINE_POINT",entityId:fit.id,operationId:undefined,pointAction:"INSERT",pointIndex:1,point:{x:3,y:7}});
  const deleteFit=new SketchEditTool("spline_delete");deleteFit.selectionInput([selectCorner(fit.id)],context);key(deleteFit,"Enter");key(deleteFit,"1");key(deleteFit,"Enter");
  assert.equal(operations.at(-1)[0].controlPointId,"fit-middle");assert.equal(operations.at(-1)[0].pointAction,"DELETE");
  const knotInsert=new SketchEditTool("spline_insert");knotInsert.selectionInput([selectCorner("spline")],context);key(knotInsert,"Enter");for(const input of ["0",".","2","5"])key(knotInsert,input);key(knotInsert,"Enter");
  assert.equal(operations.at(-1)[0].knotParameter,0.25);assert.equal(operations.at(-1)[0].point,undefined,"control knot insertion must not masquerade as a fitted point");
  const badKnot=new SketchEditTool("spline_insert");badKnot.selectionInput([selectCorner("spline")],context);key(badKnot,"Enter");key(badKnot,"0");const beforeBadKnot=operations.length;key(badKnot,"Enter");assert.equal(operations.length,beforeBadKnot);key(badKnot,"Escape");
  const fitReference={id:"fit-point-connection",kind:"COINCIDENT",references:[{target:"ENTITY",entityId:fit.id,subElement:"CONTROL",controlPointId:"fit-middle"},{target:"ENTITY",entityId:"a",subElement:"START"}]};constraints.push(fitReference);
  const exactFitTrim=new SketchEditTool("trim");exactFitTrim.selectionInput([selectCorner(fit.id)],context);key(exactFitTrim,"Enter");assert(prompts.at(-1).includes("CONTROL"));assert(prompts.at(-1).includes("不重新拟合"));key(exactFitTrim,"u");key(exactFitTrim,"Enter");assert(operations.at(-1)[0].detachConstraintIds.includes(fitReference.id));
  const closedSpline=new SketchEditTool("spline_close");closedSpline.selectionInput([selectCorner(fit.id)],context);key(closedSpline,"Enter");key(closedSpline,"Enter");assert.equal(operations.at(-1)[0].type,"SET_SPLINE_CLOSED");assert.equal(operations.at(-1)[0].closed,true);
  const convertSpline=new SketchEditTool("spline_control");convertSpline.selectionInput([selectCorner(fit.id)],context);key(convertSpline,"Enter");key(convertSpline,"Enter");assert.equal(operations.at(-1)[0].type,"CONVERT_SPLINE_TO_CONTROL");
  // Product remains the host while all edits target the stable owned Part sketch and occurrence.
  const ownedScope=structuredClone(scope);
  const productMove=new SketchEditTool("move");productMove.selectionInput([{...selectCorner("a"),ownerDocumentId:"host-product"}],context);key(productMove,"Enter");assert.equal(operations.at(-1)[0].type,"CONVERT_SPLINE_TO_CONTROL","host identity cannot edit the owned Part geometry");
  productMove.selectionInput([selectCorner("a")],context);key(productMove,"Enter");key(productMove,"4");key(productMove,"Enter");assert.equal(operations.at(-1)[0].type,"TRANSFORM_ENTITIES");assert.deepEqual(scope,ownedScope);

  assert.equal(copySketchSelection(context),true);entities[0].start.x=99;
  assert.equal(pasteSketchClipboard(context,[3,4]),true);
  const pasted=operations.at(-1),pastedEntities=pasted.filter(op=>op.entity).map(op=>op.entity),pastedConstraints=pasted.filter(op=>op.constraint).map(op=>op.constraint);
  close(pastedEntities[0].start.x,3);close(pastedEntities[0].start.y,4);
  assert.equal(pastedConstraints.length,1,"clipboard retains internal logical connectivity with explicit driver/external policy");
  assert.deepEqual(pastedConstraints[0].references.map(reference=>reference.entityId),pastedEntities.map(entity=>entity.id));
  assert(pastedEntities.every(entity=>!entities.some(source=>source.id===entity.id)));
  const select=new SelectTool();assert.equal(key(select,"Delete"),"consumed");
  assert.equal(operations.at(-1)[0].type,"DELETE_ENTITIES");assert.equal(key(select,"c",{ctrl:true}),"consumed");
  assert.equal(key(select,"v",{ctrl:true}),"consumed");
  assert.equal(select.keyDown({key:"Delete",editableTarget:true},context),"ignored");
  const selectedGeometry=[...selections];
  selections=[{kind:"sketch-constraint",id:"dimension-selection",featureId:"sketch",constraintId:"driver",constraintType:"LENGTH",ownerDocumentId:"part",occurrencePath:"outer/part"}];
  assert.equal(key(select,"Delete"),"consumed");assert.deepEqual(operations.at(-1),[{type:"DELETE_CONSTRAINT",constraintId:"driver"}],"dimension shortcut commits formal lifecycle delete rather than deleting supporting geometry");
  const beforeOtherConstraint=operations.length;selections=[{...selections[0],ownerDocumentId:"host-product"}];assert.equal(key(select,"Delete"),"ignored");assert.equal(operations.length,beforeOtherConstraint);
  constraints.push({id:"generated-mirror",kind:"MIRROR",references:[]});selections=[{...selections[0],ownerDocumentId:"part",constraintId:"generated-mirror",constraintType:"MIRROR"}];assert.equal(key(select,"Delete"),"consumed");assert.equal(operations.length,beforeOtherConstraint,"generated reflection relation cannot be deleted as ordinary dimension");
  selections=selectedGeometry;const beforeCut=operations.length;key(select,"x",{ctrl:true});assert.equal(operations.length,beforeCut+1);assert.equal(operations.at(-1)[0].type,"DELETE_ENTITIES");assert(prompts.at(-1).includes("剪切"));assert(!prompts.at(-1).includes("修剪"));
  selections=[{...selections[0],featureId:"different-sketch"}];assert.equal(copySketchSelection(context),false,"identity scope rejects another sketch with same entity ID");
  const circleForDrag={id:"drag-circle",kind:"CIRCLE",role:"PROFILE",center:{x:10,y:20},radius:5};entities.push(circleForDrag);
  context.viewport.beginDimensionDrag=()=>false;context.viewport.cancelDimensionDrag=()=>{};
  context.viewport.showPointPreview=point=>previews.push({point});context.viewport.retainSelections=()=>{};
  context.viewport.sketchReferenceAt=()=>({target:"ENTITY",entityId:"drag-circle",subElement:"CENTER"});
  const clickOnly=new SelectTool(),clickCount=operations.length;click(clickOnly,12,20);
  assert.equal(operations.length,clickCount,"picking close to center cannot move geometry on a click");
  const trajectory=new SelectTool();trajectory.pointerDown(pointer(12,20),context);
  for(const [x,y] of [[17,25],[7,15],[112,220],[12,20]])trajectory.pointerMove(pointer(x,y,"move"),context);
  assert.deepEqual(previews.at(-1).point,[10,20],"round trip returns to frozen model point without cumulative drift");
  circleForDrag.center={x:80,y:90};trajectory.pointerMove(pointer(17,25,"move"),context);
  assert.deepEqual(previews.at(-1).point,[15,25],"accepted result cannot become a new gesture baseline");
  trajectory.pointerUp(pointer(22,30,"up"),context);
  assert.equal(operations.length,clickCount+1);assert.deepEqual(operations.at(-1)[0].point,{x:20,y:30},"pointerup contributes final target");
  const endpointDrag=new SelectTool();context.viewport.sketchReferenceAt=()=>({target:"ENTITY",entityId:"b",subElement:"END",pointId:"arc-end"});
  endpointDrag.pointerDown(pointer(10,10),context);endpointDrag.pointerMove(pointer(13,14,"move"),context);endpointDrag.pointerUp(pointer(13,14,"up"),context);
  assert.equal(operations.at(-1)[0].type,"UPDATE_ENTITY_POINT");assert.equal(operations.at(-1)[0].subElement,"END");assert.equal(operations.at(-1)[0].entityId,"b");assert.deepEqual(operations.at(-1)[0].point,{x:13,y:14});
  context.viewport.sketchReferenceAt=()=>({target:"ENTITY",entityId:"drag-circle",subElement:"CENTER"});
  const dragCancel=new SelectTool();dragCancel.pointerDown(pointer(80,90),context);dragCancel.pointerMove(pointer(90,100,"move"),context);
  const dragCancelCount=operations.length;key(dragCancel,"Escape");dragCancel.pointerUp(pointer(90,100,"up"),context);
  assert.equal(operations.length,dragCancelCount,"Escape cancels the gesture and stale final preview");
  const dragStale=new SelectTool();dragStale.pointerDown(pointer(80,90),context);scope={...scope,versionId:"new-head"};
  dragStale.pointerUp(pointer(95,110,"up"),context);assert.equal(operations.length,dragCancelCount,"changed revision cancels gesture");

  const labelTargets=[];let labelFinishes=0;
  context.viewport.beginDimensionDrag=()=>true;context.viewport.updateDimensionDrag=(x,y)=>labelTargets.push([x,y]);context.viewport.finishDimensionDrag=()=>labelFinishes++;
  const labelDrag=new SelectTool(),beforeLabel=operations.length;labelDrag.pointerDown(pointer(5,5),context);labelDrag.pointerMove(pointer(8,8,"move"),context);labelDrag.pointerUp(pointer(15,12,"up"),context);
  assert.deepEqual(labelTargets,[[8,8],[15,12]],"label gesture passes the last pointerup target to its placement session");assert.equal(labelFinishes,1);assert.equal(operations.length,beforeLabel,"dimension label gesture cannot issue value or geometry operations");context.viewport.beginDimensionDrag=()=>false;

  const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const dimensionSession=Object.create(CadViewportEngine.prototype),placementCommits=[];
  Object.assign(dimensionSession,{dimensionDrag:{selection:{featureId:"sketch"},constraint:{id:"driver"},position:[3,4],scopeKey:"old-revision"},
    dimensionGestureScope:()=>"changed-revision",clearReferencePreview:()=>{},invalidate:()=>{},callbacks:{sketchOperations:(...args)=>placementCommits.push(args)}});
  dimensionSession.finishDimensionDrag();assert.equal(placementCommits.length,0);assert.equal(dimensionSession.dimensionDrag,undefined,"actual viewport session cancels late label commit on revision/occurrence scope change");
  dimensionSession.dimensionDrag={selection:{featureId:"sketch"},constraint:{id:"driver"},position:[3,4],scopeKey:"changed-revision"};dimensionSession.finishDimensionDrag();
  assert.deepEqual(placementCommits.at(-1),["sketch",[{type:"UPDATE_CONSTRAINT_PLACEMENT",constraintId:"driver",labelPosition:{x:3,y:4}}]],"label drag only writes placement, never model quantity");
  dimensionSession.activeSketchID="sketch";dimensionSession.dimensionConstraintAt=()=>({selection:{featureId:"sketch",constraintId:"unmeasurable"},constraint:{id:"unmeasurable",reference:true,unit:"mm"}});
  let unavailableDimensionEdited=false;dimensionSession.requestDimensionEdit=()=>{unavailableDimensionEdited=true;return true;};assert.equal(dimensionSession.editDimensionAt(5,6),true);assert(unavailableDimensionEdited,"unmeasurable reference remains editable for reference replacement and restoration");

  const lineRefs=[{target:"ENTITY",entityId:"parallel-a",subElement:"WHOLE"},{target:"ENTITY",entityId:"parallel-b",subElement:"WHOLE"}];
  const {resolveSketchReference}=await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
  const tangentArc={id:"arc-tangent",kind:"ARC",center:{x:0,y:0},radius:100,startAngle:0,endAngle:Math.PI/2,startPointId:"stable-arc-start",endPointId:"stable-arc-end"};
  const derivativePick=resolveSketchReference({x:99.955,y:3},[tangentArc],p=>({x:p[0],y:p[1]}),"TANGENT_CURVE",8,0,{target:"SKETCH_Y_AXIS",subElement:"DIRECTION"});
  assert.equal(derivativePick.subElement,"START");assert.equal(derivativePick.pointId,"stable-arc-start","near endpoint tangent pick retains derivative endpoint rather than own WHOLE hit");
  const ordinaryCurve=resolveSketchReference({x:99.955,y:3},[tangentArc],p=>({x:p[0],y:p[1]}),"CURVE",8,0);assert.equal(ordinaryCurve.subElement,"WHOLE");
  const requests=[];
  context.viewport.sketchReferenceAt=()=>lineRefs.shift()??null;context.viewport.measureDimension=()=>0;
  context.viewport.showReferencePreview=()=>{};context.viewport.showConstraintPreview=()=>{};
  context.viewport.requestDimensionCreation=(kind,references,value)=>requests.push({kind,references,value});
  const dimension=new LinearDimensionSketchTool();click(dimension,0,0);click(dimension,0,10);click(dimension,5,5);
  assert.equal(requests.length,1);assert.equal(requests[0].kind,"DISTANCE");assert.equal(requests[0].value,0);
  assert.deepEqual(requests[0].references.map(reference=>reference.entityId),["parallel-a","parallel-b"],"direct two-line dimensions preserve both stable line references");
  console.log("Sketch edit selection, preview, atomic operations, parameter input, cancellation, frozen context and clipboard passed");
}finally{await server.close();}
