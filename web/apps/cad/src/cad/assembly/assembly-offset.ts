import type { AssemblyConstraint, AssemblyGeometryRef } from "../../types";

export type OffsetFields = {offsetExpression?:string;offsetKey?:string;constraintMode?:string};
// Shared by Preview and both create/edit commits. No sign/endpoint conversion.
export function offsetCommandFields(fields:OffsetFields):OffsetFields {
  return {offsetExpression:fields.offsetExpression ?? "", offsetKey:fields.offsetKey?.trim() || undefined,
    constraintMode:fields.constraintMode ?? "DRIVING"};
}
export function offsetInitialFields(constraint?:AssemblyConstraint, references:AssemblyGeometryRef[]=[]):OffsetFields & {distanceRelation:string} {
  return {offsetExpression:constraint?.offsetParameter?.source.expression?.sourceText ?? "",
    offsetKey:constraint?.offsetParameter?.key ?? "", constraintMode:constraint?.mode ?? "DRIVING",
    distanceRelation:constraint?.distanceRelation ?? (references.some(r=>["PLANE","FACE"].includes(r.kind)) ? "SELECTED_PLANE_NORMAL_V1" : "UNSIGNED")};
}
