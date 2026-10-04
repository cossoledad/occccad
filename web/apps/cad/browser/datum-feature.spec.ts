import {expect,test,type Page} from "@playwright/test";
async function openCommand(page:Page, query:string, title:string) {
 await page.getByRole("button",{name:"搜索工具",exact:true}).click();
 await page.getByRole("textbox",{name:"搜索工具名称或用途"}).fill(query);
 await page.locator(".workbench-command-result").filter({has:page.locator("strong",{hasText:new RegExp(`^${query}$`)})}).click();
 const dialog=page.getByRole("dialog",{name:title,exact:true});await expect(dialog).toBeVisible();return dialog;
}
test("datum editors preview arbitrary geometry, copy references and delete through tree",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 const initial=await page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-part-bracket")).document.versionId;});
 const camera=await page.getByTestId("navigation-camera").textContent();
 let dialog=await openCommand(page,"基准面","创建基准面");
 await dialog.getByRole("spinbutton",{name:"中心（mm） X",exact:true}).fill("20");
 await dialog.getByRole("spinbutton",{name:"法向 X",exact:true}).fill("1");
 await expect(page.getByTestId("viewport-datum-preview")).toContainText('"origin":[20,0,0]');
 await expect(page.getByTestId("viewport-datum-preview")).toContainText('"direction":[0.707106');
 const afterCamera=(await page.getByTestId("navigation-camera").textContent())!;
 expect(afterCamera.split(" / ").slice(1)).toEqual(camera!.split(" / ").slice(1));
 // Clipping may move the orthographic eye along its isometric view axis.
 const position=(text:string)=>text.split(" / ")[0].replace("Camera: ","").split(",").map(Number);
 const beforePosition=position(camera!),afterPosition=position(afterCamera),delta=afterPosition.map((v,i)=>v-beforePosition[i]);
 expect(Math.abs(delta[0]+delta[1])).toBeLessThan(.001);expect(Math.abs(delta[0]-delta[2])).toBeLessThan(.001);
 await page.screenshot({path:"/tmp/occccad-datum-plane-preview.png"});
 await dialog.getByRole("button",{name:"关闭",exact:true}).click();
 await expect(page.getByTestId("viewport-datum-preview")).toHaveCount(0);
 expect(await page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-part-bracket")).document.versionId;})).toBe(initial);
 // A copied face uses its bounded center, not the analytic surface origin.
 await page.evaluate(async()=>{
  const {api}=await import("/src/api/client.ts");const {useWorkbenchStore}=await import("/src/state/workbench-store.ts");
  const view=await api.getDocument("mock-part-bracket");const body=view.part!.bodies[0];
  const original=api.getTopologyProperties;api.getTopologyProperties=async(...args)=>({...await original(...args),geometryType:"PLANE",properties:{origin:[0,0,0],snapCenter:[8,9,10],normal:[0,0,1],xDirection:[1,0,0]}});
  useWorkbenchStore.getState().setSelection({kind:"face",id:"face",entityId:"face",topologyId:1,geometryKey:body.geometryKey!,bodyId:body.id,documentId:view.document.id,versionId:view.document.versionId});
 });
 dialog=await openCommand(page,"基准面","创建基准面");
 await expect(page.getByTestId("viewport-datum-preview")).toContainText('"origin":[8,9,10]');
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});await filter.fill("Plane");
 const copiedPlane=page.getByRole("treeitem").filter({hasText:/^Plane$/});
 await expect(copiedPlane).toBeVisible();await copiedPlane.click({button:"right"});
 await page.getByRole("menuitem",{name:"删除",exact:true}).click();
 await expect.poll(async()=>page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-part-bracket")).part!.datumPlanes.filter(p=>p.plane==="CUSTOM").length;})).toBe(0);
 await filter.fill("");
 await page.evaluate(async()=>{
  const {api}=await import("/src/api/client.ts");const {useWorkbenchStore}=await import("/src/state/workbench-store.ts");const view=await api.getDocument("mock-part-bracket");
  useWorkbenchStore.getState().setSelection({kind:"axis",axis:"X",id:"root:axis-system-default:X",entityId:view.axisSystems![0].id,documentId:view.document.id,versionId:view.document.versionId});
 });
 dialog=await openCommand(page,"基准轴","创建基准轴");
 await expect(page.getByTestId("viewport-datum-preview")).toContainText('"direction":[1,0,0]');
 await dialog.getByRole("spinbutton",{name:"原点（mm） Y",exact:true}).fill("5");
 await expect(page.getByTestId("viewport-datum-preview")).toContainText('"origin":[0,5,0]');
 await page.screenshot({path:"/tmp/occccad-datum-axis-preview.png"});
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 await filter.fill("Axis");
 const custom=page.getByRole("treeitem").filter({hasText:/^Axis$/});
 await expect(custom).toBeVisible();await custom.click();
 await expect(page.getByRole("complementary",{name:"属性与历史"})).toContainText("0, 5, 0");
 await custom.click({button:"right"});
 await page.getByRole("menuitem",{name:"删除",exact:true}).click();
 await expect.poll(async()=>page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-part-bracket")).part!.datumAxes!.length;})).toBe(0);
 expect(errors).toEqual([]);
});
