import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const drawn=[],strokes=[];
class Surface extends EventTarget {style={};tabIndex=0;setAttribute(){}focus(){}closest(){return null;}getBoundingClientRect(){return {left:0,top:0};}captures=new Set();setPointerCapture(id){this.captures.add(id);}hasPointerCapture(id){return this.captures.has(id);}releasePointerCapture(id){this.captures.delete(id);}}
globalThis.window=new EventTarget();globalThis.HTMLElement=Surface;
globalThis.document=Object.assign(new EventTarget(),{createElement:()=>({width:256,height:48,getContext:()=>({measureText:text=>({width:text.length*15}),strokeText:text=>strokes.push(text),fillText:text=>drawn.push(text)})})});
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule('three');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const {CadMaterialFactory}=await server.ssrLoadModule('/src/cad/rendering/cad-material-factory.ts');
 const {CadShaderLibrary}=await server.ssrLoadModule('/src/cad/rendering/shader/cad-shader-library.ts');
 const {CATIA_VISUAL_THEME:theme}=await server.ssrLoadModule('/src/cad/rendering/cad-visual-theme.ts');
 const {SKETCH_FEEDBACK_ORDER:order}=await server.ssrLoadModule('/src/cad/rendering/sketch-feedback-style.ts');
 const {SketchEditTool}=await server.ssrLoadModule('/src/cad/tool/sketch-edit-tool.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const {InputManager}=await server.ssrLoadModule('/src/cad/input/input-manager.ts');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {SelectionController}=await server.ssrLoadModule('/src/cad/interaction/selection-controller.ts');
 const {InputResult}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 const {LinearDimensionSketchTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const entities=[{id:'a',kind:'LINE',role:'PROFILE',start:{x:10,y:10},end:{x:30,y:10}},{id:'b',kind:'LINE',role:'PROFILE',start:{x:10,y:20},end:{x:30,y:20}}];
 const engine=Object.create(CadViewportEngine.prototype);
 Object.assign(engine,{view:{document:{id:'part',versionId:'v1',type:'PART'},part:{bodies:[],features:[{id:'sketch',type:'SKETCH',bodyId:'body',sketch:{entities,constraints:[]}}]}},activeSketchID:'sketch',sketchPlane:'XY',scene:new THREE.Scene(),renderer:{domElement:{clientWidth:800,clientHeight:600}},materials:new CadMaterialFactory(new CadShaderLibrary()),treeVisibilityOverrides:{},selectedOverlays:[],preselectedOverlays:[],invalidate(){}});
 const ref=id=>({target:'ENTITY',entityId:id,subElement:'WHOLE'});
 engine.showConstraintPreview('LENGTH',[ref('a')],20,[20,5]);const dimension=engine.referencePreview;
 engine.showReferencePreview(ref('b'),[ref('a')]);assert.equal(engine.referencePreview,dimension,'hover must not erase the dimension preview');assert(engine.referenceHover);assert.equal(engine.referenceHover.children.at(-1).material.color.getHex(),theme.hover);assert(!drawn.some(text=>text.startsWith('#')),'reference feedback is geometry only');
 assert.equal(strokes.length,0,"dimension text has no white outline");
 let textOrder;dimension.traverse(child=>{if(child.userData.sketchDimensionLabel)textOrder=child.renderOrder;});assert.equal(textOrder,order.label);
 const firstHover=engine.referenceHover;engine.showConstraintPreview('LENGTH',[ref('a')],20,[22,5]);assert.equal(engine.referenceHover,firstHover,'label placement must not erase the hovered second object');
 engine.referenceHover.traverse(child=>{if(child.isLine2)assert(child.renderOrder>textOrder);});
 engine.showSketchEditCandidate({entities:[],hitEntities:[entities[1]]});assert.equal(engine.preview.children[0].renderOrder,order.trim);assert(engine.preview.children[0].material.transparent,'trim and dimensions must use the same transparent render queue');assert.equal(engine.preview.children[0].material.linewidth,4);
 assert.equal(engine.referenceHover.parent,engine.sketchPreviewLayer);assert.equal(engine.referencePreview.parent,engine.sketchPreviewLayer);assert(engine.sketchPreviewLayer.userData.transient);engine.clearReferenceHover();assert.equal(engine.referenceHover,undefined);assert(engine.referencePreview);engine.clearReferencePreview();assert.equal(engine.referencePreview,undefined);
 engine.addTopologyOverlay('selected',{kind:'visual',entityId:'a',featureId:'sketch',documentId:'part',versionId:'v1',id:'a'});assert.equal(engine.selectedOverlays.length,1);assert.equal(engine.selectedOverlays[0].renderOrder,order.selected);
 engine.addTopologyOverlay('selected',{kind:'visual',entityId:'a',featureId:'sketch',documentId:'part',versionId:'v1',occurrencePath:'different-occurrence',id:'a'});assert.equal(engine.selectedOverlays.length,1,'equal entity IDs cannot leak across occurrences');
 engine.addTopologyOverlay('selected',{kind:'visual',entityId:'a',featureId:'sketch',documentId:'part',versionId:'v1',bodyId:'other-body',id:'a'});assert.equal(engine.selectedOverlays.length,1);
 let canonical;engine.selectMany=(items)=>{canonical=items;};engine.toolViewportPort().retainSelections([{kind:'visual',id:'a',entityId:'a',featureId:'sketch',ownerDocumentId:'part'}]);assert.equal(canonical[0].documentId,'part');assert.equal(canonical[0].versionId,'v1');assert.equal(canonical[0].bodyId,'body');
 // Real dimension object: support-plane orientation and CSS-pixel zoom scale.
 const {makeConstraintDimensionLabel}=await server.ssrLoadModule('/src/cad/rendering/sketch-constraint-renderer.ts');
 const {updateScreenLines}=await server.ssrLoadModule('/src/cad/rendering/screen-space-lines.ts');
 const {SelectionIndex}=await server.ssrLoadModule('/src/cad/interaction/selection-index.ts');
 const {resolveSketchReference}=await server.ssrLoadModule('/src/cad/interaction/sketch-reference-pick.ts');
 const {sketchMarqueeContains}=await server.ssrLoadModule('/src/cad/interaction/sketch-marquee.ts');
 const label=makeConstraintDimensionLabel('R5',([x,y])=>new THREE.Vector3(x,0,y));assert(label.isMesh&&!label.isSprite);
 const normal=new THREE.Vector3(0,0,1).applyQuaternion(label.quaternion);assert(normal.distanceTo(new THREE.Vector3(0,-1,0))<1e-12);
 const {palette,colorNumber}=await server.ssrLoadModule('/src/design/visual-tokens.ts');
 const baseText=colorNumber(palette.dimensionText);
 for(const state of ['default','hover','selected','default','hover','default']){
  engine.applyHighlight(label,state);
  assert.equal(label.material.opacity,1,'dimension glyphs never inherit translucent surface opacity');
  assert.equal(label.material.color.getHex(),state==='hover'?theme.hover:state==='selected'?theme.selected:baseText,'state color is absolute, not multiplied into a colored texture');
 }
 const {makeSketchConstraintRenderable}=await server.ssrLoadModule('/src/cad/rendering/sketch-constraint-renderer.ts');
 const {buildSketchConstraintLayout}=await server.ssrLoadModule('/src/cad/sketch/sketch-constraint-layout.ts');
 const inclined=[{id:'slant',kind:'LINE',start:{x:0,y:0},end:{x:20,y:20}},{id:'horizontal',kind:'LINE',start:{x:0,y:0},end:{x:20,y:0}}];
 const toWorld=([x,y])=>new THREE.Vector3(x,0,y),lineDimension={id:'aligned',kind:'LENGTH',references:[ref('slant')],value:Math.sqrt(800),unit:'mm'};
 for(const definition of [lineDimension,{id:'angular',kind:'ANGLE',references:[ref('horizontal'),ref('slant')],value:45,unit:'deg'}]){
  const layout=buildSketchConstraintLayout(definition,inclined),display=makeSketchConstraintRenderable(definition,inclined,toWorld,engine.materials,{width:800,height:600});
  const text=display.children.find(c=>c.userData.sketchDimensionLabel);
  const baseline=new THREE.Vector3(1,0,0).applyQuaternion(text.quaternion),expected=toWorld(layout.label.direction).normalize();
  assert(Math.abs(baseline.dot(expected))>1-1e-12,'dimension text follows its leader or angle arc tangent on the sketch plane');
  engine.applyHighlight(display,'hover');engine.applyHighlight(display,'default');assert.equal(text.material.opacity,1);assert.equal(text.material.color.getHex(),baseText);
 }
 const diagnostic=makeSketchConstraintRenderable(lineDimension,inclined,toWorld,engine.materials,{width:800,height:600},theme.sketchInvalid);
 engine.applyHighlight(diagnostic,'selected');engine.applyHighlight(diagnostic,'default');assert.equal(diagnostic.children.find(c=>c.userData.sketchDimensionLabel).material.color.getHex(),theme.sketchInvalid,'reset preserves diagnostic state color');
 const root=new THREE.Group();root.add(label);const camera=new THREE.OrthographicCamera(-50,50,50,-50,.1,1000);camera.position.set(0,-100,0);camera.up.set(0,0,1);camera.lookAt(0,0,0);updateScreenLines(root,camera,800,600);const beforeScale=label.scale.y,beforeQuaternion=label.quaternion.clone();camera.zoom=2;camera.updateProjectionMatrix();camera.position.set(50,-100,30);camera.lookAt(0,0,0);updateScreenLines(root,camera,800,600);assert(Math.abs(label.scale.y-beforeScale/2)<1e-12);assert(label.quaternion.equals(beforeQuaternion),'orbit never billboards a sketch label');
 const coincident=[{...entities[0],id:'construction',role:'CONSTRUCTION'},entities[0]];
 for(const sequence of [coincident,[...coincident].reverse()])for(const scale of [.2,1,20])assert.equal(resolveSketchReference({x:20*scale,y:10*scale},sequence,([x,y])=>({x:x*scale,y:y*scale}),'ENTITY')?.entityId,'a','coincident profile geometry wins regardless of model order/zoom/dash phase');
 assert(sketchMarqueeContains([[0,0],[10,10]],{x:-1,y:-1},{x:11,y:11}));assert(!sketchMarqueeContains([[0,0],[20,20]],{x:-1,y:-1},{x:11,y:11}));assert(sketchMarqueeContains([[-10,5],[30,5]],{x:10,y:0},{x:0,y:10}),'crossing selects through-segment even when both endpoints are outside');
 // The actual viewport preview path uses immutable candidates, not local writes.
 Object.assign(engine,{helpers:new THREE.Group(),dimensionPreviewHidden:new Set(),dimensionPreviewGeneration:0,sketchModal:{visible:true},selected:[],preselected:null,selectionIndex:new SelectionIndex(),highlightedRoots:new Set(),callbacks:{},camera,viewTransition:{cancel(){this.cancelled=true;}}});
 const geometry=new THREE.Group();geometry.userData={kind:'visual',featureId:'sketch'};engine.helpers.add(geometry);
 const promises=[];let geometryResult=[{...entities[0],end:{x:50,y:10}},entities[1]];
 engine.toolViewportPort=()=>({previewSketchOperations:(_ops,signal)=>new Promise(resolve=>promises.push({signal,resolve})),projectSketchPoint:()=>[123,234]});
 const operation={type:'UPDATE_CONSTRAINT',constraintId:'length',constraint:{id:'length',kind:'LENGTH',references:[ref('a')],value:40,unit:'mm'},parameterSource:'40 mm'};
 engine.view.part.features[0].sketch.constraints=[{...operation.constraint,value:20}];
 const old=engine.previewDimensionDefinition('sketch',[operation],new AbortController().signal);const latest=engine.previewDimensionDefinition('sketch',[operation],new AbortController().signal);promises[1].resolve({entities:geometryResult});await latest;assert(engine.dimensionDefinitionPreview);assert.equal(geometry.visible,false);assert.equal(engine.view.part.features[0].sketch.entities[0].end.x,30,'preview never updates authoritative geometry');const acceptedPreview=engine.dimensionDefinitionPreview;promises[0].resolve({entities:entities});await old;assert.equal(engine.dimensionDefinitionPreview,acceptedPreview,'late generation cannot replace newest candidate');engine.clearDimensionDefinitionPreview();assert.equal(geometry.visible,true);
 const aborted=new AbortController();const closing=engine.previewDimensionDefinition('sketch',[operation],aborted.signal);aborted.abort();engine.clearDimensionDefinitionPreview();promises.at(-1).resolve({entities:geometryResult});await closing;assert.equal(engine.dimensionDefinitionPreview,undefined,'cancelled dialog cannot acquire a late candidate');
 const pose=camera.position.clone(),rotation=camera.quaternion.clone();engine.sketchModal={setOpen(){},visible:true};engine.setSketchDialogOpen(true);assert(camera.position.equals(pose)&&camera.quaternion.equals(rotation));assert(engine.viewTransition.cancelled,'opening freezes a pending view transition without moving the camera');
 engine.sketchCommandState={input:{id:'parameter',fieldIndex:0,modelAnchor:[1,2],anchor:[0,0]},fields:[{value:'2 * width'}]};engine.updateSketchParameterAnchor();assert.deepEqual(engine.sketchCommandState.input.anchor,[123,234]);assert.equal(engine.sketchCommandState.fields[0].value,'2 * width','navigation reprojects parameter targeting without resetting its source');
 // Native scene registration, model picking and marquee share real identities.
 const {DEFAULT_CAPTURE_SETTINGS}=await server.ssrLoadModule('/src/cad/interaction/capture-settings.ts');
 delete engine.selectMany;delete engine.toolViewportPort;
 Object.assign(engine,{activeToolID:'select',captureSettings:DEFAULT_CAPTURE_SETTINGS,screenStableReferences:new Map(),pointer:new THREE.Vector2(),raycaster:new THREE.Raycaster(),navigation:{target:new THREE.Vector3()},selectable:new Map(),moveInteraction:{hasUncommittedFinal:false},moveManipulator:{detach(){}},updateSketchContextVisibility(){},applyTreeVisibility(){},cancelMovePreviewGesture(){},emitDebugState(){},callbacks:{selectionsChanged:items=>{canonical=items;}}});engine.renderer.getPixelRatio=()=>1;
 camera.position.set(0,0,100);camera.up.set(0,1,0);camera.lookAt(0,0,0);camera.zoom=1;camera.updateProjectionMatrix();camera.updateMatrixWorld();
 const feature=engine.view.part.features[0];feature.sketch.solve={};feature.sketch.entities.unshift({...entities[0],id:'support',role:'CONSTRUCTION'});feature.sketch.constraints[0].labelPosition={x:20,y:5};engine.addSketch(feature,true,engine.view);engine.scene.add(engine.helpers);
 const rendered=engine.selectable.get('visual:root:sketch:a'),support=engine.selectable.get('visual:root:sketch:support');assert.equal(rendered.renderOrder,order.geometry);assert.equal(support.renderOrder,order.construction);assert(support.material.transparent&&rendered.material.transparent);
 const screen=([x,y])=>{const p=new THREE.Vector3(x,y,0).project(camera);return [(p.x+1)*400,(1-p.y)*300];};
 assert.equal(engine.hitTest(...screen([20,10])).entityId,'a','normal viewport selection agrees with typed role picking at overlapping construction geometry');assert.equal(engine.hitTest(...screen([20,5])).kind,'sketch-constraint','dimension text remains explicitly selectable');
 engine.selectSketchMarquee({x:470,y:230},{x:650,y:250},false);assert.deepEqual(engine.selected.map(selection=>selection.entityId),['support','a']);assert.equal(canonical[1].documentId,'part');assert.equal(canonical[1].bodyId,'body');engine.selectSketchMarquee({x:470,y:170},{x:650,y:190},true);assert.deepEqual(engine.selected.map(selection=>selection.entityId),['support','a','b']);
 // Real tool phases publish separate candidate/selection feedback, with no writes on hover.
 let hit=ref('b'),state;const hover=[],sent=[],selection=[];
 const port={hasActiveSketch:()=>true,currentSketchIdentity:()=>({documentId:'part',sketchId:'sketch',versionId:'v1'}),currentSketchEntities:()=>entities,currentSketchReferenceEntities:()=>entities,currentSketchConstraints:()=>[],currentSelections:()=>[],sketchReferenceAt:(_x,_y,kind)=>hit&&kind==="LINE"?{...hit,subElement:"DIRECTION"}:hit,sketchEntityAt:()=>hit?{kind:'visual',id:hit.entityId,entityId:hit.entityId,featureId:'sketch',ownerDocumentId:'part'}:null,sketchPlacementPoint:(x,y)=>[x,y],setSketchCommandState:s=>{state=s;},setToolPrompt(){},retainSelections:s=>selection.push(s),showReferencePreview:(candidate,retained)=>hover.push({candidate,retained}),clearReferenceHover:()=>hover.push(null),clearReferencePreview(){},clearToolPreview(){},showSketchEntityPreview(){},showConstraintPreview(){},measureDimension:()=>20,commitSketchOperations:ops=>{sent.push(ops);},finishToolUse(){}};
 const manager=new ToolManager({viewport:port});manager.register(new SketchEditTool('move'));manager.register(new SketchEditTool('mirror'));manager.register(new LinearDimensionSketchTool());
 const surface=new Surface(),navigation={pointerDown:()=>InputResult.Ignored,pointerMove:()=>InputResult.Ignored,pointerUp:()=>InputResult.Ignored,keyDown:()=>InputResult.Ignored,keyChanged:()=>InputResult.Ignored,cancel(){},wantsPointerPriority:()=>false};
 const input=new InputManager(surface,new InteractionRouter(manager,new SelectionController(()=>assert.fail('selection cannot steal a command gesture'),()=>{},()=>{}),navigation));
 const pointer=type=>{const e=new Event(type,{cancelable:true});Object.assign(e,{clientX:20,clientY:10,pointerId:1,pointerType:'mouse',button:0,buttons:type==='pointerdown'?1:0,ctrlKey:false,metaKey:false,shiftKey:false,altKey:false});surface.dispatchEvent(e);};
 const click=()=>{pointer('pointerdown');pointer('pointerup');};
 manager.activate('sketch.edit.move');pointer('pointermove');assert.equal(hover.at(-1).candidate.entityId,'b');click();assert.deepEqual(state.selectedIds,['b']);assert.equal(selection.at(-1)[0].entityId,'b');hit=null;pointer('pointermove');assert.equal(hover.at(-1),null);
 hit=ref('a');manager.activate('sketch.edit.mirror');click();hit=ref('b');pointer('pointermove');assert.equal(hover.at(-1).candidate.entityId,'b');assert.equal(hover.at(-1).retained[0].entityId,'a');assert.equal(state.selectedIds.length,0);assert.equal(sent.length,0);
 manager.activate('sketch.dimension.linear');hit=ref('a');click();hit=ref('b');pointer('pointermove');assert.equal(hover.at(-1).candidate.entityId,'b');assert.deepEqual(state.references,[ref('a')],'hover cannot accept the second dimension reference');assert.equal(sent.length,0);
 input.dispose();
 console.log('PASS production sketch feedback: independent hover/dimensions, no role badges, selected overlays, move/mirror/linear roles');
} finally {await server.close();}
