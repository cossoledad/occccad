import type { CadApi } from "../api";
import type {
  Artifact, DocumentSummary, DocumentView, Feature, FolderSummary, HistoryEntry, Job,
  ProductInstance, ShareGrant, User, Vec2, Vec3,
} from "../types";

const pause = async <T>(value: T, milliseconds = 90): Promise<T> =>
  new Promise((resolve) => window.setTimeout(() => resolve(structuredClone(value)), milliseconds));
const now = (): string => new Date().toISOString();
const id = (prefix: string): string => `${prefix}-${crypto.randomUUID()}`;

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
  return {
    geometryKey: key, geometryId: `geometry-${key}`,
    mesh: {
      vertices: [[0, 0, 0], [x, 0, 0], [x, y, 0], [0, y, 0], [0, 0, z], [x, 0, z], [x, y, z], [0, y, z]],
      triangles: [[0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
        [1, 2, 6], [1, 6, 5], [2, 3, 7], [2, 7, 6], [3, 0, 4], [3, 4, 7]],
      faceIds: [1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6],
    },
    bbox: { min: [0, 0, 0], max: size },
    topology: { faces: 6, edges: 12, vertices: 8, solids: 1 },
    volume: x * y * z, occtVersion: "mock-7.9.1", glbBytes: 0,
  };
}

const partID = "mock-part-bracket";
const productID = "mock-product-frame";
const partArtifact = boxArtifact("mock-bracket-v3", [72, 38, 16]);
const summaries: DocumentSummary[] = [
  {
    id: partID, name: "Mounting Bracket", description: "参数化安装支架", type: "PART",
    versionId: "mock-part-v3", canUndo: true, canRedo: false, createdAt: now(), lastUpdated: now(),
    workspaceName: "Main", permission: "OWNER",
  },
  {
    id: productID, name: "Frame Assembly", description: "支架装配示例", type: "PRODUCT",
    versionId: "mock-product-v3", canUndo: true, canRedo: false, createdAt: now(), lastUpdated: now(),
    workspaceName: "Main", permission: "OWNER",
  },
];

const views = new Map<string, DocumentView>([
  [partID, {
    document: summaries[0],
    datumPlanes: [{ id: "datum-xy", name: "XY Plane", plane: "XY" },
      { id: "datum-xz", name: "XZ Plane", plane: "XZ" }, { id: "datum-yz", name: "YZ Plane", plane: "YZ" }],
    part: { units: "mm", features: [
      { id: "mock-sketch-1", type: "RECTANGLE_SKETCH", name: "Sketch 1", plane: "XY", rectangle: { origin: [0, 0], width: 72, height: 38 } },
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
      { id: "Frame Assembly/mock-instance-a/part", name: "Bracket A", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [-45, 0, 0] },
      { id: "Frame Assembly/mock-instance-b/part", name: "Bracket B", documentId: partID, geometryKey: partArtifact.geometryKey, translation: [45, 0, 0] },
    ],
  }],
]);

const histories = new Map<string, HistoryEntry[]>([
  [partID, [
    { position: 0, versionId: "mock-part-v1", sequence: 1, commandType: "CREATE_DOCUMENT", createdAt: now(), isHead: false },
    { position: 1, versionId: "mock-part-v2", sequence: 2, commandType: "CREATE_RECTANGLE_SKETCH", createdAt: now(), isHead: false },
    { position: 2, versionId: "mock-part-v3", sequence: 3, commandType: "PAD_SKETCH", createdAt: now(), isHead: true },
  ]],
  [productID, [{ position: 0, versionId: "mock-product-v3", sequence: 3, commandType: "MOVE_INSTANCE", createdAt: now(), isHead: true }]],
]);
const folders: FolderSummary[] = [{
  id: "mock-folder", name: "Concepts", description: "Early design studies", documentCount: 0,
  trashCount: 0, childCount: 0, createdAt: now(), updatedAt: now(), permission: "OWNER",
}];
const shares: ShareGrant[] = [];
const jobs = new Map<string, Job>();

function getView(documentID: string): DocumentView {
  const view = views.get(documentID);
  if (!view) throw new Error("文档不存在");
  return view;
}

function commit(documentID: string, commandType: string, mutate?: (view: DocumentView) => void): DocumentView {
  const view = getView(documentID);
  mutate?.(view);
  const versionID = id("mock-version");
  view.document.versionId = versionID;
  view.document.lastUpdated = now();
  view.document.canUndo = true;
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
    geometryKey: partArtifact.geometryKey, translation: instance.translation,
  }));
}

