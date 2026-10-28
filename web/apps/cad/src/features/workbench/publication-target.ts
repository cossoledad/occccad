import type {ContextCatalogPublication,DocumentView,InstancePath,SelectionItem} from '../../types';
import {isSolidFeature} from './workbench-tree-model';
import {structureSelection} from './structure-selection';

function samePath(candidate:InstancePath|undefined,path:InstancePath):boolean{
 return !!candidate&&candidate.rootDocumentId===path.rootDocumentId&&candidate.canonical===path.canonical&&candidate.segments.length===path.segments.length&&candidate.segments.every((v,i)=>{const b=path.segments[i];return v.ownerDocumentId===b.ownerDocumentId&&v.ownerVersionId===b.ownerVersionId&&v.instanceId===b.instanceId&&v.referencedDocumentId===b.referencedDocumentId&&v.resolvedVersionId===b.resolvedVersionId});
}
/** Names and standard datum IDs are not authority across documents/occurrences. */
export function selectionInSnapshot(s:SelectionItem,documentId:string,revisionId:string,path?:InstancePath):boolean {
 if((s.ownerDocumentId??s.documentId)!==documentId||s.versionId!==revisionId||(s.occurrencePath??s.instancePath?.canonical??'')!==(path?.canonical??''))return false;
 if(!path)return !s.instancePath?.segments.length;
 return samePath(s.instancePath,path);
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

export function publicationForwardTarget(view:DocumentView,pick:SelectionItem|undefined,ownerPath?:InstancePath){
 let s=pick;
 // Tree picks in an activated module carry the outer Product path. Commands
 // on that module own a relative path rooted at its exact accepted Revision.
 if(ownerPath&&s?.instancePath){
  const outer=s.instancePath;
  if(outer.rootDocumentId!==ownerPath.rootDocumentId||outer.segments.length<=ownerPath.segments.length)return;
  const prefix={...outer,canonical:outer.segments.slice(0,ownerPath.segments.length).map(p=>p.instanceId).join('/'),segments:outer.segments.slice(0,ownerPath.segments.length)};
  const owner=ownerPath.segments.at(-1);
  if(!owner||owner.referencedDocumentId!==view.document.id||owner.resolvedVersionId!==view.document.versionId||!samePath(prefix,ownerPath))return;
  const segments=outer.segments.slice(ownerPath.segments.length);
  const relative={rootDocumentId:view.document.id,canonical:segments.map(p=>p.instanceId).join('/'),display:segments.map(p=>p.instanceName).join('/'),segments};
  s={...s,instanceId:segments[0].instanceId,instancePath:relative,occurrencePath:relative.canonical};
 }
 if(!view.product||s?.kind!=='publication'||!s.publicationId||!s.instancePath?.segments.length)return false;
 const path=s.instancePath,leaf=path.segments.at(-1)!;
 if(path.rootDocumentId!==view.document.id||path.segments[0].ownerVersionId!==view.document.versionId)return false;
 const visit=(node:NonNullable<DocumentView['structureTree']>):boolean=>{
  if((node.kind==='PUBLICATION'||node.kind==='PRODUCT_PUBLICATION')&&node.entityId===s.publicationId&&node.instancePath?.canonical===path.canonical&&selectionInSnapshot(s,leaf.referencedDocumentId,leaf.resolvedVersionId,node.instancePath))return true;
  return node.children?.some(visit)??false;
 };
 return view.structureTree&&visit(view.structureTree)?{instanceId:path.segments[0].instanceId,publicationId:s.publicationId,instancePath:path}:undefined;
}

/** A viewport hit on a private descendant may identify a public interface only
 * through its authoritative forwarding tree, with exactly one matching port. */
export function publicInterfaceForPick(root:DocumentView,entries:ContextCatalogPublication[],pick:SelectionItem){
 const raw=pick.highlightTarget??pick;
 const matches:ContextCatalogPublication[]=[];
 const visit=(node:NonNullable<DocumentView['structureTree']>)=>{
  if(node.kind==='PRODUCT_PUBLICATION'){
   const entry=entries.find(p=>p.publication.id===node.entityId&&p.instancePath.canonical===node.instancePath?.canonical&&p.revisionId===node.versionId);
   const target=entry?structureSelection(node,root)?.highlightTarget:undefined;
   if(entry&&target?.documentId&&target.versionId&&selectionInSnapshot(raw,target.documentId,target.versionId,target.instancePath)&&raw.kind===target.kind&&(
    raw.kind==='visual'&&target.kind==='visual'&&raw.featureId===target.featureId&&raw.entityId===target.entityId||
    ['plane','axis','axis-system'].includes(raw.kind)&&raw.entityId===target.entityId||
    (raw.kind==='face'||raw.kind==='edge'||raw.kind==='vertex')&&(target.kind==='face'||target.kind==='edge'||target.kind==='vertex')&&raw.geometryKey===target.geometryKey&&raw.topologyId===target.topologyId))matches.push(entry);
  }
  node.children?.forEach(visit);
 };
 if(root.structureTree)visit(root.structureTree);
 return matches.length===1?matches[0]:undefined;
}
