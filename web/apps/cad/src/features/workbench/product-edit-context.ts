import type { DocumentStructureNode, DocumentView, ProductUpdatePlan } from "../../types";

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
  const dirty = new Set<string>();
  const children = new Map<string, Set<string>>();
  const visit = (node: DocumentStructureNode, owners: string[]) => {
    if (node.kind === "INSTANCE" && node.referenceMode === "PINNED") return;
    if (node.kind === "INSTANCE" && node.diagnostic?.startsWith("NOT_UPDATED")) {
      owners.forEach((owner) => dirty.add(owner));
      // Its new definition can contain followed children absent from this old
      // snapshot. Inspect the Product itself before accepting it into its owner.
      if (node.documentType === "PRODUCT" && node.documentId) dirty.add(node.documentId);
    }
    let nextOwners = owners;
    if (node.kind === "INSTANCE" && node.documentType === "PRODUCT" && node.documentId) {
      const parent = owners.at(-1);
      if (parent) {
        const dependencies = children.get(parent) ?? new Set<string>();
        dependencies.add(node.documentId);
        children.set(parent, dependencies);
      }
      nextOwners = [...owners, node.documentId];
    }
    node.children?.forEach((child) => visit(child, nextOwners));
  };
  if (root?.documentId) visit(root, [root.documentId]);
  const result: string[] = [];
  const visited = new Set<string>();
  const order = (id: string) => {
    if (visited.has(id)) return;
    visited.add(id);
    children.get(id)?.forEach(order);
    if (dirty.has(id)) result.push(id);
  };
  if (root?.documentId) order(root.documentId);
  return result;
}

type ProductUpdateClient = {
  getDocument(id: string): Promise<DocumentView>;
  getProductUpdatePlan(id: string): Promise<ProductUpdatePlan>;
  acceptProductUpdatePlan(id: string, digest: string): Promise<DocumentView>;
};

/** Re-read the root after each wave: a newly accepted Product may introduce
 * followed descendants which did not exist in the previous snapshot. */
export async function followProductUpdates(initial: DocumentView, api: ProductUpdateClient): Promise<DocumentView[]> {
  const updated = new Map<string, DocumentView>();
  const visiting = new Set<string>();
  const settle = async (initialView: DocumentView, depth: number): Promise<void> => {
    const id = initialView.document.id;
    if (depth > 32 || visiting.has(id)) throw new Error("Product 自动更新检测到循环引用或超出嵌套深度");
    visiting.add(id);
    try {
      let root = initialView;
      for (let wave = 0; wave < 32; wave += 1) {
        const targets = staleProductDocumentIDs(root.structureTree);
        if (!targets.length) return;
        for (const productID of targets) {
          if (productID !== id) {
            // Query the child's current definition, rather than only traversing
            // the old child revision frozen in the parent's scene tree.
            await settle(await api.getDocument(productID), depth + 1);
            continue;
          }
          const plan = await api.getProductUpdatePlan(productID);
          if (!plan.hasUpdates) continue;
          if (!plan.canAccept) throw new Error(plan.entries.find((entry) => entry.kind !== "ASSEMBLY_SOLVE" && entry.diagnostic)?.diagnostic ?? "Product 自动更新被上游求值阻塞");
          updated.set(productID, await api.acceptProductUpdatePlan(productID, plan.digest));
        }
        const latest = await api.getDocument(id);
        updated.set(id, latest);
        if (latest.document.versionId === root.document.versionId &&
          staleProductDocumentIDs(latest.structureTree).join("|") === targets.join("|")) return;
        root = latest;
      }
      throw new Error("Product 引用持续变化，请稍后重试自动更新");
    } finally {
      visiting.delete(id);
    }
  };
  await settle(initial, 0);
  return [...updated.values()];
}
