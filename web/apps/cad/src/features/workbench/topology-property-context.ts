import type { Selection } from "../../types";

export function topologyPropertyContext(selection: Selection, activeDocumentId: string): { documentId: string; versionId?: string } {
  if (!selection || !["face", "edge", "vertex"].includes(selection.kind)) return { documentId: activeDocumentId };
  return { documentId: selection.documentId || activeDocumentId, versionId: selection.versionId };
}
