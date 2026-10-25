import {viewportProbe,expectViewport} from "./camera-helpers";
import {worldScreen} from "./camera-helpers";
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

async function clickWorld(page:Page,point:number[]){
 const position=await worldScreen(page,point);
 await page.mouse.click(position.x,position.y);
}
async function current(page:Page,id:string){return (await page.request.get(`/api/documents/${id}`)).json();}
async function ready(page:Page,dialog:ReturnType<Page["getByRole"]>){
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();
 await expect(page.locator("[data-feature-preview]")).toHaveCount(1);
}

test("new datum plane binds a body face, previews, commits and reopens",async({page})=>{
 const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let view=await api.createDocument("PART","Associated datum acceptance");const id=view.document.id;
  view=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=view.part!.features.at(-1)!.id;
  view=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:0,y:0},second:{x:20,y:20}}]});
  view=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"ADD",length:10});
  return {id,version:view.document.versionId,bodyId:view.part!.activeBodyId,featureId:view.part!.features.at(-1)!.id};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await expect(async()=>{
  await clickWorld(page,[10,10,10]);
  await expectViewport(viewportProbe(page,"viewport-selection")).toContainText('"face"',{timeout:1000});
 }).toPass({timeout:20_000});
 await openCommand(page,"基准面");const dialog=page.getByRole("dialog",{name:"创建基准面",exact:true});
 await expect(dialog.getByRole("radio",{name:"关联参考",exact:true})).toBeChecked();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id),plane=committed.part.datumPlanes.at(-1);
 expect(committed.document.versionId).not.toBe(seed.version);
 expect(plane.definition.source).toMatchObject({kind:"TOPOLOGY",featureId:seed.featureId,selection:{sourceBodyId:seed.bodyId,expectedType:"FACE"}});
 expect(plane.origin[2]).toBeCloseTo(10,6);expect(plane.normal).toEqual([0,0,1]);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();
 expect((await current(page,seed.id)).part.datumPlanes.find((candidate:{id:string})=>candidate.id===plane.id)).toEqual(plane);expect(errors).toEqual([]);
});

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
 // Wait for the exact current scene to be pickable before the first click.
 await expect.poll(async()=>{const position=await worldScreen(page,[7,7,20]);return page.evaluate(({x,y})=>{const e=(window as any).__cameraEngine,r=document.querySelector('canvas')!.getBoundingClientRect();return e.hitTest(x-r.x,y-r.y,true)?.bodyId},position)}).toBe(seed.tool);
 await clickWorld(page,[7,7,20]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"工具实体"}).getByText("1 项",{exact:true})).toBeVisible();
 await ready(page,dialog);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await clickWorld(page,[7,7,20]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"工具实体"}).getByText("0 项",{exact:true})).toBeVisible();
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
 await clickWorld(page,[7,7,20]);await expect(edit.locator(".feature-pick-field").filter({hasText:"工具实体"}).getByText("0 项",{exact:true})).toBeVisible();
 await expect(edit.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 expect((await current(page,seed.id)).document.versionId).toBe(committed.document.versionId);
 await clickWorld(page,[7,7,20]);await expect(edit.locator(".feature-pick-field").filter({hasText:"工具实体"}).getByText("1 项",{exact:true})).toBeVisible();await ready(page,edit);
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
 await expect(async()=>{await clickWorld(page,[10,0,10]);await expectViewport(viewportProbe(page,"viewport-selection")).toContainText('"edge"');}).toPass({timeout:30_000});
 await openCommand(page,"倒角");const dialog=page.getByRole("dialog",{name:"创建 倒角",exact:true});
 await expect(dialog.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await ready(page,dialog);
 await page.screenshot({path:"../../../build/acceptance/feature-form-modeling.png"});
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 // A failed preview clears the candidate and cannot submit stale geometry.
 await dialog.getByRole("textbox").fill("1000");
 await expect(dialog.getByRole("alert")).toBeVisible();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("textbox").fill("1");await ready(page,dialog);
 await expect(dialog.getByRole("alert")).toHaveCount(0);
 await clickWorld(page,[0,-10,10]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("2 项",{exact:true})).toBeVisible();await ready(page,dialog);
 await clickWorld(page,[0,-10,10]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id);const body=committed.part.bodies.find((b:{id:string})=>b.id===seed.bodyId);
 expect(committed.artifacts[body.geometryKey].volume).toBeCloseTo(3990,5);
 const feature=committed.part.features.at(-1);const filter=page.getByRole("textbox",{name:"筛选模型结构"});await filter.fill(feature.name);
 await page.getByRole("treeitem").filter({hasText:feature.name}).first().dblclick();
 const edit=page.getByRole("dialog",{name:"编辑 倒角",exact:true});await expect(edit).toBeVisible();
 await expect(edit.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await ready(page,edit);
 await expectViewport(viewportProbe(page,"viewport-feature-selection")).toHaveAttribute("data-overlays","1");
 await page.screenshot({path:"../../../build/acceptance/feature-form-chamfer-edit.png"});
 await clickWorld(page,[10,0,10]);await expect(edit.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("0 项",{exact:true})).toBeVisible();
 await clickWorld(page,[10,0,10]);await expect(edit.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await ready(page,edit);
 await edit.getByRole("button",{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);
 const edited=await current(page,seed.id);expect(edited.part.features.at(-1).selections[0].sourceFeatureId).toBeTruthy();
 expect(edited.artifacts[edited.part.bodies.find((b:{id:string})=>b.id===seed.bodyId).geometryKey].volume).toBeCloseTo(3990,5);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("rectangle to circle loft previews directly from viewport sections",async({page},testInfo)=>{
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
  return {id,version:v.document.versionId,bottom,top,bodyId:v.part!.activeBodyId,bodyCount:v.part!.bodies.length};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await openCommand(page,"放样");const dialog=page.getByRole("dialog",{name:"创建 放样",exact:true});
 await expect(dialog.getByRole("combobox")).toHaveCount(0);
 await clickWorld(page,[10,-5,0]);await expect(dialog.getByRole("group",{name:"截面",exact:true}).getByText("1 项",{exact:true})).toBeVisible();
 await clickWorld(page,[-10,0,30]);await expect(dialog.getByRole("group",{name:"截面",exact:true}).getByText("2 项",{exact:true})).toBeVisible();await ready(page,dialog);
 await dialog.getByRole("button",{name:"上移截面 2",exact:true}).click();await ready(page,dialog);
 const firstSection=dialog.getByRole('group',{name:'截面',exact:true}).getByRole('listitem').first();await expect(firstSection).toContainText('Sketch.2');
 await dialog.getByRole("button",{name:"下移截面 1",exact:true}).click();await ready(page,dialog);
 await page.screenshot({path:testInfo.outputPath("loft-current-body.png")});
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id);const f=committed.part.features.at(-1);
 expect(f.type).toBe("LOFT");expect(f.sections.map((s:{sketchId:string})=>s.sketchId)).toEqual([seed.bottom,seed.top]);
 expect(f.operation).toBe("ADD");expect(f.bodyId).toBe(seed.bodyId);expect(committed.part.bodies).toHaveLength(seed.bodyCount);
 const a=committed.artifacts[committed.part.bodies.find((b:{id:string})=>b.id===f.bodyId).geometryKey];
 expect(a.topology.solids).toBe(1);expect(a.volume).toBeGreaterThan(30*Math.PI*100);expect(a.volume).toBeLessThan(12000);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

for(const fixture of [
 {name:"thin loft shell",box:[-15,-15,15,15],height:40,radius:10},
 {name:"expanding loft shell",box:[-82.8172482034993,-87.47795914135841,111.25953546530698,69.55214784189991],height:100,radius:90},
]) test(`${fixture.name} uses a persistent opening, rejects stale preview and survives editing`,async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async fixture=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART",`${fixture.name} acceptance`);const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const bottom=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:bottom,operations:[{type:"ADD_RECTANGLE",first:{x:fixture.box[0],y:fixture.box[1]},second:{x:fixture.box[2],y:fixture.box[3]}}]});
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"Top",origin:[0,0,fixture.height],normal:[0,0,1],uDirection:[1,0,0]});
  v=await api.command(id,{type:"CREATE_SKETCH",datumPlaneId:v.part!.datumPlanes.at(-1)!.id,plane:"CUSTOM"});const top=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:top,operations:[{type:"ADD_ENTITY",entity:{id:crypto.randomUUID(),kind:"CIRCLE",role:"PROFILE",center:{x:0,y:0},radius:fixture.radius}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",feature:{type:"LOFT",operation:"NEW_BODY",sections:[{sketchId:bottom},{sketchId:top}]}});
  return {id,version:v.document.versionId,bodyId:v.part!.activeBodyId!};
 },fixture);
 test.info().annotations.push({type:"document",description:seed.id});
 console.log("Shell acceptance document",seed.id);
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await openCommand(page,"抽壳");const dialog=page.getByRole("dialog",{name:"创建 抽壳",exact:true});
 await clickWorld(page,[0,0,fixture.height]);
 await expect(dialog.locator(".feature-pick-field").filter({hasText:"面集"}).getByText("1 项",{exact:true})).toBeVisible();
 await dialog.getByRole("textbox",{name:"厚度（mm）",exact:true}).fill("0.5");await ready(page,dialog);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("textbox",{name:"厚度（mm）",exact:true}).fill("1000");
 await expect(dialog.getByRole("alert")).toBeVisible();await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole("textbox",{name:"厚度（mm）",exact:true}).fill("0.5");await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const created=await current(page,seed.id),feature=created.part.features.at(-1);
 expect(feature.type).toBe("SHELL");expect(feature.length).toBe(.5);expect(feature.selections).toHaveLength(1);
 expect(feature.selections[0].selection.anchor.outputSlot).toContain("LOFT_END_CAP");
 const artifact=(view:any)=>view.artifacts[view.part.bodies.find((b:any)=>b.id===seed.bodyId).geometryKey];
 const volume=artifact(created).volume;expect(volume).toBeGreaterThan(0);expect(artifact(created).topology.solids).toBe(1);
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill(feature.name);
 await page.getByRole("treeitem").filter({hasText:feature.name}).first().dblclick();
 const edit=page.getByRole("dialog",{name:"编辑 抽壳",exact:true});await ready(page,edit);
 await expect(edit.locator(".feature-pick-field").filter({hasText:"面集"}).getByText("1 项",{exact:true})).toBeVisible();
 await edit.getByRole("textbox",{name:"厚度（mm）",exact:true}).fill("1");await ready(page,edit);
 expect((await current(page,seed.id)).document.versionId).toBe(created.document.versionId);
 await edit.getByRole("button",{name:/确\s*定/}).click();await expect(edit).toHaveCount(0);
 const edited=await current(page,seed.id);expect(artifact(edited).volume).toBeGreaterThan(volume);
 await page.evaluate(async id=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");await api.command(id,{type:"UNDO"});
 },seed.id);
 const undone=await current(page,seed.id);expect(undone.document.versionId).not.toBe(edited.document.versionId);
 expect(artifact(undone).volume).toBeCloseTo(volume,5);
 await page.evaluate(async id=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");await api.command(id,{type:"REDO"});
 },seed.id);
 const redone=await current(page,seed.id);expect(redone.document.versionId).not.toBe(undone.document.versionId);
 expect(artifact(redone).geometryKey).toBe(artifact(edited).geometryKey);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();
 const reopened=await current(page,seed.id);expect(artifact(reopened).geometryKey).toBe(artifact(edited).geometryKey);
 expect(reopened.part.features.at(-1).selections).toEqual(edited.part.features.at(-1).selections);
 await page.screenshot({path:test.info().outputPath("shell-reopened.png")});
 expect(errors).toEqual([]);
});

for (const round of [false, true]) test(`shell ${round ? 'cylinder' : 'box'} ownership agrees with interior picking and complete tree highlight`, async ({page}) => {
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async round=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  let v=await api.createDocument('PART','Shell topology ownership');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;
  const operation=round?{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'CIRCLE',role:'PROFILE',center:{x:0,y:0},radius:10}}:{type:'ADD_RECTANGLE',first:{x:-10,y:-10},second:{x:10,y:10}};
  v=await api.command(id,{type:'EDIT_SKETCH',sketchId,operations:[operation]});
  v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'ADD',length:20});
  return {id,base:v.part!.features.at(-1)!,version:v.document.versionId};
 },round);
 await page.goto(`/documents/${seed.id}`);await worldScreen(page,[0,0,20]);
 await openCommand(page,'抽壳');const dialog=page.getByRole('dialog',{name:'创建 抽壳',exact:true});
 await expect.poll(async()=>{
  const p=await worldScreen(page,[2,2,20]);
  return page.evaluate(p=>{const e=(window as any).__cameraEngine,r=e.renderer.domElement.getBoundingClientRect();return e.hitTest(p.x-r.x,p.y-r.y)?.kind},p);
 }).toBe('face');
 await clickWorld(page,[2,2,20]);await expect(dialog.locator('.feature-pick-field').getByText('1 项',{exact:true})).toBeVisible();await ready(page,dialog);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const created=await current(page,seed.id),shell=created.part.features.at(-1),filter=page.getByRole('textbox',{name:'筛选模型结构'});
 const row=()=>page.locator(`[role=treeitem][data-tree-key$="feature:${shell.id}"]`);
 const checkHighlight=async()=>{
  await filter.fill(shell.name);await row().click();
  await expect(async()=>{
   const counts=await page.evaluate(id=>{
    const e=(window as any).__cameraEngine,b=[...e.solidBindings.values()][0] as any,a=b.artifact;
    const ids=new Set(a.visualization.featureAssociations.elements.filter((v:any)=>v.kind==='FACE'&&v.primary.includes(id)).map((v:any)=>v.localId));
    return {faces:ids.size,expected:a.mesh.faceIds.filter((n:number)=>ids.has(n)).length*3,actual:e.selectedOverlays.filter((o:any)=>o.isMesh).reduce((n:number,o:any)=>n+o.geometry.attributes.position.count,0)};
   },shell.id);
   expect(counts.faces).toBe(round?2:5);expect(counts.expected).toBeGreaterThan(0);expect(counts.actual).toBe(counts.expected);
  }).toPass();
 };
 await checkHighlight();await page.keyboard.press('Escape');await filter.fill('');
 await page.getByRole('button',{name:'顶视图',exact:true}).click();
 await worldScreen(page,[0,0,1]);
 const interior=await page.evaluate(id=>{
  const e=(window as any).__cameraEngine,r=e.renderer.domElement.getBoundingClientRect();
  for(let y=40;y<r.height-40;y+=5)for(let x=40;x<r.width-40;x+=5){
   if(document.elementFromPoint(r.x+x,r.y+y)!==e.renderer.domElement)continue;
   const hit=e.hitTest(x,y);if(hit?.kind==='face'&&hit.associatedFeatureIds?.includes(id))return {x:r.x+x,y:r.y+y};
  }
 },shell.id);
 expect(interior).toBeTruthy();
 await page.mouse.move(interior!.x,interior!.y);await page.mouse.click(interior!.x,interior!.y);
 await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.selected.map((s:any)=>({kind:s.kind,owners:s.associatedFeatureIds}))})).toContainEqual({kind:'face',owners:[shell.id]});
 await filter.fill(shell.name);
 await expect(row()).toHaveClass(/selected-descendant/);
 // Inspect the actual hit through the authoritative topology endpoint, not only display metadata.
 const property=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  const s=(window as any).__cameraEngine.selected[0];
  return api.getTopologyProperties(s.documentId,s.geometryKey,s.kind.toUpperCase(),s.topologyId,s.versionId);
 });
 expect(property.persistentSelection?.anchor.featureId).toBe(shell.id);
 await page.keyboard.press('Escape');await page.getByRole('button',{name:'等轴测',exact:true}).click();
 const outside=await worldScreen(page,round?[Math.SQRT1_2*10,-Math.SQRT1_2*10,20]:[10,-10,10]);
 await page.mouse.click(outside.x,outside.y);
 await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.selected.map((s:any)=>({kind:s.kind,owners:s.associatedFeatureIds}))})).toContainEqual({kind:'edge',owners:[seed.base.id]});
 await checkHighlight();await page.screenshot({path:test.info().outputPath('shell-interior-highlight.png')});
 await row().dblclick();const edit=page.getByRole('dialog',{name:'编辑 抽壳',exact:true});await ready(page,edit);await edit.getByRole('button',{name:/取\s*消/}).click();
 expect((await current(page,seed.id)).document.versionId).toBe(created.document.versionId);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).part.features.at(-1).id).toBe(seed.base.id);
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).part.features.at(-1).id).toBe(shell.id);
 await page.reload();await worldScreen(page,[0,0,20]);await checkHighlight();expect(errors).toEqual([]);
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
 const beforeZoom=await viewportProbe(page,"navigation-camera").textContent();
 await page.mouse.move(canvas!.x+canvas!.width/2+50,canvas!.y+canvas!.height/2);await page.mouse.wheel(0,-500);
 await expectViewport(viewportProbe(page,"navigation-camera")).not.toHaveText(beforeZoom!);
 const camera=await viewportProbe(page,"navigation-camera").textContent();
 const originScreen=await worldScreen(page,[0,0,0]);
 await openCommand(page,"旋转");const dialog=page.getByRole("dialog",{name:"创建 旋转",exact:true});
 const currentCamera=await viewportProbe(page,"navigation-camera").textContent(),currentOrigin=await worldScreen(page,[0,0,0]);
 expect(currentCamera!.split(" / ").slice(1)).toEqual(camera!.split(" / ").slice(1));
 expect(Math.abs(currentOrigin.x-originScreen.x)).toBeLessThan(.1);expect(Math.abs(currentOrigin.y-originScreen.y)).toBeLessThan(.1);
 await expect(dialog.getByRole("combobox")).toHaveCount(0);
 await clickWorld(page,[7.5,4,0]);await dialog.getByRole("button",{name:/旋转轴：/}).click();await expectViewport(viewportProbe(page,"viewport-feature-selection")).toHaveAttribute("data-role","axis");
 await clickWorld(page,[5,6,0]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"旋转轴"}).getByText("已选择 1 条轴线",{exact:true})).toBeVisible();await ready(page,dialog);
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
 expect(failed.artifacts[body.displayFallback.geometryKey].visualNamingDigest).toBeTruthy();
 const display=failed.artifacts[body.displayFallback.geometryKey].representations.VISUAL;
 const displayResponse=await page.request.get(`/api/documents/${seed.id}/representations/${display.objectId}`);expect(displayResponse.ok()).toBeTruthy();
 await page.screenshot({path:test.info().outputPath("failed-body-display.png")});
 await page.reload();await expect(page.locator(".cad-failed-body-notice")).toBeVisible();
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 for(const name of [seed.shellName,seed.chamferName]){
  await filter.fill(name);const feature=page.getByRole("treeitem").filter({hasText:name}).first();
  await feature.click({button:"right"});await page.getByRole("menuitem",{name:/对象/}).hover();
  const suppress=page.getByRole("menuitem",{name:"抑制",exact:true});await expect(suppress).toBeVisible();await suppress.click();
  await expect(feature.getByLabel("状态 已抑制")).toBeVisible();
 }
 await expect(page.locator(".cad-failed-body-notice")).toHaveCount(0);
 const recovered=await current(page,seed.id),readyBody=recovered.part.bodies.find((b:{id:string})=>b.id===seed.bodyId);
 expect(readyBody.displayFallback).toBeUndefined();expect(recovered.artifacts[readyBody.geometryKey].volume).toBeCloseTo(24000,5);
 expect(errors).toEqual([]);
});

