import {CATIA_VISUAL_THEME} from "./cad-visual-theme";
import * as THREE from "three";
import type { DatumAxis, DatumPlane, Vec3 } from "../../types";
import { makeDatumReferenceLine, makeOcclusionVisibleSegments } from "./interaction-highlight";

export type DatumPreview = { showFrame?:boolean; axis?: DatumAxis; plane?: DatumPlane; translation?: Vec3; rotation?: [number, number, number, number] };

export const DATUM_AXIS_PIXELS = 72;
export const DATUM_PLANE_PIXELS=46;
export const STANDARD_REFERENCE_RANGE=[.18,.90] as const;
export function makeDatumPlaneReference(plane:DatumPlane,material:THREE.ShaderMaterial):THREE.Mesh {
 const geometry=new THREE.PlaneGeometry(.72,.72);if(plane.plane!=="CUSTOM")geometry.translate(.54,.54,0);
 const mesh=new THREE.Mesh(geometry,material),normal=new THREE.Vector3().fromArray(plane.normal).normalize(),u=new THREE.Vector3().fromArray(plane.uDirection).normalize();
 mesh.position.fromArray(plane.origin);mesh.quaternion.setFromRotationMatrix(new THREE.Matrix4().makeBasis(u,normal.clone().cross(u).normalize(),normal));mesh.renderOrder=90;return mesh;
}

export function makeReferenceArrow(color:number):THREE.Mesh<THREE.ConeGeometry,THREE.MeshBasicMaterial> {
  const arrow=new THREE.Mesh(new THREE.ConeGeometry(.035,.14,12),new THREE.MeshBasicMaterial({color,depthTest:false,depthWrite:false,toneMapped:false}));
  arrow.rotation.z=-Math.PI/2;arrow.position.x=.93;arrow.renderOrder=94;
  arrow.material.userData.baseColor=color;
  return arrow;
}

// The preview and saved axis use the same screen-sized construction reference.
export function makeDatumAxisReference(axis: DatumAxis, color = CATIA_VISUAL_THEME.reference): THREE.Group {
  const group = new THREE.Group();
  group.position.fromArray(axis.origin);
  group.quaternion.setFromUnitVectors(new THREE.Vector3(1, 0, 0), new THREE.Vector3().fromArray(axis.direction).normalize());
  const stem=makeDatumReferenceLine([new THREE.Vector3(-0.65, 0, 0), new THREE.Vector3(.88, 0, 0)], color,true);
  stem.material.linewidth=1.5;
  group.add(stem,makeReferenceArrow(color));
  const v = (x: number, y: number, z = 0) => new THREE.Vector3(x, y, z);
  const details = makeOcclusionVisibleSegments([
    [v(0, -.035), v(0, .035)], [v(0, 0, -.035), v(0, 0, .035)],
  ], color, 1.5);
  details.renderOrder = 93;
  group.add(details);
  return group;
}
