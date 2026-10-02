import type { AssemblyConstraint, AssemblyGeometryRef, InstancePath } from "../../types";

export type AssemblyConflictRequest = {analysisId:string;baseRevisionId:string;targetConstraintIds?:string[];maxProbes?:number;timeBudgetMs?:number};
export type AssemblyConflictRepair = "EDIT" | "SUPPRESS" | "MEASURE" | "RECONNECT";
export type AssemblyConflictMember = {constraintId:string;groupId?:string;equationIds?:string[];first:AssemblyGeometryRef;second?:AssemblyGeometryRef;angleAxis?:AssemblyGeometryRef;groupMembers?:Array<{instanceId?:string;instancePath?:InstancePath;groupId?:string}>;repairActions:AssemblyConflictRepair[]};
export type AssemblyConflictItem = {kind:string;evidence:string;oracle:"SAT"|"UNSAT"|"UNKNOWN";constraintIds:string[];reason:string;members:AssemblyConflictMember[];irreducible:boolean};
export type AssemblyConflictReport = {
  schemaVersion:number;actorId:string;documentId:string;baseRevisionId:string;inputDigest:string;solverBuildPolicy:string;
  scopeConstraintIds:string[];scopeBodyIds:string[];backgroundConstraintIds:string[];
  branches:Array<{constraintId:string;directionRelation?:string;distanceRelation?:string;contactBranch?:number;angleBranchState?:unknown}>;
  status:"SAT"|"UNSAT"|"UNKNOWN"|"STALE"|"CANCELLED"|"BUDGET_EXHAUSTED";
  items:AssemblyConflictItem[];probes:Array<{constraintIds:string[];oracle:"SAT"|"UNSAT"|"UNKNOWN";reason:string;solverStatus?:string;elapsedMs:number}>;
  probeCount:number;elapsedMs:number;complete:boolean;budgetReason?:string;
};

export function assemblyConflictCurrent(report:AssemblyConflictReport|undefined,documentId:string,revisionId:string):boolean {
  return Boolean(report&&report.documentId===documentId&&report.baseRevisionId===revisionId&&report.status!=="STALE"&&report.status!=="CANCELLED");
}
export function assemblyConflictRepairs(member:AssemblyConflictMember,definition?:AssemblyConstraint):AssemblyConflictRepair[] {
  if(!definition||definition.id!==member.constraintId)return [];
  return member.repairActions.filter(action=>action!=="MEASURE"||definition.kind==="DISTANCE"||definition.kind==="ANGLE"&&["FREE","DIRECTED"].includes(definition.angleRelation??"FREE"));
}
export function assemblyConflictEvidenceLabel(item:AssemblyConflictItem):string {
  if(item.evidence==="VERIFIED_IRREDUCIBLE"&&item.irreducible&&item.oracle==="UNSAT")return "已验证不可约冲突（不是最小基数保证）";
  if(item.evidence==="ANALYTIC_INCOMPATIBILITY"&&item.oracle==="UNSAT")return "解析证明不兼容";
  if(item.kind==="CURRENT_UNSATISFIED")return "当前姿态未满足（未证明不可行）";
  if(item.kind==="LOCALIZED_SUSPECT")return "局部可疑集合（未证明冲突）";
  if(item.kind==="REDUNDANT")return "局部 Jacobian 冗余（不要求删除）";
  if(item.kind==="DEGENERATE")return "退化构型（不等于冲突）";
  if(item.kind==="SUPPORT_UNAVAILABLE")return "支持无法解析；需检查或重连";
  if(item.kind==="SATISFIED")return "已验证可满足";
  return `${item.kind} · ${item.evidence} · ${item.oracle}`;
}

/** Analysis requests are explicitly revision scoped. A new Head or newer
 * request fences an old response, even if its IDs still happen to exist. */
export class AssemblyConflictResponseGate {
  private generation=0;
  private documentId="";
  private revisionId="";
  begin(documentId:string,revisionId:string):number {this.documentId=documentId;this.revisionId=revisionId;return ++this.generation;}
  invalidate():void {this.generation++;}
  accepts(generation:number,report:AssemblyConflictReport):boolean {return generation===this.generation&&assemblyConflictCurrent(report,this.documentId,this.revisionId);}
}
