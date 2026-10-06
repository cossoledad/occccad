import type { DocumentProperties, DocumentView, DocumentStructureNode, Feature, Selection, TopologyElementProperties } from "../../types";

export function inspectorOwnerDocumentId(view: DocumentView, selection: Selection) {
  return selection?.ownerDocumentId ?? selection?.documentId ?? view.document.id;
}

export function inspectorTopologyData(selection:Selection, topology?:TopologyElementProperties) {
  if (!selection || !topology || !("topologyId" in selection)) return undefined;
  return topology.geometryKey===selection.geometryKey && topology.kind===selection.kind.toUpperCase() &&
    topology.localId===selection.topologyId ? topology : undefined;
}

/** Resolve definitions by stable identity and the selected snapshot, never a label/local index. */
export function inspectorObjectData(view: DocumentView, selection: Selection, feature?: Feature,
  topology?: TopologyElementProperties, diagnostics?: DocumentProperties, ownerView?: DocumentView) {
  if (!selection) return { document: view.document, model: view.part ?? view.product, diagnostics };
  const ownerID = inspectorOwnerDocumentId(view, selection);
  const candidate = ownerID === view.document.id ? view : ownerView;
  const definitionView = candidate?.document.id === ownerID &&
    (!selection.versionId || candidate.document.versionId === selection.versionId) ? candidate : undefined;
  const part = definitionView?.part, product = definitionView?.product;
  const id = selection.entityId ?? selection.id;
  const featureID = "featureId" in selection ? selection.featureId : id;
  const selectedFeature = part?.features.find(item => item.id === featureID) ??
    (definitionView && feature?.id === featureID ? feature : undefined);
  const occurrence = view.resolvedInstances?.filter(item => item.documentId === ownerID &&
    item.occurrencePath === (selection.occurrencePath ?? "") &&
    (!selection.bodyId || item.bodyId === selection.bodyId) &&
    (!selection.versionId || item.instancePath.segments.at(-1)?.resolvedVersionId === selection.versionId));
  const body = part?.bodies.find(item => item.id === selection.bodyId);
  const geometryKey = selection.geometryKey ?? body?.geometryKey ?? occurrence?.[0]?.geometryKey;
  let object: unknown;
  switch (selection.kind) {
    case "parameter": object = part?.parameters?.find(item => item.parameterId === id); break;
    case "part": case "product": object = part ?? product; break;
    case "body": case "solid": object = body; break;
    case "face": case "edge": case "vertex": object = inspectorTopologyData(selection,topology); break;
    case "plane": object = part?.datumPlanes.find(item => item.id === id) ?? selection.datumPlane; break;
    case "axis": object = selection.axis === "DATUM" ? part?.datumAxes?.find(item => item.id === id)
      : part?.axisSystems.find(item => item.id === id); break;
    case "axis-system": case "datum-point": object = part?.axisSystems.find(item => item.id === id); break;
    case "sketch-constraint": object = selectedFeature?.sketch?.constraints.find(item => item.id === selection.constraintId); break;
    case "visual": object = selectedFeature?.sketch?.entities.find(item => item.id === id) ??
      selectedFeature?.sketch?.externalGeometry?.find(item => item.id === id) ??
      definitionView?.sketchPatternMembers?.[featureID]?.find(item => item.id === id); break;
    case "instance": object = view.product?.instances.find(item => item.id === id); break;
    case "assembly-constraint": object = product?.constraints?.find(item => item.id === selection.constraintId) ??
      view.constraintDisplayScopes?.find(scope => scope.documentId === ownerID && scope.versionId === selection.versionId &&
        scope.instancePath.canonical === selection.occurrencePath)?.constraints.find(item => item.id === selection.constraintId); break;
    case "publication": object = selection.publication ?? part?.publications?.find(item => item.id === (selection.publicationId ?? id)) ??
      product?.publications?.find(item => item.id === (selection.publicationId ?? id)); break;
    case "context-input": object = part?.contextInputs?.find(item => item.id === id); break;
    case "external-reference": object = part?.contextReferences?.find(item => item.id === id); break;
    case "context-binding": object = product?.contextBindings?.find(item => item.id === id); break;
    default: object = id === "origin" && part ? {originVisible:part.originVisible,datumPlanes:part.datumPlanes,axisSystems:part.axisSystems,datumAxes:part.datumAxes} : selectedFeature;
  }
  if (selection.publicationId && selection.publication) object = selection.publication;
  const findNode=(node?:DocumentStructureNode):DocumentStructureNode|undefined=>{
    if (!node) return undefined;
    if (selection.treeNodeId===node.id && (!selection.versionId||node.versionId===selection.versionId) &&
      (node.ownerDocumentId??node.documentId)===ownerID) return node;
    for (const child of node.children??[]) {const found=findNode(child);if(found)return found;}
  };
  const parameters = part?.parameters?.filter(item => (selectedFeature && item.ownerFeatureId === selectedFeature.id) ||
    item.ownerFeatureId === id || (object && "parameterId" in Object(object) && item.parameterId === (object as {parameterId:string}).parameterId));
  return {
    selection, treeNode:findNode(view.structureTree), ownerDocument: definitionView?.document,
    definitionStatus: object ? "AVAILABLE" : "所选快照的完整定义尚未加载；不使用其他 Revision 或 occurrence 的数据替代。",
    object, parameters: parameters?.length ? parameters : undefined,
    sketchSolve: selectedFeature?.sketch?.solve,
    profileAnalysis: selectedFeature ? definitionView?.sketchAnalyses?.[selectedFeature.id] : undefined,
    occurrence: occurrence?.length ? occurrence : undefined,
    artifact: geometryKey ? view.artifacts?.[geometryKey] ?? definitionView?.artifacts?.[geometryKey] : undefined,
  };
}
