export const VISUAL_PRECISION_OPTIONS = [
  { value: "balanced", label: "均衡（0.15 mm）" },
  { value: "fine", label: "精细（0.05 mm）" },
  { value: "extra-fine", label: "高精细（0.02 mm）" },
] as const;

export type VisualPrecision = typeof VISUAL_PRECISION_OPTIONS[number]["value"];
export const DEFAULT_VISUAL_PRECISION: VisualPrecision = "balanced";

export function normalizeVisualPrecision(value: unknown): VisualPrecision {
  return VISUAL_PRECISION_OPTIONS.find(option => option.value === value)?.value ?? DEFAULT_VISUAL_PRECISION;
}
