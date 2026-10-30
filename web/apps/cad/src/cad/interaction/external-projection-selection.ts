import type {DocumentView,SelectionItem} from "../../types";

export type ProjectionScope = {documentId:string;versionId:string;occurrencePath?:string;geometryKeys?:string[];datumIds?:string[]};
export function projectionScopeForView(view:DocumentView,occurrencePath?:string):ProjectionScope {
  return {documentId:view.document.id,versionId:view.document.versionId,occurrencePath,
    geometryKeys:view.part?.bodies.flatMap(b=>b.geometryKey?[b.geometryKey]:[]),
    datumIds:view.part?[...view.part.axisSystems,...view.part.datumPlanes,...(view.part.datumAxes??[])].map(d=>d.id):undefined};
}
/** A projection source retains its own topology identity and editing occurrence. */
export function isExternalProjectionSource(selection:SelectionItem | null | undefined, scope?:ProjectionScope):selection is SelectionItem {
  if(!selection || !scope || !selection.versionId || selection.versionId!==scope.versionId ||
    (selection.ownerDocumentId??selection.documentId)!==scope.documentId ||
    (selection.occurrencePath??"")!==(scope.occurrencePath??""))return false;
  if(selection.kind==="edge" || selection.kind==="vertex")return !!selection.geometryKey && !!selection.topologyId && (!scope.geometryKeys||scope.geometryKeys.includes(selection.geometryKey));
  if(!selection.entityId)return false;
  return (selection.kind==="axis" || selection.kind==="axis-system" || selection.kind==="datum-point") && (!scope.datumIds||scope.datumIds.includes(selection.entityId));
}
