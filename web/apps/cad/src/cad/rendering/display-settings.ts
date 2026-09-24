export type SolidDisplaySettings = { mode: "shaded" | "wireframe"; edges: boolean; vertices: boolean };
export const DEFAULT_SOLID_DISPLAY: SolidDisplaySettings = { mode: "shaded", edges: true, vertices: true };
export type ReferenceVisibility = { planes: boolean; axes: boolean; coordinateSystems: boolean };
export const DEFAULT_REFERENCE_VISIBILITY: ReferenceVisibility = { planes: true, axes: true, coordinateSystems: true };
export function normalizeReferenceVisibility(value: unknown): ReferenceVisibility {
  const input = value && typeof value === "object" ? value as Partial<ReferenceVisibility> : {};
  return { planes: input.planes !== false, axes: input.axes !== false, coordinateSystems: input.coordinateSystems !== false };
}
export function normalizeSolidDisplaySettings(value: unknown): SolidDisplaySettings {
  // Migrate the old presets while preserving independent edge/point preferences.
  if (value === "default" || value === "smooth" || value === "wireframe") return {
    mode: value === "wireframe" ? "wireframe" : "shaded",
    edges: value !== "smooth", vertices: value === "default",
  };
  const input = value && typeof value === "object" ? value as Partial<SolidDisplaySettings> : {};
  return { mode: input.mode === "wireframe" ? "wireframe" : "shaded",
    edges: input.edges !== false, vertices: input.vertices !== false };
}
export function referenceCategory(kind: unknown, axis?: unknown): keyof ReferenceVisibility | undefined {
  if (kind === "plane") return "planes";
  if (kind === "axis-system" || (kind === "axis" && axis !== "DATUM")) return "coordinateSystems";
  if (kind === "axis") return "axes";
}

