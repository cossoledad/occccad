import {viewportProbe,expectViewport} from "./camera-helpers";
import {expect,test} from "@playwright/test";
import {writeFile} from 'node:fs/promises';

test("tree collapse handle resizes, cancels, persists and remains keyboard accessible",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 const tree=page.getByRole("complementary",{name:"模型结构",exact:true});
 const handle=page.getByRole("button",{name:"收起模型结构",exact:true});
 const width=()=>tree.evaluate(e=>e.getBoundingClientRect().width);
 const canvasWidth=await page.locator("canvas").evaluate(e=>e.getBoundingClientRect().width);
 await expect(page.getByRole("separator",{name:"调整结构树宽度"})).toHaveCount(0);
 const start=await width();
 const down=async()=>{const b=(await handle.boundingBox())!;const x=b.x+b.width/2,y=b.y+b.height/2;await page.mouse.move(x,y);await page.mouse.down();return {x,y}};
 let point=await down();await page.mouse.move(point.x+100,point.y,{steps:8});await page.mouse.up();
 await expect(handle).toBeVisible();await expect.poll(width).toBe(start+100);
 expect(await page.locator("canvas").evaluate(e=>e.getBoundingClientRect().width)).toBe(canvasWidth);
 await expectViewport(viewportProbe(page,"viewport-selection")).toHaveAttribute("data-count","0");
 await page.reload();await expect.poll(width).toBe(start+100);
 for(const cancel of ["Escape","pointercancel","lostpointercapture","blur"]){
  point=await down();await page.mouse.move(point.x+60,point.y,{steps:5});await expect.poll(width).toBe(start+160);
  if(cancel==="Escape")await page.keyboard.press("Escape");
  else if(cancel==="blur")await page.evaluate(()=>window.dispatchEvent(new Event("blur")));
  else await handle.evaluate((element,type)=>{element.dispatchEvent(new PointerEvent(type,{pointerId:1,bubbles:true,isPrimary:true}))},cancel);
  await expect.poll(width).toBe(start+100);await page.mouse.up();await expect(handle).toBeVisible();
  await expect(page.locator(".workbench-layout")).not.toHaveAttribute("data-tree-resizing");
 }
 // A completed drag suppresses its own click; a following ordinary click still collapses.
 await handle.click();const restore=page.getByRole("button",{name:"展开模型结构",exact:true});await expect(restore).toBeFocused();
 await page.keyboard.press("Enter");await expect(handle).toBeFocused();
 await page.keyboard.press("ArrowLeft");await expect.poll(width).toBe(start+84);
 await page.keyboard.press("Home");await expect.poll(width).toBe(220);
 const maximum=Math.min(640,(await page.locator(".workbench-layout").boundingBox())!.width*.4);
 await page.keyboard.press("End");await expect.poll(width).toBe(maximum);
 await page.keyboard.press("Space");await expect(restore).toBeVisible();await restore.click();
 // Sub-threshold pointer jitter is a click, rather than an accidental resize.
 point=await down();await page.mouse.move(point.x+2,point.y+1);await page.mouse.up();await expect(restore).toBeVisible();await restore.click();
 await page.setViewportSize({width:1024,height:768});await expect.poll(width).toBe(250);
 point=await down();await page.mouse.move(point.x-1000,point.y,{steps:10});await page.mouse.up();await expect.poll(width).toBe(220);await expect(handle).toBeVisible();
 await page.screenshot({path:testInfo.outputPath("tree-shared-handle.png")});expect(errors).toEqual([]);
});

