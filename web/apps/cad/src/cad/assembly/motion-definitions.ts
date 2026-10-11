import type {KinematicsDefinitions} from './motion-study';
export const kinematicKinds=['MECHANISM','MECHANISM_JOINT','MOTION_DRIVER','MOTION_LAW','MOTION_STUDY'];
export function renameMotionObject(defs:KinematicsDefinitions,id:string,name:string):KinematicsDefinitions {
 const k=structuredClone(defs);let found=false;
 for(const m of k.mechanisms){for(const v of [m,...m.joints])if(v.id===id){v.name=name;found=true}}
 for(const v of [...(k.drivers??[]),...(k.laws??[]),...k.studies])if(v.id===id){v.name=name;found=true}
 if(!found||!name.trim())throw new Error('机构对象已变化或名称为空');return k;
}
export function deleteMotionObjects(defs:KinematicsDefinitions,ids:readonly string[]):KinematicsDefinitions {
 const k=structuredClone(defs),deleted=new Set(ids);
 k.mechanisms=k.mechanisms.filter(m=>{if(deleted.has(m.id)){for(const j of m.joints)deleted.add(j.id);return false}return true});
 for(const m of k.mechanisms){const before=m.joints.length;m.joints=m.joints.filter(j=>!deleted.has(j.id));if(m.joints.length===before)continue;/* Removing a relation releases DOF; it does not remove its rigid units. */m.evaluation=undefined}
 const live=new Set(k.mechanisms.map(m=>m.id));
 k.drivers=(k.drivers??[]).filter(d=>{if(deleted.has(d.id)||deleted.has(d.jointId)||!live.has(d.mechanismId)){deleted.add(d.id);return false}return true});
 k.laws=(k.laws??[]).filter(l=>{if(deleted.has(l.id)||!live.has(l.mechanismId)||deleted.has(l.driverId)){deleted.add(l.id);return false}return true});
 k.studies=k.studies.filter(s=>{if(deleted.has(s.id)||!live.has(s.mechanismId)||deleted.has(s.lawId)||deleted.has(s.driverId??s.driverJointId??'')){deleted.add(s.id);return false}return true});
 // Removing a joint or its publishing association never removes Product constraints.
 k.associations=(k.associations??[]).filter(a=>live.has(a.mechanismId)&&!deleted.has(a.jointId));return k;
}

// Only a completed local command can advance a selected definition to its new
// snapshot. Deleted objects lose selection; geometry references are untouched.
export function qualifyMotionSelections(view:import('../../types').DocumentView,selections:readonly import('../../types').SelectionItem[]):import('../../types').SelectionItem[]{
 const k=view.product?.kinematics;
 const live=new Set([...(k?.mechanisms??[]).flatMap(m=>[m.id,...m.joints.flatMap(j=>[j.id,...(j.constraints??[]).map(c=>c.id)])]),...(k?.drivers??[]).map(d=>d.id),...(k?.laws??[]).map(l=>l.id),...(k?.studies??[]).map(s=>s.id)]);
 return selections.flatMap(s=>s.kind!=='kinematic-object'||(s.ownerDocumentId??s.documentId)!==view.document.id?[s]:live.has(s.entityId??s.id)?[{...s,versionId:view.document.versionId,snapshotScope:{...s.snapshotScope,revisionId:view.document.versionId}}]:[]);
}

// Project semantic Applications selections to every descendant Body of its
// owning rigid units. Tree identity stays on the actual application object.
export function motionSelectionTargets(view:import('../../types').DocumentView,selections:readonly import('../../types').SelectionItem[]):import('../../types').SelectionItem[]{
 const defs=view.product?.kinematics;if(!defs)return [...selections];
 return selections.flatMap(selection=>{
  if(selection.kind!=='kinematic-object')return [selection];
  const m=defs.mechanisms.find(m=>m.id===selection.mechanismId||m.id===selection.id||m.joints.some(j=>j.id===selection.id||j.constraints?.some(c=>c.id===selection.id))||defs.drivers?.some(d=>d.id===selection.id&&d.mechanismId===m.id)||defs.laws?.some(l=>l.id===selection.id&&l.mechanismId===m.id)||defs.studies.some(s=>s.id===selection.id&&s.mechanismId===m.id));
  const j=m?.joints.find(j=>j.id===selection.id||j.constraints?.some(c=>c.id===selection.id));const unitIds=j?[j.first.instanceId,...(j.second?[j.second.instanceId]:[])]:m?.unitIds??[];
  if(j&&selection.id!==j.id)return [selection]; // exact relation/support highlight belongs to the shared renderer

  return [selection,...(view.resolvedInstances??[]).filter(v=>unitIds.includes(v.instancePath.segments[0]?.instanceId)).map(v=>({kind:'body' as const,id:`${v.occurrencePath}:body:${v.bodyId}`,bodyId:v.bodyId,documentId:v.instancePath.segments.at(-1)?.referencedDocumentId,versionId:v.instancePath.segments.at(-1)?.resolvedVersionId,instanceId:v.instancePath.segments[0]?.instanceId,instancePath:v.instancePath,occurrencePath:v.occurrencePath,geometryKey:v.geometryKey}))];
 });
}
