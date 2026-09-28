import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {SelectionIndex}=await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
 const {visibilityResolverForView}=await server.ssrLoadModule("/src/cad/interaction/visibility-resolver.ts");
 const {DEFAULT_SOLID_DISPLAY}=await server.ssrLoadModule("/src/cad/rendering/display-settings.ts");
 const artifact=(bodyId,x)=>({bodyId,geometryKey:`g-${bodyId}`,mesh:{vertices:[[x,0,0],[x+2,0,0],[x,2,0]],triangles:[[0,1,2]],faceIds:[1],edges:[],topologyVertices:[]},visualization:{referenceGeometry:{datumPlanes:[],axisSystems:[]},primitives:[]}});
 const a=artifact("a",0),b=artifact("b",10);
 const view={document:{id:"part",name:"Part",type:"PART",versionId:"v1"},part:{activeBodyId:"a",bodies:[{id:"a",name:"A",visible:true,geometryKey:a.geometryKey},{id:"b",name:"B",visible:true,geometryKey:b.geometryKey}],features:[]},artifacts:{[a.geometryKey]:a,[b.geometryKey]:b}};
 function engine() {
  const e=Object.create(CadViewportEngine.prototype);
  Object.assign(e,{view,content:new THREE.Group(),helpers:new THREE.Group(),scene:new THREE.Scene(),host:{dataset:{}},solidBindings:new Map(),selectable:new Map(),instanceGroups:new Map(),selectionIndex:new SelectionIndex(),solidDisplay:DEFAULT_SOLID_DISPLAY,
  materials:{surface:()=>new THREE.MeshBasicMaterial({side:THREE.DoubleSide}),edge:()=>new THREE.LineBasicMaterial(),point:()=>new THREE.PointsMaterial()},addVisualPrimitives(){},addAssemblyConstraintMarkers(){},updateCameraClipping(){},invalidate(){}});
  return e;
 }
 const e=engine();e.renderPart(view);assert.equal(e.solidBindings.size,2);e.content.updateMatrixWorld(true);
 for(const [x,id] of [[.2,"a"],[10.2,"b"]]) {
  const pick=e.selectionIndex.pick(new THREE.Raycaster(new THREE.Vector3(x,.2,2),new THREE.Vector3(0,0,-1)));
  assert.equal(pick.kind,"face");assert.equal(pick.topologyId,1);assert.equal(pick.bodyId,id);assert.equal(pick.geometryKey,`g-${id}`);
 }
 e.showPreviewArtifact({...a,bodyId:"a"},"ADD");assert.equal(e.solidBindings.get("root/body:a").group.visible,false);assert.equal(e.solidBindings.get("root/body:b").group.visible,true);
 const hiddenView={...view,structureTree:{kind:"PART",documentId:"part",entityId:"part",children:[
  {kind:"BODY",documentId:"part",entityId:"a",localVisible:false},
  {kind:"BODY",documentId:"part",entityId:"b",localVisible:true}]}};
 const hidden=engine();hidden.renderPart(hiddenView);assert.equal(hidden.solidBindings.size,2);
 hidden.visibilityResolver=visibilityResolverForView(hiddenView);hidden.treeVisibilityOverrides={};hidden.refreshInteractionHighlights=()=>{};
 hidden.applyTreeVisibility();hidden.content.updateMatrixWorld(true);
 assert.equal(hidden.solidBindings.get("root/body:a").group.visible,false);assert.equal(hidden.solidBindings.get("root/body:b").group.visible,true);
 assert.equal(hidden.selectionIndex.pick(new THREE.Raycaster(new THREE.Vector3(.2,.2,2),new THREE.Vector3(0,0,-1))),null);
 const product=engine();const path={canonical:"instance",segments:[{resolvedVersionId:"v1"}]};product.renderProduct({document:{id:"product",name:"Root"},product:{instances:[{id:"instance",translation:[5,0,0]}]},artifacts:view.artifacts,
 resolvedInstances:["a","b"].map(bodyId=>({id:`Root/instance/body:${bodyId}`,bodyId,bodyVisible:true,documentId:"part",geometryKey:`g-${bodyId}`,translation:[5,0,0],occurrencePath:"instance",instancePath:path,bodyTreeNodeId:`part/body:${bodyId}`}))});
 assert.equal(product.solidBindings.size,2);product.content.updateMatrixWorld(true);
 for(const binding of product.solidBindings.values())assert.equal(binding.group.getWorldPosition(new THREE.Vector3()).x,5);
 console.log("Multi-Body Part/occurrence rendering, FACE 1 picking isolation, visibility and target-only preview passed");
} finally {await server.close()}
