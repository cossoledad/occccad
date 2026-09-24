import * as THREE from "three";
import { makeOcclusionVisibleHighlightLine } from "./interaction-highlight";
import { registerScreenLineUpdate } from "./screen-space-lines";

/** Extend on the actual sketch plane, rather than unprojecting at constant depth.
 * Orthographic projection keeps the two ends at a fixed CSS-pixel distance. */
export function sketchAxisEndpoints(camera: THREE.Camera, origin: THREE.Vector3, direction: THREE.Vector3,
  width: number, height: number, halfPixels = 160): [THREE.Vector3, THREE.Vector3] | undefined {
  const unit = direction.clone().normalize();
  if (Math.abs(unit.dot(camera.getWorldDirection(new THREE.Vector3()))) > 0.99995) return;
  const a = origin.clone().project(camera), b = origin.clone().add(unit).project(camera);
  const pixels = Math.hypot((b.x - a.x) * width / 2, (b.y - a.y) * height / 2);
  if (!Number.isFinite(pixels) || pixels < 1e-10) return;
  const offset = unit.multiplyScalar(halfPixels / pixels);
  return [origin.clone().sub(offset), origin.clone().add(offset)];
}

export function makeSketchReferenceAxis(origin: THREE.Vector3, direction: THREE.Vector3, color: THREE.Color, linewidth = 1.25) {
  const line = makeOcclusionVisibleHighlightLine([origin, origin], color.getHex(), linewidth);
  const geometry = line.geometry;
  line.renderOrder = 12;
  line.frustumCulled = false;
  registerScreenLineUpdate(line, (camera, width, height) => {
    const ends = sketchAxisEndpoints(camera, origin, direction, width, height);
    line.visible = Boolean(ends);
    if (!ends) return;
    const starts = geometry.getAttribute("instanceStart"), finishes = geometry.getAttribute("instanceEnd");
    starts.setXYZ(0, ...ends[0].toArray()); finishes.setXYZ(0, ...ends[1].toArray());
    starts.needsUpdate = true; finishes.needsUpdate = true;
    line.material.resolution.set(width, height);
    geometry.computeBoundingBox(); geometry.computeBoundingSphere();
  });
  return line;
}
