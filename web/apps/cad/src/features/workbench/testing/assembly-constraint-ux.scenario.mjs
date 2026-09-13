import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";

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
assert.equal(ux.assemblySupportPresentation(connected).label, "Connected");
assert.deepEqual(ux.assemblySupportPresentation(broken), {
  status: "NOT_CONNECTED", label: "NotConnected", diagnosticCode: "MISSING", diagnostic: "face was deleted", evidenceDigest: undefined,
});
assert.equal(ux.assemblySupportPresentation(ambiguous).label, "NotConnected");
assert.equal(ux.assemblySupportPresentation(ambiguous).diagnosticCode, "PERSISTENT_SELECTION_AMBIGUOUS");
assert.equal(ux.assemblySupportPresentation(reconnectedPick).label, "Connected");
assert.equal(ux.firstDisconnectedSupport({ first: connected, second: broken, evaluationStatus: "BROKEN" }), 1);
assert.match(ux.validateReconnectCandidate({ instanceId: "a", kind: "PLANE" }, connected), /不同实例/);
assert.equal(ux.validateReconnectCandidate({ instanceId: "c", kind: "PLANE" }, connected), undefined);
assert.equal(ux.assemblyConstraintGlyph("VERIFIED", 12), 12);
assert.equal(ux.assemblyConstraintGlyph("BROKEN", 12), 20);
assert.equal(ux.assemblyStatusFromDiagnostic("IMPOSSIBLE: conflicting component"), "IMPOSSIBLE");
assert.equal(ux.assemblyStatusFromDiagnostic(undefined), "NOT_UPDATED");
assert.equal(ux.assemblyStatusAfterPreviewFailure("SOLVING", false, [ux.assemblySupportPresentation(connected)]), "IMPOSSIBLE");
assert.equal(ux.assemblyStatusAfterPreviewFailure("SOLVING", false, [ux.assemblySupportPresentation(broken)]), "BROKEN");
assert.equal(ux.assemblyStatusAfterPreviewFailure("SOLVING", true, [ux.assemblySupportPresentation(connected)]), "NOT_UPDATED");
console.log("Assembly constraint UX tests passed.");
