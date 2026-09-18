export type TreeVisibilityOverrides = Record<string, boolean>;

export function treeVisibilityOverride(key: string | undefined, overrides: TreeVisibilityOverrides): boolean | undefined {
  if (!key) return undefined;
  let selected: string | undefined;
  for (const candidate of Object.keys(overrides)) {
    if ((key === candidate || key.startsWith(`${candidate}/`)) && (!selected || candidate.length > selected.length)) selected = candidate;
  }
  return selected === undefined ? undefined : overrides[selected];
}

export function sketchTreeVisible(input: {
  featureID: string;
  treeKey?: string;
  activeSketchID?: string;
  defaultVisible: boolean;
  overrides: TreeVisibilityOverrides;
}): boolean {
  if (input.activeSketchID) return input.featureID === input.activeSketchID;
  return treeVisibilityOverride(input.treeKey, input.overrides) ?? input.defaultVisible;
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
