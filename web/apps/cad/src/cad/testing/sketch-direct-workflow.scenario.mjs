import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {createInterface} from 'node:readline';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const live=process.env.OCCCCAD_SKETCH_REPLAY==='1';
const inbox=live?createInterface({input:process.stdin})[Symbol.asyncIterator]():undefined;
const receive=async()=>{const {value,done}=await inbox.next();if(done)throw new Error('production bridge closed');return JSON.parse(value);};
let queue=Promise.resolve();
const request=message=>{const result=queue.then(async()=>{process.stdout.write(JSON.stringify(message)+'\n');const reply=await receive();if(reply.error)throw new Error(reply.error);return reply;});queue=result.catch(()=>{});return result;};
class Surface extends EventTarget {
 style={};tabIndex=-1;captures=new Set();setAttribute(){}focus(){}closest(){return null;}getBoundingClientRect(){return {left:0,top:0};}
 setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}releasePointerCapture(id){this.captures.delete(id);const e=new Event('lostpointercapture');Object.assign(e,{pointerId:id});this.dispatchEvent(e);}
}
const storage=new Map();globalThis.localStorage={getItem:key=>storage.get(key)??null,setItem:(key,value)=>storage.set(key,value),removeItem:key=>storage.delete(key)};
globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.window.localStorage=globalThis.localStorage;globalThis.document=new EventTarget();document.visibilityState='visible';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {InputManager}=await server.ssrLoadModule('/src/cad/input/input-manager.ts');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {SelectionController}=await server.ssrLoadModule('/src/cad/interaction/selection-controller.ts');
 const {SketchEditTool}=await server.ssrLoadModule('/src/cad/tool/sketch-edit-tool.ts');
 const {RectangleSketchTool,CircleSketchTool,LineSketchTool,SelectTool,LinearDimensionSketchTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {cornerClickIntent}=await server.ssrLoadModule('/src/cad/tool/sketch-direct-input.ts');
 const {resolveSketchReference}=await server.ssrLoadModule('/src/cad/interaction/sketch-reference-pick.ts');
 const {measureSketchDimension}=await server.ssrLoadModule('/src/cad/sketch/sketch-constraint-layout.ts');
 const {sketchEntityPoint}=await server.ssrLoadModule('/src/cad/sketch/sketch-geometry.ts');
 const {useWorkbenchStore}=await server.ssrLoadModule('/src/state/workbench-store.ts');
 const {defaultSketchToolMode}=await server.ssrLoadModule('/src/cad/sketch/sketch-inline-parameter-input.tsx');
 const {InputResult}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 let view=live?await receive():{document:{id:'part',versionId:'v1'},part:{features:[{id:'sketch',type:'SKETCH',sketch:{entities:[{id:'bottom',kind:'LINE',role:'PROFILE',start:{x:10,y:10},end:{x:130,y:10}},{id:'right',kind:'LINE',role:'PROFILE',start:{x:130,y:10},end:{x:130,y:80}}],constraints:[]}}]}};
 const featureId=view.part.features.find(f=>f.type==='SKETCH').id;
 useWorkbenchStore.getState().beginSketch(featureId,"XY");
 const sketch=()=>view.part.features.find(f=>f.id===featureId).sketch;
 let fakeSubmit;let selected=[],source='selection',state,snap,pending=0,finished=0,browseClicks=0,unit='mm';const trace=[];
 const visual=id=>({kind:'visual',visualType:'CURVE',id,entityId:id,featureId,ownerDocumentId:view.document.id,versionId:view.document.versionId});
 const project=p=>({x:p[0]*4+100,y:p[1]*4+100});
 const world=(x,y)=>[(x-100)/4,(y-100)/4];
 const visible=()=>sketch().entities.filter(e=>!e.suppressed&&e.visible!==false);
 const pick=(x,y,kind,retained,allowed)=>resolveSketchReference({x,y},visible(),project,kind,6,150,retained,allowed);
 const port={hasActiveSketch:()=>true,currentSketchIdentity:()=>({documentId:view.document.id,versionId:view.document.versionId,sketchId:featureId}),currentSketchEntities:visible,currentSketchReferenceEntities:visible,currentSketchConstraints:()=>sketch().constraints,
 currentSelections:()=>selected,currentSelectionSource:()=>source,consumeActivationSelection:()=>{source='command';},currentLengthUnit:()=>unit,
 projectSketchPoint:p=>{const q=project(p);return [q.x,q.y];},sketchReferenceAt:pick,sketchPlacementPoint:world,sketchPoint:(x,y)=>{snap=pick(x,y,'POINT');const entity=snap?.entityId&&visible().find(e=>e.id===snap.entityId);return entity&&snap?sketchEntityPoint(entity,snap.subElement)??world(x,y):world(x,y);},sketchSnapReference:()=>snap,
 sketchEntityAt:(x,y)=>{const ref=pick(x,y,'ENTITY');return ref?.entityId?visual(ref.entityId):null;},selectionAt:(x,y)=>{const ref=pick(x,y,'ENTITY');return ref?.entityId?visual(ref.entityId):null;},
 retainSelections:next=>{selected=structuredClone(next);source='command';},
 setSketchCommandState:next=>{state=next?structuredClone(next):undefined;if(next){selected=next.selectedIds.map(visual);source='command';}},
 setToolPrompt:prompt=>trace.push({type:'prompt',prompt}),showPolylinePreview:points=>trace.push({type:'polyline',points}),showReferenceDimensions:values=>trace.push({type:'dimensions',values}),
 showPointPreview:point=>trace.push({type:'point',point}),showReferencePreview:(ref,refs)=>trace.push({type:'references',ref,refs}),showConstraintPreview:(kind,refs,value,pos)=>trace.push({type:'dimension',kind,refs,value,pos}),
 showSketchEntityPreview:entities=>trace.push({type:'geometry-preview',entities}),showSketchEditCandidate:candidate=>trace.push({type:'edit-preview',candidate}),clearToolPreview:()=>trace.push({type:'clear-preview'}),clearReferencePreview:()=>trace.push({type:'clear-reference'}),
 measureDimension:(kind,refs)=>measureSketchDimension(kind,refs,visible()),beginDimensionDrag:()=>false,cancelDimensionDrag:()=>{},
 finishToolUse:exit=>{finished++;if(exit)useWorkbenchStore.getState().setActiveTool("select","once");else useWorkbenchStore.getState().completeToolUse();if(manager.activeToolID!==useWorkbenchStore.getState().activeToolID)manager.activate(useWorkbenchStore.getState().activeToolID,"result");},
 commitSketchOperations:(operations,intent)=>{const frozen=structuredClone(operations);trace.push({type:'commit',operations:frozen,intent});pending++;return (live?request({kind:'operation',featureId,operations:frozen,intent}):fakeSubmit?fakeSubmit():Promise.resolve({...view,document:{...view.document,versionId:'v'+(finished+2)}})).then(updated=>{view=updated;source='result';return updated;}).finally(()=>pending--);},
 previewSketchOperations:(operations,signal)=>request({kind:'preview',featureId,operations}).then(preview=>{if(signal?.aborted)throw new Error('cancelled');const candidate=preview.sketchCandidates.find(c=>c.featureId===featureId);return {featureId,versionId:preview.baseVersionId,entities:candidate.entities};})};
 const manager=new ToolManager({viewport:port});for(const tool of [new SelectTool(),new RectangleSketchTool(),new CircleSketchTool(),new LineSketchTool(),new LinearDimensionSketchTool(),...['mirror','fillet','chamfer','split','trim','construction'].map(kind=>new SketchEditTool(kind))])manager.register(tool);
 const navigation={pointerDown:()=>InputResult.Ignored,pointerMove:()=>InputResult.Ignored,pointerUp:()=>InputResult.Ignored,keyDown:()=>InputResult.Ignored,keyChanged:()=>InputResult.Ignored,cancel:()=>{},wantsPointerPriority:()=>false};
 const controller=new SelectionController(()=>browseClicks++,()=>{},()=>{}),router=new InteractionRouter(manager,controller,navigation),surface=new Surface(),input=new InputManager(surface,router);
 const pointer=(type,p,buttons=0)=>{const screen=project(p),e=new Event(type,{cancelable:true});Object.assign(e,{clientX:screen.x,clientY:screen.y,pointerId:1,pointerType:'mouse',button:0,buttons,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.dispatchEvent(e);};
 const click=p=>{pointer('pointerdown',p,1);pointer('pointerup',p);};
 const key=(value,type='keydown')=>{const e=new Event(type,{cancelable:true});Object.assign(e,{key:value,code:value,repeat:false,isComposing:false,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});Object.defineProperty(e,'target',{value:surface});window.dispatchEvent(e);};
 const start=(id,continuous=false)=>{useWorkbenchStore.getState().setActiveTool('select','once');manager.activate('select','result');selected=[];source='result';useWorkbenchStore.getState().setActiveTool(id,defaultSketchToolMode(id,continuous));manager.activate(id);};
 const settle=async()=>{for(let i=0;i<10000;i++){await new Promise(r=>setTimeout(r,2));if(!pending){await new Promise(r=>setImmediate(r));return;}}throw new Error('submission did not complete');};
 const midpoint=e=>[(e.start.x+e.end.x)/2,(e.start.y+e.end.y)/2];
 if(!live){
  assert(cornerClickIntent([{id:'edge',kind:'LINE',role:'PROFILE',start:{x:10,y:0},end:{x:30,y:0}},{id:'arc',kind:'ARC',role:'PROFILE',center:{x:0,y:0},radius:10,endAngle:Math.PI/2}],[[20,0],[7,7]]),'zero omitted arc start is still a real shared endpoint');
  const support={id:'old-edge',kind:'LINE',role:'CONSTRUCTION',start:{x:10,y:10},end:{x:130,y:10}};
  const segment={id:'trimmed-edge',kind:'LINE',role:'PROFILE',start:{x:15,y:10},end:{x:125,y:10}};
  const cursor=project([70,10]);assert.equal(resolveSketchReference(cursor,[support,segment],project,'LINEAR_DIMENSION',6).entityId,segment.id);
  assert.equal(resolveSketchReference(cursor,[support],project,'LINEAR_DIMENSION',6).entityId,support.id);
  // A real active preselection, including construction, is always a source.
  selected=[visual('bottom')];source='selection';useWorkbenchStore.getState().setActiveTool('sketch.edit.mirror','once');manager.activate('sketch.edit.mirror');assert.equal(state.role,'选择镜像轴');assert.deepEqual(state.selectedIds,['bottom']);click([0,25]);await settle();assert.equal(manager.activeToolID,'select');assert.equal(trace.filter(e=>e.type==='commit').length,1);trace.length=0;
  visible().find(e=>e.id==='bottom').role='CONSTRUCTION';selected=[visual('bottom')];source='selection';useWorkbenchStore.getState().setActiveTool('sketch.edit.mirror','once');manager.activate('sketch.edit.mirror');assert.equal(state.role,'选择镜像轴');assert.deepEqual(state.selectedIds,['bottom']);click([0,25]);await settle();assert.equal(manager.activeToolID,'select');visible().find(e=>e.id==='bottom').role='PROFILE';trace.length=0;
  start('sketch.edit.mirror');assert.equal(state.role,'选择镜像轴');click([0,25]);assert.equal(state.role,'选择要镜像的元素');click([70,10]);key('Control','keyup');assert.equal(state.selectedIds.length,1);click([70,10]);assert.equal(state.selectedIds.length,0);click([70,10]);key('Enter');await settle();assert.equal(manager.activeToolID,'select');assert.equal(trace.filter(e=>e.type==='commit').length,1);assert.equal(browseClicks,0);
  start('sketch.edit.fillet',true);click([80,10]);assert.equal(state.role,'选择第二条曲线');click([130,40]);assert(state.input);assert.equal(state.fields[0].value,'5');const frozen=structuredClone(state.references);pointer('pointermove',[20,80]);assert.deepEqual(state.references,frozen);manager.commandAction({type:'field',index:0,value:'-'});assert(state.input);manager.commandAction({type:'confirm'});assert(state.error);assert.equal(trace.filter(e=>e.type==='commit').length,1);manager.commandAction({type:'field',index:0,value:'5 mm'});key('Enter');await settle();assert.equal(manager.activeToolID,'sketch.edit.fillet');assert.equal(state.role,'选择第一条曲线');assert.equal(trace.at(-1).type==='commit',false);key('Escape');assert.equal(manager.activeToolID,'select');
  start('sketch.edit.mirror');click([0,25]);click([70,10]);
  fakeSubmit=()=>Promise.reject(new Error('server rejected this branch'));key('Enter');await settle();assert.equal(state.phase,'failed');assert.deepEqual(state.selectedIds,['bottom']);assert.equal(manager.activeToolID,'sketch.edit.mirror');
  fakeSubmit=()=>Promise.reject(Object.assign(new Error('receipt unknown'),{code:'NETWORK_ERROR'}));key('Enter');await settle();assert.equal(state.phase,'unknown');const originalRequest=trace.filter(e=>e.type==='commit').at(-1);
  let acceptRequest;fakeSubmit=()=>new Promise(resolve=>{acceptRequest=resolve;});manager.commandAction({type:'retry'});key('Enter');key('Enter');const retryRequest=trace.filter(e=>e.type==='commit').at(-1);assert.equal(retryRequest.intent.requestId,originalRequest.intent.requestId);assert.equal(retryRequest.intent.baseVersionId,originalRequest.intent.baseVersionId);assert.deepEqual(retryRequest.operations,originalRequest.operations);
  // Cancelling the waiting UI cannot make the already sent response finish a new tool.
  key('Escape');start('sketch.edit.split');acceptRequest({...view,document:{...view.document,versionId:'late-confirmed'}});await settle();assert.equal(manager.activeToolID,'sketch.edit.split');assert.equal(state.operation,'分割');fakeSubmit=undefined;
  input.dispose();console.log('direct production input rules PASS; Worker replay has a separate required integration entry');
 } else {
  start('sketch.rectangle');click([10,10]);click([130,80]);await settle();assert.equal(sketch().entities.filter(e=>e.kind==='LINE').length,4);
  start('sketch.edit.fillet',true);
  for(const corner of ['lower-right','upper-right','upper-left','lower-left']){
    const lines=visible().filter(e=>e.role==='PROFILE'&&e.kind==='LINE');
    const horizontal=lines.filter(e=>Math.abs(e.end.y-e.start.y)<1e-6).sort((a,b)=>midpoint(a)[1]-midpoint(b)[1]);
    const vertical=lines.filter(e=>Math.abs(e.end.x-e.start.x)<1e-6).sort((a,b)=>midpoint(a)[0]-midpoint(b)[0]);
    const a=corner.startsWith('lower')?horizontal[0]:horizontal.at(-1),b=corner.endsWith('right')?vertical.at(-1):vertical[0];
    click(midpoint(a));assert.equal(state.role,'选择第二条曲线');click(midpoint(b));assert(state.input,'two curve clicks must directly open R input');assert.equal(state.fields[0].value.startsWith('5'),true);
    if(corner==='lower-right')manager.commandAction({type:'field',index:0,value:'5'});
    const before=finished;key('Enter');await settle();assert.equal(finished,before+1);assert.equal(manager.activeToolID,'sketch.edit.fillet');assert.equal(state.role,'选择第一条曲线');
  }
  assert.equal(visible().filter(e=>e.role==='PROFILE'&&e.kind==='ARC').length,4);
  for(const rounded of visible().filter(e=>e.role==='PROFILE'&&e.kind==='ARC')){assert(Math.abs(rounded.radius-5)<1e-6);assert(Math.abs(Math.abs((rounded.endAngle??0)-(rounded.startAngle??0))-Math.PI/2)<1e-6);}
  start('sketch.dimension.linear');const longLines=visible().filter(e=>e.role==='PROFILE'&&e.kind==='LINE'&&Math.abs(e.end.y-e.start.y)<1e-6).sort((a,b)=>midpoint(a)[1]-midpoint(b)[1]);click(midpoint(longLines[0]));assert.equal(state.completion.label,'锁定长度');click([midpoint(longLines[0])[0],midpoint(longLines[0])[1]-12]);assert(state.input);manager.commandAction({type:'field',index:0,value:'80 mm'});key('Enter');await settle();
  start('sketch.dimension.linear');const parallels=visible().filter(e=>e.role==='PROFILE'&&e.kind==='LINE'&&Math.abs(e.end.y-e.start.y)<1e-5).sort((a,b)=>midpoint(a)[1]-midpoint(b)[1]);click(midpoint(parallels[0]));click(midpoint(parallels.at(-1)));assert.equal(state.phase,'placement');assert.equal(state.references.length,2);click([Math.max(...parallels.map(e=>Math.max(e.start.x,e.end.x)))+18,midpoint(parallels[0])[1]]);manager.commandAction({type:'field',index:0,value:'50 mm'});key('Enter');await settle();
  const outer=visible().filter(e=>e.role==='PROFILE'&&e.kind==='LINE'),xs=outer.flatMap(e=>[e.start.x,e.end.x]),ys=outer.flatMap(e=>[e.start.y,e.end.y]),center=[(Math.max(...xs)+Math.min(...xs))/2,(Math.max(...ys)+Math.min(...ys))/2];
  assert(Math.abs(Math.max(...xs)-Math.min(...xs)-90)<1e-5,'rounded outer width must be 90');assert(Math.abs(Math.max(...ys)-Math.min(...ys)-50)<1e-5,'rounded outer height must be 50');
  start('sketch.circle');click(center);click([center[0]+8,center[1]]);await settle();const circle=visible().find(e=>e.kind==='CIRCLE'&&e.role==='PROFILE');assert(circle);const circlePoint=t=>[circle.center.x+circle.radius*Math.cos(t*2*Math.PI),circle.center.y+circle.radius*Math.sin(t*2*Math.PI)];
  start('sketch.edit.split');const beforeA=trace.filter(e=>e.type==='commit').length;click(circlePoint(.13));assert.equal(state.role,'在同一圆上选择第二个分割位置');assert.equal(trace.filter(e=>e.type==='commit').length,beforeA);key('Escape');assert.equal(trace.filter(e=>e.type==='commit').length,beforeA);assert.equal(manager.activeToolID,'select');
  start('sketch.edit.split');click(circlePoint(.13));click(circlePoint(.62));await settle();const pieces=visible().filter(e=>e.kind==='ARC'&&Math.abs(e.radius-8)<1e-6);assert.equal(pieces.length,2,'two circle positions preserve the shape in exactly two arcs');
  const removed=pieces.find(e=>e.endAngle-e.startAngle>Math.PI),removedPoint=[removed.center.x+removed.radius*Math.cos((removed.startAngle+removed.endAngle)/2),removed.center.y+removed.radius*Math.sin((removed.startAngle+removed.endAngle)/2)];
  start('sketch.edit.trim');pointer('pointermove',removedPoint);const highlighted=trace.filter(e=>e.type==='edit-preview').at(-1);assert(highlighted?.candidate.hitEntities.length,'trim hover must show the interval');click(removedPoint);await settle();const remaining=visible().find(e=>e.kind==='ARC'&&Math.abs(e.radius-8)<1e-6);assert(remaining);assert.equal(visible().filter(e=>e.kind==='ARC'&&Math.abs(e.radius-8)<1e-6).length,1);
  const first=sketchEntityPoint(remaining,'START'),second=sketchEntityPoint(remaining,'END');start('sketch.line');click(first);click(second);await settle();const chord=visible().filter(e=>e.kind==='LINE'&&e.role==='PROFILE').find(e=>Math.hypot(e.start.x-first[0],e.start.y-first[1])<1e-5&&Math.hypot(e.end.x-second[0],e.end.y-second[1])<1e-5);assert(chord);
  selected=[visual(chord.id)];source='selection';useWorkbenchStore.getState().setActiveTool("sketch.edit.construction",defaultSketchToolMode("sketch.edit.construction"));manager.activate('sketch.edit.construction');key('Enter');await settle();assert.equal(visible().find(e=>e.id===chord.id).role,'CONSTRUCTION');
  useWorkbenchStore.getState().setActiveTool("sketch.edit.mirror",defaultSketchToolMode("sketch.edit.mirror"));manager.activate('sketch.edit.mirror');assert.equal(state.role,'选择镜像轴','construction result highlight is not an active source preselection');const chordMid=midpoint(visible().find(e=>e.id===chord.id));click(chordMid);assert.equal(state.role,'选择要镜像的元素');const freshArc=visible().find(e=>e.id===remaining.id);const arcMid=[freshArc.center.x+freshArc.radius*Math.cos((freshArc.startAngle+freshArc.endAngle)/2),freshArc.center.y+freshArc.radius*Math.sin((freshArc.startAngle+freshArc.endAngle)/2)];click(arcMid);assert.deepEqual(state.selectedIds,[remaining.id]);assert(trace.some(e=>e.type==='geometry-preview'&&e.entities.some(entity=>entity.kind==='ARC')));key('Enter');await settle();assert.equal(manager.activeToolID,'select');assert.equal(browseClicks,0,'finishing pointerup cannot select underneath a new tool');
  useWorkbenchStore.getState().endSketch();assert.equal(useWorkbenchStore.getState().activeSketchID,undefined);
  const result=await request({kind:'command',request:{type:'PAD_SKETCH',sketchId:featureId,length:10}});view=result;
  assert.equal(view.part.bodies.length,1);assert.equal(view.part.features.filter(f=>f.type==='SKETCH')[0].sketch.entities.find(e=>e.id===chord.id).role,'CONSTRUCTION');
  await request({kind:'done'});input.dispose();
 }
} finally {await server.close();}
