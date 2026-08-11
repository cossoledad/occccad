import * as THREE from "three";

export type CadCamera = THREE.PerspectiveCamera | THREE.OrthographicCamera;

export interface CameraRig {
  readonly camera: CadCamera;
  readonly pivot: THREE.Vector3;
  readonly distance: number;

  setPivot(point: THREE.Vector3): void;
  centerViewpointAt(point: THREE.Vector3): void;
  panPixels(deltaX: number, deltaY: number, viewportWidth: number, viewportHeight: number): void;
  orbitPixels(deltaX: number, deltaY: number): void;
  orbitQuaternion(rotation: THREE.Quaternion): void;
  dollyPixels(deltaY: number, center?: THREE.Vector3): void;
  lookAtPivot(): void;
}

export type CameraRigOptions = {
  orbitSensitivity?: number;
  zoomSensitivity?: number;
  minDistance?: number;
  maxDistance?: number;
};

const DEFAULT_OPTIONS: Required<CameraRigOptions> = {
  orbitSensitivity: 0.006,
  zoomSensitivity: 0.003,
  minDistance: 1.0e-3,
  maxDistance: 1.0e9,
};

/** Camera mathematics shared by all navigation profiles. It never owns DOM/input state. */
export class ThreeCameraRig implements CameraRig {
  readonly pivot = new THREE.Vector3();
  private readonly options: Required<CameraRigOptions>;

  constructor(readonly camera: CadCamera, options: CameraRigOptions = {}) {
    this.options = { ...DEFAULT_OPTIONS, ...options };
  }

  get distance(): number { return this.camera.position.distanceTo(this.pivot); }

  setPivot(point: THREE.Vector3): void { this.pivot.copy(point); }

  centerViewpointAt(point: THREE.Vector3): void {
    // Move the complete viewing rig. The view direction and camera-to-pivot
    // vector stay unchanged, so centering is immediate and has no camera swing.
    const translation = point.clone().sub(this.pivot);
    this.camera.position.add(translation);
    this.pivot.copy(point);
    this.camera.updateMatrixWorld(true);
  }

  panPixels(deltaX: number, deltaY: number, _viewportWidth: number, viewportHeight: number): void {
    if (deltaX === 0 && deltaY === 0) return;
    const height = Math.max(viewportHeight, 1);
    const worldHeight = this.camera instanceof THREE.PerspectiveCamera
      ? 2 * Math.max(this.distance, this.options.minDistance) * Math.tan(THREE.MathUtils.degToRad(this.camera.fov) / 2)
      : (this.camera.top - this.camera.bottom) / Math.max(this.camera.zoom, 1.0e-9);
    const worldPerPixel = worldHeight / height;

    this.camera.updateMatrixWorld(true);
    const right = new THREE.Vector3().setFromMatrixColumn(this.camera.matrixWorld, 0).normalize();
    const up = new THREE.Vector3().setFromMatrixColumn(this.camera.matrixWorld, 1).normalize();
    // Moving the camera opposite to the pointer makes displayed geometry follow it.
    const translation = right.multiplyScalar(-deltaX * worldPerPixel)
      .add(up.multiplyScalar(deltaY * worldPerPixel));
    this.camera.position.add(translation);
    this.pivot.add(translation);
    this.camera.updateMatrixWorld(true);
  }

  orbitPixels(deltaX: number, deltaY: number): void {
    if (deltaX === 0 && deltaY === 0) return;
    const offset = this.camera.position.clone().sub(this.pivot);
    if (offset.lengthSq() < 1.0e-18) return;

    this.camera.updateMatrixWorld(true);
    const screenUp = new THREE.Vector3().setFromMatrixColumn(this.camera.matrixWorld, 1).normalize();
    const screenRight = new THREE.Vector3().setFromMatrixColumn(this.camera.matrixWorld, 0).normalize();
    const yaw = new THREE.Quaternion().setFromAxisAngle(
      screenUp, -deltaX * this.options.orbitSensitivity,
    );
    // Pitch uses the already-yawed screen-right axis, which keeps tumble stable
    // in arbitrary orientations and avoids Euler-angle singularities.
    const pitchedRight = screenRight.applyQuaternion(yaw).normalize();
    const pitch = new THREE.Quaternion().setFromAxisAngle(
      pitchedRight, -deltaY * this.options.orbitSensitivity,
    );
    const rotation = pitch.multiply(yaw).normalize();

    this.orbitQuaternion(rotation);
  }

  orbitQuaternion(rotation: THREE.Quaternion): void {
    const offset = this.camera.position.clone().sub(this.pivot);
    if (offset.lengthSq() < 1.0e-18) return;
    offset.applyQuaternion(rotation);
    this.camera.position.copy(this.pivot).add(offset);
    this.camera.quaternion.premultiply(rotation).normalize();
    this.camera.updateMatrixWorld(true);
  }

  dollyPixels(deltaY: number, center: THREE.Vector3 = this.pivot): void {
    if (deltaY === 0) return;
    const factor = Math.exp(deltaY * this.options.zoomSensitivity);
    if (this.camera instanceof THREE.PerspectiveCamera) {
      const offset = this.camera.position.clone().sub(center);
      const distance = offset.length();
      if (distance < 1.0e-12) return;
      const nextDistance = THREE.MathUtils.clamp(
        distance * factor, this.options.minDistance, this.options.maxDistance,
      );
      this.camera.position.copy(center).addScaledVector(offset, nextDistance / distance);
      this.camera.updateMatrixWorld(true);
      return;
    }

    const oldZoom = this.camera.zoom;
    const nextZoom = THREE.MathUtils.clamp(oldZoom / factor, 1.0e-6, 1.0e6);
    this.camera.updateMatrixWorld(true);
    const right = new THREE.Vector3().setFromMatrixColumn(this.camera.matrixWorld, 0).normalize();
    const up = new THREE.Vector3().setFromMatrixColumn(this.camera.matrixWorld, 1).normalize();
    const relative = center.clone().sub(this.camera.position);
    const planar = right.multiplyScalar(relative.dot(right)).add(up.multiplyScalar(relative.dot(up)));
    // Compensate the camera in its image plane so an off-centre pivot retains
    // the same screen position while orthographic zoom changes.
    this.camera.position.addScaledVector(planar, 1 - oldZoom / nextZoom);
    this.camera.zoom = nextZoom;
    this.camera.updateProjectionMatrix();
    this.camera.updateMatrixWorld(true);
  }

  lookAtPivot(): void {
    this.camera.lookAt(this.pivot);
    this.camera.updateMatrixWorld(true);
  }
}
