import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });

try {
  const { mockToolbarCatalog } = await server.ssrLoadModule("/src/api/mock-toolbar-catalog.ts");
  const toolbars = mockToolbarCatalog.toolbars;
  assert.ok(toolbars.some((toolbar) => toolbar.id === "part-features"));
  assert.ok(toolbars.some((toolbar) => toolbar.id === "sketch-projection"));
  assert.ok(toolbars.some((toolbar) => toolbar.id === "product-interface"));
  assert.ok(toolbars.some((toolbar) => toolbar.id === "view-navigation"));
  assert.ok(toolbars.every((toolbar) => new Set(toolbar.items.map((item) => item.groupKey)).size <= 1),
    "one toolbar must represent one user-intent category");
  const commands = toolbars.flatMap((toolbar) => toolbar.items);
  assert.ok(commands.some((item) => item.commandId === "sketch.normal"));
  assert.equal(commands.some((item) => item.commandId === "capture.settings"), false,
    "capture settings belong to the global preference center");
  assert.equal(commands.some((item) => item.commandId === "navigation.profile.toggle"), false,
    "mouse navigation belongs to the global preference center");
  assert.equal(commands.find((item) => item.commandId === "part.publications")?.iconKey, "publication");
  assert.equal(commands.find((item) => item.commandId === "product.release")?.iconKey, "release");
  assert.deepEqual(commands.filter((item) => item.commandId.startsWith("view.")).map((item) => item.commandId),
    ["view.fit", "view.top", "view.front", "view.right", "view.iso"]);
  console.log("Toolbar category and preference-boundary tests passed.");
} finally {
  await server.close();
}
