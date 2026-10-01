import type { AssemblyConstraint } from "../../types";
import type { AssemblyConstraintToolKind } from "../tool/cad-tool";

export type AngleParameters = Pick<AssemblyConstraint, "angleRelation" | "angleAxis" | "reverseAngleAxis" | "angleReferenceDirection">;

export function angleRelationSupportsMeasured(relation:AssemblyConstraint["angleRelation"]):boolean {
  return !relation || relation==="FREE" || relation==="DIRECTED";
}

// The axis is its own stable engineering reference, not a hidden property of
// support two. Replacing either support invalidates computed basis evidence,
// but must not discard an independently selected third-occurrence axis.
export function invalidateAngleReferenceDirection<T extends AngleParameters>(value:T):T {
  return {...value,angleReferenceDirection:undefined};
}

export function changeAngleRelation(value: AngleParameters, angleRelation: NonNullable<AssemblyConstraint["angleRelation"]>): AngleParameters {
  return angleRelation === "DIRECTED" ? { ...value, angleRelation } : {
    angleRelation, angleAxis: undefined, angleReferenceDirection: undefined, reverseAngleAxis: false,
  };
}

export function assemblyConstraintEntry(tool: AssemblyConstraintToolKind): {
  kind: Exclude<AssemblyConstraintToolKind, "parallel" | "perpendicular">;
  angleRelation?: AssemblyConstraint["angleRelation"];
} {
  if (tool === "parallel") return { kind: "angle", angleRelation: "PARALLEL" };
  if (tool === "perpendicular") return { kind: "angle", angleRelation: "PERPENDICULAR" };
  if (tool === "rigid") return {kind:"fix_together"};
  return { kind: tool, angleRelation: tool === "angle" ? "FREE" : undefined };
}
