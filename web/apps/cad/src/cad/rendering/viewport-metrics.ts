import * as THREE from "three";

export type ViewportMetrics = { cssWidth: number; cssHeight: number; devicePixelRatio: number };

export function viewportMetrics(renderer: THREE.WebGLRenderer): ViewportMetrics {
  return { cssWidth: Math.max(renderer.domElement.clientWidth, 1), cssHeight: Math.max(renderer.domElement.clientHeight, 1),
    devicePixelRatio: renderer.getPixelRatio() };
}

export function worldUnitsPerCssPixel(camera: THREE.Camera, worldPosition: THREE.Vector3, metrics: ViewportMetrics): number {
  if (camera instanceof THREE.PerspectiveCamera) {
    const view = worldPosition.clone().applyMatrix4(camera.matrixWorldInverse);
    return perspectiveWorldUnitsPerCssPixel(view.z, camera.fov, metrics.cssHeight);
  }
  if (camera instanceof THREE.OrthographicCamera) return (camera.top - camera.bottom) / (camera.zoom * metrics.cssHeight);
  return 1 / metrics.cssHeight;
}

export const perspectiveWorldUnitsPerCssPixel = (depth: number, verticalFovDegrees: number, cssHeight: number): number =>
  2 * Math.abs(depth) * Math.tan(THREE.MathUtils.degToRad(verticalFovDegrees / 2)) / Math.max(cssHeight, 1);

export const cssPixelsToDevicePixels = (cssPixels: number, metrics: ViewportMetrics): number =>
  cssPixels * metrics.devicePixelRatio;

export function axisDragWorldDelta(deltaCssPixels: THREE.Vector2, axisCssPixelsPerWorld: THREE.Vector2): number {
  if (axisCssPixelsPerWorld.lengthSq() < 1.0e-12) return 0;
  // Solve the least-squares screen displacement along the projected world axis. The
  // result changes with camera depth/zoom so the dragged object stays under the pointer.
  return deltaCssPixels.dot(axisCssPixelsPerWorld) / axisCssPixelsPerWorld.lengthSq();
}

export function stableAxisDragWorldDelta(deltaCssPixels: THREE.Vector2, axisCssPixelsPerWorld: THREE.Vector2,
  worldUnitsPerPixel: number): number {
  const projectedLength=axisCssPixelsPerWorld.length();
  const foreshortening=projectedLength*worldUnitsPerPixel;
  if(!Number.isFinite(foreshortening)||foreshortening<0.08||projectedLength<1e-8)return 0;
  const requested=deltaCssPixels.dot(axisCssPixelsPerWorld)/axisCssPixelsPerWorld.lengthSq();
  const limit=Math.max(deltaCssPixels.length()*worldUnitsPerPixel*4,worldUnitsPerPixel);
  return THREE.MathUtils.clamp(requested,-limit,limit);
}

export function stableAngularDragDelta(deltaCssPixels:THREE.Vector2,screenPixelsPerRadian:THREE.Vector2):number{
  const response=screenPixelsPerRadian.length();
  if(!Number.isFinite(response)||response<12)return 0;
  const requested=deltaCssPixels.dot(screenPixelsPerRadian)/screenPixelsPerRadian.lengthSq();
  return THREE.MathUtils.clamp(requested,-Math.PI/6,Math.PI/6);
}

export function solveScreenConstrainedParameter(target:THREE.Vector2,initial:number,
  screenAt:(parameter:number)=>THREE.Vector2,finiteDifferenceStep:number,maxIterations=8,
  maxCorrection=Number.POSITIVE_INFINITY):number{
  let parameter=initial;
  const step=Math.max(Math.abs(finiteDifferenceStep),1e-6);
  for(let iteration=0;iteration<maxIterations;iteration++){
    const current=screenAt(parameter);
    const derivative=screenAt(parameter+step).sub(screenAt(parameter-step)).multiplyScalar(0.5/step);
    const denominator=derivative.lengthSq();
    if(!Number.isFinite(denominator)||denominator<1e-10)break;
    const correction=THREE.MathUtils.clamp(target.clone().sub(current).dot(derivative)/denominator,-maxCorrection,maxCorrection);
    if(!Number.isFinite(correction))break;
    parameter+=correction;
    if(Math.abs(correction)<step*1e-4)break;
  }
  return parameter;
}

export function angularDragDelta(currentAngle: number, startAngle: number): number {
  return Math.atan2(Math.sin(currentAngle - startAngle), Math.cos(currentAngle - startAngle));
}

export function transformAroundWorldPivot(startPosition: THREE.Vector3, startRotation: THREE.Quaternion,
  startPivot: THREE.Vector3, desiredPivot: THREE.Vector3, deltaRotation: THREE.Quaternion):
  { position: THREE.Vector3; rotation: THREE.Quaternion } {
  return {
    position: desiredPivot.clone().add(startPosition.clone().sub(startPivot).applyQuaternion(deltaRotation)),
    rotation: deltaRotation.clone().multiply(startRotation),
  };
}

export function manipulatorFrame(direction:THREE.Vector3,kind:"line"|"plane"):THREE.Quaternion{
  const primary=direction.clone().normalize();
  const reference=[new THREE.Vector3(0,0,1),new THREE.Vector3(0,1,0),new THREE.Vector3(1,0,0)]
    .find((axis)=>Math.abs(axis.dot(primary))<0.9)!;
  let x:THREE.Vector3,y:THREE.Vector3,z:THREE.Vector3;
  if(kind==="plane"){
    z=primary;x=reference.clone().addScaledVector(z,-reference.dot(z)).normalize();y=z.clone().cross(x).normalize();
  }else{
    x=primary;z=reference.clone().addScaledVector(x,-reference.dot(x)).normalize();y=z.clone().cross(x).normalize();
    z=x.clone().cross(y).normalize();
  }
  return new THREE.Quaternion().setFromRotationMatrix(new THREE.Matrix4().makeBasis(x,y,z));
}
