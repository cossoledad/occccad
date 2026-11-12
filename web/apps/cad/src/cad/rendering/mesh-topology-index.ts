import type {MeshData} from '../../types';
import {buildContourIncidence,incidenceContours,type ContourEye} from './mesh-contour-incidence';

export type MeshBuffers = {positions:Float32Array;indices:Uint32Array;normals?:Float32Array;faceIds:Uint32Array};
export type ContourBuffers = {positions:Float32Array;first:Float32Array;second:Float32Array};

/** Display-only indices. A locator is interpreted in this immutable mesh;
 * coordinate equivalence here never supplies a persistent topology identity. */
export class MeshTopologyIndex {
  readonly buffers:MeshBuffers;
  private readonly faces=new Map<number,number[]>();
  private facesReady=false;
  private incidence(){
    if(!this.mesh.contourIncidence)Object.defineProperty(this.mesh,'contourIncidence',{value:buildContourIncidence(this.buffers)});
    return this.mesh.contourIncidence!;
  }
  faceSelection(ids?:ReadonlySet<number>):ReadonlySet<number>|undefined {
    if(!ids)return;
    const faces=this.incidence().faces;
    return faces.length&&faces.every(id=>ids.has(id))?undefined:ids;
  }
  private indexFaces():void {
    if(this.facesReady)return;this.facesReady=true;
    for(let i=0;i<this.buffers.faceIds.length;i++){const id=this.buffers.faceIds[i],triangles=this.faces.get(id);if(triangles)triangles.push(i);else this.faces.set(id,[i])}
  }
  constructor(readonly mesh:MeshData) {
    const packed=(mesh as MeshData&{buffers?:MeshBuffers}).buffers;
    if(packed)this.buffers=packed;
    else {
      const positions=new Float32Array(mesh.vertices.length*3),indices=new Uint32Array(mesh.triangles.length*3);
      mesh.vertices.forEach((v,i)=>positions.set(v,i*3));mesh.triangles.forEach((t,i)=>indices.set(t,i*3));
      this.buffers={positions,indices,faceIds:Uint32Array.from(mesh.faceIds)};
    }
  }
  forEachFaceTriangle(id:number, visit:(a:number,b:number,c:number)=>void):void {
    this.indexFaces();
    for(const triangle of this.faces.get(id)??[]){const i=triangle*3;visit(this.buffers.indices[i],this.buffers.indices[i+1],this.buffers.indices[i+2]);}
  }
  triangleIndices(ids?:ReadonlySet<number>):Uint32Array {
    if(!ids)return this.buffers.indices;
    this.indexFaces();let count=0;for(const id of ids)count+=this.faces.get(id)?.length??0;
    const triangles=new Uint32Array(count);let offset=0;
    for(const id of ids)for(const triangle of this.faces.get(id)??[])triangles[offset++]=triangle;
    triangles.sort();const result=new Uint32Array(count*3),source=this.buffers.indices;
    triangles.forEach((triangle,i)=>result.set(source.subarray(triangle*3,triangle*3+3),i*3));return result;
  }
  contours(ids?:ReadonlySet<number>,eye?:ContourEye):ContourBuffers {
    return incidenceContours(this.buffers,this.incidence(),ids,eye);
  }
  edgeSegments(ids?:ReadonlySet<number>):Float32Array {
    let count=0;for(const edge of this.mesh.edges??[])if(!ids||ids.has(edge.localId))count+=Math.max(0,edge.points.length-1);
    const result=new Float32Array(count*6);let offset=0;
    for(const edge of this.mesh.edges??[])if(!ids||ids.has(edge.localId))for(let i=1;i<edge.points.length;i++){
      result.set(edge.points[i-1],offset);result.set(edge.points[i],offset+3);offset+=6;
    }
    return result;
  }
}

const topologyIndices=new WeakMap<MeshData,MeshTopologyIndex>();
export function meshTopologyIndex(mesh:MeshData):MeshTopologyIndex {
  let index=topologyIndices.get(mesh);if(!index){index=new MeshTopologyIndex(mesh);topologyIndices.set(mesh,index);}return index;
}

/** Standalone oracle-compatible entrypoint; interaction uses the worker-owned
 * incidence through MeshTopologyIndex, never reconstructs it per selection. */
export function indexedSurfaceContours(positions:Float32Array,indices:Uint32Array):ContourBuffers {
  const mesh={positions,indices,faceIds:new Uint32Array()};
  return incidenceContours(mesh,buildContourIncidence(mesh));
}
