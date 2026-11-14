import * as THREE from "three";
import { computeBoundsTree, disposeBoundsTree } from "three-mesh-bvh";
import type { MeshData } from "../../types";

type SurfaceGeometry = THREE.BufferGeometry & {computeBoundsTree: typeof computeBoundsTree; disposeBoundsTree: typeof disposeBoundsTree};

export function makeSurfaceGeometry(mesh: MeshData, pickable = true): SurfaceGeometry {
  const geometry = new THREE.BufferGeometry() as SurfaceGeometry;
  const packed = mesh.buffers;
  const positions = packed?.positions ?? new Float32Array(mesh.vertices.length * 3);
  const indices = packed?.indices ?? new Uint32Array(mesh.triangles.length * 3);
  if (!packed) {
    mesh.vertices.forEach((point, i) => positions.set(point, i * 3));
    mesh.triangles.forEach((triangle, i) => indices.set(triangle, i * 3));
  }
  geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
  geometry.setIndex(new THREE.BufferAttribute(indices, 1));
  if (packed?.normals || (!packed && mesh.normals)) {
    const normals = packed?.normals ?? new Float32Array(mesh.normals!.length * 3);
    if (!packed) mesh.normals!.forEach((normal, i) => normals.set(normal, i * 3));
    geometry.setAttribute("normal", new THREE.BufferAttribute(normals, 3));
  } else geometry.computeVertexNormals();
  if (positions.length) geometry.computeBoundingSphere();
  geometry.computeBoundsTree = computeBoundsTree;
  geometry.disposeBoundsTree = disposeBoundsTree;
  if (pickable && indices.length) geometry.computeBoundsTree({indirect: true});
  return geometry;
}

type Entry = {
  indices?: Uint32Array; normals?: Float32Array;
  geometries: SurfaceGeometry[]; free: SurfaceGeometry[]; users: number;
};

/** GPU attributes are immutable display data. Groups, bounds-tree and userData
 * remain local to each mesh, so per-face appearance/picking never leaks between
 * occurrences. This pool belongs to one viewport/renderer.
 *
 * Three removes GPU attributes on geometry.dispose without reference counting.
 * Released wrappers are reusable and stay alive until the final lease, when all
 * wrappers and GPU attributes are disposed together. Retention is bounded by
 * peak concurrent leases; no live mesh can lose its buffers on sibling removal.
 */
export class SurfaceGeometryPool {
  private readonly sources = new WeakMap<object, Entry[]>();
  acquire(mesh: MeshData): {geometry: SurfaceGeometry; release: () => void} {
    const packed = mesh.buffers, key = packed?.positions ?? mesh;
    let variants = this.sources.get(key);
    if (!variants) this.sources.set(key, variants = []);
    let entry = variants.find(item => item.indices === packed?.indices && item.normals === packed?.normals);
    if (!entry) {
      const base = makeSurfaceGeometry(mesh, false);
      entry = {indices: packed?.indices, normals: packed?.normals, geometries: [base], free: [base], users: 0};
      variants.push(entry);
    }
    let geometry = entry.free.pop();
    if (!geometry) {
      const base = entry.geometries[0];
      geometry = new THREE.BufferGeometry() as SurfaceGeometry;
      for (const [name, attribute] of Object.entries(base.attributes)) geometry.setAttribute(name, attribute);
      geometry.setIndex(base.index);
      geometry.boundingSphere = base.boundingSphere?.clone() ?? null;
      geometry.computeBoundsTree = computeBoundsTree; geometry.disposeBoundsTree = disposeBoundsTree;
      entry.geometries.push(geometry);
    }
    geometry.clearGroups(); geometry.userData = {};
    if (geometry.index?.count && !geometry.boundsTree) geometry.computeBoundsTree({indirect: true});
    entry.users++;
    const owned = geometry, resource = entry;
    let released = false;
    return {geometry, release: () => {
      if (released) return; released = true;
      resource.free.push(owned);
      if (--resource.users) return;
      for (const wrapper of resource.geometries) {wrapper.disposeBoundsTree(); wrapper.dispose();}
      variants!.splice(variants!.indexOf(resource), 1);
      if (!variants!.length) this.sources.delete(key);
    }};
  }
}
