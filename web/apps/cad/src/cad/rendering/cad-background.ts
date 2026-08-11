import * as THREE from "three";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";
import type { CadShaderLibrary } from "./shader/cad-shader-library";

export class CadBackground {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.Camera();
  private readonly geometry = new THREE.PlaneGeometry(2, 2);
  private readonly material: THREE.ShaderMaterial;

  constructor(shaders: CadShaderLibrary) {
    this.material = shaders.createMaterial("cad.background", {
      uTop: new THREE.Color(CATIA_VISUAL_THEME.backgroundTop),
      uBottom: new THREE.Color(CATIA_VISUAL_THEME.backgroundBottom),
      uVignette: CATIA_VISUAL_THEME.backgroundVignette,
    });
    this.scene.add(new THREE.Mesh(this.geometry, this.material));
  }

  render(renderer: THREE.WebGLRenderer): void { renderer.render(this.scene, this.camera); }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
  }
}

