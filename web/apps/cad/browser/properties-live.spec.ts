import {worldScreen} from "./camera-helpers";
import {expect,test,type Page} from "@playwright/test";

async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json()}


test("real properties history, face preview, conflict recovery and adaptive scene",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 const login=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});
 expect(login.ok()).toBeTruthy();await page.goto("/");
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Properties real acceptance"),id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XZ"});const sketchId=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:0,y:-15},second:{x:20,y:-5}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"NEW_BODY",length:10});
  const body=v.part!.bodies.find(b=>b.id===v.part!.features.at(-1)!.bodyId)!;
  // @ts-ignore development projection
  const {useUIPreferences}=await import("/src/state/ui-preferences.ts");
  useUIPreferences.getState().setReferenceVisibility({planes:false,axes:false,coordinateSystems:false});
  useUIPreferences.getState().setSolidDisplay({mode:"shaded",edges:true,vertices:false});
  const bbox=v.artifacts[body.geometryKey!].bbox,dimensions=bbox.max.map((value,index)=>value-bbox.min[index]).sort((a,b)=>a-b);
  const gap=Math.max(dimensions[1]*.005,Math.min(dimensions[1]*.08,Math.cbrt(dimensions[0]*dimensions[1]*dimensions[2])*.08));
  return {id,bodyId:body.id,bodyName:body.name,key:body.geometryKey!,version:v.document.versionId,minZ:bbox.min[2],ground:Number((bbox.min[2]-gap).toFixed(2))};
 });
 let developmentReads=0;page.on('request',r=>{if(r.method()==='GET'&&r.url().endsWith('/properties'))developmentReads++});
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 const developer=page.locator('details.inspector-diagnostics');await expect(developer).not.toHaveAttribute('open','');expect(developmentReads).toBe(0);await expect(page.locator('.inspector-overlay-content')).not.toContainText(seed.version);
 await developer.locator('summary').click();await expect.poll(()=>developmentReads).toBe(1);await expect(page.getByLabel('开发数据',{exact:true})).toContainText(seed.version);await developer.locator('summary').click();await expect(page.getByLabel('开发数据',{exact:true})).toHaveCount(0);

 const dialog=page.getByRole("dialog",{name:/^属性 ·/});
 const body=()=>page.locator(`[role="treeitem"][data-tree-key$="/body:${seed.bodyId}"]`);
 const openBody=async()=>{await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.bodyName);await body().click({button:"right"});await page.getByRole("menuitem",{name:"属性…",exact:true}).click();await expect(dialog).toBeVisible()};
 const save=async()=>{await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0)};
 const appearance=async()=>{const v=await current(page,seed.id);return v.part.bodies.find((b:{id:string})=>b.id===seed.bodyId).appearance};
 await openBody();await dialog.getByRole("textbox",{name:"颜色 HEX",exact:true}).fill("#2176ff");await dialog.getByRole("spinbutton",{name:"表面透明度",exact:true}).fill("40");await save();
 await expect.poll(async()=>(await appearance())?.opacity).toBeCloseTo(.6,8);
 const styled=await current(page,seed.id);expect(styled.part.bodies.find((b:{id:string})=>b.id===seed.bodyId).geometryKey).toBe(seed.key);
 await page.locator("canvas").click({position:{x:(await page.locator("canvas").boundingBox())!.width/2,y:20}});await page.keyboard.press("Control+z");await expect.poll(appearance).toBeUndefined();
 await expect(page.getByRole("button",{name:"重做",exact:true})).toBeEnabled();await page.getByRole("button",{name:"重做",exact:true}).click();await expect.poll(async()=>(await appearance())?.color).toBe("#2176ff");
 // A cancelled preview is a server-validated candidate with no Head mutation.
 await openBody();const beforeCancel=(await current(page,seed.id)).document.versionId;
 await dialog.getByRole("textbox",{name:"颜色 HEX",exact:true}).fill("#ff2200");await expect(page.locator("[data-property-preview]")).toHaveCount(1);
 await dialog.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(beforeCancel);expect((await appearance()).color).toBe("#2176ff");
 // A top face is selected using the production raycaster, then bound to Naming.
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill("");
 const screen=await worldScreen(page,[10,-5,-5]);await page.mouse.click(screen.x,screen.y);await page.keyboard.press("Alt+Enter");await expect(dialog).toBeVisible();
 await expect(dialog).toContainText("所选面");await dialog.getByRole("textbox",{name:"颜色 HEX",exact:true}).fill("#000000");await dialog.getByRole("spinbutton",{name:"表面透明度",exact:true}).fill("75");await save();
 await expect.poll(async()=>(await appearance()).faces?.length??0).toBe(1);
 const faceStyle=(await appearance()).faces[0];expect(faceStyle.opacity).toBe(.25);expect(faceStyle.anchor.sourceIds.length).toBeGreaterThan(0);expect(faceStyle.localId).toBeUndefined();
 await page.screenshot({path:"../../../build/acceptance/properties-real-face.png"});
 // A concurrent name edit fails CAS; refresh keeps only the user's edited fields.
 await openBody();await dialog.getByRole("textbox",{name:"颜色 HEX",exact:true}).fill("#43a047");
 await page.evaluate(async({id,bodyId})=>{
  // @ts-ignore production module
  const {restApi:api}=await import("/src/api.ts");const v=await api.getDocument(id);
  await api.command(id,{type:"SET_PROPERTIES",versionId:v.document.versionId,properties:{targets:[{kind:"BODY",id:bodyId}],name:"Concurrent body name"}});
 },seed);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toContainText("属性尚未保存");
 await dialog.getByRole("button",{name:"加载最新版本并保留修改",exact:true}).click();await expect(dialog.getByRole("textbox",{name:"颜色 HEX",exact:true})).toHaveValue("#43a047");await save();
 const final=await current(page,seed.id);expect(final.part.bodies.find((b:{id:string})=>b.id===seed.bodyId).name).toBe("Concurrent body name");
 expect(final.part.bodies.find((b:{id:string})=>b.id===seed.bodyId).geometryKey).toBe(seed.key);
 const revision=final.document.versionId;
 const openScene=async()=>{await page.getByRole("button",{name:"选项",exact:true}).click();await page.getByRole("dialog",{name:"选项",exact:true}).getByRole("tab",{name:"地面与网格",exact:true}).click()};
 await openScene();const scene=page.getByRole("dialog",{name:"选项",exact:true});expect(seed.minZ).toBeLessThan(0);
 await expect.poll(async()=>Number(/当前高度 ([^\s]+)/.exec(await scene.getByLabel("当前场景地面").innerText())?.[1])).toBe(seed.ground);
 await scene.getByRole("combobox",{name:"地面位置",exact:true}).click();await page.getByTitle("世界 XY 平面（Z=0）",{exact:true}).click();await expect(scene.getByLabel("当前场景地面")).toContainText("当前高度 0 mm");
 await scene.getByRole("tab",{name:"光照",exact:true}).click();await scene.getByRole("combobox",{name:"光照预设",exact:true}).click();await page.getByTitle("柔和",{exact:true}).click();
 await page.screenshot({path:"../../../build/acceptance/properties-real-scene.png"});await scene.getByRole("button",{name:/取\s*消/}).click();
 expect((await current(page,seed.id)).document.versionId).toBe(revision);
 await openScene();await expect.poll(async()=>Number(/当前高度 ([^\s]+)/.exec(await scene.getByLabel("当前场景地面").innerText())?.[1])).toBe(seed.ground);await scene.getByRole("button",{name:/取\s*消/}).click();
 expect(errors).toEqual([]);
});

