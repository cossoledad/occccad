import assert from "node:assert/strict";
import { AssemblyInteractionController,assemblyInteractionFailureState } from "../assembly-interaction.ts";

const tick = () => new Promise(resolve => setImmediate(resolve));
for(const code of ["BASE_CHANGED","EDIT_CONTEXT_CHANGED","EXPIRED_OR_SCOPE_MISMATCH","CANCELLED","POLICY_OR_TARGET_MISMATCH","FINAL_CANDIDATE_STALE_OR_MISMATCHED"]){
 const error=Object.assign(new Error(`validation failed: ASSEMBLY_SESSION_${code}`),{code:"ASSEMBLY_OPERATION_FAILED"});
 assert.equal(assemblyInteractionFailureState(error),"invalidated");
}
assert.equal(assemblyInteractionFailureState(new Error("ASSEMBLY_SOLVER_NON_CONVERGENT")),"failed");
assert.equal(assemblyInteractionFailureState(new Error("database offline")),"failed");
const session = {sessionId:"s1",bodyId:"motion-unit",baseRevisionId:"r1",inputDigest:"frozen-input",nominalPoses:[{instanceId:"a",translation:[1,2,3],rotation:[0,0,0,1]}]};
const input = {baseRevisionId:"r1",instanceId:"a",localGrabPoint:[4,5,6],frameRotation:[0,0,0,1]};
const target = x => ({localGrabPoint:[4,5,6],frameRotation:[0,0,0,1],targetPose:{translation:[x,2,3],rotation:[0,0,0,1]},translationComponents:[true,false,false],rotationComponents:[false,false,false]});
const frame = (request, status="REACHED", overrides={}) => ({
  previewId:request.final?"final-token":"",requestId:`request-${request.sequence}`,sessionId:request.sessionId,inputDigest:"frozen-input",sequence:request.sequence,
  instancePoses:[{instanceId:"a",translation:request.target.targetPose.translation,rotation:[0,0,0,1]},{instanceId:"group-peer",translation:[request.target.targetPose.translation[0]+8,2,3],rotation:[0,0,0,1]}],
  commitCommand:request.final?{type:"MOVE_INSTANCE",sessionId:request.sessionId,previewId:"final-token",requestId:`request-${request.sequence}`,interactionTarget:request.target}:undefined,
  interaction:{status,hardFeasible:true,targetConverged:true,eligibleForCommit:request.final,targetError:status==="CONSTRAINED"?3:0,targetOptimality:0,targetSequence:request.sequence,...overrides},
});
const fixture = (begin=async()=>structuredClone(session)) => {
  const pending=[],frames=[],states=[],cancelled=[];
  const controller=new AssemblyInteractionController({begin,
    update:(request,signal)=>new Promise((resolve,reject)=>pending.push({request,signal,resolve,reject})),
    cancel:async id=>cancelled.push(id),frame:f=>frames.push(f),state:(value,reason)=>states.push({value,reason})});
  return {controller,pending,frames,states,cancelled};
};

// Constant pointer movement does not cancel the in-flight solve, and does not
// discard its accepted frame merely because a later desired target exists.
const f=fixture();f.controller.begin(input);await tick();
const nominal=structuredClone(f.controller.nominalPoses);
for(let x=1;x<=100;x++)f.controller.target(target(x));
assert.equal(f.pending.length,1);assert.equal(f.pending[0].request.sequence,1);
assert.equal(f.pending[0].signal.aborted,false);
f.pending[0].resolve(frame(f.pending[0].request));await tick();
assert.equal(f.frames.length,1);assert.equal(f.pending.length,2);
assert.equal(f.pending[1].request.target.targetPose.translation[0],100);
assert.deepEqual(f.controller.nominalPoses,nominal);
const finished=f.controller.finish();
assert.equal(f.pending.length,2); // no parallel final solve
f.pending[1].resolve(frame(f.pending[1].request,"CONSTRAINED"));await tick();
assert.equal(f.controller.state,"constrained");assert.equal(f.frames.length,2);
assert.equal(f.pending.length,3);assert.equal(f.pending[2].request.final,true);
assert.equal(f.pending[2].request.sequence,101);
f.pending[2].resolve(frame(f.pending[2].request,"CONSTRAINED"));
const candidate=await finished;
assert.equal(candidate.target.targetPose.translation[0],100);
assert.equal(candidate.previewId,"final-token");
assert.equal(candidate.commitCommand.requestId,"request-101");
assert.equal(candidate.interaction.status,"CONSTRAINED");
f.controller.committing();f.controller.invalidate("own-committed-r2");
assert.equal(f.controller.state,"committing");f.controller.committed();assert.equal(f.controller.state,"committed");

// A feasible Budget/Failed last frame cannot promote the previous candidate.
for(const status of ["BUDGET","FAILED","CANCELLED"]){
  const b=fixture();b.controller.begin(input);await tick();b.controller.target(target(3));
  b.pending[0].resolve(frame(b.pending[0].request));await tick();
  const finish=b.controller.finish();await tick();
  b.pending[1].resolve(frame(b.pending[1].request,status,{hardFeasible:status==="BUDGET",targetConverged:false,eligibleForCommit:false}));
  assert.equal(await finish,undefined);assert.notEqual(b.controller.state,"allowed");
  if(status!=="BUDGET")assert.equal(b.frames.length,1);
}

// Cancel/context invalidation fences every late callback; no cross-gesture
// finally may unlock the newer in-flight request or commit the cancelled one.
const c=fixture();c.controller.begin(input);await tick();c.controller.target(target(1));
const old=c.pending[0];const cancelledFinish=c.controller.finish();
c.controller.cancel("pointercancel");assert.equal(await cancelledFinish,undefined);
c.controller.begin(input);await tick();c.controller.target(target(7));
old.resolve(frame(old.request));await tick();assert.equal(c.frames.length,0);
assert.equal(c.pending.length,2);assert.equal(c.pending[1].signal.aborted,false);
c.controller.invalidate("externally-changed-r2");assert.equal(c.controller.state,"invalidated");
c.pending[1].resolve(frame(c.pending[1].request));await tick();assert.equal(c.frames.length,0);

// A late begin creates no leaked session after cancel; it is explicitly closed.
let resolveBegin;const late=fixture(()=>new Promise(resolve=>resolveBegin=resolve));
late.controller.begin(input);late.controller.target(target(1));late.controller.cancel("blur");
resolveBegin(session);await tick();assert.deepEqual(late.cancelled,["s1"]);assert.equal(late.pending.length,0);

// No-change final results close without constructing a Revision command.
const unchanged=fixture();unchanged.controller.begin(input);await tick();unchanged.controller.target(target(1));
unchanged.pending[0].resolve(frame(unchanged.pending[0].request));await tick();const noChange=unchanged.controller.finish();await tick();
const result=frame(unchanged.pending[1].request);result.previewId="";result.commitCommand=undefined;result.unchanged=true;
unchanged.pending[1].resolve(result);assert.equal((await noChange).unchanged,true);

// Response identity is the complete Session/input/sequence tuple, not only the
// numerically similar pose. A wrong sequence cannot update any accepted frame.
const wrong=fixture();wrong.controller.begin(input);await tick();wrong.controller.target(target(2));
const wrongResult=frame(wrong.pending[0].request);wrongResult.inputDigest="other-input";
wrong.pending[0].resolve(wrongResult);await tick();assert.equal(wrong.frames.length,0);assert.equal(wrong.controller.state,"failed");
