import * as THREE from "three";

export type InfiniteGridFrame = { origin: THREE.Vector3; normal: THREE.Vector3; u: THREE.Vector3; v: THREE.Vector3 };
const WORLD_FRAME: InfiniteGridFrame = { origin: new THREE.Vector3(0,0,-0.02), normal: new THREE.Vector3(0,0,1),
  u: new THREE.Vector3(1,0,0), v: new THREE.Vector3(0,1,0) };

/** Shared by the drawn grid and sketch snapping, in document units. */
export function adaptiveGridSpacing(camera: THREE.OrthographicCamera, height: number): number {
  const requested = (camera.top - camera.bottom) / camera.zoom / Math.max(height, 1) * 32;
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
      uniforms: { uCenter: { value: new THREE.Vector2() }, uHorizontal: { value: new THREE.Vector2() },
        uVertical: { value: new THREE.Vector2() }, uDepth:{value:new THREE.Vector3()},uSpacing: { value: 10 }, uOpacity:{value:1}, uSketch: { value: style === "sketch" ? 1 : 0 }, uColor: { value: new THREE.Color(color) } },
      vertexShader: `varying vec2 vScreen;
        void main() { vScreen = position.xy; gl_Position = vec4(position.xy, 0.0, 1.0); }`,
      fragmentShader: `varying vec2 vScreen;
        uniform vec2 uCenter, uHorizontal, uVertical;
        uniform float uSpacing, uSketch, uOpacity;
        uniform vec3 uColor;
        uniform vec3 uDepth;
        float grid(vec2 point, float spacing, float width) {
          vec2 p = point / spacing;
          vec2 footprint = max(fwidth(p), vec2(0.000001));
          vec2 line = 1.0 - smoothstep(vec2(max(0.0, width - 0.35)), vec2(width + 0.35), abs(fract(p + 0.5) - 0.5) / footprint);
          line *= 1.0 - smoothstep(vec2(0.08), vec2(0.25), footprint);
          return max(line.x, line.y);
        }
        void main() {
          vec2 point = uCenter + uHorizontal * vScreen.x + uVertical * vScreen.y;
          float minor = grid(point, uSpacing, mix(0.35, 1.0, uSketch));
          float major = grid(point, uSpacing * 10.0, mix(0.7, 1.0, uSketch));
          // Sketch subdivisions are dotted; world context remains continuous lines.
          vec2 p = point / uSpacing;
          vec2 distanceToNode = abs(fract(p + 0.5) - 0.5) / max(fwidth(p), vec2(0.000001));
          float dots = 1.0 - smoothstep(0.8, 1.8, length(distanceToNode));
          float alpha = mix(max(minor * 0.16, major * 0.38),
            max(dots * 0.65, major * 0.30), uSketch);
          float depth = dot(uDepth, vec3(1.0, vScreen));
          if (uSketch < 0.5 && depth < -1.0) discard;
          gl_FragDepth = uSketch > 0.5 ? 0.0 : clamp(depth * 0.5 + 0.5, 0.0, 1.0);
          gl_FragColor = vec4(uColor, clamp(alpha*uOpacity, 0.0, 0.85));
          #include <tonemapping_fragment>
          #include <colorspace_fragment>
        }`,
    });
    this.object = new THREE.Mesh(this.geometry, this.material);
    this.object.frustumCulled = false; this.object.renderOrder = style==="sketch"?14:0; this.scene.add(this.object);
  }
  setStyle(color:string,opacity:number,spacing=0):void {this.material.uniforms.uColor.value.set(color);this.material.uniforms.uOpacity.value=opacity;this.spacing=spacing;}
  render(renderer: THREE.WebGLRenderer, camera: THREE.OrthographicCamera, frame = WORLD_FRAME): void {
    this.update(camera, renderer.domElement.clientHeight, frame);
    renderer.render(this.scene, this.camera);
  }
  update(camera: THREE.OrthographicCamera, height: number, frame = WORLD_FRAME): void {
    camera.updateMatrixWorld(true);
    const direction = camera.getWorldDirection(new THREE.Vector3());
    // An exactly edge-on plane has no screen area; suppress numerical grazing-angle noise.
    const denominator = direction.dot(frame.normal);
    this.object.visible = Math.abs(denominator) >= 1e-5;
    if (!this.object.visible) return;
    const halfHeight = (camera.top - camera.bottom) / (2 * camera.zoom);
    const halfWidth = (camera.right - camera.left) / (2 * camera.zoom);
    const spacing = this.spacing>0?this.spacing:adaptiveGridSpacing(camera, height);
    const centerWorld = camera.position.clone().addScaledVector(direction, frame.origin.clone().sub(camera.position).dot(frame.normal) / denominator);
    this.object.position.copy(centerWorld); // Transparent scene sorting uses the plane's viewing depth.
    const center=centerWorld.clone().sub(frame.origin);
    const projectAxis = (column: number, extent: number) => {
      const axis = new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, column);
      axis.addScaledVector(direction, -axis.dot(frame.normal) / denominator).multiplyScalar(extent);
      return new THREE.Vector2(axis.dot(frame.u), axis.dot(frame.v));
    };
    const uniforms = this.material.uniforms;
    const depth=centerWorld.clone().project(camera).z;
    const projectedDepth=(column:number,extent:number)=>{
      const axis=new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld,column);
      axis.addScaledVector(direction,-axis.dot(frame.normal)/denominator).multiplyScalar(extent);
      return centerWorld.clone().add(axis).project(camera).z-depth;
    };
    uniforms.uDepth.value.set(depth,projectedDepth(0,halfWidth),projectedDepth(1,halfHeight));
    // Rebase the grid phase before converting to GPU floats, for distant model origins.
    uniforms.uCenter.value.set(center.dot(frame.u) % (spacing * 10), center.dot(frame.v) % (spacing * 10));
    uniforms.uHorizontal.value.copy(projectAxis(0, halfWidth));
    uniforms.uVertical.value.copy(projectAxis(1, halfHeight));
    uniforms.uSpacing.value = spacing;
  }
  dispose(): void { this.geometry.dispose(); this.material.dispose(); }
}
