import * as THREE from 'three';
import type {DocumentView} from '../../types';
import type {TransformTransitionSystem, TransformTarget} from '../animation/transform-transition';
import type {MotionPlayback} from './motion-study';

import {motionPlaybackTime,validMotionPose,type MotionPose,type MotionStudy,type MotionViewpoint} from './motion-study';
export {validMotionPose} from './motion-study';

export function motionViewpointAt(study:MotionStudy,time:number):MotionViewpoint|undefined {
  if(!study.animateViewpoint)return;
  const keys=study.keyframes?.filter(k=>k.viewpoint)??[];
  if(!keys.length)return;
  const index=keys.findIndex(k=>k.timeSeconds>time);
  const first=keys[index<0?keys.length-1:Math.max(0,index-1)],second=index<0?first:keys[index];
  const a=first.viewpoint!,b=second.viewpoint!,t=Math.max(0,Math.min(1,second.timeSeconds>first.timeSeconds?(time-first.timeSeconds)/(second.timeSeconds-first.timeSeconds):0));
  const vector=(from:[number,number,number],to:[number,number,number])=>new THREE.Vector3(...from).lerp(new THREE.Vector3(...to),t).toArray();
  const up=new THREE.Vector3(...a.up).lerp(new THREE.Vector3(...b.up),t);if(up.lengthSq()<1e-12)up.fromArray(a.up);up.normalize();
  return {position:vector(a.position,b.position),target:vector(a.target,b.target),up:up.toArray(),rotation:new THREE.Quaternion(...a.rotation).slerp(new THREE.Quaternion(...b.rotation),t).normalize().toArray(),zoom:a.zoom*(1-t)+b.zoom*t,top:a.top*(1-t)+b.top*t,bottom:a.bottom*(1-t)+b.bottom*t};
}

// Rigid screw interpolation preserves an off-origin revolute pivot. Plain
// position lerp would cut across the circular path. These poses never leave the viewport.
export function interpolateMotionPose(first:MotionPose,second:MotionPose,t:number):MotionPose {
  const p0=new THREE.Vector3(...first.translation),p1=new THREE.Vector3(...second.translation);
  const q0=new THREE.Quaternion(...first.rotation),q1=new THREE.Quaternion(...second.rotation);
  const relative=q1.clone().multiply(q0.clone().invert()).normalize();
  if(relative.w<0)relative.set(-relative.x,-relative.y,-relative.z,-relative.w);
  const sine=Math.hypot(relative.x,relative.y,relative.z),angle=2*Math.atan2(sine,relative.w);
  const rotation=q0.clone().slerp(q1,t).normalize();
  let position:THREE.Vector3;
  if(sine<1e-8)position=p0.lerp(p1,t);
  else {
    const axis=new THREE.Vector3(relative.x,relative.y,relative.z).divideScalar(sine);
    const displacement=p1.clone().sub(p0.clone().applyQuaternion(relative));
    const axial=axis.dot(displacement),radial=displacement.addScaledVector(axis,-axial);
    const center=radial.clone().addScaledVector(axis.clone().cross(radial),relative.w/sine).multiplyScalar(.5);
    const step=new THREE.Quaternion().setFromAxisAngle(axis,angle*t);
    position=p0.sub(center).applyQuaternion(step).add(center).addScaledVector(axis,axial*t);
  }
  return {translation:position.toArray() as MotionPose['translation'],rotation:rotation.toArray() as MotionPose['rotation']};
}

// Resolve the entire frozen frame before writing any scene object. Hydration may
// not have created all owning-unit groups yet; defer instead of showing half a loop.
export function applyMotionFrame(
  display: MotionPlayback, view: DocumentView | undefined,
  groups: ReadonlyMap<string, THREE.Object3D>, transforms: TransformTransitionSystem, now=performance.now(),
): boolean {
  if (view?.document.id !== display.view.document.id || view.document.versionId !== display.revisionId
    || display.view.document.versionId !== display.revisionId || !view.product) return false;
  let first=display.frame,second=first,progress=0;
  if(display.clock||display.timeSeconds!==undefined){
    const frames=display.clock?.frames??display.samples??[display.frame],time=display.clock?motionPlaybackTime(display.clock,now):display.timeSeconds!;
    const next=frames.findIndex(f=>f.timeSeconds>time);
    first=frames[next<0?frames.length-1:Math.max(0,next-1)];second=next<0?first:frames[next];
    if(!first?.kinematicValid||!second?.kinematicValid)return false;
    progress=second.timeSeconds>first.timeSeconds?(time-first.timeSeconds)/(second.timeSeconds-first.timeSeconds):0;
  }
  if(!first.kinematicValid||!second.kinematicValid)return false;
  const targets: TransformTarget[] = [];
  for (const unit of view.product.instances) {
    const object = groups.get(unit.id), from = first.unitPoses?.[unit.id],to=second.unitPoses?.[unit.id];
    if (!object || !validMotionPose(from)||!validMotionPose(to)) return false;
    const pose=progress>0?interpolateMotionPose(from,to,progress):from;
    if(!validMotionPose(pose))return false;
    targets.push({object, target: {position: new THREE.Vector3(...pose.translation),
      rotation: new THREE.Quaternion(...pose.rotation), scale: new THREE.Vector3(1, 1, 1)}});
  }
  transforms.stopAll();
  transforms.applyBatch(targets, 'immediate');
  return true;
}
