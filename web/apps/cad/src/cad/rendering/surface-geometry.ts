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

function equalIndices(before: Uint32Array, after: Uint32Array): boolean {
  return before === after || before.length === after.length && before.every((index, i) => index === after[i]);
}

function changedRanges(before: Float32Array, after: Float32Array): {start: number; count: number}[] | undefined {
  if (before.length !== after.length) return;
  if (before === after) return [];
  const a = new Uint32Array(before.buffer, before.byteOffset, before.length);
  const b = new Uint32Array(after.buffer, after.byteOffset, after.length);
  const ranges: {start: number; count: number}[] = [];
  let changed = 0;
  for (let start = 0; start < a.length; start += 1024) {
    const end = Math.min(start + 1024, a.length);
    let different = false;
    for (let i = start; i < end && !different; i++) different = a[i] !== b[i];
    if (!different) continue;
    changed += end - start;
    if (changed * 3 > a.length) return;
    const last = ranges.at(-1);
    if (last && last.start + last.count === start) last.count += end - start;
    else ranges.push({start, count: end - start});
    if (ranges.length > 64) return;
  }
  return ranges;
}

/** GPU attributes are immutable display data. Groups, bounds-tree and userData
 * remain local to each mesh, so per-face appearance/picking never leaks between
 * occurrences. This pool belongs to one viewport/renderer.
 *
 * Three removes GPU attributes on geometry.dispose without reference counting.
 * Released wrappers survive synchronous reconciliation. A new snapshot with
 * identical triangle/locator layout can take ownership of a released entry,
 * upload only changed attributes and refit its BVH. Other entries dispose at
 * the microtask boundary, including when the viewport closes.
 */
export class SurfaceGeometryPool {
  private readonly sources = new WeakMap<object, Entry[]>();
  private revive(mesh: MeshData): Entry | undefined {
    const before = mesh.previousSurface?.deref()?.buffers;
    const after = mesh.buffers;
    if (!before?.normals || !after?.normals || before.positions.length < 8192 ||
        !equalIndices(before.indices, after.indices) || !equalIndices(before.faceIds, after.faceIds)) return;
    const siblings = this.sources.get(before.positions);
    const entry = siblings?.find(candidate => !candidate.users && candidate.indices === before.indices && candidate.normals === before.normals);
    if (!entry || !siblings) return;
    const positions = changedRanges(before.positions, after.positions);
    const normals = changedRanges(before.normals, after.normals);
    if (!positions || !normals) return;

    const base = entry.geometries[0];
    for (const [name, array, ranges] of [
      ['position', after.positions, positions], ['normal', after.normals, normals],
    ] as const) {
      const attribute = base.getAttribute(name) as THREE.BufferAttribute;
      attribute.array = array;
      if (ranges.length) {
        attribute.clearUpdateRanges();
        for (const range of ranges) attribute.addUpdateRange(range.start, range.count);
        attribute.needsUpdate = true;
      }
    }
    if (positions.length) {
      base.computeBoundingSphere();
      for (const geometry of entry.geometries) {
        geometry.boundingSphere = base.boundingSphere?.clone() ?? null;
        if (geometry.boundingBox) geometry.computeBoundingBox();
        geometry.boundsTree?.refit();
      }
    }
    entry.indices = after.indices;
    entry.normals = after.normals;
    siblings.splice(siblings.indexOf(entry), 1);
    if (!siblings.length) this.sources.delete(before.positions);
    return entry;
  }

  acquire(mesh: MeshData): {geometry: SurfaceGeometry; release: () => void} {
    const packed = mesh.buffers, key = packed?.positions ?? mesh;
    let variants = this.sources.get(key);
    if (!variants) this.sources.set(key, variants = []);
    let entry = variants.find(item => item.indices === packed?.indices && item.normals === packed?.normals);
    if (!entry) {
      entry = this.revive(mesh);
      if (!entry) {
        const base = makeSurfaceGeometry(mesh, false);
        entry = {indices: packed?.indices, normals: packed?.normals, geometries: [base], free: [base], users: 0};
      }
      variants.push(entry);
      this.sources.set(key, variants);
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
      queueMicrotask(() => {
        const index = variants!.indexOf(resource);
        if (resource.users || index < 0) return;
        for (const wrapper of resource.geometries) {wrapper.disposeBoundsTree(); wrapper.dispose();}
        variants!.splice(index, 1);
        if (!variants!.length && this.sources.get(key) === variants) this.sources.delete(key);
      });
    }};
  }
}
