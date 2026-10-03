import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
class Surface extends EventTarget {
  style={};tabIndex=-1;captures=new Set();focusCount=0;editable=false;
  setAttribute(){}getBoundingClientRect(){return {left:0,top:0};}closest(selector){return this.editable&&selector.includes("input,")?this:null;}
  focus(){this.focusCount++;}setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}releasePointerCapture(id){this.captures.delete(id);const event=new Event("lostpointercapture");Object.assign(event,{pointerId:id});this.dispatchEvent(event);}
}
globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();globalThis.document=new EventTarget();document.visibilityState='visible';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {InputManager}=await server.ssrLoadModule('/src/cad/input/input-manager.ts');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {SelectionController}=await server.ssrLoadModule('/src/cad/interaction/selection-controller.ts');
 const {RegularPolygonSketchTool,SelectTool,polygonVertices}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {InputResult}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 const {resolveSketchSnap}=await server.ssrLoadModule('/src/cad/interaction/sketch-snap.ts');
 const requests=[],states=[],centerMarkers=[];let resolveCommit,lastSnap;
 const snapEntities=[{id:'existing-circle',kind:'CIRCLE',center:{x:13,y:29},radius:4}];
 const scope={documentId:'part',sketchId:'sketch',versionId:'v1'};
 const port={currentSelections:()=>[],currentSketchEntities:()=>snapEntities,currentSketchIdentity:()=>scope,hasActiveSketch:()=>true,sketchPoint:(x,y)=>{lastSnap=resolveSketchSnap([x,y],snapEntities,100,1,11,['CENTER']);return lastSnap?.point??[x,y];},sketchSnapReference:()=>lastSnap?.entityId?{target:'ENTITY',entityId:lastSnap.entityId,subElement:lastSnap.subElement}:undefined,
  setSketchCommandState:s=>states.push(s),setToolPrompt:()=>{},showPointPreview:p=>centerMarkers.push(p),showPolylinePreview:()=>{},showReferenceDimensions:()=>{},clearToolPreview:()=>{},clearReferencePreview:()=>{},cancelDimensionDrag:()=>{},finishToolUse:()=>manager.activate('select'),
  commitSketchOperations:ops=>{requests.push(structuredClone(ops));return new Promise(r=>{resolveCommit=r;});}};
 const manager=new ToolManager({viewport:port});manager.register(new SelectTool());manager.register(new RegularPolygonSketchTool());manager.register(new RegularPolygonSketchTool('CIRCUMSCRIBED'));
 const selection=new SelectionController(()=>{},()=>{},()=>{}),nav={wantsPointerPriority:()=>false,pointerDown:()=>InputResult.Ignored,pointerMove:()=>InputResult.Ignored,pointerUp:()=>InputResult.Ignored,cancel:()=>{},keyChanged:()=>InputResult.Ignored,wheel:()=>InputResult.Ignored,auxiliaryClick:()=>InputResult.Ignored};
 const surface=new Surface(),input=new InputManager(surface,new InteractionRouter(manager,selection,nav));
 const pointer=(type,x,y,buttons=0)=>{const e=new Event(type,{cancelable:true});Object.assign(e,{clientX:x,clientY:y,pointerId:1,pointerType:'mouse',button:0,buttons,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.dispatchEvent(e);};
 const click=(x,y)=>{pointer('pointerdown',x,y,1);pointer('pointerup',x,y);};
 const key=value=>{const e=new Event('keydown',{cancelable:true});Object.assign(e,{key:value,code:value,repeat:false,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});Object.defineProperty(e,'target',{value:surface});window.dispatchEvent(e);};
 for(const [id,mode] of [['sketch.polygon','INSCRIBED'],['sketch.polygon.circumscribed','CIRCUMSCRIBED']]){
  manager.activate(id);pointer('pointermove',13.06,29.02);assert.deepEqual(centerMarkers.at(-1),[13,29]);assert.equal(lastSnap.kind,'CENTER');assert.equal(requests.length,mode==='INSCRIBED'?0:1,'center hover is only a candidate');click(13.06,29.02);pointer('pointermove',18,29);assert.equal(requests.length,mode==='INSCRIBED'?0:1);click(18,29);assert.equal(states.at(-1).fields[0].label,'边数');
  manager.commandAction({type:'field',index:0,value:'2'});key('Enter');assert(states.at(-1).error);assert.equal(manager.activeToolID,id);
  manager.commandAction({type:'field',index:0,value:'7'});pointer('pointermove',999,999);assert.equal(states.at(-1).fields[0].value,'7');key('Enter');key('Enter');
  const op=requests.at(-1)[0];assert.equal(op.type,'CREATE_POLYGON');assert.equal(op.mode,mode);assert.equal(op.sides,7);assert.equal(op.value,5);assert.deepEqual(op.point,{x:13,y:29});assert.deepEqual(op.firstReference,{target:'ENTITY',entityId:'existing-circle',subElement:'CENTER'});
  assert.equal(requests.length,mode==='INSCRIBED'?1:2);scope.versionId+='x';resolveCommit();await new Promise(r=>setImmediate(r));assert.equal(manager.activeToolID,'select');
 }
 manager.activate('sketch.polygon');click(13,29);key('Escape');assert.equal(manager.activeToolID,'select');assert.equal(requests.length,2);assert.equal(surface.captures.size,0);
 const inside=polygonVertices([0,0],[5,0],7,'CIRCUMSCRIBED'),outside=polygonVertices([0,0],[5,0],7,'INSCRIBED');assert(Math.abs(Math.hypot(...inside[0])-5)<1e-10);assert(Math.abs(Math.hypot(...outside[0])-5/Math.cos(Math.PI/7))<1e-10);
 input.dispose();
}finally{await server.close();}
