import * as THREE from 'three';
import type {GeometryResultReference} from '../../types';
import type {DisplayArtifact as Artifact} from '../visual/visual-repository';
import {featurePreviewColors,type FeaturePreviewOperation} from './feature-preview';
import {LineSegments2} from 'three/addons/lines/LineSegments2.js';
import {LineSegmentsGeometry} from 'three/addons/lines/LineSegmentsGeometry.js';
import {makeHighlightLineMaterial} from './interaction-highlight';
import {registerScreenLineUpdate} from './screen-space-lines';
import {meshTopologyIndex} from './mesh-topology-index';
export function geometryBindingKey(occurrencePath:string|undefined,bodyId:string|undefined,result?:GeometryResultReference){return `${occurrencePath||'root'}/${result?`result:${result.featureId}/${result.resultId}`:`body:${bodyId}`}`}
export function makeCurvePreview(artifact:Artifact,operation:FeaturePreviewOperation){
 const group=new THREE.Group(),positions=meshTopologyIndex(artifact.mesh).edgeSegments();
 if(positions.length){
  const line=new LineSegments2(new LineSegmentsGeometry().setPositions(positions),makeHighlightLineMaterial(new THREE.Color(featurePreviewColors[operation]).getHex(),2.5));
  line.name='feature-preview-curve';line.renderOrder=95;
  registerScreenLineUpdate(line,(_camera,width,height)=>line.material.resolution.set(width,height));group.add(line);
 }
 const points=(artifact.mesh.topologyVertices??[]).flatMap(p=>p.point);if(points.length){const marker=new THREE.Points(new THREE.BufferGeometry().setAttribute('position',new THREE.Float32BufferAttribute(points,3)),new THREE.PointsMaterial({color:featurePreviewColors[operation],size:5,sizeAttenuation:false,depthTest:false,depthWrite:false}));marker.renderOrder=95;group.add(marker)}
 return group;
}
