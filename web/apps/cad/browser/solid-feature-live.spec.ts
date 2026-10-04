import {expect,test,type Page} from "@playwright/test";

async function login(page:Page){
 if(!process.env.OCCCCAD_ADMIN_PASSWORD)throw new Error("Requires running application and configured login");
 const response=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});
 expect(response.ok()).toBeTruthy();await page.goto("/");
}
async function openCommand(page:Page,name:string){
 await page.getByRole("button",{name:"搜索工具",exact:true}).click();
 await page.getByRole("textbox",{name:"搜索工具名称或用途"}).fill(name);
 await page.locator(".workbench-command-result").filter({has:page.locator("strong",{hasText:new RegExp(`^${name}$`)})}).click();
}
async function worldScreen(page:Page,point:number[]){
 await expect(page.getByTestId("navigation-camera")).toBeVisible();
 return page.evaluate(async(point)=>{
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
}
async function clickWorld(page:Page,point:number[]){
 const position=await worldScreen(page,point);
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
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:5,y:2},second:{x:10,y:10}}]});
  const axisId=v.part!.features.at(-1)!.sketch!.entities.find(e=>e.kind==="LINE"&&e.start?.x===5&&e.end?.x===5)!.id;
  return {id,version:v.document.versionId,sketchId,axisId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 const canvas=await page.locator("canvas").boundingBox();
 const beforeZoom=await page.getByTestId("navigation-camera").textContent();
 await page.mouse.move(canvas!.x+canvas!.width/2+50,canvas!.y+canvas!.height/2);await page.mouse.wheel(0,-500);
 await expect(page.getByTestId("navigation-camera")).not.toHaveText(beforeZoom!);
 const camera=await page.getByTestId("navigation-camera").textContent();
 const originScreen=await worldScreen(page,[0,0,0]);
 await openCommand(page,"旋转");const dialog=page.getByRole("dialog",{name:"创建 旋转",exact:true});
 const currentCamera=await page.getByTestId("navigation-camera").textContent(),currentOrigin=await worldScreen(page,[0,0,0]);
 expect(currentCamera!.split(" / ").slice(1)).toEqual(camera!.split(" / ").slice(1));
 expect(Math.abs(currentOrigin.x-originScreen.x)).toBeLessThan(.1);expect(Math.abs(currentOrigin.y-originScreen.y)).toBeLessThan(.1);
 await expect(dialog.getByRole("combobox")).toHaveCount(0);
 await clickWorld(page,[7.5,2,0]);await expect(page.getByTestId("viewport-feature-selection")).toHaveAttribute("data-role","axis");
 await clickWorld(page,[5,6,0]);await expect(dialog.getByRole("button",{name:"旋转轴：已选择 1 条轴线"})).toBeVisible();await ready(page,dialog);
 await page.screenshot({path:"/tmp/occccad-revolve-profile-axis.png"});
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("textbox",{name:"角度（deg）",exact:true}).fill("180");await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id),f=committed.part.features.at(-1);
 expect(f.type).toBe("REVOLVE");expect(f.axisEntityId).toBe(`SKETCH_LINE:${seed.sketchId}:${seed.axisId}`);
 expect(committed.artifacts[committed.part.bodies.find((b:{id:string})=>b.id===f.bodyId).geometryKey].volume).toBeCloseTo(100*Math.PI,5);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("chamfered box fillet and shell retain exact history, failed display and tree suppression",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Chamfer modifier regression");const id=v.document.id;
  const apply=async(input:Record<string,unknown>)=>v=await api.command(id,input);
  await apply({type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id;
  await apply({type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:0,y:0},second:{x:20,y:30}}]});
  await apply({type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"NEW_BODY",length:40});
  const bodyId=v.part!.activeBodyId!;
  const picks=async(kind:"EDGE"|"FACE",predicate:(p:any)=>boolean)=>{
   const a=v.artifacts![v.part!.bodies.find(b=>b.id===bodyId)!.geometryKey!];const found=[];
   for(let i=1;i<=Number(a.topology[kind==="EDGE"?"edges":"faces"]);i++){
    const p=await api.getTopologyProperties(id,a.geometryKey,kind,i,v.document.versionId);
    if(p.persistentSelection&&predicate(p))found.push({selection:p.persistentSelection,sourceVersionId:v.document.versionId});
   }return found;
  };
  const vertical=await picks("EDGE",p=>p.persistentSelection.anchor.outputSlot.startsWith("VERTICAL_FROM_PROFILE_ENDPOINTS/"));
  if(vertical.length!==4)throw new Error("box must have four upright edges");
  await apply({type:"CREATE_MODIFY_FEATURE",feature:{type:"CHAMFER",bodyId,length:2,selections:vertical}});
  const chamfer=v.part!.features.at(-1)!;
  const bottom=await picks("EDGE",p=>p.geometryType==="LINE"&&Math.abs(p.properties.origin[2])<1e-7&&Math.abs(p.properties.direction[2])<1e-7);
  if(bottom.length!==8)throw new Error(`expected eight bottom edges, got ${bottom.length}`);
  await apply({type:"CREATE_MODIFY_FEATURE",feature:{type:"FILLET",bodyId,length:1,selections:bottom}});
  const filletVolume=v.artifacts![v.part!.bodies.find(b=>b.id===bodyId)!.geometryKey!].volume;
  await apply({type:"UNDO"});
  const node=(featureId:string):any=>{
   const find=(nodes:any[]):any=>{for(const n of nodes){if(n.entityId===featureId)return n;const found=find(n.children??[]);if(found)return found;}};
   return find([v.structureTree]);
  };
  await apply({type:"EDIT_FEATURE",targetId:chamfer.id,expectedFeatureDigest:node(chamfer.id).definitionDigest,feature:{...chamfer,length:.5}});
  const top=await picks("FACE",p=>p.persistentSelection.anchor.outputSlot.startsWith("END_CAP/"));
  await apply({type:"CREATE_MODIFY_FEATURE",feature:{type:"SHELL",bodyId,length:1,selections:top}});
  const shell=v.part!.features.at(-1)!,successfulRevision=v.document.versionId;
  const shellVolume=v.artifacts![v.part!.bodies.find(b=>b.id===bodyId)!.geometryKey!].volume;
  await apply({type:"EDIT_FEATURE",targetId:chamfer.id,expectedFeatureDigest:node(chamfer.id).definitionDigest,feature:{...chamfer,length:1000}});
  return {id,bodyId,chamferName:chamfer.name,shellName:shell.name,successfulRevision,filletVolume,shellVolume};
 });
 expect(seed.filletVolume).toBeGreaterThan(0);expect(seed.filletVolume).toBeLessThan(23680);
 expect(seed.shellVolume).toBeCloseTo(4324,5);
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await expect(page.locator(".cad-failed-body-notice")).toContainText("上次成功");
 const failed=await current(page,seed.id),body=failed.part.bodies.find((b:{id:string})=>b.id===seed.bodyId);
 expect(body.geometryKey??"").toBe("");expect(body.displayFallback.sourceVersionId).toBe(seed.successfulRevision);
 expect(Object.keys(failed.artifacts[body.displayFallback.geometryKey].representations)).toEqual(["VISUAL"]);
 await page.screenshot({path:"/tmp/occccad-failed-body-display.png"});
 await page.reload();await expect(page.locator(".cad-failed-body-notice")).toBeVisible();
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 for(const name of [seed.shellName,seed.chamferName]){
  await filter.fill(name);const feature=page.getByRole("treeitem").filter({hasText:name}).first();
  await feature.click({button:"right"});await page.getByRole("menuitem",{name:"抑制",exact:true}).click();
  await expect(feature.getByLabel("状态 已抑制")).toBeVisible();
 }
 await expect(page.locator(".cad-failed-body-notice")).toHaveCount(0);
 const recovered=await current(page,seed.id),readyBody=recovered.part.bodies.find((b:{id:string})=>b.id===seed.bodyId);
 expect(readyBody.displayFallback).toBeUndefined();expect(recovered.artifacts[readyBody.geometryKey].volume).toBeCloseTo(24000,5);
 expect(errors).toEqual([]);
});
