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

function boxArtifact(key: string, size: Vec3): Artifact {
  const [x, y, z] = size;
  const vertices: Vec3[] = [[0, 0, 0], [x, 0, 0], [x, y, 0], [0, y, 0], [0, 0, z], [x, 0, z], [x, y, z], [0, y, z]];
  return {
    geometryKey: key, geometryId: `geometry-${key}`,
    mesh: {
      vertices,
      triangles: [[0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
        [1, 2, 6], [1, 6, 5], [2, 3, 7], [2, 7, 6], [3, 0, 4], [3, 4, 7]],
      faceIds: [0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5],
      edges: [
        [0, 1], [1, 2], [2, 3], [3, 0], [4, 5], [5, 6], [6, 7], [7, 4], [0, 4], [1, 5], [2, 6], [3, 7],
      ].map(([a, b], index) => ({ localId: index + 1, points: [vertices[a], vertices[b]] })),
      topologyVertices: vertices.map((point, index) => ({ localId: index + 1, point })),
    },
    bbox: { min: [0, 0, 0], max: size },
    topology: { faces: 6, edges: 12, vertices: 8, solids: 1 },
    volume: x * y * z, occtVersion: "mock-7.9.1", glbBytes: 4096, brepBytes: 2048,
    evaluatorVersion: "mock-v1", workerId: "mock-geometry-1", storageState: "DATABASE", createdAt: now(),
    visualization: { schemaVersion: 1, referenceGeometry: { datumPlanes, axisSystems }, primitives: [] },
  };
}

const partID = "mock-part-bracket";
const productID = "mock-product-frame";
const partArtifact = boxArtifact("mock-bracket-v3", [72, 38, 16]);
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
    part: { units: "mm", datumPlanes, axisSystems, features: [
      { id: "mock-sketch-1", type: "SKETCH", name: "Sketch 1", plane: "XY", sketch: { schemaVersion: 2, support: { type: "DATUM_PLANE", datumPlaneId: "datum-xy", plane: "XY", status: "CONNECTED" }, entities: [
        { id:"bottom",kind:"LINE",role:"PROFILE",start:{x:0,y:0},end:{x:72,y:0} }, { id:"right",kind:"LINE",role:"PROFILE",start:{x:72,y:0},end:{x:72,y:38} },
        { id:"top",kind:"LINE",role:"PROFILE",start:{x:72,y:38},end:{x:0,y:38} }, { id:"left",kind:"LINE",role:"PROFILE",start:{x:0,y:38},end:{x:0,y:0} },
      ], constraints: [], solve:{status:"UNDER_CONSTRAINED",degreesOfFreedom:4} } },
      { id: "mock-pad-1", type: "PAD", name: "Extrude 1", profile: "mock-sketch-1", length: 16, operation: "ADD" },
    ], parameters: [{ parameterId: "parameter:mock-pad-1:length", key: "mock_pad_1_length", label: "Length",
      valueType: "QUANTITY", dimension: lengthDimension, displayUnit: "mm", role: "INPUT",
      source: { literal: { siValue: 0.016, dimension: lengthDimension } },
      evaluatedValue: { siValue: 0.016, dimension: lengthDimension } }] },
    artifact: partArtifact,
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
      { id: "Frame Assembly/mock-instance-a/part", name: "Bracket A", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [-45, 0, 0], occurrencePath: "mock-instance-a", instancePath: mockInstancePath(productID, { id: "mock-instance-a", name: "Bracket A", documentId: partID, versionId: "mock-part-v3", translation: [-45,0,0] }), bodyTreeNodeId: `document:${productID}/instance:mock-instance-a/reference/body` },
      { id: "Frame Assembly/mock-instance-b/part", name: "Bracket B", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [45, 0, 0], occurrencePath: "mock-instance-b", instancePath: mockInstancePath(productID, { id: "mock-instance-b", name: "Bracket B", documentId: partID, versionId: "mock-part-v3", translation: [45,0,0] }), bodyTreeNodeId: `document:${productID}/instance:mock-instance-b/reference/body` },
    ],
  }],
]);
const undoSnapshots = new Map<string, DocumentView[]>();
const redoSnapshots = new Map<string, DocumentView[]>();

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
  const index = openDocumentIDs.indexOf(documentID);
  if (index >= 0) openDocumentIDs.splice(index, 1);
  openDocumentIDs.unshift(documentID);
  const summary = summaries.find((item) => item.id === documentID);
  if (summary) summary.lastOpenedAt = now();
}
const folders: FolderSummary[] = [{
  id: "mock-folder", name: "Concepts", description: "Early design studies", documentCount: 0,
  trashCount: 0, childCount: 0, createdAt: now(), updatedAt: now(), permission: "OWNER",
}];
const shares: ShareGrant[] = [];
const jobs = new Map<string, Job>();

