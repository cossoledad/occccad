import type { MeshData, Vec3 } from '../../types';

export type MeshBuffers = NonNullable<MeshData['buffers']>;
export const triangleCount = (mesh: MeshData): number => mesh.buffers ? mesh.buffers.indices.length / 3 : mesh.triangles.length;

/** The renderer borrows immutable GLB buffers. Array projections are materialized
 * only for callers which actually need the legacy CPU geometry interface. */
export function packedMesh(buffers: MeshBuffers, edges: MeshData['edges'], topologyVertices: MeshData['topologyVertices']): MeshData {
  const mesh = { edges, topologyVertices } as MeshData;
  const vectors = (values: ArrayLike<number>): Vec3[] => Array.from({length: values.length / 3}, (_, i) => [values[i*3], values[i*3+1], values[i*3+2]]);
  const lazy = (key: keyof MeshData, read: () => unknown) => {
    let value: unknown;
    Object.defineProperty(mesh, key, {enumerable: true, get: () => value ??= read()});
  };
  Object.defineProperty(mesh, 'buffers', {value: buffers});
  lazy('vertices', () => vectors(buffers.positions));
  lazy('triangles', () => vectors(buffers.indices));
  lazy('faceIds', () => Array.from(buffers.faceIds));
  if (buffers.normals) lazy('normals', () => vectors(buffers.normals!));
  return mesh;
}
