import type { DocumentView, ProductUpdatePlan } from "../../types";

/** Server projection of non-PINNED domain reference edges, including nested Products. */
export function followedDocumentIDs(view?: DocumentView): string[] {
  if (!view) return [];
  return view.followedDocumentIds ?? (view.product?.instances ?? [])
    .filter((instance) => instance.referenceMode !== "PINNED")
    .map((instance) => instance.documentId);
}

/** A hint for scheduling; the UpdatePlan remains the authority for acceptance. */
export function staleProductDocumentIDs(view?: DocumentView): string[] {
  return view?.product?.instances.some((instance) => instance.referenceMode !== "PINNED" && instance.headChanged)
    ? [view.document.id] : [];
}

type ProductUpdateClient = {
  getDocument(id: string): Promise<DocumentView>;
  getProductUpdatePlan(id: string): Promise<ProductUpdatePlan>;
  acceptProductUpdatePlan(id: string, digest: string): Promise<DocumentView>;
};

/** Product IDs arrive in postorder from the server's reference graph. Each
 * digest is fetched after descendants have settled, then the root is reread. */
export async function followProductUpdates(initial: DocumentView, api: ProductUpdateClient): Promise<DocumentView[]> {
  const updated = new Map<string, DocumentView>();
  let root = initial;
  for (let wave = 0; wave < 32; wave += 1) {
    let changed = false;
    const productIDs = [...new Set([...(root.followedProductIds ?? []), root.document.id])];
    for (const productID of productIDs) {
      const plan = await api.getProductUpdatePlan(productID);
      if (!plan.hasUpdates) continue;
      if (!plan.canAccept) throw new Error(plan.entries.find((entry) =>
        entry.kind !== "ASSEMBLY_SOLVE" && entry.diagnostic)?.diagnostic ?? "Product 自动更新被上游求值阻塞");
      updated.set(productID, await api.acceptProductUpdatePlan(productID, plan.digest));
      changed = true;
    }
    if (!changed) return [...updated.values()];
    root = await api.getDocument(root.document.id);
    updated.set(root.document.id, root);
  }
  throw new Error("Product 引用持续变化，请稍后重试自动更新");
}
