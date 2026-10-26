import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const workbench = await readFile(new URL("../workbench.tsx", import.meta.url), "utf8");
const api = await readFile(new URL("../../../api.ts", import.meta.url), "utf8");
const associations = await readFile(new URL("../design-association-dialog.tsx", import.meta.url), "utf8");
const types = await readFile(new URL("../../../types.ts", import.meta.url), "utf8");

assert.match(types, /type ProductDesignSession/);
assert.match(types, /type ContextCatalog/);
assert.match(types, /type ContextInput/);
assert.match(types, /type ContextBinding/);
assert.match(api, /\/design-session\?/);
assert.match(api, /\/context-catalog\?/);
assert.match(api, /\/context-bindings/);
assert.match(workbench, /<DesignAssociationDialog/);
assert.match(associations, /引用设计信息与关联管理/);
assert.match(associations, /getContextCatalog/);
assert.match(associations, /useFeaturePreview/);
assert.match(associations, /expectedRootRevisionId/);
assert.match(associations, /owningInstancePath/);
assert.match(associations, /sourceInstancePath/);
assert.match(associations, /onSelectionSession/);
assert.doesNotMatch(workbench, /Product Context Bindings/);
assert.doesNotMatch(associations, /label="PublicationId"/);
assert.doesNotMatch(workbench, /label="PublicationId"/);
assert.doesNotMatch(workbench, /Parameter PublicationId/);

console.log("Product context catalog UX contract tests passed.");
