import type { AssemblyGeometryRef, SelectionItem } from "../../types.ts";

export function assemblyGeometryRef(selection: SelectionItem): AssemblyGeometryRef | undefined {
  if (selection.kind === "publication") {
    const forwarded = selection.productPublication;
    const path = forwarded?.target.instancePath ?? selection.instancePath;
    const instanceId = selection.instanceId ?? path?.segments[0]?.instanceId ?? selection.highlightTarget?.instancePath?.segments[0]?.instanceId;
    const publication = selection.publication ?? forwarded;
    const publicationRef = publication ? {
      publicationId: forwarded?.target.publicationId ?? publication.id,
      expectedType: publication.type,
      compatibilityVersion: publication.compatibilityVersion,
      persistentSelection: selection.publication?.target.persistentSelection,
    } : undefined;
    const target = selection.highlightTarget ? assemblyGeometryRef({ ...selection.highlightTarget, instanceId }) : undefined;
    if (target) return publicationRef ? { ...target, publicationRef } : target;
    const kind = ({ AXIS: "AXIS", PLANE: "PLANE", SURFACE: "FACE", CURVE: "EDGE", POINT: "VERTEX" } as const)[publication?.type as "AXIS" | "PLANE" | "SURFACE" | "CURVE" | "POINT"];
    if (instanceId && kind && publicationRef) return { instanceId, kind, publicationRef, ...(path ? { instancePath: path } : {}) };
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
