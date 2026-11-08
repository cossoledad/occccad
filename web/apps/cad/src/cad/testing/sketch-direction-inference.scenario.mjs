import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {inferSketchDirection:infer}=await server.ssrLoadModule('/src/cad/sketch/sketch-direction-inference.ts');
 const screen=([x,y])=>[x*10,y*10];
 assert.equal(infer([0,0],[10,.2],[],screen).kind,'HORIZONTAL');
 assert.deepEqual(infer([0,0],[.2,-10],[],screen).point,[0,-10]);
 assert.equal(infer([0,0],[10,1],[],screen),undefined,'a visibly distinct direction is not inferred');
 assert.equal(infer([0,0],[.01,0],[],screen),undefined,'a short cursor move has no direction suggestion');
 assert.equal(infer([0,0],[10,.2],[],([x,y])=>[x*100,y*100]),undefined,'same world error is rejected when it spans many screen pixels');
 const line={id:'stable-line',name:'Support',kind:'LINE',role:'PROFILE',start:{x:0,y:0},end:{x:10,y:10}};
 const parallel=infer([0,0],[10,10.2],[line],screen);assert.equal(parallel.kind,'PARALLEL');assert.equal(parallel.reference.entityId,line.id);assert(Math.abs(parallel.point[0]-parallel.point[1])<1e-12);
 const perpendicular=infer([0,0],[-10,10.1],[line],screen);assert.equal(perpendicular.kind,'PERPENDICULAR');
 assert.equal(infer([0,0],[10,10.2],[{...line,suppressed:true}],screen),undefined);
 const fixedLength=infer([0,0],[10,.2],[],screen,true);assert(Math.abs(Math.hypot(...fixedLength.point)-Math.hypot(10,.2))<1e-12);
 const {LineSketchTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 let state,committed,snap;
 const port={hasActiveSketch:()=>true,currentSketchIdentity:()=>({documentId:'part',sketchId:'sketch',versionId:'base'}),currentLengthUnit:()=> 'mm',currentSketchEntities:()=>[line],projectSketchPoint:screen,sketchPoint:(x,y)=>[x,y],sketchSnapReference:()=>snap,showPolylinePreview(){},showReferenceDimensions(){},clearToolPreview(){},setToolPrompt(){},setSketchCommandState:value=>state=value,finishToolUse(){},commitSketchOperations:operations=>{committed=operations}};
 let manager;
 const pointer=(x,y,alt=false)=>({x,y,pointerId:1,button:0,state:{buttons:{left:false,middle:false,right:false},modifiers:{alt}}});
 const click=(point,alt=false)=>{const event=pointer(...point,alt);manager.pointerDown(event);manager.pointerUp(event)};
 const draw=(first,last,{enabled=true,alt=false,angle,target}={})=>{
  committed=undefined;snap=undefined;manager.activate(undefined);manager.activate('sketch.line');manager.keyDown({key:'c'});click(first);manager.commandAction({type:'option',name:'directionInference',value:String(enabled)});snap=target;manager.pointerMove(pointer(...last,alt));
  const offered=state.role;
  if(angle)manager.commandAction({type:'field',index:1,value:angle});
  click(last,alt);assert(committed);return {operations:committed,offered};
 };
 // Use fresh tools so construction mode is independent for every case.
 const create=(first,last,options)=>{manager=new ToolManager({viewport:port});manager.register(new LineSketchTool());return draw(first,last,options)};
 const horizontal=create([-30,0],[-20,.2]);assert.equal(horizontal.operations.find(op=>op.constraint?.kind==='HORIZONTAL').constraint.references.length,1);assert(horizontal.offered.includes('推断：水平'));
 const vertical=create([-30,0],[-29.8,10]);assert(vertical.operations.some(op=>op.constraint?.kind==='VERTICAL'));
 const parallelCase=create([0,0],[10,10.2]);assert.equal(parallelCase.operations.find(op=>op.constraint?.kind==='PARALLEL').constraint.references[1].entityId,line.id);
 const perpendicularCase=create([0,0],[-10,10.1]);assert(perpendicularCase.operations.some(op=>op.constraint?.kind==='PERPENDICULAR'));
 const disabled=create([-30,0],[-20,.2],{enabled:false});assert.equal(disabled.operations.filter(op=>op.type==='ADD_CONSTRAINT').length,0);
 const temporary=create([-30,0],[-20,.2],{alt:true});assert.equal(temporary.operations.filter(op=>op.type==='ADD_CONSTRAINT').length,0);
 const ignoredInput=create([-30,0],[-20,.2],{angle:'12 deg'});assert(!ignoredInput.operations.some(op=>op.constraint?.kind==='ANGLE'));assert(ignoredInput.operations.some(op=>op.constraint?.kind==='HORIZONTAL'),'drawing ignores removed numeric command actions');
 const captured=create([-30,0],[-20,.2],{target:{target:'ENTITY',entityId:'target',subElement:'POINT'}});assert(captured.operations.some(op=>op.constraint?.kind==='COINCIDENT'));assert(!captured.operations.some(op=>op.constraint?.kind==='HORIZONTAL'));
 port.currentModifiers=()=>({alt:true,shift:false,ctrl:false,meta:false});const stationaryAlt=create([-30,0],[-20,.2]);assert(!stationaryAlt.operations.some(op=>op.constraint?.kind==='HORIZONTAL'),'Alt pressed after hover is checked again at confirmation');
 if(process.env.OCCCCAD_SKETCH_INFERENCE_EMIT==='1')process.stdout.write(JSON.stringify({horizontal:horizontal.operations,vertical:vertical.operations,parallel:parallelCase.operations,perpendicular:perpendicularCase.operations}));
 else console.log('PASS: production creation, visible suggestions, formal stable supports, disable, Alt, capture and mouse-only creation');
}finally{await server.close();}
