import type { ToolbarCatalogEntry, ToolbarCatalogItem } from "../../types";
import type { CadWorkbenchID } from "../../cad/workbench/cad-workbench";
import type { CadCommandState } from "../../cad/command/command-registry";

export type CommandSection = "model" | "view" | "document";
export type DiscoverableCommand = ToolbarCatalogItem & { toolbarName: string; state: CadCommandState };

// The catalog owns composition and order; the registry owns runtime availability.
export function contextualToolbars(catalog: readonly ToolbarCatalogEntry[], workbench: CadWorkbenchID): ToolbarCatalogEntry[] {
  return catalog.filter((toolbar) => toolbar.workbench === "ALL" || toolbar.workbench === workbench)
    .map((toolbar) => ({ ...toolbar, items: [...toolbar.items].sort((a, b) => a.sortOrder - b.sortOrder) }))
    .sort((a, b) => a.sortOrder - b.sortOrder);
}

export function commandSection(toolbar: ToolbarCatalogEntry): CommandSection {
  if (toolbar.workbench !== "ALL" || toolbar.position === "top-left") return "model";
  return toolbar.position === "top-right" ? "view" : "document";
}

export function searchCommands(toolbars: readonly ToolbarCatalogEntry[], query: string,
  stateOf: (id: string) => CadCommandState): DiscoverableCommand[] {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  const seen = new Set<string>();
  return toolbars.flatMap((toolbar) => toolbar.items.flatMap((item) => {
    const state = stateOf(item.commandId);
    const text = `${item.name} ${toolbar.name} ${item.helpText} ${item.commandId}`.toLocaleLowerCase();
    if (!state.visible || seen.has(item.commandId) || !terms.every((term) => text.includes(term))) return [];
    seen.add(item.commandId);
    return [{ ...item, toolbarName: toolbar.name, state }];
  }));
}
