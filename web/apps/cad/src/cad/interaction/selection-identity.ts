import type { Selection, SelectionItem } from "../../types";

export const selectionKey = (selection: SelectionItem): string => {
 const path=selection.instancePath??selection.occurrenceRef?.instancePath;
 const occurrence=path?.canonical||(path?.segments.length?JSON.stringify(path.segments.map(segment=>segment.instanceId)):undefined)||selection.occurrencePath||"";
 return JSON.stringify([
  selection.kind, selection.entityRef?.documentId ?? selection.ownerDocumentId ?? selection.documentId ?? "",
  selection.kind === "boolean-region" ? [selection.analysisId,selection.regionId] : selection.kind === "profile-region" ? [selection.featureId,selection.regionId] : selection.kind === "body" ? selection.bodyId ?? selection.entityId ?? selection.id
    : selection.kind === "part" || selection.kind === "product" ? selection.documentId ?? selection.id
      : selection.kind === "instance" ? occurrence || selection.id
        : selection.kind === "sketch-constraint" || selection.kind === "assembly-constraint" ? selection.constraintId
        : "topologyId" in selection ? selection.topologyId
          : selection.kind === "tree" ? selection.id : selection.entityId ?? selection.id,
  path?.rootDocumentId ?? selection.rootDocumentId ?? "",
  occurrence,
  selection.snapshotScope?.revisionId ?? selection.versionId ?? "", selection.snapshotScope?.contextVariantKey ?? selection.contextVariantKey ?? "",
  ["profile-region", "body", "sketch", "pad", "import", "feature", "visual", "face", "edge", "vertex"].includes(selection.kind)
    ? selection.bodyId ?? "" : "",
  selection.kind === "face" || selection.kind === "edge" || selection.kind === "vertex"
    ? selection.geometryKey ?? "" : "",
  "topologyId" in selection ? selection.topologyId : "",
  selection.displayStageFeatureId??"",
  selection.kind === "axis" ? selection.axis : "",
  selection.kind === "visual" ? selection.featureId : "",
  selection.sketchReference ? [selection.sketchReference.target,selection.sketchReference.entityId,selection.sketchReference.subElement,selection.sketchReference.controlPointId??selection.sketchReference.controlPointIndex,selection.sketchReference.pointId] : null,
]);
};

export const selectionSetToken = (selections: readonly SelectionItem[]): string => selections.map(selectionKey).join("\u0000");

export function sameSelection(left: Selection, right: Selection): boolean {
  return left === right || Boolean(left && right && selectionKey(left) === selectionKey(right));
}

export function sameSelections(left: readonly SelectionItem[], right: readonly SelectionItem[]): boolean {
  return left.length === right.length && left.every((selection, index) => sameSelection(selection, right[index]));
}
