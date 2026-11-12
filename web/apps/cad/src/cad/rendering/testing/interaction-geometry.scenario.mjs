import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const T=await server.ssrLoadModule('three');
 const {surfaceContourData}=await server.ssrLoadModule('/src/cad/rendering/face-outline.ts');
 const {indexedSurfaceContours,MeshTopologyIndex}=await server.ssrLoadModule('/src/cad/rendering/mesh-topology-index.ts');
 const {InteractionGeometryCache}=await server.ssrLoadModule('/src/cad/rendering/interaction-geometry-cache.ts');
 const {projectContours,contourProjectionUpdater}=await server.ssrLoadModule('/src/cad/rendering/contour-projection.ts');
 const {LineSegmentsGeometry}=await server.ssrLoadModule('three/addons/lines/LineSegmentsGeometry.js');
 const {LineSegments2}=await server.ssrLoadModule('three/addons/lines/LineSegments2.js');
 const oracleBuffers=(positions,indices)=>{
  const flat=[];for(const i of indices)flat.push(...positions.subarray(i*3,i*3+3));const data=surfaceContourData(flat);
  return {positions:Float32Array.from(data.segments.flatMap(segment=>segment.flatMap(v=>v.toArray()))),first:Float32Array.from(data.first),second:Float32Array.from(data.second)};
 };
 for(const geometry of [new T.BoxGeometry(),new T.CylinderGeometry(1,1,3,32,3),new T.SphereGeometry(1,16,8)]){
  const positions=geometry.attributes.position.array,indices=new Uint32Array(geometry.index.array),flat=[];
  for(const index of indices)flat.push(...positions.subarray(index*3,index*3+3));
  const oracle=surfaceContourData(flat),result=indexedSurfaceContours(positions,indices);
  assert.deepEqual([...result.positions],oracle.segments.flatMap(segment=>segment.flatMap(v=>v.toArray())),'numeric incidence keeps the exact displayed contour candidates');
  for(const [a,b] of [[result.first,oracle.first],[result.second,oracle.second]])a.forEach((value,i)=>assert(Math.abs(value-b[i])<1e-6));
  const graph=new LineSegmentsGeometry().setPositions(result.positions);
  graph.setAttribute('instanceEdgeNormalA',new T.InstancedBufferAttribute(result.first,3));graph.setAttribute('instanceEdgeNormalB',new T.InstancedBufferAttribute(result.second,3));
  for(const perspective of [true,false])for(const eye of [new T.Vector3(3,5,9),new T.Vector3(-8,2,-1),new T.Vector3(0,0,1)]){
   const projected=projectContours(graph,eye,perspective),expected=[];
   for(let i=0;i<result.first.length;i+=3){
    const n=new T.Vector3().fromArray(result.first,i),m=new T.Vector3().fromArray(result.second,i),view=perspective?eye.clone().sub(new T.Vector3().fromArray(result.positions,i*2)):eye;
    if(m.lengthSq()===0||n.dot(view)*m.dot(view)<=0)expected.push([...result.positions.subarray(i*2,i*2+6)].join(','));
   }
   const actual=new Set(Array.from({length:projected.positions.length/6},(_,i)=>[...projected.positions.subarray(i*6,i*6+6)].join(',')));
   for(const segment of expected)assert(actual.has(segment),'CPU culling never removes a shader-visible contour');
  }
  const line=new LineSegments2(new LineSegmentsGeometry());line.position.set(10,-20,30);line.rotation.set(.3,.5,.7);line.updateMatrixWorld(true);
  const camera=new T.PerspectiveCamera();camera.position.set(90,50,70);camera.lookAt(10,-20,30);camera.updateMatrixWorld(true);
  const packed={buffers:{positions,indices,faceIds:new Uint32Array(indices.length/3).map((_,i)=>i%3+1)},edges:[],topologyVertices:[]};
  const index=new MeshTopologyIndex(packed),cache=new InteractionGeometryCache(1024*1024);
  const projection=contourProjectionUpdater(line,eye=>cache.acquire(packed,'face',undefined,eye));projection.update(camera);const initial=line.geometry;
  projection.update(camera);assert.equal(line.geometry,initial,'stationary camera reuses projection and GPU attributes');
  camera.position.x+=1;camera.updateMatrixWorld(true);projection.update(camera);assert.notEqual(line.geometry,initial);
  const edgeKey=(p)=>{const a=p.slice(0,3).join(','),b=p.slice(3).join(',');return [a,b].sort().join('/')};
  const keys=(p)=>Array.from({length:p.length/6},(_,i)=>edgeKey([...p.subarray(i*6,i*6+6)])).sort();
  for(const ids of [new Set([1]),new Set([2,3]),new Set([1,2,3]),new Set([999])]){
   const selected=index.triangleIndices(ids),oracle=oracleBuffers(positions,selected),actual=index.contours(ids);
   assert.deepEqual(keys(actual.positions),keys(oracle.positions),'immutable incidence preserves arbitrary face-set boundaries');
   for(const perspective of [true,false]){
    const eye={x:3,y:5,z:9,perspective},expectedGraph=new LineSegmentsGeometry().setPositions(oracle.positions);
    expectedGraph.setAttribute('instanceEdgeNormalA',new T.InstancedBufferAttribute(oracle.first,3));expectedGraph.setAttribute('instanceEdgeNormalB',new T.InstancedBufferAttribute(oracle.second,3));
    const expected=projectContours(expectedGraph,new T.Vector3(3,5,9),perspective),actual=index.contours(ids,eye);
    assert.deepEqual(keys(actual.positions),keys(expected.positions),'fused subset projection matches independent contour culling');expectedGraph.dispose();
   }
  }
  projection.release();cache.dispose();graph.dispose();
  geometry.dispose();
 }
 const mesh={vertices:[[0,0,0],[1,0,0],[1,1,0],[0,1,0]],triangles:[[0,1,2],[0,2,3]],faceIds:[1,2],edges:[{localId:9,points:[[0,0,0],[1,0,0],[1,1,0]]}],topologyVertices:[]};
 const index=new MeshTopologyIndex(mesh);
 assert.deepEqual([...index.triangleIndices(new Set([2]))],[0,2,3]);
 assert.equal(index.contours().positions.length,24);assert.equal(index.contours(new Set([1])).positions.length,18);
 assert.deepEqual([...index.edgeSegments(new Set([9]))],[0,0,0,1,0,0,1,0,0,1,1,0]);
 const cache=new InteractionGeometryCache(0),first=cache.acquire(mesh,'face'),second=cache.acquire(mesh,'face',new Set([1,2]));
 assert.equal(first.geometry,second.geometry,'two occurrences borrow the same immutable derived buffers');
 let disposed=0;first.geometry.addEventListener('dispose',()=>disposed++);
 first.release();assert.equal(disposed,0,'hover release preserves another occurrence selection');
 second.release();assert.equal(disposed,1);second.release();assert.equal(disposed,1);
 assert.equal(cache.retainedBytes,0);cache.dispose();
 const {buildContourIncidence,incidenceContours}=await server.ssrLoadModule('/src/cad/rendering/mesh-contour-incidence.ts');
 const nonmanifold={positions:new Float32Array([0,0,0,1,0,0,0,1,0,0,0,1,0,-1,0,-0,0,0]),indices:new Uint32Array([0,1,2,1,5,3,0,1,4,0,0,1]),faceIds:new Uint32Array([1,2,3,4])};
 const incidence=buildContourIncidence(nonmanifold),nonmanifoldIndex=new MeshTopologyIndex({buffers:nonmanifold});
 for(const ids of [new Set([1]),new Set([1,2]),new Set([2,3]),new Set([1,2,3,4])]){
  const expected=oracleBuffers(nonmanifold.positions,nonmanifoldIndex.triangleIndices(ids)),actual=incidenceContours(nonmanifold,incidence,ids);
  const edges=p=>Array.from({length:p.length/6},(_,i)=>{const a=[...p.subarray(i*6,i*6+3)].join(','),b=[...p.subarray(i*6+3,i*6+6)].join(',');return [a,b].sort().join('/')}).sort();
  assert.deepEqual(edges(actual.positions),edges(expected.positions),'non-manifold incidences, signed zero and degenerate triangles retain exact subset boundaries');
 }
 console.log('Indexed contours, exact subset selection, batching and shared GPU-buffer lifetime passed.');
} finally {await server.close()}
