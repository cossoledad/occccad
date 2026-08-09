import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import "./styles.css";

type MeshData = {
  vertices: [number, number, number][];
  triangles: [number, number, number][];
  faceIds: number[];
};

type TreeNode = { name: string; type: string; children?: TreeNode[] };

type DemoResult = {
  artifact: {
    geometryKey: string;
    geometryId: string;
    mesh: MeshData;
    bbox: { min: number[]; max: number[] };
    topology: { faces: number; edges: number; vertices: number; solids: number };
    volume: number;
    occtVersion: string;
    glbBytes: number;
  };
  instances: {
    id: string;
    name: string;
    geometryKey: string;
    translation: [number, number, number];
  }[];
  tree: TreeNode;
  metrics: Record<string, number>;
};

const viewport = document.querySelector<HTMLDivElement>("#viewport")!;
const scene = new THREE.Scene();
scene.background = new THREE.Color(0x111827);
scene.fog = new THREE.Fog(0x111827, 700, 1400);

const camera = new THREE.PerspectiveCamera(38, 1, 0.1, 3000);
camera.position.set(380, -420, 330);
camera.up.set(0, 0, 1);

const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
renderer.shadowMap.enabled = true;
renderer.shadowMap.type = THREE.PCFSoftShadowMap;
viewport.appendChild(renderer.domElement);

const controls = new OrbitControls(camera, renderer.domElement);
controls.target.set(120, 90, 20);
controls.enableDamping = true;

scene.add(new THREE.HemisphereLight(0xc7ddff, 0x26364f, 2.3));
const keyLight = new THREE.DirectionalLight(0xffffff, 3.2);
keyLight.position.set(250, -300, 500);
keyLight.castShadow = true;
scene.add(keyLight);

const grid = new THREE.GridHelper(700, 28, 0x334155, 0x243244);
grid.rotation.x = Math.PI / 2;
scene.add(grid);
scene.add(new THREE.AxesHelper(80));

const modelRoot = new THREE.Group();
scene.add(modelRoot);

function resize(): void {
  const width = viewport.clientWidth;
  const height = viewport.clientHeight;
  renderer.setSize(width, height, false);
  camera.aspect = width / Math.max(height, 1);
  camera.updateProjectionMatrix();
}

new ResizeObserver(resize).observe(viewport);
function animate(): void {
  controls.update();
  renderer.render(scene, camera);
  requestAnimationFrame(animate);
}
animate();

function buildGeometry(mesh: MeshData): THREE.BufferGeometry {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.Float32BufferAttribute(mesh.vertices.flat(), 3));
  geometry.setIndex(mesh.triangles.flat());
  geometry.computeVertexNormals();
  geometry.computeBoundingSphere();
  return geometry;
}

function renderScene(result: DemoResult): void {
  modelRoot.clear();
  const geometry = buildGeometry(result.artifact.mesh);
  const colors = [0x4cc9f0, 0x4895ef, 0x80ed99, 0x72efdd];
  result.instances.forEach((instance, index) => {
    const material = new THREE.MeshStandardMaterial({
      color: colors[index % colors.length], metalness: 0.08, roughness: 0.34,
    });
    const solid = new THREE.Mesh(geometry, material);
    solid.position.fromArray(instance.translation);
    solid.castShadow = true;
    solid.receiveShadow = true;
    solid.userData = instance;
    modelRoot.add(solid);

    const edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(geometry, 22),
      new THREE.LineBasicMaterial({ color: 0xdbeafe, transparent: true, opacity: 0.78 }),
    );
    edges.position.copy(solid.position);
    modelRoot.add(edges);
  });
  controls.target.set(120, 90, 20);
  camera.position.set(410, -430, 340);
  controls.update();
}

function treeElement(node: TreeNode): HTMLElement {
  const item = document.createElement("div");
  item.className = "tree-node";
  const label = document.createElement("div");
  label.className = "tree-label";
  label.innerHTML = `<span class="node-icon ${node.type.toLowerCase()}"></span><span>${node.name}</span>`;
  item.appendChild(label);
  if (node.children?.length) {
    const children = document.createElement("div");
    children.className = "tree-children";
    node.children.forEach((child) => children.appendChild(treeElement(child)));
    item.appendChild(children);
  }
  return item;
}

function renderData(result: DemoResult): void {
  renderScene(result);
  const tree = document.querySelector<HTMLDivElement>("#tree")!;
  tree.className = "tree";
  tree.replaceChildren(treeElement(result.tree));

  const artifact = result.artifact;
  document.querySelector<HTMLDivElement>("#geometry-info")!.innerHTML = `
    <dl class="geometry-list">
      <div><dt>GeometryId</dt><dd title="${artifact.geometryId}">${artifact.geometryId.slice(0, 20)}…</dd></div>
      <div><dt>OCCT</dt><dd>${artifact.occtVersion}</dd></div>
      <div><dt>Volume</dt><dd>${artifact.volume.toLocaleString()} mm³</dd></div>
      <div><dt>Topology</dt><dd>${artifact.topology.faces} F · ${artifact.topology.edges} E · ${artifact.topology.vertices} V</dd></div>
      <div><dt>Triangles</dt><dd>${artifact.mesh.triangles.length}</dd></div>
      <div><dt>GLB</dt><dd>${artifact.glbBytes.toLocaleString()} bytes</dd></div>
    </dl>`;

  const metrics = [
    [result.metrics.visibleInstances, "Visible instances"],
    [result.metrics.uniqueGeometry, "Unique geometry"],
    [result.metrics.documents, "Documents"],
    [result.metrics.commands, "Commands"],
  ];
  document.querySelector<HTMLDivElement>("#metrics")!.innerHTML = metrics
    .map(([value, label]) => `<div><strong>${value}</strong><span>${label}</span></div>`)
    .join("");
}

async function checkHealth(): Promise<void> {
  const health = document.querySelector<HTMLDivElement>("#health")!;
  try {
    const response = await fetch("/api/health");
    if (!response.ok) throw new Error(await response.text());
    const data = await response.json() as { occtVersion: string };
    health.className = "health online";
    health.innerHTML = `<span></span>服务在线 · OCCT ${data.occtVersion}`;
  } catch {
    health.className = "health offline";
    health.innerHTML = "<span></span>服务未连接";
  }
}

document.querySelector<HTMLButtonElement>("#run-demo")!.addEventListener("click", async (event) => {
  const button = event.currentTarget as HTMLButtonElement;
  const status = document.querySelector<HTMLSpanElement>("#status")!;
  button.disabled = true;
  status.textContent = "正在执行 Command、求值几何并构建装配…";
  try {
    const response = await fetch("/api/demo/seed", { method: "POST" });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error ?? "Demo 构建失败");
    renderData(body as DemoResult);
    status.textContent = "完成 · 4 instances / 1 geometry";
  } catch (error) {
    status.textContent = error instanceof Error ? error.message : "Demo 构建失败";
  } finally {
    button.disabled = false;
  }
});

void checkHealth();
