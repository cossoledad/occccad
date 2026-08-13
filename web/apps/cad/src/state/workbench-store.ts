import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { PlaneName, Selection } from "../types";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";

export type WorkbenchToolID = "select" | "sketch.point" | "sketch.line" | "sketch.rectangle" | "sketch.constraint.coincident" | "sketch.constraint.parallel";

type WorkbenchState = {
  selection: Selection;
  preselection: Selection;
  sketchPlane?: PlaneName;
  activeSketchID?: string;
  activeToolID: WorkbenchToolID;
  navigationProfile: NavigationProfileID;
  inspectorTab: "properties" | "history";
  setSelection: (selection: Selection) => void;
  setPreselection: (selection: Selection) => void;
  beginSketch: (sketchID: string, plane: PlaneName) => void;
  endSketch: () => void;
  setActiveTool: (tool: WorkbenchToolID) => void;
  setNavigationProfile: (profile: NavigationProfileID) => void;
  setInspectorTab: (tab: "properties" | "history") => void;
};

export const useWorkbenchStore = create<WorkbenchState>()(persist((set) => ({
  selection: null, preselection: null, activeToolID: "select", navigationProfile: "default", inspectorTab: "properties",
  setSelection: (selection) => set({ selection }),
  setPreselection: (preselection) => set({ preselection }),
  beginSketch: (activeSketchID, sketchPlane) => set({ activeSketchID, sketchPlane, activeToolID: "select" }),
  endSketch: () => set({ activeSketchID: undefined, sketchPlane: undefined, activeToolID: "select" }),
  setActiveTool: (activeToolID) => set({ activeToolID }),
  setNavigationProfile: (navigationProfile) => set({ navigationProfile }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", partialize: (state) => ({ inspectorTab: state.inspectorTab,
  navigationProfile: state.navigationProfile }) }));
