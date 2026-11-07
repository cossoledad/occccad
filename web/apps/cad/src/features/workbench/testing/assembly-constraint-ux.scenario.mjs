import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import {createServer} from "vite";

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../../../cad/assembly/assembly-constraint-ux.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const ux = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);

const connected = { instanceId: "a", kind: "FACE", persistentSelection: {}, resolution: { result: {
  supportingElementStatus: "CONNECTED", status: "RESOLVED", evidenceDigest: "evidence-a",
} } };
const broken = { instanceId: "b", kind: "FACE", persistentSelection: {}, resolution: { result: {
  supportingElementStatus: "NOT_CONNECTED", status: "MISSING", diagnosticCode: "MISSING", diagnostic: "face was deleted",
} } };
const ambiguous = { instanceId: "b", kind: "FACE", persistentSelection: {}, resolution: { result: {
  supportingElementStatus: "NOT_CONNECTED", status: "AMBIGUOUS", diagnosticCode: "PERSISTENT_SELECTION_AMBIGUOUS",
  diagnostic: "lineage produced multiple valid candidates",
} } };
const reconnectedPick = { instanceId: "c", kind: "FACE", geometryKey: "shape-after-cut", topologyId: 7 };
const connectedEdge = { instanceId: "edge-a", kind: "EDGE", persistentSelection: { expectedType: "EDGE" }, resolution: { result: {
  supportingElementStatus: "CONNECTED", status: "RESOLVED", evidenceDigest: "edge-evidence",
} } };
const connectedVertex = { instanceId: "vertex-a", kind: "VERTEX", persistentSelection: { expectedType: "VERTEX" }, resolution: { result: {
  supportingElementStatus: "CONNECTED", status: "RESOLVED", evidenceDigest: "vertex-evidence",
} } };
const incompleteEdge = { instanceId: "edge-b", kind: "EDGE", persistentSelection: { expectedType: "EDGE" }, resolution: { result: {
  supportingElementStatus: "NOT_CONNECTED", status: "SOURCE_UNAVAILABLE", diagnosticCode: "TOPOLOGY_HISTORY_INCOMPLETE",
  diagnostic: "target revision does not declare complete topology history",
} } };
assert.equal(ux.assemblySupportPresentation(connected).label, "有效");
assert.deepEqual(ux.assemblySupportPresentation(broken), {
  status: "NOT_CONNECTED", label: "需要重新选择", diagnosticCode: "MISSING", diagnostic: "face was deleted", evidenceDigest: undefined,
});
assert.equal(ux.assemblySupportPresentation(ambiguous).label, "需要重新选择");
assert.equal(ux.assemblySupportPresentation(ambiguous).diagnosticCode, "PERSISTENT_SELECTION_AMBIGUOUS");
assert.equal(ux.assemblySupportPresentation(reconnectedPick).label, "有效");
assert.deepEqual(ux.assemblySupportPresentation(connectedEdge), {
  status: "CONNECTED", label: "有效", diagnosticCode: undefined, diagnostic: undefined, evidenceDigest: "edge-evidence",
});
assert.equal(ux.assemblySupportPresentation(connectedVertex).label, "有效");
assert.equal(ux.assemblySupportPresentation(incompleteEdge).label, "需要重新选择");
assert.equal(ux.assemblySupportPresentation(incompleteEdge).diagnosticCode, "TOPOLOGY_HISTORY_INCOMPLETE");
assert.equal(ux.assemblyStatusAfterPreviewFailure("RESOLVING_GEOMETRY", false,
  [ux.assemblySupportPresentation(incompleteEdge), ux.assemblySupportPresentation(connectedVertex)]), "BROKEN");
assert.match(ux.validateReconnectCandidate({ instanceId: "a", kind: "PLANE" }, connected), /不同实例/);
assert.equal(ux.validateReconnectCandidate({ instanceId: "c", kind: "PLANE" }, connected), undefined);
assert.equal(ux.assemblyConstraintGlyph("VERIFIED", 12), 12);
assert.equal(ux.assemblyConstraintGlyph("BROKEN", 12), 20);
assert.equal(ux.assemblyStatusFromDiagnostic("IMPOSSIBLE: conflicting component"), "IMPOSSIBLE");
assert.equal(ux.assemblyStatusFromDiagnostic(undefined), "NOT_UPDATED");
assert.equal(ux.assemblyStatusAfterPreviewFailure("SOLVING", false, [ux.assemblySupportPresentation(connected)]), "NOT_UPDATED");
assert.equal(ux.assemblyStatusAfterPreviewFailure("SOLVING", false, [ux.assemblySupportPresentation(broken)]), "BROKEN");
assert.equal(ux.assemblyStatusAfterPreviewFailure("SOLVING", true, [ux.assemblySupportPresentation(connected)]), "NOT_UPDATED");
console.log("Assembly constraint UX tests passed.");

