import * as THREE from "three";
import type { SolidDisplaySettings } from "./display-settings";

/** Apply only to a solid's primitives, leaving its parent/tree visibility intact.
 * Wireframe uses CAD edges, not the triangles of the display tessellation. */
export function applySolidDisplaySettings(group: THREE.Group, settings: SolidDisplaySettings): void {
  for (const object of group.children) {
    if (object instanceof THREE.Mesh) object.visible = settings.mode !== "wireframe";
    else if (object instanceof THREE.LineSegments) {
      object.visible = settings.mode === "wireframe" || settings.edges;
      for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
        material.depthTest = settings.mode !== "wireframe";
      }
    } else if (object instanceof THREE.Points) object.visible = settings.vertices;
  }
}
