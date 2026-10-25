import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
class Surface extends EventTarget{
 style={};tabIndex=-1;editable=false;captures=new Set();setAttribute(){}focus(){}closest(selector){return this.editable&&selector.includes("input,")?this:null;}getBoundingClientRect(){return {left:0,top:0};}
 setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}releasePointerCapture(id){this.captures.delete(id);const e=new Event('lostpointercapture');Object.assign(e,{pointerId:id});this.dispatchEvent(e);}
}
globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.document=new EventTarget();document.visibilityState='visible';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {InputManager}=await server.ssrLoadModule('/src/cad/input/input-manager.ts');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {SelectionController}=await server.ssrLoadModule('/src/cad/interaction/selection-controller.ts');
 const {LinearDimensionSketchTool,SelectTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {InputResult,SelectionInputResult}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 const entities=[{id:'a',kind:'LINE',start:{x:4,y:7},end:{x:14,y:7}},{id:'b',kind:'LINE',start:{x:14,y:12},end:{x:4,y:12}},{id:'slope',kind:'LINE',start:{x:4,y:7},end:{x:14,y:9}},{id:'point',kind:'POINT',point:{x:3,y:5}}];
 const scope={documentId:'part',sketchId:'sketch',versionId:'v1',occurrencePath:'product/occurrence'};
 const visual=id=>({kind:'visual',id:`visual:${id}`,visualType:id==='point'?'POINT':'CURVE',featureId:'sketch',entityId:id,ownerDocumentId:'part',occurrencePath:'product/occurrence'});
 const whole=id=>({target:'ENTITY',entityId:id,subElement:'WHOLE'});
 let hit=null,selected=[],source='selection',consumed=0,finished=0;
 const states=[],previews=[],requests=[];
 const port={hasActiveSketch:()=>true,currentSketchIdentity:()=>scope,currentSketchEntities:()=>entities,currentSelections:()=>selected,currentSelectionSource:()=>source,consumeActivationSelection:()=>{consumed++;source='command';},
  sketchReferenceAt:()=>hit,sketchPlacementPoint:(x,y)=>[x,y],setSketchCommandState:state=>states.push(state?structuredClone(state):undefined),setToolPrompt:()=>{},showReferencePreview:()=>{},clearReferencePreview:()=>{},clearToolPreview:()=>{},
  showConstraintPreview:(kind,refs,value,position)=>previews.push({kind,refs:structuredClone(refs),value,position}),measureDimension:kind=>kind==='LENGTH'?10:5,
  requestDimensionCreation:(...args)=>requests.push(structuredClone(args)),commitSketchOperations:()=>assert.fail('placement must use the common editor'),
  finishToolUse:()=>{finished++;manager.activate('select','result');},beginDimensionDrag:()=>false,cancelDimensionDrag:()=>{},selectionAt:()=>null};
 const manager=new ToolManager({viewport:port});manager.register(new SelectTool());manager.register(new LinearDimensionSketchTool());
 const selection=new SelectionController(()=>{},()=>{},()=>{}),nav={wantsPointerPriority:()=>false,pointerDown:()=>InputResult.Ignored,pointerMove:()=>InputResult.Ignored,pointerUp:()=>InputResult.Ignored,cancel:()=>{},keyChanged:()=>InputResult.Ignored,wheel:()=>InputResult.Ignored,auxiliaryClick:()=>InputResult.Ignored};
 const router=new InteractionRouter(manager,selection,nav),surface=new Surface(),input=new InputManager(surface,router);
 const pointer=(type,x,y,buttons=0)=>{const event=new Event(type,{cancelable:true});Object.assign(event,{clientX:x,clientY:y,pointerId:1,pointerType:'mouse',button:0,buttons,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.dispatchEvent(event);};
 const click=(x,y)=>{pointer('pointerdown',x,y,1);pointer('pointerup',x,y);};
 const key=(key,type='keydown',target=surface,repeat=false)=>{const event=new Event(type,{cancelable:true});Object.assign(event,{key,code:key,repeat,isComposing:false,keyCode:0,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});Object.defineProperty(event,'target',{value:target});window.dispatchEvent(event);return event;};
 const settle=()=>new Promise(resolve=>setImmediate(resolve));
 const start=()=>{selected=[];source='selection';manager.activate('select');manager.activate('sketch.dimension.linear');};
 start();hit=whole('a');click(6,7);assert.equal(states.at(-1).completion.label,'锁定长度');hit=whole('b');pointer('pointermove',6,12);assert.equal(previews.at(-1).kind,'LENGTH');assert.deepEqual(states.at(-1).references.map(r=>r.entityId),['a']);
 key('Shift','keyup');key('Alt','keyup');assert.equal(states.at(-1).references.length,1);click(6,12);assert.equal(states.at(-1).phase,'placement');
 hit=whole('slope');pointer('pointermove',11,11);click(11,11);assert.equal(requests.length,1);assert.deepEqual(requests[0].slice(0,5),['DISTANCE',[whole('a'),whole('b')],5,'mm',[11,11]]);assert.equal(finished,1);
 // A nonparallel second line cannot change the retained single-line definition.
 start();hit=whole('a');click(6,7);hit=whole('slope');click(10,8);assert(states.at(-1).error.includes('平行'));assert.deepEqual(states.at(-1).references.map(r=>r.entityId),['a']);hit=null;click(15,15);assert.deepEqual(requests.at(-1).slice(0,5),['LENGTH',[whole('a')],10,'mm',[15,15]],'exact millimetres reach the editor, whose existing quantity input owns display units');
 start();hit=whole('a');click(6,7);key('Enter');assert.equal(states.at(-1).phase,'placement');const count=requests.length;key('Enter','keydown',surface,true);assert.equal(requests.length,count,'autorepeat cannot place a dimension');key('Backspace');assert.equal(states.at(-1).completion.label,'锁定长度');key('Backspace');assert.equal(states.at(-1).references.length,0);key('Escape');
 selected=[visual('a')];source='selection';manager.activate('sketch.dimension.linear');assert.equal(consumed,1);assert.equal(states.at(-1).references.length,1);key('Escape');manager.activate('sketch.dimension.linear');assert.equal(states.at(-1).references.length,0);assert.equal(consumed,1);
 assert.equal(manager.selectionInput([visual('b')],'command'),SelectionInputResult.Unhandled);assert.equal(manager.selectionInput([visual('b')],'result'),SelectionInputResult.Unhandled);key('Escape');
 selected=[visual('a'),visual('b')];source='selection';manager.activate('sketch.dimension.linear');assert.equal(states.at(-1).phase,'placement');key('Escape');selected=[visual('slope')];source='result';manager.activate('sketch.dimension.linear');assert.equal(states.at(-1).references.length,0);key('Escape');
 start();hit=whole('a');click(6,7);scope.versionId='changed-head';hit=null;const previous=requests.length;click(15,15);assert.equal(requests.length,previous);assert(states.at(-1).error.includes('版本'));key('Escape');
 const unrelated={kind:'sketch-constraint',id:'constraint-selection',featureId:'sketch',constraintId:'other-dimension',ownerDocumentId:'part',occurrencePath:'product/occurrence'};
 start();assert.equal(manager.selectionInput([visual('a'),unrelated]),SelectionInputResult.Rejected);assert.equal(states.at(-1).references.length,0);key('Escape');
 // The actual Engine tree-capture and render-projection path must retain manual provenance.
 key('Escape');scope.occurrencePath='product/occurrence';
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const engine=Object.create(CadViewportEngine.prototype);
 Object.assign(engine,{tools:manager,selected:[],selectionSource:'result',callbacks:{selectionsChanged:()=>{},sketchCommandChanged:()=>{}},moveInteraction:{hasUncommittedFinal:false},moveManipulator:{detach:()=>{}},cancelMovePreviewGesture:()=>{},updateSketchContextVisibility:()=>{},applyTreeVisibility:()=>{},refreshInteractionHighlights:()=>{},emitDebugState:()=>{},invalidate:()=>{},visibleSketchReferenceEntities:()=>entities,sketchEntitySelection:entity=>visual(entity.id)});
 const enginePort=engine.toolViewportPort();port.currentSelections=enginePort.currentSelections;port.currentSelectionSource=enginePort.currentSelectionSource;port.consumeActivationSelection=enginePort.consumeActivationSelection;
 port.setSketchCommandState=state=>{states.push(state?structuredClone(state):undefined);enginePort.setSketchCommandState(state);};
 assert.equal(engine.captureToolSelections([visual('a')]),false,'manual tree selection in Select is not consumed by a CAD tool');assert.equal(engine.selectionSource,'selection');
 engine.selectMany([visual('a')],false);assert.equal(engine.selectionSource,'selection','store-to-render notify=false projection preserves explicit tree provenance');manager.activate('sketch.dimension.linear');assert.deepEqual(states.at(-1).references.map(ref=>ref.entityId),['a'],'actual Engine command publication may clear global selections but frozen tree seed survives');assert.equal(engine.selectionSource,'command');
 hit=whole('b');assert.equal(engine.captureToolSelections([visual('b')]),true);assert.deepEqual(states.at(-1).references.map(ref=>ref.entityId),['a','b']);const committedSelectionSource=engine.selectionSource;assert.equal(engine.captureToolSelections([visual('slope')]),true,'rejected placement-stage tree pick is consumed');assert.equal(engine.selectionSource,committedSelectionSource,'rejected pick does not relabel old command highlights as manual preselection');key('Escape');
 engine.selectMany([visual('a')],true,'result');manager.activate('sketch.dimension.linear');assert.equal(states.at(-1).references.length,0,'actual result-origin selection cannot seed a new tool');key('Escape');
 input.dispose();console.log('PASS linear selection, parallel validation, shared editor handoff, cancellation and frozen scope');
}finally{await server.close();}
