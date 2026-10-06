import type {SelectionItem} from "../../types";

export type ProjectionScope = {documentId:string;versionId:string;occurrencePath?:string};
/** A projection source retains its own topology identity and editing occurrence. */
export function isExternalProjectionSource(selection:SelectionItem | null | undefined, scope?:ProjectionScope):selection is SelectionItem {
  if(!selection || !scope || !selection.versionId || selection.versionId!==scope.versionId ||
    (selection.ownerDocumentId??selection.documentId)!==scope.documentId ||
    (selection.occurrencePath??"")!==(scope.occurrencePath??""))return false;
  if(selection.kind==="edge" || selection.kind==="vertex")return !!selection.geometryKey && !!selection.topologyId;
  if(!selection.entityId)return false;
  return selection.kind==="axis" || selection.kind==="axis-system" || selection.kind==="datum-point";
}
