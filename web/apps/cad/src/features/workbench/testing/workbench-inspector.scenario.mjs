import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const modules = new Map();
// Load the actual component and its local helpers without a browser or a mock UI.
function load(file) {
  if (modules.has(file)) return modules.get(file).exports;
  const module = { exports: {} };
  modules.set(file, module);
  const js = ts.transpileModule(readFileSync(file, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX, esModuleInterop: true },
    fileName: file,
  }).outputText;
  const localRequire = createRequire(file);
  new Function("require", "module", "exports", js)((name) => {
    if (!name.startsWith(".")) return localRequire(name);
    const path = resolve(dirname(file), name);
    const target = [path, `${path}.ts`, `${path}.tsx`].find(existsSync);
    assert.ok(target, `cannot resolve ${name} from ${file}`);
    return load(target);
  }, module, module.exports);
  return module.exports;
}
const { Properties } = load(fileURLToPath(new URL("../workbench-inspector.tsx", import.meta.url)));
const { CAD_WORKBENCHES } = load(fileURLToPath(new URL("../../../cad/workbench/cad-workbench.ts", import.meta.url)));
const view = { document: { id: "part", type: "PART", name: "New Part", versionId: "revision", permission: "OWNER" }, part: { features: [] } };
for (const primitives of [null, undefined, []]) {
  const diagnostics = {
    artifacts: [{ visualization: { schemaVersion: 1, referenceGeometry: { datumPlanes: [], axisSystems: [] }, primitives } }],
    aggregate: { artifactCount: 1, solidCount: 0, vertexCount: 0, glbBytes: 0, brepBytes: 0 }, worker: { available: false },
  };
  const html = renderToStaticMarkup(React.createElement(Properties, {
    view, selection: null, diagnostics, workbench: Object.keys(CAD_WORKBENCHES)[0], activeTool: "select", navigationProfile: "cad",
  }));
  assert.ok(html.includes("New Part"));
  assert.ok(html.includes("Non-solid Geometry"));
  if (primitives == null) assert.ok(html.includes("按需从 GLB 加载"), "unloaded data must not be reported as zero primitives");
}
console.log("Properties renders lightweight Artifact descriptors with null, omitted and empty primitives");
