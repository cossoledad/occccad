import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
class Surface extends EventTarget {
 style={};tabIndex=-1;clientWidth=800;clientHeight=600;captures=new Set();setAttribute(){}closest(){return null;}focus(){}
 getBoundingClientRect(){return {left:0,top:0,width:800,height:600};}
 setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}releasePointerCapture(id){this.captures.delete(id);const e=new Event('lostpointercapture');Object.assign(e,{pointerId:id});this.dispatchEvent(e);}
}
globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.document=new EventTarget();document.visibilityState='visible';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule('three');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const {CadShaderLibrary}=await server.ssrLoadModule('/src/cad/rendering/shader/cad-shader-library.ts');
 const {SketchEditTool,resolveSplitLocation}=await server.ssrLoadModule('/src/cad/tool/sketch-edit-tool.ts');
 const {SelectTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {InputManager}=await server.ssrLoadModule('/src/cad/input/input-manager.ts');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {SelectionController}=await server.ssrLoadModule('/src/cad/interaction/selection-controller.ts');
 const {InputResult}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 const surface=new Surface(),camera=new THREE.OrthographicCamera(-50,50,50,-50,.1,1000);camera.position.set(0,0,100);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
 // Renderer alone is replaced: production viewport, handle hit tests, drag math and input transitions run unchanged.
 const engine=Object.create(CadViewportEngine.prototype);Object.assign(engine,{camera,scene:new THREE.Scene(),shaders:new CadShaderLibrary(),sketchPlane:'XY',renderer:{domElement:surface,getPixelRatio:()=>1},invalidate:()=>{}});
 const actual=engine.toolViewportPort(),scope={documentId:'part',sketchId:'sketch',versionId:'v1',instancePath:[]};
 const line={id:'line',kind:'LINE',start:{x:0,y:0},end:{x:10,y:0}},selected=[{kind:'visual',id:'visual:line',visualType:'CURVE',featureId:'sketch',entityId:'line',ownerDocumentId:'part'}];
 const states=[],requests=[],ghosts=[];let resolve,reject,picks=0;
 const port={...Object.fromEntries(Object.entries(actual).filter(([name])=>name.includes('SketchManipulator')||name.startsWith('sketchManipulator'))),currentSelections:()=>selected,currentSketchEntities:()=>[line],currentSketchConstraints:()=>[],currentSketchIdentity:()=>scope,hasActiveSketch:()=>true,
  setSketchCommandState:s=>states.push(s),setToolPrompt:()=>{},retainSelections:()=>{},showReferencePreview:()=>{},clearReferencePreview:()=>{},clearToolPreview:()=>{},showSketchEntityPreview:es=>ghosts.push(structuredClone(es)),
  finishToolUse:()=>manager.activate('select'),commitSketchOperations:(ops,intent)=>{requests.push({ops:structuredClone(ops),intent:structuredClone(intent)});return new Promise((a,b)=>{resolve=a;reject=b;});},cancelDimensionDrag:()=>{},beginDimensionDrag:()=>false,selectionAt:()=>null};
 const manager=new ToolManager({viewport:port});manager.register(new SelectTool());for(const kind of ['move','rotate','copy'])manager.register(new SketchEditTool(kind));
 const selection=new SelectionController(()=>picks++,()=>{},()=>{}),nav={wantsPointerPriority:()=>false,pointerDown:()=>InputResult.Ignored,pointerMove:()=>InputResult.Ignored,pointerUp:()=>InputResult.Ignored,cancel:()=>{},keyChanged:()=>InputResult.Ignored,wheel:()=>InputResult.Ignored,auxiliaryClick:()=>InputResult.Ignored};
 const input=new InputManager(surface,new InteractionRouter(manager,selection,nav));
 const pointer=(type,x,y,buttons=0)=>{const e=new Event(type,{cancelable:true});Object.assign(e,{clientX:x,clientY:y,pointerId:1,pointerType:'mouse',button:0,buttons,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.dispatchEvent(e);};
 const key=value=>{const e=new Event('keydown',{cancelable:true});Object.assign(e,{key:value,code:value,repeat:false,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});Object.defineProperty(e,'target',{value:surface});window.dispatchEvent(e);};
 const screen=p=>{const n=p.clone().project(camera);return [(n.x+1)*400,(1-n.y)*300];};
 const handlePoint=predicate=>{const m=engine.sketchManipulator;m.updateScale(camera,{cssWidth:800,cssHeight:600,devicePixelRatio:1});m.root.updateMatrixWorld(true);const h=m.handles.find(predicate);return screen(h.pick.getWorldPosition(new THREE.Vector3()));};
 const settle=()=>new Promise(r=>setImmediate(r));
 manager.activate('sketch.edit.move');assert.equal(states.at(-1).phase,'definition');assert.equal(states.at(-1).fields.length,0);
 let [x,y]=handlePoint(h=>h.plane?.join('')==='XY');pointer('pointerdown',x,y,1);assert(surface.captures.has(1),'actual handle raycast captures the gesture');
 const dragStateCount=states.length;pointer('pointermove',x+80,y-30,1);pointer('pointermove',x-16,y+12,1);pointer('pointermove',x+24,y-12,1);
 assert.equal(states.length,dragStateCount,'pointer trajectory only updates viewport feedback, not React command state');assert.equal(requests.length,0);assert(Math.abs(ghosts.at(-1)[0].start.x-3)<1e-9);assert(Math.abs(ghosts.at(-1)[0].start.y-2)<1e-9);assert.deepEqual(line.start,{x:0,y:0});
 key('Enter');assert.equal(requests.length,0);pointer('pointerup',x+40,y-18);assert.equal(requests.length,1);assert.equal(picks,0);assert.equal(surface.captures.size,0);
 assert.equal(requests[0].ops[0].type,'TRANSFORM_ENTITIES');assert(Math.abs(requests[0].ops[0].translation.x-5)<1e-9);assert(Math.abs(requests[0].ops[0].translation.y-3)<1e-9);
 key('Enter');assert.equal(requests.length,1);reject(Object.assign(new Error('fixed'),{code:'VALIDATION_FAILED'}));await settle();assert.equal(states.at(-1).phase,'failed');assert(engine.sketchManipulator.isAttached());assert.equal(manager.activeToolID,'sketch.edit.move');
 [x,y]=handlePoint(h=>h.plane?.join('')==='XY');pointer('pointerdown',x,y,1);pointer('pointerup',x+16,y);assert.equal(requests.length,2);scope.versionId='v2';resolve({document:{id:'part',versionId:'v2'},part:{features:[]}});await settle();assert.equal(manager.activeToolID,'select');assert(!engine.sketchManipulator.isAttached());
 manager.activate('sketch.edit.copy');[x,y]=handlePoint(h=>h.plane?.join('')==='XY');pointer('pointerdown',x,y,1);pointer('pointerup',x+8,y);assert.equal(requests.at(-1).ops[0].type,'COPY_ENTITIES');assert.equal(requests.at(-1).intent.baseVersionId,'v2');resolve({document:{id:'part',versionId:'v2'},part:{features:[]}});await settle();
 manager.activate('sketch.edit.rotate');assert.equal(engine.sketchManipulator.handles.filter(h=>h.materials.every(m=>m.visible)&&h.operation==='translate').length,0);
 [x,y]=handlePoint(h=>h.operation==='rotate');pointer('pointerdown',x,y,1);key('Escape');pointer('pointerup',x,y);assert.equal(manager.activeToolID,'select');assert.equal(requests.length,3);assert.equal(surface.captures.size,0);

 manager.activate('sketch.edit.rotate');[x,y]=handlePoint(h=>h.operation==='rotate');const [cx,cy]=screen(new THREE.Vector3(5,0,0)),rx=x-cx;
 pointer('pointerdown',x,y,1);assert(surface.captures.has(1));for(let i=1;i<=12;i++){const a=i*Math.PI/24;pointer('pointermove',cx+rx*Math.cos(a),cy-rx*6/8*Math.sin(a),1);}pointer('pointerup',cx,cy-rx*6/8);
 assert.equal(requests.length,4);assert(Math.abs(requests.at(-1).ops[0].angle-Math.PI/2)<1e-5);assert(Math.abs(requests.at(-1).ops[0].translation.x)<1e-9);resolve({document:{id:'part',versionId:'v2'},part:{features:[]}});await settle();

 // Plane frame is model-owned, not camera XY. Unknown results reuse the original receipt.
 engine.sketchPlane='XZ';camera.position.set(0,-100,0);camera.up.set(0,0,1);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
 manager.activate('sketch.edit.move');[x,y]=handlePoint(h=>h.plane?.join('')==='XY');pointer('pointerdown',x,y,1);pointer('pointerup',x+20,y-12);
 assert.equal(requests.length,5);assert(Math.abs(requests.at(-1).ops[0].translation.x-2.5)<1e-9);assert(Math.abs(requests.at(-1).ops[0].translation.y-2)<1e-9);
 const unknownRequest=structuredClone(requests.at(-1));reject(Object.assign(new Error('lost reply'),{code:'TIMEOUT'}));await settle();assert.equal(states.at(-1).phase,'unknown');
 pointer('pointerdown',x,y,1);pointer('pointerup',x+30,y);key('Enter');assert.equal(requests.length,5);assert.equal(surface.captures.size,0);
 manager.commandAction({type:'retry'});assert.equal(requests.length,6);assert.equal(requests.at(-1).intent.requestId,unknownRequest.intent.requestId);assert.deepEqual(requests.at(-1).ops,unknownRequest.ops);assert.equal(requests.at(-1).intent.retryReceipt,true);resolve({document:{id:'part',versionId:'v2'},part:{features:[]}});await settle();assert.equal(manager.activeToolID,'select');
 const target={id:'target',kind:'LINE',start:{x:0,y:0},end:{x:20,y:0}},arc={id:'arc',kind:'ARC',center:{x:5,y:-5},radius:5,startAngle:0,endAngle:Math.PI/2,endPointId:'stable-end'};
 const snap=resolveSplitLocation(target,[5.4,.3],[target,arc],p=>[p[0]*10,p[1]*10]);assert.deepEqual(snap.point,[5,0]);assert.deepEqual(snap.reference,{target:'ENTITY',entityId:'arc',subElement:'END',pointId:'stable-end'});
 const onPoint={id:'on-point',kind:'POINT',point:{x:11,y:0}};assert.equal(resolveSplitLocation(target,[11.3,.1],[target,onPoint],p=>[p[0]*10,p[1]*10]).reference.subElement,'POINT');
 const far=resolveSplitLocation(target,[8,0],[target,arc],p=>[p[0]*10,p[1]*10]);assert.equal(far.reference,undefined);
 input.dispose();engine.sketchManipulator.dispose();
}finally{await server.close();}
