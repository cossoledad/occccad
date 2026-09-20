import { expect, test, type Page } from "@playwright/test";

async function openPart(page: Page) {
  await page.goto("/documents/mock-part-bracket");
  await expect(page.getByRole("textbox", { name: "筛选模型结构" })).toBeVisible();
  await expect(page.locator("canvas")).toBeVisible();
}

// Uses the isolated Mock Adapter. These checks verify UI contracts, not B-Rep correctness.
test("contextual commands, search, sketch tools and model tree", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openPart(page);
  await page.getByRole("button", { name: "搜索工具", exact: true }).click();
  const search = page.getByRole("textbox", { name: "搜索工具名称或用途" });
  await search.fill("拉伸");
  await expect(page.locator(".workbench-command-result")).toHaveCount(1);
  await expect(page.locator(".workbench-command-result")).toBeDisabled();
  await search.fill("参数");
  await page.locator(".workbench-command-result").filter({ hasText: "集中查看" }).click();
  const parameters = page.getByRole("dialog", { name: "参数", exact: true });
  await expect(parameters).toBeVisible();
  await parameters.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(parameters).toHaveCount(0);

  const filter = page.getByRole("textbox", { name: "筛选模型结构" });
  await filter.fill("XY Plane");
  await expect(page.getByRole("treeitem")).toHaveCount(3);
  const xy = page.getByRole("treeitem").filter({ hasText: "XY Plane" });
  await xy.click();
  await expect(xy).toHaveAttribute("aria-selected", "true");
  await page.getByRole("button", { name: "草图", exact: true }).click();
  await expect(page.getByRole("button", { name: "退出草图", exact: true })).toBeVisible();
  await filter.fill("");
  await page.getByRole("button", { name: "直线", exact: true }).dblclick();
  await expect(page.locator(".workbench-current-tool")).toHaveText("直线 · 连续执行");
  await page.getByRole("button", { name: "选择", exact: true }).click();
  await expect(page.getByRole("button", { name: "选择", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "直线", exact: true })).toHaveAttribute("aria-pressed", "false");
  await page.getByRole("button", { name: "退出草图", exact: true }).click();
  const root = page.getByRole("treeitem").first();
  await root.focus();
  await page.keyboard.press("ArrowDown");
  await expect(root).not.toBeFocused();
  await expect(page.getByRole("treeitem").nth(1)).toBeFocused();
  await page.keyboard.press("Home");
  await expect(root).toBeFocused();
  expect(await filter.evaluate((element) => {
    const event = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
    element.dispatchEvent(event);
    return event.defaultPrevented;
  })).toBe(false);
  expect(errors).toEqual([]);
});

test("panel layout, viewport resizing, document switching and assembly", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openPart(page);
  const canvasWidth = () => page.locator("canvas").evaluate((canvas) => canvas.getBoundingClientRect().width);
  await expect.poll(canvasWidth).toBeGreaterThan(700);
  await page.getByRole("button", { name: "收起属性面板" }).click();
  await page.getByRole("button", { name: "收起模型结构" }).click();
  await expect.poll(canvasWidth).toBe(1440);
  await page.getByRole("button", { name: "展开模型结构" }).click();
  await page.getByRole("button", { name: "展开属性面板" }).click();
  const separator = page.getByRole("separator", { name: "调整结构树宽度" });
  await separator.focus(); await page.keyboard.press("ArrowRight");
  await expect(separator).toHaveAttribute("aria-valuenow", "346");
  await page.setViewportSize({ width: 1024, height: 768 });
  await expect.poll(canvasWidth).toBeGreaterThan(400);
  await expect(separator).toHaveAttribute("aria-valuenow", "250");
  await page.setViewportSize({ width: 768, height: 700 });
  await page.getByRole("button", { name: "收起属性面板" }).click();
  await expect.poll(canvasWidth).toBeGreaterThan(500);
  await page.getByRole("button", { name: "参数", exact: true }).click();
  const parameters = page.getByRole("dialog", { name: "参数", exact: true });
  await expect(parameters).toBeVisible();
  const bounds = await parameters.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(768);
  await page.keyboard.press("Escape");
  await expect(parameters).toHaveCount(0);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole("button", { name: "文档中心", exact: true }).click();
  await page.locator(".document-card").filter({ hasText: "Frame Assembly" }).locator(".thumbnail-button").dblclick();
  await expect(page.getByRole("button", { name: "插入", exact: true })).toBeVisible();
  await expect(page.locator(".document-tab")).toHaveCount(2);
  await page.locator(".document-tab-switch").filter({ hasText: "Mounting Bracket" }).click();
  await expect(page.getByRole("button", { name: "草图", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "关闭 Mounting Bracket", exact: true }).click();
  await expect(page.getByRole("button", { name: "插入", exact: true })).toBeVisible();
  await expect(page.locator(".document-tab")).toHaveCount(1);
  expect(errors).toEqual([]);
});

