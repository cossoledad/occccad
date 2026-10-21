import {expect,test,type Page} from "@playwright/test";
import {startAssistantProvider} from "./assistant-provider.fixture";
import {worldScreen} from "./camera-helpers";
test.use({actionTimeout:20_000});
const currentRun=async(page:Page,document:string)=>(await(await page.request.get(`/api/documents/${document}/assistant/runs`)).json()).items[0];
const currentDocument=async(page:Page,id:string)=>(await(await page.request.get(`/api/documents/${id}`)).json());
const clickWorld=async(page:Page,point:number[])=>{const p=await worldScreen(page,point);await page.mouse.click(p.x,p.y)};
async function stopFixtureRuns(page:Page,documents:string[]){
 const cookies=await page.context().cookies(),headers={"X-CSRF-Token":cookies.find(cookie=>cookie.name==="occccad_csrf")?.value??""};
 for(const document of documents){
  const response=await page.request.get(`/api/documents/${document}/assistant/runs`);if(!response.ok())continue;
  const result=await response.json();for(const run of result.items??[]){if(["STOPPED","COMPLETED"].includes(run.status))continue;await page.request.post(`/api/documents/${document}/assistant/runs/${run.id}/control`,{headers,data:{action:"stop",expectedVersion:run.version,epoch:run.epoch}})}
 }
}
async function createDocument(page:Page,box=false){
 return page.evaluate(async({box})=>{
  // @ts-ignore production Vite module
  const {restApi:api}=await import("/src/api.ts");
  let view=await api.createDocument("PART",`持续建模验收 ${crypto.randomUUID().slice(0,6)}`),id=view.document.id;
  if(box){view=await api.command(id,{type:"CREATE_SKETCH",plane:"XY"});const sketchId=view.part.features.at(-1).id;view=await api.command(id,{type:"EDIT_SKETCH",sketchId,operations:[{type:"ADD_RECTANGLE",first:{x:-10,y:-10},second:{x:10,y:10}}]});view=await api.command(id,{type:"CREATE_SOLID_FEATURE",sketchId,generator:"LINEAR_EXTRUDE",operation:"ADD",length:10})}
  return {id,revision:view.document.versionId};
 },{box});
}
test("durable conversation continues in background, accepts questions, restores the same Run and separates application from presentation",async({page})=>{
 // Run the existing app with OCCCCAD_AI_BASE_URL=http://127.0.0.1:19091.
 const provider=await startAssistantProvider(),errors:string[]=[],documents:string[]=[];page.on("pageerror",error=>errors.push(error.message));
 let connected=false;
 try{
  expect((await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
  const csrf=()=>page.context().cookies().then(cookies=>cookies.find(cookie=>cookie.name==="occccad_csrf")!.value);
  const email=`assistant-${crypto.randomUUID()}@test.invalid`,password=`Fixture-${crypto.randomUUID()}`;
  expect((await page.request.post("/api/admin/users",{headers:{"X-CSRF-Token":await csrf()},data:{email,displayName:"持续建模浏览器验收",password,status:"ACTIVE",platformRole:"MEMBER"}})).ok()).toBeTruthy();
  expect((await page.request.post("/api/auth/login",{data:{email,password}})).ok()).toBeTruthy();await page.goto("/");
  const seed=await createDocument(page);documents.push(seed.id);await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();
  const launcher=page.locator('footer[aria-label="工作台状态"]').getByRole("button",{name:"建模助手",exact:true});await launcher.click();
  const panel=page.getByRole("complementary",{name:"建模助手"});await expect(panel).toContainText("用对话开始建模");
  await page.screenshot({path:"../../../build/acceptance/assistant-connect-live.png",style:".cad-input-debug{visibility:hidden}"});
  await panel.getByLabel("API Key",{exact:true}).fill("synthetic-browser-assistant-key");await panel.getByRole("button",{name:"保存并连接",exact:true}).click();await expect(panel).toContainText("想完成什么样的模型");connected=true;
  await panel.getByRole("combobox",{name:"AI 模型",exact:true}).click();await page.getByRole("option",{name:"deepseek-v4-pro",exact:true}).click();
  await expect(panel.getByRole("combobox",{name:"AI 模型",exact:true})).toBeEnabled();await panel.getByRole("combobox",{name:"思考度",exact:true}).click();await page.getByRole("option",{name:"思考 · 高",exact:true}).click();
  await expect(panel.getByRole("combobox",{name:"思考度",exact:true})).toBeEnabled();
  const settings=await(await page.request.get("/api/assistant/settings")).json();expect(settings).toMatchObject({model:"deepseek-v4-pro",thinking:"high",tokenBudget:null});await expect(panel.getByRole("button",{name:"累计预算设置",exact:true})).toContainText("无累计上限");
  provider.holdNext();await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("在 XY 平面建立 80×50 mm 矩形，拉伸 12 mm，再对全部边做 R2 圆角。");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();
  await expect.poll(provider.blocked).toBeTruthy();const initial=await currentRun(page,seed.id);expect(initial.tokenBudget).toBeNull();
  await expect(panel.getByRole("textbox",{name:"建模需求",exact:true})).toBeEnabled();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("当前任务进展如何？");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel).toContainText("可以在运行中查询进展");
  await panel.getByRole("combobox",{name:"思考度",exact:true}).click();await page.getByRole("option",{name:"思考 · 关闭",exact:true}).click();await expect(panel.getByRole("combobox",{name:"思考度",exact:true})).toBeEnabled();
  await panel.getByRole("button",{name:"收起建模助手",exact:true}).click();await expect(panel).not.toBeVisible();provider.release();
  await expect.poll(async()=>(await currentRun(page,seed.id)).status,{timeout:90_000}).toBe("COMPLETED");
  const background=await currentRun(page,seed.id);expect(background.id).toBe(initial.id);expect(background.stageCount).toBe(2);expect(background.appliedCount).toBe(4);expect(background.presentedRevisionId).not.toBe(background.latestRevisionId);
  const final=await currentDocument(page,seed.id);expect(final.part.features.at(-1).type).toBe("FILLET");expect(final.part.features.at(-1).selections).toHaveLength(12);expect(final.part.features.at(-1).length).toBe(2);
  const volume=Object.values(final.artifacts as Record<string,{volume:number}>).reduce((sum,artifact)=>sum+artifact.volume,0);expect(volume).toBeGreaterThan(0);expect(volume).toBeLessThan(48000);
  const count=provider.calls();let rejectDisplay=true,failedDownloads=0;
  await page.route("**/api/documents/**/representations/**",async route=>{if(rejectDisplay&&route.request().url().includes(`/documents/${seed.id}/`)){failedDownloads++;await route.fulfill({status:503,body:"fixture display download unavailable"})}else await route.continue()});
  await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();
  await expect(page.getByRole("alert").filter({hasText:"几何显示加载失败"})).toBeVisible();expect(failedDownloads).toBeGreaterThan(0);await expect(panel).toContainText("目标已验证");
  const displayFailed=await currentRun(page,seed.id);expect(displayFailed.status).toBe("COMPLETED");expect(displayFailed.presentedRevisionId).not.toBe(final.document.versionId);
  let releaseDisplayRead:()=>void=()=>{};const displayReadGate=new Promise<void>(resolve=>releaseDisplayRead=resolve),documentRoute=`**/api/documents/${seed.id}`;
  await page.route(documentRoute,async route=>{await displayReadGate;await route.continue()});
  await panel.getByRole("button",{name:"重新显示结果",exact:true}).click();rejectDisplay=false;releaseDisplayRead();
  await expect.poll(async()=>(await currentRun(page,seed.id)).presentedRevisionId).toBe(final.document.versionId);expect(provider.calls()).toBe(count);expect((await currentDocument(page,seed.id)).document.versionId).toBe(final.document.versionId);await page.unroute("**/api/documents/**/representations/**");await page.unroute(documentRoute);
  await expect(panel.locator('[data-outcome="applied"]')).toContainText("模型已应用");await expect(panel.locator('[data-outcome="presented"]')).toContainText("当前结果已呈现");
  const stages=await(await page.request.get(`/api/documents/${seed.id}/assistant/runs/${initial.id}/stages`)).json();for(const stage of stages.items){const task=await(await page.request.get(`/api/documents/${seed.id}/automation/tasks/${stage.taskId}`)).json();for(const step of task.steps.filter((step:{phase:string})=>step.phase==="DISPLAYED"))expect(step.result.receipt.revisionId).toBe(final.document.versionId)}
  await page.screenshot({path:"../../../build/acceptance/assistant-chat-modeling-live.png",style:".cad-input-debug{visibility:hidden}"});

  // A new instruction invalidates an in-flight old plan within the same Run.
  const revised=await createDocument(page);documents.push(revised.id);await page.goto(`/documents/${revised.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();provider.holdNext();
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("在 XY 平面建立 80×50 mm 矩形，拉伸 12 mm，再对全部边做 R2 圆角。");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect.poll(provider.blocked).toBeTruthy();const beforeSteer=await currentRun(page,revised.id);
  await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("厚度改成 20 mm，后面不做圆角");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();provider.release();
  await expect.poll(async()=>(await currentRun(page,revised.id)).status,{timeout:90_000}).toBe("COMPLETED");const steered=await currentRun(page,revised.id);expect(steered.id).toBe(beforeSteer.id);expect(steered.goalVersion).toBe(2);expect(steered.verification.goalVersion).toBe(2);
  const revisedView=await currentDocument(page,revised.id);expect(revisedView.part.features.some((feature:{type:string})=>feature.type==="FILLET")).toBeFalsy();expect(revisedView.part.features.find((feature:{type:string})=>feature.type==="PAD").length).toBe(20);

  // An optional account default stops in WAITING_BUDGET; adjusting the active
  // budget continues the existing identity, without resending its goal.
  await panel.getByRole("button",{name:"累计预算设置",exact:true}).click();const popover=page.locator(".assistant-budget-editor");await popover.getByRole("checkbox",{name:"设置累计预算",exact:true}).check();await popover.getByRole("spinbutton",{name:"累计 Token 预算",exact:true}).fill("80000");await popover.getByRole("button",{name:"保存为新任务默认",exact:true}).click();
  await panel.getByRole("button",{name:"新建目标",exact:true}).click();await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("预算测试");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect.poll(async()=>(await currentRun(page,revised.id)).status).toBe("WAITING_BUDGET");const limited=await currentRun(page,revised.id);expect(limited.tokenBudget).toBe(80000);
  await panel.getByRole("button",{name:"累计预算设置",exact:true}).click();await popover.getByRole("spinbutton",{name:"累计 Token 预算",exact:true}).fill("300000");await popover.getByRole("button",{name:"应用到当前任务",exact:true}).click();await expect.poll(async()=>(await currentRun(page,revised.id)).status).not.toBe("WAITING_BUDGET");expect((await currentRun(page,revised.id)).id).toBe(limited.id);
  expect((await(await page.request.get("/api/assistant/settings")).json()).tokenBudget).toBe(80000);
  await panel.getByRole("button",{name:"累计预算设置",exact:true}).click();await popover.getByRole("checkbox",{name:"设置累计预算",exact:true}).uncheck();await popover.getByRole("button",{name:"保存为新任务默认",exact:true}).click();
  await panel.getByRole("button",{name:"连接设置",exact:true}).click();await expect(panel.getByLabel("API Key",{exact:true})).toHaveValue("");await panel.getByRole("button",{name:"断开连接",exact:true}).click();await expect(panel).toContainText("用对话开始建模");connected=false;
  expect(provider.protocolErrors()).toEqual([]);expect(provider.generations()[0]).toMatchObject({model:"deepseek-v4-pro",effort:"high",thinking:"enabled"});expect(provider.generations().some(value=>value.effort==="off"&&value.thinking==="disabled")).toBeTruthy();expect(await page.content()).not.toContain("private-fixture-reasoning");expect(errors).toEqual([]);
 }finally{
  provider.release();await stopFixtureRuns(page,documents);if(connected){const cookies=await page.context().cookies();await page.request.delete("/api/assistant/settings",{headers:{"X-CSRF-Token":cookies.find(cookie=>cookie.name==="occccad_csrf")?.value??""}})}await provider.close();
 }
});

test("frozen edge selection and native fillet handoff keep exact intent across changed selection, cancellation and reload",async({page})=>{
 const provider=await startAssistantProvider(),documents:string[]=[];let connected=false;
 try{
  expect((await page.request.post("/api/auth/login",{data:{email:process.env.OCCCCAD_ADMIN_EMAIL??"admin@occccad.local",password:process.env.OCCCCAD_ADMIN_PASSWORD}})).ok()).toBeTruthy();
  const csrf=()=>page.context().cookies().then(cookies=>cookies.find(cookie=>cookie.name==="occccad_csrf")!.value);
  const email=`handoff-${crypto.randomUUID()}@test.invalid`,password=`Fixture-${crypto.randomUUID()}`;
  expect((await page.request.post("/api/admin/users",{headers:{"X-CSRF-Token":await csrf()},data:{email,displayName:"AI 选区接管验收",password,status:"ACTIVE",platformRole:"MEMBER"}})).ok()).toBeTruthy();expect((await page.request.post("/api/auth/login",{data:{email,password}})).ok()).toBeTruthy();await page.goto("/");
  let seed=await createDocument(page,true);documents.push(seed.id);await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();const panel=page.getByRole("complementary",{name:"建模助手"});await panel.getByLabel("API Key",{exact:true}).fill("synthetic-browser-assistant-key");await panel.getByRole("button",{name:"保存并连接",exact:true}).click();connected=true;
  await expect(async()=>{await clickWorld(page,[10,0,10]);await expect(page.getByTestId("viewport-selection")).toContainText('"edge"',{timeout:1000})}).toPass({timeout:20_000});
  provider.holdNext();await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("给选中的边做 R1 圆角");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect.poll(provider.blocked).toBeTruthy();const frozen=await currentRun(page,seed.id);expect(frozen.scope.selectionCount).toBe(1);expect(frozen.scope.complete).toBe(true);expect(frozen.scope.selections[0].selection.expectedType).toBe("EDGE");
  await clickWorld(page,[0,-10,10]);provider.release();await expect.poll(async()=>(await currentRun(page,seed.id)).status,{timeout:90_000}).toBe("COMPLETED");const applied=await currentDocument(page,seed.id);expect(applied.part.features.at(-1).selections).toHaveLength(1);expect(applied.part.features.at(-1).selections[0].selection).toEqual(frozen.scope.selections[0].selection);
  seed=await createDocument(page,true);documents.push(seed.id);await page.goto(`/documents/${seed.id}`);await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();await clickWorld(page,[10,0,10]);
  let droppedOpenAck=false;
  await page.route("**/assistant/runs/*/control",async route=>{
   const input=route.request().postDataJSON();
   if(!droppedOpenAck&&input.action==="handoff_finish"&&input.outcome==="UI_OPENED"){
    await route.fetch();droppedOpenAck=true;await route.abort("failed");
   }else await route.continue();
  });
  await panel.getByRole("button",{name:"新建目标",exact:true}).click();await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("对选中的边做 R1 圆角，打开圆角工具让我手工确认");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await expect(panel.getByRole("button",{name:"打开工具",exact:true})).toBeVisible();await panel.getByRole("button",{name:"打开工具",exact:true}).click();const dialog=page.getByRole("dialog",{name:"创建 圆角",exact:true});await expect(dialog).toBeVisible();
  await expect.poll(()=>droppedOpenAck).toBe(true);const waiting=await currentRun(page,seed.id),count=provider.calls();expect(waiting.status).toBe("WAITING_UI");await page.reload();await expect(page.locator("canvas")).toBeVisible();await page.getByRole("button",{name:"建模助手",exact:true}).click();await expect(dialog).toHaveCount(0);await expect(panel).toContainText("不会自动重新打开");expect(provider.calls()).toBe(count);
  await panel.getByRole("button",{name:"已取消操作",exact:true}).click();await expect.poll(async()=>(await currentRun(page,seed.id)).status).not.toBe("COMPLETED");
  const cancelled=await currentRun(page,seed.id);if(cancelled.status!=="STOPPED"){await panel.getByRole("button",{name:"停止建模",exact:true}).click();await expect.poll(async()=>(await currentRun(page,seed.id)).status).toBe("STOPPED")}
  await clickWorld(page,[0,-10,10]);await panel.getByRole("button",{name:"新建目标",exact:true}).click();await panel.getByRole("textbox",{name:"建模需求",exact:true}).fill("对选中的边做 R1 圆角，打开圆角工具让我手工确认");await panel.getByRole("button",{name:"发送建模需求",exact:true}).click();await panel.getByRole("button",{name:"打开工具",exact:true}).click();await expect(dialog).toBeVisible();await dialog.getByRole("textbox").fill("1");await expect(dialog.getByRole("button",{name:/确\s*定/})).toBeEnabled();
  const issued=await currentRun(page,seed.id),task=await(await page.request.get(`/api/documents/${seed.id}/automation/tasks/${issued.activeTaskId}`)).json(),requestId=task.steps[0].handoff.domainRequestId;
  await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);await expect.poll(async()=>(await currentRun(page,seed.id)).status,{timeout:90_000}).toBe("COMPLETED");const completedTask=await(await page.request.get(`/api/documents/${seed.id}/automation/tasks/${task.id}`)).json();expect(completedTask.steps[0].result.receipt.requestId).toBe(requestId);expect(completedTask.steps[0].phase).toBe("UI_COMPLETED");
  await panel.getByRole("button",{name:"连接设置",exact:true}).click();await panel.getByRole("button",{name:"断开连接",exact:true}).click();connected=false;expect(provider.protocolErrors()).toEqual([]);
 }finally{provider.release();await stopFixtureRuns(page,documents);if(connected){const cookies=await page.context().cookies();await page.request.delete("/api/assistant/settings",{headers:{"X-CSRF-Token":cookies.find(cookie=>cookie.name==="occccad_csrf")?.value??""}})}await provider.close()}
});
