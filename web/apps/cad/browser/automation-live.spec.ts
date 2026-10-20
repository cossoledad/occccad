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
  const launcher=page.locator('footer[aria-label="工作台状态"]').getByRole("button",{name:"建模助手",exact:true});await expect(launcher).toBeVisible();await expect(page.locator(".assistant-launcher")).toHaveCount(0);
  await launcher.click();const panel=page.getByRole("complementary",{name:"建模助手"});
  await expect(panel).toContainText("用对话开始建模");
  await page.screenshot({path:"../../../build/acceptance/assistant-connect-live.png",style:".cad-input-debug{visibility:hidden}"});
  await panel.getByLabel("API Key",{exact:true}).fill("synthetic-browser-assistant-key");
  await panel.getByRole("button",{name:"保存并连接",exact:true}).click();await expect(panel).toContainText("想做一个什么样的零件");connected=true;
  await panel.getByRole("button",{name:"连接设置",exact:true}).click();await expect(panel.getByLabel("API Key",{exact:true})).toHaveValue("");await panel.getByRole("button",{name:"返回对话",exact:true}).click();
  await panel.getByRole("combobox",{name:"AI 模型",exact:true}).click();await page.getByRole("option",{name:"deepseek-v4-pro",exact:true}).click();
  await expect(panel.getByRole("combobox",{name:"AI 模型",exact:true})).toBeEnabled();
  await panel.getByRole("combobox",{name:"思考度",exact:true}).click();await page.getByRole("option",{name:"思考 · 高",exact:true}).click();
  await expect(panel.getByRole("combobox",{name:"思考度",exact:true})).toBeEnabled();
  const preferences=await(await page.request.get("/api/assistant/settings")).json();expect(preferences.model).toBe("deepseek-v4-pro");expect(preferences.thinking).toBe("high");
  expect(preferences.budget).toBe("medium");await expect(panel).toContainText("每次需求 · 20万 Token");
  await panel.getByRole("combobox",{name:"调用预算",exact:true}).click();await page.getByRole("option",{name:/预算 · 低/}).click();
  await expect(panel.getByRole("combobox",{name:"调用预算",exact:true})).toBeEnabled();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("预算测试");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();
  await expect(panel).toContainText("本次需求已达到调用预算");await expect(panel).toContainText("已报告 10万 Token / 8万 预算 · 2 次模型调用");expect(provider.calls()).toBe(2);
  expect((await(await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.revision);
  await panel.getByRole("combobox",{name:"调用预算",exact:true}).click();await page.getByRole("option",{name:/预算 · 高/}).click();
  await expect(panel.getByRole("combobox",{name:"调用预算",exact:true})).toBeEnabled();
  await panel.getByRole("button",{name:"按当前预算重新发送",exact:true}).click();await expect(panel).toContainText("预算测试已完成，可以继续建模");expect(provider.calls()).toBe(4);
  const budgetTurns=(await(await page.request.get(`/api/documents/${seed.id}/assistant/turns`)).json()).turns;expect(budgetTurns).toHaveLength(2);expect(budgetTurns[0].requestId).not.toBe(budgetTurns[1].requestId);expect(budgetTurns[0].budget.id).toBe("low");expect(budgetTurns[1].budget.id).toBe("high");
  await panel.getByRole("checkbox",{name:"逐步确认",exact:true}).check();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("在 XY 平面创建 64 × 36 mm 矩形，并拉伸 9 mm");
  await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel.locator(".assistant-steps li")).toHaveCount(3);
  expect(provider.calls()).toBe(6);await expect(panel).toContainText("64 × 36 mm");
  const canvasBounds=await page.locator(".cad-viewport-canvas").boundingBox(),chatBounds=await panel.boundingBox();expect(canvasBounds!.x+canvasBounds!.width).toBeLessThanOrEqual(chatBounds!.x+1);
  await panel.getByRole("button",{name:"预览下一步",exact:true}).click();await expect(panel).toContainText("预览就绪");
  expect((await(await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.revision);
  await panel.getByRole("button",{name:"暂停 / 接管",exact:true}).click();await expect(panel.getByRole("button",{name:"基于当前状态继续"})).toBeVisible();
  await panel.getByRole("checkbox",{name:"逐步确认",exact:true}).uncheck();
  await panel.getByRole("button",{name:"基于当前状态继续"}).click();
  await expect(panel).toContainText("已完成，可以继续提出修改",{timeout:90_000});await expect(panel.locator('[data-phase="DISPLAYED"]')).toHaveCount(3);
  const final=await(await page.request.get(`/api/documents/${seed.id}`)).json();expect(final.part.features).toHaveLength(2);expect(final.part.features[0].sketch.entities).toHaveLength(4);expect(final.part.features[1].length).toBeCloseTo(9,8);
  const volume=Object.values(final.artifacts as Record<string,{volume:number}>).reduce((sum,artifact)=>sum+artifact.volume,0);expect(volume).toBeCloseTo(20736,4);
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("给所有的边线加入一个 2 mm 的倒圆角");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();
  await expect(panel).toContainText("R2 mm");await expect(panel.locator('.assistant-exchange').last().locator('[data-phase="DISPLAYED"]')).toHaveCount(1,{timeout:90_000});expect(provider.calls()).toBe(8);
  const rounded=await(await page.request.get(`/api/documents/${seed.id}`)).json();expect(rounded.part.features.at(-1).type).toBe("FILLET");expect(rounded.part.features.at(-1).selections).toHaveLength(12);expect(rounded.part.features.at(-1).length).toBe(2);
  const roundedVolume=Object.values(rounded.artifacts as Record<string,{volume:number}>).reduce((sum,artifact)=>sum+artifact.volume,0);expect(roundedVolume).toBeGreaterThan(0);expect(roundedVolume).toBeLessThan(volume);
  await page.screenshot({path:"../../../build/acceptance/assistant-chat-modeling-live.png",style:".cad-input-debug{visibility:hidden}"});
  await panel.getByRole("button",{name:"收起建模助手",exact:true}).click();await expect(panel).not.toBeVisible();
  await page.getByRole("button",{name:"建模助手",exact:true}).click();await expect(panel).toBeVisible();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("当前零件有哪些参数？");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel).toContainText("当前拉伸长度为 9 mm");expect(provider.calls()).toBe(10);
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("适合窗口");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel).toContainText("已打开界面工具，请继续完成操作");expect(provider.calls()).toBe(12);
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("等待一次建模回复");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();
  await expect.poll(provider.calls).toBe(13);await panel.getByRole("button",{name:"停止回复",exact:true}).click();
  await expect(panel).toContainText("已停止本次回复");
  expect((await(await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(rounded.document.versionId);
  expect((await(await page.request.get(`/api/documents/${seed.id}/automation/tasks`)).json()).tasks).toHaveLength(3);
  await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();
  await expect(page.getByRole("complementary",{name:"建模助手"})).toContainText("当前拉伸长度为 9 mm");expect(provider.calls()).toBe(13);
  await expect(panel.locator(".assistant-model-controls")).toContainText("预算 · 高");expect((await(await page.request.get("/api/assistant/settings")).json()).budget).toBe("high");
  const restored=page.getByRole("complementary",{name:"建模助手"});await restored.getByRole("button",{name:"连接设置",exact:true}).click();await restored.getByRole("button",{name:"断开连接",exact:true}).click();await expect(restored).toContainText("用对话开始建模");connected=false;
  expect(provider.protocolErrors()).toEqual([]);expect(provider.generations().every(value=>value.model==="deepseek-v4-pro"&&value.effort==="high"&&value.thinking==="enabled")).toBeTruthy();
  expect(provider.generations().slice(0,2).every(value=>value.maxTokens===16384)).toBeTruthy();expect(provider.generations().slice(2).every(value=>value.maxTokens===65536)).toBeTruthy();
  expect(errors).toEqual([]);
 }finally{
  if(connected){const cookies=await page.context().cookies();await page.request.delete("/api/assistant/settings",{headers:{"X-CSRF-Token":cookies.find(c=>c.name==="occccad_csrf")?.value??""}})}
  await provider.close();
 }
});
