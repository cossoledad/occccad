import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {defaultGeometricFeature,geometryFields,splineControlMode,sketchGeometryInput}=await server.ssrLoadModule('/src/features/workbench/geometric-feature-model.ts');
 const {contextTabs,builtinCatalog}=await server.ssrLoadModule('/src/cad/command/workbench-catalog.ts');
 const {selectionKey}=await server.ssrLoadModule('/src/cad/interaction/selection-identity.ts');
 const facts={isMock:false,hostType:'PART',targetType:'PART',sketchActive:false,canEdit:true,rootCanEdit:true,busy:false,selectionKind:'',selectionCount:0,hasWorkingBody:false,moveReceiptPending:false};
 assert.equal(contextTabs(builtinCatalog,facts)[0].workbench,'PART_DESIGN');assert.equal(contextTabs(builtinCatalog,{...facts,surfaceActive:true})[0].workbench,'WIREFRAME_SURFACE');assert.equal(contextTabs(builtinCatalog,{...facts,surfaceActive:true,sketchActive:true})[0].workbench,'SKETCHER');
 const feature=defaultGeometricFeature('SPLINE');const original=feature.geometry.curve.points.map(p=>p.id);splineControlMode(feature.geometry,true,2);assert.deepEqual(feature.geometry.curve.knots,[0,1]);assert.deepEqual(feature.geometry.curve.multiplicities,[3,3]);assert.deepEqual(feature.geometry.curve.weights,[1,1,1]);
 feature.geometry.curve.points.splice(1,0,{id:'inserted',position:[10,5,2]});splineControlMode(feature.geometry,true,2);assert.deepEqual(feature.geometry.curve.knots,[0,.5,1]);assert.deepEqual(feature.geometry.curve.multiplicities,[3,1,3]);
 const fields=geometryFields(feature.geometry);const end=fields.find(f=>f.slot===`geometry:point:${original[2]}:z`);end.set(42);assert.equal(feature.geometry.curve.points.at(-1).position[2],42,'coordinate slots follow control identity after insertion');
 const weight=fields.find(f=>f.slot===`geometry:weight:${original[2]}`);weight.set(.5);assert.equal(feature.geometry.curve.weights.at(-1),.5);
 const selected={kind:'feature',id:'f',entityId:'f',documentId:'part',versionId:'v',result:{featureId:'f',resultId:'result'}};assert.notEqual(selectionKey(selected),selectionKey({...selected,result:{featureId:'f',resultId:'other'}}));
 const view={part:{features:[{id:'sketch',sketch:{entities:[{id:'first',kind:'LINE',role:'PROFILE'},{id:'axis',kind:'LINE',role:'CONSTRUCTION'},{id:'suppressed',kind:'LINE',role:'PROFILE',suppressed:true}]}}]}};
 const input=sketchGeometryInput(view,{kind:'sketch',id:'sketch'});assert.deepEqual(input.entityIds,['first']);view.part.features[0].sketch.entities.push({id:'later',kind:'LINE',role:'PROFILE'});assert.deepEqual(input.entityIds,['first'],'fixed sketch members do not absorb a later independent region');
 console.log('geometric feature authoring: PASS');
}finally{await server.close()}
