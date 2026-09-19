import {
  AimOutlined,
  ApartmentOutlined,
  BuildOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  CloudUploadOutlined,
  DatabaseOutlined,
  ExclamationCircleOutlined,
  GatewayOutlined,
  InsertRowAboveOutlined,
  NodeIndexOutlined,
  ScissorOutlined,
  SyncOutlined,
} from "@ant-design/icons";
import { ASSEMBLY_CONSTRAINT_STATUS, assemblyStatusFromDiagnostic } from "../../cad/assembly/assembly-constraint-ux";
import type { DocumentStructureNode, DocumentView, Feature, Selection, SelectionItem } from "../../types";
import type { SpecificationTreeNode } from "./specification-tree";
import { closestTreeKey } from "./tree-selection";

function featureIcon(feature: Feature) {
  if (feature.type.toUpperCase().includes("SKETCH")) return <ScissorOutlined />;
  if (["PAD", "LINEAR_EXTRUDE", "REVOLVE"].includes(feature.type.toUpperCase())) return <InsertRowAboveOutlined />;
  return <CloudUploadOutlined />;
}

export function isSolidFeature(feature: Feature): boolean {
  return ["PAD", "LINEAR_EXTRUDE", "REVOLVE", "IMPORT_BODY"].includes(feature.type.toUpperCase());
}

function featureNode(feature: Feature): SpecificationTreeNode {
  return {
    key: `${feature.type.toUpperCase().includes("SKETCH") ? "sketch" : isSolidFeature(feature) ? "solid" : "import"}:${feature.id}`,
    title: feature.name ?? feature.type, icon: featureIcon(feature),
  };
}

function structureIcon(kind: DocumentStructureNode["kind"], diagnostic?: string) {
  if (kind === "PRODUCT") return <ApartmentOutlined />;
  if (kind === "PART" || kind === "INSTANCE") return <BuildOutlined />;
  if (kind === "ORIGIN") return <GatewayOutlined />;
  if (kind === "PLANE") return <NodeIndexOutlined />;
  if (kind === "AXIS_SYSTEM") return <AimOutlined />;
  if (kind === "AXIS") return <NodeIndexOutlined />;
  if (kind === "BODY") return <DatabaseOutlined />;
  if (kind === "PUBLICATION_SET" || kind === "PUBLICATION" || kind === "PRODUCT_PUBLICATION_SET" || kind === "PRODUCT_PUBLICATION" ||
      kind === "CONTEXT_REFERENCE_SET" || kind === "CONTEXT_REFERENCE") return <GatewayOutlined />;
  if (kind === "SKETCH") return <ScissorOutlined />;
  if (kind === "SKETCH_ENTITY") return <NodeIndexOutlined />;
  if (kind === "SKETCH_CONSTRAINT") return <GatewayOutlined />;
  if (kind === "SKETCH_GEOMETRY_SET" || kind === "SKETCH_CONSTRAINT_SET") return <DatabaseOutlined />;
  if (kind === "PAD" || kind === "REVOLVE") return <InsertRowAboveOutlined />;
  if (kind === "ASSEMBLY_CONSTRAINT") {
    const status = assemblyStatusFromDiagnostic(diagnostic);
    const detail = ASSEMBLY_CONSTRAINT_STATUS[status];
    const icon = status === "VERIFIED" ? <CheckCircleOutlined /> : status === "BROKEN" ? <CloseCircleOutlined />
      : status === "IMPOSSIBLE" ? <ExclamationCircleOutlined /> : <SyncOutlined />;
    return <span aria-label={`Constraint status ${detail.label}`} title={`${detail.label}: ${detail.description}`}
      style={{ color: detail.color }}>{icon}</span>;
  }
  return <CloudUploadOutlined />;
}

