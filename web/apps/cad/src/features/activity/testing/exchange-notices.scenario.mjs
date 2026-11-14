import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {ExchangeNotices}=await server.ssrLoadModule('/src/features/activity/exchange-notices.ts');
 const notices=new ExchangeNotices(5),received=[];notices.listen(jobs=>received.push(jobs));
 const job=(id,state='SUCCEEDED')=>({id,type:'EXCHANGE_IMPORT',state,attemptCount:1,payload:{},warnings:[]});
 const batch=notices.begin();
 notices.receive(job('a'));await new Promise(resolve=>setTimeout(resolve,15));assert.equal(received.length,0,'completion waits while batch registration is open');
 batch.track(job('a','QUEUED'));batch.track(job('b','RUNNING'));batch.seal();assert.equal(received.length,0);
 notices.reconcile([job('b','FAILED'),job('old')]);assert.equal(received.length,1);assert.deepEqual(received[0].map(j=>j.state),['SUCCEEDED','FAILED']);
 notices.receive(job('a'));notices.receive(job('b','FAILED'));await new Promise(resolve=>setTimeout(resolve,15));assert.equal(received.length,1,'duplicates and historical reconciliation do not re-notify');
 const empty=notices.begin();empty.seal();assert.equal(received.length,1);
 notices.receive({...job('export'),type:'EXCHANGE_EXPORT'});await new Promise(resolve=>setTimeout(resolve,15));assert.equal(received.length,2,'standalone export retains download notice');
 notices.receive({...job('b'),attemptCount:2});await new Promise(resolve=>setTimeout(resolve,15));assert.equal(received.length,3,'a retried attempt can complete later');
 notices.receive(job('disposed'));notices.reset();await new Promise(resolve=>setTimeout(resolve,15));assert.equal(received.length,3,'logout cancels queued notices');
}finally{await server.close()}
