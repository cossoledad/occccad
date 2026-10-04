import {expect,test,type Page} from "@playwright/test";

async function login(page:Page){
 if(!process.env.OCCCCAD_ADMIN_PASSWORD)throw new Error("Requires running application and configured login");
 const response=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});
 expect(response.ok()).toBeTruthy();await page.goto("/");
}
async function openCommand(page:Page,name:string){
 await page.getByRole("button",{name:"搜索工具",exact:true}).click();
 await page.getByRole("textbox",{name:"搜索工具名称或用途"}).fill(name);
 await page.locator(".workbench-command-result").filter({hasText:name}).first().click();
}
async function clickWorld(page:Page,point:number[]){
 await expect(page.getByTestId("navigation-camera")).toBeVisible();
 const position=await page.evaluate(async(point)=>{
  // @ts-ignore Vite serves the native project dependency.
  const THREE=await import("/node_modules/.vite/deps/three.js");
  const text=document.querySelector('[data-testid="navigation-camera"]')!.textContent!;
  const [position,rotation,zoom]=text.split(" / ");
  const p=position.replace("Camera: ","").split(",").map(Number),q=rotation.split(",").map(Number);
  const local=new THREE.Vector3(...point).sub(new THREE.Vector3(...p)).applyQuaternion(new THREE.Quaternion(...q).invert());
  const box=document.querySelector(".cad-viewport-canvas canvas")!.getBoundingClientRect();
  const scale=box.height*Number(zoom.replace("zoom ",""))/300;
  return {x:box.x+box.width/2+local.x*scale,y:box.y+box.height/2-local.y*scale};
 },point);
 await page.mouse.click(position.x,position.y);
}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json();}
async function ready(page:Page,dialog:ReturnType<Page["getByRole"]>){
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();
 await expect(page.locator("[data-feature-preview]")).toHaveCount(1);
}

test("viewport Boolean tools toggle, auto preview, commit and reopen",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let view=await api.createDocument("PART","Viewport Boolean acceptance");const id=view.document.id;
  const rectangle=async(x:number,y:number,w:number,h:number,length:number)=>{
   view=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=view.part!.features.at(-1)!.id;
   view=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x,y},second:{x:x+w,y:y+h}}]});
   view=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"NEW_BODY",length});return view.part!.features.at(-1)!;
  };
  const base=await rectangle(0,0,20,20,10),tool=await rectangle(5,5,5,5,20);
  view=await api.command(id,{type:"SET_ACTIVE_BODY",bodyId:base.bodyId});
  return {id,version:view.document.versionId,base:base.bodyId!,tool:tool.bodyId!};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await openCommand(page,"布尔");const dialog=page.getByRole("dialog",{name:"布尔运算",exact:true});
 await expect(dialog.getByRole("combobox")).toHaveCount(0);
 await clickWorld(page,[7,7,20]);await expect(dialog.getByRole("button",{name:"工具实体：已选择 1 个实体"})).toBeVisible();
 await ready(page,dialog);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await clickWorld(page,[7,7,20]);await expect(dialog.getByRole("button",{name:"工具实体：已选择 0 个实体"})).toBeVisible();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await clickWorld(page,[7,7,20]);await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id);expect(committed.document.versionId).not.toBe(seed.version);
 expect(committed.part.bodies.find((b:{id:string})=>b.id===seed.tool).consumed).toBe(true);
 const body=committed.part.bodies.find((b:{id:string})=>b.id===seed.base);
 expect(committed.artifacts[body.geometryKey].volume).toBeCloseTo(3750,5);
 const feature=committed.part.features.at(-1);
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill(feature.name);
 await page.getByRole("treeitem").filter({hasText:feature.name}).first().dblclick();
 const edit=page.getByRole("dialog",{name:"编辑布尔",exact:true});await expect(edit).toBeVisible();await ready(page,edit);
 // Consumed definitions reappear temporarily at their saved input stage.
 await clickWorld(page,[7,7,20]);await expect(edit.getByRole("button",{name:"工具实体：已选择 0 个实体"})).toBeVisible();
 await expect(edit.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 expect((await current(page,seed.id)).document.versionId).toBe(committed.document.versionId);
 await clickWorld(page,[7,7,20]);await expect(edit.getByRole("button",{name:"工具实体：已选择 1 个实体"})).toBeVisible();await ready(page,edit);
 await edit.getByRole("button",{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);
 const edited=await current(page,seed.id);
 expect(edited.part.bodies.find((b:{id:string})=>b.id===seed.tool).consumed).toBe(true);
 expect(edited.artifacts[edited.part.bodies.find((b:{id:string})=>b.id===seed.base).geometryKey].volume).toBeCloseTo(3750,5);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await expect(page.getByRole("tree").getByLabel("状态 已用于布尔")).toBeVisible();expect(errors).toEqual([]);
});

