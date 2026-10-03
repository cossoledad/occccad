import { formatDimensionLabelText } from "../sketch/dimension-value-format";
import { acquireDimensionLabelTextures } from "./dimension-label-textures";
import { colorNumber, palette } from "../../design/visual-tokens";
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

export function makeConstraintDimensionLabel(text: string, toWorld?: (point: Vec2) => THREE.Vector3, direction: Vec2 = [1, 0], color = colorNumber(palette.dimensionText)): THREE.Mesh {
  const textures=acquireDimensionLabelTextures(formatDimensionLabelText(text));
  const material = new THREE.MeshBasicMaterial({ map: textures.glyph, color, transparent: true, depthTest: false, depthWrite: false,
    side: THREE.DoubleSide, toneMapped: false });
  material.userData.baseColor = color;
  material.addEventListener("dispose",textures.release);
  const label = new THREE.Mesh(new THREE.PlaneGeometry(1, 1), material);
  if (toWorld) {
    // Keep the baseline aligned with its leader (or angular arc tangent),
    // choosing an upright equivalent in sketch coordinates, never camera facing.
    const sign = direction[0] < 0 || direction[0] === 0 && direction[1] < 0 ? -1 : 1;
    const d: Vec2 = [direction[0] * sign, direction[1] * sign];
    const origin = toWorld([0, 0]), u = toWorld(d).sub(origin).normalize(), v = toWorld([-d[1], d[0]]).sub(origin).normalize();
    label.quaternion.setFromRotationMatrix(new THREE.Matrix4().makeBasis(u, v, u.clone().cross(v).normalize()));
  }
  const supportOrientation = label.quaternion.clone();
  const halfTurn = new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0, 0, 1), Math.PI);
  const baseline = new THREE.Vector3(), projectedStart = new THREE.Vector3(), projectedEnd = new THREE.Vector3();
  const screenWidth = textures.width, screenHeight = textures.height, worldPosition = new THREE.Vector3();
  label.userData.screenSize = { width: screenWidth, height: screenHeight };
  label.userData.sketchDimensionLabel = true;
  // Stay in the support plane and parallel to the leader. The only allowed
  // camera-dependent change is a half-turn to keep the text readable.
  registerScreenLineUpdate(label, (camera, width, height) => {
    label.getWorldPosition(worldPosition);
    const worldPerPixel = worldUnitsPerCssPixel(camera, worldPosition, { cssWidth: width, cssHeight: height, devicePixelRatio: 1 });
    if (toWorld) {
      baseline.set(1, 0, 0).applyQuaternion(supportOrientation);
      if (label.parent) baseline.transformDirection(label.parent.matrixWorld);
      projectedStart.copy(worldPosition).project(camera);
      projectedEnd.copy(worldPosition).addScaledVector(baseline, worldPerPixel * 16).project(camera);
      const dx = (projectedEnd.x - projectedStart.x) * width;
      const dy = (projectedEnd.y - projectedStart.y) * height;
      const length = Math.hypot(dx, dy);
      // Near vertical, prefer reading upwards; edge-on views keep the baseline.
      const flip = Number.isFinite(length) && length > 1e-9
        && (Math.abs(dx) <= length * 1e-6 ? dy < 0 : dx < 0);
      label.quaternion.copy(supportOrientation);
      if (flip) label.quaternion.multiply(halfTurn);
    }
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
    const label = makeConstraintDimensionLabel(layout.label.text, toWorld, layout.label.direction, diagnosticColor ?? colorNumber(constraint.reference?palette.dimensionReference:palette.dimensionText)); (label.material as THREE.MeshBasicMaterial).userData.dimensionDiagnosticColor=diagnosticColor; label.position.copy(toWorld(layout.label.position)); label.renderOrder = SKETCH_FEEDBACK_ORDER.label;
    group.add(label);
  }
  return group;
}
