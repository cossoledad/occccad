import * as THREE from "three";
import type { CadShaderLibrary } from "../rendering/shader/cad-shader-library";
import { solveScreenConstrainedParameter, worldUnitsPerCssPixel, type ViewportMetrics } from "../rendering/viewport-metrics";

type Axis = "X" | "Y" | "Z";
type Handle = { axis?: Axis; radialAxis?: Axis; operation: "translate" | "rotate" | "pivot";
  pick: THREE.Object3D; materials: THREE.ShaderMaterial[] };
type Drag = { pointerId: number; handle: Handle; startPosition: THREE.Vector3;
  startQuaternion: THREE.Quaternion; desiredPosition: THREE.Vector3; desiredQuaternion: THREE.Quaternion;
  axisParameter:number;angleParameter:number;pointerToHandleOffset:THREE.Vector2 };

export type AssemblyManipulatorCallbacks = {
  poseChanged(): void;
  visualChanged(): void;
  pivotChanged(anchor: ManipulatorAnchor): void;
  snapPivot(x: number, y: number): ManipulatorAnchor | undefined;
  dragStarted(): void;
  dragFinished(commit: boolean): void;
};
export type ManipulatorAnchor={position:THREE.Vector3;orientation?:THREE.Quaternion};

const AXES: Record<Axis, THREE.Vector3> = {
  X: new THREE.Vector3(1, 0, 0), Y: new THREE.Vector3(0, 1, 0), Z: new THREE.Vector3(0, 0, 1),
};
const COLORS: Record<Axis, number> = { X: 0xe05252, Y: 0x55ad63, Z: 0x4a86df };
const ARROW_COLOR = 0xf4f6f7;
const MANIPULATOR_PIXEL_SIZE = 98 * 1.2;
const ARROW_BASE = 0.64;
const ARROW_TIP = 0.78;
const ROTATION_CENTER = 0.88;

/** Application-owned, world-space assembly manipulator. It deliberately has no DOM listeners. */
export class AssemblyManipulator {
  readonly object = new THREE.Group();
  readonly root = new THREE.Group();
  private readonly handles: Handle[] = [];
  private readonly arrowLines: Array<{axis: Axis; geometry: THREE.BufferGeometry}> = [];
  private drag?: Drag;
  private hovered?: Handle;
  private visible = false;
  private readonly frame=new THREE.Quaternion();

  constructor(private readonly shaders: CadShaderLibrary, private readonly callbacks: AssemblyManipulatorCallbacks) {
    this.root.name = "occccad-assembly-manipulator";
    this.root.renderOrder = 100;
    this.root.add(this.object);
    const hubMaterial = this.shaders.createMaterial("cad.manipulator.glyph", {uColor:new THREE.Color(0xf5f7f8)});
    const hub = new THREE.Mesh(new THREE.PlaneGeometry(1,1),hubMaterial); hub.scale.setScalar(0.19);
    const hubPick = new THREE.Mesh(new THREE.SphereGeometry(0.14, 16, 12), new THREE.MeshBasicMaterial({ visible: false }));
    this.object.add(hub, hubPick);
    this.handles.push({ operation: "pivot", pick: hubPick, materials: [hubMaterial] });
    for (const axis of ["X", "Y", "Z"] as const) this.addAxis(axis);
    this.root.visible = false;
  }

  attach(position: THREE.Vector3,orientation=new THREE.Quaternion()): void {
    this.object.position.copy(position);this.frame.copy(orientation);this.object.quaternion.copy(this.frame);
    this.visible = true; this.root.visible = true;
  }
  setAuthoritativePose(position: THREE.Vector3): void {
    this.object.position.copy(position); this.object.quaternion.copy(this.frame);
  }
  frameQuaternion(): THREE.Quaternion { return this.frame.clone(); }
  setPreviewPose(position: THREE.Vector3, orientation: THREE.Quaternion): void {
    this.object.position.copy(position); this.object.quaternion.copy(orientation);
  }
  commitPreviewFrame(): void {
    this.frame.copy(this.object.quaternion).normalize();
    this.callbacks.pivotChanged({ position: this.object.position.clone(), orientation: this.frame.clone() });
  }
  candidatePose(): { position: THREE.Vector3; rotation: THREE.Quaternion } {
    return this.drag
      ? { position: this.drag.desiredPosition.clone(), rotation: this.drag.desiredQuaternion.clone() }
      : { position: this.object.position.clone(), rotation: this.object.quaternion.clone() };
  }
  detach(): void { this.drag = undefined; this.setHovered(undefined); this.visible = false; this.root.visible = false; }
  isAttached(): boolean { return this.visible; }
  isDragging(): boolean { return Boolean(this.drag); }

