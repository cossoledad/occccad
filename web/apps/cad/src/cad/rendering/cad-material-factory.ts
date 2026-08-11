import * as THREE from "three";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";
import { CadShaderLibrary } from "./shader/cad-shader-library";

export class CadMaterialFactory {
  private readonly sectionEnabled: THREE.IUniform<boolean> = { value: false };
  private readonly sectionPlane: THREE.IUniform<THREE.Vector4> = { value: new THREE.Vector4(0, 0, 1, 0) };

  constructor(readonly shaders: CadShaderLibrary, readonly theme = CATIA_VISUAL_THEME) {}

  surface(color: number = this.theme.surface): THREE.MeshPhongMaterial {
    const material = new THREE.MeshPhongMaterial({
      color, specular: 0x76848b, shininess: 46, side: THREE.DoubleSide,
      wireframe: false, depthTest: true, depthWrite: true,
      polygonOffset: true, polygonOffsetFactor: 1, polygonOffsetUnits: 1,
    });
    material.userData.cadMaterial = "surface";
    material.userData.baseColor = color;
    return material;
  }

  edge(color: number = this.theme.edge): THREE.ShaderMaterial {
    return this.withSharedCadUniforms(this.shaders.createMaterial("cad.edge", {
      uColor: new THREE.Color(color),
      uSelectedColor: new THREE.Color(this.theme.selected),
    }));
  }

  point(color: number = this.theme.edge, size = 5): THREE.ShaderMaterial {
    return this.withSharedCadUniforms(this.shaders.createMaterial("cad.point", {
      uColor: new THREE.Color(color), uPointSize: size,
    }));
  }

  setSectionPlane(enabled: boolean, plane?: THREE.Plane): void {
    this.sectionEnabled.value = enabled;
    if (plane) this.sectionPlane.value.set(plane.normal.x, plane.normal.y, plane.normal.z, plane.constant);
  }

  setSelected(object: THREE.Object3D, selected: boolean): void {
    object.traverse((child) => {
      const renderable = child as THREE.Mesh | THREE.LineSegments;
      if (!("material" in renderable) || !renderable.material) return;
      const materials = Array.isArray(renderable.material) ? renderable.material : [renderable.material];
      for (const material of materials) {
        if (material instanceof THREE.MeshPhongMaterial && material.userData.cadMaterial === "surface") {
          const baseColor = Number(material.userData.baseColor ?? this.theme.surface);
          material.color.setHex(selected ? this.theme.selected : baseColor);
          material.emissive.setHex(selected ? 0x3d2100 : 0x000000);
          material.emissiveIntensity = selected ? 0.32 : 0;
        } else if (material instanceof THREE.ShaderMaterial) {
          const selectedUniform = material.uniforms.uSelected;
          if (selectedUniform) selectedUniform.value = selected ? 1 : 0;
        } else if (material && "color" in material && material.color instanceof THREE.Color) {
          if (material.userData.baseColor === undefined) material.userData.baseColor = material.color.getHex();
          material.color.setHex(selected ? this.theme.selected : Number(material.userData.baseColor));
        }
      }
    });
  }

  private withSharedCadUniforms(material: THREE.ShaderMaterial): THREE.ShaderMaterial {
    if (material.uniforms.uSectionEnabled) material.uniforms.uSectionEnabled = this.sectionEnabled;
    if (material.uniforms.uSectionPlane) material.uniforms.uSectionPlane = this.sectionPlane;
    return material;
  }
}
