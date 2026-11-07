import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {AnnotationLayoutSession}=await server.ssrLoadModule('/src/cad/measurement/annotation-layout.ts');
 const calls=[],changes=[];let resolve;let stored={version:3,position:[5,6,-3]};
 const session=new AnnotationLayoutSession('doc',{load:async()=>({m:stored}),save:async(doc,id,input)=>{calls.push({doc,id,input});if(calls.length===1)await new Promise(r=>resolve=r);stored={version:input.version+1,position:input.position};return stored}},v=>changes.push(v),()=>assert.fail('unexpected failure'));
 await session.load();session.set('m',[10,20,-3]);session.set('m',[30,40,-3]);session.set('m',[50,60,-3]);
 assert.equal(calls.length,1);resolve();await new Promise(r=>setTimeout(r,0));
 assert.equal(calls.length,2);assert.equal(calls[1].input.version,4);assert.deepEqual(calls[1].input.position,[50,60,-3]);assert.deepEqual(changes.at(-1).m,{version:5,position:[50,60,-3]});
 session.set('m',undefined);await new Promise(r=>setTimeout(r,0));assert.equal(stored.position,null);session.dispose();session.set('m',[1,1,1]);assert.equal(calls.length,3);
 let retry;let failed=true;const recovery=new AnnotationLayoutSession('doc',{load:async()=>({m:{version:10,position:null}}),save:async(doc,id,input)=>{if(failed){failed=false;throw new Error('conflict')}assert.equal(input.version,10);return {...input,version:11}}},()=>{},r=>retry=r);
 recovery.set('m',[8,9,-10]);await new Promise(r=>setTimeout(r,0));assert.equal(typeof retry,'function');retry();await new Promise(r=>setTimeout(r,0));recovery.dispose();
 let late;let applied=0;const closed=new AnnotationLayoutSession('old',{load:()=>new Promise(r=>late=r),save:async()=>assert.fail('closed write')},()=>applied++,()=>assert.fail('closed failure'));
 const pending=closed.load();closed.dispose();late({m:stored});await pending;assert.equal(applied,0);
}finally{await server.close()}
