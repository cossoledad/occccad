import { expect, test, type Page } from "@playwright/test";
test.use({ actionTimeout: 15_000 });

async function login(page: Page) {
  const response = await page.request.post("/api/auth/login", { data: {
    email: process.env.OCCCCAD_ADMIN_EMAIL ?? "admin@occccad.local", password: process.env.OCCCCAD_ADMIN_PASSWORD,
  } });
  expect(response.ok()).toBeTruthy();
}

test("library caches visible thumbnails and opens create forms independently of name requests", async ({ page }) => {
  await login(page);
  const errors: string[] = [], requests: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("request", request => { if (request.url().includes("/api/")) requests.push(request.url()); });
  await page.addInitScript(() => {
    const revoked: string[] = [];
    const original = URL.revokeObjectURL;
    URL.revokeObjectURL = url => { revoked.push(url); original.call(URL, url); };
    Object.assign(window, { thumbnailRevocations: revoked });
  });
  await page.goto("/");
  const firstImage = page.locator(".document-thumbnail img").first();
  await expect(firstImage).toBeVisible();
  const alt = (await firstImage.getAttribute("alt"))!, oldURL = (await firstImage.getAttribute("src"))!;
  expect(requests.some(url => new URL(url).searchParams.get("limit") === "200")).toBe(false);
  expect(requests.filter(url => new URL(url).pathname === "/api/folders")).toHaveLength(1);

  // The response remains real; only delivery is held to exercise late results.
  let releaseNames!: () => void;
  let nameGate = new Promise<void>(resolve => { releaseNames = resolve; });
  let namesStarted = 0;
  await page.route(/\/api\/documents\?/, async route => {
    if (new URL(route.request().url()).searchParams.get("limit") !== "200") { await route.continue(); return; }
    const gate = nameGate; namesStarted++;
    const response = await route.fetch(); expect(response.ok()).toBeTruthy();
    await gate; await route.fulfill({ response });
  });
  const homeCreate = page.getByRole("dialog", { name: "创建设计文档", exact: true });
  try {
    await page.getByRole("button", { name: "创建文档", exact: true }).click();
    await expect(homeCreate).toBeVisible();
    await expect.poll(() => namesStarted).toBeGreaterThanOrEqual(1);
    await homeCreate.getByRole("textbox", { name: "文档名称", exact: true }).fill("保留用户输入");
    const delivered = page.waitForResponse(response => new URL(response.url()).searchParams.get("limit") === "200");
    releaseNames(); await delivered;
    await expect.poll(() => page.evaluate(async () => {
      // @ts-ignore development browser module
      const { queryClient } = await import("/src/app/providers.tsx");
      return queryClient.getQueryState(["documents", { defaultNames: true }])?.fetchStatus;
    })).toBe("idle");
    await expect(homeCreate.getByRole("textbox", { name: "文档名称", exact: true })).toHaveValue("保留用户输入");
    await homeCreate.getByRole("button", { name: /取\s*消/ }).click();
    expect(requests.some(url => url.includes("/context-bindings"))).toBe(false);

    await page.locator(".document-card .thumbnail-button").first().dblclick();
    await expect(page.locator("canvas").first()).toBeVisible();
    await expect.poll(() => page.evaluate(url => (window as unknown as { thumbnailRevocations: string[] }).thumbnailRevocations.includes(url), oldURL)).toBe(true);
    await page.evaluate(async () => {
      // @ts-ignore development browser module
      const { queryClient } = await import("/src/app/providers.tsx");
      await queryClient.invalidateQueries({ queryKey: ["documents", { defaultNames: true }], refetchType: "none" });
    });
    nameGate = new Promise<void>(resolve => { releaseNames = resolve; });
    const previousNameRequests = namesStarted;
    await page.getByRole("button", { name: "新建文档", exact: true }).click();
    const create = page.getByRole("dialog", { name: "新建文档", exact: true });
    await expect(create).toBeVisible();
    await expect.poll(() => namesStarted).toBeGreaterThan(previousNameRequests);
    await create.getByRole("button", { name: "Close", exact: true }).click();
    releaseNames();
    await expect(create).toHaveCount(0);

    let releaseImages!: () => void;
    const imageGate = new Promise<void>(resolve => { releaseImages = resolve; });
    const active = new Set();
    page.on("requestfailed", request => { active.delete(request); });
    await page.route(/\/api\/documents\/[^/]+\/preview(?:\?|$)/, async route => {
      active.add(route.request());
      try { const response = await route.fetch(); await imageGate; await route.fulfill({ response }); }
      finally { active.delete(route.request()); }
    });
    try {
      await page.getByRole("button", { name: "文档中心", exact: true }).click();
      const restored = page.getByAltText(alt, { exact: true }).first();
      await expect(restored).toBeVisible();
      await expect.poll(() => active.size).toBe(4);
      const url = (await restored.getAttribute("src"))!;
      releaseImages();
      await page.locator(".user-menu").click();
      await page.getByRole("menuitem", { name: /退出登录/ }).click();
      await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
      await expect.poll(() => page.evaluate(value => (window as unknown as { thumbnailRevocations: string[] }).thumbnailRevocations.includes(value), url)).toBe(true);
      expect(await page.evaluate(async () => {
        // @ts-ignore development browser module
        const { queryClient } = await import("/src/app/providers.tsx");
        // Observers can recreate empty entries during logout; no private data may survive.
        return queryClient.getQueryCache().findAll({ queryKey: ["document-thumbnail"] }).filter(query => query.state.data !== undefined).length;
      })).toBe(0);
    } finally { releaseImages(); }
  } finally { releaseNames(); }
  expect(errors).toEqual([]);
});

