import assert from 'node:assert/strict';
import {renameMotionObject,deleteMotionObjects,motionSelectionTargets} from '../../../cad/assembly/motion-definitions.ts';
const end=id=>({instanceId:id,frame:{translation:[0,0,0],rotation:[0,0,0,1]}});
const defs={mechanisms:[{id:'m',name:'mechanism',unitIds:['shaft','propeller'],joints:[{id:'g',name:'ground',kind:'GROUND',first:end('shaft')},{id:'j',name:'revolute',kind:'REVOLUTE',first:end('shaft'),second:end('propeller'),sources:[{constraintId:'axis',baseline:'source-baseline'}]}]}],drivers:[{id:'d',name:'driver',mechanismId:'m',jointId:'j'}],studies:[{id:'s',name:'study',mechanismId:'m',driverId:'d',driverJointId:''}],associations:[{mechanismId:'m',jointId:'j',role:'axis',constraintId:'formal-axis'}]};
const baseline=JSON.stringify(defs),constraints=[{id:'formal-axis'},{id:'axis'}];
assert.equal(renameMotionObject(defs,'j','轴接合').mechanisms[0].joints[1].name,'轴接合');assert.throws(()=>renameMotionObject(defs,'missing','name'));
const afterJoint=deleteMotionObjects(defs,['j']);assert.deepEqual(afterJoint.drivers,[]);assert.deepEqual(afterJoint.studies,[]);assert.deepEqual(afterJoint.associations,[]);assert.deepEqual(afterJoint.mechanisms[0].unitIds,['shaft']);assert.deepEqual(constraints,[{id:'formal-axis'},{id:'axis'}]);
const afterStudy=deleteMotionObjects(defs,['s']);assert.equal(afterStudy.drivers.length,1);assert.equal(afterStudy.associations.length,1);
assert.equal(deleteMotionObjects(defs,['m']).mechanisms.length,0);assert.equal(deleteMotionObjects(defs,['d']).studies.length,0);assert.equal(JSON.stringify(defs),baseline);
const path=(root,leaf)=>({rootDocumentId:'product',canonical:root+'/'+leaf,segments:[{instanceId:root,ownerDocumentId:'product',ownerVersionId:'frozen'},{instanceId:leaf,referencedDocumentId:'part',resolvedVersionId:'part-v1'}]});
const view={document:{id:'product',versionId:'frozen'},product:{kinematics:defs},resolvedInstances:[{instancePath:path('shaft','a'),occurrencePath:'shaft/a',bodyId:'b1',geometryKey:'shared'},{instancePath:path('propeller','a'),occurrencePath:'propeller/a',bodyId:'b1',geometryKey:'shared'},{instancePath:path('propeller','b'),occurrencePath:'propeller/b',bodyId:'b2',geometryKey:'other'}]};
const selected={kind:'kinematic-object',id:'j',documentId:'product',versionId:'frozen'};
const targets=motionSelectionTargets(view,[selected]);assert.equal(targets.length,4);assert.equal(new Set(targets.map(v=>v.id)).size,4);assert(targets.filter(v=>v.kind==='body').every(v=>v.versionId==='part-v1'));assert.equal(selected.id,'j');assert.equal(motionSelectionTargets(view,[{...selected,id:'g'}]).length,2);
console.log('motion application: dependency deletion, immutable rename, source/formal constraints preserved, nested Body and shared-instance highlights passed');

const {assemblyGeometryRef}=await import('../../../cad/assembly/assembly-reference.ts');
const picked=assemblyGeometryRef({kind:'publication',id:'pub',publicationId:'pub',instanceId:'shaft',publication:{id:'pub',type:'SURFACE',compatibilityVersion:1,target:{persistentSelection:{anchor:{featureId:'stable'}}}},highlightTarget:{kind:'face',id:'surface',topologyId:1,geometryKey:'snapshot',instancePath:path('shaft','a')}});assert.equal(picked.instanceId,'shaft');assert.equal(picked.publicationRef.publicationId,'pub');assert.equal(picked.publicationRef.persistentSelection.anchor.featureId,'stable');

const {createServer}=await import('vite');const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
const {mechanismConstraints}=await server.ssrLoadModule('/src/cad/assembly/mechanism-constraints.ts');
const supported=structuredClone(defs.mechanisms[0]);
const joint=supported.joints[1];
for(const endpoint of [joint.first,joint.second]){
 endpoint.axis={instanceId:endpoint.instanceId,kind:'AXIS',geometryId:'stable-axis'};
 endpoint.plane={instanceId:endpoint.instanceId,kind:'PLANE',geometryId:'stable-plane'};
}
joint.axialOffset={value:.003,unit:'m'};
const supportedBefore=JSON.stringify(supported),relations=mechanismConstraints(supported);
assert.deepEqual(relations.map(c=>[c.id,c.kind]),[['g/ground','FIX'],['j/axis','COINCIDENT'],['j/axial-location','DISTANCE']]);
assert.equal(relations[1].first.instanceId,'shaft');assert.equal(relations[1].second.instanceId,'propeller');
assert.equal(relations[2].value,3);assert.equal(relations[2].distanceRelation,'SELECTED_PLANE_NORMAL_V1');
assert.equal(JSON.stringify(supported),supportedBefore);
joint.constraints=relations.slice(1).map(c=>({...c,evaluationStatus:'VERIFIED'}));
assert.deepEqual(mechanismConstraints(supported).slice(1),joint.constraints,'persisted ordinary constraints are rendered directly');
assert.deepEqual(motionSelectionTargets(view,[{...selected,id:'j/axis'}]),[{...selected,id:'j/axis'}],'relation geometry highlight belongs to the shared constraint renderer');

} finally {await server.close()}
