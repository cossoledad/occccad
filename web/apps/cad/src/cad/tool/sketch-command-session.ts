import type { DocumentView, SketchGeometryRef } from "../../types";

export type SketchCommandAction =
  | { type: "confirm" | "back" | "cancel" | "add-batch" | "release" | "retry" }
  | { type: "field"; index: number; value: string }
  | { type: "option"; name: string; value: string };
export type SketchCommandPhase = "selection" | "definition" | "placement" | "committing" | "unknown" | "failed" | "committed" | "cancelled";
export type SketchCommandState = {
  toolId: string;
  operation: string;
  phase: SketchCommandPhase;
  role: string;
  selectedIds: string[];
  references: SketchGeometryRef[];
  fields: { label: string; value: string; placeholder?: string }[];
  options: { name: string; label: string; value: string; choices: { value: string; label: string }[] }[];
  canConfirm: boolean;
  next: string;
  error?: string;
  preview?: "approximate" | "authoritative" | "pending" | "unavailable";
};
export type SketchCommitIntent = { requestId: string; baseVersionId?: string; retryReceipt?: boolean };
export type SketchCommitResult = DocumentView | undefined;

// Unknown transport results retain their request identity. Reissuing that exact
// identity asks the existing server receipt path, rather than repeating intent.
export function sketchCommitResultUnknown(error: unknown): boolean {
  const code = (error as { code?: string } | undefined)?.code;
  return ["CONNECTION_CLOSED", "TIMEOUT", "REALTIME_BUSY", "NETWORK_ERROR"].includes(code ?? "") || error instanceof TypeError;
}

export type SketchCommitReceipt = {
  ownerDocumentId:string; featureId:string; operations:import("../../types").SketchOperation[];
  intent:SketchCommitIntent; status:"committing"|"unknown"; error?:string;
};
