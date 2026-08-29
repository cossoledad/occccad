import { create } from "zustand";
import { persist } from "zustand/middleware";

export type ToolbarOrientation = "horizontal" | "vertical";
export type ToolbarLayout = { x?: number; y?: number; orientation: ToolbarOrientation };

type UIPreferences = {
  inspectorOpen: boolean;
  toolbarLayouts: Record<string, ToolbarLayout>;
  hiddenTreeKeys: string[];
  setInspectorOpen: (open: boolean) => void;
  setToolbarLayout: (id: string, layout: ToolbarLayout) => void;
  toggleTreeVisibility: (key: string) => void;
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
  hiddenTreeKeys: [],
  setInspectorOpen: (inspectorOpen) => set({ inspectorOpen }),
  setToolbarLayout: (id, layout) => set((state) => ({
    toolbarLayouts: { ...state.toolbarLayouts, [id]: normalizeToolbarLayout(layout, "horizontal") },
  })),
  toggleTreeVisibility: (key) => set((state) => ({ hiddenTreeKeys: state.hiddenTreeKeys.includes(key)
    ? state.hiddenTreeKeys.filter((item) => item !== key) : [...state.hiddenTreeKeys, key] })),
}), {
  name: "occccad.ui-preferences.v1",
  version: 1,
  partialize: (state) => ({ inspectorOpen: state.inspectorOpen, toolbarLayouts: state.toolbarLayouts, hiddenTreeKeys: state.hiddenTreeKeys }),
}));
