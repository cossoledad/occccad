import { decodeMeshGLB } from './mesh-glb';
import { buildContourIncidence } from '../rendering/mesh-contour-incidence';

self.onmessage = ({data}: MessageEvent<{id: number; buffer: ArrayBuffer; contours: boolean}>) => {
  try {
    const decoded = decodeMeshGLB(data.buffer), mesh = decoded.mesh;
    const contourIncidence = data.contours && mesh.buffers!.indices.length ? buildContourIncidence(mesh.buffers!) : undefined;
    // Explicit DTO: structured cloning the lazy MeshData would invoke all its
    // array getters and recreate the large object graph we are avoiding.
    const result = {id: data.id, buffer: data.buffer, buffers: mesh.buffers!, edges: mesh.edges, topologyVertices: mesh.topologyVertices, association: decoded.association, visualization: decoded.visualization, contourIncidence};
    const buffers = new Set<ArrayBuffer>([data.buffer]);
    for (const array of Object.values(result.buffers)) if (array) buffers.add(array.buffer as ArrayBuffer);
    for (const array of Object.values(contourIncidence ?? {})) buffers.add(array.buffer as ArrayBuffer);
    self.postMessage(result, {transfer: [...buffers]});
  } catch (error) { self.postMessage({id: data.id, error: error instanceof Error ? error.message : String(error)}); }
};
