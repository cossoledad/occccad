import {expect,test,type Page} from "@playwright/test";
async function camera(page:Page) {
 const text=(await page.getByTestId("navigation-camera").textContent())!;
 const q=text.split(" / ")[1].split(",").map(Number);
 const [x,y,z,w]=q;
 return {direction:[-2*(x*z+w*y),-2*(y*z-w*x),-(1-2*(x*x+y*y))],zoom:Number(text.split("zoom ")[1])};
}
test("normal view aligns datum and exact face in a rotated occurrence without changing model",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");
 await expect(page.locator("canvas")).toBeVisible();
 await page.getByText("视图",{exact:true}).first().click();
 const normal=page.getByRole("toolbar",{name:"视图快捷操作"}).getByRole("button",{name:"法线视图",exact:true});
 await expect(normal).toBeDisabled();
 const filter=page.getByRole("textbox",{name:"筛选模型结构"});
 await filter.fill("XZ Plane");
 await page.getByRole("treeitem").filter({hasText:"XZ Plane"}).click();
 const before=await camera(page);
 await normal.click();
 await expect.poll(async()=>(await camera(page)).direction[1]).toBeCloseTo(1,3);
 expect((await camera(page)).zoom).toBe(before.zoom);
 await page.goto("/documents/mock-product-frame");
 await expect(page.locator("canvas")).toBeVisible();
 const revision=await page.evaluate(async()=>{
  const {api}=await import("/src/api/client.ts");
  const {queryClient}=await import("/src/app/providers.tsx");
  const view=await api.move("mock-product-frame","mock-instance-b",[45,0,10],[Math.SQRT1_2,0,0,Math.SQRT1_2]);
  await queryClient.invalidateQueries();return view.document.versionId;
 });
 await filter.fill("XY Plane");
 await page.getByRole("treeitem").filter({hasText:"XY Plane"}).last().click();
 await page.getByText("视图",{exact:true}).first().click();
 await normal.click();
 await expect.poll(async()=>(await camera(page)).direction[1]).toBeCloseTo(1,3);
 await page.getByRole("toolbar",{name:"视图快捷操作"}).getByRole("button",{name:"等轴测",exact:true}).click();
 await page.evaluate(async()=>{
  const {api}=await import("/src/api/client.ts");
  const {useWorkbenchStore}=await import("/src/state/workbench-store.ts");
  const view=await api.getDocument("mock-product-frame");
  const instance=view.resolvedInstances.find(v=>v.occurrencePath==="mock-instance-b");
  useWorkbenchStore.getState().setSelection({kind:"face",id:"normal-view-face",topologyId:1,geometryKey:instance.geometryKey,
    documentId:instance.documentId,versionId:instance.versionId,instanceId:"mock-instance-b",occurrencePath:instance.occurrencePath});
 });
 const faceZoom=(await camera(page)).zoom;
 await normal.click();
 await expect.poll(async()=>(await camera(page)).direction[1]).toBeCloseTo(1,3);
 expect((await camera(page)).zoom).toBe(faceZoom);
 expect(await page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-product-frame")).document.versionId;})).toBe(revision);
 // A late exact-surface query must not redirect the camera after selection changes.
 await page.getByRole("toolbar",{name:"视图快捷操作"}).getByRole("button",{name:"等轴测",exact:true}).click();
 const untouched=await page.getByTestId("navigation-camera").textContent();
 await page.evaluate(async()=>{
  const {api}=await import("/src/api/client.ts");
  const original=api.getTopologyProperties;
  api.getTopologyProperties=async(...args)=>{
   await new Promise(resolve=>{window.releaseNormalQuery=resolve;});
   const result=await original(...args);window.normalQueryFinished=true;
   api.getTopologyProperties=original;return result;
  };
 });
 await normal.click();
 await page.waitForFunction(()=>Boolean(window.releaseNormalQuery));
 await page.evaluate(async()=>{
  const {useWorkbenchStore}=await import("/src/state/workbench-store.ts");
  useWorkbenchStore.getState().setSelection(null);
 });
 await expect(normal).toBeDisabled();
 await page.evaluate(()=>window.releaseNormalQuery());
 await page.waitForFunction(()=>window.normalQueryFinished);
 await expect(page.getByTestId("navigation-camera")).toHaveText(untouched!);
 expect(errors).toEqual([]);
});
