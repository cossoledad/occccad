import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { PlaneName, Selection } from "../types";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";

export type WorkbenchToolID = "select" | "sketch.rectangle";

type WorkbenchState = {
  tabs: string[];
  activeDocumentID?: string;
  selection: Selection;
  sketchPlane?: PlaneName;
  activeToolID: WorkbenchToolID;
  navigationProfile: NavigationProfileID;
  inspectorTab: "properties" | "history";
  openTab: (documentID: string) => void;
  closeTab: (documentID: string) => void;
  setActiveDocument: (documentID?: string) => void;
  setSelection: (selection: Selection) => void;
  beginSketch: (plane: PlaneName) => void;
  endSketch: () => void;
  setActiveTool: (tool: WorkbenchToolID) => void;
  setNavigationProfile: (profile: NavigationProfileID) => void;
  setInspectorTab: (tab: "properties" | "history") => void;
};

export const useWorkbenchStore = create<WorkbenchState>()(persist((set) => ({
  tabs: [], selection: null, activeToolID: "select", navigationProfile: "default", inspectorTab: "properties",
  openTab: (documentID) => set((state) => ({
    tabs: state.tabs.includes(documentID) ? state.tabs : [...state.tabs, documentID],
    activeDocumentID: documentID, selection: null,
  })),
  closeTab: (documentID) => set((state) => {
    const index = state.tabs.indexOf(documentID);
    const tabs = state.tabs.filter((item) => item !== documentID);
    return { tabs, activeDocumentID: state.activeDocumentID === documentID
      ? tabs[Math.min(index, tabs.length - 1)] : state.activeDocumentID, selection: null };
  }),
  setActiveDocument: (activeDocumentID) => set({ activeDocumentID, selection: null }),
  setSelection: (selection) => set({ selection }),
  beginSketch: (sketchPlane) => set({ sketchPlane, activeToolID: "select" }),
  endSketch: () => set({ sketchPlane: undefined, activeToolID: "select" }),
  setActiveTool: (activeToolID) => set({ activeToolID }),
  setNavigationProfile: (navigationProfile) => set({ navigationProfile }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", partialize: (state) => ({ tabs: state.tabs, inspectorTab: state.inspectorTab,
  navigationProfile: state.navigationProfile }) }));
