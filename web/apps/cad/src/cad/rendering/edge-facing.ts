import * as THREE from 'three';
import type {MeshData} from '../../types';

/** Display-only normals. No topology reference is inferred or persisted here.
 * GLB duplicates vertices on face boundaries; preserve the distinct normals at
 * a shared point, then classify silhouettes in the shader as the camera moves.
 * Unmatched curve samples use the edge endpoints, or neutral contrast. */
export function edgeFacingAttributes(mesh:MeshData):{first:number[];second:number[]}{
 const box=new THREE.Box3(),point=new THREE.Vector3();
 const packed=mesh.buffers;
 if(packed)box.setFromBufferAttribute(new THREE.BufferAttribute(packed.positions,3));
 else for(const vertex of mesh.vertices)box.expandByPoint(point.fromArray(vertex));
 const epsilon=Math.max(box.getSize(new THREE.Vector3()).length()*1e-6,1e-8);
 const key=(p:readonly number[])=>p.map(v=>Math.round(v/epsilon)).join(',');
 const wanted=new Set((mesh.edges??[]).flatMap(edge=>edge.points.map(key)));
 const normals=new Map<string,THREE.Vector3[]>();
 const accept=(p:readonly number[],n:readonly number[])=>{const k=key(p);if(!wanted.has(k))return;const normal=new THREE.Vector3(n[0],n[1],n[2]).normalize(),values=normals.get(k)??[];if(values.length<3&&!values.some(v=>v.dot(normal)>.999))values.push(normal);normals.set(k,values)};
 if(packed?.normals){const p=[0,0,0],n=[0,0,0];for(let i=0;i<packed.positions.length;i+=3){for(let a=0;a<3;a++){p[a]=packed.positions[i+a];n[a]=packed.normals[i+a]}accept(p,n)}}
 else if(!packed)mesh.vertices.forEach((p,i)=>{const n=mesh.normals?.[i];if(n)accept(p,n)});
 const first:number[]=[],second:number[]=[];
 for(const edge of mesh.edges??[]){
  const fallback=normals.get(key(edge.points[0]??[]))??normals.get(key(edge.points.at(-1)??[]))??[];
  for(let i=1;i<edge.points.length;i++){
   const values=normals.get(key(edge.points[i-1]))??normals.get(key(edge.points[i]))??fallback;
   first.push(...(values[0]?.toArray()??[0,0,0]));second.push(...(values[1]?.toArray()??values[0]?.toArray()??[0,0,0]));
  }
 }
 return {first,second};
}
