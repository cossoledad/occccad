import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {owningInstanceSelection}=await server.ssrLoadModule('/src/cad/assembly/instance-selection.ts');
 const {featureSelectionHit,pickFeatureSelection}=await server.ssrLoadModule('/src/cad/interaction/feature-selection.ts');
 const view={document:{id:'root',versionId:'r'},product:{instances:[{id:'sub'}]}};
 const nested={kind:'face',id:'face',instancePath:{rootDocumentId:'root',canonical:'sub/part',segments:[{instanceId:'sub',instanceName:'重复装配.1',ownerDocumentId:'root',referencedDocumentId:'assembly'},{instanceId:'part',instanceName:'零件.1',ownerDocumentId:'assembly',referencedDocumentId:'part'}]}};
 const root=owningInstanceSelection(view,nested);
 assert.equal(root.instanceId,'sub');assert.equal(root.visualKey,'occurrence:sub');assert.equal(root.instancePath.segments.length,1);
 const editing={document:{id:'assembly'},product:{instances:[{id:'part'}]}};
 assert.equal(owningInstanceSelection(editing,nested,'sub').occurrencePath,'sub/part');
 assert.equal(owningInstanceSelection(editing,nested,'another'),null);
 assert.equal(owningInstanceSelection(view,{...nested,instancePath:{...nested.instancePath,segments:[{...nested.instancePath.segments[0],ownerDocumentId:'foreign'}]}}),null);
 let picked;
 const session={role:'instance',project:s=>owningInstanceSelection(view,s),accept:s=>Boolean(s.instanceId),onPick:s=>picked=s};
 assert.equal(featureSelectionHit(nested,session).kind,'instance');pickFeatureSelection(nested,session);assert.equal(picked.instanceId,'sub');
 pickFeatureSelection({kind:'plane',id:'root-plane'},session);assert.equal(picked.instanceId,'sub');
}finally{await server.close()}
