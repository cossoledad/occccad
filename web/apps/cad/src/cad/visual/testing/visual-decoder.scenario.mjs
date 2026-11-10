import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const original=globalThis.Worker,workers=[];
class FakeWorker {
 constructor(){workers.push(this)}
 postMessage(data,transfer){this.data=structuredClone(data,{transfer});}
 terminate(){this.terminated=true;}
 complete(){const buffers={positions:new Float32Array([0,0,0,1,0,0,0,1,0]),indices:new Uint32Array([0,1,2]),faceIds:new Uint32Array([7])};this.onmessage({data:{id:this.data.id,buffer:this.data.buffer,buffers,edges:[],topologyVertices:[],association:{geometryId:'exact'}}});}
}
try {
 globalThis.Worker=FakeWorker;
 const {VisualDecoder}=await server.ssrLoadModule('/src/cad/visual/visual-decoder.ts');
 const decoder=new VisualDecoder(),a=new AbortController(),b=new AbortController();
 const firstBuffer=new ArrayBuffer(600000),secondBuffer=new ArrayBuffer(600000);
 const first=decoder.decode(firstBuffer,true,a.signal),second=decoder.decode(secondBuffer,true,b.signal);
 const rejected=assert.rejects(first,e=>e.name==='AbortError');
 assert.equal(firstBuffer.byteLength,0,'active bytes transfer without a clone');assert.equal(secondBuffer.byteLength,600000,'bounded decode lane retains queued ownership');
 const old=workers[0];a.abort();await rejected;
 assert.equal(old.terminated,true);assert.equal(secondBuffer.byteLength,0);
 old.complete(); // late result from a terminated generation cannot complete b
 workers[1].complete();const result=await second;
 assert.equal(result.decoded.association.geometryId,'exact');assert.equal(result.buffer.byteLength,600000);
 assert.deepEqual(result.decoded.mesh.triangles,[[0,1,2]]);assert.equal(result.decoded.mesh.buffers.faceIds[0],7);
 const active=new AbortController(),queued=new AbortController();
 const pending=decoder.decode(new ArrayBuffer(600000),true,active.signal),waiting=decoder.decode(new ArrayBuffer(600000),false,queued.signal);
 const stopped=assert.rejects(pending,e=>e.name==='AbortError'),removed=assert.rejects(waiting,e=>e.name==='AbortError');
 decoder.dispose();await Promise.all([stopped,removed]);assert.equal(workers.at(-1).terminated,true);
 const shared=new AbortController(),lane=new VisualDecoder(),before=workers.length;
 const requests=Array.from({length:3},()=>assert.rejects(lane.decode(new ArrayBuffer(600000),true,shared.signal),e=>e.name==='AbortError'));
 assert.equal(workers.length,before+1);shared.abort();await Promise.all(requests);
 assert.equal(workers.length,before+1,'aborting a shared scope cannot launch already-canceled queued jobs');lane.dispose();
 console.log('Transfer ownership, bounded decode queue, abort, late-result suppression and disposal passed.');
}finally{globalThis.Worker=original;await server.close()}
