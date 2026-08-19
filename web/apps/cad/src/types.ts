export type Vec2 = [number, number];
export type Vec3 = [number, number, number];
export type PlaneName = "XY" | "XZ" | "YZ";

export type MeshData = {
  vertices: Vec3[];
  triangles: [number, number, number][];
  faceIds: number[];
  edges: Array<{ localId: number; points: Vec3[] }> | null;
  topologyVertices: Array<{ localId: number; point: Vec3 }> | null;
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
	brepBytes: number;
	evaluatorVersion: string;
	workerId: string;
	storageState: "DATABASE" | "DUAL" | "OBJECT";
	createdAt: string;
	visualization: VisualizationManifest;
};

export type CommandPreview = {
  previewId: string;
  baseVersionId: string;
  baseSequence: number;
  modelHash: string;
  artifact?: Artifact;
};

export type DatumPlane = { id: string; name: string; plane: PlaneName; origin: Vec3; normal: Vec3; size: number };
export type AxisSystem = { id: string; name: string; origin: Vec3; xDirection: Vec3; yDirection: Vec3; zDirection: Vec3 };
export type ReferenceGeometry = { datumPlanes: DatumPlane[]; axisSystems: AxisSystem[] };
export type VisualPrimitive = {
  id: string; featureId: string; kind: "POINTS" | "POLYLINE" | "LINE_SEGMENTS" | "TRIANGLES";
  semantic: "SKETCH_POINT" | "SKETCH_CURVE" | "SKETCH_CONSTRAINT" | "CURVE" | "SURFACE";
  entityType?: string;
  role?: "PROFILE" | "CONSTRUCTION"; status?: string; positions: Vec3[];
  indices?: number[]; selectable: boolean;
};
export type VisualizationManifest = {
  schemaVersion: 1; referenceGeometry: ReferenceGeometry; primitives: VisualPrimitive[];
};

export type DocumentProperties = {
  documentId: string; versionId: string; documentType: "PART" | "PRODUCT"; units: string;
  artifacts: Artifact[];
  aggregate: { artifactCount: number; triangleCount: number; vertexCount: number; solidCount: number;
    glbBytes: number; brepBytes: number; resolvedInstanceCount: number };
  worker: { available: boolean; workerId?: string; occtVersion?: string; residentGeometryCount?: number; error?: string };
};

export type Feature = {
  id: string;
  type: "SKETCH" | "sketch" | "PAD" | "pad" | "IMPORT_BODY";
  name?: string;
  plane?: PlaneName;
  sketch?: SketchFeature;
  profile?: string;
  length?: number;
  operation?: "NEW" | "ADD" | "REMOVE" | "INTERSECT";
  geometryKey?: string;
  fileName?: string;
  sourceFormat?: "STEP" | "BREP";
};

export type SketchPoint2 = { x: number; y: number };
export type SketchGeometryRef = { target: "ENTITY" | "SKETCH_ORIGIN" | "SKETCH_X_AXIS" | "SKETCH_Y_AXIS"; entityId?: string; subElement: "WHOLE" | "POINT" | "START" | "END" | "DIRECTION" };
export type SketchEntity = { id: string; kind: "POINT" | "LINE"; role: "PROFILE" | "CONSTRUCTION"; point?: SketchPoint2; start?: SketchPoint2; end?: SketchPoint2 };
export type SketchConstraint = { id: string; kind: "COINCIDENT" | "PARALLEL" | "FIXED_POINT"; references: SketchGeometryRef[]; fixedPoint?: SketchPoint2 };
export type SketchFeature = { schemaVersion: 1; support: { type: "DATUM_PLANE"; datumPlaneId: string; plane: PlaneName }; entities: SketchEntity[]; constraints: SketchConstraint[]; solve: { status: string; degreesOfFreedom: number; diagnostic?: string } };
export type SketchOperation = { type: "ADD_ENTITY"; entity: SketchEntity } | { type: "ADD_CONSTRAINT"; constraint: SketchConstraint } | { type: "ADD_RECTANGLE"; first: SketchPoint2; second: SketchPoint2 };

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
  id: string; type: "EXCHANGE_IMPORT" | "EXCHANGE_EXPORT" | "THUMBNAIL_RENDER" | "ARTIFACT_BACKFILL";
  state: "QUEUED" | "RUNNING" | "RETRY_WAIT" | "SUCCEEDED" | "FAILED" | "CANCELED";
  documentId?: string; progress: number; errorMessage?: string; resultObjectId?: string;
};

