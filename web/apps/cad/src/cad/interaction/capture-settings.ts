import type { Selection } from "../../types";

export const SELECTION_CAPTURE_KINDS = [
  "POINT", "CURVE", "SURFACE", "BODY", "SKETCH", "CONSTRAINT", "DATUM_PLANE", "DATUM_AXIS", "INSTANCE",
] as const;
export type SelectionCaptureKind = typeof SELECTION_CAPTURE_KINDS[number];

export const SKETCH_SNAP_KINDS = ["GRID", "ORIGIN", "POINT", "ENDPOINT", "CENTER", "MIDPOINT", "CURVE"] as const;
export type SketchSnapCaptureKind = typeof SKETCH_SNAP_KINDS[number];

export type CaptureSettings = {
  enabled: boolean;
  selection: SelectionCaptureKind[];
  sketch: SketchSnapCaptureKind[];
};

export const DEFAULT_CAPTURE_SETTINGS: CaptureSettings = {
  enabled: true,
  selection: [...SELECTION_CAPTURE_KINDS],
  sketch: [...SKETCH_SNAP_KINDS],
};

export function selectionCaptureKind(selection: Exclude<Selection, null>): SelectionCaptureKind {
  if (selection.kind === "vertex" || (selection.kind === "visual" && selection.visualType === "POINT")) return "POINT";
  if (selection.kind === "edge" || (selection.kind === "visual" && selection.visualType === "CURVE")) return "CURVE";
  if (selection.kind === "face" || (selection.kind === "visual" && selection.visualType === "SURFACE")) return "SURFACE";
  if (selection.kind === "plane") return "DATUM_PLANE";
  if (selection.kind === "axis" || selection.kind === "axis-system") return "DATUM_AXIS";
  if (selection.kind === "sketch") return "SKETCH";
  if (selection.kind === "sketch-constraint") return "CONSTRAINT";
  if (selection.kind === "instance") return "INSTANCE";
  return "BODY";
}

export function allowsSelection(settings: CaptureSettings, selection: Exclude<Selection, null>): boolean {
  return settings.enabled && settings.selection.includes(selectionCaptureKind(selection));
}

export function normalizeCaptureSettings(value: Partial<CaptureSettings> | undefined): CaptureSettings {
  const selection = value?.selection?.filter((kind): kind is SelectionCaptureKind =>
    SELECTION_CAPTURE_KINDS.includes(kind as SelectionCaptureKind));
  const sketch = value?.sketch?.filter((kind): kind is SketchSnapCaptureKind =>
    SKETCH_SNAP_KINDS.includes(kind as SketchSnapCaptureKind));
  return {
    enabled: value?.enabled ?? true,
    selection: selection ? [...new Set(selection)] : [...SELECTION_CAPTURE_KINDS],
    sketch: sketch ? [...new Set(sketch)] : [...SKETCH_SNAP_KINDS],
  };
}
