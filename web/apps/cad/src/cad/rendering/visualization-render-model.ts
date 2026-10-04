import type { InstancePath, Selection, VisualPrimitive } from "../../types";

export type VisualOccurrenceContext = {
  documentId: string;
  versionId?: string;
  bodyId?: string;
  contextVariantKey?: string;
  instancePath?: InstancePath;
  geometryKey: string;
  occurrencePath: string;
  treeNodeId?: string;
  instanceId?: string;
};

export function visualType(primitive: VisualPrimitive): "POINT" | "CURVE" | "SURFACE" {
  if (primitive.kind === "POINTS") return "POINT";
  if (primitive.kind === "POLYLINE" || primitive.kind === "LINE_SEGMENTS") return "CURVE";
  return "SURFACE";
}

export function visualSelection(
  primitive: VisualPrimitive,
  context: VisualOccurrenceContext,
): Exclude<Selection, null> {
  if (primitive.semantic === "SKETCH_CONSTRAINT") return {
    kind: "sketch-constraint",
    id: `${context.occurrencePath || "root"}:${primitive.featureId}:constraint:${primitive.id}`,
    featureId: primitive.featureId,
    constraintId: primitive.id,
    constraintType: primitive.entityType ?? "UNKNOWN",
    treeNodeId: context.treeNodeId,
    documentId: context.documentId,
    versionId: context.versionId,
    bodyId: context.bodyId,
    contextVariantKey: context.contextVariantKey,
    instancePath: context.instancePath,
    occurrencePath: context.occurrencePath,
    geometryKey: context.geometryKey,
    instanceId: context.instanceId,
  };
  return {
    kind: "visual",
    patternId:primitive.patternId,patternMemberSlot:primitive.patternMemberSlot,sketchReference:primitive.pointReference,
    id: `${context.occurrencePath || "root"}:${primitive.featureId}:${primitive.id}`,
    visualType: visualType(primitive),
    featureId: primitive.featureId,
    entityId: primitive.displayEntityId??primitive.id,
    role: primitive.role,
    treeNodeId: context.treeNodeId,
    documentId: context.documentId,
    versionId: context.versionId,
    bodyId: context.bodyId,
    contextVariantKey: context.contextVariantKey,
    instancePath: context.instancePath,
    occurrencePath: context.occurrencePath,
    geometryKey: context.geometryKey,
    instanceId: context.instanceId,
  };
}
