import {viewportProbe,expectViewport} from "./camera-helpers";
import {expect,test,type Page} from "@playwright/test";
import {setProjection,observeViewport} from "./camera-helpers";

async function observe(page:Page){
 await observeViewport(page);await page.evaluate(()=>{
  const e=(window as any).__cameraEngine,samples:any[]=[];(window as any).__viewSamples=samples;
  (window as any).__viewRestore?.();const render=e.renderer.render.bind(e.renderer);
  e.renderer.render=(scene:any,camera:any)=>{if(scene===e.scene)samples.push({state:{position:camera.position.toArray(),quaternion:camera.quaternion.toArray(),zoom:camera.zoom},active:e.viewTransition.active});return render(scene,camera)};
  (window as any).__viewRestore=()=>{e.renderer.render=render};
 });
}

for(const parallel of [false,true])test(`Fit and standard views animate and interrupt (${parallel?"parallel":"perspective"})`,async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 if(parallel)await setProjection(page,"平行");
 const camera=viewportProbe(page,"navigation-camera"),canvas=page.locator("canvas");
 await page.getByRole("tab",{name:"视图",exact:true}).click();
 const deck=page.locator(".workbench-command-deck");
 const moveAway=async()=>{const b=(await canvas.boundingBox())!;await page.mouse.move(b.x+b.width*.6,b.y+b.height*.4);await expect(page.getByRole("tooltip")).toHaveCount(0)};
 const invoke=async(name:string)=>{await moveAway();await deck.getByRole("button",{name,exact:true}).click();await moveAway()};
 for(const name of ["前视图","右视图","顶视图","等轴测"]){
  await observe(page);await invoke(name);
  await expect.poll(()=>page.evaluate(()=>(window as any).__viewSamples.some((s:any)=>s.active))).toBe(true);
  await expectViewport(camera).toHaveAttribute("data-transition-active","false");
  const samples=await page.evaluate(()=>(window as any).__viewSamples);
  expect(new Set(samples.map((s:any)=>JSON.stringify(s.state))).size).toBeGreaterThan(2);
 }
 const box=(await canvas.boundingBox())!;await page.mouse.move(box.x+box.width*.6,box.y+box.height*.4);await page.mouse.wheel(0,-700);
 await observe(page);await invoke("适合窗口");
 await expect.poll(()=>page.evaluate(()=>(window as any).__viewSamples.some((s:any)=>s.active))).toBe(true);
 await expectViewport(camera).toHaveAttribute("data-transition-active","false");
 const fit=await page.evaluate(()=>(window as any).__viewSamples);expect(new Set(fit.map((s:any)=>JSON.stringify(s.state))).size).toBeGreaterThan(2);
 await moveAway();await deck.getByRole("button",{name:"前视图",exact:true}).click();await expectViewport(camera).toHaveAttribute("data-transition-active","true");
 await canvas.focus();await page.keyboard.press("Escape");await expectViewport(camera).toHaveAttribute("data-transition-active","false");
 const cancelled=await camera.getAttribute("data-camera-state");
 await page.waitForTimeout(350);const later=await camera.getAttribute("data-camera-state");
 expect(later).toBe(cancelled);
 await page.emulateMedia({reducedMotion:"reduce"});await observe(page);await invoke("右视图");
 await expectViewport(camera).toHaveAttribute("data-transition-active","false");
 expect(await page.evaluate(()=>(window as any).__viewSamples.some((s:any)=>s.active))).toBe(false);
 await testInfo.attach("fit-animation-frames",{body:JSON.stringify(fit,null,2),contentType:"application/json"});
 await page.screenshot({path:testInfo.outputPath("view-command-complete.png")});expect(errors).toEqual([]);
});

test("oblique opaque solids hide their back edges with a useful near plane",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 page.on("console",m=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 await page.goto("/");
 const samples=await page.evaluate(async()=>{
  // @ts-ignore Vite production modules
  const T=await import("/node_modules/.vite/deps/three.js");
  // @ts-ignore Vite production modules
  const {CadMaterialFactory}=await import("/src/cad/rendering/cad-material-factory.ts");
  // @ts-ignore Vite production modules
  const {CadShaderLibrary}=await import("/src/cad/rendering/shader/cad-shader-library.ts");
  // @ts-ignore Vite production modules
  const {updatePerspectiveClipping}=await import("/src/cad/navigation/projection-camera.ts");
  // @ts-ignore Vite production modules
  const {NavigationPicker,markNavigationPickable}=await import("/src/cad/navigation/navigation-picker.ts");
  const factory=new CadMaterialFactory(new CadShaderLibrary()),scene=new T.Scene();scene.add(new T.AmbientLight(0xffffff,2));
  const material=factory.surface(0x7b9fca),box=new T.Mesh(new T.BoxGeometry(20,20,20),material);scene.add(box);markNavigationPickable(box);
  const vertices:number[]=[];for(let i=-8;i<=8;i++)vertices.push(-8,i,-9,8,i,-9,i,-8,-9,i,8,-9);
  const lines=new T.LineSegments(new T.BufferGeometry().setAttribute("position",new T.Float32BufferAttribute(vertices,3)),factory.edge(0));lines.renderOrder=1;scene.add(lines);scene.updateMatrixWorld(true);
  const bounds=new T.Box3().setFromObject(box),samples=[];
  const host=document.createElement("div");host.style.cssText="position:fixed;inset:0;z-index:9000;background:#172f46;display:flex;gap:8px;padding:8px;color:white";document.body.append(host);
  for(const [name,x,y,z,forceNear] of [["axial",0,0,13,false],["oblique",3,-3,13,false],["grazing envelope",3,-3,(10+Math.sqrt(268))/2+1e-8,false],["tiny near control",3,-3,13,true]] as const){
   const column=document.createElement("div");column.textContent=name;host.append(column);
   const renderer=new T.WebGLRenderer({antialias:false,preserveDrawingBuffer:true});renderer.setSize(320,320);renderer.setClearColor(0x172f46);column.append(renderer.domElement);
   const camera=new T.PerspectiveCamera(35,1,.1,1000);camera.position.set(x,y,z);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
   const picker=new NavigationPicker(camera,renderer.domElement,()=>[box]);
   updatePerspectiveClipping(camera,new T.Vector3(),bounds,()=>{
    const hit=picker.pickSurface(160,160);return hit?.point.clone().sub(camera.position).dot(camera.getWorldDirection(new T.Vector3()));
   });
   if(forceNear){camera.near=1e-7;camera.updateProjectionMatrix()}
   renderer.render(scene,camera);
   const gl=renderer.getContext(),pixels=new Uint8Array(320*320*4);gl.readPixels(0,0,320,320,gl.RGBA,gl.UNSIGNED_BYTE,pixels);
   let dark=0,shaded=0;for(let i=0;i<pixels.length;i+=4){if(Math.max(pixels[i],pixels[i+1],pixels[i+2])<40)dark++;if(pixels[i+2]>100)shaded++}
   samples.push({name,near:camera.near,far:camera.far,dark,shaded,wireframe:material.wireframe});renderer.dispose();
  }
  return samples;
 });
 for(const sample of samples){expect(sample.wireframe).toBe(false);expect(sample.shaded).toBeGreaterThan(80000);if(sample.name==="tiny near control")expect(sample.dark).toBeGreaterThan(1000);else expect(sample.dark).toBe(0)}
 await testInfo.attach("oblique-depth",{body:JSON.stringify(samples,null,2),contentType:"application/json"});
 await page.screenshot({path:testInfo.outputPath("oblique-depth-comparison.png")});expect(errors).toEqual([]);
});
