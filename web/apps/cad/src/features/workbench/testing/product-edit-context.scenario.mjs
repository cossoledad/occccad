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

const view = { document: { id: "root", versionId: "v1" },
  followedDocumentIds: ["part-live", "product-live"], followedProductIds: ["child", "product-live"],
  product: { instances: [
    { id: "live", documentId: "product-live", referenceMode: "FOLLOW_HEAD", headChanged: true },
    { id: "frozen", documentId: "product-pinned", referenceMode: "PINNED", headChanged: true },
  ] },
  structureTree: { kind: "PRODUCT", children: [] },
};
assert.deepEqual(context.followedDocumentIDs(view), ["part-live", "product-live"]);
assert.deepEqual(context.staleProductDocumentIDs(view), ["root"]);
assert.deepEqual(context.followedDocumentIDs({ ...view, followedDocumentIds: undefined }), ["product-live"]);

const dirty = new Set(["child", "product-live", "root"]);
const calls = [];
const updated = await context.followProductUpdates(view, {
  async getDocument(id) { assert.equal(id, "root"); return { ...view, document: { id, versionId: "v2" } }; },
  async getProductUpdatePlan(id) {
    calls.push(`plan:${id}`);
    return { hasUpdates: dirty.has(id), canAccept: true, digest: `digest:${id}`, entries: [] };
  },
  async acceptProductUpdatePlan(id, digest) {
    assert.equal(digest, `digest:${id}`);
    calls.push(`accept:${id}`);
    dirty.delete(id);
    return { document: { id, versionId: "v2" } };
  },
});
assert.deepEqual(calls.slice(0, 6), ["plan:child", "accept:child", "plan:product-live",
  "accept:product-live", "plan:root", "accept:root"], "Product updates follow domain dependency postorder");
assert.equal(updated.find((item) => item.document.id === "root").document.versionId, "v2");
assert(!calls.some((item) => item.includes("product-pinned")), "pinned Product remains frozen");
await assert.rejects(() => context.followProductUpdates({ ...view, followedProductIds: [] }, {
  async getProductUpdatePlan() { return { hasUpdates: true, canAccept: false,
    entries: [{ kind: "OCCURRENCE_REFERENCE", diagnostic: "actual upstream failure" }] }; },
  async acceptProductUpdatePlan() { assert.fail("accepted blocked plan"); },
  async getDocument() { assert.fail("read after blocked plan"); },
}), /actual upstream failure/);
console.log("Domain-driven Product update and pinned reference tests passed");

assert.deepEqual(await context.followProductUpdates({...view,followedProductIds:[]},{
 async getProductUpdatePlan(){return {hasUpdates:true,canAccept:true,parameterUpdates:[{documentId:"fan",needsUpdate:true}],entries:[]};},
 async acceptProductUpdatePlan(){assert.fail("background following must not commit shared parameter changes");},
 async getDocument(){assert.fail("no version was accepted");},
}),[]);
