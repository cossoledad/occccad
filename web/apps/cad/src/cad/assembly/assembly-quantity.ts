import type { AssemblyConstraint } from "../../types";

export type AssemblyQuantityFields = {quantityExpression?:string;quantityKey?:string;constraintMode?:string};

export function assemblyQuantityInitialFields(constraint?:AssemblyConstraint):AssemblyQuantityFields {
  const p=constraint?.quantityParameter;
  return {quantityExpression:p?.source.expression?.sourceText ?? "",quantityKey:p?.key ?? "",constraintMode:constraint?.mode ?? "DRIVING"};
}

// Shared by Preview and commit. Values stay user-domain; SI/mm/rad compilation
// occurs in the authoritative Quantity evaluator, never by parsing UI labels.
export function assemblyQuantityCommandFields(fields:AssemblyQuantityFields) {
  return {quantityExpression:fields.quantityExpression ?? "",quantityKey:fields.quantityKey?.trim() || undefined,
    constraintMode:fields.constraintMode ?? "DRIVING"};
}