test("global creation defaults are frozen for ordinary and assembly-created Parts",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 const login=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto("/");
 const old=await page.evaluate(async()=>{
  // @ts-ignore production API
  const {restApi:api}=await import("/src/api.ts");return api.createDocument("PART","Existing Part before options");
 });
 const options=page.getByRole("dialog",{name:"选项",exact:true});
 const changeDefault=async(color:string)=>{
  await page.getByRole("button",{name:"选项",exact:true}).click();await options.getByRole("menuitem",{name:"新建对象",exact:true}).click();
  await options.getByRole("textbox",{name:"默认实体颜色 HEX",exact:true}).fill("#bad");await expect(options.getByRole("button",{name:/确\s*定/})).toBeDisabled();
  await options.getByRole("textbox",{name:"默认实体颜色 HEX",exact:true}).fill(color);await page.screenshot({path:"../../../build/acceptance/global-options-defaults.png"});
  await options.getByRole("button",{name:/确\s*定/}).click();await expect(options).not.toBeVisible();
 };
 await changeDefault("#bb7733");
 const created=await page.evaluate(async()=>{
  // @ts-ignore production API
  const {restApi:api}=await import("/src/api.ts");const part=await api.createDocument("PART","Global default Part");
  const product=await api.createDocument("PRODUCT","Global default Product");const inserted=await api.createPartComponent(product.document.id,{name:`Default child ${crypto.randomUUID().slice(0,8)}`});
  const child=await api.getDocument(inserted.product!.instances.at(-1)!.documentId);
  return {part,product,child};
 });
 expect(created.part.part.defaultAppearance.color).toBe("#bb7733");expect(created.child.part.defaultAppearance.color).toBe("#bb7733");expect(created.product.product.defaultAppearance).toBeUndefined();
 await changeDefault("#336699");
 const existing=await current(page,created.part.document.id),previous=await current(page,old.document.id);
 expect(existing.document.versionId).toBe(created.part.document.versionId);expect(existing.part.defaultAppearance.color).toBe("#bb7733");expect(previous.document.versionId).toBe(old.document.versionId);expect(previous.part.defaultAppearance).toEqual(old.part.defaultAppearance);
 const newer=await page.evaluate(async()=>{
  // @ts-ignore production API
  const {restApi:api}=await import("/src/api.ts");return api.createDocument("PART","Newer global default Part");
 });expect(newer.part.defaultAppearance.color).toBe("#336699");expect(errors).toEqual([]);
});

