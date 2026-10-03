import type { SketchEntity, SketchPoint2, Vec2 } from "../../types";

const point = (value: SketchPoint2): Vec2 => [value.x, value.y];

export function ellipsePoint(entity: SketchEntity, parameter: number): Vec2 | undefined {
  if (!entity.center || entity.majorRadius === undefined || entity.minorRadius === undefined) return undefined;
  const rotation = entity.rotation ?? 0, c = Math.cos(rotation), s = Math.sin(rotation);
  const x = entity.majorRadius * Math.cos(parameter), y = entity.minorRadius * Math.sin(parameter);
  return [entity.center.x + c * x - s * y, entity.center.y + s * x + c * y];
}

export function splineEditablePoints(entity: SketchEntity): SketchPoint2[] {
  return entity.mode === "CONTROL" ? entity.poles ?? [] : entity.controlPoints ?? [];
}

export function splineEditablePointIDs(entity: SketchEntity): string[] {
  return entity.mode === "CONTROL" ? entity.poleIds ?? [] : entity.controlPointIds ?? [];
}

export function splineReferencePoint(entity: SketchEntity, index?: number, id?: string): Vec2 | undefined {
  const resolved = id === undefined ? index : splineEditablePointIDs(entity).indexOf(id);
  const value = resolved === undefined ? undefined : splineEditablePoints(entity)[resolved];
  return value ? point(value) : undefined;
}

// Homogeneous de Boor evaluates the persisted canonical rational B-spline.
// It neither refits points nor interprets fit points as control vertices.
export function evaluateCanonicalSpline(entity: SketchEntity, parameter: number): Vec2 | undefined {
  const degree = entity.degree ?? 0, poles = entity.poles ?? [], weights = entity.weights ?? [];
  const uniqueKnots = entity.knots ?? [], multiplicities = entity.multiplicities ?? [];
  if (entity.periodic || !Number.isInteger(degree) || degree < 1 || degree > 16 || poles.length <= degree ||
      weights.length !== poles.length || uniqueKnots.length !== multiplicities.length) return undefined;
  if (multiplicities.reduce((sum, count) => sum + count, 0) !== poles.length + degree + 1) return undefined;
  const knots: number[] = [];
  for (let index = 0; index < uniqueKnots.length; index++) {
    const knot = uniqueKnots[index], count = multiplicities[index];
    if (!Number.isFinite(knot) || (index > 0 && knot <= uniqueKnots[index - 1]) ||
        !Number.isInteger(count) || count < 1 || count > degree + 1) return undefined;
    for (let repeated = 0; repeated < count; repeated++) knots.push(knot);
  }
  const first = knots[degree], last = knots[poles.length];
  if (!Number.isFinite(parameter) || parameter < first || parameter > last || first >= last) return undefined;
  let span = degree;
  while (span < poles.length - 1 && knots[span + 1] <= parameter) span++;
  const work: Array<[number, number, number]> = [];
  for (let index = 0; index <= degree; index++) {
    const poleIndex = span - degree + index, pole = poles[poleIndex], weight = weights[poleIndex];
    if (!pole || !Number.isFinite(pole.x) || !Number.isFinite(pole.y) || !Number.isFinite(weight) || weight <= 0) return undefined;
    work.push([pole.x * weight, pole.y * weight, weight]);
  }
  for (let order = 1; order <= degree; order++) {
    for (let index = degree; index >= order; index--) {
      const knotIndex = span - degree + index;
      const denominator = knots[knotIndex + degree - order + 1] - knots[knotIndex];
      const alpha = denominator === 0 ? 0 : (parameter - knots[knotIndex]) / denominator;
      const left = work[index - 1], right = work[index];
      work[index] = [(1 - alpha) * left[0] + alpha * right[0], (1 - alpha) * left[1] + alpha * right[1], (1 - alpha) * left[2] + alpha * right[2]];
    }
  }
  const value = work[degree];
  if (!Number.isFinite(value[2]) || value[2] <= 0) return undefined;
  const result: Vec2 = [value[0] / value[2], value[1] / value[2]];
  return result.every(Number.isFinite) ? result : undefined;
}

