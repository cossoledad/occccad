import * as THREE from "three";
import type { Selection, SelectionItem } from "../../types";
import { selectionKey } from "./selection-identity";

export type PickResolver = (intersection: THREE.Intersection) => Selection;
export type SelectionHit = { selection: Selection; intersection?: THREE.Intersection };

type PickBinding = { root: THREE.Object3D; resolve: PickResolver; priority: number; interactionLayer: number };

// Scene objects and tree nodes share Selection identity through this index.
// Topology elements are resolved lazily from hit indices, so the index remains
// proportional to rendered objects/occurrences rather than B-Rep element count.
export class SelectionIndex {
  private projection:(selection:SelectionItem)=>SelectionItem=value=>value;
  private readonly semanticEntries=new Map<string,SelectionItem>();
  setSemanticProjection(project:(selection:SelectionItem)=>SelectionItem):void {
    this.projection=project;
    const changes=[...this.semanticEntries].map(([key,selection])=>({key,selection:project(selection),objects:this.objects.get(key)}));
    for(const {key} of changes){this.objects.delete(key);this.semanticEntries.delete(key);}
    for(const {selection,objects} of changes){
      const key=selectionKey(selection);this.semanticEntries.set(key,selection);
      if(objects)this.objects.set(key,new Set([...(this.objects.get(key)??[]),...objects]));
    }
    for(const [key,selection] of this.tree)if(selection)this.tree.set(key,project(selection));
  }
  private readonly objects = new Map<string, Set<THREE.Object3D>>();
  private readonly treeObjects = new Map<string, Set<THREE.Object3D>>();
  private readonly tree = new Map<string, Selection>();
  private readonly picks: PickBinding[] = [];

  clear(): void { this.objects.clear(); this.semanticEntries.clear();this.treeObjects.clear(); this.tree.clear(); this.picks.length = 0; }

  unregister(selection: Exclude<Selection, null>, root: THREE.Object3D): void {
    const removed = new Set<THREE.Object3D>(); root.traverse(object => removed.add(object));
    this.objects.delete(selectionKey(selection));
    this.semanticEntries.delete(selectionKey(selection));
    for (const map of [this.objects,this.treeObjects]) for (const [key,objects] of map) {
      for (const object of removed) objects.delete(object);
      if(!objects.size){map.delete(key);if(map===this.objects)this.semanticEntries.delete(key);}
    }
    if(selection.treeNodeId && !this.treeObjects.has(selection.treeNodeId))this.tree.delete(selection.treeNodeId);
    for(let i=this.picks.length-1;i>=0;i--)if(removed.has(this.picks[i].root))this.picks.splice(i,1);
  }

  register(selection: Exclude<Selection, null>, object: THREE.Object3D, treeNodeId = selection.treeNodeId): void {
    const key = selectionKey(selection);
    this.semanticEntries.set(key,selection);
    const entries = this.objects.get(key) ?? new Set<THREE.Object3D>();
    entries.add(object); this.objects.set(key, entries);
    if (treeNodeId) {
      this.tree.set(treeNodeId, selection);
      const treeEntries = this.treeObjects.get(treeNodeId) ?? new Set<THREE.Object3D>();
      treeEntries.add(object); this.treeObjects.set(treeNodeId, treeEntries);
    }
  }

  registerVisualKey(key: string, object: THREE.Object3D): void {
    const entries = this.objects.get(key) ?? new Set<THREE.Object3D>();
    entries.add(object); this.objects.set(key, entries);
  }

  associate(selection: Exclude<Selection, null>, object: THREE.Object3D): void {
    const key = selectionKey(selection);
    this.semanticEntries.set(key,selection);
    const entries = this.objects.get(key) ?? new Set<THREE.Object3D>();
    entries.add(object); this.objects.set(key, entries);
  }

  registerPick(root: THREE.Object3D, resolve: PickResolver, priority = 0, interactionLayer = 0): void {
    this.picks.push({ root, resolve, priority, interactionLayer });
  }

  objectsFor(selection: Selection): readonly THREE.Object3D[] {
    if (!selection) return [];
    const direct = this.objects.get(selectionKey(selection));
    const visual = selection.visualKey ? this.objects.get(selection.visualKey) : undefined;
    const related = selection.kind === "publication" && selection.highlightTarget
      ? this.objectsFor(selection.highlightTarget) : [];
    const descendants: THREE.Object3D[] = [];
    if (selection.treeNodeId && selection.expandTreeDescendants) {
      for (const [treeNodeId, objects] of this.treeObjects) {
        if (treeNodeId === selection.treeNodeId || treeNodeId.startsWith(`${selection.treeNodeId}/`)) descendants.push(...objects);
      }
    }
    return [...new Set([...(direct ?? []), ...(visual ?? []), ...related, ...descendants])];
  }

  objectsForMany(selections: readonly SelectionItem[]): readonly THREE.Object3D[] {
    return [...new Set(selections.flatMap((selection) => [...this.objectsFor(selection)]))];
  }

  selectionForTreeNode(treeNodeId: string): Selection { const selection=this.tree.get(treeNodeId);return selection?this.projection(selection):null; }

  pick(raycaster: THREE.Raycaster, accepts: (selection: Exclude<Selection, null>) => boolean = () => true): Selection {
    return this.pickWithIntersection(raycaster, accepts).selection;
  }

  pickWithIntersection(raycaster: THREE.Raycaster,
    accepts: (selection: Exclude<Selection, null>) => boolean = () => true): SelectionHit {
    const visible = (root: THREE.Object3D) => {
      for (let object: THREE.Object3D | null = root; object; object = object.parent) if (!object.visible) return false;
      return true;
    };
    const activePicks = this.picks.filter((binding) => visible(binding.root));
    const roots = activePicks.map((binding) => binding.root);
    if (roots.length === 0) return {selection:null};
    const bindings = new Map(activePicks.map((binding) => [binding.root.uuid, binding]));
    const candidates = raycaster.intersectObjects(roots, false).map((intersection) => {
      const binding = bindings.get(intersection.object.uuid);
      return binding ? { intersection, binding } : undefined;
    }).filter((value): value is { intersection: THREE.Intersection; binding: PickBinding } => Boolean(value));
    candidates.sort((left, right) => left.binding.interactionLayer !== right.binding.interactionLayer
      ? right.binding.interactionLayer - left.binding.interactionLayer
      : Math.abs(left.intersection.distance - right.intersection.distance) < 0.75
        ? right.binding.priority - left.binding.priority : left.intersection.distance - right.intersection.distance);
    for (const candidate of candidates) {
      const raw=candidate.binding.resolve(candidate.intersection);
      const selection=raw?this.projection(raw):null;
      if (selection && accepts(selection)) return {selection,intersection:candidate.intersection};
    }
    return {selection:null};
  }
}