test("paged component browser, folder breadcrumbs, insertion recovery and history shortcuts", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => { if (/THREE.WebGLProgram|VALIDATE_STATUS|Shader Error/.test(message.text())) errors.push(message.text()); });
  await page.goto("/documents/mock-product-frame");
  await expect(page.getByRole("button", { name: "插入", exact: true })).toBeVisible();
  await page.evaluate(async () => {
    // Fixture setup through the mock API, isolated in this browser context.
    const { api } = await import("/src/api/client.ts");
    const parent = await api.createFolder("Standard Components", "");
    const child = await api.createFolder("Fasteners", "", parent.id);
    await Promise.all(Array.from({ length: 15 }, (_, index) => api.createDocument("PART", `Fixture ${String(index).padStart(2, "0")}`, "Browser fixture", child.id)));
    const insert = api.insert;
    let rejectOnce = true;
    api.insert = async (...args: Parameters<typeof insert>) => {
      if (rejectOnce) { rejectOnce = false; throw new Error("插入服务暂时不可用，请重试"); }
      return insert(...args);
    };
  });
  await page.getByRole("button", { name: "插入", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "插入组件", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(".insert-document-card")).toHaveCount(12);
  await dialog.getByTitle("2", { exact: true }).click();
  await expect(dialog.locator(".insert-document-card")).toHaveCount(5);
  await expect(dialog.getByRole("button", { name: "选择 Frame Assembly", exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: "文件夹", exact: true }).click();
  await dialog.getByRole("button", { name: "Standard Components", exact: true }).click();
  await dialog.getByRole("button", { name: "Fasteners", exact: true }).click();
  await expect(dialog.locator(".ant-breadcrumb")).toContainText("Standard Components");
  const search = dialog.getByRole("textbox", { name: "检索插入文档" });
  await search.fill("Fixture 14");
  await expect(dialog.locator(".insert-document-card")).toHaveCount(1);
  await dialog.getByRole("button", { name: "选择 Fixture 14", exact: true }).click();
  await expect(dialog.getByRole("complementary", { name: "组件预览" })).toContainText("Standard Components / Fasteners");
  await dialog.getByRole("button", { name: "全部文档", exact: true }).click();
  await search.fill("Mounting");
  await dialog.getByRole("button", { name: "选择 Mounting Bracket", exact: true }).click();
  await dialog.getByRole("button", { name: "插入组件", exact: true }).click();
  await expect(dialog).toContainText("插入服务暂时不可用");
  await expect(dialog.getByRole("button", { name: "选择 Mounting Bracket", exact: true })).toHaveAttribute("aria-pressed", "true");
  await dialog.getByRole("button", { name: "插入组件", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const instanceCount = () => page.evaluate(async () => (await (await import("/src/api/client.ts")).api.getDocument("mock-product-frame")).product!.instances.length);
  await expect.poll(instanceCount).toBe(3);
  await page.locator(".workbench-status").click();
  await page.keyboard.press("Control+z");
  await expect.poll(instanceCount).toBe(2);
  await page.keyboard.press("Control+y");
  await expect.poll(instanceCount).toBe(3);
  const filter = page.getByRole("textbox", { name: "筛选模型结构" });
  await filter.focus(); await page.keyboard.type("testing"); await page.keyboard.press("Control+z");
  await expect.poll(instanceCount).toBe(3);
  await filter.fill(""); await page.locator(".workbench-status").click();
  await page.keyboard.press("Control+k");
  await expect(page.getByRole("dialog", { name: "搜索工具", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "快捷键", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "快捷键", exact: true })).toContainText("Ctrl / ⌘ + Z");
  expect(errors).toEqual([]);
});
