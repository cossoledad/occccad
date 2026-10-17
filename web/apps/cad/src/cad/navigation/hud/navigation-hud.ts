import * as THREE from "three";
import type { CadCamera } from "../camera-rig";
import type { NavigationSnapshot } from "../navigation-controller";
import type { NavigationPick } from "../navigation-picker";
import {worldUnitsPerCssPixel} from "../../rendering/viewport-metrics";

export type HudScreenPosition = { x: number; y: number };
import { CATIA_TRACKBALL_RADIUS_RATIO, CATIA_NAVIGATION_COLOR as CATIA_COLOR, SOLIDWORKS_REFERENCE_COLOR } from "../navigation-visuals";

/** View-only overlays: optional CATIA rotation sphere vs SOLIDWORKS transient rotation reference. */
export class NavigationHUD {
  private readonly screenScene = new THREE.Scene();
  private readonly worldScene = new THREE.Scene();
  private readonly referenceGroup = new THREE.Group();
  private readonly pivotMarker = new THREE.LineSegments(new THREE.BufferGeometry().setFromPoints([
    new THREE.Vector3(-1,0,0),new THREE.Vector3(1,0,0),new THREE.Vector3(0,-1,0),new THREE.Vector3(0,1,0),new THREE.Vector3(0,0,-1),new THREE.Vector3(0,0,1),
  ]),new THREE.LineBasicMaterial({color:CATIA_COLOR,depthTest:false,depthWrite:false,toneMapped:false}));
  private readonly camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 200);
  private readonly circle: THREE.LineLoop<THREE.BufferGeometry, THREE.LineDashedMaterial>;
  private readonly marker: THREE.LineSegments<THREE.BufferGeometry, THREE.LineBasicMaterial>;
  private reference?: NavigationPick;
  private worldCamera?: CadCamera;
  private screen?: HudScreenPosition;
  constructor() {
    this.camera.position.z = 100;
    const vertices = Array.from({ length: 193 }, (_, i) => new THREE.Vector3(Math.cos(i / 192 * Math.PI * 2), Math.sin(i / 192 * Math.PI * 2), 0));
    this.circle = new THREE.LineLoop(new THREE.BufferGeometry().setFromPoints(vertices), new THREE.LineDashedMaterial({ color: CATIA_COLOR, dashSize: 4, gapSize: 3, depthTest: false }));
    this.marker = new THREE.LineSegments(new THREE.BufferGeometry(), new THREE.LineBasicMaterial({ color: CATIA_COLOR, depthTest: false }));
    this.marker.geometry.setAttribute("position", new THREE.BufferAttribute(new Float32Array(144), 3).setUsage(THREE.DynamicDrawUsage));
    this.screenScene.add(this.circle, this.marker); this.screenScene.visible = false;
    this.pivotMarker.visible=false;this.worldScene.add(this.referenceGroup,this.pivotMarker);
  }
  get screenPosition(): HudScreenPosition | undefined { return this.screen; }
  update(snapshot: NavigationSnapshot, camera: CadCamera, width: number, height: number): void {
    this.worldCamera = camera;
    this.camera.left = -width / 2; this.camera.right = width / 2;
    this.camera.top = height / 2; this.camera.bottom = -height / 2; this.camera.updateProjectionMatrix();
    const catia = snapshot.catia;
    this.pivotMarker.visible=Boolean(catia?.showPivot);
    if(catia){this.pivotMarker.position.copy(catia.pivot);this.pivotMarker.scale.setScalar(9*worldUnitsPerCssPixel(camera,catia.pivot,{cssWidth:width,cssHeight:height,devicePixelRatio:1}))}
    const pivot=catia?.pivot.clone().project(camera),cx=pivot?(pivot.x+1)*width/2:width/2,cy=pivot?(1-pivot.y)*height/2:height/2;
    this.circle.position.set(cx-width/2,height/2-cy,0);this.marker.position.copy(this.circle.position);
    this.screenScene.visible = Boolean(catia?.hudVisible && width > 0 && height > 0);
    this.circle.visible = Boolean(catia?.showRotationCircle);
    const radius = Math.min(width, height) * CATIA_TRACKBALL_RADIUS_RATIO;
    // Bake radius into geometry so dash lengths remain CSS pixels.
    const attr = this.circle.geometry.getAttribute("position");
    for (let i = 0; i < attr.count; i++) attr.setXYZ(i, radius * Math.cos(i / (attr.count - 1) * Math.PI * 2), radius * Math.sin(i / (attr.count - 1) * Math.PI * 2), 0);
    attr.needsUpdate = true; this.circle.computeLineDistances();
    const x = (catia?.pointer.x ?? cx) - cx, y = cy - (catia?.pointer.y ?? cy);
    const positions: number[] = [];
    if (catia?.showRotationCircle) {
      // Tangent arcs show the grabbed point on the virtual sphere; no decorative world-axis globe.
      const r = Math.max(radius, 1), length = Math.hypot(x, y), scale = length > r ? r / length : 1;
      const point = new THREE.Vector3(x * scale, y * scale, Math.sqrt(Math.max(0, r * r - (x * scale) ** 2 - (y * scale) ** 2))).divideScalar(r);
      for (const axis of [new THREE.Vector3(0, 1, 0), new THREE.Vector3(1, 0, 0)]) {
        for (let i = 0; i < 12; i++) for (const t of [i, i + 1]) {
          const p = point.clone().applyAxisAngle(axis, (t / 12 - 0.5) * 0.16).multiplyScalar(r);
          positions.push(p.x, p.y, 0);
        }
      }
    } else positions.push(x - 6, y, 0, x + 6, y, 0, x, y - 6, 0, x, y + 6, 0);
    const markerAttribute = this.marker.geometry.getAttribute("position") as THREE.BufferAttribute;
    markerAttribute.array.set(positions); markerAttribute.needsUpdate = true;
    this.marker.geometry.setDrawRange(0, positions.length / 3);
    this.marker.frustumCulled = false;
    this.screen = catia?.showPivot ? { x:cx,y:cy } : undefined;
    this.setReference(snapshot.solidworks?.reference);
  }
  private setReference(reference?: NavigationPick): void {
    if (this.reference === reference) return;
    this.clearReference(); this.reference = reference;
    if (!reference) return;
    const highlight = reference.highlight;
    if (highlight) {
      const geometry = new THREE.BufferGeometry();
      geometry.setAttribute("position", new THREE.Float32BufferAttribute(highlight.positions, 3));
      const object = highlight.kind === "face"
        ? new THREE.Mesh(geometry, new THREE.MeshBasicMaterial({ color: SOLIDWORKS_REFERENCE_COLOR, transparent: true, opacity: 0.32, side: THREE.DoubleSide, depthTest: false, depthWrite: false }))
        : highlight.kind === "edge" ? new THREE.LineSegments(geometry, new THREE.LineBasicMaterial({ color: SOLIDWORKS_REFERENCE_COLOR, depthTest: false }))
        : new THREE.Points(geometry, new THREE.PointsMaterial({ color: SOLIDWORKS_REFERENCE_COLOR, size: 8, sizeAttenuation: false, depthTest: false }));
      this.referenceGroup.add(object);
    }
    const marker = new THREE.Points(new THREE.BufferGeometry().setFromPoints([reference.point]),
      new THREE.PointsMaterial({ color: SOLIDWORKS_REFERENCE_COLOR, size: 5, sizeAttenuation: false, depthTest: false }));
    this.referenceGroup.add(marker);
  }
  render(renderer: THREE.WebGLRenderer): void {
    if (this.worldCamera && this.worldScene.children.length) { renderer.clearDepth(); renderer.render(this.worldScene, this.worldCamera); }
    if (this.screenScene.visible) { renderer.clearDepth(); renderer.render(this.screenScene, this.camera); }
  }
  dispose(): void {
    this.clearReference(); this.circle.geometry.dispose(); this.circle.material.dispose();
    this.marker.geometry.dispose(); this.marker.material.dispose();
    this.pivotMarker.geometry.dispose();this.pivotMarker.material.dispose();
  }
  private clearReference(): void {
    for (const child of [...this.referenceGroup.children]) {
      const object = child as THREE.Mesh; object.geometry.dispose();
      (Array.isArray(object.material) ? object.material : [object.material]).forEach((material) => material.dispose());
      this.referenceGroup.remove(child);
    }
    this.reference = undefined;
  }
}
