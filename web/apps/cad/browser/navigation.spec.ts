import {setProjection} from "./camera-helpers";
import { expect, test, type Page } from "@playwright/test";

// Orthographic clipping may move the eye along its viewing axis. Compare the
// displayed projection, rather than treating that invisible shift as a view change.
async function projectedView(page:Page){
 await expect(page.getByTestId("navigation-camera")).toHaveAttribute("data-transition-active","false");
 const value=(await page.getByTestId("navigation-camera").getAttribute("data-camera-state"))!;
 return page.evaluate(async value=>{
  // @ts-ignore Vite dependency
  const T=await import("/node_modules/.vite/deps/three.js");
  const state=JSON.parse(value),box=document.querySelector("canvas")!.getBoundingClientRect(),aspect=box.width/box.height;
  const span=state.span??300;const camera=state.projection==="perspective"?new T.PerspectiveCamera(state.fov,aspect,.000001,1e12):new T.OrthographicCamera(-span*aspect/2,span*aspect/2,span/2,-span/2,0,1e12);
  camera.position.fromArray(state.position);camera.quaternion.fromArray(state.quaternion);camera.zoom=state.zoom;camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
  return {rotation:state.quaternion as number[],zoom:camera.zoom,points:[[0,0,0],[1,0,0],[0,1,0],[0,0,1]].flatMap(p=>{const v=new T.Vector3(...p).project(camera);return [v.x*box.width/2,v.y*box.height/2]})};
 },value);
}
async function expectView(page:Page,view:Awaited<ReturnType<typeof projectedView>>){
 await expect.poll(async()=>{const current=await projectedView(page),dot=Math.abs(current.rotation.reduce((sum,v,i)=>sum+v*view.rotation[i],0));if(1-dot>1e-12||Math.abs(current.zoom-view.zoom)>view.zoom*1e-12)return Infinity;return Math.max(...current.points.map((v,i)=>Math.abs(v-view.points[i])))}).toBeLessThan(.02);
}

async function openNavigation(page: Page, mode: string) {
  await page.goto("/documents/mock-part-bracket");
  await expect(page.locator("canvas")).toBeVisible();
  await page.getByRole("button", { name: "选项", exact: true }).click();
  await page.getByRole("dialog",{name:"选项",exact:true}).getByRole("menuitem",{name:"导航与捕获",exact:true}).click();
  await page.getByRole("combobox", { name: "鼠标操作模式" }).click();
  await page.getByText(mode, { exact: true }).click();
  await page.getByRole("dialog", { name: "选项" }).getByRole("button", { name: /确\s*定/ }).click();
  await expect(page.getByRole("dialog", { name: "选项" })).not.toBeVisible();
  await expect(page.locator(".cad-input-debug")).toContainText(mode === "3DEXPERIENCE CATIA" ? "catia/none" : "solidworks/none");
  const box = (await page.locator("canvas").boundingBox())!;
  return { x: box.x + box.width / 2, y: box.y + box.height / 2, box };
}