  updateScale(camera: THREE.Camera, metrics: ViewportMetrics): void {
    if (!this.visible) return;
    const worldPerPixel = worldUnitsPerCssPixel(camera, this.object.getWorldPosition(new THREE.Vector3()), metrics);
    this.object.scale.setScalar(worldPerPixel * MANIPULATOR_PIXEL_SIZE);
    const cameraLocal=this.object.worldToLocal(camera.getWorldPosition(new THREE.Vector3()));
    for(const arrow of this.arrowLines)this.updateArrowLine(arrow.axis,arrow.geometry,cameraLocal);
  }

  pointerMove(pointerId: number, x: number, y: number, camera: THREE.PerspectiveCamera, surface: HTMLElement): boolean {
    if (!this.visible) return false;
    if (!this.drag) { this.setHovered(this.pick(x, y, camera, surface)); return Boolean(this.hovered); }
    if (this.drag.pointerId !== pointerId) return true;
    const drag = this.drag;
    if (drag.handle.operation === "pivot") {
      const snapped = this.callbacks.snapPivot(x, y);
      if (snapped) {
        this.object.position.copy(snapped.position);
        drag.startPosition.copy(snapped.position);
        drag.desiredPosition.copy(snapped.position);
        if(snapped.orientation){this.frame.copy(snapped.orientation);this.object.quaternion.copy(this.frame);}
        this.callbacks.pivotChanged({position:snapped.position.clone(),orientation:snapped.orientation?.clone()});
      }
    } else if (drag.handle.operation === "translate" && drag.handle.axis) {
      const metrics={cssWidth:Math.max(surface.clientWidth,1),cssHeight:Math.max(surface.clientHeight,1),devicePixelRatio:1};
      const axis=AXES[drag.handle.axis].clone().applyQuaternion(this.frame);
      const targetScreen=new THREE.Vector2(x,y).sub(drag.pointerToHandleOffset);
      const worldStep=worldUnitsPerCssPixel(camera,drag.desiredPosition,metrics);
      const screenAt=(parameter:number)=>{
        const pivot=drag.startPosition.clone().addScaledVector(axis,parameter);
        const scale=worldUnitsPerCssPixel(camera,pivot,metrics)*MANIPULATOR_PIXEL_SIZE;
        return this.screenPoint(pivot.addScaledVector(axis,ARROW_TIP*scale),camera,surface);
      };
      drag.axisParameter=solveScreenConstrainedParameter(targetScreen,drag.axisParameter,screenAt,worldStep);
      drag.desiredPosition.copy(drag.startPosition).addScaledVector(axis,drag.axisParameter);
    } else if (drag.handle.axis&&drag.handle.radialAxis) {
      const origin=drag.startPosition;
      const axis=AXES[drag.handle.axis].clone().applyQuaternion(this.frame);
      const radialAxis=AXES[drag.handle.radialAxis].clone().applyQuaternion(this.frame);
      const metrics={cssWidth:Math.max(surface.clientWidth,1),cssHeight:Math.max(surface.clientHeight,1),devicePixelRatio:1};
      const scale=worldUnitsPerCssPixel(camera,origin,metrics)*MANIPULATOR_PIXEL_SIZE;
      const targetScreen=new THREE.Vector2(x,y).sub(drag.pointerToHandleOffset);
      const screenAt=(angle:number)=>{
        const rotation=new THREE.Quaternion().setFromAxisAngle(axis,angle);
        const radial=radialAxis.clone().applyQuaternion(rotation).multiplyScalar(ROTATION_CENTER*scale);
        return this.screenPoint(origin.clone().add(radial),camera,surface);
      };
      drag.angleParameter=solveScreenConstrainedParameter(targetScreen,drag.angleParameter,screenAt,1e-3,8,Math.PI/8);
      drag.desiredQuaternion.setFromAxisAngle(axis,drag.angleParameter).normalize();
    }
    if (drag.handle.operation !== "pivot") this.callbacks.poseChanged();
    return true;
  }

