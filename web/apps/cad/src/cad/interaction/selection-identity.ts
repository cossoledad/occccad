import type { Selection, SelectionItem } from "../../types";

export const selectionKey = (selection: SelectionItem): string =>
  `${selection.kind}:${selection.id}:${selection.expandTreeDescendants ? "tree" : "exact"}`;

export const selectionSetToken = (selections: readonly SelectionItem[]): string => selections.map(selectionKey).join("\u0000");

export function sameSelection(left: Selection, right: Selection): boolean {
  return left === right || Boolean(left && right && left.kind === right.kind && left.id === right.id &&
    Boolean(left.expandTreeDescendants) === Boolean(right.expandTreeDescendants));
}

export function sameSelections(left: readonly SelectionItem[], right: readonly SelectionItem[]): boolean {
  return left.length === right.length && left.every((selection, index) => sameSelection(selection, right[index]));
}
