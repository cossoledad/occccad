import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {assistantTarget,dispatchAssistantCommand}=await server.ssrLoadModule("/src/features/assistant/workbench-handoff.ts");
 const node={entityId:"stable-id",documentId:"part"};assert.equal(assistantTarget([{children:[{entityId:"stable-id",documentId:"other"},node]}],"part","stable-id"),node,"deep targets must respect owner document");
 let calls=0;const registry={declaration:()=>({id:"part.fillet"}),state:()=>({enabled:true,visible:true}),execute:async()=>{calls++;return true}};
 await dispatchAssistantCommand(registry,"part.fillet");assert.equal(calls,1);
 registry.state=()=>({enabled:false,visible:true});await assert.rejects(()=>dispatchAssistantCommand(registry,"part.fillet"));assert.equal(calls,1,"disabled commands must not execute");
 registry.state=()=>({enabled:true,visible:false});await assert.rejects(()=>dispatchAssistantCommand(registry,"part.fillet"));assert.equal(calls,1,"hidden commands must not execute");
 registry.state=()=>({enabled:true,visible:true});registry.declaration=()=>undefined;await assert.rejects(()=>dispatchAssistantCommand(registry,"invented.command"));assert.equal(calls,1);
 const {pendingStep,displayAcknowledgment}=await server.ssrLoadModule("/src/features/assistant/task-model.ts");
 const task={documentId:"part",steps:[{phase:"HANDED_OFF",operationId:"ui.part.fillet"}]};assert.equal(pendingStep(task),undefined);assert.equal(displayAcknowledgment(task,{documentId:"part",revisionId:"head"}),undefined,"UI dispatch cannot manufacture a render receipt");
 console.log("Workbench AI dispatch uses live availability and separates handoff from model completion.");
}finally{await server.close()}
