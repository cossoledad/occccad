import * as THREE from "three";
import { InputResult, type CadPointerEvent } from "../input/input-types";
import type { CameraRig } from "./camera-rig";
import { CatiaNavigationState, type NavigationPivotSource } from "./catia-navigation-state";
import type { NavigationPicker } from "./navigation-picker";
import type { NavigationAction } from "./navigation-profile";
import { viewFocus } from "./orthographic-view";
import { VirtualTrackball } from "./virtual-trackball";
import { CATIA_ZOOM_RATIO_PER_HEIGHT } from "./navigation-visuals";

export type CatiaNavigationOptions = { dragThreshold?: number; debugTransitions?: boolean };
export type CatiaNavigationSnapshot = {
  state: CatiaNavigationState; action: NavigationAction; pivot: THREE.Vector3;
  pivotSource: NavigationPivotSource; hitObject?: string; cameraDistance: number;
  hudVisible: boolean; showRotationCircle: boolean; showPivot: boolean; pointer: { x: number; y: number };
  rotationMode:"trackball"|"roll";
};

/** 3DEXPERIENCE CATIA native navigation: middle-led chord; releasing the side button latches zoom until the leader is released. */
export class CatiaNavigationController {
  private rotationSphereVisible = false;
  setRotationSphereVisible(visible: boolean): void {
    this.rotationSphereVisible = visible; this.updated(false);
  }
  private state = CatiaNavigationState.Idle;
  private pointerId?: number;
  private leader: "middle" | "right" = "middle";
  private down = new THREE.Vector2();
  private pointer = new THREE.Vector2();
  private readonly actionPointer=new THREE.Vector2();
  private gestureSize={width:0,height:0};
  private pivotSource: NavigationPivotSource = "existing";
  private hitObject?: string;
  private zoomLatched = false;
  private sideDown = false;
  private directCtrl = false;
  private clickCandidate = false;
  private readonly trackball = new VirtualTrackball();
  private get rotationMode():"trackball"|"roll" {
    return this.trackball.rotationMode;
  }
  constructor(private readonly rig: CameraRig, private readonly picker: NavigationPicker,
    private readonly viewportSize: () => { width: number; height: number },
    private readonly updated: (cameraChanged: boolean) => void, private readonly options: CatiaNavigationOptions = {}) {}
  get currentState(): CatiaNavigationState { return this.state; }
  get active(): boolean { return this.pointerId !== undefined; }
  retainsCapture(pointerId:number,buttons:number):boolean {return this.pointerId===pointerId&&Boolean(buttons&(this.leader==="middle"?4:2))}
  get activeAction(): NavigationAction {
    return this.state === CatiaNavigationState.Pan ? "pan" : this.state === CatiaNavigationState.Rotate ? this.rotationMode==='roll'?'roll':"orbit"
      : this.state === CatiaNavigationState.Zoom || this.state === CatiaNavigationState.ZoomArmed ? "zoom" : "none";
  }
  get snapshot(): CatiaNavigationSnapshot {
    return { state: this.state, action: this.activeAction, pivot: this.rig.pivot.clone(), pivotSource: this.pivotSource,
      hitObject: this.hitObject, cameraDistance: this.rig.distance, hudVisible: this.rotationSphereVisible && this.state === CatiaNavigationState.Rotate,
      showRotationCircle: this.rotationSphereVisible && this.state === CatiaNavigationState.Rotate, showPivot:this.active, pointer: { x: this.pointer.x, y: this.pointer.y },rotationMode:this.rotationMode };
  }
  wantsPriority(event: CadPointerEvent): boolean {
    return this.active || event.button === 1 || event.state.buttons.middle || (event.state.modifiers.alt && event.state.buttons.right);
  }
  pointerDown(event: CadPointerEvent): InputResult {
    if (!this.active) {
      if (event.button !== 1 && !(event.button === 2 && event.state.modifiers.alt)) return InputResult.Ignored;
      this.leader = event.button === 1 ? "middle" : "right";
      this.pointerId = event.pointerId; this.down.set(event.x, event.y); this.pointer.copy(this.down);
      this.actionPointer.copy(this.down);this.gestureSize=this.viewportSize();
      // A press only starts the gesture; geometry is picked on click release.
      this.pivotSource="existing";this.hitObject=undefined;
      this.directCtrl = event.state.modifiers.ctrl;
      this.zoomLatched = this.leader === "right" && this.directCtrl; this.sideDown = false; this.clickCandidate = !event.state.modifiers.ctrl && this.leader === "middle";
      this.transition(event.state.modifiers.ctrl ? CatiaNavigationState.Zoom : CatiaNavigationState.MiddlePending);
      return InputResult.Capture;
    }
    if (event.pointerId !== this.pointerId) return InputResult.Consumed;
    this.pointer.set(event.x,event.y);this.updateMode(event);
    return InputResult.Consumed;
  }
  pointerMove(event: CadPointerEvent): InputResult {
    if (!this.active) return InputResult.Ignored;
    if (event.pointerId !== this.pointerId) return InputResult.Consumed;
    if (!event.state.buttons[this.leader]) { this.forceCancel(); return InputResult.ReleaseCapture; }
    this.pointer.set(event.x, event.y);
    this.updateMode(event);
    if (this.state === CatiaNavigationState.MiddlePending) {
      if (this.down.distanceTo(this.pointer) < (this.options.dragThreshold ?? 3)) { this.updated(false); return InputResult.Consumed; }
      this.clickCandidate = false; this.transition(CatiaNavigationState.Pan);
    }
    const size = this.viewportSize();
    if(size.width!==this.gestureSize.width||size.height!==this.gestureSize.height){
      this.gestureSize=size;this.actionPointer.copy(this.pointer);
      if(this.state===CatiaNavigationState.Rotate)this.trackball.rebase(event.x,event.y,size.width,size.height,{x:size.width/2,y:size.height/2});
      this.updated(false);return InputResult.Consumed;
    }
    const dx=event.x-this.actionPointer.x,dy=event.y-this.actionPointer.y;
    if (this.state === CatiaNavigationState.Pan) {
      this.rig.setPivot(viewFocus(this.rig.camera,this.rig.pivot));
      this.rig.panPixels(dx,dy,size.width,size.height);
      this.updateCenter();
    }
    else if (this.state === CatiaNavigationState.Rotate) {
      const rotation = this.trackball.drag(event.x, event.y, size.width, size.height, this.rig.camera);
      if (rotation) this.rig.orbitQuaternion(rotation);
    } else {this.rig.setPivot(viewFocus(this.rig.camera,this.rig.pivot));this.rig.dollyLogScale(dy*Math.log(CATIA_ZOOM_RATIO_PER_HEIGHT)/Math.max(size.height,1));}
    this.actionPointer.copy(this.pointer);
    this.updated(true);
    return InputResult.Consumed;
  }
  pointerUp(event: CadPointerEvent): InputResult {
    if (!this.active || event.pointerId !== this.pointerId) return InputResult.Ignored;
    this.pointer.set(event.x,event.y);
    if (!event.state.buttons[this.leader]) {
      const pick=this.clickCandidate?this.picker.pickNearest(event.x,event.y):undefined;
      if (pick) {
        this.rig.centerViewpointAt(pick.point);
        this.pivotSource = pick.source; this.hitObject = pick.objectLabel;
        this.updated(true);
      }
      this.forceCancel(); return InputResult.ReleaseCapture;
    }
    this.updateMode(event); return InputResult.Consumed;
  }
  forceCancel(): void {
    this.pointerId = undefined; this.clickCandidate = false;
    this.sideDown = false; this.zoomLatched = false; this.trackball.reset(); this.transition(CatiaNavigationState.Idle);
  }
  private updateMode(event: CadPointerEvent): void {
    const previous=this.state;
    const buttons = event.state.buttons, modifiers = event.state.modifiers;
    if (!modifiers.ctrl) this.directCtrl = false;
    const side = this.leader === "middle" ? buttons.left || buttons.right : buttons.left || (modifiers.ctrl && !this.directCtrl);
    // Ctrl held BEFORE the leader is the documented direct zoom alternative.
    const directZoom = this.leader === "middle" && modifiers.ctrl;
    if (side && !this.sideDown) {
      this.clickCandidate = false;
      this.beginRotation(event);
      this.transition(CatiaNavigationState.Rotate);
    } else if (!side && this.sideDown) {
      this.zoomLatched = true; this.transition(CatiaNavigationState.Zoom);
    }
    this.sideDown = side;
    if (directZoom) { this.clickCandidate = false; this.transition(CatiaNavigationState.Zoom); }
    else if (side && this.state !== CatiaNavigationState.Rotate) {
      this.beginRotation(event); this.transition(CatiaNavigationState.Rotate);
    } else if (!side && this.zoomLatched) this.transition(CatiaNavigationState.Zoom);
    else if (!side && this.state === CatiaNavigationState.Zoom) this.transition(CatiaNavigationState.Pan);
    if(this.state!==previous)this.actionPointer.set(event.x,event.y);
  }
  private beginRotation(event:CadPointerEvent):void {
    this.updateCenter();
    const size=this.viewportSize();
    this.trackball.begin(event.x,event.y,size.width,size.height,{x:size.width/2,y:size.height/2});
  }
  private updateCenter():void {
    const size=this.viewportSize(),pick=this.picker.pickNearest(size.width/2,size.height/2);
    // A nearby vertex supplies depth only: the pivot itself stays on the central ray.
    this.rig.setPivot(viewFocus(this.rig.camera,pick?.point??this.rig.pivot));
    this.pivotSource=pick?.source??"existing";this.hitObject=pick?.objectLabel;
  }
  private transition(next: CatiaNavigationState): void {
    if (next === this.state) return;
    if (this.options.debugTransitions) console.debug(`[CATIA Navigation] ${this.state} -> ${next}`);
    this.state = next; this.updated(false);
  }
}
