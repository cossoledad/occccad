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
  const update=contourProjectionUpdater(line,graph);update(camera);const initial=line.geometry;
  update(camera);assert.equal(line.geometry,initial,'stationary camera reuses projection and GPU attributes');
  camera.position.x+=1;camera.updateMatrixWorld(true);update(camera);assert.notEqual(line.geometry,initial);
  line.geometry.dispose();graph.dispose();
  geometry.dispose();
 }
 const mesh={vertices:[[0,0,0],[1,0,0],[1,1,0],[0,1,0]],triangles:[[0,1,2],[0,2,3]],faceIds:[1,2],edges:[{localId:9,points:[[0,0,0],[1,0,0],[1,1,0]]}],topologyVertices:[]};
 const index=new MeshTopologyIndex(mesh);
 assert.deepEqual([...index.triangleIndices(new Set([2]))],[0,2,3]);
 assert.equal(index.contours().positions.length,24);assert.equal(index.contours(new Set([1])).positions.length,18);
 assert.deepEqual([...index.edgeSegments(new Set([9]))],[0,0,0,1,0,0,1,0,0,1,1,0]);
 const cache=new InteractionGeometryCache(0),first=cache.acquire(mesh,'face'),second=cache.acquire(mesh,'face');
 assert.equal(first.geometry,second.geometry,'two occurrences borrow the same immutable derived buffers');
 let disposed=0;first.geometry.addEventListener('dispose',()=>disposed++);
 first.release();assert.equal(disposed,0,'hover release preserves another occurrence selection');
 second.release();assert.equal(disposed,1);second.release();assert.equal(disposed,1);
 assert.equal(cache.retainedBytes,0);cache.dispose();
 console.log('Indexed contours, exact subset selection, batching and shared GPU-buffer lifetime passed.');
} finally {await server.close()}
