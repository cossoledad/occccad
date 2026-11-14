import * as THREE from "three";

type ScreenUpdate = (camera: THREE.Camera, width: number, height: number) => void;
const screenUpdates = new WeakMap<THREE.Object3D, ScreenUpdate>();
const owners = new WeakMap<THREE.Object3D, Set<SceneRenderIndex>>();

/** Display-only membership, maintained by Three's public child lifecycle events.
 * Properties (visibility, materials, transforms) are read from the real objects.
 * No model/occurrence identity or copied scene state lives here. */
export class SceneRenderIndex {
  readonly meshes = new Set<THREE.Mesh>();
  readonly points = new Set<THREE.Points>();
  readonly screenObjects = new Set<THREE.Object3D>();
  readonly sketchObjects = new Set<THREE.Object3D>();
  private readonly objects = new Set<THREE.Object3D>();
  private readonly added = (event: { child: THREE.Object3D }) => this.attach(event.child);
  private readonly removed = (event: { child: THREE.Object3D }) => this.detach(event.child);
  constructor(readonly root: THREE.Object3D) { this.attach(root); }
  get size() { return this.objects.size; }
  screenUpdateRegistered(object: THREE.Object3D): void {
    if(this.screenObjects.has(object))return;
    if(!object.children.length){this.screenObjects.add(object);return;}
    const ordered=[...this.screenObjects];
    const before=ordered.findIndex(candidate=>{
      for(let parent=candidate.parent;parent;parent=parent.parent)if(parent===object)return true;
      return false;
    });
    if(before<0){this.screenObjects.add(object);return;}
    // Registration is rare. Insert a late parent before its first registered
    // descendant here, rather than sorting or walking the scene every frame.
    this.screenObjects.clear();
    ordered.forEach((candidate,index)=>{if(index===before)this.screenObjects.add(object);this.screenObjects.add(candidate)});
  }
  private attach(object: THREE.Object3D): void {
    if (this.objects.has(object)) return;
    this.objects.add(object);
    let indexes = owners.get(object);
    if (!indexes) { indexes = new Set(); owners.set(object, indexes); }
    indexes.add(this);
    if (object instanceof THREE.Mesh) this.meshes.add(object);
    if (object instanceof THREE.Points) this.points.add(object);
    if (screenUpdates.has(object)) this.screenObjects.add(object);
    if(object.userData.sketchFeatureID||object.userData.kind==='sketch-constraint')this.sketchObjects.add(object);
    object.addEventListener("childadded", this.added);
    object.addEventListener("childremoved", this.removed);
    for (const child of object.children) this.attach(child);
  }
  private detach(object: THREE.Object3D): void {
    if (!this.objects.delete(object)) return;
    object.removeEventListener("childadded", this.added);
    object.removeEventListener("childremoved", this.removed);
    owners.get(object)?.delete(this);
    this.meshes.delete(object as THREE.Mesh);
    this.points.delete(object as THREE.Points);
    this.screenObjects.delete(object);
    this.sketchObjects.delete(object);
    for (const child of object.children) this.detach(child);
  }
  dispose(): void { this.detach(this.root); }
}

/** Sketch presentation visits owned subtrees only, never an imported Body's
 * mesh/edge hierarchy on every cursor frame. Unindexed small roots fall back. */
export function forEachSketchRoot(root:THREE.Object3D,visit:(object:THREE.Object3D)=>void):boolean {
  const index=indexFor(root);if(!index)return false;
  for(const object of index.sketchObjects){
    let parent=object.parent,covered=false;
    while(parent&&parent!==root){
      if(index.sketchObjects.has(parent)){covered=true;break}
      parent=parent.parent;
    }
    if(!covered&&(object===root||parent===root))visit(object);
  }
  return true;
}

function indexFor(root: THREE.Object3D): SceneRenderIndex | undefined {
  return owners.get(root)?.values().next().value;
}

export function visibleWithin(object: THREE.Object3D, root: THREE.Object3D): boolean {
  for (let current: THREE.Object3D | null = object; current; current = current.parent) {
    if (!current.visible) return false;
    if (current === root) return true;
  }
  return false;
}

export function forEachVisibleMesh(root: THREE.Object3D, visit: (mesh: THREE.Mesh) => void): void {
  const index = indexFor(root);
  if (index) { for (const mesh of index.meshes) if (visibleWithin(mesh, root)) visit(mesh); }
  else root.traverseVisible(object => { if (object instanceof THREE.Mesh) visit(object); });
}

export function registerScreenUpdate(object: THREE.Object3D, update: ScreenUpdate): void {
  screenUpdates.set(object, update);
  for (const index of owners.get(object) ?? []) index.screenUpdateRegistered(object);
}

export function updateScreenObjects(root: THREE.Object3D, camera: THREE.Camera, width: number, height: number): void {
  const index = indexFor(root);
  if (!index) { root.traverse(object => screenUpdates.get(object)?.(camera, width, height)); return; }
  for (const object of index.screenObjects) {
    let parent: THREE.Object3D | null = object;
    while (parent && parent !== root) parent = parent.parent;
    if (parent) screenUpdates.get(object)?.(camera, width, height);
  }
}
