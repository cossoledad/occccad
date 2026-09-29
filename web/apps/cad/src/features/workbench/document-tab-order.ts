import type { OpenDocumentItem } from "./document-tabs";

export const documentTabOrderKey = "occccad.document-tab-order";

export function readDocumentTabOrder(storage: Pick<Storage, "getItem"> | undefined =
  typeof sessionStorage === "undefined" ? undefined : sessionStorage): string[] {
  try {
    const value: unknown = JSON.parse(storage?.getItem(documentTabOrderKey) ?? "[]");
    return Array.isArray(value) ? value.filter((id): id is string => typeof id === "string") : [];
  } catch { return []; }
}

export function stableDocumentTabOrder(saved: readonly string[], documents: readonly OpenDocumentItem[],
  productHostId?: string): string[] {
  const available = new Set(documents.map((document) => document.id));
  const result = [...new Set([...saved.filter((id) => available.has(id)), ...documents.map((document) => document.id)])];
  if (!productHostId || !available.has(productHostId)) return result;
  return [productHostId, ...result.filter((id) => id !== productHostId)];
}

export function moveDocumentTab(order: readonly string[], id: string, direction: number, productHostId?: string): string[] {
  if (id === productHostId) return [...order];
  const next = [...order], index = next.indexOf(id), target = index + direction;
  const firstMovable = productHostId ? 1 : 0;
  if (index < 0 || target < firstMovable || target >= next.length) return next;
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}
