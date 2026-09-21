import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const { assemblyConstraintEntry, changeAngleRelation } = await server.ssrLoadModule("/src/cad/assembly/assembly-angle.ts");
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
