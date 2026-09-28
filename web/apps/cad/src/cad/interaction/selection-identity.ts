import type { Selection, SelectionItem } from "../../types";

export const selectionKey = (selection: SelectionItem): string => JSON.stringify([
  selection.kind, selection.documentId ?? "",
  selection.kind === "body" ? selection.bodyId ?? selection.entityId ?? selection.id
    : selection.kind === "part" || selection.kind === "product" ? selection.documentId ?? selection.id
      : selection.kind === "instance" ? selection.instancePath?.canonical ?? selection.occurrencePath ?? selection.id
        : selection.kind === "sketch-constraint" || selection.kind === "assembly-constraint" ? selection.constraintId
        : "topologyId" in selection ? selection.topologyId
          : selection.kind === "tree" ? selection.id : selection.entityId ?? selection.id,
  selection.instancePath?.rootDocumentId ?? selection.rootDocumentId ?? "",
  selection.instancePath?.canonical ?? selection.occurrencePath ?? "",
  selection.versionId ?? "", selection.contextVariantKey ?? "",
  ["body", "sketch", "pad", "import", "feature", "visual", "face", "edge", "vertex"].includes(selection.kind)
    ? selection.bodyId ?? "" : "",
  selection.kind === "face" || selection.kind === "edge" || selection.kind === "vertex"
    ? selection.geometryKey ?? "" : "",
  "topologyId" in selection ? selection.topologyId : "",
  selection.kind === "axis" ? selection.axis : "",
  selection.kind === "visual" ? selection.featureId : "",
]);

export const selectionSetToken = (selections: readonly SelectionItem[]): string => selections.map(selectionKey).join("\u0000");

export function sameSelection(left: Selection, right: Selection): boolean {
  return left === right || Boolean(left && right && selectionKey(left) === selectionKey(right));
}

export function sameSelections(left: readonly SelectionItem[], right: readonly SelectionItem[]): boolean {
  return left.length === right.length && left.every((selection, index) => sameSelection(selection, right[index]));
}
