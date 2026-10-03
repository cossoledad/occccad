import type { SketchConstraint, SketchEntity, SketchGeometryRef, Vec2 } from "../../types";
import { ellipsePoint, sampleSketchEntity, sketchEntityPoint, splineReferencePoint } from "./sketch-geometry";
import { constraintDefinition } from "./sketch-constraint-definition";
import { formatSketchDimensionValue } from "./sketch-input-policy";

export type ConstraintSegment = readonly [Vec2, Vec2] & { screenAnchor?: Vec2 };
export type SketchConstraintLayout = {
  symbol: ReturnType<typeof constraintDefinition>["symbol"];
  anchors: Vec2[];
  segments: ConstraintSegment[];
  label?: { text: string; position: Vec2; direction?: Vec2 };
};

const add = (a: Vec2, b: Vec2): Vec2 => [a[0] + b[0], a[1] + b[1]];
const sub = (a: Vec2, b: Vec2): Vec2 => [a[0] - b[0], a[1] - b[1]];
const scale = (a: Vec2, value: number): Vec2 => [a[0] * value, a[1] * value];
const midpoint = (a: Vec2, b: Vec2): Vec2 => scale(add(a, b), 0.5);
const normalize = (value: Vec2): Vec2 => { const length = Math.hypot(value[0], value[1]); return length > 1e-9 ? scale(value, 1 / length) : [1, 0]; };

function entityFor(reference: SketchGeometryRef, entities: ReadonlyMap<string, SketchEntity>): SketchEntity | undefined {
  return reference.entityId ? entities.get(reference.entityId) : undefined;
}

export function sketchReferencePoint(reference: SketchGeometryRef, entities: ReadonlyMap<string, SketchEntity>): Vec2 | undefined {
  if (reference.target === "SKETCH_ORIGIN") return [0, 0];
  const entity = entityFor(reference, entities);
  if (!entity) return undefined;
  if (reference.pointId && reference.pointId !== (reference.subElement === "START" ? entity.startPointId : reference.subElement === "END" ? entity.endPointId : undefined)) return undefined;
  if (["POINT", "START", "END", "CENTER"].includes(reference.subElement)) {
    return sketchEntityPoint(entity, reference.subElement as "POINT" | "START" | "END" | "CENTER");
  }
  if (reference.subElement === "CONTROL") {
    return splineReferencePoint(entity, reference.controlPointIndex, reference.controlPointId);
  }
  if (entity.kind === "LINE" && entity.start && entity.end) {
    return [(entity.start.x + entity.end.x) / 2, (entity.start.y + entity.end.y) / 2];
  }
  const sampled = sampleSketchEntity(entity);
  return sampled.length ? sampled[Math.floor((sampled.length - 1) / 2)] : undefined;
}

function linePoints(reference: SketchGeometryRef, entities: ReadonlyMap<string, SketchEntity>): [Vec2, Vec2] | undefined {
  if (reference.target === "SKETCH_X_AXIS") return [[-110, 0], [110, 0]];
  if (reference.target === "SKETCH_Y_AXIS") return [[0, -110], [0, 110]];
  const entity = entityFor(reference, entities);
  if (entity?.kind !== "LINE" || !entity.start || !entity.end ||
      !["WHOLE", "DIRECTION"].includes(reference.subElement)) return undefined;
  return [[entity.start.x, entity.start.y], [entity.end.x, entity.end.y]];
}

// Supporting-line distance, independent of finite segment overlap or endpoint
// order. Parallelism here is a preview check; the authoritative command owns
// the formal parallel relation and validates its solver result.
function parallelLineDistanceEndpoints(first: [Vec2, Vec2], second: [Vec2, Vec2]): [Vec2, Vec2] | undefined {
  const a = sub(first[1], first[0]), b = sub(second[1], second[0]);
  const aLength = Math.hypot(...a), bLength = Math.hypot(...b);
  if (aLength === 0 || bLength === 0) return undefined;
  if (Math.abs(a[0] * b[1] - a[1] * b[0]) > 1e-10 * aLength * bLength) return undefined;
  const p = midpoint(...first), delta = sub(p, second[0]);
  const t = (delta[0] * b[0] + delta[1] * b[1]) / (bLength * bLength);
  return [p, add(second[0], scale(b, t))];
}

function arrow(tip: Vec2, direction: Vec2, size = 3): ConstraintSegment[] {
  const unit = normalize(direction), normal: Vec2 = [-unit[1], unit[0]];
  const base = add(tip, scale(unit, size));
  return [1, -1].map(sign => Object.assign([tip, add(base, scale(normal, size * 0.45 * sign))] as const, { screenAnchor: tip }));
}

