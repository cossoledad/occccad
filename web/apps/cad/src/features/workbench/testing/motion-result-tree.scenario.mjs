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
 assert(results.every(r=>r.capabilities.includes('DELETE')));
 const partial={...job('partial'),payload:{studyId:'s',motion:{status:'KINEMATIC_FAILED',completed:false,validFrames:3}}};assert.equal(withMotionResults(tree,view,[partial])[0].children.at(-1).title,'结果.1 · 部分结果');
 const failed={...partial,payload:{studyId:'s',motion:{status:'KINEMATIC_FAILED',completed:false,validFrames:0}}};assert.equal(withMotionResults(tree,view,[failed])[0].children.at(-1).title,'结果.1 · 未通过求解');
 const draft={id:'draft',name:'模拟.2',mechanismId:'m'};const group=[{key:'p/KINEMATICS_GROUP:simulations',kind:'KINEMATICS_GROUP',children:[]}];assert.equal(withMotionResults(group,view,[],draft)[0].children[0].entityId,'draft');assert.equal(withMotionResults(group,view,[])[0].children,undefined,'cancel releases the draft projection without altering the model');
 const running={...job('running'),state:'RUNNING'};
 assert(!withMotionResults(tree,view,[running])[0].children.at(-1).capabilities.includes('DELETE'),'running jobs must be stopped before deletion');
 const after=withMotionResults(tree,view,[job('j2')])[0].children.find(c=>c.kind==='MOTION_RESULT');assert.equal(after.key,results[1].key,'deletion/reordering cannot change Job identity');
}finally{await server.close()}
