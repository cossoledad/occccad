import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const m=await server.ssrLoadModule('/src/cad/measurement/measurement.ts');
 const {measurementDrawing}=await server.ssrLoadModule('/src/cad/measurement/measurement-overlay.ts');
 const path={rootDocumentId:'root',canonical:'sub/leaf',display:'Sub / Leaf',segments:[{ownerDocumentId:'root',ownerVersionId:'r1',instanceId:'sub',referencedDocumentId:'child',resolvedVersionId:'c1'},{ownerDocumentId:'child',ownerVersionId:'c1',instanceId:'leaf',referencedDocumentId:'part',resolvedVersionId:'p1'}]};
 const view={document:{id:'root',versionId:'r1'},product:{instances:[]}};
 const pick={kind:'face',id:'face',documentId:'part',ownerDocumentId:'part',versionId:'p1',instancePath:path,occurrencePath:path.canonical,bodyId:'main',geometryKey:'exact',topologyId:4};
 assert.deepEqual(m.measurementPick(view,pick),{kind:'FACE',bodyId:'main',instancePath:path,mode:'GEOMETRY',geometryKey:'exact',localId:4});
 assert.equal(m.measurementPick(view,{...pick,versionId:'stale'}),undefined);
 assert.equal(m.measurementPick(view,{...pick,contextVariantKey:'specialized'}),undefined);
 assert.equal(m.measurementPick(view,{...pick,instancePath:undefined}),undefined);
 assert.equal(m.measurementPick(view,pick,'GEOMETRY','EDGE'),undefined);
 assert.equal(m.measurementPick(view,{...pick,kind:'feature'}),undefined,'feature tree identity must not become latest Body');
 const a=m.measurementPick(view,{...pick,kind:'body'},'GEOMETRY'),b=m.measurementPick(view,{...pick,kind:'body',instancePath:{...path,canonical:'other/leaf',segments:[{...path.segments[0],instanceId:'other'},path.segments[1]]}},'GEOMETRY');
 assert.notDeepEqual(a,b,'repeated Part/Body instances remain distinct');
 const result={complete:true,values:{distanceMm:5,deltaXMm:3,deltaYMm:4,deltaZMm:0},anchors:{first:[0,0,0],second:[3,4,0]}};
 const definition={...m.defaultMeasurement('BETWEEN'),result,showComponents:true};
 const drawing=measurementDrawing(definition,'mm');assert.deepEqual(drawing.dimension,[[0,0,0],[3,4,0]]);assert.deepEqual(drawing.components.map(c=>c.value),[3,4,0]);
 const inches=measurementDrawing(definition,'in');assert.deepEqual(inches.dimension,drawing.dimension);assert.notEqual(inches.text,drawing.text,'display units never transform witnesses');
 const angle=measurementDrawing({...m.defaultMeasurement('ANGLE_POINTS'),result:{complete:true,values:{angleRad:Math.PI/2},anchors:{first:[3,0,0],vertex:[0,0,0],second:[0,4,0]}}},'mm');assert.equal(angle.arc.length,33);assert.deepEqual(angle.points[1],[0,0,0]);assert.ok(Math.abs(angle.arc.at(-1)[0])<1e-9);
 assert.equal(m.measurementValue('areaMm2',100,'cm'),'1 cm²');assert.equal(m.measurementValue('volumeMm3',1000,'cm'),'1 cm³');assert.equal(m.measurementValue('angleRad',Math.PI,'mm'),'180°');
 assert.equal(measurementDrawing({...definition,result:{...result,complete:false}},'mm'),undefined,'unfinished computation has no markers');
 const THREE=await server.ssrLoadModule('three');const {applySolidDisplaySettings}=await server.ssrLoadModule('/src/cad/rendering/solid-render-mode.ts');
 const group=new THREE.Group(),edge=new THREE.LineSegments(),points=new THREE.Points();group.add(edge,points);group.visible=false;group.userData.appearanceEdges=false;
 const settings={mode:'shaded',edges:false,vertices:false};applySolidDisplaySettings(group,settings,'VERTEX');assert(points.visible);assert(!edge.visible);assert(!group.visible,'pick display must not reveal hidden Bodies');
 applySolidDisplaySettings(group,settings,'EDGE');assert(edge.visible,'pick field may reveal edges hidden by appearance');assert(!points.visible);applySolidDisplaySettings(group,settings);assert(!edge.visible&&!points.visible);assert.deepEqual(settings,{mode:'shaded',edges:false,vertices:false});
 const node={id:'document:root/measurements/m',kind:'MEASUREMENT',entityId:'m',documentId:'root',instancePath:{rootDocumentId:'root',canonical:'',segments:[]}};
 const annotated={...view,product:{instances:[],measurements:[{id:'m',visible:true}]},structureTree:{id:'document:root',kind:'PRODUCT',children:[node]}};
 const selected=m.measurementSelection(annotated,'m');assert.equal(selected.treeNodeId,node.id);assert.equal(selected.kind,'measurement');
 assert.deepEqual(m.measurementInteraction(annotated,[selected],selected),{selected:['m'],hovered:'m'});
 assert.deepEqual(m.measurementInteraction(annotated,[{...selected,documentId:'other',ownerDocumentId:'other'},{...selected,versionId:'old'},{...selected,occurrencePath:'child'}],{...selected,versionId:'old'}),{selected:[],hovered:undefined});
 assert.equal(m.measurementSelection({...annotated,product:{instances:[],measurements:[{id:'m',visible:false}]}},'m'),undefined);
 const {treeNodeIcon}=await server.ssrLoadModule('/src/features/workbench/tree-node-descriptors.tsx');
 for(const [status,label] of [['STALE','待更新'],['CURRENT','已更新'],['BROKEN','来源不可用'],['UNKNOWN','状态暂不可用']]){
  const icon=treeNodeIcon({kind:'MEASUREMENT',currencyStatus:status});assert.equal(icon.props.children[1].props['aria-label'],'状态 '+label);
 }
 console.log('Measurement stable picks, owner witnesses, units and angle presentation passed.');
}finally{await server.close()}
