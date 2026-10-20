import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(new URL("../../../../package.json",import.meta.url));
const {createServer}=await import(require.resolve("vite"));
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const {rectangularExtrudePlan,pendingStep,displayAcknowledgment,newerTask}=await server.ssrLoadModule("/src/features/assistant/task-model.ts");
 const context={snapshot:{workspaceId:"workspace",revisionId:"revision",sequence:1,documentId:"document",documentType:"PART"},workingBodyId:"body"};
 const plan=rectangularExtrudePlan(context,80,50,12);
 assert.equal(plan.steps.length,3);assert.equal(new Set(plan.steps.map(step=>step.id)).size,3);
 assert.equal(plan.steps[1].bindings.sketchId,plan.steps[0].id);assert.equal(plan.steps[2].bindings.sketchId,plan.steps[0].id);
 assert.deepEqual(Object.keys(plan.expectedHead).sort(),["revisionId","sequence","workspaceId"]);
 assert.throws(()=>rectangularExtrudePlan(context,NaN,50,12));
 const task={id:"task",documentId:"document",version:4,status:"ACTIVE",steps:plan.steps.map(step=>({...step,phase:"PLANNED"}))};
 task.steps[0].phase="COMMITTED";task.steps[0].result={receipt:{revisionId:"committed"}};
 assert.equal(displayAcknowledgment(task,{documentId:"document",revisionId:"revision"}),undefined,"old frame must not acknowledge a new commit");
 assert.equal(displayAcknowledgment(task,{documentId:"other-document",revisionId:"committed"}),undefined,"same revision text does not substitute owner identity");
 assert.deepEqual(displayAcknowledgment(task,{documentId:"document",revisionId:"committed"}),{stepId:plan.steps[0].id,revisionId:"committed"});
 task.steps[0].phase="DISPLAYED";assert.equal(pendingStep(task).id,plan.steps[1].id);
 task.steps[1].phase="SUPERSEDED";assert.equal(pendingStep(task).id,plan.steps[2].id,"explicit adoption of manual edits preserves a superseded receipt without claiming display");
 assert.equal(newerTask(task,{...task,version:3}),task,"late task response must not replace newer state");
 console.log("Automation recipe bindings, exact display acknowledgment and task response ordering passed.");
}finally{await server.close()}
