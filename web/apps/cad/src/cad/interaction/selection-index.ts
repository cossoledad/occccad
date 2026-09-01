import * as THREE from "three";
import type { Selection, SelectionItem } from "../../types";
import { selectionKey } from "./selection-identity";

export type PickResolver = (intersection: THREE.Intersection) => Selection;

type PickBinding = { root: THREE.Object3D; resolve: PickResolver; priority: number };

// Scene objects and tree nodes share Selection identity through this index.
// Topology elements are resolved lazily from hit indices, so the index remains
// proportional to rendered objects/occurrences rather than B-Rep element count.
export class SelectionIndex {
  private readonly objects = new Map<string, Set<THREE.Object3D>>();
  private readonly treeObjects = new Map<string, Set<THREE.Object3D>>();
  private readonly tree = new Map<string, Selection>();
  private readonly picks: PickBinding[] = [];

  clear(): void { this.objects.clear(); this.treeObjects.clear(); this.tree.clear(); this.picks.length = 0; }

  register(selection: Exclude<Selection, null>, object: THREE.Object3D, treeNodeId = selection.treeNodeId): void {
    const key = selectionKey(selection);
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
    const entries = this.objects.get(key) ?? new Set<THREE.Object3D>();
    entries.add(object); this.objects.set(key, entries);
  }

  registerPick(root: THREE.Object3D, resolve: PickResolver, priority = 0): void {
    this.picks.push({ root, resolve, priority });
  }

  objectsFor(selection: Selection): readonly THREE.Object3D[] {
    if (!selection) return [];
    const direct = this.objects.get(selectionKey(selection));
    const visual = selection.visualKey ? this.objects.get(selection.visualKey) : undefined;
    const descendants: THREE.Object3D[] = [];
    if (selection.treeNodeId && selection.expandTreeDescendants) {
      for (const [treeNodeId, objects] of this.treeObjects) {
        if (treeNodeId === selection.treeNodeId || treeNodeId.startsWith(`${selection.treeNodeId}/`)) descendants.push(...objects);
      }
    }
    return [...new Set([...(direct ?? []), ...(visual ?? []), ...descendants])];
  }

  objectsForMany(selections: readonly SelectionItem[]): readonly THREE.Object3D[] {
    return [...new Set(selections.flatMap((selection) => [...this.objectsFor(selection)]))];
  }

  selectionForTreeNode(treeNodeId: string): Selection { return this.tree.get(treeNodeId) ?? null; }

  pick(raycaster: THREE.Raycaster, accepts: (selection: Exclude<Selection, null>) => boolean = () => true): Selection {
    const visible = (root: THREE.Object3D) => {
      for (let object: THREE.Object3D | null = root; object; object = object.parent) if (!object.visible) return false;
      return true;
    };
    const activePicks = this.picks.filter((binding) => visible(binding.root));
    const roots = activePicks.map((binding) => binding.root);
    if (roots.length === 0) return null;
    const bindings = new Map(activePicks.map((binding) => [binding.root.uuid, binding]));
    const candidates = raycaster.intersectObjects(roots, false).map((intersection) => {
      const binding = bindings.get(intersection.object.uuid);
      return binding ? { intersection, binding } : undefined;
    }).filter((value): value is { intersection: THREE.Intersection; binding: PickBinding } => Boolean(value));
    candidates.sort((left, right) => Math.abs(left.intersection.distance - right.intersection.distance) < 0.75
      ? right.binding.priority - left.binding.priority : left.intersection.distance - right.intersection.distance);
    for (const candidate of candidates) {
      const selection = candidate.binding.resolve(candidate.intersection);
      if (selection && accepts(selection)) return selection;
    }
    return null;
  }
}
