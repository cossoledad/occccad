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
  concentric: 13, point_on_object: 14, midpoint: 15, symmetry: 16,
};

export function constraintSymbolCode(kind: ConstraintKind): number {
  return symbolCodes[constraintDefinition(kind).symbol];
}

export function perspectiveWorldUnitsPerPixel(depth: number, verticalFovDegrees: number, viewportHeight: number): number {
  return 2*Math.abs(depth)*Math.tan(THREE.MathUtils.degToRad(verticalFovDegrees/2))/Math.max(viewportHeight,1);
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
  const screenWidth=124,screenHeight=27,worldPosition=new THREE.Vector3(),viewPosition=new THREE.Vector3();
  sprite.userData.screenSize={width:screenWidth,height:screenHeight};
  sprite.onBeforeRender=(renderer,_scene,camera) => {
    const viewportHeight=Math.max(renderer.domElement.clientHeight,1);
    sprite.getWorldPosition(worldPosition);viewPosition.copy(worldPosition).applyMatrix4(camera.matrixWorldInverse);
    let worldPerPixel=1/viewportHeight;
    if(camera instanceof THREE.PerspectiveCamera)worldPerPixel=perspectiveWorldUnitsPerPixel(viewPosition.z,camera.fov,viewportHeight);
    else if(camera instanceof THREE.OrthographicCamera)worldPerPixel=(camera.top-camera.bottom)/(camera.zoom*viewportHeight);
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
): THREE.Group {
  const layout = buildSketchConstraintLayout(constraint, entities);
  const group = new THREE.Group();
  if (layout.anchors.length) {
    const glyphs = new THREE.Points(new THREE.BufferGeometry().setFromPoints(layout.anchors.map(toWorld)),
      materials.constraintGlyph(symbolCodes[layout.symbol], CATIA_VISUAL_THEME.constraint, 17));
    glyphs.renderOrder = 84; group.add(glyphs);
  }
  if (layout.segments.length) {
    const leaders = makeOcclusionVisibleSegments(layout.segments.map(([first, second]) => [toWorld(first), toWorld(second)]),
      CATIA_VISUAL_THEME.constraint, 1.25);
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
