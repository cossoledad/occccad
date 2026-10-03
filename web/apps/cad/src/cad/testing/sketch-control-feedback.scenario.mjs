import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
class Surface extends EventTarget {
 style={};tabIndex=-1;captures=new Set();setAttribute(){}closest(){return null;}focus(){}
 getBoundingClientRect(){return {left:0,top:0};}setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}
 releasePointerCapture(id){this.captures.delete(id);const e=new Event('lostpointercapture');Object.assign(e,{pointerId:id});this.dispatchEvent(e);}
}
globalThis.localStorage={getItem:()=>null,setItem:()=>{},removeItem:()=>{}};
globalThis.HTMLElement=Surface;globalThis.window=new EventTarget();window.localStorage=globalThis.localStorage;globalThis.document=new EventTarget();document.visibilityState='visible';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {useWorkbenchStore:store}=await server.ssrLoadModule('/src/state/workbench-store.ts');
 const face={kind:'face',id:'face',documentId:'part',occurrencePath:'part-1'};
 store.getState().setSelections([face]);store.getState().setPreselection(face);
 store.getState().beginSketch('sketch',{origin:[0,0,0],normal:[0,0,1],xAxis:[1,0,0]});
 assert.equal(store.getState().selection,null);assert.deepEqual(store.getState().selections,[]);assert.equal(store.getState().preselection,null);
 const {resolveSketchSnap}=await server.ssrLoadModule('/src/cad/interaction/sketch-snap.ts');
 const {resolveSketchReference}=await server.ssrLoadModule('/src/cad/interaction/sketch-reference-pick.ts');
 const poles=[{x:20,y:20},{x:30,y:35},{x:40,y:20}];
 const control={id:'control',kind:'SPLINE',mode:'CONTROL',role:'PROFILE',poles,poleIds:['a','b','c'],degree:2,knots:[0,1],multiplicities:[3,3],weights:[1,1,1],parameterStart:0,parameterEnd:1};
 const fit={...control,id:'fit',mode:'FIT',controlPoints:[{x:50,y:50},{x:60,y:65},{x:70,y:50}],controlPointIds:['fa','fb','fc']};
 for(const entity of [control,fit]) {
  const p=entity.mode==='FIT'?entity.controlPoints[1]:poles[1];
  // Screen-space thresholds remain stable under zoom and oblique projection.
  for(const scale of [0.2,5,40]) {
   const project=([x,y])=>[x*scale,y*scale*.7];
   const snap=resolveSketchSnap([p.x+.25/scale,p.y],[entity],scale,10,11,['POINT'],project);
   assert.deepEqual(snap.point,[p.x,p.y]);assert.equal(snap.subElement,'CONTROL');assert.equal(snap.controlPointId,entity.mode==='FIT'?'fb':'b');
   const ref=resolveSketchReference({x:p.x*scale+.25,y:p.y*scale*.7},[entity],([x,y])=>({x:x*scale,y:y*scale*.7}),'EDIT_POINT');
   assert.equal(ref.controlPointId,snap.controlPointId);
  }
  assert.equal(resolveSketchSnap([p.x,p.y],[{...entity,suppressed:true}],1,10,11,['POINT']),undefined);
  assert.equal(resolveSketchSnap([p.x,p.y],[entity],1,10,11,[]),undefined);
 }
 assert.equal(resolveSketchSnap([30,35],[fit],20,10,11,['POINT']),undefined,'FIT canonical poles are not editable fit points');
 const THREE=await server.ssrLoadModule('three');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const engine=Object.create(CadViewportEngine.prototype);
 Object.assign(engine,{camera:new THREE.OrthographicCamera(-10,10,10,-10,.1,1000),navigation:{target:new THREE.Vector3(),cancel(){},setEnabled(){}},moveManipulator:{detach(){}},
  select:selection=>store.getState().setSelection(selection),animatePlaneView(){},buildSketchContext(){},updateSketchContextVisibility(){},applyTreeVisibility(){},invalidate(){},callbacks:{toolPromptChanged(){}}});
 engine.camera.position.set(0,0,100);engine.camera.updateMatrixWorld();
 engine.beginSketch('sketch','XY');
 assert.deepEqual(store.getState().selections,[],'viewport activation must not restore the support plane as selection');
 const source={kind:'visual',id:'local',entityId:'line',featureId:'sketch'};store.getState().setSelection(source);
 engine.beginSketch('sketch','XY');assert.equal(store.getState().selection,source,'same-session display refresh preserves command selection');
 store.getState().setSelection(null);
 engine.lastSketchSnap=resolveSketchSnap([30,35],[control],20,10,11,['POINT']);
 engine.sketchView=()=>({part:{features:[{id:'sketch',sketch:{externalGeometry:[]}}]}});
 assert.deepEqual(engine.toolViewportPort().sketchSnapReference(),{target:'ENTITY',entityId:'control',subElement:'CONTROL',controlPointIndex:1,controlPointId:'b'},'accepted automatic snap keeps stable control identity');
 const {makeSplineControlFeedback}=await server.ssrLoadModule('/src/cad/rendering/spline-control-feedback.ts');
 const {CadMaterialFactory}=await server.ssrLoadModule('/src/cad/rendering/cad-material-factory.ts');
 const {CadShaderLibrary}=await server.ssrLoadModule('/src/cad/rendering/shader/cad-shader-library.ts');
 const materials=new CadMaterialFactory(new CadShaderLibrary());
 for(const mode of ['FIT','CONTROL']) {
  const group=makeSplineControlFeedback(poles.map(p=>[p.x,p.y]),mode,([x,y])=>new THREE.Vector3(x,y,0),materials,{width:800,height:600},0xffffff);
  assert.equal(group.children.filter(c=>c.isLine2).length,mode==='CONTROL'?1:0);
  assert.deepEqual(group.children.filter(c=>c.isPoints).map(c=>c.material.userData.cssPointSize),[16,12]);
  group.traverse(c=>{if(c.geometry)c.geometry.dispose();if(c.material)c.material.dispose();});
 }
 const {InputManager}=await server.ssrLoadModule('/src/cad/input/input-manager.ts');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {SelectionController}=await server.ssrLoadModule('/src/cad/interaction/selection-controller.ts');
 const {SplineSketchTool,ControlSplineSketchTool,SelectTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {InputResult}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 const controls=[],requests=[];
 const port={hasActiveSketch:()=>true,currentSelections:()=>store.getState().selections,sketchPoint:(x,y)=>[x,y],sketchSnapReference:()=>undefined,
  showPolylinePreview:()=>{},showSplineControlPreview:(points,mode)=>controls.push({points:structuredClone(points),mode}),showReferenceDimensions:()=>{},
  setToolPrompt:()=>{},clearToolPreview:()=>{},clearReferencePreview:()=>{},cancelDimensionDrag:()=>{},commitSketchOperations:ops=>{requests.push(ops);return Promise.resolve();},finishToolUse:()=>manager.activate('select')};
 const manager=new ToolManager({viewport:port});manager.register(new SelectTool());manager.register(new SplineSketchTool());manager.register(new ControlSplineSketchTool());
 const nav={wantsPointerPriority:()=>false,pointerDown:()=>InputResult.Ignored,pointerMove:()=>InputResult.Ignored,pointerUp:()=>InputResult.Ignored,cancel:()=>{},keyChanged:()=>InputResult.Ignored,wheel:()=>InputResult.Ignored,auxiliaryClick:()=>InputResult.Ignored};
 const surface=new Surface(),input=new InputManager(surface,new InteractionRouter(manager,new SelectionController(()=>{},()=>{},()=>{}),nav));
 const pointer=(type,x,y,buttons=0,alt=false)=>{const e=new Event(type,{cancelable:true});Object.assign(e,{clientX:x,clientY:y,pointerId:1,pointerType:'mouse',button:0,buttons,ctrlKey:false,metaKey:false,shiftKey:false,altKey:alt});surface.dispatchEvent(e);};
 const click=(x,y)=>{pointer('pointerdown',x,y,1);pointer('pointerup',x,y);};
 const key=value=>{const e=new Event('keydown',{cancelable:true});Object.assign(e,{key:value,code:value,repeat:false,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});Object.defineProperty(e,'target',{value:surface});window.dispatchEvent(e);};
 for(const [id,mode] of [['sketch.spline','FIT'],['sketch.spline.control','CONTROL']]) {
  manager.activate(id);click(10,10);assert.deepEqual(controls.at(-1),{points:[[10,10]],mode});
  pointer('pointermove',20,30);assert.deepEqual(controls.at(-1).points,[[10,10],[20,30]]);assert.equal(requests.length,mode==='FIT'?0:1);
  click(20,30);click(40,10);key('Enter');await new Promise(r=>setImmediate(r));
  const entity=requests.at(-1)[0].entity;assert.equal(entity.mode,mode);assert.equal((mode==='FIT'?entity.controlPoints:entity.poles).length,3);assert.equal(manager.activeToolID,'select');
 }
 const hovered=[];
 port.sketchReferenceAt=(x,y,kind,retained,allowed)=>resolveSketchReference({x,y},[control],([x,y])=>({x,y}),kind,12,110,retained,allowed);
 port.showReferencePreview=ref=>hovered.push(ref);
 pointer('pointermove',30,35);assert.equal(hovered.at(-1).controlPointId,'b','Select hover exposes an off-curve control pole');
 assert.equal(requests.length,2,'control hover creates no model operation');
 const dragPoints=[];
 port.beginDimensionDrag=()=>false;port.sketchPlacementPoint=(x,y)=>[x,y];port.currentSketchEntities=()=>[control,fit];
 port.selectionAt=()=>null;port.showPointPreview=p=>dragPoints.push(p);
 port.snapSketchEditPoint=(point,excluded)=>resolveSketchSnap(point,[control,fit].filter(e=>e.id!==excluded),10,10,11,['POINT'])?.point??point;
 pointer('pointerdown',30,35,1);pointer('pointermove',60.25,65,1);assert.deepEqual(dragPoints.at(-1),[60,65]);
 pointer('pointermove',70.4,50,1,true);assert.deepEqual(dragPoints.at(-1),[70.4,50],'Alt preserves the raw accumulated target');
 pointer('pointermove',60.25,65,1);assert.deepEqual(dragPoints.at(-1),[60,65],'returning does not add snapped displacement twice');
 assert.equal(requests.length,2,'dragging is preview only');pointer('pointerup',60.25,65);
 assert.equal(requests.length,3);assert.equal(requests.at(-1)[0].controlPointId,'b');assert.deepEqual(requests.at(-1)[0].point,{x:60,y:65});
 input.dispose();
} finally {await server.close();}
