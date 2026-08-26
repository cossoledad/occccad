import type { SketchConstraint, SketchEntity, SketchGeometryRef, Vec2 } from "../../types";
import { sampleSketchEntity, sketchEntityPoint } from "./sketch-geometry";
import { constraintDefinition } from "./sketch-constraint-definition";

export type ConstraintSegment = readonly [Vec2, Vec2];
export type SketchConstraintLayout = {
  symbol: ReturnType<typeof constraintDefinition>["symbol"];
  anchors: Vec2[];
  segments: ConstraintSegment[];
  label?: { text: string; position: Vec2 };
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
  if (["POINT", "START", "END", "CENTER"].includes(reference.subElement)) {
    return sketchEntityPoint(entity, reference.subElement as "POINT" | "START" | "END" | "CENTER");
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
  if (entity?.kind !== "LINE" || !entity.start || !entity.end) return undefined;
  return [[entity.start.x, entity.start.y], [entity.end.x, entity.end.y]];
}

function arrow(tip: Vec2, direction: Vec2, size = 3): ConstraintSegment[] {
  const unit = normalize(direction), normal: Vec2 = [-unit[1], unit[0]];
  const base = add(tip, scale(unit, size));
  return [[tip, add(base, scale(normal, size * 0.45))], [tip, add(base, scale(normal, -size * 0.45))]];
}

function linearDimension(a: Vec2, b: Vec2, text: string, placement?: Vec2): Pick<SketchConstraintLayout, "segments" | "label"> {
  const direction = normalize(sub(b, a)), normal: Vec2 = [-direction[1], direction[0]];
  const automatic = Math.max(8, Math.min(18, Math.hypot(...sub(b, a)) * 0.25));
  const projected = placement ? sub(placement, midpoint(a, b))[0] * normal[0] + sub(placement, midpoint(a, b))[1] * normal[1] : automatic;
  const offset = placement && Math.abs(projected) < 4 ? (projected < 0 ? -4 : 4) : projected;
  const qa = add(a, scale(normal, offset)), qb = add(b, scale(normal, offset));
  return { segments: [[a, qa], [b, qb], [qa, qb], ...arrow(qa, direction), ...arrow(qb, scale(direction, -1))],
    label: { text, position: placement ?? add(midpoint(qa, qb), scale(normal, offset < 0 ? -2.5 : 2.5)) } };
}

function constraintText(constraint: SketchConstraint): string {
  const value = constraint.value === undefined ? "?" : Number(constraint.value.toFixed(3)).toString();
  if (constraint.kind === "RADIUS") return `R ${value}`;
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

export function buildSketchConstraintLayout(constraint: SketchConstraint, sketchEntities: readonly SketchEntity[]): SketchConstraintLayout {
  const definition = constraintDefinition(constraint.kind);
  const entities = new Map(sketchEntities.map((entity) => [entity.id, entity]));
  const points = constraint.references.map((reference) => sketchReferencePoint(reference, entities)).filter((point): point is Vec2 => Boolean(point));
  const anchors: Vec2[] = [];
  const segments: ConstraintSegment[] = [];
  let label: SketchConstraintLayout["label"];
  if (constraint.kind === "DISTANCE" && points.length >= 2) {
    const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
    const dimension = linearDimension(points[0], points[1], constraintText(constraint), placement);
    segments.push(...dimension.segments); label = dimension.label;
  }
  if (constraint.kind === "LENGTH") {
    const line = linePoints(constraint.references[0], entities);
    if (line) { const placement = constraint.labelPosition ? [constraint.labelPosition.x, constraint.labelPosition.y] as Vec2 : undefined;
      const dimension = linearDimension(line[0], line[1], constraintText(constraint), placement); segments.push(...dimension.segments); label = dimension.label; }
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
      label = { text: constraintText(constraint), position: placement ?? add(midpoint(first, opposite), scale([-direction[1], direction[0]], 3)) };
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
      label = { text: constraintText(constraint), position: placement ?? add(origin, [Math.cos((firstAngle + secondAngle) / 2) * (radius + 4), Math.sin((firstAngle + secondAngle) / 2) * (radius + 4)]) };
    }
  }
  if (definition.dimension === "none") {
    if (constraint.kind === "FIXED_POINT" && constraint.fixedPoint) anchors.push([constraint.fixedPoint.x, constraint.fixedPoint.y]);
    else if (constraint.kind === "PARALLEL" || constraint.kind === "EQUAL") anchors.push(...points.map((point) => add(point, [4, 4])));
    else if (constraint.kind === "CONCENTRIC") {
      const circular = circularData(constraint.references[0], entities); if (circular) anchors.push(circular.center);
    } else if ((constraint.kind === "TANGENT" || constraint.kind === "PERPENDICULAR") && points.length >= 2) anchors.push(add(midpoint(points[0], points[1]), [4, 4]));
    else if (points[0]) anchors.push(add(points[0], constraint.kind === "COINCIDENT" ? [0, 0] : [4, 4]));
    else anchors.push([0, 0]);
  }
  return { symbol: definition.symbol, anchors, segments, label };
}