export function structureSelection(node: DocumentStructureNode, view: DocumentView): Selection {
  const occurrencePath = node.instancePath?.canonical ?? "";
  const resolved = (view.resolvedInstances ?? []).find((item) => item.instancePath?.canonical === occurrencePath);
  const geometryKey = resolved?.geometryKey ?? view.artifact?.geometryKey;
  const expands = ["PART", "PRODUCT", "INSTANCE", "ORIGIN", "BODY", "PUBLICATION_SET", "PRODUCT_PUBLICATION_SET", "CONTEXT_REFERENCE_SET", "SKETCH", "SKETCH_GEOMETRY_SET", "SKETCH_CONSTRAINT_SET",
    "SKETCH_LOGICAL_CONSTRAINT_SET", "SKETCH_DIMENSION_SET"].includes(node.kind);
  const context = { treeNodeId: node.id, expandTreeDescendants: expands || undefined, documentId: node.documentId,
    versionId: node.versionId, instancePath: node.instancePath, occurrencePath, geometryKey,
    instanceId: occurrencePath.split("/")[0] || undefined };
  if (node.kind === "PRODUCT_PUBLICATION" && node.entityId && node.productPublication) {
    const productPublication = node.productPublication;
    const targetPath = productPublication.target.instancePath;
    const targetOccurrence = targetPath.canonical;
    const publication = {
      id: productPublication.id, name: productPublication.name, type: productPublication.type,
      semanticPurpose: productPublication.semanticPurpose, compatibilityVersion: productPublication.compatibilityVersion,
      target: { kind: productPublication.resolution.topologyKind ? "TOPOLOGY" as const : "DATUM" as const,
        sourceVersionId: productPublication.resolution.resolvedVersionId },
      contract: productPublication.contract, resolution: productPublication.resolution,
    };
    const publicationContext = { ...context, occurrencePath: targetOccurrence, instancePath: targetPath,
      instanceId: targetOccurrence.split("/")[0] || undefined, publicationId: productPublication.id, publication,
      geometryKey: productPublication.resolution.geometryKey };
    if (productPublication.resolution.status === "CONNECTED" && productPublication.resolution.topologyKind &&
      productPublication.resolution.localId !== undefined) return {
      kind: productPublication.resolution.topologyKind.toLowerCase() as "face" | "edge" | "vertex",
      topologyId: productPublication.resolution.localId,
      id: `${targetOccurrence}:${productPublication.resolution.geometryKey}:${productPublication.resolution.topologyKind.toLowerCase()}:${productPublication.resolution.localId}`,
      ...publicationContext,
    };
    if (productPublication.type === "PLANE" && productPublication.resolution.geometryId) return {
      kind: "plane", plane: "CUSTOM", entityId: productPublication.resolution.geometryId,
      id: `${targetOccurrence}:${productPublication.resolution.geometryId}`, ...publicationContext,
    };
    if (productPublication.type === "AXIS" && productPublication.resolution.geometryId) return {
      kind: "axis", axis: "DATUM", entityId: productPublication.resolution.geometryId,
      id: `${targetOccurrence}:${productPublication.resolution.geometryId}`, ...publicationContext,
    };
    return { kind: "tree", id: node.id, ...publicationContext };
  }
  if (node.kind === "PUBLICATION" && node.entityId) {
    const publication = node.publication ?? view.part?.publications?.find((candidate) => candidate.id === node.entityId);
    const publicationContext = { ...context, publicationId: node.entityId, publication };
    if (publication?.target.kind === "TOPOLOGY" && publication.resolution.status === "CONNECTED" &&
      publication.resolution.topologyKind && publication.resolution.localId !== undefined) {
      return { kind: publication.resolution.topologyKind.toLowerCase() as "face" | "edge" | "vertex",
        topologyId: publication.resolution.localId,
        id: `${occurrencePath || "root"}:${publication.resolution.geometryKey}:${publication.resolution.topologyKind.toLowerCase()}:${publication.resolution.localId}`,
        ...publicationContext, geometryKey: publication.resolution.geometryKey };
    }
    if (publication?.target.kind === "DATUM" && publication.target.datumId) {
      if (publication.type === "PLANE") {
        const datumPlane = view.datumPlanes?.find((datum) => datum.id === publication.target.datumId);
        return { kind: "plane", plane: datumPlane?.plane ?? node.plane ?? "CUSTOM", datumPlane, entityId: publication.target.datumId,
          id: `${occurrencePath || "root"}:${publication.target.datumId}`, ...publicationContext };
      }
      if (publication.type === "FRAME") return { kind: "axis-system", entityId: publication.target.datumId,
        id: `${occurrencePath || "root"}:${publication.target.datumId}`, ...publicationContext };
      if (publication.type === "AXIS") return { kind: "axis", axis: publication.target.axis ?? "DATUM",
        entityId: publication.target.datumId,
        id: `${occurrencePath || "root"}:${publication.target.datumId}${publication.target.axis ? `:${publication.target.axis}` : ""}`,
        ...publicationContext };
    }
    if (publication?.type === "BODY") return { kind: "body", id: `${occurrencePath || "root"}:body`,
      ...publicationContext, geometryKey: publication.resolution.geometryKey };
    return { kind: "tree", id: node.id, ...publicationContext };
  }
  if (node.kind === "INSTANCE" && node.entityId) {
    const path = occurrencePath || node.entityId;
    return { kind: "instance", id: path, visualKey: `occurrence:${path}`, ...context,
      occurrencePath: path, instanceId: path.split("/")[0] };
  }
  if (node.kind === "PLANE" && node.entityId && node.plane) {
    const datumPlane = view.datumPlanes?.find((datum) => datum.id === node.entityId);
    return { kind: "plane", id: `${occurrencePath || "root"}:${node.entityId}`, entityId: node.entityId, plane: node.plane, datumPlane, ...context };
  }
  if (node.kind === "AXIS_SYSTEM" && node.entityId) return {
    kind: "axis-system", id: `${occurrencePath || "root"}:${node.entityId}`, entityId: node.entityId, ...context,
  };
  if (node.kind === "AXIS" && node.entityId && node.axis) return {
    kind: "axis", axis: node.axis, id: `${occurrencePath || "root"}:${node.entityId}:${node.axis}`, entityId: node.entityId, ...context,
  };
  if (node.kind === "DATUM_AXIS" && node.entityId) return {
    kind: "axis", axis: "DATUM", id: `${occurrencePath || "root"}:${node.entityId}`, entityId: node.entityId, ...context,
  };
  const bodyID = `${occurrencePath || "root"}:body`;
  if (node.kind === "BODY" || node.kind === "PART") return { kind: "body", id: bodyID, ...context };
  if (["SKETCH", "PAD", "REVOLVE", "IMPORT"].includes(node.kind) && node.entityId) return {
    kind: (node.kind === "REVOLVE" ? "pad" : node.kind.toLowerCase()) as "sketch" | "pad" | "import", id: node.entityId,
    visualKey: node.kind === "SKETCH" ? undefined : `body:${bodyID}`, ...context,
  };
  if ((node.kind === "SKETCH_ENTITY" || node.kind === "SKETCH_EXTERNAL_GEOMETRY") && node.entityId && node.ownerEntityId) return {
    kind: "visual", id: `${occurrencePath || "root"}:${node.ownerEntityId}:${node.entityId}`,
    visualType: node.entityType === "POINT" ? "POINT" : "CURVE", featureId: node.ownerEntityId,
    entityId: node.entityId, role: node.role, ...context,
  };
  if (node.kind === "SKETCH_CONSTRAINT" && node.entityId && node.ownerEntityId) return {
    kind: "sketch-constraint", id: `${occurrencePath || "root"}:${node.ownerEntityId}:constraint:${node.entityId}`,
    featureId: node.ownerEntityId, constraintId: node.entityId, constraintType: node.entityType ?? "UNKNOWN", ...context,
  };
  if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) return { kind: "assembly-constraint", id: node.entityId,
    constraintId: node.entityId, constraintType: node.entityType ?? "UNKNOWN", ...context };
  return { kind: "tree", id: node.id, ...context };
}

