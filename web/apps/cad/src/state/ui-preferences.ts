import { create } from "zustand";
import { persist } from "zustand/middleware";
import { DEFAULT_CAPTURE_SETTINGS, normalizeCaptureSettings, type CaptureSettings,
  type SelectionCaptureKind, type SketchSnapCaptureKind } from "../cad/interaction/capture-settings";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";
import { migrateTreeVisibilityOverrides, type TreeVisibilityOverrides } from "../cad/interaction/tree-visibility";

export type ToolbarOrientation = "horizontal" | "vertical";
export type ToolbarLayout = { x?: number; y?: number; orientation: ToolbarOrientation };
export type DisplayLengthUnit = "mm" | "cm" | "m" | "in";

export const DEFAULT_STRUCTURE_TREE_WIDTH = 330;
export const MIN_STRUCTURE_TREE_WIDTH = 220;
export const MAX_STRUCTURE_TREE_WIDTH = 640;

type UIPreferences = {
  inspectorOpen: boolean;
  toolbarLayouts: Record<string, ToolbarLayout>;
  treeVisibilityOverrides: TreeVisibilityOverrides;
  structureTreeWidth: number;
  navigationProfile: NavigationProfileID;
  captureSettings: CaptureSettings;
  displayLengthUnit: DisplayLengthUnit;
  documentLengthUnits: Record<string, DisplayLengthUnit>;
  setInspectorOpen: (open: boolean) => void;
  setToolbarLayout: (id: string, layout: ToolbarLayout) => void;
  setTreeVisibility: (key: string, visible: boolean) => void;
  setStructureTreeWidth: (width: number) => void;
  setNavigationProfile: (profile: NavigationProfileID) => void;
  setCaptureEnabled: (enabled: boolean) => void;
  toggleSelectionCapture: (kind: SelectionCaptureKind) => void;
  toggleSketchSnap: (kind: SketchSnapCaptureKind) => void;
  captureAll: () => void;
  capturePointsOnly: () => void;
  setDisplayLengthUnit: (unit: DisplayLengthUnit) => void;
  setDocumentLengthUnit: (documentID: string, unit?: DisplayLengthUnit) => void;
};

export function normalizeDisplayLengthUnit(value: unknown): DisplayLengthUnit {
  return value === "cm" || value === "m" || value === "in" ? value : "mm";
}

export function clampStructureTreeWidth(value: unknown): number {
  const width = typeof value === "number" && Number.isFinite(value) ? value : DEFAULT_STRUCTURE_TREE_WIDTH;
  return Math.round(Math.max(MIN_STRUCTURE_TREE_WIDTH, Math.min(MAX_STRUCTURE_TREE_WIDTH, width)));
}

export function effectiveLengthUnit(globalUnit: DisplayLengthUnit,
  documentUnits: Record<string, DisplayLengthUnit>, documentID?: string): DisplayLengthUnit {
  return documentID ? documentUnits[documentID] ?? globalUnit : globalUnit;
}

export function displayLengthToMillimeters(value: number, unit: DisplayLengthUnit): number {
  return value * ({ mm: 1, cm: 10, m: 1000, in: 25.4 }[unit]);
}

export function millimetersToDisplayLength(value: number, unit: DisplayLengthUnit): number {
  return value / ({ mm: 1, cm: 10, m: 1000, in: 25.4 }[unit]);
}

export function normalizeToolbarLayout(value: unknown, fallback: ToolbarOrientation): ToolbarLayout {
  if (!value || typeof value !== "object") return { orientation: fallback };
  const candidate = value as Partial<ToolbarLayout>;
  const orientation = candidate.orientation === "vertical" || candidate.orientation === "horizontal"
    ? candidate.orientation : fallback;
  const x = Number.isFinite(candidate.x) ? candidate.x : undefined;
  const y = Number.isFinite(candidate.y) ? candidate.y : undefined;
  return { orientation, ...(x === undefined ? {} : { x }), ...(y === undefined ? {} : { y }) };
}

