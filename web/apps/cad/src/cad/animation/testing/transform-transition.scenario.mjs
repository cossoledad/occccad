import assert from "node:assert/strict";
import * as THREE from "three";
import { applyInterpolatedTransform, snapshotTransform, TransformTransitionSystem } from "../transform-transition.ts";

const pose = (x, angle = 0) => ({
  position: new THREE.Vector3(x, 0, 0),
  rotation: new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0, 0, 1), angle),
  scale: new THREE.Vector3(1, 1, 1),
});

// TRS interpolation keeps the rotation normalized and follows the quaternion's short arc.
const interpolated = new THREE.Group();
applyInterpolatedTransform(interpolated, pose(0, Math.PI * 0.95), pose(10, -Math.PI * 0.95), 0.5);
assert.ok(Math.abs(interpolated.position.x - 5) < 1e-9);
assert.ok(Math.abs(interpolated.quaternion.length() - 1) < 1e-9);
assert.ok(Math.abs(interpolated.quaternion.angleTo(new THREE.Quaternion().setFromAxisAngle(
  new THREE.Vector3(0, 0, 1), Math.PI))) < 1e-9);

const jobs = [];
const drive = (update, complete, options) => {
  const job = { update, complete, options, stopped: false };
  jobs.push(job);
  return { stop: () => { job.stopped = true; }, complete };
};
let invalidations = 0;
const transitions = new TransformTransitionSystem(() => invalidations += 1, false, drive);
const object = new THREE.Group();
transitions.apply(object, pose(10), "settle");
assert.equal(jobs[0].options.duration, 0.2);
assert.deepEqual(jobs[0].options.ease, [0.2, 0.8, 0.3, 1]);
jobs[0].update(0.4);
assert.ok(Math.abs(object.position.x - 4) < 1e-9);

// Retargeting starts from the currently rendered pose and cancels the superseded animation.
transitions.apply(object, pose(14), "preview");
assert.equal(jobs[0].stopped, true);
jobs[1].update(0.5);
assert.ok(Math.abs(object.position.x - 9) < 1e-9);
transitions.finish(object);
assert.equal(object.position.x, 14);

// Reduced motion and immediate input both bypass the time driver.
const reduced = new TransformTransitionSystem(() => {}, true, drive);
reduced.apply(object, pose(20), "reconcile");
assert.equal(object.position.x, 20);
assert.deepEqual(snapshotTransform(object).position.toArray(), [20, 0, 0]);
assert.ok(invalidations >= 3);

// A solver result is one coherent batch: every affected occurrence shares one clock.
const first = new THREE.Group();
const second = new THREE.Group();
transitions.applyBatch([{ object: first, target: pose(6) }, { object: second, target: pose(12) }], "settle");
assert.equal(jobs.length, 3);
jobs[2].update(0.5);
assert.equal(first.position.x, 3);
assert.equal(second.position.x, 6);
transitions.stop(second);
assert.equal(jobs[2].stopped, true);
