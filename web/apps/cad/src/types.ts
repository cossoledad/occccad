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

export type AssemblyBodyFreedom = {
  bodyId: string; relativeToBodyId?: string;
  linearizationPose: { translation: Vec3; rotation: [number,number,number,number] };
  kind: 0|1|2|3|4|5|6|7;
  translationDof: number; rotationDof: number;
  allowedBasis: number[][]; blockedBasis: number[][];
  translationDirections: Vec3[];
  rotations: Array<{direction:Vec3;axisPoint:Vec3;pitch:number}>;
  rankThreshold: number;
};
export type AssemblyComponentDof = {
  componentId: string; bodyIds: string[]; relativeDof: number; gaugeDof: number; solved: boolean;
  preference: { status: 0|1|2|3; geometricallyFeasible: boolean;
    referenceObjective: number; totalObjective: number; referenceOptimality: number; totalOptimality: number;
    lengthScale: number; angleScale: number; iterations: number;
    bodies: Array<{bodyId:string;role:0|1|2;translation:number;rotation:number}> };
  freedoms: AssemblyBodyFreedom[];
};
export type CommandPreview = {
  assemblySolverBuild?: string;
  assemblyComponents?: AssemblyComponentDof[];
  previewId: string;
  baseVersionId: string;
  baseSequence: number;
  modelHash: string;
  artifact?: Artifact;
  constraintLimited?: boolean;
  instancePoses?: Array<{instanceId:string;translation:Vec3;rotation:[number,number,number,number]}>;
  constraintEvaluation?: { constraintId: string; status: AssemblyConstraint["evaluationStatus"]; summary?: string;
    first: { status: "CONNECTED" | "NOT_CONNECTED"; diagnosticCode?: string; diagnostic?: string };
    second?: { status: "CONNECTED" | "NOT_CONNECTED"; diagnosticCode?: string; diagnostic?: string } };
};

