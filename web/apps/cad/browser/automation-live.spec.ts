import {expect,test} from "@playwright/test";
test.use({actionTimeout:20_000});

test("provider-free modeling task previews, pauses, renders every commit and recovers after reload",async({page})=>{
 const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
 const login=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();await page.goto("/");
 const seed=await page.evaluate(async()=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");const view=await api.createDocument("PART",`Automation acceptance ${crypto.randomUUID().slice(0,8)}`);return {id:view.document.id,revision:view.document.versionId};
 });
 await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
 await page.getByRole("button",{name:"建模助手",exact:true}).click();const panel=page.getByRole("complementary",{name:"建模助手"});await expect(panel).toContainText("AI 尚未连接");
 await panel.getByRole("button",{name:"创建矩形拉伸流程"}).click();await expect(panel.locator(".assistant-steps li")).toHaveCount(3);
 await panel.getByRole("button",{name:"执行下一步",exact:true}).click();await expect(panel).toContainText("预览就绪");
 expect((await (await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.revision);
 await panel.getByRole("button",{name:"暂停 / 接管",exact:true}).click();await expect(panel.getByRole("button",{name:"基于当前状态继续"})).toBeVisible();
 await panel.getByRole("button",{name:"基于当前状态继续"}).click();await expect(panel.getByRole("button",{name:"执行下一步",exact:true})).toBeEnabled();
 await panel.getByRole("checkbox",{name:"自动逐步执行"}).check();await panel.getByRole("button",{name:"执行下一步",exact:true}).click();
 await expect(panel).toContainText("流程完成",{timeout:90_000});await expect(panel.locator('[data-phase="DISPLAYED"]')).toHaveCount(3);
 const final=await (await page.request.get(`/api/documents/${seed.id}`)).json();expect(final.part.features).toHaveLength(2);expect(final.part.features[0].sketch.entities).toHaveLength(4);expect(final.part.features[1].length).toBe(12);
 const volume=Object.values(final.artifacts as Record<string,{volume:number}>).reduce((sum,artifact)=>sum+artifact.volume,0);expect(volume).toBeCloseTo(48000,4);
 await page.screenshot({path:"../../../build/acceptance/automation-task-live.png"});await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();await expect(page.getByRole("complementary",{name:"建模助手"})).toContainText("流程完成");
 expect(errors).toEqual([]);
});
