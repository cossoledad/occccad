import type { SketchEntity, SketchGeometryRef, Vec2 } from "../../types";
import { sampleSketchEntity, sketchEntityPoint, splineEditablePoints, splineEditablePointIDs } from "../sketch/sketch-geometry";

type ScreenPoint = { x: number; y: number };
export type SketchReferencePickKind = "POINT" | "EDIT_POINT" | "LINE" | "LINEAR_DIMENSION" | "CURVE" | "CIRCULAR" | "ELLIPTICAL" | "CENTER_CURVE" | "SOLVER_CURVE" | "TANGENT_CURVE" | "EQUAL_CURVE" | "SYMMETRY_CENTER" | "ENTITY";

const segmentDistance = (point: ScreenPoint, start: ScreenPoint, end: ScreenPoint): number => {
  const dx = end.x - start.x, dy = end.y - start.y, length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1,
    ((point.x - start.x) * dx + (point.y - start.y) * dy) / length));
  return Math.hypot(point.x - (start.x + t * dx), point.y - (start.y + t * dy));
};

// Intrinsic origin/axes and user geometry participate in one typed picking
// policy. The returned reference is stable model identity, never a render ID.
export function resolveSketchReference(cursor: ScreenPoint, entities: SketchEntity[], project: (point: Vec2) => ScreenPoint,
  kind: SketchReferencePickKind, thresholdPixels = 12, axisExtent: number | Vec2 = 110, retained?: SketchGeometryRef, allowed?: (reference:SketchGeometryRef)=>boolean): SketchGeometryRef | null {
  const mode = kind;
  const [extentX, extentY] = typeof axisExtent === "number" ? [axisExtent, axisExtent] : axisExtent;
  let best: { distance: number; priority: number; reference: SketchGeometryRef } | undefined;
  const consider = (distance: number, reference: SketchGeometryRef, priority = 0) => {
    if (distance >= thresholdPixels || allowed&&!allowed(reference)) return;
    if (!best || distance < best.distance - 1.5 || (Math.abs(distance-best.distance)<=1.5&&priority>best.priority))
      best = { distance, priority, reference };
  };
  if (mode === "POINT" || mode === "LINEAR_DIMENSION" || mode === "SYMMETRY_CENTER") {
    const origin = project([0, 0]);
    consider(Math.hypot(cursor.x - origin.x, cursor.y - origin.y), { target: "SKETCH_ORIGIN", subElement: "POINT" }, 100);
  }
  for (const entity of entities) {
    if(entity.suppressed)continue;
    // Visible profile segments beat their retained construction supports at
    // the same screen distance; standalone construction geometry stays pickable.
    const dimensionPriority=mode==="LINEAR_DIMENSION"&&entity.role!=="CONSTRUCTION"?1:0;
    if(mode==="ENTITY"&&entity.kind==="POINT"&&entity.point){const value=project([entity.point.x,entity.point.y]);consider(Math.hypot(cursor.x-value.x,cursor.y-value.y),{target:"ENTITY",entityId:entity.id,subElement:"POINT"});continue;}
    if(mode==="EDIT_POINT"){
      const editableElements=entity.kind==="POINT"?["POINT"] as const:["LINE","ARC","ELLIPTICAL_ARC"].includes(entity.kind)?["START","END"] as const:[];
      for(const subElement of editableElements){
        const point=sketchEntityPoint(entity,subElement);if(!point)continue;const projected=project(point);
        const pointId=subElement==="START"?entity.startPointId:subElement==="END"?entity.endPointId:undefined;
        consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),{target:"ENTITY",entityId:entity.id,subElement,...(pointId?{pointId}:{})},90);
      }
      if(["CIRCLE","ARC","ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind)&&entity.center){const projected=project([entity.center.x,entity.center.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),{target:"ENTITY",entityId:entity.id,subElement:"CENTER"},80);}
      if(entity.kind==="SPLINE")for(const [controlPointIndex,point] of splineEditablePoints(entity).entries()){const projected=project([point.x,point.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),{target:"ENTITY",entityId:entity.id,subElement:"CONTROL",controlPointIndex,...(splineEditablePointIDs(entity)[controlPointIndex]?{controlPointId:splineEditablePointIDs(entity)[controlPointIndex]}:{})},80);}
      continue;
    }
    if (mode === "ENTITY" && entity.kind === "POINT" && entity.point) {
      const projected=project([entity.point.x,entity.point.y]);consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),
        {target:"ENTITY",entityId:entity.id,subElement:"WHOLE"});continue;
    }
    let tangentEndpointCaptured=false;
    if (mode==="TANGENT_CURVE") {
      const retainedKind=retained?.entityId?entities.find(candidate=>candidate.id===retained.entityId)?.kind:undefined;
      const retainedLine=retainedKind==="LINE"||retained?.target==="SKETCH_X_AXIS"||retained?.target==="SKETCH_Y_AXIS";
      if (retainedKind==="SPLINE"&&entity.kind!=="LINE")continue;
      const endpointAllowed=(entity.kind==="SPLINE"&&entity.mode==="CONTROL"&&!entity.closed&&(!retained||retainedLine))||((entity.kind==="ARC"||entity.kind==="ELLIPTICAL_ARC")&&(!retained||retainedLine||(entity.kind==="ARC"&&(retainedKind==="ARC"||retainedKind==="CIRCLE"))));
      if(endpointAllowed)for(const subElement of ["START","END"] as const) {
        const point=sketchEntityPoint(entity,subElement);if(!point)continue;const projected=project(point);
        const endpointDistance=Math.hypot(cursor.x-projected.x,cursor.y-projected.y);
        if(endpointDistance<thresholdPixels)tangentEndpointCaptured=true;
        consider(endpointDistance,{target:"ENTITY",entityId:entity.id,subElement,...((subElement==="START"?entity.startPointId:entity.endPointId)?{pointId:subElement==="START"?entity.startPointId:entity.endPointId}:{})},90);
      }
      if(entity.kind==="SPLINE")continue;
    }
    if (mode === "POINT" || mode === "LINEAR_DIMENSION" || mode === "SYMMETRY_CENTER") {
      if (entity.kind === "SPLINE") for (const [controlPointIndex, point] of splineEditablePoints(entity).entries()) {
        const projected = project([point.x, point.y]);
        consider(Math.hypot(cursor.x - projected.x, cursor.y - projected.y),
          { target: "ENTITY", entityId: entity.id, subElement: "CONTROL", controlPointIndex, ...(splineEditablePointIDs(entity)[controlPointIndex] ? { controlPointId: splineEditablePointIDs(entity)[controlPointIndex] } : {}) }, 85);
      }
      const candidates = entity.kind === "POINT" ? ["POINT"] as const
        : (entity.kind === "CIRCLE" || entity.kind === "ELLIPSE") ? ["CENTER"] as const
          : (entity.kind === "ARC" || entity.kind === "ELLIPTICAL_ARC") ? ["START", "END", "CENTER"] as const : ["START", "END"] as const;
      for (const subElement of candidates) {
        const value = sketchEntityPoint(entity, subElement);
        if (value) { const projected = project(value); consider(Math.hypot(cursor.x-projected.x,cursor.y-projected.y),
          { target: "ENTITY", entityId: entity.id, subElement, ...((subElement === "START" ? entity.startPointId : subElement === "END" ? entity.endPointId : undefined) ? {pointId:subElement === "START" ? entity.startPointId : entity.endPointId} : {}) },dimensionPriority); }
      }
      if (mode === "POINT") continue;
    }
    if (mode === "LINEAR_DIMENSION" && entity.kind !== "LINE") continue;
    if (mode === "LINE" && entity.kind !== "LINE") continue;
    if (mode === "SYMMETRY_CENTER" && entity.kind !== "LINE") continue;
    if (mode === "CIRCULAR" && entity.kind !== "CIRCLE" && entity.kind !== "ARC") continue;
    if (mode === "ELLIPTICAL" && !["ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind)) continue;
    if (mode === "CENTER_CURVE" && !["CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind)) continue;
    if ((mode === "SOLVER_CURVE" || mode === "TANGENT_CURVE") && !["LINE", "CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind)) continue;
    if (mode === "TANGENT_CURVE") {
      const retainedKind = retained?.entityId ? entities.find((candidate) => candidate.id === retained.entityId)?.kind : undefined;
      if (retainedKind === "LINE" && entity.kind === "LINE") continue;
      if (retainedKind && retainedKind !== "LINE" && entity.kind !== "LINE" &&
          (["ELLIPSE", "ELLIPTICAL_ARC"].includes(retainedKind) || ["ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind))) continue;
    }
    if (mode === "EQUAL_CURVE") {
      if (!["LINE", "CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind)) continue;
      const retainedKind = retained?.entityId ? entities.find((candidate) => candidate.id === retained.entityId)?.kind : undefined;
      if (retainedKind === "LINE" && entity.kind !== "LINE") continue;
      if ((retainedKind === "CIRCLE" || retainedKind === "ARC") && entity.kind !== "CIRCLE" && entity.kind !== "ARC") continue;
      if ((retainedKind === "ELLIPSE" || retainedKind === "ELLIPTICAL_ARC") && entity.kind !== "ELLIPSE" && entity.kind !== "ELLIPTICAL_ARC") continue;
    }
    // A curve hit next to a captured derivative endpoint must retain that
    // explicit endpoint meaning; its own distance-zero WHOLE hit is not a
    // competing tangent mode. Other entities still compete normally.
    if(tangentEndpointCaptured)continue;
    const sampled = sampleSketchEntity(entity).map(project);
    for (let index=1; index<sampled.length; index+=1) {
      consider(segmentDistance(cursor,sampled[index-1],sampled[index]), { target:"ENTITY",entityId:entity.id,
        subElement: entity.kind === "LINE" && (mode === "LINE" || mode === "SYMMETRY_CENTER") ? "DIRECTION" : "WHOLE" },dimensionPriority);
    }
  }
  // User lines win exact ties, while an exposed portion of either intrinsic
  // axis stays selectable for Parallel constraints.
  const retainedKind = retained?.entityId ? entities.find((candidate) => candidate.id === retained.entityId)?.kind : undefined;
  if (["LINE", "LINEAR_DIMENSION", "SOLVER_CURVE", "TANGENT_CURVE", "SYMMETRY_CENTER"].includes(mode) &&
      !(mode === "TANGENT_CURVE" && (retainedKind === "LINE" || retained?.target === "SKETCH_X_AXIS" || retained?.target === "SKETCH_Y_AXIS"))) {
    if (extentX > 0) consider(segmentDistance(cursor, project([-extentX, 0]), project([extentX, 0])),
      { target: "SKETCH_X_AXIS", subElement: "DIRECTION" });
    if (extentY > 0) consider(segmentDistance(cursor, project([0, -extentY]), project([0, extentY])),
      { target: "SKETCH_Y_AXIS", subElement: "DIRECTION" });
  }
  return best?.reference ?? null;
}

// Whole-object preselection has no cursor intent: never invent a line endpoint
// or spline control point from an object selection.
export function preselectedSketchReference(entity:SketchEntity,pick:SketchReferencePickKind,retained?:SketchEntity):SketchGeometryRef|undefined {
 if(entity.suppressed)return undefined;
 const ref=(subElement:SketchGeometryRef["subElement"]):SketchGeometryRef=>({target:"ENTITY",entityId:entity.id,subElement});
 if(pick==="POINT"||pick==="EDIT_POINT")return entity.kind==="POINT"?ref("POINT"):undefined;
 if(pick==="LINEAR_DIMENSION")return entity.kind==="POINT"?ref("POINT"):entity.kind==="LINE"?ref("WHOLE"):undefined;
 if(pick==="LINE"||pick==="SYMMETRY_CENTER")return entity.kind==="LINE"?ref("DIRECTION"):undefined;
 if(pick==="CIRCULAR")return ["CIRCLE","ARC"].includes(entity.kind)?ref("WHOLE"):undefined;
 if(pick==="ELLIPTICAL")return ["ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind)?ref("WHOLE"):undefined;
 if(pick==="CENTER_CURVE")return ["CIRCLE","ARC","ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind)?ref("WHOLE"):undefined;
 if(pick==="ENTITY")return ref("WHOLE");
 if(pick==="CURVE")return entity.kind!=="POINT"?ref("WHOLE"):undefined;
 if(!["LINE","CIRCLE","ARC","ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind))return undefined;
 if(pick==="TANGENT_CURVE"&&retained) {
  if(retained.kind==="SPLINE"&&entity.kind!=="LINE")return undefined;
  if(retained.kind==="LINE"&&entity.kind==="LINE")return undefined;
  if(retained.kind!=="LINE"&&entity.kind!=="LINE"&&[retained.kind,entity.kind].some(k=>["ELLIPSE","ELLIPTICAL_ARC"].includes(k)))return undefined;
 }
 if(pick==="EQUAL_CURVE"&&retained) {
  const group=(kind:string)=>kind==="LINE"?"line":["CIRCLE","ARC"].includes(kind)?"circle":["ELLIPSE","ELLIPTICAL_ARC"].includes(kind)?"ellipse":"unsupported";
  if(group(retained.kind)!==group(entity.kind))return undefined;
 }
 return ref("WHOLE");
}