export type DatumPlane = { id: string; name: string; plane: PlaneName | "CUSTOM"; origin: Vec3; normal: Vec3; uDirection: Vec3; size: number };
export type DatumAxis = { id: string; name: string; origin: Vec3; direction: Vec3 };
export type AxisSystem = { id: string; name: string; origin: Vec3; xDirection: Vec3; yDirection: Vec3; zDirection: Vec3 };
export type ReferenceGeometry = { datumPlanes: DatumPlane[]; axisSystems: AxisSystem[]; datumAxes?: DatumAxis[] };
export type VisualPrimitive = {
  id: string; featureId: string; kind: "POINTS" | "POLYLINE" | "LINE_SEGMENTS" | "TRIANGLES";
  semantic: "SKETCH_POINT" | "SKETCH_CURVE" | "SKETCH_EXTERNAL" | "SKETCH_CONSTRAINT" | "CURVE" | "SURFACE";
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
export type SketchGeometryRef = { target: "ENTITY" | "EXTERNAL" | "SKETCH_ORIGIN" | "SKETCH_X_AXIS" | "SKETCH_Y_AXIS"; entityId?: string; subElement: "WHOLE" | "POINT" | "START" | "END" | "CENTER" | "DIRECTION" | "CONTROL"; controlPointIndex?: number };
export type SketchEntity = { id: string; kind: "POINT" | "LINE" | "CIRCLE" | "ARC" | "SPLINE"; role: "PROFILE" | "CONSTRUCTION"; suppressed?: boolean; point?: SketchPoint2; start?: SketchPoint2; end?: SketchPoint2; center?: SketchPoint2; radius?: number; startAngle?: number; endAngle?: number; controlPoints?: SketchPoint2[]; degree?: number; closed?: boolean };
export type SketchExternalGeometry = { id:string;projectionKind:"ORTHOGONAL";geometryKind?:"POINT"|"LINE"|"CIRCLE";persistentSelection:PersistentSelection;sourceVersionId:string;
  sourceDocumentId?:string;contextReferenceId?:string;
  status:"PENDING"|"CONNECTED"|"UNRESOLVED_EXTERNAL";diagnosticCode?:string;diagnostic?:string;resolvedSourceDigest?:string;
  snapshot?:{kind:"POINT"|"LINE"|"CIRCLE";point?:SketchPoint2;start?:SketchPoint2;end?:SketchPoint2;center?:SketchPoint2;radius?:number};
  dependencySnapshot?:{geometryKey:string;manifestDigest:string;policyDigest:string;evidenceDigest?:string};
  affectedConstraintIds?:string[];affectedProfileRegionIds?:string[];downstreamFeatureIds?:string[] };
export type SketchConstraint = { id: string; kind: "COINCIDENT" | "PARALLEL" | "FIXED" | "FIXED_POINT" | "HORIZONTAL" | "VERTICAL" | "PERPENDICULAR" | "TANGENT" | "EQUAL" | "DISTANCE" | "LENGTH" | "RADIUS" | "DIAMETER" | "ANGLE" | "CONCENTRIC" | "POINT_ON_OBJECT" | "MIDPOINT" | "SYMMETRY"; references: SketchGeometryRef[]; suppressed?: boolean; fixedPoint?: SketchPoint2; value?: number; unit?: "mm" | "deg"; parameterId?: string; labelPosition?: SketchPoint2; internal?: boolean };
export type SketchSupport = { type: "DATUM_PLANE" | "PLANAR_FACE"; datumPlaneId?: string; plane: PlaneName | "CUSTOM";
  persistentSelection?: PersistentSelection; sourceVersionId?: string; origin?: Vec3; xDirection?: Vec3; normal?: Vec3;
  orientationRule?: string; status?: "CONNECTED" | "FAILED_SUPPORT"; diagnosticCode?: string; diagnostic?: string;
  dependencySnapshot?: { geometryKey: string; manifestDigest: string; policyDigest: string; evidenceDigest?: string } };
export type SketchFeature = { schemaVersion: 2; support: SketchSupport; entities: SketchEntity[]; externalGeometry?:SketchExternalGeometry[]; constraints: SketchConstraint[]; solve: { status: string; definitionStatus?: "FULLY_CONSTRAINED"|"UNDER_CONSTRAINED"|"UNRESOLVED"; degreesOfFreedom: number; diagnostic?: string; conflictingConstraintIds?: string[]; redundantConstraintIds?: string[]; components?: Array<{entityIds:string[];constraintIds:string[];status:string;definitionStatus?:"FULLY_CONSTRAINED"|"UNDER_CONSTRAINED"|"UNRESOLVED";degreesOfFreedom:number}> } };
export type SketchOperation = { type: "ADD_ENTITY"; entity: SketchEntity } | { type: "ADD_CONSTRAINT"; constraint: SketchConstraint }
  | {type:"ADD_EXTERNAL_GEOMETRY";externalId:string;geometryKey:string;topologyId:number;topologyKind:"EDGE"|"VERTEX";sourceVersionId:string}
  | {type:"RECONNECT_EXTERNAL_GEOMETRY";externalId:string;geometryKey:string;topologyId:number;topologyKind:"EDGE"|"VERTEX";sourceVersionId:string}
  | {type:"DETACH_EXTERNAL_GEOMETRY";externalId:string}
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
  referenceMode?: "FOLLOW_HEAD" | "FOLLOW_WORKSPACE_WITH_ACCEPT" | "PINNED";
  resolvedVersionId?: string;
  headChanged?: boolean;
};

export type AssemblyGeometryRef = { instanceId: string; kind: "BODY" | "POINT" | "AXIS" | "PLANE" | "CYLINDER" | "FACE" | "EDGE" | "VERTEX";
  geometryId?: string; axis?: string; geometryKey?: string; topologyId?: number; sourceVersionId?: string;
  persistentSelection?: PersistentSelection; resolution?: { sourceVersionId: string; targetVersionId: string;
    manifestDigest: string; policyDigest: string; result: SelectionResolution };
  publicationRef?: { publicationId:string; expectedType:string; compatibilityVersion:string; persistentSelection?:PersistentSelection };
  publicationResolution?: Publication["resolution"] };
