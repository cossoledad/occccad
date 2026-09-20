import * as THREE from "three";
import { InputResult, type CadPointerEvent } from "../input/input-types";
import type { CameraRig } from "./camera-rig";
import type { NavigationPick, NavigationPicker } from "./navigation-picker";
import { SolidWorksNavigationProfile, type NavigationAction } from "./navigation-profile";

export type SolidWorksNavigationSnapshot = { action: NavigationAction; pivot: THREE.Vector3; reference?: NavigationPick; active: boolean };

/** Entity selection for navigation is transient and independent of the CAD selection set. */
export class SolidWorksNavigationController {
  private readonly profile = new SolidWorksNavigationProfile();
  private pointerId?: number;
  private down = new THREE.Vector2();
  private moved = false;
  private candidate?: NavigationPick;
  private reference?: NavigationPick;
  private action: NavigationAction = "none";
  private previousClick?: { time: number; point: THREE.Vector2 };
  constructor(private readonly rig: CameraRig, private readonly picker: NavigationPicker,
    private readonly viewportSize: () => { width: number; height: number },
    private readonly bounds: () => THREE.Box3, private readonly changed: (cameraChanged: boolean) => void,
    private readonly fit: () => void, private readonly editingSketch: () => boolean) {}
  get active(): boolean { return this.pointerId !== undefined; }
  get activeAction(): NavigationAction { return this.action; }
  get snapshot(): SolidWorksNavigationSnapshot { return { action: this.action, pivot: this.rig.pivot.clone(), reference: this.reference, active: this.active }; }
  pointerDown(event: CadPointerEvent): InputResult {
    if (this.active) return InputResult.Consumed;
    if (event.button !== 1) { if (this.reference) { this.reference = undefined; this.changed(false); } return InputResult.Ignored; }
    this.pointerId = event.pointerId; this.down.set(event.x, event.y); this.moved = false;
    this.action = this.profile.pointerAction(event.state.buttons, event.state);
    this.candidate = this.action === "orbit" && !this.editingSketch() ? this.picker.pickNearest(event.x, event.y, true) : undefined;
    if (this.action === "orbit") this.rig.setPivot(this.reference?.axisOrigin ?? this.reference?.point ?? this.automaticPivot(event));
    if (this.action === "roll") {
      const bounds = this.bounds();
      if (!bounds.isEmpty()) this.rig.setPivot(bounds.getCenter(new THREE.Vector3()));
    }
    this.changed(false); return InputResult.Capture;
  }
  pointerMove(event: CadPointerEvent): InputResult {
    if (!this.active) return InputResult.Ignored;
    if (this.pointerId !== event.pointerId) return InputResult.Consumed;
    if (!event.state.buttons.middle) { this.cancel(); return InputResult.ReleaseCapture; }
    const next = this.profile.pointerAction(event.state.buttons, event.state);
    if (next !== this.action) { this.action = next; this.moved = true; this.previousClick = undefined; }
    if (!this.moved && this.down.distanceTo(new THREE.Vector2(event.x, event.y)) < 3) return InputResult.Consumed;
    this.moved = true;
    const viewport = this.viewportSize();
    if (this.action === "pan") this.rig.panPixels(event.deltaX, event.deltaY, viewport.width, viewport.height);
    else if (this.action === "zoom") this.rig.dollyPixels(event.deltaY);
    else if (this.action === "roll") this.rig.orbitQuaternion(new THREE.Quaternion().setFromAxisAngle(this.rig.camera.getWorldDirection(new THREE.Vector3()), event.deltaX * 0.006));
    else if (this.action === "orbit") {
      if (this.reference?.axis) {
        this.rig.orbitQuaternion(new THREE.Quaternion().setFromAxisAngle(this.reference.axis, -this.axisDrag(event.deltaX, event.deltaY, this.reference.axis) * 0.006));
      } else this.rig.orbitPixels(event.deltaX, event.deltaY);
    }
    this.changed(true); return InputResult.Consumed;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (!this.active || this.pointerId !== event.pointerId) return InputResult.Ignored;
    if (event.state.buttons.middle) return InputResult.Consumed;
    const time = event.originalEvent.timeStamp;
    if (!this.moved && this.action === "orbit") {
      if (this.previousClick && time - this.previousClick.time < 500 && this.previousClick.point.distanceTo(this.down) < 5) {
        this.reference = undefined; this.previousClick = undefined; this.fit();
      } else {
        this.reference = this.candidate;
        this.previousClick = { time, point: this.down.clone() };
      }
    } else { this.reference = undefined; this.previousClick = undefined; }
    this.pointerId = undefined; this.candidate = undefined; this.action = "none"; this.changed(false);
    return InputResult.ReleaseCapture;
  }
  auxiliaryClick(event: MouseEvent): InputResult {
    if (event.button !== 1 || event.detail !== 2 || !this.previousClick) return InputResult.Ignored;
    this.cancel(); this.fit(); return InputResult.Consumed;
  }
  private axisDrag(dx: number, dy: number, axis: THREE.Vector3): number {
    const viewAxis = axis.clone().applyQuaternion(this.rig.camera.quaternion.clone().invert());
    return Math.abs(viewAxis.z) > 0.8 ? dx * Math.sign(viewAxis.z) : dx * viewAxis.y + dy * viewAxis.x;
  }
  cancel(): void {
    this.pointerId = undefined; this.reference = undefined; this.candidate = undefined;
    this.previousClick = undefined; this.action = "none"; this.changed(false);
  }
  private automaticPivot(event: CadPointerEvent): THREE.Vector3 {
    const bounds = this.bounds(), camera = this.rig.camera;
    if (bounds.isEmpty()) return this.rig.pivot.clone();
    camera.updateMatrixWorld(true);
    const corners = Array.from({ length: 8 }, (_, i) => new THREE.Vector3(i & 1 ? bounds.max.x : bounds.min.x,
      i & 2 ? bounds.max.y : bounds.min.y, i & 4 ? bounds.max.z : bounds.min.z).project(camera));
    if (corners.every((point) => Math.abs(point.x) <= 1 && Math.abs(point.y) <= 1 && point.z >= -1 && point.z <= 1)) return bounds.getCenter(new THREE.Vector3());
    const viewport = this.viewportSize();
    const hit = this.picker.pickNearest(event.x, event.y, true) ?? this.picker.pickNearest(viewport.width / 2, viewport.height / 2, true);
    // Public documentation describes the visible-geometry fallback, not its proprietary ranking algorithm.
    if (hit) { this.reference = { ...hit, axis: undefined }; return hit.point.clone(); }
    return this.picker.pickViewPlane(viewport.width / 2, viewport.height / 2, bounds.getCenter(new THREE.Vector3()))?.point ?? this.rig.pivot.clone();
  }
}
