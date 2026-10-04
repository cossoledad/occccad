import type { DocumentView, Feature, SelectionItem } from "../../types";
import { projectSketchFeatureSelection } from "../../cad/interaction/selection-mode";
export function selectedSketch(view: DocumentView, selection: SelectionItem, upstream: Feature[]): Feature | undefined {
    const projected = projectSketchFeatureSelection(selection);
    if (projected?.kind !== "sketch" || (projected.ownerDocumentId ?? projected.documentId) !== view.document.id)
        return;
    return upstream.find(f => f.id === projected.id && f.sketch);
}
export function selectedAxis(view: DocumentView, selection: SelectionItem, upstream: Feature[]): string | undefined {
    if ((selection.ownerDocumentId ?? selection.documentId) !== view.document.id)
        return;
    if (selection.kind === "visual") {
        const entity = upstream.find(f => f.id === selection.featureId)?.sketch?.entities.find(e => e.id === selection.entityId);
        return entity?.kind === "LINE" ? `SKETCH_LINE:${selection.featureId}:${entity.id}` : undefined;
    }
    if (selection.kind === "axis") {
        if (selection.axis === "DATUM") {
            const axis = view.part?.datumAxes?.find(a => a.id === selection.entityId || selection.id === a.id || selection.id.endsWith(`:${a.id}`));
            return axis ? `DATUM_AXIS:${axis.id}` : undefined;
        }
        const system = view.part?.axisSystems.find(a => a.id === selection.entityId || selection.id === `${a.id}:${selection.axis}` || selection.id.endsWith(`:${a.id}:${selection.axis}`));
        return system ? `AXIS_SYSTEM:${system.id}:${selection.axis}` : undefined;
    }
}
export function bodyStage(selection: SelectionItem, stages: Feature[]): {
    bodyId: string;
    featureId: string;
} | undefined {
    if (!selection.bodyId)
        return;
    // Tree Feature selections explicitly identify a historical stage. A viewport
    // body hit identifies the eligible tip, never the future final Body result.
    const explicit = ["pad", "feature", "import"].includes(selection.kind);
    const stage = explicit ? stages.find(f => f.id === selection.entityId || f.id === selection.id) : [...stages].reverse().find(f => f.bodyId === selection.bodyId && !f.suppressed);
    return stage?.bodyId === selection.bodyId ? { bodyId: stage.bodyId, featureId: stage.id } : undefined;
}
export function sketchPick(view: DocumentView, id: string, occurrencePath?: string): SelectionItem {
    return { kind: "sketch", id, documentId: view.document.id, versionId: view.document.versionId, bodyId: view.part?.features.find(f => f.id === id)?.bodyId, occurrencePath, expandTreeDescendants: true };
}
