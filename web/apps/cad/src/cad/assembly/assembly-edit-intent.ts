import type { AssemblyConstraint, AssemblyGeometryRef, Vec3 } from "../../types";
import { assemblyPublicCommandFields } from "./assembly-public";
import { offsetCommandFields } from "./assembly-offset";
import { assemblyQuantityCommandFields } from "./assembly-quantity";
import { fixedPoseFromParameters } from "./assembly-fixed-pose";
import { displayLengthToMillimeters } from "../../state/ui-preferences";

export type AssemblyDraftFields = Parameters<typeof assemblyPublicCommandFields>[1] &
  Parameters<typeof assemblyQuantityCommandFields>[0] & {
    value?: number; directionRelation?: string; distanceRelation?: string;
    fixedTranslation?: Vec3; fixedAngles?: Vec3;
  };

// One user-intent boundary for create, preview and edit commit. Never spread an
// evaluated constraint into a command. Hidden form fields are read from the
// complete store, not validateFields' registered-field-only return value.
export function assemblyEditIntent(kind: string, fields: AssemblyDraftFields,
  references: AssemblyGeometryRef[], lengthUnit: Parameters<typeof displayLengthToMillimeters>[1],
  constraint?: AssemblyConstraint, options: Partial<AssemblyConstraint> = {}) {
  const source = constraint ?? options;
  const raw = Number(fields.value ?? 0);
  const command = {
    type: constraint ? "EDIT_ASSEMBLY_CONSTRAINT" : "ADD_ASSEMBLY_CONSTRAINT",
    ...(constraint ? {targetId: constraint.id} : {constraintKind: kind}),
    value: kind === "ANGLE" ? raw * Math.PI / 180 : kind === "DISTANCE" ? displayLengthToMillimeters(raw, lengthUnit) : raw,
    directionRelation: fields.directionRelation ?? "UNORIENTED",
    distanceRelation: fields.distanceRelation ?? "UNSIGNED",
    firstAssemblyRef: references[0], secondAssemblyRef: references[1],
    angleRelation: kind === "ANGLE" ? source.angleRelation ?? "FREE" : undefined,
    angleAxis: kind === "ANGLE" && source.angleRelation === "DIRECTED" ? source.angleAxis : undefined,
    reverseAngleAxis: kind === "ANGLE" && source.angleRelation === "DIRECTED" ? source.reverseAngleAxis ?? false : undefined,
    // FREE's frozen reference is explicit branch intent; DIRECTED has a third
    // support instead. Evaluated result/status/poses never become raw input.
    angleReferenceDirection: kind === "ANGLE" && source.angleRelation !== "DIRECTED" ? source.angleReferenceDirection : undefined,
    fixMode: kind === "FIX" ? source.fixMode ?? "SPACE" : undefined,
    fixedPose: constraint && kind === "FIX" ? fixedPoseFromParameters(
      (fields.fixedTranslation ?? [0,0,0]).map(v => displayLengthToMillimeters(v,lengthUnit)) as Vec3,
      fields.fixedAngles ?? [0,0,0]) : undefined,
    ...(kind === "DISTANCE" ? offsetCommandFields(fields) : {}),
    ...(kind === "ANGLE" ? (!source.angleRelation || source.angleRelation === "FREE" || source.angleRelation === "DIRECTED"
      ? assemblyQuantityCommandFields(fields) : {constraintMode:"DRIVING"}) : {}),
    ...assemblyPublicCommandFields(kind,fields,constraint,references),
  };
  // Freeze nested refs, AST inputs and members, and normalize wire omission.
  return JSON.parse(JSON.stringify(command)) as Record<string, unknown>;
}

export function assemblyIntentKey(command: Record<string,unknown>, documentId: string, revision: string, occurrence = "") {
  const canonical=(value:unknown):unknown=>Array.isArray(value) ? value.map(canonical)
    : value && typeof value==="object" ? Object.fromEntries(Object.entries(value).sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>[k,canonical(v)])) : value;
  return JSON.stringify(canonical([documentId,revision,occurrence,command]));
}

export class AssemblyCandidateBinding {
  private generation = 0;
  private value?: {key:string; previewId:string; command:Record<string,unknown>};
  invalidate() { this.generation++; this.value=undefined; }
  begin() { this.invalidate(); return this.generation; }
  resolve(generation:number,key:string,previewId:string,command:Record<string,unknown>) {
    if(generation!==this.generation || !previewId)return false;
    this.value={key,previewId,command:JSON.parse(JSON.stringify(command))}; return true;
  }
  freeze(key:string) {
    if(this.value?.key!==key)return undefined;
    return JSON.parse(JSON.stringify({...this.value.command,previewId:this.value.previewId})) as Record<string,unknown>;
  }
}

// The dialog's draft/session lifetime is independent of Preview requests and
// authoritative cache refreshes caused by its own successful commit.
export class AssemblyDialogLifecycle {
  private generation = 0;
  invalidate() { this.generation++; }
  capture() { return this.generation; }
  isCurrent(generation:number) { return generation===this.generation; }
}
