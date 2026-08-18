import type { Selection, VisualPrimitive } from "../../types";

export type VisualOccurrenceContext = {
  documentId: string;
  geometryKey: string;
  occurrencePath: string;
  treeNodeId?: string;
  instanceId?: string;
};

export function visualType(primitive: VisualPrimitive): "POINT" | "CURVE" | "SURFACE" {
  if (primitive.kind === "POINTS") return "POINT";
  if (primitive.kind === "POLYLINE") return "CURVE";
  return "SURFACE";
}

export function visualSelection(
  primitive: VisualPrimitive,
  context: VisualOccurrenceContext,
): Exclude<Selection, null> {
  return {
    kind: "visual",
    id: `${context.occurrencePath || "root"}:${primitive.featureId}:${primitive.id}`,
    visualType: visualType(primitive),
    featureId: primitive.featureId,
    entityId: primitive.id,
    role: primitive.role,
    treeNodeId: context.treeNodeId,
    documentId: context.documentId,
    occurrencePath: context.occurrencePath,
    geometryKey: context.geometryKey,
    instanceId: context.instanceId,
  };
}
