import type { AssemblyConstraint, AssemblyGeometryRef } from "../../types";

export type AssemblyCapability = {capabilityId:string;family:string;subtype:string;roles:Array<{role:string;descriptor:string;supportedDescriptors?:string[]}>};
export type AssemblyInspectedSupport = {status:string;constraintEligible?:boolean;constraintDiagnosticCode?:string;constraintDiagnostic?:string};
// Inspection includes source metadata followed by the actual derived supports.
// Only the actual target half controls submission; a trimmed source can validly
// supply a centre/axis/plane or explicitly selected underlying circle.
export function assemblyTargetSupportsEligible(supports:AssemblyInspectedSupport[]|undefined):boolean {
 return !!supports?.length&&supports.every(support=>support.status==="RESOLVED"&&support.constraintEligible===true);
}
export function matchingAssemblyCapabilities(capabilities:AssemblyCapability[]|undefined,family:string,types:string[]|undefined):AssemblyCapability[]{
  if(!capabilities||!types?.length||types.some(type=>!type)) return [];
  return capabilities.filter(capability=>capability.family===family&&capability.roles.length===types.length&&(
    capability.roles.every((role,index)=>(role.supportedDescriptors??[role.descriptor]).includes(types[index])) ||
    types.length===2&&(capability.roles[0].supportedDescriptors??[capability.roles[0].descriptor]).includes(types[1])&&(capability.roles[1].supportedDescriptors??[capability.roles[1].descriptor]).includes(types[0])));
}
export function contactRelationOptions(capabilities:AssemblyCapability[]):string[]{
 return [...new Set(capabilities.map(c=>c.subtype.split("-").at(-1)?.toUpperCase()).filter((s):s is string=>!!s))];
}
export function derivedSupportOptions(sourceType:string|undefined):Array<{value:string;label:string}>{
 const identity={value:"",label:"完整精确支持"};
 switch(sourceType){
  case "SPHERE":return [identity,{value:"sphere-center",label:"球心（点）"}];
  case "CYLINDER":return [identity,{value:"cylinder-axis",label:"圆柱轴（无限直线）"}];
  case "CONE":return [identity,{value:"cone-apex",label:"锥顶（点）"},{value:"cone-axis",label:"锥轴（无限直线）"}];
  case "CIRCLE":return [{value:"",label:"所选圆/圆弧（保留参数域）"},{value:"underlying-circle",label:"Underlying circle（完整数学支撑）"},{value:"circle-center",label:"圆心（点）"},{value:"circle-axis",label:"圆法向轴（无限直线）"},{value:"circle-plane",label:"圆支撑平面"}];
  case "FRAME":return [identity,{value:"frame-origin",label:"坐标系原点"},...(["x","y","z"] as const).map(axis=>({value:`frame-axis-${axis}`,label:`${axis.toUpperCase()} 轴`})),...(["xy","yz","zx"] as const).map(plane=>({value:`frame-plane-${plane}`,label:`${plane.toUpperCase()} 基准面`}))];
  default:return [];
 }
}
// Visual references only. Group identity/lifecycle remains in the domain; this
// projection must never substitute mesh centres for precise solver geometry.
export function assemblyConstraintReferences(constraint:AssemblyConstraint,constraints:AssemblyConstraint[],seen=new Set<string>()):AssemblyGeometryRef[]{
 if(constraint.kind!=="FIX_TOGETHER") return [constraint.first,constraint.second].filter((r):r is AssemblyGeometryRef=>!!r?.instanceId);
 if(seen.has(constraint.id))return [];seen.add(constraint.id);
 const references=(constraint.groupMembers??[]).flatMap(member=>{
  if(member.groupId){const group=constraints.find(c=>c.id===member.groupId);return group?assemblyConstraintReferences(group,constraints,seen):[];}
  return member.instanceId?[{instanceId:member.instanceId,instancePath:member.instancePath,kind:"BODY" as const}]:[];
 });
 return references.filter((reference,index)=>references.findIndex(other=>other.instanceId===reference.instanceId&&JSON.stringify(other.instancePath)===JSON.stringify(reference.instancePath))===index);
}
