import * as THREE from "three";

export type ViewportMetrics = { cssWidth: number; cssHeight: number; devicePixelRatio: number };

export function viewportMetrics(renderer: THREE.WebGLRenderer): ViewportMetrics {
  return { cssWidth: Math.max(renderer.domElement.clientWidth, 1), cssHeight: Math.max(renderer.domElement.clientHeight, 1),
    devicePixelRatio: renderer.getPixelRatio() };
}

export function worldUnitsPerCssPixel(camera: THREE.Camera, worldPosition: THREE.Vector3, metrics: ViewportMetrics): number {
  if (camera instanceof THREE.PerspectiveCamera) {
    const view = worldPosition.clone().applyMatrix4(camera.matrixWorldInverse);
    return perspectiveWorldUnitsPerCssPixel(view.z, camera.fov, metrics.cssHeight);
  }
  if (camera instanceof THREE.OrthographicCamera) return (camera.top - camera.bottom) / (camera.zoom * metrics.cssHeight);
  return 1 / metrics.cssHeight;
}

export const perspectiveWorldUnitsPerCssPixel = (depth: number, verticalFovDegrees: number, cssHeight: number): number =>
  2 * Math.abs(depth) * Math.tan(THREE.MathUtils.degToRad(verticalFovDegrees / 2)) / Math.max(cssHeight, 1);

export const cssPixelsToDevicePixels = (cssPixels: number, metrics: ViewportMetrics): number =>
  cssPixels * metrics.devicePixelRatio;

export function axisDragWorldDelta(deltaCssPixels: THREE.Vector2, axisCssPixelsPerWorld: THREE.Vector2,
  translationGain: number): number {
  if (axisCssPixelsPerWorld.lengthSq() < 1.0e-12) return 0;
  // Manipulation is an interaction scale, not a screen-to-world projection. Camera zoom
  // changes the rendered gizmo size but must not change model displacement per CSS pixel.
  return deltaCssPixels.dot(axisCssPixelsPerWorld.clone().normalize()) * translationGain;
}

export function angularDragDelta(currentAngle: number, startAngle: number, rotationGain: number): number {
  return Math.atan2(Math.sin(currentAngle - startAngle), Math.cos(currentAngle - startAngle)) * rotationGain;
}
