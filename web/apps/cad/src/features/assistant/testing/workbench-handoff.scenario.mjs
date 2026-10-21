import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {assistantTarget,dispatchAssistantCommand,handoffEditorIntent,handoffMatchesCommit,handoffDispatchAllowed}=await server.ssrLoadModule("/src/features/assistant/workbench-handoff.ts");
 const node={entityId:"stable-id",documentId:"part"};assert.equal(assistantTarget([{children:[{entityId:"stable-id",documentId:"other"},node]}],"part","stable-id"),node,"deep targets must respect owner document");
 let calls=0;const registry={declaration:()=>({id:"part.fillet"}),state:()=>({enabled:true,visible:true}),execute:async()=>{calls++;return true}};
 await dispatchAssistantCommand(registry,"part.fillet");assert.equal(calls,1);
 registry.state=()=>({enabled:false,visible:true});await assert.rejects(()=>dispatchAssistantCommand(registry,"part.fillet"));assert.equal(calls,1,"disabled commands must not execute");
 registry.state=()=>({enabled:true,visible:false});await assert.rejects(()=>dispatchAssistantCommand(registry,"part.fillet"));assert.equal(calls,1,"hidden commands must not execute");
 registry.state=()=>({enabled:true,visible:true});registry.declaration=()=>undefined;await assert.rejects(()=>dispatchAssistantCommand(registry,"invented.command"));assert.equal(calls,1);
 const {pendingStep,displayAcknowledgment}=await server.ssrLoadModule("/src/features/assistant/task-model.ts");
 const task={documentId:"part",steps:[{phase:"HANDED_OFF",operationId:"ui.part.fillet"}]};assert.equal(pendingStep(task),undefined);assert.equal(displayAcknowledgment(task,{documentId:"part",revisionId:"head"}),undefined,"UI dispatch cannot manufacture a render receipt");
 const handoff={state:"OPENED",uiCommand:"part.fillet",documentId:"part",baselineRevisionId:"baseline",domainRequestId:"server-issued-request"};
 assert.equal(handoffEditorIntent(handoff,"part","baseline","FILLET"),"server-issued-request");
 assert.equal(handoffEditorIntent(handoff,"part","baseline","CHAMFER"),undefined);assert.equal(handoffEditorIntent(handoff,"other","baseline","FILLET"),undefined);assert.equal(handoffEditorIntent(handoff,"part","manual-head","FILLET"),undefined);
 assert.equal(handoffEditorIntent({...handoff,state:"COMMITTED"},"part","baseline","FILLET"),undefined,"completed handoffs cannot authorize another command");
 const commit={type:"CREATE_MODIFY_FEATURE",feature:{type:"FILLET"},requestId:"server-issued-request"};
 assert.equal(handoffMatchesCommit(handoff,"part","baseline",commit),true);
 assert.equal(handoffMatchesCommit(handoff,"part","baseline",{...commit,requestId:"unrelated-user-command"}),false);assert.equal(handoffMatchesCommit(handoff,"part","baseline",{...commit,type:"EDIT_FEATURE"}),false);
 assert.equal(handoffMatchesCommit(undefined,"part","baseline",{type:"CREATE_MODIFY_FEATURE",feature:{type:"FILLET"}}),false,"reload has no editor authorization; a missing requestId cannot match an absent handoff");
 const waiting={id:"run",epoch:3,status:"WAITING_UI",activeTaskId:"task"},claim={runId:"run",epoch:3,taskId:"task"};
 assert.equal(handoffDispatchAllowed(waiting,claim),true);assert.equal(handoffDispatchAllowed({...waiting,status:"STOPPED"},claim),false);assert.equal(handoffDispatchAllowed({...waiting,status:"PAUSED"},claim),false);assert.equal(handoffDispatchAllowed({...waiting,epoch:4},claim),false);assert.equal(handoffDispatchAllowed({...waiting,activeTaskId:"replacement"},claim),false,"Stop or a replacement plan received before dispatch retires the old UI claim");
 console.log("Workbench AI dispatch uses live availability and separates handoff from model completion.");
}finally{await server.close()}
