import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {defaultGeometricFeature,geometryCommands,geometryFields,geometryReady,splineControlMode,editGeometryPoint,sketchGeometryInput,geometryInputIdentity}=await server.ssrLoadModule('/src/features/workbench/geometric-feature-model.ts');
 const {bindGeometryReference}=await server.ssrLoadModule('/src/features/workbench/geometric-input-selection.ts');
 const {contextTabs,builtinCatalog}=await server.ssrLoadModule('/src/cad/command/workbench-catalog.ts');
 const facts={isMock:false,hostType:'PART',targetType:'PART',sketchActive:false,canEdit:true,rootCanEdit:true,busy:false,selectionKind:'',selectionCount:0,hasWorkingBody:false,moveReceiptPending:false};
 const tabs=contextTabs(builtinCatalog,facts);
 assert.deepEqual(tabs.slice(0,2).map(t=>[t.id,t.name]),[['PART_DESIGN.model','实体'],['PART_DESIGN.wireframe-surface','线框与曲面']]);
 assert(tabs.every(t=>t.workbench==='PART_DESIGN'));
 assert(!builtinCatalog.commands.some(c=>['part.wireframe-surface','part.solid-workbench'].includes(c.id)));
 const sketchTabs=contextTabs(builtinCatalog,{...facts,sketchActive:true});assert(sketchTabs.every(t=>t.workbench==='SKETCHER'));
 const point=id=>({kind:'DATUM',datum:{kind:'POINT',entityId:id}});
 const feature=defaultGeometricFeature('SPLINE'),d=feature.geometry;
 assert.equal(d.curve.points.length,0,'opening a spline never creates sample geometry');assert.equal(geometryReady(d),false);
 splineControlMode(d,true,2);assert.equal(d.curve.knots,undefined,'an incomplete point list never fabricates poles');
 for(const id of ['a','b','c'])editGeometryPoint(d,point(id),{kind:'append'});
 splineControlMode(d,true,2);assert(geometryReady(d));assert.deepEqual(d.curve.multiplicities,[3,3]);
 const ids=d.curve.points.map(p=>p.id);d.curve.weights=[1,2,3];
 editGeometryPoint(d,point('inserted'),{kind:'before',id:ids[1]});splineControlMode(d,true,2);
 assert.deepEqual(d.curve.points.map(p=>p.reference.datum.entityId),['a','inserted','b','c']);
 assert.deepEqual(d.curve.weights,[1,1,2,3]);assert.deepEqual(d.curve.knots,[0,.5,1]);
 assert.throws(()=>editGeometryPoint(d,point('b'),{kind:'after',id:ids[1]}),/已在点列/);
 editGeometryPoint(d,point('replacement'),{kind:'replace',id:ids[1]});assert.equal(d.curve.points[2].id,ids[1]);assert.equal(d.curve.weights[2],2,'replacing a source preserves control identity and its weight');
 const fields=geometryFields(d),weight=fields.find(f=>f.slot===`geometry:weight:${ids[2]}`);weight.set(.5);assert.equal(d.curve.weights.at(-1),.5);
 for(const {kind} of geometryCommands){assert(!geometryReady(defaultGeometricFeature(kind).geometry),`${kind} requires explicit picks`);assert(geometryFields(defaultGeometricFeature(kind).geometry).every(f=>!/(point:|direction:|origin:|normal:|xDirection:)/.test(f.slot)),'operation dialogs expose scalar parameters only')}
 const extrude=defaultGeometricFeature('SURFACE_EXTRUDE');extrude.geometry.inputs=[{kind:'RESULT',featureId:'profile',resultId:'result'}];assert(!geometryReady(extrude.geometry));extrude.geometry.direction={kind:'DATUM',datum:{kind:'AXIS',entityId:'direction'}};assert(geometryReady(extrude.geometry));
 const view={document:{id:'part',versionId:'v'},part:{features:[{id:'sketch',name:'Sketch',sketch:{entities:[{id:'first',kind:'LINE',role:'PROFILE'},{id:'axis',kind:'LINE',role:'CONSTRUCTION'},{id:'suppressed',kind:'LINE',role:'PROFILE',suppressed:true}]}}]}};
 const input=sketchGeometryInput(view,{kind:'sketch',id:'sketch'});assert.deepEqual(input.entityIds,['first']);view.part.features[0].sketch.entities.push({id:'later',kind:'LINE',role:'PROFILE'});assert.deepEqual(input.entityIds,['first']);
 const ref=await bindGeometryReference(view,{kind:'visual',visualType:'POINT',documentId:'part',versionId:'v',featureId:'sketch',entityId:'first',sketchReference:{target:'ENTITY',entityId:'first',subElement:'START',pointId:'stable-start'}},'v','point');
 assert.deepEqual(ref,{kind:'POINT',point:{sketchId:'sketch',reference:{target:'ENTITY',entityId:'first',subElement:'START',pointId:'stable-start'}}});
 assert.equal(geometryInputIdentity({...point('a'),reversed:true}),geometryInputIdentity(point('a')),'reverse changes orientation, not selected object identity');
 console.log('geometry tabs, empty drafts, associative picks and ordered point edits: PASS');
}finally{await server.close()}
