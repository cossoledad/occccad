import { ASSEMBLY_CONSTRAINT_STATUS, assemblyStatusFromDiagnostic } from "../../cad/assembly/assembly-constraint-ux";
import { treeNodeIcon } from "./tree-node-descriptors";
import type { DocumentStructureNode, DocumentView, Feature, Selection, SelectionItem } from "../../types";
import type { SpecificationTreeNode } from "./specification-tree";

export function isSolidFeature(feature: Feature): boolean {
  return ["SOLID_PATTERN", "PAD", "LINEAR_EXTRUDE", "REVOLVE", "IMPORT_BODY", "BOOLEAN", "FILLET", "CHAMFER", "DRAFT", "SHELL", "LOFT"].includes(feature.type.toUpperCase());
}

export function structureSelection(node: DocumentStructureNode, view: DocumentView): Selection {
  const instancePath = node.occurrence?.instancePath ?? node.instancePath;
  const occurrencePath = instancePath?.canonical ?? "";
  const bodyId = node.bodyId ?? (node.kind === "BODY" ? node.entityId : undefined);
  const resolvedBody = view.resolvedInstances?.find((candidate) =>
    candidate.occurrencePath === occurrencePath && candidate.bodyId === bodyId);
  const geometryKey = resolvedBody?.geometryKey ?? node.snapshot?.geometryKey ?? node.geometryKey ??
    view.part?.bodies?.find((body) => body.id === bodyId)?.geometryKey;
  const base = {
    id: node.subject?.entityId ?? node.entityId ?? node.id,
    entityId: node.subject?.entityId ?? node.entityId,
    patternId:node.patternId,patternMemberSlot:node.patternMemberSlot,
    entityKind: node.subject?.entityKind ?? node.kind,
    entityRef: node.subject, occurrenceRef: node.occurrence, snapshotScope: node.snapshot,
    documentId: node.documentId, ownerDocumentId: node.ownerDocumentId ?? node.documentId,
    bodyId, versionId: node.snapshot?.revisionId ?? node.versionId,
    geometryKey, contextVariantKey: node.snapshot?.contextVariantKey ?? node.contextVariantKey,
    instancePath, occurrencePath, rootDocumentId: instancePath?.rootDocumentId,
    instanceId: instancePath?.segments[0]?.instanceId, treeNodeId: node.id,
  };
  switch (node.kind) {
    case "PART": return { ...base, kind: "part", id: node.documentId ?? node.id,
      visualKey: occurrencePath ? `occurrence:${occurrencePath}` : undefined };
    case "PRODUCT": return { ...base, kind: "product", id: node.documentId ?? node.id };
    case "INSTANCE": return { ...base, kind: "instance", id: occurrencePath || node.entityId || node.id,
      visualKey: `occurrence:${occurrencePath || node.entityId}` };
    case "BODY": return { ...base, kind: "body", id: occurrencePath ? `${occurrencePath}:body:${bodyId}` : bodyId ?? node.id,
      visualKey: `body:${occurrencePath ? `${occurrencePath}:body:${bodyId}` : bodyId}` };
    case "SKETCH_PATTERN_MEMBER":
    case "SKETCH":
    case "SKETCH_INPUT_REFERENCE":
      return { ...base, kind: "sketch", id: node.entityId ?? node.id };
    case "SKETCH_PATTERN_DEFINITION": return {...base,kind:"feature",id:node.entityId??node.id};
    case "PAD":
    case "REVOLVE":
    case "IMPORT":
    case "FEATURE":
      return { ...base, kind: node.kind === "IMPORT" ? "import" : "pad", id: node.entityId ?? node.id };
    case "PLANE": {
      if (!node.entityId || !node.plane) break;
      const datumPlane = (view.datumPlanes ?? view.part?.datumPlanes)?.find((datum) => datum.id === node.entityId);
      return { ...base, kind: "plane", id: `${occurrencePath || "root"}:${node.entityId}`,
        plane: node.plane, datumPlane };
    }
    case "AXIS_SYSTEM":
      return { ...base, kind: "axis-system", id: `${occurrencePath || "root"}:${node.entityId}` };
    case "AXIS":
    case "DATUM_AXIS":
      return { ...base, kind: "axis", axis: node.axis ?? "DATUM",
        id: `${occurrencePath || "root"}:${node.entityId}:${node.axis ?? "DATUM"}` };
    case "SKETCH_PATTERN_ENTITY":
    case "SKETCH_ENTITY":
    case "SKETCH_EXTERNAL_GEOMETRY":
      if (node.entityId && node.ownerEntityId) return { ...base, kind: "visual",
        id: `${occurrencePath || "root"}:${node.ownerEntityId}:${node.entityId}`,
        visualType: node.entityType === "POINT" ? "POINT" : "CURVE",
        featureId: node.ownerEntityId, entityId: node.entityId, role: node.role };
      break;
    case "SKETCH_CONSTRAINT":
      if (node.entityId && node.ownerEntityId) return { ...base, kind: "sketch-constraint",
        id: `${occurrencePath || "root"}:${node.ownerEntityId}:constraint:${node.entityId}`,
        featureId: node.ownerEntityId, constraintId: node.entityId,
        constraintType: node.entityType ?? "UNKNOWN" };
      break;
    case "ASSEMBLY_CONSTRAINT":
      if (node.entityId) return { ...base, kind: "assembly-constraint", id: node.entityId,
        constraintId: node.entityId, constraintType: node.entityType ?? "UNKNOWN" };
      break;
    case "PUBLICATION":
    case "PRODUCT_PUBLICATION": {
      const source = node.publication ?? view.part?.publications?.find((candidate) => candidate.id === node.entityId);
      const forwarded = node.productPublication;
      const resolution = source?.resolution ?? forwarded?.resolution;
      const targetPath = forwarded?.target.instancePath ?? instancePath;
      const targetOccurrence = targetPath?.canonical ?? "";
      const targetVariantKey = view.contextVariants?.find((variant) =>
        variant.owningInstancePath.canonical === targetOccurrence)?.variantKey;
      const resolvedTargetBody = source?.target.bodyId ?? source?.target.persistentSelection?.sourceBodyId;
      const matchingBodies = view.resolvedInstances?.filter((candidate) =>
        candidate.occurrencePath === targetOccurrence && candidate.geometryKey === resolution?.geometryKey) ?? [];
      const matchingPartBodies = !targetOccurrence ? view.part?.bodies?.filter((body) =>
        body.geometryKey === resolution?.geometryKey) ?? [] : [];
      const targetBodyId = resolvedTargetBody ?? (matchingBodies.length === 1 ? matchingBodies[0].bodyId :
        matchingPartBodies.length === 1 ? matchingPartBodies[0].id : undefined);
      let highlightTarget: SelectionItem | undefined;
      if (resolution?.status === "CONNECTED" && resolution.topologyKind && resolution.localId &&
          resolution.geometryKey && resolution.resolvedVersionId && targetBodyId) {
        highlightTarget = { kind: resolution.topologyKind.toLowerCase() as "face" | "edge" | "vertex",
          id: `${targetOccurrence || "root"}:${resolution.geometryKey}:${resolution.topologyKind.toLowerCase()}:${resolution.localId}`,
          topologyId: resolution.localId, documentId: node.sourceDocumentId ?? node.documentId,
          bodyId: targetBodyId, instancePath: targetPath,
          occurrencePath: targetOccurrence, geometryKey: resolution.geometryKey,
          contextVariantKey: targetVariantKey,
          versionId: resolution.resolvedVersionId };
      } else if (resolution?.status === "CONNECTED" && source?.target.kind === "BODY_RESULT" && targetBodyId) {
        highlightTarget = {kind:"body", id:targetOccurrence ? `${targetOccurrence}:body:${targetBodyId}` : targetBodyId,
          documentId:node.sourceDocumentId ?? node.documentId, bodyId:targetBodyId,
          instancePath:targetPath, occurrencePath:targetOccurrence,
          geometryKey:resolution.geometryKey, versionId:resolution.resolvedVersionId,
          visualKey:`body:${targetOccurrence ? `${targetOccurrence}:body:${targetBodyId}` : targetBodyId}`};
      } else if (resolution?.status === "CONNECTED" && source?.target.datumId) {
        const targetId = source.target.datumId;
        if (source.type === "PLANE") highlightTarget = { kind: "plane", plane: node.plane ?? "CUSTOM",
          id: `${targetOccurrence || "root"}:${targetId}`, entityId: targetId,
          documentId: node.documentId, instancePath: targetPath, occurrencePath: targetOccurrence };
        if (source.type === "AXIS") highlightTarget = { kind: "axis", axis: source.target.axis ?? "DATUM",
          id: `${targetOccurrence || "root"}:${targetId}:${source.target.axis ?? "DATUM"}`,
          entityId: targetId, documentId: node.documentId, instancePath: targetPath,
          occurrencePath: targetOccurrence };
      }
      return { ...base, kind: "publication", id: node.entityId ?? node.id,
        publicationId: node.entityId, publication: source, highlightTarget };
    }
    case "CONTEXT_REFERENCE": return { ...base, kind: "external-reference" };
    case "CONTEXT_INPUT": return { ...base, kind: "context-input" };
    case "CONTEXT_BINDING": return { ...base, kind: "context-binding" };
    case "PARAMETER": return { ...base, kind: "parameter" };
  }
  return { ...base, kind: "tree", id: node.id };
}

