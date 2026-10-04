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
  normals?: Vec3[];
  vertices: Vec3[];
  triangles: [number, number, number][];
  faceIds: number[];
  edges: Array<{ localId: number; points: Vec3[] }> | null;
  topologyVertices: Array<{ localId: number; point: Vec3 }> | null;
};

export type NamingAvailability = { status: "READY" | "UNAVAILABLE" | "FAILED" | "CORRUPT" | "INCOMPATIBLE"; canBind: boolean; diagnosticCode?: string; diagnostic?: string };

export type BodyDisplayFallback = { geometryKey: string; sourceVersionId: string };
export type PartBody = { displayFallback?: BodyDisplayFallback; consumed?: boolean; id: string; name: string; visible: boolean; geometryKey?: string; createdByFeatureId?: string };

export type Artifact = {
  visualNamingDigest?: string;
  bodyId?: string;
  naming?: NamingAvailability;
  geometryKey: string;
  geometryId: string;
  representationKind: "PERSISTENT" | "TRANSIENT_PREVIEW";
  representations: Record<string,{objectId:string;digest:string;schemaVersion:number;size:number;contentType:string;url?:string}>;
  triangleCount:number;
  displayVertexCount:number;
  bbox: { min: Vec3; max: Vec3 };
  topology: { faces: number; edges: number; vertices: number; solids: number };
  volume: number;
  occtVersion: string;
  glbBytes: number;
	brepBytes: number;
	evaluatorVersion: string;
	workerId: string;
	storageState: "OBJECT";
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
  sketchCandidates?: Array<{featureId:string;entities:SketchEntity[];solve:SketchFeature["solve"]}>;
  evaluationOutcome?: "DEFINITION_ONLY";
  evaluationFailure?: AssemblyEvaluationFailure;
  assemblySolverBuild?: string;
  assemblyComponents?: AssemblyComponentDof[];
  previewId: string;
  baseVersionId: string;
  baseSequence: number;
  modelHash: string;
  artifact?: Artifact;
  resultBodyId?: string;
  resultBodyName?: string;
  bodyAssignment?: "TARGET_BODY" | "EXPLICIT_NEW_BODY";
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
 patternId?:string;patternMemberSlot?:number;sketchMemberId?:string;pointReference?:SketchGeometryRef;
  id: string; displayEntityId?: string; featureId: string; kind: "POINTS" | "POLYLINE" | "LINE_SEGMENTS" | "TRIANGLES";
  semantic: "SKETCH_POINT" | "SKETCH_CURVE" | "SKETCH_EXTERNAL" | "SKETCH_CONSTRAINT" | "CURVE" | "SURFACE";
  entityType?: string;
  role?: "PROFILE" | "CONSTRUCTION"; status?: string; positions: Vec3[];
  label?: string; labelPosition?: Vec3;
  relatedEntityIds?: string[];
  indices?: number[]; selectable: boolean;
};
export type VisualizationManifest = {
  schemaVersion: 1 | 2; referenceGeometry: ReferenceGeometry;
  // Lightweight Artifact descriptors omit display primitives; GLB owns the full payload.
  primitives?: VisualPrimitive[] | null;
};

export type DocumentProperties = {
  documentId: string; versionId: string; documentType: "PART" | "PRODUCT"; units: string;
  artifacts: Artifact[];
  aggregate: { artifactCount: number; triangleCount: number; vertexCount: number; solidCount: number;
    glbBytes: number; brepBytes: number; resolvedInstanceCount: number };
  worker: { available: boolean; workerId?: string; occtVersion?: string; residentGeometryCount?: number; error?: string };
};

