import * as THREE from "three";

/** Reference axes inherit screen-size scaling from their parents. Test in world
 * space so the Line raycaster's local threshold cannot shrink the hit area. */
export function raycastDatumAxis(this: THREE.Line, raycaster: THREE.Raycaster, hits: THREE.Intersection[]): void {
  const positions = this.geometry.getAttribute("position");
  if (!positions || positions.count < 2) return;
  const start = new THREE.Vector3().fromBufferAttribute(positions, 0).applyMatrix4(this.matrixWorld);
  const end = new THREE.Vector3().fromBufferAttribute(positions, 1).applyMatrix4(this.matrixWorld);
  const onRay = new THREE.Vector3(), onAxis = new THREE.Vector3();
  const distanceSquared = raycaster.ray.distanceSqToSegment(start, end, onRay, onAxis);
  const threshold = raycaster.params.Line?.threshold ?? 1;
  if (distanceSquared > threshold * threshold) return;
  const distance = raycaster.ray.origin.distanceTo(onRay);
  if (distance < raycaster.near || distance > raycaster.far) return;
  hits.push({ distance, point: onAxis, object: this, index: 0 });
}
