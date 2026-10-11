import type {AssemblyGeometryRef,DocumentView,SelectionItem} from '../../types';
import type {MechanismJoint} from './motion-study';
import {assemblyGeometryRef} from './assembly-reference';
export type JointSupportRole='first.axis'|'second.axis'|'first.plane'|'second.plane';
export const jointSupportRoles:JointSupportRole[]=['first.axis','second.axis','first.plane','second.plane'];

export function changeJointSupport(joint:MechanismJoint,role:JointSupportRole,reference?:AssemblyGeometryRef):MechanismJoint {
 const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'];
 const endpoint=joint[side];if(!endpoint)return joint;
 const resetPlane=field==='axis'&&(!reference||endpoint.instanceId!==reference.instanceId);
 return {...joint,first:{...joint.first,capturedX:undefined},second:joint.second?{...joint.second,capturedX:undefined}:undefined,
  [side]:{...endpoint,instanceId:reference?.instanceId??endpoint.instanceId,[field]:reference,...(resetPlane?{plane:undefined}:{}),capturedX:undefined}};
}
export function nextJointSupportRole(joint:MechanismJoint,after:JointSupportRole):JointSupportRole|undefined {
 const start=jointSupportRoles.indexOf(after)+1;
 return [...jointSupportRoles.slice(start),...jointSupportRoles.slice(0,start)].find(role=>{
  const [side,field]=role.split('.') as ['first'|'second','axis'|'plane'];return !joint[side]?.[field];
 });
}
export function jointSupportLabel(view:DocumentView,reference:AssemblyGeometryRef):string {
 const unit=view.product?.instances.find(i=>i.id===reference.instanceId);
 const publicationName=(node:DocumentView['structureTree'],instanceId?:string):string|undefined=>{
  if(!node)return;
  const owner=node.occurrence?.instancePath.segments[0]?.instanceId??node.instancePath?.segments[0]?.instanceId??instanceId;
  if(owner===reference.instanceId&&node.entityId===reference.publicationRef?.publicationId&&node.kind==='PUBLICATION')return node.name;
  for(const child of node.children??[]){const name=publicationName(child,owner);if(name)return name;}
 };
 const derived:Record<string,string>={'cylinder-axis':'圆柱面轴线','circle-axis':'圆边轴线','cone-axis':'圆锥面轴线'};
 const geometry=reference.derivedRole?derived[reference.derivedRole]:reference.geometryId==='datum-xy'?'XY 平面':reference.geometryId==='datum-yz'?'YZ 平面':reference.geometryId==='datum-zx'?'ZX 平面':reference.axis?reference.axis+' 轴':reference.kind==='PLANE'||reference.kind==='FACE'?'平面':reference.kind==='EDGE'?'直边轴线':'轴线';
 const source=reference.publicationRef?publicationName(view.structureTree):reference.topologyId?`${reference.kind==='EDGE'?'边':'面'} ${reference.topologyId}`:undefined;
 return `${unit?.name??'零件'} · ${geometry??'几何支撑'}${source?' · '+source:''}`;
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
 const axis=role.endsWith('axis');
 if(!(axis?['AXIS','FACE','EDGE']:['PLANE','FACE']).includes(raw.kind))return false;
 const side=role.startsWith('first')?'first':'second',other=joint[side==='first'?'second':'first'];
 if(other?.axis&&raw.instanceId===other.instanceId)return false;
 return axis||raw.instanceId===joint[side]?.instanceId;
}