export const useUIPreferences = create<UIPreferences>()(persist((set) => ({
  inspectorOpen: true,
  toolbarLayouts: {},
  treeVisibilityOverrides: {},
  structureTreeWidth: DEFAULT_STRUCTURE_TREE_WIDTH,
  navigationProfile: "default",
  captureSettings: { ...DEFAULT_CAPTURE_SETTINGS, selection: [...DEFAULT_CAPTURE_SETTINGS.selection], sketch: [...DEFAULT_CAPTURE_SETTINGS.sketch] },
  displayLengthUnit: "mm",
  documentLengthUnits: {},
  setInspectorOpen: (inspectorOpen) => set({ inspectorOpen }),
  setToolbarLayout: (id, layout) => set((state) => ({
    toolbarLayouts: { ...state.toolbarLayouts, [id]: normalizeToolbarLayout(layout, "horizontal") },
  })),
  setTreeVisibility: (key, visible) => set((state) => ({
    treeVisibilityOverrides: { ...state.treeVisibilityOverrides, [key]: visible },
  })),
  setStructureTreeWidth: (structureTreeWidth) => set({ structureTreeWidth: clampStructureTreeWidth(structureTreeWidth) }),
  setNavigationProfile: (navigationProfile) => set({ navigationProfile }),
  setCaptureEnabled: (enabled) => set((state) => ({ captureSettings: { ...state.captureSettings, enabled } })),
  toggleSelectionCapture: (kind) => set((state) => ({ captureSettings: { ...state.captureSettings,
    selection: state.captureSettings.selection.includes(kind)
      ? state.captureSettings.selection.filter((item) => item !== kind) : [...state.captureSettings.selection, kind] } })),
  toggleSketchSnap: (kind) => set((state) => ({ captureSettings: { ...state.captureSettings,
    sketch: state.captureSettings.sketch.includes(kind)
      ? state.captureSettings.sketch.filter((item) => item !== kind) : [...state.captureSettings.sketch, kind] } })),
  captureAll: () => set({ captureSettings: { ...DEFAULT_CAPTURE_SETTINGS,
    selection: [...DEFAULT_CAPTURE_SETTINGS.selection], sketch: [...DEFAULT_CAPTURE_SETTINGS.sketch] } }),
  capturePointsOnly: () => set({ captureSettings: { enabled: true, selection: ["POINT"],
    sketch: ["GRID", "ORIGIN", "POINT", "ENDPOINT", "CENTER", "MIDPOINT"] } }),
  setDisplayLengthUnit: (displayLengthUnit) => set({ displayLengthUnit }),
  setDocumentLengthUnit: (documentID, unit) => set((state) => {
    const documentLengthUnits = { ...state.documentLengthUnits };
    if (unit) documentLengthUnits[documentID] = unit; else delete documentLengthUnits[documentID];
    return { documentLengthUnits };
  }),
}), {
  name: "occccad.ui-preferences.v1",
  version: 3,
  migrate: (persisted) => {
    const value = (persisted && typeof persisted === "object" ? persisted : {}) as Partial<UIPreferences> & { hiddenTreeKeys?: string[] };
    const documentLengthUnits = Object.fromEntries(Object.entries(value.documentLengthUnits ?? {})
      .map(([id, unit]) => [id, normalizeDisplayLengthUnit(unit)]));
    return { ...value, treeVisibilityOverrides: migrateTreeVisibilityOverrides(value),
      structureTreeWidth: clampStructureTreeWidth(value.structureTreeWidth),
      navigationProfile: value.navigationProfile === "catia" ? "catia" : "default",
      captureSettings: normalizeCaptureSettings(value.captureSettings),
      displayLengthUnit: normalizeDisplayLengthUnit(value.displayLengthUnit), documentLengthUnits } as UIPreferences;
  },
  partialize: (state) => ({ inspectorOpen: state.inspectorOpen, toolbarLayouts: state.toolbarLayouts,
    treeVisibilityOverrides: state.treeVisibilityOverrides, structureTreeWidth: state.structureTreeWidth,
    navigationProfile: state.navigationProfile, captureSettings: state.captureSettings,
    displayLengthUnit: state.displayLengthUnit, documentLengthUnits: state.documentLengthUnits }),
}));
