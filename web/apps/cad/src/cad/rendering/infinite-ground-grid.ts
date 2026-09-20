import * as THREE from "three";

/** Project an unbounded world XY plane into a background pass, independent of model clipping. */
export class InfiniteGroundGrid {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.Camera();
  private readonly geometry = new THREE.PlaneGeometry(2, 2);
  private readonly material: THREE.ShaderMaterial;
  constructor(color: number) {
    this.material = new THREE.ShaderMaterial({
      depthTest: false, depthWrite: false, transparent: true,
      uniforms: { uCenter: { value: new THREE.Vector2() }, uHorizontal: { value: new THREE.Vector2() },
        uVertical: { value: new THREE.Vector2() }, uSpacing: { value: 10 }, uColor: { value: new THREE.Color(color) } },
      vertexShader: `varying vec2 vScreen;
        void main() { vScreen = position.xy; gl_Position = vec4(position.xy, 0.0, 1.0); }`,
      fragmentShader: `varying vec2 vScreen;
        uniform vec2 uCenter, uHorizontal, uVertical;
        uniform float uSpacing;
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
          float alpha = max(grid(point, uSpacing) * 0.13, grid(point, uSpacing * 10.0) * 0.22);
          gl_FragColor = vec4(uColor, alpha);
          #include <tonemapping_fragment>
          #include <colorspace_fragment>
        }`,
    });
    const mesh = new THREE.Mesh(this.geometry, this.material);
    mesh.frustumCulled = false; this.scene.add(mesh);
  }
  render(renderer: THREE.WebGLRenderer, camera: THREE.OrthographicCamera): void {
    camera.updateMatrixWorld(true);
    const direction = camera.getWorldDirection(new THREE.Vector3());
    // An exactly edge-on plane has no screen area; suppress numerical grazing-angle noise.
    if (Math.abs(direction.z) < 1e-5) return;
    const halfHeight = (camera.top - camera.bottom) / (2 * camera.zoom);
    const halfWidth = (camera.right - camera.left) / (2 * camera.zoom);
    const spacing = Math.pow(10, Math.floor(Math.log10(halfHeight * 2 / Math.max(renderer.domElement.clientHeight, 1) * 80)));
    const center = camera.position.clone().addScaledVector(direction, (-0.02 - camera.position.z) / direction.z);
    const projectAxis = (column: number, extent: number) => {
      const axis = new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, column);
      axis.addScaledVector(direction, -axis.z / direction.z).multiplyScalar(extent);
      return new THREE.Vector2(axis.x, axis.y);
    };
    const uniforms = this.material.uniforms;
    // Rebase the grid phase before converting to GPU floats, for distant model origins.
    uniforms.uCenter.value.set(center.x % (spacing * 10), center.y % (spacing * 10));
    uniforms.uHorizontal.value.copy(projectAxis(0, halfWidth));
    uniforms.uVertical.value.copy(projectAxis(1, halfHeight));
    uniforms.uSpacing.value = spacing;
    renderer.render(this.scene, this.camera);
  }
  dispose(): void { this.geometry.dispose(); this.material.dispose(); }
}
