import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { assemblyConstraintEntry, changeAngleRelation, invalidateAngleReferenceDirection } = await server.ssrLoadModule("/src/cad/assembly/assembly-angle.ts");
  const {angleAxisCandidateError}=await server.ssrLoadModule("/src/features/workbench/assembly-angle-parameters.tsx");
  assert.equal(angleAxisCandidateError({instanceId:"independent-third",kind:"AXIS"},{instanceId:"second",kind:"AXIS"}),undefined,"versioned directed angle accepts independently owned reference axis");
  assert.ok(angleAxisCandidateError({instanceId:"third",kind:"BODY"},{instanceId:"second",kind:"AXIS"}));
  const axis={instanceId:"third",kind:"AXIS",geometryId:"stable-axis"};
  assert.deepEqual(invalidateAngleReferenceDirection({angleRelation:"DIRECTED",angleAxis:axis,angleReferenceDirection:[1,0,0]}),{angleRelation:"DIRECTED",angleAxis:axis,angleReferenceDirection:undefined},"support replacement discards computed basis but preserves independently owned axis");
  assert.deepEqual(assemblyConstraintEntry("angle"), { kind: "angle", angleRelation: "FREE" });
  assert.deepEqual(assemblyConstraintEntry("parallel"), { kind: "angle", angleRelation: "PARALLEL" });
  assert.deepEqual(assemblyConstraintEntry("perpendicular"), { kind: "angle", angleRelation: "PERPENDICULAR" });
  const selected = { angleRelation: "DIRECTED", angleAxis: { instanceId: "b", kind: "AXIS" }, reverseAngleAxis: true, angleReferenceDirection: [0, 0, -1] };
  for (const relation of ["FREE", "PARALLEL", "PERPENDICULAR"]) {
    const changed = changeAngleRelation(selected, relation);
    assert.equal(changed.angleRelation, relation);
    assert.equal(changed.angleAxis, undefined);
    assert.equal(changed.angleReferenceDirection, undefined);
    assert.equal(changed.reverseAngleAxis, false);
    assert.equal(changeAngleRelation(changed, "DIRECTED").angleAxis, undefined, "returning to directed mode requires a new stable axis");
  }
  assert.deepEqual(changeAngleRelation(selected, "DIRECTED"), selected);
} finally { await server.close(); }
