import { expect, test } from "@playwright/test";

test("sidebar shows completed HTTP request wall time", async ({ page }) => {
  await page.route("**/api/health", async route => {
    await new Promise(resolve => setTimeout(resolve, 40));
    await route.fulfill({ json: { status: "ok", occtVersion: "fixture" } });
  });
  await page.goto("/documents/mock-part-bracket");
  const display = page.getByLabel("最后一次后端请求耗时");
  await expect(display).toBeVisible();
  await expect(display).toHaveText("后端请求：—");
  await page.evaluate(async () => {
    // Exercise the production HTTP adapter in the isolated Mock workbench.
    const path = "/src/api.ts";
    const { restApi } = await import(/* @vite-ignore */ path);
    await restApi.health();
  });
  await expect(display).toHaveText(/后端请求：\d+ ms/);
  await expect(display).toHaveAttribute("title", "最后完成的一次请求，包含网络等待和响应处理");
});
