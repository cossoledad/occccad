import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(import.meta.url);
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

 const {copyDiagnosticText,diagnosticReference}=await server.ssrLoadModule("/src/cad/command/diagnostic-copy.tsx");
 const failed={code:"FAILED_SUPPORT",phase:"SKETCH_SUPPORT",diagnosticId:"doc/immutable-id",message:"ambiguous",requestId:"request-id",retryable:false};
 assert.equal(normalizeOperationFailure(failed).diagnosticId,failed.diagnosticId);
 assert.equal(diagnosticReference(failed),"CAD_DIAGNOSTIC doc/immutable-id FAILED_SUPPORT");
 let copied;assert.equal(await copyDiagnosticText("reference",{writeText:async value=>{copied=value;}}),true);assert.equal(copied,"reference");
 assert.equal(await copyDiagnosticText("reference",{writeText:async()=>{throw Error("permission denied");}}),false);
 assert.equal(await copyDiagnosticText("reference"),false,"unavailable clipboard offers manual text");
 assert.match(diagnosticReference(new Error("offline"),{documentId:"doc",versionId:"revision"}),/revision/);
 const error=Object.assign(new Error("invalid workspace command: secret UUID internal snapshot"),{code:"VALIDATION_FAILED",requestId:"request"});
 const view=normalizeOperationFailure(error);assert.doesNotMatch(view.message,/UUID|invalid workspace/);assert.match(view.details,/UUID/);assert.equal(view.requestId,"request");
 const gate=new OperationFailureGate();assert.equal(gate.accept(error,"command",100),true);assert.equal(gate.accept(error,"dialog",101),false,"same mutation rejection has one outlet");
 assert.equal(gate.accept({message:error.message,code:error.code,requestId:"request"},"command",102),false);
 assert.equal(gate.accept(new Error("network"),"move",1000),true);
 assert.equal(normalizeOperationFailure({code:"TIMEOUT",message:"rpc timeout"}).retryable,true);
 assert.match(normalizeOperationFailure({code:"PREVIEW_CANDIDATE_STALE_OR_MISMATCHED",message:"digest mismatch"}).message,/状态已变化/);
 const React=require("react"),{renderToStaticMarkup}=require("react-dom/server"),{App}=require("antd"),originalUseApp=App.useApp;
 const notices=[],diagnostics=[],dismissals=[];let retries=0,cancels=0,feedback;
 App.useApp=()=>({notification:{error:config=>notices.push(config),warning:config=>notices.push(config),destroy:key=>dismissals.push(key)},modal:{info:config=>diagnostics.push(config)},message:{success(){}}});
 try{
  const {useOperationFeedback}=await server.ssrLoadModule("/src/cad/command/operation-feedback.tsx");
  function Probe(){feedback=useOperationFeedback();return null;}renderToStaticMarkup(React.createElement(Probe));
  feedback({code:"TIMEOUT",message:"unknown receipt",requestId:"preserved-request",retry:()=>retries++,cancel:()=>cancels++},"草图");
  const recovery=notices.at(-1);assert.equal(recovery.duration,0);assert.equal(recovery.placement,"bottomRight");assert.equal(recovery.className,"cad-operation-notification");assert.equal(recovery.onClose,undefined,"dismissal never cancels an unknown operation");
  const buttons=recovery.description.props.children.filter(child=>child?.props?.onClick);
  buttons[0].props.onClick();buttons[1].props.onClick();assert.equal(retries,1);assert.equal(cancels,1);assert.equal(dismissals.length,2);
  buttons.at(-1).props.onClick();assert.equal(diagnostics[0].width,640);assert(diagnostics[0].content.props.children[0].props.children.includes("preserved-request"));
  feedback({code:"VALIDATION_FAILED",message:"ordinary failure",requestId:"ordinary-request"},"草图");assert.equal(notices.at(-1).duration,6);
 }finally{App.useApp=originalUseApp;}

}finally{await server.close();}
