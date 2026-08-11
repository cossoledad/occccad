import * as THREE from "three";
import type { CadCamera } from "./camera-rig";

export type TrackballOptions = {
  radiusRatio?: number;
  minRadius?: number;
  maxStepRadians?: number;
  precision?: number;
};

const DEFAULTS: Required<TrackballOptions> = {
  radiusRatio: 0.42,
  minRadius: 140,
  maxStepRadians: THREE.MathUtils.degToRad(12),
  precision: 0.82,
};

/** Shoemake-style virtual sphere centered in the CAD viewport. */
export class VirtualTrackball {
  private previous?: THREE.Vector3;
  private readonly options: Required<TrackballOptions>;

  constructor(options: TrackballOptions = {}) {
    this.options = { ...DEFAULTS, ...options };
  }

  begin(x: number, y: number, width: number, height: number): void {
    this.previous = this.mapToSphere(x, y, width, height);
  }

  drag(x: number, y: number, width: number, height: number, camera: CadCamera): THREE.Quaternion | undefined {
    const current = this.mapToSphere(x, y, width, height);
    if (!this.previous) {
      this.previous = current;
      return undefined;
    }

    // An object trackball uses previous -> current. A camera orbit needs the
    // inverse rotation so the displayed model follows the pointer like CATIA.
    const viewRotation = new THREE.Quaternion().setFromUnitVectors(current, this.previous);
    this.previous.copy(current);
    const angle = 2 * Math.acos(THREE.MathUtils.clamp(viewRotation.w, -1, 1));
    if (!Number.isFinite(angle) || angle < 1.0e-6) return undefined;

    if (angle > this.options.maxStepRadians) {
      const axis = new THREE.Vector3(viewRotation.x, viewRotation.y, viewRotation.z).normalize();
      viewRotation.setFromAxisAngle(axis, this.options.maxStepRadians);
    }
    viewRotation.slerp(new THREE.Quaternion(), 1 - this.options.precision).normalize();

    // Convert the quaternion from camera/view coordinates to world coordinates.
    const cameraRotation = camera.getWorldQuaternion(new THREE.Quaternion());
    return cameraRotation.clone().multiply(viewRotation).multiply(cameraRotation.invert()).normalize();
  }

  reset(): void { this.previous = undefined; }

  private mapToSphere(x: number, y: number, width: number, height: number): THREE.Vector3 {
    const radius = Math.max(Math.min(width, height) * this.options.radiusRatio, this.options.minRadius);
    const px = (x - width / 2) / radius;
    const py = (height / 2 - y) / radius;
    const distanceSquared = px * px + py * py;
    // Sphere/hyperbola blend avoids the hard clamp and angular jumps produced
    // by a basic arcball when the pointer travels outside the visible sphere.
    const z = distanceSquared <= 0.5
      ? Math.sqrt(Math.max(1 - distanceSquared, 0))
      : 0.5 / Math.sqrt(distanceSquared);
    return new THREE.Vector3(px, py, z).normalize();
  }
}

