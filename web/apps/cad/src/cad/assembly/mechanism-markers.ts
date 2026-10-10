import * as THREE from 'three';
import type {Mechanism} from './motion-study';
import {analysisArrow} from '../rendering/analysis-guides';
import {makeConstraintDimensionLabel} from '../rendering/sketch-constraint-renderer';

// Derived definition guides belong to the viewport engine's analysis pass.
// Neither topology identities nor source geometry are manufactured here.
export function makeMechanismMarkers(m:Mechanism,groups:ReadonlyMap<string,THREE.Object3D>,selected?:string):THREE.Group {
 const root=new THREE.Group();root.name='mechanism-definitions';
 for(const j of m.joints){
  if(selected&&selected!==m.id&&selected!==j.id&&!selected.startsWith(j.id+'/'))continue;
  const unit=groups.get(j.first.instanceId);if(!unit)continue;
  unit.updateWorldMatrix(true,false);
  const local=new THREE.Matrix4().compose(new THREE.Vector3(...j.first.frame.translation),new THREE.Quaternion(...j.first.frame.rotation),new THREE.Vector3(1,1,1));
  const matrix=unit.matrixWorld.clone().multiply(local),p=new THREE.Vector3().setFromMatrixPosition(matrix),z=new THREE.Vector3(0,0,1).transformDirection(matrix);
  const marker=new THREE.Group();marker.userData.jointId=j.id;
  if(j.kind==='GROUND'){
   const lock=new THREE.Mesh(new THREE.OctahedronGeometry(2),new THREE.MeshBasicMaterial({color:0xe0a020,depthTest:false}));lock.position.copy(p);marker.add(lock);
  }else{
   marker.add(analysisArrow(z,p,'rotation',20));
   if(j.kind!=='PRISMATIC'){
    const plane=new THREE.Mesh(new THREE.PlaneGeometry(8,8),new THREE.MeshBasicMaterial({color:0x418de8,transparent:true,opacity:.25,side:THREE.DoubleSide,depthTest:false,depthWrite:false}));
    plane.position.copy(p);plane.quaternion.setFromRotationMatrix(matrix.clone().extractRotation(matrix));marker.add(plane);
   }
  }
  const label=makeConstraintDimensionLabel(j.name);label.position.copy(p).addScaledVector(z,4);marker.add(label);root.add(marker);
 }
 return root;
}