for (const featureType of ["FILLET","CHAMFER","SHELL","DRAFT"] as const) {
 test(`repairs lost topology inputs for ${featureType} through actual viewport picks`,async({page},testInfo)=>{
  const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
  const seed=await page.evaluate(async featureType=>{
   // @ts-ignore production Vite module, real API/Router/Worker
   const {restApi:api}=await import("/src/api.ts");
   let v=await api.createDocument("PART",`Lost input ${featureType}`);const id=v.document.id;
   const apply=async(r:Record<string,unknown>)=>{v=await api.command(id,r);};
   const pad=async(x:number,y:number,w:number,h:number,length:number)=>{
    await apply({type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id;
    await apply({type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x,y},second:{x:x+w,y:y+h}}]});
    await apply({type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"ADD",length});return v.part!.features.at(-1)!;
   };
   const base=await pad(-20,-20,40,40,10),stem=await pad(-5,-5,10,10,20);
   const pick=async(featureId:string,kind:"EDGE"|"FACE",slot:string,front=false,excludeSlot?:string,bodyId=base.bodyId)=>{
    const a=v.artifacts![v.part!.bodies.find(b=>b.id===bodyId)!.geometryKey!];
    for(let localId=1;localId<=Number(a.topology[kind==="EDGE"?"edges":"faces"]);localId++){
     const p=await api.getTopologyProperties(id,a.geometryKey,kind,localId,v.document.versionId),s=p.persistentSelection;
     if(s?.anchor.featureId===featureId&&s.anchor.outputSlot.startsWith(slot)&&s.anchor.outputSlot!==excludeSlot&&(!front||(s.creationEvidence.centroid?.[1]??0)<-19))return{selection:s,sourceVersionId:v.document.versionId};
    }
    throw new Error("required stable topology not found");
   };
   const kind=featureType==="FILLET"||featureType==="CHAMFER"?"EDGE":"FACE";
   const slot=kind==="EDGE"?"END_BOUNDARY_FROM_PROFILE_EDGE/":featureType==="DRAFT"?"SIDE_FROM_PROFILE_EDGE/":"END_CAP/";
   const lost=await pick(stem.id,kind,slot),kept=await pick(base.id,kind,slot,featureType==="FILLET");
   const selections=featureType==="FILLET"?[kept,lost]:featureType==="DRAFT"?[kept]:featureType==="CHAMFER"?[lost,await pick(stem.id,kind,slot,false,lost.selection.anchor.outputSlot)]:[lost];
   let neutralPlane,neutralSource:typeof stem|undefined;
   if(featureType==="DRAFT"){
    await apply({type:"CREATE_BODY"});neutralSource=await pad(-5,-5,10,10,20);
    neutralPlane=await pick(neutralSource.id,"FACE","END_CAP/",false,undefined,neutralSource.bodyId);
    await apply({type:"SET_ACTIVE_BODY",bodyId:base.bodyId});
   }
   await apply({type:"CREATE_MODIFY_FEATURE",feature:{type:featureType,bodyId:base.bodyId,length:.5,angle:2,selections,neutralPlane}});
   const feature=v.part!.features.at(-1)!;const downstream=await pad(-2,-2,4,4,25);
   const digest=(target:string)=>{const find=(node:any):any=>node.entityId===target&&node.presentationRole!=="INPUT_REFERENCE"?node.definitionDigest:node.children?.map(find).find(Boolean);return find(v.structureTree);};
   await apply({type:"SET_FEATURE_SUPPRESSION",targetId:stem.id,expectedFeatureDigest:digest(stem.id),suppressed:true});
   if(neutralSource)await apply({type:"SET_FEATURE_SUPPRESSION",targetId:neutralSource.id,expectedFeatureDigest:digest(neutralSource.id),suppressed:true});
   if(v.part!.features.find(f=>f.id===feature.id)!.evaluationStatus!=="FAILED")throw new Error("fixture did not invalidate saved topology");
   return{id,feature,downstream:downstream.id,failedHead:v.document.versionId};
  },featureType);
  await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
  const name={FILLET:"圆角",CHAMFER:"倒角",SHELL:"抽壳",DRAFT:"拔模"}[featureType];
  const open=async()=>{await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.feature.name);await page.getByRole("treeitem").filter({hasText:seed.feature.name}).first().dblclick();};
  await open();let dialog=page.getByRole("dialog",{name:`编辑 ${name}`,exact:true});
  await expect(dialog.getByText(featureType==="DRAFT"?"中性平面需要修复":"输入需要修复",{exact:true})).toBeVisible();
  await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
  await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
  expect((await current(page,seed.id)).document.versionId).toBe(seed.failedHead);
  if(featureType==="FILLET"){
   await expect(dialog.locator('[data-state="RESOLVED"]')).toHaveCount(1);
   await expect(dialog.locator('[data-state="OUTSIDE_CURRENT_TIP"]')).toHaveCount(1);
  }
  if(featureType==="DRAFT")await dialog.getByRole("button",{name:"选择中性平面",exact:true}).click();
  else await dialog.getByRole("button",{name:`选择${["FILLET","CHAMFER"].includes(featureType)?"边":"面"} ${featureType==="FILLET"?2:1}`,exact:true}).click();
  await clickWorld(page,featureType==="FILLET"||featureType==="CHAMFER"?[20,0,10]:[10,-10,10]);
  if(featureType==="CHAMFER"){
   await expect(dialog.locator('[data-state="RESOLVED"]')).toHaveCount(1);
   await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
   await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
   await dialog.getByRole("button",{name:"移除边 2",exact:true}).click();
  }
  await ready(page,dialog);
  expect((await current(page,seed.id)).document.versionId).toBe(seed.failedHead);
  if(featureType==="FILLET"){
   // Cancel an actual successful repair preview, then restore the saved broken inputs.
   await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).toHaveCount(0);
   await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
   expect((await current(page,seed.id)).document.versionId).toBe(seed.failedHead);
   await open();dialog=page.getByRole("dialog",{name:`编辑 ${name}`,exact:true});
   await expect(dialog.locator('[data-state="OUTSIDE_CURRENT_TIP"]')).toHaveCount(1);
   await dialog.getByRole("button",{name:"选择边 2",exact:true}).click();
   await clickWorld(page,[20,0,10]);await ready(page,dialog);
  }
  await page.screenshot({path:testInfo.outputPath(`repaired-${featureType}.png`)});
  await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
  const repaired=await current(page,seed.id),feature=repaired.part.features.find((f:any)=>f.id===seed.feature.id);
  expect(feature).toMatchObject({id:seed.feature.id,name:seed.feature.name,order:seed.feature.order,bodyId:seed.feature.bodyId});
  expect(feature.evaluationStatus).not.toBe("FAILED");expect(repaired.part.features.find((f:any)=>f.id===seed.downstream).evaluationStatus).not.toBe("BLOCKED");
  if(featureType==="FILLET")expect(feature.selections[0]).toEqual(seed.feature.selections[0]);
  await page.reload();await expect(page.locator("canvas")).toBeVisible();expect((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.feature.id)).toEqual(feature);
  expect(errors).toEqual([]);
 });
}

