import {expect,test} from "@playwright/test";

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
