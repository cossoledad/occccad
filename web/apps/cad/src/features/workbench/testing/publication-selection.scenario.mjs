import assert from "node:assert/strict";
import { createServer } from "vite";

const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const { structureSelection, treeData, treeKeysForSelections } = await server.ssrLoadModule("/src/features/workbench/workbench-tree-model.tsx");
  const { bindPublicationSelection } = await server.ssrLoadModule("/src/viewport/cad-viewport-engine.ts");
  const { assemblyGeometryRef } = await server.ssrLoadModule("/src/cad/assembly/assembly-reference.ts");
  const { describeAssemblyReference } = await server.ssrLoadModule("/src/cad/assembly/assembly-reference-presentation.ts");
  const view = {
    document: { id: "part", type: "PART", versionId: "revision-2" },
    datumPlanes: [{ id: "datum-xy", plane: "XY", name: "XY", origin: [0, 0, 0], normal: [0, 0, 1], uDirection: [1, 0, 0], size: 100 }],
    part: { units: "mm", datumPlanes: [], axisSystems: [], features: [],
      bodies: [{ id: "body-a", geometryKey: "geometry-key" }], publications: [{
      id: "publication-face", name: "Mounting Surface", type: "SURFACE", compatibilityVersion: "1.0.0",
      target: { kind: "TOPOLOGY", sourceVersionId: "revision-1" }, contract: { geometryKind: "SURFACE" },
      resolution: { status: "CONNECTED", resolvedVersionId: "revision-2", topologyKind: "FACE", localId: 7,
        geometryKey: "geometry-key", sourceDigest: "digest" },
    }, {
      id: "publication-plane", name: "Base Plane", type: "PLANE", compatibilityVersion: "1.0.0",
      target: { kind: "DATUM", datumId: "datum-xy" }, contract: { geometryKind: "PLANE" },
      resolution: { status: "CONNECTED", resolvedVersionId: "revision-2" },
    }] },
  };
  const face = structureSelection({ id: "tree/publication-face", kind: "PUBLICATION", name: "Mounting Surface",
    entityId: "publication-face", documentId: "part", versionId: "revision-2" }, view);
  assert.equal(face.kind, "publication");
  assert.equal(face.highlightTarget.kind, "face");
  assert.equal(face.highlightTarget.topologyId, 7);
  assert.equal(face.highlightTarget.geometryKey, "geometry-key");
  assert.equal(face.publicationId, "publication-face");

  const plane = structureSelection({ id: "tree/publication-plane", kind: "PUBLICATION", name: "Base Plane",
    entityId: "publication-plane", documentId: "part", versionId: "revision-2" }, view);
  assert.equal(plane.kind, "publication");
  assert.equal(plane.highlightTarget.kind, "plane");
  assert.equal(plane.highlightTarget.entityId, "datum-xy");
  assert.equal(plane.publicationId, "publication-plane");

  const instancePath = { rootDocumentId: "product", canonical: "instance-a", display: "Part1.1",
    segments: [{ ownerDocumentId: "product", ownerVersionId: "product-r1", instanceId: "instance-a",
      instanceName: "Part1.1", referencedDocumentId: "part", resolvedVersionId: "revision-2" }] };
  view.structureTree = { id: "product", kind: "PRODUCT", name: "Product1", documentId: "product", documentType: "PRODUCT",
    children: [{ id: "product/instance:instance-a", kind: "INSTANCE", name: "Part1(Part1.1)", referenceName: "Part1",
      instanceName: "Part1.1", entityId: "instance-a", documentId: "part", documentType: "PART", instancePath,
      children: [{ id: "product/instance:instance-a/reference/body", kind: "BODY", name: "PartBody", documentId: "part", bodyId: "body-a",
        geometryKey: "geometry-key", versionId: "revision-2", instancePath },
        { id: "product/instance:instance-a/reference/publications/publication:publication-face", kind: "PUBLICATION",
          name: "Mounting Surface", entityId: "publication-face", documentId: "part", instancePath,
          publication: view.part.publications[0] }] }] };
  view.resolvedInstances = [{ id: "instance-a", documentId: "part", bodyId: "body-a",
    occurrencePath: "instance-a", geometryKey: "geometry-key", instancePath }];
  const forwarded = {id:"forwarded-surface",name:"Published mounting surface",type:"SURFACE",compatibilityVersion:"1.0.0",target:{instancePath,publicationId:"publication-face"},contract:{geometryKind:"SURFACE"},resolution:view.part.publications[0].resolution};
  const productPick = structureSelection({id:"product/forwarded",kind:"PRODUCT_PUBLICATION",entityId:forwarded.id,documentId:"product",productPublication:forwarded},view);
  const productReference = assemblyGeometryRef(productPick);
  assert.equal(productReference.instanceId,"instance-a");
  assert.equal(productReference.publicationRef.publicationId,"publication-face","forwarded geometry binds the stable source Publication at its exact occurrence");
  assert.equal(productReference.instancePath.canonical,"instance-a");
  const rawFace = { kind: "face", id: "pick", topologyId: 7, geometryKey: "geometry-key", versionId: "revision-2", occurrencePath: "instance-a",
    instanceId: "instance-a", documentId: "part", bodyId: "body-a" };
  const promoted = bindPublicationSelection(rawFace, view.structureTree);
  assert.equal(promoted.publicationId, "publication-face", "viewport picks should acquire the unique Publication identity");
  const nodes = treeData(view, view);
  const selectedKeys = treeKeysForSelections(nodes, [promoted]);
  assert(!selectedKeys.includes("product/instance:instance-a/reference/body"), "published topology must not select a Body");
  assert(selectedKeys.includes("product/instance:instance-a/reference/publications/publication:publication-face"), "the Publication node should be highlighted");
  const presentation = describeAssemblyReference({ instanceId: "instance-a", kind: "FACE", geometryKey: "geometry-key",
    topologyId: 7, publicationRef: { publicationId: "publication-face", expectedType: "SURFACE", compatibilityVersion: "1.0.0" } }, view);
  assert.equal(presentation.primary, "Part1(Part1.1) / Mounting Surface");
  assert.equal(presentation.publication, "Mounting Surface");
} finally {
  await server.close();
}

