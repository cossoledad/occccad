import * as THREE from "three";

export type GroundFootprint = { center: THREE.Vector2; halfSize: number };
export type SceneGridDisplay = { ground:boolean; world:boolean; sketch:boolean; height:number; center:[number,number]; halfSize:number; sketchGridColor:number };

/** A model-sized square ending on major grid lines; independent of the camera. */
export function groundFootprint(bounds: THREE.Box3, spacing: number): GroundFootprint {
  const step = Number.isFinite(spacing) && spacing > 0 ? spacing : 10;
  const center = bounds.isEmpty() ? new THREE.Vector3() : bounds.getCenter(new THREE.Vector3());
  const size = bounds.isEmpty() ? new THREE.Vector3() : bounds.getSize(new THREE.Vector3());
  const halfSize = Math.ceil(Math.max(size.x, size.y, size.z, step * 5) * 2 / step) * step;
  const major = step * 10;
  const minX = Math.floor((center.x - halfSize) / major), maxX = Math.ceil((center.x + halfSize) / major);
  const minY = Math.floor((center.y - halfSize) / major), maxY = Math.ceil((center.y + halfSize) / major);
  const cells = Math.max(maxX - minX, maxY - minY);
  // Enclose the original footprint, keep a square, and move its centre as little as possible.
  const x = THREE.MathUtils.clamp(Math.round(center.x / major - cells / 2), maxX - cells, minX);
  const y = THREE.MathUtils.clamp(Math.round(center.y / major - cells / 2), maxY - cells, minY);
  return { center: new THREE.Vector2((x + cells / 2) * major, (y + cells / 2) * major), halfSize: cells * major / 2 };
}

/** Only sketch geometry contributes: labels, screen-sized handles and constraints do not. */
export function includeVisibleSketchBounds(bounds: THREE.Box3, content: THREE.Object3D): void {
  content.updateWorldMatrix(true, true);
  content.traverseVisible(object => {
    const data = object.userData;
    if (data.kind !== "visual" || !data.featureId || !["POINT", "CURVE"].includes(data.visualType)) return;
    const geometry = (object as THREE.Mesh).geometry;
    if (!geometry) return;
    if (!geometry.boundingBox) geometry.computeBoundingBox();
    if (geometry.boundingBox) bounds.union(geometry.boundingBox.clone().applyMatrix4(object.matrixWorld));
  });
}

export function sceneGroundVisible(environment: boolean, sceneGrid: boolean, sketchEditing: boolean): boolean {
  return environment && sceneGrid && !sketchEditing;
}
