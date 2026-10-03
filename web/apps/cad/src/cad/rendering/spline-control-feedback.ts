import * as THREE from "three";
import type { Vec2 } from "../../types";
import type { CadMaterialFactory } from "./cad-material-factory";
import { makeSketchOverlayLine, updateHighlightLineResolution } from "./interaction-highlight";
import { SPLINE_CONTROL_FEEDBACK, SKETCH_FEEDBACK_ORDER } from "./sketch-feedback-style";

/** Display only: fit points remain fit points; control poles show their control polygon. */
export function makeSplineControlFeedback(points: readonly Vec2[], mode: "FIT" | "CONTROL",
  project: (point: Vec2) => THREE.Vector3, materials: CadMaterialFactory,
  resolution: { width: number; height: number }, color: number,
  order: number = SKETCH_FEEDBACK_ORDER.controlPoint): THREE.Group {
  const group = new THREE.Group();
  group.userData.splineControlFeedback = true;
  const positions = points.map(project);
  if (mode === "CONTROL" && positions.length > 1) {
    const polygon = makeSketchOverlayLine(positions, color, 1, true);
    updateHighlightLineResolution(polygon, resolution.width, resolution.height);
    polygon.renderOrder = order - 2;
    group.add(polygon);
  }
  if (positions.length) {
    const halo = new THREE.Points(new THREE.BufferGeometry().setFromPoints(positions),
      materials.point(0x152333, SPLINE_CONTROL_FEEDBACK.haloSize, false));
    const markers = new THREE.Points(new THREE.BufferGeometry().setFromPoints(positions),
      materials.point(color, SPLINE_CONTROL_FEEDBACK.size, false));
    halo.renderOrder = order - 1;
    markers.renderOrder = order;
    group.add(halo, markers);
  }
  return group;
}
