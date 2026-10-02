import type {AssemblyConstraint,AssemblyBodyFreedom,DocumentView as DocumentDescriptor} from "../../types";
import type {AssemblyConflictItem,AssemblyConflictMember} from "./assembly-conflict";
export type AssemblyEngineeringEvidence={documentId:string;revisionId:string;available:boolean;components:Array<{bodyIds:string[];relativeDof:number;gaugeDof:number;solved:boolean;freedoms:AssemblyBodyFreedom[]}>};

export function engineeringInstanceName(view:DocumentDescriptor|undefined,id:string):string {
  const instances=view?.product?.instances??[],instance=instances.find(value=>value.id===id);
  if(!instance)return "组件";
  const name=instance.name||"组件",siblings=instances.filter(value=>(value.name||"组件")===name);
  // Display-only disambiguation, never a persistent identity or repair target.
  return siblings.length>1?`${name}（第 ${siblings.findIndex(value=>value.id===id)+1} 个实例）`:name;
}

export function assemblyEngineeringOverview(view?:DocumentDescriptor){
  const definitions=view?.product?.constraints??[],instances=view?.product?.instances??[];
  const active=definitions.filter(c=>!c.suppressed&&c.mode!=="MEASURED");
  const issues=active.filter(c=>c.evaluationStatus!=="VERIFIED");
  const fixed=new Set(active.filter(c=>c.kind==="FIX"&&c.evaluationStatus==="VERIFIED").map(c=>c.first.instancePath?.segments[0]?.instanceId??c.first.instanceId));
  return {name:view?.document.name??"当前装配",total:definitions.length,suppressed:definitions.filter(c=>c.suppressed).length,
    measured:definitions.filter(c=>c.mode==="MEASURED").length,verified:active.filter(c=>c.evaluationStatus==="VERIFIED").length,
    broken:definitions.filter(c=>c.evaluationStatus==="BROKEN").length,notUpdated:definitions.filter(c=>c.evaluationStatus==="NOT_UPDATED").length,
    issues,instances:instances.map(instance=>({id:instance.id,name:engineeringInstanceName(view,instance.id),state:fixed.has(instance.id)?"已固定":
      active.some(c=>[c.first,c.second,c.angleAxis,...(c.groupMembers??[]).map(m=>({instanceId:m.instanceId,instancePath:m.instancePath}))].some(r=>r&&(r.instancePath?.segments[0]?.instanceId??r.instanceId)===instance.id))?"受约束；运动范围需查看求值证据":"可自由运动"})),
    status:issues.length?"有待处理定义":active.length?"活动定义已满足（当前求值）":"没有活动驱动约束"};
}
export function engineeringMember(definition:AssemblyConstraint):AssemblyConflictMember {
  return {constraintId:definition.id,first:definition.first,second:definition.second,angleAxis:definition.angleAxis,
    groupId:definition.kind==="FIX_TOGETHER"?definition.id:undefined,groupMembers:definition.groupMembers,
    repairActions:["EDIT","SUPPRESS",...(definition.evaluationStatus==="BROKEN"?["RECONNECT" as const]:[]),
      ...(definition.kind==="DISTANCE"||definition.kind==="ANGLE"&&["FREE","DIRECTED"].includes(definition.angleRelation??"FREE")?["MEASURE" as const]:[])]};
}
export function engineeringReason(item:AssemblyConflictItem):string {
  if(item.oracle==="UNSAT"&&["VERIFIED_IRREDUCIBLE","ANALYTIC_INCOMPATIBILITY"].includes(item.evidence))return "这些要求不能同时满足，请检查所列约束的参数或方向。";
  switch(item.kind){
    case "REDUNDANT":return "部分要求相互依赖；冗余不等于冲突，无需自动删除。";
    case "DEGENERATE":return "当前几何构型退化，需要检查支持和位置。";
    case "SUPPORT_UNAVAILABLE":return "支持来源不可用，请检查或重连。";
    case "SATISFIED":return "已找到满足所检查关系的位置。";
    default:return "尚未找到满足位置，需要进一步检查；未证明不可行。";
  }
}
export function engineeringConstraintName(definition:AssemblyConstraint,view?:DocumentDescriptor):string {
  const labels:Record<string,string>={FIX:"固定",FIX_TOGETHER:"固联组",CONTACT:"接触",DISTANCE:"偏移",ANGLE:"角度",COINCIDENT:"重合",CONCENTRIC:"同轴",RIGID:"固联"};
  const ref=(value:AssemblyConstraint["first"])=>{
    const id=value.instancePath?.segments[0]?.instanceId??value.instanceId;
    const label=engineeringInstanceName(view,id);
    return value.instancePath?.display?`${value.instancePath.display}${label.includes("个实例")?` · ${label}`:""}`:label;
  };
  return `${definition.name||labels[definition.kind]||"约束"} · ${ref(definition.first)}${definition.second?` → ${ref(definition.second)}`:""}`;
}
