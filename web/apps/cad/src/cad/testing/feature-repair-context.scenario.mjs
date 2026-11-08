import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {featureRepairSnapshotMatches,repairParameterPairs}=await server.ssrLoadModule('/src/cad/part/feature-repair-context.ts');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const {SketchModalInputController}=await server.ssrLoadModule('/src/cad/sketch/sketch-modal-input.ts');
 const THREE=await server.ssrLoadModule('three');
 const view={document:{id:'part',versionId:'head'}},data={documentId:'part',baseVersionId:'head',featureId:'sketch',snapshot:{feature:{id:'sketch'}},currentParameters:[{parameterId:'a',label:'same',key:'new-a'},{parameterId:'b',label:'same',key:'new-b'}]};
 assert(featureRepairSnapshotMatches(view,data,'sketch'));assert(!featureRepairSnapshotMatches(view,{...data,baseVersionId:'old'},'sketch'));assert(!featureRepairSnapshotMatches(view,{...data,snapshot:{feature:{id:'other'}}},'sketch'));assert(!featureRepairSnapshotMatches(view,data,'other'));
 data.snapshot.parameters=[{parameterId:'b',key:'old-b'},{parameterId:'a',key:'old-a'}];const pairs=repairParameterPairs(data);assert.equal(pairs[0].historical.key,'old-a');assert.equal(pairs[1].historical.key,'old-b','quantity comparison uses ParameterId, independently of aliases, labels and ordering');
 let registrations=0,writes=0,navigations=0;
 const engine=Object.create(CadViewportEngine.prototype);Object.assign(engine,{clearCommandPreview(){},clearPreview(){},invalidate(){},helpers:new THREE.Group(),renderer:{domElement:{clientWidth:800,clientHeight:600}},selectionIndex:{register(){registrations++}},editContext:{occurrencePath:'a',translation:[100,0,0],rotation:[0,0,0,1]}});
 const snapshot={feature:{id:'old-sketch',sketch:{entities:[{id:'old-line',kind:'LINE',role:'PROFILE',start:{x:0,y:0},end:{x:20,y:0}}]}},sketchFrame:{origin:[0,0,10],normal:[0,0,1],uDirection:[1,0,0]},artifacts:[]};
 engine.showRepairComparison(snapshot);assert(engine.repairComparisonActive);assert.equal(registrations,0,'historical entities never enter the current pick index');const bounds=new THREE.Box3().setFromObject(engine.repairComparisonGroup);assert.equal(bounds.min.x,100);assert.equal(bounds.max.x,120);assert.equal(bounds.min.z,10,'historical frame and owning occurrence are composed once');
 const modal=new SketchModalInputController({pointerDown(){writes++;},keyDown(){writes++;}},{pointerDown(){navigations++;}},{pick(){},blocked:()=>engine.repairComparisonActive});
 const pointer={pointerId:1,button:0,state:{buttons:{left:true,middle:false,right:false}}};modal.pointerDown(pointer);modal.keyDown({key:'Delete'});assert.equal(writes,0,'read-only comparison bars modeling input');modal.pointerDown({...pointer,button:1,state:{buttons:{left:false,middle:true,right:false}}});assert.equal(navigations,1,'navigation remains available');
 engine.showRepairComparison();assert.equal(engine.repairComparisonActive,false);assert.equal(engine.helpers.children.length,0);assert.equal(engine.repairComparisonGroup,undefined);
 console.log('PASS: frozen repair comparison, stable quantities, occurrence frame, non-pickable display and input barrier');
}finally{await server.close();}