test("production HUD curves retain length in WebGL and SVG cursors have transparent borders",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 page.on("console",m=>{if(/THREE.WebGLProgram|Shader Error|VALIDATE_STATUS/.test(m.text()))errors.push(m.text())});
 await page.goto("/");
 const result=await page.evaluate(async()=>{
  // @ts-ignore Vite production source
  const {T,NavigationHUD,navigationCursor,SPHERE_CURSOR_SPAN:span}=await import("/browser/fixtures/navigation.ts");
  const host=document.createElement("div");host.style.cssText="position:fixed;inset:0;z-index:9000;background:#172f46;display:grid;grid-template-columns:repeat(3,400px);gap:12px;align-content:start;padding:12px;color:#edf4fa";document.body.append(host);
  const samples=[];
  for(const [name,x,y,below,light] of [["Centre / dark",200,200,false,false],["Near rim / light",335,325,false,true],["Near rim / below",335,325,true,false],
   ["Rim / light",392,200,false,true],["Outside / dark",800,800,false,false]] as const){
   const column=document.createElement("div");column.textContent=name;host.append(column);
   const renderer=new T.WebGLRenderer({antialias:true,preserveDrawingBuffer:true});renderer.setSize(400,400);renderer.setClearColor(light?0xeef1f4:0x172f46);renderer.autoClear=false;column.append(renderer.domElement);
   const camera=new T.PerspectiveCamera(35,1,.1,1000);camera.position.set(300,-300,below?-300:300);camera.up.set(0,0,1);camera.lookAt(0,0,0);camera.updateMatrixWorld(true);
   const hud=new NavigationHUD();hud.update({action:"orbit",catia:{showPivot:true,hudVisible:true,showRotationCircle:true,pointer:{x,y}}},camera,400,400,light);
   renderer.clear();hud.render(renderer);
   const gl=renderer.getContext(),pixels=new Uint8Array(400*400*4);gl.readPixels(0,0,400,400,gl.RGBA,gl.UNSIGNED_BYTE,pixels);
   let coloured=0;for(let i=0;i<pixels.length;i+=4)if(Math.abs(pixels[i]-pixels[0])+Math.abs(pixels[i+1]-pixels[1])+Math.abs(pixels[i+2]-pixels[2])>18)coloured++;
   const p=hud.marker.geometry.attributes.position.array,half=hud.marker.geometry.drawRange.count*3/2;
   const lengths=[0,half].map(offset=>{let length=0;for(let i=offset;i<offset+half;i+=6)length+=Math.hypot(p[i+3]-p[i],p[i+4]-p[i+1]);return length});
   const radii=Array.from({length:half/3},(_,i)=>Math.hypot(p[i*3],p[i*3+1]));
   const maxRadius=Math.max(...Array.from({length:half*2/3},(_,i)=>Math.hypot(p[i*3],p[i*3+1])));
   samples.push({name,coloured,lengths,maxRadius,tangentialRadiusSpread:Math.max(...radii)-Math.min(...radii)});hud.dispose();renderer.dispose();
  }
  const cursors=[],cursorStrip=document.createElement("div");cursorStrip.style.cssText="display:flex;flex-wrap:wrap;gap:8px;align-content:start;width:208px";host.append(cursorStrip);
  for(const light of [false,true])for(const action of ["orbit","pan","zoom","roll"]){
   const cursor=navigationCursor({action},light),url=/url\("([^"]+)"\)/.exec(cursor)![1];
   const img=new Image();img.src=url;await img.decode();
   const canvas=document.createElement("canvas");canvas.width=32;canvas.height=32;canvas.title=action;canvas.style.cssText="width:80px;height:80px;image-rendering:pixelated;background:"+(light?"#eee":"#172f46");cursorStrip.append(canvas);
   const context=canvas.getContext("2d")!;context.drawImage(img,0,0);const data=context.getImageData(0,0,32,32).data;
   let white=0,opaque=0,border=0;
   for(let y=0;y<32;y++)for(let x=0;x<32;x++){
    const i=(y*32+x)*4;if(data[i+3]>0){opaque++;if(x===0||y===0||x===31||y===31)border++;if(data[i]>240&&data[i+1]>240&&data[i+2]>240)white++}
   }
   cursors.push({light,action,size:[img.naturalWidth,img.naturalHeight],white,opaque,border});
  }
  return {samples,cursors,span};
 });
 for(const sample of result.samples){
  expect(sample.coloured).toBeGreaterThan(200);for(const length of sample.lengths)expect(Math.abs(length-result.span)).toBeLessThan(.01);
  expect(sample.maxRadius).toBeLessThan(192.0001);
  if(sample.name.startsWith("Rim")||sample.name.startsWith("Outside")){expect(sample.maxRadius).toBeGreaterThan(191.9999);expect(sample.tangentialRadiusSpread).toBeLessThan(.0001)}
 }
 for(const cursor of result.cursors){expect(cursor.size).toEqual([32,32]);expect(cursor.white).toBe(0);expect(cursor.border).toBe(0);expect(cursor.opaque).toBeGreaterThan(60)}
 const measurements=testInfo.outputPath('hud-cursor-measurements.json');await writeFile(measurements,JSON.stringify(result,null,2));
 await testInfo.attach("hud-cursor-measurements",{path:measurements,contentType:"application/json"});
 await page.screenshot({path:testInfo.outputPath("hud-and-cursors.png")});expect(errors).toEqual([]);
});

