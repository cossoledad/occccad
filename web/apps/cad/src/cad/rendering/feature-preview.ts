import { makeFeatureEdges } from "./feature-edges";
import * as THREE from "three";
import { colorNumber, palette } from "../../design/visual-tokens";

export type FeaturePreviewOperation = "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
export const featurePreviewColors = {
  NEW_BODY: palette.previewNew, ADD: palette.previewAdd,
  REMOVE: palette.previewRemove, INTERSECT: palette.previewIntersect,
};

/** The result owns the shaded surface; the borrowed original is outline-only.
 * Two independently tessellated copies of unchanged faces must not compete for
 * the depth buffer. No inferred Boolean topology or changes to exact geometry. */
export function makeFeaturePreview(result: THREE.BufferGeometry, operation: FeaturePreviewOperation,
  original?: THREE.BufferGeometry): THREE.Group {
  const group = new THREE.Group();
  const accent = new THREE.Color(featurePreviewColors[operation]);
  const surface = new THREE.Mesh(result, new THREE.MeshPhongMaterial({
    color: new THREE.Color(palette.solid).lerp(accent, 0.28),
    specular: 0x343c47, shininess: 22, side: THREE.DoubleSide,
    polygonOffset: true, polygonOffsetFactor: 1, polygonOffsetUnits: 1,
  }));
  const outline = new THREE.LineSegments(makeFeatureEdges(result),
    new THREE.LineBasicMaterial({ color: accent.clone().multiplyScalar(0.48), depthWrite: false }));
  surface.name = "feature-preview-result"; outline.name = "feature-preview-result-edges";
  surface.renderOrder = 30; outline.renderOrder = 31;
  group.add(surface, outline);
  if (original) {
    const bounds = original.boundingBox ?? (original.computeBoundingBox(), original.boundingBox!);
    const dash = Math.max(bounds.getSize(new THREE.Vector3()).length() / 90, 0.001);
    const reference = new THREE.LineSegments(makeFeatureEdges(original),
      new THREE.LineDashedMaterial({ color: colorNumber(palette.previewReference), transparent: true,
        opacity: 0.55, depthWrite: false, dashSize: dash, gapSize: dash * 0.7 }));
    reference.computeLineDistances();
    reference.name = "feature-preview-original-edges";
    reference.renderOrder = 33;
    group.add(reference);
  }
  return group;
}

/** Keep authorized upstream objects raycastable without drawing their mesh or
 * edges over the candidate. Visibility refreshes may change Object3D.visible,
 * so the suppression belongs to temporary materials, never model visibility. */
export function hideFeaturePreviewInput(root: THREE.Object3D): () => void {
  const inputs: Array<{object: THREE.Mesh; original: THREE.Material | THREE.Material[]; temporary: THREE.Material[]}> = [];
  root.traverse(child => {
    const object = child as THREE.Mesh;
    if (!object.material) return;
    const original = object.material;
    const temporary = (Array.isArray(original) ? original : [original]).map(material => {
      const copy = material.clone(); copy.visible = false; return copy;
    });
    object.material = Array.isArray(original) ? temporary : temporary[0];
    inputs.push({object, original, temporary});
  });
  return () => {
    for (const {object, original, temporary} of inputs) {
      object.material = original;
      for (const material of temporary) material.dispose();
    }
    inputs.length = 0;
  };
}
