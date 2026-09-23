import type { Vec3 } from "../../types";

export type PatternAxis = "X" | "Y" | "Z";
export type InstancePatternPreview = { sourceInstanceId: string; axis: PatternAxis; count: number; spacing: number; reversed: boolean;
  parentOccurrencePath?: string; parentRotation?: [number, number, number, number] };

export function instancePatternOffsets(input: Pick<InstancePatternPreview, "axis" | "count" | "spacing" | "reversed">): Vec3[] {
  if (!Number.isInteger(input.count) || input.count < 2 || input.count > 128 ||
      !Number.isFinite(input.spacing) || input.spacing <= 0 || input.spacing > 1e6) return [];
  const coordinate = { X: 0, Y: 1, Z: 2 }[input.axis];
  if (coordinate === undefined) return [];
  return Array.from({ length: input.count - 1 }, (_, index) => {
    const result: Vec3 = [0, 0, 0];
    result[coordinate] = (index + 1) * input.spacing * (input.reversed ? -1 : 1);
    return result;
  });
}
