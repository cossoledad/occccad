import type { SketchEntity, SketchGeometryRef, Vec2 } from "../../types";

type ScreenPoint = { x: number; y: number };

const segmentDistance = (point: ScreenPoint, start: ScreenPoint, end: ScreenPoint): number => {
  const dx = end.x - start.x, dy = end.y - start.y, length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1,
    ((point.x - start.x) * dx + (point.y - start.y) * dy) / length));
  return Math.hypot(point.x - (start.x + t * dx), point.y - (start.y + t * dy));
};

// Intrinsic origin/axes and user geometry participate in one typed picking
// policy. The returned reference is stable model identity, never a render ID.
export function resolveSketchReference(cursor: ScreenPoint, entities: SketchEntity[], project: (point: Vec2) => ScreenPoint,
  kind: "COINCIDENT" | "PARALLEL", thresholdPixels = 12, axisExtent = 110): SketchGeometryRef | null {
  let best: { distance: number; reference: SketchGeometryRef } | undefined;
  const consider = (distance: number, reference: SketchGeometryRef) => {
    if (distance < (best?.distance ?? thresholdPixels)) best = { distance, reference };
  };
  if (kind === "COINCIDENT") {
    const origin = project([0, 0]);
    consider(Math.hypot(cursor.x - origin.x, cursor.y - origin.y), { target: "SKETCH_ORIGIN", subElement: "POINT" });
  }
  for (const entity of entities) {
    if (kind === "COINCIDENT" && entity.kind === "POINT" && entity.point) {
      const point = project([entity.point.x, entity.point.y]);
      consider(Math.hypot(cursor.x - point.x, cursor.y - point.y),
        { target: "ENTITY", entityId: entity.id, subElement: "POINT" });
      continue;
    }
    if (entity.kind !== "LINE" || !entity.start || !entity.end) continue;
    const start = project([entity.start.x, entity.start.y]), end = project([entity.end.x, entity.end.y]);
    if (kind === "PARALLEL") {
      consider(segmentDistance(cursor, start, end), { target: "ENTITY", entityId: entity.id, subElement: "DIRECTION" });
    } else {
      consider(Math.hypot(cursor.x - start.x, cursor.y - start.y), { target: "ENTITY", entityId: entity.id, subElement: "START" });
      consider(Math.hypot(cursor.x - end.x, cursor.y - end.y), { target: "ENTITY", entityId: entity.id, subElement: "END" });
    }
  }
  // User lines win exact ties, while an exposed portion of either intrinsic
  // axis stays selectable for Parallel constraints.
  if (kind === "PARALLEL") {
    consider(segmentDistance(cursor, project([-axisExtent, 0]), project([axisExtent, 0])),
      { target: "SKETCH_X_AXIS", subElement: "DIRECTION" });
    consider(segmentDistance(cursor, project([0, -axisExtent]), project([0, axisExtent])),
      { target: "SKETCH_Y_AXIS", subElement: "DIRECTION" });
  }
  return best?.reference ?? null;
}
