import * as THREE from "three";

export type CadShaderProgramID =
  | "cad.background"
  | "cad.surface"
  | "cad.edge"
  | "cad.point"
  | "cad.constraint.glyph"
  | "cad.manipulator"
  | "cad.overlay.line"
  | "cad.overlay.solid";

export type CadShaderDefinition = {
  uniforms: Record<string, THREE.IUniform>;
  vertexShader: string;
  fragmentShader: string;
  transparent?: boolean;
  depthTest?: boolean;
  depthWrite?: boolean;
  side?: THREE.Side;
  vertexColors?: boolean;
};

const sectionUniforms = () => ({
  uSectionEnabled: { value: false },
  uSectionPlane: { value: new THREE.Vector4(0, 0, 1, 0) },
});

const sectionFragment = `
  uniform bool uSectionEnabled;
  uniform vec4 uSectionPlane;
  void applyCadSection() {
    if (uSectionEnabled && dot(vec4(vWorldPosition, 1.0), uSectionPlane) > 0.0) discard;
  }
`;

/** Central registry for all application-owned GLSL programs. */
export class CadShaderLibrary {
  private readonly definitions = new Map<CadShaderProgramID, CadShaderDefinition>();

  constructor() { this.registerBuiltins(); }

  register(id: CadShaderProgramID, definition: CadShaderDefinition): void {
    this.definitions.set(id, definition);
  }

  createMaterial(id: CadShaderProgramID, values: Record<string, unknown> = {}): THREE.ShaderMaterial {
    const definition = this.definitions.get(id);
    if (!definition) throw new Error(`CAD shader is not registered: ${id}`);
    const uniforms = THREE.UniformsUtils.clone(definition.uniforms);
    for (const [name, value] of Object.entries(values)) {
      if (uniforms[name]) uniforms[name].value = value;
    }
    const material = new THREE.ShaderMaterial({
      uniforms,
      vertexShader: definition.vertexShader,
      fragmentShader: definition.fragmentShader,
      ...(definition.transparent === undefined ? {} : { transparent: definition.transparent }),
      ...(definition.depthTest === undefined ? {} : { depthTest: definition.depthTest }),
      ...(definition.depthWrite === undefined ? {} : { depthWrite: definition.depthWrite }),
      ...(definition.side === undefined ? {} : { side: definition.side }),
      ...(definition.vertexColors === undefined ? {} : { vertexColors: definition.vertexColors }),
      toneMapped: false,
    });
    material.userData.cadShaderProgram = id;
    return material;
  }