for(const generator of ["LINEAR_EXTRUDE","REVOLVE"] as const){
 test(`explicit profile regions ${generator}: viewport toggles, frozen sets and repair`,async({page},testInfo)=>{
  const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
  const seed=await page.evaluate(async()=>{
   // @ts-ignore production Vite module
   const {restApi:api}=await import("/src/api.ts");
   let v=await api.createDocument("PART","Explicit region viewport acceptance");const id=v.document.id;
   v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id;
   const circle=(id:string,x:number,r:number)=>({type:"ADD_ENTITY",entity:{id,kind:"CIRCLE",role:"PROFILE",center:{x,y:15},radius:r}});
   v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[circle("outer",-15,5),circle("hole",-15,2),circle("other",15,4),{type:"ADD_ENTITY",entity:{id:"axis",kind:"LINE",role:"CONSTRUCTION",start:{x:-30,y:0},end:{x:30,y:0}}}]});
   return {id,sketchId,version:v.document.versionId};
  });
  await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
  const tool=generator==="REVOLVE"?"旋转":"拉伸";
  await openCommand(page,tool);let dialog=page.getByRole("dialog",{name:`创建 ${tool}`,exact:true});
  await expect(dialog.getByRole("radio",{name:/选定区域/})).toBeChecked();
  const count=(n:number)=>dialog.locator(".feature-pick-field").filter({hasText:"轮廓"}).getByText(`${n} 项`,{exact:true});
  await clickWorld(page,[-11.5,15,0]);await expect(count(1)).toBeVisible();
  // A hole has no region pick surface.
  await clickWorld(page,[-15,15,0]);await expect(count(1)).toBeVisible();
  await clickWorld(page,[15,15,0]);await expect(count(2)).toBeVisible();
  await clickWorld(page,[15,15,0]);await expect(count(1)).toBeVisible();
  if(generator==="REVOLVE"){
   await dialog.getByRole("button",{name:/旋转轴：/}).click();await clickWorld(page,[8,0,0]);
   await expect(dialog.locator(".feature-pick-field").filter({hasText:"旋转轴"}).getByText("已选择 1 条轴线",{exact:true})).toBeVisible();
  }else await dialog.getByRole("textbox",{name:"长度（mm）",exact:true}).fill("5");
  await ready(page,dialog);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
  await page.screenshot({path:testInfo.outputPath("selected-region.png")});
  await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
  const created=await current(page,seed.id),feature=created.part.features.at(-1),selection=feature.profileSelection;
  expect(selection.mode).toBe("REGIONS");expect(selection.regionIds).toHaveLength(1);
  const volume=(v:any,body:string)=>v.artifacts[v.part.bodies.find((b:any)=>b.id===body).geometryKey].volume;
  const factor=generator==="REVOLVE"?30*Math.PI:5;
  expect(volume(created,feature.bodyId)).toBeCloseTo(21*Math.PI*factor,5);
  const expanded=await page.evaluate(async({seed,generator,firstRegion})=>{
   // @ts-ignore production Vite module
   const {restApi:api}=await import("/src/api.ts");
   let v=await api.getDocument(seed.id);
   const other=v.sketchAnalyses![seed.sketchId].regionIds!.find((r:string)=>r!==firstRegion)!;
   v=await api.command(seed.id,{type:"CREATE_SOLID_FEATURE",sketchId:seed.sketchId,generator,operation:"NEW_BODY",length:5,angle:360,axisEntityId:`SKETCH_LINE:${seed.sketchId}:axis`,profileSelection:{mode:"REGIONS",regionIds:[other]}});
   const second=v.part!.features.at(-1)!;
   v=await api.command(seed.id,{type:"EDIT_SKETCH",sketchId:seed.sketchId,operations:[{type:"ADD_ENTITY",entity:{id:"third",kind:"CIRCLE",role:"PROFILE",center:{x:40,y:15},radius:3}}]});
   return {v,second};
  },{seed,generator,firstRegion:selection.regionIds[0]});
  expect(expanded.v.part.features.find((f:any)=>f.id===feature.id).profileSelection).toEqual(selection);
  expect(volume(expanded.v,feature.bodyId)).toBeCloseTo(volume(created,feature.bodyId),5);
  expect(volume(expanded.v,expanded.second.bodyId!)).toBeCloseTo(16*Math.PI*factor,5);
  await page.reload();await expect(page.locator("canvas")).toBeVisible();
  const editFeature=async()=>{
   await openCommand(page,"适合窗口");
   const canvas=await page.locator("canvas").boundingBox();await page.mouse.move(canvas!.x+canvas!.width/2,canvas!.y+canvas!.height/2);await page.mouse.wheel(0,700);
   await expectViewport(viewportProbe(page,"navigation-camera")).toHaveAttribute("data-transition-active","false");
   await page.getByRole("textbox",{name:"筛选模型结构"}).fill(feature.name);
   await page.getByRole("treeitem").filter({hasText:feature.name}).first().dblclick();
   dialog=page.getByRole("dialog",{name:`编辑 ${tool}`,exact:true});await expect(dialog).toBeVisible();
  };
  await editFeature();await expect(count(1)).toBeVisible();await ready(page,dialog);
  await count(1).click();await clickWorld(page,[15,15,0]);await expect(count(2)).toBeVisible();await ready(page,dialog);
  await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).toHaveCount(0);
  expect((await current(page,seed.id)).document.versionId).toBe(expanded.v.document.versionId);
  await page.evaluate(async seed=>{
   // @ts-ignore production Vite module
   const {restApi:api}=await import("/src/api.ts");
   await api.command(seed.id,{type:"EDIT_SKETCH",sketchId:seed.sketchId,operations:[{type:"DELETE_ENTITIES",entityIds:["outer","hole"]}]});
  },seed);
  await page.reload();await expect(page.locator("canvas")).toBeVisible();await editFeature();
  await expect(dialog.getByText("区域输入需要修复",{exact:true})).toBeVisible();await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
  await dialog.getByRole("button",{name:"选择区域 1",exact:true}).click();await clickWorld(page,[40,15,0]);
  await expect(dialog.getByText("区域输入需要修复",{exact:true})).toHaveCount(0);await ready(page,dialog);
  await page.screenshot({path:testInfo.outputPath("repaired-region.png")});
  await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
  const repaired=await current(page,seed.id),result=repaired.part.features.find((f:any)=>f.id===feature.id);
  expect(result.id).toBe(feature.id);expect(result.bodyId).toBe(feature.bodyId);expect(result.name).toBe(feature.name);expect(result.order).toBe(feature.order);
  expect(result.evaluationStatus).toBeFalsy();expect(volume(repaired,feature.bodyId)).toBeCloseTo(9*Math.PI*factor,5);
  if(generator==="LINEAR_EXTRUDE"){
   await page.evaluate(async({id,bodyId})=>{
    // @ts-ignore production Vite module
    const {restApi:api}=await import("/src/api.ts");await api.command(id,{type:"SET_ACTIVE_BODY",bodyId});
   },{id:seed.id,bodyId:feature.bodyId});
   await page.reload();await expect(page.locator("canvas")).toBeVisible();await openCommand(page,"适合窗口");
   const canvas=await page.locator("canvas").boundingBox();await page.mouse.move(canvas!.x+canvas!.width/2,canvas!.y+canvas!.height/2);await page.mouse.wheel(0,700);
   await openCommand(page,"切除");const cut=page.getByRole("dialog",{name:"创建 拉伸",exact:true});
   await clickWorld(page,[40,15,0]);await expect(cut.locator(".feature-pick-field").filter({hasText:"轮廓"}).getByText("1 项",{exact:true})).toBeVisible();
   await cut.getByRole("textbox",{name:"长度（mm）",exact:true}).fill("2");
   const reversed=cut.getByRole("switch",{name:"反向",exact:true});if(await reversed.isChecked())await reversed.click();
   await ready(page,cut);await cut.getByRole("button",{name:/确\s*定/}).click();await expect(cut).toHaveCount(0);
   const cutView=await current(page,seed.id);expect(cutView.part.features.at(-1).profileSelection).toEqual(result.profileSelection);
   expect(cutView.part.features.at(-1).operation).toBe("REMOVE");expect(volume(cutView,feature.bodyId)).toBeCloseTo(27*Math.PI,5);
  }
  await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
 });
}

test("sketch support changes and repairs a lost face through upstream viewport picks",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Sketch support viewport acceptance");const id=v.document.id;
  const pad=async(x:number,y:number,w:number,h:number,length:number)=>{v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features.at(-1)!.id;v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x,y},second:{x:x+w,y:y+h}}]});v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"ADD",length});return v.part!.features.at(-1)!};
  const base=await pad(-20,-20,40,40,10),tip=await pad(-5,-5,10,10,20);
  const body=v.part!.bodies.find(b=>b.id===base.bodyId)!,a=v.artifacts![body.geometryKey!]!;let face:number|undefined;
  for(let local=1;local<=Number(a.topology!.faces);local++){const p=await api.getTopologyProperties(id,body.geometryKey!,"FACE",local,v.document.versionId);if(p.persistentSelection?.anchor.featureId===base.id&&p.persistentSelection.anchor.outputSlot.startsWith("END_CAP/")){face=local;break}}
  if(!face)throw new Error("fixture plane missing");
  v=await api.command(id,{type:"CREATE_SKETCH",targetKind:"FACE",geometryKey:body.geometryKey,topologyId:face,versionId:v.document.versionId});const sketch=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:sketch.id,operations:[{type:"ADD_RECTANGLE",first:{x:-15,y:-15},second:{x:-10,y:-10}}]});
  const regionIds=v.sketchAnalyses![sketch.id].regionIds!;
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:sketch.id,generator:"LINEAR_EXTRUDE",operation:"ADD",length:3,profileSelection:{mode:"REGIONS",regionIds}});const child=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",origin:[0,0,30],normal:[0,-1,0],uDirection:[1,0,0]});const datum=v.part!.datumPlanes.at(-1)!;
  return {id,base,tip,sketch:v.part!.features.find(f=>f.id===sketch.id)!,child,datum,version:v.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 const open=async()=>{
  await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.sketch.name!);
  const node=page.getByRole("treeitem").filter({hasText:seed.sketch.name!}).last();await node.click({button:"right"});
  await page.getByRole("menuitem",{name:"对象",exact:true}).hover();
  await page.getByRole("menuitem",{name:"更换草图支撑…",exact:true}).click({timeout:10_000});
  await expect(page.getByRole("dialog",{name:"更换草图支撑",exact:true})).toBeVisible();
 };
 await open();let dialog=page.getByRole("dialog",{name:"更换草图支撑",exact:true});
 // Pick a custom datum through the ordinary tree, then cancel a real preview.
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.datum.name);
 await page.getByRole("treeitem").filter({hasText:seed.datum.name}).last().click();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();
 await expect(dialog.getByText(/法向 \[0, -1, 0\]/)).toBeVisible();
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await page.screenshot({path:testInfo.outputPath("support-datum-preview.png")});
 await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 // Invalidate the saved support by a legal upstream change, keeping the stem valid.
 const failed=await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");const v=await api.getDocument(seed.id);
  const find=(n:any):any=>n.entityId===seed.base.id&&n.definitionDigest?n:n.children?.map(find).find(Boolean);
  return api.command(seed.id,{type:"SET_FEATURE_SUPPRESSION",targetId:seed.base.id,expectedFeatureDigest:find(v.structureTree).definitionDigest,suppressed:true});
 },seed);
 expect(failed.part!.features.find((f:any)=>f.id===seed.sketch.id).sketch.support.status).toBe("FAILED_SUPPORT");
 expect(failed.part!.features.find((f:any)=>f.id===seed.child.id).evaluationStatus).toBe("BLOCKED");
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await open();dialog=page.getByRole("dialog",{name:"更换草图支撑",exact:true});
 await expect(dialog.getByText("原草图支撑已失效",{exact:true})).toBeVisible();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 // This hit must be the reconstructed prefix, even though the Body's final result is failed.
 await expect(async()=>{await clickWorld(page,[0,0,20]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"新支撑"}).getByText("已选上游实体平面",{exact:true})).toBeVisible({timeout:1000})}).toPass({timeout:20_000});
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();expect((await current(page,seed.id)).document.versionId).toBe(failed.document.versionId);
 await page.screenshot({path:testInfo.outputPath("support-face-repair.png")});
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const repaired=await current(page,seed.id),sketch=repaired.part.features.find((f:any)=>f.id===seed.sketch.id),child=repaired.part.features.find((f:any)=>f.id===seed.child.id);
 expect(sketch).toMatchObject({id:seed.sketch.id,name:seed.sketch.name,bodyId:seed.sketch.bodyId,order:seed.sketch.order});
 expect(sketch.sketch.entities).toEqual(seed.sketch.sketch!.entities);expect(sketch.sketch.constraints).toEqual(seed.sketch.sketch!.constraints);
 expect(sketch.sketch.support).toMatchObject({type:"PLANAR_FACE",sourceVersionId:failed.document.versionId,sourceFeatureId:seed.tip.id,status:"CONNECTED"});
 expect(child.profileSelection).toEqual(seed.child.profileSelection);expect(child.evaluationStatus).toBeFalsy();
 const head=repaired.document.versionId;
 await page.getByRole("button",{name:"撤销",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.sketch.id).sketch.support.status).toBe("FAILED_SUPPORT")).toPass();
 await page.getByRole("button",{name:"重做",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.sketch.id).sketch.support.status).toBe("CONNECTED")).toPass();
 expect((await current(page,seed.id)).document.versionId).not.toBe(head);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("sketch support without upstream solids offers datum planes without an unavailable warning",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Sketch support empty upstream regression");
  v=await api.command(v.document.id,{type:"CREATE_SKETCH",plane:"XY"});
  const sketch=v.part!.features[0];
  const input=await api.getFeatureInput(v.document.id,{versionId:v.document.versionId,featureId:sketch.id,bodyId:sketch.bodyId});
  if(input.artifact?.geometryKey||input.artifacts?.length)throw new Error("fixture unexpectedly has upstream geometry");
  return {id:v.document.id,version:v.document.versionId,name:sketch.name!};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.name);
 const response=page.waitForResponse(r=>r.url().includes(`/api/documents/${seed.id}/feature-input`)&&r.request().method()==="POST");
 await page.getByRole("treeitem").filter({hasText:seed.name}).last().click({button:"right"});
 await page.getByRole("menuitem",{name:"对象",exact:true}).hover();
 await page.getByRole("menuitem",{name:"更换草图支撑…",exact:true}).click();
 expect((await response).ok()).toBeTruthy();
 const dialog=page.getByRole("dialog",{name:"更换草图支撑",exact:true});await expect(dialog).toBeVisible();
 await expect(dialog.getByRole("alert")).toHaveCount(0);
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill("XZ Plane");
 await page.getByRole("treeitem").filter({hasText:"XZ Plane"}).last().click();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();
 await expect(dialog.getByRole("alert")).toHaveCount(0);
 await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 expect(errors).toEqual([]);
});

