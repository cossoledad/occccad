import {expect,test} from "@playwright/test";

test("real Boolean preview, commit, output consumption and reopen",async({page})=>{
 if (!process.env.OCCCCAD_ADMIN_PASSWORD) throw new Error("Requires running application and configured login");
 const response=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});
 expect(response.ok()).toBeTruthy();
 await page.goto("/");
 // Fixtures use the same production WebSocket command client as the workbench.
 const seed=await page.evaluate(async()=>{
  // @ts-ignore Vite serves this browser-side source module.
  const {restApi:api}=await import("/src/api.ts");
  let view=await api.createDocument("PART","Solid Feature browser acceptance");
  const id=view.document.id;
  const rectangle=async(x:number,y:number,w:number,h:number,length:number)=>{
   view=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=view.part!.features.at(-1)!.id;
   view=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x,y},second:{x:x+w,y:y+h}}]});
   view=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"NEW_BODY",length});
   return view.part!.features.at(-1)!;
  };
  const base=await rectangle(0,0,20,20,10),tool=await rectangle(5,5,5,5,20);
  view=await api.command(id,{type:"SET_ACTIVE_BODY",bodyId:base.bodyId});
  return {id,version:view.document.versionId,base:base.bodyId!,tool:tool.bodyId!,toolName:tool.name!};
 });
 await page.goto(`/documents/${seed.id}`);
 await expect(page.locator("canvas")).toBeVisible();
 await page.getByRole("button",{name:"搜索工具",exact:true}).click();
 await page.getByRole("textbox",{name:"搜索工具名称或用途"}).fill("布尔");
 await page.locator(".workbench-command-result").filter({hasText:"布尔"}).first().click();
 const dialog=page.getByRole("dialog",{name:"布尔运算",exact:true});
 await dialog.getByRole("combobox").nth(2).click();
 await page.locator(".ant-select-item-option").filter({hasText:seed.toolName}).click();
 await page.keyboard.press("Escape");
 await dialog.getByRole("button",{name:/预\s*览/}).click();
 await expect(dialog.getByRole("button",{name:/预\s*览/})).toBeEnabled();
 const before=await (await page.request.get(`/api/documents/${seed.id}`)).json();
 expect(before.document.versionId).toBe(seed.version);
 await dialog.getByRole("button",{name:/确\s*定/}).click();
 await expect(dialog).toHaveCount(0);
 const committed=await (await page.request.get(`/api/documents/${seed.id}`)).json();
 expect(committed.document.versionId).not.toBe(seed.version);
 expect(committed.part.bodies.find((b:{id:string})=>b.id===seed.tool).consumed).toBe(true);
 const body=committed.part.bodies.find((b:{id:string})=>b.id===seed.base);
 expect(committed.artifacts[body.geometryKey].volume).toBeCloseTo(3750,5);
 await expect(page.getByRole("tree").getByLabel("状态 已用于布尔")).toBeVisible();
 await page.reload();await expect(page.locator("canvas")).toBeVisible();
 await expect(page.getByRole("tree").getByLabel("状态 已用于布尔")).toBeVisible();
});
