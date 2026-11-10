import * as THREE from 'three';
import type {LineSegments2} from 'three/addons/lines/LineSegments2.js';
import {LineSegmentsGeometry} from 'three/addons/lines/LineSegmentsGeometry.js';

type Projected={positions:Float32Array;first:Float32Array;second:Float32Array};
const projections=new WeakMap<THREE.BufferGeometry,Map<string,Projected>>();

/** Cull silhouette candidates before GPU submission. The conservative CPU
 * band retains grazing candidates; the existing shader makes the final test. */
export function projectContours(source:THREE.BufferGeometry,eye:THREE.Vector3,perspective:boolean):Projected {
  const start=source.getAttribute('instanceStart') as THREE.InterleavedBufferAttribute;
  const positions=start.data.array as Float32Array,first=source.getAttribute('instanceEdgeNormalA').array as Float32Array;
  const second=source.getAttribute('instanceEdgeNormalB').array as Float32Array,count=first.length/3;
  const visible=new Uint32Array(count);let kept=0;
  for(let i=0;i<count;i++){
    const n=i*3,p=i*6,x=perspective?eye.x-positions[p]:eye.x,y=perspective?eye.y-positions[p+1]:eye.y,z=perspective?eye.z-positions[p+2]:eye.z;
    const a=first[n]*x+first[n+1]*y+first[n+2]*z,b=second[n]*x+second[n+1]*y+second[n+2]*z;
    const boundary=second[n]===0&&second[n+1]===0&&second[n+2]===0;
    const epsilon=1e-5*Math.sqrt(x*x+y*y+z*z);
    if(boundary||a*b<=0||Math.abs(a)<=epsilon||Math.abs(b)<=epsilon)visible[kept++]=i;
  }
  const out:Projected={positions:new Float32Array(kept*6),first:new Float32Array(kept*3),second:new Float32Array(kept*3)};
  for(let j=0;j<kept;j++){const i=visible[j];out.positions.set(positions.subarray(i*6,i*6+6),j*6);out.first.set(first.subarray(i*3,i*3+3),j*3);out.second.set(second.subarray(i*3,i*3+3),j*3);}
  return out;
}

export function contourProjectionUpdater(line:LineSegments2,source:THREE.BufferGeometry):(camera:THREE.Camera)=>void {
  const inverse=new THREE.Matrix4(),localCamera=new THREE.Matrix4(),eye=new THREE.Vector3();let previous='';
  return camera=>{
    localCamera.multiplyMatrices(inverse.copy(line.matrixWorld).invert(),camera.matrixWorld);
    const perspective=camera.projectionMatrix.elements[11]===-1;
    if(perspective)eye.setFromMatrixPosition(localCamera);else eye.setFromMatrixColumn(localCamera,2);
    const key=Number(perspective)+':'+eye.x+','+eye.y+','+eye.z;if(previous===key)return;previous=key;
    let cache=projections.get(source);if(!cache){cache=new Map();projections.set(source,cache);}
    let data=cache.get(key);if(!data){data=projectContours(source,eye,perspective);if(cache.size>=4)cache.delete(cache.keys().next().value!);cache.set(key,data);}
    const geometry=new LineSegmentsGeometry();geometry.setPositions(data.positions);
    geometry.setAttribute('instanceEdgeNormalA',new THREE.InstancedBufferAttribute(data.first,3));
    geometry.setAttribute('instanceEdgeNormalB',new THREE.InstancedBufferAttribute(data.second,3));
    geometry.boundingBox=source.boundingBox?.clone()??null;geometry.boundingSphere=source.boundingSphere?.clone()??null;
    line.geometry.dispose();line.geometry=geometry;
  };
}
