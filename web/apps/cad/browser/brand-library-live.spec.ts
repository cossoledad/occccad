import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { observeViewport, worldScreen } from "./camera-helpers";

test.use({ actionTimeout: 15_000 });
test.beforeEach(async ({ page }) => { await page.addInitScript(() => performance.setResourceTimingBufferSize(5000)); });

async function login(page: Page) {
  const response = await page.request.post("/api/auth/login", { data: {
    email: process.env.OCCCCAD_ADMIN_EMAIL ?? "admin@occccad.local", password: process.env.OCCCCAD_ADMIN_PASSWORD,
  } });
  expect(response.ok()).toBeTruthy(); await page.goto("/");
}

test("login shows actual model images and remains usable on a narrow screen", async ({ page }, info) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  await page.goto("/"); await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
  for (const name of ["径流叶轮", "过渡壳体", "法兰联轴器"]) {
    await page.getByRole("button", { name, exact: true }).click();
    const image = page.getByAltText(`在 occccad 中建模的${name}`);
    await expect.poll(() => image.evaluate((node: HTMLImageElement) => node.complete && node.naturalWidth >= 1000)).toBe(true);
    await expect(page.getByRole("button", { name, exact: true })).toHaveAttribute("aria-pressed", "true");
  }
  await page.getByRole("button", { name: "径流叶轮", exact: true }).click();
  await page.screenshot({ path: info.outputPath("login-desktop.png"), animations: "disabled" });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: /注\s*册/ }).click();
  await expect(page.getByRole("textbox", { name: "显示名称", exact: true })).toBeVisible();
  await page.getByRole("button", { name: /登\s*录/ }).click();
  await page.getByRole("textbox", { name: "电子邮箱", exact: true }).fill(process.env.OCCCCAD_ADMIN_EMAIL ?? "admin@occccad.local");
  await page.getByLabel("密码", { exact: true }).fill(process.env.OCCCCAD_ADMIN_PASSWORD!);
  await page.getByRole("button", { name: /登\s*录/ }).click();
  await expect(page.getByRole("button", { name: "创建文档", exact: true })).toBeVisible();
  expect(await page.locator(".library-main").evaluate(node => node.getBoundingClientRect().width)).toBeGreaterThan(380);
  const trash = page.getByRole("menuitem", { name: "回收站", exact: true });
  await trash.scrollIntoViewIfNeeded(); await trash.click();
  await expect(page.getByRole("heading", { name: "回收站", exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("library-mobile.png") }); expect(errors).toEqual([]);
});

