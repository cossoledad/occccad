import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {associationPlanCurrent,associationRepairGroups,acceptedSupportState,constraintSupports}=await server.ssrLoadModule('/src/features/workbench/association-repair-model.ts');
 const instance=(id,mode='FOLLOW_HEAD')=>({id,name:'Block',documentId:'part',versionId:'v1',referenceMode:mode});
 const instances=[instance('a'),instance('b'),instance('c','PINNED')];
 const view={document:{id:'product',versionId:'r1'},product:{instances,constraints:[{id:'mate',evaluationStatus:'VERIFIED'}]},structureTree:{children:instances.map(i=>({kind:'INSTANCE',entityId:i.id,ownerDocumentId:'product',documentType:'PART',instanceName:i.name,referenceName:'Part',instancePath:{canonical:i.id,display:'Block',segments:[{instanceId:i.id,resolvedVersionId:'v1'}]}}))}};
 const plan={rootProductDocumentId:'product',rootProductRevisionId:'r1',hasUpdates:true,canAccept:true,affectedConstraintIds:['mate'],parameterUpdates:[{documentId:'part',revisionId:'v2',needsUpdate:true}],entries:[...['a','b'].map(id=>({kind:'OCCURRENCE_REFERENCE',bindingId:'occurrence:'+id,candidateRevisionId:'v2',currency:'UPDATE_AVAILABLE'})),{kind:'ASSEMBLY_SOLVE',bindingId:'constraint:mate',evaluation:'FAILED'}]};
 assert(associationPlanCurrent(view,plan));const groups=associationRepairGroups(view,plan);assert.equal(groups.length,1);assert.equal(groups[0].instances.length,3);assert.deepEqual(groups[0].instances.map(r=>r.path.canonical),['a','b','c']);assert.equal(groups[0].instances[2].update,undefined);assert(groups[0].parameters[0].needsUpdate);
 assert(groups[0].instances.every(row=>row.diagnostics.length===0),'ASSEMBLY_SOLVE is not candidate diagnostics for occurrences');assert.equal(view.product.constraints[0].evaluationStatus,'VERIFIED');assert.equal(plan.canAccept,true,'constraint results cannot create an accept gate');
 const stale={...plan,rootProductRevisionId:'old'};assert.equal(associationPlanCurrent(view,stale),false);assert(associationRepairGroups(view,stale)[0].instances.every(row=>!row.update));assert.equal(associationRepairGroups(view,stale)[0].parameters.length,0);
 const support={instanceId:'b',kind:'FACE',resolution:{targetVersionId:'v1',result:{supportingElementStatus:'NOT_CONNECTED',diagnosticCode:'OUTSIDE_CURRENT_TIP',diagnostic:'removed'}}};assert.equal(acceptedSupportState(view,support).diagnosticCode,'OUTSIDE_CURRENT_TIP');assert.equal(acceptedSupportState(view,{...support,resolution:{...support.resolution,targetVersionId:'v2'}}).diagnosticCode,'SUPPORT_SNAPSHOT_STALE');
 assert.equal(acceptedSupportState(view,{instanceId:'missing',kind:'BODY'}).status,'NOT_CONNECTED');assert.equal(acceptedSupportState(view,{instanceId:'b',kind:'FACE',geometryKey:'x',topologyId:3}).status,'UNKNOWN','transient pick evidence is not an accepted resolution');
 assert.deepEqual(constraintSupports({kind:'FIX_TOGETHER',groupMembers:[{instanceId:'a'}]}),[],'groups have members rather than a fabricated first topology support');
 const pinnedPlan=structuredClone(plan);pinnedPlan.entries.push({kind:'OCCURRENCE_REFERENCE',bindingId:'occurrence:c',candidateRevisionId:'v2'});assert.equal(associationRepairGroups(view,pinnedPlan)[0].instances[2].update,undefined,'pinned row never projects a following candidate');
 const nested=structuredClone(view);nested.structureTree.children[1].documentType='PRODUCT';assert.equal(associationRepairGroups(nested,plan)[0].instances.length,2,'only direct Part rows are included');
 console.log('Association groups, pinned isolation, revision fences and accepted support evidence passed.');
}finally{await server.close()}
