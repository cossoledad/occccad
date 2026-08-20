import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { PlaneName, Selection } from "../types";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";

export type WorkbenchToolID = "select" | "sketch.point" | "sketch.line" | "sketch.circle" | "sketch.arc" | "sketch.polyline" | "sketch.spline" | "sketch.rectangle" | "sketch.polygon" | "sketch.slot"
  | `sketch.constraint.${"coincident"|"parallel"|"fixed"|"horizontal"|"vertical"|"perpendicular"|"tangent"|"equal"|"distance"|"length"|"radius"|"diameter"|"angle"|"concentric"|"point_on_object"|"midpoint"}`;
export type WorkbenchToolMode = "once" | "continuous";

type WorkbenchState = {
  selection: Selection;
  preselection: Selection;
  sketchPlane?: PlaneName;
  activeSketchID?: string;
  activeToolID: WorkbenchToolID;
  activeToolMode: WorkbenchToolMode;
  navigationProfile: NavigationProfileID;
  inspectorTab: "properties" | "history";
  setSelection: (selection: Selection) => void;
  setPreselection: (selection: Selection) => void;
  beginSketch: (sketchID: string, plane: PlaneName) => void;
  endSketch: () => void;
  setActiveTool: (tool: WorkbenchToolID, mode?: WorkbenchToolMode) => void;
  completeToolUse: () => void;
  setNavigationProfile: (profile: NavigationProfileID) => void;
  setInspectorTab: (tab: "properties" | "history") => void;
};

export const useWorkbenchStore = create<WorkbenchState>()(persist((set) => ({
  selection: null, preselection: null, activeToolID: "select", activeToolMode: "once", navigationProfile: "default", inspectorTab: "properties",
  setSelection: (selection) => set({ selection }),
  setPreselection: (preselection) => set({ preselection }),
  beginSketch: (activeSketchID, sketchPlane) => set({ activeSketchID, sketchPlane, activeToolID: "select", activeToolMode: "once" }),
  endSketch: () => set({ activeSketchID: undefined, sketchPlane: undefined, activeToolID: "select", activeToolMode: "once" }),
  setActiveTool: (activeToolID, activeToolMode = "once") => set({ activeToolID, activeToolMode }),
  completeToolUse: () => set((state) => state.activeToolMode === "continuous" ? state : { activeToolID: "select", activeToolMode: "once" }),
  setNavigationProfile: (navigationProfile) => set({ navigationProfile }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", partialize: (state) => ({ inspectorTab: state.inspectorTab,
  navigationProfile: state.navigationProfile }) }));
