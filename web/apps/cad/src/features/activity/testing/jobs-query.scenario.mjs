import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {QueryClient,QueryObserver}=await server.ssrLoadModule('@tanstack/react-query');
 const {jobsQueryOptions,mergeJobEvent}=await server.ssrLoadModule('/src/features/activity/jobs-query.ts');
 const previous=[{id:'motion',attemptCount:2,completedAt:'2026-11-14T00:00:02Z',state:'RUNNING',payload:{result:'retained'}}];
 const event={id:'motion',attemptCount:2,completedAt:'2026-11-14T00:00:03Z',state:'SUCCEEDED'};
 const merged=mergeJobEvent(previous,event);assert.equal(merged[0].state,'SUCCEEDED');assert.equal(merged[0].payload,previous[0].payload);
 assert.equal(mergeJobEvent(previous,{...event,attemptCount:1}),previous);assert.equal(mergeJobEvent(previous,{...event,completedAt:'2026-11-14T00:00:01Z'}),previous);
 assert.equal(mergeJobEvent(previous,{...event,id:'unknown'}),previous);
 const client=new QueryClient({defaultOptions:{queries:{retry:false,staleTime:15_000}}});
 let reads=0;
 const options={...jobsQueryOptions(),queryFn:async()=>{reads++;return [{id:'exchange',type:'EXCHANGE_IMPORT',state:'SUCCEEDED'},{id:'motion',type:'MOTION_STUDY',documentId:'product',state:'RUNNING'}]}};
 const activity=new QueryObserver(client,options),motion=new QueryObserver(client,{...options,select:jobs=>jobs.filter(job=>job.type==='MOTION_STUDY')});
 const offActivity=activity.subscribe(()=>{}),offMotion=motion.subscribe(()=>{});
 try{
  await client.ensureQueryData(options);assert.equal(reads,1,'simultaneous observers share one request');
  assert.equal(activity.getCurrentResult().data.length,2);assert.deepEqual(motion.getCurrentResult().data.map(job=>job.id),['motion']);
  await client.invalidateQueries({queryKey:['jobs']});assert.equal(reads,2,'one event refreshes both observers once');
  const query=client.getQueryCache().find({queryKey:['jobs']});
  assert.equal(options.refetchInterval(query),10_000);
  client.setQueryData(['jobs'],[{id:'motion',state:'SUCCEEDED'}]);assert.equal(options.refetchInterval(query),false,'idle products do not poll');
 }finally{offActivity();offMotion();client.clear()}
}finally{await server.close()}
