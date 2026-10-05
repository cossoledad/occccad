import { selectionKey } from "../../cad/interaction/selection-identity";
import type { Selection, SelectionItem } from "../../types";
import type { SpecificationTreeNode } from "./specification-tree";
import { closestTreeKey } from "./tree-selection";

type Index = {
  byNode: Map<string, SpecificationTreeNode>;
  bySelection: Map<string, string[]>;
  publicationsByLocator: Map<string, string[]>;
  parents: Map<string, string[]>;
};
const cache = new WeakMap<SpecificationTreeNode[], Index>();

function topologyLocator(selection: SelectionItem): string | undefined {
  if (!(selection.kind === "face" || selection.kind === "edge" || selection.kind === "vertex") ||
      !selection.geometryKey || !selection.versionId) return undefined;
  return JSON.stringify([selection.kind, selection.documentId ?? "", selection.bodyId ?? "",
    selection.instancePath?.canonical ?? selection.occurrencePath ?? "",
    selection.versionId, selection.contextVariantKey ?? "", selection.geometryKey, selection.topologyId]);
}

function indexTree(nodes: SpecificationTreeNode[]): Index {
  const cached = cache.get(nodes);
  if (cached) return cached;
  const result: Index = { byNode: new Map(), bySelection: new Map(), publicationsByLocator: new Map(), parents: new Map() };
  const visit = (node: SpecificationTreeNode, parents: string[]) => {
    result.byNode.set(node.key, node);
    result.parents.set(node.key, parents);
    if (node.selection) {
      const key = selectionKey(node.selection);
      result.bySelection.set(key, [...(result.bySelection.get(key) ?? []), node.key]);
      if (node.selection.kind === "publication" && node.selection.highlightTarget) {
        const locator = topologyLocator(node.selection.highlightTarget);
        if (locator) result.publicationsByLocator.set(locator,
          [...(result.publicationsByLocator.get(locator) ?? []), node.key]);
      }
    }
    node.children?.forEach((child) => visit(child, [...parents, node.key]));
  };
  nodes.forEach((node) => visit(node, []));
  cache.set(nodes, result);
  return result;
}

export function treeKeyForSelection(nodes: SpecificationTreeNode[], selection: Selection): string | undefined {
  if (!selection) return undefined;
  const index = indexTree(nodes);
  if (selection.treeNodeId && index.byNode.get(selection.treeNodeId)?.selection &&
      selectionKey(index.byNode.get(selection.treeNodeId)!.selection!) === selectionKey(selection)) return selection.treeNodeId;
  const matches = index.bySelection.get(selectionKey(selection));
  return matches?.find((key) => index.byNode.get(key)?.presentationRole !== "INPUT_REFERENCE") ?? matches?.[0];
}

export function treeKeysForSelections(nodes: SpecificationTreeNode[], selections: readonly SelectionItem[]): string[] {
  const index = indexTree(nodes);
  const keys = new Set<string>();
  for (const selection of selections) {
    for (const key of index.bySelection.get(selectionKey(selection)) ?? []) keys.add(key);
    const locator = topologyLocator(selection);
    if (locator) for (const key of index.publicationsByLocator.get(locator) ?? []) keys.add(key);
  }
  return [...keys];
}

export function ancestorHintKeysForSelections(nodes: SpecificationTreeNode[], selections: readonly SelectionItem[]): string[] {
  const index = indexTree(nodes);
  const result = new Set<string>();
  for (const selection of selections) {
    if(["face","edge","vertex"].includes(selection.kind))for(const id of selection.associatedFeatureIds??[]) {
      for(const node of index.byNode.values()) {
        const target=node.selection;
        if(!target||node.presentationRole==="INPUT_REFERENCE")continue;
        if((target.entityId??target.id)!==id||target.documentId!==selection.documentId||target.versionId!==selection.versionId||target.bodyId!==selection.bodyId||target.geometryKey!==selection.geometryKey||target.contextVariantKey!==selection.contextVariantKey||(target.occurrencePath??"")!==(selection.occurrencePath??""))continue;
        result.add(node.key);for(const parent of index.parents.get(node.key)??[])result.add(parent);
      }
    }
    const exact = index.bySelection.get(selectionKey(selection)) ?? [];
    const location = exact.length ? exact : [closestTreeKey(nodes, selection.treeNodeId)].filter((key): key is string => Boolean(key));
    for (const key of location) {
      for (const parent of index.parents.get(key) ?? []) result.add(parent);
      if (!exact.length) result.add(key);
    }
  }
  return [...result];
}

/** Exact semantic selection only; ancestor/publication highlight projections are not mutation targets. */
export function deletableTreeNodesForSelections(nodes:SpecificationTreeNode[],selections:readonly SelectionItem[]):SpecificationTreeNode[] {
  const index=indexTree(nodes),result=new Map<string,SpecificationTreeNode>();
  for(const selection of selections)for(const key of index.bySelection.get(selectionKey(selection))??[]){
    const node=index.byNode.get(key)!;
    if(node.entityId&&node.kind&&node.presentationRole!=="INPUT_REFERENCE"&&node.capabilities?.includes("DELETE"))result.set(key,node);
  }
  return [...result.values()];
}

/** Association is a view hint only and cannot become a command target. */
export function associatedTreeKeyForSelection(nodes:SpecificationTreeNode[],selection:Selection):string|undefined {
 if(!selection||!["face","edge","vertex"].includes(selection.kind)||selection.associatedFeatureIds?.length!==1)return;
 const id=selection.associatedFeatureIds[0];
 for(const node of indexTree(nodes).byNode.values()) {const target=node.selection;
  if(target&&node.presentationRole!=="INPUT_REFERENCE"&&(target.entityId??target.id)===id&&target.documentId===selection.documentId&&target.versionId===selection.versionId&&target.bodyId===selection.bodyId&&target.geometryKey===selection.geometryKey&&target.contextVariantKey===selection.contextVariantKey&&(target.occurrencePath??"")===(selection.occurrencePath??""))return node.key;
 }
}