async function command(documentID: string, input: Record<string, unknown>): Promise<DocumentView> {
  const commandType = String(input.type ?? "COMMAND");
  return pause(commit(documentID, commandType, (view) => {
    if (commandType === "CREATE_RECTANGLE_SKETCH" && view.part) {
      view.part.features.push({ id: id("mock-sketch"), type: "RECTANGLE_SKETCH", name: `Sketch ${view.part.features.length + 1}`,
        plane: input.plane as "XY" | "XZ" | "YZ", rectangle: { origin: input.origin as Vec2, width: Number(input.width), height: Number(input.height) } });
    }
    if (commandType === "PAD_SKETCH" && view.part) {
      const sketch = view.part.features.find((feature) => feature.id === input.sketchId);
      view.part.features.push({ id: id("mock-pad"), type: "PAD", name: `Extrude ${view.part.features.length + 1}`,
        profile: String(input.sketchId), length: Number(input.length), operation: "ADD" });
      if (sketch?.rectangle) view.artifact = boxArtifact(id("mock-shape"), [sketch.rectangle.width, sketch.rectangle.height, Number(input.length)]);
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
  getDocument: async (documentID) => pause(getView(documentID)),
  getHistory: async (documentID) => pause(histories.get(documentID) ?? []),
  createVersion: async (documentID, name) => {
    const entries = histories.get(documentID) ?? []; const head = entries.find((entry) => entry.isHead);
    if (head) head.versionName = name; return pause(entries);
  },
  createDocument: async (type, name, description = "", folderID) => {
    const document: DocumentSummary = { id: id("mock-document"), name, description, type, versionId: id("mock-version"),
      canUndo: false, canRedo: false, createdAt: now(), lastUpdated: now(), folderId: folderID,
      workspaceName: "Main", permission: "OWNER" };
    const view: DocumentView = type === "PART" ? { document, datumPlanes: [
      { id: "datum-xy", name: "XY Plane", plane: "XY" }, { id: "datum-xz", name: "XZ Plane", plane: "XZ" },
      { id: "datum-yz", name: "YZ Plane", plane: "YZ" }], part: { units: "mm", features: [] } }
      : { document, product: { instances: [] }, artifacts: {}, resolvedInstances: [] };
    summaries.unshift(document); views.set(document.id, view); histories.set(document.id, []); return pause(view);
  },
  updateDocument: async (documentID, name, description) => pause(commit(documentID, "UPDATE_DOCUMENT", (view) => Object.assign(view.document, { name, description }))),
  deleteDocument: async (documentID) => { getView(documentID).document.deletedAt = now(); },
  restoreDocument: async (documentID) => { getView(documentID).document.deletedAt = undefined; return pause(getView(documentID)); },
  moveDocument: async (documentID, folderID) => pause(commit(documentID, "MOVE_DOCUMENT", (view) => { view.document.folderId = folderID; })),
  copyDocument: async (documentID, name, folderID) => {
    const source = getView(documentID); const copy = structuredClone(source); copy.document = { ...copy.document, id: id("mock-document"),
      name, folderId: folderID, versionId: id("mock-version"), createdAt: now(), lastUpdated: now() };
    summaries.unshift(copy.document); views.set(copy.document.id, copy); histories.set(copy.document.id, []); return pause(copy);
  },
  command,
  createSketch: async (documentID, plane, origin, width, height) => command(documentID, { type: "CREATE_RECTANGLE_SKETCH", plane, origin, width, height }),
  pad: async (documentID, sketchID, length) => command(documentID, { type: "PAD_SKETCH", sketchId: sketchID, length }),
  insert: async (documentID, referencedDocumentID, name) => command(documentID, { type: "INSERT_INSTANCE", referencedDocumentId: referencedDocumentID, name }),
  move: async (documentID, instanceID, translation) => command(documentID, { type: "MOVE_INSTANCE", instanceId: instanceID, translation }),
  setReferenceMode: async (documentID, instanceID, referenceMode) => command(documentID, { type: "SET_REFERENCE_MODE", instanceId: instanceID, referenceMode }),
  undo: async (documentID) => command(documentID, { type: "UNDO" }),
  redo: async (documentID) => command(documentID, { type: "REDO" }),
  restore: async (documentID, versionID) => command(documentID, { type: "RESTORE", versionId: versionID }),
  importStep: async (documentID, file) => {
    commit(documentID, "IMPORT_STEP", (view) => { view.part?.features.push({ id: id("mock-import"), type: "IMPORT_STEP", fileName: file.name }); view.artifact = boxArtifact(id("mock-step"), [48, 32, 26]); });
    const job: Job = { id: id("mock-job"), type: "STEP_IMPORT", state: "SUCCEEDED", documentId: documentID, progress: 100 };
    jobs.set(job.id, job); return pause(job);
  },
  startExportStep: async (documentID) => {
    const job: Job = { id: id("mock-job"), type: "STEP_EXPORT", state: "SUCCEEDED", documentId: documentID, progress: 100, resultObjectId: "mock-step" };
    jobs.set(job.id, job); return pause(job);
  },
  getJob: async (jobID) => pause(jobs.get(jobID)!),
  downloadJob: async () => { /* no file is produced in mock mode */ },
};
