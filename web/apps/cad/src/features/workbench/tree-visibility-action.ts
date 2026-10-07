import { canToggleNodeVisibility } from "./tree-node-descriptors";
import type { EditSession } from "./edit-session";
import type { SpecificationTreeNode } from "./specification-tree";

export function treeVisibilityAction(node: SpecificationTreeNode, session?: EditSession) {
  if (!canToggleNodeVisibility(node.kind)) return undefined;
  const occurrence = node.instancePath?.canonical ?? "";
  const isEditingDefinition = node.documentId === session?.editTarget.documentId &&
    occurrence === (session?.editTarget.instancePath?.canonical ?? "");
  if (node.kind === "INSTANCE" || occurrence && !isEditingDefinition && ["BODY", "SKETCH", "SKETCH_ENTITY"].includes(node.kind ?? "")) {
    const visible = node.visibilityMode === "SHOW" || node.visibilityMode !== "HIDE" && node.localVisible !== false;
    return { scope: "OCCURRENCE" as const, label: visible ? "hide" : "show", mode: visible ? "HIDE" as const : "SHOW" as const };
  }
  if (!occurrence || isEditingDefinition) return {
    scope: "DEFINITION" as const, label: (node.definitionVisible ?? node.localVisible) === false ? "show" : "hide", mode: undefined,
  };
  return undefined;
}
