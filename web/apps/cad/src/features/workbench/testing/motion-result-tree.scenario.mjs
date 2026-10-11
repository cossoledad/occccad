import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {withMotionResults}=await server.ssrLoadModule('/src/features/workbench/motion-result-tree.ts');
 const view={document:{id:'p',versionId:'new'},product:{kinematics:{studies:[{id:'s',mechanismId:'m'}]}}};
 const tree=[{key:'study',kind:'MOTION_STUDY',entityId:'s',children:[{key:'reference',kind:'MECHANISM_REFERENCE'}]}];
 const job=(id,study='s',document='p')=>({id,documentId:document,type:'MOTION_STUDY',state:'SUCCEEDED',createdAt:id,payload:{studyId:study}});
 const projected=withMotionResults(tree,view,[job('j2'),job('j1'),job('other','other'),job('foreign','s','another')]);
 const results=projected[0].children.filter(c=>c.kind==='MOTION_RESULT');
 assert.deepEqual(results.map(r=>r.entityId),['j1','j2']);assert.equal(results[0].selection.mechanismId,'m');assert.equal(results[0].selection.ownerEntityId,'s');
 assert.equal(tree[0].children.length,1,'artifacts must not mutate model projections');
 const after=withMotionResults(tree,view,[job('j2')])[0].children.find(c=>c.kind==='MOTION_RESULT');assert.equal(after.key,results[1].key,'deletion/reordering cannot change Job identity');
}finally{await server.close()}
