import type { SketchEntity, SketchPoint2, Vec2 } from "../../types";

const point = (value: SketchPoint2): Vec2 => [value.x, value.y];

function sampleSpline(control: Vec2[], degree: number, segments: number): Vec2[] {
  const n = control.length - 1;
  const p = Math.max(1, Math.min(degree, n));
  const maximum = n - p + 1;
  const knots = Array.from({ length: n + p + 2 }, (_, index) =>
    index <= p ? 0 : index > n ? maximum : index - p);
  const evaluate = (parameter: number): Vec2 => {
    const u = Math.min(parameter, maximum - Number.EPSILON);
    let span = Math.min(n, Math.floor(u) + p);
    if (parameter >= maximum) span = n;
    const values = Array.from({ length: p + 1 }, (_, index) => [...control[span - p + index]] as Vec2);
    for (let level = 1; level <= p; level += 1) {
      for (let index = p; index >= level; index -= 1) {
        const source = span - p + index;
        const denominator = knots[source + p - level + 1] - knots[source];
        const alpha = denominator === 0 ? 0 : (u - knots[source]) / denominator;
        values[index] = [values[index - 1][0] * (1 - alpha) + values[index][0] * alpha,
          values[index - 1][1] * (1 - alpha) + values[index][1] * alpha];
      }
    }
    return values[p];
  };
  return Array.from({ length: segments + 1 }, (_, index) => evaluate(maximum * index / segments));
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
  if (entity.kind === "SPLINE" && entity.controlPoints && entity.controlPoints.length >= 2) {
    const controls = entity.controlPoints.map(point);
    if (entity.closed) controls.push(controls[0]);
    const sampled = sampleSpline(controls, entity.degree ?? 3, segments);
    return sampled;
  }
  return [];
}

export function sketchEntityPoint(entity: SketchEntity, subElement: "POINT" | "START" | "END" | "CENTER"): Vec2 | undefined {
  if (subElement === "POINT" && entity.kind === "POINT" && entity.point) return point(entity.point);
  if (subElement === "CENTER" && entity.center) return point(entity.center);
  const sampled = sampleSketchEntity(entity);
  if (subElement === "START") return sampled[0];
  if (subElement === "END") return sampled.at(-1);
  return undefined;
}
