import * as THREE from 'three';
import type {DocumentView} from '../../types';
import type {TransformTransitionSystem, TransformTarget} from '../animation/transform-transition';
import type {MotionPlayback, MotionPose} from './motion-study';

export function validMotionPose(pose: MotionPose): boolean {
  return pose.translation.length === 3 && pose.rotation.length === 4
    && [...pose.translation, ...pose.rotation].every(Number.isFinite)
    && Math.abs(pose.rotation.reduce((sum, v) => sum + v * v, 0) - 1) < 1e-8;
}

// Resolve the entire frozen frame before writing any scene object. Hydration may
// not have created all owning-unit groups yet; defer instead of showing half a loop.
export function applyMotionFrame(
  display: MotionPlayback, view: DocumentView | undefined,
  groups: ReadonlyMap<string, THREE.Object3D>, transforms: TransformTransitionSystem,
): boolean {
  if (view?.document.id !== display.view.document.id || view.document.versionId !== display.revisionId
    || display.view.document.versionId !== display.revisionId || !view.product) return false;
  const targets: TransformTarget[] = [];
  for (const unit of view.product.instances) {
    const object = groups.get(unit.id), pose = display.frame.unitPoses[unit.id];
    if (!object || !pose || !validMotionPose(pose)) return false;
    targets.push({object, target: {position: new THREE.Vector3(...pose.translation),
      rotation: new THREE.Quaternion(...pose.rotation), scale: new THREE.Vector3(1, 1, 1)}});
  }
  transforms.stopAll();
  transforms.applyBatch(targets, 'immediate');
  return true;
}
