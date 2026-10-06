import type { DocumentStructureNode, DocumentView } from "../../types";

export const DISPLAY_KINDS = ["INSTANCE","PART","BODY","SKETCH","SKETCH_ENTITY","ORIGIN","PLANE","AXIS_SYSTEM","AXIS","DATUM_AXIS","DATUM_POINT","ASSEMBLY_CONSTRAINT"] as const;
export type DisplayKind = typeof DISPLAY_KINDS[number];
export type DisplayAddress = { documentId: string; occurrencePath: string; kind: DisplayKind; entityId: string; axis?: string; ownerEntityId?: string; bodyId?: string };
export type EffectiveVisibility = { localVisible: boolean; effectiveVisible: boolean; blockedBy?: DisplayAddress; mode: "INHERIT" | "SHOW" | "HIDE"; displayEligible: boolean };

type Entry = { address: DisplayAddress; localVisible: boolean; mode: "INHERIT" | "SHOW" | "HIDE"; eligible: boolean };
export type EditingSketchScope = { id: string; occurrencePath: string; ids?:string[]; documentId?:string };
const key = (address: Pick<DisplayAddress, "documentId" | "occurrencePath" | "kind" | "entityId" | "axis">) =>
  [address.documentId, address.occurrencePath, address.kind, address.entityId, address.kind==="AXIS"?address.axis??"":""].join("\u0000");

// Display ancestry is semantic: Sketch belongs to Body even when the tree
// presents it below its consuming Pad. Publication and input references do not
// create display parentage.
export class VisibilityResolver {
  private readonly entries = new Map<string, Entry>();
  private readonly instances = new Map<string, DisplayAddress>();
  private readonly sketches = new Map<string, DisplayAddress>();
  constructor(root?: DocumentStructureNode) {
    if (root) this.index(root);
  }

  private index(node: DocumentStructureNode): void {
    const kind = ["SKETCH_INPUT_REFERENCE","SKETCH_PATTERN_MEMBER"].includes(node.kind) ? "SKETCH" : node.kind==="SKETCH_PATTERN_ENTITY"?"SKETCH_ENTITY":node.kind;
    if (((DISPLAY_KINDS as readonly string[]).includes(kind) && node.entityId) ||
        (kind === "PART" && node.documentId)) {
      const address: DisplayAddress = {
        documentId: node.subject?.documentId ?? node.documentId ?? "",
        occurrencePath: node.instancePath?.canonical ?? "",
        kind: kind as DisplayKind, entityId: node.subject?.entityId ?? node.entityId ?? node.documentId ?? "",
        axis:node.axis, ownerEntityId: node.ownerEntityId, bodyId: node.bodyId,
      };
      if (address.documentId && address.entityId) {
        const id = key(address);
        if (kind === "INSTANCE") this.instances.set(address.occurrencePath, address);
        if (kind === "SKETCH" && node.kind !== "SKETCH_INPUT_REFERENCE") this.sketches.set(`${address.entityId}\u0000${address.occurrencePath}`, address);
        const prior = this.entries.get(id);
        // The Sketch definition carries local state; input references share it.
        if (!prior || node.kind !== "SKETCH_INPUT_REFERENCE") this.entries.set(id, {
          address, localVisible: node.localVisible ?? prior?.localVisible ?? true,
          mode: node.visibilityMode ?? prior?.mode ?? "INHERIT", eligible: !node.suppressed,
        });
      }
    }
    for (const child of node.children ?? []) this.index(child);
  }

  resolve(address: DisplayAddress, editingSketch?: EditingSketchScope): EffectiveVisibility {
    return this.resolveInner(address, editingSketch, new Set());
  }

