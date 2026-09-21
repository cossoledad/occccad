import type { AssemblyConstraint } from "../../types";
import type { AssemblyConstraintToolKind } from "../tool/cad-tool";

export type AngleParameters = Pick<AssemblyConstraint, "angleRelation" | "angleAxis" | "reverseAngleAxis" | "angleReferenceDirection">;

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
  return { kind: tool, angleRelation: tool === "angle" ? "FREE" : undefined };
}
