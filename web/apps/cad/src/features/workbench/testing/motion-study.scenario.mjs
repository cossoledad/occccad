import assert from 'node:assert/strict';
import {motionPlayback,motionFrameAt,quantityIn} from '../../../cad/assembly/motion-study.ts';
const pose=x=>({translation:[x,0,0],rotation:[0,0,0,1]});
const run={snapshot:{documentId:'product',revisionId:'v1',view:{document:{id:'product',versionId:'v1'},product:{instances:[{id:'g'},{id:'link'}]}}},frames:[{timeSeconds:0,kinematicValid:true,unitPoses:{g:pose(0),link:pose(10)}},{timeSeconds:1,kinematicValid:true,unitPoses:{g:pose(0),link:pose(20)}},{timeSeconds:2,kinematicValid:true,unitPoses:{g:pose(0),link:pose(30)}}]};
const before=JSON.stringify(run);
assert.equal(motionFrameAt(run,.99),0);assert.equal(motionFrameAt(run,1),1);assert.equal(motionFrameAt(run,1.99),1);assert.equal(motionFrameAt(run,2.1),2);
assert.equal(motionPlayback(run,1).frame,run.frames[1]);assert.equal(motionPlayback(run,1).view,run.snapshot.view);assert.equal(motionPlayback(run,3),undefined);
const unqualified=structuredClone(run);unqualified.frames[0].kinematicValid=false;assert.equal(motionPlayback(unqualified,0),undefined,'an unqualified matrix is never a playback sample');
const incomplete=structuredClone(run);delete incomplete.frames[0].unitPoses.link;assert.equal(motionPlayback(incomplete,0),undefined);
const wrong=structuredClone(run);wrong.snapshot.view.document.versionId='v2';assert.equal(motionPlayback(wrong,0),undefined);
assert.equal(JSON.stringify(run),before);assert.equal(quantityIn({value:Math.PI,unit:'rad'},'deg'),180);assert.equal(quantityIn({value:.02,unit:'m'},'mm'),20);
assert.throws(()=>quantityIn({value:1,unit:'rad'},'mm'));
console.log('motion study: whole accepted frames, frozen identity, units passed');

