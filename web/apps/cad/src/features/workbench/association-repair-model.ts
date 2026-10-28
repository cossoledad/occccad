import type {AssemblyConstraint,AssemblyGeometryRef,DocumentView,ProductUpdatePlan} from "../../types";

export function associationPlanCurrent(view:DocumentView,plan?:ProductUpdatePlan){
 return !!plan&&plan.rootProductDocumentId===view.document.id&&plan.rootProductRevisionId===view.document.versionId;
}
export function associationRepairGroups(view:DocumentView,plan?:ProductUpdatePlan){
 const current=associationPlanCurrent(view,plan),nodes=view.structureTree?.children??[];
 const direct=(view.product?.instances??[]).flatMap(instance=>{
  const node=nodes.find(n=>n.kind==="INSTANCE"&&n.entityId===instance.id&&n.ownerDocumentId===view.document.id);
  if(!['PART','PRODUCT'].includes(node?.documentType??'')||node?.instancePath?.segments.length!==1)return [];
  const path=node.instancePath;
  const update=current&&instance.referenceMode!=="PINNED"?plan!.entries.find(e=>e.kind==="OCCURRENCE_REFERENCE"&&e.bindingId===`occurrence:${path.canonical}`):undefined;
  const bindings=(view.product?.contextBindings??[]).filter(b=>[b.sourceInstancePath,b.owningInstancePath].some(p=>p.segments.length===1&&p.segments[0].instanceId===instance.id));
  const diagnostics=current?plan!.entries.filter(e=>e.kind==="CONTEXT_BINDING"&&bindings.some(b=>b.id===e.bindingId)):[];
  return [{instance,path,name:node.instanceName??instance.name,partName:node.referenceName??"关联零件",update,diagnostics}];
 });
 return [...new Set(direct.map(i=>i.instance.documentId))].map(documentId=>({documentId,name:direct.find(i=>i.instance.documentId===documentId)!.partName,instances:direct.filter(i=>i.instance.documentId===documentId),parameters:current?plan!.parameterUpdates?.filter(p=>p.documentId===documentId)??[]:[]}));
}
export function acceptedSupportState(view:DocumentView,reference:AssemblyGeometryRef){
 const id=reference.instancePath?.segments[0]?.instanceId??reference.instanceId;
 const instance=view.product?.instances.find(i=>i.id===id);
 if(!instance)return {status:"NOT_CONNECTED",label:"支撑已断开",diagnosticCode:"INSTANCE_UNAVAILABLE",diagnostic:"支持所属实例不在当前已接受 Product 中。"};
 if(reference.publicationResolution&&reference.publicationResolution.status!=="CONNECTED")return {status:"NOT_CONNECTED",label:"支撑已断开",diagnosticCode:reference.publicationResolution.diagnosticCode,diagnostic:reference.publicationResolution.diagnostic};
 if(reference.resolution){
  if(reference.resolution.targetVersionId!==instance.versionId)return {status:"NOT_CONNECTED",label:"支撑已断开",diagnosticCode:"SUPPORT_SNAPSHOT_STALE",diagnostic:"支持解析快照不属于该实例的已接受版本。"};
  const r=reference.resolution.result;return {status:r.supportingElementStatus,label:r.supportingElementStatus==="CONNECTED"?"有效":"支撑已断开",diagnosticCode:r.diagnosticCode,diagnostic:r.diagnostic};
 }
 return reference.kind==="BODY"||reference.capturedFrame?{status:"CONNECTED",label:"有效"}:{status:"UNKNOWN",label:"待检查",diagnosticCode:"RESOLUTION_UNAVAILABLE",diagnostic:"当前已接受 Revision 没有支持解析快照。"};
}
export function constraintSupports(c:AssemblyConstraint){
 if(c.kind==="FIX_TOGETHER")return [];
 return [{label:"支持 1",reference:c.first},...(c.second?[{label:"支持 2",reference:c.second}]:[]),...(c.angleAxis?[{label:"角度轴",reference:c.angleAxis}]:[])];
}