function mockStructure(view: DocumentView, path = `document:${view.document.id}`, visiting = new Set<string>()): DocumentStructureNode {
  if (visiting.has(view.document.id)) return { id: path, kind: "REFERENCE_CYCLE", name: view.document.name,
    documentId: view.document.id, documentType: view.document.type, versionId: view.document.versionId };
  const nextVisiting = new Set(visiting).add(view.document.id);
  if (view.document.type === "PART") {
    const features = view.part?.features ?? [];
    const sketches = new Map(features.filter((feature) => feature.type.toUpperCase().includes("SKETCH"))
      .map((feature) => [feature.id, feature]));
    const consumed = new Set(features.filter((feature) => feature.type.toUpperCase() === "PAD" && feature.profile)
      .map((feature) => feature.profile!));
    const editable = path === `document:${view.document.id}`;
    const featureNode = (feature: Feature, parent: string, deletable: boolean): DocumentStructureNode => {
      const node: DocumentStructureNode = {
        id: `${parent}/${feature.type.toLowerCase()}:${feature.id}`,
        kind: feature.type.toUpperCase().includes("SKETCH") ? "SKETCH" : feature.type.toUpperCase() === "PAD" ? "PAD" : "IMPORT",
        name: feature.name ?? feature.type, entityId: feature.id, entityType: feature.type,
        documentId: view.document.id, versionId: view.document.versionId,
        definitionDigest: feature.type.toUpperCase() === "PAD" || feature.type.toUpperCase() === "LINEAR_EXTRUDE" ? JSON.stringify(feature) : undefined,
        capabilities: deletable ? ["DELETE", ...(["PAD","LINEAR_EXTRUDE"].includes(feature.type.toUpperCase()) ? ["EDIT" as const] : [])] : undefined,
      };
      if (feature.sketch) node.children = [
        { id: `${node.id}/geometry`, kind: "SKETCH_GEOMETRY_SET", name: "Geometry", ownerEntityId: feature.id,
          children: feature.sketch.entities.map((entity, index) => ({ id: `${node.id}/geometry/entity:${entity.id}`,
            kind: "SKETCH_ENTITY", name: `${entity.kind === "LINE" ? "Line" : "Point"} ${index + 1}`, entityId: entity.id,
            ownerEntityId: feature.id, entityType: entity.kind, role: entity.role, suppressed: entity.suppressed, documentId: view.document.id,
            capabilities: editable ? ["DELETE","SUPPRESS"] : undefined })) },
        { id: `${node.id}/external-geometry`, kind: "SKETCH_EXTERNAL_GEOMETRY_SET", name: "External Geometry", ownerEntityId: feature.id,
          children: (feature.sketch.externalGeometry??[]).map((external,index)=>({id:`${node.id}/external-geometry/external:${external.id}`,
            kind:"SKETCH_EXTERNAL_GEOMETRY",name:`External ${index+1}`,entityId:external.id,ownerEntityId:feature.id,entityType:"EXTERNAL",
            role:"CONSTRUCTION",diagnostic:external.diagnosticCode,documentId:view.document.id,capabilities:editable?["DETACH","RECONNECT"]:undefined})) },
        { id: `${node.id}/constraints`, kind: "SKETCH_CONSTRAINT_SET", name: "Constraints", ownerEntityId: feature.id,
          children: [
            { id: `${node.id}/constraints/logical`, kind: "SKETCH_LOGICAL_CONSTRAINT_SET", name: "Geometric Constraints",
              children: feature.sketch.constraints.filter((constraint) => !["DISTANCE","LENGTH","RADIUS","DIAMETER","ANGLE"].includes(constraint.kind))
                .map((constraint, index) => ({ id: `${node.id}/constraints/logical/constraint:${constraint.id}`,
                  kind: "SKETCH_CONSTRAINT", name: `${constraint.kind} ${index + 1}`, entityId: constraint.id,
                  ownerEntityId: feature.id, entityType: constraint.kind, suppressed: constraint.suppressed,
                  diagnostic: feature.sketch?.solve.conflictingConstraintIds?.includes(constraint.id)?"CONFLICTING":feature.sketch?.solve.redundantConstraintIds?.includes(constraint.id)?"REDUNDANT":undefined,
                  documentId: view.document.id, capabilities: editable ? ["DELETE","SUPPRESS"] : undefined })) },
            { id: `${node.id}/constraints/dimensions`, kind: "SKETCH_DIMENSION_SET", name: "Dimensions",
              children: feature.sketch.constraints.filter((constraint) => ["DISTANCE","LENGTH","RADIUS","DIAMETER","ANGLE"].includes(constraint.kind))
                .map((constraint, index) => ({ id: `${node.id}/constraints/dimensions/constraint:${constraint.id}`,
                  kind: "SKETCH_CONSTRAINT", name: `${constraint.kind} ${index + 1}`, entityId: constraint.id,
                  ownerEntityId: feature.id, entityType: constraint.kind, suppressed: constraint.suppressed,
                  diagnostic: feature.sketch?.solve.conflictingConstraintIds?.includes(constraint.id)?"CONFLICTING":feature.sketch?.solve.redundantConstraintIds?.includes(constraint.id)?"REDUNDANT":undefined,
                  documentId: view.document.id, capabilities: editable ? ["DELETE","SUPPRESS"] : undefined })) },
          ] },
      ];
      return node;
    };
    const bodyChildren = features.filter((feature) => !consumed.has(feature.id)).map((feature) => {
      const node = featureNode(feature, `${path}/body`, editable && !consumed.has(feature.id));
      const sketch = feature.profile ? sketches.get(feature.profile) : undefined;
      if (sketch) node.children = [featureNode(sketch, node.id, false)];
      return node;
    });
    return { id: path, kind: "PART", name: view.document.name, documentId: view.document.id,
      documentType: "PART", versionId: view.document.versionId, children: [
        { id: `${path}/origin`, kind: "ORIGIN", name: "Origin", documentId: view.document.id, children: [
          ...(view.datumPlanes ?? []).map((plane) => ({ id: `${path}/origin/plane:${plane.id}`, kind: "PLANE" as const,
            name: plane.name, entityId: plane.id, documentId: view.document.id, plane: plane.plane })),
          ...(view.axisSystems ?? []).map((axis) => ({ id: `${path}/origin/axis:${axis.id}`, kind: "AXIS_SYSTEM" as const,
            name: axis.name, entityId: axis.id, documentId: view.document.id, children: (["X", "Y", "Z"] as const).map((name) => ({
              id: `${path}/origin/axis:${axis.id}/${name.toLowerCase()}`, kind: "AXIS" as const, name: `${name} Axis`,
              entityId: axis.id, axis: name, documentId: view.document.id,
            })) })),
        ] },
        { id: `${path}/body`, kind: "BODY", name: "PartBody", documentId: view.document.id, children: bodyChildren },
      ] };
  }
  const instanceNodes: DocumentStructureNode[] = (view.product?.instances ?? []).map((instance) => {
      const referenced = views.get(instance.documentId);
      const referenceTree = referenced ? mockStructure(referenced, `${path}/instance:${instance.id}/reference`, nextVisiting) : undefined;
      const referenceName = referenced?.document.name ?? "Reference";
      const referenceMode = instance.referenceMode ?? "FOLLOW_HEAD";
      const instancePath = mockInstancePath(view.document.id,instance);
      const inOccurrence = (node:DocumentStructureNode):DocumentStructureNode => ({...node,
        instancePath:node.instancePath ?? instancePath,versionId:node.versionId ?? instance.versionId,
        children:node.children?.map(inOccurrence)});
      return { id: `${path}/instance:${instance.id}`, kind: "INSTANCE" as const, name: `${referenceName}(${instance.name})`,
        referenceName, instanceName: instance.name,
        entityId: instance.id, documentId: instance.documentId, documentType: referenced?.document.type,
        versionId: instance.versionId, referenceMode,
        instancePath: mockInstancePath(view.document.id, instance),
        capabilities: path === `document:${view.document.id}` ? ["DELETE" as const,
          referenceMode === "PINNED" ? "FOLLOW_HEAD" as const : "PIN_VERSION" as const] : undefined, children: referenceTree?.children?.map(inOccurrence) };
    });
  const constraints = view.product?.constraints ?? [];
  const constraintGroup = constraints.length ? [{ id: `${path}/assembly-constraints`, kind: "ASSEMBLY_CONSTRAINT_SET" as const, name: "约束", documentId:view.document.id, capabilities:["SUPPRESS" as const], suppressed:constraints.every(c=>c.suppressed),
    children: constraints.map((constraint, index) => {
      const disconnected = constraint.evaluationStatus === "BROKEN" || [constraint.first, constraint.second].filter(Boolean).some((reference) =>
        reference?.resolution?.result.supportingElementStatus === "NOT_CONNECTED");
      return { id: `${path}/assembly-constraints/constraint:${constraint.id}`, kind: "ASSEMBLY_CONSTRAINT" as const,
        suppressed:constraint.suppressed, name: `#${constraint.kind}.${index + 1}`, entityId: constraint.id, entityType: constraint.kind, documentId: view.document.id,
        diagnostic: `${constraint.evaluationStatus}: ${constraint.evaluationSummary ?? ""}`,
        capabilities: ["EDIT" as const, "DELETE" as const, "SUPPRESS" as const,
          ...(disconnected ? ["RECONNECT" as const] : []), ...(constraint.evaluationStatus !== "VERIFIED" ? ["REFRESH" as const] : [])] };
    }) }] : [];
  return { id: path, kind: "PRODUCT", name: view.document.name, documentId: view.document.id,
    documentType: "PRODUCT", versionId: view.document.versionId, children: [...instanceNodes, ...constraintGroup] };
}