test("preselected chamfer edge, automatic preview, input-stage edit and toggle",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Viewport chamfer acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"NEW_BODY",length:10});
  return {id,bodyId:v.part!.activeBodyId,version:v.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await clickWorld(page,[10,0,10]);await expect(page.getByTestId("viewport-selection")).toContainText('"edge"');
 await openCommand(page,"倒角");const dialog=page.getByRole("dialog",{name:"创建 倒角",exact:true});
 await expect(dialog.getByRole("button",{name:"边集：已选择 1 条边"})).toBeVisible();await ready(page,dialog);
 await page.screenshot({path:"/tmp/feature-ux-modeling.png"});
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 // A failed preview clears the candidate and cannot submit stale geometry.
 await dialog.getByRole("textbox").fill("1000");
 await expect(dialog.getByRole("alert")).toBeVisible();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("textbox").fill("1");await ready(page,dialog);
 await expect(dialog.getByRole("alert")).toHaveCount(0);
 await clickWorld(page,[0,-10,10]);await expect(dialog.getByRole("button",{name:"边集：已选择 2 条边"})).toBeVisible();await ready(page,dialog);
 await clickWorld(page,[0,-10,10]);await expect(dialog.getByRole("button",{name:"边集：已选择 1 条边"})).toBeVisible();await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id);const body=committed.part.bodies.find((b:{id:string})=>b.id===seed.bodyId);
 expect(committed.artifacts[body.geometryKey].volume).toBeCloseTo(3990,5);
 const feature=committed.part.features.at(-1);const filter=page.getByRole("textbox",{name:"筛选模型结构"});await filter.fill(feature.name);
 await page.getByRole("treeitem").filter({hasText:feature.name}).first().dblclick();
 const edit=page.getByRole("dialog",{name:"编辑 倒角",exact:true});await expect(edit).toBeVisible();
 await expect(edit.getByRole("button",{name:"边集：已选择 1 条边"})).toBeVisible();await ready(page,edit);
 await expect(page.getByTestId("viewport-feature-selection")).toHaveAttribute("data-overlays","1");
 await page.screenshot({path:"/tmp/feature-ux-chamfer-edit.png"});
 await clickWorld(page,[10,0,10]);await expect(edit.getByRole("button",{name:"边集：已选择 0 条边"})).toBeVisible();
 await clickWorld(page,[10,0,10]);await expect(edit.getByRole("button",{name:"边集：已选择 1 条边"})).toBeVisible();await ready(page,edit);
 await edit.getByRole("button",{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);
 const edited=await current(page,seed.id);expect(edited.part.features.at(-1).selections[0].sourceFeatureId).toBeTruthy();
 expect(edited.artifacts[edited.part.bodies.find((b:{id:string})=>b.id===seed.bodyId).geometryKey].volume).toBeCloseTo(3990,5);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("rectangle to circle loft previews directly from viewport sections",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Rectangle circle loft acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const bottom=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:bottom,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"Top",datumPlaneId:"datum-xy",origin:[0,0,30],normal:[0,0,1],uDirection:[1,0,0]});
  const plane=v.part!.datumPlanes.at(-1)!.id;
  v=await api.command(id,{type:"CREATE_SKETCH",datumPlaneId:plane,plane:"CUSTOM"});const top=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:top,operations:[{type:"ADD_ENTITY",entity:{id:crypto.randomUUID(),kind:"CIRCLE",role:"PROFILE",center:{x:0,y:0},radius:10}}]});
  return {id,version:v.document.versionId,bottom,top};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await openCommand(page,"放样");const dialog=page.getByRole("dialog",{name:"创建 放样",exact:true});
 await expect(dialog.getByRole("combobox")).toHaveCount(0);
 await clickWorld(page,[10,-5,0]);await expect(dialog.getByRole("button",{name:"截面：已选择 1 个草图"})).toBeVisible();
 await clickWorld(page,[-10,0,30]);await expect(dialog.getByRole("button",{name:"截面：已选择 2 个草图"})).toBeVisible();await ready(page,dialog);
 await page.screenshot({path:"/tmp/feature-ux-modeling.png"});
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id);const f=committed.part.features.at(-1);
 expect(f.type).toBe("LOFT");expect(f.sections.map((s:{sketchId:string})=>s.sketchId)).toEqual([seed.bottom,seed.top]);
 const a=committed.artifacts[committed.part.bodies.find((b:{id:string})=>b.id===f.bodyId).geometryKey];
 expect(a.topology.solids).toBe(1);expect(a.volume).toBeGreaterThan(30*Math.PI*100);expect(a.volume).toBeLessThan(12000);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("revolve picks an individual sketch line and previews parameter changes",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Viewport revolve axis acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id,axisId=crypto.randomUUID();
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:5,y:2},second:{x:10,y:10}},{type:"ADD_ENTITY",entity:{id:axisId,kind:"LINE",role:"CONSTRUCTION",start:{x:0,y:0},end:{x:0,y:20}}}]});
  return {id,version:v.document.versionId,sketchId,axisId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await openCommand(page,"旋转");const dialog=page.getByRole("dialog",{name:"创建 旋转",exact:true});
 await expect(dialog.getByRole("combobox")).toHaveCount(0);
 await clickWorld(page,[7.5,2,0]);await expect(page.getByTestId("viewport-feature-selection")).toHaveAttribute("data-role","axis");
 await clickWorld(page,[0,15,0]);await expect(dialog.getByRole("button",{name:"旋转轴：已选择 1 条轴线"})).toBeVisible();await ready(page,dialog);
 await page.screenshot({path:"/tmp/feature-ux-modeling.png"});
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("textbox",{name:"角度（deg）",exact:true}).fill("180");await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id),f=committed.part.features.at(-1);
 expect(f.type).toBe("REVOLVE");expect(f.axisEntityId).toBe(`SKETCH_LINE:${seed.sketchId}:${seed.axisId}`);
 expect(committed.artifacts[committed.part.bodies.find((b:{id:string})=>b.id===f.bodyId).geometryKey].volume).toBeCloseTo(300*Math.PI,5);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});
