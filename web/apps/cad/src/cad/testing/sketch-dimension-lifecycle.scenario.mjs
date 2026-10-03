import assert from "node:assert/strict";
import { createRequire } from "node:module";
import {readFileSync,existsSync} from "node:fs";
import {dirname,resolve} from "node:path";
import {fileURLToPath} from "node:url";
const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType:"custom", logLevel:"silent", server:{middlewareMode:true} });
try {
 const {SketchReferenceSelectionSession}=await server.ssrLoadModule("/src/cad/sketch/sketch-reference-selection-session.ts");
 const accepted=[],highlights=[],cancelled=[];
 let currentVersion="v1";
 const session=new SketchReferenceSelectionSession({version:()=>currentVersion,validate:(candidate,slot)=>candidate.entityId!=="wrong"&&slot===1,onAccept:(candidate,slot)=>accepted.push([candidate,slot]),onHighlight:candidate=>highlights.push(candidate),onCancel:()=>cancelled.push(true)});
 const first=session.begin(1);session.preview(first,{target:"ENTITY",entityId:"wrong",subElement:"WHOLE"});
 assert.equal(highlights.at(-1),undefined);assert.equal(session.accept(first,{target:"ENTITY",entityId:"wrong",subElement:"WHOLE"}),false);
 assert.equal(session.active,true,"incompatible pick must keep replacement active");
 const replacement={target:"ENTITY",entityId:"new",subElement:"START",pointId:"stable-point"};
 session.preview(first,replacement);assert.deepEqual(highlights.at(-1),replacement);
 assert.equal(session.accept(first,replacement),true);assert.deepEqual(accepted,[[replacement,1]]);assert.equal(session.active,false);
 const late=session.begin(1);session.cancel();assert.equal(session.accept(late,replacement),false,"late response after Esc cannot update refs");
 const stale=session.begin(1);currentVersion="v2";assert.equal(session.accept(stale,replacement),false,"version change invalidates pick");
 const older=session.begin(1),latest=session.begin(1);assert.equal(session.accept(older,replacement),false);assert.equal(session.accept(latest,replacement),true);
 assert.equal(accepted.length,2);assert.equal(highlights.at(-1),undefined);assert(cancelled.length>=2);
 const {dimensionSourceInput,dimensionDefinitionOperation,logicalDefinitionOperation,constraintReferenceChoices,validReferenceReplacement,sketchReferenceLabel,SketchDimensionCommitSession,dimensionDisplayValue}=await server.ssrLoadModule("/src/cad/sketch/sketch-dimension-editor.tsx");
 const {measureSketchDimension,sketchReferencePoint,buildSketchConstraintLayout}=await server.ssrLoadModule("/src/cad/sketch/sketch-constraint-layout.ts");
 const {resolveSketchReference}=await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
 assert.equal(dimensionSourceInput("-12","mm"),"-12 mm");assert.equal(dimensionSourceInput("2 * width","mm"),"2 * width");assert.throws(()=>dimensionSourceInput("","mm"));
 const ref=id=>({target:"ENTITY",entityId:id,subElement:"POINT"});
 const original={id:"dim",parameterId:"stable-param",kind:"HORIZONTAL_DISTANCE",unit:"mm",value:-12,references:[ref("a"),ref("b")]};
 const definition={...original,reference:true,references:[ref("b"),ref("a")]};
 const op=dimensionDefinitionOperation(definition,original,"999","-12 mm","width",false,"ORIGINAL");
 assert.equal(op.type,"UPDATE_CONSTRAINT");assert.equal(op.constraintId,"dim");assert.equal(op.constraint.parameterId,"stable-param");assert.equal(op.parameterSource,undefined);assert.equal(op.parameterKey,"width");assert.deepEqual(op.constraint.references,definition.references);
 const {parameterSourceText}=await server.ssrLoadModule("/src/features/workbench/parameter-editor.ts");
 const literalParameter={parameterId:"stable-param",key:"old_name",displayUnit:"mm",source:{literal:{siValue:0.0635}}};
 const inchSource=parameterSourceText(literalParameter,"in");assert.equal(inchSource,"2.5");assert.equal(dimensionDisplayValue(63.5,"in"),2.5);assert.equal(dimensionDisplayValue(90,"deg"),90);
 const renameInches=dimensionDefinitionOperation(original,original,inchSource,inchSource,"new_name",false,"ORIGINAL","in");assert.equal(renameInches.parameterSource,undefined,"rename in preferred units preserves the exact original Quantity/AST");assert.equal(renameInches.constraint.parameterId,literalParameter.parameterId);assert.equal(literalParameter.source.literal.siValue,0.0635);
 const editInches=dimensionDefinitionOperation(original,original,"3",inchSource,"new_name",false,"ORIGINAL","in");assert.equal(editInches.parameterSource,"3 in");const expressionInches=dimensionDefinitionOperation(original,original,"2 * width",inchSource,"new_name",false,"ORIGINAL","in");assert.equal(expressionInches.parameterSource,"2 * width");
 const restored=dimensionDefinitionOperation(original,{...original,reference:true},"-12 mm","-12 mm","width",false,"MEASUREMENT");assert.equal(restored.restoreMode,"MEASUREMENT");assert.equal(restored.parameterSource,undefined);
 const formula=dimensionDefinitionOperation(original,original,"2 * width","-12 mm","dx",false,"ORIGINAL");assert.equal(formula.parameterSource,"2 * width");assert.throws(()=>dimensionDefinitionOperation(original,original,"12","12","bad name",false,"ORIGINAL"));
 const entities=[{id:"a",kind:"POINT",point:{x:30,y:40}},{id:"b",kind:"POINT",point:{x:18,y:40}}];assert.equal(measureSketchDimension("HORIZONTAL_DISTANCE",original.references,entities),-12);assert.equal(measureSketchDimension("VERTICAL_DISTANCE",original.references,entities),0);
 const line={id:"l",kind:"LINE",start:{x:5,y:5},end:{x:20,y:5},startPointId:"stable-start",endPointId:"stable-end"};
 const picked=resolveSketchReference({x:5,y:5},[line],p=>({x:p[0],y:p[1]}),"POINT",1,0);assert.equal(picked.pointId,"stable-start");assert.equal(sketchReferencePoint({...picked,pointId:"deleted-endpoint"},new Map([[line.id,line]])),undefined);
 // A real replacement session uses the production role matrix and measurement;
 // changing the slot never substitutes its measured number for the source draft.
 const geometry=[...entities,line],sourceDraft="2 * width",parameterID=original.parameterId;
 let draftConstraint={...original,reference:true},draftMeasurement=measureSketchDimension(original.kind,original.references,geometry);
 const actualSession=new SketchReferenceSelectionSession({version:()=>"doc:v1",validate:(candidate,slot)=>validReferenceReplacement(draftConstraint,slot,candidate,geometry),onAccept:(candidate,slot)=>{draftConstraint={...draftConstraint,references:draftConstraint.references.map((ref,index)=>index===slot?candidate:ref)};draftMeasurement=measureSketchDimension(draftConstraint.kind,draftConstraint.references,geometry);},onHighlight:()=>{},onCancel:()=>{}});
 const replacementToken=actualSession.begin(1);
 assert.equal(actualSession.accept(replacementToken,{target:"ENTITY",entityId:line.id,subElement:"WHOLE"}),false,"point dimension cannot accept an entire line");
 assert.equal(actualSession.accept(replacementToken,{target:"ENTITY",entityId:line.id,subElement:"END",pointId:"deleted"}),false,"stale endpoint identity cannot replace a role");
 assert.equal(actualSession.accept(replacementToken,{target:"ENTITY",entityId:line.id,subElement:"END",pointId:line.endPointId}),true);
 assert.equal(draftMeasurement,-10);assert.equal(sourceDraft,"2 * width");assert.equal(draftConstraint.parameterId,parameterID);
 assert.equal(sketchReferenceLabel(draftConstraint.references[1],geometry),"直线 · 终点");
 const cancelledToken=actualSession.begin(1);actualSession.cancel();assert.equal(actualSession.accept(cancelledToken,ref("b")),false);assert.equal(draftMeasurement,-10);
 const secondLine={id:"l2",kind:"LINE",start:{x:0,y:0},end:{x:0,y:10}};
 const parallel={id:"stable-parallel",kind:"PARALLEL",references:[{target:"ENTITY",entityId:line.id,subElement:"DIRECTION"},{target:"ENTITY",entityId:secondLine.id,subElement:"DIRECTION"}]};
 const choices=constraintReferenceChoices(parallel,1,[line,secondLine,...entities,{id:"circle",kind:"CIRCLE",center:{x:3,y:3},radius:2}]);assert(choices.some(choice=>choice.entityId===secondLine.id));assert(!choices.some(choice=>choice.entityId==="circle"));assert(!choices.some(choice=>choice.entityId==="a"));
 const logicOperation=logicalDefinitionOperation(parallel,[parallel.references[0],{target:"SKETCH_Y_AXIS",subElement:"DIRECTION"}],true);assert.equal(logicOperation.type,"UPDATE_CONSTRAINT");assert.equal(logicOperation.constraintId,parallel.id);assert.equal(logicOperation.constraint.id,parallel.id);assert(logicOperation.constraint.suppressed);assert.equal(logicOperation.parameterSource,undefined);
 assert.throws(()=>logicalDefinitionOperation({...parallel,kind:"MIRROR"},parallel.references,false));assert.throws(()=>logicalDefinitionOperation({...parallel,internal:true},parallel.references,false));
 const coincident={id:"join",kind:"COINCIDENT",references:[ref("a"),ref("b")]};assert(constraintReferenceChoices(coincident,1,[line]).some(choice=>choice.pointId==="stable-end"&&choice.subElement==="END"));
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts"),engine=Object.create(CadViewportEngine.prototype),requests=[];
 Object.assign(engine,{activeSketchID:"sketch",sketchView:()=>({document:{id:"part",versionId:"v1"},part:{features:[{id:"sketch",sketch:{constraints:[parallel]}}]}}),selectMany:selections=>{engine.selected=structuredClone(selections);},callbacks:{dimensionEditRequested:request=>requests.push(request)}});
 assert.equal(engine.requestDimensionEdit({kind:"sketch-constraint",documentId:"part",versionId:"v1",featureId:"sketch",constraintId:parallel.id},3,4),true);assert.equal(requests[0].constraintId,parallel.id,"real viewport tree-edit entry must accept ordinary logical constraints");
 const measured=buildSketchConstraintLayout({...original,reference:true,value:undefined},entities);assert.match(measured.label.text,/不可测/);
 const receiptSession=new SketchDimensionCommitSession(),receiptCalls=[],receipts=new Map();
 let ambiguous=true;
 const applyAtomic=async(operations,intent)=>{receiptCalls.push({operations,intent});if(!receipts.has(intent.requestId))receipts.set(intent.requestId,{version:"accepted-revision",operations:structuredClone(operations)});if(ambiguous){ambiguous=false;throw Object.assign(new Error("lost response"),{code:"TIMEOUT"});}return receipts.get(intent.requestId);};
 await assert.rejects(()=>receiptSession.submit([op],"base",applyAtomic));assert(receiptSession.unknown);
 const acceptedReceipt=await receiptSession.submit([],"newer-local-view",applyAtomic);assert.equal(receiptCalls[0].intent.requestId,receiptCalls[1].intent.requestId);assert.equal(receiptCalls[1].intent.baseVersionId,"base");assert.equal(receiptCalls[0].intent.retryReceipt,undefined);assert.equal(receiptCalls[1].intent.retryReceipt,true);assert.deepEqual(receiptCalls[0].operations,receiptCalls[1].operations);assert.equal(receipts.size,1);assert.equal(acceptedReceipt.version,"accepted-revision");assert(!receiptSession.unknown);
 await assert.rejects(()=>receiptSession.submit([formula],"base",async(operations,intent)=>{receiptCalls.push({operations,intent});throw Object.assign(new Error("invalid"),{code:"VALIDATION_FAILED"});}));assert(!receiptSession.unknown);
 await receiptSession.submit([restored],"base",applyAtomic);assert.notEqual(receiptCalls.at(-2).intent.requestId,receiptCalls.at(-1).intent.requestId,"known rejection permits an edited atomic definition with a new receipt identity");
 // Exercise the actual surface -> InputManager -> modal -> Router -> ToolManager
 // path with a nonempty selection and a synchronous dimension-tool finish.
 class Surface extends EventTarget {
  style={};tabIndex=-1;captures=new Set();editable=false;
  setAttribute(){}getBoundingClientRect(){return {left:0,top:0};}closest(){return this.editable?this:null;}
  focus(){}setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}releasePointerCapture(id){this.captures.delete(id);const lost=new Event("lostpointercapture");Object.assign(lost,{pointerId:id});this.dispatchEvent(lost);}
 }
 globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.document=new EventTarget();document.visibilityState="visible";
 const {InputManager}=await server.ssrLoadModule("/src/cad/input/input-manager.ts"),{InteractionRouter}=await server.ssrLoadModule("/src/cad/interaction/interaction-router.ts"),{ToolManager}=await server.ssrLoadModule("/src/cad/tool/tool-manager.ts"),{SelectionController}=await server.ssrLoadModule("/src/cad/interaction/selection-controller.ts"),{SelectTool,LinearDimensionSketchTool}=await server.ssrLoadModule("/src/cad/tool/cad-tool.ts"),{InputResult}=await server.ssrLoadModule("/src/cad/input/input-types.ts"),{SketchModalInputController}=await server.ssrLoadModule("/src/cad/sketch/sketch-modal-input.ts");
 const scope={documentId:"part",sketchId:"sketch",versionId:"rev"},selected=[{kind:"visual",id:"visible:l",visualType:"CURVE",featureId:"sketch",entityId:line.id,ownerDocumentId:"part"}];
 let dimensionCommits=0,createdDimension,geometryCommits=0,selectionChanges=0,dragStarts=0,finishes=0,panDown=0,panMove=0,panUp=0,hoverRef=ref("b"),acceptedPicks=0,childCancels=0;
 const port={currentSelections:()=>selected,currentSketchEntities:()=>geometry,currentSketchConstraints:()=>[],currentSketchIdentity:()=>scope,hasActiveSketch:()=>true,
  sketchReferenceAt:(_x,_y,mode)=>mode==="LINEAR_DIMENSION"?undefined:({target:"ENTITY",entityId:line.id,subElement:"END",pointId:line.endPointId}),sketchPlacementPoint:(x,y)=>[x,y],measureDimension:()=>15,
  beginDimensionDrag:()=>{dragStarts++;return false;},cancelDimensionDrag:()=>{},clearToolPreview:()=>{},showPointPreview:()=>{},retainSelections:()=>{},selectionAt:()=>selected[0],
  clearReferencePreview:()=>{},showConstraintPreview:()=>{},showReferencePreview:()=>{},setToolPrompt:()=>{},commitSketchOperations:operations=>{if(operations.some(operation=>operation.type==="ADD_CONSTRAINT")){dimensionCommits++;createdDimension=structuredClone(operations);return Promise.resolve({document:{id:"part",versionId:"rev"},part:{features:[]}});}geometryCommits++;},
  requestDimensionCreation:()=>modal.setOpen(true),finishToolUse:()=>{finishes++;tools.activate("select");}};
 const tools=new ToolManager({viewport:port});tools.register(new SelectTool());tools.register(new LinearDimensionSketchTool());
 const nav={wantsPointerPriority:event=>event.state.buttons.middle,pointerDown:()=>{panDown++;return InputResult.Capture;},pointerMove:()=>{panMove++;return InputResult.Consumed;},pointerUp:()=>{panUp++;return InputResult.ReleaseCapture;},cancel:()=>{},keyChanged:()=>InputResult.Ignored,wheel:()=>InputResult.Consumed,auxiliaryClick:()=>InputResult.Consumed};
 const router=new InteractionRouter(tools,new SelectionController(()=>selectionChanges++,()=>{},()=>{}),nav);
 let blocked=false;
 const modal=new SketchModalInputController(router,nav,{pick:()=>hoverRef,blocked:()=>blocked});
 const surface=new Surface(),input=new InputManager(surface,modal,{keyDown:modal.modalKeyDown,keyUp:modal.modalKeyUp});
 const pointer=(type,x=2,y=2,buttons=0,button=0)=>{const event=new Event(type,{cancelable:true});Object.assign(event,{clientX:x,clientY:y,pointerId:1,pointerType:"mouse",button,buttons,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.dispatchEvent(event);return event;};
 const click=()=>{pointer("pointerdown",2,2,1);pointer("pointerup");};
 const key=(value,composing=false)=>{const event=new Event("keydown",{cancelable:true});Object.assign(event,{key:value,code:value,repeat:false,isComposing:composing,keyCode:composing?229:0,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});window.dispatchEvent(event);return event;};
 tools.activate("sketch.dimension.linear");pointer("pointerdown",2,2,1);assert.equal(finishes,0,"placement now requests inline value without opening a creation dialog");tools.commandAction({type:"field",index:0,value:"2 * width"});tools.commandAction({type:"confirm"});await new Promise(resolve=>setImmediate(resolve));assert.equal(dimensionCommits,1);assert.equal(createdDimension[0].type,"ADD_CONSTRAINT");assert.equal(createdDimension[0].parameterSource,"2 * width");modal.setOpen(true);pointer("pointerup");assert.equal(finishes,1);assert.equal(tools.activeToolID,"select");assert.equal(modal.visible,true);assert.equal(selectionChanges,0);assert.equal(dragStarts,0,"finish-up must not start the new Select tool");
 click();pointer("pointermove",30,30,1);pointer("pointerup",30,30);assert.equal(geometryCommits,0);assert.equal(dragStarts,0);assert.equal(selected.length,1,"visible panel keeps an existing selection without dragging it");assert(key("Delete").defaultPrevented);assert.equal(geometryCommits,0,"panel blocks model shortcuts on the still-selected entity");
 let inputDraft={...original,reference:true},inputDraftMeasurement=-12;
 const replacementRequest={featureId:"sketch",slot:1,pick:"POINT",references:original.references,allowed:candidate=>validReferenceReplacement(original,1,candidate,geometry),onCandidate:candidate=>{if(!validReferenceReplacement(original,1,candidate,geometry))return false;acceptedPicks++;inputDraft={...inputDraft,references:[inputDraft.references[0],candidate]};inputDraftMeasurement=measureSketchDimension(inputDraft.kind,inputDraft.references,geometry);return true;},onPreview:candidate=>highlights.push(candidate),onCancel:()=>childCancels++};
 const endOld=modal.begin(replacementRequest);hoverRef={target:"ENTITY",entityId:line.id,subElement:"WHOLE"};click();assert.equal(acceptedPicks,0);assert(modal.selecting);
 hoverRef={target:"ENTITY",entityId:line.id,subElement:"END",pointId:line.endPointId};pointer("pointermove");assert.deepEqual(highlights.at(-1),hoverRef);click();assert.equal(acceptedPicks,1);assert.equal(inputDraftMeasurement,-10);assert.equal(inputDraft.parameterId,"stable-param");assert(modal.visible);assert(!modal.selecting);assert.equal(highlights.at(-1),undefined);assert.equal(dragStarts,0);
 modal.begin(replacementRequest);assert(!key("Escape",true).defaultPrevented);assert(modal.selecting,"IME Escape leaves the child alive");assert(key("Escape").defaultPrevented);assert(!modal.selecting);assert(modal.visible);assert.equal(childCancels,1);
 modal.begin(replacementRequest);const endLatest=modal.begin({...replacementRequest,slot:0});endOld();assert(modal.selecting,"late old release cannot clear a newer child");endLatest();assert(!modal.selecting);
 modal.begin(replacementRequest);pointer("pointerdown",0,0,2,2);pointer("pointerup",0,0,0,2);assert(!modal.selecting);assert(modal.visible);
 pointer("pointerdown",0,0,4,1);pointer("pointermove",4,4,4,1);pointer("pointerup",4,4,0,1);assert.equal(panDown,1);assert.equal(panMove,1);assert.equal(panUp,1);assert.equal(dragStarts,0);
 modal.begin(replacementRequest);pointer("pointerdown",2,2,1);modal.setOpen(false);pointer("pointerup");assert.equal(selectionChanges,0,"parent release does not feed a stale up into Select");
 modal.setOpen(true);modal.begin(replacementRequest);hoverRef=undefined;pointer("pointerdown",2,2,1);const lost=new Event("lostpointercapture");Object.assign(lost,{pointerId:1,clientX:2,clientY:2,pointerType:"mouse",buttons:0,button:0,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.captures.delete(1);surface.dispatchEvent(lost);assert(!modal.selecting);assert.equal(surface.captures.size,0);pointer("pointerup");assert.equal(geometryCommits,0);assert.equal(selectionChanges,0);modal.setOpen(false);pointer("pointerdown",2,2,1);pointer("pointermove",20,20,1);pointer("pointerup",20,20);assert.equal(geometryCommits,1,"a fresh Select gesture resumes only after the panel closes");
 const priorDrags=dragStarts;blocked=true;tools.activate("sketch.dimension.linear");click();pointer("pointermove",20,20,1);pointer("pointerup",20,20);assert.equal(tools.activeToolID,"sketch.dimension.linear","blocked predicate does not cancel a retained tool draft");assert.equal(finishes,1);assert.equal(dragStarts,priorDrags);assert.equal(geometryCommits,1);assert(key("Delete").defaultPrevented);assert.equal(geometryCommits,1);window.dispatchEvent(new Event("blur"));assert.equal(tools.activeToolID,"sketch.dimension.linear","blur under pending receipt does not clear the retained command draft");
 pointer("pointerdown",0,0,4,1);pointer("pointermove",4,4,4,1);pointer("pointerup",4,4,0,1);assert.equal(panDown,2);assert.equal(panUp,2,"closed panel pending receipt still permits navigation");
 assert(key("Escape").defaultPrevented);assert.equal(tools.activeToolID,"select","blocked receipt permits ordinary tool Escape without clearing server intent");
 pointer("pointerdown",2,2,1);blocked=false;pointer("pointerup");assert.equal(selectionChanges,0,"a blocked down retains modal up ownership even after receipt resolution");input.dispose();
 // Render the production CommandDialog with React SSR, capturing its actual
 // section props through the JSX runtime. No replacement component or hook mock.
 const React=require("react"),{renderToStaticMarkup}=require("react-dom/server"),ts=require("typescript"),jsxRuntime=require("react/jsx-runtime"),actualSections=[];
 const componentModules=new Map();
 const captureJSX=(fn)=>(type,props,...rest)=>{const element=fn(type,props,...rest);if(type==="section"&&props?.className==="cad-command-dialog")actualSections.push(element);return element;};
 function loadComponent(file){
  if(componentModules.has(file))return componentModules.get(file).exports;
  const module={exports:{}};componentModules.set(file,module);
  const js=ts.transpileModule(readFileSync(file,"utf8").replaceAll("import.meta.env","({})"),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,jsx:ts.JsxEmit.ReactJSX,esModuleInterop:true},fileName:file}).outputText;
  const localRequire=createRequire(file);
  new Function("require","module","exports",js)(name=>{
   if(name==="react/jsx-runtime")return {...jsxRuntime,jsx:captureJSX(jsxRuntime.jsx),jsxs:captureJSX(jsxRuntime.jsxs)};
   if(!name.startsWith("."))return localRequire(name);
   const path=resolve(dirname(file),name),target=[path,`${path}.ts`,`${path}.tsx`].find(existsSync);assert(target,`missing ${name}`);return loadComponent(target);
  },module,module.exports);return module.exports;
 }
 const {CommandDialog}=loadComponent(resolve(dirname(fileURLToPath(import.meta.url)),"../overlay/floating-panel.tsx")),{App}=require("antd");
 let confirmCount=0,closeCount=0,completeConfirm;
 const html=renderToStaticMarkup(React.createElement(App,null,React.createElement(CommandDialog,{id:"test-dimension",open:true,title:"尺寸",onClose:()=>closeCount++,onConfirm:()=>{confirmCount++;return new Promise(resolve=>completeConfirm=resolve);}},React.createElement("input",{"aria-label":"尺寸草稿",value:"2 * width",readOnly:true}))));
 assert.match(html,/尺寸草稿/);assert.match(html,/2 \* width/);assert(!html.includes("<select"));
 const productionKeyDown=actualSections.at(-1).props.onKeyDown;
 const field=new Surface();field.tagName="INPUT";field.isContentEditable=false;field.closest=()=>null;
 const reactKey=(overrides={})=>{const event={key:"Enter",repeat:false,target:field,defaultPrevented:false,nativeEvent:{isComposing:false,keyCode:13},stopPropagation(){},preventDefault(){this.defaultPrevented=true;},...overrides};productionKeyDown(event);return event;};
 assert(reactKey().defaultPrevented);reactKey();assert.equal(confirmCount,1,"actual dialog confirms only once while its first request is pending");
 completeConfirm();await Promise.resolve();await Promise.resolve();
 reactKey({repeat:true});reactKey({nativeEvent:{isComposing:true,keyCode:229}});assert.equal(confirmCount,1,"held Enter and IME Enter cannot issue a new command");
 field.tagName="TEXTAREA";reactKey();field.tagName="INPUT";field.closest=()=>({});reactKey();assert.equal(confirmCount,1,"Select candidates and multiline editing keep their Enter");assert.equal(closeCount,0);
 const {SketchDimensionEditor}=loadComponent(resolve(dirname(fileURLToPath(import.meta.url)),"../sketch/sketch-dimension-editor.tsx"));
 const panelView={document:{id:"part",versionId:"v1"},part:{features:[{id:"sketch",sketch:{entities:geometry,constraints:[{...original,reference:true}]}}],parameters:[{parameterId:original.parameterId,key:"width",source:{expression:{sourceText:"2 * height"}},displayUnit:"mm"}]}};
 const panelHTML=renderToStaticMarkup(React.createElement(App,null,React.createElement(SketchDimensionEditor,{request:{mode:"edit",featureId:"sketch",constraintId:original.id,value:-12,unit:"mm",x:0,y:0},view:panelView,onClose:()=>{},onSubmit:async()=>{},onSelectReference:()=>()=>{},onHighlightReference:()=>{},onLocateReference:()=>{}})));
 assert.match(panelHTML,/草稿测量：/);assert.match(panelHTML,/-12 mm/);assert.match(panelHTML,/#1 /);assert.match(panelHTML,/#2 /);assert(!panelHTML.includes("点 1 ·"));assert.match(panelHTML,/定位基准点/);assert.match(panelHTML,/更换基准点/);assert.match(panelHTML,/更换目标点/);assert.match(panelHTML,/2 \* height/);assert(!panelHTML.includes("<select"),"production role slots do not enumerate all geometry in a native dropdown");assert(!panelHTML.includes("stable-param"),"internal parameter identity stays outside visible controls");
 const drivingPanelHTML=renderToStaticMarkup(React.createElement(App,null,React.createElement(SketchDimensionEditor,{request:{mode:"edit",featureId:"sketch",constraintId:original.id,value:-12,unit:"mm",x:0,y:0},view:{...panelView,part:{...panelView.part,features:[{id:"sketch",sketch:{entities:geometry,constraints:[original]}}]}},onClose:()=>{},onSubmit:async()=>{},onSelectReference:()=>()=>{}})));
 const initialDrivingField=drivingPanelHTML.match(/<input\b[^>]*>/)?.[0];assert.match(initialDrivingField,/aria-label="尺寸值或表达式"/);assert(!initialDrivingField.includes("disabled"),"the production dialog's first focusable input is the editable value, not alias or suppression");assert.match(drivingPanelHTML,/更多设置/);assert(!drivingPanelHTML.includes('aria-label="尺寸名称"'),"alias, suppression and delete stay in the collapsed secondary region");assert(!drivingPanelHTML.includes("删除此尺寸"));
 const inchPanelHTML=renderToStaticMarkup(React.createElement(App,null,React.createElement(SketchDimensionEditor,{request:{mode:"edit",featureId:"sketch",constraintId:original.id,value:-12,unit:"mm",x:0,y:0},view:{...panelView,part:{...panelView.part,parameters:[literalParameter]}},preferredLengthUnit:"in",onClose:()=>{},onSubmit:async()=>{},onSelectReference:()=>()=>{},onHighlightReference:()=>{},onLocateReference:()=>{}})));
 assert.match(inchPanelHTML,/2.5/);assert.match(inchPanelHTML,/-0.4724409449 in/);assert.match(inchPanelHTML,/值或表达式（in）/);
 const inchDraft={source:inchSource,parameterId:literalParameter.parameterId};const cancelInchPick=actualSession.begin(1);actualSession.cancel();assert.equal(actualSession.accept(cancelInchPick,ref("b")),false);assert.deepEqual(inchDraft,{source:"2.5",parameterId:"stable-param"});
 console.log("PASS sketch dimension lifecycle: atomic source/reference/rename/refs, signed dimensions, stable endpoints");
} finally {await server.close();}