export type ResolvedInstance = {
  id: string;
  name: string;
  documentId: string;
  geometryKey: string;
  translation: Vec3;
  occurrencePath: string;
  bodyTreeNodeId: string;
};

export type DocumentStructureNode = {
  id: string;
  kind: "PART" | "PRODUCT" | "INSTANCE" | "ORIGIN" | "PLANE" | "AXIS_SYSTEM" | "AXIS" | "BODY" | "SKETCH" | "PAD" | "IMPORT" | "FEATURE" | "SKETCH_GEOMETRY_SET" | "SKETCH_CONSTRAINT_SET" | "SKETCH_ENTITY" | "SKETCH_CONSTRAINT" | "REFERENCE_CYCLE";
  name: string;
  entityId?: string;
  documentId?: string;
  documentType?: "PART" | "PRODUCT";
  versionId?: string;
  plane?: PlaneName;
  axis?: "X" | "Y" | "Z";
  referenceMode?: "FOLLOW_HEAD" | "PINNED";
  ownerEntityId?: string;
  entityType?: string;
  role?: "PROFILE" | "CONSTRUCTION";
  capabilities?: Array<"DELETE">;
  children?: DocumentStructureNode[];
};

export type DocumentView = {
  document: DocumentSummary;
  datumPlanes?: DatumPlane[];
  axisSystems?: AxisSystem[];
  part?: { units: string; datumPlanes: DatumPlane[]; axisSystems: AxisSystem[]; features: Feature[] };
  product?: { instances: ProductInstance[] };
  artifact?: Artifact;
  artifacts?: Record<string, Artifact>;
  resolvedInstances?: ResolvedInstance[];
  structureTree?: DocumentStructureNode;
};

export type SelectionIdentity = {
  id: string;
  treeNodeId?: string;
  documentId?: string;
  occurrencePath?: string;
  geometryKey?: string;
  instanceId?: string;
  visualKey?: string;
};

export type Selection =
  | (SelectionIdentity & { kind: "plane"; plane: PlaneName })
  | (SelectionIdentity & { kind: "axis-system" })
  | (SelectionIdentity & { kind: "axis"; axis: "X" | "Y" | "Z" })
  | (SelectionIdentity & { kind: "sketch" })
  | (SelectionIdentity & { kind: "pad" })
  | (SelectionIdentity & { kind: "import" })
  | (SelectionIdentity & { kind: "instance" })
  | (SelectionIdentity & { kind: "body" })
  | (SelectionIdentity & { kind: "solid" })
  | (SelectionIdentity & { kind: "visual"; visualType: "POINT" | "CURVE" | "SURFACE";
      featureId: string; entityId: string; role?: "PROFILE" | "CONSTRUCTION" })
  | (SelectionIdentity & { kind: "sketch-constraint"; featureId: string; constraintId: string; constraintType: string })
  | (SelectionIdentity & { kind: "face" | "edge" | "vertex"; topologyId: number })
  | null;

export type TopologyElementProperties = {
  geometryKey: string; geometryId: string; kind: "FACE" | "EDGE" | "VERTEX"; localId: number;
  geometryType: string; bbox?: { min: Vec3; max: Vec3 }; point?: Vec3;
  properties: Record<string, number | boolean | string | Vec3>; workerId: string; occtVersion: string;
};
