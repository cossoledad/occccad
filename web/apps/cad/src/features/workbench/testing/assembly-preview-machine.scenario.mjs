import assert from "node:assert/strict";
import { assemblyAutomaticPreviewAllowed, createAssemblyPreviewActor } from "../assembly-preview-machine.ts";

const actor = createAssemblyPreviewActor();
actor.start();

actor.send({ type: "START" });
assert.equal(actor.getSnapshot().value, "drafting");

actor.send({ type: "REQUEST", sequence: 1 });
assert.equal(actor.getSnapshot().value, "pending");

// A superseded response cannot overwrite the latest preview state.
actor.send({ type: "REQUEST", sequence: 2 });
actor.send({ type: "REJECT", sequence: 1, error: "stale failure" });
assert.equal(actor.getSnapshot().value, "pending");

actor.send({ type: "REJECT", sequence: 2, error: "solver failed",
  errorCode: "ASSEMBLY_SOLVER_NON_CONVERGENT", phase: "SOLVING" });
assert.equal(actor.getSnapshot().value, "failed");
assert.equal(actor.getSnapshot().context.errorCode, "ASSEMBLY_SOLVER_NON_CONVERGENT");

actor.send({ type: "REQUEST", sequence: 3 });
actor.send({ type: "RESOLVE", sequence: 3 });
assert.equal(actor.getSnapshot().value, "succeeded");

actor.send({ type: "CONFIRM" });
assert.equal(actor.getSnapshot().value, "committing");
assert.equal(assemblyAutomaticPreviewAllowed(actor.getSnapshot().value), false);
// Realtime/cache Head publication may precede the mutation's success callback.
// An automatic preview must not replace an in-flight commit with a stale error.
actor.send({ type: "REQUEST", sequence: 4 });
actor.send({ type: "REJECT", sequence: 4, error: "基线已变化" });
assert.equal(actor.getSnapshot().value, "committing");
assert.equal(actor.getSnapshot().context.error, undefined);
actor.send({ type: "COMMIT_SUCCESS" });
assert.equal(actor.getSnapshot().value, "committed");
assert.equal(assemblyAutomaticPreviewAllowed(actor.getSnapshot().value), false);

actor.send({ type: "RESET" });
assert.equal(actor.getSnapshot().value, "idle");
assert.equal(assemblyAutomaticPreviewAllowed(actor.getSnapshot().value), true);
assert.equal(actor.getSnapshot().context.error, undefined);
actor.stop();

const evidenceActor = createAssemblyPreviewActor();
evidenceActor.start();
evidenceActor.send({ type: "REQUEST", sequence: 10 });
evidenceActor.send({ type: "REQUEST", sequence: 11 });
evidenceActor.send({ type: "RESOLVE", sequence: 10, components: [{componentId:"stale"}] });
assert.equal(evidenceActor.getSnapshot().context.components, undefined);
evidenceActor.send({ type: "RESOLVE", sequence: 11, components: [{componentId:"current"}] });
assert.equal(evidenceActor.getSnapshot().context.components[0].componentId, "current");
evidenceActor.send({ type: "REQUEST", sequence: 12 });
assert.equal(evidenceActor.getSnapshot().context.components, undefined);
evidenceActor.send({ type: "CANCEL", sequence: 12 });
assert.equal(evidenceActor.getSnapshot().value, "idle");
assert.equal(evidenceActor.getSnapshot().context.components, undefined);
evidenceActor.send({type:"START"});
evidenceActor.send({type:"REQUEST",sequence:13});
evidenceActor.send({type:"CHANGE"}); // support/member/parameter identity changed before next request
evidenceActor.send({type:"RESOLVE",sequence:13,components:[{componentId:"obsolete-members"}]});
assert.equal(evidenceActor.getSnapshot().value,"drafting");
assert.equal(evidenceActor.getSnapshot().context.components,undefined);
evidenceActor.send({type:"REQUEST",sequence:14});
evidenceActor.send({type:"RESOLVE",sequence:13,definitionOnly:true});
assert.equal(evidenceActor.getSnapshot().value,"pending");
evidenceActor.send({type:"RESOLVE",sequence:14,definitionOnly:true});
assert.equal(evidenceActor.getSnapshot().value,"definitionReady","legal definition may confirm without successful pose evidence");
assert.equal(evidenceActor.getSnapshot().context.components,undefined);
evidenceActor.send({type:"CONFIRM"});
evidenceActor.send({type:"REFRESH",sequence:13});
assert.equal(evidenceActor.getSnapshot().value,"committing","stale candidate refresh cannot interrupt the commit");
evidenceActor.send({type:"REFRESH",sequence:14}); // explicit expired-candidate recovery re-evaluates the same draft
assert.equal(evidenceActor.getSnapshot().value,"pending");
evidenceActor.send({type:"RESOLVE",sequence:14,definitionOnly:true});
assert.equal(evidenceActor.getSnapshot().value,"definitionReady");
evidenceActor.send({type:"CONFIRM"});
evidenceActor.send({type:"COMMIT_SUCCESS"});
evidenceActor.send({type:"START"});
assert.equal(evidenceActor.getSnapshot().value,"drafting");
assert.equal(assemblyAutomaticPreviewAllowed(evidenceActor.getSnapshot().value),true);
evidenceActor.send({type:"REQUEST",sequence:15});
evidenceActor.send({type:"REJECT",sequence:15,error:"external Head changed"});
assert.equal(evidenceActor.getSnapshot().value,"failed","real baseline changes remain visible outside commit");
evidenceActor.send({type:"REQUEST",sequence:16});
evidenceActor.send({type:"RESOLVE",sequence:16});
evidenceActor.send({type:"CONFIRM"});
evidenceActor.send({type:"COMMIT_FAILURE",error:"CAS rejected"});
assert.equal(evidenceActor.getSnapshot().context.error,"CAS rejected","authoritative commit failures are still reported");
evidenceActor.send({type:"REQUEST",sequence:17});
evidenceActor.send({type:"RESOLVE",sequence:17});
evidenceActor.send({type:"CONFIRM"});
evidenceActor.send({type:"RESET"});
evidenceActor.send({type:"COMMIT_SUCCESS"});
assert.equal(evidenceActor.getSnapshot().value,"idle","context reset prevents late commit completion from reviving the panel");
evidenceActor.stop();
