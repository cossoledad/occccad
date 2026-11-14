import type {IconDefinition} from "../command/workbench-catalog";

export type IconSource = {
  schemaVersion: number;
  symbols: Record<string, NonNullable<IconDefinition["elements"]>>;
  icons: Record<string, { family: string; parts?: string[]; elements?: IconDefinition["elements"]; resource?: string }>;
};
const families = new Set(["view", "solid", "surface", "curve", "sketch", "constraint", "assembly", "reference", "status"]);

/** Expand bounded, non-recursive authoring parts into the existing wire format. */
export function compileIconCatalog(source: IconSource): Record<string, IconDefinition> {
  if (source.schemaVersion !== 1) throw new Error("Unsupported icon source");
  return Object.fromEntries(Object.entries(source.icons).map(([id, icon]) => {
    if (!id || !families.has(icon.family) || (icon.parts?.length ?? 0) > 8) throw new Error(`Invalid icon composition: ${id}`);
    const elements = (icon.parts ?? []).flatMap(part => {
      const symbol = source.symbols[part];
      if (!symbol) throw new Error(`Missing icon symbol: ${part}`);
      return symbol;
    }).concat(icon.elements ?? []);
    if (!elements.length && !icon.resource) throw new Error(`Empty icon: ${id}`);
    return [id, {className: `cad-command-icon family-${icon.family}`, elements, ...(icon.resource ? {resource: icon.resource} : {})}];
  }));
}
