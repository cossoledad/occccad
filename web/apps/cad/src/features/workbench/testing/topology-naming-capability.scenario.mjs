import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
const ts = createRequire(new URL("../../../../package.json", import.meta.url))("typescript");
const source = await readFile(new URL("../topology-naming-capability.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
const {selectionNamingIssue, topologyNamingIssue} = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);
const unavailable = {status:"UNAVAILABLE",canBind:false,diagnosticCode:"TOPOLOGY_NAMING_UNAVAILABLE",diagnostic:"missing naming"};
const imported = {artifact:{geometryKey:"import",naming:unavailable}};
const product = {artifacts:{import:imported.artifact,native:{geometryKey:"native",naming:{status:"READY",canBind:true}}}};
for(const kind of ["face","edge","vertex"]) {
 assert.equal(selectionNamingIssue({kind,geometryKey:"import"},product),unavailable);
 assert.equal(selectionNamingIssue({kind,geometryKey:"native"},product),undefined);
}
for(const kind of ["plane","axis","axis-system","instance","body","sketch"]) {
 assert.equal(selectionNamingIssue({kind,geometryKey:"import"},imported),undefined,`${kind} must not depend on sub-topology naming`);
}
for(const status of ["FAILED","CORRUPT","INCOMPATIBLE"]) {
 const naming={status,canBind:false,diagnostic:status};
 assert.equal(topologyNamingIssue("bad",{artifact:{geometryKey:"bad",naming}}),naming);
}
assert.equal(topologyNamingIssue("import",{artifact:{geometryKey:"native"}},product),unavailable,"resolve nested occurrence's artifact");
assert.equal(selectionNamingIssue(null,product),undefined);
assert.equal(topologyNamingIssue("unknown",product),undefined,"unknown capability must still be checked by the server");
console.log("Naming capability isolation passed.");
