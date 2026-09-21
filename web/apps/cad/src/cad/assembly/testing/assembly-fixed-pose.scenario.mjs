import assert from "node:assert/strict";
import { fixedPoseAngles, fixedPoseFromParameters } from "../assembly-fixed-pose.ts";
for (const angles of [[0,0,0],[30,45,60],[15,90,80],[15,-90,80],[0,180,0],[359,-89,270]]) {
  const pose = fixedPoseFromParameters([2,3,5],angles);
  assert.ok(Math.abs(Math.hypot(...pose.rotation)-1)<1e-12);
  const roundTrip = fixedPoseFromParameters(pose.translation,fixedPoseAngles(pose));
  const dot = pose.rotation.reduce((s,v,i)=>s+v*roundTrip.rotation[i],0);
  assert.ok(Math.abs(Math.abs(dot)-1)<1e-10, `${angles}: same rotation through Euler display singularities`);
  assert.deepEqual(roundTrip.translation,[2,3,5]);
}
assert.deepEqual(fixedPoseFromParameters([1,2,3],[0,0,180]).rotation.map(v=>Math.round(v)),[0,0,1,0]);