  private registerBuiltins(): void {
    this.register("cad.manipulator", {
      uniforms: { uColor: { value: new THREE.Color() }, uActive: { value: 0 }, uOpacity: { value: 1 } },
      vertexShader: `
        varying vec3 vNormal;
        void main() {
          vNormal = normalize(normalMatrix * normal);
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        uniform float uActive;
        uniform float uOpacity;
        varying vec3 vNormal;
        void main() {
          float light = 0.72 + 0.28 * abs(dot(normalize(vNormal), normalize(vec3(0.25, 0.4, 1.0))));
          vec3 color = mix(uColor * light, vec3(1.0), uActive * 0.68);
          gl_FragColor = vec4(color, uOpacity);
          #include <colorspace_fragment>
        }
      `,
      transparent: true, depthTest: false, depthWrite: false,
    });

    this.register("cad.background", {
      uniforms: {
        uTop: { value: new THREE.Color() },
        uBottom: { value: new THREE.Color() },
        uVignette: { value: 0.12 },
      },
      vertexShader: `
        varying vec2 vUv;
        void main() {
          vUv = uv;
          gl_Position = vec4(position.xy, 0.999, 1.0);
        }
      `,
      fragmentShader: `
        uniform vec3 uTop;
        uniform vec3 uBottom;
        uniform float uVignette;
        varying vec2 vUv;
        void main() {
          float vertical = smoothstep(0.0, 1.0, vUv.y);
          vec3 color = mix(uBottom, uTop, vertical);
          vec2 centered = vUv - 0.5;
          color *= 1.0 - dot(centered, centered) * uVignette;
          gl_FragColor = vec4(color, 1.0);
          #include <colorspace_fragment>
        }
      `,
      depthTest: false,
      depthWrite: false,
    });

    this.register("cad.surface", {
      uniforms: {
        uBaseColor: { value: new THREE.Color() },
        uSelectedColor: { value: new THREE.Color() },
        uSelected: { value: 0 },
        ...sectionUniforms(),
      },
      vertexShader: `
        varying vec3 vViewNormal;
        varying vec3 vViewPosition;
        varying vec3 vWorldPosition;
        void main() {
          vec4 world = modelMatrix * vec4(position, 1.0);
          vec4 view = viewMatrix * world;
          vWorldPosition = world.xyz;
          vViewPosition = view.xyz;
          vViewNormal = normalize(normalMatrix * normal);
          gl_Position = projectionMatrix * view;
        }
      `,
      fragmentShader: `
        uniform vec3 uBaseColor;
        uniform vec3 uSelectedColor;
        uniform float uSelected;
        varying vec3 vViewNormal;
        varying vec3 vViewPosition;
        ${sectionFragment}
        void main() {
          applyCadSection();
          vec3 n = normalize(vViewNormal);
          vec3 viewDir = normalize(-vViewPosition);
          vec3 key = normalize(vec3(-0.35, 0.48, 0.80));
          vec3 fill = normalize(vec3(0.65, -0.20, 0.45));
          float diffuse = max(dot(n, key), 0.0) * 0.54 + max(dot(n, fill), 0.0) * 0.14;
          float rim = pow(1.0 - max(dot(n, viewDir), 0.0), 2.6) * 0.16;
          vec3 halfVector = normalize(key + viewDir);
          float specular = pow(max(dot(n, halfVector), 0.0), 42.0) * 0.22;
          vec3 base = mix(uBaseColor, uSelectedColor, uSelected * 0.42);
          vec3 color = base * (0.48 + diffuse) + vec3(rim + specular);
          gl_FragColor = vec4(color, 1.0);
          #include <colorspace_fragment>
        }
      `,
      side: THREE.DoubleSide,
    });

    this.register("cad.edge", {
      uniforms: {
        uColor: { value: new THREE.Color() },
        uSelectedColor: { value: new THREE.Color() },
        uSelected: { value: 0 },
        uOpacity: { value: 1 },
        ...sectionUniforms(),
      },
      vertexShader: `
        varying vec3 vWorldPosition;
        void main() {
          vec4 world = modelMatrix * vec4(position, 1.0);
          vWorldPosition = world.xyz;
          gl_Position = projectionMatrix * viewMatrix * world;
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        uniform vec3 uSelectedColor;
        uniform float uSelected;
        uniform float uOpacity;
        varying vec3 vWorldPosition;
        ${sectionFragment}
        void main() {
          applyCadSection();
          gl_FragColor = vec4(mix(uColor, uSelectedColor, uSelected), uOpacity);
          #include <colorspace_fragment>
        }
      `,
      transparent: true,
    });

    this.register("cad.point", {
      uniforms: {
        uColor: { value: new THREE.Color() },
        uSelectedColor: { value: new THREE.Color() },
        uSelected: { value: 0 },
        uPointSize: { value: 5 },
        ...sectionUniforms(),
      },
      vertexShader: `
        uniform float uPointSize;
        varying vec3 vWorldPosition;
        void main() {
          vec4 world = modelMatrix * vec4(position, 1.0);
          vWorldPosition = world.xyz;
          gl_Position = projectionMatrix * viewMatrix * world;
          gl_PointSize = uPointSize;
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        uniform vec3 uSelectedColor;
        uniform float uSelected;
        varying vec3 vWorldPosition;
        ${sectionFragment}
        void main() {
          applyCadSection();
          vec2 p = gl_PointCoord - 0.5;
          float diagonal = min(abs(p.x - p.y), abs(p.x + p.y));
          if (max(abs(p.x), abs(p.y)) > 0.48 || diagonal > 0.085) discard;
          gl_FragColor = vec4(mix(uColor, uSelectedColor, uSelected), 1.0);
          #include <colorspace_fragment>
        }
      `,
      transparent: true,
    });

    this.register("cad.constraint.glyph", {
      uniforms: {
        uColor: { value: new THREE.Color() },
        uSelectedColor: { value: new THREE.Color() },
        uSelected: { value: 0 },
        uGlyph: { value: 0 },
        uPointSize: { value: 17 },
      },
      vertexShader: `
        uniform float uPointSize;
        void main() {
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
          gl_PointSize = uPointSize;
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        uniform vec3 uSelectedColor;
        uniform float uSelected;
        uniform float uGlyph;
        float segmentDistance(vec2 p, vec2 a, vec2 b) {
          vec2 ab = b - a;
          float t = clamp(dot(p - a, ab) / max(dot(ab, ab), 0.0001), 0.0, 1.0);
          return length(p - (a + ab * t));
        }
        float ring(vec2 p, float radius) { return abs(length(p) - radius); }
        void main() {
          vec2 p = gl_PointCoord - 0.5;
          float d = 1.0;
          if (uGlyph < 0.5) d = min(ring(p, 0.24), length(p));
          else if (uGlyph < 1.5) d = min(segmentDistance(p, vec2(-0.30,-0.22),vec2(0.05,0.24)),segmentDistance(p,vec2(-0.05,-0.24),vec2(0.30,0.22)));
          else if (uGlyph < 2.5) d = min(max(abs(p.x)-0.24,abs(p.y)-0.24),min(segmentDistance(p,vec2(-0.2,-0.2),vec2(0.2,0.2)),segmentDistance(p,vec2(-0.2,0.2),vec2(0.2,-0.2))));
          else if (uGlyph < 3.5) d = min(segmentDistance(p,vec2(-0.32,0.0),vec2(0.32,0.0)),min(segmentDistance(p,vec2(-0.22,-0.16),vec2(-0.22,0.16)),segmentDistance(p,vec2(0.22,-0.16),vec2(0.22,0.16))));
          else if (uGlyph < 4.5) d = min(segmentDistance(p,vec2(0.0,-0.32),vec2(0.0,0.32)),min(segmentDistance(p,vec2(-0.16,-0.22),vec2(0.16,-0.22)),segmentDistance(p,vec2(-0.16,0.22),vec2(0.16,0.22))));
          else if (uGlyph < 5.5) d = min(segmentDistance(p,vec2(-0.24,0.26),vec2(-0.24,-0.20)),segmentDistance(p,vec2(-0.24,-0.20),vec2(0.25,-0.20)));
          else if (uGlyph < 6.5) d = min(ring(p-vec2(0.0,0.06),0.22),segmentDistance(p,vec2(-0.34,-0.18),vec2(0.34,-0.18)));
          else if (uGlyph < 7.5) d = min(segmentDistance(p,vec2(-0.28,-0.10),vec2(0.28,-0.10)),segmentDistance(p,vec2(-0.28,0.10),vec2(0.28,0.10)));
          else if (uGlyph < 8.5) d = min(segmentDistance(p,vec2(-0.34,0.0),vec2(0.34,0.0)),min(segmentDistance(p,vec2(-0.34,0.0),vec2(-0.20,0.12)),segmentDistance(p,vec2(0.34,0.0),vec2(0.20,-0.12))));
          else if (uGlyph < 9.5) d = min(segmentDistance(p,vec2(-0.28,-0.24),vec2(0.28,0.24)),min(segmentDistance(p,vec2(-0.28,-0.24),vec2(-0.12,-0.20)),segmentDistance(p,vec2(0.28,0.24),vec2(0.12,0.20))));
          else if (uGlyph < 10.5) d = min(ring(p,0.25),segmentDistance(p,vec2(0.0,0.0),vec2(0.28,0.0)));
          else if (uGlyph < 11.5) d = min(ring(p,0.25),segmentDistance(p,vec2(-0.28,-0.28),vec2(0.28,0.28)));
          else if (uGlyph < 12.5) d = min(segmentDistance(p,vec2(-0.30,-0.22),vec2(0.0,0.0)),min(segmentDistance(p,vec2(0.0,0.0),vec2(0.30,-0.22)),ring(p-vec2(0.0,-0.22),0.24)));
          else if (uGlyph < 13.5) d = min(ring(p,0.28),ring(p,0.14));
          else if (uGlyph < 14.5) d = min(segmentDistance(p,vec2(-0.32,0.0),vec2(0.32,0.0)),length(p));
          else if (uGlyph < 15.5) d = min(min(segmentDistance(p,vec2(-0.28,-0.20),vec2(0.0,0.26)),segmentDistance(p,vec2(0.0,0.26),vec2(0.28,-0.20))),length(p));
          else d = min(segmentDistance(p,vec2(0.0,-0.36),vec2(0.0,0.36)),min(length(p-vec2(-0.24,0.0)),length(p-vec2(0.24,0.0))));
          float alpha = 1.0 - smoothstep(0.022, 0.052, d);
          if (alpha < 0.02) discard;
          gl_FragColor = vec4(mix(uColor, uSelectedColor, uSelected), alpha);
          #include <colorspace_fragment>
        }
      `,
      transparent: true,
      depthTest: false,
      depthWrite: false,
    });

    const overlayVertex = `
      attribute float intensity;
      varying float vIntensity;
      void main() {
        vIntensity = intensity;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `;
    this.register("cad.overlay.line", {
      uniforms: {
        uColor: { value: new THREE.Color() },
        uHighlight: { value: new THREE.Color() },
        uOpacity: { value: 1 },
      },
      vertexShader: overlayVertex,
      fragmentShader: `
        uniform vec3 uColor;
        uniform vec3 uHighlight;
        uniform float uOpacity;
        varying float vIntensity;
        void main() {
          gl_FragColor = vec4(mix(uColor, uHighlight, vIntensity), uOpacity);
          #include <colorspace_fragment>
        }
      `,
      transparent: true,
      depthTest: false,
      depthWrite: false,
    });

    this.register("cad.overlay.solid", {
      uniforms: {
        uColor: { value: new THREE.Color() },
        uHighlight: { value: new THREE.Color() },
      },
      vertexShader: `
        varying vec3 vNormal;
        void main() {
          vNormal = normalize(normalMatrix * normal);
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        uniform vec3 uHighlight;
        varying vec3 vNormal;
        void main() {
          float light = 0.35 + max(dot(normalize(vNormal), normalize(vec3(-0.4, 0.6, 0.7))), 0.0) * 0.65;
          gl_FragColor = vec4(mix(uColor, uHighlight, light * 0.45) * light, 1.0);
          #include <colorspace_fragment>
        }
      `,
      depthTest: false,
      depthWrite: false,
    });
  }
}