function getView(documentID: string): DocumentView {
  const view = views.get(documentID);
  if (!view) throw new Error("文档不存在");
  view.structureTree = mockStructure(view);
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
    bodyTreeNodeId: `document:${view.document.id}/instance:${instance.id}/reference/body`,
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
    if (commandType === "CREATE_SKETCH" && view.part) {
      const plane=(input.targetKind === "FACE" ? "CUSTOM" : input.plane) as "XY"|"XZ"|"YZ"|"CUSTOM";
      const datumPlaneId=String(input.datumPlaneId ?? `datum-${plane.toLowerCase()}`);
      view.part.features.push({ id: id("mock-sketch"), type: "SKETCH", name: `Sketch ${view.part.features.length + 1}`, plane,
        sketch:{schemaVersion:2,support:input.targetKind === "FACE"
          ? {type:"PLANAR_FACE",plane:"CUSTOM",status:"CONNECTED",origin:[0,0,0],xDirection:[1,0,0],normal:[0,0,1]}
          : {type:"DATUM_PLANE",datumPlaneId,plane,status:"CONNECTED"},entities:[],constraints:[],solve:{status:"EMPTY",degreesOfFreedom:0}} });
    }
    if(commandType==="CREATE_DATUM_PLANE"&&view.part){const datum={id:id("mock-plane"),name:String(input.name),plane:"CUSTOM" as const,
      origin:input.origin as Vec3,normal:input.normal as Vec3,uDirection:input.uDirection as Vec3,size:180};
      view.part.datumPlanes.push(datum);view.datumPlanes?.push(datum);}
    if(commandType==="CREATE_DATUM_AXIS"&&view.part){const datum={id:id("mock-axis"),name:String(input.name),origin:input.origin as Vec3,direction:input.direction as Vec3};
      view.part.datumAxes=[...(view.part.datumAxes??[]),datum];view.datumAxes=[...(view.datumAxes??[]),datum];}
    if (commandType === "EDIT_SKETCH" && view.part) {
      const sketch=view.part.features.find((feature)=>feature.id===input.sketchId)?.sketch;
      for (const operation of input.operations as SketchOperation[] ?? []) {
        if (operation.type==="ADD_ENTITY") sketch?.entities.push(operation.entity);
        if (operation.type==="ADD_CONSTRAINT") sketch?.constraints.push(operation.constraint);
        if (operation.type==="UPDATE_CONSTRAINT_PLACEMENT" && sketch) {
          const constraint=sketch.constraints.find((candidate)=>candidate.id===operation.constraintId);
          if(constraint)constraint.labelPosition=operation.labelPosition;
        }
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
      if (sketch?.sketch) { const points=sketch.sketch.entities.flatMap((entity)=>sampleSketchEntity(entity));const xs=points.map((point)=>point[0]),ys=points.map((point)=>point[1]);if(points.length>0)view.artifact=boxArtifact(id("mock-shape"),[Math.max(...xs)-Math.min(...xs),Math.max(...ys)-Math.min(...ys),Number(input.length)]); }
    }
    if (commandType === "CREATE_SOLID_FEATURE" && view.part) {
      const generator = String(input.generator) as "LINEAR_EXTRUDE" | "REVOLVE";
      const featureID = id(generator === "REVOLVE" ? "mock-revolve" : "mock-extrude");
      const referenced = view.part.parameters?.find((parameter) => parameter.key === input.lengthExpression);
      const resolvedLength = Number(input.length) || (referenced?.evaluatedValue?.siValue ?? 0.04) * 1000;
      view.part.features.push({ id: featureID, type: generator,
        name: `${generator === "REVOLVE" ? "Revolve" : "Extrude"} ${view.part.features.length + 1}`,
        profile: String(input.sketchId), length: generator === "LINEAR_EXTRUDE" ? resolvedLength : undefined, angle: Number(input.angle) || undefined,
        axisEntityId: input.axisEntityId ? String(input.axisEntityId) : undefined,
        operation: String(input.operation) as "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT" });
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
	if (commandType === "CREATE_PUBLICATION" && view.part) {
		const publicationType = String(input.publicationType) as Publication["type"];
		const targetKind = String(input.targetKind);
		const publication: Publication = { id: id("mock-publication"), name: String(input.name || publicationType), type: publicationType,
			semanticPurpose: String(input.semanticPurpose ?? ""), compatibilityVersion: String(input.compatibilityVersion || "1.0.0"),
			target: targetKind === "PARAMETER" ? { kind: "PARAMETER", parameterId: String(input.targetId) }
				: targetKind === "FACE" || targetKind === "EDGE" ? { kind: "TOPOLOGY", sourceVersionId: view.document.versionId }
				: targetKind === "BODY" ? { kind: "FEATURE_OUTPUT", featureId: String(input.targetId), outputSlot: "BODY" }
				: { kind: "DATUM", datumId: String(input.targetId), axis: input.axis as "X"|"Y"|"Z"|undefined },
			contract: {}, resolution: { status: "CONNECTED", resolvedVersionId: view.document.versionId,
				geometryKey: String(input.geometryKey ?? view.artifact?.geometryKey ?? "") } };
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
				geometryKey: String(input.geometryKey ?? view.artifact?.geometryKey ?? "") };
		}
	}
	if (commandType === "DELETE_PUBLICATION" && view.part) {
		view.part.publications = (view.part.publications ?? []).filter((candidate) => candidate.id !== input.publicationId);
	}
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
        } else if (kind === "FEATURE" && view.part) {
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
    let documents = summaries.filter((item) => options.scope === "trash" ? item.deletedAt : !item.deletedAt);
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
  closeOpenDocument: async (documentID) => {
    const index = openDocumentIDs.indexOf(documentID);
    if (index >= 0) openDocumentIDs.splice(index, 1);
    await pause(undefined);
  },
  listFolders: async (parentID = "") => pause(folders.filter((folder) => (folder.parentId ?? "") === parentID)),
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
  deleteFolder: async (folderID) => { const index = folders.findIndex((folder) => folder.id === folderID); if (index >= 0) folders.splice(index, 1); },
  getDocument: async (documentID) => { markDocumentOpen(documentID); return pause(getView(documentID)); },
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
      part: { units: "mm", datumPlanes, axisSystems, features: [] } };
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
    const artifacts = view.artifact ? [view.artifact] : Object.values(view.artifacts ?? {});
    return pause({ documentId: documentID, versionId: view.document.versionId, documentType: view.document.type, units: "mm", artifacts,
      aggregate: { artifactCount: artifacts.length, triangleCount: artifacts.reduce((sum, item) => sum + item.mesh.triangles.length, 0),
        vertexCount: artifacts.reduce((sum, item) => sum + item.mesh.vertices.length, 0),
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
      part: { units: "mm", datumPlanes, axisSystems, features: [] } }
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
    return pause({ previewId: id("mock-command-preview"), baseVersionId: view.document.versionId,
      baseSequence: 0, modelHash: "mock-preview", artifact });
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
  detachContextReference: async (documentID,contextReferenceID) => command(documentID,{type:"DETACH_CONTEXT_REFERENCE",contextReferenceId:contextReferenceID}),
  move: async (documentID, instanceID, translation,rotation) => command(documentID, { type: "MOVE_INSTANCE", instanceId: instanceID, translation,rotation }),
  addAssemblyConstraint: async (documentID, input) => command(documentID, { type: "ADD_ASSEMBLY_CONSTRAINT", ...input }),
  editAssemblyConstraint: async (documentID, constraintId, input) => command(documentID, { type: "EDIT_ASSEMBLY_CONSTRAINT", targetId: constraintId, ...input }),
  setReferenceMode: async (documentID, instanceID, referenceMode) => command(documentID, { type: "SET_REFERENCE_MODE", instanceId: instanceID, referenceMode }),
  updateReferences: async (documentID) => command(documentID, { type: "UPDATE_REFERENCES" }),
  undo: async (documentID) => command(documentID, { type: "UNDO" }),
  redo: async (documentID) => command(documentID, { type: "REDO" }),
  restore: async (documentID, versionID) => command(documentID, { type: "RESTORE", versionId: versionID }),
  importDocument: async (file, folderID) => {
    const documentID = id("mock-document"); const name = file.name;
    const document: DocumentSummary = { id: documentID, name, description: "", type: "PART", versionId: id("mock-version"),
      canUndo: true, canRedo: false, createdAt: now(), lastUpdated: now(), folderId: folderID || undefined,
      workspaceName: "Main", permission: "OWNER" };
    const view: DocumentView = { document, datumPlanes, axisSystems, part: { units: "mm", datumPlanes, axisSystems,
      features: [{ id: id("mock-import"), type: "IMPORT_BODY", fileName: file.name }] }, artifact: boxArtifact(id("mock-exchange"), [48, 32, 26]) };
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
	downloadAssemblyReplay: async () => { throw new Error("Mock 模式没有真实三维求解记录"); },
	downloadDiagnosticBundle: async () => { /* server diagnostics are unavailable in mock mode */ },
};
