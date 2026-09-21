import * as THREE from "three";
import type { DatumPlane, TopologyElementProperties, Vec3 } from "../../types";

export type NormalViewPlane = Pick<DatumPlane,"origin"|"normal"|"uDirection">;
const vector = (v:unknown):v is Vec3 => Array.isArray(v) && v.length===3 && v.every(Number.isFinite);
export function exactNormalViewPlane(properties:TopologyElementProperties):NormalViewPlane|undefined {
  const {origin,normal,xDirection} = properties.properties;
  if (properties.geometryType!=="PLANE" || !vector(origin) || !vector(normal) || !vector(xDirection)) return;
  return {origin,normal,uDirection:xDirection};
}

export function normalViewFrame(plane:NormalViewPlane, placement:THREE.Matrix4) {
  if (![plane.origin,plane.normal,plane.uDirection].every(vector)) return;
  const origin = new THREE.Vector3().fromArray(plane.origin).applyMatrix4(placement);
  const normal = new THREE.Vector3().fromArray(plane.normal).applyMatrix3(new THREE.Matrix3().getNormalMatrix(placement));
  const u = new THREE.Vector3().fromArray(plane.uDirection).transformDirection(placement);
  if (normal.lengthSq()<1e-20) return;
  normal.normalize();
  const up = normal.clone().cross(u);
  if (up.lengthSq()<1e-20 || ![...origin,...normal,...up].every(Number.isFinite)) return;
  return {origin,normal,up:up.normalize()};
}
