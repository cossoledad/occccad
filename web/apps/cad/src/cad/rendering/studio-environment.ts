import * as THREE from "three";

/** Reflection cards in a Z-up studio. Linear HDR radiance is intentional:
 * PMREM turns their finite area into roughness-dependent highlight bands. */
export function makeStudioEnvironmentScene(): THREE.Scene {
  const scene = new THREE.Scene();
  scene.background = new THREE.Color().setRGB(0.18, 0.19, 0.20);
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
  // Broad, lower-contrast cards keep coarse curved meshes from reflecting sharp strips.
  card("studio-key", 11, 9, new THREE.Vector3(-4, -5, 7), [2.0, 2.0, 2.0]);
  card("studio-side", 7, 9, new THREE.Vector3(6, 2, 3), [1.45, 1.48, 1.52]);
  card("studio-top", 7, 5, new THREE.Vector3(1, 4, 8), [1.7, 1.72, 1.75]);
  card("studio-fill", 7, 6, new THREE.Vector3(-5, 4, 1), [0.8, 0.82, 0.85]);
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
