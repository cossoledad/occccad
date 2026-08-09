import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { TransformControls } from "three/examples/jsm/controls/TransformControls.js";
import type {
  Artifact, DocumentView, Feature, PlaneName, RectangleDraft, Selection, Vec2, Vec3,
} from "./types";

type Callbacks = {
  selectionChanged: (selection: Selection) => void;
  rectangleCreated: (rectangle: RectangleDraft) => void;
  instanceMoved: (instanceId: string, translation: Vec3) => void;
};

const planeColors: Record<PlaneName, number> = { XY: 0x3b82f6, XZ: 0x22c55e, YZ: 0xef4444 };

function sketchRectangle(feature: Feature): { origin: Vec2; width: number; height: number } {
  return feature.rectangle ?? {
    origin: feature.origin ?? [0, 0], width: feature.width ?? 1, height: feature.height ?? 1,
  };
}

function localToWorld(plane: PlaneName, point: Vec2): THREE.Vector3 {
  if (plane === "XY") return new THREE.Vector3(point[0], point[1], 0);
  if (plane === "XZ") return new THREE.Vector3(point[0], 0, point[1]);
  return new THREE.Vector3(0, point[0], point[1]);
}

function worldToLocal(plane: PlaneName, point: THREE.Vector3): Vec2 {
  if (plane === "XY") return [point.x, point.y];
  if (plane === "XZ") return [point.x, point.z];
  return [point.y, point.z];
}

function rayPlane(plane: PlaneName): THREE.Plane {
  if (plane === "XY") return new THREE.Plane(new THREE.Vector3(0, 0, 1), 0);
  if (plane === "XZ") return new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
  return new THREE.Plane(new THREE.Vector3(1, 0, 0), 0);
}

function makeGeometry(artifact: Artifact): THREE.BufferGeometry {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.Float32BufferAttribute(artifact.mesh.vertices.flat(), 3));
  geometry.setIndex(artifact.mesh.triangles.flat());
  geometry.computeVertexNormals();
  geometry.computeBoundingSphere();
  return geometry;
}