const scopedServer=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try {
 const {publicationSelectionTarget,selectionInSnapshot}=await scopedServer.ssrLoadModule('/src/features/workbench/publication-target.ts');
 const view={document:{id:'part-a',versionId:'v1'},part:{datumPlanes:[{id:'datum-xy'}],axisSystems:[],features:[],bodies:[]}};
 const pick={kind:'plane',plane:'XY',id:'datum-xy',entityId:'datum-xy',documentId:'part-a',versionId:'v1'};
 assert.equal(publicationSelectionTarget(view,pick).targetId,'datum-xy');
 assert.equal(publicationSelectionTarget(view,{...pick,documentId:'part-b'}),undefined,'shared standard datum names must not authorize another Part');
 assert.equal(publicationSelectionTarget(view,{...pick,versionId:'old'}),undefined);
 const path={rootDocumentId:'product',canonical:'a',segments:[{ownerDocumentId:'product',ownerVersionId:'r1',instanceId:'a',referencedDocumentId:'part-a',resolvedVersionId:'v1'}]};
 const instancePick={...pick,occurrencePath:'a',instancePath:path};
 assert(selectionInSnapshot(instancePick,'part-a','v1',path));
 assert(!selectionInSnapshot({...instancePick,instancePath:{...path,segments:[{...path.segments[0],ownerVersionId:'r0'}]}},'part-a','v1',path),'full path revision required');
 assert(!publicationSelectionTarget(view,instancePick),'independent Part must not accept occurrence pick');
}finally{await scopedServer.close()}

