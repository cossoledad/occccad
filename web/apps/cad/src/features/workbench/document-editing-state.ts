import { registerDocumentState } from "../../cad/document/document-session";
import type { DocumentView } from "../../types";
import { prepareOccurrenceEditSession, rootEditSession, type EditSession } from "./edit-session";
import type { NewSketchSession } from "./sketch-session-policy";

export type DocumentEditingState = { session: EditSession; sketchId?: string; newSketchSession?: NewSketchSession };
export const documentEditingState = registerDocumentState<DocumentEditingState>("workbench.editing");

export async function restoreDocumentEditingState(host: DocumentView, saved: DocumentEditingState | undefined, generation: number,
  getDesignSession: Parameters<typeof prepareOccurrenceEditSession>[0]["getDesignSession"],
  getDocument: (id: string) => Promise<DocumentView>) {
  const state = saved?.session.hostDocumentId === host.document.id ? saved : undefined;
  const target = state?.session.editTarget;
  const prepared = target?.instancePath?.canonical
    ? await prepareOccurrenceEditSession({ host, node: { kind: target.documentType, documentId: target.documentId,
      documentType: target.documentType, instancePath: target.instancePath }, activationGeneration: generation, getDesignSession, getDocument })
    : { session: rootEditSession(host, generation), targetView: host };
  const workingBody = state?.session.workingBodyId;
  if (workingBody && prepared.targetView.part?.bodies.some(body => body.id === workingBody)) prepared.session.workingBodyId = workingBody;
  const editable = ["OWNER", "EDITOR"].includes(prepared.targetView.document.permission ?? "");
  const sketchId = editable && prepared.targetView.part?.features.some(feature => feature.type === "SKETCH" && feature.id === state?.sketchId)
    ? state?.sketchId : undefined;
  return { ...prepared, sketchId, newSketchSession: sketchId && state?.newSketchSession?.sketchId === sketchId &&
    state.newSketchSession.documentId === prepared.targetView.document.id ? state.newSketchSession : undefined };
}
