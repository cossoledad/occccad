export type Vec2 = [number, number];
export type Vec3 = [number, number, number];
export type PlaneName = "XY" | "XZ" | "YZ";

export type MeshData = {
  vertices: Vec3[];
  triangles: [number, number, number][];
  faceIds: number[];
};

export type Artifact = {
  geometryKey: string;
  geometryId: string;
  mesh: MeshData;
  bbox: { min: Vec3; max: Vec3 };
  topology: { faces: number; edges: number; vertices: number; solids: number };
  volume: number;
  occtVersion: string;
  glbBytes: number;
};

export type DatumPlane = { id: string; name: string; plane: PlaneName };

export type Feature = {
  id: string;
  type: "RECTANGLE_SKETCH" | "rectangle_sketch" | "PAD" | "pad" | "IMPORT_STEP";
  name?: string;
  plane?: PlaneName;
  rectangle?: { origin: Vec2; width: number; height: number };
  origin?: Vec2;
  width?: number;
  height?: number;
  profile?: string;
  length?: number;
  operation?: "NEW" | "ADD" | "REMOVE" | "INTERSECT";
  geometryKey?: string;
  fileName?: string;
};

export type HistoryEntry = {
  position: number;
  versionId: string;
  sequence: number;
  commandType: string;
  createdAt: string;
  isHead: boolean;
  versionName?: string;
};

export type ProductInstance = {
  id: string;
  name: string;
  documentId: string;
  versionId: string;
  translation: Vec3;
  referenceMode?: "FOLLOW_HEAD" | "PINNED";
  resolvedVersionId?: string;
  headChanged?: boolean;
};

export type DocumentSummary = {
  id: string;
  name: string;
  description: string;
  type: "PART" | "PRODUCT";
  versionId: string;
  canUndo: boolean;
  canRedo: boolean;
  createdAt: string;
  lastUpdated: string;
  deletedAt?: string;
  folderId?: string;
  lastOpenedAt?: string;
  copiedFromDocumentId?: string;
  workspaceName?: string;
  permission: "OWNER" | "EDITOR" | "VIEWER";
};

export type DocumentScope = "active" | "trash" | "all";

export type DocumentPage = {
  documents: DocumentSummary[];
  total: number;
  limit: number;
  offset: number;
};

export type FolderSummary = {
  id: string;
  parentId?: string;
  name: string;
  description: string;
  documentCount: number;
  trashCount: number;
  childCount: number;
  createdAt: string;
  updatedAt: string;
  permission: "OWNER" | "EDITOR" | "VIEWER";
};

export type User = {
  id: string; email: string; displayName: string;
  status: "PENDING" | "ACTIVE" | "DISABLED";
  platformRole?: "ADMIN" | "MEMBER";
  mustChangePassword?: boolean;
  createdAt?: string;
  lastLoginAt?: string;
};
export type Team = { id: string; name: string; description: string; ownerUserId: string; memberCount: number };
export type AccessRole = "VIEWER" | "EDITOR" | "OWNER";
export type ShareGrant = {
  id: string; subjectType: "USER" | "TEAM"; subjectId: string; subjectName: string;
  role: "VIEWER" | "EDITOR"; inherited: boolean; sourceName?: string;
};
export type AuditEvent = {
  id: number; actorName: string; action: string; resourceType?: string; resourceId?: string;
  requestId?: string; metadata: Record<string, unknown>; createdAt: string;
};
export type Job = {
  id: string; type: "STEP_IMPORT" | "STEP_EXPORT" | "THUMBNAIL_RENDER" | "ARTIFACT_BACKFILL";
  state: "QUEUED" | "RUNNING" | "RETRY_WAIT" | "SUCCEEDED" | "FAILED" | "CANCELED";
  documentId?: string; progress: number; errorMessage?: string; resultObjectId?: string;
};

export type ResolvedInstance = {
  id: string;
  name: string;
  documentId: string;
  geometryKey: string;
  translation: Vec3;
};

export type DocumentView = {
  document: DocumentSummary;
  datumPlanes?: DatumPlane[];
  part?: { units: string; features: Feature[] };
  product?: { instances: ProductInstance[] };
  artifact?: Artifact;
  artifacts?: Record<string, Artifact>;
  resolvedInstances?: ResolvedInstance[];
};

export type Selection =
  | { kind: "plane"; id: string; plane: PlaneName }
  | { kind: "sketch"; id: string }
  | { kind: "pad"; id: string }
  | { kind: "import"; id: string }
  | { kind: "instance"; id: string }
  | { kind: "solid"; id: string }
  | null;

export type RectangleDraft = {
  plane: PlaneName;
  origin: Vec2;
  width: number;
  height: number;
};
