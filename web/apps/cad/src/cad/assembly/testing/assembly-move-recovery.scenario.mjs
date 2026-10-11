import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule('three');
 const {CadViewportEngine}=await server.ssrLoadModule('/src/viewport/cad-viewport-engine.ts');
 const {AssemblyManipulator}=await server.ssrLoadModule('/src/cad/interaction/assembly-manipulator.ts');
 const {AssemblyInteractionController}=await server.ssrLoadModule('/src/cad/assembly/assembly-interaction.ts');
 const {TransformTransitionSystem}=await server.ssrLoadModule('/src/cad/animation/transform-transition.ts');
 const tick=()=>new Promise(resolve=>setImmediate(resolve)),begins=[],updates=[],cancelled=[],failures=[];
 const engine=Object.create(CadViewportEngine.prototype),group=new THREE.Group(),content=new THREE.Group();
 group.userData={id:'a',kind:'instance',instanceId:'a'};group.add(new THREE.Mesh(new THREE.BoxGeometry(2,2,2),new THREE.MeshBasicMaterial()));content.add(group);
 const shaders={createMaterial:(_program,uniforms)=>new THREE.ShaderMaterial({uniforms:{uActive:{value:0},...Object.fromEntries(Object.entries(uniforms).map(([k,v])=>[k,{value:v}]))}})};
 const manipulator=new AssemblyManipulator(shaders,{poseChanged(){},visualChanged(){},pivotChanged(){},snapPivot(){},dragStarted(){},dragFinished(){}});
 const path={rootDocumentId:'doc',canonical:'a',segments:[{instanceId:'a',ownerDocumentId:'doc',ownerVersionId:'head',referencedDocumentId:'part',resolvedVersionId:'part-head'}]};
 const pose={instanceId:'a',translation:[0,0,0],rotation:[0,0,0,1]};
 Object.assign(engine,{content,instanceGroups:new Map([['a',group]]),selectable:new Map([['instance:a',group]]),
  manipulatorFrames:new Map(),manipulatorPivots:new Map(),moveManipulator:manipulator,selected:[{kind:'instance',id:'a',instanceId:'a'}],
  activeToolID:'assembly.move',moveGestureGeneration:0,transforms:new TransformTransitionSystem(()=>{},false,()=>{}),
  view:{document:{id:'doc',versionId:'head',type:'PRODUCT'},product:{instances:[{id:'a',translation:pose.translation,rotation:pose.rotation}],constraints:[]},resolvedInstances:[{occurrencePath:'a',instancePath:path}]},
  showRemainingMotion(){},addAssemblyConstraintMarkers(){},refreshContentBounds(){},invalidate(){},
  callbacks:{instanceMoved(){throw Error('an unconfirmed final must not commit');}}});
 engine.moveInteraction=new AssemblyInteractionController({
  begin:async input=>{begins.push(input);return {sessionId:'session-'+begins.length,bodyId:'a',baseRevisionId:input.baseRevisionId,inputDigest:'digest',nominalPoses:[pose]};},
  update:input=>new Promise(resolve=>updates.push({input,resolve})),cancel:async id=>cancelled.push(id),
  frame:frame=>engine.applyAcceptedMoveFrame(frame),state(){},failure:error=>failures.push(error)});
 engine.attachMoveManipulator();engine.beginMovePreviewGesture();await tick();
 engine.moveInteraction.target({localGrabPoint:[0,0,0],targetPose:{translation:[8,0,0],rotation:[0,0,0,1]},frameRotation:[0,0,0,1],translationComponents:[true,false,false],rotationComponents:[false,false,false]});
 const respond=(status,final=false)=>{
  const {input,resolve}=updates.shift();assert.equal(input.final,final);
  resolve({sessionId:input.sessionId,inputDigest:'digest',sequence:input.sequence,goalSequence:input.goalSequence,requestId:'final-request',previewId:'',
   instancePoses:[{...pose,translation:[8,0,0]}],evaluationFailure:final?{code:'ASSEMBLY_INTERACTION_UNCONFIRMED',phase:'DRIVEN_TARGET',retryable:true,diagnosticId:'doc/retained-final'}:undefined,
   interaction:{status,hardFeasible:!final,targetConverged:!final,eligibleForCommit:!final,targetSequence:input.sequence,targetError:0,targetOptimality:0}});
 };
 respond('CONSTRAINED');await tick();assert.deepEqual(group.position.toArray(),[8,0,0]);
 engine.finishMovePreviewGesture();respond('FAILED',true);await engine.moveFinalization;await tick();
 assert.equal(engine.moveInteraction.state,'blocked');assert.equal(engine.moveInteraction.hasUncommittedFinal,true);
 assert.equal(failures.length,1);assert.equal(failures[0].diagnosticId,'doc/retained-final');
 await tick();assert.deepEqual(group.position.toArray(),[8,0,0],'the final failure retains its visible preview until explicit cancellation');
 assert.deepEqual(engine.view.product.instances[0].translation,[0,0,0],'the preview is not authority');
 // A display refresh can detach the Group while its final is blocked.
 manipulator.detach();engine.moveTarget=undefined;engine.attachMoveManipulator();assert.equal(manipulator.isAttached(),false);
 engine.cancelAssemblyInteraction();await tick();
 assert.equal(engine.moveInteraction.hasUncommittedFinal,false);assert.deepEqual(group.position.toArray(),[0,0,0]);
 assert.equal(manipulator.isAttached(),true,'cancelling a blocked final restores the current move handle');
 assert.ok(cancelled.includes('session-1'));engine.beginMovePreviewGesture();await tick();
 assert.equal(begins.length,2);assert.equal(begins[1].baseRevisionId,'head');assert.deepEqual(engine.moveTarget.startPosition.toArray(),[0,0,0]);
 engine.cancelAssemblyInteraction();manipulator.dispose();engine.transforms.stopAll();
}finally{await server.close();}
