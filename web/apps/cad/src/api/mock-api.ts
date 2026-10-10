import { encodeMeshGLB } from "../cad/visual/mesh-glb";
import type { CadApi } from "../api";
import type {
  Artifact, AssemblyConstraint, AssemblyGeometryRef, ContextCatalog, DocumentProperties, DocumentStructureNode, DocumentSummary, DocumentView,
  Feature, FolderSummary, HistoryEntry, Job, InstancePath, ProductInstance, Publication, ShareGrant, SketchOperation, User, Vec3,
} from "../types";
import { sampleSketchEntity } from "../cad/sketch/sketch-geometry";
import { randomUUID } from "../utils/random-uuid";
import { mockToolbarCatalog } from "./mock-toolbar-catalog";

const pause = async <T>(value: T, milliseconds = 90): Promise<T> =>
  new Promise((resolve) => window.setTimeout(() => resolve(structuredClone(value)), milliseconds));
const now = (): string => new Date().toISOString();
const id = (prefix: string): string => `${prefix}-${randomUUID()}`;
const lengthInMillimeters = (value: unknown, unit: unknown): number => Number(value) * ({ mm: 1, cm: 10, m: 1000, in: 25.4 }[String(unit || "mm").toLowerCase()] ?? 1);
const lengthDimension = { Length: 1, Mass: 0, Time: 0, Current: 0, Temperature: 0, Amount: 0, Luminous: 0, Semantic: "" };
const mockInstancePath = (rootDocumentId: string, instance: ProductInstance): InstancePath => ({
  rootDocumentId, canonical: instance.id, display: instance.name, segments: [{
    ownerDocumentId: rootDocumentId, ownerVersionId: "mock-product-v3", instanceId: instance.id,
    instanceName: instance.name, referencedDocumentId: instance.documentId, resolvedVersionId: instance.versionId,
  }],
});
const appendMockInstancePath = (owner: DocumentView, instance: ProductInstance, parent?: InstancePath): InstancePath => {
  const segments = [...(parent?.segments ?? []), {
    ownerDocumentId: owner.document.id, ownerVersionId: owner.document.versionId,
    instanceId: instance.id, instanceName: instance.name,
    referencedDocumentId: instance.documentId, resolvedVersionId: instance.versionId,
  }];
  return { rootDocumentId: parent?.rootDocumentId ?? owner.document.id,
    canonical: segments.map((segment) => segment.instanceId).join("/"),
    display: segments.map((segment) => segment.instanceName).join("/"), segments };
};
const withMockOccurrence = (node: DocumentStructureNode, path?: InstancePath): DocumentStructureNode => {
  if (!path) return node;
  node.instancePath ??= path;
  node.children?.forEach((child) => withMockOccurrence(child, child.instancePath ?? path));
  return node;
};
const mockTopologyProperties = (kind: "FACE" | "EDGE" | "VERTEX"): Record<string, number | boolean | string | Vec3> => {
  if (kind === "FACE") return { area: 100, origin: [0, 0, 0], normal: [0, 0, 1], xDirection: [1, 0, 0] };
  if (kind === "EDGE") return { length: 10, direction: [1, 0, 0] };
  return { tolerance: 1e-7 };
};
const datumPlanes = [
  { id: "datum-xy", name: "XY Plane", plane: "XY" as const, origin: [0, 0, 0] as Vec3, normal: [0, 0, 1] as Vec3, uDirection: [1, 0, 0] as Vec3, size: 180 },
  { id: "datum-xz", name: "XZ Plane", plane: "XZ" as const, origin: [0, 0, 0] as Vec3, normal: [0, -1, 0] as Vec3, uDirection: [1, 0, 0] as Vec3, size: 180 },
  { id: "datum-yz", name: "YZ Plane", plane: "YZ" as const, origin: [0, 0, 0] as Vec3, normal: [1, 0, 0] as Vec3, uDirection: [0, 1, 0] as Vec3, size: 180 },
];
const axisSystems = [{ id: "axis-system-default", name: "Absolute Axis System", origin: [0, 0, 0] as Vec3,
  xDirection: [1, 0, 0] as Vec3, yDirection: [0, 1, 0] as Vec3, zDirection: [0, 0, 1] as Vec3 }];

const administrator: User = {
  id: "mock-admin", email: "admin@occccad.local", displayName: "CAD Designer",
  status: "ACTIVE", platformRole: "ADMIN", createdAt: now(),
};
const users: User[] = [administrator, {
  id: "mock-member", email: "engineer@occccad.local", displayName: "Mechanical Engineer",
  status: "ACTIVE", platformRole: "MEMBER", createdAt: now(),
}];