test("closed sketch after boundary replacement offers explicit region repair and restores the original extrusion",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","Reclosed profile repair acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=v.part!.features[0].id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:-20,y:-20},second:{x:20,y:20}}]});
  const before=v.part!.features[0].sketch!,edge=before.entities[0],regionIds=v.sketchAnalyses![sketchId].regionIds!;
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"ADD",length:5,profileSelection:{mode:"REGIONS",regionIds}});const feature=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"DELETE_ENTITIES",entityIds:[edge.id]}]});
  if(v.sketchAnalyses![sketchId].status!=="OPEN"||v.part!.features.at(-1)!.evaluationStatus!=="FAILED")throw new Error("fixture did not fail after deletion");
  const replacement={...edge,id:"reclosed-edge",startPointId:undefined,endPointId:undefined};
  const operations:any[]=[{type:"ADD_ENTITY",entity:replacement}];
  for(const old of before.constraints){if(old.kind!=="COINCIDENT"||!old.references.some(r=>r.entityId===edge.id))continue;
   operations.push({type:"ADD_CONSTRAINT",constraint:{...old,id:`reclose-coincident-${operations.length}`,references:old.references.map(r=>r.entityId===edge.id?{...r,entityId:replacement.id,pointId:undefined}:r)}});
  }
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations});
  if(v.sketchAnalyses![sketchId].status!=="CLOSED"||!v.part!.features.at(-1)!.diagnostic?.includes("PROFILE_REGION_MISSING"))throw new Error("fixture did not separate closed geometry from saved region identity");
  return {id,sketchId,sketchName:v.part!.features[0].name!,feature,head:v.document.versionId,regionIds:v.sketchAnalyses![sketchId].regionIds};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await expect(page.getByText(/草图已闭合，但 .* 的原轮廓区域已失效/)).toBeVisible();
 // Enter the actual sketch editor, then follow the repair action back into the
 // existing Feature editor; leaving sketch mode must not write a Revision.
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.sketchName);
 await page.getByRole("treeitem").filter({hasText:seed.sketchName}).first().dblclick();
 await page.getByRole("button",{name:"结束草图并修复区域",exact:true}).click();
 const dialog=page.getByRole("dialog",{name:"编辑 拉伸",exact:true});await expect(dialog).toBeVisible();
 await expect(dialog.getByText("区域输入需要修复",{exact:true})).toBeVisible();
 await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await dialog.getByRole("button",{name:"选择区域 1",exact:true}).click();await clickWorld(page,[0,0,0]);
 await ready(page,dialog);expect((await current(page,seed.id)).document.versionId).toBe(seed.head);
 await page.screenshot({path:testInfo.outputPath("reclosed-region-repair.png")});
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const repaired=await current(page,seed.id),feature=repaired.part.features.find((f:any)=>f.id===seed.feature.id);
 expect(feature).toMatchObject({id:seed.feature.id,bodyId:seed.feature.bodyId,name:seed.feature.name,order:seed.feature.order});
 expect(feature.evaluationStatus).toBeFalsy();expect(feature.profileSelection.regionIds).toEqual(seed.regionIds);
 expect(repaired.artifacts[repaired.part.bodies.find((b:any)=>b.id===feature.bodyId).geometryKey].volume).toBeCloseTo(8000,5);
 await expect(page.getByRole("button",{name:"修复轮廓区域",exact:true})).toHaveCount(0);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("to-plane extent picks datum and upstream face, cancels and repairs a lost target",async({page},testInfo)=>{
 page.setDefaultTimeout(15_000);
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let v=await api.createDocument("PART","To-plane viewport acceptance");const id=v.document.id;
  const rectangle=async(x:number,y:number,w:number,h:number)=>{
   v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const s=v.part!.features.at(-1)!.id;
   v=await api.command(id,{type:"EDIT_SKETCH",sketchId:s,operations:[{type:"ADD_RECTANGLE",first:{x,y},second:{x:x+w,y:y+h}}]});return s;
  };
  let s=await rectangle(-20,-10,15,20);v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:s,generator:"LINEAR_EXTRUDE",operation:"ADD",length:10});const base=v.part!.features.at(-1)!;
  s=await rectangle(-15,-5,5,10);v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:s,generator:"LINEAR_EXTRUDE",operation:"ADD",length:20});const tip=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"终止目标",origin:[0,0,10],normal:[-.2,0,1],uDirection:[1,0,.2]});const datum=v.part!.datumPlanes.at(-1)!;
  v=await api.command(id,{type:"CREATE_BODY"});const body=v.part!.activeBodyId;
  const sketch=await rectangle(5,-5,10,10);
  return {id,base,tip,datum,body,sketch,version:v.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"顶视图",exact:true}).click();
 const box=await page.locator("canvas").boundingBox();await page.mouse.move(box!.x+box!.width/2,box!.y+box!.height/2);await page.mouse.wheel(0,700);
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 const datumPick=async()=>{await filter.fill(seed.datum.name);await page.getByRole("treeitem").filter({hasText:seed.datum.name}).last().click();};
 const create=async()=>{
  await filter.fill("");await openCommand(page,"拉伸");const dialog=page.getByRole("dialog",{name:"创建 拉伸",exact:true});
  await clickWorld(page,[10,0,0]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"轮廓"}).getByText("1 项",{exact:true})).toBeVisible();
  await dialog.getByText("到平面",{exact:true}).click();await expect(dialog.getByRole("textbox",{name:"长度（mm）",exact:true})).toHaveCount(0);return dialog;
 };
 let dialog=await create();await datumPick();await ready(page,dialog);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await page.screenshot({path:testInfo.outputPath("to-plane-inclined-datum.png")});
 await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 dialog=await create();
 await expect(async()=>{await clickWorld(page,[-18,0,10]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"终止平面"}).getByText("已选择上游实体平面",{exact:true})).toBeVisible({timeout:1000})}).toPass({timeout:20_000});
 await ready(page,dialog);await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const created=await current(page,seed.id),feature=created.part.features.at(-1);
 expect(feature.extent).toBe("TO_PLANE");expect(feature.terminationPlane.sourceFeatureId).toBe(seed.tip.id);expect(feature.terminationPlane.selection.anchor.featureId).toBe(seed.base.id);
 const volume=(v:any)=>v.artifacts[v.part.bodies.find((b:any)=>b.id===seed.body).geometryKey].volume;
 expect(volume(created)).toBeCloseTo(1000,5);
 const failed=await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");const v=await api.getDocument(seed.id);
  const find=(n:any):any=>n.entityId===seed.base.id&&n.definitionDigest?n:n.children?.map(find).find(Boolean);
  return api.command(seed.id,{type:"SET_FEATURE_SUPPRESSION",targetId:seed.base.id,expectedFeatureDigest:find(v.structureTree).definitionDigest,suppressed:true});
 },seed);
 expect(failed.part!.features.find((f:any)=>f.id===feature.id).evaluationStatus).toBe("FAILED");
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"修复终止平面",exact:true}).click();
 dialog=page.getByRole("dialog",{name:"编辑 拉伸",exact:true});await expect(dialog.getByText("终止平面需要修复",{exact:true})).toBeVisible();await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await dialog.getByRole("button",{name:"选择终止平面",exact:true}).click();
 await expect(async()=>{await clickWorld(page,[-12,0,20]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"终止平面"}).getByText("已选择上游实体平面",{exact:true})).toBeVisible({timeout:1000})}).toPass({timeout:20_000});
 await ready(page,dialog);expect((await current(page,seed.id)).document.versionId).toBe(failed.document.versionId);
 await page.screenshot({path:testInfo.outputPath("to-plane-face-repair.png")});
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const repaired=await current(page,seed.id),result=repaired.part.features.find((f:any)=>f.id===feature.id);
 expect(result).toMatchObject({id:feature.id,name:feature.name,bodyId:feature.bodyId,order:feature.order,profileSelection:feature.profileSelection});
 expect(result.terminationPlane.sourceVersionId).toBe(failed.document.versionId);expect(result.terminationPlane.sourceFeatureId).toBe(seed.tip.id);expect(result.evaluationStatus).toBeFalsy();expect(volume(repaired)).toBeCloseTo(2000,5);
 await page.getByRole("button",{name:"撤销",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features.find((f:any)=>f.id===feature.id).evaluationStatus).toBe("FAILED")).toPass();
 await page.getByRole("button",{name:"重做",exact:true}).click();await expect(async()=>expect(volume(await current(page,seed.id))).toBeCloseTo(2000,5)).toPass();
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await filter.fill(feature.name);await page.getByRole("treeitem").filter({hasText:feature.name}).first().dblclick();
 dialog=page.getByRole("dialog",{name:"编辑 拉伸",exact:true});await ready(page,dialog);
 await dialog.getByRole("button",{name:"选择终止平面",exact:true}).click();await datumPick();await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const inclined=await current(page,seed.id);expect(volume(inclined)).toBeCloseTo(1200,5);expect(inclined.part.features.find((f:any)=>f.id===feature.id).terminationPlaneId).toBe(seed.datum.id);
 expect(errors).toEqual([]);
});

test("to-plane cut selects a hole region and reverse datum termination",async({page},testInfo)=>{
 page.setDefaultTimeout(15_000);
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","To-plane cut acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const base=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:base,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:base,generator:"LINEAR_EXTRUDE",operation:"ADD",length:30});const body=v.part!.activeBodyId;
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"切除起始面",origin:[0,0,30],normal:[0,0,1],uDirection:[1,0,0]});const start=v.part!.datumPlanes.at(-1)!;
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"切除终止面",origin:[0,0,20],normal:[-.25,0,1],uDirection:[1,0,.25]});const end=v.part!.datumPlanes.at(-1)!;
  v=await api.command(id,{type:"CREATE_SKETCH",datumPlaneId:start.id});const sketch=v.part!.features.at(-1)!.id;
  const circle=(id:string,r:number)=>({type:"ADD_ENTITY",entity:{id,kind:"CIRCLE",role:"PROFILE",center:{x:0,y:0},radius:r}});
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:sketch,operations:[circle("outer",4),circle("hole",2)]});return {id,body,end,version:v.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"顶视图",exact:true}).click();
 const box=await page.locator("canvas").boundingBox();await page.mouse.move(box!.x+box!.width/2,box!.y+box!.height/2);await page.mouse.wheel(0,700);await openCommand(page,"切除");
 const dialog=page.getByRole("dialog",{name:"创建 拉伸",exact:true});await clickWorld(page,[3,0,30]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"轮廓"}).getByText("1 项",{exact:true})).toBeVisible();
 await dialog.getByText("到平面",{exact:true}).click();await page.getByRole("textbox",{name:"筛选模型结构"}).fill(seed.end.name);await page.getByRole("treeitem").filter({hasText:seed.end.name}).last().click();
 const reversed=dialog.getByRole("switch",{name:"反向",exact:true});if(!await reversed.isChecked())await reversed.click();
 await ready(page,dialog);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await page.screenshot({path:testInfo.outputPath("to-plane-reverse-cut-hole.png")});
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const v=await current(page,seed.id),feature=v.part.features.at(-1);expect(feature).toMatchObject({extent:"TO_PLANE",operation:"REMOVE",reversed:true,terminationPlaneId:seed.end.id});
 expect(v.artifacts[v.part.bodies.find((b:any)=>b.id===seed.body).geometryKey].volume).toBeCloseTo(12000-120*Math.PI,5);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("Hole picks one explicit point, edits, repairs and keeps mouth chamfer editable",async({page},testInfo)=>{
 page.setDefaultTimeout(15_000);const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Independent Hole acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const base=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:base,operations:[{type:"ADD_RECTANGLE",first:{x:-20,y:-15},second:{x:20,y:15}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:base,generator:"LINEAR_EXTRUDE",operation:"ADD",length:20});const body=v.part!.activeBodyId;
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"孔定位面",origin:[0,0,20],normal:[0,0,1],uDirection:[1,0,0]});const plane=v.part!.datumPlanes.at(-1)!.id;
  v=await api.command(id,{type:"CREATE_SKETCH",datumPlaneId:plane});const sketch=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:sketch,operations:[{type:"ADD_ENTITY",entity:{id:"p",kind:"POINT",role:"CONSTRUCTION",point:{x:-5,y:0}}},{type:"ADD_ENTITY",entity:{id:"q",kind:"POINT",role:"CONSTRUCTION",point:{x:5,y:0}}}]});
  return {id,body,sketch,plane,version:v.document.versionId,featureCount:v.part!.features.length};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"顶视图",exact:true}).click();
 const box=await page.locator("canvas").boundingBox();await page.mouse.move(box!.x+box!.width/2,box!.y+box!.height/2);await page.mouse.wheel(0,700);
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 const open=async()=>{await openCommand(page,"孔");const dialog=page.getByRole("dialog",{name:"创建 孔",exact:true});await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();return dialog;};
 let dialog=await open();await clickWorld(page,[-5,0,20]);await expect(dialog.getByRole("button",{name:/定位点：.*已选择显式点/})).toBeVisible();
 await dialog.getByRole("textbox",{name:"直径（mm）",exact:true}).fill("6");await dialog.getByRole("textbox",{name:"深度（mm）",exact:true}).fill("8");await ready(page,dialog);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await page.screenshot({path:testInfo.outputPath("hole-blind-preview.png")});
 await dialog.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 dialog=await open();await clickWorld(page,[-5,0,20]);await dialog.getByRole("textbox",{name:"直径（mm）",exact:true}).fill("6");await dialog.getByRole("textbox",{name:"深度（mm）",exact:true}).fill("8");await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const created=await current(page,seed.id),hole=created.part.features.at(-1);
 const volume=(v:any)=>v.artifacts[v.part.bodies.find((b:any)=>b.id===seed.body).geometryKey].volume;
 expect(created.part.features).toHaveLength(seed.featureCount+1);expect(hole).toMatchObject({type:"HOLE",operation:"REMOVE",diameter:6,depth:8,holePoint:{sketchId:seed.sketch,reference:{target:"ENTITY",entityId:"p",subElement:"POINT"}}});expect(hole.profile).toBeUndefined();expect(volume(created)).toBeCloseTo(24000-72*Math.PI,5);
 const edit=async()=>{await filter.fill(hole.name);await page.getByRole("treeitem").filter({hasText:hole.name}).first().dblclick();const d=page.getByRole("dialog",{name:"编辑 孔",exact:true});await expect(d).toBeVisible();return d;};
 dialog=await edit();await ready(page,dialog);await dialog.getByRole("textbox",{name:"直径（mm）",exact:true}).fill("8");await dialog.getByRole("textbox",{name:"深度（mm）",exact:true}).fill("5");await ready(page,dialog);
 await dialog.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(created.document.versionId);
 dialog=await edit();await ready(page,dialog);await dialog.getByRole("button",{name:/定位点：/}).click();await clickWorld(page,[5,0,20]);await ready(page,dialog);
 await dialog.getByText("单向贯穿",{exact:true}).click();await expect(dialog.getByRole("textbox",{name:"深度（mm）",exact:true})).toHaveCount(0);await ready(page,dialog);
 const reversed=dialog.getByRole("switch",{name:"反向",exact:true});await reversed.click();await expect(dialog.getByRole("alert")).toContainText(/NO_MATERIAL|HOLE_NO_MATERIAL/);await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(created.document.versionId);await reversed.click();await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const through=await current(page,seed.id);expect(volume(through)).toBeCloseTo(24000-180*Math.PI,5);expect(through.part.features.find((f:any)=>f.id===hole.id)).toMatchObject({extent:"THROUGH_ALL",holePoint:{reference:{entityId:"q"}}});expect(through.part.parameters.filter((p:any)=>p.ownerFeatureId===hole.id).map((p:any)=>p.propertySlot)).toEqual(["diameter"]);
 const failed=await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");return api.command(seed.id,{type:"EDIT_SKETCH",sketchId:seed.sketch,operations:[{type:"DELETE_ENTITIES",entityIds:["q"]}]});
 },seed);
 expect(failed.part!.features.find((f:any)=>f.id===hole.id).diagnostic).toContain("HOLE_POINT_MISSING");
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"修复孔位",exact:true}).click();dialog=page.getByRole("dialog",{name:"编辑 孔",exact:true});await expect(dialog.getByText("孔位需要修复",{exact:true})).toBeVisible();await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeDisabled();
 await dialog.getByRole("button",{name:/定位点：/}).click();await clickWorld(page,[-5,0,20]);await ready(page,dialog);await page.screenshot({path:testInfo.outputPath("hole-point-repair.png")});
 await dialog.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(failed.document.versionId);
 await page.getByRole("button",{name:"修复孔位",exact:true}).click();dialog=page.getByRole("dialog",{name:"编辑 孔",exact:true});await dialog.getByRole("button",{name:/定位点：/}).click();await clickWorld(page,[-5,0,20]);await ready(page,dialog);await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const repaired=await current(page,seed.id),restored=repaired.part.features.find((f:any)=>f.id===hole.id);expect(restored).toMatchObject({id:hole.id,name:hole.name,bodyId:hole.bodyId,order:hole.order,holePoint:{reference:{entityId:"p"}}});expect(restored.evaluationStatus).toBeFalsy();
 await page.getByRole("button",{name:"撤销",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features.find((f:any)=>f.id===hole.id).evaluationStatus).toBe("FAILED")).toPass();await page.getByRole("button",{name:"重做",exact:true}).click();await expect(async()=>expect(volume(await current(page,seed.id))).toBeCloseTo(24000-180*Math.PI,5)).toPass();
 await filter.fill("");await openCommand(page,"倒角");const chamfer=page.getByRole("dialog",{name:"创建 倒角",exact:true});await clickWorld(page,[-5+3*Math.SQRT1_2,3*Math.SQRT1_2,20]);await expect(chamfer.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await chamfer.getByRole("textbox").fill("0.5");await ready(page,chamfer);await chamfer.getByRole("button",{name:/确\s*定/}).click();await expect(chamfer).toHaveCount(0);
 const finished=await current(page,seed.id),child=finished.part.features.at(-1);expect(child.type).toBe("CHAMFER");expect(child.selections[0].selection.anchor.featureId).toBe(hole.id);
 dialog=await edit();await ready(page,dialog);await dialog.getByRole("textbox",{name:"直径（mm）",exact:true}).fill("7");await ready(page,dialog);await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 expect((await current(page,seed.id)).part.features.find((f:any)=>f.id===child.id).evaluationStatus).toBeFalsy();await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("Hole support reassignment and both through directions protect the opposite half",async({page},testInfo)=>{
 page.setDefaultTimeout(15_000);await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Directional Hole support acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const base=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:base,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:base,generator:"LINEAR_EXTRUDE",operation:"ADD",length:20});const body=v.part!.activeBodyId;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketch=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:sketch.id,operations:[{type:"ADD_ENTITY",entity:{id:"center",kind:"POINT",role:"CONSTRUCTION",point:{x:0,y:0}}}]});
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",name:"孔中间面",origin:[0,0,10],normal:[0,0,1],uDirection:[1,0,0]});
  return {id,body,sketch,plane:v.part!.datumPlanes.at(-1)!,version:v.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"顶视图",exact:true}).click();const box=await page.locator("canvas").boundingBox();await page.mouse.move(box!.x+box!.width/2,box!.y+box!.height/2);await page.mouse.wheel(0,700);
 await openCommand(page,"孔");let dialog=page.getByRole("dialog",{name:"创建 孔",exact:true});await clickWorld(page,[0,0,0]);await dialog.getByRole("textbox",{name:"直径（mm）",exact:true}).fill("4");await dialog.getByText("单向贯穿",{exact:true}).click();await dialog.getByRole("switch",{name:"反向",exact:true}).click();await ready(page,dialog);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);const created=await current(page,seed.id),hole=created.part.features.at(-1);
 const volume=(v:any)=>v.artifacts[v.part.bodies.find((b:any)=>b.id===seed.body).geometryKey].volume;expect(volume(created)).toBeCloseTo(8000-80*Math.PI,5);
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});await filter.fill(seed.sketch.name!);await page.getByRole("treeitem",{name:new RegExp(`scissor ${seed.sketch.name}$`)}).click({button:"right"});await page.getByRole("menuitem",{name:"对象",exact:true}).hover();await page.getByRole("menuitem",{name:"更换草图支撑…",exact:true}).click();
 const support=page.getByRole("dialog",{name:"更换草图支撑",exact:true});await expect(support).toBeVisible();await filter.fill(seed.plane.name);await page.getByRole("treeitem").filter({hasText:seed.plane.name}).last().click();await expect(support.getByRole("button",{name:/确\s*定/})).toBeEnabled();expect((await current(page,seed.id)).document.versionId).toBe(created.document.versionId);
 await support.getByRole("button",{name:/确\s*定/}).click();await expect(support).toHaveCount(0);const moved=await current(page,seed.id);expect(volume(moved)).toBeCloseTo(8000-40*Math.PI,5);expect(moved.part.features.find((f:any)=>f.id===hole.id).holePoint).toEqual(hole.holePoint);
 await filter.fill(hole.name);await page.getByRole("treeitem").filter({hasText:hole.name}).first().dblclick();dialog=page.getByRole("dialog",{name:"编辑 孔",exact:true});await ready(page,dialog);await dialog.getByRole("switch",{name:"反向",exact:true}).click();await ready(page,dialog);await page.screenshot({path:testInfo.outputPath("hole-reversed-half-through.png")});await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const reverse=await current(page,seed.id);expect(volume(reverse)).toBeCloseTo(8000-40*Math.PI,5);expect(reverse.part.bodies.find((b:any)=>b.id===seed.body).geometryKey).not.toBe(moved.part.bodies.find((b:any)=>b.id===seed.body).geometryKey);
 // The opposite half still accepts a full blind tool through the real API/Worker.
 const probe=await page.evaluate(async({seed,hole})=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");return api.previewCommand(seed.id,{type:"CREATE_SOLID_FEATURE",feature:{...hole,id:"",extent:"BLIND",depth:3,reversed:false}});
 },{seed,hole});expect(probe.artifact.volume).toBeCloseTo(8000-52*Math.PI,5);expect((await current(page,seed.id)).document.versionId).toBe(reverse.document.versionId);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();
});

