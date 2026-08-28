import type { SketchEntity, SketchGeometryRef, Vec2 } from "../../types";
import { sampleSketchEntity, sketchEntityPoint } from "../sketch/sketch-geometry";

type ScreenPoint = { x: number; y: number };
export type SketchReferencePickKind = "POINT" | "EDIT_POINT" | "LINE" | "LINEAR_DIMENSION" | "CURVE" | "CIRCULAR" | "SOLVER_CURVE" | "TANGENT_CURVE" | "EQUAL_CURVE" | "SYMMETRY_CENTER" | "ENTITY";

const segmentDistance = (point: ScreenPoint, start: ScreenPoint, end: ScreenPoint): number => {
  const dx = end.x - start.x, dy = end.y - start.y, length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1,
    ((point.x - start.x) * dx + (point.y - start.y) * dy) / length));
  return Math.hypot(point.x - (start.x + t * dx), point.y - (start.y + t * dy));
};

// Intrinsic origin/axes and user geometry participate in one typed picking
// policy. The returned reference is stable model identity, never a render ID.
export function resolveSketchReference(cursor: ScreenPoint, entities: SketchEntity[], project: (point: Vec2) => ScreenPoint,
  kind: SketchReferencePickKind, thresholdPixels = 12, axisExtent = 110, retained?: SketchGeometryRef): SketchGeometryRef | null {
  const mode = kind;
  let best: { distance: number; priority: number; reference: SketchGeometryRef } | undefined;
  const consider = (distance: number, reference: SketchGeometryRef, priority = 0) => {
    if (distance >= thresholdPixels) return;
    if (!best || distance < best.distance - 1.5 || (Math.abs(distance-best.distance)<=1.5&&priority>best.priority))
      best = { distance, priority, reference };
  };
  if (mode === "POINT" || mode === "LINEAR_DIMENSION" || mode === "SYMMETRY_CENTER") {
    const origin = project([0, 0]);
    consider(Math.hypot(cursor.x - origin.x, cursor.y - origin.y), { target: "SKETCH_ORIGIN", subElement: "POINT" }, 100);
  }
  for (const entity of entities) {
    if(mode==="EDIT_POINT"){
      if((entity.kind==="CIRCLE"||entity.kind==="ARC")&&entity.center){const projected=project([entity.center.x,entity.center.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),{target:"ENTITY",entityId:entity.id,subElement:"CENTER"},80);}
      if(entity.kind==="SPLINE")for(const [controlPointIndex,point] of (entity.controlPoints??[]).entries()){const projected=project([point.x,point.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),{target:"ENTITY",entityId:entity.id,subElement:"CONTROL",controlPointIndex},80);}
      continue;
    }
    if (mode === "ENTITY" && entity.kind === "POINT" && entity.point) {
      const projected=project([entity.point.x,entity.point.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),
        {target:"ENTITY",entityId:entity.id,subElement:"WHOLE"});continue;
    }
    if (mode === "POINT" || mode === "LINEAR_DIMENSION" || mode === "SYMMETRY_CENTER") {
      const candidates = entity.kind === "POINT" ? ["POINT"] as const
        : entity.kind === "CIRCLE" ? ["CENTER"] as const
          : entity.kind === "ARC" ? ["START", "END", "CENTER"] as const : ["START", "END"] as const;
      for (const subElement of candidates) {
        const value = sketchEntityPoint(entity, subElement);
        if (value) { const projected = project(value); consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),
          { target: "ENTITY", entityId: entity.id, subElement }); }
      }
      if (mode === "POINT") continue;
    }
    if (mode === "LINEAR_DIMENSION" && entity.kind !== "LINE") continue;
    if (mode === "LINE" && entity.kind !== "LINE") continue;
    if (mode === "SYMMETRY_CENTER" && entity.kind !== "LINE") continue;
    if (mode === "CIRCULAR" && entity.kind !== "CIRCLE" && entity.kind !== "ARC") continue;
    if ((mode === "SOLVER_CURVE" || mode === "TANGENT_CURVE") && !["LINE", "CIRCLE", "ARC"].includes(entity.kind)) continue;
    if (mode === "TANGENT_CURVE") {
      const retainedKind = retained?.entityId ? entities.find((candidate) => candidate.id === retained.entityId)?.kind : undefined;
      if (retainedKind === "LINE" && entity.kind === "LINE") continue;
    }
    if (mode === "EQUAL_CURVE") {
      if (!["LINE", "CIRCLE", "ARC"].includes(entity.kind)) continue;
      const retainedKind = retained?.entityId ? entities.find((candidate) => candidate.id === retained.entityId)?.kind : undefined;
      if (retainedKind === "LINE" && entity.kind !== "LINE") continue;
      if ((retainedKind === "CIRCLE" || retainedKind === "ARC") && entity.kind !== "CIRCLE" && entity.kind !== "ARC") continue;
    }
    const sampled = sampleSketchEntity(entity).map(project);
    for (let index=1; index<sampled.length; index+=1) {
      consider(segmentDistance(cursor,sampled[index-1],sampled[index]), { target:"ENTITY",entityId:entity.id,
        subElement: entity.kind === "LINE" && (mode === "LINE" || mode === "SYMMETRY_CENTER") ? "DIRECTION" : "WHOLE" });
    }
  }
  // User lines win exact ties, while an exposed portion of either intrinsic
  // axis stays selectable for Parallel constraints.
  const retainedKind = retained?.entityId ? entities.find((candidate) => candidate.id === retained.entityId)?.kind : undefined;
  if (["LINE", "LINEAR_DIMENSION", "SOLVER_CURVE", "TANGENT_CURVE", "SYMMETRY_CENTER"].includes(mode) &&
      !(mode === "TANGENT_CURVE" && (retainedKind === "LINE" || retained?.target === "SKETCH_X_AXIS" || retained?.target === "SKETCH_Y_AXIS"))) {
    consider(segmentDistance(cursor, project([-axisExtent, 0]), project([axisExtent, 0])),
      { target: "SKETCH_X_AXIS", subElement: "DIRECTION" });
    consider(segmentDistance(cursor, project([0, -axisExtent]), project([0, axisExtent])),
      { target: "SKETCH_Y_AXIS", subElement: "DIRECTION" });
  }
  return best?.reference ?? null;
}
