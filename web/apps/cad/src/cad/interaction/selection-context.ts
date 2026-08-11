import type { Selection } from "../../types";

export type SelectionContext = { selection: Selection; documentType: "PART" | "PRODUCT" };

export interface SelectionContextResolver {
  resolve(context: SelectionContext): string[];
}

// ContextToolbar consumes command IDs from this boundary. It never inspects the
// Three.js scene; richer face/edge rules can be added when topology picking exists.
export class EmptySelectionContextResolver implements SelectionContextResolver {
  resolve(): string[] { return []; }
}