// The actual scene writer rejects a partially hydrated frame and a stale revision.
const THREE = await import('three');
const {createServer}=await import('vite');const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try {
const {applyMotionFrame,interpolateMotionPose,motionViewpointAt} = await server.ssrLoadModule('/src/cad/assembly/motion-frame.ts');
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

// Display time is continuous, while source samples and persistence stay discrete.
const continuous={...motionPlayback(run,0),clock:{startedAt:1000,timeSeconds:0,frames:run.frames}};
assert.equal(applyMotionFrame(continuous,run.snapshot.view,groups,transforms,1500),true);
assert.equal(link.position.x,15);assert.equal(continuous.frame,run.frames[0]);
const missingNeighbor=structuredClone(continuous);delete missingNeighbor.clock.frames[1].unitPoses.link;
assert.equal(applyMotionFrame(missingNeighbor,run.snapshot.view,groups,transforms,1500),false);
assert.equal(link.position.x,15,'invalid neighbor cannot partially move the scene');
const failedNeighbor=structuredClone(continuous);failedNeighbor.clock.frames[1].kinematicValid=false;assert.equal(applyMotionFrame(failedNeighbor,run.snapshot.view,groups,transforms,1500),false);
const paused={...motionPlayback(run,0),samples:run.frames,timeSeconds:.3712345};assert.equal(applyMotionFrame(paused,run.snapshot.view,groups,transforms),true);assert(Math.abs(link.position.x-13.712345)<1e-10);
const camera={position:[0,0,10],rotation:[0,0,0,1],up:[0,1,0],target:[0,0,0],zoom:1,top:10,bottom:-10};
const cameraStudy={animateViewpoint:true,keyframes:[{timeSeconds:0,viewpoint:camera},{timeSeconds:1,viewpoint:{...camera,position:[10,0,10],zoom:2}}]};assert.equal(motionViewpointAt(cameraStudy,.375).position[0],3.75);assert.equal(motionViewpointAt(cameraStudy,.375).zoom,1.375);
const aroundPivot=interpolateMotionPose({translation:[12,3,0],rotation:[0,0,0,1]},
 {translation:[2,13,0],rotation:[0,0,Math.SQRT1_2,Math.SQRT1_2]},.5);
assert(Math.abs(aroundPivot.translation[0]-(2+Math.sqrt(50)))<1e-10);
assert(Math.abs(aroundPivot.translation[1]-(3+Math.sqrt(50)))<1e-10);
assert(Math.abs(Math.hypot(aroundPivot.translation[0]-2,aroundPivot.translation[1]-3)-10)<1e-10);
assert.equal(JSON.stringify(run),before,'presentation must not synthesize authoritative frames');
assert.equal(applyMotionFrame(motionPlayback(run,0),run.snapshot.view,groups,transforms),true);

// Invalid wire poses must never apply a partial frame.
const malformed=structuredClone(display);delete malformed.frame.unitPoses.link.rotation;
assert.equal(motionPlayback({...run,frames:[malformed.frame]},0),undefined);
assert.equal(applyMotionFrame(malformed,run.snapshot.view,groups,transforms),false);
assert.equal(ground.position.x,0);assert.equal(link.position.x,10);


// Exercise the actual atomic application writer and record every submitted batch.
const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
const {applicationDisplay,restoredApplicationSources}=await server.ssrLoadModule('/src/cad/assembly/application-display.ts');
const engine=Object.create(CadViewportEngine.prototype);
const scene=new THREE.Group(),fixed=new THREE.Group(),moving=new THREE.Group(),outsider=new THREE.Group();scene.add(fixed,moving,outsider);
const frames=[];const traced={stop:object=>transforms.stop(object),applyBatch(targets,mode){transforms.applyBatch(targets,mode);frames.push([moving.position.x,outsider.position.x])}};
const product={document:{id:'product',versionId:'v1'},product:{instances:[{id:'g',...pose(0)},{id:'link',...pose(100)},{id:'other',...pose(77)}]}};
Object.assign(engine,{instanceGroups:new Map([['g',fixed],['link',moving],['other',outsider]]),transforms:traced,content:scene,host:{dataset:{}},view:product,
 previewInputMaterials:[],previewSketchHidden:[],moveManipulator:{detach(){}},
 addAssemblyConstraintMarkers(){},refreshInteractionHighlights(){},refreshContentBounds(){},invalidate(){},setActiveTool(){},syncAttachedManipulatorPosition(){},
});
const mechanism={id:'m',joints:[],unitIds:['g','link'],poses:{g:pose(0),link:pose(10)}};
const restorationView={...product,product:{...product.product,kinematics:{mechanisms:[mechanism,{id:'other-m'}]}}};
assert.deepEqual(restoredApplicationSources(restorationView,mechanism,new URLSearchParams('applicationRevision=v1&mechanismSources=other-m,m,other-m,deleted')),['other-m','m']);
assert.deepEqual(restoredApplicationSources(restorationView,mechanism,new URLSearchParams('applicationRevision=old&mechanismSources=other-m,m')),['m'],'external Head invalidates the old nonmember environment');
engine.setApplicationDisplay(applicationDisplay(product,mechanism));engine.commitApplicationDisplay();assert.equal(moving.position.x,10);assert.equal(outsider.position.x,77);
engine.setApplicationDisplay(applicationDisplay(product,mechanism,undefined,[{instanceId:'link',...pose(20)}]));engine.commitApplicationDisplay();assert.equal(moving.position.x,20);
const count=frames.length;engine.clearCommandPreview(false);assert.equal(frames.length,count,'resource cleanup must not submit a restoring frame');
// Pending, rejected and superseded requests retain this complete displayed frame.
assert.equal(moving.position.x,20);
engine.setApplicationDisplay(applicationDisplay(product,mechanism));engine.commitApplicationDisplay();assert.equal(moving.position.x,10,'explicit cancel adopts the command baseline');
// A returned accepted pose is installed with its document Revision. Clearing the
// draft before hydration holds the last frame, then adopts exactly one new frame.
const updated={...product,document:{...product.document,versionId:'v2'}},accepted={...mechanism,poses:{g:pose(0),link:pose(25)}};
engine.setApplicationDisplay(applicationDisplay(updated,accepted));engine.commitApplicationDisplay();assert.equal(moving.position.x,10);
engine.view=updated;engine.commitApplicationDisplay();assert.equal(moving.position.x,25);
const exiting={...updated,document:{...updated.document,versionId:'v3'},product:{instances:updated.product.instances.map(v=>v.id==='link'?{...v,...pose(25)}:v)}};
engine.setApplicationDisplay(applicationDisplay(exiting));engine.view=exiting;engine.commitApplicationDisplay();assert.equal(moving.position.x,25,'Product adopts the accepted pose when its equations are empty');
assert.deepEqual(frames.map(v=>v[0]),[10,20,10,25,25]);assert(frames.every(v=>v[1]===77),'mechanism snapshots never own nonmembers');assert.equal(animated,0);

} finally {await server.close()}
