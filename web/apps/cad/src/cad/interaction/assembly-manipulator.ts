import * as THREE from "three";
import type { CadShaderLibrary } from "../rendering/shader/cad-shader-library";
import { angularDragDelta, axisDragWorldDelta, worldUnitsPerCssPixel, type ViewportMetrics } from "../rendering/viewport-metrics";

type Axis = "X" | "Y" | "Z";
type Handle = { axis: Axis; operation: "translate" | "rotate"; pick: THREE.Object3D; material: THREE.ShaderMaterial };
type Drag = { pointerId: number; handle: Handle; startX: number; startY: number; startPosition: THREE.Vector3;
  startQuaternion: THREE.Quaternion; screenAxis: THREE.Vector2; startAngle: number };

export type AssemblyManipulatorCallbacks = {
  changed(): void;
  dragStarted(): void;
  dragFinished(commit: boolean): void;
};

const AXES: Record<Axis, THREE.Vector3> = {
  X: new THREE.Vector3(1, 0, 0), Y: new THREE.Vector3(0, 1, 0), Z: new THREE.Vector3(0, 0, 1),
};
const COLORS: Record<Axis, number> = { X: 0xd84c4c, Y: 0x55a85b, Z: 0x4384d8 };
const TRANSLATION_GAIN = 0.8;
const ROTATION_GAIN = 0.4;

/** Application-owned, world-space assembly manipulator. It deliberately has no DOM listeners. */
export class AssemblyManipulator {
  readonly object = new THREE.Group();
  readonly root = new THREE.Group();
  private readonly handles: Handle[] = [];
  private drag?: Drag;
  private hovered?: Handle;
  private visible = false;

  constructor(private readonly shaders: CadShaderLibrary, private readonly callbacks: AssemblyManipulatorCallbacks) {
    this.root.name = "occccad-assembly-manipulator";
    this.root.renderOrder = 100;
    this.root.add(this.object);
    const hubMaterial = this.shaders.createMaterial("cad.manipulator", {
      uColor: new THREE.Color(0xd8e0e4), uOpacity: 0.92,
    });
    this.object.add(new THREE.Mesh(new THREE.SphereGeometry(0.075, 18, 12), hubMaterial));
    for (const axis of ["X", "Y", "Z"] as const) this.addAxis(axis);
    this.root.visible = false;
  }

  attach(position: THREE.Vector3): void { this.object.position.copy(position); this.object.quaternion.identity(); this.visible = true; this.root.visible = true; }
  detach(): void { this.drag = undefined; this.setHovered(undefined); this.visible = false; this.root.visible = false; }
  isAttached(): boolean { return this.visible; }
  isDragging(): boolean { return Boolean(this.drag); }

  updateScale(camera: THREE.Camera, metrics: ViewportMetrics): void {
    if (!this.visible) return;
    const worldPerPixel = worldUnitsPerCssPixel(camera, this.object.getWorldPosition(new THREE.Vector3()), metrics);
    this.object.scale.setScalar(worldPerPixel * 98);
  }

  pointerMove(pointerId: number, x: number, y: number, camera: THREE.PerspectiveCamera, surface: HTMLElement): boolean {
    if (!this.visible) return false;
    if (!this.drag) { this.setHovered(this.pick(x, y, camera, surface)); return Boolean(this.hovered); }
    if (this.drag.pointerId !== pointerId) return true;
    const drag = this.drag;
    if (drag.handle.operation === "translate") {
      const delta = axisDragWorldDelta(new THREE.Vector2(x - drag.startX, y - drag.startY), drag.screenAxis, TRANSLATION_GAIN);
      this.object.position.copy(drag.startPosition).addScaledVector(AXES[drag.handle.axis], delta);
    } else {
      const center = this.screenPoint(this.object.getWorldPosition(new THREE.Vector3()), camera, surface);
      const angle = angularDragDelta(Math.atan2(y - center.y, x - center.x), drag.startAngle, ROTATION_GAIN);
      this.object.quaternion.setFromAxisAngle(AXES[drag.handle.axis], -angle).multiply(drag.startQuaternion);
    }
    this.callbacks.changed();
    return true;
  }

