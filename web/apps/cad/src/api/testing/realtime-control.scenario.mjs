import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../package.json", import.meta.url));
const ts = require("typescript");
async function compile(file, replacements = []) {
 let source = await readFile(new URL(file, import.meta.url), "utf8");
 for (const [from, to] of replacements) source = source.replaceAll(from, to);
 const output = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
 return `data:text/javascript;base64,${Buffer.from(output).toString("base64")}`;
}
const lifecycle = await compile("../websocket-lifecycle.ts"), uuid = await compile("../../utils/random-uuid.ts");
const perf = await compile("../../utils/performance.ts");
const { RealtimeClient } = await import(await compile("../realtime-client.ts", [
 ['"../utils/performance"', JSON.stringify(perf)], ['"./websocket-lifecycle"', JSON.stringify(lifecycle)], ['"../utils/random-uuid"', JSON.stringify(uuid)], ['import.meta.env.VITE_API_BASE_URL', '""'],
]));
const timers = new Map();
const nativeSet = globalThis.setTimeout, nativeClear = globalThis.clearTimeout;
globalThis.window = { location: { href: "http://cad.test/documents/part" },
 setTimeout: (fn, ms) => { const id = nativeSet(() => { timers.delete(id); fn(); }, ms); timers.set(id, { fn, ms }); return id; },
 clearTimeout: id => { timers.delete(id); nativeClear(id); },
};
globalThis.document = { cookie: "occccad_csrf=test" };
const protocol = "occccad.realtime.v1";
const sent = [], sockets = [];
let onRequest = () => {};
class Socket {
 static OPEN = 1;
 readyState = 0;
 constructor() { sockets.push(this); queueMicrotask(() => { this.readyState = 1; this.onopen?.(); }); }
 send(raw) { const request = JSON.parse(raw); sent.push(request); if (request.type === "connection.initialize.v1") this.reply(request, {}); else if (request.type === "document.subscribe.v1") this.reply(request, { documentId: "part", sequence: 0 }); else if (request.type === "document.unsubscribe.v1" || request.type === "workspace.preview.cancel.v1") this.reply(request, {}); else if (request.kind !== "ack") onRequest(this, request); }
 reply(request, payload, error) { queueMicrotask(() => this.onmessage?.({ data: JSON.stringify({ protocol, kind: error ? "error" : "response", correlationId: request.id, payload, error }) })); }
 event(sequence) { this.onmessage?.({ data: JSON.stringify({ protocol, kind: "event", type: "workspace.transaction.committed.v1", sequence, payload: { documentId: "part" } }) }); }
 close() { this.readyState = 3; queueMicrotask(() => this.onclose?.()); }
}
globalThis.WebSocket = Socket;
let snapshotSequence = 0, fetchCount = 0;
const snapshot = sequence => ({ documentId: "part", workspaceId: "workspace", sequence, view: { document: { id: "part", versionId: `revision-${sequence}` } } });
globalThis.fetch = async () => { fetchCount++; return Response.json(snapshot(snapshotSequence)); };
const tick = async () => { for (let i = 0; i < 15; i++) await new Promise(resolve => setImmediate(resolve)); };
const client = new RealtimeClient();
try {
 const events = [];
 const unsubscribe = await client.subscribe("part", event => events.push(event));
 assert.equal(events.at(-1).payload.view.document.versionId, "revision-0");
 // Lost acknowledgement: one new correlation per transport attempt, one stable
 // request ID for the logical command. Snapshot remains authoritative.
 let commandAttempts = 0;
 onRequest = (socket, request) => {
  if (request.type !== "workspace.command.execute.v1") return;
  commandAttempts++;
  if (commandAttempts === 1) socket.close();
  else { snapshotSequence = 1; socket.reply(request, { requestId: request.payload.command.requestId, sequence: 1 }); }
 };
 const result = await client.executeCommand("part", { type: "CREATE_DATUM_PLANE" });
 assert.equal(result.document.versionId, "revision-1");
 const commands = sent.filter(e => e.type === "workspace.command.execute.v1");
 assert.equal(commands.length, 2); assert.equal(commands[0].payload.command.requestId, commands[1].payload.command.requestId); assert.notEqual(commands[0].id, commands[1].id);
 await tick();
 // Small authoritative snapshots complete without another HTTP request.
 const inlineFetches=fetchCount;
 onRequest=(socket,request)=>{ if(request.type==="workspace.command.execute.v1") socket.reply(request,{sequence:2,versionId:"revision-2",view:snapshot(2).view}); };
 assert.equal((await client.executeCommand("part",{type:"CREATE_DATUM_PLANE"})).document.versionId,"revision-2");
 assert.equal(fetchCount,inlineFetches,"inline command does not fetch HTTP snapshot");
 // A delayed receipt must not roll the observed stream back.
 onRequest=(socket,request)=>{ if(request.type==="workspace.command.execute.v1") { snapshotSequence=2; socket.reply(request,{sequence:1,versionId:"revision-1",view:snapshot(1).view}); } };
 assert.equal((await client.executeCommand("part",{type:"CREATE_DATUM_PLANE"})).document.versionId,"revision-2");
 assert.equal(fetchCount,inlineFetches+1,"old receipt falls back to authority");
 // The client's own recovery handles gaps, even if the UI listener only records.
 snapshotSequence = 4;
 sockets.at(-1).event(4); await tick();
 assert.equal(events.at(-1).type, "document.snapshot.v1"); assert.equal(events.at(-1).sequence, 4);
 const count = fetchCount; sockets.at(-1).event(3); await tick(); assert.equal(fetchCount, count);
 // Supersede, delayed ready and explicit abort never resolve a stale preview.
 const requests = [];
 onRequest = (socket, request) => { if (request.type === "workspace.preview.request.v1") requests.push({ socket, request }); };
 const first = client.previewCommand("part", { type: "MOVE_INSTANCE", interactionId: "drag", previewSequence: 1 });
 const firstRejected = assert.rejects(first, error => error.name === "AbortError"); await tick();
 const controller = new AbortController();
 const second = client.previewCommand("part", { type: "MOVE_INSTANCE", interactionId: "drag", previewSequence: 2 }, controller.signal);
 await tick(); await firstRejected;
 const ready = item => item.socket.reply(item.request, { interactionId: "drag", previewSequence: item.request.payload.previewSequence, preview: { previewId: `candidate-${item.request.payload.previewSequence}`, instancePoses: [] } });
 ready(requests[0]); ready(requests[1]);
 assert.equal((await second).previewId, "candidate-2");
 controller.abort(); await tick();
 assert.ok(sent.some(e => e.type === "workspace.preview.cancel.v1" && e.payload.previewSequence === 2));
 const timed = client.previewCommand("part", { type: "MOVE_INSTANCE", interactionId: "timed", previewSequence: 1 });
 const timeoutRejected = assert.rejects(timed, error => error.code === "TIMEOUT"); await tick();
 const [timer, entry] = [...timers].find(([, value]) => value.ms === 125_000);
 window.clearTimeout(timer); entry.fn(); await timeoutRejected;
 assert.ok(sent.some(e => e.type === "workspace.preview.cancel.v1" && e.payload.interactionId === "timed"));
 // Confirmation refresh keeps the draft identity while wire sequences advance.
 onRequest = (socket, request) => { if (request.type === "workspace.preview.request.v1") socket.reply(request, {interactionId:request.payload.interactionId,previewSequence:request.payload.previewSequence,preview:{previewId:`refresh-${request.payload.previewSequence}`,baseSequence:4}}); };
 const refreshSequences=[];
 for(let attempt=0;attempt<3;attempt++) refreshSequences.push((await client.previewCommand("part",{type:"ADD_ASSEMBLY_CONSTRAINT",interactionId:"confirmation"})).previewId);
 assert.deepEqual(refreshSequences,["refresh-1","refresh-2","refresh-3"]);
 // M4 uses direct typed RPC responses, not the generic preview-ready envelope.
 // Server-bound final MOVE keeps its exact logical requestId across retries.
 const interactionTarget={bodyId:"a",localGrabPoint:[1,2,3],targetPose:{translation:[4,5,6],rotation:[0,0,0,1]},frameRotation:[0,0,0,1],translationComponents:[true,false,false],rotationComponents:[false,false,false],targetSequence:9};
 const commitCommand={type:"MOVE_INSTANCE",requestId:"final-move-receipt",sessionId:"transient-solver-session",previewId:"exact-final",interactionTarget};
 onRequest=(socket,request)=>{
  if(request.type==="assembly.interaction.begin.v1")socket.reply(request,{sessionId:"transient-solver-session",bodyId:"a",baseRevisionId:"r4",inputDigest:"frozen",nominalPoses:[]});
  if(request.type==="assembly.interaction.update.v1")socket.reply(request,{sessionId:"transient-solver-session",sequence:9,inputDigest:"frozen",requestId:commitCommand.requestId,previewId:"exact-final",instancePoses:[],commitCommand,interaction:{status:"CONSTRAINED",hardFeasible:true,targetConverged:true,eligibleForCommit:true,targetSequence:9,targetError:2,targetOptimality:0}});
  if(request.type==="workspace.command.execute.v1")socket.reply(request,{sequence:4,versionId:"revision-4",view:snapshot(4).view});
 };
 const session=await client.beginAssemblyInteraction("part",{baseRevisionId:"r4",instanceId:"a",localGrabPoint:[1,2,3],frameRotation:[0,0,0,1]});
 const final=await client.updateAssemblyInteraction("part",{sessionId:session.sessionId,sequence:9,final:true,target:interactionTarget});
 assert.equal(final.interaction.status,"CONSTRAINED");assert.equal(final.previewId,"exact-final");
 await client.executeCommand("part",final.commitCommand);await client.executeCommand("part",final.commitCommand);
 const moves=sent.filter(e=>e.type==="workspace.command.execute.v1"&&e.payload.command.sessionId===session.sessionId);
 assert.equal(moves.length,2);for(const move of moves)assert.deepEqual(move.payload.command,commitCommand);
 let lateBegin;
 onRequest=(socket,request)=>{
  if(request.type==="assembly.interaction.begin.v1")lateBegin={socket,request};
  if(request.type==="assembly.interaction.cancel.v1")socket.reply(request,{});
 };
 const beginAbort=new AbortController(),lateSession=client.beginAssemblyInteraction("part",{baseRevisionId:"r4",instanceId:"a",localGrabPoint:[0,0,0],frameRotation:[0,0,0,1]},beginAbort.signal);
 await tick();beginAbort.abort();lateBegin.socket.reply(lateBegin.request,{sessionId:"cancelled-late-begin",bodyId:"a",baseRevisionId:"r4",inputDigest:"frozen",nominalPoses:[]});
 // Receiving the identity does not activate it: controller epoch handles that.
 // It permits explicit cleanup instead of silently leaking the allocated ID.
 const allocated=await lateSession;await client.cancelAssemblyInteraction("part",allocated.sessionId);
 assert.ok(sent.some(e=>e.type==="assembly.interaction.cancel.v1"&&e.payload.sessionId==="cancelled-late-begin"));
 // Analysis Abort cancels actual server work using the transport analysisId;
 // it cannot merely abandon a local Promise and leave numerical probes running.
 let delayedAnalysis;
 onRequest=(socket,request)=>{
  if(request.type==="assembly.conflict.analyze.v1")delayedAnalysis={socket,request};
  if(request.type==="assembly.conflict.cancel.v1")socket.reply(request,{});
 };
 const analysisAbort=new AbortController();
 const analysis=client.analyzeAssemblyConflicts("part",{analysisId:"analysis-identity",baseRevisionId:"r4",maxProbes:7,timeBudgetMs:500},analysisAbort.signal);
 const analysisRejected=assert.rejects(analysis,e=>e.name==="AbortError");await tick();analysisAbort.abort();await analysisRejected;await tick();
 assert.ok(delayedAnalysis);assert.equal(delayedAnalysis.request.payload.maxProbes,7);
 assert.equal(sent.filter(e=>e.type==="assembly.conflict.cancel.v1"&&e.payload.analysisId==="analysis-identity").length,1);
 delayedAnalysis.socket.reply(delayedAnalysis.request,{status:"UNSAT"});await tick();
 // Validation failures and oversize messages must never be retried as transport loss.
 onRequest = (socket, request) => socket.reply(request, undefined, { code: "VALIDATION_FAILED", message: "bad input", retryable: false, phase: "SOLVING", details: "PREVIEW_CANDIDATE_STALE_OR_MISMATCHED: candidate_expired" });
 const before = sent.length;
 await assert.rejects(client.executeCommand("part", { type: "BAD" }), error => error.code === "VALIDATION_FAILED" && error.phase === "SOLVING" && error.details.endsWith("candidate_expired"));
 assert.equal(sent.length, before + 1);
 await assert.rejects(client.executeCommand("part", { type: "BIG", value: "x".repeat(1 << 20) }), error => error.code === "MESSAGE_TOO_LARGE");
 const connectionCount = sockets.length;
 client.stop(); unsubscribe(); await tick();
 assert.equal(sockets.length, connectionCount, "unmount cleanup must not reconnect after stop");
} finally { client.stop(); for (const id of timers.keys()) window.clearTimeout(id); }
console.log("Realtime control: retry identity, snapshots, gap recovery, preview supersede/cancel/timeout and message bounds passed");