test("constrained feature move previews tree order, restores history and rejects source crossing",async({page},testInfo)=>{
 page.setDefaultTimeout(15_000);const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Constrained feature move acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const profile=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:profile,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:profile,generator:"LINEAR_EXTRUDE",operation:"ADD",length:20});const base=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"CREATE_DATUM_PLANE",origin:[0,0,20],normal:[0,0,1],uDirection:[1,0,0]});
  v=await api.command(id,{type:"CREATE_SKETCH",datumPlaneId:v.part!.datumPlanes.at(-1)!.id});const location=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:location,operations:[{type:"ADD_ENTITY",entity:{id:"center",kind:"POINT",role:"CONSTRUCTION",point:{x:0,y:0}}}]});
  const key=v.part!.bodies.find(b=>b.id===base.bodyId)!.geometryKey!,artifact=v.artifacts[key];let selection;
  for(let i=1;i<=Number(artifact.topology.edges);i++){const p=await api.getTopologyProperties(id,key,"EDGE",i,v.document.versionId);if(p.persistentSelection?.anchor.featureId===base.id&&p.persistentSelection.anchor.outputSlot.startsWith("VERTICAL_FROM_PROFILE_ENDPOINTS/")){selection=p.persistentSelection;break;}}
  if(!selection)throw Error("outer edge unavailable");
  v=await api.command(id,{type:"CREATE_MODIFY_FEATURE",feature:{type:"FILLET",bodyId:base.bodyId,length:1,selections:[{selection,sourceVersionId:v.document.versionId}]}});const fillet=v.part!.features.at(-1)!;
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",feature:{type:"HOLE",bodyId:base.bodyId,operation:"REMOVE",extent:"BLIND",diameter:4,depth:5,reversed:true,holePoint:{sketchId:location,reference:{target:"ENTITY",entityId:"center",subElement:"POINT"}}}});const hole=v.part!.features.at(-1)!;
  return {id,base,fillet,hole,version:v.document.versionId,ids:v.part!.features.map(f=>f.id),volume:v.artifacts[v.part!.bodies.find(b=>b.id===base.bodyId)!.geometryKey!].volume};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 const open=async(name:string)=>{await filter.fill(name);await page.getByRole("treeitem").filter({hasText:name}).first().click({button:"right"});await page.getByRole("menuitem",{name:"对象",exact:true}).hover();await page.getByRole("menuitem",{name:"调整特征顺序",exact:true}).click();const d=page.getByRole("dialog",{name:"调整特征顺序",exact:true});await expect(d).toBeVisible();return d;};
 const anchor=async(d:ReturnType<Page["getByRole"]>,name:string)=>{await d.getByRole("combobox",{name:"锚点特征"}).click();await page.locator(".ant-select-item-option-content").filter({hasText:name}).click();};
 const confirm=async(d:ReturnType<Page["getByRole"]>)=>{await ready(page,d);await d.getByRole("button",{name:/确\s*定/}).click();await expect(d).toHaveCount(0);};
 let d=await open(seed.hole.name!);await anchor(d,seed.fillet.name!);await ready(page,d);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await page.screenshot({path:testInfo.outputPath("feature-move-preview.png")});
 await d.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 d=await open(seed.hole.name!);await anchor(d,seed.fillet.name!);await confirm(d);
 const first=await current(page,seed.id),firstIds=first.part.features.map((f:any)=>f.id);expect(firstIds).toEqual([...seed.ids.slice(0,-2),seed.hole.id,seed.fillet.id]);
 expect(first.artifacts[first.part.bodies.find((b:any)=>b.id===seed.base.bodyId).geometryKey].volume).toBeCloseTo(seed.volume,5);
 await filter.fill("");const bodyNode=page.getByRole("treeitem").filter({hasText:"Body.1 · 工作中"});if(await bodyNode.getByRole("button",{name:"展开",exact:true}).count())await bodyNode.getByRole("button",{name:"展开",exact:true}).click();await expect(async()=>{const labels=await page.locator(".specification-tree-label").allTextContents();expect(labels).toContain(seed.hole.name!);expect(labels).toContain(seed.fillet.name!);expect(labels.indexOf(seed.hole.name!)).toBeLessThan(labels.indexOf(seed.fillet.name!));}).toPass();
 const stage=await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");const v=await api.getDocument(seed.id);return api.getFeatureInput(seed.id,{versionId:v.document.versionId,featureId:seed.hole.id,resultStage:true});
 },seed);expect(stage.artifact.volume).toBeCloseTo(8000-20*Math.PI,5);
 d=await open(seed.hole.name!);await anchor(d,seed.fillet.name!);await d.getByText("之后",{exact:true}).click();await confirm(d);
 for(const expected of [firstIds,seed.ids]){await page.getByRole("button",{name:"撤销",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features.map((f:any)=>f.id)).toEqual(expected)).toPass();}
 for(const expected of [firstIds,seed.ids]){await page.getByRole("button",{name:"重做",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features.map((f:any)=>f.id)).toEqual(expected)).toPass();}
 await filter.fill(seed.hole.name!);await page.getByRole("treeitem").filter({hasText:seed.hole.name!}).first().dblclick();d=page.getByRole("dialog",{name:"编辑 孔",exact:true});await ready(page,d);await d.getByRole("textbox",{name:"直径（mm）",exact:true}).fill("5");await confirm(d);
 // A real downstream mouth reference must block crossing its Hole source.
 const child=await page.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.getDocument(seed.id);const key=v.part!.bodies.find(b=>b.id===seed.base.bodyId)!.geometryKey!,a=v.artifacts[key];let selection;
  for(let i=1;i<=Number(a.topology.edges);i++){const p=await api.getTopologyProperties(seed.id,key,"EDGE",i,v.document.versionId);if(p.persistentSelection?.anchor.featureId===seed.hole.id&&p.persistentSelection.anchor.outputSlot.startsWith("START_BOUNDARY_FROM_PROFILE_EDGE/")){selection=p.persistentSelection;break;}}
  if(!selection)throw Error("mouth unavailable");v=await api.command(seed.id,{type:"CREATE_MODIFY_FEATURE",feature:{type:"CHAMFER",bodyId:seed.base.bodyId,length:.2,selections:[{selection,sourceVersionId:v.document.versionId}]}});return {feature:v.part!.features.at(-1)!,version:v.document.versionId};
 },seed);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();d=await open(child.feature.name!);await anchor(d,seed.hole.name!);await expect(d.getByRole("alert")).toContainText(child.feature.id);await expect(d.getByRole("button",{name:/确\s*定/})).toBeDisabled();await expect(page.locator("[data-feature-preview]")).toHaveCount(0);expect((await current(page,seed.id)).document.versionId).toBe(child.version);await page.screenshot({path:testInfo.outputPath("feature-move-source-blocked.png")});await d.getByRole("button",{name:/取\s*消/}).click();
 await filter.fill(seed.base.name!);await page.getByRole("treeitem").filter({hasText:seed.base.name!}).first().click({button:"right"});await expect(page.getByRole("menu").first()).toBeVisible();const objectMenu=page.getByRole("menuitem",{name:"对象",exact:true});if(await objectMenu.count())await objectMenu.hover();await expect(page.getByRole("menuitem",{name:"调整特征顺序",exact:true})).toHaveCount(0);await page.keyboard.press("Escape");
 await page.reload();await expect(page.locator("canvas")).toBeVisible();const final=await current(page,seed.id);expect(final.part.features.find((f:any)=>f.id===seed.hole.id)).toMatchObject({id:seed.hole.id,bodyId:seed.hole.bodyId,name:seed.hole.name,diameter:5});expect(final.part.features.every((f:any)=>!f.evaluationStatus)).toBeTruthy();expect(errors).toEqual([]);
});