const moduleServer=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {publicationForwardTarget,publicInterfaceForPick}=await moduleServer.ssrLoadModule('/src/features/workbench/publication-target.ts');
 const {structureSelection}=await moduleServer.ssrLoadModule('/src/features/workbench/workbench-tree-model.tsx');
 const inner={rootDocumentId:'module',canonical:'adapter',display:'Adapter.1',segments:[{ownerDocumentId:'module',ownerVersionId:'m1',instanceId:'adapter',instanceName:'Adapter.1',referencedDocumentId:'adapterdoc',resolvedVersionId:'a1'}]};
 const forwarded={id:'public-hole',name:'Hole location',type:'POINT',compatibilityVersion:'1.0.0',contract:{geometryKind:'POINT'},target:{instancePath:{rootDocumentId:'adapterdoc',canonical:'layout',segments:[]},publicationId:'native'},resolution:{status:'CONNECTED'}};
 const node={id:'adapter-publication',kind:'PRODUCT_PUBLICATION',entityId:forwarded.id,documentId:'adapterdoc',versionId:'a1',instancePath:inner,productPublication:forwarded};
 const view={document:{id:'module',versionId:'m1'},product:{instances:[],publications:[]},structureTree:{kind:'PRODUCT',children:[node]}};
 const selection=structureSelection(node,view);
 assert.equal(selection.productPublication.id,'public-hole','public Product interface must not unwrap to its private native source');
 assert.equal(publicationForwardTarget(view,selection).publicationId,'public-hole');
 assert(!publicationForwardTarget(view,{...selection,versionId:'stale'}));
 const outer={rootDocumentId:'assembly',canonical:'module-2',segments:[{ownerDocumentId:'assembly',ownerVersionId:'root1',instanceId:'module-2',referencedDocumentId:'module',resolvedVersionId:'m1'}]};
 const globalPath={rootDocumentId:'assembly',canonical:'module-2/adapter',segments:[...outer.segments,...inner.segments]};
 const globalPick={...selection,instancePath:globalPath,occurrencePath:globalPath.canonical};
 assert.deepEqual(publicationForwardTarget(view,globalPick,outer).instancePath,inner,'nested editing submits a module-owned exact path');
 assert(!publicationForwardTarget(view,globalPick,{...outer,segments:[{...outer.segments[0],instanceId:'module-1'}]}),'repeated module occurrences cannot authorize one another');
 assert(!publicationForwardTarget(view,{...globalPick,instancePath:{...globalPath,segments:[{...outer.segments[0],ownerVersionId:'old'},...inner.segments]}},outer));
 const leafPath={rootDocumentId:'assembly',canonical:'module-2/leaf',segments:[...outer.segments,{ownerDocumentId:'module',ownerVersionId:'m1',instanceId:'leaf',referencedDocumentId:'layout',resolvedVersionId:'s1'}]};
 const native={id:'point-port',name:'Point',type:'POINT',target:{kind:'SKETCH_ENTITY',sketchId:'sketch',entityId:'point',entityKind:'POINT'},resolution:{status:'CONNECTED',resolvedVersionId:'s1'}};
 const nativeNode={kind:'PUBLICATION',entityId:'point-port',documentId:'layout',versionId:'s1',instancePath:leafPath,publication:native};
 const publicNode={kind:'PRODUCT_PUBLICATION',entityId:'public-point',documentId:'module',versionId:'m1',instancePath:outer,productPublication:{...forwarded,id:'public-point',target:{instancePath:{canonical:'leaf'},publicationId:'point-port'}}};
 const root={document:{id:'assembly',versionId:'root1'},structureTree:{kind:'PRODUCT',children:[publicNode,nativeNode]}};
 const entry={publication:{id:'public-point'},instancePath:outer,documentId:'module',revisionId:'m1'};
 const raw={kind:'visual',id:'point',entityId:'point',featureId:'sketch',documentId:'layout',versionId:'s1',instancePath:leafPath,occurrencePath:leafPath.canonical};
 assert.equal(publicInterfaceForPick(root,[entry],raw),entry,'viewport native pick binds the module public identity');
 assert(!publicInterfaceForPick(root,[entry],{...raw,versionId:'stale'}));
 const duplicate={...entry,publication:{id:'second-public-point'}};root.structureTree.children.push({...publicNode,entityId:'second-public-point',productPublication:{...publicNode.productPublication,id:'second-public-point'}});
 assert(!publicInterfaceForPick(root,[entry,duplicate],raw),'two public interfaces on one source require explicit interface selection');

 // Published datums retain the true leaf Revision when selected through a module.
 for(const [type,datumKind,pickKind] of [['PLANE','PLANE','plane'],['AXIS','DATUM_AXIS','axis']]){
  const datumPub={...native,id:'datum-port',type,target:{kind:'DATUM',datumId:'shared-datum'},resolution:{status:'CONNECTED',resolvedVersionId:'s1'}};
  const datumNode={...nativeNode,entityId:'datum-port',publication:datumPub};
  const forwardedDatum={...publicNode,entityId:'public-datum',productPublication:{...publicNode.productPublication,id:'public-datum',type,target:{instancePath:{canonical:'leaf'},publicationId:'datum-port'}}};
  const v={...root,structureTree:{kind:'PRODUCT',children:[forwardedDatum,datumNode]}};
  const entry={...entryForDatum(),publication:{id:'public-datum'}};
  function entryForDatum(){return {instancePath:outer,documentId:'module',revisionId:'m1'}}
  const leaf=structureSelection(datumNode,v).highlightTarget;
  assert.equal(leaf.versionId,'s1');assert.equal(leaf.documentId,'layout');
  const raw={kind:pickKind,entityId:'shared-datum',documentId:'layout',versionId:'s1',instancePath:leafPath,occurrencePath:leafPath.canonical};
  assert.equal(publicInterfaceForPick(v,[entry],raw),entry);
  assert(!publicInterfaceForPick(v,[entry],{...raw,versionId:'wrong'}));
 }
 const {replacementImpact,replacementCompatible,replacementInputCompatible,receiverMappingComplete,receiverMappingProblem}=await moduleServer.ssrLoadModule('/src/features/workbench/instance-replacement-model.ts');
 const old={id:'old',name:'Mount',type:'POINT',compatibilityVersion:'1',contract:{geometryKind:'POINT',dimension:{length:1}},resolution:{status:'CONNECTED'}};
 assert(replacementCompatible(old,{...old,id:'new'}));
 assert(!replacementCompatible(old,{...old,contract:{...old.contract,dimension:{length:2}}}));
 assert(!replacementCompatible(old,{...old,resolution:{status:'BROKEN_PUBLICATION'}}));
 const bind={id:'binding',name:'Hole input',contextInputId:'local',referenceMode:'FOLLOW_WORKSPACE_WITH_ACCEPT',owningInstancePath:{segments:[{instanceId:'plate'}]},sourceInstancePath:{segments:[{instanceId:'a'}]},publication:{publicationId:'old'}};
 const impactView={product:{instances:[],contextBindings:[bind,{...bind,id:'other',sourceInstancePath:{segments:[{instanceId:'a2'}]}}],publications:[{name:'Public mount',target:{instancePath:bind.sourceInstancePath,publicationId:'old'}}],constraints:[{name:'Mate',first:{instanceId:'a',publicationRef:{publicationId:'plane'}},second:{instanceId:'a2',publicationRef:{publicationId:'unrelated'}}}]}};
 const impact=replacementImpact(impactView,'a');assert.deepEqual(impact.used,['old','plane']);assert.equal(impact.bindings.length,1);assert.equal(impact.constraints.length,1);assert.equal(impact.forwarding.length,1);assert.equal(impact.blocked.length,0);
 assert.equal(replacementImpact({product:{constraints:[{name:'Rigid group',kind:'FIX_TOGETHER',groupMembers:[{instanceId:'a'},{instanceId:'a2'}]}]}},'a').constraints.length,1);
 assert(replacementImpact({product:{contextBindings:[{...bind,referenceMode:'PINNED'}]}},'a').blocked.length);
 assert(replacementImpact({product:{contextBindings:[{...bind,owningInstancePath:bind.sourceInstancePath}]}},'a').blocked.length);
 const input=(id,name,required=false)=>({id,name,type:'POINT',required,contract:{geometryKind:'POINT',symmetry:'NONE'},target:{kind:'SKETCH_EXTERNAL_GEOMETRY',targetId:'sketch'}});
 const oldInputs=[input('old1','Position.1'),input('old2','Position.2')],nextInputs=[input('new1','Position.1',true),input('new2','Position.2',true),input('optional','Extra')];
 assert(replacementInputCompatible(oldInputs[0],nextInputs[0]));
 assert(receiverMappingComplete(oldInputs,nextInputs,['old1','old2'],{old1:'new1',old2:'new2'}));
 assert(!receiverMappingComplete(oldInputs,nextInputs,['old1','old2'],{old1:'new1',old2:'new1'}),'distinct old ports cannot collapse into one local slot');
 assert.equal(receiverMappingProblem(oldInputs,nextInputs,{old1:'new1',old2:'new1'}),'输入“Position.1”与“Position.2”同时对应“Position.1”，请选择不同目标');
 assert.equal(receiverMappingProblem(oldInputs,nextInputs,{old1:'new1',old2:'new2'}),undefined);
 assert(replacementImpact({product:{contextBindings:[{...bind,local:{type:'POINT'},sourceInstancePath:{segments:[]}}]}},'plate').blocked.some(p=>p.includes('本地输入')));
 const {bindingSourceSelection}=await moduleServer.ssrLoadModule('/src/features/workbench/context-input-source.tsx');
 const localBinding={...bind,local:{type:'POINT'},sourceInstancePath:{segments:[]},owningInstancePath:{canonical:'plate',segments:[{instanceId:'plate',referencedDocumentId:'receiver',resolvedVersionId:'accepted'}]}};
 assert.equal(bindingSourceSelection(localBinding).id,'local');assert.equal(bindingSourceSelection(localBinding).kind,'context-input');assert.equal(bindingSourceSelection(localBinding).documentId,'receiver');

 assert(!receiverMappingComplete(oldInputs,[...nextInputs,input('required','Extra',true)],['old1','old2'],{old1:'new1',old2:'new2'}));
 const receiverImpact=replacementImpact({product:{contextBindings:[bind]},contextVariants:[{owningInstancePath:bind.owningInstancePath,part:{contextInputs:oldInputs}}]},'plate');
 assert.equal(receiverImpact.blocked.length,0);assert.equal(receiverImpact.incoming.length,1);assert.deepEqual(receiverImpact.receiverInputs,oldInputs);assert.deepEqual(receiverImpact.used,[]);


}finally{await moduleServer.close()}