export type AssemblyConstraint = { id: string; fixMode?: "SPACE" | "RELATIVE"; fixedPose?: {translation:Vec3;rotation:[number,number,number,number]}; angleRelation?: "FREE" | "DIRECTED" | "PARALLEL" | "PERPENDICULAR"; measuredValue?: number; suppressed?: boolean; mode?: "DRIVING" | "MEASURED" | "CONTROLLED"; kind: "FIX" | "RIGID" | "COINCIDENT" | "CONCENTRIC" | "ANGLE" | "DISTANCE";
  first: AssemblyGeometryRef; second?: AssemblyGeometryRef; value?: number; directionRelation?: string; distanceRelation?: string;
  angleAxis?: AssemblyGeometryRef; reverseAngleAxis?: boolean; angleReferenceDirection?: Vec3; evaluationStatus: "NOT_UPDATED" | "BROKEN" | "IMPOSSIBLE" | "VERIFIED";
  evaluationSummary?: string };

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
  kind: "PART" | "PRODUCT" | "INSTANCE" | "ORIGIN" | "PLANE" | "AXIS_SYSTEM" | "AXIS" | "DATUM_AXIS" | "BODY" | "SKETCH" | "PAD" | "REVOLVE" | "IMPORT" | "FEATURE" | "PUBLICATION_SET" | "PUBLICATION" | "PRODUCT_PUBLICATION_SET" | "PRODUCT_PUBLICATION" | "CONTEXT_REFERENCE_SET" | "CONTEXT_REFERENCE" | "CONTEXT_INPUT_SET" | "CONTEXT_INPUT" | "CONTEXT_BINDING_SET" | "CONTEXT_BINDING" | "SKETCH_GEOMETRY_SET" | "SKETCH_EXTERNAL_GEOMETRY_SET" | "SKETCH_EXTERNAL_GEOMETRY" | "SKETCH_CONSTRAINT_SET" | "SKETCH_LOGICAL_CONSTRAINT_SET" | "SKETCH_DIMENSION_SET" | "SKETCH_ENTITY" | "SKETCH_CONSTRAINT" | "ASSEMBLY_CONSTRAINT_SET" | "ASSEMBLY_CONSTRAINT" | "REFERENCE_CYCLE";
  name: string;
  referenceName?: string;
  instanceName?: string;
  entityId?: string;
  documentId?: string;
  documentType?: "PART" | "PRODUCT";
  versionId?: string;
  plane?: PlaneName | "CUSTOM";
  axis?: "X" | "Y" | "Z";
  geometryKey?: string;
  topologyId?: number;
  referenceMode?: "FOLLOW_HEAD" | "FOLLOW_WORKSPACE_WITH_ACCEPT" | "PINNED";
  instancePath?: InstancePath;
  ownerEntityId?: string;
  entityType?: string;
  role?: "PROFILE" | "CONSTRUCTION";
  suppressed?: boolean;
  diagnostic?: string;
  definitionDigest?: string;
  publication?: Publication;
  productPublication?: ProductPublication;
  contextInput?: ContextInput;
  contextBinding?: ContextBinding;
  capabilities?: Array<"ACTIVATE" | "DEACTIVATE" | "DELETE" | "SUPPRESS" | "EDIT" | "DETACH" | "RECONNECT" | "REFRESH" | "CREATE_PART" | "UPDATE_REFERENCES" | "PIN_VERSION" | "FOLLOW_HEAD">;
  children?: DocumentStructureNode[];
};

export type DocumentView = {
  document: DocumentSummary;
  datumPlanes?: DatumPlane[];
  axisSystems?: AxisSystem[];
  datumAxes?: DatumAxis[];
  part?: { units: string; datumPlanes: DatumPlane[]; axisSystems: AxisSystem[]; datumAxes?: DatumAxis[]; features: Feature[]; parameters?: ParameterDefinition[]; publications?: Publication[]; contextInputs?:ContextInput[]; contextReferences?:ContextReference[] };
  product?: { instances: ProductInstance[]; constraints?: AssemblyConstraint[]; publications?:ProductPublication[]; contextBindings?:ContextBinding[] };
  artifact?: Artifact;
  artifacts?: Record<string, Artifact>;
  resolvedInstances?: ResolvedInstance[];
  structureTree?: DocumentStructureNode;
  referenceUpdates?: ReferenceUpdate[];
  designSession?: ProductDesignSession;
  contextVariants?: ContextVariantSnapshot[];
};

export type ReferenceUpdate = { consumerKind:string;consumerId:string;sourceDocumentId:string;acceptedRevisionId:string;
  candidateRevisionId?:string;status:"CURRENT"|"UPDATE_AVAILABLE"|"BROKEN";diagnosticCode?:string;diagnostic?:string };

