import * as THREE from 'three';
import type {SpatialAnalysis} from '../assembly/spatial-analysis';
import {CATIA_VISUAL_THEME as theme} from './cad-visual-theme';

export function sectionClippingPlanes(d:SpatialAnalysis):THREE.Plane[]{
 if(d.cutMode==='NONE')return [];
 const q=new THREE.Quaternion().fromArray(d.frame.rotation),origin=new THREE.Vector3().fromArray(d.frame.translation);
 if(d.sectionType==='PLANE')return [new THREE.Plane().setFromNormalAndCoplanarPoint(new THREE.Vector3(0,0,d.cutMode==='KEEP_INSIDE'?1:-1).applyQuaternion(q),origin)];
 const axes:[THREE.Vector3,number][]=[[new THREE.Vector3(0,0,1),d.size[2]]];
 if(d.sectionType==='BOX')axes.push([new THREE.Vector3(1,0,0),d.size[0]],[new THREE.Vector3(0,1,0),d.size[1]]);
 return axes.flatMap(([axis,size])=>[-1,1].map(side=>{const n=axis.clone().applyQuaternion(q).multiplyScalar(side);return new THREE.Plane().setFromNormalAndCoplanarPoint(n.clone().multiplyScalar(d.cutMode==='KEEP_OUTSIDE'?1:-1),origin.clone().addScaledVector(n,size/2))}));
}
export function sectionRepresentation(d:SpatialAnalysis,editing=false):THREE.Group {
 const group=new THREE.Group();group.name='section-representation';group.position.fromArray(d.frame.translation);group.quaternion.fromArray(d.frame.rotation);
 const offsets=d.sectionType==='SLICE'?[-d.size[2]/2,d.size[2]/2,...(editing?[0]:[])]:[0];
 if(d.sectionType!=='BOX'||editing)for(const z of offsets){
  const mesh=new THREE.Mesh(new THREE.PlaneGeometry(d.size[0],d.size[1]),new THREE.MeshBasicMaterial({color:theme.selected,side:THREE.DoubleSide,transparent:true,opacity:.045,depthWrite:false}));mesh.position.z=z;group.add(mesh);
  const outline=new THREE.LineSegments(new THREE.EdgesGeometry(mesh.geometry),new THREE.LineBasicMaterial({color:theme.selected,transparent:true,opacity:.65,depthWrite:false}));outline.position.z=z;group.add(outline);
  const hidden=new THREE.LineSegments(outline.geometry.clone(),new THREE.LineDashedMaterial({color:theme.selected,transparent:true,opacity:.16,depthTest:true,depthFunc:THREE.GreaterDepth,depthWrite:false,dashSize:2,gapSize:2}));hidden.position.z=z;hidden.computeLineDistances();hidden.userData.sectionOccluded=true;group.add(hidden);
 }
 if(d.sectionType!=='PLANE'){const box=new THREE.BoxGeometry(...d.size);const outline=new THREE.LineSegments(new THREE.EdgesGeometry(box),new THREE.LineBasicMaterial({color:theme.selected,transparent:true,opacity:.65,depthWrite:false}));box.dispose();group.add(outline)}
 return group;
}
export function disposeSectionObjects(group?:THREE.Object3D){group?.traverse(o=>{const mesh=o as THREE.Mesh;if(mesh.geometry)mesh.geometry.dispose();if(mesh.material)for(const material of Array.isArray(mesh.material)?mesh.material:[mesh.material])material.dispose()});group?.removeFromParent()}
