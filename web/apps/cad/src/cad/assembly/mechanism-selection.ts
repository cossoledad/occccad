import {describeAssemblyReference} from "./assembly-reference-presentation";
import type {AssemblyGeometryRef,DocumentView,SelectionItem} from '../../types';
import type {MechanismJoint} from './motion-study';
import {assemblyGeometryRef} from './assembly-reference';
import {jointDefinitions} from './joint-definitions';
export type JointSupportRole='first.axis'|'second.axis'|'first.plane'|'second.plane'|'first.point'|'second.point'|'first.instance'|'second.instance';

export function changeJointSupport(joint:MechanismJoint,role:JointSupportRole,reference?:AssemblyGeometryRef):MechanismJoint {
 const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'|'point'|'instance'];
 const endpoint=joint[side];if(!endpoint)return joint;
 const resetSupports=!reference||endpoint.instanceId!==reference.instanceId;
 return {...joint,first:{...joint.first,capturedX:undefined},second:joint.second?{...joint.second,capturedX:undefined}:undefined,
  constraints:undefined,[side]:{...endpoint,...(resetSupports?{axis:undefined,plane:undefined,point:undefined}:{}),instanceId:reference?.instanceId??(field==='instance'?'':endpoint.instanceId),...(field==='instance'?{}:{[field]:reference}),capturedX:undefined}};
}
export function nextJointSupportRole(joint:MechanismJoint,after:JointSupportRole):JointSupportRole|undefined {
 const jointSupportRoles=jointDefinitions[joint.kind].roles;
 const start=jointSupportRoles.indexOf(after)+1;
 return [...jointSupportRoles.slice(start),...jointSupportRoles.slice(0,start)].find(role=>{
  const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'|'point'|'instance'];return field==='instance'?!joint[side]?.instanceId:!joint[side]?.[field];
 });
}
export function jointSupportLabel(view:DocumentView,reference:AssemblyGeometryRef):string {
 const info=describeAssemblyReference(reference,view);
 return `${info.primary} · ${info.secondary}`;
}

// Coarse identity/role admission precedes exact type inspection. The same
// predicate is used by the shared tree and ray-pick session, never by a second
// mouse controller. Exact inspection remains authoritative for curved faces.
export function mechanismSupportCandidate(view:DocumentView,joint:MechanismJoint,role:JointSupportRole,item:SelectionItem):boolean {
 const raw=assemblyGeometryRef(item);if(!raw)return false;
 const unit=view.product?.instances.find(u=>u.id===raw.instanceId);if(!unit)return false;
 const path=raw.instancePath;
 if(path){
  if(path.rootDocumentId!==view.document.id||path.segments[0]?.ownerVersionId!==view.document.versionId)return false;
  const current=view.resolvedInstances?.find(v=>v.instancePath.canonical===path.canonical)?.instancePath;
  if(!current||path.segments.length!==current.segments.length||path.segments.some((s,i)=>s.instanceId!==current.segments[i].instanceId||s.referencedDocumentId!==current.segments[i].referencedDocumentId||s.resolvedVersionId!==current.segments[i].resolvedVersionId))return false;
 }else if(item.documentId!==unit.documentId||item.versionId&&item.versionId!==unit.versionId)return false;
 const field=role.split('.')[1];
 const kinds=field==='axis'?['AXIS','FACE','EDGE','FRAME']:field==='plane'?['PLANE','FACE','FRAME']:field==='point'?['POINT','VERTEX','FACE','FRAME']:['BODY'];
 if(!kinds.includes(raw.kind))return false;
 const side=role.startsWith('first')?'first':'second',other=joint[side==='first'?'second':'first'];
 if(other?.instanceId&&raw.instanceId===other.instanceId)return false;
 const primary=jointDefinitions[joint.kind].roles.find(r=>r.startsWith(side));
 return role===primary||raw.instanceId===joint[side]?.instanceId;
}