test("folders open by keyboard and selection actions appear when needed", async ({ page }, info) => {
  await login(page);
  const seed = await page.evaluate(async () => {
    const { restApi: api } = await import("/src/api.ts");
    const folder = await api.createFolder(`界面验收 ${Date.now()}`, "");
    const view = await api.createDocument("PART", "文档排版验收", "", folder.id);
    return { folder, id: view.document.id };
  });
  try {
    await page.reload(); const folder = page.getByRole("button", { name: `打开文件夹 ${seed.folder.name}`, exact: true });
    await folder.focus(); await folder.press("Enter"); await expect(page.locator(".folder-breadcrumb")).toContainText(seed.folder.name);
    const card = page.locator(".document-card").filter({ hasText: "文档排版验收" }); await expect(card).toBeVisible();
    await expect(page.getByRole("button", { name: "移入回收站", exact: true })).toHaveCount(0);
    await card.getByRole("checkbox").check(); await expect(page.getByRole("button", { name: "移入回收站", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "取消选择", exact: true }).click();
    await card.locator(".card-menu").click(); await page.getByRole("menuitem", { name: /导出/ }).click();
    const dialog = page.getByRole("dialog", { name: "导出“文档排版验收”", exact: true });
    await expect(dialog).not.toContainText("按文档保存"); await expect(dialog.getByRole("checkbox", { name: "不导出隐藏对象" })).toBeChecked();
    await expect(dialog.getByRole("button", { name: /导\s*出/ })).toBeVisible();
    await page.screenshot({ path: info.outputPath("export-dialog.png") }); await dialog.getByRole("button", { name: /取\s*消/ }).click();
    await page.getByRole("button", { name: "导入", exact: true }).click();
    const importing = page.getByRole("dialog", { name: "导入文档", exact: true });
    await expect(importing).toContainText("拖入文件或点击选择"); await expect(importing).not.toContainText("每个文件独立");
    await expect(importing.getByRole("button", { name: /导\s*入/ })).toBeDisabled();
    await importing.getByRole("button", { name: /取\s*消/ }).click();
    await card.getByRole("button", { name: "文档排版验收", exact: true }).click(); await expect(page).toHaveURL(new RegExp(`/documents/${seed.id}$`));
  } finally { await page.evaluate(async id => { const { restApi: api } = await import("/src/api.ts"); await api.deleteFolder(id); }, seed.folder.id); }
});

test("user management approves accounts and keeps secondary actions reachable", async ({ page, browser }, info) => {
  await login(page);
  const seed = await page.evaluate(async () => {
    const { restApi: api } = await import("/src/api.ts");
    return api.adminCreateUser({ email: `visual-${Date.now()}@example.invalid`, displayName: "界面验收账号", password: "Visual-example-12345", status: "PENDING", platformRole: "MEMBER" });
  });
  try {
    await page.getByRole("button", { name: "管理", exact: true }).click(); const drawer = page.getByRole("dialog", { name: "用户管理", exact: true });
    await drawer.getByRole("textbox", { name: "搜索账号", exact: true }).fill(seed.email);
    const row = drawer.getByRole("row").filter({ hasText: seed.email });
    await expect(row).toContainText("待审批"); await row.getByRole("button", { name: /批\s*准/ }).click(); await expect(row).toContainText("已启用");
    await row.getByRole("button", { name: "账号操作 界面验收账号", exact: true }).click(); await page.getByRole("menuitem", { name: /重置密码/ }).click();
    const reset = page.getByRole("dialog", { name: "重置密码 · 界面验收账号", exact: true });
    await reset.getByLabel("新密码", { exact: true }).fill("Updated-example-12345"); await reset.getByRole("button", { name: /确\s*定/ }).click(); await expect(reset).toHaveCount(0);
    const account = await browser.newContext({ baseURL: "http://127.0.0.1:5175" });
    try { expect((await account.request.post("/api/auth/login", { data: { email: seed.email, password: "Updated-example-12345" } })).ok()).toBe(true); } finally { await account.close(); }
    await page.setViewportSize({ width: 390, height: 844 }); expect((await drawer.boundingBox())!.width).toBeLessThanOrEqual(390);
    const more = row.getByRole("button", { name: "账号操作 界面验收账号", exact: true });
    const action = (await more.boundingBox())!; expect(action.x).toBeGreaterThanOrEqual(0); expect(action.x + action.width).toBeLessThanOrEqual(390);
    await page.screenshot({ path: info.outputPath("users-mobile.png") }); await page.setViewportSize({ width: 1440, height: 1000 });
    await row.getByRole("button", { name: "账号操作 界面验收账号", exact: true }).click(); await page.getByRole("menuitem", { name: /禁用账号/ }).click();
    await page.getByRole("button", { name: /禁\s*用/ }).click(); await expect(row).toContainText("已禁用");
    await page.screenshot({ path: info.outputPath("users-desktop.png") });
  } finally { await page.evaluate(async id => { const { restApi: api } = await import("/src/api.ts"); await api.adminDisableUser(id); }, seed.id); }
});

for (const dpr of [1, 2]) test.describe(`viewport PNG at DPR ${dpr}`, () => {
  test.use({ deviceScaleFactor: dpr });
  test("captures the actual frame and measurement labels without changing the document", async ({ page }, info) => {
    await login(page); const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
    const seed = await page.evaluate(async () => {
      const { restApi: api } = await import("/src/api.ts");
      let v = await api.createDocument("PART", `视口截图 ${Date.now()}`); const id = v.document.id;
      v = await api.command(id, { type: "CREATE_SKETCH", plane: "XY" }); const sketchId = v.part!.features.at(-1)!.id;
      v = await api.command(id, { type: "EDIT_SKETCH", sketchId, operations: [{ type: "ADD_RECTANGLE", first: { x: -30, y: -20 }, second: { x: 30, y: 20 } }] });
      v = await api.command(id, { type: "CREATE_SOLID_FEATURE", sketchId, generator: "LINEAR_EXTRUDE", operation: "ADD", length: 15 });
      const report = await api.measure(id, { revisionId: v.document.versionId, definition: { id: "", name: "实体体积", kind: "ITEM", supports: [{ kind: "BODY", bodyId: v.part!.activeBodyId, mode: "GEOMETRY" }], visible: true, showComponents: false } });
      v = await api.command(id, { type: "SAVE_MEASUREMENT", versionId: v.document.versionId, measurement: { ...report.definition, result: report.result } }); return { id, head: v.document.versionId };
    });
    try {
      await page.goto(`/documents/${seed.id}`); await observeViewport(page); await expect(page.locator(".measurement-label")).toHaveCount(1);
      const point = await worldScreen(page, [-20, -12, 15]); await page.mouse.click(point.x, point.y);
      const state = () => page.evaluate(() => { const e = (window as any).__cameraEngine; return { selected: JSON.stringify(e.selected), camera: e.camera.matrixWorld.toArray(), preserved: e.renderer.getContext().getContextAttributes().preserveDrawingBuffer }; });
      const before = await state(); expect(before.preserved).toBe(false);
      const downloaded = page.waitForEvent("download"); await page.getByRole("button", { name: "保存视口截图", exact: true }).click();
      const download = await downloaded, file = info.outputPath(`viewport-dpr${dpr}.png`); await download.saveAs(file); const data = await readFile(file);
      expect(data.subarray(1, 4).toString()).toBe("PNG");
      const dimensions = await page.locator("canvas").first().evaluate((canvas: HTMLCanvasElement) => ({ width: canvas.width, height: canvas.height, cssWidth: canvas.clientWidth, cssHeight: canvas.clientHeight }));
      expect(data.readUInt32BE(16)).toBe(dimensions.width); expect(data.readUInt32BE(20)).toBe(dimensions.height);
      expect(dimensions.width).toBe(dimensions.cssWidth * dpr); expect(dimensions.height).toBe(dimensions.cssHeight * dpr);
      const pixels = await page.evaluate(async ({ encoded, dpr }) => {
        const bytes = Uint8Array.from(atob(encoded), c => c.charCodeAt(0)), image = await createImageBitmap(new Blob([bytes], { type: "image/png" }));
        const canvas = document.createElement("canvas"); canvas.width = image.width; canvas.height = image.height;
        const ctx = canvas.getContext("2d")!; ctx.drawImage(image, 0, 0); image.close();
        const frame = document.querySelector(".cad-viewport-canvas canvas")!.getBoundingClientRect(), label = document.querySelector(".measurement-label rect")!.getBoundingClientRect();
        const pixel = [...ctx.getImageData(Math.round((label.x - frame.x + 5) * dpr), Math.round((label.y - frame.y + 5) * dpr), 1, 1).data];
        const data = ctx.getImageData(0, 0, canvas.width, canvas.height).data, colors = new Set<string>();
        for (let i = 0; i < data.length; i += 64) colors.add(`${data[i]},${data[i + 1]},${data[i + 2]}`);
        return { pixel, colors: colors.size };
      }, { encoded: data.toString("base64"), dpr });
      expect(pixels.colors).toBeGreaterThan(30); expect(pixels.pixel[0]).toBeLessThan(80); expect(pixels.pixel[1]).toBeLessThan(100); expect(pixels.pixel[2]).toBeLessThan(110);
      expect(await state()).toEqual(before); expect((await (await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.head); expect(errors).toEqual([]);
    } finally { await page.evaluate(async id => { const { restApi: api } = await import("/src/api.ts"); await api.deleteDocument(id); }, seed.id); }
  });
});
