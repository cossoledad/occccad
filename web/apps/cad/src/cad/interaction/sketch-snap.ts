import type { SketchEntity, Vec2 } from "../../types";

export type SketchSnapKind = "ORIGIN" | "POINT" | "ENDPOINT" | "MIDPOINT" | "LINE" | "GRID";

export type SketchSnapResult = {
  point: Vec2;
  kind: SketchSnapKind;
  distancePixels: number;
  entityId?: string;
  subElement?: "POINT" | "START" | "END" | "DIRECTION";
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

export function resolveSketchSnap(raw: Vec2, entities: SketchEntity[], pixelsPerUnit: number,
  gridSpacing = 10, thresholdPixels = 11): SketchSnapResult | undefined {
  const scale = Math.max(pixelsPerUnit, 1.0e-6);
  const candidates: Candidate[] = [];
  const offer = (point: Vec2, kind: SketchSnapKind, priority: number, entityId?: string,
    subElement?: SketchSnapResult["subElement"], threshold = thresholdPixels) => {
    const distancePixels = distance(raw, point) * scale;
    if (distancePixels <= threshold) candidates.push({ point, kind, priority, distancePixels, entityId, subElement });
  };

  offer([0, 0], "ORIGIN", 600, undefined, undefined, 13);
  for (const entity of entities) {
    if (entity.kind === "POINT" && entity.point) {
      offer([entity.point.x, entity.point.y], "POINT", 550, entity.id, "POINT", 13);
      continue;
    }
    if (entity.kind !== "LINE" || !entity.start || !entity.end) continue;
    const start: Vec2 = [entity.start.x, entity.start.y];
    const end: Vec2 = [entity.end.x, entity.end.y];
    offer(start, "ENDPOINT", 550, entity.id, "START", 13);
    offer(end, "ENDPOINT", 550, entity.id, "END", 13);
    offer([(start[0] + end[0]) / 2, (start[1] + end[1]) / 2], "MIDPOINT", 420, entity.id, "DIRECTION");
    offer(projectToSegment(raw, start, end), "LINE", 300, entity.id, "DIRECTION", 8);
  }

  const grid: Vec2 = [Math.round(raw[0] / gridSpacing) * gridSpacing, Math.round(raw[1] / gridSpacing) * gridSpacing];
  offer(grid, "GRID", 100, undefined, undefined, 8);
  candidates.sort((left, right) => right.priority - left.priority || left.distancePixels - right.distancePixels);
  const best = candidates[0];
  if (!best) return undefined;
  const result: SketchSnapResult = { point: best.point, kind: best.kind, distancePixels: best.distancePixels };
  if (best.entityId !== undefined) result.entityId = best.entityId;
  if (best.subElement !== undefined) result.subElement = best.subElement;
  return result;
}
