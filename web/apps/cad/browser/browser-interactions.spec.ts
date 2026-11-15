import { expect, test, type Locator } from "@playwright/test";
import { treeNavigator } from "./tree-helpers";

async function contextPrevented(target: Locator) {
  return target.evaluate(element => !element.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true })));
}

test("native history restores folders and filters, and tab closing replaces its visit", async ({ page }) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  await page.goto("/");
  await page.getByRole("button", { name: "打开文件夹 Concepts", exact: true }).click();
  await expect(page).toHaveURL(/folder=mock-folder/);
  await page.getByRole("button", { name: "新建文件夹", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新建文件夹", exact: true });
  await dialog.getByRole("textbox", { name: /文件夹名称/ }).fill("History child");
  await dialog.getByRole("button", { name: "创 建", exact: true }).click();
  await page.getByRole("button", { name: "打开文件夹 History child", exact: true }).click();
  await expect(page.locator(".folder-breadcrumb").getByRole("button", { name: "History child", exact: true })).toBeVisible();
  const child = page.url();
  const search = page.getByRole("textbox", { name: "搜索文档", exact: true });
  await search.fill("Mounting"); await expect(page).toHaveURL(/q=Mounting/);
  const library = page.url();
  await page.getByRole("button", { name: "打开 Mounting Bracket", exact: true }).click();
  await expect(page.locator("canvas")).toBeVisible();
  await page.goBack(); await expect(page).toHaveURL(library); await expect(search).toHaveValue("Mounting");
  await search.fill(""); await expect(page).toHaveURL(child);
  await page.goBack(); await expect(page).toHaveURL(/folder=mock-folder$/);
  await page.goForward(); await expect(page).toHaveURL(child);
  // CDP sends trusted browser side-button events, unlike dispatchEvent.
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchMouseEvent", { type: "mousePressed", x: 700, y: 220, button: "forward", buttons: 16, clickCount: 1 });
  await cdp.send("Input.dispatchMouseEvent", { type: "mouseReleased", x: 700, y: 220, button: "forward", buttons: 0, clickCount: 1 });
  await expect(page).toHaveURL(/\/documents\/mock-part-bracket$/);
  const tab = page.locator('.document-tab-switch[aria-current="page"]');
  const history = await page.evaluate(() => ({ length: window.history.length, idx: window.history.state.idx }));
  await tab.click(); expect(await page.evaluate(() => ({ length: window.history.length, idx: window.history.state.idx }))).toEqual(history);
  // Alt+Left remains browser navigation even while a tree resize button owns focus.
  await page.getByRole("button", { name: "收起模型结构", exact: true }).focus();
  await page.evaluate(() => window.addEventListener("keydown", event => {
    if(event.altKey && event.key === "ArrowLeft") (window as any).browserBackPrevented = event.defaultPrevented;
  }));
  await page.keyboard.press("Alt+ArrowLeft");
  expect(await page.evaluate(() => (window as any).browserBackPrevented)).toBe(false);
  // Headless Chromium need not execute browser chrome keyboard accelerators.
  // Verify the trusted native history button too, without navigating twice.
  if(page.url() !== child) {
    await cdp.send("Input.dispatchMouseEvent", { type: "mousePressed", x: 700, y: 400, button: "back", buttons: 8, clickCount: 1 });
    await cdp.send("Input.dispatchMouseEvent", { type: "mouseReleased", x: 700, y: 400, button: "back", buttons: 0, clickCount: 1 });
  }
  await expect(page).toHaveURL(child);
  await page.goForward(); await expect(page).toHaveURL(/\/documents\/mock-part-bracket$/);
  await page.getByRole("button", { name: "关闭 Mounting Bracket", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  await page.goBack(); await expect(page).toHaveURL(child);
  expect(errors).toEqual([]);
});

test("switching document hosts closes transient command forms on tab and history navigation", async ({ page }) => {
  await page.goto("/documents/mock-product-frame"); await expect(page.locator("canvas")).toBeVisible();
  await page.getByRole("button", { name: "文档中心", exact: true }).click();
  await page.getByRole("button", { name: "打开 Mounting Bracket", exact: true }).click();
  await treeNavigator(page)("Sketch 1"); await page.getByRole("treeitem").filter({hasText:"Sketch 1"}).click();
  await page.getByRole("button", { name: "拉伸", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "创建 拉伸", exact: true }); await expect(dialog).toBeVisible();
  await page.locator(".document-tab-switch").filter({hasText:"Frame Assembly"}).click();
  await expect(page).toHaveURL(/mock-product-frame$/); await expect(dialog).toHaveCount(0);
  await page.goBack(); await expect(page).toHaveURL(/mock-part-bracket$/); await expect(dialog).toHaveCount(0);
  await expect(page.locator("canvas")).toBeVisible();
});

test("late tab closure cannot redirect a page reached with Back", async ({page}) => {
  await page.goto("/?scope=parts&q=Mounting");
  await page.getByRole("button",{name:"打开 Mounting Bracket",exact:true}).click();await expect(page.locator("canvas")).toBeVisible();
  await page.evaluate(async()=>{
    // @ts-ignore Explicit Mock API latency injection, no domain or history fake.
    const {api}=await import('/src/api/client.ts');const close=api.closeOpenDocument;
    api.closeOpenDocument=async(id:string)=>{await new Promise<void>(resolve=>(window as any).completeTabClose=resolve);const result=await close(id);(window as any).tabCloseCompleted=true;return result};
  });
  await page.getByRole("button",{name:"关闭 Mounting Bracket",exact:true}).click();
  await expect.poll(()=>page.evaluate(()=>typeof(window as any).completeTabClose)).toBe("function");
  await page.goBack();await expect(page).toHaveURL(/scope=parts.*q=Mounting/);
  await page.evaluate(()=> (window as any).completeTabClose());
  await expect(page.getByRole("button",{name:"打开 Mounting Bracket",exact:true})).toBeVisible();
  await expect.poll(()=>page.evaluate(()=>Boolean((window as any).tabCloseCompleted))).toBe(true);
  await expect(page).toHaveURL(/scope=parts.*q=Mounting/);
});

test("application chrome rejects selection and native menus while editing and dialogs retain them", async ({ page }, info) => {
  await page.goto("/");
  const heading = page.getByRole("heading", { name: "全部文档", exact: true });
  expect(await contextPrevented(heading)).toBe(true);
  const rect = (await heading.boundingBox())!;
  await page.mouse.move(rect.x + 1, rect.y + 12); await page.mouse.down({ button: "right" });
  await page.mouse.move(1100, 700, { steps: 12 }); await page.mouse.up({ button: "right" });
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe("");
  await heading.click(); await page.keyboard.press("Control+a");
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe("");
  const search = page.getByRole("textbox", { name: "搜索文档", exact: true });
  await search.fill("Mounting"); await search.press("Control+a");
  await expect(page).toHaveURL(/q=Mounting/);
  expect(await search.evaluate((element: HTMLInputElement) => [element.selectionStart, element.selectionEnd])).toEqual([0, 8]);
  expect(await contextPrevented(search)).toBe(false);
  const card = page.locator(".document-card").filter({ hasText: "Mounting Bracket" });
  await card.click({ button: "right" });
  await page.getByRole("menuitem", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑文档", exact: true }); await expect(dialog).toBeVisible();
  const body = dialog.locator(".ant-modal-body"); expect(await contextPrevented(body)).toBe(false);
  const label = dialog.locator(".ant-form-item-label label").first();
  const box = (await label.boundingBox())!;
  await page.mouse.move(box.x, box.y + box.height / 2); await page.mouse.down();
  await page.mouse.move(box.x + box.width, box.y + box.height / 2, { steps: 8 }); await page.mouse.up();
  expect(await page.evaluate(() => window.getSelection()?.toString().length)).toBeGreaterThan(0);
  await page.screenshot({ path: info.outputPath("document-context-edit.png") });
  await dialog.getByRole("button", { name: "取 消", exact: true }).click();
  await search.fill(""); await expect(page).not.toHaveURL(/q=/);
  const folder = page.locator(".folder-card").filter({ hasText: "Concepts" }); await folder.click({ button: "right" });
  await page.getByRole("menuitem", { name: "打开", exact: true }).click(); await expect(page).toHaveURL(/folder=mock-folder/);
});

test("deep links normalize mechanism metadata through the router and library URLs survive reload", async ({ page }) => {
  await page.goto("/documents/mock-product-frame?application=mechanism&mechanismId=missing");
  await expect(page.locator("canvas")).toBeVisible(); await expect(page).toHaveURL(/\/documents\/mock-product-frame$/);
  const state = await page.evaluate(() => window.history.state); expect(state.idx).toBe(0); expect(typeof state.key).toBe("string");
  await page.getByRole("button", { name: "文档中心", exact: true }).click(); await page.goBack();
  await expect(page.locator("canvas")).toBeVisible(); await expect(page).toHaveURL(/\/documents\/mock-product-frame$/);
  await page.goto("/?scope=parts&q=Mounting&sort=name&view=list&size=50"); await page.reload();
  await expect(page.getByRole("textbox", { name: "搜索文档", exact: true })).toHaveValue("Mounting");
  await expect(page.locator(".document-grid.list")).toBeVisible(); await expect(page.getByRole("heading", { name: "零件", exact: true })).toBeVisible();
  // A pending debounced search cannot overwrite a historical destination.
  await page.getByRole("menuitem", { name: "全部文档", exact: true }).click();
  await page.getByRole("textbox", { name: "搜索文档", exact: true }).fill("not committed");
  await page.goBack(); await expect(page.getByRole("textbox", { name: "搜索文档", exact: true })).toHaveValue("Mounting");
  await page.waitForTimeout(350); await expect(page).toHaveURL(/scope=parts.*q=Mounting/);
});

test("login copy remains control-like and its text fields retain native editing", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("occccad.mock.auth", "false")); await page.goto("/");
  expect(await contextPrevented(page.locator(".auth-hero h2"))).toBe(true);
  const email = page.getByRole("textbox", { name: "电子邮箱", exact: true });
  await email.fill("designer@example.com"); expect(await contextPrevented(email)).toBe(false);
  await expect(email).toHaveCSS("user-select", "text"); await expect(page.locator(".auth-hero h2")).toHaveCSS("user-select", "none");
});

test("logout replaces its visit and history remains behind the authentication gate", async ({ page }) => {
  await page.goto("/documents/mock-part-bracket"); await expect(page.locator("canvas")).toBeVisible();
  await page.getByRole("button", { name: "文档中心", exact: true }).click();
  await page.getByRole("button", { name: "CAD Designer", exact: true }).hover();
  await page.getByRole("menuitem", { name: /退出登录/ }).click();
  const email=page.getByRole("textbox", {name:"电子邮箱",exact:true});await expect(email).toBeVisible();
  await page.goBack(); await expect(page).toHaveURL(/mock-part-bracket$/); await expect(email).toBeVisible();await expect(page.locator("canvas")).toHaveCount(0);
  await page.getByRole("button",{name:/^登\s*录$/,exact:true}).click();
  await expect(page.locator("canvas")).toBeVisible(); await expect(page).toHaveURL(/mock-part-bracket$/);
});
