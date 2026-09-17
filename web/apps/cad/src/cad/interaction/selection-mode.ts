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
  return ["assembly.move", "assembly.fix", "assembly.rigid"].includes(toolID)
    ? SELECTION_MODES.instance : SELECTION_MODES.geometry;
}

const sketchNodeMarkers = ["/geometry/", "/constraints/", "/logical-constraints/", "/dimensions/"];

/** Outside Sketcher, sketch sub-elements are implementation details of one selectable feature. */
export function projectSketchFeatureSelection(selection: Selection, activeSketchID?: string): Selection {
  if (!selection || activeSketchID || (selection.kind !== "visual" && selection.kind !== "sketch-constraint")) {
    return selection;
  }
  const marker = sketchNodeMarkers
    .map((candidate) => selection.treeNodeId?.indexOf(candidate) ?? -1)
    .find((index) => index >= 0);
  return {
    kind: "sketch",
    id: selection.featureId,
    documentId: selection.documentId,
    occurrencePath: selection.occurrencePath,
    instancePath: selection.instancePath,
    instanceId: selection.instanceId,
    geometryKey: selection.geometryKey,
    treeNodeId: marker === undefined ? selection.treeNodeId : selection.treeNodeId?.slice(0, marker),
    expandTreeDescendants: true,
  };
}

export function sketchOverlayVisible(featureID: string, activeSketchID: string | undefined, visibleOutsideSketchEdit: boolean): boolean {
  return activeSketchID ? featureID === activeSketchID : visibleOutsideSketchEdit;
}

export function sketchContextLayerVisibility(activeSketchID?: string, editingOccurrence = false) {
  const editing = Boolean(activeSketchID);
  return {
    // The evaluated Body remains visible as read-only design context. Sketch
    // selection policy still limits interaction to the active sketch.
    body: true,
    environment: !editing || editingOccurrence,
    sketch: editing,
  };
}
