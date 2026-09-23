export type RenderMode = "default" | "smooth" | "wireframe";
export type ReferenceVisibility = { planes: boolean; axes: boolean; coordinateSystems: boolean };
export const DEFAULT_REFERENCE_VISIBILITY: ReferenceVisibility = { planes: true, axes: true, coordinateSystems: true };
export function normalizeReferenceVisibility(value: unknown): ReferenceVisibility {
  const input = value && typeof value === "object" ? value as Partial<ReferenceVisibility> : {};
  return { planes: input.planes !== false, axes: input.axes !== false, coordinateSystems: input.coordinateSystems !== false };
}
export function normalizeRenderMode(value: unknown): RenderMode {
  return value === "smooth" || value === "wireframe" ? value : "default";
}
export function referenceCategory(kind: unknown, axis?: unknown): keyof ReferenceVisibility | undefined {
  if (kind === "plane") return "planes";
  if (kind === "axis-system" || (kind === "axis" && axis !== "DATUM")) return "coordinateSystems";
  if (kind === "axis") return "axes";
}