export function findStructureEntity(node: DocumentStructureNode | undefined, entityID: string): DocumentStructureNode | undefined {
  if (!node) return undefined;
  if (node.entityId === entityID && ["SKETCH", "PAD", "REVOLVE", "IMPORT"].includes(node.kind)) return node;
  for (const child of node.children ?? []) {
    const found = findStructureEntity(child, entityID);
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
  const assemblyStatus = node.kind === "ASSEMBLY_CONSTRAINT" ? assemblyStatusFromDiagnostic(node.diagnostic) : undefined;
  return { key: node.id, title: assemblyStatus ? <>{node.name}<span className={`assembly-tree-status status-${assemblyStatus.toLowerCase()}`}
    aria-label={`状态 ${ASSEMBLY_CONSTRAINT_STATUS[assemblyStatus].label}`}>{ASSEMBLY_CONSTRAINT_STATUS[assemblyStatus].label}</span></> : node.name,
    icon: structureIcon(node.kind, node.diagnostic), kind: node.kind,
    entityId: node.entityId, documentId: node.documentId, documentType: node.documentType, instancePath: node.instancePath,
    plane: node.plane, ownerEntityId: node.ownerEntityId,
	role: node.role, definitionDigest: node.definitionDigest, suppressed: node.suppressed, diagnostic: node.diagnostic??(node.kind==="SKETCH_ENTITY"&&node.entityId&&conflictEntities.has(node.entityId)?"CONFLICTING":component?.definitionStatus==="FULLY_CONSTRAINED"||component?.status==="SOLVED"?"FULLY_CONSTRAINED":undefined),
    capabilities: canEdit ? [...new Set([...(node.capabilities??[]), ...(["SKETCH","SKETCH_GEOMETRY_SET","SKETCH_CONSTRAINT_SET","SKETCH_LOGICAL_CONSTRAINT_SET","SKETCH_DIMENSION_SET"].includes(node.kind)?["SUPPRESS" as const]:[])])] : undefined,
    selection: structureSelection(node, view), children: node.children?.map((child) => mapStructureNode(child, view, editingView)) };
}

export function treeData(view: DocumentView, editingView?: DocumentView): SpecificationTreeNode[] {
  if (view.structureTree) return [mapStructureNode(view.structureTree, view, editingView)];
  if (view.document.type === "PART") {
    const features = view.part?.features ?? [];
    const sketches = new Map(features.filter((feature) => feature.type.toUpperCase().includes("SKETCH"))
      .map((feature) => [feature.id, feature]));
    const consumedSketches = new Set(features.filter((feature) => isSolidFeature(feature) && feature.profile)
      .map((feature) => feature.profile!));
    const bodyFeatures = features.filter((feature) => !consumedSketches.has(feature.id)).map((feature) => {
      const node = featureNode(feature);
      const profile = isSolidFeature(feature) && feature.profile ? sketches.get(feature.profile) : undefined;
      return profile ? { ...node, children: [featureNode(profile)] } : node;
    });
    return [{ key: "document", title: view.document.name, icon: <BuildOutlined />, children: [
      { key: "origin", title: "Origin", icon: <GatewayOutlined />, children: (view.datumPlanes ?? []).map((plane) => ({
        key: `plane:${plane.id}:${plane.plane}`, title: plane.name, icon: <NodeIndexOutlined />,
      })).concat((view.axisSystems ?? []).map((axis) => ({
        key: `axis:${axis.id}`, title: axis.name, icon: <AimOutlined />,
      }))) },
      { key: "body", title: "PartBody", icon: <DatabaseOutlined />, children: bodyFeatures },
    ] }];
  }
  return [{ key: "document", title: view.document.name, icon: <ApartmentOutlined />, kind: "PRODUCT",
    documentId: view.document.id, documentType: "PRODUCT", capabilities: view.document.permission === "OWNER" || view.document.permission === "EDITOR" ? ["CREATE_PART"] : undefined,
    children:
    (view.product?.instances ?? []).map((instance) => ({
      key: `instance:${instance.id}`, title: instance.name, icon: <BuildOutlined />,
    })) }];
}

export function selectedFeature(view: DocumentView, selection: Selection): Feature | undefined {
  if (!selection || !["sketch", "pad", "import"].includes(selection.kind)) return undefined;
  return view.part?.features.find((feature) => feature.id === selection.id);
}

export function treeKeyForSelection(nodes: SpecificationTreeNode[], selection: Selection): string | undefined {
  if (!selection) return undefined;
  const visit = (items: SpecificationTreeNode[]): string | undefined => {
    for (const node of items) {
      if (selection.treeNodeId === node.key || (selection.documentId && node.documentId === selection.documentId &&
        selection.id === node.entityId)) return node.key;
      const child = node.children ? visit(node.children) : undefined;
      if (child) return child;
    }
    return undefined;
  };
  return visit(nodes) ?? closestTreeKey(nodes, selection.treeNodeId);
}

export function treeKeysForSelections(nodes: SpecificationTreeNode[], selections: readonly SelectionItem[]): string[] {
  const keys = new Set<string>();
  const all: SpecificationTreeNode[] = [];
  const visit = (items: SpecificationTreeNode[]) => items.forEach((node) => { all.push(node); if (node.children) visit(node.children); });
  visit(nodes);
  for (const selection of selections) {
    const primary = treeKeyForSelection(nodes, selection);
    if (primary) keys.add(primary);
    const publications = all.filter((node) => (node.kind === "PUBLICATION" || node.kind === "PRODUCT_PUBLICATION") &&
      node.selection?.publicationId && (selection.publicationId
        ? node.selection.publicationId === selection.publicationId
        : samePublishedGeometry(node.selection, selection)));
    for (const publication of publications) {
      keys.add(publication.key);
      const body = all.find((node) => node.kind === "BODY" &&
        (node.instancePath?.canonical ?? "") === (publication.selection?.occurrencePath ?? selection.occurrencePath ?? ""));
      if (body) keys.add(body.key);
    }
  }
  return [...keys];
}

function samePublishedGeometry(candidate: SelectionItem, selection: SelectionItem): boolean {
  if (candidate.kind !== selection.kind || (candidate.occurrencePath ?? "") !== (selection.occurrencePath ?? "")) return false;
  if (["face", "edge", "vertex"].includes(candidate.kind)) return "topologyId" in candidate && "topologyId" in selection &&
    candidate.topologyId === selection.topologyId && (!candidate.geometryKey || !selection.geometryKey || candidate.geometryKey === selection.geometryKey);
  if (["plane", "axis", "axis-system", "body"].includes(candidate.kind)) return candidate.entityId === selection.entityId || candidate.kind === "body";
  return false;
}
