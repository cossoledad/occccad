import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../../package.json',import.meta.url));
const {createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const {ToolManager}=await server.ssrLoadModule('/src/cad/tool/tool-manager.ts');
 const THREE=await server.ssrLoadModule('three');
 const {VisibilityResolver}=await server.ssrLoadModule('/src/cad/interaction/visibility-resolver.ts');
 const {selectionKey}=await server.ssrLoadModule('/src/cad/interaction/selection-identity.ts');
 const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject};};
 const view=(owner='part',version='v1')=>({document:{id:owner,versionId:version,type:"PART"},part:{features:[{id:'sketch',bodyId:'body',type:'SKETCH',sketch:{entities:[],constraints:[],externalGeometry:[]}}]}});
 const ops=[{type:'DELETE_ENTITIES',entityIds:['old-line']}],intent={requestId:'same-request',baseVersionId:'v1'};
 function harness(){
  const events=[],engine=Object.create(CadViewportEngine.prototype);
  Object.assign(engine,{view:view(),activeSketchID:'sketch',selected:[],treeVisibilityOverrides:{},visualGeneration:0,
   clearSnapPreview(){events.push('clear-snap');},renderReady(display){events.push('render:'+display.document.id+':'+display.document.versionId);this.view=display;},
   callbacks:{sketchOperations:async()=>view('part','v2'),sketchReceiptChanged:receipt=>events.push(receipt?{receipt:structuredClone(receipt)}:'receipt-clear'),toolPromptChanged:message=>events.push({prompt:message}),operationFailed:error=>events.push({failure:error.message}),toolUseCompleted:()=>events.push('finish')},
   visuals:{hydrate:async display=>{events.push('hydrate');return display;}}});
  const port=engine.toolViewportPort();
  engine.tools=new ToolManager({viewport:port});
  engine.tools.register({id:'select'});engine.tools.register({id:'sketch.edit.trim',cancel:()=>events.push('tool-cancel')});engine.tools.activate('sketch.edit.trim');
  return {engine,events,port};
 }
 // Actual Engine pending guard and completion callback order; hydration is only
 // an I/O boundary here, never a replacement for receipt/model transitions.
 {
  const {engine,events,port}=harness(),network=deferred(),hydration=deferred();
  engine.callbacks.sketchOperations=()=>network.promise;engine.visuals.hydrate=()=>{events.push('hydrate');return hydration.promise;};
  const commit=engine.commitDimensionOperations('sketch',ops,intent);port.finishToolUse();
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,{requestId:'other'}),/仍在提交/);
  assert(!events.includes('finish'));network.resolve(view('part','v2'));await Promise.resolve();
  assert(!events.includes('finish'));assert.equal(engine.view.document.versionId,'v1');
  hydration.resolve(view('part','v2'));await commit;
  assert(events.indexOf('render:part:v2')<events.indexOf('finish'));assert.equal(engine.sketchReceipt,undefined);assert.equal(engine.sketchCommitPending,undefined);
 }
 {
  const {engine,events}=harness();engine.callbacks.sketchOperations=async()=>{throw Object.assign(new Error('invalid constraint'),{code:'VALIDATION_FAILED'});};
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,intent),/invalid constraint/);
  assert.equal(engine.sketchReceipt,undefined);assert(events.some(event=>event.failure==='invalid constraint'));assert(!events.some(event=>typeof event==='string'&&event.startsWith('render:')));
 }
 {
  const {engine}=harness();engine.callbacks.sketchOperations=()=>{throw Object.assign(new Error('synchronous refusal'),{code:'VALIDATION_FAILED'});};
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,intent),/synchronous refusal/);
  assert.equal(engine.sketchReceipt,undefined);assert.equal(engine.sketchCommitPending,undefined);
 }
 {
  const {engine}=harness(),inputOps=structuredClone(ops),inputIntent=structuredClone(intent);let sent;
  engine.callbacks.sketchOperations=async(feature,operations,request)=>{sent={feature,operations:structuredClone(operations),request:structuredClone(request)};return view('part','v2');};
  const commit=engine.commitDimensionOperations('sketch',inputOps,inputIntent);
  inputOps[0].entityIds[0]='late-change';inputIntent.requestId='late-request';await commit;
  assert.deepEqual(sent,{feature:'sketch',operations:ops,request:intent},'dispatch must use the same frozen snapshot as its receipt');
 }
 // Unknown transport retains the frozen domain intent globally after the tool
 // ends, and retries inspect precisely that same request and operation list.
 {
  const {engine,port}=harness();engine.callbacks.sketchOperations=async()=>{throw Object.assign(new Error('connection closed'),{code:'CONNECTION_CLOSED'});};
  const mutable=structuredClone(ops),mutableIntent=structuredClone(intent);await assert.rejects(engine.commitDimensionOperations('sketch',mutable,mutableIntent),/connection closed/);mutable[0].entityIds[0]='mutated';mutableIntent.requestId='mutated-request';mutableIntent.baseVersionId='mutated-head';
  port.finishToolUse(true);assert.equal(engine.tools.activeToolID,'select');assert.equal(engine.sketchReceipt.status,'unknown');assert.deepEqual(engine.sketchReceipt.operations,ops);assert.equal(engine.sketchReceipt.intent.requestId,intent.requestId);
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,{requestId:'different'}),/先确认上一/);
  let checks=0;engine.callbacks.sketchReceiptCheck=async receipt=>{checks++;assert.deepEqual(receipt.operations,ops);assert.equal(receipt.intent.requestId,intent.requestId);return view('part','v2');};
  await engine.retrySketchReceipt();assert.equal(checks,1);assert.equal(engine.sketchReceipt,undefined);assert.equal(engine.view.document.versionId,'v2');
 }
 {
  const {engine,events,port}=harness();let submissions=0;
  engine.callbacks.sketchOperations=async()=>{submissions++;return view('part','v2');};engine.visuals.hydrate=async()=>{throw new Error('missing artifact');};
  const commit=engine.commitDimensionOperations('sketch',ops,intent);port.finishToolUse();const accepted=await commit;
  assert.equal(accepted.document.versionId,'v2');assert.equal(engine.sketchReceipt,undefined);assert.equal(submissions,1);await engine.retrySketchReceipt();assert.equal(submissions,1);assert(events.some(event=>event.prompt?.includes('操作已确认')));
 }
 // A late commit cannot render into another owner's currently active sketch.
 {
  const {engine,events}=harness(),network=deferred();engine.callbacks.sketchOperations=()=>network.promise;
  const commit=engine.commitDimensionOperations('sketch',ops,intent);engine.view=view('other');network.resolve(view('part','v2'));await commit;
  assert.equal(engine.view.document.id,'other');assert(!events.includes('hydrate'));assert(!events.some(event=>typeof event==='string'&&event.startsWith('render:')));
 }
 // The receipt check itself is authoritative: hydration failure after a known
 // answer must never resurrect an unknown request or issue that request again.
 {
  const {engine,events}=harness();engine.callbacks.sketchOperations=async()=>{throw Object.assign(new Error('timeout'),{code:'TIMEOUT'});};
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,intent));
  let checks=0;engine.callbacks.sketchReceiptCheck=async()=>{checks++;return view('part','v2');};engine.visuals.hydrate=async()=>{throw new Error('artifact unavailable');};
  await engine.retrySketchReceipt();assert.equal(engine.sketchReceipt,undefined);assert.equal(checks,1);
  const lastReceipt=events.filter(event=>typeof event==='string'&&event==='receipt-clear'||event.receipt).at(-1);
  assert.equal(lastReceipt,'receipt-clear');await engine.retrySketchReceipt();assert.equal(checks,1);assert(events.some(event=>event.prompt?.includes('原请求已确认')));
 }
 {
  const {engine,events}=harness();engine.callbacks.sketchOperations=async()=>{throw Object.assign(new Error('network'),{code:'NETWORK_ERROR'});};
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,intent));
  engine.callbacks.sketchReceiptCheck=async()=>{throw Object.assign(new Error('constraint refused'),{code:'VALIDATION_FAILED'});};
  await engine.retrySketchReceipt();assert.equal(engine.sketchReceipt,undefined);assert(events.some(event=>event.failure==='constraint refused'));
 }
 for(const changedScope of ['owner','feature','occurrence']) {
  const {engine,events}=harness();engine.callbacks.sketchOperations=async()=>{throw Object.assign(new Error('network'),{code:'NETWORK_ERROR'});};
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,intent));
  engine.callbacks.sketchReceiptCheck=async()=>view('part','v2');const hydration=deferred(),entered=deferred();
  engine.visuals.hydrate=()=>{entered.resolve();return hydration.promise;};
  const retry=engine.retrySketchReceipt();await entered.promise;
  if(changedScope==='owner')engine.view=view('other');else if(changedScope==='feature')engine.activeSketchID='other-sketch';else engine.editContext={view:engine.view,occurrencePath:'other/instance'};
  hydration.resolve(view('part','v2'));await retry;
  assert(!events.some(event=>typeof event==='string'&&event.startsWith('render:')),changedScope+' stale receipt rendered');
  assert.equal(engine.tools.activeToolID,'sketch.edit.trim',changedScope+' stale receipt changed tool');
 }
 // Actual typed picking filters hidden/suppressed entities and the requested
 // semantic role, including an external boundary's typed ownership.
 {
  const {engine}=harness();const sketch=engine.view.part.features[0].sketch;
  sketch.entities=[{id:'shown',kind:'LINE',role:'PROFILE',start:{x:-5,y:2},end:{x:5,y:2}},{id:'hidden',kind:'LINE',role:'PROFILE',start:{x:-5,y:3},end:{x:5,y:3}},{id:'suppressed',kind:'LINE',role:'PROFILE',suppressed:true,start:{x:-5,y:4},end:{x:5,y:4}}];
  sketch.externalGeometry=[{id:'external',status:'CONNECTED',snapshot:{kind:'LINE',start:{x:-5,y:5},end:{x:5,y:5}}},{id:'unresolved',status:'UNRESOLVED_EXTERNAL',snapshot:{kind:'LINE',start:{x:-5,y:6},end:{x:5,y:6}}}];
  engine.visibilityResolver=new VisibilityResolver({kind:'PART',documentId:'part',children:[{kind:'BODY',documentId:'part',entityId:'body',localVisible:false,children:[{kind:'SKETCH',documentId:'part',entityId:'sketch',bodyId:'body',children:sketch.entities.map(entity=>({kind:'SKETCH_ENTITY',documentId:'part',entityId:entity.id,ownerEntityId:'sketch',bodyId:'body',localVisible:entity.id!=='hidden'}))}]}]});
  assert.deepEqual(engine.visibleSketchReferenceEntities().map(entity=>entity.id),['shown','external']);
  engine.sketchPlane='XY';engine.renderer={domElement:{clientWidth:200,clientHeight:200}};engine.camera=new THREE.OrthographicCamera(-10,10,10,-10,.1,100);engine.camera.position.set(0,0,20);engine.camera.lookAt(0,0,0);engine.camera.updateMatrixWorld();
  const hit=engine.sketchReferenceAt(100,50,'ENTITY',undefined,reference=>reference.target==='ENTITY'&&reference.entityId==='external');
  assert.equal(hit.entityId,'external');assert.equal(hit.target,'EXTERNAL');
  assert.equal(engine.sketchReferenceAt(100,70,'ENTITY',undefined,reference=>reference.entityId==='hidden'),null);
  assert.equal(engine.sketchReferenceAt(100,80,'ENTITY',undefined,reference=>reference.entityId==='shown'&&reference.subElement==='CENTER'),null,'typed role cannot pick a Line center');
  const selection=engine.sketchEntitySelection(sketch.entities[0]);assert.equal(selection.ownerDocumentId,'part');assert.equal(selection.featureId,'sketch');assert.equal(selection.bodyId,'body');
  engine.treeVisibilityOverrides={[selectionKey(selection)]:false};assert.deepEqual(engine.visibleSketchReferenceEntities().map(entity=>entity.id),['external']);
  engine.treeVisibilityOverrides={};engine.editContext={view:engine.view,occurrencePath:'product/second'};
  const occurrenceSelection=engine.sketchEntitySelection(sketch.entities[0]);assert.equal(occurrenceSelection.occurrencePath,'product/second');assert.notEqual(selectionKey(selection),selectionKey(occurrenceSelection));
  engine.treeVisibilityOverrides={[selectionKey(selection)]:false};assert(engine.visibleSketchReferenceEntities().some(entity=>entity.id==='shown'),'root override leaked into a different occurrence scope');
 }
 // Receipt recovery has the same exclusive commit barrier as first dispatch.
 {
  const {engine}=harness();engine.callbacks.sketchOperations=async()=>{throw Object.assign(new Error('timeout'),{code:'TIMEOUT'});};
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,intent));
  const response=deferred();engine.callbacks.sketchReceiptCheck=()=>response.promise;
  const retry=engine.retrySketchReceipt();
  await assert.rejects(engine.commitDimensionOperations('sketch',ops,{requestId:'new-request'}),/等待回执/);
  assert.equal(engine.sketchReceipt.intent.requestId,intent.requestId);
  response.resolve(view('part','v2'));await retry;assert.equal(engine.sketchReceipt,undefined);
 }
 // A tree/marker with a reused ConstraintId from another occurrence or head
 // must not open the active dialog and bypass the viewport role scope.
 {
  const {engine,events}=harness();let opened=0;engine.callbacks.dimensionEditRequested=()=>opened++;
  engine.view.part.features[0].sketch.constraints=[{id:'dimension',kind:'LENGTH',references:[]}];
  const selection={kind:'sketch-constraint',id:'marker',documentId:'part',featureId:'sketch',constraintId:'dimension',versionId:'v1'};
  for(const patch of [{documentId:'other'},{occurrencePath:'product/second'},{versionId:'v0'},{featureId:'other-sketch'}])assert.equal(engine.requestDimensionEdit({...selection,...patch}),false);
  assert.equal(opened,0);assert.equal(engine.tools.activeToolID,'sketch.edit.trim');assert(events.some(event=>event.prompt?.includes('occurrence')));
 }
 console.log('engine sketch commit receipt and typed visibility PASS');
} finally {await server.close();}
