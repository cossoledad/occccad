import * as THREE from 'three';
import type {LineSegments2} from 'three/addons/lines/LineSegments2.js';
import {LineSegmentsGeometry} from 'three/addons/lines/LineSegmentsGeometry.js';
import type {GeometryLease} from './interaction-geometry-cache';
import type {ContourEye} from './mesh-contour-incidence';

type Projected={positions:Float32Array;first:Float32Array;second:Float32Array};

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

export function contourProjectionUpdater(line:LineSegments2,acquire:(eye:ContourEye)=>GeometryLease) {
  const inverse=new THREE.Matrix4(),localCamera=new THREE.Matrix4(),eye=new THREE.Vector3();let previous='',lease:GeometryLease|undefined;
  return {update:(camera:THREE.Camera)=>{
    localCamera.multiplyMatrices(inverse.copy(line.matrixWorld).invert(),camera.matrixWorld);
    const perspective=camera.projectionMatrix.elements[11]===-1;
    if(perspective)eye.setFromMatrixPosition(localCamera);else eye.setFromMatrixColumn(localCamera,2);
    const key=Number(perspective)+':'+eye.x+','+eye.y+','+eye.z;if(previous===key)return;previous=key;
    const next=acquire({x:eye.x,y:eye.y,z:eye.z,perspective});
    if(lease)lease.release();else line.geometry.dispose();lease=next;line.geometry=next.geometry as LineSegmentsGeometry;
  },release:()=>{if(lease){lease.release();lease=undefined}else line.geometry.dispose()}};
}
