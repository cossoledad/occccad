import { expect, test } from "@playwright/test";
// Browser interaction coverage is separate from the exact Router/Worker corpus.
test("solid feature dialogs expose definitions and cancel cleanly", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", e => errors.push(e.message));
    await page.goto("/documents/mock-part-bracket");
    await expect(page.locator("canvas")).toBeVisible();
    for (const [command, title] of [["旋转", "创建 旋转"], ["布尔", "布尔运算"], ["圆角", "创建 圆角"], ["倒角", "创建 倒角"], ["拔模", "创建 拔模"], ["抽壳", "创建 抽壳"], ["放样", "创建 放样"]]) {
        const camera = await page.getByTestId("navigation-camera").textContent();
        await page.getByRole("button", { name: "搜索工具", exact: true }).click();
        await page.getByRole("textbox", { name: "搜索工具名称或用途" }).fill(command);
        await page.locator(".workbench-command-result").filter({ hasText: command }).first().click();
        const dialog = page.getByRole("dialog", { name: title, exact: true });
        await expect(dialog).toBeVisible();
        await expect(page.getByTestId("navigation-camera")).toHaveText(camera!);
        await expect(dialog.getByRole("button", { name: /确\s*定/ })).toBeDisabled();
        await dialog.getByRole("button", { name: "关闭", exact: true }).click();
        await expect(dialog).toHaveCount(0);
    }
    expect(errors).toEqual([]);
});
