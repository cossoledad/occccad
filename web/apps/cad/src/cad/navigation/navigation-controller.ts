import * as THREE from "three";
import { InputResult, type CadPointerEvent, type CadWheelEvent } from "../input/input-types";
import { ThreeCameraRig, type CadCamera } from "./camera-rig";
import {
  CatiaNavigationController, type CatiaNavigationSnapshot,
} from "./catia-navigation-controller";
import { CatiaNavigationState } from "./catia-navigation-state";
import type { NavigationPicker } from "./navigation-picker";
import {
  createNavigationProfile, type NavigationAction, type NavigationProfile, type NavigationProfileID,
} from "./navigation-profile";

export type NavigationSnapshot = {
  profile: NavigationProfileID;
  action: NavigationAction;
  catia?: CatiaNavigationSnapshot;
};

type NavigationListener = (
  action: NavigationAction,
  profile: NavigationProfileID,
  snapshot: NavigationSnapshot,
) => void;

/** Profile facade. CATIA semantics live in its dedicated state machine. */
export class NavigationController {
  private readonly rig: ThreeCameraRig;
  readonly target: THREE.Vector3;
  private profile: NavigationProfile;
  private action: NavigationAction = "none";
  private enabled = true;
  private readonly catia: CatiaNavigationController;
  private readonly listeners = new Set<NavigationListener>();

  constructor(
    camera: CadCamera,
    private readonly viewportSize: () => { width: number; height: number },
    private readonly changed: () => void,
    private readonly picker: NavigationPicker,
    profileID: NavigationProfileID = "default",
    debugTransitions = false,
  ) {
    this.rig = new ThreeCameraRig(camera);
    this.target = this.rig.pivot;
    this.profile = createNavigationProfile(profileID);
    this.rig.lookAtPivot();
    this.catia = new CatiaNavigationController(
      this.rig,
      this.picker,
      viewportSize,
      (cameraChanged) => this.onCatiaUpdated(cameraChanged),
      // Right alone remains a context-menu button; while Middle is held both
      // side buttons are valid CATIA chord leaders.
      { auxiliaryButtons: ["left", "right"], debugTransitions },
    );
  }

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    if (!enabled) this.cancel();
  }

  setProfile(profileID: NavigationProfileID): void {
    this.cancel();
    this.profile = createNavigationProfile(profileID);
    this.emit(true);
  }

  get profileID(): NavigationProfileID { return this.profile.id; }
  get activeAction(): NavigationAction { return this.action; }
  get snapshot(): NavigationSnapshot {
    return { profile: this.profile.id, action: this.action, catia: this.profile.id === "catia" ? this.catia.snapshot : undefined };
  }

  subscribe(listener: NavigationListener): () => void {
    this.listeners.add(listener);
    listener(this.action, this.profile.id, this.snapshot);
    return () => this.listeners.delete(listener);
  }

  wantsPointerPriority(event: CadPointerEvent): boolean {
    if (!this.enabled) return false;
    if (this.profile.id === "catia") return this.catia.wantsPriority(event);
    return this.action !== "none" || event.button === 1 || event.button === 2
      || event.state.buttons.middle || event.state.buttons.right;
  }

  pointerDown(event: CadPointerEvent): InputResult {
    if (!this.enabled) return InputResult.Ignored;
    if (this.profile.id === "catia") return this.catia.pointerDown(event);
    const action = this.profile.pointerAction(event.state.buttons, event.state);
    if (action === "none") return InputResult.Ignored;
    this.setAction(action);
    return InputResult.Capture;
  }

  pointerMove(event: CadPointerEvent): InputResult {
    if (!this.enabled) return InputResult.Ignored;
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
    if (this.profile.id === "catia") return this.catia.pointerUp(event);
    const previous = this.action;
    this.setAction(this.profile.pointerAction(event.state.buttons, event.state));
    if ((event.button === 1 || event.button === 2) && previous !== "none") return InputResult.ReleaseCapture;
    return previous === "none" ? InputResult.Ignored : InputResult.Consumed;
  }

  wheel(event: CadWheelEvent): InputResult {
    if (!this.enabled || this.profile.wheelAction(event.state) !== "zoom") return InputResult.Ignored;
    // Classic CATIA gestures remain unchanged; wheel is a CloudCAD enhancement.
    // Prefer the nearest display-surface point under the cursor and fall back to
    // the persistent navigation pivot on empty background.
    const wheelCenter = this.picker.pickNearest(event.x, event.y)?.point;
    this.rig.dollyPixels(event.deltaY, wheelCenter);
    this.cameraChanged();
    return InputResult.Consumed;
  }

  cancel(): void {
    this.catia.forceCancel();
    this.setAction("none");
  }

  lookAt(target: THREE.Vector3): void {
    this.rig.setPivot(target);
    this.rig.lookAtPivot();
    this.cameraChanged();
  }

  syncCamera(): void {
    this.rig.lookAtPivot();
    this.cameraChanged();
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
