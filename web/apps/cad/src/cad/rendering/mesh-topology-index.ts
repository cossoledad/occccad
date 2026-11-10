import type {MeshData} from '../../types';

export type MeshBuffers = {positions:Float32Array;indices:Uint32Array;normals?:Float32Array;faceIds:Uint32Array};
export type ContourBuffers = {positions:Float32Array;first:Float32Array;second:Float32Array};

/** Display-only indices. A locator is interpreted in this immutable mesh;
 * coordinate equivalence here never supplies a persistent topology identity. */
export class MeshTopologyIndex {
  readonly buffers:MeshBuffers;
  private readonly faces=new Map<number,number[]>();
  constructor(readonly mesh:MeshData) {
    const packed=(mesh as MeshData&{buffers?:MeshBuffers}).buffers;
    if(packed)this.buffers=packed;
    else {
      const positions=new Float32Array(mesh.vertices.length*3),indices=new Uint32Array(mesh.triangles.length*3);
      mesh.vertices.forEach((v,i)=>positions.set(v,i*3));mesh.triangles.forEach((t,i)=>indices.set(t,i*3));
      this.buffers={positions,indices,faceIds:Uint32Array.from(mesh.faceIds)};
    }
    for(let i=0;i<this.buffers.faceIds.length;i++){
      const id=this.buffers.faceIds[i],triangles=this.faces.get(id);
      if(triangles)triangles.push(i);else this.faces.set(id,[i]);
    }
  }
  forEachFaceTriangle(id:number, visit:(a:number,b:number,c:number)=>void):void {
    for(const triangle of this.faces.get(id)??[]){const i=triangle*3;visit(this.buffers.indices[i],this.buffers.indices[i+1],this.buffers.indices[i+2]);}
  }
  triangleIndices(ids?:ReadonlySet<number>):Uint32Array {
    if(!ids)return this.buffers.indices;
    let count=0;for(const id of ids)count+=this.faces.get(id)?.length??0;
    const triangles=new Uint32Array(count);let offset=0;
    for(const id of ids)for(const triangle of this.faces.get(id)??[])triangles[offset++]=triangle;
    triangles.sort();const result=new Uint32Array(count*3),source=this.buffers.indices;
    triangles.forEach((triangle,i)=>result.set(source.subarray(triangle*3,triangle*3+3),i*3));return result;
  }
  contours(ids?:ReadonlySet<number>):ContourBuffers {
    return !ids && this.mesh.contours ? this.mesh.contours : indexedSurfaceContours(this.buffers.positions,this.triangleIndices(ids));
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

/** Linear incidence construction, with numeric hash buckets and exact float
 * collision checks. No per-triangle Vector3, coordinate strings or edge objects. */
export function indexedSurfaceContours(positions:Float32Array,indices:Uint32Array):ContourBuffers {
  const count=positions.length/3,bits=new Uint32Array(positions.buffer,positions.byteOffset,positions.length);
  if(count*count>Number.MAX_SAFE_INTEGER)throw new Error('Mesh exceeds display incidence index capacity');
  const canonical=new Uint32Array(count),next=new Uint32Array(count),heads=new Map<number,number>();
  for(let i=0;i<count;i++) {
    const j=i*3,x=positions[j]===0?0:bits[j],y=positions[j+1]===0?0:bits[j+1],z=positions[j+2]===0?0:bits[j+2];
    let hash=Math.imul(x^0x9e3779b9,0x85ebca6b);hash=Math.imul(hash^y,0xc2b2ae35);hash=Math.imul(hash^z,0x85ebca6b);hash^=hash>>>16;
    let found=heads.get(hash)??0;
    while(found){const k=(found-1)*3;if(positions[j]===positions[k]&&positions[j+1]===positions[k+1]&&positions[j+2]===positions[k+2])break;found=next[found-1];}
    if(found)canonical[i]=found-1;else {next[i]=heads.get(hash)??0;heads.set(hash,i+1);canonical[i]=i;}
  }
  const capacity=indices.length,a=new Uint32Array(capacity),b=new Uint32Array(capacity),normals=new Float64Array(capacity*6);
  const uses=new Uint8Array(capacity),nonflat=new Uint8Array(capacity),edges=new Map<number,number>();let size=0;
  const add=(first:number,second:number,nx:number,ny:number,nz:number)=>{
    const lo=Math.min(canonical[first],canonical[second]),hi=Math.max(canonical[first],canonical[second]),key=lo*count+hi;
    const known=edges.get(key);
    if(known===undefined){edges.set(key,size);a[size]=first;b[size]=second;uses[size]=1;const p=size*6;normals[p]=nx;normals[p+1]=ny;normals[p+2]=nz;size++;}
    else {const p=known*6;if(uses[known]===1){normals[p+3]=nx;normals[p+4]=ny;normals[p+5]=nz;}if(normals[p]*nx+normals[p+1]*ny+normals[p+2]*nz<=1-1e-12)nonflat[known]=1;uses[known]=2;}
  };
  for(let i=0;i<indices.length;i+=3){
    const ia=indices[i]*3,ib=indices[i+1]*3,ic=indices[i+2]*3;
    const ux=positions[ib]-positions[ia],uy=positions[ib+1]-positions[ia+1],uz=positions[ib+2]-positions[ia+2];
    const vx=positions[ic]-positions[ia],vy=positions[ic+1]-positions[ia+1],vz=positions[ic+2]-positions[ia+2];
    let nx=uy*vz-uz*vy,ny=uz*vx-ux*vz,nz=ux*vy-uy*vx;const length=Math.sqrt(nx*nx+ny*ny+nz*nz);if(!length)continue;nx/=length;ny/=length;nz/=length;
    add(indices[i],indices[i+1],nx,ny,nz);add(indices[i+1],indices[i+2],nx,ny,nz);add(indices[i+2],indices[i],nx,ny,nz);
  }
  let kept=0;for(let i=0;i<size;i++)if(uses[i]===1||nonflat[i])kept++;
  const output=new Float32Array(kept*6),first=new Float32Array(kept*3),second=new Float32Array(kept*3);let offset=0;
  for(let i=0;i<size;i++)if(uses[i]===1||nonflat[i]){
    for(let j=0;j<3;j++){output[offset*6+j]=positions[a[i]*3+j];output[offset*6+3+j]=positions[b[i]*3+j];first[offset*3+j]=normals[i*6+j];second[offset*3+j]=normals[i*6+3+j];}offset++;
  }
  return {positions:output,first,second};
}