for(const dpr of [1,2])test.describe(`2K navigation HUD at DPR ${dpr}`,()=>{
 test.use({viewport:{width:2560,height:1440},deviceScaleFactor:dpr});
 test('direction marker and sphere cross retain their CSS pixel size',async({page},info)=>{
  await page.goto('/');
  const result=await page.evaluate(async()=>{
   // @ts-ignore Vite production fixture
   const {T,NavigationHUD,CATIA_PIVOT_SPAN:pivotSpan,SPHERE_CURSOR_SPAN:guideSpan}=await import('/browser/fixtures/navigation.ts');
   const width=innerWidth,height=innerHeight,dpr=devicePixelRatio;
   const host=document.createElement('div');host.style.cssText='position:fixed;inset:0;z-index:9000;background:#eef1f4';document.body.append(host);
   const renderer=new T.WebGLRenderer({antialias:true,preserveDrawingBuffer:true});renderer.setPixelRatio(dpr);renderer.setSize(width,height);renderer.setClearColor(0xeef1f4);renderer.autoClear=false;host.append(renderer.domElement);
   const camera=new T.PerspectiveCamera(35,width/height,.1,1000);camera.position.z=100;camera.updateMatrixWorld(true);
   const hud=new NavigationHUD(),bounds=[];
   for(const sphere of [false,true]){
    hud.update({action:'orbit',catia:{showPivot:true,hudVisible:sphere,showRotationCircle:sphere,pointer:{x:width/2,y:height/2}}},camera,width,height,true);
    renderer.clear();hud.render(renderer);
    const side=120*dpr,pixels=new Uint8Array(side*side*4),gl=renderer.getContext();gl.readPixels((width/2-60)*dpr,(height/2-60)*dpr,side,side,gl.RGBA,gl.UNSIGNED_BYTE,pixels);
    let minX=side,minY=side,maxX=-1,maxY=-1;
    for(let y=0;y<side;y++)for(let x=0;x<side;x++){
     const offset=(y*side+x)*4;
     if(Math.abs(pixels[offset]-pixels[0])+Math.abs(pixels[offset+1]-pixels[1])+Math.abs(pixels[offset+2]-pixels[2])>18){minX=Math.min(minX,x);minY=Math.min(minY,y);maxX=Math.max(maxX,x);maxY=Math.max(maxY,y)}
    }
    bounds.push({sphere,width:(maxX-minX+1)/dpr,height:(maxY-minY+1)/dpr});
   }
   const label=document.createElement('div');label.style.cssText='position:absolute;left:24px;top:24px;color:#2d3b48;font:18px sans-serif';label.textContent=`${width} × ${height} CSS px · DPR ${dpr} · centre ${pivotSpan}px · guide ${guideSpan}px`;host.append(label);
   return {viewport:[width,height],dpr,pivotSpan,guideSpan,bounds};
  });
  expect(result.viewport).toEqual([2560,1440]);expect(result.dpr).toBe(dpr);
  expect(Math.abs(result.bounds[0].width-result.pivotSpan)).toBeLessThanOrEqual(2);
  expect(Math.abs(result.bounds[0].height-result.pivotSpan*17/22)).toBeLessThanOrEqual(2);
  expect(Math.abs(result.bounds[1].width-result.guideSpan)).toBeLessThanOrEqual(2);expect(Math.abs(result.bounds[1].height-result.guideSpan)).toBeLessThanOrEqual(2);
  await writeFile(info.outputPath('2k-pixel-measurements.json'),JSON.stringify(result,null,2));
  await page.screenshot({path:info.outputPath('2k-sphere-and-cross.png')});
 });
});
