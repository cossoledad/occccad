import {expect,test} from "@playwright/test";

test("viewport properties menu, Alt Enter and document rename",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");const canvas=page.locator("canvas");await expect(canvas).toBeVisible();
 await canvas.click({button:"right",position:{x:25,y:25}});
 await page.getByRole("menuitem",{name:"属性…",exact:true}).click();
 const dialog=page.getByRole("dialog",{name:/^属性 ·/});await expect(dialog).toBeVisible();
 await dialog.getByRole("textbox",{name:"属性名称"}).fill("属性测试文档");
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 await canvas.click({position:{x:25,y:25}});await page.keyboard.press("Alt+Enter");
 await expect(dialog.getByRole("textbox",{name:"属性名称"})).toHaveValue("属性测试文档");
 await dialog.getByRole("button",{name:/取\s*消/}).click();
 expect(errors).toEqual([]);
});

test("scene properties preview cancel and per-document persistence",async({page})=>{
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await page.goto("/documents/mock-part-bracket");await expect(page.locator("canvas")).toBeVisible();
 const head=()=>page.evaluate(async()=>{const {api}=await import("/src/api/client.ts");return (await api.getDocument("mock-part-bracket")).document.versionId});
 const original=await head();
 const open=async()=>{await page.getByRole("button",{name:"视区设置",exact:true}).click();await page.getByRole("button",{name:"场景属性…",exact:true}).click()};
 await open();const dialog=page.getByRole("dialog",{name:/^场景属性 ·/});
 await dialog.getByRole("combobox",{name:"地面位置",exact:true}).click();await page.getByTitle("指定高度",{exact:true}).click();
 await dialog.getByRole("spinbutton",{name:"地面高度（mm）",exact:true}).fill("-42");
 await expect(dialog.getByLabel("当前场景地面")).toContainText("-42 mm");
 await dialog.getByRole("tab",{name:"光照",exact:true}).click();await dialog.getByRole("combobox",{name:"光照预设",exact:true}).click();await page.getByTitle("柔和",{exact:true}).click();
 await page.screenshot({path:"../../../build/acceptance/scene-properties.png"});
 await dialog.getByRole("button",{name:/确\s*定/}).click();await expect(dialog).toHaveCount(0);
 const settings=()=>page.evaluate(async()=>{const {useUIPreferences}=await import("/src/state/ui-preferences.ts");return useUIPreferences.getState().documentSceneSettings["mock-part-bracket"]});
 expect((await settings()).groundHeight).toBe(-42);expect((await settings()).lighting).toBe("SOFT");expect(await head()).toBe(original);
 await open();await dialog.getByRole("spinbutton",{name:"地面高度（mm）",exact:true}).fill("12");
 await expect(dialog.getByLabel("当前场景地面")).toContainText("12 mm");await dialog.getByRole("button",{name:/取\s*消/}).click();
 expect((await settings()).groundHeight).toBe(-42);expect(await head()).toBe(original);
 await page.reload();await expect(page.locator("canvas")).toBeVisible();await open();await expect(dialog.getByRole("spinbutton",{name:"地面高度（mm）",exact:true})).toHaveValue("-42");
 expect(errors).toEqual([]);
});
