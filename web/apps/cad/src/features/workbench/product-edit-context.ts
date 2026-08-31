import type { DocumentStructureNode } from "../../types";

// FOLLOW_HEAD belongs to an occurrence edge. A PINNED edge freezes the whole
// referenced subtree as projected by that Product revision.
export function followedDocumentIDs(root?: DocumentStructureNode): string[] {
  const result = new Set<string>();
  const visit = (node: DocumentStructureNode, follows: boolean) => {
    const nextFollows = follows && (node.kind !== "INSTANCE" || node.referenceMode !== "PINNED");
    if (nextFollows && node.kind === "INSTANCE" && node.documentId) result.add(node.documentId);
    if (nextFollows) node.children?.forEach((child) => visit(child, nextFollows));
  };
  if (root) visit(root, true);
  return [...result];
}