export class CadView {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(42, 1, 0.1, 5000);
  private readonly renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: "high-performance" });
  private readonly orbit: OrbitControls;
  private readonly transform: TransformControls;
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();
  private readonly content = new THREE.Group();
  private readonly helpers = new THREE.Group();
  private readonly selectable = new Map<string, THREE.Object3D>();
  private readonly instanceGroups = new Map<string, THREE.Group>();
  private selected: Selection = null;
  private view?: DocumentView;
  private sketchPlane?: PlaneName;
  private drawingStart?: Vec2;
  private preview?: THREE.Line;
  private pointerDown = false;
  private suppressSelection = false;
  private lastMiddleClick = 0;
  private sketchTool: "SELECT" | "RECTANGLE" = "SELECT";

  constructor(private readonly host: HTMLElement, private readonly callbacks: Callbacks) {
    this.scene.background = new THREE.Color(0x10151d);
    this.camera.position.set(310, -360, 270);
    this.camera.up.set(0, 0, 1);
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 1.5));
    this.renderer.shadowMap.enabled = false;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    host.appendChild(this.renderer.domElement);

    this.orbit = new OrbitControls(this.camera, this.renderer.domElement);
    this.orbit.target.set(0, 0, 20);
    this.orbit.enableDamping = false;
    this.orbit.rotateSpeed = 1.0;
    this.orbit.panSpeed = 1.0;
    this.orbit.zoomSpeed = 1.35;
    this.orbit.zoomToCursor = true;
    this.orbit.screenSpacePanning = true;
    this.orbit.minDistance = 0.001;
    this.orbit.maxDistance = 1.0e9;
    this.orbit.mouseButtons.LEFT = null;
    this.orbit.mouseButtons.MIDDLE = THREE.MOUSE.PAN;
    this.orbit.mouseButtons.RIGHT = THREE.MOUSE.ROTATE;
    this.orbit.addEventListener("change", () => this.updateCameraClipping());
    this.transform = new TransformControls(this.camera, this.renderer.domElement);
    this.transform.setMode("translate");
    this.transform.setSpace("world");
    this.scene.add(this.transform.getHelper());
    this.transform.addEventListener("dragging-changed", (event) => {
      this.orbit.enabled = !Boolean(event.value);
      if (event.value) this.suppressSelection = true;
    });
    this.transform.addEventListener("mouseUp", () => this.commitTransform());

    this.scene.add(new THREE.HemisphereLight(0xd8e8ff, 0x1f2937, 2.4));
    const light = new THREE.DirectionalLight(0xffffff, 3.4);
    light.position.set(280, -320, 520);
    light.castShadow = true;
    this.scene.add(light);
    const grid = new THREE.GridHelper(800, 40, 0x45627f, 0x263445);
    grid.rotation.x = Math.PI / 2;
    this.scene.add(grid, new THREE.AxesHelper(70), this.content, this.helpers);

    new ResizeObserver(() => this.resize()).observe(host);
    this.renderer.domElement.addEventListener("pointerdown", (event) => this.onPointerDown(event));
    this.renderer.domElement.addEventListener("pointermove", (event) => this.onPointerMove(event));
    this.renderer.domElement.addEventListener("pointerup", (event) => this.onPointerUp(event));
    this.renderer.domElement.addEventListener("pointerleave", () => {
      this.host.classList.remove("navigating");
      this.cancelDrawing();
    });
    this.renderer.domElement.addEventListener("contextmenu", (event) => event.preventDefault());
    this.animate();
  }

  render(view: DocumentView): void {
    this.view = view;
    this.transform.detach();
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.instanceGroups.clear();
    this.selected = null;
    if (view.document.type === "PART") this.renderPart(view);
    else this.renderProduct(view);
    this.frameContent();
  }

  clear(): void {
    this.view = undefined;
    this.transform.detach();
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.instanceGroups.clear();
    this.selected = null;
  }

  beginSketch(plane: PlaneName): void {
    this.sketchPlane = plane;
    this.transform.detach();
    this.select({ kind: "plane", id: `datum-${plane.toLowerCase()}`, plane });
    this.orbit.enabled = true;
    this.orbit.enableRotate = false;
    if (plane === "XY") this.camera.position.set(0, 0, 420);
    else if (plane === "XZ") this.camera.position.set(0, -420, 0);
    else this.camera.position.set(420, 0, 0);
    this.orbit.target.set(0, 0, 0);
    this.camera.up.set(0, 0, 1);
    this.camera.lookAt(0, 0, 0);
    this.orbit.update();
  }

  endSketch(): void {
    this.sketchPlane = undefined;
    this.host.classList.remove("drawing");
    this.cancelDrawing();
    this.orbit.enabled = true;
    this.orbit.enableRotate = true;
    this.frameContent();
  }

  setSketchTool(tool: "SELECT" | "RECTANGLE"): void {
    this.sketchTool = tool;
    this.cancelDrawing();
    this.host.classList.toggle("drawing", tool === "RECTANGLE" && Boolean(this.sketchPlane));
  }

  fit(): void {
    this.frameContent();
  }

  setStandardView(view: "TOP" | "FRONT" | "RIGHT" | "ISO"): void {
    const box = new THREE.Box3().setFromObject(this.content);
    if (box.isEmpty()) box.setFromCenterAndSize(new THREE.Vector3(), new THREE.Vector3(180, 180, 100));
    const center = box.getCenter(new THREE.Vector3());
    const distance = this.fitDistance(box);
    const directions = {
      TOP: new THREE.Vector3(0, 0, 1).multiplyScalar(distance),
      FRONT: new THREE.Vector3(0, -1, 0).multiplyScalar(distance),
      RIGHT: new THREE.Vector3(1, 0, 0).multiplyScalar(distance),
      ISO: new THREE.Vector3(1, -1, 1).normalize().multiplyScalar(distance),
    };
    this.camera.position.copy(center).add(directions[view]);
    this.camera.up.set(0, 0, 1);
    this.orbit.target.copy(center);
    this.camera.lookAt(center);
    this.updateCameraClipping(box);
    this.orbit.update();
  }

  select(selection: Selection): void {
    this.selected = selection;
    this.transform.detach();
    for (const object of this.selectable.values()) this.applyHighlight(object, false);
    if (selection) {
      const object = this.selectable.get(`${selection.kind}:${selection.id}`);
      if (object) {
        this.applyHighlight(object, true);
        if (selection.kind === "instance" && object instanceof THREE.Group) this.transform.attach(object);
      }
    }
    this.callbacks.selectionChanged(selection);
  }

  private renderPart(view: DocumentView): void {
    for (const datum of view.datumPlanes ?? []) this.addDatumPlane(datum.id, datum.plane);
    for (const feature of view.part?.features ?? []) {
      if (feature.type.toUpperCase().includes("SKETCH")) this.addSketch(feature);
    }
    if (view.artifact) {
      const solid = this.makeSolid(view.artifact, 0x78aef8);
      solid.userData = { kind: "solid", id: "body-1" };
      this.content.add(solid);
      this.selectable.set("solid:body-1", solid);
    }
  }

  private renderProduct(view: DocumentView): void {
    const rootName = view.document.name;
    for (const instance of view.product?.instances ?? []) {
      const group = new THREE.Group();
      group.position.fromArray(instance.translation);
      group.userData = { kind: "instance", id: instance.id };
      this.content.add(group);
      this.instanceGroups.set(instance.id, group);
      this.selectable.set(`instance:${instance.id}`, group);
      const prefix = `${rootName}/${instance.id}`;
      for (const resolved of view.resolvedInstances ?? []) {
        if (!resolved.id.startsWith(prefix)) continue;
        const artifact = view.artifacts?.[resolved.geometryKey];
        if (!artifact) continue;
        const solid = this.makeSolid(artifact, 0x7dd3fc);
        solid.position.set(
          resolved.translation[0] - instance.translation[0],
          resolved.translation[1] - instance.translation[1],
          resolved.translation[2] - instance.translation[2],
        );
        solid.userData = { kind: "instance", id: instance.id };
        group.add(solid);
      }
      if (group.children.length === 0) {
        const placeholder = new THREE.Mesh(
          new THREE.BoxGeometry(20, 20, 20),
          new THREE.MeshStandardMaterial({ color: 0x64748b, wireframe: true }),
        );
        placeholder.userData = { kind: "instance", id: instance.id };
        group.add(placeholder);
      }
    }
  }

  private addDatumPlane(id: string, plane: PlaneName): void {
    const geometry = new THREE.PlaneGeometry(180, 180);
    if (plane === "XZ") geometry.rotateX(Math.PI / 2);
    if (plane === "YZ") geometry.rotateY(Math.PI / 2);
    const material = new THREE.MeshBasicMaterial({
      color: planeColors[plane], transparent: true, opacity: 0.075,
      side: THREE.DoubleSide, depthWrite: false,
    });
    const mesh = new THREE.Mesh(geometry, material);
    mesh.userData = { kind: "plane", id, plane };
    const edge = new THREE.LineSegments(
      new THREE.EdgesGeometry(geometry),
      new THREE.LineBasicMaterial({ color: planeColors[plane], transparent: true, opacity: 0.5 }),
    );
    mesh.add(edge);
    this.helpers.add(mesh);
    this.selectable.set(`plane:${id}`, mesh);
  }

  private addSketch(feature: Feature): void {
    const plane = feature.plane ?? "XY";
    const rectangle = sketchRectangle(feature);
    const points: Vec2[] = [
      rectangle.origin,
      [rectangle.origin[0] + rectangle.width, rectangle.origin[1]],
      [rectangle.origin[0] + rectangle.width, rectangle.origin[1] + rectangle.height],
      [rectangle.origin[0], rectangle.origin[1] + rectangle.height],
      rectangle.origin,
    ];
    const geometry = new THREE.BufferGeometry().setFromPoints(points.map((point) => localToWorld(plane, point)));
    const line = new THREE.Line(geometry, new THREE.LineBasicMaterial({
      color: 0xffc857, linewidth: 2, depthTest: false,
    }));
    line.renderOrder = 20;
    line.userData = { kind: "sketch", id: feature.id };
    this.helpers.add(line);
    this.selectable.set(`sketch:${feature.id}`, line);
  }

  private makeSolid(artifact: Artifact, color: number): THREE.Group {
    const geometry = makeGeometry(artifact);
    const group = new THREE.Group();
    const mesh = new THREE.Mesh(geometry, new THREE.MeshStandardMaterial({
      color, metalness: 0.08, roughness: 0.38, emissive: 0x000000,
    }));
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    const edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(geometry, 24), new THREE.LineBasicMaterial({ color: 0xe2e8f0 }),
    );
    group.add(mesh, edges);
    return group;
  }

  private onPointerDown(event: PointerEvent): void {
    if (event.button === 1 || event.button === 2) {
      this.host.classList.add("navigating");
      if (event.button === 1) {
        const now = performance.now();
        if (now - this.lastMiddleClick < 360) this.fit();
        this.lastMiddleClick = now;
      }
      return;
    }
    if (event.button !== 0) return;
    this.pointerDown = true;
    if (!this.sketchPlane || this.sketchTool !== "RECTANGLE") return;
    const point = this.intersectDrawingPlane(event);
    if (!point) return;
    this.renderer.domElement.setPointerCapture(event.pointerId);
    this.drawingStart = worldToLocal(this.sketchPlane, point);
    this.orbit.enabled = false;
  }

  private onPointerMove(event: PointerEvent): void {
    if (!this.sketchPlane || !this.drawingStart || !this.pointerDown) return;
    const point = this.intersectDrawingPlane(event);
    if (!point) return;
    const current = worldToLocal(this.sketchPlane, point);
    this.drawPreview(this.drawingStart, current, this.sketchPlane);
  }

  private onPointerUp(event: PointerEvent): void {
    this.host.classList.remove("navigating");
    if (this.renderer.domElement.hasPointerCapture(event.pointerId)) {
      this.renderer.domElement.releasePointerCapture(event.pointerId);
    }
    if (this.suppressSelection) {
      this.suppressSelection = false;
      this.pointerDown = false;
      return;
    }
    if (!this.pointerDown) return;
    this.pointerDown = false;
    if (this.sketchPlane && this.drawingStart) {
      const point = this.intersectDrawingPlane(event);
      const start = this.drawingStart;
      this.drawingStart = undefined;
      this.orbit.enabled = true;
      this.clearPreview();
      if (!point) return;
      const end = worldToLocal(this.sketchPlane, point);
      const origin: Vec2 = [Math.min(start[0], end[0]), Math.min(start[1], end[1])];
      const width = Math.abs(end[0] - start[0]);
      const height = Math.abs(end[1] - start[1]);
      if (width >= 0.5 && height >= 0.5) {
        this.callbacks.rectangleCreated({ plane: this.sketchPlane, origin, width, height });
      }
      return;
    }
    if (this.transform.dragging) return;
    this.updatePointer(event);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    this.raycaster.params.Line = { threshold: 5 };
    const roots = [...this.selectable.values()];
    const hits = this.raycaster.intersectObjects(roots, true);
    if (hits.length === 0) {
      this.select(null);
      return;
    }
    const selections = hits.map((hit) => {
      let object: THREE.Object3D | null = hit.object;
      while (object && !object.userData.kind) object = object.parent;
      return object?.userData as Exclude<Selection, null> | undefined;
    }).filter((value): value is Exclude<Selection, null> => Boolean(value));
    // A visible sketch must remain selectable even when it is coplanar with a datum or body face.
    const selection = selections.find((value) => value.kind === "sketch")
      ?? selections.find((value) => value.kind !== "plane") ?? selections[0];
    if (selection) this.select(selection);
  }

  private intersectDrawingPlane(event: PointerEvent): THREE.Vector3 | null {
    if (!this.sketchPlane) return null;
    this.updatePointer(event);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    return this.raycaster.ray.intersectPlane(rayPlane(this.sketchPlane), new THREE.Vector3());
  }

  private updatePointer(event: PointerEvent): void {
    const box = this.renderer.domElement.getBoundingClientRect();
    this.pointer.set(((event.clientX - box.left) / box.width) * 2 - 1,
      -((event.clientY - box.top) / box.height) * 2 + 1);
  }

  private drawPreview(start: Vec2, end: Vec2, plane: PlaneName): void {
    this.clearPreview();
    const points = [start, [end[0], start[1]] as Vec2, end,
      [start[0], end[1]] as Vec2, start].map((point) => localToWorld(plane, point));
    this.preview = new THREE.Line(
      new THREE.BufferGeometry().setFromPoints(points),
      new THREE.LineBasicMaterial({ color: 0xffffff }),
    );
    this.scene.add(this.preview);
  }

  private clearPreview(): void {
    if (!this.preview) return;
    this.scene.remove(this.preview);
    this.preview.geometry.dispose();
    (this.preview.material as THREE.Material).dispose();
    this.preview = undefined;
  }

  private cancelDrawing(): void {
    this.pointerDown = false;
    this.drawingStart = undefined;
    this.orbit.enabled = true;
    this.clearPreview();
  }

  private commitTransform(): void {
    const object = this.transform.object;
    if (!object?.userData.id) return;
    this.callbacks.instanceMoved(object.userData.id as string,
      [object.position.x, object.position.y, object.position.z]);
  }

  private applyHighlight(object: THREE.Object3D, selected: boolean): void {
    object.traverse((child) => {
      const material = (child as THREE.Mesh).material;
      if (material instanceof THREE.MeshStandardMaterial) {
        material.emissive.setHex(selected ? 0x5b3b00 : 0x000000);
      } else if (material instanceof THREE.MeshBasicMaterial) {
        material.opacity = selected ? 0.28 : 0.075;
      } else if (material instanceof THREE.LineBasicMaterial) {
        material.color.setHex(selected ? 0xffffff : 0xffc857);
      }
    });
  }

  private frameContent(): void {
    const box = new THREE.Box3().setFromObject(this.content);
    if (box.isEmpty()) box.setFromCenterAndSize(new THREE.Vector3(), new THREE.Vector3(180, 180, 100));
    const center = box.getCenter(new THREE.Vector3());
    const distance = this.fitDistance(box);
    this.orbit.target.copy(center);
    this.camera.position.copy(center).add(
      new THREE.Vector3(1, -1.2, 0.8).normalize().multiplyScalar(distance));
    this.camera.up.set(0, 0, 1);
    this.camera.lookAt(center);
    this.updateCameraClipping(box);
    this.orbit.update();
  }

  private fitDistance(box: THREE.Box3): number {
    const sphere = box.getBoundingSphere(new THREE.Sphere());
    const radius = Math.max(sphere.radius, 1);
    const verticalFov = THREE.MathUtils.degToRad(this.camera.fov);
    const horizontalFov = 2 * Math.atan(Math.tan(verticalFov / 2) * Math.max(this.camera.aspect, 0.01));
    return radius / Math.sin(Math.min(verticalFov, horizontalFov) / 2) * 1.15;
  }

  private updateCameraClipping(box?: THREE.Box3): void {
    const bounds = box ?? new THREE.Box3().setFromObject(this.content);
    const sphere = bounds.isEmpty()
      ? new THREE.Sphere(this.orbit.target.clone(), 100)
      : bounds.getBoundingSphere(new THREE.Sphere());
    const distance = this.camera.position.distanceTo(this.orbit.target);
    const radius = Math.max(sphere.radius, 1.0e-3);
    this.camera.near = Math.max(1.0e-4, Math.min(radius * 1.0e-3, distance * 0.1));
    this.camera.far = Math.max(this.camera.near * 1000, distance + radius * 20, 1000);
    this.camera.updateProjectionMatrix();
  }

  private resize(): void {
    const width = Math.max(this.host.clientWidth, 1);
    const height = Math.max(this.host.clientHeight, 1);
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
  }

  private disposeGroup(group: THREE.Group): void {
    for (const child of [...group.children]) {
      group.remove(child);
      child.traverse((object) => {
        const mesh = object as THREE.Mesh;
        mesh.geometry?.dispose();
        const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material];
        materials.forEach((material) => material?.dispose());
      });
    }
  }

  private animate = (): void => {
    this.renderer.render(this.scene, this.camera);
    requestAnimationFrame(this.animate);
  };
}
