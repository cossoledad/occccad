import * as THREE from "three";
import {palette} from "../../design/visual-tokens";
import type {CurveMarkerKind} from "../interaction/curve-input-feedback";

/** One screen-sized icon per world-space anchor. The viewport disposes its map with the marker group. */
export function curveStatusIcon(kind: Exclude<CurveMarkerKind, "endpoint">): THREE.PointsMaterial {
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = 64;
  const ctx = canvas.getContext("2d")!;
  ctx.lineCap = ctx.lineJoin = "round";
  ctx.fillStyle = kind === "closed" ? palette.success : palette.warning;
  ctx.strokeStyle = palette.surface;
  ctx.lineWidth = 4;
  ctx.beginPath();
  ctx.arc(32, 32, 28, 0, 2 * Math.PI);
  ctx.fill();
  ctx.stroke();
  ctx.lineWidth = 6;
  ctx.beginPath();
  if (kind === "closed") {
    ctx.moveTo(17, 32); ctx.lineTo(27, 42); ctx.lineTo(46, 22);
  } else {
    ctx.moveTo(32, 17); ctx.lineTo(32, 34);
  }
  ctx.stroke();
  if (kind === "warning") {
    ctx.fillStyle = palette.surface;
    ctx.beginPath(); ctx.arc(32, 45, 3.5, 0, 2 * Math.PI); ctx.fill();
  }
  const map = new THREE.CanvasTexture(canvas);
  map.colorSpace = THREE.SRGBColorSpace;
  const material = new THREE.PointsMaterial({map, size:18, sizeAttenuation:false, transparent:true,
    depthTest:false, depthWrite:false, toneMapped:false});
  material.userData.ownedTexture = map;
  return material;
}
