import * as THREE from "three";
import type { Line2 } from "three/addons/lines/Line2.js";

/** Dash distances are projected CSS pixels, including occurrence transforms.
 * Changing line geometry or camera scale never changes the dash/gap cadence. */
export function updateScreenDashDistances(line: Line2, camera: THREE.Camera, width: number, height: number): void {
  const geometry = line.geometry;
  const starts = geometry.getAttribute("instanceStart"), ends = geometry.getAttribute("instanceEnd");
  const distancesStart = geometry.getAttribute("instanceDistanceStart"), distancesEnd = geometry.getAttribute("instanceDistanceEnd");
  if (!starts || !ends || !distancesStart || !distancesEnd) return;
  const a = new THREE.Vector3(), b = new THREE.Vector3();
  let distance = 0;
  for (let i = 0; i < starts.count; i++) {
    a.fromBufferAttribute(starts, i).applyMatrix4(line.matrixWorld).project(camera);
    b.fromBufferAttribute(ends, i).applyMatrix4(line.matrixWorld).project(camera);
    const length = Math.hypot((b.x-a.x)*width/2, (b.y-a.y)*height/2);
    distancesStart.setX(i, distance);
    distance += Number.isFinite(length) ? length : 0;
    distancesEnd.setX(i, distance);
  }
  distancesStart.needsUpdate = true; distancesEnd.needsUpdate = true;
}

/** Keep an arrow wing fixed in CSS pixels while its anchor stays on the model. */
export function screenSpaceEndpoint(anchor: THREE.Vector3, toward: THREE.Vector3, camera: THREE.Camera,
  width: number, height: number, pixels: number): THREE.Vector3 {
  const a = anchor.clone().project(camera), b = toward.clone().project(camera);
  const dx = (b.x-a.x)*width/2, dy = (b.y-a.y)*height/2;
  const length = Math.hypot(dx, dy);
  if (length < 1e-10) return anchor.clone();
  return new THREE.Vector3(a.x + dx/length*pixels*2/width, a.y + dy/length*pixels*2/height, a.z).unproject(camera);
}


// Keep dynamic geometry updates before GPU upload and before hit testing.
// Weak ownership also permits normal scene disposal without a second registry.
type ScreenUpdate = (camera: THREE.Camera, width: number, height: number) => void;
const screenUpdates = new WeakMap<THREE.Object3D, ScreenUpdate>();
export function registerScreenLineUpdate(object: THREE.Object3D, update: ScreenUpdate): void {
  screenUpdates.set(object, update);
}
export function updateScreenLines(root: THREE.Object3D, camera: THREE.Camera, width: number, height: number): void {
  camera.updateMatrixWorld();
  root.updateMatrixWorld(true);
  root.traverse(object => screenUpdates.get(object)?.(camera, width, height));
}
