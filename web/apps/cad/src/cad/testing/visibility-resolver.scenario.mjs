import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });

try {
  const { VisibilityResolver, visibilityResolverForView } = await server.ssrLoadModule("/src/cad/interaction/visibility-resolver.ts");
  const referenceTree={kind:"PART",documentId:"part",entityId:"part",children:[{kind:"ORIGIN",documentId:"part",entityId:"origin",localVisible:true,children:[
    {kind:"AXIS_SYSTEM",documentId:"part",entityId:"system",children:[{kind:"AXIS",documentId:"part",entityId:"system",axis:"X",localVisible:false},{kind:"AXIS",documentId:"part",entityId:"system",axis:"Y",localVisible:true}]},
    {kind:"PLANE",documentId:"part",entityId:"xy",localVisible:true}]}]};
  let references=new VisibilityResolver(referenceTree);
  const address={documentId:"part",occurrencePath:"",kind:"AXIS",entityId:"system"};
  assert.equal(references.resolve({...address,axis:"X"}).effectiveVisible,false);
  assert.equal(references.resolve({...address,axis:"Y"}).effectiveVisible,true,"axis child states are distinct");
  referenceTree.children[0].localVisible=false;references=new VisibilityResolver(referenceTree);
  assert.equal(references.resolve({...address,axis:"Y"},{id:"sketch",occurrencePath:""}).effectiveVisible,false,"projection/sketch edit cannot reveal domain-hidden Origin");
  assert.equal(references.resolve({...address,kind:"PLANE",entityId:"xy"}).effectiveVisible,false);
  const THREE=await server.ssrLoadModule("three");
  const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const helpers=new THREE.Group(),axisObject=new THREE.Object3D();
  axisObject.userData={kind:"axis",id:"system:X",entityId:"system",axis:"X",documentId:"part"};helpers.add(axisObject);
  const engine=Object.assign(Object.create(CadViewportEngine.prototype),{helpers,content:new THREE.Group(),referenceVisibility:{planes:true,axes:true,coordinateSystems:true},treeVisibilityOverrides:{},visibilityResolver:references,
    tools:{activeToolID:"sketch.project"},sketchPlane:{},sketchView:()=>({document:{id:"part"}}),editingSketchScope:()=>undefined,refreshInteractionHighlights:()=>{}});
  engine.applyTreeVisibility();assert.equal(axisObject.visible,false,"projection honors persistent hidden Origin");
  referenceTree.children[0].localVisible=true;referenceTree.children[0].children[0].children[0].localVisible=true;engine.visibilityResolver=new VisibilityResolver(referenceTree);
  engine.referenceVisibility.coordinateSystems=false;engine.applyTreeVisibility();assert.equal(axisObject.visible,true,"projection exposes available standard axes without changing preferences");
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
  const editingPart = { document: { id: "part", type: "PART" }, structureTree: {
    kind: "PART", documentId: "part", entityId: "part", children: [
      { kind: "BODY", documentId: "part", entityId: "body-one", localVisible: true },
    ] } };
  const host = { structureTree: frozenRoot };
  const scoped = visibilityResolverForView(host, { view: editingPart, occurrencePath: "first", liveDefinitionProjection: true });
  assert.equal(scoped.resolve(body("first")).effectiveVisible, true, "current definition is projected only in the active edit occurrence");
  assert.equal(scoped.resolve(body("first")).definitionVisible, true);
  assert.equal(scoped.resolve(body("first")).mode, "INHERIT", "editing does not apply the host's object override to definition controls");
  assert.equal(new VisibilityResolver(frozenRoot).resolve(body("first")).effectiveVisible, false, "leaving editing restores persisted host projection");
  editingPart.structureTree.children[0].localVisible = false;
  const hiddenDefinition = visibilityResolverForView(host, { view: editingPart, occurrencePath: "first", liveDefinitionProjection: true });
  assert.equal(hiddenDefinition.resolve(body("first")).effectiveVisible, false);
  assert.equal(hiddenDefinition.resolve(body("second")).effectiveVisible, true, "another occurrence keeps its accepted definition");
  assert.equal(visibilityResolverForView(host, { view: editingPart, occurrencePath: "first", liveDefinitionProjection: false }).resolve(body("first")).effectiveVisible, false);
  console.log("Semantic visibility resolver scenarios passed.");
} finally {
  await server.close();
}
