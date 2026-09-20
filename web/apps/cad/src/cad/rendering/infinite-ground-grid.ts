import * as THREE from "three";

export type InfiniteGridFrame = { origin: THREE.Vector3; normal: THREE.Vector3; u: THREE.Vector3; v: THREE.Vector3 };
const WORLD_FRAME: InfiniteGridFrame = { origin: new THREE.Vector3(0,0,-0.02), normal: new THREE.Vector3(0,0,1),
  u: new THREE.Vector3(1,0,0), v: new THREE.Vector3(0,1,0) };

/** Project an unbounded plane into a background pass, independent of model clipping. */
export class InfiniteGroundGrid {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.Camera();
  private readonly geometry = new THREE.PlaneGeometry(2, 2);
  readonly object: THREE.Mesh;
  private readonly material: THREE.ShaderMaterial;
  constructor(color: number, style: "ground" | "sketch" = "ground") {
    this.material = new THREE.ShaderMaterial({
      depthTest: false, depthWrite: false, transparent: true,
      uniforms: { uCenter: { value: new THREE.Vector2() }, uHorizontal: { value: new THREE.Vector2() },
        uVertical: { value: new THREE.Vector2() }, uSpacing: { value: 10 }, uSketch: { value: style === "sketch" ? 1 : 0 }, uColor: { value: new THREE.Color(color) } },
      vertexShader: `varying vec2 vScreen;
        void main() { vScreen = position.xy; gl_Position = vec4(position.xy, 0.0, 1.0); }`,
      fragmentShader: `varying vec2 vScreen;
        uniform vec2 uCenter, uHorizontal, uVertical;
        uniform float uSpacing, uSketch;
        uniform vec3 uColor;
        float grid(vec2 point, float spacing) {
          vec2 p = point / spacing;
          vec2 footprint = max(fwidth(p), vec2(0.000001));
          vec2 line = 1.0 - smoothstep(vec2(0.35), vec2(1.35), abs(fract(p + 0.5) - 0.5) / footprint);
          line *= 1.0 - smoothstep(vec2(0.08), vec2(0.25), footprint);
          return max(line.x, line.y);
        }
        void main() {
          vec2 point = uCenter + uHorizontal * vScreen.x + uVertical * vScreen.y;
          float minor = grid(point, uSpacing);
          // Sketch subdivisions are dotted; world context remains continuous lines.
          vec2 p = point / uSpacing;
          vec2 distanceToNode = abs(fract(p + 0.5) - 0.5) / max(fwidth(p), vec2(0.000001));
          float dots = 1.0 - smoothstep(0.8, 1.8, length(distanceToNode));
          float alpha = mix(max(minor * 0.13, grid(point, uSpacing * 10.0) * 0.22),
            max(dots * 0.65, grid(point, uSpacing * 10.0) * 0.30), uSketch);
          gl_FragColor = vec4(uColor, alpha);
          #include <tonemapping_fragment>
          #include <colorspace_fragment>
        }`,
    });
    this.object = new THREE.Mesh(this.geometry, this.material);
    this.object.frustumCulled = false; this.object.renderOrder = 14; this.scene.add(this.object);
  }
  render(renderer: THREE.WebGLRenderer, camera: THREE.OrthographicCamera): void {
    this.update(camera, renderer.domElement.clientHeight);
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
    const requestedSpacing = halfHeight * 2 / Math.max(height, 1) * 32;
    const decade = Math.pow(10, Math.floor(Math.log10(requestedSpacing)));
    const ratio = requestedSpacing / decade;
    const spacing = decade * (ratio <= 1 ? 1 : ratio <= 2 ? 2 : ratio <= 5 ? 5 : 10);
    const center = camera.position.clone().addScaledVector(direction, frame.origin.clone().sub(camera.position).dot(frame.normal) / denominator).sub(frame.origin);
    const projectAxis = (column: number, extent: number) => {
      const axis = new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, column);
      axis.addScaledVector(direction, -axis.dot(frame.normal) / denominator).multiplyScalar(extent);
      return new THREE.Vector2(axis.dot(frame.u), axis.dot(frame.v));
    };
    const uniforms = this.material.uniforms;
    // Rebase the grid phase before converting to GPU floats, for distant model origins.
    uniforms.uCenter.value.set(center.dot(frame.u) % (spacing * 10), center.dot(frame.v) % (spacing * 10));
    uniforms.uHorizontal.value.copy(projectAxis(0, halfWidth));
    uniforms.uVertical.value.copy(projectAxis(1, halfHeight));
    uniforms.uSpacing.value = spacing;
  }
  dispose(): void { this.geometry.dispose(); this.material.dispose(); }
}
