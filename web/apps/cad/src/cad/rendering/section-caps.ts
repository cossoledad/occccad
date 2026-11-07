import * as THREE from 'three';
import type {SpatialAnalysis} from '../assembly/spatial-analysis';
import {sectionClippingPlanes} from './section-presentation';

/** Display-only stencil caps preserve holes without guessing contour topology.
 * Exact BREP results remain the source of areas, saved results and 2D curves. */
export class SectionCaps {
 readonly root=new THREE.Group();
 private entries:{source:THREE.Mesh;plane:THREE.Plane;counters:THREE.Mesh[];cap:THREE.Mesh<THREE.PlaneGeometry,THREE.MeshBasicMaterial>}[]=[];
 private sources:THREE.Mesh[]=[];private count=0;
 constructor(){this.root.name='section-caps'}
 clear(){for(const entry of this.entries){entry.cap.geometry.dispose();entry.cap.material.dispose();entry.counters.forEach(mesh=>(mesh.material as THREE.Material).dispose())}this.root.clear();this.entries=[];this.sources=[];this.count=0;this.root.removeFromParent()}
 update(d:SpatialAnalysis,sources:THREE.Mesh[]){
  const planes=sectionClippingPlanes(d);
  if(this.count!==planes.length||sources.length!==this.sources.length||sources.some((mesh,i)=>mesh!==this.sources[i])){
   const parent=this.root.parent;this.clear();parent?.add(this.root);this.sources=[...sources];this.count=planes.length;
   if(!planes.length)return;
   for(const source of sources)for(let i=0;i<planes.length;i++){
    const plane=planes[i].clone(),order=100+this.entries.length*3;
    const counters=[THREE.BackSide,THREE.FrontSide].map((side,index)=>{
     const operation=index===0?THREE.IncrementWrapStencilOp:THREE.DecrementWrapStencilOp;
     const material=new THREE.MeshBasicMaterial({side,depthTest:false,depthWrite:false,colorWrite:false,clippingPlanes:[plane],stencilWrite:true,stencilFunc:THREE.AlwaysStencilFunc,stencilFail:operation,stencilZFail:operation,stencilZPass:operation});
     const mesh=new THREE.Mesh(source.geometry,material);mesh.matrixAutoUpdate=false;mesh.renderOrder=order+index;mesh.frustumCulled=false;this.root.add(mesh);return mesh;
    });
    const original=Array.isArray(source.material)?source.material[0]:source.material;
    const color=((original as THREE.MeshStandardMaterial).color??new THREE.Color(0x8c9caa)).clone().lerp(new THREE.Color(0xffffff),.28);
    const material=new THREE.MeshBasicMaterial({color,side:THREE.DoubleSide,toneMapped:false,stencilWrite:true,stencilRef:0,stencilFunc:THREE.NotEqualStencilFunc,stencilFail:THREE.ReplaceStencilOp,stencilZFail:THREE.ReplaceStencilOp,stencilZPass:THREE.ReplaceStencilOp,polygonOffset:true,polygonOffsetFactor:-1,polygonOffsetUnits:-1});
    const pixelRatio={value:1};material.userData.sectionHatch=true;
    material.onBeforeCompile=shader=>{shader.uniforms.sectionPixelRatio=pixelRatio;shader.fragmentShader='uniform float sectionPixelRatio;\n'+shader.fragmentShader;shader.fragmentShader=shader.fragmentShader.replace('#include <color_fragment>',`#include <color_fragment>
     float stripe=(gl_FragCoord.x+gl_FragCoord.y)/(9.0*sectionPixelRatio);
     float distanceToLine=abs(fract(stripe+0.5)-0.5);
     float aa=fwidth(stripe);
     float ink=1.0-smoothstep(0.055-aa,0.055+aa,distanceToLine);
     diffuseColor.rgb=mix(diffuseColor.rgb,diffuseColor.rgb*0.48,ink);`)};
    material.customProgramCacheKey=()=> 'section-hatch-v1';
    const cap=new THREE.Mesh(new THREE.PlaneGeometry(1,1),material);cap.renderOrder=order+2;cap.frustumCulled=false;
    cap.onBeforeRender=renderer=>{pixelRatio.value=renderer.getPixelRatio()};cap.onAfterRender=renderer=>renderer.clearStencil();this.root.add(cap);
    this.entries.push({source,plane,counters,cap});
   }
  }
  for(let j=0;j<this.entries.length;j++){
   const entry=this.entries[j],index=j%planes.length;entry.plane.copy(planes[index]);entry.source.updateWorldMatrix(true,false);
   entry.counters.forEach(mesh=>mesh.matrix.copy(entry.source.matrixWorld));
   const bounds=new THREE.Box3().setFromObject(entry.source),center=bounds.getCenter(new THREE.Vector3()),radius=bounds.getSize(new THREE.Vector3()).length();
   entry.plane.projectPoint(center,entry.cap.position);entry.cap.quaternion.setFromUnitVectors(new THREE.Vector3(0,0,1),entry.plane.normal);entry.cap.scale.setScalar(Math.max(1,radius*1.01));
   // Both sides of a box/slab expose the same bounded cross-section face.
   // KEEP_OUTSIDE reverses the solid's clip union, but not this face boundary.
   entry.cap.material.clippingPlanes=planes.filter((_,i)=>i!==index).map(p=>d.cutMode==='KEEP_OUTSIDE'?p.clone().negate():p);
  }
 }
}
