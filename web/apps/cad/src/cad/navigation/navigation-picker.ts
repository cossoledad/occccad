import * as THREE from "three";
import { circularRotationAxis, cylindricalRotationAxis } from "./display-rotation-axis";
import type { CadCamera } from "./camera-rig";
import { worldUnitsPerCssPixel } from "../rendering/viewport-metrics";

export const CAD_GEOMETRY_LAYER = 1;

export type NavigationPick = {
  point: THREE.Vector3;
  distance: number;
  object?: THREE.Object3D;
  objectLabel?: string;
  source: "raycast" | "view-plane";
  axis?: THREE.Vector3;
  axisOrigin?: THREE.Vector3;
  highlight?: { kind: "face" | "edge" | "vertex"; positions: number[] };
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
    private camera: CadCamera,
    private readonly surface: HTMLElement,
    private readonly roots: () => readonly THREE.Object3D[],
    private readonly hoveredPoint?: () => THREE.Vector3 | undefined,
  ) {
    this.raycaster.layers.set(CAD_GEOMETRY_LAYER);
  }

  setCamera(camera:CadCamera):void {this.camera=camera;}

  pickNearest(x: number, y: number, withReference = false): NavigationPick | undefined {
    return this.pick(x, y, withReference, false);
  }

  /** Zoom follows the cursor ray, never a nearby/occluded vertex promoted for rotation. */
  pickSurface(x: number, y: number): NavigationPick | undefined {
    return this.pick(x, y, false, true);
  }

  private pick(x: number, y: number, withReference: boolean, surfaceOnly: boolean): NavigationPick | undefined {
    const width = Math.max(this.surface.clientWidth, 1);
    const height = Math.max(this.surface.clientHeight, 1);
    this.ndc.set((x / width) * 2 - 1, -(y / height) * 2 + 1);
    this.raycaster.setFromCamera(this.ndc, this.camera);
    const roots = [...this.roots()];
    const surfaces: THREE.Object3D[] = [], features: THREE.Object3D[] = [];
    for (const root of roots) root.traverseVisible(object => {
      if (this.isValidGeometry(object)) (object instanceof THREE.Mesh ? surfaces : features).push(object);
    });
    const surfaceHits = this.raycaster.intersectObjects(surfaces, false);
    const surface = surfaceHits[0];
    if (surfaceOnly) return surface ? this.toPick(surface, false) : undefined;
    const bounds = new THREE.Box3();
    if (!surface) for (const root of roots) if (root.visible) bounds.expandByObject(root);
    const focus = surface?.point ?? (bounds.isEmpty() ? new THREE.Vector3() : bounds.getCenter(new THREE.Vector3()));
    const worldPerPixel = worldUnitsPerCssPixel(this.camera, focus,
      {cssWidth: width, cssHeight: height, devicePixelRatio: 1});
    this.raycaster.params.Points = { threshold: worldPerPixel * 6 };
    this.raycaster.params.Line = { threshold: worldPerPixel * 4 };

    const hits = [...surfaceHits, ...this.raycaster.intersectObjects(features, false)];
    const closestSurface = surface?.distance ?? Infinity;
    const hovered=this.hoveredPoint?.();
    if(hovered){
      const projected=hovered.clone().project(this.camera),depth=hovered.clone().sub(this.raycaster.ray.origin).dot(this.raycaster.ray.direction);
      if(Math.hypot((projected.x+1)*width/2-x,(1-projected.y)*height/2-y)<=8&&depth>=0&&depth<=closestSurface+worldPerPixel*2)
        return {point:hovered.clone(),distance:depth,source:"raycast",objectLabel:"preselected-point"};
    }
    const rank = (object: THREE.Object3D) => object instanceof THREE.Points ? 0 : object instanceof THREE.Line ? 1 : 2;
    const hit = hits.filter((hit) => hit.distance <= closestSurface + worldPerPixel * 2)
      .sort((a, b) => rank(a.object) - rank(b.object) || a.distance - b.distance)[0];
    if (!hit) return undefined;
    return this.toPick(hit, withReference);
  }

  private toPick(hit: THREE.Intersection, withReference: boolean): NavigationPick {
    return {
      point: hit.object instanceof THREE.Points
        ? new THREE.Vector3().fromBufferAttribute(hit.object.geometry.getAttribute("position"), hit.index ?? 0).applyMatrix4(hit.object.matrixWorld)
        : hit.point.clone(),
      distance: hit.distance,
      object: hit.object,
      objectLabel: this.objectLabel(hit.object),
      source: "raycast",
      ...(withReference ? navigationReference(hit) : {}),
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
    if (!(object instanceof THREE.Mesh || object instanceof THREE.Points || object instanceof THREE.Line)) return false;
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

/** Display-only reference, never serialized into model selections or revisions. */
function navigationReference(hit: THREE.Intersection): Pick<NavigationPick, "axis" | "axisOrigin" | "highlight"> {
  const object = hit.object as THREE.Mesh | THREE.LineSegments | THREE.Points;
  const geometry = object.geometry, positions: number[] = [];
  const attribute = geometry.getAttribute("position");
  if (!attribute) return {};
  const point = (index: number) => new THREE.Vector3().fromBufferAttribute(attribute, index).applyMatrix4(object.matrixWorld);
  const append = (value: THREE.Vector3) => positions.push(value.x, value.y, value.z);
  if (object instanceof THREE.Points) {
    append(point(hit.index ?? 0)); return { highlight: { kind: "vertex", positions } };
  }
  if (object instanceof THREE.LineSegments) {
    const ids = geometry.userData.navigationEdgeIds as number[] | undefined;
    if (!ids) return {};
    const selected = ids[Math.floor((hit.index ?? 0) / 2)];
    let axis: THREE.Vector3 | undefined, straight = true;
    for (let i = 0; i < ids.length; i++) if (ids[i] === selected) {
      const a = point(i * 2), b = point(i * 2 + 1), direction = b.clone().sub(a).normalize();
      append(a); append(b);
      if (!axis) axis = direction; else if (Math.abs(axis.dot(direction)) < 0.99999) straight = false;
    }
    const samples = Array.from({ length: positions.length / 3 }, (_, i) => new THREE.Vector3().fromArray(positions, i * 3));
    return { ...(straight ? { axis } : circularRotationAxis(samples)), highlight: { kind: "edge", positions } };
  }
  const ids = geometry.userData.navigationFaceIds as number[] | undefined;
  if (!ids || hit.faceIndex == null) return {};
  const selected = ids[hit.faceIndex];
  let axis: THREE.Vector3 | undefined, planar = true;
  const normals: THREE.Vector3[] = [];
  for (let i = 0; i < ids.length; i++) if (ids[i] === selected) {
    const vertices = [0, 1, 2].map((k) => point(geometry.index ? geometry.index.getX(i * 3 + k) : i * 3 + k));
    vertices.forEach(append);
    const normal = vertices[1].clone().sub(vertices[0]).cross(vertices[2].clone().sub(vertices[0])).normalize();
    if (normal.lengthSq() < 0.5) continue;
    normals.push(normal);
    if (!axis) axis = normal; else if (Math.abs(axis.dot(normal)) < 0.99999) planar = false;
  }
  const samples = Array.from({ length: positions.length / 3 }, (_, i) => new THREE.Vector3().fromArray(positions, i * 3));
  return { ...(planar ? { axis } : cylindricalRotationAxis(samples, normals)), highlight: { kind: "face", positions } };
}