test("new API documents acquire previews in the list without entering the editor",async({page})=>{
 const login=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
 const name=`Thumbnail live ${Date.now()}`;
 await page.goto("/");
 const view=await page.evaluate(async name=>{
  // @ts-ignore production API includes the normal CSRF and creation defaults.
  const {restApi}=await import("/src/api.ts");return restApi.createDocument("PART",name);
 },name);
 let loads=0;
 await page.route(`**/api/documents/${view.document.id}/preview**`,async route=>{
  const response=await route.fetch();loads++;
  // Hold the first response in pending to exercise the list/job completion race.
  await route.fulfill({response,headers:{...response.headers(),"x-occccad-thumbnail-state":loads===1?"pending":response.headers()["x-occccad-thumbnail-state"]}});
 });
 await page.reload();await page.getByRole("textbox",{name:"搜索文档",exact:true}).fill(name);
 const card=page.locator(".document-card").filter({hasText:name});await expect(card).toBeVisible();
 await expect.poll(async()=>{const r=await page.request.get(`/api/documents/${view.document.id}/preview`);return r.headers()["x-occccad-thumbnail-state"]},{timeout:30000}).toBe("ready");
 await expect.poll(()=>loads).toBeGreaterThan(1);
 await expect(card.locator(".document-thumbnail img")).toHaveAttribute("src",/^blob:/);
 await expect.poll(()=>card.locator(".document-thumbnail img").evaluate((img:HTMLImageElement)=>img.complete&&img.naturalWidth===640)).toBe(true);
 expect(page.url()).not.toContain(`/documents/${view.document.id}`);
 const current=await page.request.get(`/api/documents/${view.document.id}`);expect((await current.json()).document.versionId).toBe(view.document.versionId);
});
