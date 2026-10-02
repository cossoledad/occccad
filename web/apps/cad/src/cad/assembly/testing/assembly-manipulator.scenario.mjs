import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import {createRequire} from "node:module";
import * as THREE from "three";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const ts=require("typescript");
const compile=async(file,replacements=[])=>{
 let source=await readFile(new URL(file,import.meta.url),"utf8");
 for(const [from,to]of replacements)source=source.replaceAll(from,to);
 const output=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
 return `data:text/javascript;base64,${Buffer.from(output).toString("base64")}`;
};
const three=JSON.stringify(import.meta.resolve("three"));
const metrics=await compile("../../rendering/viewport-metrics.ts",[['"three"',three]]);
const {AssemblyManipulator}=await import(await compile("../../interaction/assembly-manipulator.ts",[['"three"',three],['"../rendering/viewport-metrics"',JSON.stringify(metrics)]]));
const events=[];
const shaders={createMaterial:(_program,uniforms)=>new THREE.ShaderMaterial({uniforms:{uActive:{value:0},...Object.fromEntries(Object.entries(uniforms).map(([k,v])=>[k,{value:v}]))}})};
const m=new AssemblyManipulator(shaders,{poseChanged:()=>events.push("move"),visualChanged:()=>{},pivotChanged:()=>{},snapPivot:()=>undefined,dragStarted:()=>events.push("start"),dragFinished:commit=>events.push(commit?"finish":"cancel")});
const camera=new THREE.OrthographicCamera(-5,5,5,-5,.1,100);camera.position.set(0,0,10);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
const surface={clientWidth:1000,clientHeight:1000,getBoundingClientRect:()=>({width:1000,height:1000})};
const frozen=new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0,0,1),Math.PI/4);
m.attach(new THREE.Vector3(1,2,0),frozen);m.object.updateMatrixWorld(true);
// Invoke actual production drag logic, isolating only hit testing. Ray-plane
// motion has an independent orthographic geometry oracle (100px = 1 world mm).
const xy=m.handles.find(h=>h.plane?.join("")==="XY");m.pick=()=>xy;
assert.equal(m.pointerDown(1,600,300,camera,surface),true);
assert.deepEqual(m.components(),{translationComponents:[true,true,false],rotationComponents:[false,false,false]});
m.pointerMove(1,700,250,camera,surface);
assert.ok(m.candidatePose().position.distanceTo(new THREE.Vector3(2,2.5,0))<1e-12,JSON.stringify(m.candidatePose().position.toArray()));
assert.ok(m.frameQuaternion().angleTo(frozen)<1e-12);
// Final pointer coordinates must be sampled before releasing capture. They
// differ from the last move event; retained components are the final identity.
m.pointerMove(1,750,200,camera,surface);assert.equal(m.pointerUp(1,true),true);
assert.deepEqual(m.components().translationComponents,[true,true,false]);
assert.equal(events.at(-1),"finish");
// Every axis/plane has exactly its declared objective mask. Rotation also
// carries all grab point components, but never hidden rotation components.
for(const handle of m.handles.filter(h=>h.operation!=="pivot")){
 m.attach(new THREE.Vector3(),frozen);m.object.updateMatrixWorld(true);m.pick=()=>handle;
 assert.equal(m.pointerDown(2,500,500,camera,surface),true);
 const components=m.components(),axes=["X","Y","Z"];
 assert.deepEqual(components.translationComponents,axes.map(a=>handle.operation==="rotate"||handle.plane?.includes(a)||handle.axis===a));
 assert.deepEqual(components.rotationComponents,axes.map(a=>handle.operation==="rotate"&&handle.axis===a));
 assert.equal(m.pointerUp(2,false),true);assert.equal(events.at(-1),"cancel");
}
m.dispose();
