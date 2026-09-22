import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../sketch-session-policy.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const { canDiscardNewSketch, defaultSolidReversed } = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);

const session = { documentId: "part", sketchId: "new-sketch", edited: false };
const view = { document: { id: "part" }, part: { features: [
  { id: "new-sketch", sketch: { entities: [], constraints: [], externalGeometry: [] } },
  { id: "existing-empty", sketch: { entities: [], constraints: [] } },
] } };
assert.equal(canDiscardNewSketch(session, view), true);
assert.equal(canDiscardNewSketch({ ...session, edited: true }, view), false, "editing then undoing is not an untouched session");
assert.equal(canDiscardNewSketch(session, undefined), false);
assert.equal(canDiscardNewSketch(session, { ...view, document: { id: "another-part" } }), false);
assert.equal(canDiscardNewSketch({ ...session, sketchId: "missing" }, view), false);
assert.equal(canDiscardNewSketch(session, { ...view, part: { features: [view.part.features[1]] } }), false,
  "an existing empty sketch must not be substituted for the newly created one");
const dependent = structuredClone(view);
dependent.part.features.push({ id: "pad", profile: "new-sketch" });
assert.equal(canDiscardNewSketch(session, dependent), false, "dependent features must not be deleted by cleanup");
for (const collection of ["entities", "constraints", "externalGeometry"]) {
  const populated = structuredClone(view);
  populated.part.features[0].sketch[collection].push({ id: "external-edit" });
  assert.equal(canDiscardNewSketch(session, populated), false, `${collection} must prevent automatic deletion`);
}
for (const operation of ["NEW_BODY", "ADD", "INTERSECT"]) {
  assert.equal(defaultSolidReversed("LINEAR_EXTRUDE", operation), false);
}
assert.equal(defaultSolidReversed("LINEAR_EXTRUDE", "REMOVE"), true);
assert.equal(defaultSolidReversed("REVOLVE", "REMOVE"), false, "revolution has its own axis convention");
console.log("Sketch session and solid direction policies passed.");
