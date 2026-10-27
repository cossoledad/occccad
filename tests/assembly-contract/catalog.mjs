// Production owns semantics; evidence owns executable mappings only.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
export const ROOT = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
export const DATA = path.join(ROOT, "tests/test.data/assembly-contract");
export const read = (p) => JSON.parse(fs.readFileSync(p, "utf8"));
export function composeCatalog(production, evidence) {
  production = structuredClone(production);
  const used = new Set();
  const cases = evidence.cases.map((declared) => {
    const { bindingId, ...c } = declared;
    used.add(bindingId);
    const binding = evidence.bindings[bindingId];
    if (!binding) throw Error(`${c.caseId}: missing binding ${bindingId}`);
    if (Object.keys(c).some((k) => k in binding))
      throw Error(`${c.caseId}: duplicated binding fields`);
    return { ...binding, ...c };
  });
  if (Object.keys(evidence.bindings).some((k) => !used.has(k)))
    throw Error("unused test binding");
  if (
    JSON.stringify(
      production.capabilities.map((c) => c.capabilityId).sort(),
    ) !== JSON.stringify(Object.keys(evidence.capabilityEvidence).sort())
  )
    throw Error("production/test capability references differ");
  for (const c of production.capabilities) {
    const e = evidence.capabilityEvidence[c.capabilityId];
    if (Object.keys(c).some((k) => k in e))
      throw Error("test evidence overwrites production semantics");
    Object.assign(c, e);
    c.caseIds = cases
      .filter((t) => t.capabilityIds.includes(c.capabilityId))
      .map((t) => t.caseId);
  }
  return {
    ...production,
    layers: evidence.layers,
    implementationProfiles: evidence.implementationProfiles,
    cases,
  };
}
export function loadCatalog() {
  const index = read(path.join(DATA, "catalog.json"));
  return composeCatalog(
    read(path.resolve(DATA, index.production)),
    read(path.join(DATA, index.evidence)),
  );
}
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const { values: v } = parseArgs({
    options: {
      capability: { type: "string" },
      family: { type: "string" },
      case: { type: "string" },
      expanded: { type: "boolean" },
    },
  });
  const c = loadCatalog();
  let out = v.case
    ? c.cases.filter((t) => t.caseId === v.case)
    : v.capability || v.family
      ? c.capabilities.filter(
          (t) =>
            (!v.capability || t.capabilityId === v.capability) &&
            (!v.family || t.family === v.family),
        )
      : v.expanded
        ? c
        : {
            contractVersion: c.contractVersion,
            families: c.families,
            capabilities: c.capabilities.length,
            cases: c.cases.length,
            query:
              "--capability ID / --family NAME / --case ID; --expanded for adapters",
          };
  if (Array.isArray(out) && !out.length)
    throw Error("selection matched no declaration");
  console.log(JSON.stringify(out, null, 2));
}
