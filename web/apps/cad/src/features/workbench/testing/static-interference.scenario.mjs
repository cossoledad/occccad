import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const crypto=globalThis.crypto,descriptor=Object.getOwnPropertyDescriptor(crypto,'randomUUID');
 Object.defineProperty(crypto,'randomUUID',{configurable:true,value:undefined});
 try{
  const {defaultSpatial}=await server.ssrLoadModule('/src/cad/assembly/spatial-analysis.ts');
  const definitions=['CLASH','SECTION','DISTANCE','BAND'].map(kind=>defaultSpatial(kind));
  assert.equal(new Set(definitions.map(d=>d.id)).size,definitions.length,'independent analysis drafts retain distinct identities without native randomUUID');
  for(const d of definitions)assert.match(d.id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
 }finally{if(descriptor)Object.defineProperty(crypto,'randomUUID',descriptor);else delete crypto.randomUUID;}
 const {staticPartPick,staticPartSelection,staticBodySelection,staticPairLabel,staticReportStatus,staticResultMatches,interferenceGroups,interferenceDisplayIndex,withInterferenceDetail}=await server.ssrLoadModule('/src/cad/assembly/static-interference.ts');
 const {FeaturePickScope}=await server.ssrLoadModule('/src/cad/interaction/feature-selection.ts');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts'),THREE=await server.ssrLoadModule('three');
 const path=id=>({rootDocumentId:'assembly',canonical:`module/${id}`,display:`Module.1 / Part.${id}`,segments:[{ownerDocumentId:'assembly',ownerVersionId:'rev',instanceId:'module',referencedDocumentId:'product-module',resolvedVersionId:'module-rev'},{ownerDocumentId:'product-module',ownerVersionId:'module-rev',instanceId:id,referencedDocumentId:'part',resolvedVersionId:'part-rev'}]});
 const bodies=['a','b'].map((id,i)=>({id:`module/${id}/body:body`,name:`Part.${id} / Body.1`,documentId:'part',revisionId:'part-rev',instancePath:path(id),bodyId:'body',geometryKey:'shared',pose:{translation:[i*12,0,0],rotation:[0,0,0,1]}}));
 const view={document:{id:'assembly',versionId:'rev',type:'PRODUCT'},product:{instances:[]},resolvedInstances:bodies.map(b=>({documentId:'part',occurrencePath:b.instancePath.canonical,instancePath:b.instancePath,bodyId:'body',geometryKey:'shared',name:b.name,bodyTreeNodeId:'tree/'+b.id}))};
 const pick={kind:'face',id:'face',documentId:'part',versionId:'part-rev',instancePath:path('a'),occurrencePath:'module/a',geometryKey:'shared'};
 assert.equal(staticPartPick(view,pick).canonical,'module/a');
 assert.equal(staticPartPick(view,{...pick,versionId:'later-head'}),undefined);
 assert.equal(staticPartPick(view,{...pick,documentId:'another-part'}),undefined);
 assert.equal(staticPartPick(view,{...pick,occurrencePath:'module'}),undefined,'a subassembly is not silently expanded into selected leaves');
 assert.equal(staticPartPick(view,{...pick,kind:'instance',documentId:'product-module',versionId:'module-rev'}).canonical,'module/a','tree instance ownership and viewport geometry refer to the same accepted leaf');
 const wholeLeaf=staticPartSelection(view,path('a'));assert.equal(wholeLeaf.visualKey,'occurrence:module/a','collection picking/locating covers every Body of this leaf, not the containing module');assert.equal(staticPartPick(view,wholeLeaf).canonical,'module/a');
 const first=staticBodySelection(view,bodies[0]),second=staticBodySelection(view,bodies[1]);assert.notEqual(first.visualKey,second.visualKey);assert.notEqual(first.treeNodeId,second.treeNodeId);
 const pair={firstId:bodies[0].id,secondId:bodies[1].id,classification:'SEPARATED',distanceExact:true,witnessKind:'DISTANCE',complete:true,clearanceSatisfied:false,distanceMm:2,commonVolumeMm3:0,firstWitness:[5,0,0],secondWitness:[7,0,0]};
 const report={documentId:'assembly',revisionId:'rev',scope:'ALL',clearanceMm:3,bodies,pairs:[pair],issues:[],complete:true,checkedPairs:1};
 assert.equal(staticPairLabel(pair),'间隙不足');assert.equal(staticPairLabel({...pair,classification:'CONTAINMENT'}),'完全包含');assert.equal(staticPairLabel({...pair,complete:false}),'未完成');
 assert.equal(staticReportStatus({...report,pairs:[],checkedPairs:0}),'没有可检查的零件对');
 assert.equal(staticReportStatus({...report,complete:false}),'已发现问题，检查未完成');
 assert.equal(staticReportStatus({...report,pairs:[{...pair,classification:'INCONCLUSIVE'}]}),'检查未完成');
 assert.equal(staticReportStatus({...report,issues:[{name:'missing',diagnosticCode:'BREP_MISSING'}]}),'已发现问题，检查未完成');
 assert.equal(interferenceGroups(report).length,2,'each occurrence lists its own conflicts');assert(interferenceGroups(report).every(g=>g.pairs.length===1));
 const input={revisionId:'rev',scope:'ALL',clearanceMm:3};assert(staticResultMatches(view,input,report));
 const overlap={...pair,classification:'PENETRATION',detailStatus:'COMPLETE',overlapTriangles:[[0,0,0],[1,0,0],[0,1,0]]};
 const checked={...report,pairs:[pair,overlap]};assert.equal(interferenceDisplayIndex(checked),1,'saved objects and motion prefer actual overlap over separated pairs');
 const detailInput={...input,detailPair:[pair.firstId,pair.secondId],thresholdOnly:false},detailed={...report,pairs:[overlap]};assert.equal(withInterferenceDetail(view,detailInput,report,detailed,pair).pairs[0],overlap);
 assert.throws(()=>withInterferenceDetail(view,detailInput,report,{...detailed,bodies:detailed.bodies.map((b,i)=>i?{...b,pose:{...b.pose,translation:[13,0,0]}}:b)},pair),'a detail response for another pose cannot replace accepted graphics');

 assert.equal(staticResultMatches(view,{...input,clearanceMm:4},report),false);assert.equal(staticResultMatches({...view,document:{...view.document,versionId:'next'}},input,report),false);
 assert.equal(staticResultMatches(view,{...input,scope:'SELECTED',selected:[path('a')]},{...report,scope:'SELECTED'}),false,'an unrelated leaf cannot enter selected scope');
 // The panel uses the existing field/session epoch for immediate revision,
 // scope and explicit cancellation invalidation, including pending responses.
 const scope=new FeaturePickScope();scope.activate('static','rev/ALL/3');const n=scope.latest();assert(scope.current(n));scope.activate('static','rev/SELECTED/3');assert(!scope.current(n));const cancelled=scope.latest();scope.cancel();assert(!scope.current(cancelled));
 const engine=Object.create(CadViewportEngine.prototype),content=new THREE.Group(),analysis=new THREE.Scene(),bindings=new Map();
 const originals=[];let disposedOriginal=0;
 for(const b of bodies){const group=new THREE.Group();group.position.fromArray(b.pose.translation);const mesh=new THREE.Mesh(new THREE.BoxGeometry(10,10,10),new THREE.MeshBasicMaterial());mesh.geometry.addEventListener('dispose',()=>disposedOriginal++);group.add(mesh);content.add(group);bindings.set(`${b.instancePath.canonical}/body:body`,{group,mesh,artifact:{geometryKey:'shared'}});originals.push(mesh.geometry)}
 Object.assign(engine,{view,content,analysisScene:analysis,solidBindings:bindings,moveManipulator:{isDragging:()=>false},invalidate(){}});
const neutralized=[];engine.materials={setInteractionState:(group,state)=>neutralized.push([group,state])};
  assert(engine.showStaticInterference(report,0));assert.equal(analysis.children.length,1);assert.equal(neutralized.length,2);assert(neutralized.every(([,state])=>state==='default'),'tree/list and motion role surfaces preserve their engineering material');assert.equal(analysis.children[0].children.length,4,'two body roles, certified witnesses and connector');
 const witnesses=analysis.children[0].children.find(c=>c.isPoints);assert.equal(witnesses.material.transparent,true);assert(witnesses.renderOrder>Math.max(...analysis.children[0].children.filter(c=>c.isMesh).map(c=>c.renderOrder)),'witness points draw after both role surfaces and the connector');
 const wires=analysis.children[0].children.filter(c=>c.isLineSegments&&c.geometry.type==='EdgesGeometry');assert.equal(wires.length,2);assert.notEqual(wires[0].material.color.getHex(),wires[1].material.color.getHex());assert.notEqual(wires[0].geometry,originals[0]);assert.deepEqual(wires[1].position.toArray(),[12,0,0]);assert(!analysis.children[0].children.some(c=>c.isMesh&&!c.isLineSegments2),'original material stays visible without painted role surfaces');
 engine.showStaticInterference();assert.equal(analysis.children.length,0);assert.equal(disposedOriginal,0,'analysis owns resources and never disposes model geometry');
 assert(engine.showStaticInterference({...report,pairs:[{...pair,complete:false}]},0));assert.equal(analysis.children[0].children.length,2,'uncertified distance has no witnesses');
 assert(engine.showStaticInterference({...report,pairs:[{...pair,classification:'CONTAINMENT',witnessKind:'NONE',overlapTriangles:[[0,0,0],[1,0,0],[0,1,0]],sectionPoints:[[0,0,0],[1,0,0]]}]},0));
 assert.equal(analysis.children[0].children.filter(c=>c.isPoints).length,0,'containment never invents a penetration vector');assert.equal(analysis.children[0].children.filter(c=>c.geometry?.type==='WireframeGeometry').length,1,'intersection volume is a fine triangular overlay');
 const third=new THREE.Group();third.visible=false;content.add(third);bindings.set('other/body:third',{group:third,mesh:new THREE.Mesh(new THREE.BoxGeometry(),new THREE.MeshBasicMaterial()),artifact:{geometryKey:'other'}});
 assert(engine.showStaticInterference(report,0,true));engine.showStaticInterference();assert.equal(third.visible,false,'isolation restores original visibility');
 engine.camera=new THREE.PerspectiveCamera();engine.camera.position.set(30,30,30);engine.camera.lookAt(0,0,0);
 bindings.delete('other/body:third');
 const section={documentId:'assembly',revisionId:'rev',definition:{kind:'SECTION',frame:{translation:[0,0,5],rotation:[0,0,0,1]},size:[20,20,2],sectionType:'BOX',cutMode:'KEEP_OUTSIDE'},check:report,geometry:{bodies:[{id:bodies[0].id,complete:true,segments:[[0,0,5],[1,0,5]],triangles:[[0,0,5],[1,0,5],[0,1,5]],areaMm2:.5}]}};
 assert(engine.showSpatialAnalysis(section));assert.equal(bindings.values().next().value.group.visible,false);
 const clipped=analysis.children[0].children.filter(c=>c.isMesh&&c.material.clippingPlanes?.length);assert.equal(clipped.length,2);assert(clipped.every(c=>c.material.clipIntersection&&c.material.clippingPlanes.length===6));
 engine.showSpatialAnalysis();assert.equal(bindings.values().next().value.group.visible,true);assert.equal(disposedOriginal,0);
 assert.equal(engine.showSpatialAnalysis({...section,check:{...report,revisionId:'old'}}),false);assert.equal(analysis.children.length,0);

 // Interactive sectioning reuses private material clones and restores original
 // visibility, including children that were already hidden before the command.
 engine.sectionMaterials=new Map();engine.sectionEdgeVisibility=new Map();
 const binding=bindings.values().next().value,originalMaterial=binding.mesh.material;
 const edge=new THREE.LineSegments(new THREE.EdgesGeometry(binding.mesh.geometry),new THREE.LineBasicMaterial()),hidden=new THREE.Group();hidden.visible=false;binding.group.add(edge,hidden);
 const edit={...section.definition,scope:'ALL',cutMode:'KEEP_INSIDE'};
 engine.previewSectionClipping(edit);const draftMaterial=binding.mesh.material;
 assert.notEqual(draftMaterial,originalMaterial);assert.equal(draftMaterial.clippingPlanes.length,6);assert.equal(edge.visible,false);assert.equal(hidden.visible,false);
 engine.previewSectionClipping({...edit,frame:{...edit.frame,translation:[0,0,6]}});assert.equal(binding.mesh.material,draftMaterial,'pointermove reuses material and original mesh');assert.equal(binding.mesh.geometry,originals[0]);
 engine.sectionRequested={picking:true};engine.previewSectionClipping(edit);assert.equal(binding.mesh.material,originalMaterial);assert.equal(edge.visible,true);assert.equal(hidden.visible,false,'picking restores original hidden state');
 engine.sectionRequested=undefined;engine.previewSectionClipping(edit);engine.restoreSectionClipping();assert.equal(binding.mesh.material,originalMaterial);assert.equal(engine.sectionMaterials.size,0);assert.equal(engine.sectionEdgeVisibility.size,0);assert.equal(edge.visible,true);assert.equal(hidden.visible,false);assert.equal(disposedOriginal,0);

 bindings.values().next().value.group.position.x=1;assert.equal(engine.showStaticInterference(report,0),false,'temporary poses cannot display saved-pose evidence');assert.equal(analysis.children.length,0);bindings.values().next().value.group.position.x=0;
 for(const state of ['preview','motionDisplay','assemblyPosePreview','mechanismDefinition']){engine[state]={};assert.equal(engine.showStaticInterference(report,0),false,state);engine[state]=undefined}
 engine.view={...view,document:{...view.document,versionId:'new'}};assert.equal(engine.showStaticInterference(report,0),false,'late results cannot restore markers for another Revision');
 assert.equal(disposedOriginal,0);
 console.log('static accepted scope, incomplete status, epoch fences and real Three.js role/witness overlay PASS');
}finally{await server.close()}