  pointerDown(pointerId: number, x: number, y: number, camera: THREE.PerspectiveCamera, surface: HTMLElement): boolean {
    if (!this.visible || this.drag) return false;
    const handle = this.pick(x, y, camera, surface);
    if (!handle) return false;
    const origin = this.object.getWorldPosition(new THREE.Vector3());
    const handlePoint=handle.operation==="translate"&&handle.axis
      ? this.object.localToWorld(AXES[handle.axis].clone().multiplyScalar(ARROW_TIP))
      :handle.operation==="rotate"&&handle.radialAxis
        ?this.object.localToWorld(AXES[handle.radialAxis].clone().multiplyScalar(ROTATION_CENTER)):origin;
    const pointerToHandleOffset=new THREE.Vector2(x,y).sub(this.screenPoint(handlePoint,camera,surface));
    this.drag = { pointerId, handle, startPosition: this.object.position.clone(),
      startQuaternion: new THREE.Quaternion(), desiredPosition: this.object.position.clone(),
      desiredQuaternion: new THREE.Quaternion(),axisParameter:0,angleParameter:0,pointerToHandleOffset };
    this.setHandleActive(handle,1);
    if (handle.operation !== "pivot") this.callbacks.dragStarted();
    return true;
  }

  pointerUp(pointerId: number, commit: boolean): boolean {
    if (!this.drag || (pointerId >= 0 && this.drag.pointerId !== pointerId)) return false;
    const operation = this.drag.handle.operation;
    if (operation === "rotate" && !commit) this.object.quaternion.copy(this.frame);
    this.setHandleActive(this.drag.handle,0);
    this.drag = undefined;
    if (operation !== "pivot") this.callbacks.dragFinished(commit);
    return true;
  }

  dispose(): void {
    this.root.traverse((object) => {
      if (object instanceof THREE.Mesh || object instanceof THREE.Line) object.geometry.dispose();
      if (object instanceof THREE.Mesh || object instanceof THREE.Line || object instanceof THREE.Sprite)
        (object.material as THREE.Material).dispose();
    });
  }

  private addAxis(axis: Axis): void {
    const color = new THREE.Color(COLORS[axis]);
    const lineMaterial = this.shaders.createMaterial("cad.manipulator.line", { uColor: new THREE.Color(ARROW_COLOR) });
    const lineGeometry=new THREE.BufferGeometry();
    lineGeometry.setAttribute("position",new THREE.Float32BufferAttribute(new Array(24).fill(0),3));
    const arrow=new THREE.LineSegments(lineGeometry,lineMaterial);this.arrowLines.push({axis,geometry:lineGeometry});
    const axisPick = new THREE.Mesh(new THREE.CylinderGeometry(0.06, 0.06, 0.76, 8), new THREE.MeshBasicMaterial({ visible: false }));
    axisPick.position.copy(AXES[axis]).multiplyScalar(0.38); this.orientY(axisPick, axis);
    this.object.add(arrow,axisPick);
    this.handles.push({ axis, operation: "translate", pick: axisPick, materials: [lineMaterial] });

    const ringMaterial=this.shaders.createMaterial("cad.manipulator.line",{uColor:color});
    const rotationAxis: Record<Axis, Axis> = { X: "Z", Y: "X", Z: "Y" };
    const ring=new THREE.LineSegments(this.rotationControlGeometry(axis,rotationAxis[axis]),ringMaterial);
    const ringPick = new THREE.Mesh(new THREE.SphereGeometry(0.11, 12, 8), new THREE.MeshBasicMaterial({ visible: false }));
    ringPick.position.copy(AXES[axis]).multiplyScalar(ROTATION_CENTER);this.object.add(ring,ringPick);
    this.handles.push({ axis: rotationAxis[axis], radialAxis:axis, operation: "rotate", pick: ringPick, materials: [ringMaterial] });
  }

