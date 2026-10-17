import * as THREE from "three";
import type { CadCamera } from "./camera-rig";
import { restoreView, saveView, type SavedView } from "./orthographic-view";

/** Camera-only animation, driven by the viewport's existing render clock. */
export class ViewTransition {
  private pending?: { from: SavedView; to: SavedView; started: number; duration: number };
  constructor(private camera: CadCamera, private target: THREE.Vector3) {}
  setCamera(camera:CadCamera):void {this.cancel();this.camera=camera;}
  get active(): boolean { return Boolean(this.pending); }
  cancel(): void { this.pending = undefined; }

  start(to: SavedView, now: number, duration = 280): void {
    // Retarget from the currently displayed frame, including an interrupted transition.
    const destination=this.camera.clone();restoreView(destination,new THREE.Vector3(),to);
    const normalized=saveView(destination,to.target);
    const copy = { ...normalized, position: normalized.position.clone(), rotation: normalized.rotation.clone(),
      up: normalized.up.clone(), target: normalized.target.clone(), zoom: normalized.zoom };
    this.pending = { from: saveView(this.camera, this.target), to: copy, started: now, duration };
    if (duration <= 0) this.update(now);
  }

  update(now: number): boolean {
    if (!this.pending) return false;
    const { from, to, started, duration } = this.pending;
    const t = duration <= 0 ? 1 : THREE.MathUtils.clamp((now - started) / duration, 0, 1);
    if (t === 1) {
      restoreView(this.camera, this.target, to);
      this.pending = undefined;
      return true;
    }
    const eased = t * t * (3 - 2 * t);
    this.camera.quaternion.slerpQuaternions(from.rotation, to.rotation, eased);
    // Interpolate the offset in camera space: orbit around the focus instead of
    // cutting through it on a straight position interpolation.
    const startOffset = from.position.clone().sub(from.target).applyQuaternion(from.rotation.clone().invert());
    const endOffset = to.position.clone().sub(to.target).applyQuaternion(to.rotation.clone().invert());
    const offset=startOffset.clone(),startDistance=offset.length(),endDistance=endOffset.length();
    if(startDistance>1e-12&&endDistance>1e-12){
      const direction=offset.divideScalar(startDistance).lerp(endOffset.clone().divideScalar(endDistance),eased);
      if(direction.lengthSq()>1e-12)direction.normalize().multiplyScalar(Math.exp(THREE.MathUtils.lerp(Math.log(startDistance),Math.log(endDistance),eased)));
      else offset.lerpVectors(startOffset,endOffset,eased);
    }else offset.lerp(endOffset,eased);
    this.camera.zoom = Math.exp(THREE.MathUtils.lerp(Math.log(from.zoom), Math.log(to.zoom), eased));
    // Recentring follows the visible span, so a Fit from a detail does not pan
    // away from that detail before it has zoomed out enough to reveal the part.
    const perspective=this.camera instanceof THREE.PerspectiveCamera;
    const startSpan=(perspective?startOffset.z:1)/from.zoom,endSpan=(perspective?endOffset.z:1)/to.zoom;
    const span=(perspective?offset.z:1)/this.camera.zoom;
    const travel=Math.abs(endSpan-startSpan)>Math.max(Math.abs(startSpan),Math.abs(endSpan))*1e-10
      ? THREE.MathUtils.clamp((span-startSpan)/(endSpan-startSpan),0,1):eased;
    this.target.lerpVectors(from.target,to.target,travel);
    offset.applyQuaternion(this.camera.quaternion);
    this.camera.position.copy(this.target).add(offset);
    this.camera.up.set(0, 1, 0).applyQuaternion(this.camera.quaternion);
    this.camera.updateProjectionMatrix();
    this.camera.updateMatrixWorld(true);
    return true;
  }
}