test("CATIA native browser chords, sphere, zoom latch, cancellation and persistence", async ({ page }, testInfo) => {
  test.setTimeout(90_000);
  const errors: string[] = []; page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => { if (/THREE.WebGLProgram|VALIDATE_STATUS|Shader Error/.test(message.text())) errors.push(message.text()); });
  const { x, y } = await openNavigation(page, "3DEXPERIENCE CATIA");
  const debug = page.locator(".cad-input-debug"), camera = page.getByTestId("navigation-camera");
  await page.mouse.move(x, y); const initial = await camera.textContent();
  await page.mouse.wheel(0, 120); await expect(camera).toHaveText(initial!);
  await page.mouse.down({ button: "middle" }); await page.mouse.move(x + 30, y + 15, { steps: 3 });
  await expect(debug).toContainText("catia/pan"); await expect(camera).not.toHaveText(initial!);
  await page.mouse.down({ button: "right" }); await page.mouse.move(x + 80, y + 50, { steps: 4 });
  await expect(debug).toContainText("catia/orbit");
  await expect(debug).toContainText("HUD: hidden");
  await page.screenshot({ path: testInfo.outputPath("catia-native-rotation.png") });
  await page.mouse.up({ button: "right" });
  await expect(debug).toContainText("catia/zoom");
  const rotated = await camera.textContent();
  await page.mouse.move(x + 80, y, { steps: 4 }); await expect(camera).not.toHaveText(rotated!);
  await page.mouse.down({ button: "left" }); await expect(debug).toContainText("catia/orbit");
  await page.mouse.up({ button: "middle" }); await page.mouse.up({ button: "left" });
  await expect(debug).toContainText("catia/none"); await expect(debug).toContainText("HUD: hidden");
  await page.keyboard.down("Control"); await page.mouse.down({ button: "middle" });
  await expect(debug).toContainText("catia/zoom"); await page.keyboard.press("Escape");
  await page.mouse.up({ button: "middle" }); await page.keyboard.up("Control"); await expect(debug).toContainText("catia/none");
  await page.reload(); await expect(page.locator(".workbench-navigation-label")).toHaveText("3DEXPERIENCE CATIA 导航");
  await page.getByRole("button", { name: "选项", exact: true }).click();
  await page.getByRole("dialog",{name:"选项",exact:true}).getByRole("menuitem",{name:"导航与捕获",exact:true}).click();
  await page.getByRole("checkbox", { name: "显示旋转球" }).check();
  await page.getByRole("dialog", { name: "选项" }).getByRole("button", { name: /确\s*定/ }).click();
  await expect(page.getByRole("dialog", { name: "选项" })).not.toBeVisible();
  await page.mouse.move(x, y); await page.mouse.down({ button: "middle" }); await page.mouse.down({ button: "left" });
  await expect(debug).not.toContainText("HUD: hidden");
  await page.screenshot({ path: testInfo.outputPath("catia-optional-sphere.png") });
  await page.mouse.up({ button: "left" }); await page.mouse.up({ button: "middle" });
  await page.reload();
  await page.getByRole("button", { name: "选项", exact: true }).click();
  await page.getByRole("dialog",{name:"选项",exact:true}).getByRole("menuitem",{name:"导航与捕获",exact:true}).click();
  await expect(page.getByRole("checkbox", { name: "显示旋转球" })).toBeChecked();
  expect(errors).toEqual([]);
});

test("SOLIDWORKS modifiers, entity reference, middle double click and focus loss", async ({ page }, testInfo) => {
  test.setTimeout(90_000);
  const errors: string[] = []; page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => { if (/THREE.WebGLProgram|VALIDATE_STATUS|Shader Error/.test(message.text())) errors.push(message.text()); });
  const { x, y } = await openNavigation(page, "SOLIDWORKS");
  const debug = page.locator(".cad-input-debug"), camera = page.getByTestId("navigation-camera");
  await page.mouse.move(x, y); const initial = await camera.textContent();
  await page.mouse.click(x, y, { button: "middle" });
  await expect(debug).toContainText("Reference: face"); await expect(camera).toHaveText(initial!);
  await page.screenshot({ path: testInfo.outputPath("solidworks-reference.png") });
  await page.mouse.down({ button: "middle" }); await page.mouse.move(x + 40, y + 20, { steps: 3 });
  await expect(debug).toContainText("solidworks/orbit"); await expect(camera).not.toHaveText(initial!);
  await page.keyboard.down("Control"); await expect(debug).toContainText("solidworks/pan");
  await page.mouse.move(x + 60, y + 40, { steps: 3 });
  await page.keyboard.up("Control"); await page.keyboard.down("Shift"); await expect(debug).toContainText("solidworks/zoom");
  await page.mouse.move(x + 60, y, { steps: 3 });
  await page.keyboard.up("Shift"); await page.keyboard.down("Alt"); await expect(debug).toContainText("solidworks/roll");
  await page.mouse.move(x + 90, y, { steps: 3 }); await page.keyboard.up("Alt"); await page.mouse.up({ button: "middle" });
  await expect(debug).toContainText("Reference: none");
  const moved = await camera.textContent();
  await page.mouse.dblclick(x, y, { button: "middle", delay: 70 }); await expect(camera).not.toHaveText(moved!);
  await page.mouse.down({ button: "middle" }); await page.evaluate(() => window.dispatchEvent(new Event("blur")));
  await expect(debug).toContainText("solidworks/none"); await page.mouse.up({ button: "middle" });
  expect(errors).toEqual([]);
});

