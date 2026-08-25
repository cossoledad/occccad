import * as THREE from "three";
import { Line2 } from "three/addons/lines/Line2.js";
import { LineGeometry } from "three/addons/lines/LineGeometry.js";
import { LineMaterial } from "three/addons/lines/LineMaterial.js";

export function makeOcclusionVisibleHighlightLine(
  points: readonly THREE.Vector3[], color: number, linewidth = 4,
): Line2 {
  const geometry = new LineGeometry();
  geometry.setPositions(points.flatMap((point) => [point.x, point.y, point.z]));
  const material = new LineMaterial({ color, linewidth, worldUnits: false, transparent: true, opacity: 0.96,
    depthTest: false, depthWrite: false, toneMapped: false });
  material.resolution.set(1, 1);
  const line = new Line2(geometry, material);
  line.computeLineDistances();
  return line;
}

export function updateHighlightLineResolution(root: THREE.Object3D, width: number, height: number): void {
  root.traverse((object) => {
    const material = (object as Line2).material;
    if (material instanceof LineMaterial) material.resolution.set(width, height);
  });
}
