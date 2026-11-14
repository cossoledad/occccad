import * as THREE from "three";

/** Fixed-size numeric comparisons avoid per-frame JSON keys or vertex scans. */
class Numbers {
  private values: number[] = [];
  private offset = 0;
  changed = false;
  begin() { this.offset = 0; this.changed = false; }
  add(value: number) {
    if (this.values[this.offset] !== value) { this.values[this.offset] = value; this.changed = true; }
    this.offset++;
  }
  end() {
    if (this.values.length !== this.offset) { this.values.length = this.offset; this.changed = true; }
    return this.changed;
  }
}

type MeshState = {
  geometry: THREE.BufferGeometry;
  position: unknown; positionArray: unknown;
  index: unknown; indexArray: unknown;
  numbers: Numbers; epoch: number;
};

/** Describes the depth inputs actually rendered. Surface colour, roughness,
 * hover and AO intensity are deliberately absent; they only affect colour. */
export class SurfaceDepthInputs {
  private readonly global = new Numbers();
  private readonly clearColor = new THREE.Color(0);
  private readonly meshes = new Map<THREE.Mesh, MeshState>();
  private source?: THREE.Scene;
  private renderer?: THREE.WebGLRenderer;
  private epoch = 0;

  update(renderer: THREE.WebGLRenderer, source: THREE.Scene, camera: THREE.Camera,
    meshes: readonly THREE.Mesh[], size: THREE.Vector2, linearDepth: boolean, planes: readonly THREE.Plane[]): boolean {
    let changed = source !== this.source || renderer !== this.renderer;
    this.source = source; this.renderer = renderer;
    const global = this.global, epoch = ++this.epoch;
    global.begin();
    for (const value of camera.matrixWorldInverse.elements) global.add(value);
    for (const value of camera.projectionMatrix.elements) global.add(value);
    global.add(camera.layers.mask); global.add(size.x); global.add(size.y);
    global.add((camera as THREE.PerspectiveCamera).near); global.add((camera as THREE.PerspectiveCamera).far);
    global.add(Number(camera instanceof THREE.PerspectiveCamera));
    global.add(Number(linearDepth)); global.add(Number(!!renderer.localClippingEnabled));
    if(typeof renderer.getClearColor==='function')renderer.getClearColor(this.clearColor);else this.clearColor.setRGB(0,0,0);
    global.add(this.clearColor.r);global.add(this.clearColor.g);global.add(this.clearColor.b);global.add(renderer.getClearAlpha?.()??0);
    for (const clipping of [renderer.clippingPlanes ?? [], planes]) {
      global.add(clipping.length);
      for (const plane of clipping) { global.add(plane.normal.x); global.add(plane.normal.y); global.add(plane.normal.z); global.add(plane.constant); }
    }
    changed = global.end() || changed;
    for (const mesh of meshes) {
      const geometry = mesh.geometry, position = geometry.getAttribute("position"), index = geometry.index;
      let state = this.meshes.get(mesh);
      if (!state) {
        state = {geometry, position, positionArray: position?.array, index, indexArray: index?.array, numbers: new Numbers(), epoch};
        this.meshes.set(mesh, state); changed = true;
      }
      if (state.geometry !== geometry || state.position !== position || state.index !== index || state.positionArray !== position?.array || state.indexArray !== index?.array) changed = true;
      state.geometry = geometry; state.position = position; state.index = index;
      state.positionArray = position?.array; state.indexArray = index?.array; state.epoch = epoch;
      const values = state.numbers;
      values.begin();
      values.add(position instanceof THREE.InterleavedBufferAttribute ? position.data.version : position?.version ?? 0);
      values.add(index?.version ?? 0); values.add(mesh.layers.mask); values.add(mesh.renderOrder);
      for (const value of mesh.matrixWorld.elements) values.add(value);
      values.add(geometry.drawRange.start); values.add(geometry.drawRange.count);
      values.add(geometry.groups.length);
      for (const group of geometry.groups) { values.add(group.start); values.add(group.count); values.add(group.materialIndex ?? 0); }
      const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material];
      for (const material of materials) {
        values.add(Number(material.visible)); values.add(material.opacity); values.add(Number(material.transparent));
        values.add(Number(material.userData.cadMaterial === "surface"));
      }
      changed = values.end() || changed;
      // CAD uses static meshes. An unexpected deformed mesh cannot reuse a
      // static snapshot solely on the basis of its base position version.
      if ((mesh as THREE.SkinnedMesh).isSkinnedMesh || (mesh as THREE.InstancedMesh).isInstancedMesh || geometry.morphAttributes.position?.length) changed = true;
    }
    for (const [mesh, state] of this.meshes) if (state.epoch !== epoch) { this.meshes.delete(mesh); changed = true; }
    return changed;
  }
  clear() { this.meshes.clear(); this.source = undefined; this.renderer = undefined; }
}
