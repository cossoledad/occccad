export type Vec2 = [number, number];
export type Vec3 = [number, number, number];
export type PlaneName = "XY" | "XZ" | "YZ";
export type SketchPlane = { datumPlaneId: string; plane: PlaneName | "CUSTOM"; origin: Vec3; normal: Vec3; uDirection: Vec3 };
export type ToolbarCatalogItem = { commandId:string;name:string;helpText:string;iconKey:string;groupKey:string;sortOrder:number;repeatable:boolean };
export type ToolbarCatalogEntry = { id:string;name:string;workbench:"ALL"|"PART_DESIGN"|"SKETCHER"|"ASSEMBLY_DESIGN";
  position:"top-left"|"top-center"|"top-right"|"bottom-left"|"bottom-center"|"bottom-right";
  orientation:"horizontal"|"vertical";styleKey:"standard"|"part"|"sketch"|"assembly"|"debug";sortOrder:number;items:ToolbarCatalogItem[] };
export type ToolbarCatalog = { schemaVersion:1;toolbars:ToolbarCatalogEntry[] };

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

export type DatumPlane = { id: string; name: string; plane: PlaneName | "CUSTOM"; origin: Vec3; normal: Vec3; uDirection: Vec3; size: number };
export type DatumAxis = { id: string; name: string; origin: Vec3; direction: Vec3 };
export type AxisSystem = { id: string; name: string; origin: Vec3; xDirection: Vec3; yDirection: Vec3; zDirection: Vec3 };
export type ReferenceGeometry = { datumPlanes: DatumPlane[]; axisSystems: AxisSystem[]; datumAxes?: DatumAxis[] };
export type VisualPrimitive = {
  id: string; featureId: string; kind: "POINTS" | "POLYLINE" | "LINE_SEGMENTS" | "TRIANGLES";
  semantic: "SKETCH_POINT" | "SKETCH_CURVE" | "SKETCH_CONSTRAINT" | "CURVE" | "SURFACE";
  entityType?: string;
  role?: "PROFILE" | "CONSTRUCTION"; status?: string; positions: Vec3[];
  label?: string; labelPosition?: Vec3;
  relatedEntityIds?: string[];
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
  type: "SKETCH" | "sketch" | "PAD" | "pad" | "LINEAR_EXTRUDE" | "REVOLVE" | "IMPORT_BODY";
  name?: string;
  plane?: PlaneName | "CUSTOM";
  sketch?: SketchFeature;
  profile?: string;
  length?: number;
  angle?: number;
  axisEntityId?: string;
  reversed?: boolean;
  operation?: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
  geometryKey?: string;
  fileName?: string;
  sourceFormat?: "STEP" | "BREP";
};

