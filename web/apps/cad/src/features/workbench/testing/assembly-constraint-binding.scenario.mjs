import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
 const THREE=await server.ssrLoadModule('three');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const {SelectionIndex}=await server.ssrLoadModule('/src/cad/interaction/selection-index.ts');
 const engine=Object.create(CadViewportEngine.prototype),index=new SelectionIndex();
 const content=new THREE.Group(),a=new THREE.Group(),b=new THREE.Group();b.position.x=10;content.add(a,b);
 const meshA=new THREE.Mesh(new THREE.BoxGeometry(),new THREE.MeshBasicMaterial());a.add(meshA);
 const meshB=meshA.clone();b.add(meshB);
 const path=id=>({rootDocumentId:'root',canonical:id,segments:[{ownerDocumentId:'root',ownerVersionId:'r1',instanceId:id,referencedDocumentId:'part',resolvedVersionId:'part-v1'}]});
 const context=id=>({documentId:'part',versionId:'part-v1',bodyId:'body-'+id,geometryKey:'shape-'+id,occurrencePath:id,instanceId:id,instancePath:path(id)});
 // Distinct faces with the same centroid expose stale bindings that a position
 // signature alone cannot detect. Geometry stays cached across the revision.
 const mesh={vertices:[[-1,-1,0],[1,-1,0],[0,2,0],[-1,0,-1],[1,0,-1],[0,0,2]],triangles:[[0,1,2],[3,4,5]],faceIds:[1,2]};
 const binding=(id,group,bodyMesh)=>({context:context(id),group,mesh:bodyMesh,artifact:{geometryKey:'shape-'+id,mesh}});
 const c={id:'mate',kind:'COINCIDENT',first:{instanceId:'a',kind:'FACE',publicationRef:{scope:'LOCAL',publicationId:'port',expectedType:'SURFACE'}},
  second:{instanceId:'b',kind:'FACE',geometryKey:'shape-b',topologyId:1},evaluationStatus:'NOT_UPDATED'};
 const view=(face,revision,status)=>({document:{id:'root',versionId:revision,type:'PRODUCT'},product:{instances:[],constraints:[{...c,evaluationStatus:status}]},
  resolvedInstances:['a','b'].map(id=>({occurrencePath:id,documentId:'part',bodyId:'body-'+id,geometryKey:'shape-'+id,instancePath:path(id)})),
  structureTree:{id:'document:root',kind:'PRODUCT',documentId:'root',versionId:revision,children:[{id:'document:root/publications/port',kind:'PRODUCT_PUBLICATION',entityId:'port',name:'Mount',documentId:'root',versionId:revision,
   productPublication:{id:'port',name:'Mount',type:'SURFACE',target:{instancePath:path('a'),element:{kind:'TOPOLOGY',bodyId:'body-a'}},resolution:{status:'CONNECTED',topologyKind:'FACE',localId:face,geometryKey:'shape-a',resolvedVersionId:'part-v1'}}}]}});
 Object.assign(engine,{content,helpers:new THREE.Group(),instanceGroups:new Map([['a',a],['b',b]]),solidBindings:new Map([['a',binding('a',a,meshA)],['b',binding('b',b,meshB)]]),
  selectionIndex:index,assemblyConstraintMarkers:new Map(),assemblyConstraintReferences:new Map(),selected:[],preselected:null,selectedOverlays:[],preselectedOverlays:[],highlightedRoots:new Set(),
  renderer:{domElement:{clientWidth:800,clientHeight:600}},invalidate(){},materials:{constraintGlyph:(glyph,color)=>{const m=new THREE.PointsMaterial({color});m.userData.glyph=glyph;return m},setInteractionState(){}},callbacks:{}});
 engine.view=view(1,'r1','NOT_UPDATED');engine.addAssemblyConstraintMarkers(engine.view);
 const marker=()=>[...engine.assemblyConstraintMarkers.values()][0];
 assert.equal(marker().group.children[0].material.userData.glyph,18);
 engine.selected=[marker().selection];engine.refreshInteractionHighlights();
 assert.equal(engine.selectedOverlays.length,2,'constraint selection highlights both actual faces, including the published one');
 engine.preselected=marker().selection;engine.refreshInteractionHighlights();
 assert.equal(engine.preselectedOverlays.length,2,'constraint hover uses the same exact face binding');
 const originalSelection=engine.selected[0];
 engine.view=view(1,'r2','VERIFIED');engine.addAssemblyConstraintMarkers(engine.view);
 assert.equal(marker().group.children[0].material.userData.glyph,0,'successful update replaces the pending glyph');
 assert.equal(engine.selected[0],originalSelection,'display refresh preserves the logical constraint selection');
 const oldMarker=marker().group;
 const oldOverlay=engine.selectedOverlays.find(o=>o.parent===a);
 let disposed=false;oldOverlay.geometry.addEventListener('dispose',()=>{disposed=true});
 engine.view=view(2,'r3','VERIFIED');engine.addAssemblyConstraintMarkers(engine.view);
 assert.notEqual(marker().group,oldMarker,'same-position retarget rebinds the marker');
 assert.equal(engine.assemblyConstraintReferences.values().next().value[0].highlightTarget.topologyId,2);
 assert.equal(engine.selectedOverlays.length,2);assert.ok(disposed,'obsolete face overlay is disposed');
 assert.equal(engine.selectedOverlays.find(o=>o.parent===a).geometry.attributes.position.getZ(0),-1,'highlight follows the new face');
 const refs=engine.assemblyConstraintReferences.values().next().value;
 assert.ok(index.objectsFor(refs[0].highlightTarget).includes(marker().group),'actual face picking associates the current constraint glyph');
 const position=marker().group.position.clone();
 a.position.x=6;engine.assemblyConstraintDisplayDirty=true;engine.refreshAssemblyConstraintDisplay();
 assert.equal(marker().group.position.x,position.x+3,'coalesced transform refresh follows both support anchors');
 assert.equal(marker().group.position.z,position.z);
 assert.equal(engine.assemblyConstraintDisplayDirty,false);
 assert.equal(engine.selectedOverlays.length,2);
 const settled=marker().group;engine.refreshAssemblyConstraintDisplay();assert.equal(marker().group,settled,'no transform change causes no reconstruction');
} finally {await server.close()}
