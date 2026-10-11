import type {DocumentView,Job} from '../../types';
import type {SpecificationTreeNode} from './specification-tree';

const states:Record<string,string>={QUEUED:'等待',RUNNING:'计算中',SUCCEEDED:'完成',FAILED:'失败',CANCELED:'已停止'};
// Job IDs identify retained result artifacts, not new model definitions.
export function withMotionResults(nodes:SpecificationTreeNode[],view:DocumentView,jobs:readonly Job[]):SpecificationTreeNode[]{
 return nodes.map(node=>{
  const children=node.children?withMotionResults(node.children,view,jobs):[];
  if(node.kind==='MOTION_STUDY'){
   const study=view.product?.kinematics?.studies.find(s=>s.id===node.entityId);
   const results=jobs.filter(j=>j.documentId===view.document.id&&j.type==='MOTION_STUDY'&&j.payload.studyId===node.entityId)
    .sort((a,b)=>a.createdAt.localeCompare(b.createdAt)||a.id.localeCompare(b.id));
   children.push(...results.map((job,index)=>({key:node.key+'/result:'+job.id,kind:'MOTION_RESULT',entityId:job.id,
    documentId:view.document.id,ownerDocumentId:view.document.id,ownerEntityId:node.entityId,
    title:`结果.${index+1} · ${states[job.state]??job.state}`,capabilities:['ACTIVATE' as const,...(['SUCCEEDED','FAILED','CANCELED'].includes(job.state)?['DELETE' as const]:[])],
    selection:{kind:'kinematic-object' as const,id:job.id,entityKind:'MOTION_RESULT',documentId:view.document.id,
     ownerDocumentId:view.document.id,ownerEntityId:node.entityId,mechanismId:study?.mechanismId}})));
  }
  return {...node,children:children.length?children:undefined};
 });
}
