import type { DocumentStructureNode } from "../../types";

export function resultBodyFeatureTreeNode(
  root: DocumentStructureNode | undefined, bodyTreeNodeId: string,
): string | undefined {
  let result: string | undefined;
  const visit = (node: DocumentStructureNode | undefined) => {
    if (!node) return;
    if (node.id.startsWith(`${bodyTreeNodeId}/`) && (node.kind === "PAD" || node.kind === "IMPORT")) result = node.id;
    for (const child of node.children ?? []) visit(child);
  };
  visit(root);
  return result;
}
