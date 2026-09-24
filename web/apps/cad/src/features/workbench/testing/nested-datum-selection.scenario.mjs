import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {assemblyGeometryRef}=await server.ssrLoadModule("/src/cad/assembly/assembly-reference.ts");
 const engine=Object.create(CadViewportEngine.prototype);
 Object.assign(engine,{content:new THREE.Group(),instanceGroups:new Map(),selectable:new Map(),selectionIndex:{register(){}},addVisualPrimitives(){},addAssemblyConstraintMarkers(){}});
 const selections=[];
 const add=(kind,datum,parent,context,axis)=>{parent.add(new THREE.Group());selections.push({...context,kind,entityId:datum.id,axis})};
 engine.addDatumPlane=(datum,parent,selectable,context)=>add("plane",datum,parent,context);
 engine.addAxisSystem=(datum,parent,context)=>{add("axis-system",datum,parent,context);for(const axis of ["X","Y","Z"])add("axis",datum,parent,context,axis)};
 engine.addDatumAxis=(datum,parent,context)=>add("axis",datum,parent,context,"DATUM");
 const path={rootDocumentId:"root",canonical:"sub/leaf",segments:[{instanceId:"sub",referencedDocumentId:"child",resolvedVersionId:"child-v1"},{instanceId:"leaf",referencedDocumentId:"part",resolvedVersionId:"part-v1"}]};
 engine.renderProduct({document:{id:"root",name:"Root"},product:{instances:[{id:"sub",documentId:"child",translation:[0,0,0]}]},
  resolvedInstances:[{id:"Root/sub/leaf",documentId:"part",geometryKey:"geometry",translation:[10,0,0],instancePath:path,occurrencePath:path.canonical,bodyTreeNodeId:"leaf/body"}],
  artifacts:{geometry:{geometryKey:"geometry",mesh:{triangles:[]},visualization:{referenceGeometry:{datumPlanes:[{id:"datum-xy"}],axisSystems:[{id:"axis-system-default"}],datumAxes:[{id:"custom-axis"}]}}}}});
 assert.equal(selections.length,6);
 for(const selection of selections){
  const reference=assemblyGeometryRef(selection);
  assert.equal(reference.instanceId,"sub");
  assert.deepEqual(reference.instancePath,path);
  assert.equal(selection.documentId,"part");assert.equal(selection.versionId,"part-v1");
 }
 console.log("Nested datum selections preserve the accepted leaf path.");
}finally{await server.close()}