test("fillet keeps untouched opposite edge owned by extrusion during real picking",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Fillet edge ancestry acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketch=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:sketch,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:sketch,generator:"LINEAR_EXTRUDE",operation:"ADD",length:20});const base=v.part!.features.at(-1)!;
  const key=v.part!.bodies[0].geometryKey!;let selection;
  for(let i=1;i<=Number(v.artifacts[key].topology.edges);i++){const p=await api.getTopologyProperties(id,key,"EDGE",i,v.document.versionId);if(p.persistentSelection?.creationEvidence?.centroid?.every((x:number,j:number)=>Math.abs(x-[10,10,10][j])<1e-7)){selection=p.persistentSelection;break;}}
  if(!selection)throw Error("round input missing");v=await api.command(id,{type:"CREATE_MODIFY_FEATURE",feature:{type:"FILLET",bodyId:base.bodyId,length:1,selections:[{selection,sourceVersionId:v.document.versionId}]}});
  return {id,base,fillet:v.part!.features.at(-1)!,version:v.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await openCommand(page,"倒角");let dialog=page.getByRole("dialog",{name:"创建 倒角",exact:true});
 await clickWorld(page,[10,-10,10]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await dialog.getByRole("textbox").fill("0.2");await ready(page,dialog);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await page.screenshot({path:testInfo.outputPath("extrude-edge-after-fillet-pick.png")});await dialog.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await openCommand(page,"倒角");dialog=page.getByRole("dialog",{name:"创建 倒角",exact:true});await clickWorld(page,[10,-10,10]);await expect(dialog.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await dialog.getByRole("textbox").fill("0.2");await ready(page,dialog);await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const v=await current(page,seed.id),chamfer=v.part.features.at(-1);expect(chamfer.selections[0].selection.anchor.featureId).toBe(seed.base.id);expect(chamfer.selections[0].selection.anchor.outputSlot).toMatch(/^VERTICAL_FROM_PROFILE_ENDPOINTS\//);expect(chamfer.evaluationStatus).toBeFalsy();await page.reload();await expect(page.locator("canvas")).toBeVisible();expect(errors).toEqual([]);
});

test("insert fillet after an exact base stage before existing chamfer in one history step",async({page},testInfo)=>{
 page.setDefaultTimeout(15_000);const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Insert after solid stage acceptance");const id=v.document.id;
  v=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketch=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:"EDIT_SKETCH",sketchId:sketch,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId:sketch,generator:"LINEAR_EXTRUDE",operation:"ADD",length:20});const base=v.part!.features.at(-1)!;
  const key=v.part!.bodies[0].geometryKey!;let selection;
  for(let i=1;i<=Number(v.artifacts[key].topology.edges);i++){const p=await api.getTopologyProperties(id,key,"EDGE",i,v.document.versionId);if(p.persistentSelection?.creationEvidence?.centroid?.every((x:number,j:number)=>Math.abs(x-[10,10,10][j])<1e-7)){selection=p.persistentSelection;break;}}
  if(!selection)throw Error("chamfer seed edge missing");v=await api.command(id,{type:"CREATE_MODIFY_FEATURE",feature:{type:"CHAMFER",bodyId:base.bodyId,length:1,selections:[{selection,sourceVersionId:v.document.versionId}]}});
  return {id,base,chamfer:v.part!.features.at(-1)!,version:v.document.versionId,features:v.part!.features,parameters:v.part!.parameters};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 const open=async(kind="圆角")=>{await filter.fill(seed.base.name!);await page.getByRole("treeitem",{name:new RegExp(`insert-row-above ${seed.base.name}$`)}).click({button:"right"});await page.getByRole("menuitem",{name:"对象",exact:true}).hover();await page.getByRole("menuitem",{name:/^在此步骤后插入/}).hover();await page.getByRole("menuitem",{name:kind,exact:true}).click();const d=page.getByRole("dialog",{name:`创建 ${kind}`,exact:true});await expect(d).toBeVisible();await expect(d.getByText(`插入位置：Body.1 / ${seed.base.name} 之后`,{exact:true})).toBeVisible();return d;};
 let d=await open();await clickWorld(page,[10,-10,10]);await expect(d.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await d.getByRole("textbox").fill("1");await ready(page,d);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await page.screenshot({path:testInfo.outputPath("insertion-stage-preview.png")});await d.getByRole("button",{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await expect(page.locator("[data-feature-preview]")).toHaveCount(0);
 // A Revision change disposes the temporary insertion context and its picks.
 d=await open("倒角");const peer=await page.context().newPage();await peer.goto("/");const changed=await peer.evaluate(async seed=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");return api.command(seed.id,{type:"SET_ACTIVE_BODY",bodyId:seed.base.bodyId});
 },seed);expect(changed.document.versionId).not.toBe(seed.version);await expect(d).toHaveCount(0);await peer.close();
 const before=await current(page,seed.id);d=await open();await clickWorld(page,[10,-10,10]);await expect(d.locator(".feature-pick-field").filter({hasText:"边集"}).getByText("1 项",{exact:true})).toBeVisible();await d.getByRole("textbox").fill("1");await ready(page,d);await d.getByRole("button",{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 const committed=await current(page,seed.id),inserted=committed.part.features[2];expect(inserted.type).toBe("FILLET");expect(committed.part.features.at(-1).id).toBe(seed.chamfer.id);expect(inserted.selections[0]).toMatchObject({sourceVersionId:before.document.versionId,sourceFeatureId:seed.base.id,selection:{anchor:{featureId:seed.base.id}}});expect(committed.part.features.map((f:any)=>f.order)).toEqual([1,2,3,4]);
 const a=committed.artifacts[committed.part.bodies[0].geometryKey];expect(a.volume).toBeCloseTo(7990-20*(1-Math.PI/4),5);
 await filter.fill("");const bodyNode=page.getByRole("treeitem").filter({hasText:"Body.1 · 工作中"});if(await bodyNode.getByRole("button",{name:"展开",exact:true}).count())await bodyNode.getByRole("button",{name:"展开",exact:true}).click();await expect(async()=>{const labels=await page.locator(".specification-tree-label").allTextContents();expect(labels).toContain(inserted.name);expect(labels).toContain(seed.chamfer.name);expect(labels.indexOf(inserted.name)).toBeLessThan(labels.indexOf(seed.chamfer.name!));}).toPass({timeout:15_000});
 const stage=await page.evaluate(async({seed,inserted})=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");const v=await api.getDocument(seed.id);return api.getFeatureInput(seed.id,{versionId:v.document.versionId,featureId:inserted.id,resultStage:true});
 },{seed,inserted});expect(stage.artifact.volume).toBeCloseTo(8000-20*(1-Math.PI/4),5);
 await page.getByRole("button",{name:"撤销",exact:true}).click();await expect(async()=>{const v=await current(page,seed.id);expect(v.part.features).toEqual(before.part.features);expect(v.part.parameters).toEqual(before.part.parameters);}).toPass({timeout:15_000});await page.getByRole("button",{name:"重做",exact:true}).click();await expect(async()=>expect((await current(page,seed.id)).part.features[2].id).toBe(inserted.id)).toPass({timeout:15_000});
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect((await current(page,seed.id)).part.features.every((f:any)=>!f.evaluationStatus)).toBeTruthy();
 // ResultStage remains read-only; insertion is a separate authorized context.
 const illegal=await page.request.post(`/api/documents/${seed.id}/feature-input`,{data:{versionId:(await current(page,seed.id)).document.versionId,featureId:seed.base.id,resultStage:true,geometryKey:stage.artifact.geometryKey,kind:"EDGE",localId:1}});expect(illegal.ok()).toBeFalsy();expect(errors).toEqual([]);
});

test("merged loft side picks highlight the generating Feature rather than section sketches",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");let v=await api.createDocument("PART","Loft section ownership acceptance");const id=v.document.id,sections=[];
  for(const z of [0,30]){
   let datumPlaneId;
   if(z){v=await api.command(id,{type:"CREATE_DATUM_PLANE",origin:[0,0,z],normal:[0,0,1],uDirection:[1,0,0]});datumPlaneId=v.part!.datumPlanes.at(-1)!.id;}
   v=await api.command(id,{type:"CREATE_SKETCH",...(datumPlaneId?{datumPlaneId}:{plane:"XY"})});const sketchId=v.part!.features.at(-1)!.id;
   v=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:-30,y:-30},second:{x:30,y:30}}]});sections.push({sketchId});
  }
  v=await api.command(id,{type:"CREATE_SOLID_FEATURE",feature:{type:"LOFT",operation:"NEW_BODY",sections}});const loft=v.part!.features.at(-1)!,body=v.part!.bodies.find(b=>b.id===loft.bodyId)!,key=body.geometryKey!;
  let face;
  for(let i=1;i<=Number(v.artifacts[key].topology.faces);i++){const p=await api.getTopologyProperties(id,key,"FACE",i,v.document.versionId);if(p.persistentSelection?.anchor.outputSlot.startsWith("MERGED_FROM/")&&p.persistentSelection.creationEvidence!.direction[1]<-.9){face=p;break}}
  if(!face)throw Error("Expected a merged visible side from real Naming");return {id,loft,body,face,version:v.document.versionId,sections};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 const tree=page.getByRole("textbox",{name:"筛选模型结构"});await tree.fill("Loft 1");
 const row=page.locator(`[data-tree-key$="/feature:${seed.loft.id}"]`);
 const selected=()=>page.evaluate(async()=>{
  // @ts-ignore read-only observation of the real viewport selection
  const {useWorkbenchStore}=await import("/src/state/workbench-store.ts");return useWorkbenchStore.getState().selection;
 });
 const pick=async()=>{
  await expect(async()=>{await clickWorld(page,seed.face.persistentSelection.creationEvidence.centroid);expect((await selected())?.topologyId).toBe(seed.face.localId);}).toPass({timeout:20_000});
  await expect(row).toHaveClass(/selected-descendant/);
  const hit=await selected();expect(hit).toMatchObject({kind:"face",bodyId:seed.body.id,associatedFeatureIds:[seed.loft.id],sourceFeatureIds:[seed.loft.id]});expect(hit!.treeNodeId).toContain(`/feature:${seed.loft.id}`);
 };
 await pick();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await page.screenshot({path:testInfo.outputPath("loft-merged-face-feature-highlight.png")});
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await tree.fill("Loft 1");await pick();expect(errors).toEqual([]);
});

test("command forms keep field ownership across late picks, IME, expressions and history",async({page},testInfo)=>{
 page.setDefaultTimeout(20000);await login(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','命令表单交互验收');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'NEW_BODY',length:10});
  v=await api.command(id,{type:'CREATE_DATUM_PLANE',name:'安装支撑平面 · 长名称用于窄窗口定位检查',origin:[0,0,3],normal:[0,0,1],uDirection:[1,0,0]});
  return {id,version:v.document.versionId,plane:v.part!.datumPlanes.at(-1)!};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await worldScreen(page,[0,0,10]);
 await openCommand(page,'基准面');let dialog=page.getByRole('dialog',{name:'创建基准面',exact:true});
 const source=()=>dialog.locator('.feature-pick-field').filter({hasText:'源基准'});
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();const original=await source().locator('.feature-pick-value').innerText();
 let release=()=>{},started=false,fulfilled=false;const gate=new Promise<void>(r=>release=r);
 const pattern=`**/api/documents/${seed.id}/topology-properties?*`;
 await page.route(pattern,async route=>{const response=await route.fetch();if(new URL(route.request().url()).searchParams.get('kind')==='FACE'){started=true;await gate;}await route.fulfill({response});fulfilled=true;});
 await dialog.getByRole('button',{name:'选择源基准',exact:true}).click();await clickWorld(page,[-5,0,10]);await expect.poll(()=>started).toBeTruthy();
 await dialog.getByRole('button',{name:'选择旋转轴',exact:true}).click();release();await expect.poll(()=>fulfilled).toBeTruthy();
 await openCommand(page,'开发信息');const developer=page.getByRole('dialog',{name:'开发信息',exact:true});await expect(developer).toBeVisible();await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'选择旋转轴',exact:true})).toHaveAttribute('aria-pressed','true');await developer.getByRole('button',{name:'关闭',exact:true}).click();
 await expect(dialog.getByRole('button',{name:'选择旋转轴',exact:true})).toHaveAttribute('aria-pressed','true');await expect(source().locator('.feature-pick-value')).toHaveText(original);
 await page.unroute(pattern);await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await expect(dialog.locator('[data-pick-active]')).toHaveCount(0);
 await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await openCommand(page,'基准面');dialog=page.getByRole('dialog',{name:'创建基准面',exact:true});await dialog.getByRole('button',{name:'选择源基准',exact:true}).click();
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill(seed.plane.name);await page.getByRole('treeitem').filter({hasText:seed.plane.name}).last().click();
 await expect(source().locator('.feature-pick-value')).toHaveText(seed.plane.name);await expect(dialog.locator('[data-pick-active]')).toHaveCount(0);await filter.fill('');
 await dialog.getByRole('button',{name:'定位源基准',exact:true}).click();await expectViewport(viewportProbe(page,'viewport-selection')).toContainText(seed.plane.id);
 const distance=dialog.getByRole('textbox',{name:/平移距离/});await distance.fill('2 mm + 3 mm');await distance.blur();
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await distance.evaluate(el=>{el.dispatchEvent(new CompositionEvent('compositionstart',{bubbles:true}));el.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',isComposing:true,bubbles:true}));el.dispatchEvent(new CompositionEvent('compositionend',{bubbles:true}));});
 await expect(dialog).toBeVisible();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await page.setViewportSize({width:1024,height:768});await page.evaluate(()=>document.documentElement.style.zoom='1.25');
 await expect.poll(async()=>{const r=await dialog.boundingBox();return !!r&&r.x>=0&&r.y>=0&&r.x+r.width<=1025&&r.y+r.height<=769;}).toBeTruthy();
 await page.screenshot({path:'../../../build/acceptance/command-form-125-percent.png'});
 await page.evaluate(()=>document.documentElement.style.zoom='1');await page.setViewportSize({width:1440,height:1000});await distance.focus();await distance.press('Enter');await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id),plane=committed.part.datumPlanes.at(-1);expect(plane.origin[2]).toBeCloseTo(8,6);expect(plane.definition.source.entityId).toBe(seed.plane.id);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>((await current(page,seed.id)).part.datumPlanes.some((p:any)=>p.id===plane.id))).toBeFalsy();
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>((await current(page,seed.id)).part.datumPlanes.find((p:any)=>p.id===plane.id)?.origin[2])).toBe(8);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();expect((await current(page,seed.id)).part.datumPlanes.find((p:any)=>p.id===plane.id)).toEqual(plane);expect(errors).toEqual([]);
});

