// Real Domain Commands → configured API/Worker → current viewport → PNG download.
// Start `mise run dev --profile release`, then `pnpm --dir web/apps/cad showcase`.
import { chromium, expect } from "@playwright/test";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { readDevelopmentEnvironment } from "../../../../tools/development_environment.mjs";

const root = fileURLToPath(new URL("../../../../", import.meta.url));
const evidence = path.join(root, "build/agent-work/model-showcase");
const output = path.join(root, "web/apps/cad/public/showcase");
const env = readDevelopmentEnvironment(root);
const baseURL = process.env.OCCCCAD_SHOWCASE_URL ?? "http://127.0.0.1:5173";
const selected = process.argv.find(value => value.startsWith("--model="))?.slice(8);
const models = ["impeller", "enclosure", "assembly"];
if (selected && !models.includes(selected)) throw new Error("Unknown showcase model");
await mkdir(evidence, { recursive: true });
await mkdir(output, { recursive: true });
const manifestPath = path.join(evidence, "scenes.json");
let manifest;
try { manifest = JSON.parse(await readFile(manifestPath, "utf8")); } catch (error) { if (error.code !== "ENOENT") throw error; }
manifest ??= { scenes: {} };
const saveManifest = () => writeFile(manifestPath, JSON.stringify(manifest, null, 2) + "\n");

