import type { AssemblyConstraint, Vec3 } from "../../types";

export type FixedPose = NonNullable<AssemblyConstraint["fixedPose"]>;
// UI uses extrinsic X, then Y, then Z rotations: R = Rz * Ry * Rx.
// The durable and numerical representation remains a unit quaternion.
export function fixedPoseFromParameters(translation: Vec3, angles: Vec3): FixedPose {
  const [x,y,z] = angles.map(v => v * Math.PI / 360);
  const [cx,cy,cz] = [Math.cos(x),Math.cos(y),Math.cos(z)];
  const [sx,sy,sz] = [Math.sin(x),Math.sin(y),Math.sin(z)];
  return { translation, rotation:[sx*cy*cz-cx*sy*sz, cx*sy*cz+sx*cy*sz,
    cx*cy*sz-sx*sy*cz, cx*cy*cz+sx*sy*sz] };
}

export function fixedPoseAngles(pose: FixedPose): Vec3 {
  const norm = Math.hypot(...pose.rotation);
  const [x,y,z,w] = pose.rotation.map(v => v / norm);
  const sinPitch = Math.max(-1, Math.min(1, 2*(w*y-z*x)));
  const pitch = Math.asin(sinPitch);
  // At the display singularity choose roll = 0 and preserve the full rotation.
  const roll = Math.abs(sinPitch) > 1-1e-12 ? 0 : Math.atan2(2*(w*x+y*z),1-2*(x*x+y*y));
  const yaw = Math.abs(sinPitch) > 1-1e-12
    ? Math.atan2(2*(w*z-x*y),1-2*(x*x+z*z))
    : Math.atan2(2*(w*z+x*y),1-2*(y*y+z*z));
  return [roll,pitch,yaw].map(v => Number((v*180/Math.PI).toPrecision(15))) as Vec3;
}
