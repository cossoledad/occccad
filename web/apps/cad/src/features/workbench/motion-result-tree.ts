import type {DocumentView,Job} from '../../types';
import type {SpecificationTreeNode} from './specification-tree';
import type {MotionStudy} from '../../cad/assembly/motion-study';

const states:Record<string,string>={QUEUED:'等待',RUNNING:'计算中',SUCCEEDED:'完成',FAILED:'失败',CANCELED:'已停止'};
function resultState(job:Job):string {
 const motion=job.payload.motion as {status?:string;completed?:boolean;validFrames?:number}|undefined;
 if(motion){if(motion.completed)return '完成';return (motion.validFrames??0)>0?'部分结果':'未通过求解'}
 return states[job.state]??'待检查';
}
// Job IDs identify retained result artifacts, not new model definitions.
export function withMotionResults(nodes:SpecificationTreeNode[],view:DocumentView,jobs:readonly Job[],draft?:MotionStudy):SpecificationTreeNode[]{
 return nodes.map(node=>{
  const children=node.children?withMotionResults(node.children,view,jobs,draft):[];
  if(draft&&node.kind==='KINEMATICS_GROUP'&&node.key.endsWith('/KINEMATICS_GROUP:simulations')){
   const existing=children.find(n=>n.entityId===draft.id);
   if(existing)existing.title=draft.name+' · 编辑中';
   else children.push({key:node.key+'/draft:'+draft.id,kind:'MOTION_STUDY',entityId:draft.id,documentId:view.document.id,ownerDocumentId:view.document.id,ownerEntityId:draft.mechanismId,title:draft.name+' · 编辑中',capabilities:[]});
  }
  if(node.kind==='MOTION_STUDY'){
   const study=view.product?.kinematics?.studies.find(s=>s.id===node.entityId);
   const results=jobs.filter(j=>j.documentId===view.document.id&&j.type==='MOTION_STUDY'&&j.payload.studyId===node.entityId)
    .sort((a,b)=>a.createdAt.localeCompare(b.createdAt)||a.id.localeCompare(b.id));
   children.push(...results.map((job,index)=>({key:node.key+'/result:'+job.id,kind:'MOTION_RESULT',entityId:job.id,
    documentId:view.document.id,ownerDocumentId:view.document.id,ownerEntityId:node.entityId,
    title:`结果.${index+1} · ${resultState(job)}`,capabilities:['ACTIVATE' as const,...(['SUCCEEDED','FAILED','CANCELED'].includes(job.state)?['DELETE' as const]:[])],
    selection:{kind:'kinematic-object' as const,id:job.id,entityKind:'MOTION_RESULT',documentId:view.document.id,
     ownerDocumentId:view.document.id,ownerEntityId:node.entityId,mechanismId:study?.mechanismId}})));
  }
  return {...node,children:children.length?children:undefined};
 });
}
