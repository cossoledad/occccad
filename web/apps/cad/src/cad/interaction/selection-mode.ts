import type { Selection, SelectionItem } from "../../types";

export type SelectionModeID = "geometry" | "instance";

export type SelectionMode = {
  id: SelectionModeID;
  project(selection: Selection): Selection;
};

const instanceSelection = (selection: SelectionItem): Selection => {
  if (!selection.instanceId) return null;
  return {
    kind: "instance",
    id: selection.instanceId,
    instanceId: selection.instanceId,
    occurrencePath: selection.instanceId,
    visualKey: `occurrence:${selection.instanceId}`,
    documentId: selection.documentId,
  };
};

export const SELECTION_MODES: Record<SelectionModeID, SelectionMode> = {
  geometry: { id: "geometry", project: (selection) => selection },
  instance: { id: "instance", project: (selection) => selection ? instanceSelection(selection) : null },
};

export function selectionModeForTool(toolID: string): SelectionMode {
  return toolID === "assembly.move" ? SELECTION_MODES.instance : SELECTION_MODES.geometry;
}
