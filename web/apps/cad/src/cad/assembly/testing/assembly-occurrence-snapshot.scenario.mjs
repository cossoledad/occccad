import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {occurrenceSnapshot,refreshOccurrenceSelection}=await server.ssrLoadModule("/src/cad/assembly/assembly-occurrence-snapshot.ts");
 const {SelectionIndex}=await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const makePath=(outer,head,leaf)=>({rootDocumentId:"product",canonical:`${outer}/leaf`,display:`${outer}/共享零件`,segments:[
  {ownerDocumentId:"product",ownerVersionId:head,instanceId:outer,referencedDocumentId:"nested",resolvedVersionId:"pinned-product",instanceName:outer},
  {ownerDocumentId:"nested",ownerVersionId:"pinned-product",instanceId:"leaf",referencedDocumentId:"part",resolvedVersionId:leaf,instanceName:"共享零件"}]});
 const view=(head,leaf)=>({document:{id:"product",versionId:head,type:"PRODUCT"},product:{instances:[]},resolvedInstances:["left","right"].flatMap(outer=>["body-a","body-b"].map(bodyId=>({bodyId,occurrencePath:`${outer}/leaf`,instancePath:makePath(outer,head,leaf),geometryKey:`mesh-${bodyId}`,translation:[0,0,0],rotation:[0,0,0,1]})))});
 const oldView=view("old","part-old"),newView=view("new","part-new");
 assert.notDeepEqual(occurrenceSnapshot(newView,"left/leaf"),occurrenceSnapshot(newView,"right/leaf"));
 assert.equal(occurrenceSnapshot(newView,"missing"),undefined);
 const old={kind:"body",id:"body-a",bodyId:"body-a",instanceId:"left",documentId:"part",versionId:"part-old",occurrencePath:"left/leaf",instancePath:makePath("left","old","part-old"),treeNodeId:"body:left",occurrenceRef:{rootDocumentId:"product",instancePath:makePath("left","old","part-old")},snapshotScope:{revisionId:"part-old"}};
 const fresh=refreshOccurrenceSelection(newView,old);
 assert.deepEqual(fresh.instancePath,makePath("left","new","part-new"));assert.equal(fresh.snapshotScope.revisionId,"part-new");assert.equal(fresh.occurrenceRef.instancePath.segments[0].ownerVersionId,"new");
 assert.equal(fresh.instancePath.segments[1].ownerVersionId,"pinned-product","do not rewrite accepted nested/PINNED revisions to root Head");
 const index=new SelectionIndex(),object=new THREE.Mesh(new THREE.BoxGeometry(1,1,1),new THREE.MeshBasicMaterial());
 const geometry=object.geometry;index.register(old,object);index.registerPick(object,()=>old);
 index.setSemanticProjection(s=>refreshOccurrenceSelection(newView,s));assert.deepEqual(index.objectsFor(fresh),[object]);
 assert.deepEqual(index.selectionForTreeNode("body:left").instancePath,fresh.instancePath);
 object.updateMatrixWorld(true);const ray=new THREE.Raycaster(new THREE.Vector3(0,0,10),new THREE.Vector3(0,0,-1));
 const picked=index.pick(ray);assert.deepEqual(picked.instancePath,fresh.instancePath);assert.equal(object.geometry,geometry);
 const engine=Object.create(CadViewportEngine.prototype);
 assert.equal(engine.geometrySignature(oldView),engine.geometrySignature(newView),"metadata and source version alone do not require geometry/BVH/GLB rebuild");
}finally{await server.close();}
