import * as THREE from "three";
import type { SketchConstraint, SketchEntity, Vec2 } from "../../types";
import { buildSketchConstraintLayout } from "../sketch/sketch-constraint-layout";
import { constraintDefinition, type ConstraintKind, type ConstraintSymbol } from "../sketch/sketch-constraint-definition";
import type { CadMaterialFactory } from "./cad-material-factory";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";
import { makeOcclusionVisibleSegments, updateHighlightLineResolution } from "./interaction-highlight";

const symbolCodes: Record<ConstraintSymbol, number> = {
  coincident: 0, parallel: 1, fixed: 2, horizontal: 3, vertical: 4, perpendicular: 5,
  tangent: 6, equal: 7, distance: 8, length: 9, radius: 10, diameter: 11, angle: 12,
  concentric: 13, point_on_object: 14, midpoint: 15,
};

export function constraintSymbolCode(kind: ConstraintKind): number {
  return symbolCodes[constraintDefinition(kind).symbol];
}

export function makeConstraintDimensionLabel(text: string): THREE.Sprite {
  const canvas = document.createElement("canvas");
  canvas.width = 256; canvas.height = 64;
  const context = canvas.getContext("2d")!;
  context.fillStyle = "rgba(12, 29, 39, 0.88)";
  context.beginPath(); context.roundRect(2, 4, 252, 56, 12); context.fill();
  context.strokeStyle = "rgba(114, 221, 160, 0.88)"; context.lineWidth = 3; context.stroke();
  context.fillStyle = "#e8fff1"; context.font = "600 30px system-ui, sans-serif";
  context.textAlign = "center"; context.textBaseline = "middle"; context.fillText(text, 128, 33);
  const texture = new THREE.CanvasTexture(canvas); texture.colorSpace = THREE.SRGBColorSpace;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthTest: false, depthWrite: false,
    toneMapped: false });
  material.userData.baseColor = 0xffffff;
  material.userData.ownedTexture = texture;
  const sprite = new THREE.Sprite(material); sprite.scale.set(18, 4.5, 1);
  return sprite;
}

export function makeSketchConstraintRenderable(
  constraint: SketchConstraint,
  entities: readonly SketchEntity[],
  toWorld: (point: Vec2) => THREE.Vector3,
  materials: CadMaterialFactory,
  viewport: { width: number; height: number },
): THREE.Group {
  const layout = buildSketchConstraintLayout(constraint, entities);
  const group = new THREE.Group();
  if (layout.anchors.length) {
    const glyphs = new THREE.Points(new THREE.BufferGeometry().setFromPoints(layout.anchors.map(toWorld)),
      materials.constraintGlyph(symbolCodes[layout.symbol]));
    glyphs.renderOrder = 84; group.add(glyphs);
  }
  if (layout.segments.length) {
    const leaders = makeOcclusionVisibleSegments(layout.segments.map(([first, second]) => [toWorld(first), toWorld(second)]),
      CATIA_VISUAL_THEME.constraint, 2.5);
    leaders.renderOrder = 82;
    updateHighlightLineResolution(leaders, viewport.width, viewport.height);
    group.add(leaders);
  }
  if (layout.label) {
    const label = makeConstraintDimensionLabel(layout.label.text); label.position.copy(toWorld(layout.label.position)); label.renderOrder = 86;
    group.add(label);
  }
  return group;
}
