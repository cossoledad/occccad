import * as THREE from "three";
import { restoreView, saveView, type SavedView } from "./orthographic-view";

/** Camera-only animation, driven by the viewport's existing render clock. */
export class ViewTransition {
  private pending?: { from: SavedView; to: SavedView; started: number; duration: number };
  constructor(private camera: THREE.OrthographicCamera, private target: THREE.Vector3) {}
  get active(): boolean { return Boolean(this.pending); }
  cancel(): void { this.pending = undefined; }

  start(to: SavedView, now: number, duration = 280): void {
    // Retarget from the currently displayed frame, including an interrupted transition.
    const copy = { position: to.position.clone(), rotation: to.rotation.clone(),
      up: to.up.clone(), target: to.target.clone(), zoom: to.zoom };
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
    this.target.lerpVectors(from.target, to.target, eased);
    this.camera.quaternion.slerpQuaternions(from.rotation, to.rotation, eased);
    // Interpolate the offset in camera space: orbit around the focus instead of
    // cutting through it on a straight position interpolation.
    const offset = from.position.clone().sub(from.target).applyQuaternion(from.rotation.clone().invert());
    const endOffset = to.position.clone().sub(to.target).applyQuaternion(to.rotation.clone().invert());
    offset.lerp(endOffset, eased).applyQuaternion(this.camera.quaternion);
    this.camera.position.copy(this.target).add(offset);
    this.camera.up.set(0, 1, 0).applyQuaternion(this.camera.quaternion);
    this.camera.zoom = Math.exp(THREE.MathUtils.lerp(Math.log(from.zoom), Math.log(to.zoom), eased));
    this.camera.updateProjectionMatrix();
    this.camera.updateMatrixWorld(true);
    return true;
  }
}
