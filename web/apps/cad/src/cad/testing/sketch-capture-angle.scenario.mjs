import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {resolveSketchReference}=await server.ssrLoadModule('/src/cad/interaction/sketch-reference-pick.ts');
 const {normalizeSketchCaptureOperations}=await server.ssrLoadModule('/src/cad/sketch/sketch-point-capture.ts');
 const {chooseSketchAngleSector,sketchAngleSweep}=await server.ssrLoadModule('/src/cad/sketch/sketch-angle.ts');
 const {measureSketchDimension,buildSketchConstraintLayout}=await server.ssrLoadModule('/src/cad/sketch/sketch-constraint-layout.ts');
 const {SketchIntersectionCache}=await server.ssrLoadModule('/src/cad/interaction/sketch-intersections.ts');
 const {PointSketchTool,LineSketchTool,RectangleSketchTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const point={id:'point',kind:'POINT',role:'CONSTRUCTION',point:{x:30,y:0}},line={id:'line',kind:'LINE',role:'PROFILE',start:{x:10,y:0},end:{x:60,y:0}};
 for(const scale of [1,4,.5])for(const entities of [[line,point],[point,line]]){
  const project=([x,y])=>({x:x*scale,y:y*scale});
  assert.equal(resolveSketchReference({x:30*scale+5,y:0},entities,project,'LINEAR_DIMENSION').entityId,'point','point marker beats a distance-zero line');
  assert.equal(resolveSketchReference({x:30*scale+13,y:0},entities,project,'LINEAR_DIMENSION').entityId,'line','outside point radius curve selection resumes');
  assert.equal(resolveSketchReference({x:30*scale,y:0},entities,project,'LINE').entityId,'line');
  const retained={target:'ENTITY',entityId:'point',subElement:'POINT'};
  assert.equal(resolveSketchReference({x:30*scale,y:0},entities,project,'LINEAR_DIMENSION',12,110,retained).entityId,'line','accepted input is filtered before ranking');
 }
 const ops=[],ctx={viewport:{hasActiveSketch:()=>true,sketchPoint:(x,y)=>[x,y],sketchSnapReference:()=>snap,currentSketchConstraints:()=>[],commitSketchOperations:value=>ops.push(value),setToolPrompt:()=>{},finishToolUse:()=>{},clearToolPreview:()=>{},showReferenceDimensions:()=>{},showPolylinePreview:()=>{}}};let snap;
 const pointer=(x,y,alt=false)=>({x,y,pointerId:1,button:0,state:{buttons:{left:false,middle:false,right:false},modifiers:{alt}}});
 const click=(tool,x,y,alt)=>{tool.pointerDown(pointer(x,y,alt),ctx);tool.pointerUp(pointer(x,y),ctx)};
 const support={target:'ENTITY',entityId:'line',subElement:'WHOLE'};
 for(const kind of ['POINT_ON_OBJECT','MIDPOINT','COINCIDENT']){
  const ref=kind==='COINCIDENT'?{...support,subElement:'START'}:support;snap={...ref,capture:{kind,references:[ref]}};
  const tool=new PointSketchTool();click(tool,30,0);assert.equal(ops.at(-1).length,2);assert.equal(ops.at(-1)[1].constraint.kind,kind);assert.deepEqual(ops.at(-1)[1].constraint.references[1],ref);assert(!JSON.stringify(ops.at(-1)).includes('capture'));
  click(tool,30,0,true);assert.equal(ops.at(-1).length,1,'Alt suppresses automatic relations');tool.keyDown({key:'a'},ctx);click(tool,30,0);assert.equal(ops.at(-1).length,1,'A disables automatic relations');
 }
 const rect=new RectangleSketchTool();snap={...support,capture:{kind:'MIDPOINT',references:[support]}};click(rect,30,0);snap=undefined;click(rect,40,20);assert.equal(ops.at(-1)[0].firstCapture.kind,'MIDPOINT','macros carry an explicit capture intent');
 const draw=new LineSketchTool();snap={...support,capture:{kind:'POINT_ON_OBJECT',references:[support]}};click(draw,30,0);draw.pointerMove(pointer(40,10),ctx);draw.keyDown({key:'5'},ctx);draw.keyDown({key:'Enter'},ctx);assert.equal(ops.at(-1).filter(o=>o.constraint).length,1,'numeric endpoint overrides its snap without dropping the accepted start');
 const a={target:'ENTITY',entityId:'a',subElement:'END'},b={target:'ENTITY',entityId:'b',subElement:'START'},target={target:'ENTITY',entityId:'support',subElement:'START'};
 const joined=[{type:'ADD_CONSTRAINT',constraint:{id:'join',kind:'COINCIDENT',references:[a,b]}},...[a,b].map((point,i)=>({type:'ADD_CONSTRAINT',constraint:{id:'snap'+i,kind:'COINCIDENT',references:[point,{...target,capture:{kind:'COINCIDENT',references:[target]}}]}}))];
 assert.equal(normalizeSketchCaptureOperations(joined).length,2,'a closed vertex receives one automatic connection, not duplicate equations');
 const angleLine={...line,id:'angle',start:{x:0,y:0},end:{x:Math.sqrt(3)*10,y:10}};
 for(const swap of [false,true])for(const reverse of [false,true])for(const [cursor,value] of [[15,30],[100,150],[195,30],[280,150]])for(const reflex of [false,true]){
  const l=reverse?{...angleLine,start:angleLine.end,end:angleLine.start}:angleLine;let refs=[{target:'SKETCH_X_AXIS',subElement:'DIRECTION'},{target:'ENTITY',entityId:'angle',subElement:'DIRECTION'}];if(swap)refs.reverse();
  const p=[30*Math.cos(cursor*Math.PI/180),30*Math.sin(cursor*Math.PI/180)],sector=chooseSketchAngleSector(refs,[l],p,reflex),want=reflex?360-value:value;
  assert(sector);assert(Math.abs(measureSketchDimension('ANGLE',refs,[l],sector)-want)<1e-9);
  const c={id:'angle',kind:'ANGLE',references:refs,value:want,unit:'deg',angleSector:sector,labelPosition:{x:p[0],y:p[1]}};
  const first=buildSketchConstraintLayout(c,[l]),dragged=buildSketchConstraintLayout({...c,labelPosition:{x:-p[0],y:-p[1]}},[l]);
  assert.deepEqual(first.segments[0],dragged.segments[0],'text crossing quadrants does not redefine the angle rays');assert.equal(first.segments.length,66,'reflex uses the full directed arc');
 }
 let resolve,calls=0,ready=0,aborted;
 const cache=new SketchIntersectionCache(),curves=[{...line,id:'a'},{...line,id:'b',start:{x:30,y:-20},end:{x:30,y:20}}],project=p=>p;
 const query=(ids,signal)=>{calls++;aborted=signal;return new Promise(done=>resolve=done)};
 for(let i=0;i<10;i++)cache.near('part/v1/s','v1','s',[30,0],curves,project,query,()=>ready++);
 assert.equal(calls,1,'unchanged nearby pairs are requested once');
 resolve({versionId:'v1',sketchId:'s',sourceDigests:['a','b'],intersections:[{point:[30,0],parameters:[.4,.5],references:[a,b],constraintSupported:true}]});await new Promise(setImmediate);
 assert.equal(ready,1);assert.equal(cache.near('part/v1/s','v1','s',[30,0],curves,project,query,()=>ready++).length,1);assert.equal(calls,1);
 const crowded=new SketchIntersectionCache(),requested=[];
 const overlaps=Array.from({length:8},(_,i)=>({...line,id:`overlap-${i}`}));
 crowded.near('crowded/v1/s','v1','s',[30,0],[...overlaps,...curves],project,async(ids)=>{requested.push(ids);return {versionId:'v1',sketchId:'s',intersections:[]}},()=>{},['a','b']);
 assert.deepEqual(requested,[['a','b']],'explicit supports remain authoritative in crowded overlapping candidates');
 cache.near('part/v2/s','v2','s',[30,0],curves,project,query,()=>ready++);cache.reset();assert(aborted.aborted);resolve({versionId:'v2',sketchId:'s',intersections:[{}]});await new Promise(setImmediate);assert.equal(ready,1,'cancelled/late results cannot resurrect markers');
}finally{await server.close()}
