export const SKETCH_INPUT_POLICY = {
  gridSpacing: 10,
  snapThresholdPixels: 11,
  minimumGeometryLength: 0.01,
  lengthDecimals: 2,
  angleDecimals: 1,
} as const;

export function normalizeSketchDimensionValue(value: number, unit: "mm" | "deg"): number {
  const decimals = unit === "deg" ? SKETCH_INPUT_POLICY.angleDecimals : SKETCH_INPUT_POLICY.lengthDecimals;
  const scale = 10 ** decimals;
  return Math.round(value * scale) / scale;
}

export function formatSketchDimensionValue(value: number, unit: "mm" | "deg"): string {
  const decimals = unit === "deg" ? SKETCH_INPUT_POLICY.angleDecimals : SKETCH_INPUT_POLICY.lengthDecimals;
  return normalizeSketchDimensionValue(value, unit).toFixed(decimals).replace(/\.?0+$/u, "");
}

export function formatSketchDimension(value: number, unit: "mm" | "deg", prefix = ""): string {
  return `${prefix}${formatSketchDimensionValue(value, unit)}${unit === "deg" ? "°" : " mm"}`;
}

export type SketchReferenceGeometry =
  | { kind: "POINT"; point: readonly [number, number] }
  | { kind: "LINE"; start: readonly [number, number]; end: readonly [number, number] }
  | { kind: "CIRCLE"; center: readonly [number, number]; edge: readonly [number, number] };

export type SketchReferenceDimension = { text: string; position: [number, number] };

// Creation tools describe only their foundational geometry. This policy owns
// dimension semantics, formatting and placement, so compound tools do not
// invent feature-specific labels.
export function sketchReferenceDimensions(geometry: readonly SketchReferenceGeometry[]): SketchReferenceDimension[] {
  const result: SketchReferenceDimension[] = [];
  for (const primitive of geometry) {
    if (primitive.kind === "POINT") {
      result.push({ text: `X ${formatSketchDimension(primitive.point[0], "mm")}  Y ${formatSketchDimension(primitive.point[1], "mm")}`,
        position: [primitive.point[0], primitive.point[1] + 3] });
      continue;
    }
    const first = primitive.kind === "LINE" ? primitive.start : primitive.center;
    const second = primitive.kind === "LINE" ? primitive.end : primitive.edge;
    const dx=second[0]-first[0],dy=second[1]-first[1],length=Math.hypot(dx,dy);
    if (length < SKETCH_INPUT_POLICY.minimumGeometryLength) continue;
    const midpoint:[number,number]=[(first[0]+second[0])/2,(first[1]+second[1])/2];
    if (primitive.kind === "CIRCLE") {
      result.push({ text: formatSketchDimension(length,"mm","R "), position: midpoint });
      continue;
    }
    const normal:[number,number]=[-dy/length,dx/length];
    result.push({ text: formatSketchDimension(length,"mm"), position: [midpoint[0]+normal[0]*3,midpoint[1]+normal[1]*3] });
  }
  return result;
}
