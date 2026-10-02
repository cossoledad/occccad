import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule("three");
 const {AssemblyManipulator}=await server.ssrLoadModule("/src/cad/interaction/assembly-manipulator.ts");
 const {transformAroundWorldPivot}=await server.ssrLoadModule("/src/cad/rendering/viewport-metrics.ts");
 const {AssemblyInteractionController}=await server.ssrLoadModule("/src/cad/assembly/assembly-interaction.ts");
 const tick=()=>new Promise(resolve=>setImmediate(resolve)),pending=[],display=[],failures=[];
 const baseline=new THREE.Vector3(3,2,0),pivot=new THREE.Vector3(3,4,0),rotation=new THREE.Quaternion();
 const camera=new THREE.OrthographicCamera(-10,10,10,-10,.1,100);camera.position.z=20;camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
 const surface={clientWidth:1000,clientHeight:1000,getBoundingClientRect:()=>({width:1000,height:1000})};
 const material={createMaterial:(_program,uniforms)=>new THREE.ShaderMaterial({uniforms:{uActive:{value:0},...Object.fromEntries(Object.entries(uniforms).map(([k,v])=>[k,{value:v}]))}})};
 const controller=new AssemblyInteractionController({begin:async()=>({sessionId:"s",bodyId:"a",baseRevisionId:"r",inputDigest:"input",nominalPoses:[]}),update:request=>new Promise(resolve=>pending.push({request,resolve})),cancel:async()=>{},state:()=>{},failure:e=>failures.push(e),frame:f=>{display.push(f);manipulator.setAuthoritativePose(new THREE.Vector3(...f.instancePoses[0].translation).add(new THREE.Vector3(0,2,0)));}});
 const manipulator=new AssemblyManipulator(material,{visualChanged(){},pivotChanged(){},snapPivot(){},dragStarted(){},dragFinished(){},poseChanged(){const pose=manipulator.candidatePose();const target=transformAroundWorldPivot(baseline,rotation,pivot,pose.position,pose.rotation);controller.target({localGrabPoint:[0,2,0],frameRotation:[0,0,0,1],...manipulator.components(),targetPose:{translation:target.position.toArray(),rotation:target.rotation.toArray()}});}});
 manipulator.attach(pivot,rotation);manipulator.object.updateMatrixWorld(true);
 manipulator.updateScale(camera,{cssWidth:1000,cssHeight:1000,devicePixelRatio:1});manipulator.object.updateMatrixWorld(true);
 manipulator.pick=()=>({axis:"X",operation:"translate",pick:new THREE.Object3D(),materials:[]});
 controller.begin({baseRevisionId:"r",instanceId:"a",localGrabPoint:[0,2,0],frameRotation:[0,0,0,1]});await tick();
 assert.equal(manipulator.pointerDown(1,650,300,camera,surface),true);
 const resolveFrame=async(status="REACHED")=>{const {request,resolve}=pending.shift();resolve({sessionId:"s",inputDigest:"input",sequence:request.sequence,goalSequence:request.goalSequence,previewId:request.final&&status==="REACHED"?"candidate":"",requestId:"commit",commitCommand:request.final?{type:"MOVE_INSTANCE"}:undefined,instancePoses:[{instanceId:"a",...request.target.targetPose}],interaction:{status,hardFeasible:true,targetConverged:status==="REACHED",eligibleForCommit:status==="REACHED",targetSequence:request.sequence,targetError:status==="REACHED"?0:1,targetOptimality:0}});await tick();return request;};
 for(let i=1;i<=100;i++){manipulator.pointerMove(1,650+i,300,camera,surface);assert.equal(pending.length,1,"one in flight, latest targets coalesced");}
 await resolveFrame("BUDGET");assert.equal(failures.length,0,"intermediate budget is not a network timeout or a destructive cancel");
 await resolveFrame();const checkpoint=controller.lastQualifiedFrame;
 for(let i=99;i>=60;i--)manipulator.pointerMove(1,650+i,300,camera,surface);
 await resolveFrame();await resolveFrame();
 const goal=controller.lastQualifiedFrame.instancePoses[0];assert.ok(Math.abs(goal.translation[0]-4.2)<1e-6,"cumulative 60px = 1.2mm, not acceptedPose plus accumulated input");assert.deepEqual(goal.rotation,[0,0,0,1]);
 const final=controller.finish();await resolveFrame("BUDGET");await resolveFrame("BUDGET");const lastRequest=await resolveFrame();const candidate=await final;
 assert.ok(candidate);assert.equal(candidate.goalSequence,lastRequest.goalSequence);assert.deepEqual(candidate.target.targetPose,{translation:goal.translation,rotation:goal.rotation});assert.equal(failures.length,0);
 assert.deepEqual(candidate.target.holdRotationComponents,[true,true,true]);assert.deepEqual(candidate.target.holdTranslationComponents,[false,true,true]);
 controller.committed();manipulator.pointerUp(1,true);manipulator.dispose();assert.ok(checkpoint);
}finally{await server.close();}
