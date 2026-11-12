import { expect, test, type Page } from "@playwright/test";
import { observeViewport, worldScreen } from "./camera-helpers";
import { treeNavigator } from "./tree-helpers";

test.use({ actionTimeout: 15_000 });
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => performance.setResourceTimingBufferSize(5000));
  const login = await page.request.post("/api/auth/login", { data: {
    email: process.env.OCCCCAD_ADMIN_EMAIL ?? "admin@occccad.local", password: process.env.OCCCCAD_ADMIN_PASSWORD,
  } });
  expect(login.ok()).toBeTruthy(); await page.goto("/");
});

async function createFixture(page: Page, withProduct = false) {
  return page.evaluate(async withProduct => {
    const { restApi: api } = await import("/src/api.ts");
    const folder = await api.createFolder(`工作台验收 ${Date.now()}`, "");
    let part = await api.createDocument("PART", "工作台零件", "", folder.id);
    const id = part.document.id;
    part = await api.command(id, { type: "CREATE_SKETCH", plane: "XY" });
    const profile = part.part!.features.at(-1)!.id;
    part = await api.command(id, { type: "EDIT_SKETCH", sketchId: profile, operations: [
      { type: "ADD_RECTANGLE", first: { x: -30, y: -20 }, second: { x: 30, y: 20 } },
    ] });
    part = await api.command(id, { type: "CREATE_SOLID_FEATURE", sketchId: profile, generator: "LINEAR_EXTRUDE", operation: "NEW_BODY", length: 12 });
    if (withProduct) part = await api.command(id, { type: "CREATE_SKETCH", plane: "XY" });
    let product;
    if (withProduct) {
      product = await api.createDocument("PRODUCT", "工作台装配", "", folder.id);
      product = await api.command(product.document.id, { type: "INSERT_INSTANCE", referencedDocumentId: id });
    }
    return { folder, id, head: part.document.versionId, sketch: part.part!.features.at(-1)!, productId: product?.document.id };
  }, withProduct);
}

async function cleanup(page: Page, folderId: string) {
  await page.evaluate(async id => { const { restApi: api } = await import("/src/api.ts"); await api.deleteFolder(id); }, folderId);
}

async function stripMetrics(page: Page) {
  return page.locator(".workbench-toolbar-scroll").evaluate(strip => {
    const bounds = strip.getBoundingClientRect(), root = document.documentElement;
    return {
      width: strip.clientWidth, content: strip.scrollWidth, height: strip.clientHeight, contentHeight: strip.scrollHeight,
      clipped: [...strip.querySelectorAll<HTMLElement>(".cad-tool-label")].filter(label => label.scrollWidth > label.clientWidth + 1).map(label => label.textContent),
      outside: [...strip.querySelectorAll<HTMLButtonElement>("button")].filter(button => { const r = button.getBoundingClientRect(); return r.left < bounds.left - 1 || r.right > bounds.right + 1 || r.top < bounds.top - 1 || r.bottom > bounds.bottom + 1; }).map(button => button.getAttribute("aria-label")),
      targetHeight: Math.min(...[...strip.querySelectorAll<HTMLElement>("button")].map(button => button.getBoundingClientRect().height)),
      pageOverflow: root.scrollWidth > root.clientWidth || root.scrollHeight > root.clientHeight,
      rows: Number(strip.closest<HTMLElement>(".workbench-command-deck")!.dataset.toolbarRows),
    };
  });
}

