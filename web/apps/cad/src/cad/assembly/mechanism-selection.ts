import type {DocumentView,SelectionItem} from '../../types';
import type {MechanismJoint} from './motion-study';
import {assemblyGeometryRef} from './assembly-reference';
export type JointSupportRole='first.axis'|'second.axis'|'first.plane'|'second.plane';

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
