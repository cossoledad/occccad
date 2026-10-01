import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import {createServer} from "vite";

// Consume the production catalog, not implementationProfile or test PASS counts.
// Every marker is emitted only after assertions for that exact capability.
const catalog=JSON.parse(await readFile(new URL("../../../../../../../services/internal/assemblycontract/catalog.json",import.meta.url),"utf8"));
const requested=process.env.OCCCCAD_ASSEMBLY_CONTRACT_CASES?JSON.parse(process.env.OCCCCAD_ASSEMBLY_CONTRACT_CASES):undefined;
const cases=catalog.capabilities.map(capability=>({
 caseId:`${capability.capabilityId}.web-production-capability`,capability,
 expected:catalog.cases.find(c=>c.caseId===`${capability.capabilityId}.web-production-capability`)?.expected,
})).filter(c=>!requested||requested.includes(c.caseId));
assert.ok(cases.length,"zero production capability assertions selected");
if(requested)for(const caseId of requested.filter(id=>id.endsWith(".web-production-capability")))assert.ok(cases.some(c=>c.caseId===caseId),`unknown production capability case ${caseId}`);
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {matchingAssemblyCapabilities,contactRelationOptions,assemblyConstraintReferences}=await server.ssrLoadModule("/src/cad/assembly/assembly-capability.ts");
 const {assemblyConstraintEntry,changeAngleRelation,invalidateAngleReferenceDirection,angleRelationSupportsMeasured}=await server.ssrLoadModule("/src/cad/assembly/assembly-angle.ts");
 const {offsetInitialFields,offsetCommandFields}=await server.ssrLoadModule("/src/cad/assembly/assembly-offset.ts");
 const {assemblyQuantityInitialFields,assemblyQuantityCommandFields}=await server.ssrLoadModule("/src/cad/assembly/assembly-quantity.ts");
 const {assemblyPublicCommandFields}=await server.ssrLoadModule("/src/cad/assembly/assembly-public.ts");
 const {fixedPoseFromParameters,fixedPoseAngles}=await server.ssrLoadModule("/src/cad/assembly/assembly-fixed-pose.ts");
 const {angleAxisCandidateError}=await server.ssrLoadModule("/src/features/workbench/assembly-angle-parameters.tsx");
 const tools={Coincidence:"coincident",Contact:"contact",Offset:"distance",Angle:"angle",Fix:"fix",FixTogether:"fix_together"};
 for(const {caseId,capability:c,expected} of cases){
  if(expected){assert.equal(c.family,expected.family);assert.equal(c.subtype,expected.subtype);assert.deepEqual(c.roles.map(r=>r.descriptor),expected.roles);}
  assert.equal(assemblyConstraintEntry(tools[c.family]).kind,tools[c.family],caseId);
  const combinations=c.roles.reduce((sets,role)=>sets.flatMap(set=>(role.supportedDescriptors??[role.descriptor]).map(type=>[...set,type])),[[]]);
  if(expected?.exactCombinations)assert.deepEqual(combinations,expected.exactCombinations,`${caseId}: supported exact types cannot disappear from the frozen test mapping`);
  for(const types of combinations){
   assert.ok(!types.some(type=>type.endsWith("_BOUNDARY")),`${caseId}: abstract boundary must declare finite exact supportedDescriptors`);
   assert.ok(matchingAssemblyCapabilities(catalog.capabilities,c.family,types).some(match=>match.capabilityId===c.capabilityId),caseId);
   assert.ok(matchingAssemblyCapabilities(catalog.capabilities,c.family,["FACE",...types.slice(1)]).every(match=>match.capabilityId!==c.capabilityId),`${caseId}: generic FACE cannot certify exact type`);
   assert.deepEqual(matchingAssemblyCapabilities(catalog.capabilities,c.family,["",...types.slice(1)]),[]);
   if(types.length===2)assert.ok(matchingAssemblyCapabilities(catalog.capabilities,c.family,[...types].reverse()).some(match=>match.capabilityId===c.capabilityId),`${caseId}: legal selection exchange`);
   if(c.family==="Offset"){
    const refs=types.map((kind,index)=>({kind,instanceId:`${caseId}-${index}`}));
    const fields=offsetInitialFields(undefined,refs,types);
    assert.equal(fields.distanceRelation,types.includes("PLANE")?"SELECTED_PLANE_NORMAL_V1":"UNSIGNED");
    const definition={kind:"DISTANCE",first:refs[0],second:refs[1],distanceRelation:fields.distanceRelation,value:types.includes("PLANE")?-3:3,mode:"MEASURED",quantityParameter:{key:"Gap",source:{expression:{sourceText:"StableLength - 3 mm"}}}};
    const reopened=offsetInitialFields(definition);assert.equal(reopened.distanceRelation,fields.distanceRelation);
    assert.deepEqual(offsetCommandFields(reopened),{offsetExpression:"StableLength - 3 mm",offsetKey:"Gap",constraintMode:"MEASURED"});
   }
   if(c.family==="Contact"){
    const relation=c.subtype.split("-").at(-1).toUpperCase();assert.ok(contactRelationOptions([c]).includes(relation));
    assert.deepEqual(assemblyPublicCommandFields("CONTACT",{contactKind:relation,contactSide:"INTERNAL",contactBranch:-1}),{constraintFamily:"Contact",contactKind:relation,contactSide:"INTERNAL",contactBranch:-1});
   }
   if(c.family==="Angle"){
    const directed={angleRelation:"DIRECTED",angleAxis:{instanceId:"third-independent",kind:"AXIS"},reverseAngleAxis:true};
    const changed=changeAngleRelation(directed,c.subtype);assert.equal(changed.angleRelation,c.subtype);
    assert.equal(changed.angleAxis===undefined,c.subtype!=="DIRECTED");
    if(c.subtype==="DIRECTED"){
     assert.deepEqual(invalidateAngleReferenceDirection({...changed,angleReferenceDirection:[1,0,0]}).angleAxis,directed.angleAxis);
     assert.equal(angleAxisCandidateError(directed.angleAxis,{instanceId:"second",kind:types[1]}),undefined);
     assert.ok(matchingAssemblyCapabilities(catalog.capabilities,"Angle",[directed.angleAxis.kind,directed.angleAxis.kind]).length>0);
    }
    const measurable=c.subtype==="FREE"||c.subtype==="DIRECTED";
    assert.equal(angleRelationSupportsMeasured(c.subtype),measurable,`${caseId}: Parallel/Perpendicular are not measurable angle modes`);
    assert.equal(catalog.policies[c.policy].modes.includes("MEASURED"),measurable);
    if(measurable){
     const p=assemblyQuantityInitialFields({kind:"ANGLE",mode:"MEASURED",quantityParameter:{key:"Angle",source:{expression:{sourceText:"StableAngle + 5 deg"}}}});
     assert.deepEqual(assemblyQuantityCommandFields(p),{quantityExpression:"StableAngle + 5 deg",quantityKey:"Angle",constraintMode:"MEASURED"});
    }
   }
   if(c.family==="Fix"){
    const pose=fixedPoseFromParameters([17,-4,9],[20,-30,40]);assert.deepEqual(pose.translation,[17,-4,9]);
    assert.ok(Math.abs(Math.hypot(...pose.rotation)-1)<1e-14);
    fixedPoseAngles(pose).forEach((value,index)=>assert.ok(Math.abs(value-[20,-30,40][index])<1e-12));
   }
   if(c.family==="FixTogether"){
    const prior={id:"stable-group",kind:"FIX_TOGETHER",groupMembers:[{instanceId:"a"},{instanceId:"b"},{instanceId:"c"}]};
    const members=c.subtype==="member-lifecycle"?["instance:c","instance:b"]:c.subtype==="overlap-nested"?["group:inner","instance:c"]:["instance:c","instance:b","instance:a"];
    const command=assemblyPublicCommandFields("FIX_TOGETHER",{groupName:"renamed",groupMembers:members},prior);
    assert.equal(command.firstAssemblyRef,undefined);assert.equal(command.secondAssemblyRef,undefined);assert.equal(command.groupMembers.length,members.length);
    const groupWire=JSON.parse(JSON.stringify({value:17,directionRelation:"UNORIENTED",distanceRelation:"UNSIGNED",...command}));
    assert.equal("value" in groupWire,false);assert.equal("directionRelation" in groupWire,false);assert.equal("distanceRelation" in groupWire,false);
    if(c.subtype==="overlap-nested"){
     const inner={id:"inner",kind:"FIX_TOGETHER",groupMembers:[{instanceId:"a"},{instanceId:"c"}]};
     assert.deepEqual(assemblyConstraintReferences({...prior,groupMembers:command.groupMembers},[inner]).map(r=>r.instanceId),["a","c"]);
    }
   }
  }
  console.log(`CONTRACT_OBSERVED ${caseId} ${JSON.stringify({family:c.family,subtype:c.subtype,exactCombinations:combinations,evidenceLayer:"frontend-production-functions"})}`);
  console.log(`CONTRACT_PASS ${caseId}`);
 }
}finally{await server.close();}
