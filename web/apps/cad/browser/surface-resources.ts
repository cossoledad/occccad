import * as THREE from 'three';
import {makeSurfaceGeometry,SurfaceGeometryPool} from '../src/cad/rendering/surface-geometry';
import {surfaceAppearanceMaterials} from '../src/cad/rendering/surface-appearance';
import type {MeshData} from '../src/types';

/** Same immutable mesh, camera, materials and poses. Counts real WebGL uploads;
 * wrapper counts are not GPU-buffer counts. Test-only, not a capacity test. */
export function measureSurfaceResources(data:MeshData,count:number,shared:boolean){
 const renderer=new THREE.WebGLRenderer({antialias:true,preserveDrawingBuffer:true});renderer.setSize(960,720);renderer.setPixelRatio(1);
 renderer.setClearColor('#eef1f4');renderer.outputColorSpace=THREE.SRGBColorSpace;
 const gl=renderer.getContext(),originalUpload=gl.bufferData.bind(gl),originalDelete=gl.deleteBuffer.bind(gl);
 let uploadedBytes=0,uploads=0,deletes=0;
 gl.bufferData=((...args:any[])=>{const source=args[1];uploads++;uploadedBytes+=typeof source==='number'?source:source?.byteLength??0;return (originalUpload as any)(...args)}) as typeof gl.bufferData;
 gl.deleteBuffer=(buffer)=>{deletes++;originalDelete(buffer)};
 const scene=new THREE.Scene(),camera=new THREE.PerspectiveCamera(32,960/720,.1,5000);camera.up.set(0,0,1);
 const bounds=new THREE.Box3(),pool=new SurfaceGeometryPool(),leases:Array<{geometry:THREE.BufferGeometry;release:()=>void}>=[],objects:THREE.Mesh[]=[];
 const start=performance.now();
 for(let i=0;i<count;i++){
  const lease=shared?pool.acquire(data):(()=>{const geometry=makeSurfaceGeometry(data);return {geometry,release:()=>{geometry.disposeBoundsTree();geometry.dispose()}}})();leases.push(lease);
  const materials=surfaceAppearanceMaterials(lease.geometry,data.buffers?.faceIds??data.faceIds,
   i===0?{faces:{[data.buffers?.faceIds[0]??data.faceIds[0]]:'#cf664d'}}:undefined,i%3===0?0x253849:i%3===1?0x416db5:0x8e9faf,
   color=>new THREE.MeshStandardMaterial({color,roughness:.5,metalness:.02}));
  const object=new THREE.Mesh(lease.geometry,materials);object.position.set((i%4)*55,Math.floor(i/4)*55,0);object.rotation.z=i*.13;scene.add(object);objects.push(object);bounds.expandByObject(object);
 }
 const prepareMs=performance.now()-start;
 scene.add(new THREE.HemisphereLight(0xffffff,0x607080,2));const light=new THREE.DirectionalLight(0xffffff,3);light.position.set(100,-150,220);scene.add(light);
 const center=bounds.getCenter(new THREE.Vector3()),radius=bounds.getSize(new THREE.Vector3()).length();camera.position.copy(center).add(new THREE.Vector3(.8,-1,.9).normalize().multiplyScalar(radius*1.7));camera.lookAt(center);
 const submit=performance.now();renderer.render(scene,camera);const submitMs=performance.now()-submit;
 const wait=performance.now();gl.finish();const finishMs=performance.now()-wait;
 const initial={uploads,uploadedBytes,drawCalls:renderer.info.render.calls,triangles:renderer.info.render.triangles,geometries:renderer.info.memory.geometries,image:renderer.domElement.toDataURL(),prepareMs,submitMs,finishMs};
 scene.remove(objects[0]);leases[0].release();renderer.render(scene,camera);gl.finish();
 const remaining={additionalUploads:uploads-initial.uploads,image:renderer.domElement.toDataURL(),glError:gl.getError()};
 for(let i=1;i<count;i++)leases[i].release();
 for(const object of objects)for(const material of Array.isArray(object.material)?object.material:[object.material])material.dispose();
 const released={deletes,geometries:renderer.info.memory.geometries};renderer.dispose();
 return {shared,count,vertices:data.buffers?.positions.length! / 3,initial,remaining,released};
}