function linearDimension(a: Vec2, b: Vec2, text: string, placement?: Vec2): Pick<SketchConstraintLayout, "segments" | "label"> {
  const direction = normalize(sub(b, a)), normal: Vec2 = [-direction[1], direction[0]];
  const automatic = Math.max(8, Math.min(18, Math.hypot(...sub(b, a)) * 0.25));
  const projected = placement ? sub(placement, midpoint(a, b))[0] * normal[0] + sub(placement, midpoint(a, b))[1] * normal[1] : automatic;
  const offset = placement && Math.abs(projected) < 4 ? (projected < 0 ? -4 : 4) : projected;
  const qa = add(a, scale(normal, offset)), qb = add(b, scale(normal, offset));
  return { segments: [[a, qa], [b, qb], [qa, qb], ...arrow(qa, direction), ...arrow(qb, scale(direction, -1))],
    label: { text, direction, position: placement ?? add(midpoint(qa, qb), scale(normal, offset < 0 ? -2.5 : 2.5)) } };
}

export function sketchDimensionText(constraint: SketchConstraint): string {
  const measured = constraint.value === undefined ? "不可测" : formatSketchDimensionValue(constraint.value, constraint.unit ?? "mm");
  const value = constraint.reference ? `(${measured})` : measured;
  if (constraint.kind === "HORIZONTAL_DISTANCE") return `ΔX ${value}`;
  if (constraint.kind === "VERTICAL_DISTANCE") return `ΔY ${value}`;
  if (constraint.kind === "RADIUS") return `R ${value}`;
  if (constraint.kind === "MAJOR_RADIUS") return `a ${value}`;
  if (constraint.kind === "MINOR_RADIUS") return `b ${value}`;
  if (constraint.kind === "DIAMETER") return `Ø ${value}`;
  if (constraint.kind === "ANGLE") return `${value}°`;
  return value;
}

function circularData(reference: SketchGeometryRef, entities: ReadonlyMap<string, SketchEntity>) {
  const entity = entityFor(reference, entities);
  if (!entity || (entity.kind !== "CIRCLE" && entity.kind !== "ARC") || !entity.center || !entity.radius) return undefined;
  const angle = entity.kind === "ARC" ? ((entity.startAngle ?? 0) + (entity.endAngle ?? 0)) / 2 : Math.PI / 4;
  const center: Vec2 = [entity.center.x, entity.center.y];
  return { center, radius: entity.radius, direction: [Math.cos(angle), Math.sin(angle)] as Vec2 };
}

function lineIntersection(first: [Vec2, Vec2], second: [Vec2, Vec2]): Vec2 {
  const a = sub(first[1], first[0]), b = sub(second[1], second[0]), denominator = a[0] * b[1] - a[1] * b[0];
  if (Math.abs(denominator) < 1e-9) return midpoint(midpoint(...first), midpoint(...second));
  const delta = sub(second[0], first[0]);
  return add(first[0], scale(a, (delta[0] * b[1] - delta[1] * b[0]) / denominator));
}

export function measureSketchDimension(kind: SketchConstraint["kind"], references: readonly SketchGeometryRef[],
  sketchEntities: readonly SketchEntity[]): number | undefined {
  const entities = new Map(sketchEntities.map((entity) => [entity.id, entity]));
  if (kind === "HORIZONTAL_DISTANCE" || kind === "VERTICAL_DISTANCE") {
    const a = references[0] ? sketchReferencePoint(references[0], entities) : undefined;
    const b = references[1] ? sketchReferencePoint(references[1], entities) : undefined;
    if (a && b) return kind === "HORIZONTAL_DISTANCE" ? b[0] - a[0] : b[1] - a[1];
  }
  if (kind === "DISTANCE") {
    const lines=references.map((reference)=>linePoints(reference,entities));
    const points = references.map((reference,index) => lines[index]?undefined:sketchReferencePoint(reference, entities));
    if (lines[0] && lines[1]) {
      const endpoints = parallelLineDistanceEndpoints(lines[0], lines[1]);
      return endpoints ? Math.hypot(...sub(endpoints[1], endpoints[0])) : undefined;
    }
    if (!lines[0]&&!lines[1]&&points.length >= 2 && points[0] && points[1]) return Math.hypot(...sub(points[1], points[0]));
    const pointIndex=points[0]?0:points[1]?1:-1,lineIndex=pointIndex===0?1:0;
    const line=lines[lineIndex];
    if(pointIndex>=0&&line){const p=points[pointIndex]!,direction=sub(line[1],line[0]);
      if (Math.hypot(...direction) === 0) return undefined;
      return Math.abs(direction[0]*(line[0][1]-p[1])-(line[0][0]-p[0])*direction[1])/Math.hypot(...direction);}
  }
  if (kind === "LENGTH") {
    const line = references[0] ? linePoints(references[0], entities) : undefined;
    if (line) return Math.hypot(...sub(line[1], line[0]));
  }
  if (kind === "RADIUS" || kind === "DIAMETER") {
    const circular = references[0] ? circularData(references[0], entities) : undefined;
    if (circular) return circular.radius * (kind === "DIAMETER" ? 2 : 1);
  }
  if (kind === "MAJOR_RADIUS" || kind === "MINOR_RADIUS") {
    const entity = references[0] ? entityFor(references[0], entities) : undefined;
    if (entity && (entity.kind === "ELLIPSE" || entity.kind === "ELLIPTICAL_ARC"))
      return kind === "MAJOR_RADIUS" ? entity.majorRadius : entity.minorRadius;
  }
  if (kind === "ANGLE") {
    const first = references[0] ? linePoints(references[0], entities) : undefined;
    const second = references[1] ? linePoints(references[1], entities) : undefined;
    if (first && second) {
      const a = normalize(sub(first[1], first[0])), b = normalize(sub(second[1], second[0]));
      return Math.acos(Math.min(1, Math.max(-1, a[0] * b[0] + a[1] * b[1]))) * 180 / Math.PI;
    }
  }
  return undefined;
}

