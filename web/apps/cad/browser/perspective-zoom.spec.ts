import {expect,test} from "@playwright/test";

for(const profile of ["default","catia","solidworks"] as const)test(`${profile} perspective zoom eases towards a surface and can retreat`,async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 page.on("console",m=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 await page.goto("/");
 await page.evaluate(async(profile)=>{
  // @ts-ignore Vite production modules
  const T=await import("/node_modules/.vite/deps/three.js");
  // @ts-ignore Vite production modules
  const {NavigationController}=await import("/src/cad/navigation/navigation-controller.ts");
  // @ts-ignore Vite production modules
  const {NavigationPicker,markNavigationPickable}=await import("/src/cad/navigation/navigation-picker.ts");
  // @ts-ignore Vite production modules
  const {InputManager}=await import("/src/cad/input/input-manager.ts");
  // @ts-ignore Vite production modules
  const {updatePerspectiveClipping}=await import("/src/cad/navigation/projection-camera.ts");
  // @ts-ignore Vite production modules
  const {CadMaterialFactory}=await import("/src/cad/rendering/cad-material-factory.ts");
  // @ts-ignore Vite production modules
  const {CadShaderLibrary}=await import("/src/cad/rendering/shader/cad-shader-library.ts");
  const factory=new CadMaterialFactory(new CadShaderLibrary()),scene=new T.Scene();scene.add(new T.AmbientLight(0xffffff,2));
  const renderer=new T.WebGLRenderer({antialias:false,preserveDrawingBuffer:true});renderer.setSize(600,600);renderer.setClearColor(0x172f46);
  const canvas=renderer.domElement;canvas.style.cssText="position:fixed;left:300px;top:220px;z-index:9000";canvas.dataset.easingFixture="true";document.body.append(canvas);
  const camera=new T.PerspectiveCamera(35,1,.01,1000),origin=new T.Vector3(250,-180,75);
  camera.position.copy(origin).add(new T.Vector3(0,0,30));camera.updateMatrixWorld(true);
  const box=new T.Mesh(new T.BoxGeometry(20,20,20),factory.surface(0x558dcc));box.position.copy(origin);markNavigationPickable(box);scene.add(box);scene.updateMatrixWorld(true);
  const bounds=new T.Box3().setFromObject(box),picker=new NavigationPicker(camera,canvas,()=>[box]),samples:any[]=[];
  const margin=picker.pickZoomBarrier(camera.position,new T.Vector3(0,0,-1)).clearance;
  let nav:any;
  const render=()=>{
   updatePerspectiveClipping(camera,nav.target,bounds,()=>picker.pickSurface(300,300)?.point.clone().sub(camera.position).dot(camera.getWorldDirection(new T.Vector3())));
   renderer.render(scene,camera);const gl=renderer.getContext(),pixel=new Uint8Array(4);gl.readPixels(300,300,1,1,gl.RGBA,gl.UNSIGNED_BYTE,pixel);
   const sample={eye:camera.position.toArray(),pivot:nav.target.toArray(),gap:camera.position.z-origin.z-10,near:camera.near,pixel:Array.from(pixel),inside:bounds.containsPoint(camera.position),margin};
   samples.push(sample);canvas.dataset.state=JSON.stringify(sample);canvas.dataset.samples=JSON.stringify(samples);
  };
  nav=new NavigationController(camera,()=>({width:600,height:600}),render,picker,()=>origin.clone(),profile,false,()=>bounds);nav.target.copy(origin);nav.syncCamera();
  const input=new InputManager(canvas,{pointerDown:(e:any)=>nav.pointerDown(e),pointerMove:(e:any)=>nav.pointerMove(e),pointerUp:(e:any)=>nav.pointerUp(e),wheel:(e:any)=>nav.wheel(e)});
  window.addEventListener("pagehide",()=>{input.dispose();renderer.dispose()},{once:true});
 },profile);
 const canvas=page.locator('canvas[data-easing-fixture="true"]');
 const initial=JSON.parse((await canvas.getAttribute("data-state"))!);
 const modifier=profile==="catia"?"Control":"Shift";
 const zoom=async(inward:boolean)=>{
  if(profile==="default"){
   await page.mouse.move(600,520);await page.mouse.wheel(0,inward?-120:120);
  }else{
   await page.keyboard.down(modifier);await page.mouse.move(600,650);await page.mouse.down({button:"middle"});
   await page.mouse.move(600,inward?530:770);await page.mouse.up({button:"middle"});await page.keyboard.up(modifier);
  }
 };
 const steps=[initial];
 for(let i=0;i<32;i++){
  const previous=steps.at(-1);await zoom(true);
  await expect.poll(async()=>JSON.parse((await canvas.getAttribute("data-state"))!).gap).toBeLessThan(previous.gap);
  steps.push(JSON.parse((await canvas.getAttribute("data-state"))!));
 }
 for(const step of steps){expect(step.inside).toBe(false);expect(step.gap).toBeGreaterThanOrEqual(step.margin-1e-9);expect(step.gap).toBeGreaterThan(step.near);expect(step.pixel[2]).toBeGreaterThan(step.pixel[0]+40)}
 for(let i=2;i<steps.length;i++)expect(steps[i-1].gap-steps[i].gap).toBeLessThan(steps[i-2].gap-steps[i-1].gap);
 expect(steps.at(-1).gap-initial.margin).toBeLessThan(.001);
 if(profile!=="default")expect(steps.at(-1).pivot).toEqual([250,-180,75]);
 const close=steps.at(-1);await zoom(false);await expect.poll(async()=>JSON.parse((await canvas.getAttribute("data-state"))!).gap).toBeGreaterThan(close.gap);
 expect(JSON.parse((await canvas.getAttribute("data-state"))!).gap-close.gap).toBeLessThan(initial.margin);
 await testInfo.attach("surface-easing",{body:JSON.stringify({profile,steps,retreated:JSON.parse((await canvas.getAttribute("data-state"))!)},null,2),contentType:"application/json"});
 await page.screenshot({path:testInfo.outputPath(`surface-easing-${profile}.png`)});expect(errors).toEqual([]);
});

