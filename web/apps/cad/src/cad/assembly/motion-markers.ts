import * as THREE from "three";
import type {MotionPresentation} from "./motion-presentation";

// Presentation-only objects: no SelectionIndex entry, shared materials, mesh
// download, physical equation or persistent identity.
export function makeMotionMarkers(motion:MotionPresentation,length:number):THREE.Group {
 const group=new THREE.Group();group.name="remaining-motion";
 const arrow=(d:THREE.Vector3,p:THREE.Vector3,color:number)=>{const a=new THREE.ArrowHelper(d,p,length,color);a.line.geometry=a.line.geometry.clone();a.cone.geometry=a.cone.geometry.clone();return a;};
 const f=motion.freedom;if(!f)return group;
 const origin=new THREE.Vector3().fromArray(f.linearizationPose.translation);
 // A six-dimensional component gauge is an authoritative overall rigid
 // motion, not six independent freedoms copied into the relative body basis.
 const overallRigidFree=f.kind===0&&motion.gaugeDof===6;
 const axes:[number,number,number][]=[[1,0,0],[0,1,0],[0,0,1]];
 for(const d of overallRigidFree?axes:f.translationDirections){const direction=new THREE.Vector3().fromArray(d).normalize();
  for(const sign of [-1,1])group.add(arrow(direction.clone().multiplyScalar(sign),origin,0x60ccff));}
 const rotations=overallRigidFree?axes.map(direction=>({direction,axisPoint:f.linearizationPose.translation,pitch:0})):f.rotations;
 for(const r of rotations){const d=new THREE.Vector3().fromArray(r.direction).normalize();
  // Canonical closest point to the body origin: equivalent points along an
  // offset screw axis generate the same displayed line.
  const p=new THREE.Vector3().fromArray(r.axisPoint);p.addScaledVector(d,origin.clone().sub(p).dot(d));
  for(const sign of [-1,1])group.add(arrow(d.clone().multiplyScalar(sign),p,0xffc060));}
 return group;
}
export function disposeMotionMarkers(group:THREE.Group|undefined):void {
 group?.removeFromParent();group?.traverse(object=>{const renderable=object as THREE.Mesh;renderable.geometry?.dispose();
  const material=renderable.material;if(material)(Array.isArray(material)?material:[material]).forEach(m=>m.dispose());});
}
