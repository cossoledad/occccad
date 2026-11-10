import { decodeMeshGLB } from './mesh-glb';
import { indexedSurfaceContours } from '../rendering/mesh-topology-index';

self.onmessage = ({data}: MessageEvent<{id: number; buffer: ArrayBuffer; contours: boolean}>) => {
  try {
    const decoded = decodeMeshGLB(data.buffer), mesh = decoded.mesh;
    const contours = data.contours && mesh.buffers!.indices.length ? indexedSurfaceContours(mesh.buffers!.positions, mesh.buffers!.indices) : undefined;
    // Explicit DTO: structured cloning the lazy MeshData would invoke all its
    // array getters and recreate the large object graph we are avoiding.
    const result = {id: data.id, buffer: data.buffer, buffers: mesh.buffers!, edges: mesh.edges, topologyVertices: mesh.topologyVertices, association: decoded.association, visualization: decoded.visualization, contours};
    const buffers = new Set<ArrayBuffer>([data.buffer]);
    for (const array of Object.values(result.buffers)) if (array) buffers.add(array.buffer as ArrayBuffer);
    for (const array of Object.values(contours ?? {})) buffers.add(array.buffer as ArrayBuffer);
    self.postMessage(result, {transfer: [...buffers]});
  } catch (error) { self.postMessage({id: data.id, error: error instanceof Error ? error.message : String(error)}); }
};
