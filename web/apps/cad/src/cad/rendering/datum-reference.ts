import * as THREE from "three";
import type { DatumAxis, DatumPlane, Vec3 } from "../../types";
import { makeDatumReferenceLine, makeOcclusionVisibleSegments } from "./interaction-highlight";

export type DatumPreview = { axis?: DatumAxis; plane?: DatumPlane; translation?: Vec3; rotation?: [number, number, number, number] };

// A bidirectional solid axis with an origin tick and a positive-direction arrow.
export function makeDatumAxisReference(axis: DatumAxis, color = 0x7065bd): THREE.Group {
  const group = new THREE.Group();
  group.position.fromArray(axis.origin);
  group.quaternion.setFromUnitVectors(new THREE.Vector3(1, 0, 0), new THREE.Vector3().fromArray(axis.direction).normalize());
  group.add(makeDatumReferenceLine([new THREE.Vector3(-0.65, 0, 0), new THREE.Vector3(1, 0, 0)], color));
  const v = (x: number, y: number, z = 0) => new THREE.Vector3(x, y, z);
  const details = makeOcclusionVisibleSegments([
    [v(0, -.08), v(0, .08)], [v(0, 0, -.08), v(0, 0, .08)],
    [v(.82, .08), v(1, 0)], [v(.82, -.08), v(1, 0)],
    [v(.82, 0, .08), v(1, 0)], [v(.82, 0, -.08), v(1, 0)],
  ], color, 2.25);
  details.renderOrder = 93;
  group.add(details);
  return group;
}
