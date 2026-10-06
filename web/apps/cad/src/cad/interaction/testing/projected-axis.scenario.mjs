import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {resolveSketchReference,preselectedSketchReference}=await server.ssrLoadModule("/src/cad/interaction/sketch-reference-pick.ts");
 const {resolveSketchSnap}=await server.ssrLoadModule("/src/cad/interaction/sketch-snap.ts");
 const axis={id:"axis",kind:"LINE",role:"CONSTRUCTION",infinite:true,start:{x:20,y:30},end:{x:21,y:30}};
 const project=([x,y])=>({x,y});
 assert.equal(resolveSketchReference({x:100,y:30},[axis],project,"LINE",5,0).subElement,"DIRECTION");
 assert.equal(resolveSketchReference({x:21,y:30},[axis],project,"POINT",5,0),null,"canonical unit vector has no selectable endpoint");
 assert.equal(resolveSketchReference({x:21,y:30},[axis],project,"CIRCULAR",5,0),null);
 assert.equal(preselectedSketchReference(axis,"LINE").subElement,"DIRECTION");
 assert.equal(preselectedSketchReference(axis,"EQUAL_CURVE"),undefined);
 assert.equal(resolveSketchSnap([21,30],[axis],1,10,5,["ENDPOINT","MIDPOINT"]),undefined);
 assert.deepEqual(resolveSketchSnap([100,32],[axis],1,10,5,["CURVE"]).point,[100,30]);
 const THREE=await server.ssrLoadModule("three");
 const {makeSketchReferenceAxis}=await server.ssrLoadModule("/src/cad/rendering/sketch-reference-axis.ts");
 const {updateScreenLines}=await server.ssrLoadModule("/src/cad/rendering/screen-space-lines.ts");
 const camera=new THREE.OrthographicCamera(-100,100,75,-75,1,1000);camera.position.z=200;camera.updateProjectionMatrix();
 const group=new THREE.Group();group.position.set(15,25,0);group.rotation.z=Math.PI/2;
 const line=makeSketchReferenceAxis(new THREE.Vector3(20,30,0),new THREE.Vector3(1,0,0),new THREE.Color(0x00ffff));group.add(line);
 updateScreenLines(group,camera,800,600);
 const a=new THREE.Vector3().fromBufferAttribute(line.geometry.getAttribute("instanceStart"),0).applyMatrix4(line.matrixWorld);
 const b=new THREE.Vector3().fromBufferAttribute(line.geometry.getAttribute("instanceEnd"),0).applyMatrix4(line.matrixWorld);
 assert.ok(a.clone().add(b).multiplyScalar(.5).distanceTo(new THREE.Vector3(-15,45,0))<1e-6,"reference remains on the transformed occurrence axis");
 assert.ok(Math.abs(a.distanceTo(b)-80)<1e-6,"direction display keeps 320 CSS pixels");
 console.log("Projected axes preserve infinite-direction, snapping and occurrence display semantics.");
}finally{await server.close()}
