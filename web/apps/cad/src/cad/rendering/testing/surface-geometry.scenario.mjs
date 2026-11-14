import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const T=await server.ssrLoadModule('three');
 const {SurfaceGeometryPool,makeSurfaceGeometry}=await server.ssrLoadModule('/src/cad/rendering/surface-geometry.ts');
 const {acceleratedRaycast}=await server.ssrLoadModule('three-mesh-bvh');
 const pool=new SurfaceGeometryPool(),mesh={vertices:[],triangles:[],buffers:{positions:new Float32Array([0,0,0,1,0,0,0,1,0,1,1,0]),indices:new Uint32Array([0,1,2,1,3,2]),faceIds:new Uint32Array([1,2])}};
 const a=pool.acquire(mesh),b=pool.acquire({...mesh,buffers:{...mesh.buffers}});
 assert.notEqual(a.geometry,b.geometry);assert.equal(a.geometry.getAttribute('position'),b.geometry.getAttribute('position'));
 assert.equal(a.geometry.getAttribute('normal'),b.geometry.getAttribute('normal'));assert.equal(a.geometry.index,b.geometry.index);
 assert.notEqual(a.geometry.boundsTree,b.geometry.boundsTree,'BVH retains each occurrence geometry/material-group context');
 a.geometry.addGroup(0,3,0);a.geometry.addGroup(3,3,1);b.geometry.addGroup(0,6,0);
 assert.equal(a.geometry.groups.length,2);assert.equal(b.geometry.groups.length,1);
 const object=new T.Mesh(a.geometry,[new T.MeshBasicMaterial({side:T.DoubleSide}),new T.MeshBasicMaterial({side:T.DoubleSide})]);object.raycast=acceleratedRaycast;
 const ray=new T.Raycaster(new T.Vector3(.8,.8,1),new T.Vector3(0,0,-1));
 assert.equal(ray.intersectObject(object)[0].faceIndex,1);assert.equal(ray.intersectObject(object)[0].face.materialIndex,1);
 let disposed=0;a.geometry.addEventListener('dispose',()=>disposed++);b.geometry.addEventListener('dispose',()=>disposed++);
 a.release();a.release();assert.equal(disposed,0,'releasing one sibling cannot delete a live shared GPU attribute');
 const c=pool.acquire(mesh);assert.equal(c.geometry,a.geometry,'a released wrapper/BVH is reused while other leases live');
 c.geometry.addGroup(0,6,0);assert.equal(b.geometry.groups.length,1);
 c.release();b.release();assert.equal(disposed,2,'all wrappers dispose exactly once at last release');
 assert.equal(a.geometry.boundsTree,null);assert.deepEqual([...mesh.buffers.indices],[0,1,2,1,3,2]);
 const fresh=pool.acquire(mesh);assert.notEqual(fresh.geometry.getAttribute('position'),a.geometry.getAttribute('position'));fresh.release();
 const raw=makeSurfaceGeometry({vertices:[[0,0,0],[1,0,0],[0,1,0]],triangles:[[0,1,2]],faceIds:[1]});assert.equal(raw.getAttribute('normal').getZ(0),1);raw.disposeBoundsTree();raw.dispose();
}finally{await server.close()}
