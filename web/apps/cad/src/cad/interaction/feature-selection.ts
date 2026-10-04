import type { Selection, SelectionItem } from "../../types";
import { projectSketchFeatureSelection } from "./selection-mode";
export type FeaturePickRole = "profile" | "axis" | "edge" | "face" | "plane" | "body" | "seam";
export type FeatureSelectionSession = {
    role: FeaturePickRole;
    documentId: string;
    versionId: string;
    occurrencePath?: string;
    bodyId?: string;
    sketchIds?: string[];
    selections: SelectionItem[];
    onPick: (selection: SelectionItem) => void;
};
// Filter raw scene hits before projection: a rotation axis keeps its individual
// sketch entity, while a section intentionally projects to the whole sketch.
export function featureSelectionHit(raw: Selection, session: FeatureSelectionSession): Selection {
    if (!raw || (raw.ownerDocumentId ?? raw.documentId) !== session.documentId ||
        (raw.occurrencePath ?? "") !== (session.occurrencePath ?? "") ||
        (raw.versionId && raw.versionId !== session.versionId))
        return null;
    if (session.bodyId && raw.bodyId !== session.bodyId && (session.role === "edge" || session.role === "face"))
        return null;
    if (session.role === "edge" || session.role === "face")
        return raw.kind === session.role ? raw : null;
    if (session.role === "plane")
        return raw.kind === "plane" ? raw : null;
    if (session.role === "body")
        return raw.bodyId && ["face", "edge", "vertex", "body", "solid", "pad"].includes(raw.kind) ? raw : null;
    if (session.role === "axis")
        return raw.kind === "axis" || raw.kind === "visual" && raw.visualType === "CURVE" ? raw : null;
    if (session.role === "seam")
        return raw.kind === "visual" && raw.visualType === "CURVE" && session.sketchIds?.includes(raw.featureId) ? raw : null;
    const sketch = projectSketchFeatureSelection(raw);
    return sketch?.kind === "sketch" && (!session.sketchIds || session.sketchIds.includes(sketch.id)) ? sketch : null;
}
