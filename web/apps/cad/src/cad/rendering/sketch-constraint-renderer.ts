import { SKETCH_FEEDBACK_ORDER } from "./sketch-feedback-style";
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

export function makeConstraintDimensionLabel(text: string, toWorld?: (point: Vec2) => THREE.Vector3): THREE.Mesh {
  const canvas = document.createElement("canvas");
  canvas.width = 256; canvas.height = 48;
  const context = canvas.getContext("2d")!;
  context.font = "500 28px system-ui, sans-serif";
  canvas.width = Math.ceil(context.measureText(text).width) + 16;
  context.font = "500 28px system-ui, sans-serif";
  context.textAlign = "center"; context.textBaseline = "middle";
  context.fillStyle = "#175e40"; context.fillText(text, canvas.width / 2, 24);
  const texture = new THREE.CanvasTexture(canvas); texture.colorSpace = THREE.SRGBColorSpace;
  const material = new THREE.MeshBasicMaterial({ map: texture, transparent: true, depthTest: false, depthWrite: false,
    side: THREE.DoubleSide, toneMapped: false });
  material.userData.baseColor = 0xffffff; material.userData.ownedTexture = texture;
  const label = new THREE.Mesh(new THREE.PlaneGeometry(1, 1), material);
  if (toWorld) {
    const origin = toWorld([0, 0]), u = toWorld([1, 0]).sub(origin).normalize(), v = toWorld([0, 1]).sub(origin).normalize();
    label.quaternion.setFromRotationMatrix(new THREE.Matrix4().makeBasis(u, v, u.clone().cross(v).normalize()));
  }
  const screenWidth = canvas.width / 2, screenHeight = 24, worldPosition = new THREE.Vector3();
  label.userData.screenSize = { width: screenWidth, height: screenHeight };
  label.userData.sketchDimensionLabel = true;
  // Update before rendering AND picking. Orientation belongs to the support
  // plane; only scale follows zoom, so orbiting never billboards the text.
  registerScreenLineUpdate(label, (camera, width, height) => {
    label.getWorldPosition(worldPosition);
    const worldPerPixel = worldUnitsPerCssPixel(camera, worldPosition, { cssWidth: width, cssHeight: height, devicePixelRatio: 1 });
    label.scale.set(screenWidth * worldPerPixel, screenHeight * worldPerPixel, 1);
    label.updateMatrix(); label.updateMatrixWorld(true);
  });
  return label;
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
  if (layout.anchors.length&&!layout.label) {
    const glyphs = new THREE.Points(new THREE.BufferGeometry().setFromPoints(layout.anchors.map(toWorld)),
      materials.constraintGlyph(symbolCodes[layout.symbol], diagnosticColor ?? CATIA_VISUAL_THEME.constraint, 17));
    glyphs.renderOrder = SKETCH_FEEDBACK_ORDER.glyph; group.add(glyphs);
  }
  if (layout.segments.length) {
    const leaders = makeOcclusionVisibleSegments(layout.segments.filter(segment => !segment.screenAnchor).map(([first, second]) => [toWorld(first), toWorld(second)]),
      diagnosticColor ?? CATIA_VISUAL_THEME.constraint, 1.25);
    leaders.renderOrder = SKETCH_FEEDBACK_ORDER.leader;
    updateHighlightLineResolution(leaders, viewport.width, viewport.height);
    group.add(leaders);
  }
  for (const segment of layout.segments.filter(segment => segment.screenAnchor)) {
    const anchor = toWorld(segment[0]), toward = toWorld(segment[1]);
    const arrow = makeOcclusionVisibleSegments([[anchor, toward]], diagnosticColor ?? CATIA_VISUAL_THEME.constraint, 1.25);
    arrow.frustumCulled = false;
    arrow.renderOrder = SKETCH_FEEDBACK_ORDER.leader;
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
    const label = makeConstraintDimensionLabel(layout.label.text, toWorld); label.position.copy(toWorld(layout.label.position)); label.renderOrder = SKETCH_FEEDBACK_ORDER.label;
    group.add(label);
  }
  return group;
}
