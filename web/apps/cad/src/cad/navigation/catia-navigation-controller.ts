import * as THREE from "three";
import { InputResult, type CadPointerEvent, type PointerButtons } from "../input/input-types";
import type { CameraRig } from "./camera-rig";
import {
  CatiaNavigationState, type CatiaAuxiliaryButton, type NavigationPivotSource,
} from "./catia-navigation-state";
import type { NavigationPick, NavigationPicker } from "./navigation-picker";
import type { NavigationAction } from "./navigation-profile";
import { VirtualTrackball } from "./virtual-trackball";

export type CatiaNavigationOptions = {
  dragThreshold?: number;
  auxiliaryButtons?: readonly CatiaAuxiliaryButton[];
  debugTransitions?: boolean;
};

export type CatiaNavigationSnapshot = {
  state: CatiaNavigationState;
  action: NavigationAction;
  pivot: THREE.Vector3;
  pivotSource: NavigationPivotSource;
  hitObject?: string;
  cameraDistance: number;
  hudVisible: boolean;
  showRotationCircle: boolean;
};

type InitialCameraState = {
  position: THREE.Vector3;
  quaternion: THREE.Quaternion;
  pivot: THREE.Vector3;
};

const AUXILIARY_BUTTON_NUMBER: Record<CatiaAuxiliaryButton, number> = { left: 0, right: 2 };

/**
 * Explicit CATIA V5 Examine-mode gesture state machine.
 * InputManager owns raw button/capture state; CameraRig owns camera mathematics.
 */
export class CatiaNavigationController {
  private state = CatiaNavigationState.Idle;
  private pointerId?: number;
  private middleDown = new THREE.Vector2();
  private candidateDown = new THREE.Vector2();
  private middleDownTime = 0;
  private initialCamera?: InitialCameraState;
  private pendingPick?: NavigationPick;
  private pivotSource: NavigationPivotSource = "existing";
  private hitObject?: string;
  private auxiliaryButton?: CatiaAuxiliaryButton;
  private buttons: PointerButtons = { left: false, middle: false, right: false };
  private readonly dragThreshold: number;
  private readonly auxiliaryButtons: ReadonlySet<CatiaAuxiliaryButton>;
  private readonly debugTransitions: boolean;
  private readonly trackball = new VirtualTrackball();

  constructor(
    private readonly rig: CameraRig,
    private readonly picker: NavigationPicker,
    private readonly viewportSize: () => { width: number; height: number },
    private readonly updated: (cameraChanged: boolean) => void,
    options: CatiaNavigationOptions = {},
  ) {
    this.dragThreshold = options.dragThreshold ?? 3;
    this.auxiliaryButtons = new Set(options.auxiliaryButtons ?? ["left"]);
    this.debugTransitions = options.debugTransitions ?? false;
  }

  get currentState(): CatiaNavigationState { return this.state; }
  get active(): boolean { return this.state !== CatiaNavigationState.Idle; }
  get activeAction(): NavigationAction {
    if (this.state === CatiaNavigationState.Pan) return "pan";
    if (this.state === CatiaNavigationState.Rotate) return "orbit";
    if (this.state === CatiaNavigationState.Zoom) return "zoom";
    return "none";
  }

  get snapshot(): CatiaNavigationSnapshot {
    const pendingCenter = this.state === CatiaNavigationState.MiddlePending ? this.pendingPick : undefined;
    return {
      state: this.state,
      action: this.activeAction,
      pivot: (pendingCenter?.point ?? this.rig.pivot).clone(),
      pivotSource: pendingCenter?.source ?? this.pivotSource,
      hitObject: pendingCenter?.objectLabel ?? this.hitObject,
      cameraDistance: this.rig.distance,
      hudVisible: this.active,
      showRotationCircle: this.state === CatiaNavigationState.Rotate,
    };
  }

  wantsPriority(event: CadPointerEvent): boolean {
    return this.active || event.button === 1 || event.state.buttons.middle;
  }

  pointerDown(event: CadPointerEvent): InputResult {
    this.buttons = { ...event.state.buttons };
    if (this.state === CatiaNavigationState.Idle) {
      if (event.button !== 1) return InputResult.Ignored;
      this.beginMiddleGesture(event);
      return InputResult.Capture;
    }
    if (event.pointerId !== this.pointerId) return InputResult.Consumed;

    const auxiliary = this.auxiliaryFromButton(event.button);
    if (auxiliary && event.state.buttons.middle && this.auxiliaryButtons.has(auxiliary)
      && (this.state === CatiaNavigationState.MiddlePending || this.state === CatiaNavigationState.Pan
        || this.state === CatiaNavigationState.ZoomArmed || this.state === CatiaNavigationState.Zoom)) {
      this.auxiliaryButton = auxiliary;
      this.candidateDown.set(event.x, event.y);
      const viewport = this.viewportSize();
      this.trackball.begin(event.x, event.y, viewport.width, viewport.height);
      this.transition(CatiaNavigationState.Rotate);
    }
    return InputResult.Consumed;
  }

