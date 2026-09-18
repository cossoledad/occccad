import { create } from "zustand";
import { persist } from "zustand/middleware";
import { migrateTreeVisibilityOverrides, type TreeVisibilityOverrides } from "../cad/interaction/tree-visibility";

export type ToolbarOrientation = "horizontal" | "vertical";
export type ToolbarLayout = { x?: number; y?: number; orientation: ToolbarOrientation };

type UIPreferences = {
  inspectorOpen: boolean;
  toolbarLayouts: Record<string, ToolbarLayout>;
  treeVisibilityOverrides: TreeVisibilityOverrides;
  setInspectorOpen: (open: boolean) => void;
  setToolbarLayout: (id: string, layout: ToolbarLayout) => void;
  setTreeVisibility: (key: string, visible: boolean) => void;
};

export function normalizeToolbarLayout(value: unknown, fallback: ToolbarOrientation): ToolbarLayout {
  if (!value || typeof value !== "object") return { orientation: fallback };
  const candidate = value as Partial<ToolbarLayout>;
  const orientation = candidate.orientation === "vertical" || candidate.orientation === "horizontal"
    ? candidate.orientation : fallback;
  const x = Number.isFinite(candidate.x) ? candidate.x : undefined;
  const y = Number.isFinite(candidate.y) ? candidate.y : undefined;
  return { orientation, ...(x === undefined ? {} : { x }), ...(y === undefined ? {} : { y }) };
}

export const useUIPreferences = create<UIPreferences>()(persist((set) => ({
  inspectorOpen: true,
  toolbarLayouts: {},
  treeVisibilityOverrides: {},
  setInspectorOpen: (inspectorOpen) => set({ inspectorOpen }),
  setToolbarLayout: (id, layout) => set((state) => ({
    toolbarLayouts: { ...state.toolbarLayouts, [id]: normalizeToolbarLayout(layout, "horizontal") },
  })),
  setTreeVisibility: (key, visible) => set((state) => ({
    treeVisibilityOverrides: { ...state.treeVisibilityOverrides, [key]: visible },
  })),
}), {
  name: "occccad.ui-preferences.v1",
  version: 2,
  migrate: (persisted) => {
    const value = (persisted && typeof persisted === "object" ? persisted : {}) as Partial<UIPreferences> & { hiddenTreeKeys?: string[] };
    return { ...value, treeVisibilityOverrides: migrateTreeVisibilityOverrides(value) } as UIPreferences;
  },
  partialize: (state) => ({ inspectorOpen: state.inspectorOpen, toolbarLayouts: state.toolbarLayouts,
    treeVisibilityOverrides: state.treeVisibilityOverrides }),
}));
