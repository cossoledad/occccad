import type { Selection, SelectionItem } from "../../types";
import { projectSketchFeatureSelection } from "./selection-mode";
export type FeaturePickRole = "direction" | "geometry" | "point" | "profile" | "axis" | "edge" | "face" | "plane" | "body" | "seam";
export type FeatureSelectionSession = {
    role: FeaturePickRole;
    // Shared scene/tree filtering for cross-occurrence command supports.
    accept?: (selection:SelectionItem)=>boolean;
    // Exact support inspection may finish after the click. It must not select
    // an unqualified candidate or require the user to click it again.
    pendingCandidate?: (selection:SelectionItem)=>boolean;
    resolvePick?: (selection:SelectionItem)=>void;
    connectionLines?: [number,number,number][][];
    localSketchId?: string;
    documentId: string;
    versionId: string;
    occurrencePath?: string;
    bodyId?: string;
    geometryKey?:string;
    contextVariantKey?:string;
    rootDocumentId?:string;
    sketchIds?: string[];
    contextSelections?: SelectionItem[];
    selections: SelectionItem[];
    onPick: (selection: SelectionItem) => void;
};
// Filter raw scene hits before projection: a rotation axis keeps its individual
// sketch entity, while a section intentionally projects to the whole sketch.
export function featureSelectionHit(raw: Selection, session: FeatureSelectionSession): Selection {
    if(session.accept)return raw&&session.accept(raw)?raw:null;
    if (!raw || (raw.ownerDocumentId ?? raw.documentId) !== session.documentId ||
        (raw.occurrencePath ?? "") !== (session.occurrencePath ?? "") ||
        (raw.versionId && raw.versionId !== session.versionId))
        return null;
    if(session.contextVariantKey!==undefined&&raw.contextVariantKey!==session.contextVariantKey)return null;
    if(session.rootDocumentId&&(raw.instancePath?.rootDocumentId??raw.rootDocumentId)!==session.rootDocumentId)return null;
    if(session.localSketchId && (raw.kind!=="visual" || raw.featureId!==session.localSketchId || ((session.role==="point"||session.role==="axis")&&!raw.sketchReference)))return null;
    if (session.bodyId && raw.bodyId !== session.bodyId && (session.role === "edge" || session.role === "face"))
        return null;
    if (session.role === "edge" || session.role === "face")
        return (!session.geometryKey || raw.geometryKey===session.geometryKey) && raw.kind === session.role ? raw : null;
    if (session.role === "plane")
        return raw.kind === "plane" || raw.kind === "face" ? raw : null;
    if (session.role === "body")
        return raw.bodyId && ["face", "edge", "vertex", "body", "solid", "pad", "feature", "import"].includes(raw.kind) ? raw : null;
    if (session.role === "point") return ["axis-system","datum-point"].includes(raw.kind)||raw.kind==="visual"&&raw.visualType==="POINT"?raw:null;
    if(session.role==="direction")return raw.kind==="axis"||raw.kind==="edge"||raw.kind==="visual"&&raw.visualType==="CURVE"?raw:null;
    if (session.role === "axis")
        return raw.kind === "axis" || raw.kind === "visual" && raw.visualType === "CURVE" ? raw : null;
    if (session.role === "geometry") return raw.kind === "visual" && !!session.sketchIds?.includes(raw.featureId) ? raw : null;
    if (session.role === "seam")
        return raw.kind === "visual" && raw.visualType === "CURVE" && session.sketchIds?.includes(raw.featureId) ? raw : null;
    const sketch = projectSketchFeatureSelection(raw);
    return sketch?.kind === "sketch" && (!session.sketchIds || session.sketchIds.includes(sketch.id)) ? sketch : null;
}

export function pickFeatureSelection(raw:Selection,session:FeatureSelectionSession):void {
    const hit=featureSelectionHit(raw,session);
    if(hit)session.onPick(hit);
    else if(raw&&session.pendingCandidate?.(raw))session.resolvePick?.(raw);
}
