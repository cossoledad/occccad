import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule("three");
 const {CadViewportEngine}=await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
 const {SelectionIndex}=await server.ssrLoadModule("/src/cad/interaction/selection-index.ts");
 const {AssemblyInteractionController}=await server.ssrLoadModule("/src/cad/assembly/assembly-interaction.ts");
 const engine=Object.create(CadViewportEngine.prototype), index=new SelectionIndex();
 let frames=0,hydrates=0,rebuilds=0;
 const body=new THREE.Group();body.add(new THREE.Mesh(new THREE.BoxGeometry(2,2,2),new THREE.MeshBasicMaterial()));
 const other=body.clone();other.position.x=10;
 Object.assign(engine,{helpers:new THREE.Group(),content:new THREE.Group(),instanceGroups:new Map([["a",body],["b",other]]),
  solidBindings:new Map(),selectionIndex:index,assemblyConstraintReferences:new Map(),assemblyConstraintMarkers:new Map(),selected:[],preselected:null,
  highlightedRoots:new Set(),selectedOverlays:[],preselectedOverlays:[],
  materials:{constraintGlyph:(_glyph,color)=>new THREE.PointsMaterial({color}),setInteractionState(){}},
  invalidate:()=>frames++,updateSketchContextVisibility(){},applyTreeVisibility(){},
  moveInteraction:new AssemblyInteractionController({begin:async()=>{throw Error("display-only fixture has no gesture");},update:async()=>{throw Error("display-only fixture must not solve");},cancel:async()=>{},frame(){},state(){}}),
  visualGeneration:0,visuals:{hydrate(){hydrates++;throw Error("constraint update downloaded mesh");}},renderReady(){rebuilds++;},pendingVisualSnapshot:false});
 const c={id:"same-id",kind:"COINCIDENT",first:{instanceId:"a",kind:"BODY"},second:{instanceId:"b",kind:"BODY"},evaluationStatus:"VERIFIED",suppressed:false};
 const view=constraints=>({document:{id:"product",versionId:"revision",type:"PRODUCT"},product:{instances:[{id:"a",translation:[0,0,0]},{id:"b",translation:[10,0,0]}],constraints},resolvedInstances:[]});
 engine.view=view([c]);engine.renderGeometrySignature=engine.geometrySignature(engine.view);
 engine.render(view([c]));assert.equal(engine.assemblyConstraintMarkers.size,1);
 const entry=[...engine.assemblyConstraintMarkers.values()][0],geometry=body.children[0].geometry;
 engine.selected=[entry.selection];engine.preselected=entry.selection;
 const unrelated={kind:"instance",id:"a",instanceId:"a"};index.register(unrelated,body);
 let disposed=0;entry.group.children[0].geometry.addEventListener("dispose",()=>disposed++);
 engine.render(view([{...c,suppressed:true}]));
 assert.equal(engine.assemblyConstraintMarkers.size,1);assert.equal(engine.selected.length,1,"suppression is not deletion");
 let current=[...engine.assemblyConstraintMarkers.values()][0];assert.equal(current.group.children[0].material.userData.baseColor,0x808080,"suppressed base styling remains distinct beneath selection highlight");
 assert.ok(engine.highlightedRoots.has(current.group));assert.equal(engine.highlightedRoots.has(entry.group),false,"updated group removes previous highlight record");
 const stopped=current.group;engine.render(view([{...c,suppressed:true}]));assert.equal([...engine.assemblyConstraintMarkers.values()][0].group,stopped,"unchanged glyph retained");
 engine.render(view([{...c,mode:"MEASURED",evaluationStatus:"BROKEN"}]));assert.notEqual([...engine.assemblyConstraintMarkers.values()][0].group,stopped);
 engine.render(view([]));assert.equal(engine.assemblyConstraintMarkers.size,0);assert.equal(engine.helpers.children.length,0);
 assert.equal(index.objectsFor(entry.selection).length,0);assert.equal(index.selectionForTreeNode(entry.selection.treeNodeId),null);
 assert.equal(index.picks.length,0);assert.equal(engine.assemblyConstraintReferences.size,0);
 assert.equal(engine.selected.length,0);assert.equal(engine.preselected,null);assert.ok(disposed);
 assert.equal(engine.highlightedRoots.size,0);assert.equal(engine.selectedOverlays.length,0);assert.equal(engine.preselectedOverlays.length,0);
 assert.deepEqual(index.objectsFor(unrelated),[body],"do not delete support body/index/shared resources");
 engine.render(view([c]));engine.render(view([]));assert.equal(engine.helpers.children.length,0,"Undo/Redo projection");
 assert.equal(body.children[0].geometry,geometry);assert.equal(hydrates,0);assert.equal(rebuilds,0);assert.ok(frames>=7,"demand renderer receives a frame without camera movement");
 const nested={...view([]),constraintDisplayScopes:["outer-a","outer-b"].map(occurrence=>({documentId:"child",versionId:"pinned-v1",instancePath:{rootDocumentId:"product",canonical:occurrence},treeNodeId:`document:product/instance:${occurrence}/reference`,constraints:[c]}))};
 engine.instanceGroups=new Map([["outer-a",body],["outer-b",other]]);
 engine.renderGeometrySignature=engine.geometrySignature(nested);engine.render(nested);
 assert.equal(engine.assemblyConstraintMarkers.size,2,"same ConstraintId in separate occurrences never aliases");
 const nestedSelections=[...engine.assemblyConstraintMarkers.values()].map(e=>e.selection);
 assert.notEqual(nestedSelections[0].occurrencePath,nestedSelections[1].occurrencePath);
 assert.notEqual(nestedSelections[0].treeNodeId,nestedSelections[1].treeNodeId);
 assert.equal(nestedSelections[0].instancePath.canonical,"outer-a");
 assert.equal(nestedSelections[0].rootDocumentId,"product");
 engine.render({...nested,constraintDisplayScopes:[nested.constraintDisplayScopes[1]]});
 assert.equal(engine.assemblyConstraintMarkers.size,1);assert.equal(index.objectsFor(nestedSelections[0]).length,0);
 assert.ok(index.objectsFor(nestedSelections[1]).length>0);
 assert.equal(hydrates,0);assert.equal(rebuilds,0);
 // A live editable occurrence immediately projects its own authoritative state,
 // without advancing another occurrence's accepted (including PINNED) version.
 const editing={view:{document:{id:"child",versionId:"head-v2",type:"PRODUCT"},product:{instances:[],constraints:[]}},occurrencePath:"outer-a",liveConstraintProjection:true};
 engine.renderGeometrySignature=engine.geometrySignature(nested,editing);
 engine.render(nested,editing);
 assert.equal(engine.assemblyConstraintMarkers.size,1,"live deletion must not wait for parent UPDATE_REFERENCES");
 assert.equal([...engine.assemblyConstraintMarkers.values()][0].selection.occurrencePath,"outer-b");
 const suppressedEditing={...editing,view:{...editing.view,product:{instances:[],constraints:[{...c,suppressed:true}]}}};
 engine.render(nested,suppressedEditing);
 assert.equal(engine.assemblyConstraintMarkers.size,2);
 const active=[...engine.assemblyConstraintMarkers.values()].find(e=>e.selection.occurrencePath==="outer-a");
 assert.equal(active.group.children[0].material.color.getHex(),0x808080);
 const pinned=[...engine.assemblyConstraintMarkers.values()].find(e=>e.selection.occurrencePath==="outer-b");
 assert.notEqual(pinned.group.children[0].material.color.getHex(),0x808080);
 engine.render(nested,editing);engine.render(nested,editing);
 assert.equal(engine.assemblyConstraintMarkers.size,1,"closing constraint dialog retains live edit context; old root snapshot cannot resurrect glyph");
 engine.render(nested,{...editing,liveConstraintProjection:false});
 assert.equal(engine.assemblyConstraintMarkers.size,2,"noneditable context cannot override accepted snapshot");
 assert.equal(hydrates,0);assert.equal(rebuilds,0);
 engine.editContext=undefined;
 // Even a pending same-snapshot geometry request cannot mount its obsolete
 // constraints after a newer fast-path deletion projection.
 let finish;engine.visuals={hydrate:()=>new Promise(resolve=>{finish=resolve}),retain(){}};
 engine.view=view([]);engine.renderGeometrySignature=engine.geometrySignature(engine.view);
 const old={...view([c]),product:{...view([c]).product,instances:[{id:"a",translation:[3,0,0]},{id:"b",translation:[10,0,0]}]}};
 engine.render(old);engine.render(view([]));finish(old);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(rebuilds,0,"late hydration may not restore obsolete constraint collection");
 assert.equal(engine.assemblyConstraintMarkers.size,0);
} finally {await server.close();}
