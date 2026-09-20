import * as THREE from "three";
import { InputResult, type CadPointerEvent, type CadWheelEvent, type CadKeyboardEvent } from "../input/input-types";
import { ThreeCameraRig, type CadCamera } from "./camera-rig";
import {
  CatiaNavigationController, type CatiaNavigationSnapshot,
} from "./catia-navigation-controller";
import { SolidWorksNavigationController, type SolidWorksNavigationSnapshot } from "./solidworks-navigation-controller";
import { CatiaNavigationState } from "./catia-navigation-state";
import type { NavigationPick, NavigationPicker } from "./navigation-picker";
import {
  createNavigationProfile, type NavigationAction, type NavigationProfile, type NavigationProfileID,
} from "./navigation-profile";

export type NavigationSnapshot = {
  profile: NavigationProfileID;
  action: NavigationAction;
  cameraPosition: number[];
  cameraQuaternion: number[];
  cameraZoom: number;
  projection: "orthographic" | "perspective";
  catia?: CatiaNavigationSnapshot;
  solidworks?: SolidWorksNavigationSnapshot;
};

type NavigationListener = (
  action: NavigationAction,
  profile: NavigationProfileID,
  snapshot: NavigationSnapshot,
) => void;

export function defaultOrbitPivot(hit: NavigationPick | undefined, visibleBoundsCenter: THREE.Vector3): THREE.Vector3 {
  if (hit && (hit.object as THREE.Points | undefined)?.isPoints) return hit.point.clone();
  return visibleBoundsCenter.clone();
}

/** Profile facade. CATIA semantics live in its dedicated state machine. */
export class NavigationController {
  private readonly rig: ThreeCameraRig;
  readonly target: THREE.Vector3;
  private profile: NavigationProfile;
  private action: NavigationAction = "none";
  private enabled = true;
  private readonly solidworks: SolidWorksNavigationController;
  private lastPointer?: CadPointerEvent;
  private readonly catia: CatiaNavigationController;
  private readonly listeners = new Set<NavigationListener>();

