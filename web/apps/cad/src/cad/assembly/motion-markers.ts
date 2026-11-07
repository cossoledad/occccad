import * as THREE from "three";
import {motionDegrees,type MotionPresentation} from "./motion-presentation";
import {analysisRotationArc,analysisArrow,disposeAnalysisGuides,type AnalysisRole} from "../rendering/analysis-guides";

// Presentation-only objects: no SelectionIndex entry, shared materials, mesh
// download, physical equation or persistent identity.
export function makeMotionMarkers(motion:MotionPresentation,length:number):THREE.Group {
 const group=new THREE.Group();group.name="remaining-motion";
 const arrow=(d:THREE.Vector3,p:THREE.Vector3,role:AnalysisRole)=>analysisArrow(d,p,role,length);
 const f=motion.freedom;if(!f)return group;
 const origin=new THREE.Vector3().fromArray(f.linearizationPose.translation);
 const degrees=motionDegrees(motion)!;
 for(const item of degrees.directions){
  const role:AnalysisRole=motion.activeDirections?.includes(item.id)?"highlight":"freedom";
  const d=new THREE.Vector3().fromArray(item.direction).normalize();
  if(item.kind==="translation"){
   for(const sign of [-1,1])group.add(arrow(d.clone().multiplyScalar(sign),origin,role));
  }else{
   const p=new THREE.Vector3().fromArray(item.axisPoint!);p.addScaledVector(d,origin.clone().sub(p).dot(d));
   group.add(analysisRotationArc(d,p,length,role));
  }
 }
 return group;
}
export function disposeMotionMarkers(group:THREE.Group|undefined):void {
 disposeAnalysisGuides(group);
}
