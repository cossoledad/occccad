import type { AssemblyConstraint, AssemblyGeometryRef } from "../../types";

export type AssemblyConstraintStatus = AssemblyConstraint["evaluationStatus"];

export const ASSEMBLY_CONSTRAINT_STATUS: Record<AssemblyConstraintStatus, {
  label: string; color: string; glyph: number; description: string;
}> = {
  VERIFIED: { label: "已满足", color: "#238b57", glyph: 17, description: "支持元素已连接，约束求解满足容差。" },
  NOT_UPDATED: { label: "需要更新", color: "#ad6800", glyph: 18, description: "需要重新计算，当前有效性尚未确认。" },
  IMPOSSIBLE: { label: "无法满足", color: "#9c36b5", glyph: 19, description: "支持元素已连接，但几何与该约束定义不兼容。" },
  BROKEN: { label: "支撑失效", color: "#c93636", glyph: 20, description: "至少一个支撑已失效，请重新选择。" },
};

export type AssemblySupportPresentation = {
  status: "CONNECTED" | "NOT_CONNECTED";
  label: "有效" | "需要重新选择";
  diagnosticCode?: string;
  diagnostic?: string;
  evidenceDigest?: string;
};

export function assemblySupportPresentation(reference?: AssemblyGeometryRef): AssemblySupportPresentation {
  if (!reference) return { status: "NOT_CONNECTED", label: "需要重新选择", diagnosticCode: "MISSING_SUPPORT", diagnostic: "尚未选择支持元素。" };
  if (!["FACE", "EDGE", "VERTEX"].includes(reference.kind)) return { status: "CONNECTED", label: "有效" };
  const result = reference.resolution?.result;
  if (!result && reference.geometryKey && reference.topologyId) {
    return { status: "CONNECTED", label: "有效", diagnostic: "新选择待确认。" };
  }
  const connected = result?.supportingElementStatus === "CONNECTED";
  return {
    status: connected ? "CONNECTED" : "NOT_CONNECTED",
    label: connected ? "有效" : "需要重新选择",
    diagnosticCode: result?.diagnosticCode ?? (!result ? "RESOLUTION_UNAVAILABLE" : undefined),
    diagnostic: result?.diagnostic ?? (!result ? "当前支撑尚未完成检查。" : undefined),
    evidenceDigest: result?.evidenceDigest,
  };
}

export function firstDisconnectedSupport(constraint: AssemblyConstraint): 0 | 1 | undefined {
  if (constraint.kind === "FIX_TOGETHER") return undefined;
  if (assemblySupportPresentation(constraint.first).status === "NOT_CONNECTED") return 0;
  if (constraint.second && assemblySupportPresentation(constraint.second).status === "NOT_CONNECTED") return 1;
  return undefined;
}

export function validateReconnectCandidate(candidate: AssemblyGeometryRef | undefined,
  other: AssemblyGeometryRef | undefined): string | undefined {
  if (!candidate) return "请选择点、轴、平面、面、边、顶点或实体。";
  if (other && candidate.instanceId === other.instanceId) return "二元装配约束的两个支持元素必须来自不同实例。";
  return undefined;
}

export function assemblyConstraintGlyph(status: AssemblyConstraintStatus, kindGlyph: number): number {
  return status === "VERIFIED" ? kindGlyph : ASSEMBLY_CONSTRAINT_STATUS[status].glyph;
}

export function assemblyStatusFromDiagnostic(diagnostic?: string): AssemblyConstraintStatus {
  const status = diagnostic?.split(":", 1)[0] as AssemblyConstraintStatus | undefined;
  return status && status in ASSEMBLY_CONSTRAINT_STATUS ? status : "NOT_UPDATED";
}

export function assemblyStatusAfterPreviewFailure(phase: string | undefined, retryable: boolean,
  supports: readonly AssemblySupportPresentation[]): AssemblyConstraintStatus {
  if (supports.some((support) => support.status === "NOT_CONNECTED")) return "BROKEN";
  // A failed solve alone does not prove intrinsic geometric incompatibility.
  return "NOT_UPDATED";
}
