import type { Artifact, DocumentStructureNode, DocumentView } from "../types";

function inputBodyNode(node: DocumentStructureNode | undefined, ownerId: string, bodyId: string, occurrencePath: string): DocumentStructureNode | undefined {
    if (!node) return;
    if (node.kind === "BODY" && node.subject?.documentId === ownerId && node.subject.entityId === bodyId &&
        (node.occurrence?.instancePath.canonical ?? node.instancePath?.canonical) === occurrencePath) return node;
    for (const child of node.children ?? []) {
        const found = inputBodyNode(child, ownerId, bodyId, occurrencePath);
        if (found) return found;
    }
}
// Temporary geometry is scoped to the committed edit occurrence; definitions,
// Head, persistent visibility and every other occurrence keep their own state.
export function featureInputDisplay(view: DocumentView, inputs: Artifact[] | undefined, ownerId: string, occurrencePath?: string): DocumentView {
    if (!inputs?.length)
        return view;
    const artifacts = { ...view.artifacts, ...Object.fromEntries(inputs.map(a => [a.geometryKey, a])) };
    if (view.part && view.document.id === ownerId && !occurrencePath)
        return { ...view, artifacts,
            part: { ...view.part, bodies: view.part.bodies.map(body => { const input = inputs.find(a => a.bodyId === body.id); return input ? { ...body, geometryKey: input.geometryKey, consumed: false } : body; }) } };
    if (!view.product || !occurrencePath)
        return view;
    const template = view.resolvedInstances?.find(r => r.occurrencePath === occurrencePath && r.documentId === ownerId);
    if (!template)
        return view;
    const resolvedInstances = (view.resolvedInstances ?? []).map(r => {
        const input = r.occurrencePath === occurrencePath && r.documentId === ownerId ? inputs.find(a => a.bodyId === r.bodyId) : undefined;
        return input ? { ...r, geometryKey: input.geometryKey } : r;
    });
    for (const input of inputs) {
        if (!input.bodyId || resolvedInstances.some(r => r.occurrencePath === occurrencePath && r.documentId === ownerId && r.bodyId === input.bodyId)) continue;
        const node = inputBodyNode(view.structureTree, ownerId, input.bodyId, occurrencePath);
        // Restore only a Body whose semantic owner and occurrence are evidenced
        // by the tree; never borrow the target Body's tree identity or label.
        if (node) resolvedInstances.push({ ...template, id: `${template.id}/feature-input:${input.bodyId}`, name: node.name, bodyId: input.bodyId,
            bodyTreeNodeId: node.id, geometryKey: input.geometryKey, ownedSketchIds: [] });
    }
    return { ...view, artifacts, resolvedInstances };
}
