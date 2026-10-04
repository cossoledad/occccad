import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {SelectionIndex}=await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
 const {visibilityResolverForView}=await server.ssrLoadModule("/src/cad/interaction/visibility-resolver.ts");
 const {DEFAULT_SOLID_DISPLAY}=await server.ssrLoadModule("/src/cad/rendering/display-settings.ts");
 const {featureInputDisplay}=await server.ssrLoadModule("/src/viewport/feature-input-display.ts");
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

 const picking=engine();picking.previewInputMaterials=[];picking.previewVisualGeneration=0;
 picking.featureSelection={role:"face",selections:[],onPick(){}};
 picking.renderPart(view);picking.content.updateMatrixWorld(true);
 const originalMaterial=picking.solidBindings.get("root/body:a").mesh.material;
 picking.showPreviewArtifact({...a,bodyId:"a"},"REMOVE");
 assert.equal(picking.solidBindings.get("root/body:a").group.visible,true,"input remains eligible for picking during preview");
 assert.equal(picking.solidBindings.get("root/body:a").mesh.material.visible,false,"opaque input does not cover the computed result");
 assert.equal(picking.selectionIndex.pick(new THREE.Raycaster(new THREE.Vector3(.2,.2,2),new THREE.Vector3(0,0,-1))).bodyId,"a");
 picking.clearCommandPreview();
 assert.equal(picking.solidBindings.get("root/body:a").mesh.material,originalMaterial,"cancel restores exact original material ownership");
 const hiddenView={...view,structureTree:{kind:"PART",documentId:"part",entityId:"part",children:[
  {kind:"BODY",documentId:"part",entityId:"a",localVisible:false},
  {kind:"BODY",documentId:"part",entityId:"b",localVisible:true}]}};
 const hidden=engine();hidden.renderPart(hiddenView);assert.equal(hidden.solidBindings.size,2);
 hidden.visibilityResolver=visibilityResolverForView(hiddenView);hidden.treeVisibilityOverrides={};hidden.refreshInteractionHighlights=()=>{};
 hidden.applyTreeVisibility();hidden.content.updateMatrixWorld(true);
 assert.equal(hidden.solidBindings.get("root/body:a").group.visible,false);assert.equal(hidden.solidBindings.get("root/body:b").group.visible,true);
 assert.equal(hidden.selectionIndex.pick(new THREE.Raycaster(new THREE.Vector3(.2,.2,2),new THREE.Vector3(0,0,-1))),null);
 const consumedView={...view,part:{...view.part,bodies:view.part.bodies.map(body=>({...body,consumed:body.id==="b"}))}};
 const consumed=engine();consumed.renderPart(consumedView);consumed.content.updateMatrixWorld(true);
 assert.equal(consumed.solidBindings.size,1,"Boolean consumed Body is excluded from final display");
 assert.equal(consumed.selectionIndex.pick(new THREE.Raycaster(new THREE.Vector3(10.2,.2,2),new THREE.Vector3(0,0,-1))),null,"consumed tools cannot be picked");
 assert.equal(consumedView.part.bodies[1].visible,true,"consumption does not rewrite visibility");
 const product=engine();const path={canonical:"instance",segments:[{resolvedVersionId:"v1"}]};product.renderProduct({document:{id:"product",name:"Root"},product:{instances:[{id:"instance",translation:[5,0,0]}]},artifacts:view.artifacts,
 resolvedInstances:["a","b"].map(bodyId=>({id:`Root/instance/body:${bodyId}`,bodyId,bodyVisible:true,documentId:"part",geometryKey:`g-${bodyId}`,translation:[5,0,0],occurrencePath:"instance",instancePath:path,bodyTreeNodeId:`part/body:${bodyId}`}))});
 assert.equal(product.solidBindings.size,2);product.content.updateMatrixWorld(true);
 for(const binding of product.solidBindings.values())assert.equal(binding.group.getWorldPosition(new THREE.Vector3()).x,5);
 const formalProduct={document:{id:"product",name:"Root"},product:{instances:[{id:"instance",translation:[5,0,0]}]},artifacts:view.artifacts,
  structureTree:{kind:"PRODUCT",children:[{id:"tool-node",kind:"BODY",name:"Tool B",subject:{documentId:"part",entityId:"b"},occurrence:{instancePath:path}}]},
  resolvedInstances:[{id:"Root/instance/body:a",bodyId:"a",bodyVisible:true,documentId:"part",geometryKey:"g-a",translation:[5,0,0],occurrencePath:"instance",instancePath:path,bodyTreeNodeId:"target-node"}]};
 const staged=featureInputDisplay(formalProduct,[a,b],"part","instance"),stagedEngine=engine();stagedEngine.renderProduct(staged);
 assert.equal(stagedEngine.solidBindings.size,2,"temporarily restored consumed tool is rendered in the actual Product path");
 assert.equal(staged.resolvedInstances[1].bodyTreeNodeId,"tool-node");
 assert.equal(formalProduct.resolvedInstances.length,1,"temporary edit display never restores tools in formal output");
 const references=engine();references.treeVisibilityOverrides={};references.refreshInteractionHighlights=()=>{};references.referenceVisibility={planes:false};
 const ownPlane=new THREE.Mesh(),otherPlane=new THREE.Mesh();ownPlane.userData={kind:"plane",id:"p",documentId:"part"};otherPlane.userData={kind:"plane",id:"q",documentId:"other"};references.helpers.add(ownPlane,otherPlane);
 references.featureSelection={role:"plane",documentId:"part"};references.applyTreeVisibility();
 assert.equal(ownPlane.visible,true,"active neutral-plane input temporarily reveals its references");assert.equal(otherPlane.visible,false);
 references.featureSelection=undefined;references.applyTreeVisibility();assert.equal(ownPlane.visible,false,"closing input restores display preferences");
 if (process.env.OCCCCAD_TEST_CURVED_GLB) {
  const {decodeMeshGLB}=await server.ssrLoadModule("/src/cad/visual/mesh-glb.ts");
  const bytes=await readFile(process.env.OCCCCAD_TEST_CURVED_GLB);
  const decoded=decodeMeshGLB(bytes.buffer.slice(bytes.byteOffset,bytes.byteOffset+bytes.byteLength));
  const native={...a,mesh:decoded.mesh};
  const nativeView={...view,artifacts:{...view.artifacts,[a.geometryKey]:native}};
  const rendered=engine();rendered.view=nativeView;rendered.renderPart(nativeView);rendered.content.updateMatrixWorld(true);
  const binding=rendered.solidBindings.get("root/body:a");
  const edge=decoded.mesh.edges.find(edge=>Math.abs(edge.points[0][2]-edge.points.at(-1)[2])>10);
  assert.ok(edge,"native cylinder includes its periodic seam");
  const p=new THREE.Vector3().fromArray(edge.points[0]).add(new THREE.Vector3().fromArray(edge.points.at(-1))).multiplyScalar(.5);
  const ray=new THREE.Raycaster(p.clone().add(new THREE.Vector3(20,0,0)),new THREE.Vector3(-1,0,0));ray.params.Line.threshold=.001;
  const picked=rendered.selectionIndex.pick(ray);
  assert.equal(picked.kind,"edge");assert.equal(picked.topologyId,edge.localId);assert.equal(picked.bodyId,"a");
  Object.assign(rendered,{selectedOverlays:[],preselectedOverlays:[],renderer:{domElement:{clientWidth:800,clientHeight:600}}});
  for(const layer of ["selected","preselected"]) {
   rendered.addTopologyOverlay(layer,picked);
   const overlay=(layer==="selected"?rendered.selectedOverlays:rendered.preselectedOverlays).at(-1);
   assert.equal(overlay.parent,binding.group);
   const starts=overlay.geometry.getAttribute("instanceStart"),ends=overlay.geometry.getAttribute("instanceEnd");
   assert.equal(starts.count,edge.points.length-1);
   for(let i=0;i<starts.count;i++) {
    assert.deepEqual([starts.getX(i),starts.getY(i),starts.getZ(i)],edge.points[i]);
    assert.deepEqual([ends.getX(i),ends.getY(i),ends.getZ(i)],edge.points[i+1]);
   }
  }
  rendered.addTopologyOverlay("selected",{...picked,geometryKey:"previous-snapshot"});
  assert.equal(rendered.selectedOverlays.length,1,"stale snapshot picks cannot highlight the new geometry");
  console.log("Native curved GLB: seam picking and hover/selection reuse the exact snapshot polyline");
 }
 console.log("Multi-Body Part/occurrence rendering, FACE 1 picking isolation, visibility and target-only preview passed");
} finally {await server.close()}