export function sampleInterpolatingSpline(fitPoints: Vec2[], closed: boolean, segments = 64): Vec2[] {
  if (fitPoints.length < 2) return [...fitPoints];
  const segmentCount = closed ? fitPoints.length : fitPoints.length - 1;
  const perSegment = Math.max(4, Math.ceil(segments / segmentCount));
  const at = (index: number): Vec2 => closed
    ? fitPoints[(index % fitPoints.length + fitPoints.length) % fitPoints.length]
    : fitPoints[Math.max(0, Math.min(fitPoints.length - 1, index))];
  const result: Vec2[] = [];
  for (let segment = 0; segment < segmentCount; segment += 1) {
    const p0=at(segment-1),p1=at(segment),p2=at(segment+1),p3=at(segment+2);
    for (let step=0;step<perSegment;step+=1) {
      const t=step/perSegment,t2=t*t,t3=t2*t;
      result.push([0.5*((2*p1[0])+(-p0[0]+p2[0])*t+(2*p0[0]-5*p1[0]+4*p2[0]-p3[0])*t2+(-p0[0]+3*p1[0]-3*p2[0]+p3[0])*t3),
        0.5*((2*p1[1])+(-p0[1]+p2[1])*t+(2*p0[1]-5*p1[1]+4*p2[1]-p3[1])*t2+(-p0[1]+3*p1[1]-3*p2[1]+p3[1])*t3)]);
    }
  }
  result.push(closed ? fitPoints[0] : fitPoints.at(-1)!);
  return result;
}

export function sampleSketchEntity(entity: SketchEntity, segments = 64): Vec2[] {
  if (entity.kind === "POINT" && entity.point) return [point(entity.point)];
  if (entity.kind === "LINE" && entity.start && entity.end) return [point(entity.start), point(entity.end)];
  if ((entity.kind === "CIRCLE" || entity.kind === "ARC") && entity.center && entity.radius) {
    const start = entity.kind === "CIRCLE" ? 0 : entity.startAngle ?? 0;
    const end = entity.kind === "CIRCLE" ? Math.PI * 2 : entity.endAngle ?? 0;
    const count = Math.max(8, Math.ceil(segments * Math.abs(end - start) / (Math.PI * 2)));
    return Array.from({ length: count + 1 }, (_, index) => {
      const angle = start + (end - start) * index / count;
      return [entity.center!.x + entity.radius! * Math.cos(angle), entity.center!.y + entity.radius! * Math.sin(angle)];
    });
  }
  if (entity.kind === "SPLINE") {
    if (entity.poles?.length) {
      const start = entity.parameterStart ?? entity.knots?.[0], end = entity.parameterEnd ?? entity.knots?.at(-1);
      if (start === undefined || end === undefined || start >= end) return [];
      const sampled = Array.from({ length: Math.max(2, segments) + 1 }, (_, index) => evaluateCanonicalSpline(entity, start + (end - start) * index / Math.max(2, segments)));
      return sampled.every((value): value is Vec2 => Boolean(value)) ? sampled : [];
    }
    if (entity.mode !== "CONTROL" && entity.controlPoints && entity.controlPoints.length >= 2)
      return sampleInterpolatingSpline(entity.controlPoints.map(point), Boolean(entity.closed), segments);
    return [];
  }
  if (entity.kind === "ELLIPSE" || entity.kind === "ELLIPTICAL_ARC") {
    const start = entity.kind === "ELLIPSE" ? 0 : entity.startAngle ?? 0;
    const end = entity.kind === "ELLIPSE" ? 2 * Math.PI : entity.endAngle ?? 0;
    const count = Math.max(8, Math.ceil(segments * Math.abs(end - start) / (2 * Math.PI)));
    if (!ellipsePoint(entity, start)) return [];
    return Array.from({ length: count + 1 }, (_, index) => ellipsePoint(entity, start + (end - start) * index / count)!);
  }
  return [];
}

export function sketchEntityPoint(entity: SketchEntity, subElement: "POINT" | "START" | "END" | "CENTER"): Vec2 | undefined {
  if (subElement === "POINT" && entity.kind === "POINT" && entity.point) return point(entity.point);
  if (subElement === "CENTER" && entity.center) return point(entity.center);
  if (entity.kind === "ELLIPSE" || entity.kind === "CIRCLE" || (entity.kind === "SPLINE" && entity.closed)) return undefined;
  const sampled = sampleSketchEntity(entity);
  if (subElement === "START") return sampled[0];
  if (subElement === "END") return sampled.at(-1);
  return undefined;
}
