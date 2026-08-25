import type { SketchEntity, Vec2 } from "../../types";
import type { SketchSnapCaptureKind } from "./capture-settings";
import { sampleSketchEntity } from "../sketch/sketch-geometry";

export type SketchSnapKind = "ORIGIN" | "POINT" | "ENDPOINT" | "CENTER" | "MIDPOINT" | "CURVE" | "GRID";

export type SketchSnapResult = {
  point: Vec2;
  kind: SketchSnapKind;
  distancePixels: number;
  entityId?: string;
  subElement?: "POINT" | "START" | "END" | "CENTER" | "DIRECTION";
};

type Candidate = SketchSnapResult & { priority: number };

const distance = (left: Vec2, right: Vec2): number => Math.hypot(left[0] - right[0], left[1] - right[1]);

function projectToSegment(point: Vec2, start: Vec2, end: Vec2): Vec2 {
  const dx = end[0] - start[0], dy = end[1] - start[1];
  const denominator = dx * dx + dy * dy;
  const t = denominator === 0 ? 0 : Math.max(0, Math.min(1,
    ((point[0] - start[0]) * dx + (point[1] - start[1]) * dy) / denominator));
  return [start[0] + dx * t, start[1] + dy * t];
}

function projectToCurve(point: Vec2, sampled: Vec2[]): Vec2 | undefined {
  let best: Vec2 | undefined;
  let bestDistance = Number.POSITIVE_INFINITY;
  for (let index = 1; index < sampled.length; index += 1) {
    const candidate = projectToSegment(point, sampled[index - 1], sampled[index]);
    const candidateDistance = distance(point, candidate);
    if (candidateDistance < bestDistance) { best = candidate; bestDistance = candidateDistance; }
  }
  return best;
}

export function resolveSketchSnap(raw: Vec2, entities: SketchEntity[], pixelsPerUnit: number,
  gridSpacing = 10, thresholdPixels = 11,
  enabled: readonly SketchSnapCaptureKind[] = ["GRID", "ORIGIN", "POINT", "ENDPOINT", "CENTER", "MIDPOINT", "CURVE"]): SketchSnapResult | undefined {
  const scale = Math.max(pixelsPerUnit, 1.0e-6);
  const candidates: Candidate[] = [];
  const offer = (point: Vec2, kind: SketchSnapKind, priority: number, entityId?: string,
    subElement?: SketchSnapResult["subElement"], threshold = thresholdPixels) => {
    const distancePixels = distance(raw, point) * scale;
    if (distancePixels <= threshold) candidates.push({ point, kind, priority, distancePixels, entityId, subElement });
  };

  if (enabled.includes("ORIGIN")) offer([0, 0], "ORIGIN", 600, undefined, undefined, 13);
  for (const entity of entities) {
    if (enabled.includes("POINT") && entity.kind === "POINT" && entity.point) {
      offer([entity.point.x, entity.point.y], "POINT", 550, entity.id, "POINT", 13);
      continue;
    }
    const sampled = sampleSketchEntity(entity, 64);
    if (sampled.length < 2) continue;
    const start = sampled[0], end = sampled.at(-1)!;
    const closed = entity.kind === "CIRCLE" || (entity.kind === "SPLINE" && entity.closed);
    if (enabled.includes("ENDPOINT")) {
      if (!closed) {
        offer(start, "ENDPOINT", 550, entity.id, "START", 13);
        offer(end, "ENDPOINT", 550, entity.id, "END", 13);
      }
    }
    if (enabled.includes("CENTER") && entity.center) offer([entity.center.x, entity.center.y], "CENTER", 520, entity.id, "CENTER", 13);
    if (enabled.includes("MIDPOINT") && !closed) {
      const position = (sampled.length - 1) / 2, lower = Math.floor(position), upper = Math.ceil(position), blend = position - lower;
      offer([sampled[lower][0] * (1 - blend) + sampled[upper][0] * blend,
        sampled[lower][1] * (1 - blend) + sampled[upper][1] * blend], "MIDPOINT", 420, entity.id, "DIRECTION");
    }
    const projected = enabled.includes("CURVE") ? projectToCurve(raw, sampled) : undefined;
    if (projected) offer(projected, "CURVE", 300, entity.id, "DIRECTION", 8);
  }

  if (enabled.includes("GRID")) {
    const grid: Vec2 = [Math.round(raw[0] / gridSpacing) * gridSpacing, Math.round(raw[1] / gridSpacing) * gridSpacing];
    offer(grid, "GRID", 100, undefined, undefined, 8);
  }
  candidates.sort((left, right) => right.priority - left.priority || left.distancePixels - right.distancePixels);
  const best = candidates[0];
  if (!best) return undefined;
  const result: SketchSnapResult = { point: best.point, kind: best.kind, distancePixels: best.distancePixels };
  if (best.entityId !== undefined) result.entityId = best.entityId;
  if (best.subElement !== undefined) result.subElement = best.subElement;
  return result;
}
