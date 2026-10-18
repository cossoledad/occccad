import * as THREE from "three";
import type { CadCamera } from "../navigation/camera-rig";
import { worldUnitsPerCssPixel } from "./viewport-metrics";
import type { GroundFootprint } from "./scene-ground";

export type GridFrame = { origin: THREE.Vector3; normal: THREE.Vector3; u: THREE.Vector3; v: THREE.Vector3 };
const WORLD_FRAME: GridFrame = { origin: new THREE.Vector3(0,0,-0.02), normal: new THREE.Vector3(0,0,1),
  u: new THREE.Vector3(1,0,0), v: new THREE.Vector3(0,1,0) };

/** Shared by the drawn grid and sketch snapping, in document units. */
export function adaptiveGridSpacing(camera: CadCamera, height: number, point=new THREE.Vector3()): number {
  const requested = worldUnitsPerCssPixel(camera,point,{cssWidth:1,cssHeight:Math.max(height,1),devicePixelRatio:1})*32;
  const decade = Math.pow(10, Math.floor(Math.log10(requested)));
  const ratio = requested / decade;
  return decade * (ratio <= 1 ? 1 : ratio <= 2 ? 2 : ratio <= 5 ? 5 : 10);
}

/** Every level is a fixed power-of-ten lattice anchored to the same document origin. */
export function groundGridLevels(spacing:number,worldPerPixel:number):{lower:number;upper:number;blend:number;opacity:number} {
  const pixel=Number.isFinite(worldPerPixel)&&worldPerPixel>0?worldPerPixel:spacing/32;
  const level=THREE.MathUtils.clamp(Math.log10(pixel*32/spacing),-12,12);
  const lower=Math.floor(level),pixels=spacing/pixel;
  const fine=THREE.MathUtils.smoothstep(pixels,128,512)*.14;
  const coarse=(1-THREE.MathUtils.smoothstep(pixels,4,12))*.30;
  return {lower:spacing*10**lower,upper:spacing*10**(lower+1),blend:level-lower,opacity:Math.max(fine,coarse)};
}

