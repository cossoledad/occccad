import assert from "node:assert/strict";
import {createServer} from "vite";
import {spawnSync} from "node:child_process";
import {readFileSync} from "node:fs";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {assemblyEditIntent,assemblyIntentKey,AssemblyCandidateBinding,AssemblyDialogLifecycle}=await server.ssrLoadModule("/src/cad/assembly/assembly-edit-intent.ts");
 const first={instanceId:"a",kind:"PLANE",geometryId:"plane-a"};
 const second={instanceId:"b",kind:"PLANE",geometryId:"plane-b"};
 const c={id:"c",kind:"COINCIDENT",family:"Coincidence",first,second,directionRelation:"OPPOSITE",evaluationStatus:"VERIFIED",previewId:"never-copy",angleAxis:{instanceId:"old",kind:"AXIS"},angleReferenceDirection:[1,0,0]};
 const store={value:0,directionRelation:"SAME",distanceRelation:"UNSIGNED",fixedTranslation:[0,0,0],fixedAngles:[0,0,0]};
 // Isolate Antd's native React import from Vite's SSR React runtime, whose
 // scheduler MessagePorts otherwise outlive this no-browser test server.
 const result=spawnSync(process.execPath,["-e",`
 const path=require('node:path');
 const {FormStore}=require(require.resolve('@rc-component/form/lib/hooks/useForm.js',{paths:[path.dirname(require.resolve('antd'))]}));
 const form=new FormStore(()=>{}).getForm();
 form.getInternalHooks('RC_FORM_INTERNAL_HOOKS').registerField({props:{},getNamePath:()=>['directionRelation'],onStoreChange(){},getMeta:()=>({}),isFieldDirty:()=>false,isListField:()=>false,isList:()=>false});
 form.setFieldsValue(${JSON.stringify(store)});
 form.validateFields().then(visible=>{
   console.log(JSON.stringify({visible,all:form.getFieldsValue(true),hidden:form.getFieldValue('distanceRelation')}));
   // This isolated fixture owns its process; no browser or pending domain job.
   // Form's React scheduler ports are irrelevant after its actual result.
   process.exit(0);
 },error=>{console.error(error);process.exit(1)});
 `],{encoding:"utf8",timeout:10000});
 assert.equal(result.status,0,result.stderr);
 const {visible,all,hidden}=JSON.parse(result.stdout);
 assert.equal(visible.directionRelation,"SAME");assert.equal(visible.distanceRelation,undefined,"real Antd FormStore omits unregistered hidden field");
 assert.equal(hidden,"UNSIGNED");assert.deepEqual(all,store);
 // Original split, independently of normalized helper: hidden stored defaults
 // survive Preview but validateFields' mounted-only result omits them.
 assert.notEqual(JSON.stringify({directionRelation:store.directionRelation,distanceRelation:store.distanceRelation}),JSON.stringify({directionRelation:visible.directionRelation,distanceRelation:visible.distanceRelation}));
 const build=(fields=store,refs=[first,second],constraint=c)=>assemblyEditIntent(constraint.kind,fields,refs,"mm",constraint);
 const intent=build();assert.equal(intent.directionRelation,"SAME");assert.equal(intent.distanceRelation,"UNSIGNED");
 for(const field of ["previewId","evaluationStatus","angleAxis","angleReferenceDirection"])assert.equal(field in intent,false);
 const slot=new AssemblyCandidateBinding();
 const key=command=>assemblyIntentKey(command,"product","revision","outer/owning");
 assert.equal(key({second:{kind:"PLANE",instanceId:"b"},first:{kind:"PLANE",instanceId:"a"}}),key({first:{instanceId:"a",kind:"PLANE"},second:{instanceId:"b",kind:"PLANE"}}),"object property ordering is transport, support order is intent");
 const generation=slot.begin();assert.equal(slot.freeze(key(intent)),undefined);
 assert.equal(slot.resolve(generation,key(intent),"candidate",intent),true);
 const frozen=slot.freeze(key(build()));assert.equal(frozen.previewId,"candidate");
 for(const direction of ["OPPOSITE","UNDEFINED","SAME"]) {
  const next=build({...store,directionRelation:direction});
  assert.equal(slot.freeze(key(next))?.previewId,direction==="SAME"?"candidate":undefined);
 }
 assert.equal(slot.freeze(assemblyIntentKey(intent,"product","new-head","outer/owning")),undefined);
 assert.equal(slot.freeze(assemblyIntentKey(intent,"product","revision","other-occurrence")),undefined);
 const newFirst={instanceId:"d",kind:"POINT",geometryId:"point",persistentSelection:{sourceDocumentId:"d"}};
 const newSecond={instanceId:"e",kind:"AXIS",geometryId:"axis"};
 for(const refs of [[newFirst,second],[first,newSecond],[newFirst,newSecond]]) {
  const next=build(store,refs);assert.equal(slot.freeze(key(next)),undefined);
  assert.deepEqual(next.firstAssemblyRef,refs[0]);assert.deepEqual(next.secondAssemblyRef,refs[1]);
  assert.equal(next.firstAssemblyRef.publicationRef,undefined,"replace entire support, not IDs on old refs");
 }
 const later=slot.begin();assert.equal(slot.resolve(generation,key(intent),"late",intent),false);
 slot.invalidate();assert.equal(slot.resolve(later,key(intent),"cancelled",intent),false);assert.equal(slot.freeze(key(intent)),undefined);
 const current=slot.begin();slot.resolve(current,key(intent),"fresh",intent);
 intent.firstAssemblyRef.geometryId="mutable";
 assert.equal(frozen.firstAssemblyRef.geometryId,"plane-a","confirmation deep freezes current candidate pair");
 for(const kind of ["DISTANCE","ANGLE","CONTACT","FIX_TOGETHER","FIX"]) {
  const fields={...store,contactKind:"FACE",contactSide:"EXTERNAL",contactBranch:-1,groupName:"g",groupMembers:["instance:a","instance:b"],quantityExpression:"angle + 10 deg",offsetExpression:"length + 2 mm"};
  const original={...c,kind,angleRelation:"DIRECTED",angleAxis:{instanceId:"axis",kind:"AXIS"},reverseAngleAxis:true,fixMode:"SPACE"};
  const preview=assemblyEditIntent(kind,fields,[first,second],"mm",original);
  const commit=assemblyEditIntent(kind,{...fields},structuredClone([first,second]),"mm",structuredClone(original));
  assert.deepEqual(preview,commit,"shared family draft has one normalization");
  if(kind!=="ANGLE")assert.equal(preview.angleAxis,undefined);
  if(kind==="FIX_TOGETHER")assert.equal(preview.firstAssemblyRef,undefined);
 }
 // Hidden stale expression/key values are inapplicable to relation-only tools.
 for(const relation of ["PARALLEL","PERPENDICULAR"])for(const editing of [false,true]) {
  const fields={value:42,quantityExpression:"obsolete_angle + 5 deg",quantityKey:"old_key",constraintMode:"MEASURED"};
  const command=assemblyEditIntent("ANGLE",fields,[first,second],"mm",editing?{...c,kind:"ANGLE",angleRelation:relation}:undefined,{angleRelation:relation});
  assert.equal(command.angleRelation,relation);assert.equal(command.constraintMode,"DRIVING");
  for(const field of ["quantityExpression","quantityKey","offsetExpression","offsetKey","angleAxis","reverseAngleAxis"])assert.equal(field in command,false);
 }
 for(const relation of ["FREE","DIRECTED"]) {
  const command=assemblyEditIntent("ANGLE",{quantityExpression:"a + 5 deg",quantityKey:"angle_key",constraintMode:"MEASURED"},[first,second],"mm",undefined,{angleRelation:relation});
  assert.equal(command.quantityExpression,"a + 5 deg");assert.equal(command.quantityKey,"angle_key");assert.equal(command.constraintMode,"MEASURED");
 }
 // Reproduce actual TanStack ordering: mutation-level cache refresh runs before
 // observer/per-call success. Preview sequence changes must not suppress close.
 const {QueryClient,MutationObserver}=await server.ssrLoadModule("@tanstack/react-query");
 const client=new QueryClient({defaultOptions:{mutations:{retry:false,gcTime:0}}});
 for(const abandon of [false,true]) {
  const lifecycle=new AssemblyDialogLifecycle(),generation=lifecycle.capture();let open=true,previewSequence=1,oldGuardWouldClose=false;
  const observer=new MutationObserver(client,{mutationFn:async()=>({revision:"new-head"}),onSuccess:updated=>{
   client.setQueryData(["document"],updated);previewSequence++;
   if(abandon)lifecycle.invalidate(); // close/cancel/change target during flight
  }});
  const stop=observer.subscribe(()=>{});
  await observer.mutate(undefined,{onSuccess:()=>{oldGuardWouldClose=previewSequence===1;if(lifecycle.isCurrent(generation))open=false;}});
  assert.equal(oldGuardWouldClose,false,"original Preview-sequence success guard drops own authoritative commit");
  assert.equal(open,abandon,"successful current dialog closes; abandoned/new dialog is untouched");stop();
 }
 client.clear();
 const source=readFileSync(new URL("../../../features/workbench/workbench.tsx",import.meta.url),"utf8");
 assert.equal((source.match(/const dialogGeneration=assemblyDialogLifecycle.current.capture\(\)/g)??[]).length,2,"create and edit both bind dialog lifecycle");
 assert.equal((source.match(/onSuccess:[^\n]*isCurrent\(dialogGeneration\)/g)??[]).length,2,"both actual close callbacks use dialog lifetime, not Preview sequence");
} finally {await server.close();}
