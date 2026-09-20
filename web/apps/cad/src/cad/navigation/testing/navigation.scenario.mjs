import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });
try {
  const THREE = await server.ssrLoadModule("three");
  const { NavigationController } = await server.ssrLoadModule("/src/cad/navigation/navigation-controller.ts");
  const { ThreeCameraRig } = await server.ssrLoadModule("/src/cad/navigation/camera-rig.ts");
  const { NavigationPicker, markNavigationPickable } = await server.ssrLoadModule("/src/cad/navigation/navigation-picker.ts");
  const { VirtualTrackball } = await server.ssrLoadModule("/src/cad/navigation/virtual-trackball.ts");
  const { circularRotationAxis, cylindricalRotationAxis } = await server.ssrLoadModule("/src/cad/navigation/display-rotation-axis.ts");
  const circleSamples = Array.from({length:32},(_,i)=>new THREE.Vector3(3+5*Math.cos(i/32*Math.PI*2),4+5*Math.sin(i/32*Math.PI*2),7));
  const circleAxis = circularRotationAxis(circleSamples);
  assert.ok(circleAxis.axisOrigin.distanceTo(new THREE.Vector3(3,4,7))<1e-7);
  assert.ok(Math.abs(circleAxis.axis.z)>.999);
  assert.equal(circularRotationAxis(circleSamples.map(p=>new THREE.Vector3(p.x*2,p.y,p.z))),undefined,"ellipses must not masquerade as circular axes");
  const cylinderSamples=circleSamples.flatMap(p=>[p,p.clone().add(new THREE.Vector3(0,0,20))]);
  const normals=circleSamples.map(p=>p.clone().sub(new THREE.Vector3(3,4,7)).normalize());
  const cylinderAxis=cylindricalRotationAxis(cylinderSamples,normals);
  assert.ok(Math.abs(cylinderAxis.axis.z)>.999);
  assert.ok(Math.hypot(cylinderAxis.axisOrigin.x-3,cylinderAxis.axisOrigin.y-4)<1e-7);
  assert.equal(cylindricalRotationAxis(cylinderSamples,normals.map(p=>p.clone().add(new THREE.Vector3(0,0,.2)).normalize())),undefined,"conical normals are not cylindrical");
  let time = 0, x = 400, y = 300;
  const event = (phase, button, mask, nx = x, ny = y, modifiers = {}) => {
    const result = { phase, button, pointerId: 1, pointerType: "mouse", x: nx, y: ny, deltaX: nx - x, deltaY: ny - y,
      state: { buttons: { left: !!(mask & 1), right: !!(mask & 2), middle: !!(mask & 4) }, modifiers: { ctrl: false, shift: false, alt: false, meta: false, ...modifiers }, keys: new Set(), pointer: {x: nx,y: ny,deltaX: nx-x,deltaY:ny-y} }, originalEvent: { timeStamp: time += 50 } };
    x = nx; y = ny; return result;
  };
  const camera = new THREE.PerspectiveCamera(45, 800 / 600, .1, 10000); camera.position.set(0, 0, 100); camera.updateProjectionMatrix();
  let hit = { point: new THREE.Vector3(10, 5, 0), axis: new THREE.Vector3(0, 0, 1), source: "raycast" };
  let fits = 0;
  const picker = { pickNearest: () => hit, pickViewPlane: () => ({ point: new THREE.Vector3(15, 10, 0), source: "view-plane" }) };
  const nav = new NavigationController(camera, () => ({ width: 800, height: 600 }), () => {}, picker, () => new THREE.Vector3(), "catia", false,
    () => new THREE.Box3(new THREE.Vector3(-10,-10,-10),new THREE.Vector3(10,10,10)), () => fits++);
  const q = camera.quaternion.clone();
  nav.pointerDown(event("down",1,4)); nav.pointerUp(event("up",1,0));
  assert.ok(nav.target.distanceTo(hit.point) < 1e-8, "CATIA click centers chosen geometry");
  assert.ok(Math.abs(hit.point.clone().project(camera).x) < 1e-8);
  assert.ok(q.angleTo(camera.quaternion) < 1e-8, "centering preserves orientation");
  nav.pointerDown(event("down",1,4));nav.pointerMove(event("move",-1,4,420,310));
  assert.equal(nav.activeAction,"pan"); assert.ok(q.angleTo(camera.quaternion)<1e-8);
  nav.pointerDown(event("down",2,6)); nav.pointerMove(event("move",-1,6,460,335));
  assert.equal(nav.activeAction,"orbit");assert.ok(q.angleTo(camera.quaternion)>.01);
  assert.equal(nav.snapshot.catia.hudVisible,false,"3DEXPERIENCE native defaults to hidden rotation sphere");
  nav.setCatiaRotationSphereVisible(true);
  assert.equal(nav.snapshot.catia.showRotationCircle,true);
  nav.setCatiaRotationSphereVisible(false);
  assert.equal(nav.snapshot.catia.showRotationCircle,false,"display preference must not end rotation");
  assert.equal(nav.activeAction,"orbit");
  const pivot = nav.target.clone();
  nav.pointerUp(event("up",2,4));const distance = camera.position.distanceTo(pivot);
  nav.pointerMove(event("move",-1,4,460,300));assert.equal(nav.activeAction,"zoom");assert.ok(camera.position.distanceTo(pivot)<distance);
  nav.pointerDown(event("down",0,5));assert.equal(nav.activeAction,"orbit");
  nav.pointerUp(event("up",1,1));assert.equal(nav.activeAction,"none");
  nav.pointerUp(event("up",0,0));
  nav.pointerDown(event("down",1,4,x,y,{ctrl:true}));nav.pointerMove(event("move",-1,4,x,y-20,{ctrl:true}));assert.equal(nav.activeAction,"zoom");nav.cancel();
  nav.pointerDown(event("down",2,2,x,y,{alt:true,ctrl:true}));nav.pointerMove(event("move",-1,2,x,y-20,{alt:true,ctrl:true}));assert.equal(nav.activeAction,"zoom");nav.cancel();
  nav.pointerDown(event("down",2,2,x,y,{alt:true}));nav.pointerDown(event("down",0,3,x,y,{alt:true}));assert.equal(nav.activeAction,"orbit");
  nav.pointerUp(event("up",0,2,x,y,{alt:true}));assert.equal(nav.activeAction,"zoom");nav.cancel();
  const wheel = { ...event("move",-1,0), deltaY: 100, originalEvent: { deltaMode: 0 } };
  const beforeWheel = camera.position.clone();assert.equal(nav.wheel(wheel),"ignored");assert.deepEqual(camera.position,beforeWheel);
  nav.setProfile("solidworks");
  const beforeSwWheel = camera.position.distanceTo(hit.point);
  nav.wheel(wheel); assert.ok(camera.position.distanceTo(hit.point)<beforeSwWheel,"SW wheel towards user zooms in");
  nav.pointerDown(event("down",1,4));nav.pointerMove(event("move",-1,4,x+20,y+10));assert.equal(nav.activeAction,"orbit");nav.pointerUp(event("up",1,0));
  for (const [modifier,action] of [["ctrl","pan"],["shift","zoom"],["alt","roll"]]) {
    nav.pointerDown(event("down",1,4,x,y,{[modifier]:true}));nav.pointerMove(event("move",-1,4,x+10,y+10,{[modifier]:true}));assert.equal(nav.activeAction,action);nav.pointerUp(event("up",1,0));
  }
  const position = camera.position.clone();
  nav.pointerDown(event("down",1,4));nav.pointerUp(event("up",1,0));
  assert.equal(nav.snapshot.solidworks.reference,hit);assert.deepEqual(camera.position,position,"SW middle click selects reference without recentering");
  nav.pointerDown(event("down",1,4));nav.pointerMove(event("move",-1,4,x+30,y+10));assert.ok(nav.target.distanceTo(hit.point)<1e-8);
  nav.pointerUp(event("up",1,0));assert.equal(nav.snapshot.solidworks.reference,undefined,"reference lasts one rotation");
  time+=500;
  for (let i=0;i<2;i++){nav.pointerDown(event("down",1,4));nav.pointerUp(event("up",1,0));}
  assert.equal(fits,1,"SW double middle click fits");
  nav.pointerDown(event("down",1,4));nav.setProfile("catia");assert.equal(nav.activeAction,"none");assert.equal(nav.snapshot.solidworks,undefined);
  // Off-centre pivot must not corrupt subsequent screen centering, including perspective depth.
  const rig=new ThreeCameraRig(camera);rig.setPivot(new THREE.Vector3(100,20,-30));rig.centerViewpointAt(new THREE.Vector3(2,3,4));
  const centered=new THREE.Vector3(2,3,4).project(camera);assert.ok(Math.hypot(centered.x,centered.y)<1e-8);
  // Trackball path is independent of pointer-event frequency (no per-event angle clamp).
  const c=new THREE.PerspectiveCamera();c.position.set(0,0,100);c.lookAt(0,0,0);
  const ball=new VirtualTrackball();ball.begin(400,300,800,600);const turn=ball.drag(600,300,800,600,c);assert.ok(2*Math.acos(turn.w)>.5);
  const outside=new VirtualTrackball();outside.begin(790,300,800,600);const roll=outside.drag(400,0,800,600,c);assert.ok(Math.abs(roll.x)<1e-8&&Math.abs(roll.y)<1e-8);
  // Real picker: face scope, face axis, occlusion and exact point coordinate.
  c.aspect=800/600;c.updateProjectionMatrix();c.updateMatrixWorld(true);
  const geo=new THREE.BoxGeometry(20,20,20);geo.userData.navigationFaceIds=Array.from({length:12},(_,i)=>Math.floor(i/2));
  const mesh=new THREE.Mesh(geo,new THREE.MeshBasicMaterial());markNavigationPickable(mesh);mesh.updateMatrixWorld(true);
  const actualPicker=new NavigationPicker(c,{clientWidth:800,clientHeight:600},()=>[mesh]);
  const face=actualPicker.pickNearest(400,300,true);assert.equal(face.highlight.kind,"face");assert.equal(face.highlight.positions.length,18);assert.ok(Math.abs(face.axis.z)>0.99);
  mesh.visible=false;assert.equal(actualPicker.pickNearest(400,300),undefined);geo.dispose();mesh.material.dispose();
  console.log("Navigation gesture, pivot, trackball and display-reference scenarios passed.");
} finally { await server.close(); }
