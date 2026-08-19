import * as THREE from "three";
import type { Selection } from "../../types";

export type PickResolver = (intersection: THREE.Intersection) => Selection;

type PickBinding = { root: THREE.Object3D; resolve: PickResolver; priority: number };

const keyOf = (selection: Exclude<Selection, null>): string => `${selection.kind}:${selection.id}`;

// Scene objects and tree nodes share Selection identity through this index.
// Topology elements are resolved lazily from hit indices, so the index remains
// proportional to rendered objects/occurrences rather than B-Rep element count.
export class SelectionIndex {
  private readonly objects = new Map<string, Set<THREE.Object3D>>();
  private readonly tree = new Map<string, Selection>();
  private readonly picks: PickBinding[] = [];

  clear(): void { this.objects.clear(); this.tree.clear(); this.picks.length = 0; }

  register(selection: Exclude<Selection, null>, object: THREE.Object3D, treeNodeId = selection.treeNodeId): void {
    const key = keyOf(selection);
    const entries = this.objects.get(key) ?? new Set<THREE.Object3D>();
    entries.add(object); this.objects.set(key, entries);
    if (treeNodeId) this.tree.set(treeNodeId, selection);
  }

  registerVisualKey(key: string, object: THREE.Object3D): void {
    const entries = this.objects.get(key) ?? new Set<THREE.Object3D>();
    entries.add(object); this.objects.set(key, entries);
  }

  registerPick(root: THREE.Object3D, resolve: PickResolver, priority = 0): void {
    this.picks.push({ root, resolve, priority });
  }

  objectsFor(selection: Selection): readonly THREE.Object3D[] {
    if (!selection) return [];
    const direct = this.objects.get(keyOf(selection));
    const visual = selection.visualKey ? this.objects.get(selection.visualKey) : undefined;
    return [...new Set([...(direct ?? []), ...(visual ?? [])])];
  }

  selectionForTreeNode(treeNodeId: string): Selection { return this.tree.get(treeNodeId) ?? null; }

  pick(raycaster: THREE.Raycaster): Selection {
    const roots = this.picks.map((binding) => binding.root);
    if (roots.length === 0) return null;
    const bindings = new Map(this.picks.map((binding) => [binding.root.uuid, binding]));
    const candidates = raycaster.intersectObjects(roots, false).map((intersection) => {
      const binding = bindings.get(intersection.object.uuid);
      return binding ? { intersection, binding } : undefined;
    }).filter((value): value is { intersection: THREE.Intersection; binding: PickBinding } => Boolean(value));
    candidates.sort((left, right) => Math.abs(left.intersection.distance - right.intersection.distance) < 0.75
      ? right.binding.priority - left.binding.priority : left.intersection.distance - right.intersection.distance);
    return candidates[0]?.binding.resolve(candidates[0].intersection) ?? null;
  }
}

export function sameSelection(left: Selection, right: Selection): boolean {
  return left === right || Boolean(left && right && left.kind === right.kind && left.id === right.id);
}
