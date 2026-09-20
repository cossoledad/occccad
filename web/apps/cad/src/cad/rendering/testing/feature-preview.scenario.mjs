import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const THREE = await server.ssrLoadModule("three");
  const { makeFeaturePreview, featurePreviewColors } = await server.ssrLoadModule("/src/cad/rendering/feature-preview.ts");
  assert.equal(new Set(Object.values(featurePreviewColors)).size, 4);
  for (const operation of Object.keys(featurePreviewColors)) {
    const original = new THREE.BoxGeometry(10, 10, 10);
    const result = new THREE.BoxGeometry(10, 10, 5);
    const group = makeFeaturePreview(result, operation, original);
    const surface = group.getObjectByName("feature-preview-result");
    const ghost = group.getObjectByName("feature-preview-original");
    const edges = group.getObjectByName("feature-preview-original-edges");
    assert.equal(surface.material.transparent, false, "evaluated result must occlude original-body reference");
    assert.equal(surface.material.depthWrite, true);
    assert.equal(ghost.material.depthWrite, false);
    assert.equal(ghost.material.depthTest, true, "reference must not shine through the result");
    assert.equal(edges.material.isLineDashedMaterial, true);
    assert.ok(edges.geometry.getAttribute("lineDistance"));
    assert.equal(group.children.length, 4);
    group.traverse(object => { object.geometry?.dispose(); object.material?.dispose(); });
  }
} finally { await server.close(); }
