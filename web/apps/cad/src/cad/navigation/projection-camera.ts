import * as THREE from "three";
import type { CadCamera } from "./camera-rig";

export type CameraProjection = "PERSPECTIVE" | "ORTHOGRAPHIC";
export const DEFAULT_PERSPECTIVE_FOV = 35;
export const projectionOf = (camera: CadCamera): CameraProjection => camera instanceof THREE.PerspectiveCamera ? "PERSPECTIVE" : "ORTHOGRAPHIC";

/** Preserve the visible scale at the screen centre and the camera orientation. */
export function convertProjection(camera: CadCamera, target: THREE.Vector3, projection: CameraProjection, aspect: number): CadCamera {
  if (projectionOf(camera) === projection) return camera;
  const direction = camera.getWorldDirection(new THREE.Vector3());
  const depth = Math.max(1e-9, target.clone().sub(camera.position).dot(direction));
  const span = camera instanceof THREE.OrthographicCamera ? (camera.top-camera.bottom)/camera.zoom
    : 2*depth*Math.tan(THREE.MathUtils.degToRad(camera.fov/2))/camera.zoom;
  const next: CadCamera = projection === "PERSPECTIVE" ? new THREE.PerspectiveCamera(DEFAULT_PERSPECTIVE_FOV,aspect,camera.near,camera.far)
    : new THREE.OrthographicCamera(-150*aspect,150*aspect,150,-150,camera.near,camera.far);
  next.position.copy(camera.position);next.quaternion.copy(camera.quaternion);next.up.copy(camera.up);next.layers.mask=camera.layers.mask;
  if(next instanceof THREE.PerspectiveCamera){
    const distance=span/(2*Math.tan(THREE.MathUtils.degToRad(next.fov/2)));
    next.position.addScaledVector(direction,depth-distance);
  } else next.zoom=300/span;
  resizeCamera(next,aspect);return next;
}
export function resizeCamera(camera: CadCamera, aspect: number): void {
  if(camera instanceof THREE.PerspectiveCamera)camera.aspect=aspect;
  else {const half=(camera.top-camera.bottom)/2;camera.left=-half*aspect;camera.right=half*aspect;}
  camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
}

export function fitPerspectiveView(camera: THREE.PerspectiveCamera,target:THREE.Vector3,bounds:THREE.Box3):void {
  if(bounds.isEmpty())return;
  const center=bounds.getCenter(new THREE.Vector3()),inverse=camera.quaternion.clone().invert();
  const tanY=Math.tan(THREE.MathUtils.degToRad(camera.fov/2))/camera.zoom,tanX=tanY*camera.aspect;
  let distance=1e-6;
  for(let i=0;i<8;i++){
    const p=new THREE.Vector3(i&1?bounds.max.x:bounds.min.x,i&2?bounds.max.y:bounds.min.y,i&4?bounds.max.z:bounds.min.z).sub(center).applyQuaternion(inverse);
    distance=Math.max(distance,p.z+Math.abs(p.x)*1.15/tanX,p.z+Math.abs(p.y)*1.15/tanY);
  }
  camera.position.copy(center).addScaledVector(camera.getWorldDirection(new THREE.Vector3()),-distance);target.copy(center);
  camera.updateMatrixWorld(true);
}
/** Perspective clipping changes planes only: moving the eye would change the image. */
export function updatePerspectiveClipping(camera:THREE.PerspectiveCamera,target:THREE.Vector3,bounds:THREE.Box3):void {
  const direction=camera.getWorldDirection(new THREE.Vector3());
  const focus=Math.max(1e-6,target.clone().sub(camera.position).dot(direction));
  let maxDepth=focus;
  if(!bounds.isEmpty())for(let i=0;i<8;i++)maxDepth=Math.max(maxDepth,new THREE.Vector3(i&1?bounds.max.x:bounds.min.x,i&2?bounds.max.y:bounds.min.y,i&4?bounds.max.z:bounds.min.z).sub(camera.position).dot(direction));
  camera.near=Math.max(1e-7,Math.min(focus*1e-4,1));
  camera.far=Math.max(camera.near*100,maxDepth*2,focus*20);
  camera.updateProjectionMatrix();camera.updateMatrixWorld(true);
}
