import {expect,test} from "@playwright/test";

test("parameter double click, focused Enter and context edit share one editor",async({page})=>{
 const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill("Length");
 const row=page.getByRole("treeitem").filter({hasText:"Length"}).last(),dialog=page.getByRole("dialog",{name:"编辑参数",exact:true});
 await row.dblclick();await expect(dialog).toBeVisible();await expect(dialog.getByLabel("值或表达式")).toBeVisible();
 await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).not.toBeVisible();
 await row.focus();await page.keyboard.press("Enter");await expect(dialog).toBeVisible();
 await dialog.getByRole("button",{name:/取\s*消/}).click();await expect(dialog).not.toBeVisible();
 await row.click({button:"right"});await page.getByRole("menuitem",{name:/Length 对象/}).hover();await page.getByRole("menuitem",{name:"编辑",exact:true}).click();
 await expect(dialog).toBeVisible();await expect(dialog.getByLabel("值或表达式")).toHaveValue("16");
 await dialog.getByRole("button",{name:/取\s*消/}).click();expect(errors).toEqual([]);
});

test("sketch focus hides both scene grids and adapts sketch feedback to bright backgrounds",async({page},testInfo)=>{
 const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 const state=async()=>JSON.parse((await page.getByTestId("scene-grid").getAttribute("data-state"))!);
 await expect.poll(async()=>(await state()).ground).toBe(true);const before=await state();
 await page.getByRole("button",{name:"选项",exact:true}).click();const options=page.getByRole("dialog",{name:"选项",exact:true});
 await options.getByRole("button",{name:/明\s*亮/}).click();await options.getByRole("button",{name:/确\s*定/}).click();
 await expect.poll(async()=>(await state()).sketchGridColor).not.toBe(before.sketchGridColor);
 await page.getByRole("textbox",{name:"筛选模型结构"}).fill("Sketch");
 await page.getByRole("treeitem").filter({hasText:"Sketch"}).last().dblclick();await expect(page.getByRole("button",{name:"退出草图",exact:true})).toBeVisible();
 await expect.poll(async()=>{const s=await state();return [s.ground,s.world,s.sketch]}).toEqual([false,false,true]);
 await expect(page.getByTestId("navigation-camera")).toHaveAttribute("data-transition-active","false");
 await page.screenshot({path:testInfo.outputPath("bright-sketch.png")});
 await page.getByRole("button",{name:"退出草图",exact:true}).click();await expect.poll(async()=>(await state()).ground).toBe(true);
 await expect(page.getByTestId("navigation-camera")).toHaveAttribute("data-transition-active","false");
 const after=await state();expect(after.center).toEqual(before.center);expect(after.halfSize).toBe(before.halfSize);
 await page.screenshot({path:testInfo.outputPath("bright-bounded-ground.png")});expect(errors).toEqual([]);
});
