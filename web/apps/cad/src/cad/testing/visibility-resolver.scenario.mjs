import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });

try {
  const { VisibilityResolver } = await server.ssrLoadModule("/src/cad/interaction/visibility-resolver.ts");
  const occurrence = (id, instanceMode) => {
    const instancePath = {rootDocumentId:"product",canonical:id,display:id,segments:[{instanceId:id}]};
    const body = {kind:"BODY",entityId:"body-one",subject:{documentId:"part",entityKind:"BODY",entityId:"body-one"},
      documentId:"part",instancePath,localVisible:false,visibilityMode:id === "first" ? "SHOW" : undefined,
      children:[{kind:"PAD",entityId:"pad",documentId:"part",instancePath,children:[
        {kind:"SKETCH",entityId:"sketch",subject:{documentId:"part",entityKind:"SKETCH",entityId:"sketch"},
          documentId:"part",instancePath,bodyId:"body-one",localVisible:true,children:[
            {kind:"SKETCH_ENTITY",entityId:"line",subject:{documentId:"part",entityKind:"SKETCH_ENTITY",entityId:"line"},
              documentId:"part",instancePath,bodyId:"body-one",ownerEntityId:"sketch",localVisible:false},
          ]},
      ]}]};
    return {kind:"INSTANCE",entityId:id,subject:{documentId:"product",entityKind:"INSTANCE",entityId:id},
      documentId:"part",ownerDocumentId:"product",instancePath,visibilityMode:instanceMode,children:[
        {kind:"PART",documentId:"part",entityId:"part",subject:{documentId:"part",entityKind:"PART",entityId:"part"},
          instancePath,children:[body]},
      ]};
  };
  const root = {kind:"PRODUCT",documentId:"product",children:[occurrence("first","HIDE"),occurrence("second",undefined)]};
  const resolver = new VisibilityResolver(root);
  const body = id => ({documentId:"part",occurrencePath:id,kind:"BODY",entityId:"body-one"});
  const line = id => ({documentId:"part",occurrencePath:id,kind:"SKETCH_ENTITY",entityId:"line",ownerEntityId:"sketch",bodyId:"body-one"});
  assert.equal(resolver.resolve(body("first")).localVisible,true,"occurrence SHOW replaces inherited Body definition");
  assert.equal(resolver.resolve(body("first")).effectiveVisible,false,"hidden occurrence blocks its Body override");
  assert.equal(resolver.resolve(body("second")).effectiveVisible,false,"other occurrence inherits hidden Body definition");
  assert.equal(resolver.resolve(line("first")).effectiveVisible,false,"element stays hidden through its semantic Sketch and Body owners");
  const editFirst = {id:"sketch",occurrencePath:"first"};
  assert.equal(resolver.resolve(body("first"),editFirst).effectiveVisible,true,"editing reveals the selected occurrence's necessary ancestors");
  assert.equal(resolver.resolve(body("second"),editFirst).effectiveVisible,false,"editing one occurrence does not reveal another");
  assert.equal(resolver.resolve(line("first"),editFirst).effectiveVisible,false,"editing reveal preserves the element's own hidden setting");
  root.children[0].visibilityMode = undefined;
  const restored = new VisibilityResolver(root);
  assert.equal(restored.resolve(body("first")).effectiveVisible,true,"restoring parent visibility preserves the Body override");
  assert.equal(restored.resolve(line("first")).effectiveVisible,false,"restoring parent keeps independently hidden entity");
  const frozenRoot={kind:"PRODUCT",documentId:"product",children:[occurrence("first",undefined),occurrence("second",undefined)]};
  frozenRoot.children[0].children[0].snapshot={revisionId:"pinned-revision"};
  frozenRoot.children[0].children[0].children[0].visibilityMode=undefined;
  frozenRoot.children[1].children[0].snapshot={revisionId:"follow-revision"};
  frozenRoot.children[1].children[0].children[0].localVisible=true;
  const frozen=new VisibilityResolver(frozenRoot);
  assert.equal(frozen.resolve(body("first")).effectiveVisible,false,"pinned occurrence keeps its resolved revision's hidden definition");
  assert.equal(frozen.resolve(body("second")).effectiveVisible,true,"updated occurrence reads its own resolved revision's visible definition");
  console.log("Semantic visibility resolver scenarios passed.");
} finally {
  await server.close();
}
