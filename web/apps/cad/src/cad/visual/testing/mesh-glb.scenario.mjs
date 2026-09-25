import {createHash} from "node:crypto";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
const ts=createRequire(new URL("../../../../package.json",import.meta.url))("typescript");
const moduleURL=async(file,replacements=[])=>{
 let source=await readFile(new URL(file,import.meta.url),"utf8");
 let output=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
 for(const [from,to] of replacements)output=output.replace(from,to);
 return `data:text/javascript;base64,${Buffer.from(output).toString("base64")}`;
};
const codec=await moduleURL("../mesh-glb.ts"),{encodeMeshGLB,decodeMeshGLB}=await import(codec);
const {VisualRepository}=await import(await moduleURL("../visual-repository.ts",[['"./mesh-glb"',JSON.stringify(codec)]]));
const mesh={vertices:[[0,0,0],[2,0,0],[0,3,0]],triangles:[[0,1,2]],faceIds:[0],edges:[{localId:7,points:[[0,0,0],[2,0,0]]}],topologyVertices:[{localId:9,point:[0,0,0]}],stableIds:{"1:1":"face-anchor","2:7":"edge-anchor","3:9":"vertex-anchor"}};
const visualization={schemaVersion:1,referenceGeometry:{datumPlanes:[],axisSystems:[]},primitives:[]};
const glb=encodeMeshGLB(mesh,visualization);
assert.deepEqual(decodeMeshGLB(glb),{mesh,visualization});
const invalid=glb.slice(0);new DataView(invalid).setUint32(12,0xffffffff,true);assert.throws(()=>decodeMeshGLB(invalid),/length/);
const broken=encodeMeshGLB({...mesh,triangles:[[0,1,10]]},visualization);assert.throws(()=>decodeMeshGLB(broken),/index/);
assert.throws(()=>decodeMeshGLB(encodeMeshGLB({...mesh,faceIds:[]},visualization)),/mapping count/);
const descriptor={geometryKey:"key",representations:{VISUAL:{objectId:"object",digest:createHash("sha256").update(new Uint8Array(glb)).digest("hex"),schemaVersion:1,size:glb.byteLength,contentType:"model/gltf-binary"}},visualization};
const view={document:{id:"part"},artifact:descriptor};let calls=0;
const originalFetch=globalThis.fetch;
try{
 globalThis.fetch=async()=>{calls++;return new Response(glb)};
 const repository=new VisualRepository();
 const [first,second]=await Promise.all([repository.hydrate(view),repository.hydrate(view)]);
 assert.equal(calls,1,"deduplicate artifact downloads");assert.equal(first.artifact.mesh,second.artifact.mesh);
 assert.equal(view.artifact.mesh,undefined,"keep server/query state lightweight");assert.deepEqual(first.artifact.mesh,mesh);
 repository.dispose();
 // A verified preview can become the committed display without another GET or
 // decode, even after its transient download scope has been disposed.
 const main=new VisualRepository();
 const preview=main.previewRepository();
 const before=calls;
 const displayed=await preview.hydrate(view);
 preview.dispose();
 const committed=await main.hydrate(view);
 assert.equal(calls-before,1,"promotion reuses verified preview bytes");
 assert.equal(displayed.artifact.mesh,committed.artifact.mesh,"promotion reuses decoded geometry");
 main.dispose();
 const bounded=new VisualRepository();
 for(const objectId of ["older","newer"]){
  const scope=bounded.previewRepository();
  await scope.hydrate({...view,artifact:{...descriptor,representations:{VISUAL:{...descriptor.representations.VISUAL,objectId}}}});
  scope.dispose();
 }
 const boundedBefore=calls;
 await bounded.hydrate({...view,artifact:{...descriptor,representations:{VISUAL:{...descriptor.representations.VISUAL,objectId:"older"}}}});
 assert.equal(calls,boundedBefore+1,"retain only latest preview, not drag history");
 bounded.dispose();
 let fail=true;globalThis.fetch=async()=>{calls++;return fail?new Response("failed",{status:503}):new Response(glb)};
 const retry=new VisualRepository();await assert.rejects(retry.hydrate(view),/503/);fail=false;
 assert.deepEqual((await retry.hydrate(view)).artifact.mesh,mesh,"failed downloads can retry");retry.dispose();
 // Transient preview display uses the same GLB decoder but a cancellable file
 // request. Clearing the viewport aborts bytes, not merely the control response.
 const previewRepository=new VisualRepository();
 globalThis.fetch=(url,options)=>new Promise((resolve,reject)=>{
  assert.equal(url,"/api/documents/part/representations/object?previewId=candidate");
  assert.equal(options.credentials,"include");
  options.signal.addEventListener("abort",()=>reject(new DOMException("Aborted","AbortError")),{once:true});
 });
 const pending=previewRepository.hydrate({...view,artifact:{...descriptor,representationKind:"TRANSIENT_PREVIEW",representations:{VISUAL:{...descriptor.representations.VISUAL,url:"/api/documents/part/representations/object?previewId=candidate"}}}});
 const canceled=assert.rejects(pending,error=>error.name==="AbortError");previewRepository.dispose();await canceled;
}finally{globalThis.fetch=originalFetch}
if(process.env.OCCCCAD_TEST_GLB_FIXTURE){
 const bytes=await readFile(process.env.OCCCCAD_TEST_GLB_FIXTURE);
 const native=decodeMeshGLB(bytes.buffer.slice(bytes.byteOffset,bytes.byteOffset+bytes.byteLength));
 assert.equal(native.mesh.triangles.length,12);assert.equal(native.mesh.edges.length,12);assert.equal(native.mesh.topologyVertices.length,8);assert.equal(Object.keys(native.mesh.stableIds).length,26);
 console.log("Native C++ GLB: faces, edges, vertices and stable mappings decoded");
}
console.log("CAD GLB codec and artifact loading scenarios passed");