test("reference picking, persistent names and compact expressions use the real command chain",async({page},testInfo)=>{
 test.setTimeout(240_000);page.setDefaultTimeout(20_000);await login(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const seed=await page.evaluate(async()=>{
  const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','UI 参考与命名验收');const id=v.document.id;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketch=v.part!.features.at(-1)!.id;
  v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-10,y:-10},second:{x:10,y:10}}]});
  v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:20});const feature=v.part!.features.at(-1)!;
  v=await api.command(id,{type:'CREATE_PARAMETER',name:'len',value:20,unit:'mm'});const parameter=v.part!.parameters!.find(p=>p.key==='len')!.parameterId;
  v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});return {id,feature,sketch2:v.part!.features.at(-1)!,parameter,version:v.document.versionId};
 });
 expect(seed.feature.name).toBe('Extrude.1');expect(seed.sketch2.name).toBe('Sketch.2');
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();
 await worldScreen(page,[0,0,0]);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine,p=e.navigation.target.clone().set(0,0,0).project(e.camera),r=e.renderer.domElement.getBoundingClientRect();return e.solidBindings.size>0&&e.hitTest((p.x+1)*r.width/2,(1-p.y)*r.height/2)?.kind})).toBe('datum-point');const origin=await worldScreen(page,[0,0,0]);await page.mouse.move(origin.x,origin.y);await page.mouse.click(origin.x,origin.y);
 await expectViewport(viewportProbe(page,'viewport-selection')).toContainText('datum-point');await page.keyboard.press('Escape');await expectViewport(viewportProbe(page,'viewport-selection')).toContainText('none');
 // A plane's real preview and saved object use the same projected construction geometry.
 await openCommand(page,'基准面');let d=page.getByRole('dialog',{name:'创建基准面',exact:true});await d.getByRole('textbox',{name:/平移距离/}).fill('5');await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();
 const planeSpan=async(id?:string)=>page.evaluate(id=>{const e=(window as any).__cameraEngine;let mesh:any;if(id)e.scene.traverse((o:any)=>{if(o.isMesh&&o.userData.kind==='plane'&&o.userData.entityId===id)mesh=o});else e.datumPreviewGroup?.traverse((o:any)=>{if(o.isMesh&&o.geometry?.type==='PlaneGeometry')mesh=o});if(!mesh)return null;mesh.updateWorldMatrix(true,true);const a=mesh.geometry.attributes.position,r=e.renderer.domElement.getBoundingClientRect(),points=[];for(let i=0;i<a.count;i++){const p=e.navigation.target.clone().set(a.getX(i),a.getY(i),a.getZ(i)).applyMatrix4(mesh.matrixWorld).project(e.camera);points.push([p.x*r.width/2,p.y*r.height/2]);}return {width:Math.max(...points.map(p=>p[0]))-Math.min(...points.map(p=>p[0])),height:Math.max(...points.map(p=>p[1]))-Math.min(...points.map(p=>p[1]))};},id);
 await expect.poll(()=>planeSpan()).not.toBeNull();const beforeSpan=await planeSpan();expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 let v=await current(page,seed.id);const plane=v.part.datumPlanes.at(-1);expect(plane.name).toBe('Plane.1');await expect.poll(()=>planeSpan(plane.id)).not.toBeNull();const afterSpan=await planeSpan(plane.id);expect(afterSpan!.width).toBeCloseTo(beforeSpan!.width,1);expect(afterSpan!.height).toBeCloseTo(beforeSpan!.height,1);
 // Cancelling a new plane does not reserve a durable number.
 await openCommand(page,'基准面');d=page.getByRole('dialog',{name:'创建基准面',exact:true});await d.getByRole('button',{name:/取\s*消/}).click();await openCommand(page,'基准面');d=page.getByRole('dialog',{name:'创建基准面',exact:true});await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);expect((await current(page,seed.id)).part.datumPlanes.at(-1).name).toBe('Plane.2');
 await openCommand(page,'基准轴');d=page.getByRole('dialog',{name:'创建基准轴',exact:true});await expect(d.getByRole('button',{name:/确\s*定/})).toBeEnabled();await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);expect((await current(page,seed.id)).part.datumAxes.at(-1).name).toBe('Axis.1');
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});await filter.fill(seed.feature.name!);await page.locator(`[role=treeitem][data-tree-key$="pad:${seed.feature.id}"]`).dblclick();d=page.getByRole('dialog',{name:'编辑 拉伸',exact:true});await ready(page,d);const head=(await current(page,seed.id)).document.versionId;
 for(const text of ['len/2','len / 2','(len+2mm)/2','-(-len)*5e-1']){await d.getByRole('textbox',{name:/^长度/}).fill(text);await ready(page,d);expect((await current(page,seed.id)).document.versionId).toBe(head);}
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);v=await current(page,seed.id);expect(v.part.features.find((f:any)=>f.id===seed.feature.id).length).toBeCloseTo(10,8);const parameter=v.part.parameters.find((p:any)=>p.ownerFeatureId===seed.feature.id&&p.propertySlot==='length');expect(parameter.source.expression.sourceText).toBe('-(-len)*5e-1');expect(parameter.source.expression.reads).toContain(`parameter:${seed.parameter}`);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.feature.id).length)).toBe(20);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.feature.id).length)).toBe(10);
 // Compact and spaced formulas both pass the actual edit/commit path unchanged.
 for(const expression of ['len/2','len / 2']){await filter.fill(seed.feature.name!);await page.locator(`[role=treeitem][data-tree-key$="pad:${seed.feature.id}"]`).dblclick();d=page.getByRole('dialog',{name:'编辑 拉伸',exact:true});await ready(page,d);await d.getByRole('textbox',{name:/^长度/}).fill(expression);await ready(page,d);await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);const saved=await current(page,seed.id);expect(saved.part.features.find((f:any)=>f.id===seed.feature.id).length).toBe(10);expect(saved.part.parameters.find((p:any)=>p.ownerFeatureId===seed.feature.id&&p.propertySlot==='length').source.expression.sourceText).toBe(expression);}
 // All menu depths share slots and keep within the actual window.
 await filter.fill(seed.feature.name!);const sourceRow=page.locator(`[role=treeitem][data-tree-key$="pad:${seed.feature.id}"]`);await sourceRow.click({button:'right',position:{x:120,y:12}});await page.getByRole('menuitem',{name:'对象',exact:true}).hover();await page.getByRole('menuitem',{name:/在此步骤后插入/}).hover();await expect(page.getByRole('menuitem',{name:'圆角',exact:true})).toBeVisible();for(const menu of await page.locator('.specification-tree-context-menu:visible > .ant-dropdown-menu,.cad-command-submenu:visible > .ant-dropdown-menu').all()){await expect(menu).toHaveCSS('width','256px');const r=await menu.boundingBox();expect(r!.x).toBeGreaterThanOrEqual(0);expect(r!.x+r!.width).toBeLessThanOrEqual(1440)}await page.screenshot({path:testInfo.outputPath('three-level-context-menu.png')});await page.keyboard.press('Escape');await sourceRow.focus();await sourceRow.press('Shift+F10');await expect(page.locator('.specification-tree-context-menu:visible')).toBeVisible();await page.keyboard.press('Escape');
 // Continuous real sketch creation, deleting the middle name, and creating again.
 await filter.fill(seed.sketch2.name!);await page.getByRole('treeitem').filter({hasText:seed.sketch2.name!}).last().dblclick();await filter.fill('');await page.getByRole('button',{name:'圆',exact:true}).dblclick();await expect.poll(()=>page.evaluate(async()=>{const {useWorkbenchStore}=await import('/src/state/workbench-store.ts');return useWorkbenchStore.getState().activeToolMode})).toBe('continuous');
 for(const [index,[x,y]] of [[-4,-4],[0,4],[4,-4]].entries()){await clickWorld(page,[x,y,0]);await clickWorld(page,[x+1,y,0]);await expect.poll(async()=>((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.sketch2.id).sketch.entities.filter((e:any)=>e.kind==='CIRCLE').length)).toBe(index+1);await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.activeToolID==='sketch.circle'&&!e.sketchCommitPending&&!e.tools.active.creationPending})).toBeTruthy();}
 const circles=async()=>((await current(page,seed.id)).part.features.find((f:any)=>f.id===seed.sketch2.id).sketch.entities.filter((e:any)=>e.kind==='CIRCLE'));
 await expect.poll(async()=>(await circles()).map((e:any)=>e.name)).toEqual(['Circle.1','Circle.2','Circle.3']);await page.keyboard.press('Escape');await filter.fill('Circle.2');await page.getByRole('treeitem').filter({hasText:'Circle.2'}).last().click();await page.keyboard.press('Delete');await expect.poll(async()=>(await circles()).map((e:any)=>e.name)).toEqual(['Circle.1','Circle.3']);await filter.fill('');await openCommand(page,'圆');await clickWorld(page,[-4,4,0]);await clickWorld(page,[-3,4,0]);await expect.poll(async()=>(await circles()).map((e:any)=>e.name)).toEqual(['Circle.1','Circle.3','Circle.4']);
 const accepted=await circles();await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await circles()).length).toBe(2);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(circles).toEqual(accepted);await openCommand(page,'退出草图');await page.reload();await expect(page.locator('canvas')).toBeVisible();expect(await circles()).toEqual(accepted);
 await filter.fill('Circle.3');await expect(page.getByRole('treeitem').filter({hasText:'Circle.3'})).toBeVisible();await page.screenshot({path:testInfo.outputPath('stable-sketch-name.png')});expect(errors).toEqual([]);
});

test("document paging and insertion start in the owning Product folder",async({page},testInfo)=>{
 test.setTimeout(180_000);page.setDefaultTimeout(20_000);await login(page);
 const seed=await page.evaluate(async()=>{const {restApi:api}=await import('/src/api.ts');const stamp=Date.now(),a=await api.createFolder(`UI 宿主 ${stamp}`,''),b=await api.createFolder(`UI 目标 ${stamp}`,''),child=await api.createFolder('导航子目录','',b.id);const root=await api.createDocument('PRODUCT','UI 宿主装配','',a.id),target=await api.createDocument('PRODUCT','UI 目标装配','',b.id),docs=[];for(let start=0;start<51;start+=4)docs.push(...await Promise.all(Array.from({length:Math.min(4,51-start)},(_,j)=>api.createDocument('PART',`UI 分页零件 ${String(start+j+1).padStart(2,'0')}`,'',b.id))));const v=await api.command(root.document.id,{type:'INSERT_INSTANCE',referencedDocumentId:target.document.id});return {root:root.document.id,target:target.document.id,instance:v.product!.instances[0].id,folder:b,child,docs:docs.map(v=>v.document)};});
 await page.goto('/');await page.locator('.folder-card').filter({hasText:seed.folder.name}).dblclick();await page.locator('.ant-segmented-item').filter({hasText:'Part'}).click({timeout:20_000});await expect(page.locator('.document-card')).toHaveCount(24);
 const sizes=page.locator('.ant-pagination-options-size-changer');await sizes.click();await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({hasText:/^50/}).click();await expect(page.locator('.document-card')).toHaveCount(50);await page.locator('.ant-pagination-next button').click();await expect(page.locator('.document-card')).toHaveCount(1);
 await page.locator('.document-card .card-menu').click();await page.getByRole('menuitem',{name:/移入回收站/}).click();await page.getByRole('dialog').getByRole('button',{name:/确\s*定/}).click();await expect(page.locator('.document-card')).toHaveCount(50);await expect(page.locator('.ant-pagination-item-active')).toHaveText('1');await sizes.click();await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({hasText:/^24/}).click();await expect(page.locator('.document-card')).toHaveCount(24);
 await page.goto(`/documents/${seed.root}`);await expect(page.locator('canvas')).toBeVisible();await page.locator(`[role=treeitem][data-tree-key*="instance:${seed.instance}"]`).first().dblclick();await openCommand(page,'插入');let d=page.getByRole('dialog',{name:'插入组件',exact:true});await expect(d.getByRole('button',{name:seed.child.name,exact:true})).toBeVisible();await d.getByRole('button',{name:seed.child.name,exact:true}).click();await expect(d.locator('.ant-breadcrumb')).toContainText(seed.child.name);
 await page.evaluate(async()=>{const {queryClient}=await import('/src/app/providers.tsx');await queryClient.invalidateQueries({queryKey:['documents']});});await expect(d.locator('.ant-breadcrumb')).toContainText(seed.child.name);await d.getByRole('button',{name:/取\s*消/}).click();await openCommand(page,'插入');d=page.getByRole('dialog',{name:'插入组件',exact:true});await expect(d.getByRole('button',{name:seed.child.name,exact:true})).toBeVisible();
 const names=[];const first=d.locator('.insert-document-card').filter({hasNot:page.locator('[disabled]')}).first();names.push(await first.locator('strong').innerText());await first.click();await d.locator('.ant-pagination-next button').click();const second=d.locator('.insert-document-card').filter({hasNot:page.locator('[disabled]')}).first();names.push(await second.locator('strong').innerText());await second.click();await expect(d.getByRole('button',{name:'当前选择 (2)',exact:true})).toBeVisible();await page.screenshot({path:testInfo.outputPath('owning-folder-cross-page-selection.png')});await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 const inserted=await current(page,seed.target);expect(inserted.product.instances.map((i:any)=>i.documentId).sort()).toEqual(names.map(n=>seed.docs.find(v=>v.name===n)!.id).sort());expect((await current(page,seed.root)).product.instances).toHaveLength(1);
 // The same draggable options container works in the home page and restores a saved position.
 await page.goto('/');await page.getByRole('button',{name:'选项',exact:true}).click();const options=page.getByRole('dialog',{name:'选项',exact:true});const header=await options.locator('header').boundingBox();if(!header)throw Error('Missing options header');await page.mouse.move(header.x+100,header.y+20);await page.mouse.down();await page.mouse.move(header.x+50,header.y+80,{steps:6});await page.mouse.up();const moved=await options.boundingBox();await options.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'选项',exact:true}).click();expect((await options.boundingBox())!.x).toBeCloseTo(moved!.x,0);await options.getByRole('button',{name:'关闭',exact:true}).click();
});

test('unified patterns preserve mirrored cut tools and highlight each actual owner',async({page},testInfo)=>{
 test.setTimeout(180_000);page.setDefaultTimeout(20_000);await login(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const seed=await page.evaluate(async()=>{const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','UI 镜像切削工具验收');const id=v.document.id;
 v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});let sketch=v.part!.features.at(-1)!.id;v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-30,y:-30},second:{x:30,y:30}}]});v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:5});
 v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});sketch=v.part!.features.at(-1)!.id;v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'CIRCLE',role:'PROFILE',center:{x:10,y:10},radius:2}}]});v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'REMOVE',length:5});const tool=v.part!.features.at(-1)!;const mirrors=[];
 for(const plane of ['datum-yz','datum-xz']){v=await api.command(id,{type:'CREATE_PATTERN',feature:{id:'',type:'SOLID_PATTERN',bodyId:tool.bodyId,operation:'ADD',pattern:{id:'',kind:'MIRROR',distribution:'FIXED_STEP',count:2,origin:[0,0,0],direction:[1,0,0],mirrorPlaneId:plane,sourceKind:'AUTO',source:{bodyId:tool.bodyId!,featureId:tool.id}}}});mirrors.push(v.part!.features.at(-1)!)}
 return {id,tool,mirrors,version:v.document.versionId};});
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();await worldScreen(page,[0,0,0]);await openCommand(page,'镜像');const d=page.getByRole('dialog',{name:'创建镜像',exact:true});await expect(d.getByText('生成特征 / 切削工具',{exact:true})).toHaveCount(0);
 const pick=await page.evaluate(target=>{const e=(window as any).__cameraEngine,canvas=e.renderer.domElement,r=canvas.getBoundingClientRect();for(let y=35;y<r.height-30;y+=5)for(let x=35;x<r.width-30;x+=5){if(document.elementFromPoint(r.x+x,r.y+y)!==canvas)continue;const hit=e.hitTest(x,y);if(hit?.kind==='face'&&hit.associatedFeatureIds?.includes(target))return {x:r.x+x,y:r.y+y}}},seed.mirrors[1].id);expect(pick).toBeTruthy();await page.mouse.click(pick!.x,pick!.y);await expect(d.getByRole('group',{name:'种子',exact:true})).toContainText('Mirror.2');await ready(page,d);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);
 const saved=await current(page,seed.id),last=saved.part.features.at(-1),body=saved.part.bodies.find((b:any)=>b.id===last.bodyId),volume=saved.artifacts[body.geometryKey].volume;expect(last.operation).toBe('REMOVE');expect(last.pattern.sourceKind).toBe('GENERATOR_TOOL');expect(last.pattern.source.featureId).toBe(seed.mirrors[1].id);expect(volume).toBeCloseTo(18000-4*Math.PI*4*5,5);
 const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 for(const f of [...seed.mirrors,last]){await filter.fill(f.name);await page.locator(`[role=treeitem][data-tree-key$="feature:${f.id}"]`).click();await expect.poll(()=>page.evaluate(id=>{const e=(window as any).__cameraEngine;const binding=[...e.solidBindings.values()][0] as any;const entries=binding.artifact.visualization.featureAssociations.elements.filter((a:any)=>a.kind==='FACE'&&a.primary.includes(id));return {count:entries.length,owners:[...new Set(entries.flatMap((a:any)=>a.primary))],overlays:e.selectedOverlays.filter((o:any)=>o.isMesh).length}},f.id)).toMatchObject({owners:[f.id],overlays:1});}
 await page.screenshot({path:testInfo.outputPath('mirror-primary-contribution.png')});await filter.fill('');await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).part.features.length).toBe(saved.part.features.length-1);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).part.features.at(-1).id).toBe(last.id);
 await openCommand(page,'阵列');const array=page.getByRole('dialog',{name:'创建阵列',exact:true});await filter.fill(seed.tool.name!);await page.locator(`[role=treeitem][data-tree-key$="pad:${seed.tool.id}"]`).click();await ready(page,array);await array.getByLabel('阵列方式').getByText('圆周',{exact:true}).click();await ready(page,array);const head=(await current(page,seed.id)).document.versionId;await array.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(head);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();const reopened=await current(page,seed.id);expect(reopened.part.features.at(-1)).toEqual(last);expect(reopened.artifacts[reopened.part.bodies.find((b:any)=>b.id===last.bodyId).geometryKey].volume).toBeCloseTo(volume,8);expect(errors).toEqual([]);
});

