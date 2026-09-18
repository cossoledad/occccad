import type { AssemblyGeometryRef, SelectionItem } from "../../types.ts";

export function assemblyGeometryRef(selection: SelectionItem): AssemblyGeometryRef | undefined {
  if (!selection.instanceId) return undefined;
  const publicationRef = selection.publicationId && selection.publication ? { publicationRef: {
    publicationId: selection.publicationId, expectedType: selection.publication.type,
    compatibilityVersion: selection.publication.compatibilityVersion,
    persistentSelection: selection.publication.target.persistentSelection,
  }} : {};
  if (selection.kind === "instance") return { instanceId: selection.instanceId, kind: "BODY", ...publicationRef };
  if (selection.kind === "body" && selection.publicationId) return { instanceId: selection.instanceId, kind: "BODY", ...publicationRef };
  if (selection.kind === "face" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "FACE", geometryKey: selection.geometryKey, topologyId: selection.topologyId, ...publicationRef };
  if (selection.kind === "edge" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "EDGE", geometryKey: selection.geometryKey, topologyId: selection.topologyId, ...publicationRef };
  if (selection.kind === "vertex" && selection.geometryKey && selection.topologyId)
    return { instanceId: selection.instanceId, kind: "VERTEX", geometryKey: selection.geometryKey, topologyId: selection.topologyId, ...publicationRef };
  if (selection.kind === "plane" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "PLANE", geometryId: selection.entityId, ...publicationRef };
  if (selection.kind === "axis" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "AXIS", geometryId: selection.entityId,
      ...(selection.axis === "DATUM" ? {} : { axis: selection.axis }), ...publicationRef };
  if (selection.kind === "axis-system" && selection.entityId)
    return { instanceId: selection.instanceId, kind: "POINT", geometryId: selection.entityId, ...publicationRef };
  return undefined;
}
