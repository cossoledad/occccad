import assert from "node:assert/strict";
import { createAssemblyPreviewActor } from "../assembly-preview-machine.ts";

const actor = createAssemblyPreviewActor();
actor.start();

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

actor.send({ type: "RESET" });
assert.equal(actor.getSnapshot().value, "idle");
assert.equal(actor.getSnapshot().context.error, undefined);
actor.stop();