test("default wheel keeps a close solid exterior visible despite an occluded point",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 page.on("console",m=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 await page.goto("/");
 await page.evaluate(async()=>{
  // @ts-ignore Vite production modules
  const T=await import("/node_modules/.vite/deps/three.js");
  // @ts-ignore Vite production modules
  const {NavigationController}=await import("/src/cad/navigation/navigation-controller.ts");
  // @ts-ignore Vite production modules
  const {NavigationPicker,markNavigationPickable}=await import("/src/cad/navigation/navigation-picker.ts");
  // @ts-ignore Vite production modules
  const {InputManager}=await import("/src/cad/input/input-manager.ts");
  // @ts-ignore Vite production modules
  const {updatePerspectiveClipping}=await import("/src/cad/navigation/projection-camera.ts");
  const renderer=new T.WebGLRenderer({antialias:false,preserveDrawingBuffer:true});renderer.setSize(600,600);renderer.setClearColor(0x172f46);
  const canvas=renderer.domElement;canvas.style.cssText="position:fixed;left:300px;top:220px;z-index:9000";canvas.dataset.zoomFixture="true";document.body.append(canvas);
  const scene=new T.Scene(),camera=new T.PerspectiveCamera(35,1,1e-5,1000);camera.position.set(0,0,10.01);camera.updateMatrixWorld(true);
  const box=new T.Mesh(new T.BoxGeometry(20,20,20),new T.MeshBasicMaterial({color:0x558dcc}));markNavigationPickable(box);scene.add(box);
  const point=new T.Vector3(0,0,9.995),hidden=new T.Points(new T.BufferGeometry().setFromPoints([point]),new T.PointsMaterial({color:0xff0000,size:20}));markNavigationPickable(hidden);scene.add(hidden);scene.updateMatrixWorld(true);
  const bounds=new T.Box3().setFromObject(box),picker=new NavigationPicker(camera,canvas,()=>[box,hidden],()=>point),samples:unknown[]=[];
  let steps=0;
  const render=()=>{
   updatePerspectiveClipping(camera,nav.target,bounds);renderer.render(scene,camera);
   const gl=renderer.getContext(),pixel=new Uint8Array(4);gl.readPixels(300,300,1,1,gl.RGBA,gl.UNSIGNED_BYTE,pixel);
   const sample={steps,eyeZ:camera.position.z,near:camera.near,clearance:camera.position.z-10,inside:bounds.containsPoint(camera.position),pixel:Array.from(pixel)};
   samples.push(sample);canvas.dataset.state=JSON.stringify(sample);canvas.dataset.samples=JSON.stringify(samples);
  };
  const nav=new NavigationController(camera,()=>({width:600,height:600}),()=>{steps++;render()},picker,()=>new T.Vector3(),"default",false,()=>bounds);
  const input=new InputManager(canvas,{wheel:(event:any)=>nav.wheel(event)});render();
  (window as any).__closeSurface=()=>{camera.position.set(0,0,10.0001);camera.updateMatrixWorld(true);nav.target.set(0,0,0);render()};
  // Keep the production input owner and renderer alive until this page closes.
  window.addEventListener("pagehide",()=>{input.dispose();renderer.dispose()},{once:true});
 });
 const canvas=page.locator('canvas[data-zoom-fixture="true"]');await page.mouse.move(600,520);
 for(let i=1;i<=24;i++){
  await page.mouse.wheel(0,i===20?-1e6:-120);
  await expect.poll(async()=>JSON.parse((await canvas.getAttribute("data-state"))!).steps).toBe(i);
 }
 const samples=JSON.parse((await canvas.getAttribute("data-samples"))!);
 for(const sample of samples){expect(sample.inside).toBe(false);expect(sample.clearance).toBeGreaterThan(sample.near);expect(sample.pixel[2]).toBeGreaterThan(sample.pixel[0]+50)}
 const before=samples.at(-1).eyeZ;await page.mouse.wheel(0,120);
 await expect.poll(async()=>JSON.parse((await canvas.getAttribute("data-state"))!).eyeZ).toBeGreaterThan(before);
 await page.evaluate(()=>(window as any).__closeSurface());
 const close=JSON.parse((await canvas.getAttribute("data-state"))!);expect(close.near).toBeLessThan(close.clearance);expect(close.pixel[2]).toBeGreaterThan(close.pixel[0]+50);
 await testInfo.attach("close-surface-zoom",{body:JSON.stringify({samples,close},null,2),contentType:"application/json"});
 await page.screenshot({path:testInfo.outputPath("exterior-after-zoom.png")});expect(errors).toEqual([]);
});
