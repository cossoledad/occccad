import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {mergeConversation,latestPlan}=await server.ssrLoadModule("/src/features/assistant/conversation-model.ts");
 const first={id:"a",requestId:"request",createdAt:"2026-10-20T00:00:00.000000001Z",status:"THINKING"};
 const complete={...first,status:"COMPLETED",taskId:"task"};
 let state=mergeConversation(undefined,{turns:[complete],hasEarlier:false});
 state=mergeConversation(state,{turns:[first],hasEarlier:false});
 assert.equal(state.turns[0].status,"COMPLETED","late polling must not revive a completed provider request");
 assert.equal(latestPlan(state.turns),"task");
 const next={...first,id:"b",requestId:"new",createdAt:"2026-10-20T00:00:00.000000002Z",status:"COMPLETED",taskId:"next-task"};
 state=mergeConversation(state,{turns:[next],hasEarlier:false});
 state=mergeConversation(state,{turns:[complete],hasEarlier:false});
 assert.equal(state.turns.length,2,"an old list response must not erase a newly accepted plan");
 assert.equal(latestPlan(state.turns),"next-task");
 state=mergeConversation(state,{turns:Array.from({length:35},(_,i)=>({...first,id:`other-${i}`,createdAt:`2026-10-20T00:00:${String(i+1).padStart(2,"0")}.000000000Z`,status:"CANCELLED"})),hasEarlier:false});
 assert.equal(state.turns.length,30);assert.equal(state.hasEarlier,true);assert.equal(latestPlan(state.turns),undefined,"cancelled turns do not introduce executable plans");
 console.log("Conversation ordering, terminal result recovery and bounded history passed.");
}finally{await server.close()}
