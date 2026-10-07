import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Selection, SelectionItem, SketchPlane } from "../types";

export type WorkbenchToolID = "select" | "sketch.project" | "sketch.point" | "sketch.line" | "sketch.circle" | "sketch.arc" | "sketch.polyline" | "sketch.spline" | "sketch.spline.control" | "sketch.rectangle" | "sketch.polygon" | "sketch.polygon.circumscribed" | "sketch.ellipse" | "sketch.elliptical_arc" | "sketch.circle.three_point" | "sketch.arc.three_point" | "sketch.rectangle.center" | "sketch.rectangle.oriented"
	| `assembly.${"move"|"fix"|"rigid"|"fix_together"|"contact"|"coincident"|"concentric"|"angle"|"parallel"|"perpendicular"|"distance"}`
  | `sketch.edit.${"delete"|"copy"|"move"|"rotate"|"mirror"|"split"|"trim"|"fillet"|"chamfer"|"extend"|"complement"|"close"|"offset"|"spline_insert"|"spline_delete"|"spline_close"|"spline_control"|"construction"}`
  | "sketch.dimension.linear"
  | `sketch.constraint.${"coincident"|"parallel"|"collinear"|"fixed"|"horizontal"|"vertical"|"perpendicular"|"tangent"|"equal"|"distance"|"horizontal_distance"|"vertical_distance"|"length"|"radius"|"diameter"|"major_radius"|"minor_radius"|"angle"|"concentric"|"point_on_object"|"midpoint"|"symmetry"}`;
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
  beginSketch: (activeSketchID, sketchPlane) => set({ activeSketchID, sketchPlane, activeToolID: "select", activeToolMode: "once", selection: null, selections: [], preselection: null }),
  endSketch: () => set({ activeSketchID: undefined, sketchPlane: undefined, activeToolID: "select", activeToolMode: "once" }),
  setActiveTool: (activeToolID, activeToolMode) => set((state) => ({ activeToolID,
    activeToolMode: activeToolMode ?? (state.activeToolID === activeToolID ? state.activeToolMode : "once") })),
  completeToolUse: () => set((state) => state.activeToolMode === "continuous" ? state : { activeToolID: "select", activeToolMode: "once" }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
}), { name: "occccad.workbench", partialize: (state) => ({ inspectorTab: state.inspectorTab }) }));
