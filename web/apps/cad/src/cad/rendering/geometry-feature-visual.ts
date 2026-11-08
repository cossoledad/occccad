import * as THREE from 'three';
import type {GeometryResultReference} from '../../types';
import type {DisplayArtifact as Artifact} from '../visual/visual-repository';
import {featurePreviewColors,type FeaturePreviewOperation} from './feature-preview';
export function geometryBindingKey(occurrencePath:string|undefined,bodyId:string|undefined,result?:GeometryResultReference){return `${occurrencePath||'root'}/${result?`result:${result.featureId}/${result.resultId}`:`body:${bodyId}`}`}
export function makeCurvePreview(artifact:Artifact,operation:FeaturePreviewOperation){
 const group=new THREE.Group(),positions:number[]=[];
 for(const edge of artifact.mesh.edges??[])for(let i=1;i<edge.points.length;i++)positions.push(...edge.points[i-1],...edge.points[i]);
 if(positions.length)group.add(new THREE.LineSegments(new THREE.BufferGeometry().setAttribute('position',new THREE.Float32BufferAttribute(positions,3)),new THREE.LineBasicMaterial({color:featurePreviewColors[operation],depthWrite:false})));
 const points=(artifact.mesh.topologyVertices??[]).flatMap(p=>p.point);if(points.length)group.add(new THREE.Points(new THREE.BufferGeometry().setAttribute('position',new THREE.Float32BufferAttribute(points,3)),new THREE.PointsMaterial({color:featurePreviewColors[operation],size:5,sizeAttenuation:false,depthWrite:false})));
 return group;
}
