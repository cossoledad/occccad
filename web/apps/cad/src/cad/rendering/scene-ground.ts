import * as THREE from "three";

export type GroundFootprint = { center: THREE.Vector2; halfSize: number };
export type SceneGridDisplay = { ground:boolean; world:boolean; sketch:boolean; height:number; center:[number,number]; halfSize:number; sketchGridColor:number };

/** A model-sized square. Camera pose and zoom never determine its world size. */
export function groundFootprint(bounds: THREE.Box3, spacing: number): GroundFootprint {
  const step = Number.isFinite(spacing) && spacing > 0 ? spacing : 10;
  const center = bounds.isEmpty() ? new THREE.Vector3() : bounds.getCenter(new THREE.Vector3());
  const size = bounds.isEmpty() ? new THREE.Vector3() : bounds.getSize(new THREE.Vector3());
  const halfSize = Math.ceil(Math.max(size.x, size.y, size.z, step * 5) * 2 / step) * step;
  return { center: new THREE.Vector2(center.x, center.y), halfSize };
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
