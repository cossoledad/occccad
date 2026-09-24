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
const staleTree = { id: "root", kind: "PRODUCT", name: "Root", documentId: "root-product", children: [
  { id: "sub", kind: "INSTANCE", name: "Sub", documentId: "sub-product", documentType: "PRODUCT", children: [
    { id: "part", kind: "INSTANCE", name: "Part", documentId: "part-live", documentType: "PART", diagnostic: "NOT_UPDATED: newer revision" },
  ] },
  { id: "pinned", kind: "INSTANCE", name: "Pinned", documentId: "frozen-product", documentType: "PRODUCT", referenceMode: "PINNED", children: [
    { id: "frozen-part", kind: "INSTANCE", name: "Frozen Part", documentId: "frozen-part", documentType: "PART", diagnostic: "NOT_UPDATED: ignored below pinned" },
  ] },
] };
assert.deepEqual(context.staleProductDocumentIDs(staleTree), ["sub-product", "root-product"]);
console.log("Product edit context tests passed.");

// A stale Product edge used to append the root before visiting its own stale
// descendants. Shared Products must also be ordered once before every owner.
const bothStale = structuredClone(staleTree);
bothStale.children[0].diagnostic = "NOT_UPDATED: newer Product";
assert.deepEqual(context.staleProductDocumentIDs(bothStale), ["sub-product", "root-product"]);
const shared = {kind:"PRODUCT",documentId:"root",children:[
  {kind:"INSTANCE",documentType:"PRODUCT",documentId:"shared",diagnostic:"NOT_UPDATED: newer",children:[]},
  {kind:"INSTANCE",documentType:"PRODUCT",documentId:"owner",children:[
    {kind:"INSTANCE",documentType:"PRODUCT",documentId:"shared",children:[
      {kind:"INSTANCE",documentType:"PART",documentId:"leaf",diagnostic:"NOT_UPDATED: newer"}
    ]}
  ]}
]};
assert.deepEqual(context.staleProductDocumentIDs(shared),["shared","owner","root"]);

// Accept plans are fetched at execution time; the parent's digest is generated
// only after the child commit. A new child definition can reveal another wave.

const calls=[];
const dirtyPart={kind:"INSTANCE",documentType:"PART",documentId:"part",diagnostic:"NOT_UPDATED: newer"};
const product=(id,children=[],versionId="v1")=>({document:{id,versionId},structureTree:{kind:"PRODUCT",documentId:id,children}});
// The root's frozen sub-product has no visible child, while its latest Head
// introduced a deeper Product whose Part is already stale.
const initial=product("root",[{kind:"INSTANCE",documentType:"PRODUCT",documentId:"sub",diagnostic:"NOT_UPDATED: newer",children:[]}]);
const views=new Map([
 ["root",initial],
 ["sub",product("sub",[{kind:"INSTANCE",documentType:"PRODUCT",documentId:"new-child",children:[dirtyPart]}])],
 ["new-child",product("new-child",[dirtyPart])],
]);
const updated=await context.followProductUpdates(initial,{
 async getDocument(id){return views.get(id)},
 async getProductUpdatePlan(id){calls.push(`plan:${id}`);return {hasUpdates:true,canAccept:true,digest:`digest:${id}`,entries:[]}},
 async acceptProductUpdatePlan(id,digest){assert.equal(digest,`digest:${id}`);calls.push(`accept:${id}`);const clean=product(id,[],"v2");views.set(id,clean);return clean},
});
assert.deepEqual(calls,["plan:new-child","accept:new-child","plan:sub","accept:sub","plan:root","accept:root"]);
assert.equal(updated.find(v=>v.document.id==="root").document.versionId,"v2");
await assert.rejects(()=>context.followProductUpdates(product("blocked",[dirtyPart]),{
 async getProductUpdatePlan(){return {hasUpdates:true,canAccept:false,entries:[{diagnostic:"actual upstream failure"}]}},
 async acceptProductUpdatePlan(){assert.fail("accepted blocked plan")},async getDocument(){assert.fail("read after blocked plan")},
}),/actual upstream failure/);
