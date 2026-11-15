import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try{
 const T=await server.ssrLoadModule("three");
 const {selectionOrbitTarget}=await server.ssrLoadModule("/src/cad/navigation/navigation-orbit-target.ts");
 const {NavigationPicker,markNavigationPickable}=await server.ssrLoadModule("/src/cad/navigation/navigation-picker.ts");
 const {NavigationController}=await server.ssrLoadModule("/src/cad/navigation/navigation-controller.ts");
 const data={vertices:[[0,0,0],[4,0,0],[0,2,0],[100,100,100],[110,100,100],[100,110,100]],triangles:[[0,1,2],[3,4,5]],faceIds:[7,8],edges:[{localId:9,points:[[1,2,3],[4,8,5],[2,3,1]]}],topologyVertices:[{localId:12,point:[2,3,4]}]};
 const matrix=new T.Matrix4().compose(new T.Vector3(5,-8,11),new T.Quaternion().setFromAxisAngle(new T.Vector3(1,2,3).normalize(),.6),new T.Vector3(-2,.7,3));
 const expected=points=>new T.Box3().setFromPoints(points.map(p=>new T.Vector3(...p).applyMatrix4(matrix))).getCenter(new T.Vector3());
 const close=(actual,want)=>assert.ok(actual.distanceTo(want)<1e-9,`${actual.toArray()} != ${want.toArray()}`);
 for(const [kind,id,points] of [["face",7,data.vertices.slice(0,3)],["edge",9,data.edges[0].points],["vertex",12,[[2,3,4]]]]){
  const target=selectionOrbitTarget({kind,topologyId:id},[],{data,matrixWorld:matrix});
  assert.equal(target.kind,kind==='vertex'?'point':'bounds');close(target.point,expected(points));
 }
 assert.equal(selectionOrbitTarget({kind:'face',topologyId:999},[],{data,matrixWorld:matrix}),undefined);
 const anchor=new T.Vector3(17,18,19);close(selectionOrbitTarget({kind:'sketch'},[],undefined,anchor).point,anchor);
 const root=new T.Group(),geometry=new T.BoxGeometry(2,4,6),mesh=new T.Mesh(geometry,new T.MeshBasicMaterial());root.add(mesh);root.applyMatrix4(matrix);root.updateMatrixWorld(true);
 close(selectionOrbitTarget({kind:'body'},[root]).point,new T.Vector3().applyMatrix4(root.matrixWorld));
 mesh.visible=false;assert.equal(selectionOrbitTarget({kind:'body'},[root]),undefined);mesh.visible=true;
 const camera=new T.OrthographicCamera(-20,20,15,-15,.1,1000);camera.position.set(0,0,100);camera.updateMatrixWorld(true);
 const hovered=new T.Vector3(0,0,0),picker=new NavigationPicker(camera,{clientWidth:800,clientHeight:600},()=>[],()=>hovered);
 assert.equal(picker.pickOrbit(400,300).orbitTarget.kind,'point','promoted points retain their kind without a render object');
 close(picker.pickOrbit(400,300).orbitTarget.point,hovered);
 markNavigationPickable(mesh);root.position.set(3,2,0);root.quaternion.identity();root.scale.set(1,1,1);root.updateMatrixWorld(true);
 const raw=new NavigationPicker(camera,{clientWidth:800,clientHeight:600},()=>[root]);
 close(raw.pickOrbit(460,260).orbitTarget.point,new T.Vector3(3,2,0));
 const input=(phase,mask,x=400,y=300,dx=0,dy=0)=>({phase,button:phase==='move'?-1:2,pointerId:1,x,y,deltaX:dx,deltaY:dy,state:{buttons:{left:false,right:!!(mask&2),middle:!!(mask&4)},modifiers:{ctrl:false,alt:false,shift:false,meta:false}}});
 for(const projection of ['orthographic','perspective']){
  const c=projection==='orthographic'?new T.OrthographicCamera(-20,20,15,-15,.1,1000):new T.PerspectiveCamera(35,4/3,.1,1000);c.position.set(0,0,100);
  let hit,queries=0;const picks={pickOrbit:()=>{queries++;return hit},pickNearest:()=>({point:new T.Vector3(0,0,10)}),pickZoomBarrier:()=>undefined};
  const nav=new NavigationController(c,()=>({width:800,height:600}),()=>{},picks,()=>new T.Vector3(50,0,0));
  const point=new T.Vector3(3,4,5);hit={point,orbitTarget:{kind:'bounds',point}};
  nav.pointerDown(input('down',2));close(nav.target,point);const picked=queries;
  hit={point:new T.Vector3(100,100,100)};nav.pointerMove(input('move',2,410,310,10,10));assert.equal(queries,picked,'locked rotation never repicks');close(nav.target,point);
  nav.wheel({...input('move',2),deltaY:-40,originalEvent:{deltaMode:0}});close(nav.target,point);
  nav.pointerUp(input('up',0));
  hit=undefined;nav.pointerDown(input('down',2));close(nav.target,new T.Vector3(50,0,0));
  nav.pointerMove(input('move',2,430,320,20,10));const position=c.position.clone(),rotation=c.quaternion.clone();
  hit={point,orbitTarget:{kind:'point',point}};nav.pointerMove(input('move',2,440,330,10,10));
  close(nav.target,point);close(c.position,position);assert.ok(c.quaternion.angleTo(rotation)<1e-7,'acquisition does not apply an old delta around the new centre');
  const lockedQueries=queries;nav.pointerMove(input('move',2,450,340,10,10));assert.equal(queries,lockedQueries);close(nav.target,point);
  nav.cancel();hit={point:new T.Vector3(-3,-4,-5),orbitTarget:{kind:'bounds',point:new T.Vector3(-3,-4,-5)}};nav.pointerDown(input('down',2));close(nav.target,hit.point);nav.cancel();
  // A valid element wins even when the whole model does not fit the viewport.
  const detail=new NavigationController(c,()=>({width:800,height:600}),()=>{},picks,()=>new T.Vector3(),"default",false,()=>new T.Box3(new T.Vector3(-1e4,-1e4,-1e4),new T.Vector3(1e4,1e4,1e4)));
  detail.pointerDown(input('down',2));close(detail.target,hit.point);detail.cancel();
  hit=undefined;detail.pointerDown(input('down',2));close(detail.target,new T.Vector3(0,0,10));detail.cancel();
 }
 geometry.dispose();mesh.material.dispose();
}finally{await server.close()}
