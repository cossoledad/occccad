import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const THREE=await server.ssrLoadModule('three');
 const {SectionManipulator,resizeSection}=await server.ssrLoadModule('/src/cad/interaction/section-manipulator.ts');
 const {defaultSpatial}=await server.ssrLoadModule('/src/cad/assembly/spatial-analysis.ts');
 const {sectionClippingPlanes}=await server.ssrLoadModule('/src/cad/rendering/section-presentation.ts');
 const {sectionViewGeometry}=await server.ssrLoadModule('/src/features/workbench/section-result-view.tsx');
 const {InteractionRouter}=await server.ssrLoadModule('/src/cad/interaction/interaction-router.ts');
 const {InputResult:R}=await server.ssrLoadModule('/src/cad/input/input-types.ts');
 const {sectionTargetPick}=await server.ssrLoadModule('/src/cad/assembly/section-positioning.ts');
 const path={rootDocumentId:'product',canonical:'instance',segments:[{instanceId:'instance',ownerDocumentId:'product',ownerVersionId:'revision',referencedDocumentId:'part',resolvedVersionId:'part-revision'}]};
 const view={document:{id:'product'},resolvedInstances:[{occurrencePath:'instance',instancePath:path,geometryKey:'solid',documentId:'part'}]},pick={kind:'face',instanceId:'instance',instancePath:path,documentId:'part',versionId:'part-revision',geometryKey:'solid',topologyId:1};
 assert(sectionTargetPick(view,pick));assert(!sectionTargetPick(view,{...pick,versionId:'new-head'}));assert(!sectionTargetPick(view,{...pick,instancePath:{...path,segments:[{...path.segments[0],ownerVersionId:'old'}]}}));assert(!sectionTargetPick({...view,resolvedInstances:[{...view.resolvedInstances[0],displayFallback:{}}]},pick));
 const d=defaultSpatial('SECTION');d.size=[24,20,4];const frozen=JSON.stringify(d);
 const rotated={...d,frame:{translation:[3,4,5],rotation:new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0,0,1),Math.PI/2).toArray()}};
 const resized=resizeSection(rotated,0,1,6);assert.equal(resized.size[0],30);assert.deepEqual(resized.frame.translation.map(v=>Math.round(v)),[3,7,5]);
 const opposite=x=>new THREE.Vector3(-x.size[0]/2,0,0).applyQuaternion(new THREE.Quaternion().fromArray(x.frame.rotation)).add(new THREE.Vector3().fromArray(x.frame.translation));assert(opposite(rotated).distanceTo(opposite(resized))<1e-10);assert(resizeSection(d,0,1,-100).size[0]>0);
 const camera=new THREE.OrthographicCamera(-35,35,28,-28,.1,200);camera.position.set(35,-40,50);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
 const surface={clientWidth:1000,clientHeight:800,getBoundingClientRect:()=>({width:1000,height:800})};
 const phases=[];const shaders={createMaterial:(_id,uniforms)=>new THREE.ShaderMaterial({uniforms:{uActive:{value:0},...Object.fromEntries(Object.entries(uniforms).map(([k,v])=>[k,{value:v}]))}})};
 const m=new SectionManipulator(shaders,()=>camera,surface,()=>{},()=>{});
 m.setSession({documentId:'product',revisionId:'revision',definition:d,enabled:true,onChange:(_d,p)=>phases.push(p)});m.updateScale({cssWidth:1000,cssHeight:800,devicePixelRatio:1});m.root.updateMatrixWorld(true);
 const screen=v=>{const p=v.clone().project(camera);return {x:(p.x+1)*500,y:(1-p.y)*400}};
 const event=(v,button=0,buttons={left:true,middle:false,right:false},shift=false)=>({...screen(v),pointerId:1,button,state:{buttons,modifiers:{shift,ctrl:false,alt:false,meta:false},keys:new Set()},phase:'move',originalEvent:{}});
 const grab=new THREE.Vector3(-7,-6,0),end=grab.clone().add(new THREE.Vector3(0,0,3));assert.equal(m.pointerDown(event(grab)),R.Capture);m.pointerMove(event(end));assert(Math.abs(m.definition().frame.translation[2]-3)<1e-6);m.pointerUp(event(end,0,{left:false,middle:false,right:false}));assert(!m.isDragging());assert.equal(JSON.stringify(d),frozen,'draft manipulation never mutates its accepted input');
 // Esc restores the whole entry draft, including a W -> UV chord transition.
 m.root.updateMatrixWorld(true);const start=new THREE.Vector3(-7,-6,3);assert.equal(m.pointerDown(event(start)),R.Capture);m.pointerMove(event(start.clone().add(new THREE.Vector3(0,0,2))));m.pointerMove(event(start.clone().add(new THREE.Vector3(0,0,2)),1,{left:true,middle:true,right:false}));m.pointerMove(event(start.clone().add(new THREE.Vector3(4,2,2)),0,{left:true,middle:true,right:false}));assert(m.cancelGesture());assert(Math.abs(m.definition().frame.translation[2]-3)<1e-6);assert(Math.abs(m.definition().frame.translation[0])<1e-6);
 // Rotation uses the reusable gizmo's world delta once, and the next gesture
 // starts in the newly accepted local frame.
 m.root.updateMatrixWorld(true);const handle=m.gizmo.handles.find(h=>h.operation==='rotate'&&h.axis==='Z'),p=handle.pick.getWorldPosition(new THREE.Vector3()),center=new THREE.Vector3().fromArray(m.definition().frame.translation),angle=.3;
 assert.equal(m.pointerDown(event(p)),R.Capture);const target=p.clone().sub(center).applyAxisAngle(new THREE.Vector3(0,0,1),angle).add(center);m.pointerMove(event(target));m.pointerUp(event(target,0,{left:false,middle:false,right:false}));const q=new THREE.Quaternion().fromArray(m.definition().frame.rotation);assert(1-Math.abs(q.dot(new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0,0,1),angle)))<1e-6);assert(new THREE.Vector3().fromArray(m.definition().frame.translation).distanceTo(new THREE.Vector3(0,0,3))<1e-9);
 let navigations=0;const ignored=()=>R.Ignored,stub={pointerDown:ignored,pointerMove:ignored,pointerUp:ignored,pointerCancel:ignored,keyDown:ignored,keyUp:ignored,cancel(){}};const navigation={...stub,wantsPointerPriority:e=>e.state.buttons.middle,pointerDown(){navigations++;return R.Capture},keyChanged:ignored};const router=new InteractionRouter(stub,{...stub},navigation,m);
 m.setSession({documentId:'product',revisionId:'revision',definition:d,enabled:true,onChange:(_d,p)=>phases.push(p)});m.root.updateMatrixWorld(true);
 assert.equal(router.pointerDown(event(grab)),R.Capture);router.pointerDown(event(grab,1,{left:true,middle:true,right:false}));assert.equal(navigations,0,'Left-led section chord retains command ownership');router.pointerMove(event(grab.clone().add(new THREE.Vector3(2,3,0)),0,{left:true,middle:true,right:false}));router.pointerUp(event(grab,1,{left:true,middle:false,right:false}));assert(m.isDragging());router.pointerCancel(event(grab));assert(!m.isDragging());assert.deepEqual(m.definition().frame.translation,d.frame.translation);
 router.pointerDown(event(grab,1,{left:false,middle:true,right:false}));assert.equal(navigations,1,'Middle-led navigation keeps its existing priority');router.cancel();
 const inside={...d,sectionType:'BOX',cutMode:'KEEP_INSIDE'};assert(sectionClippingPlanes(inside).every(p=>p.distanceToPoint(new THREE.Vector3())>0));assert(sectionClippingPlanes({...inside,cutMode:'KEEP_OUTSIDE'}).every(p=>p.distanceToPoint(new THREE.Vector3())<0));
 const report={definition:{...d,frame:{translation:[1,2,3],rotation:[0,0,0,1]}},geometry:{bodies:[{id:'a',complete:true,segments:[[1,2,3],[3,2,3],[1,2,5],[9,2,5]],triangles:[[1,2,3],[3,2,3],[1,4,3]]}]}};const geometry=sectionViewGeometry(report,{axis:2,offset:0});assert.equal(geometry.bodies[0].lines,'M0,0L2,0');assert(geometry.bodies[0].fill.includes('L0,-2'));assert.equal(geometry.bounds.width,2,'a different slice must not enter the front section view');
 m.dispose();assert.equal(m.root.parent,null);assert(phases.includes('cancel'));console.log('section draft gestures, resize anchors, exact-frame projection, chord ownership and rollback PASS');
}finally{await server.close()}
