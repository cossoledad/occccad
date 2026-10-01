import type { AssemblyConstraint, AssemblyGeometryRef } from "../../types";

// An explicit editable draft only: opening/cancelling cannot rewrite a frozen
// legacy definition. The formal edit command owns promotion on confirmation.
export function assemblyPublicEditDraft(constraint:AssemblyConstraint):AssemblyConstraint {
  if(constraint.kind!=="RIGID")return {...constraint};
  return {...constraint,kind:"FIX_TOGETHER",family:"FixTogether",groupMembers:[constraint.first,constraint.second]
    .filter((reference):reference is AssemblyGeometryRef=>!!reference?.instanceId)
    .map(reference=>({instanceId:reference.instanceId,...(reference.instancePath?{instancePath:reference.instancePath}:{})}))};
}

export function assemblyPublicCommandFields(kind:string,fields:{contactKind?:string;contactSide?:string;contactBranch?:number;groupName?:string;groupMembers?:string[]},constraint?:AssemblyConstraint,references:AssemblyGeometryRef[]=[]) {
  if(kind==="CONTACT") return {constraintFamily:"Contact",contactKind:fields.contactKind,contactSide:fields.contactSide,contactBranch:fields.contactBranch};
  if(kind==="FIX_TOGETHER") return {constraintFamily:"FixTogether",firstAssemblyRef:undefined,secondAssemblyRef:undefined,
    value:undefined,directionRelation:undefined,distanceRelation:undefined,angleRelation:undefined,fixMode:undefined,fixedPose:undefined,
    angleAxis:undefined,reverseAngleAxis:undefined,angleReferenceDirection:undefined,quantityExpression:undefined,quantityKey:undefined,offsetExpression:undefined,offsetKey:undefined,
    groupName:fields.groupName,groupMembers:fields.groupMembers?.map(value=>{
    if(value.startsWith("group:"))return {groupId:value.slice(6)};
    if(!value.startsWith("instance:")||!value.slice(9))throw new Error("固联成员必须是稳定组件或组身份");
    const instanceId=value.slice(9),existing=constraint?.groupMembers?.find(m=>m.instanceId===instanceId)??references.find(reference=>reference.instanceId===instanceId);
    return {instanceId,...(existing?.instancePath?{instancePath:existing.instancePath}:{})};
  })};
  return {constraintFamily:constraint?.family};
}

export function assemblyPublicInitialFields(kind:string,references:AssemblyGeometryRef[],constraint?:AssemblyConstraint) {
  return {contactKind:constraint?.contactKind,contactSide:constraint?.contactSide??"EXTERNAL",contactBranch:constraint?.contactBranch??1,
    groupName:constraint?.name??"固联组",groupMembers:constraint?.groupMembers?.map(m=>m.groupId?`group:${m.groupId}`:`instance:${m.instanceId}`)??references.filter(r=>r.kind==="BODY").map(r=>`instance:${r.instanceId}`)};
}
