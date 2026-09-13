import type { DocumentStructureNode } from "../../types";

// A follow mode belongs to an occurrence edge. A PINNED edge freezes the whole
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

/** Find Products that must accept a changed descendant Head, ordered leaf to root. */
export function staleProductDocumentIDs(root?: DocumentStructureNode): string[] {
  const result: string[] = [];
  const visit = (node: DocumentStructureNode, owners: string[]) => {
    if (node.kind === "INSTANCE" && node.referenceMode === "PINNED") return;
    if (node.kind === "INSTANCE" && node.diagnostic?.startsWith("NOT_UPDATED")) {
      for (const owner of [...owners].reverse()) if (owner && !result.includes(owner)) result.push(owner);
    }
    const nextOwners = node.kind === "INSTANCE" && node.documentType === "PRODUCT" && node.documentId
      ? [...owners, node.documentId] : owners;
    node.children?.forEach((child) => visit(child, nextOwners));
  };
  if (root?.documentId) visit(root, [root.documentId]);
  return result;
}
