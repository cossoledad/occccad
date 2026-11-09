import * as THREE from 'three';
import {curvatureRange,type ShapeQualityReport,type CurvatureMode} from '../analysis/shape-quality';
export class CurvatureOverlay {
 readonly root=new THREE.Group();
 constructor(){this.root.name='curvature-analysis';this.root.userData.transient=true;this.root.userData.pickable=false}
 clear():void {this.root.traverse(object=>{if(object instanceof THREE.Mesh||object instanceof THREE.Line||object instanceof THREE.Points){object.geometry.dispose();for(const material of Array.isArray(object.material)?object.material:[object.material])material.dispose()}});this.root.clear()}
 show(report:ShapeQualityReport,mode:CurvatureMode,gain:number):void {
  this.clear();if(!report.result.complete||!Number.isFinite(gain)||gain<=0)return;
  const samples=report.result.samples,range=curvatureRange(report,mode);if(!range)return;
  const box=new THREE.Box3().setFromPoints(samples.map(s=>new THREE.Vector3(...s.position))),size=box.getSize(new THREE.Vector3()).length();
  const maximum=Math.max(...samples.filter(s=>!s.surface).map(s=>s.maximum),0),scale=maximum>0?Math.max(size,1)*.15/maximum*gain:0;
  const positions:number[]=[],colors:number[]=[],segments:THREE.Vector3[]=[];
  for(const s of samples){if(s.surface){positions.push(...s.position);const t=range[1]===range[0]?.5:Math.max(0,Math.min(1,(s[mode]-range[0])/(range[1]-range[0]))),color=new THREE.Color().setHSL((1-t)*.67,.9,.55);colors.push(color.r,color.g,color.b)}else {const p=new THREE.Vector3(...s.position);segments.push(p,p.clone().addScaledVector(new THREE.Vector3(...s.normal),s.maximum*scale))}}
  if(positions.length){const g=new THREE.BufferGeometry();g.setAttribute('position',new THREE.Float32BufferAttribute(positions,3));g.setAttribute('color',new THREE.Float32BufferAttribute(colors,3));const material=new THREE.PointsMaterial({vertexColors:true,size:4,sizeAttenuation:false,depthTest:true,depthWrite:false,toneMapped:false});
   material.onBeforeCompile=shader=>{shader.vertexShader=shader.vertexShader.replace('#include <project_vertex>','#include <project_vertex>\ngl_Position.z -= 0.00005 * gl_Position.w;');shader.fragmentShader=shader.fragmentShader.replace('#include <clipping_planes_fragment>','#include <clipping_planes_fragment>\nif (length(gl_PointCoord - vec2(0.5)) > 0.5) discard;')};material.customProgramCacheKey=()=> 'curvature-circular-samples-v1';
   const points=new THREE.Points(g,material);points.renderOrder=8;this.root.add(points)}
  if(segments.length){const lines=new THREE.LineSegments(new THREE.BufferGeometry().setFromPoints(segments),new THREE.LineBasicMaterial({color:0x65e5e0,depthTest:false,depthWrite:false,toneMapped:false}));lines.renderOrder=8;this.root.add(lines)}
 }
 dispose():void {this.clear();this.root.removeFromParent()}
}
