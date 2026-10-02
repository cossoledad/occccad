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
  if(!canonical)return selection;
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