test('pattern seed collections use tree and viewport with named locatable references',async({page},testInfo)=>{
 page.setDefaultTimeout(20000);await login(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const seed=await page.evaluate(async()=>{const {restApi:api}=await import('/src/api.ts');let v=await api.createDocument('PART','多种子镜像交互');const id=v.document.id;
 v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});let sketch=v.part!.features.at(-1)!.id;v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_RECTANGLE',first:{x:-50,y:-50},second:{x:50,y:50}}]});v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'ADD',length:5});const stock=v.part!.features.at(-1)!;const cuts=[];
 for(const x of [-15,15]){v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});sketch=v.part!.features.at(-1)!.id;v=await api.command(id,{type:'EDIT_SKETCH',sketchId:sketch,operations:[{type:'ADD_ENTITY',entity:{id:crypto.randomUUID(),kind:'CIRCLE',role:'PROFILE',center:{x,y:10},radius:2}}]});v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId:sketch,generator:'LINEAR_EXTRUDE',operation:'REMOVE',length:5});cuts.push(v.part!.features.at(-1)!)}
 return {id,stock,cuts,version:v.document.versionId};});
 await page.goto(`/documents/${seed.id}`);await worldScreen(page,[0,0,0]);const filter=page.getByRole('textbox',{name:'筛选模型结构'});
 const treePick=async(f:{id:string;name?:string})=>{await filter.fill(f.name!);await page.locator(`[role=treeitem][data-tree-key$="pad:${f.id}"]`).click()};
 await openCommand(page,'镜像');const d=page.getByRole('dialog',{name:'创建镜像',exact:true});await expect(d.getByRole('combobox',{name:'选择种子'})).toHaveCount(0);await treePick(seed.cuts[0]);
 const second=await page.evaluate(id=>{const e=(window as any).__cameraEngine,r=e.renderer.domElement.getBoundingClientRect();for(let y=35;y<r.height-30;y+=4)for(let x=35;x<r.width-30;x+=4){if(document.elementFromPoint(r.x+x,r.y+y)!==e.renderer.domElement)continue;const hit=e.hitTest(x,y);if(hit?.kind==='face'&&hit.associatedFeatureIds?.includes(id))return {x:r.x+x,y:r.y+y}}},seed.cuts[1].id);expect(second).toBeTruthy();await page.mouse.click(second!.x,second!.y);await expect(d.getByRole('button',{name:'定位种子 2',exact:true})).toBeEnabled();await d.getByRole('button',{name:'定位种子 2',exact:true}).click();await expect.poll(()=>page.evaluate(()=>(window as any).__cameraEngine.selectedOverlays.filter((o:any)=>o.isMesh).length)).toBeGreaterThan(0);
 await d.getByRole('button',{name:'选择镜像平面',exact:true}).click();await filter.fill('XZ');await page.locator('[role=treeitem]').filter({has:page.getByText('XZ Plane',{exact:true})}).last().click();await ready(page,d);expect((await current(page,seed.id)).document.versionId).toBe(seed.version);
 await d.getByRole('button',{name:'选择种子 1',exact:true}).click();await d.getByRole('button',{name:'选择种子 1',exact:true}).click();await expect(d.getByRole('button',{name:'定位种子 2',exact:true})).toBeEnabled();
 await d.getByRole('button',{name:'移除种子 1',exact:true}).click();await expect(d.getByRole('button',{name:'定位种子 2',exact:true})).toHaveCount(0);await d.getByRole('button',{name:'选择种子',exact:true}).click();await treePick(seed.cuts[0]);await ready(page,d);
 await d.getByRole('button',{name:/确\s*定/}).click();await expect(d).toHaveCount(0);const saved=await current(page,seed.id),mirror=saved.part.features.at(-1),b=saved.part.bodies.find((b:any)=>b.id===mirror.bodyId);expect([mirror.pattern.source.featureId,...mirror.pattern.additionalSources.map((s:any)=>s.source.featureId)].sort()).toEqual(seed.cuts.map(f=>f.id).sort());expect(saved.artifacts[b.geometryKey].volume).toBeCloseTo(50000-4*Math.PI*4*5,5);
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).part.features.at(-1).id).toBe(seed.cuts[1].id);await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>(await current(page,seed.id)).part.features.at(-1).id).toBe(mirror.id);await page.reload();await worldScreen(page,[0,0,0]);
 await openCommand(page,'阵列');const a=page.getByRole('dialog',{name:'创建阵列',exact:true});await treePick(seed.cuts[0]);const axis=a.locator('.feature-pick-field').filter({has:page.locator('label',{hasText:/^方向$/})});await expect(axis).toContainText('X 轴');await expect(axis.getByRole('button',{name:'定位方向',exact:true})).toBeEnabled();await axis.getByRole('button',{name:'定位方向',exact:true}).click();await axis.getByRole('button',{name:'移除方向',exact:true}).click();await expect(a.getByRole('button',{name:/确\s*定/})).toBeDisabled();await a.getByLabel('阵列方式').getByText('圆周',{exact:true}).click();await expect(a).toContainText('Z 轴');
 const pickReference=async(predicate:string)=>{const point=await page.evaluate(predicate=>{const e=(window as any).__cameraEngine,r=e.renderer.domElement.getBoundingClientRect();for(let y=20;y<r.height-20;y+=3)for(let x=20;x<r.width-20;x+=3){if(document.elementFromPoint(r.x+x,r.y+y)!==e.renderer.domElement)continue;const h=e.hitTest(x,y);if(predicate==='axis'?h?.kind==='axis'&&h.axis==='Z':h?.kind==='datum-point')return {x:r.x+x,y:r.y+y}}},predicate);expect(point).toBeTruthy();await page.mouse.click(point!.x,point!.y)};
 await a.getByRole('button',{name:'选择轴',exact:true}).click();await pickReference('axis');await expect(a).toContainText('Z 轴');await a.getByRole('button',{name:'定位轴',exact:true}).click();await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.selectionIndex.objectsFor(e.selected[0]).length})).toBeGreaterThan(0);
 await a.getByRole('button',{name:'选择中心点',exact:true}).click();await pickReference('point');const centre=a.locator('.feature-pick-field').filter({has:page.locator('label',{hasText:/^中心点$/})});await expect(centre).toContainText('原点');await expect(centre.getByRole('button',{name:'定位中心点',exact:true})).toBeEnabled();await centre.getByRole('button',{name:'定位中心点',exact:true}).click();await expect.poll(()=>page.evaluate(()=>{const e=(window as any).__cameraEngine;return e.selectionIndex.objectsFor(e.selected[0]).length})).toBeGreaterThan(0);
 const labels=await a.locator('.command-field > label').allTextContents();expect(labels.at(-1)).toBe('反向');await expect(a.getByRole('radiogroup',{name:'材料',exact:true})).toContainText('切除');await page.screenshot({path:testInfo.outputPath('named-pattern-references.png')});const head=(await current(page,seed.id)).document.versionId;await a.getByRole('button',{name:/取\s*消/}).click();expect((await current(page,seed.id)).document.versionId).toBe(head);expect(errors).toEqual([]);
});

test("Boolean result regions pick exact volumes, exclude, edit, cancel and reopen",async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  let v=await api.createDocument('PART','Boolean region browser acceptance');const id=v.document.id;
  const rectangle=async(x:number,y:number,w:number,h:number,length:number)=>{
   v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;
   v=await api.command(id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x,y},second:{x:x+w,y:y+h}}]});
   v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'NEW_BODY',length});return v.part!.features.at(-1)!;
  };
  const base=await rectangle(0,0,30,10,10),tool=await rectangle(10,-5,10,20,20);
  v=await api.command(id,{type:'SET_ACTIVE_BODY',bodyId:base.bodyId});return {id,head:v.document.versionId,base:base.bodyId!,tool:tool.bodyId!};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();
 await openCommand(page,'布尔');let dialog=page.getByRole('dialog',{name:'布尔运算',exact:true});
 await clickWorld(page,[15,5,20]);await ready(page,dialog);
 let analysisCalls=0;page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/boolean-regions'))analysisCalls++});
 await dialog.getByText('选择区域',{exact:true}).click();
 await expect(page.locator('[data-boolean-regions="2"]')).toBeVisible();
 const rows=()=>dialog.getByRole('list',{name:'结果区域列表'});
 await expect(rows().getByRole('switch',{checked:true})).toHaveCount(2);
 await expect.poll(()=>page.evaluate(({target,tool})=>{const e=(window as any).__cameraEngine;return [target,tool].every(id=>e.solidBindings.get(`root/body:${id}`)?.group.visible===false)},{target:seed.base,tool:seed.tool})).toBe(true);
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();
 // Surface picks toggle a complete volume, including its other faces.
 await clickWorld(page,[5,5,10]);await expect(rows().getByRole('switch',{checked:true})).toHaveCount(1);
 await clickWorld(page,[25,5,10]);await expect(rows().getByRole('switch',{checked:true})).toHaveCount(0);
 await expect(dialog.getByText('至少保留一个结果区域',{exact:true})).toBeVisible();
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeDisabled();
 // Excluded geometry remains pickable and can be selected back.
 await clickWorld(page,[5,5,10]);await expect(rows().getByRole('switch',{checked:true})).toHaveCount(1);
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();
 const selectedRow=rows().getByRole('listitem').filter({has:page.getByRole('switch',{checked:true})});
 await selectedRow.getByRole('button',{name:/单独显示区域/}).click();
 await expect(selectedRow.getByRole('button',{name:/单独显示区域/})).toHaveAttribute('aria-pressed','true');
 await selectedRow.getByRole('button',{name:/单独显示区域/}).click();
 await page.screenshot({path:'../../../build/agent-check/boolean-regions/selected-region.png'});
 expect(analysisCalls).toBe(1);
 expect((await current(page,seed.id)).document.versionId).toBe(seed.head);
 await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const committed=await current(page,seed.id),feature=committed.part.features.at(-1),body=committed.part.bodies.find((b:{id:string})=>b.id===seed.base);
 expect(feature.booleanResult.mode).toBe('EXPLICIT');expect(feature.booleanResult.regions).toHaveLength(1);
 expect(committed.artifacts[body.geometryKey].volume).toBeCloseTo(1000,5);
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill(feature.name);
 await page.getByRole('treeitem').filter({hasText:feature.name}).first().dblclick();dialog=page.getByRole('dialog',{name:'编辑布尔',exact:true});
 await expect(page.locator('[data-boolean-regions="2"]')).toBeVisible();
 await expect(rows().getByRole('switch',{checked:true})).toHaveCount(1);
 await rows().getByRole('switch',{checked:false}).click();await expect(rows().getByRole('switch',{checked:true})).toHaveCount(2);
 await dialog.getByRole('button',{name:/取\s*消/}).click();await expect(dialog).toHaveCount(0);
 await expect(page.locator('[data-boolean-regions]')).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(committed.document.versionId);
 await page.getByRole('textbox',{name:'筛选模型结构'}).clear();
 await page.getByRole('button',{name:'撤销',exact:true}).click();await expect.poll(async()=>((await current(page,seed.id)).part.features as {id:string}[]).some(f=>f.id===feature.id)).toBe(false);
 await page.getByRole('button',{name:'重做',exact:true}).click();await expect.poll(async()=>((await current(page,seed.id)).part.features as {id:string}[]).some(f=>f.id===feature.id)).toBe(true);
 await page.reload();await expect(page.locator('canvas')).toBeVisible();
 const reopened=await current(page,seed.id);expect(reopened.part.features.find((f:{id:string})=>f.id===feature.id).booleanResult).toEqual(feature.booleanResult);
 await page.screenshot({path:'../../../build/agent-check/boolean-regions/reopened.png'});expect(errors).toEqual([]);
});

test("Boolean lost region reselects in viewport and late analysis cannot reopen cancelled command",async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page);
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import('/src/api.ts');
  let v=await api.createDocument('PART','Boolean region repair acceptance');const id=v.document.id;
  const rectangle=async(x:number,y:number,w:number,h:number,length:number)=>{
   v=await api.command(id,{type:'CREATE_SKETCH',plane:'XY'});const sketchId=v.part!.features.at(-1)!.id;
   v=await api.command(id,{type:'EDIT_SKETCH',sketchId,operations:[{type:'ADD_RECTANGLE',first:{x,y},second:{x:x+w,y:y+h}}]});
   v=await api.command(id,{type:'CREATE_SOLID_FEATURE',sketchId,generator:'LINEAR_EXTRUDE',operation:'NEW_BODY',length});return v.part!.features.at(-1)!;
  };
  const base=await rectangle(0,0,30,10,10),tool=await rectangle(10,-5,10,20,20);
  const cells=await api.analyzeBooleanRegions(id,{versionId:v.document.versionId,bodyId:base.bodyId!,tool:{bodyId:tool.bodyId!,featureId:tool.id},operation:'REMOVE'});
  v=await api.command(id,{type:'CREATE_BOOLEAN_FEATURE',bodyId:base.bodyId,operation:'REMOVE',tools:[{bodyId:tool.bodyId,featureId:tool.id}],booleanResult:{mode:'EXPLICIT',regions:[cells.regions.find(c=>c.eligible)!.reference]}});
  const feature=v.part!.features.at(-1)!;
  let digest='';const visit=(n:any)=>{if(n.entityId===tool.id)digest=n.definitionDigest;(n.children??[]).forEach(visit)};visit(v.structureTree);
  v=await api.command(id,{type:'EDIT_FEATURE',targetId:tool.id,expectedFeatureDigest:digest,feature:{...tool,length:5}});
  return {id,head:v.document.versionId,base:base.bodyId!,feature};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator('canvas')).toBeVisible();
 await page.getByRole('textbox',{name:'筛选模型结构'}).fill(seed.feature.name!);
 await page.getByRole('treeitem').filter({hasText:seed.feature.name!}).first().dblclick();
 let dialog=page.getByRole('dialog',{name:'编辑布尔',exact:true});
 await expect(page.locator('[data-boolean-regions="1"]')).toBeVisible();
 const missing=dialog.getByRole('listitem').filter({hasText:'失效区域'});
 await expect(missing).toHaveCount(1);await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeDisabled();
 await dialog.getByRole('button',{name:'定位区域 1',exact:true}).click();
 await page.getByRole('button',{name:'适合窗口',exact:true}).click();
 await missing.getByRole('button',{name:/选择结果区域/}).click();
 await clickWorld(page,[5,5,10]);await expect(missing).toHaveCount(0);
 await expect(dialog.getByRole('button',{name:/确\s*定/})).toBeEnabled();
 expect((await current(page,seed.id)).document.versionId).toBe(seed.head);
 await page.screenshot({path:'../../../build/agent-check/boolean-regions/repair-preview.png'});
 await dialog.getByRole('button',{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const repaired=await current(page,seed.id),feature=repaired.part.features.find((f:{id:string})=>f.id===seed.feature.id),body=repaired.part.bodies.find((b:{id:string})=>b.id===seed.base);
 expect(feature).toMatchObject({id:seed.feature.id,name:seed.feature.name,bodyId:seed.base});
 expect(feature.evaluationStatus??'').not.toMatch(/FAILED|BLOCKED/);
 expect(repaired.artifacts[body.geometryKey].volume).toBeCloseTo(2500,5);
 // Delay the genuine response, then cancel before it is delivered.
 let release!:()=>void,received!:()=>void;
 const held=new Promise<void>(resolve=>{release=resolve}),arrived=new Promise<void>(resolve=>{received=resolve});
 await page.route('**/boolean-regions',async route=>{const response=await route.fetch();received();await held;try{await route.fulfill({response})}catch{/* browser cancelled the request */}});
 await page.getByRole('treeitem').filter({hasText:seed.feature.name!}).first().dblclick();dialog=page.getByRole('dialog',{name:'编辑布尔',exact:true});
 await arrived;await dialog.getByRole('button',{name:/取\s*消/}).click();release();
 await expect(dialog).toHaveCount(0);await expect(page.locator('[data-boolean-regions]')).toHaveCount(0);
 await page.waitForTimeout(350);await expect(page.locator('[data-feature-preview]')).toHaveCount(0);
 expect((await current(page,seed.id)).document.versionId).toBe(repaired.document.versionId);expect(errors).toEqual([]);
});
