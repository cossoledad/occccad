import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule('three');
 const {NavigationController}=await server.ssrLoadModule('/src/cad/navigation/navigation-controller.ts');
 const {VirtualTrackball}=await server.ssrLoadModule('/src/cad/navigation/virtual-trackball.ts');
 const {ThreeCameraRig}=await server.ssrLoadModule('/src/cad/navigation/camera-rig.ts');
 const input=(phase,button,mask,x,y,ctrl=false)=>({phase,button,pointerId:1,x,y,deltaX:999,deltaY:999,state:{buttons:{left:!!(mask&1),right:!!(mask&2),middle:!!(mask&4)},modifiers:{ctrl,alt:false,shift:false,meta:false}}});
 const setup=height=>{
  const camera=new T.OrthographicCamera(-20,20,15,-15,.1,1000);camera.position.set(0,0,100);
  const size={width:height*4/3,height};
  const nav=new NavigationController(camera,()=>({...size}),()=>{},{pickNearest:()=>undefined},()=>new T.Vector3(),'catia');
  return {camera,nav,size};
 };
 const zooms=[];
 for(const height of [300,600,1200]){
  const {nav,camera,size}=setup(height),x=size.width/2,y=height/2;
  nav.pointerDown(input('down',1,4,x,y,true));nav.pointerMove(input('move',-1,4,x,y-height/6,true));zooms.push(camera.zoom);nav.cancel();
 }
 assert.ok(Math.max(...zooms)-Math.min(...zooms)<1e-12);assert.ok(Math.abs(zooms[0]-2)<1e-12,'one sixth viewport height doubles orthographic scale');
 const {nav,camera,size}=setup(600),origin=new T.Vector3(),before=origin.clone().project(camera);
 nav.pointerDown(input('down',1,4,400,300));nav.pointerMove(input('move',-1,4,420,290));
 const after=origin.clone().project(camera);assert.ok(Math.abs((after.x-before.x)*400-20)<1e-9&&Math.abs((before.y-after.y)*300+10)<1e-9,'pan follows the pointer 1:1');
 nav.pointerDown(input('down',0,5,420,290));nav.pointerMove(input('move',-1,5,470,325));
 const rotation=camera.quaternion.clone(),zoom=camera.zoom,position=camera.position.clone();
 // The release coordinates can differ from the last move. They are the next mode's baseline.
 nav.pointerUp(input('up',0,4,480,335));nav.pointerMove(input('move',-1,4,480,335));
 assert.equal(camera.zoom,zoom);assert.deepEqual(camera.position,position);assert.ok(camera.quaternion.angleTo(rotation)<1e-7);
 nav.pointerMove(input('move',-1,4,480,305));assert.ok(camera.zoom>zoom);
 nav.pointerDown(input('down',2,6,480,305));const rotated=camera.quaternion.clone();nav.pointerMove(input('move',-1,6,480,305));assert.ok(camera.quaternion.angleTo(rotated)<1e-7);
 size.height=700;size.width=900;nav.pointerMove(input('move',-1,6,500,330));assert.ok(camera.quaternion.angleTo(rotated)<1e-7,'viewport resize rebases the gesture');
 nav.pointerUp(input('up',1,2,500,330));assert.equal(nav.activeAction,'none');nav.cancel();
 const exercise=(points)=>{
  const camera=new T.PerspectiveCamera(35,4/3,.1,1000);camera.position.set(0,0,100);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
  const rig=new ThreeCameraRig(camera),ball=new VirtualTrackball();ball.begin(...points[0],800,600);
  let maximumStep=0;
  for(const [x,y] of points.slice(1)){const rotation=ball.drag(x,y,800,600,camera);if(rotation){maximumStep=Math.max(maximumStep,rotation.angleTo(new T.Quaternion()));rig.orbitQuaternion(rotation)}}
  return {camera,maximumStep};
 };
 const coarse=exercise([[400,300],[600,300]]),fine=exercise(Array.from({length:101},(_,i)=>[400+i*2,300]));
 assert.ok(coarse.camera.quaternion.angleTo(fine.camera.quaternion)<1e-4,'radial drag converges below .006 degrees with coarse/fine sampling');
 const arc=steps=>Array.from({length:steps+1},(_,i)=>[400+200*Math.cos(i/steps*Math.PI/2),300+200*Math.sin(i/steps*Math.PI/2)]);
 const coarseArc=exercise(arc(32)),fineArc=exercise(arc(320));
 assert.ok(coarseArc.camera.quaternion.angleTo(fineArc.camera.quaternion)<.0035,'curved-path transport converges within .2 degrees at ~10px versus ~1px sampling');
 const around=exercise(Array.from({length:101},(_,i)=>[400+350*Math.cos(i/100*Math.PI*2),300+350*Math.sin(i/100*Math.PI*2)]));
 assert.ok(around.camera.getWorldDirection(new T.Vector3()).distanceTo(new T.Vector3(0,0,-1))<1e-9,'outside remains pure roll throughout the circle');
 assert.ok(around.camera.quaternion.angleTo(new T.Quaternion())<1e-7);
 const antipodal=exercise([[750,300],[50,300]]);assert.ok(Math.abs(antipodal.camera.quaternion.z)>.9999,'antipodal outside samples cannot invent an X/Y turn');
 const rim=exercise([[687.99,300],[688.01,300],[688.02,300.01],[687.99,300.01]]);assert.ok(rim.maximumStep<.001,'crossing the rim has a bounded continuous angle');
 const onSphere=exercise([[400,300],[688.01,300],[688.01,310],[688.01,320]]);assert.ok(onSphere.camera.quaternion.toArray().every(Number.isFinite));
 // Regression: a guide clamped at the rim must not clamp actual radial input.
 for(const [sx,sy] of [[1,0],[-1,0],[0,1],[0,-1],[Math.SQRT1_2,Math.SQRT1_2]]){
  const start=[400,300],at=d=>[400+sx*d,300+sy*d];
  const edge=exercise([start,at(288)]),beyond=exercise([start,at(288),at(328)]),further=exercise([start,at(288),at(328),at(368)]);
  assert.ok(edge.camera.quaternion.angleTo(beyond.camera.quaternion)>.1,'outward motion beyond the sphere continues rotating');
  assert.ok(beyond.camera.quaternion.angleTo(further.camera.quaternion)>.1,'the next outward increment also continues rotating');
  const back=exercise([start,at(288),at(328),at(368),at(328),at(288),start]);
  assert.ok(back.camera.quaternion.angleTo(new T.Quaternion())<1e-7,'reversing the radial path returns without accumulated roll');
 }
 const crossing=setup(600);crossing.nav.pointerDown(input('down',1,4,400,300));crossing.nav.pointerDown(input('down',0,5,400,300));
 crossing.nav.pointerMove(input('move',-1,5,700,300));assert.equal(crossing.nav.activeAction,'orbit','crossing the guide does not change a free rotation into roll');
 crossing.size.width=900;crossing.size.height=700;crossing.nav.pointerMove(input('move',-1,5,800,350));assert.equal(crossing.nav.activeAction,'orbit','resize preserves the gesture kind');crossing.nav.cancel();
}finally{await server.close()}
