import {expect,test} from "@playwright/test";
import {startAssistantProvider} from "./assistant-provider.fixture";
test.use({actionTimeout:20_000});

test("conversation connects with one key, plans through provider tools and renders native CAD steps",async({page})=>{
 // Start the app with OCCCCAD_AI_BASE_URL=http://127.0.0.1:19091 for this test.
 const provider=await startAssistantProvider();
 const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
 let connected=false;
 try{
  const login=await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}});expect(login.ok()).toBeTruthy();
  const csrf=()=>page.context().cookies().then(cookies=>cookies.find(cookie=>cookie.name==="occccad_csrf")!.value);
  // Use a distinct account so acceptance never replaces a person's saved key.
  const email=`assistant-${crypto.randomUUID()}@test.invalid`,password=`Fixture-${crypto.randomUUID()}`;
  const user=await page.request.post("/api/admin/users",{headers:{"X-CSRF-Token":await csrf()},data:{email,displayName:"AI 浏览器验收",password,status:"ACTIVE",platformRole:"MEMBER"}});expect(user.ok()).toBeTruthy();
  expect((await page.request.post("/api/auth/login",{data:{email,password}})).ok()).toBeTruthy();
  await page.goto("/");
  const seed=await page.evaluate(async()=>{
   // @ts-ignore production Vite module
   const {restApi:api}=await import("/src/api.ts");const view=await api.createDocument("PART",`对话建模验收 ${crypto.randomUUID().slice(0,6)}`);return {id:view.document.id,revision:view.document.versionId};
  });
  await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
  await page.getByRole("button",{name:"建模助手",exact:true}).click();const panel=page.getByRole("complementary",{name:"建模助手"});
  await expect(panel).toContainText("用对话开始建模");
  await page.screenshot({path:"../../../build/acceptance/assistant-connect-live.png",style:".cad-input-debug{visibility:hidden}"});
  await panel.getByLabel("API Key",{exact:true}).fill("synthetic-browser-assistant-key");
  await panel.getByRole("button",{name:"保存并连接",exact:true}).click();await expect(panel).toContainText("想做一个什么样的零件");connected=true;
  await panel.getByRole("button",{name:"连接设置",exact:true}).click();await expect(panel.getByLabel("API Key",{exact:true})).toHaveValue("");await panel.getByRole("button",{name:"返回对话",exact:true}).click();
  await panel.getByRole("checkbox",{name:"逐步确认",exact:true}).check();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("在 XY 平面创建 64 × 36 mm 矩形，并拉伸 9 mm");
  await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel.locator(".assistant-steps li")).toHaveCount(3);
  expect(provider.calls()).toBe(2);await expect(panel).toContainText("64 × 36 mm");
  const canvasBounds=await page.locator(".cad-viewport-canvas").boundingBox(),chatBounds=await panel.boundingBox();expect(canvasBounds!.x+canvasBounds!.width).toBeLessThanOrEqual(chatBounds!.x+1);
  await panel.getByRole("button",{name:"预览下一步",exact:true}).click();await expect(panel).toContainText("预览就绪");
  expect((await(await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.revision);
  await panel.getByRole("button",{name:"暂停 / 接管",exact:true}).click();await expect(panel.getByRole("button",{name:"基于当前状态继续"})).toBeVisible();
  await panel.getByRole("checkbox",{name:"逐步确认",exact:true}).uncheck();
  await panel.getByRole("button",{name:"基于当前状态继续"}).click();
  await expect(panel).toContainText("已完成，可以继续提出修改",{timeout:90_000});await expect(panel.locator('[data-phase="DISPLAYED"]')).toHaveCount(3);
  const final=await(await page.request.get(`/api/documents/${seed.id}`)).json();expect(final.part.features).toHaveLength(2);expect(final.part.features[0].sketch.entities).toHaveLength(4);expect(final.part.features[1].length).toBeCloseTo(9,8);
  const volume=Object.values(final.artifacts as Record<string,{volume:number}>).reduce((sum,artifact)=>sum+artifact.volume,0);expect(volume).toBeCloseTo(20736,4);
  await page.screenshot({path:"../../../build/acceptance/assistant-chat-modeling-live.png",style:".cad-input-debug{visibility:hidden}"});
  await panel.getByRole("button",{name:"收起建模助手",exact:true}).click();await expect(panel).not.toBeVisible();
  await page.getByRole("button",{name:"建模助手",exact:true}).click();await expect(panel).toBeVisible();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("当前零件有哪些参数？");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel).toContainText("当前拉伸长度为 9 mm");expect(provider.calls()).toBe(4);
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("等待一次建模回复");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();
  await expect.poll(provider.calls).toBe(5);await panel.getByRole("button",{name:"停止回复",exact:true}).click();
  await expect(panel).toContainText("已停止本次回复");
  expect((await(await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(final.document.versionId);
  expect((await(await page.request.get(`/api/documents/${seed.id}/automation/tasks`)).json()).tasks).toHaveLength(1);
  await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();
  await expect(page.getByRole("complementary",{name:"建模助手"})).toContainText("当前拉伸长度为 9 mm");expect(provider.calls()).toBe(5);
  const restored=page.getByRole("complementary",{name:"建模助手"});await restored.getByRole("button",{name:"连接设置",exact:true}).click();await restored.getByRole("button",{name:"断开连接",exact:true}).click();await expect(restored).toContainText("用对话开始建模");connected=false;
  expect(errors).toEqual([]);
 }finally{
  if(connected){const cookies=await page.context().cookies();await page.request.delete("/api/assistant/settings",{headers:{"X-CSRF-Token":cookies.find(c=>c.name==="occccad_csrf")?.value??""}})}
  await provider.close();
 }
});
