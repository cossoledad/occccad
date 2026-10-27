import type {DocumentView,InstancePath,SelectionItem} from '../../types';
import {isSolidFeature} from './workbench-tree-model';

/** Names and standard datum IDs are not authority across documents/occurrences. */
export function selectionInSnapshot(s:SelectionItem,documentId:string,revisionId:string,path?:InstancePath):boolean {
 if((s.ownerDocumentId??s.documentId)!==documentId||s.versionId!==revisionId||(s.occurrencePath??s.instancePath?.canonical??'')!==(path?.canonical??''))return false;
 if(!path)return !s.instancePath?.segments.length;
 const candidate=s.instancePath;
 return !!candidate&&candidate.rootDocumentId===path.rootDocumentId&&candidate.canonical===path.canonical&&candidate.segments.length===path.segments.length&&candidate.segments.every((v,i)=>{const b=path.segments[i];return v.ownerDocumentId===b.ownerDocumentId&&v.ownerVersionId===b.ownerVersionId&&v.instanceId===b.instanceId&&v.referencedDocumentId===b.referencedDocumentId&&v.resolvedVersionId===b.resolvedVersionId});
}
export function publicationSelectionTarget(view:DocumentView,s:SelectionItem|undefined,path?:InstancePath){
 if(!s||!view.part||!selectionInSnapshot(s,view.document.id,view.document.versionId,path))return;
 const scope={sourceDocumentId:view.document.id,versionId:view.document.versionId,instancePath:path};
 if(s.kind==='plane'&&s.entityId&&view.part.datumPlanes?.some(p=>p.id===s.entityId))return {...scope,publicationType:'PLANE',targetKind:'PLANE',targetId:s.entityId};
 if(s.kind==='axis-system'&&s.entityId&&view.part.axisSystems?.some(p=>p.id===s.entityId))return {...scope,publicationType:'FRAME',targetKind:'AXIS_SYSTEM',targetId:s.entityId};
 if(s.kind==='axis'&&s.entityId&&(s.axis==='DATUM'?view.part.datumAxes?.some(p=>p.id===s.entityId):view.part.axisSystems?.some(p=>p.id===s.entityId)))return {...scope,publicationType:'AXIS',targetKind:s.axis==='DATUM'?'DATUM_AXIS':'AXIS',targetId:s.entityId,axis:s.axis==='DATUM'?undefined:s.axis};
 if((s.kind==='face'||s.kind==='edge')&&s.geometryKey&&view.part.bodies.some(b=>b.geometryKey===s.geometryKey))return {...scope,publicationType:s.kind==='face'?'SURFACE':'CURVE',targetKind:s.kind.toUpperCase(),geometryKey:s.geometryKey,topologyId:s.topologyId};
 if(s.kind==='body'&&view.part.bodies.some(b=>b.id===(s.bodyId??s.entityId)))return {...scope,publicationType:'BODY',targetKind:'BODY',targetId:s.bodyId??s.entityId};
 if((s.kind==='pad'||s.kind==='import'||s.kind==='feature')&&view.part.features.some(f=>f.id===(s.entityId??s.id)&&isSolidFeature(f)))return {...scope,publicationType:'BODY',targetKind:'FEATURE_OUTPUT',targetId:s.entityId??s.id};
 if(s.kind==='parameter'&&view.part.parameters?.some(p=>p.parameterId===s.id))return {...scope,publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:s.id};
 if(s.kind==='visual'){
  const entity=view.part.features.find(f=>f.id===s.featureId)?.sketch?.entities.find(e=>e.id===s.entityId);
  if(entity&&['POINT','LINE','CIRCLE','ARC'].includes(entity.kind))return {...scope,publicationType:entity.kind==='POINT'?'POINT':'CURVE',targetKind:'SKETCH_ENTITY',targetId:entity.id,sketchId:s.featureId};
 }
}

export function publicationForwardSource(view:DocumentView,s:SelectionItem|undefined):boolean {
 if(!view.product||s?.kind!=='publication'||!s.publicationId||!s.instancePath?.segments.length)return false;
 const path=s.instancePath,leaf=path.segments.at(-1)!;
 if(path.rootDocumentId!==view.document.id||path.segments[0].ownerVersionId!==view.document.versionId)return false;
 const visit=(node:NonNullable<DocumentView['structureTree']>):boolean=>{
  if(node.kind==='PUBLICATION'&&node.entityId===s.publicationId&&node.instancePath?.canonical===path.canonical&&selectionInSnapshot(s,leaf.referencedDocumentId,leaf.resolvedVersionId,node.instancePath))return true;
  return node.children?.some(visit)??false;
 };
 return !!view.structureTree&&visit(view.structureTree);
}
