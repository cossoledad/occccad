import {nextObjectName} from "../../utils/automatic-name";
import type { DocumentSummary } from "../../types";

export type LibraryScope = "active" | "recent" | "shared" | "parts" | "products" | "trash";

export function defaultDocumentName(type: "PART" | "PRODUCT", documents: readonly Pick<DocumentSummary, "name">[]): string {
  const prefix = type === "PART" ? "Part" : "Product";
  return nextObjectName(prefix,documents.map(document=>document.name));
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