const pending = {id:'mate',kind:'COINCIDENT',first:connected,second:connected,evaluationStatus:'NOT_UPDATED',
  evaluationFailure:{code:'ASSEMBLY_PREFERENCE_NOT_CONVERGED',diagnosticId:'kept-diagnostic',phase:'SOLVING',retryable:false},evaluationSummary:'retained preference failure'};
const actionFailure=ux.assemblyConstraintActionFailure;
assert.equal(actionFailure('REFRESH_ASSEMBLY_CONSTRAINTS',[pending],'mate'),undefined);
assert.equal(actionFailure('REFRESH_ASSEMBLY_CONSTRAINTS',[{...pending,evaluationStatus:'BROKEN'}],'mate'),undefined);
assert.equal(actionFailure('UPDATE_ASSEMBLY_CONSTRAINTS',[pending],'mate').code,'ASSEMBLY_PREFERENCE_NOT_CONVERGED');
assert.equal(actionFailure('UPDATE_ASSEMBLY_CONSTRAINTS',[pending],'mate').diagnosticId,'kept-diagnostic');
assert.equal(actionFailure('UPDATE_ASSEMBLY_CONSTRAINTS',[{...pending,evaluationStatus:'VERIFIED'}],'mate'),undefined);
const other={...pending,id:'other',evaluationStatus:'BROKEN'};
assert.equal(actionFailure('UPDATE_ASSEMBLY_CONSTRAINTS',[{...pending,evaluationStatus:'VERIFIED'},other],'mate'),undefined,'a different constraint cannot mask a successful selected update');
assert.equal(actionFailure('UPDATE_ASSEMBLY_CONSTRAINTS',[{...pending,evaluationFailure:undefined}],'mate').code,'ASSEMBLY_UPDATE_INCOMPLETE');
assert.equal(actionFailure('UPDATE_ASSEMBLY_CONSTRAINTS',[{...pending,evaluationStatus:'BROKEN',evaluationFailure:undefined}],'mate').code,'NOT_CONNECTED');
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
const {normalizeOperationFailure}=await server.ssrLoadModule('/src/cad/command/operation-failure.ts');
assert.equal(normalizeOperationFailure({code:'ASSEMBLY_PREFERENCE_NOT_CONVERGED',message:'raw solver evidence'}).message,'位置优化尚未收敛，未采用新位置。可通过约束分析检查。');
} finally {await server.close();}

const referenceSource = await readFile(new URL("../../../cad/assembly/assembly-reference.ts", import.meta.url), "utf8");
const referenceOutput = ts.transpileModule(referenceSource, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const { assemblyGeometryRef } = await import(`data:text/javascript;base64,${Buffer.from(referenceOutput).toString("base64")}`);
const nestedPath = { rootDocumentId: "root", canonical: "sub/leaf", segments: [
  { instanceId: "sub", referencedDocumentId: "product" }, { instanceId: "leaf", referencedDocumentId: "part" }
] };
assert.deepEqual(assemblyGeometryRef({ kind: "face", id: "face", instanceId: "sub", instancePath: nestedPath,
  geometryKey: "leaf-geometry", topologyId: 3 }), {
  instanceId: "sub", instancePath: nestedPath, kind: "FACE", geometryKey: "leaf-geometry", topologyId: 3
});

for(const kind of ['FACE','EDGE','VERTEX','PLANE','AXIS','FRAME','BODY']){
 const published={instanceId:'p',kind,publicationRef:{publicationId:'port'},publicationResolution:{status:'CONNECTED'}};
 assert.equal(ux.assemblySupportPresentation(published).status,'CONNECTED',kind+' publication uses its own checked snapshot');
 const missing={...published,publicationResolution:{status:'BROKEN_PUBLICATION',diagnosticCode:'PUBLICATION_MISSING',diagnostic:'missing'}};
 assert.equal(ux.assemblySupportPresentation(missing).status,'NOT_CONNECTED');
 assert.equal(ux.assemblySupportPresentation(missing).diagnosticCode,'PUBLICATION_MISSING');
 const replaced={...published,resolution:broken.resolution};
 assert.equal(ux.assemblySupportPresentation(replaced).status,'NOT_CONNECTED','raw failure must override a stale connected publication');
}
