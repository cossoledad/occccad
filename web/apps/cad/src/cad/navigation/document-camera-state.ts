import * as THREE from "three";
import type { CadCamera } from "./camera-rig";
import type { CameraProjection } from "./projection-camera";
import { registerDocumentState } from "../document/document-session";
import { restoreView, type SavedView } from "./orthographic-view";

type CameraPose = {
  position: [number, number, number]; rotation: [number, number, number, number]; up: [number, number, number];
  target: [number, number, number]; zoom: number; projection?:CameraProjection; fov?:number; verticalSpan?:number;
};
type CameraState = CameraPose & { top: number; bottom: number; sketchReturn?: CameraPose };
export const documentCameraState = registerDocumentState<CameraState>("viewport.camera");
export function captureDocumentCamera(camera: CadCamera, target: THREE.Vector3, sketchReturn?: SavedView): CameraState {
  return { position: camera.position.toArray(), rotation: camera.quaternion.toArray(), up: camera.up.toArray(),
    target: target.toArray(), zoom: camera.zoom, top: camera instanceof THREE.OrthographicCamera?camera.top:150, bottom: camera instanceof THREE.OrthographicCamera?camera.bottom:-150, projection:camera instanceof THREE.PerspectiveCamera?"PERSPECTIVE":"ORTHOGRAPHIC",fov:camera instanceof THREE.PerspectiveCamera?camera.fov:undefined,
    sketchReturn: sketchReturn && { position: sketchReturn.position.toArray(), rotation: sketchReturn.rotation.toArray(),
      up: sketchReturn.up.toArray(), target: sketchReturn.target.toArray(), zoom: sketchReturn.zoom,projection:sketchReturn.projection,fov:sketchReturn.fov,verticalSpan:sketchReturn.verticalSpan } };
}
export function documentCameraPose(state: CameraPose): SavedView {
  return { position: new THREE.Vector3().fromArray(state.position), rotation: new THREE.Quaternion().fromArray(state.rotation),
    up: new THREE.Vector3().fromArray(state.up), target: new THREE.Vector3().fromArray(state.target), zoom: state.zoom,projection:state.projection??"ORTHOGRAPHIC",fov:state.fov,verticalSpan:state.verticalSpan??300 };
}
export function restoreDocumentCamera(camera: CadCamera, target: THREE.Vector3, state: CameraState, aspect: number): void {
  if(camera instanceof THREE.OrthographicCamera){camera.top = state.top; camera.bottom = state.bottom;
  const halfWidth = (state.top - state.bottom) * .5 * aspect;
  camera.left = -halfWidth; camera.right = halfWidth;}
  restoreView(camera, target, documentCameraPose({...state,verticalSpan:state.top-state.bottom}));
}