test("orthographic detail navigation, isometric shortcut and sketch view round trip", async ({ page }, testInfo) => {
  test.setTimeout(90_000);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => { if (/THREE.WebGLProgram|VALIDATE_STATUS|Shader Error/.test(message.text())) errors.push(message.text()); });
  await page.goto("/documents/mock-part-bracket");
  await setProjection(page,"平行（无透视）");
  const canvas = page.locator("canvas"), camera = page.getByTestId("navigation-camera");
  const projection = page.getByTestId("navigation-projection");
  await expect(projection).toContainText("orthographic");
  const bounds = (await canvas.boundingBox())!;
  const x = bounds.x + bounds.width / 2, y = bounds.y + bounds.height / 2;
  // Establish the ISO Fit baseline in the selected projection after conversion.
  await canvas.focus();await page.keyboard.press("1");
  await expect(camera).toHaveAttribute("data-transition-active","false");
  const initialScale = await projection.textContent();
  await page.mouse.move(x, y); await page.mouse.wheel(0, -1800);
  await expect(projection).not.toHaveText(initialScale!);
  await canvas.focus();
  let scale = await projection.textContent();
  await page.mouse.down({ button: "right" }); await page.mouse.move(x + 23, y + 15, { steps: 3 }); await page.mouse.up({ button: "right" });
  const rotatedCamera = await camera.textContent();
  await page.keyboard.press("1");
  await expect(camera).not.toHaveText(rotatedCamera!);
  await expect(projection).not.toHaveText(scale!);
  await expect(projection).toHaveText(initialScale!);
  scale = await projection.textContent();
  await page.screenshot({ path: testInfo.outputPath("isometric-fit-grid.png") });
  const iso = await projectedView(page);
  await page.keyboard.press("1"); await expectView(page,iso);
  // Beyond the old 1200-unit grid: the background plane still fills the viewport.
  await page.mouse.wheel(0, 3000); await expect(projection).not.toHaveText(scale!);
  await page.mouse.down({ button: "middle" }); await page.mouse.move(x + 200, y + 80, { steps: 3 }); await page.mouse.up({ button: "middle" });
  await page.screenshot({ path: testInfo.outputPath("unbounded-grid-zoomed-out.png") });
  await page.keyboard.press("1"); await expectView(page,iso);
  await page.mouse.down({ button: "middle" }); await page.mouse.move(x + 40, y + 25, { steps: 3 }); await page.mouse.up({ button: "middle" });
  await expect(projection).toHaveText(scale!);
  const beforeSketch = await projectedView(page);
  const filter = page.getByRole("textbox", { name: "筛选模型结构" });
  await filter.fill("XY Plane");
  await page.getByRole("treeitem").filter({ hasText: "XY Plane" }).click();
  await page.getByRole("button", { name: "草图", exact: true }).click();
  await expect(page.getByRole("button", { name: "退出草图", exact: true })).toBeVisible();
  await expect(projection).toHaveText(scale!);
  await page.mouse.move(x, y); await page.mouse.wheel(0, 250);
  await expect(projection).not.toHaveText(scale!);
  await page.getByRole("button", { name: "退出草图", exact: true }).click();
  await expectView(page,beforeSketch);
  await page.screenshot({ path: testInfo.outputPath("orthographic-detail-restored.png") });
  expect(errors).toEqual([]);
});

test("selected face sketch keeps scale and restores the modelling view", async ({ page }) => {
  await page.goto("/documents/mock-part-bracket");
  const camera = page.getByTestId("navigation-camera"), projection = page.getByTestId("navigation-projection");
  await expect(projection).toContainText("perspective");
  const box = (await page.locator("canvas").boundingBox())!;
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  const before = await camera.textContent(), scale = await projection.textContent();
  const sketch = page.getByRole("button", { name: "草图", exact: true });
  await expect(sketch).toBeEnabled(); await sketch.click();
  const exit = page.getByRole("button", { name: "退出草图", exact: true });
  await expect(exit).toBeVisible(); await expect(projection).toHaveText(scale!);
  await expect(camera).not.toHaveText(before!);
  await exit.click(); await expect(camera).toHaveText(before!);
});

