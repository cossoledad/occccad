import * as THREE from "three";
export type DisplayRotationAxis = { axis: THREE.Vector3; axisOrigin: THREE.Vector3 };

/** Fit and validate a circle in display samples; never used as authoritative CAD geometry. */
export function circularRotationAxis(points: THREE.Vector3[]): DisplayRotationAxis | undefined {
  if (points.length < 3) return;
  const a = points[0];
  let b = a;
  for (const point of points) if (point.distanceToSquared(a) > b.distanceToSquared(a)) b = point;
  const u = b.clone().sub(a);
  let v = new THREE.Vector3(), normal = new THREE.Vector3();
  for (const point of points) {
    const candidate = point.clone().sub(a), cross = u.clone().cross(candidate);
    if (cross.lengthSq() > normal.lengthSq()) { v = candidate; normal = cross; }
  }
  const denominator = 2 * normal.lengthSq();
  if (denominator < u.lengthSq() ** 2 * 1e-12 || denominator === 0) return;
  const center = a.clone().add(normal.clone().cross(u).multiplyScalar(v.lengthSq())
    .add(v.clone().cross(normal).multiplyScalar(u.lengthSq())).divideScalar(denominator));
  const radius = center.distanceTo(a), axis = normal.normalize(), tolerance = Math.max(radius * 1e-4, 1e-7);
  if (points.some((point) => Math.abs(point.distanceTo(center) - radius) > tolerance
    || Math.abs(point.clone().sub(center).dot(axis)) > tolerance)) return;
  return { axis, axisOrigin: center };
}

/** Cylinder recognition requires both parallel tangent normals and circular projected samples. */
export function cylindricalRotationAxis(points: THREE.Vector3[], normals: THREE.Vector3[]): DisplayRotationAxis | undefined {
  if (!normals.length || !points.length) return;
  let axis = new THREE.Vector3();
  for (const normal of normals) {
    const candidate = normals[0].clone().cross(normal);
    if (candidate.lengthSq() > axis.lengthSq()) axis = candidate;
  }
  if (axis.lengthSq() < 1e-6) return;
  axis.normalize();
  if (normals.some((normal) => Math.abs(normal.dot(axis)) > 1e-4)) return;
  const anchor = points[0];
  const projected = points.map((point) => point.clone().addScaledVector(axis, -point.clone().sub(anchor).dot(axis)));
  const circle = circularRotationAxis(projected);
  return circle ? { axis, axisOrigin: circle.axisOrigin } : undefined;
}
