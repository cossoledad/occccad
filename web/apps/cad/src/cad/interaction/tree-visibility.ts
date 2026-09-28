export type TreeVisibilityOverrides = Record<string, boolean>;

export function treeVisibilityOverride(key: string | undefined, overrides: TreeVisibilityOverrides): boolean | undefined {
  return key === undefined ? undefined : overrides[key];
}

export function sketchTreeVisible(input: {
  featureID: string;
  treeKey?: string;
  activeSketchID?: string;
  defaultVisible: boolean;
  selected?: boolean;
  overrides: TreeVisibilityOverrides;
}): boolean {
  if (input.activeSketchID) return input.featureID === input.activeSketchID;
  return treeVisibilityOverride(input.treeKey, input.overrides) ?? (input.selected === true || input.defaultVisible);
}

export function migrateTreeVisibilityOverrides(value: unknown): TreeVisibilityOverrides {
  if (!value || typeof value !== "object") return {};
  const persisted = value as { treeVisibilityOverrides?: unknown; hiddenTreeKeys?: unknown };
  if (persisted.treeVisibilityOverrides && typeof persisted.treeVisibilityOverrides === "object") {
    return Object.fromEntries(Object.entries(persisted.treeVisibilityOverrides as Record<string, unknown>)
      .filter((entry): entry is [string, boolean] => typeof entry[1] === "boolean"));
  }
  return Array.isArray(persisted.hiddenTreeKeys)
    ? Object.fromEntries(persisted.hiddenTreeKeys.filter((key): key is string => typeof key === "string").map((key) => [key, false]))
    : {};
}
