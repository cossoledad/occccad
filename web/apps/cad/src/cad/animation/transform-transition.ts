import { animate, type AnimationPlaybackControls } from "motion";
import * as THREE from "three";

export type TransformTransitionIntent = "immediate" | "preview" | "settle" | "rollback" | "reconcile";

export type TransformPose = {
  position: THREE.Vector3;
  rotation: THREE.Quaternion;
  scale: THREE.Vector3;
};

type ProgressControls = Pick<AnimationPlaybackControls, "stop" | "complete">;
export type TransformTarget = { object: THREE.Object3D; target: TransformPose; frame?: (object: THREE.Object3D) => void };
type ActiveTransition = { objects: THREE.Object3D[]; controls?: ProgressControls; finish: () => void; stopped: boolean };
export type ProgressDriver = (
  update: (progress: number) => void,
  complete: () => void,
  options: { duration: number; ease: readonly [number, number, number, number] },
) => ProgressControls;

const POLICIES: Record<Exclude<TransformTransitionIntent, "immediate">,
  { duration: number; ease: readonly [number, number, number, number] }> = {
  // Fast initial response avoids input lag; the longer deceleration tail makes
  // solver-driven motion perceptible without overshooting the authoritative pose.
  preview: { duration: 0.12, ease: [0.2, 0.8, 0.3, 1] },
  settle: { duration: 0.2, ease: [0.2, 0.8, 0.3, 1] },
  rollback: { duration: 0.17, ease: [0.2, 0.8, 0.3, 1] },
  reconcile: { duration: 0.24, ease: [0.2, 0.8, 0.3, 1] },
};

const motionProgress: ProgressDriver = (update, complete, options) => animate(0, 1, {
  duration: options.duration,
  ease: [...options.ease],
  onUpdate: update,
  onComplete: complete,
});

export function snapshotTransform(object: THREE.Object3D): TransformPose {
  return { position: object.position.clone(), rotation: object.quaternion.clone().normalize(), scale: object.scale.clone() };
}

export function applyInterpolatedTransform(
  object: THREE.Object3D,
  from: TransformPose,
  to: TransformPose,
  progress: number,
): void {
  const bounded = THREE.MathUtils.clamp(progress, 0, 1);
  object.position.lerpVectors(from.position, to.position, bounded);
  object.quaternion.slerpQuaternions(from.rotation, to.rotation, bounded).normalize();
  object.scale.lerpVectors(from.scale, to.scale, bounded);
  object.updateMatrix();
  object.updateMatrixWorld(true);
}

function samePose(left: TransformPose, right: TransformPose): boolean {
  return left.position.distanceToSquared(right.position) < 1e-12
    && 1 - Math.abs(left.rotation.dot(right.rotation)) < 1e-10
    && left.scale.distanceToSquared(right.scale) < 1e-12;
}

/**
 * The only time-based transform writer in the viewport. Domain/preview state remains
 * authoritative; this class owns only the short-lived rendered pose between updates.
 */
export class TransformTransitionSystem {
  private readonly active = new Map<THREE.Object3D, ActiveTransition>();
  private readonly invalidate: () => void;
  private readonly reducedMotion: boolean;
  private readonly drive: ProgressDriver;

  constructor(
    invalidate: () => void,
    reducedMotion = typeof matchMedia === "function"
      && matchMedia("(prefers-reduced-motion: reduce)").matches,
    drive: ProgressDriver = motionProgress,
  ) {
    this.invalidate = invalidate;
    this.reducedMotion = reducedMotion;
    this.drive = drive;
  }

  apply(object: THREE.Object3D, target: TransformPose, intent: TransformTransitionIntent,
    frame?: (object: THREE.Object3D) => void): void {
    this.applyBatch([{ object, target, frame }], intent);
  }

  applyBatch(targets: readonly TransformTarget[], intent: TransformTransitionIntent): void {
    const unique = [...new Map(targets.map((target) => [target.object, target])).values()];
    if (unique.length === 0) return;
    for (const target of unique) this.stop(target.object);
    const frames = unique.map(({ object, target, frame }) => ({
      object, frame, from: snapshotTransform(object),
      to: { position: target.position.clone(), rotation: target.rotation.clone().normalize(), scale: target.scale.clone() },
    }));
    const apply = (progress: number) => {
      for (const frame of frames) {
        applyInterpolatedTransform(frame.object, frame.from, frame.to, progress);
        frame.frame?.(frame.object);
      }
      this.invalidate();
    };
    if (intent === "immediate" || this.reducedMotion || frames.every((frame) => samePose(frame.from, frame.to))) {
      apply(1);
      return;
    }

    const transition: ActiveTransition = { objects: frames.map((frame) => frame.object), finish: () => {}, stopped: false };
    transition.finish = () => {
      if (transition.stopped) return;
      apply(1);
      transition.stopped = true;
      for (const object of transition.objects) if (this.active.get(object) === transition) this.active.delete(object);
    };
    transition.controls = this.drive((progress) => {
      if (!transition.stopped) apply(progress);
    }, transition.finish, POLICIES[intent]);
    if (!transition.stopped) for (const object of transition.objects) this.active.set(object, transition);
  }

  stop(object: THREE.Object3D): void {
    const transition = this.active.get(object);
    if (!transition) return;
    transition.stopped = true;
    for (const participant of transition.objects) if (this.active.get(participant) === transition) this.active.delete(participant);
    transition.controls?.stop();
  }

  finish(object: THREE.Object3D): void {
    const transition = this.active.get(object);
    if (!transition) return;
    transition.controls?.complete();
    transition.finish();
  }

  stopAll(): void {
    for (const transition of new Set(this.active.values())) {
      transition.stopped = true;
      transition.controls?.stop();
    }
    this.active.clear();
  }
}