function boxArtifact(key: string, size: Vec3, sketchFeatureId?: string): Artifact {
  const [x, y, z] = size;
  const vertices: Vec3[] = [[0, 0, 0], [x, 0, 0], [x, y, 0], [0, y, 0], [0, 0, z], [x, 0, z], [x, y, z], [0, y, z]];
  const display:Artifact & {mesh: import("../types").MeshData} = {
    representationKind:"PERSISTENT",representations:{},triangleCount:12,displayVertexCount:8,
    geometryKey: key, geometryId: `geometry-${key}`,
    mesh: {
      vertices,
      triangles: [[0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
        [1, 2, 6], [1, 6, 5], [2, 3, 7], [2, 7, 6], [3, 0, 4], [3, 4, 7]],
      faceIds: [1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6],
      edges: [
        [0, 1], [1, 2], [2, 3], [3, 0], [4, 5], [5, 6], [6, 7], [7, 4], [0, 4], [1, 5], [2, 6], [3, 7],
      ].map(([a, b], index) => ({ localId: index + 1, points: [vertices[a], vertices[b]] })),
      topologyVertices: vertices.map((point, index) => ({ localId: index + 1, point })),
    },
    bbox: { min: [0, 0, 0], max: size },
    topology: { faces: 6, edges: 12, vertices: 8, solids: 1 },
    volume: x * y * z, occtVersion: "mock-7.9.1", glbBytes: 4096, brepBytes: 2048,
    evaluatorVersion: "mock-v1", workerId: "mock-geometry-1", storageState: "OBJECT", createdAt: now(),
    visualization: { schemaVersion: 1, referenceGeometry: { datumPlanes, axisSystems }, primitives:
      sketchFeatureId ? [
        {id:"bottom",featureId:sketchFeatureId,kind:"POLYLINE",semantic:"SKETCH_CURVE",entityType:"LINE",role:"PROFILE",status:"UNDER_CONSTRAINED",positions:[[0,0,0],[x,0,0]],selectable:true},
        {id:"right",featureId:sketchFeatureId,kind:"POLYLINE",semantic:"SKETCH_CURVE",entityType:"LINE",role:"PROFILE",status:"UNDER_CONSTRAINED",positions:[[x,0,0],[x,y,0]],selectable:true},
        {id:"top",featureId:sketchFeatureId,kind:"POLYLINE",semantic:"SKETCH_CURVE",entityType:"LINE",role:"PROFILE",status:"UNDER_CONSTRAINED",positions:[[x,y,0],[0,y,0]],selectable:true},
        {id:"left",featureId:sketchFeatureId,kind:"POLYLINE",semantic:"SKETCH_CURVE",entityType:"LINE",role:"PROFILE",status:"UNDER_CONSTRAINED",positions:[[0,y,0],[0,0,0]],selectable:true},
      ] : [] },
  };
  const glb=encodeMeshGLB(display.mesh,display.visualization);
  const url=URL.createObjectURL(new Blob([glb],{type:"model/gltf-binary"}));
  const {mesh,...descriptor}=display;
  descriptor.representations={VISUAL:{objectId:key,digest:key,schemaVersion:2,size:glb.byteLength,contentType:"model/gltf-binary",url}};
  if(key.startsWith("mock-preview")){descriptor.representationKind="TRANSIENT_PREVIEW";}
  return descriptor;
}

const partID = "mock-part-bracket";
const productID = "mock-product-frame";
const partArtifact = boxArtifact("mock-bracket-v3", [72, 38, 16], "mock-sketch-1");
const summaries: DocumentSummary[] = [
  {
    id: partID, name: "Mounting Bracket", description: "参数化安装支架", type: "PART",
    versionId: "mock-part-v3", canUndo: false, canRedo: false, createdAt: now(), lastUpdated: now(),
    workspaceName: "Main", permission: "OWNER",
  },
  {
    id: productID, name: "Frame Assembly", description: "支架装配示例", type: "PRODUCT",
    versionId: "mock-product-v3", canUndo: false, canRedo: false, createdAt: now(), lastUpdated: now(),
    workspaceName: "Main", permission: "OWNER",
  },
];

const views = new Map<string, DocumentView>([
  [partID, {
    document: summaries[0],
    datumPlanes, axisSystems,
    part: { bodies:[{id:"body-main",name:"Body.1",visible:true,geometryKey:partArtifact.geometryKey}],activeBodyId:"body-main",units: "mm", datumPlanes, axisSystems, features: [
      { id: "mock-sketch-1", type: "SKETCH", name: "Sketch 1", plane: "XY", sketch: { schemaVersion: 2, support: { type: "DATUM_PLANE", datumPlaneId: "datum-xy", plane: "XY", status: "CONNECTED" }, entities: [
        { id:"bottom",kind:"LINE",role:"PROFILE",start:{x:0,y:0},end:{x:72,y:0} }, { id:"right",kind:"LINE",role:"PROFILE",start:{x:72,y:0},end:{x:72,y:38} },
        { id:"top",kind:"LINE",role:"PROFILE",start:{x:72,y:38},end:{x:0,y:38} }, { id:"left",kind:"LINE",role:"PROFILE",start:{x:0,y:38},end:{x:0,y:0} },
      ], constraints: [], solve:{status:"UNDER_CONSTRAINED",degreesOfFreedom:4} } },
      { id: "mock-pad-1", type: "PAD", name: "Extrude 1", profile: "mock-sketch-1", length: 16, operation: "ADD" },
    ], parameters: [{ parameterId: "parameter:mock-pad-1:length", key: "mock_pad_1_length", label: "Length",
      valueType: "QUANTITY", dimension: lengthDimension, displayUnit: "mm", role: "INPUT",
      source: { literal: { siValue: 0.016, dimension: lengthDimension } },
      evaluatedValue: { siValue: 0.016, dimension: lengthDimension } }] },
    artifacts: {[partArtifact.geometryKey]:{...partArtifact,bodyId:"body-main"}},
  }],
  [productID, {
    document: summaries[1], product: { instances: [
      { id: "mock-instance-a", name: "Bracket A", documentId: partID, versionId: "mock-part-v3", translation: [-45, 0, 0], referenceMode: "FOLLOW_HEAD" },
      { id: "mock-instance-b", name: "Bracket B", documentId: partID, versionId: "mock-part-v3", translation: [45, 0, 0], referenceMode: "FOLLOW_HEAD" },
    ], constraints: [{ id: "mock-constraint-1", kind: "COINCIDENT",
      first: { instanceId: "mock-instance-a", kind: "PLANE", geometryId: "datum-xy" },
      second: { instanceId: "mock-instance-b", kind: "PLANE", geometryId: "datum-xy" },
      directionRelation: "SAME", evaluationStatus: "VERIFIED", evaluationSummary: "mock supports resolved and solver residual is within tolerance" }] },
    artifacts: { [partArtifact.geometryKey]: partArtifact },
    resolvedInstances: [
      { id: "Frame Assembly/mock-instance-a/part", name: "Bracket A", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [-45, 0, 0], occurrencePath: "mock-instance-a", instancePath: mockInstancePath(productID, { id: "mock-instance-a", name: "Bracket A", documentId: partID, versionId: "mock-part-v3", translation: [-45,0,0] }), bodyId:"body-main",bodyVisible:true,ownedSketchIds:["mock-sketch-1"],bodyTreeNodeId: `document:${productID}/instance:mock-instance-a/reference/body:body-main` },
      { id: "Frame Assembly/mock-instance-b/part", name: "Bracket B", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [45, 0, 0], occurrencePath: "mock-instance-b", instancePath: mockInstancePath(productID, { id: "mock-instance-b", name: "Bracket B", documentId: partID, versionId: "mock-part-v3", translation: [45,0,0] }), bodyId:"body-main",bodyVisible:true,ownedSketchIds:["mock-sketch-1"],bodyTreeNodeId: `document:${productID}/instance:mock-instance-b/reference/body:body-main` },
    ],
  }],
]);
const undoSnapshots = new Map<string, DocumentView[]>();
const redoSnapshots = new Map<string, DocumentView[]>();
const mockViewAtRevision = (documentId: string, versionId: string): DocumentView | undefined => {
  const current = views.get(documentId);
  if (current?.document.versionId === versionId) return current;
  return [...(undoSnapshots.get(documentId) ?? []), ...(redoSnapshots.get(documentId) ?? [])]
    .find((snapshot) => snapshot.document.versionId === versionId);
};

const histories = new Map<string, HistoryEntry[]>([
  [partID, [
    { position: 0, versionId: "mock-part-v1", sequence: 1, commandType: "CREATE_DOCUMENT", createdAt: now(), isHead: false },
    { position: 1, versionId: "mock-part-v2", sequence: 2, commandType: "CREATE_SKETCH", createdAt: now(), isHead: false },
    { position: 2, versionId: "mock-part-v3", sequence: 3, commandType: "PAD_SKETCH", createdAt: now(), isHead: true },
  ]],
  [productID, [{ position: 0, versionId: "mock-product-v3", sequence: 3, commandType: "MOVE_INSTANCE", createdAt: now(), isHead: true }]],
]);
const openDocumentIDs: string[] = [];

function markDocumentOpen(documentID: string): void {
  if (!openDocumentIDs.includes(documentID)) openDocumentIDs.push(documentID);
  const summary = summaries.find((item) => item.id === documentID);
  if (summary) summary.lastOpenedAt = now();
}
const folders: FolderSummary[] = [{
  id: "mock-folder", name: "Concepts", description: "Early design studies", documentCount: 0,
  trashCount: 0, childCount: 0, createdAt: now(), updatedAt: now(), permission: "OWNER",
}];
const folderTrashRoots = new Map<string, string>();
const shares: ShareGrant[] = [];
const jobs = new Map<string, Job>();

function mockStructure(view: DocumentView, path = `document:${view.document.id}`, visiting = new Set<string>(), occurrence?: InstancePath): DocumentStructureNode {
  if (visiting.has(view.document.id)) return { id: path, kind: "REFERENCE_CYCLE", name: view.document.name,
    documentId: view.document.id, documentType: view.document.type, versionId: view.document.versionId };
  const nextVisiting = new Set(visiting).add(view.document.id);
  if (view.document.type === "PART") {
    if (view.part) for (const feature of view.part.features) feature.bodyId ??= view.part.activeBodyId;
    const features = view.part?.features ?? [];
    const sketches = new Map(features.filter((feature) => feature.type.toUpperCase().includes("SKETCH"))
      .map((feature) => [feature.id, feature]));
    const uses = new Map<string, Feature[]>();
    for (const feature of features) if (feature.profile) uses.set(feature.profile,
      [...(uses.get(feature.profile) ?? []), feature]);
    const consumed = new Set([...uses].filter(([sketchID, consumers]) =>
      consumers.length === 1 && consumers[0].bodyId === sketches.get(sketchID)?.bodyId).map(([id]) => id));
    const editable = path === `document:${view.document.id}`;
    const featureNode = (feature: Feature, parent: string, deletable: boolean): DocumentStructureNode => {
      const node: DocumentStructureNode = {
        id: `${parent}/${feature.type.toLowerCase()}:${feature.id}`,
        kind: feature.type.toUpperCase().includes("SKETCH") ? "SKETCH" : ["PAD","LINEAR_EXTRUDE"].includes(feature.type.toUpperCase()) ? "PAD" : feature.type === "REVOLVE" ? "REVOLVE" : feature.type === "IMPORT_BODY" ? "IMPORT" : "FEATURE",
        name: feature.name ?? feature.type, entityId: feature.id, entityType: feature.type,
        documentId: view.document.id, versionId: view.document.versionId,
        bodyId: feature.bodyId, operation: feature.operation, localVisible: feature.sketch ? feature.visible ?? !consumed.has(feature.id) : undefined,
        definitionDigest: !feature.sketch ? JSON.stringify(feature) : undefined,
        capabilities: [...(deletable ? ["DELETE" as const] : []), ...(!feature.sketch && feature.type !== "IMPORT_BODY" && editable ? ["EDIT" as const, "SUPPRESS" as const] : [])],
        suppressed:feature.suppressed, evaluationStatus:feature.evaluationStatus,
      };
      if (feature.sketch) node.children = [
        { id: `${node.id}/geometry`, kind: "SKETCH_GEOMETRY_SET", name: "Geometry", ownerEntityId: feature.id,
          children: feature.sketch.entities.map((entity, index) => ({ id: `${node.id}/geometry/entity:${entity.id}`,
            kind: "SKETCH_ENTITY", name: `${entity.kind === "LINE" ? "Line" : "Point"} ${index + 1}`, entityId: entity.id,
            ownerEntityId: feature.id, entityType: entity.kind, role: entity.role, suppressed: entity.suppressed, localVisible: entity.visible ?? true, documentId: view.document.id,
            capabilities: editable ? ["DELETE","SUPPRESS"] : undefined })) },
        { id: `${node.id}/external-geometry`, kind: "SKETCH_EXTERNAL_GEOMETRY_SET", name: "External Geometry", ownerEntityId: feature.id,
          children: (feature.sketch.externalGeometry??[]).map((external,index)=>({id:`${node.id}/external-geometry/external:${external.id}`,
            kind:"SKETCH_EXTERNAL_GEOMETRY",name:`External ${index+1}`,entityId:external.id,ownerEntityId:feature.id,entityType:"EXTERNAL",
            role:"CONSTRUCTION",diagnostic:external.diagnosticCode,documentId:view.document.id,capabilities:editable?["DETACH","RECONNECT"]:undefined})) },
        { id: `${node.id}/constraints`, kind: "SKETCH_CONSTRAINT_SET", name: "Constraints", ownerEntityId: feature.id,
          children: [
            { id: `${node.id}/constraints/logical`, kind: "SKETCH_LOGICAL_CONSTRAINT_SET", name: "Geometric Constraints",
              children: feature.sketch.constraints.filter((constraint) => !["DISTANCE","HORIZONTAL_DISTANCE","VERTICAL_DISTANCE","LENGTH","RADIUS","DIAMETER","MAJOR_RADIUS","MINOR_RADIUS","ANGLE"].includes(constraint.kind))
                .map((constraint, index) => ({ id: `${node.id}/constraints/logical/constraint:${constraint.id}`,
                  kind: "SKETCH_CONSTRAINT", name: `${constraint.kind} ${index + 1}`, entityId: constraint.id,
                  ownerEntityId: feature.id, entityType: constraint.kind, suppressed: constraint.suppressed,
                  diagnostic: feature.sketch?.solve.conflictingConstraintIds?.includes(constraint.id)?"CONFLICTING":feature.sketch?.solve.redundantConstraintIds?.includes(constraint.id)?"REDUNDANT":undefined,
                  documentId: view.document.id, capabilities: editable ? ["DELETE","SUPPRESS"] : undefined })) },
            { id: `${node.id}/constraints/dimensions`, kind: "SKETCH_DIMENSION_SET", name: "Dimensions",
              children: feature.sketch.constraints.filter((constraint) => ["DISTANCE","HORIZONTAL_DISTANCE","VERTICAL_DISTANCE","LENGTH","RADIUS","DIAMETER","MAJOR_RADIUS","MINOR_RADIUS","ANGLE"].includes(constraint.kind))
                .map((constraint, index) => ({ id: `${node.id}/constraints/dimensions/constraint:${constraint.id}`,
                  kind: "SKETCH_CONSTRAINT", name: `${constraint.kind} ${index + 1}`, entityId: constraint.id,
                  ownerEntityId: feature.id, entityType: constraint.kind, suppressed: constraint.suppressed,
                  diagnostic: feature.sketch?.solve.conflictingConstraintIds?.includes(constraint.id)?"CONFLICTING":feature.sketch?.solve.redundantConstraintIds?.includes(constraint.id)?"REDUNDANT":undefined,
                  documentId: view.document.id, capabilities: editable ? ["DELETE","SUPPRESS"] : undefined })) },
          ] },
      ];
      return node;
    };
    const bodyNodes:DocumentStructureNode[]=(view.part?.bodies??[]).map(body=>({id:`${path}/body:${body.id}`,kind:"BODY",name:body.name,entityId:body.id,bodyId:body.id,geometryKey:body.geometryKey,localVisible:body.visible,documentId:view.document.id,
      children:features.filter(f=>f.bodyId===body.id && !consumed.has(f.id)).map(feature=>{const node=featureNode(feature,`${path}/body:${body.id}`,editable && !uses.has(feature.id));const sketch=feature.profile?sketches.get(feature.profile):undefined;
        if(sketch)node.children=consumed.has(sketch.id)?[{...featureNode(sketch,node.id,false),presentationRole:"FEATURE_INPUT"}]
          :[{id:`${node.id}/input-sketch:${sketch.id}`,kind:"SKETCH_INPUT_REFERENCE",name:sketch.name??"Sketch",entityId:sketch.id,
            bodyId:sketch.bodyId,ownerEntityId:feature.id,presentationRole:"INPUT_REFERENCE",documentId:view.document.id,versionId:view.document.versionId}];return node;})}));
    const parameters:DocumentStructureNode[]=(view.part?.parameters?.length??0)>0?[{id:`${path}/parameters`,kind:"PARAMETER_SET",name:"Parameters / Relations",documentId:view.document.id,
      children:(view.part?.parameters??[]).map(parameter=>({id:`${path}/parameters/parameter:${parameter.parameterId}`,kind:"PARAMETER",parameterAlias:parameter.key,
        name:parameter.qualifiedDisplayPath||parameter.displayName||parameter.label,entityId:parameter.parameterId,documentId:view.document.id,versionId:view.document.versionId,capabilities:["EDIT"]}))}]:[];
    const publications:DocumentStructureNode[]=(view.part?.publications?.length??0)>0?[{id:`${path}/publications`,kind:"PUBLICATION_SET",name:"Publications",documentId:view.document.id,
      children:(view.part?.publications??[]).map(publication=>({id:`${path}/publications/publication:${publication.id}`,kind:"PUBLICATION",
        name:publication.name,entityId:publication.id,documentId:view.document.id,versionId:view.document.versionId,
        resolutionStatus:publication.resolution.status,sourceRevisionId:publication.resolution.resolvedVersionId,
        publication,capabilities:["EDIT","DELETE"]}))}]:[];
    const references:DocumentStructureNode[]=(view.part?.contextReferences?.length??0)>0?[{id:`${path}/context-references`,kind:"CONTEXT_REFERENCE_SET",name:"Context References",documentId:view.document.id,
      children:(view.part?.contextReferences??[]).map(reference=>({id:`${path}/context-references/context:${reference.id}`,kind:"CONTEXT_REFERENCE",
        name:reference.name,entityId:reference.id,documentId:view.document.id,versionId:view.document.versionId,
        sourceDocumentId:reference.sourceDocumentId,sourceRevisionId:reference.resolvedRevisionId,
        sourceDisplayPath:reference.sourceInstancePath?.display,resolutionStatus:reference.resolution.status,
        referenceMode:reference.referenceMode,capabilities:reference.referenceMode==="ISOLATED"?[]:["DETACH","REFRESH"]}))}]:[];
    const inputs:DocumentStructureNode[]=(view.part?.contextInputs?.length??0)>0?[{id:`${path}/context-inputs`,kind:"CONTEXT_INPUT_SET",name:"Context Inputs",documentId:view.document.id,
      children:(view.part?.contextInputs??[]).map(input=>({id:`${path}/context-inputs/input:${input.id}`,kind:"CONTEXT_INPUT",
        name:input.name,entityId:input.id,documentId:view.document.id,versionId:view.document.versionId,
        ownerEntityId:input.target.targetId,contextInput:input,capabilities:["EDIT","DELETE"]}))}]:[];
    return withMockOccurrence({ id: path, kind: "PART", name: view.document.name, documentId: view.document.id,
      documentType: "PART", versionId: view.document.versionId, children: [
        { id: `${path}/origin`, kind: "ORIGIN", name: "Origin", entityId:"origin", localVisible:view.part?.originVisible!==false, documentId: view.document.id, children: [
          ...(view.datumPlanes ?? []).map((plane) => ({ id: `${path}/origin/plane:${plane.id}`, kind: "PLANE" as const,
            localVisible:plane.visible!==false, name: plane.name, entityId: plane.id, documentId: view.document.id, plane: plane.plane, capabilities:editable && !["datum-xy","datum-yz","datum-xz"].includes(plane.id) ? ["DELETE" as const] : [] })),
          ...(view.datumAxes ?? view.part?.datumAxes ?? []).map(axis => ({id:`${path}/origin/datum-axis:${axis.id}`,kind:"DATUM_AXIS" as const,localVisible:axis.visible!==false,name:axis.name,entityId:axis.id,documentId:view.document.id,capabilities:editable ? ["DELETE" as const] : []})),
          ...(view.axisSystems ?? []).map((axis) => ({ id: `${path}/origin/axis:${axis.id}`, kind: "AXIS_SYSTEM" as const,
            localVisible:axis.visible!==false, name: axis.name, entityId: axis.id, documentId: view.document.id, children: [{id:`${path}/origin/axis:${axis.id}/point`,kind:"DATUM_POINT" as const,name:"Origin Point",entityId:axis.id,ownerEntityId:axis.id,documentId:view.document.id,localVisible:axis.pointVisible!==false},...(["X", "Y", "Z"] as const).map((name) => ({
              id: `${path}/origin/axis:${axis.id}/${name.toLowerCase()}`, kind: "AXIS" as const, name: `${name} Axis`,
              ownerEntityId:axis.id,localVisible:axis.axisVisibility?.[name]!==false, entityId: axis.id, axis: name, documentId: view.document.id,
            }))] })),
        ] },
        ...bodyNodes,
        ...parameters,
        ...inputs,
        ...references,
        ...publications,
      ] }, occurrence);
  }
  const instanceNodes: DocumentStructureNode[] = (view.product?.instances ?? []).map((instance) => {
      const referenced = mockViewAtRevision(instance.documentId, instance.versionId);
      const instancePath = appendMockInstancePath(view, instance, occurrence);
      const referenceTree = referenced ? mockStructure(referenced, `${path}/instance:${instance.id}/reference`, nextVisiting, instancePath) : undefined;
      const referenceName = referenced?.document.name ?? views.get(instance.documentId)?.document.name ?? "Reference";
      const referenceMode = instance.referenceMode ?? "FOLLOW_HEAD";
      return { id: `${path}/instance:${instance.id}`, kind: "INSTANCE" as const, name: `${referenceName}(${instance.name})`,
        referenceName, instanceName: instance.name,
        entityId: instance.id, documentId: instance.documentId, documentType: referenced?.document.type,
        versionId: instance.versionId, referenceMode, ownerDocumentId:view.document.id,
        resolutionStatus: referenced ? "CONNECTED" : "UNRESOLVED_REVISION",
        connectionStatus: referenced ? "CONNECTED" : "BROKEN",
        currencyStatus: instance.headChanged ? "UPDATE_AVAILABLE" : "CURRENT",
        childrenState: referenced ? "COMPLETE" as const : "FAILED" as const,
        instancePath,
        capabilities: path === `document:${view.document.id}` ? ["DELETE" as const,
          referenceMode === "PINNED" ? "FOLLOW_HEAD" as const : "PIN_VERSION" as const] : undefined,
        children: referenceTree ? [referenceTree] : undefined };
    });
  const constraints = view.product?.constraints ?? [];
  const publicationGroup:DocumentStructureNode[]=(view.product?.publications?.length??0)>0?[{id:`${path}/publications`,kind:"PRODUCT_PUBLICATION_SET",name:"Publications",documentId:view.document.id,
    children:(view.product?.publications??[]).map(publication=>({id:`${path}/publications/publication:${publication.id}`,kind:"PRODUCT_PUBLICATION",
      name:publication.name,entityId:publication.id,documentId:view.document.id,versionId:view.document.versionId,
      productPublication:publication,resolutionStatus:publication.resolution.status,capabilities:["EDIT","DELETE"]}))}]:[];
  const bindingGroup:DocumentStructureNode[]=(view.product?.contextBindings?.length??0)>0?[{id:`${path}/context-bindings`,kind:"CONTEXT_BINDING_SET",name:"Context Bindings",documentId:view.document.id,
    children:(view.product?.contextBindings??[]).map(binding=>({id:`${path}/context-bindings/binding:${binding.id}`,kind:"CONTEXT_BINDING",
      name:binding.name,entityId:binding.id,documentId:view.document.id,versionId:view.document.versionId,
      contextBinding:binding,resolutionStatus:binding.resolution.status,sourceDisplayPath:binding.sourceInstancePath.display,
      sourceRevisionId:binding.accepted.sourceRevisionId,capabilities:["DELETE","REFRESH"]}))}]:[];
  const constraintGroup = constraints.length ? [{ id: `${path}/assembly-constraints`, kind: "ASSEMBLY_CONSTRAINT_SET" as const, name: "约束", documentId:view.document.id, capabilities:["SUPPRESS" as const], suppressed:constraints.every(c=>c.suppressed),
    children: constraints.map((constraint, index) => {
      const disconnected = constraint.evaluationStatus === "BROKEN" || [constraint.first, constraint.second].filter(Boolean).some((reference) =>
        reference?.resolution?.result.supportingElementStatus === "NOT_CONNECTED");
      return { id: `${path}/assembly-constraints/constraint:${constraint.id}`, kind: "ASSEMBLY_CONSTRAINT" as const,localVisible:constraint.visible!==false,
        suppressed:constraint.suppressed, name: `#${constraint.kind}.${index + 1}`, entityId: constraint.id, entityType: constraint.kind, documentId: view.document.id,
        diagnostic: `${constraint.evaluationStatus}: ${constraint.evaluationSummary ?? ""}`,
        capabilities: ["EDIT" as const, "DELETE" as const, "SUPPRESS" as const,
          ...(disconnected ? ["RECONNECT" as const] : []), ...(constraint.evaluationStatus !== "VERIFIED" ? ["REFRESH" as const] : [])] };
    }) }] : [];
  return withMockOccurrence({ id: path, kind: "PRODUCT", name: view.document.name, documentId: view.document.id,
    documentType: "PRODUCT", versionId: view.document.versionId,
    children: [...instanceNodes, ...publicationGroup, ...bindingGroup, ...constraintGroup] }, occurrence);
}

function setBodyArtifact(view:DocumentView, artifact:Artifact) {const body=view.part?.bodies.find(b=>b.id===view.part?.activeBodyId);if(!body)return;body.geometryKey=artifact.geometryKey;(view.artifacts??={})[artifact.geometryKey]={...artifact,bodyId:body.id};}

function annotateMockStructure(node: DocumentStructureNode, ownerDocumentId: string, bodyId?: string): void {
  const documentId = node.documentId ?? ownerDocumentId;
  const currentBodyId = node.kind === "BODY" ? node.entityId : node.bodyId ?? bodyId;
  node.ownerDocumentId ??= documentId;
  node.bodyId ??= currentBodyId;
  const entityId = node.kind === "PART" || node.kind === "PRODUCT" ? node.documentId : node.entityId;
  if (entityId) node.subject = { documentId: node.kind === "INSTANCE" ? node.ownerDocumentId : documentId,
    entityKind: node.kind === "SKETCH_INPUT_REFERENCE" ? "SKETCH" : node.kind, entityId };
  if (node.instancePath?.segments.length) node.occurrence = {
    rootDocumentId: node.instancePath.rootDocumentId, instancePath: node.instancePath };
  if (node.versionId) node.snapshot = { revisionId: node.versionId,
    contextVariantKey: node.contextVariantKey, geometryKey: node.geometryKey };
  node.presentationRole ??= node.kind === "ORIGIN" || node.kind.endsWith("_SET") ? "GROUP" : "DEFINITION";
  if (["PUBLICATION", "PRODUCT_PUBLICATION", "CONTEXT_REFERENCE", "CONTEXT_BINDING"].includes(node.kind)) {
    node.connectionStatus ??= node.resolutionStatus;
  }
  node.childrenState ??= node.children?.length ? "COMPLETE" : "EMPTY";
  for (const child of node.children ?? []) annotateMockStructure(child, documentId, currentBodyId);
}

function getView(documentID: string): DocumentView {
  const view = views.get(documentID);
  if (!view) throw new Error("文档不存在");
  if(view.part)for(const f of view.part.features)f.bodyId??=view.part.activeBodyId;
  if(view.part)view.replicationSources=Object.fromEntries(view.part.features.map(f=>{
    const tool=["PAD","LINEAR_EXTRUDE","REVOLVE","LOFT"].includes(f.type)&&f.extent!=="THROUGH_ALL"&&!f.suppressed;
    return [f.id,{tool,rangeStart:tool&&f.operation!=="REMOVE",rangeModifier:!f.suppressed&&["FILLET","CHAMFER"].includes(f.type),bodyStage:!f.suppressed&&["PAD","LINEAR_EXTRUDE","REVOLVE","LOFT","BOOLEAN","FILLET","CHAMFER","DRAFT","SHELL","SOLID_PATTERN","IMPORT_BODY"].includes(f.type)}];
  }));
  view.structureTree = mockStructure(view);
  annotateMockStructure(view.structureTree, view.document.id);
  if (view.product?.visibilityOverrides?.length) {
    const overrides = new Map(view.product.visibilityOverrides.map((item) =>
      [`${item.instancePath.canonical}\u0000${item.entityKind}\u0000${item.entityId}`, item.mode] as const));
    const visitVisibility = (node: DocumentStructureNode) => {
      if (node.entityId && node.instancePath) node.visibilityMode = overrides.get(
        `${node.instancePath.canonical}\u0000${node.kind}\u0000${node.entityId}`);
      node.children?.forEach(visitVisibility);
    };
    visitVisibility(view.structureTree);
  }
  if (view.product) {
    const documents = new Set<string>(), visited = new Set<string>(), products: string[] = [];
    const visit = (product: DocumentView, depth: number) => {
      if (depth > 32) return;
      for (const instance of product.product?.instances ?? []) {
        if (instance.referenceMode === "PINNED") continue;
        documents.add(instance.documentId);
        if (visited.has(instance.documentId)) continue;
        visited.add(instance.documentId);
        const referenced = views.get(instance.documentId);
        if (referenced?.product) { visit(referenced, depth + 1); products.push(instance.documentId); }
      }
    };
    visit(view, 0);
    view.followedDocumentIds = [...documents].sort();
    view.followedProductIds = products;
  }
  return view;
}

function commit(documentID: string, commandType: string, mutate?: (view: DocumentView) => void): DocumentView {
	const view = getView(documentID);
	const undo = undoSnapshots.get(documentID) ?? [];
	undo.push(structuredClone(view));
	undoSnapshots.set(documentID, undo);
	redoSnapshots.set(documentID, []);
	mutate?.(view);
  const versionID = id("mock-version");
  view.document.versionId = versionID;
  view.document.lastUpdated = now();
	view.document.canUndo = true;
	view.document.canRedo = false;
  const entries = histories.get(documentID) ?? [];
  entries.forEach((entry) => { entry.isHead = false; });
  entries.push({ position: entries.length, versionId: versionID, sequence: entries.length + 1, commandType, createdAt: now(), isHead: true });
  histories.set(documentID, entries);
  return getView(documentID);
}

function rebuildProduct(view: DocumentView): void {
  view.artifacts = { [partArtifact.geometryKey]: partArtifact };
  view.resolvedInstances = (view.product?.instances ?? []).map((instance) => ({
    id: `${view.document.name}/${instance.id}/part`, name: instance.name, documentId: partID,
    geometryKey: partArtifact.geometryKey, translation: instance.translation, rotation:instance.rotation, occurrencePath: instance.id,
    instancePath: mockInstancePath(view.document.id, instance),
    bodyId:"body-main",bodyVisible:true,ownedSketchIds:["mock-sketch-1"],bodyTreeNodeId: `document:${view.document.id}/instance:${instance.id}/reference/body:body-main`,
  }));
}

async function command(documentID: string, input: Record<string, unknown>): Promise<DocumentView> {
	const commandType = String(input.type ?? "COMMAND");
	if (commandType === "UNDO" || commandType === "REDO") {
		const source = commandType === "UNDO" ? (undoSnapshots.get(documentID) ?? []) : (redoSnapshots.get(documentID) ?? []);
		if (source.length === 0) return pause(getView(documentID));
		const current = structuredClone(getView(documentID));
		const target = source.pop()!;
		const destination = commandType === "UNDO" ? (redoSnapshots.get(documentID) ?? []) : (undoSnapshots.get(documentID) ?? []);
		destination.push(current);
		if (commandType === "UNDO") { undoSnapshots.set(documentID, source); redoSnapshots.set(documentID, destination); }
		else { redoSnapshots.set(documentID, source); undoSnapshots.set(documentID, destination); }
		target.document.canUndo = (undoSnapshots.get(documentID)?.length ?? 0) > 0;
		target.document.canRedo = (redoSnapshots.get(documentID)?.length ?? 0) > 0;
		target.document.versionId = id("mock-version"); target.document.lastUpdated = now();
		views.set(documentID, target);
		const summary = summaries.find((item) => item.id === documentID);
		if (summary) Object.assign(summary, target.document);
		return pause(getView(documentID));
	}
	return pause(commit(documentID, commandType, (view) => {
    if(view.part) {
      const part=view.part,body=part.bodies.find(b=>b.id===input.bodyId);
      if(commandType==="CREATE_BODY"){const body={id:id("body"),name:String(input.name??`Body.${part.bodies.length+1}`),visible:true};part.bodies.push(body);part.activeBodyId=body.id;}
      if(body && commandType==="DELETE_BODY"){part.bodies=part.bodies.filter(b=>b!==body);part.features=part.features.filter(f=>f.bodyId!==body.id);if(part.activeBodyId===body.id)part.activeBodyId=part.bodies[0]?.id??"";if(body.geometryKey)delete view.artifacts?.[body.geometryKey];}
      if(body && commandType==="RENAME_BODY")body.name=String(input.name);
      if(commandType==="RENAME_FEATURE") { const feature=part.features.find((item)=>item.id===input.targetId);if(feature)feature.name=String(input.name); }
      if(body && commandType==="SET_ACTIVE_BODY")part.activeBodyId=body.id;
      if(body && commandType==="SET_BODY_VISIBILITY")body.visible=Boolean(input.visible);
      if(commandType==="CREATE_PARAMETER") {
        const unit=String(input.unit??"mm"),number=Number(input.value??0),alias=String(input.name||`Parameter${(part.parameters?.length??0)+1}`);
        const dimension={Length:unit==="mm"?1:0,Mass:0,Time:0,Current:0,Temperature:0,Amount:0,Luminous:0,Semantic:unit==="deg"?"ANGLE":""};
        const quantity={siValue:unit==="mm"?number/1000:unit==="deg"?number*Math.PI/180:number,dimension};
        part.parameters=[...(part.parameters??[]),{parameterId:id("parameter"),key:alias,label:String(input.name||`Parameter.${(part.parameters?.length??0)+1}`),
          lifecycle:"USER",valueType:"QUANTITY",dimension,displayUnit:unit,role:"DESIGN_INPUT",source:{literal:quantity},evaluatedValue:quantity}];
      }
      if(commandType==="DELETE_PARAMETER") part.parameters=(part.parameters??[]).filter((item)=>item.parameterId!==input.parameterId);
      if(commandType==="SET_DEFINITION_VISIBILITY") {
        const targetKind=String(input.targetKind),targetID=String(input.targetId);
        if(targetKind==="BODY") { const target=part.bodies.find((item)=>item.id===targetID);if(target)target.visible=Boolean(input.visible); }
        else if(targetKind==="SKETCH") { const target=part.features.find((item)=>item.id===targetID&&item.sketch);if(target)target.visible=Boolean(input.visible); }
        else if(targetKind==="SKETCH_ENTITY") { const owner=part.features.find((item)=>item.id===input.ownerEntityId);const target=owner?.sketch?.entities.find((item)=>item.id===targetID);if(target)target.visible=Boolean(input.visible); }
        else if(targetKind==="ORIGIN"&&targetID==="origin")part.originVisible=Boolean(input.visible);
        else if(targetKind==="PLANE"){const target=part.datumPlanes.find(p=>p.id===targetID);if(target)target.visible=Boolean(input.visible);}
        else if(targetKind==="DATUM_AXIS"){const target=part.datumAxes?.find(p=>p.id===targetID);if(target)target.visible=Boolean(input.visible);}
        else if(["AXIS_SYSTEM","AXIS","DATUM_POINT"].includes(targetKind)){const target=part.axisSystems.find(p=>p.id===targetID);if(target){if(targetKind==="AXIS_SYSTEM")target.visible=Boolean(input.visible);else if(targetKind==="DATUM_POINT")target.pointVisible=Boolean(input.visible);else if(input.axis==="X"||input.axis==="Y"||input.axis==="Z")(target.axisVisibility??={})[input.axis]=Boolean(input.visible);}}
        else throw new Error("此对象没有独立显示结果");
      }
    }
    if(commandType==="SET_DEFINITION_VISIBILITY"&&view.product){const target=view.product.constraints?.find(c=>c.id===input.targetId);if(input.targetKind!=="ASSEMBLY_CONSTRAINT"||!target)throw new Error("约束不存在");target.visible=Boolean(input.visible);}
    if(commandType==="SET_OCCURRENCE_VISIBILITY"&&view.product) {
      const path=input.instancePath as InstancePath;
      const kind=String(input.targetKind),targetID=String(input.targetId),mode=String(input.visibilityMode);
      const overrides=(view.product.visibilityOverrides??=[]);
      const index=overrides.findIndex((item)=>item.instancePath.canonical===path.canonical&&item.entityKind===kind&&item.entityId===targetID);
      if(index>=0)overrides.splice(index,1);
      if(mode!=="INHERIT")overrides.push({instancePath:path,entityKind:kind,entityId:targetID,mode:mode as "SHOW"|"HIDE"});
    }
    if (commandType === "CREATE_SKETCH" && view.part) {
      const plane=(input.targetKind === "FACE" ? "CUSTOM" : input.plane) as "XY"|"XZ"|"YZ"|"CUSTOM";
      const datumPlaneId=String(input.datumPlaneId ?? `datum-${plane.toLowerCase()}`);
      view.part.features.push({ id: id("mock-sketch"), type: "SKETCH", name: `Sketch ${view.part.features.length + 1}`, plane,
        bodyId: String(input.bodyId ?? view.part.activeBodyId),
        sketch:{schemaVersion:2,support:input.targetKind === "FACE"
          ? {type:"PLANAR_FACE",plane:"CUSTOM",status:"CONNECTED",origin:[0,0,0],xDirection:[1,0,0],normal:[0,0,1]}
          : {type:"DATUM_PLANE",datumPlaneId,plane,status:"CONNECTED"},entities:[],constraints:[],solve:{status:"EMPTY",degreesOfFreedom:0}} });
    }
    if(commandType==="CREATE_DATUM_PLANE"&&view.part){const datum={id:id("mock-plane"),name:String(input.name),plane:"CUSTOM" as const,
      origin:input.origin as Vec3,normal:input.normal as Vec3,uDirection:input.uDirection as Vec3,size:180};
      view.part.datumPlanes.push(datum);view.datumPlanes=view.part.datumPlanes;}
    if(commandType==="CREATE_DATUM_AXIS"&&view.part){const datum={id:id("mock-axis"),name:String(input.name),origin:input.origin as Vec3,direction:input.direction as Vec3};
      view.part.datumAxes=[...(view.part.datumAxes??[]),datum];view.datumAxes=[...(view.datumAxes??[]),datum];}
    if (commandType === "EDIT_SKETCH" && view.part) {
      const sketch=view.part.features.find((feature)=>feature.id===input.sketchId)?.sketch;
      for (const operation of input.operations as SketchOperation[] ?? []) {
        if (operation.type==="ADD_ENTITY") sketch?.entities.push(operation.entity);
        if (operation.type==="ADD_CONSTRAINT") sketch?.constraints.push(operation.constraint);
        if (operation.type==="UPDATE_CONSTRAINT_VALUE" && sketch) {
          const constraint=sketch.constraints.find((candidate)=>candidate.id===operation.constraintId);
          if(constraint)constraint.value=operation.value;
        }
        if(operation.type==="UPDATE_ENTITY_SUPPRESSION"&&sketch){const entity=sketch.entities.find((item)=>item.id===operation.entityId);if(entity)entity.suppressed=operation.suppressed;}
        if(operation.type==="UPDATE_CONSTRAINT_SUPPRESSION"&&sketch){const constraint=sketch.constraints.find((item)=>item.id===operation.constraintId);if(constraint)constraint.suppressed=operation.suppressed;}
        if(operation.type==="DETACH_EXTERNAL_GEOMETRY"&&sketch){const external=sketch.externalGeometry?.find((item)=>item.id===operation.externalId);
          if(external?.snapshot){sketch.entities.push({id:external.id,kind:external.snapshot.kind,role:"CONSTRUCTION",point:external.snapshot.point,
            start:external.snapshot.start,end:external.snapshot.end,center:external.snapshot.center,radius:external.snapshot.radius});
            sketch.externalGeometry=sketch.externalGeometry?.filter((item)=>item.id!==operation.externalId);}}
        if (operation.type==="ADD_RECTANGLE" && sketch) {
          const {first,second}=operation, x0=Math.min(first.x,second.x),x1=Math.max(first.x,second.x),y0=Math.min(first.y,second.y),y1=Math.max(first.y,second.y);
          sketch.entities.push({id:id("line"),kind:"LINE",role:"PROFILE",start:{x:x0,y:y0},end:{x:x1,y:y0}},{id:id("line"),kind:"LINE",role:"PROFILE",start:{x:x1,y:y0},end:{x:x1,y:y1}},{id:id("line"),kind:"LINE",role:"PROFILE",start:{x:x1,y:y1},end:{x:x0,y:y1}},{id:id("line"),kind:"LINE",role:"PROFILE",start:{x:x0,y:y1},end:{x:x0,y:y0}});
        }
      }
    }
    if (commandType === "PAD_SKETCH" && view.part) {
      const sketch = view.part.features.find((feature) => feature.id === input.sketchId);
      view.part.features.push({ id: id("mock-pad"), type: "PAD", name: `Extrude ${view.part.features.length + 1}`,
        profile: String(input.sketchId), length: Number(input.length), operation: "ADD" });
      if (sketch?.sketch) { const points=sketch.sketch.entities.flatMap((entity)=>sampleSketchEntity(entity));const xs=points.map((point)=>point[0]),ys=points.map((point)=>point[1]);if(points.length>0)setBodyArtifact(view,boxArtifact(id("mock-shape"),[Math.max(...xs)-Math.min(...xs),Math.max(...ys)-Math.min(...ys),Number(input.length)])); }
    }
    if (commandType === "CREATE_SOLID_FEATURE" && view.part) {
      const generator = String(input.generator) as "LINEAR_EXTRUDE" | "REVOLVE";
      const featureID = id(generator === "REVOLVE" ? "mock-revolve" : "mock-extrude");
      const sketchBodyID=view.part.features.find((feature)=>feature.id===input.sketchId)?.bodyId;
      let bodyId = String(input.bodyId ?? sketchBodyID ?? view.part.activeBodyId);
      if (input.operation === "NEW_BODY") {
        bodyId = `body-${featureID}`;
        view.part.bodies.push({ id: bodyId, name: `Body.${view.part.bodies.length + 1}`, visible: true,
          createdByFeatureId: featureID });
        view.part.activeBodyId = bodyId;
      }
      const referenced = view.part.parameters?.find((parameter) => parameter.key === input.lengthExpression);
      const resolvedLength = Number(input.length) || (referenced?.evaluatedValue?.siValue ?? 0.04) * 1000;
      view.part.features.push({ id: featureID, bodyId, type: generator,
        name: `${generator === "REVOLVE" ? "Revolve" : "Extrude"} ${view.part.features.length + 1}`,
        profile: String(input.sketchId), length: generator === "LINEAR_EXTRUDE" ? resolvedLength : undefined, angle: Number(input.angle) || undefined,
        axisEntityId: input.axisEntityId ? String(input.axisEntityId) : undefined,
        operation: input.operation === "NEW_BODY" ? "ADD" : String(input.operation) as "ADD" | "REMOVE" | "INTERSECT" });
      if (generator === "LINEAR_EXTRUDE") view.part.parameters = [...(view.part.parameters ?? []), {
        parameterId: `parameter:${featureID}:length`, key: `${featureID.replaceAll("-", "_")}_length`, label: "Length",
        valueType: "QUANTITY", dimension: lengthDimension, displayUnit: "mm", role: "INPUT",
        source: input.lengthExpression ? { expression: { sourceText: String(input.lengthExpression) } }
          : { literal: { siValue: resolvedLength / 1000, dimension: lengthDimension } },
        evaluatedValue: { siValue: resolvedLength / 1000, dimension: lengthDimension },
      }];
    }
	if (commandType === "SET_PARAMETER_VALUE" && view.part) {
		const parameter = view.part.parameters?.find((candidate) => candidate.parameterId === input.parameterId);
		if (parameter) {
			const quantity = { siValue: lengthInMillimeters(input.value, input.unit) / 1000, dimension: parameter.dimension };
			parameter.source = { literal: quantity }; parameter.evaluatedValue = quantity;
		}
	}
	if (commandType === "SET_PARAMETER_EXPRESSION" && view.part) {
		const parameter = view.part.parameters?.find((candidate) => candidate.parameterId === input.parameterId);
		const referenced = view.part.parameters?.find((candidate) => candidate.key === input.expression);
		if (parameter) { parameter.source = { expression: { sourceText: String(input.expression) } }; parameter.evaluatedValue = referenced?.evaluatedValue; }
	}
	if (commandType === "RENAME_PARAMETER" && view.part) {
		const parameter = view.part.parameters?.find((candidate) => candidate.parameterId === input.parameterId);
		if (parameter) parameter.key = String(input.name);
	}
	if (commandType === "EDIT_PARAMETER" && view.part) {
		const parameter = view.part.parameters?.find((candidate) => candidate.parameterId === input.parameterId);
		if (parameter) {
			const nextKey = String(input.name ?? "").trim();
			if (!/^[A-Za-z_][A-Za-z_0-9]*$/.test(nextKey) || view.part.parameters?.some((candidate) => candidate.parameterId !== parameter.parameterId && candidate.key === nextKey))
				throw new Error("参数别名无效或已存在");
			if (input.expression) {
				const referenced = view.part.parameters?.find((candidate) => candidate.key === input.expression);
				if (!referenced?.evaluatedValue) throw new Error("参数表达式无法解析");
				parameter.source = { expression: { sourceText: String(input.expression) } }; parameter.evaluatedValue = referenced.evaluatedValue;
			} else {
				const quantity = { siValue: lengthInMillimeters(input.value, input.unit) / 1000, dimension: parameter.dimension };
				parameter.source = { literal: quantity }; parameter.evaluatedValue = quantity;
			}
			parameter.key = nextKey;
		}
	}
	if (commandType === "CREATE_PUBLICATION" && view.part) {
		const publicationType = String(input.publicationType) as Publication["type"];
		const targetKind = String(input.targetKind);
		const publication: Publication = { id: id("mock-publication"), name: String(input.name || `${targetKind === "BODY" ? "Body" : targetKind === "PARAMETER" ? "Parameter" : publicationType}.1`), type: publicationType,
			semanticPurpose: String(input.semanticPurpose ?? ""), compatibilityVersion: String(input.compatibilityVersion || "1.0.0"),
			target: targetKind === "PARAMETER" ? { kind: "PARAMETER", parameterId: String(input.targetId) }
				: targetKind === "FACE" || targetKind === "EDGE" ? { kind: "TOPOLOGY", sourceVersionId: view.document.versionId }
				: targetKind === "BODY" ? { kind: "BODY_RESULT", bodyId: String(input.targetId) }
				: targetKind === "FEATURE_OUTPUT" ? { kind: "FEATURE_OUTPUT", featureId: String(input.targetId), outputSlot: "BODY" }
				: { kind: "DATUM", datumId: String(input.targetId), axis: input.axis as "X"|"Y"|"Z"|undefined },
			contract: {}, resolution: { status: "CONNECTED", resolvedVersionId: view.document.versionId,
				geometryKey: String(input.geometryKey ?? view.part?.bodies.find(b=>b.id===view.part?.activeBodyId)?.geometryKey ?? "") } };
		view.part.publications = [...(view.part.publications ?? []), publication];
	}
	if (commandType === "EDIT_PUBLICATION" && view.part) {
		const publication = view.part.publications?.find((candidate) => candidate.id === input.publicationId);
		if (publication) { publication.name = String(input.name); publication.semanticPurpose = String(input.semanticPurpose ?? ""); }
	}
	if (commandType === "REDIRECT_PUBLICATION" && view.part) {
		const publication = view.part.publications?.find((candidate) => candidate.id === input.publicationId);
		if (publication) {
			const targetKind = String(input.targetKind);
			publication.target = targetKind === "PARAMETER" ? { kind: "PARAMETER", parameterId: String(input.targetId) }
				: targetKind === "FACE" || targetKind === "EDGE" ? { kind: "TOPOLOGY", sourceVersionId: view.document.versionId }
				: targetKind === "BODY" ? { kind: "FEATURE_OUTPUT", featureId: String(input.targetId), outputSlot: "BODY" }
				: { kind: "DATUM", datumId: String(input.targetId), axis: input.axis as "X"|"Y"|"Z"|undefined };
			publication.resolution = { status: "CONNECTED", resolvedVersionId: view.document.versionId,
				geometryKey: String(input.geometryKey ?? view.part?.bodies.find(b=>b.id===view.part?.activeBodyId)?.geometryKey ?? "") };
		}
	}
	if (commandType === "DELETE_PUBLICATION" && view.part) {
		view.part.publications = (view.part.publications ?? []).filter((candidate) => candidate.id !== input.publicationId);
	}
	if (commandType === "DELETE_CONTEXT_INPUT" && view.part)
		view.part.contextInputs = (view.part.contextInputs ?? []).filter((candidate) => candidate.id !== input.contextInputId);
	if (commandType === "DELETE_CONTEXT_BINDING" && view.product)
		view.product.contextBindings = (view.product.contextBindings ?? []).filter((candidate) => candidate.id !== input.contextBindingId);
	if (commandType === "CREATE_CONTEXT_REFERENCE" && view.part) {
		const source = getView(String(input.sourceDocumentId));
		const publication = source.part?.publications?.find((candidate) => candidate.id === input.publicationId);
		if (publication) view.part.contextReferences = [...(view.part.contextReferences ?? []), {
			id:id("mock-context"),name:String(input.name),owningWorkspace:"main",sourceDocumentId:source.document.id,
			referenceMode:"FOLLOW_HEAD",publication:{publicationId:publication.id,expectedType:publication.type,
				compatibilityVersion:publication.compatibilityVersion},transform:{translation:[0,0,0],rotation:[0,0,0,1]},
			resolvedRevisionId:source.document.versionId,resolution:publication.resolution,localTargetId:String(input.parameterId??"") }];
	}
	if (commandType === "DETACH_CONTEXT_REFERENCE" && view.part) {
		const reference=view.part.contextReferences?.find((candidate)=>candidate.id===input.contextReferenceId);
		if(reference)reference.referenceMode="ISOLATED";
	}
	if (commandType === "SET_PARAMETER_EXTERNAL" && view.part) {
		const parameter = view.part.parameters?.find((candidate) => candidate.parameterId === input.parameterId);
		const source = getView(String(input.sourceDocumentId)).part?.publications?.find((candidate) => candidate.id === input.publicationId);
		if (parameter && source?.resolution.value) {
			parameter.source = { external: { sourceDocumentId: String(input.sourceDocumentId), revision: { mode: "PINNED", revisionId: source.resolution.resolvedVersionId! },
				publicationId: source.id, expectedType: parameter.valueType, expectedDimension: parameter.dimension,
				contractVersion: source.compatibilityVersion, resolvedRevisionId: source.resolution.resolvedVersionId!,
				resolvedValue: source.resolution.value, resolvedValueDigest: source.resolution.valueDigest ?? "mock" } };
			parameter.evaluatedValue = source.resolution.value;
		}
	}
	if (commandType === "EDIT_FEATURE" && view.part) {
		const feature=view.part.features.find((candidate)=>candidate.id===input.targetId);
		const parameter=view.part.parameters?.find((candidate)=>candidate.parameterId===`parameter:${input.targetId}:length`);
		const referenced=view.part.parameters?.find((candidate)=>candidate.key===input.lengthExpression);
		const length=Number(input.length)||(referenced?.evaluatedValue?.siValue??parameter?.evaluatedValue?.siValue??0.04)*1000;
		if(feature)feature.length=length;
		if(parameter){parameter.source=input.lengthExpression?{expression:{sourceText:String(input.lengthExpression)}}
			:{literal:{siValue:length/1000,dimension:parameter.dimension}};parameter.evaluatedValue={siValue:length/1000,dimension:parameter.dimension};}
	}
    if (commandType === "INSERT_INSTANCE" && view.product) {
      const reference = getView(String(input.referencedDocumentId));
      const used = new Set(view.product.instances.map((instance) => instance.name.toLocaleLowerCase()));
      let ordinal = 1; while (used.has(`${reference.document.name}.${ordinal}`.toLocaleLowerCase())) ordinal++;
      view.product.instances.push({ id: id("mock-instance"), name: `${reference.document.name}.${ordinal}`,
        documentId: String(input.referencedDocumentId), versionId: reference.document.versionId,
        translation: [0, 0, 0], referenceMode: "FOLLOW_HEAD" });
      rebuildProduct(view);
    }
    if (commandType === "INSERT_INSTANCES" && view.product) {
      const used = new Set(view.product.instances.map((instance) => instance.name.toLocaleLowerCase()));
      const uniqueName = (base: string) => {
        let ordinal = 1;
        while (used.has(`${base}.${ordinal}`.toLocaleLowerCase())) ordinal++;
        const name = `${base}.${ordinal}`;
        used.add(name.toLocaleLowerCase());
        return name;
      };
      const source = view.product.instances.find((instance) => instance.id === input.instanceId);
      if (source) {
        const axis = { X: 0, Y: 1, Z: 2 }[String(input.patternAxis) as "X" | "Y" | "Z"];
        if (axis === undefined) throw new Error("无效的阵列方向");
        for (let index = 1; index < Number(input.patternCount); index++) {
          const translation = [...source.translation] as [number, number, number];
          translation[axis] += index * Number(input.patternSpacing) * (input.patternReversed ? -1 : 1);
          view.product.instances.push({ ...source, id: id("mock-instance"), name: uniqueName(getView(source.documentId).document.name), translation });
        }
      } else {
        for (const documentID of input.referencedDocumentIds as string[] ?? []) {
          const reference = getView(documentID);
          view.product.instances.push({ id: id("mock-instance"), name: uniqueName(reference.document.name),
            documentId: documentID, versionId: reference.document.versionId, translation: [0, 0, 0], referenceMode: "FOLLOW_HEAD" });
        }
      }
      rebuildProduct(view);
    }
	if(commandType==="REPLACE_INSTANCE"&&view.product){const instance=view.product.instances.find((item)=>item.id===input.instanceId);
		const replacement=getView(String(input.referencedDocumentId));if(instance){instance.documentId=replacement.document.id;instance.versionId=replacement.document.versionId;rebuildProduct(view);}}
	if(commandType==="RENAME_INSTANCE"&&view.product){const instance=view.product.instances.find((item)=>item.id===input.instanceId);
		if(instance)instance.name=String(input.name);rebuildProduct(view);}
	if (commandType === "CREATE_PRODUCT_PUBLICATION" && view.product) {
		const instance=view.product.instances.find((candidate)=>candidate.id===input.instanceId);
		const source=instance&&getView(instance.documentId).part?.publications?.find((candidate)=>candidate.id===input.publicationId);
		if(instance&&source)view.product.publications=[...(view.product.publications??[]),{id:id("mock-product-publication"),name:String(input.name),
			type:source.type,semanticPurpose:String(input.semanticPurpose??""),compatibilityVersion:source.compatibilityVersion,
			target:{instancePath:(input.instancePath as InstancePath|undefined)??mockInstancePath(view.document.id,instance),publicationId:source.id},
			contract:source.contract,resolution:source.resolution}];
	}
	if(commandType==="EDIT_PRODUCT_PUBLICATION"&&view.product){const publication=view.product.publications?.find((item)=>item.id===input.publicationId);
		if(publication){publication.name=String(input.name);publication.semanticPurpose=String(input.semanticPurpose??"");}}
	if(commandType==="REDIRECT_PRODUCT_PUBLICATION"&&view.product){const publication=view.product.publications?.find((item)=>item.id===input.publicationId);
		const path=input.instancePath as InstancePath|undefined;const sourceDocumentID=path?.segments.at(-1)?.referencedDocumentId;
		const source=sourceDocumentID?getView(sourceDocumentID).part?.publications?.find((item)=>item.id===input.targetId):undefined;
		if(publication&&path&&source&&publication.type===source.type){publication.target={instancePath:path,publicationId:source.id};publication.resolution=source.resolution;}}
	if(commandType==="DELETE_PRODUCT_PUBLICATION"&&view.product)view.product.publications=(view.product.publications??[]).filter((item)=>item.id!==input.publicationId);
	if(commandType==="CREATE_CONTEXT_INPUT"&&view.part){view.part.contextInputs=[...(view.part.contextInputs??[]),{
		id:id("mock-context-input"),name:String(input.name||"ContextInput"),type:String(input.publicationType||"PARAMETER") as Publication["type"],
		required:Boolean(input.required),target:{kind:String(input.targetKind) as "PARAMETER"|"DATUM"|"SKETCH_EXTERNAL_GEOMETRY"|"FEATURE_INPUT",targetId:String(input.targetId)},contract:{}}];}
	if(commandType==="EDIT_CONTEXT_INPUT"&&view.part){const contextInput=view.part.contextInputs?.find((item)=>item.id===input.contextInputId);
		if(contextInput){contextInput.name=String(input.name);contextInput.required=Boolean(input.required);}}
    if (commandType === "MOVE_INSTANCE" && view.product) {
      const instance = view.product.instances.find((candidate) => candidate.id === input.instanceId);
      if (instance) { instance.translation = input.translation as Vec3; instance.rotation = input.rotation as [number,number,number,number] ?? instance.rotation; }
      rebuildProduct(view);
    }
    if (commandType === "ADD_ASSEMBLY_CONSTRAINT" && view.product) {
      view.product.constraints = [...(view.product.constraints ?? []), {
        id: id("mock-constraint"), kind: String(input.constraintKind) as AssemblyConstraint["kind"],
        first: input.firstAssemblyRef as AssemblyGeometryRef, second: input.secondAssemblyRef as AssemblyGeometryRef | undefined,
        value: Number(input.value ?? 0), directionRelation: String(input.directionRelation ?? "UNORIENTED"),
        fixedPose:input.fixedPose as AssemblyConstraint["fixedPose"], fixMode:input.fixMode as AssemblyConstraint["fixMode"],
        angleAxis:input.angleAxis as AssemblyGeometryRef | undefined, reverseAngleAxis:Boolean(input.reverseAngleAxis),
        angleRelation:input.angleRelation as AssemblyConstraint["angleRelation"],
        distanceRelation: String(input.distanceRelation ?? "UNSIGNED"), angleReferenceDirection: input.angleReferenceDirection as Vec3 | undefined,
        evaluationStatus: "VERIFIED", evaluationSummary: "mock supports resolved and solver residual is within tolerance",
      }];
    }
    if (commandType === "SET_ASSEMBLY_CONSTRAINT_STATE" && view.product) {
      for (const constraint of view.product.constraints ?? []) {
        if (!(input.constraintIds as string[]).includes(constraint.id)) continue;
        if (typeof input.suppressed === "boolean") constraint.suppressed = input.suppressed;
        if (input.constraintMode) constraint.mode = input.constraintMode as AssemblyConstraint["mode"];
        if (!constraint.suppressed) constraint.evaluationStatus = "NOT_UPDATED";
      }
    }
    if (commandType === "EDIT_ASSEMBLY_CONSTRAINT" && view.product) {
      const constraint = view.product.constraints?.find((candidate) => candidate.id === input.targetId);
      if (constraint) Object.assign(constraint, {
        first: input.firstAssemblyRef ?? constraint.first, second: input.secondAssemblyRef ?? constraint.second,
        value: Number(input.value ?? constraint.value ?? 0), directionRelation: input.directionRelation ?? constraint.directionRelation,
        distanceRelation: input.distanceRelation ?? constraint.distanceRelation,
        angleReferenceDirection: input.angleReferenceDirection ?? constraint.angleReferenceDirection,
        angleRelation: input.angleRelation ?? constraint.angleRelation,
        angleAxis:input.angleAxis ?? constraint.angleAxis, reverseAngleAxis:input.reverseAngleAxis ?? constraint.reverseAngleAxis,
        fixMode: input.fixMode ?? constraint.fixMode, fixedPose: input.fixedPose ?? constraint.fixedPose,
        evaluationStatus: "VERIFIED", evaluationSummary: "mock supports reconnected and solver residual is within tolerance",
      });
      if (constraint && input.angleRelation && input.angleRelation !== "DIRECTED") {
        constraint.angleAxis = undefined;
        constraint.angleReferenceDirection = undefined;
        constraint.reverseAngleAxis = false;
      }
    }
    if (commandType === "UPDATE_REFERENCES" && view.product) {
      for (const instance of view.product.instances) instance.headChanged = false;
      for (const constraint of view.product.constraints ?? []) {
        if (constraint.evaluationStatus === "NOT_UPDATED") {
          constraint.evaluationStatus = "VERIFIED";
          constraint.evaluationSummary = "mock references refreshed";
        }
      }
    }
    if (commandType === "SET_REFERENCE_MODE" && view.product) {
      const instance = view.product.instances.find((candidate) => candidate.id === input.instanceId);
      if (instance) instance.referenceMode = input.referenceMode as ProductInstance["referenceMode"];
    }
    if (commandType === "SET_FEATURE_SUPPRESSION" && view.part) {
      const feature = view.part.features.find(f => f.id === input.targetId);
      if (feature) { feature.suppressed = Boolean(input.suppressed); feature.evaluationStatus = feature.suppressed ? "SUPPRESSED" : undefined; }
    }
    if (commandType === "DELETE_NODE" || commandType === "DELETE_NODES") {
      const targets = commandType === "DELETE_NODES" ? input.targets as Array<Record<string, unknown>> : [input];
      for (const item of targets) {
        const kind = String(item.targetKind);
        const target = String(item.targetId);
        const owner = String(item.ownerEntityId ?? "");
        if (kind === "INSTANCE" && view.product) {
          view.product.instances = view.product.instances.filter((instance) => instance.id !== target); rebuildProduct(view);
        } else if (kind === "ASSEMBLY_CONSTRAINT" && view.product) {
          view.product.constraints = (view.product.constraints ?? []).filter((constraint) => constraint.id !== target);
        } else if (kind === "DATUM_PLANE" && view.part) { view.part.datumPlanes = view.part.datumPlanes.filter(p=>p.id !== target); view.datumPlanes = view.part.datumPlanes; }
        if (kind === "DATUM_AXIS" && view.part) { view.part.datumAxes = view.part.datumAxes?.filter(a=>a.id !== target); view.datumAxes = view.part.datumAxes; }
        if (kind === "FEATURE" && view.part) {
          view.part.features = view.part.features.filter((feature) => feature.id !== target);
        } else if (view.part) {
          const sketch = view.part.features.find((feature) => feature.id === owner)?.sketch;
          if (sketch && kind === "SKETCH_CONSTRAINT") sketch.constraints = sketch.constraints.filter((constraint) => constraint.id !== target);
          if (sketch && kind === "SKETCH_ENTITY") {
            sketch.entities = sketch.entities.filter((entity) => entity.id !== target);
            sketch.constraints = sketch.constraints.filter((constraint) => !constraint.references.some((reference) => reference.target === "ENTITY" && reference.entityId === target));
          }
        }
      }
    }
  }));
}

export const mockApi: CadApi = {
 bindTopologySelection:async()=>{throw new Error("Exact datum topology requires the real Worker");},
  getFeatureInput: async () => { throw new Error("Mock 没有精确特征输入阶段；请使用真实 API 验证编辑"); },
  beginAssemblyInteraction:async()=>{throw new Error("ENVIRONMENT_BLOCKED: assembly manipulation requires the real solver; Mock is not numerical validation");},
  updateAssemblyInteraction:async()=>{throw new Error("ENVIRONMENT_BLOCKED: assembly manipulation requires the real solver");},
  cancelAssemblyInteraction:async()=>undefined,
  analyzeAssemblyConflicts:async()=>{throw new Error("ENVIRONMENT_BLOCKED: numerical conflict evidence requires a real Worker, not Mock");},
  cancelAssemblyConflicts:async()=>undefined,
	assemblyCapabilities: async () => { throw new Error("Exact assembly capabilities require the real server"); },
	inspectAssemblySupports: async () => { throw new Error("Exact support inspection requires the real Worker; mock geometry is not authoritative"); },
	getAssemblyEngineeringEvidence:async(id:string,revisionId:string)=>({documentId:id,revisionId,available:false,components:[]}),
	toolbarCatalog: async () => pause(mockToolbarCatalog),
  session: async () => {
    if (localStorage.getItem("occccad.mock.auth") === "false") throw new Error("authentication required");
    return pause({ user: administrator, authenticationMode: "mock" });
  },
  login: async () => { localStorage.setItem("occccad.mock.auth", "true"); return pause({ user: administrator }); },
  register: async (email, displayName) => pause({ user: { id: id("mock-user"), email, displayName, status: "PENDING", platformRole: "MEMBER" }, message: "submitted" }),
  logout: async () => { localStorage.setItem("occccad.mock.auth", "false"); },
  changePassword: async () => pause({ message: "password changed" }),
  adminUsers: async (query = "", status = "") => pause(users.filter((user) =>
    (!query || `${user.displayName} ${user.email}`.toLowerCase().includes(query.toLowerCase())) && (!status || user.status === status))),
  adminStats: async () => pause({ users: users.length, pending: users.filter((user) => user.status === "PENDING").length, activeSessions: 1, documents: summaries.length }),
  adminCreateUser: async (input) => {
    const user: User = { id: id("mock-user"), email: input.email, displayName: input.displayName,
      status: input.status as User["status"], platformRole: input.platformRole as User["platformRole"] };
    users.push(user); return pause(user);
  },
  adminUpdateUser: async (userID, input) => {
    const user = users.find((candidate) => candidate.id === userID)!;
    Object.assign(user, input); return pause(user);
  },
  adminDisableUser: async (userID) => { const user = users.find((candidate) => candidate.id === userID); if (user) user.status = "DISABLED"; },
  adminResetPassword: async () => pause({ message: "password reset" }),
  listUsers: async () => pause(users),
  listTeams: async () => pause([]),
  listShares: async () => pause(shares),
  share: async (_type, _resourceID, subjectType, subjectID, role) => {
    const grant: ShareGrant = { id: id("mock-grant"), subjectType, subjectId: subjectID,
      subjectName: users.find((user) => user.id === subjectID)?.displayName ?? subjectID, role, inherited: false };
    shares.push(grant); return pause(grant);
  },
  unshare: async (_type, _resourceID, grantID) => { const index = shares.findIndex((grant) => grant.id === grantID); if (index >= 0) shares.splice(index, 1); },
  listAudit: async () => pause([]),
  health: async () => pause({ status: "ok", occtVersion: "Mock 7.9.1" }),
  listDocuments: async (options = {}) => {
    let documents = summaries.filter((item) => (options.scope === "trash" ? item.deletedAt : !item.deletedAt)
      && (!item.folderId || !folders.find((folder) => folder.id === item.folderId)?.deletedAt));
    if (options.query) documents = documents.filter((item) => `${item.name} ${item.description}`.toLowerCase().includes(options.query!.toLowerCase()));
    if (options.type) documents = documents.filter((item) => item.type === options.type);
    if (!options.allFolders) documents = documents.filter((item) => (item.folderId ?? "") === (options.folderId ?? ""));
    if (options.recent) documents = documents.filter((item) => Boolean(item.lastOpenedAt));
    if (options.shared) documents = documents.filter((item) => item.permission !== "OWNER");
    documents = [...documents].sort((left, right) => options.sort === "name" ? left.name.localeCompare(right.name)
      : options.sort === "created" ? right.createdAt.localeCompare(left.createdAt)
      : options.sort === "recent" ? (right.lastOpenedAt ?? "").localeCompare(left.lastOpenedAt ?? "")
      : right.lastUpdated.localeCompare(left.lastUpdated));
    const offset = options.offset ?? 0; const limit = options.limit ?? 50;
    return pause({ documents: documents.slice(offset, offset + limit), total: documents.length, offset, limit });
  },
  listOpenDocuments: async () => pause(openDocumentIDs.map((documentID) => getView(documentID).document)),
  openDocument: async (documentID) => { markDocumentOpen(documentID); return pause(getView(documentID)); },
  closeOpenDocument: async (documentID) => {
    const index = openDocumentIDs.indexOf(documentID);
    if (index >= 0) openDocumentIDs.splice(index, 1);
    await pause(undefined);
  },
  listFolders: async (parentID = "") => pause(folders.filter((folder) => !folder.deletedAt && (folder.parentId ?? "") === parentID)),
  listTrashedFolders: async () => pause(folders.filter((folder) => folder.deletedAt && folderTrashRoots.get(folder.id) === folder.id
    && (!folder.parentId || !folders.find((parent) => parent.id === folder.parentId)?.deletedAt))),
  folderBreadcrumbs: async (folderID) => {
    const path: FolderSummary[] = []; const visited = new Set<string>();
    let current = folders.find((folder) => folder.id === folderID);
    while (current && !visited.has(current.id)) {
      visited.add(current.id); path.unshift(current);
      current = folders.find((folder) => folder.id === current!.parentId);
    }
    return pause(path);
  },
  createFolder: async (name, description, parentID) => {
    const folder: FolderSummary = { id: id("mock-folder"), name, description, parentId: parentID,
      documentCount: 0, trashCount: 0, childCount: 0, createdAt: now(), updatedAt: now(), permission: "OWNER" };
    folders.push(folder); return pause(folder);
  },
  updateFolder: async (folderID, name, description) => {
    const folder = folders.find((candidate) => candidate.id === folderID)!; Object.assign(folder, { name, description }); return pause(folder);
  },
  deleteFolder: async (folderID) => {
    const descendants = new Set([folderID]);
    for (let changed = true; changed;) {
      changed = false;
      for (const folder of folders) if (folder.parentId && descendants.has(folder.parentId) && !descendants.has(folder.id)) {
        descendants.add(folder.id); changed = true;
      }
    }
    for (const folder of folders) if (descendants.has(folder.id) && !folder.deletedAt) {
      folder.deletedAt = now(); folderTrashRoots.set(folder.id, folderID);
    }
  },
  restoreFolder: async (folderID) => {
    for (const folder of folders) if (folderTrashRoots.get(folder.id) === folderID) {
      folder.deletedAt = undefined; folderTrashRoots.delete(folder.id);
    }
  },
  getDocument: async (documentID) => pause(getView(documentID)),
  getProductDesignSession: async (documentID, activePath = "") => {
    const root = getView(documentID);
    const active = root.resolvedInstances?.find((item) => item.instancePath?.canonical === activePath);
    const activeDocument = active ? getView(active.documentId) : root;
    return pause({ rootProductDocumentId: documentID, rootProductRevisionId: root.document.versionId,
      rootSnapshotDigest: `mock-snapshot:${root.document.versionId}`, activeInstancePath: active?.instancePath,
      activeDocumentId: activeDocument.document.id, activeRevisionId: activeDocument.document.versionId,
      contextCatalogDigest: `mock-catalog:${root.document.versionId}` });
  },
  getContextCatalog: async (documentID, activePath, expectedType = "") => {
    const root = getView(documentID);
    const publications: ContextCatalog["publications"] = [];
    for (const occurrence of root.resolvedInstances ?? []) {
      if (!occurrence.instancePath) continue;
      const source = getView(occurrence.documentId);
      for (const publication of source.part?.publications ?? []) {
        if (expectedType && publication.type !== expectedType) continue;
        publications.push({ instancePath: occurrence.instancePath, documentId: source.document.id,
          revisionId: source.document.versionId, publication, displayPath: `${occurrence.instancePath.display}/${publication.name}`,
          selectable: publication.resolution.status === "CONNECTED" });
      }
    }
    const activeInstancePath = root.resolvedInstances?.find((item) => item.instancePath?.canonical === activePath)?.instancePath;
    return pause({ rootProductDocumentId: documentID, rootProductRevisionId: root.document.versionId,
      activeInstancePath, expectedType: expectedType || undefined, digest: `mock-catalog:${root.document.versionId}`, publications });
  },
  createProductContextBinding: async (documentID, input) => {
    const ownerDocumentID = input.owningInstancePath.segments.at(-1)?.referencedDocumentId;
    const sourceDocumentID = input.sourceInstancePath.segments.at(-1)?.referencedDocumentId;
    if (!ownerDocumentID || !sourceDocumentID) throw new Error("context binding paths must resolve to documents");
    const owner = await command(ownerDocumentID, { type: "CREATE_CONTEXT_INPUT", name: input.contextInputName,
      publicationType: input.publicationType, targetKind: input.targetKind, targetId: input.targetId, required: input.required });
    const contextInput = owner.part?.contextInputs?.at(-1);
    const sourcePublication = getView(sourceDocumentID).part?.publications?.find((item) => item.id === input.publicationId);
    if (!contextInput || !sourcePublication) throw new Error("context binding source or target does not exist");
    return pause(commit(documentID, "CREATE_CONTEXT_BINDING", (root) => {
      if (!root.product) throw new Error("root document is not a Product");
      root.product.contextBindings = [...(root.product.contextBindings ?? []), { id: id("mock-context-binding"),
        name: input.name || contextInput.name, owningInstancePath: input.owningInstancePath, contextInputId: contextInput.id,
        sourceInstancePath: input.sourceInstancePath, publication: { publicationId: sourcePublication.id,
          expectedType: sourcePublication.type, compatibilityVersion: sourcePublication.compatibilityVersion },
        referenceMode: input.referenceMode ?? "FOLLOW_WORKSPACE_WITH_ACCEPT", transform: { translation: [0,0,0], rotation: [0,0,0,1] },
        resolution: sourcePublication.resolution, accepted: { rootProductRevisionId: root.document.versionId,
          sourceRevisionId: getView(sourceDocumentID).document.versionId, owningRevisionId: owner.document.versionId,
          contractDigest: "mock-contract", sourceDigest: sourcePublication.resolution.sourceDigest, status: "ACCEPTED" } }];
    }));
  },
  createPartComponent: async (documentID, input) => {
    const requested = input.name?.trim();
    let partName = requested;
    if (!partName) {
      const used = new Set(summaries.filter((item) => item.type === "PART").map((item) => item.name.toLocaleLowerCase()));
      for (let ordinal = 1; !partName; ordinal += 1) if (!used.has(`part${ordinal}`)) partName = `Part${ordinal}`;
    }
    const partDocument: DocumentSummary = { id: id("mock-document"), name: partName!, description: input.description ?? "", type: "PART",
      versionId: id("mock-version"), canUndo: false, canRedo: false, createdAt: now(), lastUpdated: now(),
      workspaceName: "Main", permission: "OWNER" };
    const partView: DocumentView = { document: partDocument, datumPlanes, axisSystems,
      part: { bodies:[{id:"body-main",name:"Body.1",visible:true}],activeBodyId:"body-main",units: "mm", datumPlanes, axisSystems, features: [] } };
    summaries.unshift(partDocument); views.set(partDocument.id, partView); histories.set(partDocument.id, []);
    undoSnapshots.set(partDocument.id, []); redoSnapshots.set(partDocument.id, []);

    const segments = input.targetProductInstancePath?.segments ?? [];
    const targetDocumentID = segments.at(-1)?.referencedDocumentId ?? documentID;
    let child = commit(targetDocumentID, "CREATE_PART_COMPONENT", (target) => {
      if (!target.product) throw new Error("target document is not a Product");
      const base = partName!;
      const used = new Set(target.product.instances.map((instance) => instance.name.toLocaleLowerCase()));
      let instanceName = "";
      for (let ordinal = 1; !instanceName; ordinal += 1) if (!used.has(`${base}.${ordinal}`.toLocaleLowerCase())) instanceName = `${base}.${ordinal}`;
      target.product.instances.push({ id: id("mock-instance"), name: instanceName, documentId: partDocument.id,
        versionId: partDocument.versionId, resolvedVersionId: partDocument.versionId, translation: [0,0,0],
        rotation: [0,0,0,1], referenceMode: "FOLLOW_HEAD" });
    });
    for (let index = segments.length - 1; index >= 0; index -= 1) {
      const segment = segments[index];
      child = commit(segment.ownerDocumentId, "ACCEPT_NEW_PART_REVISION", (owner) => {
        const instance = owner.product?.instances.find((candidate) => candidate.id === segment.instanceId);
        if (!instance) throw new Error("target Product occurrence disappeared");
        instance.versionId = child.document.versionId;
        instance.resolvedVersionId = child.document.versionId;
      });
    }
    return pause(getView(documentID));
  },
  getProductUpdatePlan: async (documentID) => {
    const root = getView(documentID);
    const entries = (root.product?.contextBindings ?? []).map((binding) => {
      const sourceDocumentID = binding.sourceInstancePath.segments.at(-1)?.referencedDocumentId;
      const candidate = sourceDocumentID ? getView(sourceDocumentID).document.versionId : undefined;
      const hasUpdate = Boolean(candidate && candidate !== binding.accepted.sourceRevisionId);
      return { kind:"CONTEXT_BINDING" as const, bindingId: binding.id, name: binding.name, sourceDisplayPath: binding.sourceInstancePath.display,
        owningDisplayPath: binding.owningInstancePath.display, acceptedRevisionId: binding.accepted.sourceRevisionId,
        candidateRevisionId: candidate, connection: "CONNECTED" as const,
        currency: hasUpdate ? "UPDATE_AVAILABLE" as const : "CURRENT" as const, evaluation: "READY" as const };
    });
    const hasUpdates = entries.some((entry) => entry.currency === "UPDATE_AVAILABLE");
    return pause({ rootProductDocumentId: documentID, rootProductRevisionId: root.document.versionId,
      digest: `mock-update-plan:${root.document.versionId}:${entries.map((entry) => entry.candidateRevisionId).join(":")}`,
      canAccept: true, hasUpdates, entries, contextVariants: [] });
  },
  acceptProductUpdatePlan: async (documentID) => command(documentID, {type:"UPDATE_REFERENCES"}),
  listProductReleases: async () => pause([]),
  createProductRelease: async (documentID, name) => {
    const root = getView(documentID);
    return pause({id:id("mock-release"),name,createdAt:now(),manifest:{schemaVersion:1,
      digest:`mock-release:${root.document.versionId}`,rootProductDocumentId:documentID,
      rootProductRevisionId:root.document.versionId,rootSnapshotDigest:`mock-snapshot:${root.document.versionId}`,
      gates:[{code:"MOCK_RELEASE",status:"PASSED" as const}]}});
  },
  replayProductRelease: async (_documentID, releaseId) => pause({releaseId,manifestDigest:"mock-release",status:"CONVERGED"}),
  replayProductSolveManifest: async (_documentID, digest) => pause({manifestDigest:digest,status:"CONVERGED"}),
  getProductSolveResult: async (_documentID, requestId) => pause({manifestDigest:"mock-manifest",requestId,
    resultDigest:"mock-result",status:"CONVERGED",result:{}}),
  getDocumentProperties: async (documentID): Promise<DocumentProperties> => {
    const view = getView(documentID);
    const artifacts = Object.values(view.artifacts ?? {});
    return pause({ documentId: documentID, versionId: view.document.versionId, documentType: view.document.type, units: "mm", artifacts,
      aggregate: { artifactCount: artifacts.length, triangleCount: artifacts.reduce((sum, item) => sum + item.triangleCount, 0),
        vertexCount: artifacts.reduce((sum, item) => sum + item.displayVertexCount, 0),
        solidCount: artifacts.reduce((sum, item) => sum + item.topology.solids, 0),
        glbBytes: artifacts.reduce((sum, item) => sum + item.glbBytes, 0), brepBytes: artifacts.reduce((sum, item) => sum + item.brepBytes, 0),
        resolvedInstanceCount: view.resolvedInstances?.length ?? 0 },
      worker: { available: true, workerId: "mock-geometry-1", occtVersion: "mock-7.9.1", residentGeometryCount: artifacts.length } });
  },
  getTopologyProperties: async (_documentID, geometryKey, kind, localId) => pause({
    geometryKey, geometryId: `geometry-${geometryKey}`, kind, localId,
    geometryType: kind === "FACE" ? "PLANE" : kind === "EDGE" ? "LINE" : "POINT",
    properties: mockTopologyProperties(kind), namingStatus: "UNAVAILABLE" as const,
    workerId: "mock-geometry-1", occtVersion: "mock-7.9.1",
  }),
  getHistory: async (documentID) => pause(histories.get(documentID) ?? []),
  createVersion: async (documentID, name) => {
    const entries = histories.get(documentID) ?? []; const head = entries.find((entry) => entry.isHead);
    if (head) head.versionName = name; return pause(entries);
  },
  createDocument: async (type, name, description = "", folderID) => {
    const document: DocumentSummary = { id: id("mock-document"), name, description, type, versionId: id("mock-version"),
      canUndo: false, canRedo: false, createdAt: now(), lastUpdated: now(), folderId: folderID,
      workspaceName: "Main", permission: "OWNER" };
    const view: DocumentView = type === "PART" ? { document, datumPlanes, axisSystems,
      part: { bodies:[{id:"body-main",name:"Body.1",visible:true}],activeBodyId:"body-main",units: "mm", datumPlanes, axisSystems, features: [] } }
      : { document, product: { instances: [] }, artifacts: {}, resolvedInstances: [] };
    summaries.unshift(document); views.set(document.id, view); histories.set(document.id, []); undoSnapshots.set(document.id, []); redoSnapshots.set(document.id, []); markDocumentOpen(document.id); return pause(view);
  },
  updateDocument: async (documentID, name, description) => pause(commit(documentID, "UPDATE_DOCUMENT", (view) => Object.assign(view.document, { name, description }))),
  deleteDocument: async (documentID) => { getView(documentID).document.deletedAt = now(); },
  restoreDocument: async (documentID) => { getView(documentID).document.deletedAt = undefined; return pause(getView(documentID)); },
  purgeDocument: async (documentID) => {
    const view = getView(documentID);
    if (!view.document.deletedAt) throw new Error("move the document to trash before permanent deletion");
    const index = summaries.findIndex((item) => item.id === documentID);
    if (index >= 0) summaries.splice(index, 1);
    views.delete(documentID);
  },
  moveDocument: async (documentID, folderID) => pause(commit(documentID, "MOVE_DOCUMENT", (view) => { view.document.folderId = folderID; })),
  copyDocument: async (documentID, name, folderID) => {
    const source = getView(documentID); const copy = structuredClone(source); copy.document = { ...copy.document, id: id("mock-document"),
      name, folderId: folderID, versionId: id("mock-version"), createdAt: now(), lastUpdated: now() };
    summaries.unshift(copy.document); views.set(copy.document.id, copy); histories.set(copy.document.id, []); return pause(copy);
  },
  command,
  previewCommand: async (documentID, input, signal) => {
    if (signal?.aborted) throw new DOMException("Preview cancelled", "AbortError");
    const view = getView(documentID);
    if (input.type === "EDIT_SKETCH") {
      const sketch = view.part?.features.find((feature) => feature.id === input.sketchId)?.sketch;
      if (!sketch) throw new Error("Sketch does not exist");
      throw new Error("Mock 不支持权威草图编辑候选，请连接计算服务");
    }
    if(input.type==="MOVE_INSTANCE"&&view.product){return pause({previewId:id("mock-move-preview"),baseVersionId:view.document.versionId,baseSequence:0,modelHash:"mock-move",
      instancePoses:view.product.instances.map((instance)=>({instanceId:instance.id,translation:instance.id===input.instanceId?input.translation as Vec3:instance.translation,rotation:instance.rotation??[0,0,0,1]}))});}
    if ((input.type === "ADD_ASSEMBLY_CONSTRAINT" || input.type === "EDIT_ASSEMBLY_CONSTRAINT") && view.product) {
      return pause({ previewId: id("mock-assembly-preview"), baseVersionId: view.document.versionId, baseSequence: 0,
        modelHash: "mock-assembly", assemblyComponents: [], constraintEvaluation: {
          constraintId: String(input.targetId ?? "mock-preview-constraint"), status: "VERIFIED" as const,
          summary: "mock supports resolved and solver residual is within tolerance",
          first: { status: "CONNECTED" as const }, ...(input.secondAssemblyRef ? { second: { status: "CONNECTED" as const } } : {}),
        }, instancePoses: view.product.instances.map((instance) => ({
          instanceId: instance.id, translation: instance.translation, rotation: instance.rotation ?? [0, 0, 0, 1],
        })) });
    }
    if (input.type === "EDIT_FEATURE" && view.part) {
		const feature=view.part.features.find((candidate)=>candidate.id===input.targetId);
		if(!feature)throw new Error("Feature does not exist");
		const parameter=view.part.parameters?.find((candidate)=>candidate.parameterId===`parameter:${input.targetId}:length`);
		const referenced=view.part.parameters?.find((candidate)=>candidate.key===input.lengthExpression);
		const length=Number(input.length)||(referenced?.evaluatedValue?.siValue??parameter?.evaluatedValue?.siValue??0.04)*1000;
		return pause({previewId:id("mock-edit-preview"),baseVersionId:view.document.versionId,baseSequence:0,modelHash:"mock-edit",
			artifact:boxArtifact(id("mock-preview"),[40,30,length])});
	}
    if ((input.type !== "PAD_SKETCH" && input.type !== "CREATE_SOLID_FEATURE") || !view.part) throw new Error("Mock preview currently supports solid generators only");
    const sketch = view.part.features.find((feature) => feature.id === input.sketchId)?.sketch;
    const points = sketch?.entities.flatMap((entity) => [entity.start, entity.end]).filter(Boolean) as Array<{x:number;y:number}>;
    if (!points?.length) throw new Error("Preview profile is empty");
    const xs = points.map((point) => point.x), ys = points.map((point) => point.y);
    const referenced = view.part.parameters?.find((parameter) => parameter.key === input.lengthExpression);
    const depth = input.generator === "REVOLVE" ? Math.max(...xs)-Math.min(...xs)
      : Number(input.length) || (referenced?.evaluatedValue?.siValue ?? 0.04) * 1000;
    const artifact = boxArtifact(id("mock-preview"), [Math.max(...xs)-Math.min(...xs), Math.max(...ys)-Math.min(...ys), depth]);
    const bodyId = String(input.bodyId ?? view.part.activeBodyId);
    artifact.bodyId = bodyId;
    return pause({ previewId: id("mock-command-preview"), baseVersionId: view.document.versionId,
      baseSequence: 0, modelHash: "mock-preview", artifact,
      resultBodyId: input.operation === "NEW_BODY" ? "preview-new-body" : bodyId,
      resultBodyName: input.operation === "NEW_BODY" ? `Body.${view.part.bodies.length + 1}` : view.part.bodies.find((body) => body.id === bodyId)?.name,
      bodyAssignment: input.operation === "NEW_BODY" ? "EXPLICIT_NEW_BODY" : "TARGET_BODY" });
  },
  createSketch: async (documentID, support) => command(documentID, { type: "CREATE_SKETCH", ...support }),
  editSketch: async (documentID, sketchID, operations) => command(documentID, { type: "EDIT_SKETCH", sketchId: sketchID, operations }),
  setParameterValue: async (documentID, parameterId, value, unit) => command(documentID, { type: "SET_PARAMETER_VALUE", parameterId, value, unit }),
  setParameterExpression: async (documentID, parameterId, expression) => command(documentID, { type: "SET_PARAMETER_EXPRESSION", parameterId, expression }),
  renameParameter: async (documentID, parameterId, name) => command(documentID, { type: "RENAME_PARAMETER", parameterId, name }),
  setParameterExternal: async (documentID, parameterId, sourceDocumentId, publicationId, versionId) => command(documentID,
    { type: "SET_PARAMETER_EXTERNAL", parameterId, sourceDocumentId, publicationId, versionId }),
  createPublication: async (documentID, input) => command(documentID, { type: "CREATE_PUBLICATION", ...input }),
  editPublication: async (documentID, publicationId, input) => command(documentID, { type: "EDIT_PUBLICATION", publicationId, ...input }),
  redirectPublication: async (documentID, publicationId, input) => command(documentID, { type: "REDIRECT_PUBLICATION", publicationId, ...input }),
  deletePublication: async (documentID, publicationId) => command(documentID, { type: "DELETE_PUBLICATION", publicationId }),
  deleteNode: async (documentID, targetKind, targetID, ownerEntityID) => command(documentID, { type: "DELETE_NODE", targetKind, targetId: targetID, ownerEntityId: ownerEntityID }),
  deleteNodes: async (documentID, targets) => command(documentID, { type: "DELETE_NODES", targets }),
  pad: async (documentID, sketchID, length, intentRequestID) => command(documentID, { type: "PAD_SKETCH", sketchId: sketchID, length,
    ...(intentRequestID ? { requestId: intentRequestID } : {}) }),
  createSolidFeature: async (documentID, input, intentRequestID) => command(documentID, { type: "CREATE_SOLID_FEATURE", ...input,
    ...(intentRequestID ? { requestId: intentRequestID } : {}) }),
	editFeature: async (documentID, input) => command(documentID, {type:"EDIT_FEATURE",targetId:input.featureId,
		expectedFeatureDigest:input.expectedFeatureDigest,length:input.length,unit:input.length === undefined?undefined:"mm",
		lengthExpression:input.lengthExpression,previewId:input.previewId}),
  createDatumPlane: async (documentID, input) => command(documentID, { type: "CREATE_DATUM_PLANE", ...input }),
  createDatumAxis: async (documentID, input) => command(documentID, { type: "CREATE_DATUM_AXIS", ...input }),
  insert: async (documentID, referencedDocumentID) => command(documentID, { type: "INSERT_INSTANCE", referencedDocumentId: referencedDocumentID }),
  insertMany: async (documentID, referencedDocumentIds) => command(documentID, { type: "INSERT_INSTANCES", referencedDocumentIds }),
  patternInstances: async (documentID, input) => command(documentID, { type: "INSERT_INSTANCES", instanceId: input.sourceInstanceId,
    patternAxis: input.axis, patternCount: input.count, patternSpacing: input.spacing, patternReversed: input.reversed }),
  replaceInstance: async (documentID, instanceID, referencedDocumentID) => command(documentID, {type:"REPLACE_INSTANCE",instanceId:instanceID,referencedDocumentId:referencedDocumentID}),
  renameInstance: async (documentID, instanceID, name) => command(documentID, {type:"RENAME_INSTANCE",instanceId:instanceID,name}),
  createProductPublication: async (documentID, instanceID, publicationID, name, semanticPurpose, instancePath) => command(documentID,
	{type:"CREATE_PRODUCT_PUBLICATION",instanceId:instanceID,publicationId:publicationID,name,semanticPurpose,instancePath}),
  editProductPublication: async (documentID, publicationID, name, semanticPurpose) => command(documentID,
	{type:"EDIT_PRODUCT_PUBLICATION",publicationId:publicationID,name,semanticPurpose}),
  deleteProductPublication: async (documentID, publicationID) => command(documentID,{type:"DELETE_PRODUCT_PUBLICATION",publicationId:publicationID}),
  createContextReference: async (documentID,input) => command(documentID,{type:"CREATE_CONTEXT_REFERENCE",...input}),
  createContextInput: async (documentID,input) => command(documentID,{type:"CREATE_CONTEXT_INPUT",...input}),
  editContextInput: async (documentID,contextInputID,name,required) => command(documentID,{type:"EDIT_CONTEXT_INPUT",contextInputId:contextInputID,name,required}),
  deleteContextInput: async (documentID,contextInputID) => command(documentID,{type:"DELETE_CONTEXT_INPUT",contextInputId:contextInputID}),
  deleteContextBinding: async (documentID,contextBindingID) => command(documentID,{type:"DELETE_CONTEXT_BINDING",contextBindingId:contextBindingID}),
  detachContextReference: async (documentID,contextReferenceID) => command(documentID,{type:"DETACH_CONTEXT_REFERENCE",contextReferenceId:contextReferenceID}),
  move: async (documentID, instanceID, translation,rotation) => command(documentID, { type: "MOVE_INSTANCE", instanceId: instanceID, translation,rotation }),
  addAssemblyConstraint: async (documentID, input) => command(documentID, { type: "ADD_ASSEMBLY_CONSTRAINT", ...input }),
  editAssemblyConstraint: async (documentID, constraintId, input) => command(documentID, { type: "EDIT_ASSEMBLY_CONSTRAINT", targetId: constraintId, ...input }),
  setReferenceMode: async (documentID, instanceID, referenceMode) => command(documentID, { type: "SET_REFERENCE_MODE", instanceId: instanceID, referenceMode }),
  updateReferences: async (documentID) => command(documentID, { type: "UPDATE_REFERENCES" }),
  undo: async (documentID) => command(documentID, { type: "UNDO" }),
  redo: async (documentID) => command(documentID, { type: "REDO" }),
  restore: async (documentID, versionID) => command(documentID, { type: "RESTORE", versionId: versionID }),
  exchangeCapabilities: async () => ({ maxUploadBytes: 16 * 1024 ** 3 }),
  importDocument: async (file, folderID) => {
    const documentID = id("mock-document"); const name = file.name;
    const document: DocumentSummary = { id: documentID, name, description: "", type: "PART", versionId: id("mock-version"),
      canUndo: true, canRedo: false, createdAt: now(), lastUpdated: now(), folderId: folderID || undefined,
      workspaceName: "Main", permission: "OWNER" };
    const view: DocumentView = { document, datumPlanes, axisSystems, part: { bodies:[{id:"body-main",name:"Body.1",visible:true}],activeBodyId:"body-main",units: "mm", datumPlanes, axisSystems,
      features: [{ id: id("mock-import"), type: "IMPORT_BODY", fileName: file.name }] } };
    setBodyArtifact(view,boxArtifact(id("mock-exchange"), [48, 32, 26]));
    summaries.unshift(document); views.set(documentID, view); histories.set(documentID, []);
    const job: Job = { id: id("mock-job"), type: "EXCHANGE_IMPORT", state: "SUCCEEDED", documentId: documentID,
      progress: 100, payload: { fileName: file.name, format: "STEP" }, attemptCount: 1, maxAttempts: 3,
      createdAt: now(), completedAt: now(), canCancel: false, canRetry: false, userVisible: true };
    jobs.set(job.id, job); return pause(job);
  },
  startExport: async (documentID, _format, _releaseId) => {
    const job: Job = { id: id("mock-job"), type: "EXCHANGE_EXPORT", state: "SUCCEEDED", documentId: documentID,
      progress: 100, resultObjectId: "mock-exchange", payload: { fileName: "mock.step", format: "STEP" },
      attemptCount: 1, maxAttempts: 3, createdAt: now(), completedAt: now(), canCancel: false, canRetry: false, userVisible: true };
    jobs.set(job.id, job); return pause(job);
  },
  createMotionDemo:async()=>{throw new Error("演示创建需要实际后端");},
  planMotionApply:async()=>{throw new Error("转换需要实际后端");},
 motionJointProposals:async()=>[],
 startMotionRun:async()=>{throw new Error("机构与 DMU 需要连接实际后端");},
  getMotionRun:async()=>{throw new Error("机构与 DMU 需要连接实际后端");},
  getJob: async (jobID) => pause(jobs.get(jobID)!),
  listJobs: async () => pause([...jobs.values()].filter((job) => job.userVisible).reverse()),
  cancelJob: async (jobID) => {
    const job = jobs.get(jobID)!;
    const updated = { ...job, state: "CANCELED" as const, canCancel: false, canRetry: true,
      cancelRequestedAt: now(), completedAt: now() };
    jobs.set(jobID, updated); return pause(updated);
  },
  retryJob: async (jobID) => {
    const job = jobs.get(jobID)!;
    const updated = { ...job, state: "QUEUED" as const, progress: 0, canCancel: true, canRetry: false,
      cancelRequestedAt: undefined, completedAt: undefined, errorCode: undefined, errorMessage: undefined };
    jobs.set(jobID, updated); return pause(updated);
  },
  downloadJob: async () => { /* no file is produced in mock mode */ },
	downloadAssemblyDiagnostic: async () => { throw new Error("Mock 模式没有真实三维求解记录"); },
	downloadDiagnosticBundle: async () => { /* server diagnostics are unavailable in mock mode */ },
};
