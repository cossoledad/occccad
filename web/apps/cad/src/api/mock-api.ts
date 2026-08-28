import type { CadApi } from "../api";
import type {
  Artifact, DocumentProperties, DocumentStructureNode, DocumentSummary, DocumentView, Feature, FolderSummary, HistoryEntry, Job,
  ProductInstance, ShareGrant, SketchOperation, User, Vec3,
} from "../types";
import { sampleSketchEntity } from "../cad/sketch/sketch-geometry";
import { randomUUID } from "../utils/random-uuid";

const pause = async <T>(value: T, milliseconds = 90): Promise<T> =>
  new Promise((resolve) => window.setTimeout(() => resolve(structuredClone(value)), milliseconds));
const now = (): string => new Date().toISOString();
const id = (prefix: string): string => `${prefix}-${randomUUID()}`;
const mockTopologyProperties = (kind: "FACE" | "EDGE" | "VERTEX"): Record<string, number | boolean | string | Vec3> => {
  if (kind === "FACE") return { area: 100, normal: [0, 0, 1] };
  if (kind === "EDGE") return { length: 10, direction: [1, 0, 0] };
  return { tolerance: 1e-7 };
};
const datumPlanes = [
  { id: "datum-xy", name: "XY Plane", plane: "XY" as const, origin: [0, 0, 0] as Vec3, normal: [0, 0, 1] as Vec3, size: 180 },
  { id: "datum-xz", name: "XZ Plane", plane: "XZ" as const, origin: [0, 0, 0] as Vec3, normal: [0, 1, 0] as Vec3, size: 180 },
  { id: "datum-yz", name: "YZ Plane", plane: "YZ" as const, origin: [0, 0, 0] as Vec3, normal: [1, 0, 0] as Vec3, size: 180 },
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
      { id: "mock-sketch-1", type: "SKETCH", name: "Sketch 1", plane: "XY", sketch: { schemaVersion: 1, support: { type: "DATUM_PLANE", datumPlaneId: "datum-xy", plane: "XY" }, entities: [
        { id:"bottom",kind:"LINE",role:"PROFILE",start:{x:0,y:0},end:{x:72,y:0} }, { id:"right",kind:"LINE",role:"PROFILE",start:{x:72,y:0},end:{x:72,y:38} },
        { id:"top",kind:"LINE",role:"PROFILE",start:{x:72,y:38},end:{x:0,y:38} }, { id:"left",kind:"LINE",role:"PROFILE",start:{x:0,y:38},end:{x:0,y:0} },
      ], constraints: [], solve:{status:"UNDER_CONSTRAINED",degreesOfFreedom:4} } },
      { id: "mock-pad-1", type: "PAD", name: "Extrude 1", profile: "mock-sketch-1", length: 16, operation: "ADD" },
    ] },
    artifact: partArtifact,
  }],
  [productID, {
    document: summaries[1], product: { instances: [
      { id: "mock-instance-a", name: "Bracket A", documentId: partID, versionId: "mock-part-v3", translation: [-45, 0, 0], referenceMode: "FOLLOW_HEAD" },
      { id: "mock-instance-b", name: "Bracket B", documentId: partID, versionId: "mock-part-v3", translation: [45, 0, 0], referenceMode: "FOLLOW_HEAD" },
    ] },
    artifacts: { [partArtifact.geometryKey]: partArtifact },
    resolvedInstances: [
      { id: "Frame Assembly/mock-instance-a/part", name: "Bracket A", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [-45, 0, 0], occurrencePath: "mock-instance-a", bodyTreeNodeId: `document:${productID}/instance:mock-instance-a/reference/body` },
      { id: "Frame Assembly/mock-instance-b/part", name: "Bracket B", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [45, 0, 0], occurrencePath: "mock-instance-b", bodyTreeNodeId: `document:${productID}/instance:mock-instance-b/reference/body` },
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
        capabilities: deletable ? ["DELETE"] : undefined,
      };
      if (feature.sketch) node.children = [
        { id: `${node.id}/geometry`, kind: "SKETCH_GEOMETRY_SET", name: "Geometry", ownerEntityId: feature.id,
          children: feature.sketch.entities.map((entity, index) => ({ id: `${node.id}/geometry/entity:${entity.id}`,
            kind: "SKETCH_ENTITY", name: `${entity.kind === "LINE" ? "Line" : "Point"} ${index + 1}`, entityId: entity.id,
            ownerEntityId: feature.id, entityType: entity.kind, role: entity.role, documentId: view.document.id,
            capabilities: editable ? ["DELETE"] : undefined })) },
        { id: `${node.id}/constraints`, kind: "SKETCH_CONSTRAINT_SET", name: "Constraints", ownerEntityId: feature.id,
          children: [
            { id: `${node.id}/constraints/logical`, kind: "SKETCH_LOGICAL_CONSTRAINT_SET", name: "Geometric Constraints",
              children: feature.sketch.constraints.filter((constraint) => !["DISTANCE","LENGTH","RADIUS","DIAMETER","ANGLE"].includes(constraint.kind))
                .map((constraint, index) => ({ id: `${node.id}/constraints/logical/constraint:${constraint.id}`,
                  kind: "SKETCH_CONSTRAINT", name: `${constraint.kind} ${index + 1}`, entityId: constraint.id,
                  ownerEntityId: feature.id, entityType: constraint.kind, documentId: view.document.id,
                  capabilities: editable ? ["DELETE"] : undefined })) },
            { id: `${node.id}/constraints/dimensions`, kind: "SKETCH_DIMENSION_SET", name: "Dimensions",
              children: feature.sketch.constraints.filter((constraint) => ["DISTANCE","LENGTH","RADIUS","DIAMETER","ANGLE"].includes(constraint.kind))
                .map((constraint, index) => ({ id: `${node.id}/constraints/dimensions/constraint:${constraint.id}`,
                  kind: "SKETCH_CONSTRAINT", name: `${constraint.kind} ${index + 1}`, entityId: constraint.id,
                  ownerEntityId: feature.id, entityType: constraint.kind, documentId: view.document.id,
                  capabilities: editable ? ["DELETE"] : undefined })) },
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
  return { id: path, kind: "PRODUCT", name: view.document.name, documentId: view.document.id,
    documentType: "PRODUCT", versionId: view.document.versionId, children: (view.product?.instances ?? []).map((instance) => {
      const referenced = views.get(instance.documentId);
      const referenceTree = referenced ? mockStructure(referenced, `${path}/instance:${instance.id}/reference`, nextVisiting) : undefined;
      return { id: `${path}/instance:${instance.id}`, kind: "INSTANCE", name: instance.name,
        entityId: instance.id, documentId: instance.documentId, documentType: referenced?.document.type,
        versionId: instance.versionId, referenceMode: instance.referenceMode ?? "FOLLOW_HEAD",
        capabilities: path === `document:${view.document.id}` ? ["DELETE"] : undefined, children: referenceTree?.children };
    }) };
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
  return view;
}

function rebuildProduct(view: DocumentView): void {
  view.artifacts = { [partArtifact.geometryKey]: partArtifact };
  view.resolvedInstances = (view.product?.instances ?? []).map((instance) => ({
    id: `${view.document.name}/${instance.id}/part`, name: instance.name, documentId: partID,
    geometryKey: partArtifact.geometryKey, translation: instance.translation, occurrencePath: instance.id,
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
      const plane=input.plane as "XY"|"XZ"|"YZ";
      view.part.features.push({ id: id("mock-sketch"), type: "SKETCH", name: `Sketch ${view.part.features.length + 1}`, plane,
        sketch:{schemaVersion:1,support:{type:"DATUM_PLANE",datumPlaneId:`datum-${plane.toLowerCase()}`,plane},entities:[],constraints:[],solve:{status:"EMPTY",degreesOfFreedom:0}} });
    }
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
    if (commandType === "INSERT_INSTANCE" && view.product) {
      view.product.instances.push({ id: id("mock-instance"), name: String(input.name || "Instance"),
        documentId: String(input.referencedDocumentId), versionId: getView(String(input.referencedDocumentId)).document.versionId,
        translation: [0, 0, 0], referenceMode: "FOLLOW_HEAD" });
      rebuildProduct(view);
    }
    if (commandType === "MOVE_INSTANCE" && view.product) {
      const instance = view.product.instances.find((candidate) => candidate.id === input.instanceId);
      if (instance) instance.translation = input.translation as Vec3;
      rebuildProduct(view);
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
    if (options.shared) documents = documents.filter((item) => item.permission !== "OWNER");
    documents = [...documents].sort((left, right) => options.sort === "name" ? left.name.localeCompare(right.name)
      : options.sort === "created" ? right.createdAt.localeCompare(left.createdAt)
      : (right.lastOpenedAt ?? right.lastUpdated).localeCompare(left.lastOpenedAt ?? left.lastUpdated));
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
  folderBreadcrumbs: async (folderID) => pause(folders.filter((folder) => folder.id === folderID)),
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
    properties: mockTopologyProperties(kind),
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
    if (input.type !== "PAD_SKETCH" || !view.part) throw new Error("Mock preview currently supports PAD_SKETCH only");
    const sketch = view.part.features.find((feature) => feature.id === input.sketchId)?.sketch;
    const points = sketch?.entities.flatMap((entity) => [entity.start, entity.end]).filter(Boolean) as Array<{x:number;y:number}>;
    if (!points?.length) throw new Error("Preview profile is empty");
    const xs = points.map((point) => point.x), ys = points.map((point) => point.y);
    const artifact = boxArtifact(id("mock-preview"), [Math.max(...xs)-Math.min(...xs), Math.max(...ys)-Math.min(...ys), Number(input.length)]);
    return pause({ previewId: id("mock-command-preview"), baseVersionId: view.document.versionId,
      baseSequence: 0, modelHash: "mock-preview", artifact });
  },
  createSketch: async (documentID, plane) => command(documentID, { type: "CREATE_SKETCH", plane }),
  editSketch: async (documentID, sketchID, operations) => command(documentID, { type: "EDIT_SKETCH", sketchId: sketchID, operations }),
  deleteNode: async (documentID, targetKind, targetID, ownerEntityID) => command(documentID, { type: "DELETE_NODE", targetKind, targetId: targetID, ownerEntityId: ownerEntityID }),
  deleteNodes: async (documentID, targets) => command(documentID, { type: "DELETE_NODES", targets }),
  pad: async (documentID, sketchID, length, intentRequestID) => command(documentID, { type: "PAD_SKETCH", sketchId: sketchID, length,
    ...(intentRequestID ? { requestId: intentRequestID } : {}) }),
  insert: async (documentID, referencedDocumentID, name) => command(documentID, { type: "INSERT_INSTANCE", referencedDocumentId: referencedDocumentID, name }),
  move: async (documentID, instanceID, translation) => command(documentID, { type: "MOVE_INSTANCE", instanceId: instanceID, translation }),
  setReferenceMode: async (documentID, instanceID, referenceMode) => command(documentID, { type: "SET_REFERENCE_MODE", instanceId: instanceID, referenceMode }),
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
  startExport: async (documentID) => {
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
};
