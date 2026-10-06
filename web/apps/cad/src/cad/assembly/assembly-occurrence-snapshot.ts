import type {DocumentView as DocumentDescriptor, InstancePath, SelectionItem} from "../../types";

/** Resolve by stable occurrence identity in ONE authoritative projection. Never
 * patch revision fields on a path captured by an older render/selection. */
export function occurrenceSnapshot(view:DocumentDescriptor, canonical:string):InstancePath|undefined {
  const resolved=view.resolvedInstances?.find(value=>value.occurrencePath===canonical);
  if(resolved)return structuredClone(resolved.instancePath);
  const visit=(nodes:NonNullable<DocumentDescriptor["structureTree"]>[]):InstancePath|undefined=>{
    for(const node of nodes){
      if(node.instancePath?.canonical===canonical)return structuredClone(node.instancePath);
      const found=visit(node.children??[]);if(found)return found;
    }
  };
  return view.structureTree?visit([view.structureTree]):undefined;
}

export function refreshOccurrenceSelection(view:DocumentDescriptor, selection:SelectionItem):SelectionItem {
  const canonical=selection.occurrencePath??selection.instancePath?.canonical;
  if(!canonical){
    if((selection.ownerDocumentId??selection.documentId)!==view.document.id)return selection;
    // A reused GPU object still belongs to the new root Part snapshot. Never
    // promote topology from a replaced geometry artifact.
    if(selection.geometryKey&&!Object.values(view.artifacts??{}).some(artifact=>artifact.geometryKey===selection.geometryKey))return selection;
    return {...selection,versionId:view.document.versionId,
      ...(selection.snapshotScope?{snapshotScope:{...selection.snapshotScope,revisionId:view.document.versionId}}:{})};
  }
  const path=occurrenceSnapshot(view,canonical);
  if(!path)return selection;
  const last=path.segments.at(-1);
  const versionId=last?.resolvedVersionId??selection.versionId;
  return {...selection,instancePath:path,versionId,documentId:last?.referencedDocumentId??selection.documentId,
    ...(selection.ownerDocumentId?{ownerDocumentId:selection.kind==="instance"?last?.ownerDocumentId:last?.referencedDocumentId}:{}),
    contextVariantKey:view.contextVariants?.find(variant=>variant.owningInstancePath.canonical===canonical)?.variantKey,
    ...(selection.occurrenceRef?{occurrenceRef:{rootDocumentId:path.rootDocumentId,instancePath:path}}:{}),
    ...(selection.snapshotScope&&versionId?{snapshotScope:{...selection.snapshotScope,revisionId:versionId}}:{})};
}