test("library search, page size and real create/open/reopen remain usable", async ({ page }) => {
  await login(page);
  const errors: string[] = [], searches: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.pathname === "/api/documents" && url.searchParams.has("q")) searches.push(url.searchParams.get("q")!);
  });
  await page.goto("/"); await expect(page.locator(".document-card").first()).toBeVisible();
  await page.locator(".ant-pagination-options-size-changer").click();
  const changed = page.waitForRequest(request => new URL(request.url()).pathname === "/api/documents" && new URL(request.url()).searchParams.get("limit") === "50");
  await page.getByTitle("50 条/页", { exact: true }).click();
  expect(new URL((await changed).url()).searchParams.get("offset")).toBe("0");
  await expect(page.locator(".document-card")).toHaveCount(50);
  const search = page.getByRole("textbox", { name: "搜索文档", exact: true });
  await search.pressSequentially("missing-library-name", { delay: 15 });
  await expect(page.getByText("没有找到文档", { exact: true })).toBeVisible();
  expect(searches).toEqual(["missing-library-name"]);
  await search.fill(""); await expect(page.locator(".document-card")).toHaveCount(50);
  await page.getByRole("button", { name: "创建文档", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "创建设计文档", exact: true });
  const name = `Library performance ${Date.now()}`;
  await dialog.getByRole("textbox", { name: "文档名称", exact: true }).fill(name);
  const saved = page.waitForResponse(response => new URL(response.url()).pathname === "/api/documents" && response.request().method() === "POST");
  await dialog.getByRole("button", { name: /确\s*定/ }).click();
  const response = await saved; expect(response.ok()).toBeTruthy();
  const view = await response.json();
  await expect(page).toHaveURL(new RegExp(`/documents/${view.document.id}$`));
  await expect(page.locator("canvas").first()).toBeVisible();
  await expect(page.locator(".document-tab-switch", { hasText: name })).toBeVisible();
  await page.reload(); await expect(page.locator("canvas").first()).toBeVisible();
  await expect(page.locator(".document-tab-switch", { hasText: name })).toBeVisible();
  await page.getByRole("button", { name: "文档中心", exact: true }).click();
  await expect(page.locator(".document-title", { hasText: name })).toBeVisible();
  await page.screenshot({ path: "../../../build/acceptance/document-center-live.png" });
  expect(errors).toEqual([]);
});
