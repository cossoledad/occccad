import assert from 'node:assert/strict';
import * as THREE from 'three';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
const {mechanismSupportCandidate}=await server.ssrLoadModule('/src/cad/assembly/mechanism-selection.ts');
const {featureSelectionHit,pickFeatureSelection}=await server.ssrLoadModule('/src/cad/interaction/feature-selection.ts');
const {SelectionIndex}=await server.ssrLoadModule('/src/cad/interaction/selection-index.ts');
const path=(unit,rev='part-r')=>({rootDocumentId:'product',canonical:unit,segments:[{instanceId:unit,ownerVersionId:'head',referencedDocumentId:'part',resolvedVersionId:rev}]});
const view={document:{id:'product',versionId:'head'},product:{instances:['a','b'].map(id=>({id,documentId:'part',versionId:'part-r'}))},resolvedInstances:['a','b'].map(id=>({instancePath:path(id)}))};
const selection=(id,kind='face',topologyId=1)=>({id:id+kind+topologyId,kind,instanceId:id,documentId:'part',versionId:'part-r',instancePath:path(id),geometryKey:'shared',topologyId,entityId:'datum'});
const j={first:{instanceId:'a'},second:{instanceId:'b'}};
const plane={...selection('a','plane'),plane:{}};
assert(!mechanismSupportCandidate(view,j,'first.axis',plane));
assert(mechanismSupportCandidate(view,j,'first.axis',selection('a')));
assert(!mechanismSupportCandidate({...view,document:{id:'product',versionId:'new'}},j,'first.axis',selection('a')));
assert(!mechanismSupportCandidate(view,j,'first.axis',{...selection('a'),instancePath:path('a','old-part')}));
assert(!mechanismSupportCandidate(view,j,'first.plane',{...plane,instanceId:'b',instancePath:path('b')}));
j.first.axis={};assert(!mechanismSupportCandidate(view,j,'second.axis',selection('a')));
const index=new SelectionIndex(),front=selection('b','face',99),axis=selection('b','face',2);
const mesh=(z)=>{const m=new THREE.Mesh(new THREE.PlaneGeometry(10,10),new THREE.MeshBasicMaterial());m.position.z=z;m.updateMatrixWorld();return m};
const near=mesh(4),far=mesh(0);index.registerPick(near,()=>front);index.registerPick(far,()=>axis);
// Exact inspection has classified 99 as SPHERE and 2 as CYLINDER. Unknown and
// ineligible candidates do not become the selected hit, even if nearer.
const session={role:'axis',accept:s=>mechanismSupportCandidate(view,j,'second.axis',s)&&s.topologyId===2};
const ray=new THREE.Raycaster(new THREE.Vector3(0,0,10),new THREE.Vector3(0,0,-1));
const hit=index.pick(ray,s=>!!featureSelectionHit(s,session));assert.equal(hit.id,axis.id);
assert.equal(featureSelectionHit(front,session),null);assert.equal(featureSelectionHit(axis,session),axis);
let picked=0,resolved=0;
const pendingSession={...session,pendingCandidate:s=>s.topologyId===2,resolvePick:()=>resolved++,onPick:()=>picked++};
pickFeatureSelection(front,pendingSession);assert.equal(resolved,0);assert.equal(picked,0);
pickFeatureSelection(axis,{...pendingSession,accept:()=>false});assert.equal(resolved,1);assert.equal(picked,0);
pickFeatureSelection(axis,pendingSession);assert.equal(picked,1);
near.geometry.dispose();far.geometry.dispose();near.material.dispose();far.material.dispose();
console.log('Mechanism selection: role, owning-unit, Revision, occurrence and exact-filtered ray/tree identity passed');

}finally{await server.close()}
