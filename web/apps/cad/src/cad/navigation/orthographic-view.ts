import * as THREE from "three";

export type StandardView = "TOP" | "FRONT" | "RIGHT" | "ISO";
export type SavedView = {
  position: THREE.Vector3; rotation: THREE.Quaternion; up: THREE.Vector3;
  target: THREE.Vector3; zoom: number;
};

export function saveView(camera: THREE.OrthographicCamera, target: THREE.Vector3): SavedView {
  return { position: camera.position.clone(), rotation: camera.quaternion.clone(), up: camera.up.clone(), target: target.clone(), zoom: camera.zoom };
}

export function restoreView(camera: THREE.OrthographicCamera, target: THREE.Vector3, saved: SavedView): void {
  camera.position.copy(saved.position); camera.quaternion.copy(saved.rotation); camera.up.copy(saved.up);
  target.copy(saved.target); camera.zoom = saved.zoom;
  camera.updateProjectionMatrix(); camera.updateMatrixWorld(true);
}

/** The screen centre at the pivot's depth, even when an entity pivot is off axis. */
export function viewFocus(camera: THREE.Camera, target: THREE.Vector3): THREE.Vector3 {
  const direction = camera.getWorldDirection(new THREE.Vector3());
  return camera.position.clone().addScaledVector(direction, target.clone().sub(camera.position).dot(direction));
}

export function orientView(camera: THREE.OrthographicCamera, target: THREE.Vector3,
  focus: THREE.Vector3, normal: THREE.Vector3, up: THREE.Vector3): void {
  const distance = Math.max(Math.abs(target.clone().sub(camera.position).dot(camera.getWorldDirection(new THREE.Vector3()))), 1);
  target.copy(focus);
  camera.position.copy(focus).addScaledVector(normal, distance);
  camera.up.copy(up); camera.lookAt(focus); camera.updateMatrixWorld(true);
}

/** Align to the nearest side with the shortest camera rotation, preserving roll.
 * Plane orientation remains modeling truth; choosing a viewing side never flips it.
 */
export function orientPlaneView(camera: THREE.OrthographicCamera, target: THREE.Vector3,
  focus: THREE.Vector3, normal: THREE.Vector3): void {
  const back = camera.getWorldDirection(new THREE.Vector3()).negate().normalize();
  const nearest = normal.clone().normalize();
  if (back.dot(nearest) < 0) nearest.negate();
  const swing = new THREE.Quaternion().setFromUnitVectors(back, nearest);
  const up = new THREE.Vector3(0, 1, 0).applyQuaternion(camera.quaternion).applyQuaternion(swing);
  orientView(camera, target, focus, nearest, up);
}

/** Orientation primitive preserves scale and screen centre; ISO commands then explicitly fit. */
export function standardView(camera: THREE.OrthographicCamera, target: THREE.Vector3, view: StandardView): void {
  const directions = { TOP: new THREE.Vector3(0, 0, 1), FRONT: new THREE.Vector3(0, -1, 0),
    RIGHT: new THREE.Vector3(1, 0, 0), ISO: new THREE.Vector3(1, -1, 1).normalize() };
  orientView(camera, target, viewFocus(camera, target), directions[view],
    view === "TOP" ? new THREE.Vector3(0, 1, 0) : new THREE.Vector3(0, 0, 1));
}

/** Fit projected bounds, not a perspective field of view or world-origin distance. */
export function fitOrthographicView(camera: THREE.OrthographicCamera, target: THREE.Vector3, bounds: THREE.Box3): void {
  if (bounds.isEmpty()) return;
  const center = bounds.getCenter(new THREE.Vector3());
  const inverse = camera.quaternion.clone().invert();
  const projected = new THREE.Box3();
  for (let i = 0; i < 8; i++) projected.expandByPoint(new THREE.Vector3(
    i & 1 ? bounds.max.x : bounds.min.x, i & 2 ? bounds.max.y : bounds.min.y,
    i & 4 ? bounds.max.z : bounds.min.z).sub(center).applyQuaternion(inverse));
  const size = projected.getSize(new THREE.Vector3());
  camera.zoom = THREE.MathUtils.clamp(Math.min((camera.right - camera.left) / Math.max(size.x * 1.15, 1e-6),
    (camera.top - camera.bottom) / Math.max(size.y * 1.15, 1e-6)), 1e-6, 1e6);
  const distance = Math.max(bounds.getBoundingSphere(new THREE.Sphere()).radius * 2, 1);
  camera.position.copy(center).addScaledVector(camera.getWorldDirection(new THREE.Vector3()), -distance);
  target.copy(center); camera.updateProjectionMatrix(); camera.updateMatrixWorld(true);
}

/** Expand around the actual scene, including geometry behind the previous eye position.
 * Orthographic translation along the view axis does not change screen positions or scale. */
export function updateOrthographicClipping(camera: THREE.OrthographicCamera, bounds: THREE.Box3): void {
  const direction = camera.getWorldDirection(new THREE.Vector3());
  const visibleHeight = (camera.top - camera.bottom) / camera.zoom;
  let minDepth = Infinity, maxDepth = -Infinity;
  if (!bounds.isEmpty()) for (let i = 0; i < 8; i++) {
    const depth = new THREE.Vector3(i & 1 ? bounds.max.x : bounds.min.x,
      i & 2 ? bounds.max.y : bounds.min.y, i & 4 ? bounds.max.z : bounds.min.z)
      .sub(camera.position).dot(direction);
    minDepth = Math.min(minDepth, depth); maxDepth = Math.max(maxDepth, depth);
  }
  const margin = Math.max(visibleHeight * 2, Number.isFinite(minDepth) ? (maxDepth - minDepth) * 0.25 : 0, 1);
  if (Number.isFinite(minDepth)) {
    const shift = margin - minDepth;
    // Also move forward after zooming back in: depth precision must not depend on navigation history.
    if (Math.abs(shift) > Math.max(1e-9, margin * 1e-12)) {
      camera.position.addScaledVector(direction, -shift);
      maxDepth += shift;
    }
  }
  camera.near = 0;
  camera.far = Math.max(Number.isFinite(maxDepth) ? maxDepth + margin : 0, margin * 2);
  camera.updateProjectionMatrix(); camera.updateMatrixWorld(true);
}
