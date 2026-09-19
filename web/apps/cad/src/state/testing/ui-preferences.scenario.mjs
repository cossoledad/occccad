import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({ appType: "custom", logLevel: "silent", server: { middlewareMode: true } });

try {
  const { clampStructureTreeWidth, displayLengthToMillimeters, effectiveLengthUnit, millimetersToDisplayLength,
    normalizeDisplayLengthUnit, normalizeToolbarLayout } = await server.ssrLoadModule("/src/state/ui-preferences.ts");
  const { migrateTreeVisibilityOverrides, sketchTreeVisible, treeVisibilityOverride } = await server.ssrLoadModule("/src/cad/interaction/tree-visibility.ts");
  assert.deepEqual(normalizeToolbarLayout(undefined, "vertical"), { orientation: "vertical" });
  assert.deepEqual(normalizeToolbarLayout({ orientation: "horizontal", x: 18, y: 42 }, "vertical"),
    { orientation: "horizontal", x: 18, y: 42 });
  assert.deepEqual(normalizeToolbarLayout({ orientation: "broken", x: Number.NaN, y: "42" }, "vertical"),
    { orientation: "vertical" });
  assert.equal(clampStructureTreeWidth(120), 220);
  assert.equal(clampStructureTreeWidth(512.4), 512);
  assert.equal(clampStructureTreeWidth(900), 640);
  assert.equal(normalizeDisplayLengthUnit("in"), "in");
  assert.equal(normalizeDisplayLengthUnit("feet"), "mm");
  assert.equal(effectiveLengthUnit("cm", { "part-1": "in" }, "part-1"), "in");
  assert.equal(effectiveLengthUnit("cm", { "part-1": "in" }, "part-2"), "cm");
  assert.equal(displayLengthToMillimeters(2.5, "in"), 63.5);
  assert.equal(millimetersToDisplayLength(63.5, "in"), 2.5);
  assert.deepEqual(migrateTreeVisibilityOverrides({ hiddenTreeKeys: ["document:a/body/sketch:s1"] }),
    { "document:a/body/sketch:s1": false }, "v1 hidden keys migrate to explicit hidden overrides");
  assert.equal(treeVisibilityOverride("document:a/body/sketch:s1/geometry", { "document:a/body/sketch:s1": false }), false);
  assert.equal(sketchTreeVisible({ featureID: "s1", treeKey: "sketch:s1", defaultVisible: false,
    overrides: { "sketch:s1": true } }), true, "a remembered visible override reveals a consumed sketch");
  assert.equal(sketchTreeVisible({ featureID: "s1", treeKey: "sketch:s1", activeSketchID: "s1", defaultVisible: false,
    overrides: { "sketch:s1": false } }), true, "editing temporarily reveals its active sketch");
  console.log("UI preference normalization tests passed.");
} finally {
  await server.close();
}