export type SketchPoint2 = { x: number; y: number };
export type SketchGeometryRef = { target: "ENTITY" | "SKETCH_ORIGIN" | "SKETCH_X_AXIS" | "SKETCH_Y_AXIS"; entityId?: string; subElement: "WHOLE" | "POINT" | "START" | "END" | "CENTER" | "DIRECTION" | "CONTROL"; controlPointIndex?: number };
export type SketchEntity = { id: string; kind: "POINT" | "LINE" | "CIRCLE" | "ARC" | "SPLINE"; role: "PROFILE" | "CONSTRUCTION"; suppressed?: boolean; point?: SketchPoint2; start?: SketchPoint2; end?: SketchPoint2; center?: SketchPoint2; radius?: number; startAngle?: number; endAngle?: number; controlPoints?: SketchPoint2[]; degree?: number; closed?: boolean };
export type SketchConstraint = { id: string; kind: "COINCIDENT" | "PARALLEL" | "FIXED" | "FIXED_POINT" | "HORIZONTAL" | "VERTICAL" | "PERPENDICULAR" | "TANGENT" | "EQUAL" | "DISTANCE" | "LENGTH" | "RADIUS" | "DIAMETER" | "ANGLE" | "CONCENTRIC" | "POINT_ON_OBJECT" | "MIDPOINT" | "SYMMETRY"; references: SketchGeometryRef[]; suppressed?: boolean; fixedPoint?: SketchPoint2; value?: number; unit?: "mm" | "deg"; labelPosition?: SketchPoint2; internal?: boolean };
export type SketchFeature = { schemaVersion: 1; support: { type: "DATUM_PLANE"; datumPlaneId: string; plane: PlaneName | "CUSTOM" }; entities: SketchEntity[]; constraints: SketchConstraint[]; solve: { status: string; definitionStatus?: "FULLY_CONSTRAINED"|"UNDER_CONSTRAINED"|"UNRESOLVED"; degreesOfFreedom: number; diagnostic?: string; conflictingConstraintIds?: string[]; redundantConstraintIds?: string[]; components?: Array<{entityIds:string[];constraintIds:string[];status:string;definitionStatus?:"FULLY_CONSTRAINED"|"UNDER_CONSTRAINED"|"UNRESOLVED";degreesOfFreedom:number}> } };
export type SketchOperation = { type: "ADD_ENTITY"; entity: SketchEntity } | { type: "ADD_CONSTRAINT"; constraint: SketchConstraint }
  | { type: "UPDATE_CONSTRAINT_PLACEMENT"; constraintId: string; labelPosition: SketchPoint2 }
  | { type: "UPDATE_CONSTRAINT_VALUE"; constraintId: string; value: number }
  | { type: "ADD_RECTANGLE"; first: SketchPoint2; second: SketchPoint2; firstReference?: SketchGeometryRef; secondReference?: SketchGeometryRef }
  | { type: "UPDATE_ENTITY_ROLE"; entityId: string; role: "PROFILE" | "CONSTRUCTION" }
  | { type: "UPDATE_ENTITY_POINT"; entityId: string; subElement: "POINT" | "CENTER" | "CONTROL"; controlPointIndex?: number; point: SketchPoint2 }
  | { type: "UPDATE_ENTITY_SUPPRESSION"; entityId: string; suppressed: boolean }
  | { type: "UPDATE_CONSTRAINT_SUPPRESSION"; constraintId: string; suppressed: boolean };

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
  rotation?: [number, number, number, number];
  referenceMode?: "FOLLOW_HEAD" | "PINNED";
  resolvedVersionId?: string;
  headChanged?: boolean;
};

export type AssemblyGeometryRef = { instanceId: string; kind: "BODY" | "POINT" | "AXIS" | "PLANE" | "CYLINDER" | "FACE";
  geometryId?: string; axis?: string; geometryKey?: string; topologyId?: number };
export type AssemblyConstraint = { id: string; kind: "FIX" | "RIGID" | "COINCIDENT" | "CONCENTRIC" | "ANGLE" | "DISTANCE";
  first: AssemblyGeometryRef; second?: AssemblyGeometryRef; value?: number; directionRelation?: string; distanceRelation?: string };

export type InstancePathSegment = {
  ownerDocumentId: string;
  ownerVersionId: string;
  instanceId: string;
  instanceName: string;
  referencedDocumentId: string;
  resolvedVersionId: string;
};

export type InstancePath = {
  rootDocumentId: string;
  segments: InstancePathSegment[];
  canonical: string;
  display: string;
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
  id: string; type: string;
  state: "QUEUED" | "RUNNING" | "RETRY_WAIT" | "SUCCEEDED" | "FAILED" | "CANCELED";
  documentId?: string; versionId?: string; progress: number; errorCode?: string; errorMessage?: string;
  resultObjectId?: string; payload: Record<string, unknown>; attemptCount: number; maxAttempts: number;
  createdAt: string; startedAt?: string; completedAt?: string; cancelRequestedAt?: string;
  canCancel: boolean; canRetry: boolean; userVisible: boolean;
};

export type ResolvedInstance = {
  id: string;
  name: string;
  documentId: string;
  geometryKey: string;
  translation: Vec3;
  rotation?: [number, number, number, number];
  occurrencePath: string;
  instancePath: InstancePath;
  bodyTreeNodeId: string;
};

