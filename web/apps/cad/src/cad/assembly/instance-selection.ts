import type {DocumentView,Selection,SelectionItem} from '../../types';

// Resolve a visible sub-element to the direct instance owned by the editing
// Product. Nested descendants stay one rigid instance; names are presentation.
export function owningInstanceSelection(view:DocumentView,item:SelectionItem,ownerOccurrencePath=''):Selection {
 const source=item.highlightTarget??item,path=source.instancePath;
 const prefix=ownerOccurrencePath.split('/').filter(Boolean);
 const segment=path?.segments[prefix.length];
 if(path&&(prefix.some((id,i)=>path.segments[i]?.instanceId!==id)||segment?.ownerDocumentId!==view.document.id))return null;
 const id=segment?.instanceId??source.instanceId;
 if(!id||!view.product?.instances.some(v=>v.id===id))return null;
 const segments=path?.segments.slice(0,prefix.length+1),canonical=[...prefix,id].join('/');
 return {kind:'instance',id:canonical,instanceId:id,occurrencePath:canonical,visualKey:`occurrence:${canonical}`,
  documentId:segment?.referencedDocumentId??source.documentId,
  versionId:segment?.resolvedVersionId??source.versionId,
  ...(path&&segments?{rootDocumentId:path.rootDocumentId,instancePath:{...path,segments,canonical,display:segments.map(v=>v.instanceName).join('/')}}:{})};
}