  pointerDown(pointerId: number, x: number, y: number, camera: THREE.PerspectiveCamera, surface: HTMLElement): boolean {
    if (!this.visible || this.drag) return false;
    const handle = this.pick(x, y, camera, surface);
    if (!handle) return false;
    const origin = this.object.getWorldPosition(new THREE.Vector3());
    const originScreen = this.screenPoint(origin, camera, surface);
    const axisScreen = this.screenPoint(origin.clone().add(AXES[handle.axis]), camera, surface).sub(originScreen);
    this.drag = { pointerId, handle, startX: x, startY: y, startPosition: this.object.position.clone(),
      startQuaternion: this.object.quaternion.clone(), screenAxis: axisScreen,
      startAngle: Math.atan2(y - originScreen.y, x - originScreen.x) };
    handle.material.uniforms.uActive.value = 1;
    this.callbacks.dragStarted();
    return true;
  }

  pointerUp(pointerId: number, commit: boolean): boolean {
    if (!this.drag || (pointerId >= 0 && this.drag.pointerId !== pointerId)) return false;
    this.drag.handle.material.uniforms.uActive.value = 0;
    this.drag = undefined;
    this.callbacks.dragFinished(commit);
    return true;
  }

  dispose(): void {
    this.root.traverse((object) => {
      if (object instanceof THREE.Mesh) { object.geometry.dispose(); (object.material as THREE.Material).dispose(); }
    });
  }

  private addAxis(axis: Axis): void {
    const color = new THREE.Color(COLORS[axis]);
    const lineMaterial = this.shaders.createMaterial("cad.manipulator", { uColor: color });
    const arrow = new THREE.Mesh(new THREE.CylinderGeometry(0.026, 0.026, 0.76, 14), lineMaterial);
    arrow.position.copy(AXES[axis]).multiplyScalar(0.38); this.orientY(arrow, axis);
    const cone = new THREE.Mesh(new THREE.ConeGeometry(0.082, 0.21, 18), lineMaterial);
    cone.position.copy(AXES[axis]).multiplyScalar(0.84); this.orientY(cone, axis);
    const axisPick = new THREE.Mesh(new THREE.CylinderGeometry(0.075, 0.075, 0.94, 8), new THREE.MeshBasicMaterial({ visible: false }));
    axisPick.position.copy(AXES[axis]).multiplyScalar(0.47); this.orientY(axisPick, axis);
    this.object.add(arrow, cone, axisPick);
    this.handles.push({ axis, operation: "translate", pick: axisPick, material: lineMaterial });

    const ringMaterial = this.shaders.createMaterial("cad.manipulator", { uColor: color, uOpacity: 0.86 });
    const ring = new THREE.Mesh(new THREE.TorusGeometry(0.64, 0.019, 10, 80), ringMaterial);
    const ringPick = new THREE.Mesh(new THREE.TorusGeometry(0.64, 0.065, 8, 56), new THREE.MeshBasicMaterial({ visible: false }));
    this.orientRing(ring, axis); this.orientRing(ringPick, axis); this.object.add(ring, ringPick);
    this.handles.push({ axis, operation: "rotate", pick: ringPick, material: ringMaterial });
  }

  private pick(x: number, y: number, camera: THREE.PerspectiveCamera, surface: HTMLElement): Handle | undefined {
    const rect = surface.getBoundingClientRect();
    const pointer = new THREE.Vector2(x / Math.max(rect.width, 1) * 2 - 1, 1 - y / Math.max(rect.height, 1) * 2);
    const ray = new THREE.Raycaster(); ray.setFromCamera(pointer, camera);
    const hit = ray.intersectObjects(this.handles.map((handle) => handle.pick), false)[0]?.object;
    return this.handles.find((handle) => handle.pick === hit);
  }

  private screenPoint(point: THREE.Vector3, camera: THREE.Camera, surface: HTMLElement): THREE.Vector2 {
    const rect = surface.getBoundingClientRect(); const projected = point.project(camera);
    return new THREE.Vector2((projected.x + 1) * rect.width / 2, (1 - projected.y) * rect.height / 2);
  }
  private setHovered(handle?: Handle): void {
    if (this.hovered === handle) return;
    if (this.hovered && this.hovered !== this.drag?.handle) this.hovered.material.uniforms.uActive.value = 0;
    this.hovered = handle;
    if (handle) handle.material.uniforms.uActive.value = 0.55;
    this.callbacks.changed();
  }
  private orientY(object: THREE.Object3D, axis: Axis): void {
    object.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), AXES[axis]);
  }
  private orientRing(object: THREE.Object3D, axis: Axis): void {
    if (axis === "X") object.rotation.y = Math.PI / 2;
    else if (axis === "Y") object.rotation.x = Math.PI / 2;
  }
}