  constructor(
    camera: CadCamera,
    private readonly viewportSize: () => { width: number; height: number },
    private readonly changed: () => void,
    private readonly picker: NavigationPicker,
    private readonly visibleBoundsCenter: () => THREE.Vector3,
    profileID: NavigationProfileID = "default",
    debugTransitions = false,
    private readonly bounds?: () => THREE.Box3,
    fit: () => void = () => {},
    editingSketch: () => boolean = () => false,
  ) {
    this.rig = new ThreeCameraRig(camera);
    this.target = this.rig.pivot;
    this.profile = createNavigationProfile(profileID);
    this.rig.lookAtPivot();
    this.solidworks = new SolidWorksNavigationController(this.rig, picker, viewportSize,
      bounds ?? (() => new THREE.Box3(this.visibleBoundsCenter(), this.visibleBoundsCenter())),
      (changed) => { this.action = this.solidworks.activeAction; if (changed) this.changed(); this.emit(); }, fit, editingSketch);
    this.catia = new CatiaNavigationController(
      this.rig,
      this.picker,
      viewportSize,
      (cameraChanged) => this.onCatiaUpdated(cameraChanged),
      // Right alone remains a context-menu button; while Middle is held both
      // side buttons are valid CATIA chord leaders.
      { debugTransitions, detailPivot: () => {
        if (this.contentFullyVisible()) return undefined;
        const { width, height } = this.viewportSize();
        return this.picker.pickNearest(width / 2, height / 2);
      } },
    );
  }

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    if (!enabled) this.cancel();
  }

  setCatiaRotationSphereVisible(visible: boolean): void { this.catia.setRotationSphereVisible(visible); }

  setProfile(profileID: NavigationProfileID): void {
    this.cancel();
    this.profile = createNavigationProfile(profileID);
    this.emit(true);
  }

  get profileID(): NavigationProfileID { return this.profile.id; }
  get activeAction(): NavigationAction { return this.action; }
  get snapshot(): NavigationSnapshot {
    return { profile: this.profile.id, action: this.action, cameraPosition: this.rig.camera.position.toArray(),
      cameraQuaternion: this.rig.camera.quaternion.toArray(), cameraZoom: this.rig.camera.zoom,
      projection: this.rig.camera instanceof THREE.OrthographicCamera ? "orthographic" : "perspective", catia: this.profile.id === "catia" ? this.catia.snapshot : undefined,
      solidworks: this.profile.id === "solidworks" ? this.solidworks.snapshot : undefined };
  }

  subscribe(listener: NavigationListener): () => void {
    this.listeners.add(listener);
    listener(this.action, this.profile.id, this.snapshot);
    return () => this.listeners.delete(listener);
  }

  wantsPointerPriority(event: CadPointerEvent): boolean {
    if (!this.enabled) return false;
    if (this.profile.id === "catia") return this.catia.wantsPriority(event);
    if (this.profile.id === "solidworks") return this.solidworks.active || event.button === 1 || event.state.buttons.middle || Boolean(this.solidworks.snapshot.reference);
    return this.action !== "none" || event.button === 1 || event.button === 2
      || event.state.buttons.middle || event.state.buttons.right;
  }

  pointerDown(event: CadPointerEvent): InputResult {
    if (!this.enabled) return InputResult.Ignored;
    this.lastPointer = event;
    if (this.profile.id === "solidworks") return this.solidworks.pointerDown(event);
    if (this.profile.id === "catia") return this.catia.pointerDown(event);
    const action = this.profile.pointerAction(event.state.buttons, event.state);
    if (action === "none") return InputResult.Ignored;
    if (action === "orbit" && this.action === "none") {
      const hit = this.picker.pickNearest(event.x, event.y);
      let pivot = defaultOrbitPivot(hit, this.visibleBoundsCenter());
      if (!this.contentFullyVisible()) {
        const { width, height } = this.viewportSize();
        // Detail navigation rotates near the inspected region, never about a distant model centre.
        pivot = hit?.point ?? this.picker.pickNearest(width / 2, height / 2)?.point
          ?? this.picker.pickViewPlane(width / 2, height / 2, this.rig.pivot)?.point ?? this.rig.pivot;
      }
      this.rig.setPivot(pivot);
    }
    this.setAction(action);
    return InputResult.Capture;
  }

  pointerMove(event: CadPointerEvent): InputResult {
    if (!this.enabled) return InputResult.Ignored;
    this.lastPointer = event;
    if (this.profile.id === "solidworks") return this.solidworks.pointerMove(event);
    if (this.profile.id === "catia") return this.catia.pointerMove(event);
    const action = this.profile.pointerAction(event.state.buttons, event.state);
    this.setAction(action);
    if (action === "none") return InputResult.Ignored;
    if (action === "orbit") this.rig.orbitPixels(event.deltaX, event.deltaY);
    else if (action === "pan") {
      const viewport = this.viewportSize();
      this.rig.panPixels(event.deltaX, event.deltaY, viewport.width, viewport.height);
    } else this.rig.dollyPixels(event.deltaY);
    this.cameraChanged();
    return InputResult.Consumed;
  }

  pointerUp(event: CadPointerEvent): InputResult {
    if (!this.enabled) return InputResult.Ignored;
    this.lastPointer = event;
    if (this.profile.id === "solidworks") return this.solidworks.pointerUp(event);
    if (this.profile.id === "catia") return this.catia.pointerUp(event);
    const previous = this.action;
    this.setAction(this.profile.pointerAction(event.state.buttons, event.state));
    if ((event.button === 1 || event.button === 2) && previous !== "none") return InputResult.ReleaseCapture;
    return previous === "none" ? InputResult.Ignored : InputResult.Consumed;
  }

  wheel(event: CadWheelEvent): InputResult {
    if (!this.enabled || this.profile.wheelAction(event.state) !== "zoom") return InputResult.Ignored;
    const wheelCenter = this.picker.pickNearest(event.x, event.y)?.point
      ?? this.picker.pickViewPlane(event.x, event.y, this.target)?.point;
    const unit = event.originalEvent.deltaMode === 1 ? 16 : event.originalEvent.deltaMode === 2 ? this.viewportSize().height : 1;
    // SOLIDWORKS default: wheel towards the user zooms IN (opposite the default profile).
    const direction = this.profile.id === "solidworks" ? -1 : 1;
    this.rig.dollyPixels(event.deltaY * unit * direction, wheelCenter);
    this.cameraChanged();
    return InputResult.Consumed;
  }

  auxiliaryClick(event: MouseEvent): InputResult {
    return this.enabled && this.profile.id === "solidworks" ? this.solidworks.auxiliaryClick(event) : InputResult.Ignored;
  }

  keyChanged(event: CadKeyboardEvent): InputResult {
    if (event.editableTarget) return InputResult.Ignored;
    if (event.key === "Escape" && (this.catia.active || this.solidworks.active || this.solidworks.snapshot.reference)) {
      this.cancel(); return InputResult.ReleaseCapture;
    }
    if (this.lastPointer && (this.catia.active || this.solidworks.active) && ["Control", "Shift", "Alt"].includes(event.key)) {
      return this.pointerMove({ ...this.lastPointer, phase: "move", deltaX: 0, deltaY: 0, state: event.state });
    }
    return InputResult.Ignored;
  }

  cancel(): void {
    this.lastPointer = undefined;
    this.solidworks.cancel();
    this.catia.forceCancel();
    this.setAction("none");
  }

  lookAt(target: THREE.Vector3): void {
    this.rig.setPivot(target);
    this.rig.lookAtPivot();
    this.cameraChanged();
  }

  syncCamera(lookAtPivot = true): void {
    if (lookAtPivot) this.rig.lookAtPivot();
    this.cameraChanged();
  }

  private contentFullyVisible(): boolean {
    const bounds = this.bounds?.();
    if (!bounds || bounds.isEmpty()) return true;
    return Array.from({ length: 8 }, (_, i) => new THREE.Vector3(
      i & 1 ? bounds.max.x : bounds.min.x, i & 2 ? bounds.max.y : bounds.min.y,
      i & 4 ? bounds.max.z : bounds.min.z).project(this.rig.camera))
      .every((point) => Math.abs(point.x) <= 1 && Math.abs(point.y) <= 1 && Math.abs(point.z) <= 1);
  }

  private onCatiaUpdated(cameraChanged: boolean): void {
    this.action = this.catia.activeAction;
    if (cameraChanged) this.changed();
    this.emit();
  }

  private cameraChanged(): void {
    this.changed();
    this.emit();
  }

  private setAction(action: NavigationAction): void {
    if (action === this.action) return;
    this.action = action;
    this.emit();
  }

  private emit(force = false): void {
    if (!force && this.listeners.size === 0) return;
    const snapshot = this.snapshot;
    for (const listener of this.listeners) listener(this.action, this.profile.id, snapshot);
  }
}

export { CatiaNavigationState };