  pointerMove(event: CadPointerEvent): InputResult {
    this.buttons = { ...event.state.buttons };
    if (!this.active) return InputResult.Ignored;
    if (event.pointerId !== this.pointerId) return InputResult.Consumed;
    if (!event.state.buttons.middle) {
      this.forceCancel();
      return InputResult.Consumed;
    }

    switch (this.state) {
      case CatiaNavigationState.MiddlePending:
        if (this.distanceFrom(this.middleDown, event) >= this.dragThreshold) {
          this.transition(CatiaNavigationState.Pan);
          this.pan(event.deltaX, event.deltaY);
        }
        break;
      case CatiaNavigationState.Pan:
        this.pan(event.deltaX, event.deltaY);
        break;
      case CatiaNavigationState.Rotate:
        if (this.auxiliaryStillDown(event)) this.rotate(event.x, event.y);
        break;
      case CatiaNavigationState.ZoomArmed:
        if (this.distanceFrom(this.candidateDown, event) >= this.dragThreshold) {
          const deltaY = event.y - this.candidateDown.y;
          this.transition(CatiaNavigationState.Zoom);
          this.zoom(deltaY);
        }
        break;
      case CatiaNavigationState.Zoom:
        this.zoom(event.deltaY);
        break;
      default:
        break;
    }
    return InputResult.Consumed;
  }

  pointerUp(event: CadPointerEvent): InputResult {
    this.buttons = { ...event.state.buttons };
    if (!this.active || event.pointerId !== this.pointerId) return InputResult.Ignored;

    if (event.button === 1) {
      // Only a click (no Pan/Rotate/Zoom) commits the pending screen point as
      // the new centered viewpoint. A completed Pan already moved the viewing
      // rig continuously and therefore needs no release-time jump.
      if (this.state === CatiaNavigationState.MiddlePending && this.pendingPick) {
        this.rig.centerViewpointAt(this.pendingPick.point);
        this.rig.setPivot(this.pendingPick.point);
        this.pivotSource = this.pendingPick.source;
        this.hitObject = this.pendingPick.objectLabel;
        this.updated(true);
      }
      this.finishGesture();
      return InputResult.ReleaseCapture;
    }

    const releasedAuxiliary = this.auxiliaryFromButton(event.button);
    if (releasedAuxiliary && releasedAuxiliary === this.auxiliaryButton) {
      if (this.state === CatiaNavigationState.Rotate) {
        // CATIA toggles Rotate -> Zoom when the side button is released while
        // Middle remains held, regardless of whether rotation already moved.
        this.candidateDown.set(event.x, event.y);
        this.auxiliaryButton = undefined;
        this.transition(CatiaNavigationState.ZoomArmed);
      }
    }
    return InputResult.Consumed;
  }

  forceCancel(): void {
    if (!this.active) return;
    this.finishGesture();
  }

  private beginMiddleGesture(event: CadPointerEvent): void {
    this.pointerId = event.pointerId;
    this.middleDown.set(event.x, event.y);
    this.candidateDown.copy(this.middleDown);
    this.middleDownTime = performance.now();
    this.initialCamera = {
      position: this.rig.camera.position.clone(),
      quaternion: this.rig.camera.quaternion.clone(),
      pivot: this.rig.pivot.clone(),
    };
    this.pendingPick = this.picker.pickNearest(event.x, event.y)
      ?? this.picker.pickViewPlane(event.x, event.y, this.rig.pivot);
    this.auxiliaryButton = undefined;
    this.transition(CatiaNavigationState.MiddlePending);
  }

  private finishGesture(): void {
    this.pointerId = undefined;
    this.pendingPick = undefined;
    this.auxiliaryButton = undefined;
    this.initialCamera = undefined;
    this.middleDownTime = 0;
    this.buttons = { left: false, middle: false, right: false };
    this.trackball.reset();
    this.transition(CatiaNavigationState.Idle);
  }

  private pan(deltaX: number, deltaY: number): void {
    const viewport = this.viewportSize();
    this.rig.panPixels(deltaX, deltaY, viewport.width, viewport.height);
    this.updated(true);
  }

  private rotate(x: number, y: number): void {
    const viewport = this.viewportSize();
    const rotation = this.trackball.drag(x, y, viewport.width, viewport.height, this.rig.camera);
    if (!rotation) return;
    this.rig.orbitQuaternion(rotation);
    this.updated(true);
  }

  private zoom(deltaY: number): void {
    this.rig.dollyPixels(deltaY);
    this.updated(true);
  }

  private auxiliaryStillDown(event: CadPointerEvent): boolean {
    return this.auxiliaryButton === "left" ? event.state.buttons.left
      : this.auxiliaryButton === "right" ? event.state.buttons.right : false;
  }

  private auxiliaryFromButton(button: number): CatiaAuxiliaryButton | undefined {
    return (Object.entries(AUXILIARY_BUTTON_NUMBER) as Array<[CatiaAuxiliaryButton, number]>)
      .find(([, number]) => number === button)?.[0];
  }

  private distanceFrom(start: THREE.Vector2, event: CadPointerEvent): number {
    return Math.hypot(event.x - start.x, event.y - start.y);
  }

  private transition(next: CatiaNavigationState): void {
    if (next === this.state) return;
    if (this.debugTransitions) {
      console.debug(`[CATIA Navigation] ${this.state} -> ${next}`);
    }
    this.state = next;
    this.updated(false);
  }
}
