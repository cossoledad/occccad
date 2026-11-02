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
 if(!s||!selectionInSnapshot(s,view.document.id,view.document.versionId,path))return;
 const scope={sourceDocumentId:view.document.id,versionId:view.document.versionId,instancePath:path};
 if(s.kind==='parameter'&&(view.part?.parameters??view.product?.parameters)?.some(p=>p.parameterId===s.id))return {...scope,publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:s.id};
 if(!view.part)return;
 if(s.kind==='plane'&&s.entityId&&view.part.datumPlanes?.some(p=>p.id===s.entityId))return {...scope,publicationType:'PLANE',targetKind:'PLANE',targetId:s.entityId};
 if(s.kind==='reference-point'&&s.entityId&&view.part.datumPoints?.some(p=>p.id===s.entityId))return {...scope,publicationType:'POINT',targetKind:'REFERENCE_POINT',targetId:s.entityId};
 if(s.kind==='axis-system'&&s.entityId&&view.part.axisSystems?.some(p=>p.id===s.entityId))return {...scope,publicationType:'FRAME',targetKind:'AXIS_SYSTEM',targetId:s.entityId};
 if(s.kind==='axis'&&s.entityId&&(s.axis==='DATUM'?view.part.datumAxes?.some(p=>p.id===s.entityId):view.part.axisSystems?.some(p=>p.id===s.entityId)))return {...scope,publicationType:'AXIS',targetKind:s.axis==='DATUM'?'DATUM_AXIS':'AXIS',targetId:s.entityId,axis:s.axis==='DATUM'?undefined:s.axis};
 if((s.kind==='face'||s.kind==='edge'||s.kind==='vertex')&&s.geometryKey&&view.part.bodies.some(b=>b.geometryKey===s.geometryKey))return {...scope,publicationType:s.kind==='face'?'SURFACE':s.kind==='edge'?'CURVE':'POINT',targetKind:s.kind.toUpperCase(),geometryKey:s.geometryKey,topologyId:s.topologyId};
 if(s.kind==='body'&&view.part.bodies.some(b=>b.id===(s.bodyId??s.entityId)))return {...scope,publicationType:'BODY',targetKind:'BODY',targetId:s.bodyId??s.entityId};
 if(s.kind==='pad'||s.kind==='import'||s.kind==='feature'){
  const feature=view.part.features.find(f=>f.id===(s.entityId??s.id)&&isSolidFeature(f));
  if(feature?.bodyId&&!feature.suppressed&&!['FAILED','BLOCKED'].includes(feature.evaluationStatus??''))return {...scope,publicationType:'BODY',targetKind:'BODY',targetId:feature.bodyId};
 }
 if(s.kind==='visual'){
  const entity=view.part.features.find(f=>f.id===s.featureId)?.sketch?.entities.find(e=>e.id===s.entityId);
  if(entity&&['POINT','LINE','CIRCLE','ARC'].includes(entity.kind))return {...scope,publicationType:entity.kind==='POINT'?'POINT':'CURVE',targetKind:'SKETCH_ENTITY',targetId:entity.id,sketchId:s.featureId};
 }
}

