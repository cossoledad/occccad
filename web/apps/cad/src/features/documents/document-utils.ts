import type { DocumentSummary, FolderSummary } from "../../types";

export type LibraryScope = "active" | "recent" | "shared" | "parts" | "products" | "trash";

export function defaultDocumentName(type: "PART" | "PRODUCT", documents: readonly Pick<DocumentSummary, "name">[]): string {
  const prefix = type === "PART" ? "Part" : "Product";
  const occupied = new Set(documents.map((document) => document.name.trim().toLocaleLowerCase()));
  let sequence = 1;
  while (occupied.has(`${prefix}${sequence}`.toLocaleLowerCase())) sequence++;
  return `${prefix}${sequence}`;
}

export function documentMetrics(documents: DocumentSummary[]) {
  const recentBoundary = Date.now() - 604_800_000;
  return {
    parts: documents.filter((item) => item.type === "PART").length,
    products: documents.filter((item) => item.type === "PRODUCT").length,
    recentlyUpdated: documents.filter((item) => new Date(item.lastUpdated).getTime() > recentBoundary).length,
  };
}

export function relativeDate(value: string): string {
  const date = new Date(value);
  const elapsed = Date.now() - date.getTime();
  if (elapsed < 60_000) return "刚刚";
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)} 分钟前`;
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)} 小时前`;
  if (elapsed < 604_800_000) return `${Math.floor(elapsed / 86_400_000)} 天前`;
  return date.toLocaleDateString();
}

export async function flattenFolderTree(
  loadChildren: (parentID: string) => Promise<FolderSummary[]>, parentID = "", prefix = "",
): Promise<Array<{ id: string; label: string }>> {
  const folders = await loadChildren(parentID);
  const result: Array<{ id: string; label: string }> = [];
  for (const folder of folders) {
    const label = `${prefix}${folder.name}`;
    result.push({ id: folder.id, label });
    result.push(...await flattenFolderTree(loadChildren, folder.id, `${label} / `));
  }
  return result;
}
