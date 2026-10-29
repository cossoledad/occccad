import assert from "node:assert/strict";
import { createServer } from "vite";
const server = await createServer({ server: { middlewareMode: true }, appType: "custom", logLevel: "silent" });
try {
  const THREE = await server.ssrLoadModule("three");
  const { makeFeaturePreview, featurePreviewColors, hideFeaturePreviewInput } = await server.ssrLoadModule("/src/cad/rendering/feature-preview.ts");
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
    assert.equal(ghost, undefined, "unchanged faces must not be shaded twice with different triangulations");
    assert.equal(group.children.filter(object => object.isMesh).length, 1);
    assert.equal(edges.material.isLineDashedMaterial, true);
    assert.equal(edges.material.depthTest, true, "original edges respect the evaluated result's depth");
    assert.ok(edges.geometry.getAttribute("lineDistance"));
    assert.equal(group.children.length, 3);
    group.traverse(object => { object.geometry?.dispose(); object.material?.dispose(); });
    original.dispose();
  }
  const root = new THREE.Group(), geometry = new THREE.BoxGeometry(10, 10, 10);
  for (const group of geometry.groups) group.materialIndex %= 2;
  const accepted = new THREE.MeshBasicMaterial(), hidden = new THREE.MeshBasicMaterial({visible:false});
  const surface = new THREE.Mesh(geometry, [accepted, hidden]);
  const edges = new THREE.LineSegments(new THREE.EdgesGeometry(geometry), new THREE.LineBasicMaterial());
  root.add(surface, edges);root.updateMatrixWorld(true);
  const original = surface.material, originalEdge = edges.material;
  let disposed = 0, originalDisposed = 0;
  accepted.addEventListener("dispose", () => originalDisposed++);
  const restore = hideFeaturePreviewInput(root);
  for(const material of [...surface.material, edges.material])material.addEventListener("dispose", () => disposed++);
  root.visible = false;root.visible = true; // semantic visibility/pick-field refresh
  assert.ok(surface.material.every(material => !material.visible));
  assert.equal(edges.material.visible, false);
  const ray = new THREE.Raycaster(new THREE.Vector3(0,0,20),new THREE.Vector3(0,0,-1));
  assert.ok(ray.intersectObject(surface).length, "hidden render materials preserve the exact upstream pick geometry");
  restore();restore();
  assert.equal(surface.material,original);assert.equal(edges.material,originalEdge);
  assert.equal(accepted.visible,true);assert.equal(hidden.visible,false);
  assert.equal(disposed,3);assert.equal(originalDisposed,0);
  geometry.dispose();edges.geometry.dispose();accepted.dispose();hidden.dispose();originalEdge.dispose();
} finally { await server.close(); }
