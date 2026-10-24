import {viewportProbe,expectViewport} from "./camera-helpers";
import {worldScreen,setProjection} from "./camera-helpers";
import {expect,test} from "@playwright/test";

test("icon-only options, transparent tree, CATIA point/surface/empty pivots and a valid orbit cursor",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");const canvas=page.locator("canvas");await expect(canvas).toBeVisible();
 const options=page.getByRole("button",{name:"选项",exact:true});expect((await options.textContent())?.trim()).toBe("");
 const tree=page.getByRole("complementary",{name:"模型结构",exact:true});await expect(tree.locator(".workbench-panel-heading,.workbench-document-name")).toHaveCount(0);expect(await tree.evaluate(e=>getComputedStyle(e).backgroundColor)).toBe("rgba(0, 0, 0, 0)");
 const box=(await canvas.boundingBox())!,treeBox=(await tree.boundingBox())!;expect(box.x).toBe(treeBox.x);
 await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down({button:"right"});
 const cursor=await canvas.evaluate(e=>getComputedStyle(e).cursor);expect(cursor).toContain("16 16");
 const dimensions=await page.evaluate(cursor=>new Promise<number[]>((resolve,reject)=>{const url=/url\("([^"]+)"\)/.exec(cursor)?.[1];if(!url)return reject("Missing SVG cursor");const img=new Image();img.onload=()=>resolve([img.naturalWidth,img.naturalHeight]);img.onerror=()=>reject("Invalid SVG cursor");img.src=url}),cursor);
 expect(dimensions).toEqual([32,32]);await page.mouse.up({button:"right"});
 await page.keyboard.press("1");
 await options.click();const dialog=page.getByRole("dialog",{name:"选项",exact:true});await dialog.getByRole("menuitem",{name:"导航与捕获",exact:true}).click();await dialog.getByRole("combobox",{name:"鼠标操作模式"}).click();await page.getByText("3DEXPERIENCE CATIA",{exact:true}).click();await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).not.toBeVisible();
 const at=(point:number[])=>worldScreen(page,point);
 const vertex=await at([0,0,16]),pivot=viewportProbe(page,"navigation-pivot");
 const before=await pivot.textContent();await page.mouse.move(vertex.x,vertex.y);await page.mouse.down({button:"middle"});await expectViewport(pivot).toHaveAttribute("data-visible","true");expect(await pivot.textContent()).toBe(before);
 await page.screenshot({path:"../../../build/acceptance/catia-point-centre.png"});await page.mouse.up({button:"middle"});await expectViewport(pivot).toHaveAttribute("data-visible","false");await expectViewport(pivot).toContainText("0.000000,0.000000,16.000000");
 const face=await at([36,19,16]);await page.mouse.move(face.x,face.y);await page.mouse.down({button:"middle"});await page.mouse.up({button:"middle"});await expectViewport(pivot).toContainText("body:body-main");
 const centre=(await pivot.innerText()).split(" · ")[0];
 await page.mouse.move(box.x+box.width-20,box.y+100);await page.mouse.down({button:"middle"});await expectViewport(pivot).toContainText(centre);await expectViewport(pivot).toContainText("existing");await page.mouse.up({button:"middle"});
 await expectViewport(viewportProbe(page,"viewport-selection")).toHaveAttribute("data-count","0");expect(errors).toEqual([]);
});

