export type SolidDisplaySettings = { mode: "shaded" | "wireframe"; edges: boolean; vertices: boolean; edgeStyle?:'natural'|'uniform'; smoothEdgeOpacity?:number; sharpEdgeAngle?:number };
export const DEFAULT_SOLID_DISPLAY: SolidDisplaySettings = { mode: "shaded", edges: true, vertices: false,edgeStyle:'natural',smoothEdgeOpacity:.12,sharpEdgeAngle:35 };
export type ReferenceVisibility = { planes: boolean; axes: boolean; points:boolean; coordinateSystems: boolean };
export const DEFAULT_REFERENCE_VISIBILITY: ReferenceVisibility = { planes: true, axes: true, points:true, coordinateSystems: true };
export function normalizeReferenceVisibility(value: unknown): ReferenceVisibility {
  const input = value && typeof value === "object" ? value as Partial<ReferenceVisibility> : {};
  return { points:input.points!==false,planes: input.planes !== false, axes: input.axes !== false, coordinateSystems: input.coordinateSystems !== false };
}
export function normalizeSolidDisplaySettings(value: unknown): SolidDisplaySettings {
  // Migrate the old presets while preserving independent edge/point preferences.
  if (value === "default" || value === "smooth" || value === "wireframe") return {
    ...DEFAULT_SOLID_DISPLAY,
    mode: value === "wireframe" ? "wireframe" : "shaded",
    edges: value !== "smooth", vertices: value === "default",
  };
  const input = value && typeof value === "object" ? value as Partial<SolidDisplaySettings> : {};
  const bounded=(value:unknown,fallback:number,low:number,high:number)=>typeof value==='number'&&Number.isFinite(value)?Math.max(low,Math.min(high,value)):fallback;
  return { mode: input.mode === "wireframe" ? "wireframe" : "shaded",
    edges: input.edges !== false, vertices: input.vertices === true,edgeStyle:input.edgeStyle==='uniform'?'uniform':'natural',
    smoothEdgeOpacity:bounded(input.smoothEdgeOpacity,.12,0,.4),sharpEdgeAngle:bounded(input.sharpEdgeAngle,35,10,75) };
}
export function referenceCategory(kind: unknown, axis?: unknown): keyof ReferenceVisibility | undefined {
  if(kind==="reference-point")return "points";
  if (kind === "plane") return "planes";
  if (kind === "datum-point" || kind === "axis-system" || (kind === "axis" && axis !== "DATUM")) return "coordinateSystems";
  if (kind === "axis") return "axes";
}


export type GridVisibility = { scene: boolean; sketch: boolean };
export const DEFAULT_GRID_VISIBILITY: GridVisibility = {scene:true,sketch:true};
export function normalizeGridVisibility(value: unknown): GridVisibility {
  const input=value&&typeof value==="object"?value as Partial<GridVisibility>:{};
  return {scene:input.scene!==false,sketch:input.sketch!==false};
}
