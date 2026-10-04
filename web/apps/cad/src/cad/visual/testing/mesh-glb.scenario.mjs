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
const mesh={vertices:[[0,0,0],[2,0,0],[0,3,0]],triangles:[[0,1,2]],faceIds:[1],edges:[{localId:7,points:[[0,0,0],[2,0,0]]}],topologyVertices:[{localId:9,point:[0,0,0]}]};
const visualization={schemaVersion:2,referenceGeometry:{datumPlanes:[],axisSystems:[]},primitives:[]};
const glb=encodeMeshGLB(mesh,visualization);
assert.deepEqual(decodeMeshGLB(glb),{mesh,visualization,association:undefined});
const invalid=glb.slice(0);new DataView(invalid).setUint32(12,0xffffffff,true);assert.throws(()=>decodeMeshGLB(invalid),/length/);
const broken=encodeMeshGLB({...mesh,triangles:[[0,1,10]]},visualization);assert.throws(()=>decodeMeshGLB(broken),/index/);
assert.throws(()=>decodeMeshGLB(encodeMeshGLB({...mesh,faceIds:[]},visualization)),/mapping count/);
const descriptor={geometryKey:"key",representations:{VISUAL:{objectId:"object",digest:createHash("sha256").update(new Uint8Array(glb)).digest("hex"),schemaVersion:2,size:glb.byteLength,contentType:"model/gltf-binary"}},visualization};
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
 assert.equal(native.mesh.triangles.length,12);assert.equal(native.mesh.edges.length,12);assert.equal(native.mesh.topologyVertices.length,8);assert.ok(native.mesh.faceIds.every(id=>id>=1&&id<=6));
 console.log("Native C++ GLB: faces, edges, vertices and stable mappings decoded");
}
console.log("CAD GLB codec and artifact loading scenarios passed");

function rewriteDocument(source, change) {
 const data=new DataView(source), length=data.getUint32(12,true);
 const doc=JSON.parse(new TextDecoder().decode(new Uint8Array(source,20,length)));
 change(doc);
 const encoded=new TextEncoder().encode(JSON.stringify(doc)), json=new Uint8Array(Math.ceil(encoded.length/4)*4).fill(32);json.set(encoded);
 const binary=new Uint8Array(source,28+length), out=new ArrayBuffer(28+json.length+binary.length), h=new DataView(out), bytes=new Uint8Array(out);
 for(const [offset,value] of [[0,0x46546c67],[4,2],[8,out.byteLength],[12,json.length],[16,0x4e4f534a],[20+json.length,binary.length],[24+json.length,0x004e4942]])h.setUint32(offset,value,true);
 bytes.set(json,20);bytes.set(binary,28+json.length);return out;
}
const multi=rewriteDocument(glb,doc=>{
 const face=structuredClone(doc.meshes[0].primitives[0]);
 doc.meshes[0].primitives.push(face);
 doc.meshes.push({primitives:[structuredClone(face)]});doc.nodes.push({mesh:1});doc.scenes[0].nodes.push(1);
});
const multiMesh=decodeMeshGLB(multi).mesh;
assert.deepEqual(multiMesh.faceIds,[1,1,1]);assert.deepEqual(multiMesh.triangles,[[0,1,2],[3,4,5],[6,7,8]]);
const wire={...visualization,primitives:[{id:"wire",kind:"POLYLINE",positions:[[1,2,3],[4,5,6]],indices:[0,1]}]};
const wireGLB=encodeMeshGLB(mesh,wire);
rewriteDocument(wireGLB,doc=>{assert.equal(typeof doc.extensions.OCCCCAD_visualization.primitives[0].positions,"number");assert.equal(typeof doc.extensions.OCCCCAD_visualization.primitives[0].indices,"number");assert.equal(doc.extensions.OCCCCAD_cad.stableIds,undefined);});
assert.deepEqual(decodeMeshGLB(wireGLB).visualization,wire);
assert.throws(()=>decodeMeshGLB(encodeMeshGLB({...mesh,faceIds:[0]},visualization)),/locator/);
const withNormals={...mesh,normals:[[0,0,1],[0,0,1],[0,0,1]]};assert.deepEqual(decodeMeshGLB(encodeMeshGLB(withNormals)).mesh,withNormals);
const paired=rewriteDocument(glb,doc=>doc.extensions.OCCCCAD_cad.association={geometryId:"geometry",namingDigest:"naming"});
const pairedView={...view,artifact:{...descriptor,geometryId:"geometry",representations:{VISUAL:{...descriptor.representations.VISUAL,digest:createHash("sha256").update(new Uint8Array(paired)).digest("hex"),size:paired.byteLength},NAMING:{digest:"naming"}}}};
try {
 globalThis.fetch=async()=>new Response(paired);
 const repository=new VisualRepository();
 await repository.hydrate(pairedView);
 await assert.rejects(repository.hydrate({...pairedView,artifact:{...pairedView.artifact,geometryId:"wrong"}}),/geometry identity/);
 await assert.rejects(repository.hydrate({...pairedView,artifact:{...pairedView.artifact,representations:{...pairedView.artifact.representations,NAMING:{digest:"wrong"}}}}),/Naming digest/);
 repository.dispose();
}finally{globalThis.fetch=originalFetch}
console.log("GLB v2 multiple primitives, binary wire, normals and association checks passed");
// A named production snapshot must carry its binding even if an otherwise
// well-formed GLB has been accidentally paired with the wrong descriptor.
try {
 globalThis.fetch=async()=>new Response(glb);
 const repository=new VisualRepository();
 await assert.rejects(repository.hydrate({...view,artifact:{...descriptor,geometryId:"geometry",representations:{...descriptor.representations,NAMING:{digest:"naming"}}}}),/identity mismatch/);
 repository.dispose();
 const missingNaming=rewriteDocument(paired,doc=>delete doc.extensions.OCCCCAD_cad.association.namingDigest);
 globalThis.fetch=async()=>new Response(missingNaming);
 const second=new VisualRepository();
 await assert.rejects(second.hydrate({...pairedView,artifact:{...pairedView.artifact,representations:{...pairedView.artifact.representations,VISUAL:{...pairedView.artifact.representations.VISUAL,size:missingNaming.byteLength,digest:createHash("sha256").update(new Uint8Array(missingNaming)).digest("hex")}}}}),/Naming digest mismatch/);
 second.dispose();
} finally {globalThis.fetch=originalFetch}
