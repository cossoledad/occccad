import type { AssemblyConstraint, AssemblyGeometryRef } from "../../types";

export type OffsetFields = {offsetExpression?:string;offsetKey?:string;constraintMode?:string};
// Shared by Preview and both create/edit commits. No sign/endpoint conversion.
export function offsetCommandFields(fields:OffsetFields):OffsetFields {
  return {offsetExpression:fields.offsetExpression ?? "", offsetKey:fields.offsetKey?.trim() || undefined,
    constraintMode:fields.constraintMode ?? "DRIVING"};
}
export function offsetInitialFields(constraint?:AssemblyConstraint, references:AssemblyGeometryRef[]=[], exactTypes?:string[]):OffsetFields & {distanceRelation:string} {
  const parameter=constraint?.quantityParameter ?? constraint?.offsetParameter;
  return {offsetExpression:parameter?.source.expression?.sourceText ?? "",
    offsetKey:parameter?.key ?? "", constraintMode:constraint?.mode ?? "DRIVING",
    distanceRelation:constraint?.distanceRelation ?? ((exactTypes ?? references.map(r=>r.kind)).includes("PLANE") ? "SELECTED_PLANE_NORMAL_V1" : "UNSIGNED")};
}
