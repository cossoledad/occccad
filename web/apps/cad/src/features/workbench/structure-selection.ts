import type {DocumentStructureNode,DocumentView,Selection,SelectionItem} from "../../types";

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
    entityKind: node.subject?.entityKind ?? node.kind, entityType: node.entityType,
    entityRef: node.subject, occurrenceRef: node.occurrence, snapshotScope: node.snapshot,
    documentId: node.documentId, ownerDocumentId: node.ownerDocumentId ?? node.documentId,
    bodyId, versionId: node.snapshot?.revisionId ?? node.versionId,
    geometryKey, contextVariantKey: node.snapshot?.contextVariantKey ?? node.contextVariantKey,
    instancePath, occurrencePath, rootDocumentId: instancePath?.rootDocumentId,
    instanceId: instancePath?.segments[0]?.instanceId, treeNodeId: node.id,
  };
  switch (node.kind) {
    case "ASSEMBLY_GROUP_MEMBER_REFERENCE": {
      if(!node.entityId)return null;
      const target:SelectionItem=node.entityType==='FIX_TOGETHER'?{kind:'assembly-constraint',id:node.entityId,constraintId:node.entityId,constraintType:'FIX_TOGETHER',documentId:node.documentId,occurrencePath}: {kind:'instance',id:[occurrencePath,node.entityId].filter(Boolean).join('/'),instanceId:node.entityId,occurrencePath:[occurrencePath,node.entityId].filter(Boolean).join('/'),visualKey:'occurrence:'+ [occurrencePath,node.entityId].filter(Boolean).join('/')};
      return {...base,kind:'tree',id:node.id,entityKind:node.kind,ownerEntityId:node.ownerEntityId,mechanismId:node.mechanismId,highlightTarget:target};
    }
 case "MECHANISM_REFERENCE":case "JOINT_EXPANSION":case "JOINT_SUPPORT":case "MECHANISM":case "MECHANISM_JOINT":case "MOTION_DRIVER":case "MOTION_STUDY":case "INTERFERENCE_ANALYSIS": return {...base,kind:"kinematic-object",id:node.entityId??node.id,mechanismId:node.mechanismId,ownerEntityId:node.ownerEntityId,role:node.relationRole};
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
      if(node.kind==="SKETCH_INPUT_REFERENCE"&&node.inputEntityId&&node.entityId)return {...base,kind:"visual",id:`${occurrencePath||"root"}:${node.entityId}:${node.inputEntityId}`,featureId:node.entityId,entityId:node.inputEntityId,visualType:node.inputRole==="AXIS"?"CURVE":"POINT"};
      return { ...base, kind: "sketch", id: node.entityId ?? node.id };
    case "SKETCH_PATTERN_DEFINITION": return {...base,kind:"feature",id:node.entityId??node.id};
    case "PAD":
    case "REVOLVE":
    case "IMPORT":
    case "FEATURE_INPUT_REFERENCE":
    case "FEATURE":
      return { ...base, kind: node.kind === "IMPORT" ? "import" : "pad", id: node.entityId ?? node.id };
    case "DATUM_INPUT_REFERENCE":
      if(node.entityType==="PLANE")return structureSelection({...node,kind:"PLANE"},view);
      if(node.entityType==="DATUM_AXIS")return structureSelection({...node,kind:"DATUM_AXIS"},view);
      if(node.entityType==="AXIS_SYSTEM")return structureSelection({...node,kind:"AXIS"},view);
      break;
    case "PLANE": {
      if (!node.entityId || !node.plane) break;
      const datumPlane = (view.datumPlanes ?? view.part?.datumPlanes)?.find((datum) => datum.id === node.entityId);
      return { ...base, kind: "plane", id: `${occurrencePath || "root"}:${node.entityId}`,
        plane: node.plane, datumPlane };
    }
    case "DATUM_POINT": return {...base,kind:"datum-point",id:`${occurrencePath||"root"}:${node.entityId}:point`};
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
      const forwarded = node.productPublication;
      const source = node.publication ?? view.part?.publications?.find((candidate) => candidate.id === node.entityId);
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
      if(resolution?.status==='CONNECTED'&&source?.target.kind==='SKETCH_ENTITY'&&source.target.sketchId&&source.target.entityId){
       highlightTarget={kind:'visual',visualType:source.target.entityKind==='POINT'?'POINT':'CURVE',id:source.target.entityId,featureId:source.target.sketchId,entityId:source.target.entityId,documentId:node.sourceDocumentId??node.documentId,versionId:resolution.resolvedVersionId,instancePath:targetPath,occurrencePath:targetOccurrence,contextVariantKey:targetVariantKey};
      } else if (resolution?.status === "CONNECTED" && resolution.topologyKind && resolution.localId &&
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
          documentId: node.sourceDocumentId ?? node.documentId, versionId: resolution.resolvedVersionId, instancePath: targetPath, occurrencePath: targetOccurrence };
        if (source.type === "AXIS") highlightTarget = { kind: "axis", axis: source.target.axis ?? "DATUM",
          id: `${targetOccurrence || "root"}:${targetId}:${source.target.axis ?? "DATUM"}`,
          entityId: targetId, documentId: node.sourceDocumentId ?? node.documentId, versionId: resolution.resolvedVersionId, instancePath: targetPath,
          occurrencePath: targetOccurrence };
      }
      if (forwarded && view.structureTree) {
        const canonical=[instancePath?.canonical,forwarded.target.instancePath.canonical].filter(Boolean).join('/');
        const find=(n:DocumentStructureNode):DocumentStructureNode|undefined=>{
          if((n.kind==='PUBLICATION'||n.kind==='PRODUCT_PUBLICATION')&&n.entityId===forwarded.target.publicationId&&n.instancePath?.canonical===canonical)return n;
          for(const child of n.children??[]){const found=find(child);if(found)return found}
        };
        const child=find(view.structureTree);
        if(child&&child!==node)highlightTarget=structureSelection(child,view)?.highlightTarget;
      }
      return { ...base, kind: "publication", id: node.entityId ?? node.id,
        publicationId: node.entityId, publication: source, productPublication: forwarded, highlightTarget };
    }
    case "CONTEXT_REFERENCE": return { ...base, kind: "external-reference" };
    case "CONTEXT_INPUT": return { ...base, kind: "context-input" };
    case "CONTEXT_BINDING": return { ...base, kind: "context-binding" };
    case "PARAMETER": return { ...base, kind: "parameter" };
  }
  return { ...base, kind: "tree", id: node.id };
}

