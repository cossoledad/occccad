import type {MeshBuffers,ContourBuffers} from './mesh-topology-index';

/** Immutable display incidence, prepared by the decode worker. Triangle/vertex
 * indices are local display addresses, never persistent topology identities. */
export type ContourIncidence={endpoints:Uint32Array;offsets:Uint32Array;triangles:Uint32Array;normals:Float64Array;faces:Uint32Array;faceOffsets:Uint32Array;faceEdges:Uint32Array};
export type ContourEye={x:number;y:number;z:number;perspective:boolean};
const tableSize=(count:number)=>2**Math.ceil(Math.log2(Math.max(2,count*1.5)));

/** Exact float welding and integer pair hashing, without per-vertex/edge JS
 * Map entries or six double normals reserved for every possible half edge. */
export function buildContourIncidence({positions,indices,faceIds}:MeshBuffers):ContourIncidence {
 const count=positions.length/3,bits=new Uint32Array(positions.buffer,positions.byteOffset,positions.length);
 const canonical=new Uint32Array(count),vertices=new Uint32Array(tableSize(count)),vertexMask=vertices.length-1;
 for(let i=0;i<count;i++){
  const j=i*3,x=positions[j]===0?0:bits[j],y=positions[j+1]===0?0:bits[j+1],z=positions[j+2]===0?0:bits[j+2];
  let h=Math.imul(x^0x9e3779b9,0x85ebca6b);h=Math.imul(h^y,0xc2b2ae35);h=Math.imul(h^z,0x85ebca6b);h=(h^(h>>>16))&vertexMask;
  while(vertices[h]){const k=(vertices[h]-1)*3;if(positions[j]===positions[k]&&positions[j+1]===positions[k+1]&&positions[j+2]===positions[k+2])break;h=(h+1)&vertexMask}
  if(!vertices[h])vertices[h]=i+1;canonical[i]=vertices[h]-1;
 }
 const capacity=indices.length,table=new Uint32Array(tableSize(capacity)),mask=table.length-1;
 const endpoints=new Uint32Array(capacity*2),counts=new Uint32Array(capacity),halfEdges=new Uint32Array(capacity),normals=new Float64Array(capacity);
 let size=0,halves=0;
 const add=(a:number,b:number,half:number)=>{
  const lo=Math.min(canonical[a],canonical[b]),hi=Math.max(canonical[a],canonical[b]);
  let h=(Math.imul(lo,0x9e3779b1)^Math.imul(hi,0x85ebca6b))&mask;
  while(table[h]){const e=table[h]-1,p=e*2,first=canonical[endpoints[p]],second=canonical[endpoints[p+1]];if(Math.min(first,second)===lo&&Math.max(first,second)===hi)break;h=(h+1)&mask}
  if(!table[h]){endpoints[size*2]=a;endpoints[size*2+1]=b;table[h]=++size}
  const edge=table[h]-1;halfEdges[half]=edge+1;counts[edge]++;halves++;
 };
 for(let i=0;i<capacity;i+=3){
  const a=indices[i]*3,b=indices[i+1]*3,c=indices[i+2]*3;
  const ux=positions[b]-positions[a],uy=positions[b+1]-positions[a+1],uz=positions[b+2]-positions[a+2],vx=positions[c]-positions[a],vy=positions[c+1]-positions[a+1],vz=positions[c+2]-positions[a+2];
  let nx=uy*vz-uz*vy,ny=uz*vx-ux*vz,nz=ux*vy-uy*vx;const length=Math.sqrt(nx*nx+ny*ny+nz*nz);if(!length)continue;
  nx/=length;ny/=length;nz/=length;normals[i]=nx;normals[i+1]=ny;normals[i+2]=nz;
  add(indices[i],indices[i+1],i);add(indices[i+1],indices[i+2],i+1);add(indices[i+2],indices[i],i+2);
 }
 const offsets=new Uint32Array(size+1);for(let e=0;e<size;e++)offsets[e+1]=offsets[e]+counts[e];
 const triangles=new Uint32Array(halves);counts.fill(0,0,size);
 for(let h=0;h<capacity;h++)if(halfEdges[h]){const e=halfEdges[h]-1;triangles[offsets[e]+counts[e]++]=Math.floor(h/3)}
 // A flat interior of one CAD face can never become a face-selection boundary.
 // Retain cross-face seams, even flat ones, and all non-manifold incidences.
 let kept=0,total=0;
 for(let e=0;e<size;e++){
  const begin=offsets[e],end=offsets[e+1],first=triangles[begin],n=first*3;let keep=end-begin===1;
  for(let j=begin+1;j<end&&!keep;j++){const t=triangles[j],m=t*3;keep=faceIds.length===0||faceIds[t]!==faceIds[first]||normals[n]*normals[m]+normals[n+1]*normals[m+1]+normals[n+2]*normals[m+2]<=1-1e-12}
  if(keep){endpoints[kept*2]=endpoints[e*2];endpoints[kept*2+1]=endpoints[e*2+1];counts[kept]=end-begin;triangles.copyWithin(total,begin,end);total+=end-begin;kept++}
 }
 const packedOffsets=new Uint32Array(kept+1);for(let e=0;e<kept;e++)packedOffsets[e+1]=packedOffsets[e]+counts[e];
 const faces=Uint32Array.from(new Set(faceIds)).sort(),slots=new Map(Array.from(faces,(id,i)=>[id,i]));
 const faceOffsets=new Uint32Array(faces.length+1);
 const visitFaces=(visit:(slot:number,edge:number)=>void)=>{
  for(let e=0;e<kept;e++)for(let j=packedOffsets[e];j<packedOffsets[e+1];j++){
   const face=faceIds[triangles[j]];let duplicate=false;
   for(let k=packedOffsets[e];k<j;k++)if(faceIds[triangles[k]]===face){duplicate=true;break}
   if(!duplicate){const slot=slots.get(face);if(slot!==undefined)visit(slot,e)}
  }
 };
 visitFaces(slot=>faceOffsets[slot+1]++);
 for(let i=0;i<faces.length;i++)faceOffsets[i+1]+=faceOffsets[i];
 const faceEdges=new Uint32Array(faceOffsets[faces.length]),cursor=faceOffsets.slice();
 visitFaces((slot,edge)=>{faceEdges[cursor[slot]++]=edge});
 return {endpoints:endpoints.slice(0,kept*2),offsets:packedOffsets,triangles:triangles.slice(0,total),normals,faces,faceOffsets,faceEdges};
}

