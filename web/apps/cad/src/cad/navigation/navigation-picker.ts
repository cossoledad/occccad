import * as THREE from "three";
import type { CadCamera } from "./camera-rig";

export const CAD_GEOMETRY_LAYER = 1;

export type NavigationPick = {
  point: THREE.Vector3;
  distance: number;
  object?: THREE.Object3D;
  objectLabel?: string;
  source: "raycast" | "view-plane";
};

export function markNavigationPickable(object: THREE.Object3D, pickable = true): void {
  object.userData.navigationPickable = pickable;
  if (pickable) object.layers.enable(CAD_GEOMETRY_LAYER);
  else object.layers.disable(CAD_GEOMETRY_LAYER);
}

/** Display-geometry raycast only; this intentionally has no topology/FaceId dependency. */
export class NavigationPicker {
  private readonly raycaster = new THREE.Raycaster();
  private readonly ndc = new THREE.Vector2();

  constructor(
    private readonly camera: CadCamera,
    private readonly surface: HTMLElement,
    private readonly roots: () => readonly THREE.Object3D[],
  ) {
    this.raycaster.layers.set(CAD_GEOMETRY_LAYER);
  }

  pickNearest(x: number, y: number): NavigationPick | undefined {
    const width = Math.max(this.surface.clientWidth, 1);
    const height = Math.max(this.surface.clientHeight, 1);
    this.ndc.set((x / width) * 2 - 1, -(y / height) * 2 + 1);
    this.raycaster.setFromCamera(this.ndc, this.camera);

    const hit = this.raycaster.intersectObjects([...this.roots()], true)
      .filter((intersection) => this.isValidGeometry(intersection.object))
      .sort((left, right) => left.distance - right.distance)[0];
    if (!hit) return undefined;
    return {
      point: hit.point.clone(),
      distance: hit.distance,
      object: hit.object,
      objectLabel: this.objectLabel(hit.object),
      source: "raycast",
    };
  }

  /**
   * Empty-background fallback at the current pivot depth. The plane is normal
   * to the camera, so centering this point is equivalent to screen-space pan
   * without inventing world-origin depth.
   */
  pickViewPlane(x: number, y: number, planePoint: THREE.Vector3): NavigationPick | undefined {
    const width = Math.max(this.surface.clientWidth, 1);
    const height = Math.max(this.surface.clientHeight, 1);
    this.ndc.set((x / width) * 2 - 1, -(y / height) * 2 + 1);
    this.raycaster.setFromCamera(this.ndc, this.camera);
    const normal = this.camera.getWorldDirection(new THREE.Vector3()).normalize();
    const plane = new THREE.Plane().setFromNormalAndCoplanarPoint(normal, planePoint);
    const point = this.raycaster.ray.intersectPlane(plane, new THREE.Vector3());
    if (!point) return undefined;
    return {
      point,
      distance: point.distanceTo(this.camera.position),
      source: "view-plane",
    };
  }

  private isValidGeometry(object: THREE.Object3D): boolean {
    if (!(object instanceof THREE.Mesh)) return false;
    const materials = Array.isArray(object.material) ? object.material : [object.material];
    if (!materials.some((material) => material.visible)) return false;
    for (let current: THREE.Object3D | null = object; current; current = current.parent) {
      if (!current.visible || current.userData.navigationPickable === false) return false;
    }
    return object.userData.navigationPickable === true;
  }

  private objectLabel(object: THREE.Object3D): string {
    for (let current: THREE.Object3D | null = object; current; current = current.parent) {
      if (typeof current.userData.id === "string") {
        return `${String(current.userData.kind ?? "geometry")}:${current.userData.id}`;
      }
      if (current.name) return current.name;
    }
    return `${object.type}:${object.id}`;
  }
}
