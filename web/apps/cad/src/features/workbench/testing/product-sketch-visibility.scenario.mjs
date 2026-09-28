import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
  const THREE = await server.ssrLoadModule("three");
  const {CadViewportEngine} = await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const {SelectionIndex} = await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
  const {visibilityResolverForView} = await server.ssrLoadModule("/src/cad/interaction/visibility-resolver.ts");
  const {encodeMeshGLB,decodeMeshGLB} = await server.ssrLoadModule("/src/cad/visual/mesh-glb.ts");
  const artifact = bodyId => {
    const mesh={vertices:[],triangles:[],faceIds:[],edges:[],topologyVertices:[]};
    const visualization={schemaVersion:1,referenceGeometry:{datumPlanes:[],axisSystems:[]},primitives:[
      {id:"point",featureId:"shared-sketch",kind:"POINTS",semantic:"SKETCH_POINT",entityType:"POINT",
        role:"PROFILE",status:"SOLVED",positions:[[2,3,0]],selectable:true},
    ]};
    return {geometryKey:`geometry-${bodyId}`,...decodeMeshGLB(encodeMeshGLB(mesh,visualization))};
  };
  const artifacts = {"geometry-a":artifact("a"),"geometry-b":artifact("b")};
  const instancePath = id => ({rootDocumentId:"product",canonical:id,segments:[{instanceId:id,resolvedVersionId:"part-revision"}]});
  const occurrence = (id, x, sketchMode) => {
    const path=instancePath(id);
    const sketch={id:`document:product/instance:${id}/reference/body:a/sketch:shared-sketch`,kind:"SKETCH",entityId:"shared-sketch",documentId:"part",versionId:"part-revision",
      instancePath:path,bodyId:"a",localVisible:false,visibilityMode:sketchMode,
      children:[{kind:"SKETCH_ENTITY",entityId:"point",ownerEntityId:"shared-sketch",bodyId:"a",
        documentId:"part",instancePath:path,localVisible:true}]};
    const bodyA={kind:"BODY",entityId:"a",documentId:"part",instancePath:path,localVisible:true,
      id:`document:product/instance:${id}/reference/body:a`,children:[sketch]};
    const bodyB={kind:"BODY",entityId:"b",documentId:"part",instancePath:path,localVisible:true,
      id:`document:product/instance:${id}/reference/body:b`,children:[]};
    return {instance:{id,documentId:"part",versionId:"part-revision",translation:[x,0,0]},
      node:{kind:"INSTANCE",entityId:id,documentId:"part",ownerDocumentId:"product",instancePath:path,
        children:[{kind:"PART",entityId:"part",documentId:"part",instancePath:path,children:[bodyA,bodyB]}]},
      resolved:["a","b"].map(bodyId=>({id:`Assembly/${id}/body:${bodyId}`,bodyId,bodyVisible:true,
        ownedSketchIds:bodyId==="a"?["shared-sketch"]:[],documentId:"part",geometryKey:`geometry-${bodyId}`,
        translation:[x,0,0],occurrencePath:id,instancePath:path,
        bodyTreeNodeId:`document:product/instance:${id}/reference/body:${bodyId}`}))};
  };
  const first=occurrence("first",0,"SHOW"), second=occurrence("second",20,"HIDE");
  const view={document:{id:"product",type:"PRODUCT",name:"Assembly"},
    product:{instances:[first.instance,second.instance]},artifacts,
    resolvedInstances:[...first.resolved,...second.resolved],
    structureTree:{kind:"PRODUCT",documentId:"product",children:[first.node,second.node]}};
  const engine=Object.create(CadViewportEngine.prototype);
  Object.assign(engine,{view,content:new THREE.Group(),helpers:new THREE.Group(),selectable:new Map(),
    instanceGroups:new Map(),solidBindings:new Map(),selectionIndex:new SelectionIndex(),treeVisibilityOverrides:{},
    referenceVisibility:{},materials:{point:()=>new THREE.PointsMaterial({size:1})},
    addAssemblyConstraintMarkers(){},refreshInteractionHighlights(){}});
  engine.visibilityResolver=visibilityResolverForView(view);
  engine.renderProduct(view);
  engine.applyTreeVisibility();
  engine.content.updateMatrixWorld(true);
  const points=[...engine.selectable.entries()].filter(([key])=>key.startsWith("visual:"));
  assert.equal(points.length,2,"each occurrence draws the owning Body's Sketch once, even if another Body's GLB carries it as input");
  const visible=object=>{for(let current=object;current;current=current.parent)if(!current.visible)return false;return true;};
  const point=id=>points.find(([key])=>key.startsWith(`visual:${id}:`))?.[1];
  assert.ok(point("first"));assert.ok(point("second"));
  assert.equal(visible(point("first")),true,"SHOW reveals the first occurrence");
  assert.equal(visible(point("second")),false,"HIDE affects only the second occurrence");
  const ray=x=>new THREE.Raycaster(new THREE.Vector3(x,3,10),new THREE.Vector3(0,0,-1));
  assert.equal(engine.selectionIndex.pick(ray(2))?.entityId,"point","visible SketchEntity can be picked in Product");
  assert.equal(engine.selectionIndex.pick(ray(22)),null,"hidden occurrence SketchEntity cannot be picked");
  const sketchSelection={kind:"sketch",id:"shared-sketch",entityId:"shared-sketch",documentId:"part",bodyId:"a",
    versionId:"part-revision",instancePath:instancePath("first"),occurrencePath:"first"};
  assert.equal(engine.selectionIndex.objectsFor(sketchSelection).length,1,"tree Sketch selection locates its Product render group");
  engine.editContext={occurrencePath:"first"};engine.activeSketchID="shared-sketch";
  engine.applyTreeVisibility();
  assert.equal(visible(point("first")),false,"active edit overlay replaces the ordinary Product Sketch primitive");
  engine.editContext=undefined;engine.activeSketchID=undefined;engine.applyTreeVisibility();
  assert.equal(visible(point("first")),true,"ordinary display returns after exiting Sketch edit");
  first.node.children[0].children[0].localVisible=false;
  engine.visibilityResolver=visibilityResolverForView(view);engine.applyTreeVisibility();
  assert.equal(visible(point("first")),false,"hidden owning Body blocks an explicitly shown Sketch");
  first.node.children[0].children[0].localVisible=true;
  first.node.visibilityMode="HIDE";
  engine.visibilityResolver=visibilityResolverForView(view);engine.applyTreeVisibility();
  assert.equal(visible(point("first")),false,"Sketch SHOW cannot break hidden occurrence ancestry");
  first.node.visibilityMode=undefined;
  first.node.children[0].children[0].children[0].visibilityMode="HIDE";
  engine.visibilityResolver=visibilityResolverForView(view);engine.applyTreeVisibility();
  assert.equal(visible(point("first")),false,"display metadata updates already loaded objects without rebuilding the scene");
  first.node.children[0].children[0].children[0].visibilityMode="SHOW";
  first.node.children[0].children[0].children[0].children[0].localVisible=false;
  engine.visibilityResolver=visibilityResolverForView(view);engine.applyTreeVisibility();
  assert.equal(visible(point("first")),false,"hidden SketchEntity stays hidden when its Sketch is shown");
  assert.equal(engine.selectionIndex.pick(ray(2)),null,"hidden element leaves normal picking without changing geometry");
  console.log("Product Sketch ownership, occurrence visibility, tree selection, and incremental display passed");
} finally {await server.close();}