test("world grid occludes from below and glass distinguishes front/back thin and wide edges",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));page.on("console",m=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 await page.goto("/");
 const result=await page.evaluate(async()=>{
  // @ts-ignore Vite source modules, exercising the production materials in WebGL.
  const T=await import("/node_modules/.vite/deps/three.js");
  // @ts-ignore Vite source
  const {CadMaterialFactory}=await import("/src/cad/rendering/cad-material-factory.ts");
  // @ts-ignore Vite source
  const {CadShaderLibrary}=await import("/src/cad/rendering/shader/cad-shader-library.ts");
  // @ts-ignore Vite source
  const {PlaneGrid}=await import("/src/cad/rendering/plane-grid.ts");
  // @ts-ignore Vite source
  const {LineSegments2}=await import("/node_modules/.vite/deps/three_addons_lines_LineSegments2__js.js");
  // @ts-ignore Vite source
  const {LineSegmentsGeometry}=await import("/node_modules/.vite/deps/three_addons_lines_LineSegmentsGeometry__js.js");
  // @ts-ignore Vite source
  const {LineMaterial}=await import("/node_modules/.vite/deps/three_addons_lines_LineMaterial__js.js");
  const renderer=new T.WebGLRenderer({antialias:false,preserveDrawingBuffer:true});renderer.setSize(400,400);renderer.autoClear=false;
  renderer.domElement.style.cssText="position:fixed;left:0;top:0;z-index:9000";document.body.append(renderer.domElement);
  const factory=new CadMaterialFactory(new CadShaderLibrary()),scene=new T.Scene();let camera=new T.OrthographicCamera(-25,25,25,-25,.1,300);
  scene.background=new T.Color("#eeeeee");scene.add(new T.AmbientLight(0xffffff,2));
  const material=factory.surface(0x899baa),mesh=new T.Mesh(new T.BoxGeometry(20,20,20),material);scene.add(mesh);
  const grid=new PlaneGrid(0xffffff);grid.setStyle("#ffffff",1,10);scene.add(grid.object);
  const frame={origin:new T.Vector3(0,0,-15),normal:new T.Vector3(0,0,1),u:new T.Vector3(1,0,0),v:new T.Vector3(0,1,0)};
  const pixels=()=>{const gl=renderer.getContext(),data=new Uint8Array(400*400*4);gl.readPixels(0,0,400,400,gl.RGBA,gl.UNSIGNED_BYTE,data);return data};
  const lum=(data:Uint8Array,x:number,y:number)=>(data[(y*400+x)*4]+data[(y*400+x)*4+1]+data[(y*400+x)*4+2])/3;
  const render=(below:boolean,visible:boolean)=>{camera.position.set(0,0,below?-100:100);camera.up.set(0,1,0);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);grid.update(camera,400,frame);grid.object.visible=visible;renderer.clear();factory.surfaceDepth.render(renderer,scene,camera);renderer.render(scene,camera);return pixels()};
  const top=render(false,false),topGrid=render(false,true),bottom=render(true,false),bottomGrid=render(true,true);
  const ground={topDifference:Math.abs(lum(topGrid,200,200)-lum(top,200,200)),bottomDifference:lum(bottomGrid,200,200)-lum(bottom,200,200)};
  const parallelCamera=camera;camera=new T.PerspectiveCamera(35,1,.1,300);
  const pt=render(false,false),ptg=render(false,true),pb=render(true,false),pbg=render(true,true);
  const perspectiveGround={topDifference:Math.abs(lum(ptg,200,200)-lum(pt,200,200)),bottomDifference:lum(pbg,200,200)-lum(pb,200,200)};
  camera=parallelCamera;
  grid.object.visible=false;material.opacity=.35;material.transparent=true;material.depthWrite=false;material.needsUpdate=true;
  const positions=[-7,4,10,7,4,10,-7,-4,-10,7,-4,-10];
  const line=new T.LineSegments(new T.BufferGeometry().setAttribute("position",new T.Float32BufferAttribute(positions,3)),factory.edge(0));line.renderOrder=1;scene.add(line);
  const contrast=()=>{const data=render(false,false);const row=(y:number)=>Math.min(...[y-1,y,y+1].map(row=>Array.from({length:80},(_,i)=>lum(data,160+i,row)).reduce((a,b)=>a+b)/80));return {front:row(232),back:row(168)}};
  const thin=contrast();line.visible=false;
  const wideMaterial=new LineMaterial({color:0,linewidth:3,transparent:true,depthTest:true,depthWrite:false});wideMaterial.resolution.set(400,400);factory.sectionLine(wideMaterial);
  const wide=new LineSegments2(new LineSegmentsGeometry().setPositions(positions),wideMaterial);wide.renderOrder=1;scene.add(wide);const thick=contrast();
  renderer.domElement.setAttribute("data-render-fixture","glass-edges");
  // Keep the last rendered canvas for the screenshot; the page owns its lifetime.
  return {ground,perspectiveGround,thin,thick};
 });
 expect(result.ground.topDifference).toBeLessThan(1);expect(result.ground.bottomDifference).toBeGreaterThan(15);
 expect(result.perspectiveGround.topDifference).toBeLessThan(1);expect(result.perspectiveGround.bottomDifference).toBeGreaterThan(15);
 expect(result.thin.back-result.thin.front).toBeGreaterThan(30);expect(result.thick.back-result.thick.front).toBeGreaterThan(30);
 await page.screenshot({path:"../../../build/acceptance/viewport-glass-depth.png"});expect(errors).toEqual([]);
});

test("perspective is default, projection preview cancels and global choice survives reload",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");const projection=viewportProbe(page,"navigation-projection");await expectViewport(projection).toContainText("perspective");
 const origin=await worldScreen(page,[36,19,8]),landmark=await worldScreen(page,[36,19,16]);
 await setProjection(page,"平行（无透视）",false);await expectViewport(projection).toContainText("perspective");
 const restored=await worldScreen(page,[36,19,16]);expect(Math.hypot(restored.x-landmark.x,restored.y-landmark.y)).toBeLessThan(.05);
 await setProjection(page,"平行（无透视）");await expectViewport(projection).toContainText("orthographic");await page.reload();await expectViewport(projection).toContainText("orthographic");
 await setProjection(page,"透视（近大远小）");await expectViewport(projection).toContainText("perspective");
 const centre=await worldScreen(page,[36,19,8]);expect(Math.hypot(centre.x-origin.x,centre.y-origin.y)).toBeLessThan(1);
 expect(errors).toEqual([]);
});
