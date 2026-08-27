import type { SketchEntity, SketchPoint2, Vec2 } from "../../types";

const point = (value: SketchPoint2): Vec2 => [value.x, value.y];

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
  if (entity.kind === "SPLINE" && entity.controlPoints && entity.controlPoints.length >= 2) {
    return sampleInterpolatingSpline(entity.controlPoints.map(point), Boolean(entity.closed), segments);
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
