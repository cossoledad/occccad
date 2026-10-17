import * as THREE from "three";
import type { CadCamera } from "../navigation/camera-rig";
import { worldUnitsPerCssPixel } from "./viewport-metrics";

export type InfiniteGridFrame = { origin: THREE.Vector3; normal: THREE.Vector3; u: THREE.Vector3; v: THREE.Vector3 };
const WORLD_FRAME: InfiniteGridFrame = { origin: new THREE.Vector3(0,0,-0.02), normal: new THREE.Vector3(0,0,1),
  u: new THREE.Vector3(1,0,0), v: new THREE.Vector3(0,1,0) };

/** Shared by the drawn grid and sketch snapping, in document units. */
export function adaptiveGridSpacing(camera: CadCamera, height: number, point=new THREE.Vector3()): number {
  const requested = worldUnitsPerCssPixel(camera,point,{cssWidth:1,cssHeight:Math.max(height,1),devicePixelRatio:1})*32;
  const decade = Math.pow(10, Math.floor(Math.log10(requested)));
  const ratio = requested / decade;
  return decade * (ratio <= 1 ? 1 : ratio <= 2 ? 2 : ratio <= 5 ? 5 : 10);
}

/** An unbounded plane with real scene depth; sketch grids remain view overlays. */
export class InfiniteGroundGrid {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.Camera();
  private readonly geometry = new THREE.PlaneGeometry(2, 2);
  readonly object: THREE.Mesh;
  private readonly material: THREE.ShaderMaterial;
  private spacing:number;
  constructor(color: number, style: "ground" | "sketch" = "ground") {
    this.spacing=style==="ground"?10:0;
    this.material = new THREE.ShaderMaterial({
      depthTest: style==="ground", depthWrite: false, transparent: true,
      uniforms: { uOrigin:{value:new THREE.Vector3()},uRight:{value:new THREE.Vector3()},uUp:{value:new THREE.Vector3()},uForward:{value:new THREE.Vector3()},uPerspective:{value:0},uClip:{value:new THREE.Matrix4()},uSpacing: { value: 10 }, uOpacity:{value:1}, uSketch: { value: style === "sketch" ? 1 : 0 }, uColor: { value: new THREE.Color(color) } },
      vertexShader: `varying vec2 vScreen;
        void main() { vScreen = position.xy; gl_Position = vec4(position.xy, 0.0, 1.0); }`,
      fragmentShader: `varying vec2 vScreen;
        uniform vec3 uOrigin,uRight,uUp,uForward;
        uniform float uPerspective;
        uniform mat4 uClip;
        uniform float uSpacing, uSketch, uOpacity;
        uniform vec3 uColor;
        float grid(vec2 point, float spacing, float width) {
          vec2 p = point / spacing;
          vec2 footprint = max(fwidth(p), vec2(0.000001));
          vec2 line = 1.0 - smoothstep(vec2(max(0.0, width - 0.35)), vec2(width + 0.35), abs(fract(p + 0.5) - 0.5) / footprint);
          line *= 1.0 - smoothstep(vec2(0.08), vec2(0.25), footprint);
          return max(line.x, line.y);
        }
        void main() {
          vec3 offset=uRight*vScreen.x+uUp*vScreen.y;
          vec3 origin=uOrigin+offset*(1.0-uPerspective);
          vec3 ray=uForward+offset*uPerspective;
          if(abs(ray.z)<1e-8)discard;
          float distance=-origin.z/ray.z;
          if(distance<0.0)discard;
          vec2 point=(origin+ray*distance).xy;
          vec4 clip=uClip*vec4(point,0.0,1.0);
          float depth=clip.z/clip.w;
          if(clip.w<=0.0||depth< -1.0||depth>1.0)discard;
          float minor = grid(point, uSpacing, mix(0.35, 1.0, uSketch));
          float major = grid(point, uSpacing * 10.0, mix(0.7, 1.0, uSketch));
          // Sketch subdivisions are dotted; world context remains continuous lines.
          vec2 p = point / uSpacing;
          vec2 distanceToNode = abs(fract(p + 0.5) - 0.5) / max(fwidth(p), vec2(0.000001));
          float dots = 1.0 - smoothstep(0.8, 1.8, length(distanceToNode));
          float alpha = mix(max(minor * 0.16, major * 0.38),
            max(dots * 0.65, major * 0.30), uSketch);
          gl_FragDepth = uSketch > 0.5 ? 0.0 : depth * 0.5 + 0.5;
          gl_FragColor = vec4(uColor, clamp(alpha*uOpacity, 0.0, 0.85));
          #include <tonemapping_fragment>
          #include <colorspace_fragment>
        }`,
    });
    this.object = new THREE.Mesh(this.geometry, this.material);
    this.object.frustumCulled = false; this.object.renderOrder = style==="sketch"?14:0; this.scene.add(this.object);
  }
  setStyle(color:string,opacity:number,spacing=0):void {this.material.uniforms.uColor.value.set(color);this.material.uniforms.uOpacity.value=opacity;this.spacing=spacing;}
  render(renderer: THREE.WebGLRenderer, camera: CadCamera, frame = WORLD_FRAME): void {
    this.update(camera, renderer.domElement.clientHeight, frame);
    renderer.render(this.scene, this.camera);
  }
  update(camera: CadCamera, height: number, frame = WORLD_FRAME): void {
    camera.updateMatrixWorld(true);
    const direction=camera.getWorldDirection(new THREE.Vector3());
    const denominator=direction.dot(frame.normal),perspective=camera instanceof THREE.PerspectiveCamera;
    this.object.visible=perspective||Math.abs(denominator)>=1e-5;
    if(!this.object.visible)return;
    const focus=camera.position.clone().addScaledVector(direction,Math.abs(denominator)>1e-8?frame.origin.clone().sub(camera.position).dot(frame.normal)/denominator:0);
    const spacing=this.spacing>0?this.spacing:adaptiveGridSpacing(camera,height,frame.origin);
    // Use a lattice-aligned local origin before uploading floats to the GPU.
    const relative=focus.clone().sub(frame.origin),period=spacing*10;
    const anchor=frame.origin.clone().addScaledVector(frame.u,Math.round(relative.dot(frame.u)/period)*period)
      .addScaledVector(frame.v,Math.round(relative.dot(frame.v)/period)*period);
    const local=(p:THREE.Vector3)=>new THREE.Vector3(p.dot(frame.u),p.dot(frame.v),p.dot(frame.normal));
    const right=new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld,0),up=new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld,1);
    const halfHeight=perspective?Math.tan(THREE.MathUtils.degToRad(camera.fov/2))/camera.zoom:(camera.top-camera.bottom)/(2*camera.zoom);
    const halfWidth=halfHeight*(perspective?camera.aspect:(camera.right-camera.left)/(camera.top-camera.bottom));
    const uniforms=this.material.uniforms;
    uniforms.uOrigin.value.copy(local(camera.position.clone().sub(anchor)));
    uniforms.uRight.value.copy(local(right.multiplyScalar(halfWidth)));
    uniforms.uUp.value.copy(local(up.multiplyScalar(halfHeight)));
    uniforms.uForward.value.copy(local(direction));uniforms.uPerspective.value=perspective?1:0;
    const frameMatrix=new THREE.Matrix4().makeBasis(frame.u,frame.v,frame.normal).setPosition(anchor);
    uniforms.uClip.value.copy(camera.projectionMatrix).multiply(camera.matrixWorldInverse).multiply(frameMatrix);
    uniforms.uSpacing.value=spacing;
    this.object.position.copy(focus);
  }
  dispose(): void { this.geometry.dispose(); this.material.dispose(); }
}
