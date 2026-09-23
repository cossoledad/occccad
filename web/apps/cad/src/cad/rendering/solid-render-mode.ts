import * as THREE from "three";
import type { RenderMode } from "./display-settings";

/** Apply only to a solid's primitives, leaving its parent/tree visibility intact.
 * Wireframe uses CAD edges, not the triangles of the display tessellation. */
export function applySolidRenderMode(group: THREE.Group, mode: RenderMode): void {
  for (const object of group.children) {
    if (object instanceof THREE.Mesh) object.visible = mode !== "wireframe";
    else if (object instanceof THREE.LineSegments) {
      object.visible = mode !== "smooth";
      for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
        material.depthTest = mode !== "wireframe";
      }
    } else if (object instanceof THREE.Points) object.visible = mode === "default";
  }
}