export type PatternDefinition = { directionReference?:SketchGeometryRef; reversed?:boolean; centerReference?:{sketchId?:string;axisEntityId?:string;reference:SketchGeometryRef}; axisEntityId?:string; id:string; kind:"LINEAR"|"CIRCULAR"; distribution:"FIXED_STEP"|"TOTAL_SPAN"|"FULL_CIRCLE"; count:number; spacing?:number; angle?:number; phase?:number; origin:Vec3; direction:Vec3; skippedSlots?:number[]; suppressed?:boolean };
export type SketchPattern = PatternDefinition & {entityIds:string[]};
export type Feature = {
  pattern?:PatternDefinition & {resultMode?:"COMBINE"|"INDEPENDENT";source:{bodyId:string;featureId:string};sourceKind:"SKETCH_FRAME"|"GENERATOR_TOOL"|"BODY_STAGE"};
  profileMemberSlot?:number;
  evaluationStatus?: "FAILED" | "BLOCKED" | "SUPPRESSED";
  diagnostic?: string;
  sections?:{sketchId:string;memberSlot?:number;reversed?:boolean;seamEntityId?:string;seamAngle?:number}[];
  ruled?:boolean;
  selections?:{selection:PersistentSelection;sourceVersionId:string;sourceFeatureId?:string}[];
  neutralPlaneId?:string;
  neutralPlane?: NonNullable<Feature["selections"]>[number];
  extent?: "FINITE"|"TWO_SIDED"|"SYMMETRIC"|"THROUGH_ALL";
  length2?:number;
  tools?: {bodyId:string; featureId:string}[];
  keepTools?: boolean;
  suppressed?: boolean;
  bodyId?: string;
  visible?: boolean;
  importDefinitionId?: string;
  id: string;
  type: "SKETCH_PATTERN" | "SOLID_PATTERN" | "SKETCH" | "sketch" | "PAD" | "pad" | "LINEAR_EXTRUDE" | "REVOLVE" | "IMPORT_BODY" | "BOOLEAN" | "FILLET" | "CHAMFER" | "DRAFT" | "SHELL" | "LOFT";
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
export type SketchGeometryRef = { target: "ENTITY" | "EXTERNAL" | "SKETCH_ORIGIN" | "SKETCH_X_AXIS" | "SKETCH_Y_AXIS"; entityId?: string; subElement: "WHOLE" | "POINT" | "START" | "END" | "CENTER" | "DIRECTION" | "CONTROL"; controlPointIndex?: number; controlPointId?: string; pointId?: string };
export type SketchEntity = { id: string; createdByOperationId?: string; sourceEntityId?: string; startPointId?:string; endPointId?:string; kind: "POINT" | "LINE" | "CIRCLE" | "ARC" | "ELLIPSE" | "ELLIPTICAL_ARC" | "SPLINE"; role: "PROFILE" | "CONSTRUCTION"; visible?: boolean; suppressed?: boolean; point?: SketchPoint2; start?: SketchPoint2; end?: SketchPoint2; center?: SketchPoint2; radius?: number; majorRadius?: number; minorRadius?: number; rotation?: number; startAngle?: number; endAngle?: number; controlPoints?: SketchPoint2[]; controlPointIds?: string[]; degree?: number; closed?: boolean; mode?: "FIT" | "CONTROL"; poles?: SketchPoint2[]; poleIds?: string[]; knots?: number[]; multiplicities?: number[]; weights?: number[]; periodic?: boolean; parameterStart?: number; parameterEnd?: number };
export type SketchExternalGeometry = { id:string;projectionKind:"ORTHOGONAL";geometryKind?:"POINT"|"LINE"|"CIRCLE";persistentSelection:PersistentSelection;sourceVersionId:string;
  sourceDocumentId?:string;contextReferenceId?:string;
  status:"PENDING"|"CONNECTED"|"UNRESOLVED_EXTERNAL";diagnosticCode?:string;diagnostic?:string;resolvedSourceDigest?:string;
  snapshot?:{kind:"POINT"|"LINE"|"CIRCLE";point?:SketchPoint2;start?:SketchPoint2;end?:SketchPoint2;center?:SketchPoint2;radius?:number};
  dependencySnapshot?:{geometryKey:string;manifestDigest:string;policyDigest:string;evidenceDigest?:string};
  affectedConstraintIds?:string[];affectedProfileRegionIds?:string[];downstreamFeatureIds?:string[] };
export type SketchConstraint = { id: string; kind: "COINCIDENT" | "PARALLEL" | "COLLINEAR" | "FIXED" | "FIXED_POINT" | "HORIZONTAL" | "VERTICAL" | "PERPENDICULAR" | "TANGENT" | "EQUAL" | "DISTANCE" | "HORIZONTAL_DISTANCE" | "VERTICAL_DISTANCE" | "LENGTH" | "RADIUS" | "DIAMETER" | "MAJOR_RADIUS" | "MINOR_RADIUS" | "ANGLE" | "CONCENTRIC" | "POINT_ON_OBJECT" | "MIDPOINT" | "SYMMETRY" | "MIRROR" | "SAME_SUPPORT"; references: SketchGeometryRef[]; reference?: boolean; suppressed?: boolean; fixedPoint?: SketchPoint2; value?: number; unit?: "mm" | "deg"; parameterId?: string; labelPosition?: SketchPoint2; internal?: boolean; selfMirrorMode?: "ON_AXIS" | "PAIRED" | "MAJOR_PARALLEL" | "MAJOR_PERPENDICULAR" };
export type SketchSupport = { type: "DATUM_PLANE" | "PLANAR_FACE"; datumPlaneId?: string; plane: PlaneName | "CUSTOM";
  persistentSelection?: PersistentSelection; sourceVersionId?: string; origin?: Vec3; xDirection?: Vec3; normal?: Vec3;
  orientationRule?: string; status?: "CONNECTED" | "FAILED_SUPPORT"; diagnosticCode?: string; diagnostic?: string;
  dependencySnapshot?: { geometryKey: string; manifestDigest: string; policyDigest: string; evidenceDigest?: string } };
export type SketchFeature = { patterns?:SketchPattern[]; schemaVersion: 2; support: SketchSupport; entities: SketchEntity[]; externalGeometry?:SketchExternalGeometry[]; constraints: SketchConstraint[]; solve: { status: string; definitionStatus?: "FULLY_CONSTRAINED"|"UNDER_CONSTRAINED"|"UNRESOLVED"; degreesOfFreedom: number; diagnostic?: string; conflictingConstraintIds?: string[]; redundantConstraintIds?: string[]; components?: Array<{entityIds:string[];constraintIds:string[];status:string;definitionStatus?:"FULLY_CONSTRAINED"|"UNDER_CONSTRAINED"|"UNRESOLVED";degreesOfFreedom:number}> } };
export type SketchOperation = {type:"CREATE_PATTERN"|"EDIT_PATTERN";pattern:SketchPattern;patternId?:string;patternParameterExpressions?:Record<string,string>} | {type:"DELETE_PATTERN"|"DETACH_PATTERN";patternId:string} | { type: "ADD_ENTITY"; entity: SketchEntity } | { type: "ADD_CONSTRAINT"; constraint: SketchConstraint; parameterSource?: string; parameterKey?: string }
  | { type: "UPDATE_CONSTRAINT"; constraintId: string; constraint: SketchConstraint; parameterSource?: string; parameterKey?: string; restoreMode?: "ORIGINAL" | "MEASUREMENT" }
  | {type:"ADD_EXTERNAL_GEOMETRY";externalId:string;geometryKey:string;topologyId:number;topologyKind:"EDGE"|"VERTEX";sourceVersionId:string}
  | {type:"RECONNECT_EXTERNAL_GEOMETRY";externalId:string;geometryKey:string;topologyId:number;topologyKind:"EDGE"|"VERTEX";sourceVersionId:string}
  | {type:"DETACH_EXTERNAL_GEOMETRY";externalId:string}
  | { type: "UPDATE_CONSTRAINT_VALUE"; constraintId: string; value: number }
  | {type:"CREATE_POLYGON";operationId:string;point:SketchPoint2;value:number;angle:number;sides:number;mode:"INSCRIBED"|"CIRCUMSCRIBED";role:"PROFILE"|"CONSTRUCTION";firstReference?:SketchGeometryRef}
  | { type: "ADD_RECTANGLE"; first: SketchPoint2; second: SketchPoint2; firstReference?: SketchGeometryRef; secondReference?: SketchGeometryRef }
  | { type: "UPDATE_ENTITY_ROLE"; entityId: string; role: "PROFILE" | "CONSTRUCTION" }
  | { type: "UPDATE_ENTITY_POINT"; entityId: string; subElement: "POINT" | "CENTER" | "CONTROL" | "START" | "END"; controlPointIndex?: number; controlPointId?: string; point: SketchPoint2 }
  | { type: "UPDATE_ENTITY_SUPPRESSION"; entityId: string; suppressed: boolean }
  | { type: "UPDATE_CONSTRAINT_SUPPRESSION"; constraintId: string; suppressed: boolean }
  | { type: "DELETE_CONSTRAINT"; constraintId: string }
  | { type: "DELETE_ENTITIES"; entityIds: string[] }
  | { type: "COPY_ENTITIES"; operationId: string; entityIds: string[]; origin?: SketchPoint2; translation?: SketchPoint2; angle?: number; constraintPolicy?: "INTERNAL" | "GEOMETRY_ONLY" }
  | { type: "DRAG_ENTITIES"; operationId: string; entityIds: string[]; origin?: SketchPoint2; translation?: SketchPoint2; angle?: number }
  | { type: "TRANSFORM_ENTITIES"; operationId: string; entityIds: string[]; origin?: SketchPoint2; translation?: SketchPoint2; angle?: number; copy?: boolean; constraintPolicy?: "INTERNAL" | "GEOMETRY_ONLY"; detachConstraintIds?: string[] }
  | { type: "MIRROR_ENTITIES"; operationId: string; entityIds: string[]; axis: SketchGeometryRef; mirrorMode?: "INDEPENDENT" | "LINKED"; constraintPolicy?: "INTERNAL" | "GEOMETRY_ONLY" }
  | {type:"EDIT_SPLINE_POINT";operationId:string;entityId:string;pointAction:"INSERT"|"DELETE";pointIndex?:number;controlPointId?:string;point?:SketchPoint2;knotParameter?:number;detachConstraintIds?:string[]}
  | {type:"SET_SPLINE_CLOSED";operationId:string;entityId:string;closed:boolean;detachConstraintIds?:string[]}
  | {type:"CONVERT_SPLINE_TO_CONTROL";operationId:string;entityId:string;detachConstraintIds?:string[]}
  | { type: "SPLIT_ENTITY"; operationId: string; entityIds: string[]; parameters: number[]; firstReference?: SketchGeometryRef; secondReference?:SketchGeometryRef; detachConstraintIds?: string[] }
  | { type:"TRIM_ENTITY";operationId:string;entityIds:string[];parameters:[number,number];detachConstraintIds?:string[] }
  | { type:"QUICK_TRIM";operationId:string;entityIds:string[];boundaryIds:string[];hitParameter:number;trimMode:"DELETE_HIT"|"KEEP_HIT"|"BREAK";detachConstraintIds?:string[] }
  | {type:"FILLET_ENTITIES";parameterSource?:string;operationId:string;entityIds:[string,string];firstReference:SketchGeometryRef;secondReference:SketchGeometryRef;point:SketchPoint2;value:number;trimMode:"TRIM"|"KEEP"}
  | {type:"CHAMFER_ENTITIES";parameterSource?:string;operationId:string;entityIds:[string,string];firstReference:SketchGeometryRef;secondReference:SketchGeometryRef;point:SketchPoint2;chamferMode:"EQUAL"|"TWO_LENGTHS"|"LENGTH_ANGLE";chamferFirst:number;chamferSecond?:number;chamferAngle?:number;trimMode:"TRIM"|"KEEP"}
  | {type:"EXTEND_ENTITY";operationId:string;entityIds:string[];firstReference:SketchGeometryRef;boundaryIds:[]|[string];point:SketchPoint2;detachConstraintIds?:string[]}
  | {type:"ARC_COMPLEMENT"|"CLOSE_CURVE";operationId:string;entityIds:string[];detachConstraintIds?:string[]}
  | {type:"OFFSET_ENTITIES";operationId:string;entityIds:string[];value:number;mode:"MITER"|"ROUND"};

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

export type AssemblyGeometryRef = { derivedRole?:string; instancePath?: InstancePath; instanceId: string; kind: "BODY" | "POINT" | "AXIS" | "PLANE" | "CYLINDER" | "CIRCLE" | "SPHERE" | "CONE" | "FRAME" | "FACE" | "EDGE" | "VERTEX";
  geometryId?: string; axis?: string; geometryKey?: string; topologyId?: number; sourceVersionId?: string;
  persistentSelection?: PersistentSelection; resolution?: { sourceVersionId: string; targetVersionId: string;
    manifestDigest: string; policyDigest: string; result: SelectionResolution };
  publicationRef?: { publicationId:string; expectedType:string; compatibilityVersion:string; persistentSelection?:PersistentSelection };
  publicationResolution?: Publication["resolution"] };
export type AssemblyEvaluationFailure = {code:string;phase:string;retryable:boolean};
export type AssemblyConstraint = { evaluationFailure?:AssemblyEvaluationFailure; id: string; definitionVersion?:number;family?:"Coincidence"|"Contact"|"Offset"|"Angle"|"Fix"|"FixTogether";subtype?:string;name?:string; contactKind?:"FACE"|"LINE"|"POINT"|"RING";contactSide?:"EXTERNAL"|"INTERNAL";contactBranch?:number;groupMembers?:Array<{instanceId?:string;instancePath?:InstancePath;groupId?:string}>; fixMode?: "SPACE" | "RELATIVE"; fixedPose?: {translation:Vec3;rotation:[number,number,number,number]}; angleRelation?: "FREE" | "DIRECTED" | "PARALLEL" | "PERPENDICULAR"; measuredValue?: number; suppressed?: boolean; mode?: "DRIVING" | "MEASURED" | "CONTROLLED"; kind: "FIX" | "RIGID" | "COINCIDENT" | "CONCENTRIC" | "ANGLE" | "DISTANCE" | "CONTACT" | "FIX_TOGETHER";
  first: AssemblyGeometryRef; second?: AssemblyGeometryRef; value?: number; directionRelation?: string; distanceRelation?: string;
  quantityParameter?: {parameterId:string;key:string;source:{literal?:{siValue:number};expression?:{sourceText:string}}};
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
  deletedAt?: string;
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
  displayFallback?: BodyDisplayFallback;
  bodyId: string; bodyVisible: boolean;
  ownedSketchIds?: string[];
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

export type EntityRef = { documentId: string; entityKind: string; entityId: string };
export type OccurrenceRef = { rootDocumentId: string; instancePath: InstancePath };
export type SnapshotScope = { revisionId: string; contextVariantKey?: string; geometryKey?: string };

export type DocumentStructureNode = {
 patternId?:string;patternMemberSlot?:number;
  id: string;
  subject?: EntityRef;
  occurrence?: OccurrenceRef;
  snapshot?: SnapshotScope;
  kind: "SKETCH_PATTERN_MEMBER" | "SKETCH_PATTERN_DEFINITION" | "SKETCH_PATTERN_ENTITY" | "PART" | "PRODUCT" | "INSTANCE" | "ORIGIN" | "PLANE" | "AXIS_SYSTEM" | "AXIS" | "DATUM_AXIS" | "BODY" | "SKETCH" | "SKETCH_INPUT_REFERENCE" | "PAD" | "REVOLVE" | "IMPORT" | "FEATURE" | "PARAMETER_SET" | "PARAMETER" | "PUBLICATION_SET" | "PUBLICATION" | "PRODUCT_PUBLICATION_SET" | "PRODUCT_PUBLICATION" | "CONTEXT_REFERENCE_SET" | "CONTEXT_REFERENCE" | "CONTEXT_INPUT_SET" | "CONTEXT_INPUT" | "CONTEXT_BINDING_SET" | "CONTEXT_BINDING" | "SKETCH_GEOMETRY_SET" | "SKETCH_EXTERNAL_GEOMETRY_SET" | "SKETCH_EXTERNAL_GEOMETRY" | "SKETCH_CONSTRAINT_SET" | "SKETCH_LOGICAL_CONSTRAINT_SET" | "SKETCH_DIMENSION_SET" | "SKETCH_ENTITY" | "SKETCH_CONSTRAINT" | "ASSEMBLY_CONSTRAINT_SET" | "ASSEMBLY_CONSTRAINT" | "REFERENCE_CYCLE";
  presentationRole?: "DEFINITION" | "FEATURE_INPUT" | "INPUT_REFERENCE" | "GROUP";
  ownerDocumentId?: string;
  bodyId?: string;
  localVisible?: boolean;
  visibilityMode?: "SHOW" | "HIDE";
  consumed?: boolean;
  childrenState?: "COMPLETE" | "EMPTY" | "UNLOADED" | "LOADING" | "FAILED";
  connectionStatus?: string;
  currencyStatus?: string;
  evaluationStatus?: string;
  resolutionStatus?: string;
  contextVariantKey?: string;
  sourceDocumentId?: string;
  sourceRevisionId?: string;
  sourceDisplayPath?: string;
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
  referenceMode?: "FOLLOW_HEAD" | "FOLLOW_WORKSPACE_WITH_ACCEPT" | "PINNED" | "ISOLATED";
  instancePath?: InstancePath;
  ownerEntityId?: string;
  entityType?: string;
  operation?: Feature["operation"];
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

export type SketchProfileAnalysis = {
  geometryVerified?: boolean;
  status: "EMPTY" | "OPEN" | "INVALID" | "CLOSED";
  regionCount: number;
  loopCount: number;
  regionIds?: string[];
  issues: Array<{ code: string; message: string; entityIds: string[]; references?: SketchGeometryRef[]; position?: SketchPoint2 }>;
};

export type DocumentView = {
  sketchPatternMembers?:Record<string,SketchEntity[]>;
  document: DocumentSummary;
  sketchAnalyses?: Record<string, SketchProfileAnalysis>;
  datumPlanes?: DatumPlane[];
  axisSystems?: AxisSystem[];
  datumAxes?: DatumAxis[];
  part?: { bodies: PartBody[]; activeBodyId: string; units: string; datumPlanes: DatumPlane[]; axisSystems: AxisSystem[]; datumAxes?: DatumAxis[]; features: Feature[]; parameters?: ParameterDefinition[]; publications?: Publication[]; contextInputs?:ContextInput[]; contextReferences?:ContextReference[] };
  product?: { instances: ProductInstance[]; constraints?: AssemblyConstraint[]; publications?:ProductPublication[]; contextBindings?:ContextBinding[];
    visibilityOverrides?: Array<{instancePath:InstancePath;entityKind:string;entityId:string;mode:"SHOW"|"HIDE"}> };
  artifact?: Artifact;
  artifacts?: Record<string, Artifact>;
  resolvedInstances?: ResolvedInstance[];
  constraintDisplayScopes?: Array<{documentId:string;versionId:string;instancePath:InstancePath;treeNodeId:string;constraints:AssemblyConstraint[]}>;
  structureTree?: DocumentStructureNode;
  referenceUpdates?: ReferenceUpdate[];
  followedDocumentIds?: string[];
  followedProductIds?: string[];
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
export type ParameterDefinition = { parameterId: string; key: string; label: string; ownerFeatureId?: string; propertySlot?: string;
  lifecycle?: "FEATURE_REQUIRED" | "SKETCH_DIMENSION" | "USER"; displayName?: string; displayAlias?: string; qualifiedDisplayPath?: string;
  valueType: "QUANTITY" | "REAL";
  dimension: Dimension; displayUnit: string; role: string; source: { literal?: Quantity; expression?: { sourceText: string }; external?: ExternalParameterRef };
  evaluatedValue?: Quantity };

export type Publication = { id: string; name: string; type: "POINT" | "AXIS" | "PLANE" | "FRAME" | "CURVE" | "SURFACE" | "BODY" | "PARAMETER";
  semanticPurpose?: string; compatibilityVersion: string;
  target: { kind: "DATUM" | "TOPOLOGY" | "BODY_RESULT" | "FEATURE_OUTPUT" | "PARAMETER"; bodyId?: string; datumId?: string; axis?: "X" | "Y" | "Z";
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
  bindingIds:string[];bindingDigest:string;evaluationManifestDigest?:string;bodies:PartBody[];status:"READY"|"FAILED";
  publications?:Publication[];diagnosticCode?:string;diagnostic?:string};
export type ProductUpdatePlanEntry = {kind:"CONTEXT_BINDING"|"OCCURRENCE_REFERENCE"|"ASSEMBLY_SOLVE";bindingId:string;name:string;sourceDisplayPath:string;owningDisplayPath:string;
  acceptedRevisionId:string;candidateRevisionId?:string;connection:"CONNECTED"|"BROKEN"|"INCOMPATIBLE";
  currency:"CURRENT"|"UPDATE_AVAILABLE"|"UPDATE_BLOCKED";evaluation:"READY"|"FAILED"|"BLOCKED_BY_UPSTREAM";
  diagnosticCode?:string;diagnostic?:string};
export type ProductUpdatePlan = {rootProductDocumentId:string;rootProductRevisionId:string;digest:string;canAccept:boolean;
  parameterUpdates?:Array<{documentId:string;revisionId:string;needsUpdate:boolean;dependencies?:string[]}>;
  hasUpdates:boolean;entries:ProductUpdatePlanEntry[];contextVariants:ContextVariantSnapshot[];affectedConstraintIds?:string[]};
export type ProductReleaseGate = {code:string;status:"PASSED"|"FAILED";diagnostic?:string};
export type ProductRelease = {id:string;name:string;createdAt:string;manifest:{schemaVersion:number;digest:string;
  rootProductDocumentId:string;rootProductRevisionId:string;rootSnapshotDigest:string;assemblySolveManifestDigest?:string;
  gates:ProductReleaseGate[]}};
export type AssemblySolveManifestResult = {manifestDigest:string;requestId:string;resultDigest?:string;status:string;
  diagnostic?:string;result:unknown};
export type ProductReleaseReplay = {releaseId:string;manifestDigest:string;status:string;assembly?:AssemblySolveManifestResult};

export type SelectionIdentity = {
 patternId?:string;patternMemberSlot?:number;associatedSourceEntityId?:string;sketchReference?:SketchGeometryRef;
  entityRef?: EntityRef;
  occurrenceRef?: OccurrenceRef;
  snapshotScope?: SnapshotScope;
  bodyId?: string;
  id: string;
  entityKind?: string;
  ownerDocumentId?: string;
  rootDocumentId?: string;
  contextVariantKey?: string;
  highlightTarget?: SelectionItem;
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
  | (SelectionIdentity & { kind: "part" | "product" | "feature" | "publication" | "external-reference" | "context-input" | "context-binding" | "parameter" })
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
  namingDiagnostic?: NamingAvailability;
  namingStatus: "FAILED" | "CORRUPT" | "INCOMPATIBLE" | "RESOLVED" | "MISSING" | "AMBIGUOUS" | "TYPE_MISMATCH" | "OUTSIDE_CURRENT_TIP" | "UNAVAILABLE" | "";
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
