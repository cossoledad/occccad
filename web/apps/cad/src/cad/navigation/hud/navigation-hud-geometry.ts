import * as THREE from "three";
import { CATIA_NAVIGATION_COLOR } from "../navigation-visuals";

export const SPHERE_CURSOR_SPAN = 52;
export const SPHERE_CURSOR_SEGMENTS = 24;

/** Invert the screen arc length of a projected great circle (an ellipse). */
function greatCircleHalfAngle(radius: number, radial: number, halfSpan: number): number {
  if (radial === radius) return halfSpan / radius;
  if (radial === 0) return Math.asin(Math.min(1, halfSpan / radius));
  let low = 0, high = Math.asin(Math.min(1, halfSpan / radius));
  const speed = (angle: number) => Math.hypot(radial * Math.sin(angle), radius * Math.cos(angle));
  for (let i = 0; i < 22; i++) {
    const angle = (low + high) / 2, step = angle / 8;
    let length = speed(0) + speed(angle);
    for (let j = 1; j < 8; j++) length += (j % 2 ? 4 : 2) * speed(j * step);
    if (length * step / 3 < halfSpan) low = angle; else high = angle;
  }
  return (low + high) / 2;
}

/** The anchor follows the same hemisphere/rim mapping as the trackball.
 * The circumferential arm is a projected great circle: at the rim it is the
 * outer circle itself. The radial arm shifts inwards to stay inside the disk.
 */
export function sphereCursorVertices(x: number, y: number, radius: number): number[] {
  const r = Math.max(1, radius), distance = Math.hypot(x, y), radial = Math.min(distance, r);
  const nx = distance > 1e-9 ? x / distance : 1, ny = distance > 1e-9 ? y / distance : 0;
  const span = Math.min(SPHERE_CURSOR_SPAN, 2 * r);
  const halfAngle = greatCircleHalfAngle(r, radial, span / 2);
  const positions: number[] = [];
  for (let i = 0; i < SPHERE_CURSOR_SEGMENTS; i++) for (const end of [i, i + 1]) {
    const angle = (end / SPHERE_CURSOR_SEGMENTS * 2 - 1) * halfAngle;
    const along = radial * Math.cos(angle), across = r * Math.sin(angle);
    positions.push(nx * along - ny * across, ny * along + nx * across, 0);
  }
  const radialEnd = Math.min(r, radial + span / 2), radialStart = radialEnd - span;
  for (let i = 0; i < SPHERE_CURSOR_SEGMENTS; i++) for (const end of [i, i + 1]) {
    const along = radialStart + span * end / SPHERE_CURSOR_SEGMENTS;
    positions.push(nx * along, ny * along, 0);
  }
  return positions;
}

export function pivotMarkerGeometry(): THREE.BufferGeometry {
  const positions = [
    -22,0,0, -5,0,0, 5,0,0, 22,0,0,
    0,-17,0, 0,-5,0, 0,5,0, 0,17,0,
    0,0,-8, 0,0,8,
  ];
  const xColor = new THREE.Color(CATIA_NAVIGATION_COLOR), yColor = new THREE.Color(0x70d6e3);
  const yellow = new THREE.Color(0xffdf55);
  const colors = Array.from({length: positions.length / 3}, (_, i) =>
    (i >= 8 ? yellow : (i < 4 ? xColor : yColor).clone()
      .multiplyScalar(i < 2 || i === 4 || i === 5 ? .55 : 1)).toArray()).flat();
  return new THREE.BufferGeometry().setAttribute("position", new THREE.Float32BufferAttribute(positions,3))
    .setAttribute("color", new THREE.Float32BufferAttribute(colors,3));
}
