import {contextScopeForView,type ContextEditScope} from "../api/context-edit-scope";
import type { Artifact, DatumReference, DocumentStructureNode, DocumentView } from "../types";

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
export type FeatureInputBoundary = { afterFeatureId: string; versionId: string; occurrencePath?:string; context?:ContextEditScope };
export function featureInputDisplay(view: DocumentView, inputs: Artifact[] | undefined, ownerId: string, occurrencePath?: string, boundary?:FeatureInputBoundary): DocumentView {
    if (boundary && view.part && view.document.id === ownerId){
        const context=boundary.context;
        if(boundary.occurrencePath&&(!context||view.contextOccurrencePath!==boundary.occurrencePath||JSON.stringify(contextScopeForView(view))!==JSON.stringify(context)))
            return {...view,part:undefined,artifact:undefined,artifacts:{},sketchAnalyses:{},datumPlanes:[],datumAxes:[],axisSystems:[]};
        return historicalPartDisplay(view,boundary.afterFeatureId,boundary.versionId===view.document.versionId?inputs??[]:[]);
    }
    if(boundary && view.product && occurrencePath){
        const scope=boundary.context;
        const variant=view.contextVariants?.find(v=>v.owningInstancePath.canonical===occurrencePath&&v.baseDocumentId===ownerId);
        const valid=!!scope&&scope.rootProductDocumentId===view.document.id&&scope.rootProductRevisionId===view.document.versionId&&scope.instancePath.canonical===occurrencePath&&variant?.variantKey===scope.contextVariantId&&variant?.baseRevisionId===boundary.versionId&&variant.status==='READY'&&!!variant.part;
        const accepted=valid?inputs??[]:[];
        const local=variant?.part?historicalPartDisplay({document:{...view.document,id:ownerId,versionId:boundary.versionId},part:variant.part,sketchAnalyses:variant.sketchAnalyses} as DocumentView,boundary.afterFeatureId,accepted):undefined;
        const ids=new Set(local?.part?.features.map(f=>f.id)??[]);
        // Remove every active Body tip/fallback, including Bodies with no prefix
        // output. A sibling occurrence retains its complete accepted snapshot.
        const resolvedInstances=(view.resolvedInstances??[]).map(r=>r.occurrencePath===occurrencePath&&r.documentId===ownerId?{...r,geometryKey:accepted.find(a=>a.bodyId===r.bodyId)?.geometryKey??'',displayFallback:undefined,ownedSketchIds:(r.ownedSketchIds??[]).filter(id=>ids.has(id))}:r);
        const template=resolvedInstances.find(r=>r.occurrencePath===occurrencePath&&r.documentId===ownerId);
        if(template)for(const input of accepted){
            if(!input.bodyId||resolvedInstances.some(r=>r.occurrencePath===occurrencePath&&r.bodyId===input.bodyId))continue;
            const node=inputBodyNode(view.structureTree,ownerId,input.bodyId,occurrencePath);
            if(node)resolvedInstances.push({...template,id:`${template.id}/feature-input:${input.bodyId}`,name:node.name,bodyId:input.bodyId,bodyTreeNodeId:node.id,geometryKey:input.geometryKey,ownedSketchIds:[]});
        }
        return {...view,resolvedInstances,artifacts:{...view.artifacts,...Object.fromEntries(accepted.map(a=>[a.geometryKey,a]))},contextVariants:view.contextVariants?.map(v=>v===variant&&local?{...v,part:local.part,sketchAnalyses:local.sketchAnalyses}:v)};
    }
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

/** A display projection only. The full definition remains the command baseline. */
export function historicalPartDisplay(view: DocumentView, after: string, inputs: Artifact[]): DocumentView {
  if (!view.part) return view;
  const index = view.part.features.findIndex(f => f.id === after);
  if (index < 0) inputs = [];
  const features = view.part.features.slice(0, index + 1), ids = new Set(features.map(f => f.id));
  const definitions = new Map([...view.part.datumPlanes, ...(view.part.datumAxes??[])].map(d => [d.id, d.definition]));
  const available = (id: string, seen = new Set<string>()): boolean => {
    if (seen.has(id)) return false;
    const definition = definitions.get(id); if (!definition) return definitions.has(id) || view.part!.axisSystems.some(a => a.id === id);
    const next = new Set(seen).add(id);
    const legal = (ref: DatumReference): boolean => (!ref.featureId || ids.has(ref.featureId)) && (!ref.selection?.anchor.featureId || ids.has(ref.selection.anchor.featureId)) &&
      (!["PLANE", "AXIS", "AXIS_SYSTEM", "POINT"].includes(ref.kind) || !!ref.entityId && available(ref.entityId, next));
    return [definition.source, definition.rotationAxis, definition.translationDirection].every(ref => !ref || legal(ref));
  };
  const datumPlanes = view.part.datumPlanes.filter(d => available(d.id)), datumAxes = view.part.datumAxes?.filter(d => available(d.id));
  return { ...view, artifact: undefined, datumPlanes, datumAxes, axisSystems:view.part.axisSystems, artifacts: Object.fromEntries(inputs.map(a => [a.geometryKey, a])),
    sketchAnalyses: Object.fromEntries(Object.entries(view.sketchAnalyses ?? {}).filter(([id]) => ids.has(id))),
    part: { ...view.part, features, datumPlanes, datumAxes,
      bodies: view.part.bodies.map(b => ({ ...b, geometryKey: inputs.find(a => a.bodyId === b.id)?.geometryKey, displayFallback: undefined, consumed: false })) } };
}
