import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../../package.json", import.meta.url));
const ts = require("typescript");
const source = await readFile(new URL("../product-edit-context.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const context = await import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);

const tree = { id: "root", kind: "PRODUCT", name: "Root", children: [
  { id: "follow-product", kind: "INSTANCE", name: "Live", documentId: "product-live", referenceMode: "FOLLOW_HEAD", children: [
    { id: "follow-part", kind: "INSTANCE", name: "Live Part", documentId: "part-live", referenceMode: "FOLLOW_HEAD" },
    { id: "pinned-part", kind: "INSTANCE", name: "Pinned Part", documentId: "part-pinned", referenceMode: "PINNED" },
  ] },
  { id: "pinned-product", kind: "INSTANCE", name: "Frozen", documentId: "product-pinned", referenceMode: "PINNED", children: [
    { id: "hidden-live-part", kind: "INSTANCE", name: "Frozen child", documentId: "part-below-pin", referenceMode: "FOLLOW_HEAD" },
  ] },
] };

assert.deepEqual(context.followedDocumentIDs(tree).sort(), ["part-live", "product-live"]);
console.log("Product edit context tests passed.");
