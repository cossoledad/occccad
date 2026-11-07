import * as THREE from "three";

/** Display-only boundary of a selected triangle set, with exact float keys.
 * It neither changes mesh ordering nor creates persistent topology identities. */
export function faceOutlineSegments(positions:readonly number[]):Array<[THREE.Vector3,THREE.Vector3]> {
  const edges=new Map<string,{a:THREE.Vector3;b:THREE.Vector3;count:number}>();
  for(let i=0;i+8<positions.length;i+=9){
    const vertices=[0,3,6].map(offset=>new THREE.Vector3(positions[i+offset],positions[i+offset+1],positions[i+offset+2]));
    for(let j=0;j<3;j++){
      const a=vertices[j],b=vertices[(j+1)%3],ak=a.toArray().join(","),bk=b.toArray().join(",");
      const key=ak<bk?`${ak}|${bk}`:`${bk}|${ak}`,edge=edges.get(key);
      if(edge)edge.count++;else edges.set(key,{a,b,count:1});
    }
  }
  return [...edges.values()].filter(edge=>edge.count===1).map(edge=>[edge.a,edge.b]);
}

/** Trim boundaries and non-flat adjacency candidates. The shader retains only
 * camera-facing silhouette candidates, never triangle tessellation wire. */
export function surfaceContourData(positions:readonly number[]){
  const edges=new Map<string,{a:THREE.Vector3;b:THREE.Vector3;normals:THREE.Vector3[]}>();
  for(let i=0;i+8<positions.length;i+=9){
    const vertices=[0,3,6].map(j=>new THREE.Vector3(positions[i+j],positions[i+j+1],positions[i+j+2]));
    const normal=vertices[1].clone().sub(vertices[0]).cross(vertices[2].clone().sub(vertices[0]));if(!normal.lengthSq())continue;normal.normalize();
    for(let j=0;j<3;j++){const a=vertices[j],b=vertices[(j+1)%3],ak=a.toArray().join(','),bk=b.toArray().join(','),key=ak<bk?ak+'|'+bk:bk+'|'+ak,edge=edges.get(key);if(edge)edge.normals.push(normal);else edges.set(key,{a,b,normals:[normal]});}
  }
  const segments:Array<[THREE.Vector3,THREE.Vector3]>=[],first:number[]=[],second:number[]=[];
  for(const {a,b,normals} of edges.values()){
    if(normals.length>1&&normals.every(n=>n.dot(normals[0])>1-1e-12))continue;
    segments.push([a,b]);first.push(...normals[0].toArray());second.push(...(normals[1]?.toArray()??[0,0,0]));
  }
  return {segments,first,second};
}