export function buildSketchConstraintLayout(constraint: SketchConstraint, sketchEntities: readonly SketchEntity[]): SketchConstraintLayout {
  const definition = constraintDefinition(constraint.kind);
  const entities = new Map(sketchEntities.map((entity) => [entity.id, entity]));
  const points = constraint.references.map((reference) => sketchReferencePoint(reference, entities)).filter((point): point is Vec2 => Boolean(point));
  const anchors: Vec2[] = [];
  const segments: ConstraintSegment[] = [];
  let label: SketchConstraintLayout["label"];
  if (constraint.kind === "HORIZONTAL_DISTANCE" || constraint.kind === "VERTICAL_DISTANCE") {
    const a = sketchReferencePoint(constraint.references[0], entities), b = sketchReferencePoint(constraint.references[1], entities);
    if (a && b) {
      const projected: Vec2 = constraint.kind === "HORIZONTAL_DISTANCE" ? [b[0], a[1]] : [a[0], b[1]];
      const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
      const dimension = linearDimension(a, projected, sketchDimensionText(constraint), placement);
      segments.push(...dimension.segments); segments[1] = [b, segments[1][1]]; label = dimension.label;
    }
  }
  if (constraint.kind === "DISTANCE") {
    const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
    const lines=constraint.references.map((reference)=>linePoints(reference,entities));
    const pointReferences=constraint.references.map((reference,index)=>lines[index]?undefined:sketchReferencePoint(reference,entities));
    let endpoints:readonly [Vec2,Vec2]|undefined;
    if(lines[0]&&lines[1])endpoints=parallelLineDistanceEndpoints(lines[0],lines[1]);
    else if(pointReferences[0]&&pointReferences[1])endpoints=[pointReferences[0],pointReferences[1]];
    else {
      const pointIndex=pointReferences[0]?0:pointReferences[1]?1:-1,lineIndex=pointIndex===0?1:0;
      const line=constraint.references[lineIndex]?linePoints(constraint.references[lineIndex],entities):undefined;
      if(pointIndex>=0&&line){const p=pointReferences[pointIndex]!,direction=sub(line[1],line[0]);const length2=direction[0]**2+direction[1]**2;
        if (length2 === 0) return { symbol: definition.symbol, anchors, segments };
        const t=((p[0]-line[0][0])*direction[0]+(p[1]-line[0][1])*direction[1])/length2;
        endpoints=[p,add(line[0],scale(direction,t))];}
    }
    if(endpoints){const dimension = linearDimension(endpoints[0], endpoints[1], sketchDimensionText(constraint), placement);
      segments.push(...dimension.segments); label = dimension.label;}
  }
  if (constraint.kind === "LENGTH") {
    const line = linePoints(constraint.references[0], entities);
    if (line) { const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
      const dimension = linearDimension(line[0], line[1], sketchDimensionText(constraint), placement); segments.push(...dimension.segments); label = dimension.label; }
  }
  if (constraint.kind === "RADIUS" || constraint.kind === "DIAMETER") {
    const circular = circularData(constraint.references[0], entities);
    if (circular) {
      const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
      const direction = placement ? normalize(sub(placement, circular.center)) : circular.direction;
      const first = add(circular.center, scale(direction, circular.radius));
      const opposite = constraint.kind === "DIAMETER" ? add(circular.center, scale(direction, -circular.radius)) : circular.center;
      segments.push([opposite, first], ...arrow(first, sub(opposite, first)));
      if (constraint.kind === "DIAMETER") segments.push(...arrow(opposite, sub(first, opposite)));
      if (placement) segments.push([first, placement]);
      label = { text: sketchDimensionText(constraint), direction: placement ? normalize(sub(placement, first)) : direction, position: placement ?? add(midpoint(first, opposite), scale([-direction[1], direction[0]], 3)) };
    }
  }
  if (constraint.kind === "MAJOR_RADIUS" || constraint.kind === "MINOR_RADIUS") {
    const entity = entityFor(constraint.references[0], entities);
    if (entity && ["ELLIPSE", "ELLIPTICAL_ARC"].includes(entity.kind) && entity.center) {
      const end = ellipsePoint(entity, constraint.kind === "MAJOR_RADIUS" ? 0 : Math.PI / 2);
      const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
      if (end) { const dimension = linearDimension([entity.center.x, entity.center.y], end, sketchDimensionText(constraint), placement);
        segments.push(...dimension.segments); label = dimension.label; }
    }
  }
  if (constraint.kind === "ANGLE") {
    const first = linePoints(constraint.references[0], entities), second = linePoints(constraint.references[1], entities);
    if (first && second) {
      const origin = lineIntersection(first, second);
      const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
      let a = normalize(sub(first[1], first[0])), b = normalize(sub(second[1], second[0]));
      if (placement) {
        const towardLabel=normalize(sub(placement,origin));
        if(a[0]*towardLabel[0]+a[1]*towardLabel[1]<0)a=scale(a,-1);
        if(b[0]*towardLabel[0]+b[1]*towardLabel[1]<0)b=scale(b,-1);
      }
      let firstAngle = Math.atan2(a[1], a[0]), secondAngle = Math.atan2(b[1], b[0]);
      while (secondAngle < firstAngle) secondAngle += Math.PI * 2;
      if (secondAngle - firstAngle > Math.PI) [firstAngle, secondAngle] = [secondAngle, firstAngle + Math.PI * 2];
      const radius = placement ? Math.max(6, Math.hypot(...sub(placement, origin)) - 4) : 14;
      const arc = Array.from({ length: 13 }, (_, index) => firstAngle + (secondAngle - firstAngle) * index / 12)
        .map((angle) => add(origin, [Math.cos(angle) * radius, Math.sin(angle) * radius]));
      segments.push([origin, arc[0]], [origin, arc.at(-1)!]);
      for (let index = 1; index < arc.length; index++) segments.push([arc[index - 1], arc[index]]);
      const labelPosition = placement ?? add(origin, [Math.cos((firstAngle + secondAngle) / 2) * (radius + 4), Math.sin((firstAngle + secondAngle) / 2) * (radius + 4)]);
      const radial = normalize(sub(labelPosition, origin));
      label = { text: sketchDimensionText(constraint), position: labelPosition, direction: [-radial[1], radial[0]] };
    }
  }
  if (definition.dimension === "none") {
    if (constraint.kind === "MIRROR") {
      for (const index of [0, 2]) {
        const reference = constraint.references[index], entity = reference ? entityFor(reference, entities) : undefined;
        const point = entity?.center ? [entity.center.x, entity.center.y] as Vec2 : reference ? sketchReferencePoint(reference, entities) : undefined;
        if (point) anchors.push(add(point, [4, 4]));
      }
    }
    else if (constraint.kind === "FIXED_POINT" && constraint.fixedPoint) anchors.push([constraint.fixedPoint.x, constraint.fixedPoint.y]);
    else if (constraint.kind === "PARALLEL" || constraint.kind === "COLLINEAR" || constraint.kind === "EQUAL") anchors.push(...points.map((point) => add(point, [4, 4])));
    else if (constraint.kind === "CONCENTRIC") {
      const entity = entityFor(constraint.references[0], entities);
      if (entity?.center) anchors.push([entity.center.x, entity.center.y]);
    } else if ((constraint.kind === "TANGENT" || constraint.kind === "PERPENDICULAR") && points.length >= 2) anchors.push(add(midpoint(points[0], points[1]), [4, 4]));
    else if (points[0]) anchors.push(add(points[0], constraint.kind === "COINCIDENT" ? [0, 0] : [4, 4]));
    else anchors.push([0, 0]);
  }
  return { symbol: definition.symbol, anchors, segments, label };
}
