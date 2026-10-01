import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {matchingAssemblyCapabilities,contactRelationOptions,derivedSupportOptions,assemblyConstraintReferences,assemblyTargetSupportsEligible}=await server.ssrLoadModule("/src/cad/assembly/assembly-capability.ts");
 assert.equal(assemblyTargetSupportsEligible(undefined),false);
 assert.equal(assemblyTargetSupportsEligible([]),false);
 assert.equal(assemblyTargetSupportsEligible([{status:"RESOLVED"}]),false,"old/unknown inspection eligibility must fail closed");
 const trimmedSource={status:"RESOLVED",constraintEligible:false,constraintDiagnosticCode:"ASSEMBLY_SUPPORT_REQUIRES_UNDERLYING_CIRCLE"};
 assert.equal(assemblyTargetSupportsEligible([trimmedSource]),false,"a resolved trimmed circle is not a full-circle contract support");
 for(const role of ["underlying-circle","circle-center","circle-axis","circle-plane"]){
  const inspection=[trimmedSource,{status:"RESOLVED",constraintEligible:true,reference:{derivedRole:role}}];
  assert.equal(assemblyTargetSupportsEligible(inspection.slice(1)),true,"source-only trim diagnostics must not block explicit valid derived supports");
 }
 assert.equal(assemblyTargetSupportsEligible([{status:"BROKEN",constraintEligible:true}]),false);
 const {assemblyPublicCommandFields,assemblyPublicEditDraft}=await server.ssrLoadModule("/src/cad/assembly/assembly-public.ts");
 const capabilities=[{capabilityId:"contact.plane-sphere",family:"Contact",subtype:"plane-sphere-point",roles:[{descriptor:"PLANE"},{descriptor:"SPHERE"}]}];
 assert.deepEqual(matchingAssemblyCapabilities(capabilities,"Contact",["FACE","SPHERE"]),[]);
 assert.deepEqual(matchingAssemblyCapabilities(capabilities,"Contact",undefined),[]);
 assert.equal(matchingAssemblyCapabilities(capabilities,"Contact",["SPHERE","PLANE"]).length,1);
 assert.deepEqual(contactRelationOptions(capabilities),["POINT"]);
 assert.ok(derivedSupportOptions("CIRCLE").some(option=>option.value==="underlying-circle"));
 assert.ok(derivedSupportOptions("FRAME").some(option=>option.value==="frame-plane-yz"));
 assert.deepEqual(derivedSupportOptions("FACE"),[]);
 const path={canonical:"a/nested",segments:[{instanceId:"a"},{instanceId:"nested"}]};
 const group={id:"group-a",kind:"FIX_TOGETHER",groupMembers:[{instanceId:"a",instancePath:path},{instanceId:"b"}]};
 const fields=assemblyPublicCommandFields("FIX_TOGETHER",{groupName:"renamed",groupMembers:["instance:b","instance:a","group:sub"]},group);
 assert.equal(fields.firstAssemblyRef,undefined);assert.equal(fields.secondAssemblyRef,undefined);
 const wire=JSON.parse(JSON.stringify({value:15,directionRelation:"UNORIENTED",distanceRelation:"UNSIGNED",angleAxis:{instanceId:"fake"},...fields}));
 for(const key of ["value","directionRelation","distanceRelation","angleAxis","quantityExpression"])assert.equal(key in wire,false,"group payload cannot retain generic binary/value fields");
 assert.deepEqual(fields.groupMembers,[{instanceId:"b"},{instanceId:"a",instancePath:path},{groupId:"sub"}]);
 assert.deepEqual(assemblyPublicCommandFields("FIX_TOGETHER",{groupMembers:["instance:a"]},undefined,[{instanceId:"a",instancePath:path,kind:"BODY"}]).groupMembers,[{instanceId:"a",instancePath:path}]);
 assert.throws(()=>assemblyPublicCommandFields("FIX_TOGETHER",{groupMembers:["display-name"]}),/稳定/);
 const nested={id:"nested",kind:"FIX_TOGETHER",groupMembers:[{groupId:group.id},{instanceId:"c"},{instanceId:"b"}]};
 assert.deepEqual(assemblyConstraintReferences(nested,[group,nested]).map(ref=>ref.instanceId),["a","b","c"]);
 const cyclic={...group,groupMembers:[{groupId:nested.id},{instanceId:"a"}]};
 assert.deepEqual(assemblyConstraintReferences(nested,[cyclic,nested]).map(ref=>ref.instanceId),["a","c","b"]);
 const legacy={id:"legacy-stable-id",kind:"RIGID",name:"Prior rigid",first:{instanceId:"a",kind:"BODY",instancePath:path},second:{instanceId:"b",kind:"BODY"},fixedPose:{translation:[1,2,3],rotation:[0,0,0,1]}};
 const frozen=JSON.stringify(legacy),draft=assemblyPublicEditDraft(legacy);
 assert.equal(JSON.stringify(legacy),frozen,"read/open/preview draft must not migrate immutable old definition");
 assert.equal(draft.id,legacy.id);assert.equal(draft.kind,"FIX_TOGETHER");assert.equal(draft.family,"FixTogether");assert.equal(draft.name,legacy.name);
 assert.deepEqual(draft.groupMembers,[{instanceId:"a",instancePath:path},{instanceId:"b"}]);
 const promotion=assemblyPublicCommandFields(draft.kind,{groupName:draft.name,groupMembers:["instance:a","instance:b"]},draft);
 assert.equal(promotion.fixedPose,undefined);assert.equal(promotion.constraintFamily,"FixTogether");
}finally{await server.close();}
