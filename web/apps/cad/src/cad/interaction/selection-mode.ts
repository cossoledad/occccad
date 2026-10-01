import type { Selection, SelectionItem } from "../../types";

export type SelectionModeID = "geometry" | "instance";

export type SelectionMode = {
  id: SelectionModeID;
  project(selection: Selection): Selection;
};

const instanceSelection = (selection: SelectionItem): Selection => {
  const direct = selection.instancePath?.segments[0];
  const instanceId = direct?.instanceId ?? selection.instanceId;
  if (!instanceId) return null;
  const instancePath = direct ? {
    rootDocumentId: selection.instancePath!.rootDocumentId,
    canonical: direct.instanceId, display: direct.instanceName, segments: [direct],
  } : undefined;
  return {
    kind: "instance",
    id: instanceId,
    instanceId,
    occurrencePath: instanceId,
    ...(instancePath ? { instancePath, rootDocumentId: instancePath.rootDocumentId,
      versionId: direct!.resolvedVersionId } : {}),
    visualKey: `occurrence:${instanceId}`,
    documentId: direct?.referencedDocumentId ?? selection.documentId,
  };
};

export const SELECTION_MODES: Record<SelectionModeID, SelectionMode> = {
  geometry: { id: "geometry", project: (selection) => selection },
  instance: { id: "instance", project: (selection) => selection ? instanceSelection(selection) : null },
};

export function selectionModeForTool(toolID: string): SelectionMode {
  return ["assembly.move", "assembly.fix", "assembly.rigid", "assembly.fix_together"].includes(toolID)
    ? SELECTION_MODES.instance : SELECTION_MODES.geometry;
}

const sketchNodeMarkers = ["/geometry/", "/external-geometry/", "/constraints/", "/logical-constraints/", "/dimensions/"];

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
    entityId: selection.featureId,
    documentId: selection.documentId,
    ownerDocumentId: selection.ownerDocumentId,
    bodyId: selection.bodyId,
    versionId: selection.versionId,
    contextVariantKey: selection.contextVariantKey,
    rootDocumentId: selection.rootDocumentId,
    occurrencePath: selection.occurrencePath,
    instancePath: selection.instancePath,
    instanceId: selection.instanceId,
    geometryKey: selection.geometryKey,
    treeNodeId: marker === undefined ? selection.treeNodeId : selection.treeNodeId?.slice(0, marker),
    expandTreeDescendants: true,
  };
}

export function sketchContextLayerVisibility(activeSketchID?: string, _editingOccurrence = false) {
  const editing = Boolean(activeSketchID);
  return {
    // The evaluated Body remains visible as read-only design context. Sketch
    // selection policy still limits interaction to the active sketch.
    body: true,
    lighting: true,
    environment: true,
    sketch: editing,
  };
}
