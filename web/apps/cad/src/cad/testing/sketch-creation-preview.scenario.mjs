import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {LineSketchTool}=await server.ssrLoadModule('/src/cad/tool/cad-tool.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const requests=[],commits=[],displays=[];let state,revision='base',contextKey='root-version';
 const port={hasActiveSketch:()=>true,currentLengthUnit:()=> 'mm',currentSketchIdentity:()=>({documentId:'part',sketchId:'sketch',versionId:revision,contextKey}),sketchPoint:(x,y)=>[x,y],sketchSnapReference:()=>undefined,projectSketchPoint:p=>p.map(v=>v*10),
  showPolylinePreview(){},showReferenceDimensions(){},clearToolPreview(){},setToolPrompt(){},setSketchCommandState:s=>state=s,finishToolUse(){},showSketchCreationCandidate:(ops,entities)=>displays.push({ops,entities}),
  previewSketchOperations:(operations,signal)=>new Promise((resolve,reject)=>requests.push({operations:structuredClone(operations),signal,resolve:()=>resolve({entities:operations.flatMap(op=>op.entity?[op.entity]:[]),versionId:revision,featureId:'sketch'}),reject})),
  commitSketchOperations:operations=>{commits.push(structuredClone(operations));return Promise.resolve()}};
 const manager=new ToolManager({viewport:port});manager.register(new LineSketchTool());
 const pointer=(x,y)=>({x,y,pointerId:1,button:0,state:{buttons:{left:true,middle:false,right:false}}});
 const tick=()=>new Promise(resolve=>setImmediate(resolve)),debounce=()=>new Promise(resolve=>setTimeout(resolve,175));
 const begin=()=>{manager.activate(undefined);manager.activate('sketch.line');manager.pointerDown(pointer(0,0));manager.pointerUp(pointer(0,0));manager.pointerMove(pointer(20,0));};
 const move=x=>manager.pointerMove(pointer(x,.1));
 begin();move(21);move(22);await debounce();assert.equal(requests.length,1,'rapid pointer movement coalesces into one immutable candidate');
 move(23);await debounce();assert.equal(requests.length,1,'only one solver request runs until cancellation is acknowledged');assert(requests[0].signal.aborted);
 requests[0].resolve();await tick();assert.equal(requests.length,2);assert.equal(displays.length,0,'superseded result is never drawn');
 manager.commandAction({type:'confirm'});manager.commandAction({type:'confirm'});assert.equal(state.phase,'committing');assert.equal(commits.length,0,'confirmation waits for its exact candidate');manager.commandAction({type:'cancel'});requests[1].resolve();await tick();assert.equal(commits.length,0);assert.equal(state,undefined,'cancel prevents a queued confirmation from committing');
 begin();move(40);manager.commandAction({type:'confirm'});await tick();assert.equal(requests.length,3,'confirmation flushes the debounce');requests[2].resolve();await tick();await tick();assert.equal(commits.length,1);assert.deepEqual(commits[0],requests[2].operations,'commit preserves the exact preview entity and constraint identities');
 begin();move(50);manager.commandAction({type:'confirm'});await tick();revision='other';requests[3].resolve();await tick();assert.equal(commits.length,1,'a late result cannot commit into a changed Revision');assert.equal(state.phase,'failed');
 revision='other';begin();move(55);manager.commandAction({type:'confirm'});await tick();requests[4].reject(new Error('unknown parameter'));await tick();assert.equal(commits.length,1);assert.deepEqual(state.fields,[]);assert.equal(state.phase,'failed');manager.commandAction({type:'cancel'});
 begin();move(60);manager.commandAction({type:'confirm'});await tick();const last=requests.at(-1);contextKey='other-root-version';last.resolve();await tick();assert.equal(commits.length,1,'a changed root/variant invalidates a draft even when the Part Revision is unchanged');manager.commandAction({type:'cancel'});
 console.log('PASS: direction-inference authority, coalescing, exact IDs, duplicate Enter, scope changes and cancellation');
}finally{await server.close();}
