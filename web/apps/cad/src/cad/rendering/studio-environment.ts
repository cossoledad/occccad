import * as THREE from "three";

/** Reflection cards in a Z-up studio. Linear HDR radiance is intentional:
 * PMREM turns their finite area into roughness-dependent highlight bands. */
export function makeStudioEnvironmentScene(): THREE.Scene {
  const scene = new THREE.Scene();
  scene.background = new THREE.Color().setRGB(0.08, 0.09, 0.11);
  const card = (name: string, width: number, height: number, position: THREE.Vector3,
    radiance: [number, number, number]) => {
    const material = new THREE.MeshBasicMaterial({
      color: new THREE.Color().setRGB(...radiance), side: THREE.DoubleSide, toneMapped: false,
    });
    const mesh = new THREE.Mesh(new THREE.PlaneGeometry(width, height), material);
    mesh.name = name;
    mesh.position.copy(position);
    mesh.up.set(0, 0, 1);
    mesh.lookAt(0, 0, 0);
    scene.add(mesh);
  };
  // Asymmetry and a broad vertical key reveal curvature without a chrome strip.
  // Surface normals, rather than extra display triangles, carry this gradient.
  card("studio-key", 4, 12, new THREE.Vector3(-3, 7, 4), [2.6, 2.65, 2.7]);
  card("studio-side", 6, 8, new THREE.Vector3(6, 1, 3), [0.75, 0.8, 0.85]);
  card("studio-top", 10, 4, new THREE.Vector3(0, 3, 8), [1.0, 1.02, 1.05]);
  card("studio-fill", 6, 8, new THREE.Vector3(-5, -4, 2), [0.3, 0.32, 0.35]);
  scene.updateMatrixWorld(true);
  return scene;
}

/** One baked environment per viewport; never captures model geometry or adds
 * visible objects, shadows, network assets, or per-frame environment passes. */
export function createStudioEnvironment(renderer: THREE.WebGLRenderer): THREE.WebGLRenderTarget {
  const scene = makeStudioEnvironmentScene();
  const generator = new THREE.PMREMGenerator(renderer);
  try {
    return generator.fromScene(scene, 0.03, 0.1, 100, { size: 256 });
  } finally {
    generator.dispose();
    scene.traverse(object => {
      if (object instanceof THREE.Mesh) {
        object.geometry.dispose();
        for (const material of Array.isArray(object.material) ? object.material : [object.material]) material.dispose();
      }
    });
  }
}