/** Select and project in one scan. Scratch grows with the visible output,
 * rather than allocating the entire selected surface before camera culling. */
export function incidenceContours(mesh:MeshBuffers,graph:ContourIncidence,ids?:ReadonlySet<number>,eye?:ContourEye):ContourBuffers {
 const {positions,faceIds}=mesh,{endpoints,offsets,triangles,normals,faces,faceOffsets,faceEdges}=graph;
 let candidates:Uint32Array|undefined;
 if(ids){
  const ranges:number[]=[];let count=0;
  for(const id of ids){let lo=0,hi=faces.length;while(lo<hi){const mid=(lo+hi)>>>1;if(faces[mid]<id)lo=mid+1;else hi=mid}if(faces[lo]===id){ranges.push(lo);count+=faceOffsets[lo+1]-faceOffsets[lo]}}
  if(count<endpoints.length/4){candidates=new Uint32Array(count);let n=0;for(const slot of ranges){candidates.set(faceEdges.subarray(faceOffsets[slot],faceOffsets[slot+1]),n);n+=faceOffsets[slot+1]-faceOffsets[slot]}candidates.sort()}
 }
 let visible=new Uint32Array(768),kept=0;
 const emit=(e:number,n:number,m:number)=>{
  if(eye){
   const p=endpoints[e*2]*3,x=eye.perspective?eye.x-positions[p]:eye.x,y=eye.perspective?eye.y-positions[p+1]:eye.y,z=eye.perspective?eye.z-positions[p+2]:eye.z;
   const a=Math.fround(normals[n])*x+Math.fround(normals[n+1])*y+Math.fround(normals[n+2])*z,b=m<0?0:Math.fround(normals[m])*x+Math.fround(normals[m+1])*y+Math.fround(normals[m+2])*z,epsilon=1e-5*Math.sqrt(x*x+y*y+z*z);
   if(m>=0&&a*b>0&&Math.abs(a)>epsilon&&Math.abs(b)>epsilon)return;
  }
  if(kept*3===visible.length){const grown=new Uint32Array(visible.length*2);grown.set(visible);visible=grown}
  visible[kept*3]=e;visible[kept*3+1]=n;visible[kept*3+2]=m+1;kept++;
 };
 const count=candidates?.length??endpoints.length/2;
 for(let i=0;i<count;i++){
  const e=candidates?candidates[i]:i;if(candidates&&i>0&&e===candidates[i-1])continue;
  let first=-1,second=-1,count=0,nonflat=false;
  for(let j=offsets[e];j<offsets[e+1];j++){
   const t=triangles[j];if(ids&&!ids.has(faceIds[t]))continue;const n=t*3;
   if(count===0)first=n;else{if(count===1)second=n;if(normals[first]*normals[n]+normals[first+1]*normals[n+1]+normals[first+2]*normals[n+2]<=1-1e-12)nonflat=true}count++;
  }
  if(count===1||nonflat)emit(e,first,second);
 }
 const output=new Float32Array(kept*6),first=new Float32Array(kept*3),second=new Float32Array(kept*3);
 for(let i=0;i<kept;i++){
  const e=visible[i*3],n=visible[i*3+1],m=visible[i*3+2]-1,a=endpoints[e*2]*3,b=endpoints[e*2+1]*3;
  for(let k=0;k<3;k++){output[i*6+k]=positions[a+k];output[i*6+3+k]=positions[b+k];first[i*3+k]=normals[n+k];second[i*3+k]=m<0?0:normals[m+k]}
 }
 return {positions:output,first,second};
}
