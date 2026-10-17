import * as THREE from "three";
import { CATIA_NAVIGATION_COLOR } from "../navigation-visuals";

export const SPHERE_CURSOR_SPAN = 52;
export const SPHERE_CURSOR_SEGMENTS = 24;

/** Screen-space spherical cue: fixed arc length, with curvature increasing towards the rim.
 * A bounded curvature avoids the foreshortening/singularity of a projected 3D arc.
 */
export function sphereCursorVertices(x: number, y: number, radius: number): number[] {
  const r = Math.max(1, radius), distance = Math.hypot(x, y);
  const scale = 1 / Math.max(r, distance), u = x * scale, v = y * scale;
  const denominator = Math.max(r * (1 - u * u - v * v), SPHERE_CURSOR_SPAN * .85);
  const positions: number[] = [];
  for (const [tx, ty, curvature] of [[1, 0, -v / denominator], [0, 1, u / denominator]]) {
    for (let i = 0; i < SPHERE_CURSOR_SEGMENTS; i++) for (const end of [i, i + 1]) {
      const s = (end / SPHERE_CURSOR_SEGMENTS - .5) * SPHERE_CURSOR_SPAN;
      const along = Math.abs(curvature) < 1e-8 ? s : Math.sin(curvature * s) / curvature;
      const bend = Math.abs(curvature) < 1e-8 ? curvature * s * s / 2 : (1 - Math.cos(curvature * s)) / curvature;
      positions.push(x + tx * along - ty * bend, y + ty * along + tx * bend, 0);
    }
  }
  return positions;
}

export function pivotMarkerGeometry(): THREE.BufferGeometry {
  const positions = [
    -22,0,0, -5,0,0, 5,0,0, 22,0,0,
    0,-17,0, 0,-5,0, 0,5,0, 0,17,0,
    0,0,-8, 0,0,8,
  ];
  // Four arrow wings keep positive directions legible from either side of an axis.
  for (const wing of [[18,2,0],[18,-2,0],[18,0,2],[18,0,-2]]) positions.push(...wing,22,0,0);
  for (const wing of [[2,13,0],[-2,13,0],[0,13,2],[0,13,-2]]) positions.push(...wing,0,17,0);
  const positive = new THREE.Color(CATIA_NAVIGATION_COLOR), negative = positive.clone().multiplyScalar(.55);
  const yellow = new THREE.Color(0xffdf55);
  const colors = Array.from({length: positions.length / 3}, (_, i) =>
    (i === 8 || i === 9 ? yellow : i < 2 || i === 4 || i === 5 ? negative : positive).toArray()).flat();
  return new THREE.BufferGeometry().setAttribute("position", new THREE.Float32BufferAttribute(positions,3))
    .setAttribute("color", new THREE.Float32BufferAttribute(colors,3));
}
