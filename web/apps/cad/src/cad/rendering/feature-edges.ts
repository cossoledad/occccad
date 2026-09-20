import * as THREE from "three";
import { mergeVertices } from "three/examples/jsm/utils/BufferGeometryUtils.js";

export function makeFeatureEdges(geometry: THREE.BufferGeometry): THREE.EdgesGeometry {
  // OCCT tessellation can repeat the same vertex for adjacent triangles/faces.
  // EdgesGeometry interprets those repetitions as open triangle boundaries and
  // makes a shaded solid look like a wireframe. Weld position-only geometry
  // before extracting display edges; keep the original mesh untouched.
  const edgeSource = new THREE.BufferGeometry();
  edgeSource.setAttribute("position", geometry.getAttribute("position").clone());
  if (geometry.index) edgeSource.setIndex(geometry.index.clone());
  const welded = mergeVertices(edgeSource, 1.0e-4);
  const edges = new THREE.EdgesGeometry(welded, 32);
  edgeSource.dispose();
  welded.dispose();
  return edges;
}
