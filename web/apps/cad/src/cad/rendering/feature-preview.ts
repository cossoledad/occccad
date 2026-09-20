import { makeFeatureEdges } from "./feature-edges";
import * as THREE from "three";
import { colorNumber, palette } from "../../design/visual-tokens";

export type FeaturePreviewOperation = "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
export const featurePreviewColors = {
  NEW_BODY: palette.previewNew, ADD: palette.previewAdd,
  REMOVE: palette.previewRemove, INTERSECT: palette.previewIntersect,
};

/** Evaluated result and original-body reference; no inferred Boolean topology. */
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
    const ghost = new THREE.Mesh(original, new THREE.MeshBasicMaterial({
      color: colorNumber(palette.previewReference), transparent: true, opacity: 0.09,
      depthWrite: false, side: THREE.FrontSide, polygonOffset: true, polygonOffsetFactor: 2, polygonOffsetUnits: 2,
    }));
    const bounds = original.boundingBox ?? (original.computeBoundingBox(), original.boundingBox!);
    const dash = Math.max(bounds.getSize(new THREE.Vector3()).length() / 90, 0.001);
    const reference = new THREE.LineSegments(makeFeatureEdges(original),
      new THREE.LineDashedMaterial({ color: colorNumber(palette.previewReference), transparent: true,
        opacity: 0.55, depthWrite: false, dashSize: dash, gapSize: dash * 0.7 }));
    reference.computeLineDistances();
    ghost.name = "feature-preview-original"; reference.name = "feature-preview-original-edges";
    ghost.renderOrder = 32; reference.renderOrder = 33;
    group.add(ghost, reference);
  }
  return group;
}
