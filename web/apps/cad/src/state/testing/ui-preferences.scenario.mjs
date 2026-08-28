import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });

try {
  const { normalizeToolbarLayout } = await server.ssrLoadModule("/src/state/ui-preferences.ts");
  assert.deepEqual(normalizeToolbarLayout(undefined, "vertical"), { orientation: "vertical" });
  assert.deepEqual(normalizeToolbarLayout({ orientation: "horizontal", x: 18, y: 42 }, "vertical"),
    { orientation: "horizontal", x: 18, y: 42 });
  assert.deepEqual(normalizeToolbarLayout({ orientation: "broken", x: Number.NaN, y: "42" }, "vertical"),
    { orientation: "vertical" });
  console.log("UI preference normalization tests passed.");
} finally {
  await server.close();
}
