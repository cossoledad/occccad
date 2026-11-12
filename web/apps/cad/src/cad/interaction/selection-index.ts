import * as THREE from "three";
import type { Selection, SelectionItem } from "../../types";
import { selectionKey } from "./selection-identity";

export type PickResolver = (intersection: THREE.Intersection) => Selection;
export type SelectionHit = { selection: Selection; intersection?: THREE.Intersection };

type PickBinding = { root: THREE.Object3D; resolve: PickResolver; priority: number; interactionLayer: number; domain?: "sketch" };
export type PickOptions = { preferSolidTopology?: boolean; depthTolerance?: number };

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

  registerPick(root: THREE.Object3D, resolve: PickResolver, priority = 0, interactionLayer = 0, domain?: "sketch"): void {
    this.picks.push({ root, resolve, priority, interactionLayer, domain });
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

  pick(raycaster: THREE.Raycaster, accepts: (selection: Exclude<Selection, null>) => boolean = () => true, options:PickOptions={}): Selection {
    return this.pickWithIntersection(raycaster, accepts, options).selection;
  }

  pickWithIntersection(raycaster:THREE.Raycaster,accepts:(selection:Exclude<Selection,null>)=>boolean=()=>true,options:PickOptions={}):SelectionHit {
    return this.pickAll(raycaster,accepts,1,options)[0]??{selection:null};
  }

  pickAll(raycaster: THREE.Raycaster,
    accepts: (selection: Exclude<Selection, null>) => boolean = () => true, limit=256, options:PickOptions={}): SelectionHit[] {
    const visible = (root: THREE.Object3D) => {
      for (let object: THREE.Object3D | null = root; object; object = object.parent) if (!object.visible||object.userData.appearancePickable===false) return false;
      return true;
    };
    const activePicks = this.picks.filter((binding) => visible(binding.root));
    const roots = activePicks.map((binding) => binding.root);
    if (roots.length === 0) return [];
    const bindings = new Map(activePicks.map((binding) => [binding.root.uuid, binding]));
    const candidates = raycaster.intersectObjects(roots, false).flatMap((intersection) => {
      const binding = bindings.get(intersection.object.uuid);
      const raw=binding?.resolve(intersection),selection=raw?this.projection(raw):null;
      if(!binding||!selection||!accepts(selection))return [];
      // Outside sketch editing: vertex (50), edge (40), sketch point (35),
      // sketch curve (30), sketch annotation (25), face (20). Explicit command
      // filters still apply first; inactive corner glyphs cannot steal vertices.
      const priority=options.preferSolidTopology&&selection.kind==="sketch-constraint"?Math.min(binding.priority,25)
        :options.preferSolidTopology&&binding.domain==="sketch"&&selection.kind==="visual"
          ? Math.min(binding.priority,selection.visualType==="POINT"?35:30):binding.priority;
      return [{intersection,binding,selection,priority}];
    });
    candidates.sort((a,b)=>b.binding.interactionLayer-a.binding.interactionLayer||a.intersection.distance-b.intersection.distance);
    // Anchor each depth band at its nearest hit. Pairwise "close enough"
    // comparisons are non-transitive and depend on registration order.
    const tolerance=Math.max(0,options.depthTolerance??0.75);
    for(let start=0;start<candidates.length;){
      let end=start+1;const first=candidates[start];
      while(end<candidates.length&&candidates[end].binding.interactionLayer===first.binding.interactionLayer&&candidates[end].intersection.distance-first.intersection.distance<=tolerance)end++;
      const band=candidates.slice(start,end).sort((a,b)=>b.priority-a.priority||a.intersection.distance-b.intersection.distance);
      for(let i=0;i<band.length;i++)candidates[start+i]=band[i];start=end;
    }
    const results:SelectionHit[]=[];const seen=new Set<string>();
    for (const candidate of candidates) {
      const selection=candidate.selection;
      if(!seen.has(selectionKey(selection))){seen.add(selectionKey(selection));results.push({selection,intersection:candidate.intersection});if(results.length>=limit)break}
    }
    return results;
  }
}