test("real workbench keeps the canvas stable and commands reachable across desktop widths", async ({ page }, info) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  const seed = await createFixture(page);
  try {
    await page.goto(`/documents/${seed.id}`); await observeViewport(page);
    await expect(page.getByRole("button", { name: "展开属性面板", exact: true })).toBeVisible();
    const point = await worldScreen(page, [0, 0, 12]); await page.mouse.click(point.x, point.y);
    const state = () => page.evaluate(() => { const e = (window as any).__cameraEngine; return {
      position: e.camera.position.toArray(), quaternion: e.camera.quaternion.toArray(), zoom: e.camera.zoom,
      target: e.navigation.target.toArray(), selected: JSON.stringify(e.selected),
    }; });
    const before = await state(), bounds = await page.locator("canvas").boundingBox();
    await page.getByRole("button", { name: "展开属性面板", exact: true }).click();
    await expect(page.getByRole("complementary", { name: "属性与历史", exact: true })).toBeVisible();
    expect(await page.locator("canvas").boundingBox()).toEqual(bounds); expect(await state()).toEqual(before);
    await page.screenshot({ path: info.outputPath("properties-overlay.png") });
    await page.getByRole("button", { name: "收起属性面板", exact: true }).click();
    await expect(page.getByRole("button", { name: "展开属性面板", exact: true })).toBeFocused();
    await page.getByRole("button", { name: "收起模型结构", exact: true }).click();
    expect(await page.locator("canvas").boundingBox()).toEqual(bounds); expect(await state()).toEqual(before);
    await page.getByRole("button", { name: "展开模型结构", exact: true }).click();
    expect(await page.locator("canvas").boundingBox()).toEqual(bounds); expect(await state()).toEqual(before);

    const metrics = [];
    for (const width of [1920, 1440, 1280, 1024, 800]) {
      await page.setViewportSize({ width, height: 1000 });
      for (const name of ["实体", "线框与曲面"]) {
        await page.getByRole("tab", { name, exact: true }).click();
        await expect.poll(async () => (await page.locator("canvas").boundingBox())!.width).toBe(width);
        await expect.poll(async () => (await stripMetrics(page)).clipped).toEqual([]);
        await expect.poll(async () => { const strip = await stripMetrics(page); return strip.content <= strip.width + 1 && strip.outside.length === 0; }).toBe(true);
        const strip = await stripMetrics(page); metrics.push({ width, name, ...strip });
        expect(strip.rows).toBe(2);
        expect(strip.content).toBeLessThanOrEqual(strip.width + 1);
        expect(strip.contentHeight).toBeLessThanOrEqual(strip.height);
        expect(strip.outside).toEqual([]); expect(strip.pageOverflow).toBe(false); expect(strip.targetHeight).toBeGreaterThanOrEqual(32);
        await expect(page.locator(".workbench-page-control")).toHaveCount(0);
        await expect(page.getByRole("button", { name: "搜索工具", exact: true })).toBeVisible();
        if (name === "实体") await expect(page.locator(".workbench-toolbar-scroll .cad-split-arrow")).toHaveCount(0);
        if (width >= 1280) await expect(page.getByRole("button", { name: "更多工具", exact: true })).toHaveCount(0);
        if (width === 1440) await page.screenshot({ path: info.outputPath(name === "实体" ? "part-desktop.png" : "surfaces-desktop.png") });
      }
    }
    // A narrow window keeps the same targets and two rows. Off-strip groups
    // remain reachable through a real menu, including portalled submenus.
    const more = page.getByRole("button", { name: "更多工具", exact: true });
    const selectedBeforeMenu = (await state()).selected;
    await more.press("Enter"); await page.keyboard.press("Escape");
    await expect(more).toHaveAttribute("aria-expanded", "false"); expect((await state()).selected).toBe(selectedBeforeMenu);
    await more.click(); await page.getByRole("menuitem", { name: "参考元素", exact: true }).hover();
    await page.getByRole("menuitem", { name: "几何图形集", exact: true }).click();
    const editor = page.getByRole("dialog", { name: "创建几何图形集", exact: true });
    await expect(editor).toBeVisible(); await editor.getByRole("button", { name: "关闭", exact: true }).click();
    await more.press("Enter"); await page.getByRole("menu").first().getByRole("menuitem").first().focus(); await page.keyboard.press("Escape");
    await expect(more).toBeFocused(); await expect(more).toHaveAttribute("aria-expanded", "false");
    await more.click(); const canvas = (await page.locator("canvas").boundingBox())!;
    await page.mouse.click(canvas.x + canvas.width * .5, canvas.y + canvas.height * .7);
    await expect(more).toHaveAttribute("aria-expanded", "false");
    await page.screenshot({ path: info.outputPath("surfaces-narrow.png") });
    await info.attach("responsive-toolbar.json", { body: JSON.stringify(metrics, null, 2), contentType: "application/json" });
    // Existing saved panel preferences remain effective, independently of the new default.
    await page.getByRole("button", { name: "展开属性面板", exact: true }).click(); await page.reload();
    await expect(page.getByRole("complementary", { name: "属性与历史", exact: true })).toBeVisible();
    expect((await (await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.head);
    expect(errors).toEqual([]);
  } finally { await cleanup(page, seed.folder.id); }
});

test("rectangle variants keep a session default through resizing and document tab changes", async ({ page, context }, info) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  const seed = await createFixture(page, true);
  try {
    await page.goto(`/documents/${seed.id}`); await observeViewport(page);
    await treeNavigator(page)(seed.sketch.name, `/sketch:${seed.sketch.id}`);
    await page.locator(`[role=treeitem][data-tree-key$="/sketch:${seed.sketch.id}"]`).dblclick();
    await expect(page.getByRole("button", { name: "退出草图", exact: true })).toBeVisible();
    const toolbar = page.locator(".workbench-toolbar-scroll");
    const rectangle = toolbar.locator(".cad-split-tool-button").filter({ has: page.getByRole("button", { name: "矩形方式", exact: true }) });
    for (const width of [1920, 1280, 1024]) {
      await page.setViewportSize({ width, height: 1000 });
      await expect(rectangle).toHaveCount(1);
      await expect(toolbar.getByRole("button", { name: "中心矩形", exact: true })).toHaveCount(0);
      await expect(toolbar.getByRole("button", { name: "定向矩形", exact: true })).toHaveCount(0);
      await expect.poll(async () => { const strip = await stripMetrics(page); return strip.content <= strip.width + 1 && strip.outside.length === 0; }).toBe(true);
      const metrics = await stripMetrics(page); expect(metrics.rows).toBe(2); expect(metrics.content).toBeLessThanOrEqual(metrics.width + 1); expect(metrics.outside).toEqual([]); expect(metrics.pageOverflow).toBe(false);
    }
    await rectangle.getByRole("button", { name: "矩形方式", exact: true }).click();
    await page.getByRole("menuitem", { name: "中心矩形", exact: true }).click();
    await expect(rectangle.getByRole("button", { name: "中心矩形", exact: true })).toHaveAttribute("aria-pressed", "true");
    await page.keyboard.press("Escape");
    await rectangle.getByRole("button", { name: "中心矩形", exact: true }).dblclick();
    await expect(page.locator(".workbench-current-tool")).toHaveText("中心矩形 · 连续执行");
    await page.keyboard.press("Escape");
    const arrow = rectangle.getByRole("button", { name: "矩形方式", exact: true });
    await arrow.click(); await page.getByRole("menuitem", { name: "定向矩形", exact: true }).focus(); await page.keyboard.press("Escape");
    await expect(arrow).toBeFocused(); await expect(page.getByRole("menu")).toHaveCount(0);
    await expect(rectangle.getByRole("button", { name: "中心矩形", exact: true })).toBeVisible();
    await page.setViewportSize({ width: 1440, height: 1000 });
    await expect.poll(async () => (await stripMetrics(page)).clipped).toEqual([]);
    await page.screenshot({ path: info.outputPath("sketch-desktop.png") });

    await page.getByRole("button", { name: "文档中心", exact: true }).click();
    await page.getByRole("button", { name: `打开文件夹 ${seed.folder.name}`, exact: true }).click();
    await page.locator(".document-card").getByRole("button", { name: "工作台装配", exact: true }).click();
    await expect(page.getByRole("tab", { name: "装配设计", exact: true })).toBeVisible();
    await page.screenshot({ path: info.outputPath("assembly-desktop.png") });
    const partTab = page.locator(".document-tab-switch").filter({ hasText: "工作台零件" });
    await partTab.click(); await expect(page.getByRole("button", { name: "退出草图", exact: true })).toBeVisible();
    await expect(rectangle.getByRole("button", { name: "中心矩形", exact: true })).toBeVisible();
    await partTab.focus(); await page.keyboard.press("Alt+ArrowLeft");
    await expect(partTab).toHaveAttribute("aria-current", "page");
    expect((await (await page.request.get(`/api/documents/${seed.id}`)).json()).document.versionId).toBe(seed.head);
    expect(await page.evaluate(() => sessionStorage.getItem("occccad.toolbar.variant.rectangle"))).toBe("sketch.rectangle.center");
    expect(await page.evaluate(() => localStorage.getItem("occccad.toolbar.variant.rectangle"))).toBeNull();
    const other = await context.newPage();
    try {
      await other.goto(`/documents/${seed.id}`);
      expect(await other.evaluate(() => sessionStorage.getItem("occccad.toolbar.variant.rectangle"))).toBeNull();
      await treeNavigator(other)(seed.sketch.name, `/sketch:${seed.sketch.id}`);
      await other.locator(`[role=treeitem][data-tree-key$="/sketch:${seed.sketch.id}"]`).dblclick();
      await expect(other.locator(".workbench-toolbar-scroll").getByRole("button", { name: "矩形", exact: true })).toBeVisible();
      await expect(other.locator(".workbench-toolbar-scroll").getByRole("button", { name: "中心矩形", exact: true })).toHaveCount(0);
    } finally { await other.close(); }
    await page.getByRole("button", { name: "关闭 工作台零件", exact: true }).click();
    await expect(page.getByRole("tab", { name: "装配设计", exact: true })).toBeVisible();
    await expect(page.locator(".document-tab-switch").filter({ hasText: "工作台零件" })).toHaveCount(0);
    expect(errors).toEqual([]);
  } finally { await cleanup(page, seed.folder.id); }
});
