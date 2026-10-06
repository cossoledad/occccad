import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const {LatestPreviewQueue}=await server.ssrLoadModule('/src/features/workbench/latest-preview-queue.ts');
 const queue=new LatestPreviewQueue(),started=[],resolvers=[];let active=0,peak=0;
 const task=value=>signal=>new Promise(resolve=>{started.push({value,signal});peak=Math.max(peak,++active);resolvers.push(()=>{active--;resolve(value);});});
 const outcome=p=>p.then(value=>({value}),error=>({error:error.name}));
 const a=outcome(queue.submit(task(1),new AbortController().signal));
 const b=outcome(queue.submit(task(2),new AbortController().signal));
 const c=outcome(queue.submit(task(3),new AbortController().signal));
 assert.deepEqual(started.map(t=>t.value),[1]);assert(started[0].signal.aborted);
 assert.deepEqual(await b,{error:'AbortError'});
 resolvers.shift()();assert.deepEqual(await a,{error:'AbortError'});await Promise.resolve();
 assert.deepEqual(started.map(t=>t.value),[1,3]);resolvers.shift()();assert.deepEqual(await c,{value:3});assert.equal(peak,1);
 const cancel=new AbortController();cancel.abort();assert.deepEqual(await outcome(queue.submit(task(4),cancel.signal)),{error:'AbortError'});assert.equal(started.length,2);
 assert.deepEqual(await queue.submit(async()=>5,new AbortController().signal),5,'cancellation never poisons the next request');
 console.log('Preview keeps one active computation and only the latest pending input.');
}finally{await server.close()}