async function buildModel({ key, folderId }) {
  const { restApi: api } = await import("/src/api.ts");
  const ledger = [], documents = [];
  let v;
  const create = async (name, type = "PART") => {
    v = await api.createDocument(type, name, "occccad 展示模型", folderId);
    documents.push(v.document.id); return v;
  };
  const command = async input => {
    v = await api.command(v.document.id, { versionId: v.document.versionId, ...input });
    ledger.push({ documentId: v.document.id, type: input.type, revisionId: v.document.versionId });
    const failed = v.part?.features.find(f => f.evaluationStatus === "FAILED" || f.evaluationStatus === "BLOCKED");
    if (failed) throw new Error(`${failed.name}: ${failed.diagnostic}`);
    return v;
  };
  const last = () => v.part.features.at(-1);
  const circle = (radius, x = 0, y = 0, id = "circle") => ({ type: "ADD_ENTITY", entity: { id, kind: "CIRCLE", role: "PROFILE", center: { x, y }, radius } });
  const ellipse = (a, b, x = 0, id = "ellipse") => ({ type: "ADD_ENTITY", entity: { id, kind: "ELLIPSE", role: "PROFILE", center: { x, y: 0 }, majorRadius: a, minorRadius: b, rotation: 0 } });
  const joins = ids => ids.map((id, i) => ({ type: "ADD_CONSTRAINT", constraint: { id: `join-${i}`, kind: "COINCIDENT", references: [{ target: "ENTITY", entityId: id, subElement: "END" }, { target: "ENTITY", entityId: ids[(i + 1) % ids.length], subElement: "START" }] } }));
  const polygon = points => [...points.map((p, i) => ({ type: "ADD_ENTITY", entity: { id: `edge-${i}`, kind: "LINE", role: "PROFILE", startPointId: `point-${i}`, endPointId: `point-${(i + 1) % points.length}`, start: { x: p[0], y: p[1] }, end: { x: points[(i + 1) % points.length][0], y: points[(i + 1) % points.length][1] } } })), ...joins(points.map((_, i) => `edge-${i}`))];
  const sketch = async (operations, z = 0, plane = "XY") => {
    if (z) {
      await command({ type: "CREATE_DATUM_PLANE", origin: [0, 0, z], normal: [0, 0, 1], uDirection: [1, 0, 0] });
      await command({ type: "CREATE_SKETCH", datumPlaneId: v.part.datumPlanes.at(-1).id });
    } else await command({ type: "CREATE_SKETCH", plane });
    const id = last().id;
    await command({ type: "EDIT_SKETCH", sketchId: id, operations });
    return id;
  };
  const pad = async (operations, z, length, operation = "ADD") => {
    const id = await sketch(operations, z);
    await command({ type: "CREATE_SOLID_FEATURE", sketchId: id, generator: "LINEAR_EXTRUDE", operation, length });
    return last();
  };
  const revolve = async points => {
    const id = await sketch([...polygon(points), { type: "ADD_ENTITY", entity: { id: "axis", kind: "LINE", role: "CONSTRUCTION", start: { x: 0, y: -50 }, end: { x: 0, y: 50 } } }], 0, "XZ");
    await command({ type: "CREATE_SOLID_FEATURE", sketchId: id, generator: "REVOLVE", operation: "NEW_BODY", angle: 360, axisEntityId: `SKETCH_LINE:${id}:axis` });
    return last();
  };
  const color = async (hex, bodyId = v.part.activeBodyId) => {
    await command({ type: "SET_PROPERTIES", properties: { targets: [{ kind: "BODY", id: bodyId }], appearance: { color: hex, roughness: .43, metalness: .22, edges: false } } });
    for (const feature of v.part.features.filter(f => f.type === "SKETCH" && f.visible !== false))
      await command({ type: "SET_DEFINITION_VISIBILITY", targetKind: "SKETCH", targetId: feature.id, visible: false });
  };
  const pattern = async (feature, count, operation = "ADD") => command({ type: "CREATE_PATTERN", feature: { type: "SOLID_PATTERN", bodyId: feature.bodyId, operation,
    pattern: { id: "", kind: "CIRCULAR", distribution: "FULL_CIRCLE", count, angle: 360, origin: [0, 0, 0], direction: [0, 0, 1], sourceKind: "GENERATOR_TOOL", source: { bodyId: feature.bodyId, featureId: feature.id } } } });
  const picks = async (kind, predicate) => {
    const body = v.part.bodies.find(b => b.id === v.part.activeBodyId), artifact = v.artifacts[body.geometryKey], result = [];
    for (let localId = 1; localId <= artifact.topology[kind === "FACE" ? "faces" : "edges"]; localId++) {
      const p = await api.getTopologyProperties(v.document.id, artifact.geometryKey, kind, localId, v.document.versionId);
      if (p.persistentSelection && predicate(p)) result.push({ selection: p.persistentSelection, sourceVersionId: v.document.versionId });
    }
    return result;
  };

  if (key === "impeller") {
    await create("径流叶轮");
    await revolve([[7, 0], [54, 0], [54, 4], [21, 4], [14, 23], [7, 23]]);
    const sections = [];
    for (const [z, scale, angle] of [[3, 1, 0], [14, .95, .10], [26, .83, .24]]) {
      const transform = ([x, y]) => ({ x: scale * (x * Math.cos(angle) - y * Math.sin(angle)), y: scale * (x * Math.sin(angle) + y * Math.cos(angle)) });
      const a = [[15.5, 5], [27, 1], [39, -9], [48, -23]].map(transform);
      const b = [[49.5, -21.5], [40.7, -7.2], [28.4, 3], [16.7, 7]].map(transform);
      sections.push({ sketchId: await sketch([
        { type: "ADD_ENTITY", entity: { id: "pressure", kind: "SPLINE", role: "PROFILE", mode: "FIT", degree: 3, controlPoints: a } },
        { type: "ADD_ENTITY", entity: { id: "tip", kind: "LINE", role: "PROFILE", start: a.at(-1), end: b[0] } },
        { type: "ADD_ENTITY", entity: { id: "suction", kind: "SPLINE", role: "PROFILE", mode: "FIT", degree: 3, controlPoints: b } },
        { type: "ADD_ENTITY", entity: { id: "root", kind: "LINE", role: "PROFILE", start: b.at(-1), end: a[0] } },
        ...joins(["pressure", "tip", "suction", "root"]),
      ], z) });
    }
    await command({ type: "CREATE_SOLID_FEATURE", feature: { type: "LOFT", bodyId: v.part.activeBodyId, operation: "ADD", sections, ruled: false } });
    await pattern(last(), 11);
    await pad(polygon([[6, -2], [10, -2], [10, 2], [6, 2]]), -1, 30, "REMOVE");
    await color("#8eaabe");
  } else if (key === "enclosure") {
    await create("过渡壳体");
    const sections = [];
    for (const [z, a, b, x] of [[0, 48, 32, 0], [28, 39, 29, 7], [64, 24, 24, 16]]) sections.push({ sketchId: await sketch(a === b ? [circle(a, x)] : [ellipse(a, b, x)], z) });
    await command({ type: "CREATE_SOLID_FEATURE", feature: { type: "LOFT", operation: "NEW_BODY", sections, ruled: false } });
    const caps = await picks("FACE", p => /LOFT_(START|END)_CAP/.test(p.persistentSelection.anchor.outputSlot));
    if (caps.length !== 2) throw new Error(`Expected two loft caps, found ${caps.length}`);
    await command({ type: "CREATE_MODIFY_FEATURE", feature: { type: "SHELL", bodyId: v.part.activeBodyId, length: 2, selections: caps } });
    await pad([ellipse(56, 40, 0, "outer"), ellipse(46, 30, 0, "inner")], 0, 5);
    await pad([circle(27, 16, 0, "outer"), circle(22, 16, 0, "inner")], 60, 6);
    await pad([[-42, -20], [-42, 20], [42, -20], [42, 20]].map(([x, y], i) => circle(2.5, x, y, `mount-${i}`)), -1, 8, "REMOVE");
    await color("#bac6d1");
  } else {
    const flange = async (name, hex) => {
      await create(name);
      await revolve([[6, 0], [29, 0], [29, 7], [13, 7], [13, 29], [6, 29]]);
      const hole = await pad([circle(2.7, 21)], -1, 10, "REMOVE");
      await pattern(hole, 6, "REMOVE");
      const edges = await picks("EDGE", p => p.geometryType === "CIRCLE" && Number(p.properties.radius) > 10);
      if (edges.length) await command({ type: "CREATE_MODIFY_FEATURE", feature: { type: "CHAMFER", bodyId: v.part.activeBodyId, length: .6, selections: edges } });
      await command({ type: "CREATE_DATUM_AXIS", origin: [0, 0, 0], direction: [0, 0, 1] });
      await color(hex);
      return { id: v.document.id, axis: v.part.datumAxes.at(-1).id };
    };
    const first = await flange("联轴器 · 主动法兰", "#829bb3");
    const second = await flange("联轴器 · 从动法兰", "#c4ad86");
    await create("联轴器 · 螺栓");
    await revolve([[0, -14], [2.35, -14], [2.35, 9], [0, 9]]);
    const hexagon = Array.from({ length: 6 }, (_, i) => [4.4 * Math.cos(i * Math.PI / 3), 4.4 * Math.sin(i * Math.PI / 3)]);
    await pad(polygon(hexagon), 7, 4);
    await color("#c2cbd2");
    const bolt = v.document.id;
    await create("联轴器 · 螺母");
    await pad([...polygon(hexagon), circle(2.35)], -14, 4);
    await color("#c2cbd2");
    const nut = v.document.id;
    await create("法兰联轴器", "PRODUCT");
    const insert = async (id, translation, rotation = [0, 0, 0, 1]) => {
      await command({ type: "INSERT_INSTANCE", referencedDocumentId: id });
      const instanceId = v.product.instances.at(-1).id;
      await command({ type: "MOVE_INSTANCE", instanceId, translation, rotation });
      return instanceId;
    };
    const a = await insert(first.id, [0, 0, 0]), b = await insert(second.id, [0, 0, -3], [1, 0, 0, 0]);
    await command({ type: "ADD_ASSEMBLY_CONSTRAINT", constraintKind: "FIX", firstAssemblyRef: { instanceId: a, kind: "BODY" } });
    await command({ type: "ADD_ASSEMBLY_CONSTRAINT", constraintKind: "CONCENTRIC", firstAssemblyRef: { instanceId: a, kind: "AXIS", geometryId: first.axis }, secondAssemblyRef: { instanceId: b, kind: "AXIS", geometryId: second.axis } });
    await command({ type: "ADD_ASSEMBLY_CONSTRAINT", constraintKind: "DISTANCE", firstAssemblyRef: { instanceId: a, kind: "PLANE", geometryId: "datum-xy" }, secondAssemblyRef: { instanceId: b, kind: "PLANE", geometryId: "datum-xy" }, distanceRelation: "SELECTED_PLANE_NORMAL_V1", value: 3 });
    const accepted = v.product.instances.find(i => i.id === b);
    if (Math.abs(accepted.translation[2] + 3) > 1e-6 || v.product.constraints.some(c => c.evaluationStatus !== "VERIFIED")) throw new Error("Coupling alignment or flange clearance was not verified");
    for (let i = 0; i < 6; i++) {
      const position = [21 * Math.cos(i * Math.PI / 3), 21 * Math.sin(i * Math.PI / 3), 0];
      await insert(bolt, position); await insert(nut, position);
    }
    await command({ type: "SET_DEFINITION_VISIBILITY", targetKind: "ASSEMBLY_CONSTRAINT_SET", targetId: "assembly-constraints", visible: false });
  }
  const bodies = v.part?.bodies.filter(b => !b.consumed && v.artifacts[b.geometryKey]?.topology.solids > 0).map(b => ({ id: b.id, volume: v.artifacts[b.geometryKey]?.volume, topology: v.artifacts[b.geometryKey]?.topology }));
  if (v.part && (!bodies.length || bodies.some(b => !(b.volume > 0)))) throw new Error("Showcase requires evaluated solid geometry");
  return { id: v.document.id, name: v.document.name, revisionId: v.document.versionId, documents, ledger,
    features: v.part?.features.map(f => ({ id: f.id, type: f.type, evaluationStatus: f.evaluationStatus })), bodies, instances: v.product?.instances.length, constraints: v.product?.constraints?.length };
}

