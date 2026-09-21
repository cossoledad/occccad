import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Selection, SelectionItem, SketchPlane } from "../types";

export type WorkbenchToolID = "select" | "sketch.project" | "sketch.point" | "sketch.line" | "sketch.circle" | "sketch.arc" | "sketch.polyline" | "sketch.spline" | "sketch.rectangle" | "sketch.polygon" | "sketch.slot"
	| `assembly.${"move"|"fix"|"rigid"|"coincident"|"concentric"|"angle"|"parallel"|"perpendicular"|"distance"}`
  | "sketch.dimension.linear"
  | `sketch.constraint.${"coincident"|"parallel"|"fixed"|"horizontal"|"vertical"|"perpendicular"|"tangent"|"equal"|"distance"|"length"|"radius"|"diameter"|"angle"|"concentric"|"point_on_object"|"midpoint"|"symmetry"}`;
export type WorkbenchToolMode = "once" | "continuous";

type WorkbenchState = {
  selection: Selection;
  selections: SelectionItem[];
  preselection: Selection;
  sketchPlane?: SketchPlane;
  activeSketchID?: string;
  activeToolID: WorkbenchToolID;
  activeToolMode: WorkbenchToolMode;
  inspectorTab: "properties" | "history";
  setSelection: (selection: Selection) => void;
  setSelections: (selections: SelectionItem[]) => void;
  setPreselection: (selection: Selection) => void;
  beginSketch: (sketchID: string, plane: SketchPlane) => void;
  endSketch: () => void;
  setActiveTool: (tool: WorkbenchToolID, mode?: WorkbenchToolMode) => void;
  completeToolUse: () => void;
  setInspectorTab: (tab: "properties" | "history") => void;
};

export const useWorkbenchStore = create<WorkbenchState>()(persist((set) => ({
  selection: null, selections: [], preselection: null, activeToolID: "select", activeToolMode: "once", inspectorTab: "properties",
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
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", partialize: (state) => ({ inspectorTab: state.inspectorTab }) }));
