import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { PlaneName, Selection } from "../types";

type WorkbenchState = {
  tabs: string[];
  activeDocumentID?: string;
  selection: Selection;
  sketchPlane?: PlaneName;
  sketchTool: "SELECT" | "RECTANGLE";
  inspectorTab: "properties" | "history";
  openTab: (documentID: string) => void;
  closeTab: (documentID: string) => void;
  setActiveDocument: (documentID?: string) => void;
  setSelection: (selection: Selection) => void;
  beginSketch: (plane: PlaneName) => void;
  endSketch: () => void;
  setSketchTool: (tool: "SELECT" | "RECTANGLE") => void;
  setInspectorTab: (tab: "properties" | "history") => void;
};

export const useWorkbenchStore = create<WorkbenchState>()(persist((set) => ({
  tabs: [], selection: null, sketchTool: "SELECT", inspectorTab: "properties",
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
  beginSketch: (sketchPlane) => set({ sketchPlane, sketchTool: "SELECT" }),
  endSketch: () => set({ sketchPlane: undefined, sketchTool: "SELECT" }),
  setSketchTool: (sketchTool) => set({ sketchTool }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", partialize: (state) => ({ tabs: state.tabs, inspectorTab: state.inspectorTab }) }));