  private resolveInner(address: DisplayAddress, editingSketch: EditingSketchScope | undefined, visited: Set<string>): EffectiveVisibility {
    const id = key(address);
    if (visited.has(id)) return {localVisible: false, effectiveVisible: false, mode: "INHERIT", displayEligible: false};
    visited.add(id);
    const entry = this.entries.get(id);
    const mode = entry?.mode ?? "INHERIT";
    let localVisible = mode === "SHOW" ? true : mode === "HIDE" ? false : entry?.localVisible ?? true;
    const displayEligible = entry?.eligible ?? true;
    const editingAddresses = editingSketch ? (editingSketch.ids??[editingSketch.id]).map(id=>this.sketches.get(`${id}\u0000${editingSketch.occurrencePath}`)).filter((entry):entry is DisplayAddress=>!!entry&&(!editingSketch.documentId||entry.documentId===editingSketch.documentId)) : [];
    const editReveal = editingAddresses.some(editingAddress=>{
      const sameDefinition = editingAddress.documentId === address.documentId && editingAddress.occurrencePath === address.occurrencePath;
      return sameDefinition && (address.kind === "SKETCH" && address.entityId === editingAddress.entityId ||
        address.kind === "BODY" && address.entityId === editingAddress.bodyId || address.kind === "PART") ||
        address.kind === "INSTANCE" && editingAddress.occurrencePath &&
        (editingAddress.occurrencePath === address.occurrencePath || editingAddress.occurrencePath.startsWith(`${address.occurrencePath}/`));
    });
    if (editReveal) localVisible = true;
    const parent = this.parent(entry?.address ?? address);
    const ancestor = parent ? this.resolveInner(parent, editingSketch, visited) : undefined;
    const effectiveVisible = localVisible && displayEligible && (ancestor?.effectiveVisible ?? true);
    return { localVisible, effectiveVisible, mode, displayEligible,
      blockedBy: !ancestor?.effectiveVisible ? ancestor?.blockedBy ?? parent : !localVisible || !displayEligible ? address : undefined };
  }

  private parent(address: DisplayAddress): DisplayAddress | undefined {
    const common = {documentId: address.documentId, occurrencePath: address.occurrencePath};
    if(["AXIS","DATUM_POINT"].includes(address.kind))return {...common,kind:"AXIS_SYSTEM",entityId:address.ownerEntityId??address.entityId};
    if(["PLANE","DATUM_AXIS","AXIS_SYSTEM"].includes(address.kind))return {...common,kind:"ORIGIN",entityId:"origin"};
    if(address.kind==="ORIGIN")return {...common,kind:"PART",entityId:address.documentId};
    if(address.kind==="ASSEMBLY_CONSTRAINT"&&address.occurrencePath)return this.instances.get(address.occurrencePath);
    if (address.kind === "SKETCH_ENTITY" && address.ownerEntityId) return {...common, kind: "SKETCH", entityId: address.ownerEntityId, bodyId: address.bodyId};
    if (address.kind === "SKETCH" && address.ownerEntityId) return {...common,kind:"SKETCH",entityId:address.ownerEntityId,bodyId:address.bodyId};
    if (address.kind === "SKETCH" && address.bodyId) return {...common, kind: "BODY", entityId: address.bodyId};
    if (address.kind === "BODY") return {...common, kind: "PART", entityId: address.documentId};
    if (address.kind === "PART" && address.occurrencePath) {
      return this.instances.get(address.occurrencePath);
    }
    if (address.kind === "INSTANCE" && address.occurrencePath.includes("/")) {
      const parentPath = address.occurrencePath.split("/").slice(0, -1).join("/");
      return this.instances.get(parentPath);
    }
    return undefined;
  }

  resolveNode(node: DocumentStructureNode, editingSketch?: EditingSketchScope): EffectiveVisibility | undefined {
    const kind = node.subject?.entityKind ?? node.kind;
    if (!(DISPLAY_KINDS as readonly string[]).includes(kind)) return undefined;
    return this.resolve({documentId: node.subject?.documentId ?? node.documentId ?? "",
      occurrencePath: node.instancePath?.canonical ?? "", kind: kind as DisplayKind,
      entityId: node.subject?.entityId ?? node.entityId ?? node.documentId ?? "",
      axis:node.axis, ownerEntityId: node.ownerEntityId, bodyId: node.bodyId}, editingSketch);
  }
}

export function visibilityResolverForView(view?: DocumentView): VisibilityResolver {
  return new VisibilityResolver(view?.structureTree);
}
