import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {SelectionIndex}=await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
 const {structureSelection}=await server.ssrLoadModule("/src/features/workbench/workbench-tree-model.tsx");
 const engine=Object.create(CadViewportEngine.prototype);
 Object.assign(engine,{selectionIndex:new SelectionIndex(),screenStableReferences:new Map(),renderer:{domElement:{clientWidth:800,clientHeight:600}},materials:{point:()=>new THREE.PointsMaterial()},raycaster:new THREE.Raycaster(),datumAxisPickToleranceWorld:0.01});
 const axis={id:"axis-system-default",name:"Absolute",origin:[0,0,0],xDirection:[1,0,0],yDirection:[0,1,0],zDirection:[0,0,1]};
 const group=new THREE.Group();
 for(const occurrence of ["first","second"]){
  const path={rootDocumentId:"product",canonical:occurrence,segments:[{instanceId:occurrence,referencedDocumentId:"part",resolvedVersionId:"revision"}]};
  const treeNodeId=`document:product/instance:${occurrence}/origin/axis:${axis.id}`;
  const context={documentId:"part",versionId:"revision",occurrencePath:occurrence,instancePath:path,treeNodeId};
  engine.addAxisSystem(axis,group,context);
  for(const [kind,suffix,axisName] of [["AXIS","x","X"],["AXIS","y","Y"],["AXIS","z","Z"],["DATUM_POINT","point",undefined]]){
   const node={id:`${treeNodeId}/${suffix}`,kind,entityId:axis.id,documentId:"part",versionId:"revision",instancePath:path,axis:axisName};
   const tree=structureSelection(node,{document:{id:"product"}});
   const picked=engine.selectionIndex.selectionForTreeNode(node.id);
   assert.equal(picked.treeNodeId,node.id,"hover locates the exact axis child");
   assert.equal(picked.versionId,"revision");
   const objects=engine.selectionIndex.objectsFor(tree);
   assert.equal(objects.length,1,"tree highlights a single axis/point in this occurrence");
   assert.equal(objects[0].userData.occurrencePath,occurrence);
   assert.deepEqual(engine.selectionIndex.objectsFor(picked),objects,"viewport and tree preserve one identity");
   assert.equal(engine.selectionIndex.objectsFor({...tree,versionId:"stale"}).length,0);
  }
 }
 console.log("XYZ/origin selection, hover association and occurrence isolation pass.");
}finally{await server.close()}