test("switching documents clears the previous sketch camera session", async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto("/documents/mock-part-bracket");
  await expect(page.getByTestId("navigation-projection")).toContainText("perspective");
  await page.getByRole("button", { name: "文档中心", exact: true }).click();
  await page.locator(".document-card").filter({ hasText: "Frame Assembly" }).locator(".thumbnail-button").dblclick();
  await expect(page.getByRole("button", { name: "插入", exact: true })).toBeVisible();
  const assemblyCamera = await page.getByTestId("navigation-camera").textContent();
  await page.locator(".document-tab-switch").filter({ hasText: "Mounting Bracket" }).click();
  await expect(page.getByRole("button", { name: "草图", exact: true })).toBeVisible();
  const filter = page.getByRole("textbox", { name: "筛选模型结构" });
  await filter.fill("XY Plane");
  await page.getByRole("treeitem").filter({ hasText: "XY Plane" }).click();
  await page.getByRole("button", { name: "草图", exact: true }).click();
  await expect(page.getByRole("button", { name: "退出草图", exact: true })).toBeVisible();
  await page.locator(".document-tab-switch").filter({ hasText: "Frame Assembly" }).click();
  await expect(page.getByRole("button", { name: "插入", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "退出草图", exact: true })).not.toBeVisible();
  await expect(page.getByTestId("navigation-camera")).toHaveText(assemblyCamera!);
});

test("sketch normal view, persistent world context and exit selection synchronization", async ({ page }, testInfo) => {
  test.setTimeout(90_000);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => { if (/THREE.WebGLProgram|VALIDATE_STATUS|Shader Error/.test(message.text())) errors.push(message.text()); });
  await page.goto("/documents/mock-part-bracket");
  const filter = page.getByRole("textbox", { name: "筛选模型结构" });
  await filter.fill("Sketch 1");
  const sketch = page.getByRole("treeitem").filter({ hasText: "Sketch 1" });
  // This consumed sketch is hidden in the fixture. Explicitly show it so the
  // exit-selection assertion can require a visible highlight without bypassing
  // the model's definition visibility contract.
  await sketch.click({button:"right"});await page.getByRole("menuitem",{name:"显示",exact:true}).click();
  await sketch.dblclick();
  const normal = page.getByRole("button", { name: "正对草图平面", exact: true });
  await expect(normal).toBeVisible();
  const camera = page.getByTestId("navigation-camera"), projection = page.getByTestId("navigation-projection");
  // The toolbar appears before the transition finishes; sample the final XY view.
  await expect.poll(async()=>(await camera.innerText()).split(" / ")[1]).toBe("0.0000,0.0000,0.0000,1.0000");
  const initial = await camera.textContent(), scale = await projection.textContent();
  const box = (await page.locator("canvas").boundingBox())!;
  const x = box.x + box.width/2, y = box.y + box.height/2;
  await page.mouse.move(x,y); await page.mouse.down({button:"right"});
  await page.mouse.move(x+110,y+80,{steps:5}); await page.mouse.up({button:"right"});
  await expect(camera).not.toHaveText(initial!);
  await page.screenshot({path:testInfo.outputPath("sketch-oblique-world-and-plane-grids.png")});
  await normal.click(); await expect(projection).toHaveText(scale!);
  await expect.poll(async () => (await camera.textContent())?.split(" / ")[1]).toBe(initial!.split(" / ")[1]);
  const beforeWheel=await camera.getAttribute("data-camera-state");
  await page.mouse.move(x,y); await page.mouse.wheel(0,2300); await expect(camera).not.toHaveAttribute("data-camera-state",beforeWheel!);
  await page.screenshot({path:testInfo.outputPath("sketch-infinite-plane-grid.png")});
  await page.getByRole("button",{name:"退出草图",exact:true}).click();
  const selection = page.getByTestId("viewport-selection");
  await expect(sketch).toHaveAttribute("aria-selected","true");
  await expect(selection).toHaveAttribute("data-count","1");
  await expect.poll(async () => Number(await selection.getAttribute("data-highlighted"))).toBeGreaterThan(0);
  await expect(page.getByRole("button",{name:"拉伸",exact:true})).toBeEnabled();
  await page.screenshot({path:testInfo.outputPath("sketch-exit-selected.png")});
  await page.mouse.click(box.x+box.width-12,box.y+12);
  await expect(selection).toHaveAttribute("data-count","0");
  await expect(sketch).toHaveAttribute("aria-selected","false");
  // The current feature command may start without a preselection. It must ask
  // for a profile instead of silently reusing the cleared sketch selection.
  await page.getByRole("button",{name:"拉伸",exact:true}).click();
  const extrude=page.getByRole("dialog",{name:/拉伸/});await expect(extrude).toContainText("请在视图区选择草图");
  await extrude.getByRole("button",{name:/取\s*消/}).click();
  expect(errors).toEqual([]);
});
