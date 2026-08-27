import * as THREE from "three";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";
import { CadShaderLibrary } from "./shader/cad-shader-library";
import { LineMaterial } from "three/addons/lines/LineMaterial.js";

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

  point(color: number = this.theme.edge, size = 5, depthTest = true): THREE.ShaderMaterial {
    const material = this.withSharedCadUniforms(this.shaders.createMaterial("cad.point", {
      uColor: new THREE.Color(color), uSelectedColor: new THREE.Color(this.theme.selected), uPointSize: size,
    }));
    material.depthTest = depthTest;
    if (!depthTest) material.depthWrite = false;
    return material;
  }

  constraintGlyph(glyph: number, color = this.theme.constraint, size = 17): THREE.ShaderMaterial {
    return this.shaders.createMaterial("cad.constraint.glyph", {
      uColor: new THREE.Color(color), uSelectedColor: new THREE.Color(this.theme.selected), uGlyph: glyph, uPointSize: size,
    });
  }

  setSectionPlane(enabled: boolean, plane?: THREE.Plane): void {
    this.sectionEnabled.value = enabled;
    if (plane) this.sectionPlane.value.set(plane.normal.x, plane.normal.y, plane.normal.z, plane.constant);
  }

  setSelected(object: THREE.Object3D, selected: boolean): void {
    this.setInteractionState(object, selected ? "selected" : "default");
  }

  setInteractionState(object: THREE.Object3D, state: "default" | "hover" | "selected"): void {
    object.traverse((child) => {
      const renderable = child as THREE.Mesh | THREE.LineSegments;
      if (!("material" in renderable) || !renderable.material) return;
      const materials = Array.isArray(renderable.material) ? renderable.material : [renderable.material];
      for (const material of materials) {
        if (material instanceof THREE.MeshPhongMaterial && material.userData.cadMaterial === "surface") {
          const baseColor = Number(material.userData.baseColor ?? this.theme.surface);
          material.color.setHex(state === "selected" ? this.theme.selected : state === "hover" ? this.theme.hover : baseColor);
          material.emissive.setHex(state === "selected" ? this.theme.selectedEmissive : 0x000000);
          material.emissiveIntensity = state === "selected" ? 0.32 : state === "hover" ? 0.12 : 0;
        } else if (material instanceof LineMaterial) {
          const baseColor = Number(material.userData.baseColor ?? material.color.getHex());
          material.userData.baseColor = baseColor;
          material.color.setHex(state === "selected" ? this.theme.selected : state === "hover" ? this.theme.hover : baseColor);
        } else if (material instanceof THREE.ShaderMaterial) {
          const selectedUniform = material.uniforms.uSelected;
          const selectedColor = material.uniforms.uSelectedColor;
          if (selectedColor) selectedColor.value.setHex(state === "hover" ? this.theme.hover : this.theme.selected);
          if (selectedUniform) selectedUniform.value = state === "default" ? 0 : 1;
        } else if (material && "color" in material && material.color instanceof THREE.Color) {
          if (material.userData.baseColor === undefined) material.userData.baseColor = material.color.getHex();
          material.color.setHex(state === "selected" ? this.theme.selected : state === "hover" ? this.theme.hover : Number(material.userData.baseColor));
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
