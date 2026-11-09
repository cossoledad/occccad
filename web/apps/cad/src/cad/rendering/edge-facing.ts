import * as THREE from 'three';
import type {MeshData} from '../../types';

/** Display-only normals. No topology reference is inferred or persisted here.
 * GLB duplicates vertices on face boundaries; preserve the distinct normals at
 * a shared point, then classify silhouettes in the shader as the camera moves.
 * Unmatched curve samples use the edge endpoints, or neutral contrast. */
export function edgeFacingAttributes(mesh:MeshData):{first:number[];second:number[]}{
 const box=new THREE.Box3(),point=new THREE.Vector3();
 for(const vertex of mesh.vertices)box.expandByPoint(point.fromArray(vertex));
 const epsilon=Math.max(box.getSize(new THREE.Vector3()).length()*1e-6,1e-8);
 const key=(p:readonly number[])=>p.map(v=>Math.round(v/epsilon)).join(',');
 const wanted=new Set((mesh.edges??[]).flatMap(edge=>edge.points.map(key)));
 const normals=new Map<string,THREE.Vector3[]>();
 mesh.vertices.forEach((p,i)=>{const n=mesh.normals?.[i];if(!n)return;const k=key(p);if(!wanted.has(k))return;const normal=new THREE.Vector3(...n).normalize(),values=normals.get(k)??[];if(values.length<3&&!values.some(v=>v.dot(normal)>.999))values.push(normal);normals.set(k,values)});
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
