import type { SketchEntity, SketchGeometryRef, Vec2 } from "../../types";
import { sampleSketchEntity, sketchEntityPoint } from "../sketch/sketch-geometry";

type ScreenPoint = { x: number; y: number };
export type SketchReferencePickKind = "POINT" | "LINE" | "CURVE" | "CIRCULAR" | "ENTITY";

const segmentDistance = (point: ScreenPoint, start: ScreenPoint, end: ScreenPoint): number => {
  const dx = end.x - start.x, dy = end.y - start.y, length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1,
    ((point.x - start.x) * dx + (point.y - start.y) * dy) / length));
  return Math.hypot(point.x - (start.x + t * dx), point.y - (start.y + t * dy));
};

// Intrinsic origin/axes and user geometry participate in one typed picking
// policy. The returned reference is stable model identity, never a render ID.
export function resolveSketchReference(cursor: ScreenPoint, entities: SketchEntity[], project: (point: Vec2) => ScreenPoint,
  kind: SketchReferencePickKind, thresholdPixels = 12, axisExtent = 110): SketchGeometryRef | null {
  const mode:SketchReferencePickKind = (kind as string)==="COINCIDENT"?"POINT":(kind as string)==="PARALLEL"?"LINE":kind;
  let best: { distance: number; reference: SketchGeometryRef } | undefined;
  const consider = (distance: number, reference: SketchGeometryRef) => {
    if (distance < (best?.distance ?? thresholdPixels)) best = { distance, reference };
  };
  if (mode === "POINT") {
    const origin = project([0, 0]);
    consider(Math.hypot(cursor.x - origin.x, cursor.y - origin.y), { target: "SKETCH_ORIGIN", subElement: "POINT" });
  }
  for (const entity of entities) {
    if (mode === "ENTITY" && entity.kind === "POINT" && entity.point) {
      const projected=project([entity.point.x,entity.point.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),
        {target:"ENTITY",entityId:entity.id,subElement:"WHOLE"});continue;
    }
    if (mode === "POINT") {
      const candidates = entity.kind === "POINT" ? ["POINT"] as const
        : entity.kind === "CIRCLE" ? ["CENTER"] as const
          : entity.kind === "ARC" ? ["START", "END", "CENTER"] as const : ["START", "END"] as const;
      for (const subElement of candidates) {
        const value = sketchEntityPoint(entity, subElement);
        if (value) { const projected = project(value); consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),
          { target: "ENTITY", entityId: entity.id, subElement }); }
      }
      continue;
    }
    if (mode === "LINE" && entity.kind !== "LINE") continue;
    if (mode === "CIRCULAR" && entity.kind !== "CIRCLE" && entity.kind !== "ARC") continue;
    const sampled = sampleSketchEntity(entity).map(project);
    for (let index=1; index<sampled.length; index+=1) {
      consider(segmentDistance(cursor,sampled[index-1],sampled[index]), { target:"ENTITY",entityId:entity.id,
        subElement: entity.kind === "LINE" && mode === "LINE" ? "DIRECTION" : "WHOLE" });
    }
  }
  // User lines win exact ties, while an exposed portion of either intrinsic
  // axis stays selectable for Parallel constraints.
  if (mode === "LINE") {
    consider(segmentDistance(cursor, project([-axisExtent, 0]), project([axisExtent, 0])),
      { target: "SKETCH_X_AXIS", subElement: "DIRECTION" });
    consider(segmentDistance(cursor, project([0, -axisExtent]), project([0, axisExtent])),
      { target: "SKETCH_Y_AXIS", subElement: "DIRECTION" });
  }
  return best?.reference ?? null;
}
