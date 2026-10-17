import {expect,test} from "@playwright/test";

test("tree-only properties, consolidated object menu and no viewport context menu",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");const canvas=page.locator("canvas");await expect(canvas).toBeVisible();
 await canvas.click({button:"right",position:{x:(await canvas.boundingBox())!.width/2,y:25}});await expect(page.getByRole("menu")).toHaveCount(0);
 await page.keyboard.press("Alt+Enter");const dialog=page.getByRole("dialog",{name:/^属性 ·/});await expect(dialog).toBeVisible();
 await dialog.getByRole("textbox",{name:"属性名称"}).fill("属性测试文档");
 await expect(dialog.getByRole("button",{name:"场景属性…",exact:true})).toHaveCount(0);
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 await page.getByRole("treeitem").first().click({button:"right"});
 await expect(page.getByRole("menuitem",{name:"重命名",exact:true})).toHaveCount(0);await expect(page.getByRole("menuitem",{name:"颜色材质",exact:true})).toHaveCount(0);
 await page.getByRole("menuitem",{name:"属性…",exact:true}).click();await expect(dialog.getByRole("textbox",{name:"属性名称"})).toHaveValue("属性测试文档");
 await dialog.getByRole("button",{name:/取\s*消/}).click();expect(errors).toEqual([]);
});

test("one global options entry previews, cancels and persists across documents",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 const head=()=>page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-part-bracket")).document.versionId});const original=await head();
 const open=async()=>{await expect(page.getByRole("button",{name:"选项",exact:true})).toHaveCount(1);await page.getByRole("button",{name:"选项",exact:true}).click()};
 const dialog=page.getByRole("dialog",{name:"选项",exact:true});
 await open();await dialog.getByRole("tab",{name:"地面与网格",exact:true}).click();await dialog.getByRole("combobox",{name:"地面位置",exact:true}).click();await page.getByTitle("指定高度",{exact:true}).click();
 await dialog.getByRole("spinbutton",{name:"地面高度（mm）",exact:true}).fill("-42");await expect(dialog.getByLabel("当前场景地面")).toContainText("-42 mm");
 await dialog.getByRole("tab",{name:"光照",exact:true}).click();await dialog.getByRole("combobox",{name:"光照预设",exact:true}).click();await page.getByTitle("柔和",{exact:true}).click();
 await page.screenshot({path:"../../../build/acceptance/global-options-scene.png"});await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).not.toBeVisible();
 const settings=()=>page.evaluate(async()=>{const {useUIPreferences}=await import("/src/state/ui-preferences.ts");return useUIPreferences.getState().sceneSettings});
 expect((await settings()).groundHeight).toBe(-42);expect((await settings()).lighting).toBe("SOFT");expect(await head()).toBe(original);
 await open();await dialog.getByRole("tab",{name:"地面与网格",exact:true}).click();await dialog.getByRole("spinbutton",{name:"地面高度（mm）",exact:true}).fill("12");await expect(dialog.getByLabel("当前场景地面")).toContainText("12 mm");await dialog.getByRole("button",{name:/取\s*消/}).click();expect((await settings()).groundHeight).toBe(-42);
 await page.goto("/documents/mock-product-frame");await expect(page.locator("canvas")).toBeVisible();await open();await dialog.getByRole("tab",{name:"地面与网格",exact:true}).click();await expect(dialog.getByRole("spinbutton",{name:"地面高度（mm）",exact:true})).toHaveValue("-42");await dialog.getByRole("button",{name:/取\s*消/}).click();
 await page.reload();await expect(page.locator("canvas")).toBeVisible();expect((await settings()).groundHeight).toBe(-42);expect(errors).toEqual([]);
});
