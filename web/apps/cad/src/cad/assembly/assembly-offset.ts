import type { AssemblyConstraint, AssemblyGeometryRef } from "../../types";

export type OffsetFields = {quantityExpression?:string;quantityKey?:string;constraintMode?:string};
// Shared by Preview and both create/edit commits. No sign/endpoint conversion.
export function offsetCommandFields(fields:OffsetFields):OffsetFields {
  return {quantityExpression:fields.quantityExpression ?? "", quantityKey:fields.quantityKey?.trim() || undefined,
    constraintMode:fields.constraintMode ?? "DRIVING"};
}
export function offsetInitialFields(constraint?:AssemblyConstraint, references:AssemblyGeometryRef[]=[], exactTypes?:string[]):OffsetFields & {distanceRelation:string} {
  const parameter=constraint?.quantityParameter;
  return {quantityExpression:parameter?.source.expression?.sourceText ?? "",
    quantityKey:parameter?.key ?? "", constraintMode:constraint?.mode ?? "DRIVING",
    distanceRelation:constraint?.distanceRelation ?? ((exactTypes ?? references.map(r=>r.kind)).includes("PLANE") ? "SELECTED_PLANE_NORMAL_V1" : "UNSIGNED")};
}