export function findStructureEntity(node: DocumentStructureNode | undefined, entityID: string): DocumentStructureNode | undefined {
  if (!node) return undefined;
  if (node.entityId === entityID && ["SKETCH", "PAD", "REVOLVE", "IMPORT", "FEATURE"].includes(node.kind)) return node;
  for (const child of node.children ?? []) {
    const found = findStructureEntity(child, entityID);
    if (found) return found;
  }
  return undefined;
}

export function findStructureOccurrenceEntity(root: DocumentStructureNode | undefined, entityID: string,
  occurrencePath: string, bodyID?: string): DocumentStructureNode | undefined {
  if (!root) return undefined;
  if (root.entityId === entityID && root.instancePath?.canonical === occurrencePath &&
      (!bodyID || root.bodyId === bodyID) && ["SKETCH", "PAD", "REVOLVE", "IMPORT", "FEATURE"].includes(root.kind)) return root;
  for (const child of root.children ?? []) {
    const found = findStructureOccurrenceEntity(child, entityID, occurrencePath, bodyID);
    if (found) return found;
  }
  return undefined;
}

function mapStructureNode(node: DocumentStructureNode, view: DocumentView, editingView?: DocumentView): SpecificationTreeNode {
  const ownerDocumentID = node.kind === "INSTANCE" ? node.instancePath?.segments.at(-1)?.ownerDocumentId : node.documentId;
  const nodeView = node.documentId === editingView?.document.id ? editingView : node.documentId === view.document.id ? view : undefined;
  const capabilityView = ownerDocumentID === editingView?.document.id ? editingView : ownerDocumentID === view.document.id ? view : nodeView;
  const canEdit = capabilityView?.document.permission === "OWNER" || capabilityView?.document.permission === "EDITOR";
  const sketch=nodeView?.part?.features.find((feature)=>feature.id===node.ownerEntityId)?.sketch;
  const conflictConstraints=new Set(sketch?.solve.conflictingConstraintIds??[]), conflictEntities=new Set<string>();
  let changed=true;while(changed){changed=false;for(const constraint of sketch?.constraints??[]){if(constraint.suppressed)continue;
    const ids=constraint.references.flatMap((reference)=>reference.entityId?[reference.entityId]:[]);
    if(conflictConstraints.has(constraint.id)||ids.some((id)=>conflictEntities.has(id))){if(!conflictConstraints.has(constraint.id)){conflictConstraints.add(constraint.id);changed=true;}
      for(const id of ids)if(!conflictEntities.has(id)){conflictEntities.add(id);changed=true;}}
  }}
	const component=node.entityId?sketch?.solve.components?.find((candidate)=>candidate.entityIds.includes(node.entityId!)):undefined;
  const assemblyStatus = node.kind === "ASSEMBLY_CONSTRAINT"
    ? assemblyStatusFromDiagnostic(node.evaluationStatus ?? node.diagnostic) : undefined;
  const featureStatus = node.evaluationStatus === "FAILED" ? "求值失败" : node.evaluationStatus === "BLOCKED" ? "上游失败" : node.evaluationStatus === "SUPPRESSED" ? "已抑制" : undefined;
  const outputStatus = node.consumed ? "已用于布尔" : featureStatus;
  return { key: node.id, title: outputStatus ? <>{node.name}<span title={node.diagnostic} aria-label={`状态 ${outputStatus}`}> · {outputStatus}</span></> : assemblyStatus ? <>{node.name}<span className={`assembly-tree-status status-${node.suppressed ? "not_updated" : assemblyStatus.toLowerCase()}`}
    aria-label={`状态 ${node.suppressed ? "停用" : ASSEMBLY_CONSTRAINT_STATUS[assemblyStatus].label}`}>{node.suppressed ? "停用" : ASSEMBLY_CONSTRAINT_STATUS[assemblyStatus].label}</span></> : node.name,
    icon: treeNodeIcon(node), kind: node.kind,
    entityId: node.entityId, documentId: node.documentId, documentType: node.documentType, instancePath: node.instancePath,
    ownerDocumentId: node.ownerDocumentId, bodyId: node.bodyId, presentationRole: node.presentationRole,
    localVisible: node.localVisible, visibilityMode: node.visibilityMode,
    childrenState: node.childrenState, resolutionStatus: node.resolutionStatus,
    connectionStatus: node.connectionStatus, currencyStatus: node.currencyStatus,
    evaluationStatus: node.evaluationStatus,
    sourceDocumentId: node.sourceDocumentId, sourceRevisionId: node.sourceRevisionId,
    sourceDisplayPath: node.sourceDisplayPath,
    plane: node.plane, ownerEntityId: node.ownerEntityId,
	role: node.role, definitionDigest: node.definitionDigest, suppressed: node.suppressed, diagnostic: node.diagnostic??(node.kind==="SKETCH_ENTITY"&&node.entityId&&conflictEntities.has(node.entityId)?"CONFLICTING":component?.definitionStatus==="FULLY_CONSTRAINED"||component?.status==="SOLVED"?"FULLY_CONSTRAINED":undefined),
    capabilities: canEdit ? [...new Set([...(node.capabilities??[]), ...(["SKETCH","SKETCH_GEOMETRY_SET","SKETCH_CONSTRAINT_SET","SKETCH_LOGICAL_CONSTRAINT_SET","SKETCH_DIMENSION_SET"].includes(node.kind)?["SUPPRESS" as const]:[])])] : undefined,
    selection: structureSelection(node, view), children: node.children?.map((child) => mapStructureNode(child, view, editingView)) };
}

export function treeData(view: DocumentView, editingView?: DocumentView): SpecificationTreeNode[] {
  return view.structureTree ? [mapStructureNode(view.structureTree, view, editingView)] : [];
}

export function selectedFeature(view: DocumentView, selection: Selection): Feature | undefined {
  if (!selection || !["sketch", "pad", "import"].includes(selection.kind)) return undefined;
  if (selection.documentId !== view.document?.id || selection.versionId !== view.document.versionId) return undefined;
  return view.part?.features.find((feature) => feature.id === selection.id);
}

export { deletableTreeNodesForSelections, ancestorHintKeysForSelections, treeKeyForSelection, treeKeysForSelections } from "./tree-projection-index";
