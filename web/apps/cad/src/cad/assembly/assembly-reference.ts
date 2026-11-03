import type { AssemblyGeometryRef, SelectionItem } from "../../types.ts";

export function assemblyGeometryRef(selection: SelectionItem): AssemblyGeometryRef | undefined {
  if (selection.kind === "publication") {
    const publication=selection.productPublication??selection.publication;
    if(!publication)return;
    const ownerPath=selection.instancePath?.segments.length?selection.instancePath:undefined;
    const sourcePath=selection.productPublication?.target.instancePath??selection.highlightTarget?.instancePath;
    const motionPath=ownerPath??(sourcePath?.segments.length?{...sourcePath,segments:sourcePath.segments.slice(0,1),canonical:sourcePath.segments[0].instanceId,display:sourcePath.segments[0].instanceName}:undefined);
    const instanceId=motionPath?.segments[0]?.instanceId??selection.instanceId;
    if(!instanceId)return;
    const publicationRef={publicationId:publication.id,expectedType:publication.type,compatibilityVersion:publication.compatibilityVersion,
      scope:ownerPath?'OCCURRENCE' as const:'LOCAL' as const,...(ownerPath?{ownerPath}:{})};
    const kind=({AXIS:'AXIS',PLANE:'PLANE',SURFACE:'FACE',CURVE:'EDGE',POINT:'VERTEX',FRAME:'FRAME',BODY:'BODY'} as const)[publication.type as 'AXIS'|'PLANE'|'SURFACE'|'CURVE'|'POINT'|'FRAME'|'BODY'];
    if(kind)return {instanceId,kind,publicationRef,...(motionPath?{instancePath:motionPath}:{})};
    return undefined;
  }
 if (!selection.instanceId) return undefined;
  const path = selection.instancePath ? { instancePath: selection.instancePath } : {};
  const publicationRef = selection.publicationId && selection.publication ? { publicationRef: {
    publicationId: selection.publicationId, expectedType: selection.publication.type,
    compatibilityVersion: selection.publication.compatibilityVersion,
    persistentSelection: selection.publication.target.persistentSelection,
  }} : {};
  if (selection.kind === "instance") return { instanceId: selection.instanceId, kind: "BODY", ...path, ...publicationRef };
  if (selection.kind === "body" && selection.publicationId) return { instanceId: selection.instanceId, kind: "BODY", ...path, ...publicationRef };
  if (selection.kind === "face" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "FACE", geometryKey: selection.geometryKey, topologyId: selection.topologyId, ...path, ...publicationRef };
  if (selection.kind === "edge" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "EDGE", geometryKey: selection.geometryKey, topologyId: selection.topologyId, ...path, ...publicationRef };
  if (selection.kind === "vertex" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "VERTEX", geometryKey: selection.geometryKey, topologyId: selection.topologyId, ...path, ...publicationRef };
  if (selection.kind === "plane" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "PLANE", geometryId: selection.entityId, ...path, ...publicationRef };
  if (selection.kind === "axis" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "AXIS", geometryId: selection.entityId,
      ...(selection.axis === "DATUM" ? {} : { axis: selection.axis }), ...path, ...publicationRef };
  if (["axis-system","datum-point"].includes(selection.kind) && selection.entityId)
    return { instanceId: selection.instanceId, kind: "FRAME", geometryId: selection.entityId, ...path, ...publicationRef };
  return undefined;
}

/** Convert an outer-tree selection to the active Product's typed relative scope. */
export function assemblyReferenceInContext(selection:SelectionItem,owner?:{id:string;versionId:string},ownerOccurrence=''){
 if(!ownerOccurrence||!owner)return assemblyGeometryRef(selection);
 const path=selection.instancePath;if(!path)return;
 const depth=ownerOccurrence.split('/').length,prefix=path.segments.slice(0,depth),last=prefix.at(-1);
 if(prefix.map(s=>s.instanceId).join('/')!==ownerOccurrence||last?.referencedDocumentId!==owner.id||last.resolvedVersionId!==owner.versionId)return;
 const segments=path.segments.slice(depth),relative=segments.length?{rootDocumentId:owner.id,canonical:segments.map(s=>s.instanceId).join('/'),display:segments.map(s=>s.instanceName).join('/'),segments}:undefined;
 return assemblyGeometryRef({...selection,instancePath:relative,occurrencePath:relative?.canonical,instanceId:segments[0]?.instanceId,rootDocumentId:owner.id});
}
