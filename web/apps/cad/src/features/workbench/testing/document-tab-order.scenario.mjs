import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../document-tab-order.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const tabs = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);

const docs = [
  { id: "part-a", name: "A", type: "PART" },
  { id: "product", name: "Product", type: "PRODUCT" },
  { id: "part-b", name: "B", type: "PART" },
];
assert.deepEqual(tabs.stableDocumentTabOrder(["part-b", "part-a"], docs, "product"), ["product", "part-b", "part-a"],
  "the current Product host is fixed first without globally sorting document types");
assert.deepEqual(tabs.stableDocumentTabOrder(["product", "part-b", "part-a"],
  docs.map((document) => document.id === "part-b" ? { ...document, name: "B2" } : document), "product"),
  ["product", "part-b", "part-a"], "summary refresh must preserve positions");
assert.deepEqual(tabs.stableDocumentTabOrder(["product", "part-b", "closed"], [docs[1], docs[2]], "product"),
  ["product", "part-b"], "a closed tab must not return from stale saved order");
assert.deepEqual(tabs.moveDocumentTab(["product", "part-a", "part-b"], "part-b", -1, "product"),
  ["product", "part-b", "part-a"]);
assert.deepEqual(tabs.moveDocumentTab(["product", "part-a", "part-b"], "part-a", -1, "product"),
  ["product", "part-a", "part-b"], "other tabs cannot cross the fixed host");
assert.deepEqual(tabs.moveDocumentTab(["product", "part-a", "part-b"], "product", 1, "product"),
  ["product", "part-a", "part-b"], "the active Product host itself cannot be reordered");
console.log("Window-local stable document tab ordering passed");
