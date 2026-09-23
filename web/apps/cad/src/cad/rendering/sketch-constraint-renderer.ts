import { registerScreenLineUpdate, screenSpaceEndpoint } from "./screen-space-lines";
import * as THREE from "three";
import type { SketchConstraint, SketchEntity, Vec2 } from "../../types";
import { buildSketchConstraintLayout } from "../sketch/sketch-constraint-layout";
import { constraintDefinition, type ConstraintKind, type ConstraintSymbol } from "../sketch/sketch-constraint-definition";
import type { CadMaterialFactory } from "./cad-material-factory";
import { CATIA_VISUAL_THEME } from "./cad-visual-theme";
import { makeOcclusionVisibleSegments, updateHighlightLineResolution } from "./interaction-highlight";
import { perspectiveWorldUnitsPerCssPixel, viewportMetrics, worldUnitsPerCssPixel } from "./viewport-metrics";

export const perspectiveWorldUnitsPerPixel = perspectiveWorldUnitsPerCssPixel;

const symbolCodes: Record<ConstraintSymbol, number> = {
  coincident: 0, parallel: 1, fixed: 2, horizontal: 3, vertical: 4, perpendicular: 5,
  tangent: 6, equal: 7, distance: 8, length: 9, radius: 10, diameter: 11, angle: 12,
  concentric: 13, point_on_object: 14, midpoint: 15, symmetry: 16,
};

export function constraintSymbolCode(kind: ConstraintKind): number {
  return symbolCodes[constraintDefinition(kind).symbol];
}

export function makeConstraintDimensionLabel(text: string): THREE.Sprite {
  const canvas = document.createElement("canvas");
  canvas.width = 256; canvas.height = 56;
  const context = canvas.getContext("2d")!;
  context.fillStyle = "rgba(17, 31, 39, 0.76)";
  context.beginPath(); context.roundRect(5, 5, 246, 46, 9); context.fill();
  context.strokeStyle = "rgba(114, 221, 160, 0.72)"; context.lineWidth = 2; context.stroke();
  context.fillStyle = "#effff5"; context.font = "500 27px system-ui, sans-serif";
  context.textAlign = "center"; context.textBaseline = "middle"; context.fillText(text, 128, 29);
  const texture = new THREE.CanvasTexture(canvas); texture.colorSpace = THREE.SRGBColorSpace;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthTest: false, depthWrite: false,
    toneMapped: false });
  material.userData.baseColor = 0xffffff;
  material.userData.ownedTexture = texture;
  const sprite = new THREE.Sprite(material);
  const screenWidth=124,screenHeight=27,worldPosition=new THREE.Vector3();
  sprite.userData.screenSize={width:screenWidth,height:screenHeight};
  sprite.onBeforeRender=(renderer,_scene,camera) => {
    sprite.getWorldPosition(worldPosition);
    const worldPerPixel=worldUnitsPerCssPixel(camera,worldPosition,viewportMetrics(renderer));
    sprite.scale.set(screenWidth*worldPerPixel,screenHeight*worldPerPixel,1);
    sprite.updateMatrix();sprite.updateMatrixWorld(true);
  };
  return sprite;
}

export function makeSketchConstraintRenderable(
  constraint: SketchConstraint,
  entities: readonly SketchEntity[],
  toWorld: (point: Vec2) => THREE.Vector3,
  materials: CadMaterialFactory,
  viewport: { width: number; height: number },
  diagnosticColor?: number,
): THREE.Group {
  const layout = buildSketchConstraintLayout(constraint, entities);
  const group = new THREE.Group();
  if (layout.anchors.length) {
    const glyphs = new THREE.Points(new THREE.BufferGeometry().setFromPoints(layout.anchors.map(toWorld)),
      materials.constraintGlyph(symbolCodes[layout.symbol], diagnosticColor ?? CATIA_VISUAL_THEME.constraint, 17));
    glyphs.renderOrder = 84; group.add(glyphs);
  }
  if (layout.segments.length) {
    const leaders = makeOcclusionVisibleSegments(layout.segments.filter(segment => !segment.screenAnchor).map(([first, second]) => [toWorld(first), toWorld(second)]),
      diagnosticColor ?? CATIA_VISUAL_THEME.constraint, 1.25);
    leaders.renderOrder = 82;
    updateHighlightLineResolution(leaders, viewport.width, viewport.height);
    group.add(leaders);
  }
  for (const segment of layout.segments.filter(segment => segment.screenAnchor)) {
    const anchor = toWorld(segment[0]), toward = toWorld(segment[1]);
    const arrow = makeOcclusionVisibleSegments([[anchor, toward]], diagnosticColor ?? CATIA_VISUAL_THEME.constraint, 1.25);
    arrow.frustumCulled = false;
    arrow.renderOrder = 83;
    registerScreenLineUpdate(arrow, (camera, cssWidth, cssHeight) => {
      arrow.material.resolution.set(cssWidth, cssHeight);
      const worldAnchor = anchor.clone().applyMatrix4(arrow.matrixWorld);
      const worldToward = toward.clone().applyMatrix4(arrow.matrixWorld);
      const end = arrow.worldToLocal(screenSpaceEndpoint(worldAnchor, worldToward, camera, cssWidth, cssHeight, 9));
      const ends = arrow.geometry.getAttribute("instanceEnd");
      ends.setXYZ(0, end.x, end.y, end.z); ends.needsUpdate = true;
      arrow.geometry.computeBoundingBox(); arrow.geometry.computeBoundingSphere();
    });
    updateHighlightLineResolution(arrow, viewport.width, viewport.height);
    group.add(arrow);
  }
  if (layout.label) {
    const label = makeConstraintDimensionLabel(layout.label.text); label.position.copy(toWorld(layout.label.position)); label.renderOrder = 86;
    group.add(label);
  }
  return group;
}
