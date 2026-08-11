import * as THREE from "three";
import type { CadCamera } from "../camera-rig";
import type { CatiaNavigationSnapshot } from "../catia-navigation-controller";
import type { CadShaderLibrary } from "../../rendering/shader/cad-shader-library";
import { CATIA_VISUAL_THEME } from "../../rendering/cad-visual-theme";
import { CATIA_NAVIGATION_STYLE } from "./catia-navigation-style";

export type HudScreenPosition = { x: number; y: number };

type Segment = [THREE.Vector3, THREE.Vector3, number, number];

function segmentGeometry(segments: Segment[]): THREE.BufferGeometry {
  const positions: number[] = [];
  const intensities: number[] = [];
  for (const [start, end, startIntensity, endIntensity] of segments) {
    positions.push(start.x, start.y, start.z, end.x, end.y, end.z);
    intensities.push(startIntensity, endIntensity);
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.Float32BufferAttribute(positions, 3));
  geometry.setAttribute("intensity", new THREE.Float32BufferAttribute(intensities, 1));
  return geometry;
}

function ellipseSegments(radiusX: number, radiusY: number, rotationX = 0, segments = 96): Segment[] {
  const result: Segment[] = [];
  const transform = new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(1, 0, 0), rotationX);
  for (let index = 0; index < segments; index += 1) {
    const a = index / segments * Math.PI * 2;
    const b = (index + 1) / segments * Math.PI * 2;
    const start = new THREE.Vector3(Math.cos(a) * radiusX, Math.sin(a) * radiusY, 0).applyQuaternion(transform);
    const end = new THREE.Vector3(Math.cos(b) * radiusX, Math.sin(b) * radiusY, 0).applyQuaternion(transform);
    const startLight = start.z >= 0 ? 0.82 : 0.12;
    const endLight = end.z >= 0 ? 0.82 : 0.12;
    result.push([start, end, startLight, endLight]);
  }
  return result;
}

/** WebGL screen-space navigation layer rendered after the CAD scene. */
export class CatiaNavigationHUD {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 200);
  private readonly root = new THREE.Group();
  private readonly rotationSphere = new THREE.Group();
  private readonly resources: Array<THREE.BufferGeometry | THREE.Material> = [];
  private screen?: HudScreenPosition;

  constructor(shaders: CadShaderLibrary) {
    this.camera.position.z = 100;
    this.camera.lookAt(0, 0, 0);
    const style = CATIA_NAVIGATION_STYLE;
    const lineMaterial = shaders.createMaterial("cad.overlay.line", {
      uColor: new THREE.Color(CATIA_VISUAL_THEME.navigation),
      uHighlight: new THREE.Color(CATIA_VISUAL_THEME.navigationHighlight),
      uOpacity: style.opacity,
    });
    const sphereMaterial = shaders.createMaterial("cad.overlay.solid", {
      uColor: new THREE.Color(CATIA_VISUAL_THEME.navigation),
      uHighlight: new THREE.Color(CATIA_VISUAL_THEME.navigationHighlight),
    });

    const cross = segmentGeometry([
      [new THREE.Vector3(-style.crossSize, 0, 0), new THREE.Vector3(style.crossSize, 0, 0), 0.3, 0.9],
      [new THREE.Vector3(0, -style.crossSize, 0), new THREE.Vector3(0, style.crossSize, 0), 0.3, 0.9],
      [new THREE.Vector3(-style.depthAxisSize, -style.depthAxisSize * 0.72, -5),
        new THREE.Vector3(style.depthAxisSize, style.depthAxisSize * 0.72, 5), 0.15, 1],
    ]);
    const crossLines = new THREE.LineSegments(cross, lineMaterial);
    const centerGeometry = new THREE.SphereGeometry(3.2, 18, 12);
    const center = new THREE.Mesh(centerGeometry, sphereMaterial);

    const radius = style.rotationCircleRadius;
    const sphereGeometry = segmentGeometry([
      ...ellipseSegments(radius, radius),
      ...ellipseSegments(radius, radius, THREE.MathUtils.degToRad(67)),
      ...ellipseSegments(radius * 0.42, radius, 0),
    ]);
    this.rotationSphere.add(new THREE.LineSegments(sphereGeometry, lineMaterial));
    this.rotationSphere.visible = false;
    this.root.add(this.rotationSphere, crossLines, center);
    this.root.visible = false;
    this.scene.add(this.root);
    this.resources.push(cross, centerGeometry, sphereGeometry, lineMaterial, sphereMaterial);
  }

  get screenPosition(): HudScreenPosition | undefined {
    return this.screen ? { ...this.screen } : undefined;
  }

  update(snapshot: CatiaNavigationSnapshot | undefined, _camera: CadCamera, width: number, height: number): void {
    this.resize(width, height);
    this.root.visible = Boolean(snapshot?.hudVisible && width > 0 && height > 0);
    this.rotationSphere.visible = Boolean(snapshot?.showRotationCircle);
    this.screen = this.root.visible ? { x: width / 2, y: height / 2 } : undefined;
  }

  render(renderer: THREE.WebGLRenderer): void {
    if (!this.root.visible) return;
    renderer.clearDepth();
    renderer.render(this.scene, this.camera);
  }

  dispose(): void {
    for (const resource of this.resources) resource.dispose();
  }

  private resize(width: number, height: number): void {
    this.camera.left = -width / 2;
    this.camera.right = width / 2;
    this.camera.top = height / 2;
    this.camera.bottom = -height / 2;
    this.camera.updateProjectionMatrix();
  }
}