export type DocumentStructureNode = {
  id: string;
  kind: "PART" | "PRODUCT" | "INSTANCE" | "ORIGIN" | "PLANE" | "AXIS_SYSTEM" | "AXIS" | "DATUM_AXIS" | "BODY" | "SKETCH" | "PAD" | "REVOLVE" | "IMPORT" | "FEATURE" | "SKETCH_GEOMETRY_SET" | "SKETCH_CONSTRAINT_SET" | "SKETCH_LOGICAL_CONSTRAINT_SET" | "SKETCH_DIMENSION_SET" | "SKETCH_ENTITY" | "SKETCH_CONSTRAINT" | "ASSEMBLY_CONSTRAINT_SET" | "ASSEMBLY_CONSTRAINT" | "REFERENCE_CYCLE";
  name: string;
  entityId?: string;
  documentId?: string;
  documentType?: "PART" | "PRODUCT";
  versionId?: string;
  plane?: PlaneName | "CUSTOM";
  axis?: "X" | "Y" | "Z";
  referenceMode?: "FOLLOW_HEAD" | "PINNED";
  instancePath?: InstancePath;
  ownerEntityId?: string;
  entityType?: string;
  role?: "PROFILE" | "CONSTRUCTION";
  suppressed?: boolean;
  diagnostic?: string;
  capabilities?: Array<"DELETE" | "SUPPRESS">;
  children?: DocumentStructureNode[];
};

export type DocumentView = {
  document: DocumentSummary;
  datumPlanes?: DatumPlane[];
  axisSystems?: AxisSystem[];
  datumAxes?: DatumAxis[];
  part?: { units: string; datumPlanes: DatumPlane[]; axisSystems: AxisSystem[]; datumAxes?: DatumAxis[]; features: Feature[] };
  product?: { instances: ProductInstance[]; constraints?: AssemblyConstraint[] };
  artifact?: Artifact;
  artifacts?: Record<string, Artifact>;
  resolvedInstances?: ResolvedInstance[];
  structureTree?: DocumentStructureNode;
};

export type SelectionIdentity = {
  id: string;
  treeNodeId?: string;
  expandTreeDescendants?: boolean;
  documentId?: string;
  occurrencePath?: string;
  instancePath?: InstancePath;
  geometryKey?: string;
  instanceId?: string;
  entityId?: string;
  visualKey?: string;
};

export type SelectionItem =
  | (SelectionIdentity & { kind: "plane"; plane: PlaneName | "CUSTOM"; datumPlane?: DatumPlane })
  | (SelectionIdentity & { kind: "axis-system" })
  | (SelectionIdentity & { kind: "axis"; axis: "X" | "Y" | "Z" | "DATUM" })
  | (SelectionIdentity & { kind: "sketch" })
  | (SelectionIdentity & { kind: "pad" })
  | (SelectionIdentity & { kind: "import" })
  | (SelectionIdentity & { kind: "instance" })
  | (SelectionIdentity & { kind: "body" })
  | (SelectionIdentity & { kind: "solid" })
  | (SelectionIdentity & { kind: "visual"; visualType: "POINT" | "CURVE" | "SURFACE";
      featureId: string; entityId: string; role?: "PROFILE" | "CONSTRUCTION" })
  | (SelectionIdentity & { kind: "sketch-constraint"; featureId: string; constraintId: string; constraintType: string })
  | (SelectionIdentity & { kind: "assembly-constraint"; constraintId: string; constraintType: string })
  | (SelectionIdentity & { kind: "face" | "edge" | "vertex"; topologyId: number })
  | (SelectionIdentity & { kind: "tree" });
export type Selection = SelectionItem | null;

export type TopologyElementProperties = {
  geometryKey: string; geometryId: string; kind: "FACE" | "EDGE" | "VERTEX"; localId: number;
  geometryType: string; bbox?: { min: Vec3; max: Vec3 }; point?: Vec3;
  properties: Record<string, number | boolean | string | Vec3>; workerId: string; occtVersion: string;
};