export type Dimension = { Length: number; Mass: number; Time: number; Current: number; Temperature: number; Amount: number; Luminous: number; Semantic: string };
export type Quantity = { siValue: number; dimension: Dimension };
export type ExternalParameterRef = { sourceDocumentId: string; revision: { mode: "PINNED"|"FOLLOW_HEAD"|"FOLLOW_WORKSPACE_WITH_ACCEPT"; revisionId: string };
  publicationId: string; expectedType: "QUANTITY" | "REAL"; expectedDimension: Dimension; contractVersion: string;
  resolvedRevisionId: string; resolvedValue: Quantity; resolvedValueDigest: string;
  resolutionSnapshot?:{sourceRevisionId:string;publicationId:string;contractDigest:string;valueDigest:string;status:string} };
export type ParameterDefinition = { parameterId: string; key: string; label: string; valueType: "QUANTITY" | "REAL";
  dimension: Dimension; displayUnit: string; role: string; source: { literal?: Quantity; expression?: { sourceText: string }; external?: ExternalParameterRef };
  evaluatedValue?: Quantity };

export type Publication = { id: string; name: string; type: "POINT" | "AXIS" | "PLANE" | "FRAME" | "CURVE" | "SURFACE" | "BODY" | "PARAMETER";
  semanticPurpose?: string; compatibilityVersion: string;
  target: { kind: "DATUM" | "TOPOLOGY" | "FEATURE_OUTPUT" | "PARAMETER"; datumId?: string; axis?: "X" | "Y" | "Z";
    persistentSelection?: PersistentSelection; sourceVersionId?: string; featureId?: string; outputSlot?: string; parameterId?: string };
  contract: { geometryKind?: string; valueType?: "QUANTITY" | "REAL"; dimension?: Dimension; unitPolicy?: string;
    bounds?: { minimum?: Quantity; maximum?: Quantity }; symmetry?: string };
  resolution: { status: "CONNECTED" | "BROKEN_PUBLICATION" | "PENDING"; diagnosticCode?: string; diagnostic?: string;
    resolvedVersionId?: string; geometryKey?: string; geometryId?: string; topologyKind?: "FACE" | "EDGE" | "VERTEX";
    geometryKind?: string; symmetry?: string; localId?: number; radius?: number;
    origin?: Vec3; xDirection?: Vec3; yDirection?: Vec3; zDirection?: Vec3; sourceDigest?: string; value?: Quantity;
    valueDigest?: string; manifestDigest?: string; namingPolicyDigest?: string; evaluatorVersion?: string; workerId?: string; occtVersion?: string } };

export type ProductPublication = { id:string;name:string;type:Publication["type"];semanticPurpose?:string;compatibilityVersion:string;
  target:{instancePath:InstancePath;publicationId:string};contract:Publication["contract"];resolution:Publication["resolution"] };
export type ContextInput = { id:string;name:string;type:Publication["type"];required?:boolean;
  target:{kind:"PARAMETER"|"DATUM"|"SKETCH_EXTERNAL_GEOMETRY"|"FEATURE_INPUT";targetId:string};contract:Publication["contract"] };
export type ContextBinding = { id:string;name:string;owningInstancePath:InstancePath;contextInputId:string;
  sourceInstancePath:InstancePath;publication:{publicationId:string;expectedType:string;compatibilityVersion:string};
  referenceMode:"PINNED"|"FOLLOW_HEAD"|"FOLLOW_WORKSPACE_WITH_ACCEPT";transform:{translation:Vec3;rotation:[number,number,number,number]};
  resolution:Publication["resolution"];accepted:{rootProductRevisionId:string;sourceRevisionId:string;owningRevisionId:string;
    contractDigest:string;sourceDigest?:string;status:string} };
export type ContextReference = { id:string;name:string;owningWorkspace:string;sourceDocumentId:string;
  referenceMode:"PINNED"|"FOLLOW_HEAD"|"FOLLOW_WORKSPACE_WITH_ACCEPT"|"ISOLATED";rootProductDocumentId?:string;rootProductRevisionId?:string;
  rootProductSnapshotDigest?:string;
  sourceInstancePath?:InstancePath;owningInstancePath?:InstancePath;contextVariantId?:string;publication:{publicationId:string;expectedType:string;compatibilityVersion:string;selectionSourceVersionId?:string};
  transform:{translation:Vec3;rotation:[number,number,number,number]};resolvedRevisionId:string;resolution:Publication["resolution"];localTargetId?:string };
