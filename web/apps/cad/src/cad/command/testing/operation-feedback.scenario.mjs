import assert from "node:assert/strict";
import {createServer} from "vite";
import {AssemblyInteractionController} from "../../assembly/assembly-interaction.ts";

// A sustained gesture reports real numerical failure once, never constrained
// motion; a new gesture owns a new feedback lifetime. This exercises the real
// controller, not just notification string matching.
const tick=()=>new Promise(resolve=>setImmediate(resolve));
const failures=[],pending=[];
const controller=new AssemblyInteractionController({
 begin:async()=>({sessionId:"session",bodyId:"body",baseRevisionId:"revision",inputDigest:"input",nominalPoses:[]}),
 update:request=>new Promise(resolve=>pending.push({request,resolve})),cancel:async()=>{},frame:()=>{},state:()=>{},failure:error=>failures.push(error),
});
const input={baseRevisionId:"revision",instanceId:"body",localGrabPoint:[0,0,0],frameRotation:[0,0,0,1]};
const target={localGrabPoint:[0,0,0],frameRotation:[0,0,0,1],targetPose:{translation:[1,1,1],rotation:[0,0,0,1]},translationComponents:[true,true,true],rotationComponents:[false,false,false]};
const respond=async status=>{
 controller.target(target);const {request,resolve}=pending.shift();
 resolve({sessionId:"session",inputDigest:"input",sequence:request.sequence,instancePoses:[],interaction:{status,hardFeasible:status==="CONSTRAINED",targetConverged:status==="CONSTRAINED",eligibleForCommit:false,targetSequence:request.sequence,targetError:1,targetOptimality:0}});await tick();
};
controller.begin(input);await tick();await respond("CONSTRAINED");assert.equal(failures.length,0);
await respond("FAILED");await respond("BUDGET");assert.equal(failures.length,1);assert.equal(controller.state,"blocked");
controller.cancel();controller.begin(input);await tick();await respond("FAILED");assert.equal(failures.length,2);controller.cancel();
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {normalizeOperationFailure,OperationFailureGate}=await server.ssrLoadModule("/src/cad/command/operation-feedback.tsx");
 const error=Object.assign(new Error("invalid workspace command: secret UUID internal snapshot"),{code:"VALIDATION_FAILED",requestId:"request"});
 const view=normalizeOperationFailure(error);assert.doesNotMatch(view.message,/UUID|invalid workspace/);assert.match(view.details,/UUID/);assert.equal(view.requestId,"request");
 const gate=new OperationFailureGate();assert.equal(gate.accept(error,"command",100),true);assert.equal(gate.accept(error,"dialog",101),false,"same mutation rejection has one outlet");
 assert.equal(gate.accept({message:error.message,code:error.code,requestId:"request"},"command",102),false);
 assert.equal(gate.accept(new Error("network"),"move",1000),true);
 assert.equal(normalizeOperationFailure({code:"TIMEOUT",message:"rpc timeout"}).retryable,true);
 assert.match(normalizeOperationFailure({code:"PREVIEW_CANDIDATE_STALE_OR_MISMATCHED",message:"digest mismatch"}).message,/状态已变化/);
}finally{await server.close();}