/** Ray-plane grid with a finite square ground; sketch grids remain plane overlays. */
export class PlaneGrid {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.Camera();
  private readonly geometry = new THREE.PlaneGeometry(2, 2);
  readonly object: THREE.Mesh;
  private readonly material: THREE.ShaderMaterial;
  private spacing:number;
  private footprint:GroundFootprint={center:new THREE.Vector2(),halfSize:100};
  constructor(color: number, style: "ground" | "sketch" = "ground") {
    this.spacing=style==="ground"?10:0;
    this.material = new THREE.ShaderMaterial({
      depthTest: style==="ground", depthWrite: false, transparent: true,
      uniforms: { uBounds:{value:new THREE.Vector4()},uOrigin:{value:new THREE.Vector3()},uRight:{value:new THREE.Vector3()},uUp:{value:new THREE.Vector3()},uForward:{value:new THREE.Vector3()},uPerspective:{value:0},uBasePhase:{value:new THREE.Vector2()},uLevelPhaseA:{value:new THREE.Vector2()},uLevelPhaseB:{value:new THREE.Vector2()},uLevels:{value:new THREE.Vector2()},uLevelBlend:{value:0},uLevelOpacity:{value:0},uClip:{value:new THREE.Matrix4()},uSpacing: { value: 10 }, uOpacity:{value:1}, uSketch: { value: style === "sketch" ? 1 : 0 }, uColor: { value: new THREE.Color(color) } },
      vertexShader: `varying vec2 vScreen;
        void main() { vScreen = position.xy; gl_Position = vec4(position.xy, 0.0, 1.0); }`,
      fragmentShader: `varying vec2 vScreen;
        uniform vec3 uOrigin,uRight,uUp,uForward;
        uniform float uPerspective;
        uniform mat4 uClip;
        uniform float uSpacing, uSketch, uOpacity;
        uniform vec3 uColor;
        uniform vec4 uBounds;
        uniform vec2 uBasePhase,uLevelPhaseA,uLevelPhaseB,uLevels;
        uniform float uLevelBlend,uLevelOpacity;
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
          if(uSketch<0.5&&(any(lessThan(point,uBounds.xy))||any(greaterThan(point,uBounds.zw))))discard;
          vec4 clip=uClip*vec4(point,0.0,1.0);
          float depth=clip.z/clip.w;
          if(clip.w<=0.0)discard;
          // The finite viewing aid keeps real depth inside the frustum and
          // saturates outside the model clip range; its square still limits it.
          if(uSketch>0.5&&(depth< -1.0||depth>1.0))discard;
          vec2 base=point+uBasePhase;
          float minor = grid(base, uSpacing, mix(0.35, 1.0, uSketch));
          float major = grid(base, uSpacing * 10.0, mix(0.7, 1.0, uSketch));
          // Sketch subdivisions are dotted; world context remains continuous lines.
          vec2 p = base / uSpacing;
          vec2 distanceToNode = abs(fract(p + 0.5) - 0.5) / max(fwidth(p), vec2(0.000001));
          float dots = 1.0 - smoothstep(0.8, 1.8, length(distanceToNode));
          float alpha = mix(max(minor * 0.16, major * 0.38),
            max(dots * 0.65, major * 0.30), uSketch);
          float levelA=grid(point+uLevelPhaseA,uLevels.x,0.7);
          float levelB=grid(point+uLevelPhaseB,uLevels.y,0.7);
          alpha=max(alpha,mix(levelA,levelB,uLevelBlend)*uLevelOpacity);
          gl_FragDepth = uSketch > 0.5 ? 0.0 : clamp(depth * 0.5 + 0.5,0.0,1.0);
          gl_FragColor = vec4(uColor, clamp(alpha*uOpacity, 0.0, 0.85));
          #include <tonemapping_fragment>
          #include <colorspace_fragment>
        }`,
    });
    this.object = new THREE.Mesh(this.geometry, this.material);
    this.object.frustumCulled = false; this.object.renderOrder = style==="sketch"?14:0; this.scene.add(this.object);
  }
  setStyle(color:string,opacity:number,spacing=0):void {this.material.uniforms.uColor.value.set(color);this.material.uniforms.uOpacity.value=opacity;this.spacing=spacing;}
  setFootprint(footprint:GroundFootprint):void {this.footprint={center:footprint.center.clone(),halfSize:footprint.halfSize};}
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
    const right=new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld,0),up=new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld,1);
    const halfHeight=perspective?Math.tan(THREE.MathUtils.degToRad(camera.fov/2))/camera.zoom:(camera.top-camera.bottom)/(2*camera.zoom);
    const halfWidth=halfHeight*(perspective?camera.aspect:(camera.right-camera.left)/(camera.top-camera.bottom));
    const planeDistance=frame.origin.clone().sub(camera.position).dot(frame.normal);
    let focus=camera.position.clone();
    if(!perspective||Math.abs(denominator)>=.05&&planeDistance/denominator>0){
      focus.addScaledVector(direction,planeDistance/denominator);
    } else {
      // At a horizon, use the nearest visible sample instead of a huge/behind-eye centre.
      let nearest=Infinity;
      for(const [x,y] of [[0,0],[-1,0],[1,0],[0,-1],[0,1]]){
        const ray=direction.clone().addScaledVector(right,x*halfWidth).addScaledVector(up,y*halfHeight);
        const depth=planeDistance/ray.dot(frame.normal);
        if(depth>0&&Number.isFinite(depth)&&depth<nearest){nearest=depth;focus.copy(camera.position).addScaledVector(ray,depth);}
      }
    }
    const spacing=this.spacing>0?this.spacing:adaptiveGridSpacing(camera,height,frame.origin);
    const levels=groundGridLevels(spacing,worldUnitsPerCssPixel(camera,focus,{cssWidth:1,cssHeight:Math.max(1,height),devicePixelRatio:1}));
    const ground=this.material.uniforms.uSketch.value===0;
    // Rebase against the finest displayed lattice; retain a CPU-computed phase for each level.
    const relative=focus.clone().sub(frame.origin),period=(ground?Math.min(spacing,levels.lower):spacing)*10;
    const anchor=frame.origin.clone().addScaledVector(frame.u,Math.round(relative.dot(frame.u)/period)*period)
      .addScaledVector(frame.v,Math.round(relative.dot(frame.v)/period)*period);
    const phase=anchor.clone().sub(frame.origin);
    const local=(p:THREE.Vector3)=>new THREE.Vector3(p.dot(frame.u),p.dot(frame.v),p.dot(frame.normal));
    const uniforms=this.material.uniforms;
    const cx=this.footprint.center.x-phase.dot(frame.u),cy=this.footprint.center.y-phase.dot(frame.v),half=this.footprint.halfSize;
    uniforms.uBounds.value.set(cx-half,cy-half,cx+half,cy+half);
    const setPhase=(key:string,period:number)=>uniforms[key].value.set(phase.dot(frame.u)%period,phase.dot(frame.v)%period);
    setPhase("uBasePhase",spacing*10);setPhase("uLevelPhaseA",levels.lower);setPhase("uLevelPhaseB",levels.upper);
    uniforms.uLevels.value.set(levels.lower,levels.upper);uniforms.uLevelBlend.value=levels.blend;uniforms.uLevelOpacity.value=ground?levels.opacity:0;
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