  private pick(x: number, y: number, camera: THREE.PerspectiveCamera, surface: HTMLElement): Handle | undefined {
    const rect = surface.getBoundingClientRect();
    const pointer = new THREE.Vector2(x / Math.max(rect.width, 1) * 2 - 1, 1 - y / Math.max(rect.height, 1) * 2);
    const ray = new THREE.Raycaster(); ray.setFromCamera(pointer, camera);
    const hit = ray.intersectObjects(this.handles.map((handle) => handle.pick), false)[0]?.object;
    return this.handles.find((handle) => handle.pick === hit);
  }

  private screenPoint(point: THREE.Vector3, camera: THREE.Camera, surface: HTMLElement): THREE.Vector2 {
    const rect = surface.getBoundingClientRect(); const projected = point.project(camera);
    return new THREE.Vector2((projected.x + 1) * rect.width / 2, (1 - projected.y) * rect.height / 2);
  }
  private setHovered(handle?: Handle): void {
    if (this.hovered === handle) return;
    if (this.hovered && this.hovered !== this.drag?.handle) this.setHandleActive(this.hovered,0);
    this.hovered = handle;
    if (handle) this.setHandleActive(handle,0.55);
    this.callbacks.visualChanged();
  }
  private orientY(object: THREE.Object3D, axis: Axis): void {
    object.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), AXES[axis]);
  }
  private updateArrowLine(axis:Axis,geometry:THREE.BufferGeometry,cameraLocal:THREE.Vector3):void{
    const direction=AXES[axis],base=direction.clone().multiplyScalar(ARROW_BASE),tip=direction.clone().multiplyScalar(ARROW_TIP);
    let side=cameraLocal.clone().normalize().cross(direction).normalize();
    if(side.lengthSq()<1e-8)side=new THREE.Vector3(axis==="X"?0:1,axis==="X"?1:0,0);
    const left=base.clone().addScaledVector(side,0.055),right=base.clone().addScaledVector(side,-0.055);
    const points=[direction.clone().multiplyScalar(0.10),base,left,right,left,tip,tip,right];
    const attribute=geometry.getAttribute("position") as THREE.BufferAttribute;
    points.forEach((point,index)=>attribute.setXYZ(index,point.x,point.y,point.z));attribute.needsUpdate=true;
    geometry.computeBoundingSphere();
  }
  private rotationControlGeometry(radialAxis:Axis,rotationAxis:Axis):THREE.BufferGeometry{
    const radial=AXES[radialAxis],normal=AXES[rotationAxis],tangent=normal.clone().cross(radial).normalize();
    const center=radial.clone().multiplyScalar(ROTATION_CENTER),points:THREE.Vector3[]=[];
    const addSegment=(a:THREE.Vector3,b:THREE.Vector3)=>points.push(a,b);
    const circleSegments=24,circleRadius=0.052;
    for(let index=0;index<circleSegments;index++){
      const a=index/circleSegments*Math.PI*2,b=(index+1)/circleSegments*Math.PI*2;
      addSegment(center.clone().addScaledVector(radial,Math.cos(a)*circleRadius).addScaledVector(tangent,Math.sin(a)*circleRadius),
        center.clone().addScaledVector(radial,Math.cos(b)*circleRadius).addScaledVector(tangent,Math.sin(b)*circleRadius));
    }
    const arcPoint=(angle:number)=>radial.clone().multiplyScalar(Math.cos(angle)*ROTATION_CENTER).addScaledVector(tangent,Math.sin(angle)*ROTATION_CENTER);
    for(const [from,to] of [[-0.22,-0.07],[0.07,0.22]])for(let index=0;index<8;index++){
      const a=from+(to-from)*index/8,b=from+(to-from)*(index+1)/8;addSegment(arcPoint(a),arcPoint(b));
    }
    return new THREE.BufferGeometry().setFromPoints(points);
  }
  private setHandleActive(handle:Handle,value:number):void{
    for(const material of handle.materials)material.uniforms.uActive.value=value;
  }
}
