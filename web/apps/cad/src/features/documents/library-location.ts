import type { LibraryScope } from "./document-utils";

export type LibraryLocation = {
  scope: LibraryScope; currentFolderID: string; search: string; type: string;
  sort: "updated" | "name" | "created"; offset: number; pageSize: number; viewMode: "grid" | "list";
};
const defaults: LibraryLocation = { scope: "active", currentFolderID: "", search: "", type: "",
  sort: "updated", offset: 0, pageSize: 24, viewMode: "grid" };
const fields = { scope: "scope", currentFolderID: "folder", search: "q", type: "type", sort: "sort",
  offset: "offset", pageSize: "size", viewMode: "view" } as const;
export function readLibraryLocation(search: string): LibraryLocation {
  const p = new URLSearchParams(search);
  const scope = p.get("scope") ?? "active", size = Number(p.get("size") ?? 24), offset = Number(p.get("offset") ?? 0);
  return {
    scope: ["active", "recent", "shared", "parts", "products", "trash"].includes(scope) ? scope as LibraryScope : "active",
    currentFolderID: p.get("folder") ?? "", search: (p.get("q") ?? "").trim(),
    type: ["PART", "PRODUCT"].includes(p.get("type") ?? "") ? p.get("type")! : "",
    sort: p.get("sort") === "name" ? "name" : p.get("sort") === "created" ? "created" : "updated",
    offset: Number.isSafeInteger(offset) && offset >= 0 ? offset : 0,
    pageSize: [24, 50, 100].includes(size) ? size : 24, viewMode: p.get("view") === "list" ? "list" : "grid",
  };
}
export function writeLibraryLocation(search: string, patch: Partial<LibraryLocation>): string {
  const p = new URLSearchParams(search), next = { ...readLibraryLocation(search), ...patch };
  for (const key of Object.keys(fields) as (keyof LibraryLocation)[]) {
    if (next[key] === defaults[key]) p.delete(fields[key]); else p.set(fields[key], String(next[key]));
  }
  return p.size ? `?${p}` : "";
}
