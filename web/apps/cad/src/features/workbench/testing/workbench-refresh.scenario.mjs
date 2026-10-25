import assert from 'node:assert/strict';
import {createServer} from 'vite';
import {QueryClient,QueryObserver} from '@tanstack/react-query';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {refreshWorkbenchQueries:refresh}=await server.ssrLoadModule('/src/features/workbench/refresh-workbench-queries.ts');
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}}),unsub=[];let finishHistory,finishHost,finishDocument;
 const watch=(key,fn)=>{client.setQueryData(key,{accepted:true});const observer=new QueryObserver(client,{queryKey:key,queryFn:fn,staleTime:Infinity});unsub.push(observer.subscribe(()=>{}));return observer};
 const history=watch(['history','part'],()=>new Promise(resolve=>finishHistory=resolve));
 const host=watch(['document','product'],()=>new Promise(resolve=>finishHost=resolve));
 const document=watch(['document','part'],()=>new Promise(resolve=>finishDocument=resolve));
 try{
  await refresh(client,'part','part',true);
  assert.equal(history.getCurrentResult().isFetching,true,'history still refreshes after a commit');assert.equal(document.getCurrentResult().isFetching,false,'accepted snapshot needs no duplicate document fetch');
  let done=false;const cross=refresh(client,'part','product',true).then(()=>done=true);await Promise.resolve();assert.equal(done,false,'owning Product refresh remains an authoritative barrier');finishHost({revision:'accepted-host'});await cross;
  done=false;const read=refresh(client,'part','part',false).then(()=>done=true);await Promise.resolve();assert.equal(done,false,'missing snapshot must await a real document read');finishDocument({revision:'accepted-part'});await read;
  assert.equal(history.getCurrentResult().isFetching,true,'slow ancillary reads cannot prolong either barrier');finishHistory({transactions:['accepted']});await new Promise(resolve=>setTimeout(resolve,0));assert.equal(history.getCurrentResult().data.transactions[0],'accepted');
 }finally{unsub.forEach(fn=>fn());client.clear()}
 console.log('Accepted document barriers and independent ancillary refresh passed');
}finally{await server.close()}