async function observeEngine(page) {
  await page.evaluate(async () => {
    delete window.__showcaseEngine;
    const paths = new Set(["/src/viewport/cad-viewport-engine.ts", ...performance.getEntriesByType("resource").map(e => new URL(e.name)).filter(u => u.pathname === "/src/viewport/cad-viewport-engine.ts").map(u => u.pathname + u.search)]);
    for (const path of paths) {
      const { CadViewportEngine } = await import(path), prototype = CadViewportEngine.prototype;
      if (prototype.__showcaseObserved) continue;
      prototype.__showcaseObserved = true;
      for (const name of ["hitTest", "invalidate"]) {
        const original = prototype[name];
        prototype[name] = function (...args) { window.__showcaseEngine = this; return original.apply(this, args); };
      }
    }
  });
  await expect.poll(async () => {
    const canvas = await page.locator("canvas").first().boundingBox();
    await page.mouse.move(canvas.x + canvas.width / 2, canvas.y + canvas.height / 2);
    await page.mouse.move(canvas.x + canvas.width / 2 + 2, canvas.y + canvas.height / 2 + 2);
    return page.evaluate(() => Boolean(window.__showcaseEngine));
  }, { timeout: 20_000 }).toBeTruthy();
}

const browser = await chromium.launch({ headless: true, args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"] });
let activePage;
try {
  const context = await browser.newContext({ baseURL, viewport: { width: 1600, height: 1300 }, deviceScaleFactor: 1 });
  const login = await context.request.post("/api/auth/login", { data: { email: env.OCCCCAD_ADMIN_EMAIL ?? "admin@occccad.local", password: env.OCCCCAD_ADMIN_PASSWORD } });
  if (!login.ok()) throw new Error(`Login failed: ${login.status()}`);
  const page = await context.newPage();
  activePage = page;
  await page.addInitScript(() => performance.setResourceTimingBufferSize(5000));
  const errors = []; page.on("pageerror", error => errors.push(error.message));
  await page.goto("/"); await expect(page.getByRole("button", { name: "创建文档", exact: true })).toBeVisible();
  if (!manifest.folderId || !(await context.request.get(`/api/folders/${manifest.folderId}/breadcrumbs`)).ok()) {
    manifest.folderId = await page.evaluate(async () => { const { restApi: api } = await import("/src/api.ts"); return (await api.createFolder("展示模型", "真实参数化建模示例")).id; });
    await saveManifest();
  }
  for (const key of selected ? [selected] : models) {
    let scene = manifest.scenes[key];
    if (!scene || !(await context.request.get(`/api/documents/${scene.id}`)).ok()) {
      console.log(`Building ${key} through real commands`);
      scene = await page.evaluate(buildModel, { key, folderId: manifest.folderId });
      manifest.scenes[key] = scene; await saveManifest();
    }
    await page.goto(`/documents/${scene.id}`); await expect(page.locator("canvas").first()).toBeVisible();
    await observeEngine(page);
    await expect.poll(() => page.evaluate(() => window.__showcaseEngine.solidBindings.size)).toBeGreaterThan(0);
    await page.getByRole("button", { name: "收起模型结构", exact: true }).click();
    if (await page.getByRole("button", { name: "收起属性面板", exact: true }).isVisible()) await page.getByRole("button", { name: "收起属性面板", exact: true }).click();
    await expect.poll(() => page.evaluate(() => window.__showcaseEngine.renderer.domElement.height)).toBeGreaterThan(600);
    await page.evaluate(() => {
      const e = window.__showcaseEngine;
      e.setSceneSettings({ ...e.sceneSettings, background: "SOLID", backgroundTop: "#e9edf1", backgroundBottom: "#e9edf1", grid: false, worldGrid: false, groundMode: "AUTO", shadows: true, shadowOpacity: .3, ambientOcclusion: false, projection: "ORTHOGRAPHIC" });
      e.setDisplaySettings({ points: false, axes: false, planes: false, coordinateSystems: false }, { mode: "shaded", edges: false, vertices: false });
      e.setGridVisibility({ scene: false, sketch: false }); e.preselect(null, true); e.selectMany([], true);
      e.setStandardView("ISO"); e.fit();
    });
    await expect.poll(() => page.evaluate(() => window.__showcaseEngine.viewTransition.active)).toBe(false);
    // A slightly higher angle exposes the blade channels and hollow transition.
    scene.camera = await page.evaluate(key => {
      const e = window.__showcaseEngine, target = e.navigation.target;
      const radius = e.camera.position.distanceTo(target);
      const direction = key === "assembly" ? [1.1, -1.7, 1.15] : [1.1, -1.5, 1.8];
      const length = Math.hypot(...direction);
      e.camera.position.set(...direction.map((value, index) => target.getComponent(index) + radius * value / length));
      e.camera.lookAt(target); e.navigation.syncCamera(false); e.invalidate();
      return { position: e.camera.position.toArray(), target: target.toArray(), quaternion: e.camera.quaternion.toArray() };
    }, key);
    await page.mouse.move(1595, 1295);
    const downloaded = page.waitForEvent("download");
    await page.getByRole("button", { name: "保存视口截图", exact: true }).click();
    const download = await downloaded;
    await download.saveAs(path.join(output, `${key}.png`));
    await page.screenshot({ path: path.join(evidence, `${key}-workbench.png`) });
    scene.image = `showcase/${key}.png`; scene.capturedAt = new Date().toISOString();
    scene.imageSize = await page.evaluate(() => { const c = window.__showcaseEngine.renderer.domElement; return [c.width, c.height]; });
    if (errors.length) throw new Error(errors.join("\n"));
    await saveManifest(); console.log(`Captured ${key}: ${scene.imageSize.join("×")}`);
  }
} catch (error) {
  if (activePage) await activePage.screenshot({ path: path.join(evidence, "failure.png") }).catch(() => undefined);
  await writeFile(path.join(evidence, "failure.json"), JSON.stringify({ message: error.message, url: activePage?.url(), at: new Date().toISOString() }, null, 2));
  throw error;
} finally { await browser.close(); }
