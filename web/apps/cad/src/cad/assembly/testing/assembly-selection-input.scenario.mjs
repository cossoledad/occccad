import assert from "node:assert/strict";
import { createServer } from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
  const { ToolManager }=await server.ssrLoadModule("/src/cad/tool/tool-manager.ts");
  const { AssemblyConstraintTool, SelectTool }=await server.ssrLoadModule("/src/cad/tool/cad-tool.ts");
  const {assemblyGeometryRef}=await server.ssrLoadModule("/src/cad/assembly/assembly-reference.ts");
  const {assemblyPublicInitialFields}=await server.ssrLoadModule('/src/cad/assembly/assembly-public.ts');
  assert.equal(assemblyPublicInitialFields('FIX_TOGETHER',[]).groupName,'','server naming counters own the default group name');
  assert.equal(assemblyGeometryRef({kind:"axis-system",id:"frame",entityId:"stable-frame",instanceId:"a"}).kind,"FRAME","whole Frame must not silently become its origin point");
  const face=(id)=>({kind:"face",id:"face-"+id,instanceId:id,geometryKey:"mesh-"+id,topologyId:2});
  const axis=(id)=>({kind:"axis",axis:"DATUM",id:"axis-"+id,entityId:"datum-"+id,instanceId:id});
  let selected=[],requests=[],pick=null,events=[];
  let manager;
  const viewport={
    currentSelections:()=>selected,
    clearReferencePreview:()=>{}, clearToolPreview:()=>{}, cancelDimensionDrag:()=>{},
    setToolPrompt:()=>{},
    retainSelections:values=>{selected=values;},
    requestAssemblyConstraint:(kind,references,members)=>{events.push("request");requests.push({kind,references,...(members?{members}: {})});},
    finishToolUse:()=>manager.activate("select"),
    selectionAt:()=>pick,
  };
  manager=new ToolManager({viewport});
  manager.register(new SelectTool());
  for(const kind of ["fix","coincident","rigid","fix_together","contact"]) manager.register(new AssemblyConstraintTool(kind));
  manager.subscribe(id=>events.push(id));
  selected=[face("a"),face("b")]; manager.activate("assembly.coincident");
  assert.equal(requests.length,1);assert.equal(requests[0].references.length,2);
  assert.deepEqual(events,["assembly.coincident","request","select"],"seeds must not reactivate a completed command");
  selected=[face("a")]; manager.activate("assembly.coincident");
  assert.equal(requests.length,1,"a single seed waits for another occurrence");
  manager.selectionInput([face("a")]); assert.equal(requests.length,1,"duplicate support is ignored");
  manager.selectionInput([face("b")]); assert.equal(requests.length,2,"tree input completes a viewport seed");

  selected=[];manager.activate("assembly.coincident");
  manager.selectionInput([axis("a")]);
  pick=axis("b");
  manager.pointerDown({button:0,pointerId:1,x:1,y:1,state:{buttons:{middle:false,right:false}}});
  assert.equal(requests.at(-1).references[0].geometryId,"datum-a");
  assert.equal(requests.at(-1).references[1].geometryId,"datum-b","tree and viewport share the same axis reference path");

  selected=[];manager.activate("assembly.fix");
  manager.selectionInput([{kind:"instance",id:"a",instanceId:"a"}]);
  assert.deepEqual(requests.at(-1),{kind:"fix",references:[{instanceId:"a",kind:"BODY"}]});
  selected=[face("b")]; manager.activate("assembly.fix");
  assert.deepEqual(requests.at(-1),{kind:"fix",references:[{instanceId:"b",kind:"BODY"}]},"preselected geometry is promoted for Fix");
  selected=[face("a"),face("b")];manager.activate("assembly.fix_together");
  assert.deepEqual(requests.at(-1),{kind:"fix_together",references:[],members:[{instanceId:"a",instancePath:undefined},{instanceId:"b",instancePath:undefined}]},"group creation seeds motion occurrences, never CAD face bodies");
  selected=[];manager.activate('assembly.fix_together');assert.equal(requests.at(-1).members.length,0,'no selection still opens the persistent member owner');
  selected=[face('a'),face('b'),face('c')];manager.activate('assembly.fix_together');assert.equal(requests.at(-1).members.length,3,'all preselected instances belong to one draft');
  const {assemblyGroupMember,groupMemberToken,groupMemberSelection}=await server.ssrLoadModule('/src/cad/assembly/group-selection.ts');
  const groupView={document:{id:'product',versionId:'head'},product:{instances:[{id:'root-unit'}],constraints:[{id:'group',kind:'FIX_TOGETHER'}]}};
  const nestedPath={rootDocumentId:'product',segments:[{instanceId:'root-unit',ownerDocumentId:'product'},{instanceId:'child',ownerDocumentId:'nested'}]};
  const member=assemblyGroupMember(groupView,{...face('child'),instancePath:nestedPath});
  assert.equal(member.instanceId,'root-unit','nested visible geometry belongs to its editable rigid occurrence');
  assert.equal(member.instancePath.segments.length,1);assert.equal(groupMemberToken(member),'instance:root-unit');
  const groupSelection={kind:'assembly-constraint',id:'group',constraintId:'group',documentId:'product',occurrencePath:''};
  assert.deepEqual(assemblyGroupMember(groupView,groupSelection),{groupId:'group'});
  assert.equal(assemblyGroupMember(groupView,groupSelection,'','group'),undefined,'a group cannot be its own member');
  assert.equal(assemblyGroupMember(groupView,{...groupSelection,documentId:'other'}),undefined,'foreign groups cannot cross owning Product boundaries');
  const reference={kind:'assembly-group-member-reference',id:'reference',highlightTarget:groupSelection};
  assert.deepEqual(assemblyGroupMember(groupView,reference),{groupId:'group'},'tree references preserve formal group identity');
  assert.equal(groupMemberSelection(groupView,{groupId:'group'}).constraintId,'group');
  selected=[face("a"),face("b")];manager.activate("assembly.contact");
  assert.equal(requests.at(-1).kind,"contact");
  assert.deepEqual(requests.at(-1).references.map(r=>r.kind),["FACE","FACE"],"Contact preserves unresolved source picks for authoritative exact inspection");

  selected=[face("a")];manager.activate("assembly.coincident");
  manager.activate("select");
  selected=[axis("b")];manager.activate("assembly.coincident");
  const count=requests.length;
  manager.selectionInput([axis("a")]);
  assert.equal(requests.length,count+1);
  assert.equal(requests.at(-1).references[0].geometryId,"datum-b","cancel must clear the previous first support");
  assert.equal(manager.selectionInput([face("a")]),"unhandled","ordinary selection remains owned by the workbench");
  console.log("Tree/viewport assembly supports, one/two seeds, Fix projection and cancellation passed.");
} finally {await server.close();}
