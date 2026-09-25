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
const { RealtimeClient } = await import(await compile("../realtime-client.ts", [
 ['"./websocket-lifecycle"', JSON.stringify(lifecycle)], ['"../utils/random-uuid"', JSON.stringify(uuid)], ['import.meta.env.VITE_API_BASE_URL', '""'],
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
 const [timer, entry] = [...timers].find(([, value]) => value.ms === 20_000);
 window.clearTimeout(timer); entry.fn(); await timeoutRejected;
 assert.ok(sent.some(e => e.type === "workspace.preview.cancel.v1" && e.payload.interactionId === "timed"));
 // Validation failures and oversize messages must never be retried as transport loss.
 onRequest = (socket, request) => socket.reply(request, undefined, { code: "VALIDATION_FAILED", message: "bad input", retryable: false, phase: "SOLVING" });
 const before = sent.length;
 await assert.rejects(client.executeCommand("part", { type: "BAD" }), error => error.code === "VALIDATION_FAILED" && error.phase === "SOLVING");
 assert.equal(sent.length, before + 1);
 await assert.rejects(client.executeCommand("part", { type: "BIG", value: "x".repeat(1 << 20) }), error => error.code === "MESSAGE_TOO_LARGE");
 const connectionCount = sockets.length;
 client.stop(); unsubscribe(); await tick();
 assert.equal(sockets.length, connectionCount, "unmount cleanup must not reconnect after stop");
} finally { client.stop(); for (const id of timers.keys()) window.clearTimeout(id); }
console.log("Realtime control: retry identity, snapshots, gap recovery, preview supersede/cancel/timeout and message bounds passed");