export type ProductDesignSession = { rootProductDocumentId:string;rootProductRevisionId:string;rootSnapshotDigest:string;
  activeInstancePath?:InstancePath;activeDocumentId:string;activeRevisionId:string;contextCatalogDigest:string };
export type ContextCatalogPublication = { instancePath:InstancePath;documentId:string;revisionId:string;publication:Publication;
  displayPath:string;selectable:boolean;diagnostic?:string };
export type ContextCatalog = { rootProductDocumentId:string;rootProductRevisionId:string;activeInstancePath?:InstancePath;
  expectedType?:string;digest:string;publications:ContextCatalogPublication[] };
export type ContextVariantSnapshot = {variantKey:string;owningInstancePath:InstancePath;baseDocumentId:string;baseRevisionId:string;
  bindingIds:string[];bindingDigest:string;evaluationManifestDigest?:string;geometryKey?:string;status:"READY"|"FAILED";
  publications?:Publication[];diagnosticCode?:string;diagnostic?:string};
export type ProductUpdatePlanEntry = {kind:"CONTEXT_BINDING"|"OCCURRENCE_REFERENCE"|"ASSEMBLY_SOLVE";bindingId:string;name:string;sourceDisplayPath:string;owningDisplayPath:string;
  acceptedRevisionId:string;candidateRevisionId?:string;connection:"CONNECTED"|"BROKEN"|"INCOMPATIBLE";
  currency:"CURRENT"|"UPDATE_AVAILABLE"|"UPDATE_BLOCKED";evaluation:"READY"|"FAILED"|"BLOCKED_BY_UPSTREAM";
  diagnosticCode?:string;diagnostic?:string};
export type ProductUpdatePlan = {rootProductDocumentId:string;rootProductRevisionId:string;digest:string;canAccept:boolean;
  hasUpdates:boolean;entries:ProductUpdatePlanEntry[];contextVariants:ContextVariantSnapshot[];affectedConstraintIds?:string[]};
export type ProductReleaseGate = {code:string;status:"PASSED"|"FAILED";diagnostic?:string};
export type ProductRelease = {id:string;name:string;createdAt:string;manifest:{schemaVersion:number;digest:string;
  rootProductDocumentId:string;rootProductRevisionId:string;rootSnapshotDigest:string;assemblySolveManifestDigest?:string;
  gates:ProductReleaseGate[]}};
export type AssemblySolveManifestResult = {manifestDigest:string;requestId:string;resultDigest?:string;status:string;
  diagnostic?:string;result:unknown};
export type ProductReleaseReplay = {releaseId:string;manifestDigest:string;status:string;assembly?:AssemblySolveManifestResult};

export type SelectionIdentity = {
  id: string;
  treeNodeId?: string;
  expandTreeDescendants?: boolean;
  documentId?: string;
  versionId?: string;
  occurrencePath?: string;
  instancePath?: InstancePath;
  geometryKey?: string;
  instanceId?: string;
  entityId?: string;
  visualKey?: string;
  publicationId?: string;
  publication?: Publication;
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
  namingStatus: "RESOLVED" | "MISSING" | "AMBIGUOUS" | "TYPE_MISMATCH" | "OUTSIDE_CURRENT_TIP" | "UNAVAILABLE" | "";
  persistentSelection?: PersistentSelection;
  namingResolution?: SelectionResolution;
};

export type SemanticTopologyRef = { featureId: string; outputSlot: string; sourceIds?: string[] };
export type SelectionEvidence = { geometryType?: string; measureSI?: number; measureDimension?: string;
  centroid?: Vec3; origin?: Vec3; direction?: Vec3; adjacent?: SemanticTopologyRef[]; evidenceDigest?: string };
export type PersistentSelection = { schemaVersion: number; sourceDocumentId: string; sourceBodyId: string;
  anchor: SemanticTopologyRef; expectedType: "FACE" | "EDGE" | "VERTEX";
  selector: { kind: string; operands?: SemanticTopologyRef[] }; creationEvidence: SelectionEvidence };
export type SelectionResolution = { status: string; supportingElementStatus: "CONNECTED" | "NOT_CONNECTED";
  candidates?: Array<{ geometryId: string; geometryKey: string; type: string; localId: number;
    semanticRef: SemanticTopologyRef; evidence: SelectionEvidence }>;
  diagnosticCode?: string; diagnostic?: string; evidenceDigest?: string };
