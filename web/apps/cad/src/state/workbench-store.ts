import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { PlaneName, Selection, SelectionItem } from "../types";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";
import { DEFAULT_CAPTURE_SETTINGS, normalizeCaptureSettings, type CaptureSettings,
  type SelectionCaptureKind, type SketchSnapCaptureKind } from "../cad/interaction/capture-settings";

export type WorkbenchToolID = "select" | "sketch.point" | "sketch.line" | "sketch.circle" | "sketch.arc" | "sketch.polyline" | "sketch.spline" | "sketch.rectangle" | "sketch.polygon" | "sketch.slot"
  | `sketch.constraint.${"coincident"|"parallel"|"fixed"|"horizontal"|"vertical"|"perpendicular"|"tangent"|"equal"|"distance"|"length"|"radius"|"diameter"|"angle"|"concentric"|"point_on_object"|"midpoint"}`;
export type WorkbenchToolMode = "once" | "continuous";

type WorkbenchState = {
  selection: Selection;
  selections: SelectionItem[];
  preselection: Selection;
  sketchPlane?: PlaneName;
  activeSketchID?: string;
  activeToolID: WorkbenchToolID;
  activeToolMode: WorkbenchToolMode;
  navigationProfile: NavigationProfileID;
  captureSettings: CaptureSettings;
  inspectorTab: "properties" | "history";
  setSelection: (selection: Selection) => void;
  setSelections: (selections: SelectionItem[]) => void;
  setPreselection: (selection: Selection) => void;
  beginSketch: (sketchID: string, plane: PlaneName) => void;
  endSketch: () => void;
  setActiveTool: (tool: WorkbenchToolID, mode?: WorkbenchToolMode) => void;
  completeToolUse: () => void;
  setNavigationProfile: (profile: NavigationProfileID) => void;
  setCaptureEnabled: (enabled: boolean) => void;
  toggleSelectionCapture: (kind: SelectionCaptureKind) => void;
  toggleSketchSnap: (kind: SketchSnapCaptureKind) => void;
  captureAll: () => void;
  capturePointsOnly: () => void;
  setInspectorTab: (tab: "properties" | "history") => void;
};

export const useWorkbenchStore = create<WorkbenchState>()(persist((set) => ({
  selection: null, selections: [], preselection: null, activeToolID: "select", activeToolMode: "once", navigationProfile: "default",
  captureSettings: DEFAULT_CAPTURE_SETTINGS, inspectorTab: "properties",
  setSelection: (selection) => set({ selection, selections: selection ? [selection] : [], preselection: null }),
  setSelections: (selections) => {
    const unique = [...new Map(selections.map((selection) => [`${selection.kind}:${selection.id}`, selection])).values()];
    set({ selections: unique, selection: unique.at(-1) ?? null, preselection: null });
  },
  setPreselection: (preselection) => set({ preselection }),
  beginSketch: (activeSketchID, sketchPlane) => set({ activeSketchID, sketchPlane, activeToolID: "select", activeToolMode: "once" }),
  endSketch: () => set({ activeSketchID: undefined, sketchPlane: undefined, activeToolID: "select", activeToolMode: "once" }),
  setActiveTool: (activeToolID, activeToolMode) => set((state) => ({ activeToolID,
    activeToolMode: activeToolMode ?? (state.activeToolID === activeToolID ? state.activeToolMode : "once") })),
  completeToolUse: () => set((state) => state.activeToolMode === "continuous" ? state : { activeToolID: "select", activeToolMode: "once" }),
  setNavigationProfile: (navigationProfile) => set({ navigationProfile }),
  setCaptureEnabled: (enabled) => set((state) => ({ captureSettings: { ...state.captureSettings, enabled }, preselection: null })),
  toggleSelectionCapture: (kind) => set((state) => ({ captureSettings: { ...state.captureSettings,
    selection: state.captureSettings.selection.includes(kind)
      ? state.captureSettings.selection.filter((item) => item !== kind) : [...state.captureSettings.selection, kind] }, preselection: null })),
  toggleSketchSnap: (kind) => set((state) => ({ captureSettings: { ...state.captureSettings,
    sketch: state.captureSettings.sketch.includes(kind)
      ? state.captureSettings.sketch.filter((item) => item !== kind) : [...state.captureSettings.sketch, kind] } })),
  captureAll: () => set({ captureSettings: DEFAULT_CAPTURE_SETTINGS, preselection: null }),
  capturePointsOnly: () => set({ captureSettings: { enabled: true, selection: ["POINT"],
    sketch: ["GRID", "ORIGIN", "POINT", "ENDPOINT", "CENTER", "MIDPOINT"] }, preselection: null }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", merge: (persisted, current) => {
  const saved = persisted as Partial<WorkbenchState> | undefined;
  return { ...current, ...saved, captureSettings: normalizeCaptureSettings(saved?.captureSettings) };
}, partialize: (state) => ({ inspectorTab: state.inspectorTab,
  navigationProfile: state.navigationProfile, captureSettings: state.captureSettings }) }));
