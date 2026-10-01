import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";

// Included by the existing assembly scenario discovery, so catalog-only edits
// also check mapping/semantic locks without running a new full test platform.
execFileSync("python3", [resolve("../../../tests/assembly-contract/runner.py"), "validate"], { stdio: "pipe" });

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../assembly-angle.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const actual = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);
const catalog = JSON.parse(await readFile(resolve("../../../tests/assembly-contract/catalog.json"), "utf8"));
const selection = process.env.OCCCCAD_ASSEMBLY_CONTRACT_CASES
  ? JSON.parse(process.env.OCCCCAD_ASSEMBLY_CONTRACT_CASES) : undefined;
const cases = catalog.cases.filter(c => c.adapter === "web-catalog" && (selection ? selection.includes(c.caseId) : c.baseline));
assert.ok(cases.length, "no actual catalog web cases selected");
for (const c of cases) {
  if (c.selector === "entry") {
    assert.deepEqual(actual.assemblyConstraintEntry(c.input.tool), c.expected, c.caseId);
  } else if (c.selector === "clear-axis") {
    const directed = { angleRelation: "DIRECTED", angleAxis: { kind: "AXIS", instanceId: "reference" },
      reverseAngleAxis: true, angleReferenceDirection: [0, 0, -1] };
    for (const relation of c.input.relations) {
      const changed = actual.changeAngleRelation(directed, relation);
      assert.equal(changed.angleRelation, relation);
      assert.equal(changed.angleAxis === undefined, c.expected.axisAbsent, c.caseId);
      assert.equal(changed.angleReferenceDirection, undefined);
      assert.equal(changed.reverseAngleAxis, false);
      assert.equal(actual.changeAngleRelation(changed, "DIRECTED").angleAxis, undefined);
    }
  } else if (c.selector === "offset-normal") {
    // Read the actual Select options exposed by the existing editor. This is a
    // source/UI contract witness, not a browser test or numeric sign oracle.
    const editor = await readFile(resolve(c.input.source), "utf8");
    const field = editor.match(/name="distanceRelation"[\s\S]*?options=\{\[([\s\S]*?)\]\}/);
    assert.ok(field, "actual distance editor options not found");
    const labels = [...field[1].matchAll(/label:"([^"]+)"/g)].map(m => m[1]);
    console.log(`CONTRACT_OBSERVED ${c.caseId} ${JSON.stringify(labels)}`);
    assert.ok(labels.some(label => label.includes(c.expected.positiveNormalLabel)),
      `${c.caseId}: expected ${c.expected.positiveNormalLabel}, observed ${JSON.stringify(labels)}`);
  } else {
    assert.fail(`unknown test adapter selector ${c.selector}`);
  }
  console.log(`CONTRACT_PASS ${c.caseId}`);
}
