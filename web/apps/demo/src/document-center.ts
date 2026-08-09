import type { DocumentSummary, FolderSummary } from "./types";

export type LibraryScope = "active" | "recent" | "shared" | "parts" | "products" | "trash";

export type DocumentMetrics = { parts: number; products: number; recentlyUpdated: number };

export function documentMetrics(documents: DocumentSummary[]): DocumentMetrics {
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

export function pageLabel(offset: number, limit: number, total: number): string {
  const first = total === 0 ? 0 : offset + 1;
  const last = Math.min(offset + limit, total);
  return `${first}–${last} / ${total} 个文档`;
}

export async function flattenFolderTree(
  loadChildren: (parentId: string) => Promise<FolderSummary[]>, parentId = "", prefix = "",
): Promise<Array<{ id: string; label: string }>> {
  const folders = await loadChildren(parentId);
  const result: Array<{ id: string; label: string }> = [];
  for (const folder of folders) {
    const label = `${prefix}${folder.name}`;
    result.push({ id: folder.id, label });
    result.push(...await flattenFolderTree(loadChildren, folder.id, `${label} / `));
  }
  return result;
}
