import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {staticPartPick,staticPartSelection,staticBodySelection,staticPairLabel,staticReportStatus,staticResultMatches}=await server.ssrLoadModule('/src/cad/assembly/static-interference.ts');
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
 const pair={firstId:bodies[0].id,secondId:bodies[1].id,classification:'SEPARATED',complete:true,clearanceSatisfied:false,distanceMm:2,commonVolumeMm3:0,firstWitness:[5,0,0],secondWitness:[7,0,0]};
 const report={documentId:'assembly',revisionId:'rev',scope:'ALL',clearanceMm:3,bodies,pairs:[pair],issues:[],complete:true,checkedPairs:1};
 assert.equal(staticPairLabel(pair),'间隙不足');assert.equal(staticPairLabel({...pair,classification:'CONTAINMENT'}),'包含');assert.equal(staticPairLabel({...pair,complete:false}),'未完成');
 assert.equal(staticReportStatus({...report,pairs:[],checkedPairs:0}),'没有可检查的零件对');
 assert.equal(staticReportStatus({...report,complete:false}),'检查未完成');
 assert.equal(staticReportStatus({...report,pairs:[{...pair,classification:'INCONCLUSIVE'}]}),'检查未完成');
 assert.equal(staticReportStatus({...report,issues:[{name:'missing',diagnosticCode:'BREP_MISSING'}]}),'检查未完成');
 const input={revisionId:'rev',scope:'ALL',clearanceMm:3};assert(staticResultMatches(view,input,report));
 assert.equal(staticResultMatches(view,{...input,clearanceMm:4},report),false);assert.equal(staticResultMatches({...view,document:{...view.document,versionId:'next'}},input,report),false);
 assert.equal(staticResultMatches(view,{...input,scope:'SELECTED',selected:[path('a')]},{...report,scope:'SELECTED'}),false,'an unrelated leaf cannot enter selected scope');
 // The panel uses the existing field/session epoch for immediate revision,
 // scope and explicit cancellation invalidation, including pending responses.
 const scope=new FeaturePickScope();scope.activate('static','rev/ALL/3');const n=scope.latest();assert(scope.current(n));scope.activate('static','rev/SELECTED/3');assert(!scope.current(n));const cancelled=scope.latest();scope.cancel();assert(!scope.current(cancelled));
 const engine=Object.create(CadViewportEngine.prototype),content=new THREE.Group(),analysis=new THREE.Scene(),bindings=new Map();
 const originals=[];let disposedOriginal=0;
 for(const b of bodies){const group=new THREE.Group();group.position.fromArray(b.pose.translation);const mesh=new THREE.Mesh(new THREE.BoxGeometry(10,10,10),new THREE.MeshBasicMaterial());mesh.geometry.addEventListener('dispose',()=>disposedOriginal++);group.add(mesh);content.add(group);bindings.set(`${b.instancePath.canonical}/body:body`,{group,mesh,artifact:{geometryKey:'shared'}});originals.push(mesh.geometry)}
 Object.assign(engine,{view,content,analysisScene:analysis,solidBindings:bindings,moveManipulator:{isDragging:()=>false},invalidate(){}});
 assert(engine.showStaticInterference(report,0));assert.equal(analysis.children.length,1);assert.equal(analysis.children[0].children.length,4,'two body roles, certified witnesses and connector');
 const witnesses=analysis.children[0].children.find(c=>c.isPoints);assert.equal(witnesses.material.transparent,true);assert(witnesses.renderOrder>Math.max(...analysis.children[0].children.filter(c=>c.isMesh).map(c=>c.renderOrder)),'witness points draw after both role surfaces and the connector');
 const meshes=analysis.children[0].children.filter(c=>c.isMesh);assert.notEqual(meshes[0].material.color.getHex(),meshes[1].material.color.getHex());assert.notEqual(meshes[0].geometry,originals[0]);assert.deepEqual(meshes[1].position.toArray(),[12,0,0]);
 engine.showStaticInterference();assert.equal(analysis.children.length,0);assert.equal(disposedOriginal,0,'analysis owns resources and never disposes model geometry');
 assert(engine.showStaticInterference({...report,pairs:[{...pair,complete:false}]},0));assert.equal(analysis.children[0].children.length,2,'uncertified distance has no witnesses');
 bindings.values().next().value.group.position.x=1;assert.equal(engine.showStaticInterference(report,0),false,'temporary poses cannot display saved-pose evidence');assert.equal(analysis.children.length,0);bindings.values().next().value.group.position.x=0;
 for(const state of ['preview','motionDisplay','assemblyPosePreview','mechanismDefinition']){engine[state]={};assert.equal(engine.showStaticInterference(report,0),false,state);engine[state]=undefined}
 engine.view={...view,document:{...view.document,versionId:'new'}};assert.equal(engine.showStaticInterference(report,0),false,'late results cannot restore markers for another Revision');
 assert.equal(disposedOriginal,0);
 console.log('static accepted scope, incomplete status, epoch fences and real Three.js role/witness overlay PASS');
}finally{await server.close()}
