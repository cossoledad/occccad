import assert from 'node:assert/strict';
import {motionPlayback,motionFrameAt,quantityIn} from '../../../cad/assembly/motion-study.ts';
const pose=x=>({translation:[x,0,0],rotation:[0,0,0,1]});
const run={snapshot:{documentId:'product',revisionId:'v1',view:{document:{id:'product',versionId:'v1'},product:{instances:[{id:'g'},{id:'link'}]}}},frames:[{timeSeconds:0,unitPoses:{g:pose(0),link:pose(10)}},{timeSeconds:1,unitPoses:{g:pose(0),link:pose(20)}},{timeSeconds:2,unitPoses:{g:pose(0),link:pose(30)}}]};
const before=JSON.stringify(run);
assert.equal(motionFrameAt(run,.99),0);assert.equal(motionFrameAt(run,1),1);assert.equal(motionFrameAt(run,1.99),1);assert.equal(motionFrameAt(run,2.1),2);
assert.equal(motionPlayback(run,1).frame,run.frames[1]);assert.equal(motionPlayback(run,1).view,run.snapshot.view);assert.equal(motionPlayback(run,3),undefined);
const incomplete=structuredClone(run);delete incomplete.frames[0].unitPoses.link;assert.equal(motionPlayback(incomplete,0),undefined);
const wrong=structuredClone(run);wrong.snapshot.view.document.versionId='v2';assert.equal(motionPlayback(wrong,0),undefined);
assert.equal(JSON.stringify(run),before);assert.equal(quantityIn({value:Math.PI,unit:'rad'},'deg'),180);assert.equal(quantityIn({value:.02,unit:'m'},'mm'),20);
assert.throws(()=>quantityIn({value:1,unit:'rad'},'mm'));
console.log('motion study: whole accepted frames, frozen identity, units passed');

// The actual scene writer rejects a partially hydrated frame and a stale revision.
const THREE = await import('three');
const {createServer}=await import('vite');const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
const {applyMotionFrame} = await server.ssrLoadModule('/src/cad/assembly/motion-frame.ts');
const {TransformTransitionSystem} = await import('../../../cad/animation/transform-transition.ts');
let invalidations=0, animated=0;
const transforms=new TransformTransitionSystem(()=>invalidations++,false,()=>{animated++;throw Error('Motion frames must not interpolate')});
const ground=new THREE.Group(),link=new THREE.Group(),body=new THREE.Group();body.position.set(3,4,5);link.add(body);
const groups=new Map([['g',ground],['link',link]]),display=motionPlayback(run,1);
assert.equal(applyMotionFrame(display,run.snapshot.view,new Map([['g',ground]]),transforms),false);
assert.equal(ground.position.x,0);assert.equal(invalidations,0);
assert.equal(applyMotionFrame(display,wrong.snapshot.view,groups,transforms),false);
assert.equal(applyMotionFrame(display,run.snapshot.view,groups,transforms),true);
assert.equal(link.position.x,20);assert.equal(ground.position.x,0);
assert.deepEqual(body.getWorldPosition(new THREE.Vector3()).toArray(),[23,4,5]);
assert.equal(animated,0);assert.equal(invalidations,1);
const invalid=structuredClone(display);invalid.frame.unitPoses.link.rotation=[0,0,0,0];invalid.frame.unitPoses.g.translation=[10,0,0];
assert.equal(applyMotionFrame(invalid,run.snapshot.view,groups,transforms),false);assert.equal(ground.position.x,0);assert.equal(link.position.x,20);
// Repeated play, seek and reset always apply complete discrete poses.
assert.equal(applyMotionFrame(motionPlayback(run,0),run.snapshot.view,groups,transforms),true);assert.equal(link.position.x,10);
assert.equal(JSON.stringify(run),before);

// Invalid wire poses must never apply a partial frame.
const malformed=structuredClone(display);delete malformed.frame.unitPoses.link.rotation;
assert.equal(motionPlayback({...run,frames:[malformed.frame]},0),undefined);
assert.equal(applyMotionFrame(malformed,run.snapshot.view,groups,transforms),false);
assert.equal(ground.position.x,0);assert.equal(link.position.x,10);


// Exercise the real viewport pose lifecycle without constructing a renderer.
// Cancelling a draft restores the owning mechanism baseline, while leaving the
// application restores Product placement. Neither operation may animate a jump.
const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
const engine=Object.create(CadViewportEngine.prototype);
const scene=new THREE.Group(),fixed=new THREE.Group(),moving=new THREE.Group();scene.add(fixed,moving);
Object.assign(engine,{instanceGroups:new Map([['g',fixed],['link',moving]]),transforms,content:scene,host:{dataset:{}},
 view:{document:{id:'product',versionId:'v1'},product:{instances:[{id:'g',...pose(0)},{id:'link',...pose(100)}]}},
 previewInputMaterials:[],previewSketchHidden:[],moveManipulator:{detach(){}},
 addAssemblyConstraintMarkers(){},refreshInteractionHighlights(){},refreshContentBounds(){},invalidate(){},setActiveTool(){},syncAttachedManipulatorPosition(){},
});
const mechanism={id:'m',joints:[],unitIds:['g','link'],poses:{g:pose(0),link:pose(10)}};
engine.showMechanism(mechanism);assert.equal(moving.position.x,10);
engine.previewAssemblyPoses([{instanceId:'g',...pose(0)},{instanceId:'link',...pose(20)}],'immediate');
assert.equal(moving.position.x,20);engine.clearCommandPreview();assert.equal(moving.position.x,10,'cancel must restore accepted mechanism pose');
engine.setMotionDisplay(motionPlayback(run,1));assert.equal(moving.position.x,20);
engine.showMechanism({...mechanism,poses:{g:pose(0),link:pose(20)}});engine.setMotionDisplay();assert.equal(moving.position.x,20,'closing simulation keeps the adopted mechanism pose');
engine.showMechanism();assert.equal(moving.position.x,100,'exiting mechanism restores Product placement');
assert.equal(animated,0);

} finally {await server.close()}
