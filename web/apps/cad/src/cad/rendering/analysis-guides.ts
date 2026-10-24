import * as THREE from "three";
import {Line2} from "three/addons/lines/Line2.js";
import {LineGeometry} from "three/addons/lines/LineGeometry.js";
import {LineMaterial} from "three/addons/lines/LineMaterial.js";
import {worldUnitsPerCssPixel,type ViewportMetrics} from "./viewport-metrics";
import {colorNumber,palette} from "../../design/visual-tokens";

export type AnalysisRole="translation"|"rotation";
const roles={translation:palette.hover,rotation:palette.preview};
const updates=new WeakMap<THREE.Object3D,(camera:THREE.Camera,metrics:ViewportMetrics)=>void>();

/** Owned primitives, rendered in a separate post-model pass. No picking or
 * model bounds. Camera updates change transforms/uniforms, not resources. */
export function analysisArrow(direction:THREE.Vector3,anchor:THREE.Vector3,role:AnalysisRole,initialLength:number):THREE.ArrowHelper{
 const arrow=new THREE.ArrowHelper(direction,anchor,initialLength,colorNumber(roles[role]));
 // ArrowHelper's cone geometry is global; own a clone before any disposal.
 arrow.cone.geometry=arrow.cone.geometry.clone();
 arrow.line.removeFromParent(); // shared native line geometry is never disposed
 (arrow.line.material as THREE.Material).dispose(); // instance-owned, now unused
 const geometry=new LineGeometry().setPositions([0,0,0,0,1,0]);
 const material=new LineMaterial({color:colorNumber(roles[role]),linewidth:2,depthTest:false,depthWrite:false,toneMapped:false});
 const shaft=new Line2(geometry,material);arrow.add(shaft);
 const cone=arrow.cone.material as THREE.MeshBasicMaterial;
 cone.depthTest=false;cone.depthWrite=false;cone.toneMapped=false;
 let cssWidth=1,cssHeight=1;
 shaft.onBeforeRender=()=>material.resolution.set(cssWidth,cssHeight);
 updates.set(arrow,(camera,metrics)=>{
  const world=arrow.getWorldPosition(new THREE.Vector3());
  const view=world.clone().applyMatrix4(camera.matrixWorldInverse);
  const unit=worldUnitsPerCssPixel(camera,world,metrics);
  const near=(camera as THREE.PerspectiveCamera).near ?? 0;
  arrow.visible=Number.isFinite(unit)&&unit>0&&Number.isFinite(world.lengthSq())&&view.z < -near;
  if(!arrow.visible)return;
  cssWidth=metrics.cssWidth;cssHeight=metrics.cssHeight;
  const length=THREE.MathUtils.clamp(unit*64,1e-10,1e10);
  arrow.setLength(length,unit*12,unit*5);shaft.scale.set(1,Math.max(length-unit*12,0),1);
  material.resolution.set(cssWidth,cssHeight);
 });
 return arrow;
}

export function updateAnalysisGuides(root:THREE.Object3D,camera:THREE.Camera,metrics:ViewportMetrics):void{
 camera.updateMatrixWorld();root.updateMatrixWorld(true);
 root.traverse(object=>updates.get(object)?.(camera,metrics));
 root.updateMatrixWorld(true);
}
export function disposeAnalysisGuides(root:THREE.Object3D|undefined):void{
 root?.removeFromParent();root?.traverse(object=>{
  const mesh=object as THREE.Mesh;mesh.geometry?.dispose();
  const material=mesh.material;if(material)(Array.isArray(material)?material:[material]).forEach(m=>m.dispose());
  updates.delete(object);
 });
}

/** A bidirectional angular guide in the plane normal to the physical axis. */
export function analysisRotationArc(direction:THREE.Vector3,anchor:THREE.Vector3,initialLength:number):THREE.Group{
 const group=new THREE.Group();group.name='analysis-rotation-arc';group.position.copy(anchor);group.quaternion.setFromUnitVectors(new THREE.Vector3(0,0,1),direction.clone().normalize());
 const points:number[]=[];const start=-Math.PI*.7,end=Math.PI*.7;
 for(let i=0;i<=48;i++){const a=start+(end-start)*i/48;points.push(Math.cos(a),Math.sin(a),0)}
 const material=new LineMaterial({color:colorNumber(roles.rotation),linewidth:2,depthTest:false,depthWrite:false,toneMapped:false});
 group.add(new Line2(new LineGeometry().setPositions(points),material));
 for(const [angle,sign] of [[start,-1],[end,1]]){
  const cone=new THREE.Mesh(new THREE.ConeGeometry(.09,.25,12),new THREE.MeshBasicMaterial({color:colorNumber(roles.rotation),depthTest:false,depthWrite:false,toneMapped:false}));
  cone.position.set(Math.cos(angle),Math.sin(angle),0);cone.quaternion.setFromUnitVectors(new THREE.Vector3(0,1,0),new THREE.Vector3(-Math.sin(angle)*sign,Math.cos(angle)*sign,0));group.add(cone);
 }
 const axisMaterial=new LineMaterial({color:colorNumber(roles.rotation),linewidth:1,transparent:true,opacity:.5,depthTest:false,depthWrite:false});
 group.add(new Line2(new LineGeometry().setPositions([0,0,-1.4,0,0,1.4]),axisMaterial));
 group.scale.setScalar(initialLength*.45);
 updates.set(group,(camera,metrics)=>{const world=group.getWorldPosition(new THREE.Vector3()),unit=worldUnitsPerCssPixel(camera,world,metrics);group.visible=Number.isFinite(unit)&&unit>0&&world.clone().applyMatrix4(camera.matrixWorldInverse).z<-(camera as THREE.PerspectiveCamera).near;if(group.visible)group.scale.setScalar(THREE.MathUtils.clamp(unit*30,1e-10,1e10));material.resolution.set(metrics.cssWidth,metrics.cssHeight);axisMaterial.resolution.copy(material.resolution)});
 return group;
}