/** Geometry picked in Product stays owned by its occurrence; no hidden Part port. */
export function publicationElementTarget(view:DocumentView,pick:SelectionItem|undefined,ownerPath?:InstancePath){
 if(!view.product||!pick||pick.kind==='publication')return;
 let path=pick.instancePath;
 if(!path?.segments.length)return;
 if(ownerPath){
  if(path.rootDocumentId!==ownerPath.rootDocumentId||path.segments.length<=ownerPath.segments.length)return;
  const prefix={...path,canonical:path.segments.slice(0,ownerPath.segments.length).map(p=>p.instanceId).join('/'),segments:path.segments.slice(0,ownerPath.segments.length)};
  if(!samePath(prefix,ownerPath))return;
  const segments=path.segments.slice(ownerPath.segments.length);
  path={rootDocumentId:view.document.id,canonical:segments.map(p=>p.instanceId).join('/'),display:segments.map(p=>p.instanceName).join('/'),segments};
 }
 if(path.rootDocumentId!==view.document.id||path.segments[0].ownerVersionId!==view.document.versionId)return;
 const leaf=path.segments.at(-1)!;
 const local={...pick,instancePath:path,occurrencePath:path.canonical};
 if(!selectionInSnapshot(local,leaf.referencedDocumentId,leaf.resolvedVersionId,path))return;
 let partNode:NonNullable<DocumentView['structureTree']>|undefined;
 const visit=(n:NonNullable<DocumentView['structureTree']>)=>{if(n.kind==='PART'&&n.documentId===leaf.referencedDocumentId&&n.versionId===leaf.resolvedVersionId&&n.instancePath?.canonical===path!.canonical)partNode=n;n.children?.forEach(visit)};
 if(view.structureTree)visit(view.structureTree);if(!partNode)return;
 const nodes:NonNullable<DocumentView['structureTree']>[]=[];const collect=(n:typeof partNode)=>{if(n){nodes.push(n);n.children?.forEach(collect)}};collect(partNode);
 const scope={instancePath:path,sourceDocumentId:leaf.referencedDocumentId,sourceVersionId:leaf.resolvedVersionId};
 const entity=pick.entityId??pick.id;
 if((pick.kind==='face'||pick.kind==='edge'||pick.kind==='vertex')&&pick.geometryKey&&nodes.some(n=>n.kind==='BODY'&&n.geometryKey===pick.geometryKey&&n.bodyId===pick.bodyId))return {...scope,publicationType:pick.kind==='face'?'SURFACE':pick.kind==='edge'?'CURVE':'POINT',targetKind:pick.kind.toUpperCase(),geometryKey:pick.geometryKey,topologyId:pick.topologyId};
 if(pick.kind==='plane'&&nodes.some(n=>n.kind==='PLANE'&&n.entityId===entity))return {...scope,publicationType:'PLANE',targetKind:'PLANE',targetId:entity};
 if(pick.kind==='axis'&&nodes.some(n=>n.entityId===entity&&['DATUM_AXIS','AXIS'].includes(n.kind)))return {...scope,publicationType:'AXIS',targetKind:pick.axis==='DATUM'?'DATUM_AXIS':'AXIS',targetId:entity,axis:pick.axis==='DATUM'?undefined:pick.axis};
 if(pick.kind==='reference-point'&&nodes.some(n=>n.kind==='DATUM_POINT'&&n.entityId===entity))return {...scope,publicationType:'POINT',targetKind:'REFERENCE_POINT',targetId:entity};
 if(pick.kind==='axis-system'&&nodes.some(n=>n.kind==='AXIS'&&n.entityId===entity))return {...scope,publicationType:'FRAME',targetKind:'AXIS_SYSTEM',targetId:entity};
 if(pick.kind==='parameter'&&nodes.some(n=>n.kind==='PARAMETER'&&n.entityId===entity))return {...scope,publicationType:'PARAMETER',targetKind:'PARAMETER',targetId:entity};
 if(pick.kind==='visual'&&pick.featureId&&pick.entityId&&nodes.some(n=>n.entityId===pick.entityId&&n.ownerEntityId===pick.featureId))return {...scope,publicationType:pick.visualType==='POINT'?'POINT':'CURVE',targetKind:'SKETCH_ENTITY',targetId:pick.entityId,sketchId:pick.featureId};
 const body=nodes.find(n=>n.kind==='BODY'&&(n.bodyId===pick.bodyId||n.entityId===entity));
 if(pick.kind==='body'&&body)return {...scope,publicationType:'BODY',targetKind:'BODY',targetId:body.bodyId??body.entityId};
 if(['pad','import','feature'].includes(pick.kind)){const f=nodes.find(n=>n.entityId===entity&&['PAD','REVOLVE','IMPORT','FEATURE'].includes(n.kind));if(f?.bodyId&&!f.suppressed&&!['FAILED','BLOCKED'].includes(f.evaluationStatus??''))return {...scope,publicationType:'BODY',targetKind:'BODY',targetId:f.bodyId}}
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
 if(!view.product||!s?.publicationId||!s.instancePath?.segments.length)return false;
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
