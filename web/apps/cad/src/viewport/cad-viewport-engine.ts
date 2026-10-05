import { featureContribution, topologyFeatureAssociation, sameDisplayContext } from "../cad/interaction/feature-association";
import { patternEntityStatus } from "../features/workbench/pattern-selection";
import { makeDatumAxisReference, type DatumPreview } from "../cad/rendering/datum-reference";
import { featureSelectionHit, type FeatureSelectionSession } from "../cad/interaction/feature-selection";
import { makeSplineControlFeedback } from "../cad/rendering/spline-control-feedback";
import { useUIPreferences, sketchLabelPositionKey } from "../state/ui-preferences";
import { sketchMarqueeContains, type SketchScreenPoint } from "../cad/interaction/sketch-marquee";
import { SketchModalInputController } from "../cad/sketch/sketch-modal-input";
import type { SketchReferenceSelectionRequest } from "../cad/sketch/sketch-reference-selection-session";
import type { SketchEditCandidatePreview } from "../cad/sketch/sketch-edit-preview";
import { sketchCommitResultUnknown } from "../cad/tool/sketch-command-session";
import { SelectionInputResult } from "../cad/input/input-types";
import type { SketchCommandState, SketchCommandAction, SketchCommitIntent, SketchCommitResult, SketchCommitReceipt } from "../cad/tool/sketch-command-session";
import { VisualRepository, type DisplayArtifact as Artifact, type DisplayDocumentView as DocumentView } from "../cad/visual/visual-repository";
import { assemblyConstraintReferences } from "../cad/assembly/assembly-capability";
import type {MotionPresentation} from "../cad/assembly/motion-presentation";
import {makeMotionMarkers,disposeMotionMarkers} from "../cad/assembly/motion-markers";
import {updateAnalysisGuides} from "../cad/rendering/analysis-guides";
import { AssemblyInteractionController,assemblyInteractionFailureState, type AssemblyInteractionBegin, type AssemblyInteractionSession, type AssemblyInteractionUpdate, type AssemblyInteractionFrame, type AssemblyInteractionCommit, type AssemblyInteractionState } from "../cad/assembly/assembly-interaction";
import { makeSketchReferenceAxis, sketchAxisEndpoints } from "../cad/rendering/sketch-reference-axis";
import { createStudioEnvironment } from "../cad/rendering/studio-environment";
import { raycastDatumAxis } from "../cad/interaction/datum-axis-picking";
import { applySolidDisplaySettings } from "../cad/rendering/solid-render-mode";
import { updateScreenLines } from "../cad/rendering/screen-space-lines";
import { DEFAULT_SOLID_DISPLAY, DEFAULT_REFERENCE_VISIBILITY, referenceCategory, type ReferenceVisibility, type SolidDisplaySettings } from "../cad/rendering/display-settings";
import { instancePatternOffsets, type InstancePatternPreview } from "../features/workbench/instance-pattern";
import { ViewTransition } from "../cad/navigation/view-transition";
import { normalViewFrame, type NormalViewPlane } from "../cad/navigation/normal-view";
import { makeFeatureEdges } from "../cad/rendering/feature-edges";
import { makeFeaturePreview, type FeaturePreviewOperation } from "../cad/rendering/feature-preview";
import { adaptiveGridSpacing, InfiniteGroundGrid } from "../cad/rendering/infinite-ground-grid";
import { fitOrthographicView, updateOrthographicClipping, orientPlaneView, saveView, standardView, viewFocus, type SavedView } from "../cad/navigation/orthographic-view";
import * as THREE from "three";
import {occurrenceSnapshot,refreshOccurrenceSelection} from "../cad/assembly/assembly-occurrence-snapshot";
import {exactSnapCandidates,ManipulatorSnapCache,type SupportInspection} from "../cad/interaction/manipulator-snap";
import {assemblyGeometryRef} from "../cad/assembly/assembly-reference";
import { acceleratedRaycast, computeBoundsTree, disposeBoundsTree } from "three-mesh-bvh";
import { InputManager } from "../cad/input/input-manager";
import { snapshotTransform, TransformTransitionSystem, type TransformPose } from "../cad/animation/transform-transition";
import type { InputState } from "../cad/input/input-types";
import { InteractionRouter } from "../cad/interaction/interaction-router";
import { SelectionController } from "../cad/interaction/selection-controller";
import { SelectionIndex } from "../cad/interaction/selection-index";
import { AssemblyManipulator, type ManipulatorAnchor } from "../cad/interaction/assembly-manipulator";
import { projectSketchFeatureSelection, selectionModeForTool, sketchContextLayerVisibility, type SelectionMode } from "../cad/interaction/selection-mode";
import { treeVisibilityOverride, type TreeVisibilityOverrides } from "../cad/interaction/tree-visibility";
import { visibilityResolverForView, type DisplayAddress, type VisibilityResolver } from "../cad/interaction/visibility-resolver";
import { sameSelection, sameSelections, selectionKey } from "../cad/interaction/selection-identity";
import { resolveSketchReference, type SketchReferencePickKind } from "../cad/interaction/sketch-reference-pick";
import { resolveSketchSnap, type SketchSnapResult } from "../cad/interaction/sketch-snap";
import { allowsSelection, allowsSelectionInContext, DEFAULT_CAPTURE_SETTINGS, type CaptureSettings } from "../cad/interaction/capture-settings";
import { NavigationController, type NavigationSnapshot } from "../cad/navigation/navigation-controller";
import { navigationCursor } from "../cad/navigation/navigation-cursor";
import { NavigationHUD } from "../cad/navigation/hud/navigation-hud";
import { CAD_GEOMETRY_LAYER, markNavigationPickable, NavigationPicker } from "../cad/navigation/navigation-picker";
import type { NavigationAction, NavigationProfileID } from "../cad/navigation/navigation-profile";
import { CadBackground } from "../cad/rendering/cad-background";
import { CadMaterialFactory } from "../cad/rendering/cad-material-factory";
import { visualSelection, visualType } from "../cad/rendering/visualization-render-model";
import { CATIA_VISUAL_THEME } from "../cad/rendering/cad-visual-theme";
import { SKETCH_FEEDBACK_ORDER, sketchGeometryPickPriority } from "../cad/rendering/sketch-feedback-style";
import { makeDatumReferenceLine, makeOcclusionVisibleHighlightLine, makeOcclusionVisibleSegments,
  makeSketchOverlayLine, updateHighlightLineResolution } from "../cad/rendering/interaction-highlight";
import { constraintSymbolCode, makeConstraintDimensionLabel, makeSketchConstraintRenderable } from "../cad/rendering/sketch-constraint-renderer";
import { isDimensionConstraintKind, type ConstraintKind } from "../cad/sketch/sketch-constraint-definition";
import { measureSketchDimension, sketchDimensionText } from "../cad/sketch/sketch-constraint-layout";
import { sketchReferenceDimensions, SKETCH_INPUT_POLICY } from "../cad/sketch/sketch-input-policy";
import { sampleSketchEntity, ellipsePoint, sketchEntityPoint, splineEditablePoints, splineReferencePoint } from "../cad/sketch/sketch-geometry";
import { sketchProfileFeedback } from "../cad/sketch/sketch-profile-analysis";
import { CadShaderLibrary } from "../cad/rendering/shader/cad-shader-library";
import { manipulatorFrame, transformAroundWorldPivot, viewportMetrics, worldUnitsPerCssPixel } from "../cad/rendering/viewport-metrics";
import { randomUUID } from "../utils/random-uuid";
import { assemblyConstraintGlyph } from "../cad/assembly/assembly-constraint-ux";
import { ControlSplineSketchTool, EllipseSketchTool, EllipticalArcSketchTool, CenterRectangleSketchTool, OrientedRectangleSketchTool, ThreePointCircleSketchTool, ThreePointArcSketchTool, ArcSketchTool, AssemblyConstraintTool, AssemblyMoveTool, CircleSketchTool, ConstraintSketchTool, LineSketchTool, LinearDimensionSketchTool, PointSketchTool, PolylineSketchTool, ProjectExternalGeometrySketchTool, RectangleSketchTool, RegularPolygonSketchTool, SelectTool, SplineSketchTool, type AssemblyConstraintToolKind, type ToolViewportPort } from "../cad/tool/cad-tool";
import { SketchEditTool } from "../cad/tool/sketch-edit-tool";
import { ToolManager } from "../cad/tool/tool-manager";
import type {
  Artifact as ArtifactDescriptor, AssemblyGeometryRef, AxisSystem, DatumAxis, DatumPlane, DocumentStructureNode, DocumentView as DocumentDescriptor, Feature, PlaneName, Publication, ReferenceGeometry, Selection, SelectionItem, SketchConstraint, SketchEntity, SketchGeometryRef, SketchOperation, SketchPlane, Vec2, Vec3, VisualizationManifest,
} from "../types";

type Callbacks = {
  selectionsChanged: (selections: SelectionItem[]) => void;
  preselectionChanged: (selection: Selection) => void;
  sketchOperations: (featureID: string, operations: SketchOperation[], intent?:SketchCommitIntent) => Promise<SketchCommitResult>;
  sketchCommandChanged?: (state:SketchCommandState|undefined)=>void;
  sketchReceiptChanged?:(receipt:SketchCommitReceipt|undefined)=>void;
  sketchReceiptCheck?:(receipt:SketchCommitReceipt)=>Promise<SketchCommitResult>;
  sketchPreview?:(featureId:string,operations:SketchOperation[],signal?:AbortSignal)=>Promise<import("../types").CommandPreview>;
  toolPromptChanged: (prompt: string) => void;
  toolUseCompleted: () => void;
  dimensionEditRequested: (request: { mode: "edit"; featureId: string; constraintId: string; value?: number; unit: "mm" | "deg"; x: number; y: number }) => void;
  dimensionCreateRequested: (request: { mode: "create"; featureId: string; kind: "DISTANCE"|"HORIZONTAL_DISTANCE"|"VERTICAL_DISTANCE"|"LENGTH"|"RADIUS"|"DIAMETER"|"MAJOR_RADIUS"|"MINOR_RADIUS"|"ANGLE";
    references: SketchGeometryRef[]; labelPosition: Vec2; value: number; unit: "mm"|"deg"; x: number; y: number }) => void;
  activeToolChanged: (toolID: import("../state/workbench-store").WorkbenchToolID) => void;
  instanceMoved: (documentId: string, candidate: AssemblyInteractionCommit) => Promise<void>;
  assemblyInteractionBegin: (documentId: string, input: AssemblyInteractionBegin, signal: AbortSignal) => Promise<AssemblyInteractionSession>;
  assemblyInteractionUpdate: (documentId: string, input: AssemblyInteractionUpdate, signal: AbortSignal) => Promise<AssemblyInteractionFrame>;
  assemblyInteractionCancel: (documentId: string, sessionId: string) => Promise<unknown>;
  assemblyInteractionState?: (state: AssemblyInteractionState, reason?: string) => void;
  operationFailed?:(error:unknown)=>void;
  inspectAssemblySupports?:(documentId:string,references:AssemblyGeometryRef[],signal:AbortSignal)=>Promise<SupportInspection>;
  assemblyConstraintRequested: (kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]) => void;
  debugStateChanged?: (state: ViewportDebugState) => void;
};

type SolidContext = {
  bodyId?: string;
  displayStageFeatureId?:string;
  contextVariantKey?: string;
  instancePath?: SelectionItem["instancePath"];
  documentId: string; versionId?: string; geometryKey: string; occurrencePath: string; treeNodeId: string; instanceId?: string;
};

type SolidBinding = { group: THREE.Group; mesh: THREE.Mesh; artifact: Artifact; context: SolidContext };
export type ViewportEditContext = { view: DocumentDescriptor; occurrencePath?: string; translation?: Vec3;
  rotation?: [number, number, number, number]; bodyTreeNodeId?: string; liveConstraintProjection?: boolean };

// A Product viewport owns the assembly scene, while sketch interaction belongs
// to the active occurrence's reference document. Keeping this decision in one
// place prevents hit testing and constraint tools from accidentally reading the
// root Product (which intentionally has no Part feature collection).
export function sketchInteractionView(view?: DocumentDescriptor, editContext?: ViewportEditContext): DocumentDescriptor | undefined {
  if (editContext?.view.document.type === "PART") return editContext.view;
  return view?.document.type === "PART" ? view : undefined;
}

export function datumAxisHitAccepted(distanceToRay: number | undefined, tolerance: number): boolean {
  return distanceToRay !== undefined && distanceToRay <= tolerance;
}

export function bindPublicationSelection(selection: SelectionItem, root?: DocumentStructureNode): SelectionItem {
  if (!root || selection.publicationId) return selection;
  const matches: Array<{ id: string; publication: Publication }> = [];
  const matchingBodyIds = new Set<string>();
  const scanBodies = (node: DocumentStructureNode) => {
    if (node.kind === "BODY" && node.geometryKey === selection.geometryKey &&
        node.versionId === selection.versionId &&
        (node.instancePath?.canonical ?? "") === (selection.occurrencePath ?? "") && node.bodyId) {
      matchingBodyIds.add(node.bodyId);
    }
    node.children?.forEach(scanBodies);
  };
  scanBodies(root);
  const visit = (node: DocumentStructureNode) => {
    const source = node.publication;
    const forwarded = node.productPublication;
    const resolution = source?.resolution ?? forwarded?.resolution;
    const occurrencePath = source ? node.instancePath?.canonical ?? "" : forwarded?.target.instancePath.canonical ?? "";
    const topologyMatches = resolution?.status === "CONNECTED" && resolution.topologyKind?.toLowerCase() === selection.kind &&
      "topologyId" in selection && resolution.localId === selection.topologyId &&
      resolution.geometryKey === selection.geometryKey && resolution.resolvedVersionId === selection.versionId &&
      (source?.target.persistentSelection?.sourceBodyId
        ? source.target.persistentSelection.sourceBodyId === selection.bodyId
        : matchingBodyIds.size === 1 && matchingBodyIds.has(selection.bodyId ?? ""));
    const datumID = source?.target.datumId ?? resolution?.geometryId;
    const datumMatches = resolution?.status === "CONNECTED" &&
      (["plane", "axis", "axis-system"].includes(selection.kind) && datumID && selection.entityId === datumID);
    if ((source || forwarded) && occurrencePath === (selection.occurrencePath ?? "") && (topologyMatches || datumMatches)) {
      const publication: Publication = source ?? {
        id: forwarded!.id, name: forwarded!.name, type: forwarded!.type, semanticPurpose: forwarded!.semanticPurpose,
        compatibilityVersion: forwarded!.compatibilityVersion,
        target: { kind: resolution?.topologyKind ? "TOPOLOGY" : "DATUM", sourceVersionId: resolution?.resolvedVersionId },
        contract: forwarded!.contract, resolution: forwarded!.resolution,
      };
      matches.push({ id: source?.id ?? forwarded!.id, publication });
    }
    node.children?.forEach(visit);
  };
  visit(root);
  return matches.length === 1 ? { ...selection, publicationId: matches[0].id, publication: matches[0].publication } : selection;
}

export type ViewportDebugState = {
  input: InputState;
  activeTool: string;
  selectionKeys?: string[];
  highlightedVisible?: number;
  featureSelection?:{role:string;count:number;overlays:number};
  datumPreview?:{kind:string;origin:Vec3;direction:Vec3};
  navigationProfile: NavigationProfileID;
  navigationAction: NavigationAction;
  navigation?: NavigationSnapshot;
  hudScreen?: { x: number; y: number };
};

function sketchReferenceEntities(feature?: Feature): SketchEntity[] {
  return [...(feature?.sketch?.entities ?? []), ...(feature?.sketch?.externalGeometry ?? []).flatMap((external) => external.status==="CONNECTED"&&external.snapshot ? [{
    id: external.id, kind: external.snapshot.kind, role: "CONSTRUCTION" as const, point: external.snapshot.point,
    start: external.snapshot.start, end: external.snapshot.end, center: external.snapshot.center, radius: external.snapshot.radius,
  } satisfies SketchEntity] : [])];
}

const planeColors: Record<PlaneName | "CUSTOM", number> = { XY: CATIA_VISUAL_THEME.axisZ, XZ: CATIA_VISUAL_THEME.axisY, YZ: CATIA_VISUAL_THEME.axisX, CUSTOM: CATIA_VISUAL_THEME.sketchExternal };

function planeFrame(plane: PlaneName | SketchPlane): { origin: THREE.Vector3; normal: THREE.Vector3; u: THREE.Vector3; v: THREE.Vector3 } {
  if (typeof plane !== "string") {
    const normal = new THREE.Vector3().fromArray(plane.normal).normalize();
    const u = new THREE.Vector3().fromArray(plane.uDirection).normalize();
    return { origin: new THREE.Vector3().fromArray(plane.origin), normal, u, v: normal.clone().cross(u).normalize() };
  }
  if (plane === "XY") return { origin: new THREE.Vector3(), normal: new THREE.Vector3(0, 0, 1), u: new THREE.Vector3(1, 0, 0), v: new THREE.Vector3(0, 1, 0) };
  if (plane === "XZ") return { origin: new THREE.Vector3(), normal: new THREE.Vector3(0, -1, 0), u: new THREE.Vector3(1, 0, 0), v: new THREE.Vector3(0, 0, 1) };
  return { origin: new THREE.Vector3(), normal: new THREE.Vector3(1, 0, 0), u: new THREE.Vector3(0, 1, 0), v: new THREE.Vector3(0, 0, 1) };
}

function localToWorld(plane: PlaneName | SketchPlane, point: Vec2): THREE.Vector3 {
  const frame = planeFrame(plane);
  return frame.origin.clone().addScaledVector(frame.u, point[0]).addScaledVector(frame.v, point[1]);
}

function worldToLocal(plane: PlaneName | SketchPlane, point: THREE.Vector3): Vec2 {
  const frame = planeFrame(plane); const relative = point.clone().sub(frame.origin);
  return [relative.dot(frame.u), relative.dot(frame.v)];
}

function sketchDiagnosticColor(status: string | undefined, construction = false): number {
  if (construction) return CATIA_VISUAL_THEME.sketchConstruction;
  switch (status) {
  case "SOLVED":
  case "FULLY_CONSTRAINED": return CATIA_VISUAL_THEME.sketchSolved;
  case "CONFLICTING":
  case "FAILED":
  case "INVALID_MODEL": return CATIA_VISUAL_THEME.sketchInvalid;
  case "REDUNDANT": return CATIA_VISUAL_THEME.sketchRedundant;
  default: return CATIA_VISUAL_THEME.sketchProfile;
  }
}

function rayPlane(plane: PlaneName | SketchPlane): THREE.Plane {
  const frame = planeFrame(plane);
  return new THREE.Plane().setFromNormalAndCoplanarPoint(frame.normal, frame.origin);
}

function constraintTreeNodeID(featureTreeNode: string, kind: ConstraintKind, constraintID: string): string {
  const group = isDimensionConstraintKind(kind) ? "dimensions" : "logical";
  return `${featureTreeNode}/constraints/${group}/constraint:${constraintID}`;
}

function makeGeometry(artifact: Artifact): THREE.BufferGeometry {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.Float32BufferAttribute(artifact.mesh.vertices.flat(), 3));
  geometry.setIndex(artifact.mesh.triangles.flat());
  if (artifact.mesh.normals) geometry.setAttribute("normal", new THREE.Float32BufferAttribute(artifact.mesh.normals.flat(),3));
  else geometry.computeVertexNormals();
  geometry.computeBoundingSphere();
  const accelerated = geometry as THREE.BufferGeometry & {
    computeBoundsTree: typeof computeBoundsTree; disposeBoundsTree: typeof disposeBoundsTree;
  };
  accelerated.computeBoundsTree = computeBoundsTree;
  accelerated.disposeBoundsTree = disposeBoundsTree;
  accelerated.computeBoundsTree({ indirect: true });
  return geometry;
}


export class CadViewportEngine {
  private moveFinalization?:Promise<void>;
  private toolActivationGeneration=0;
  async settleAssemblyInteraction():Promise<void>{
    await this.moveFinalization;
    if(this.moveCommitPending||this.moveInteraction.hasUncommittedFinal||this.moveManipulator.isDragging())
      throw Object.assign(new Error("请先确认、重试或取消尚未保存的移动。"),{code:"ASSEMBLY_OPERATION_PENDING"});
  }
  private readonly scene = new THREE.Scene();
  private readonly analysisScene = new THREE.Scene();
  private motionMarkers?:THREE.Group;
  private motionPresentation?:MotionPresentation;
  showRemainingMotion(motion?:MotionPresentation):void {
    disposeMotionMarkers(this.motionMarkers);this.motionMarkers=undefined;this.motionPresentation=undefined;
    const context=this.editContext?.view??this.view;
    if(motion?.freedom&&context?.document.id===motion.documentId&&context.document.versionId===motion.revisionId&&motion.ownerOccurrence===(this.editContext?.occurrencePath??"")){
      const markers=makeMotionMarkers(motion,Math.max(1,this.camera.position.distanceTo(this.navigation.target)*0.08));
      if(this.editContext?.occurrencePath){markers.position.fromArray(this.editContext.translation??[0,0,0]);markers.quaternion.fromArray(this.editContext.rotation??[0,0,0,1]);}
      this.motionPresentation=motion;this.motionMarkers=markers;this.analysisScene.add(markers);
    }
    this.invalidate();
  }
  private readonly camera = new THREE.OrthographicCamera(-150, 150, 150, -150, 0.1, 5000);
  private readonly renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: "high-performance" });
  private readonly shaders = new CadShaderLibrary();
  private readonly materials = new CadMaterialFactory(this.shaders);
  private readonly sketchGrid = new InfiniteGroundGrid(CATIA_VISUAL_THEME.sketchGrid, "sketch");
  private readonly groundGrid = new InfiniteGroundGrid(CATIA_VISUAL_THEME.gridMinor);
  private readonly background = new CadBackground(this.shaders);
  private readonly moveManipulator: AssemblyManipulator;
  private moveTarget?: { group: THREE.Group; startPosition: THREE.Vector3; startQuaternion: THREE.Quaternion;
    startPivot: THREE.Vector3; localPivot: THREE.Vector3 };
  private readonly manipulatorPivots = new Map<string, THREE.Vector3>();
  private readonly manipulatorFrames = new Map<string, THREE.Quaternion>();
  private pendingManipulatorAnchor?:{instanceId:string;anchor:ManipulatorAnchor};
  private readonly manipulatorSnapCache=new ManipulatorSnapCache();
  private snapLock?:{key:string;anchor:ManipulatorAnchor};
  private snapInput?:{key:string;hit:THREE.Intersection};
  private lastManipulatorPick?:{selection:SelectionItem;hit:THREE.Intersection;revisionId:string};
  private readonly navigation: NavigationController;
  private readonly navigationHUD: NavigationHUD;
  private readonly tools: ToolManager;
  private readonly sketchModal:SketchModalInputController;
  private readonly selectionController: SelectionController;
  private readonly interaction: InteractionRouter;
  private readonly input: InputManager;
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();
  private readonly content = new THREE.Group();
  private readonly helpers = new THREE.Group();
  private datumPreview?: DatumPreview;
  private datumPreviewGroup?: THREE.Group;
  private readonly sketchContext = new THREE.Group();
  private readonly environment = new THREE.Group();
  private readonly studioEnvironment: THREE.WebGLRenderTarget;
  private viewTransition!: ViewTransition;
  private readonly interruptViewTransition = () => this.viewTransition?.cancel();
  private readonly lighting = new THREE.Group();
  private readonly contentBounds = new THREE.Box3();
  private readonly selectable = new Map<string, THREE.Object3D>();
  private readonly selectionIndex = new SelectionIndex();
  private readonly solidBindings = new Map<string, SolidBinding>();
  private readonly instanceGroups = new Map<string, THREE.Group>();
  private insertPatternPreview?: THREE.Group;
  private readonly assemblyConstraintReferences = new Map<string, SelectionItem[]>();
  private assemblyConstraintMarkers = new Map<string,{signature:string;group:THREE.Group;selection:SelectionItem}>();
  private referenceVisibility: ReferenceVisibility = { ...DEFAULT_REFERENCE_VISIBILITY };
  private solidDisplay: SolidDisplaySettings = { ...DEFAULT_SOLID_DISPLAY };
  private readonly screenStableReferences = new Map<THREE.Object3D, number>();
  private datumAxisPickToleranceWorld = 0;
  private selected: SelectionItem[] = [];
  private preselected: Selection = null;
  private selectedOverlays: THREE.Object3D[] = [];
  private preselectedOverlays: THREE.Object3D[] = [];
  private highlightedRoots = new Set<THREE.Object3D>();
  private view?: DocumentView;
  private renderGeometrySignature?: string;
  private editContext?: ViewportEditContext;
  private sketchPlane?: SketchPlane;
  private activeSketchID?: string;
  // All sketch transient geometry, dimensions, snap and role feedback share
  // one non-pickable rendering layer. Their owning tool/modal retains lifecycle
  // and generation control; attaching display here never accepts model input.
  private sketchPreviewLayer?:THREE.Group;
  private attachSketchPreview(object:THREE.Object3D):void {
    if(!this.sketchPreviewLayer){
      this.sketchPreviewLayer=new THREE.Group();this.sketchPreviewLayer.name="sketch-preview";
      this.sketchPreviewLayer.userData.transient=true;this.scene.add(this.sketchPreviewLayer);
    }
    object.userData.transient=true;this.sketchPreviewLayer.add(object);
  }
  private preview?: THREE.Object3D;
  private referencePreview?: THREE.Object3D;
  private referenceHover?: THREE.Object3D;
  private snapPreview?: THREE.Object3D;
  private sketchManipulator?:AssemblyManipulator;
  private sketchManipulatorChanged?:(value:{translation:Vec2;angle:number;origin:Vec2;finish?:boolean})=>void;
  private sketchManipulatorLastValue?:{translation:Vec2;angle:number;origin:Vec2};
  private sketchManipulatorOrigin:Vec2=[0,0];
  private lastSketchSnap?: SketchSnapResult;
  private commandPreview?: THREE.Object3D;
  private previewBody?: { group: THREE.Group; visible: boolean };
  private assemblyPosePreview?: Map<string, { position: THREE.Vector3; rotation: THREE.Quaternion }>;
  private dimensionDrag?: { selection: Extract<SelectionItem, { kind: "sketch-constraint" }>; constraint: SketchConstraint;
    root?: THREE.Object3D; rootParent?: THREE.Object3D; rootIndex?: number; startX: number; startY: number; position?: Vec2; scopeKey:string };
  private readonly moveInteraction: AssemblyInteractionController;
  private moveCommitPending?:{documentId:string;candidate:AssemblyInteractionCommit};
  private moveCommitInFlight=false;
  private moveGestureGeneration=0;
  private moveNominalBase?:{documentId:string;revisionId:string};
  private moveSessionDocumentId = "";
  private readonly moveSessionDocuments = new Map<string,string>();
  private moveNominalScene?: Map<string, TransformPose>;
  private moveFrameRotation?: [number,number,number,number];
  private desiredMovePose?:{translation:Vec3;rotation:[number,number,number,number]};
	private acceptedMovePose?:{translation:Vec3;rotation:[number,number,number,number];previewId?:string};
  private activeToolID = "select";
  private reconnectExternalID?: string;
  private featureSelection?: FeatureSelectionSession;
  private loftConnections?: THREE.Group;
  private releaseFeatureSketchPick?:()=>void;
  private selectionMode: SelectionMode = selectionModeForTool("select");
  private sketchReturnView?: SavedView;
  private navigationProfile: NavigationProfileID = "default";
  private captureSettings: CaptureSettings = DEFAULT_CAPTURE_SETTINGS;
  private treeVisibilityOverrides: TreeVisibilityOverrides = {};
  private visibilityResolver?: VisibilityResolver;
  private readonly resizeObserver: ResizeObserver;
  private animationFrame = 0;
  private disposed = false;
  private readonly transforms = new TransformTransitionSystem(() => this.invalidate());

  constructor(private readonly host: HTMLElement, private readonly callbacks: Callbacks) {
    this.scene.background = null;
    this.camera.position.set(300, -300, 300);
    this.camera.up.set(0, 0, 1);
    this.camera.layers.enable(CAD_GEOMETRY_LAYER);
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 1.5));
    this.renderer.shadowMap.enabled = false;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.autoClear = false;
    this.studioEnvironment = createStudioEnvironment(this.renderer);
    this.scene.environment = this.studioEnvironment.texture;
    host.appendChild(this.renderer.domElement);

    this.moveInteraction = new AssemblyInteractionController({
      begin:(input,signal)=>{
        const documentId=this.moveSessionDocumentId,generation=this.moveGestureGeneration;
        return this.callbacks.assemblyInteractionBegin(documentId,input,signal).then(session=>{
          this.moveSessionDocuments.set(session.sessionId,documentId);
          if(generation===this.moveGestureGeneration&&!signal.aborted){
            // Rollback is the server's frozen nominal, not a possibly halfway
            // ordinary render transition sampled at pointerdown.
            this.moveNominalScene=new Map(session.nominalPoses.map(p=>[p.instanceId,this.transformPose(p.translation,p.rotation)]));
          }
          return session;
        });
      },
      update:(input,signal)=>this.callbacks.assemblyInteractionUpdate(this.moveSessionDocuments.get(input.sessionId)??this.moveSessionDocumentId,input,signal),
      cancel:sessionId=>{const documentId=this.moveSessionDocuments.get(sessionId)??this.moveSessionDocumentId;this.moveSessionDocuments.delete(sessionId);return this.callbacks.assemblyInteractionCancel(documentId,sessionId);},
      frame:frame=>this.applyAcceptedMoveFrame(frame),
      failure:error=>this.callbacks.operationFailed?.(this.moveInteraction.hasUncommittedFinal&&error&&typeof error==="object"?
        Object.assign(error,{retry:()=>this.retryAssemblyMoveCommit(),cancel:()=>this.cancelMovePreviewGesture("显式取消未保存移动")}):error),
      state:(state,reason)=>{
        // A numerical FAILED frame still owns a live, recoverable Session.
        // Only invalidation or a closed failed transport restores authority;
        // otherwise retain the last feasible display for same-goal recovery.
        if((state==="invalidated"||state==="failed"&&!this.moveInteraction.sessionId)&&!this.moveCommitPending)this.restoreMoveNominalScene();
        this.callbacks.assemblyInteractionState?.(state,reason);
      },
    });
    this.moveManipulator = new AssemblyManipulator(this.shaders, {
      dragStarted: () => { this.navigation.setEnabled(false); this.beginMovePreviewGesture(); },
      snapPivot: (x, y) => this.snapManipulatorPivot(x, y),
      pivotChanged: (anchor) => this.updateManipulatorPivot(anchor),
      poseChanged: () => { this.updateMoveTarget(); this.invalidate(); },
      visualChanged: () => this.invalidate(),
      dragFinished: (commit) => {
        this.navigation.setEnabled(true);
        if (commit) this.finishMovePreviewGesture();
        else this.cancelMovePreviewGesture();
      },
    });
    this.scene.add(this.moveManipulator.root);

    const hemisphere = new THREE.HemisphereLight(CATIA_VISUAL_THEME.lightSky, CATIA_VISUAL_THEME.lightGround, CATIA_VISUAL_THEME.hemisphereIntensity);
    const keyLight = new THREE.DirectionalLight(CATIA_VISUAL_THEME.lightKey, CATIA_VISUAL_THEME.keyIntensity);
    keyLight.position.set(-3, -4, 7);
    const fillLight = new THREE.DirectionalLight(CATIA_VISUAL_THEME.lightFill, CATIA_VISUAL_THEME.fillIntensity);
    fillLight.position.set(5, 2, 3);
    const rimLight = new THREE.DirectionalLight(CATIA_VISUAL_THEME.lightRim, CATIA_VISUAL_THEME.rimIntensity);
    rimLight.position.set(-4, 6, 3);
    this.lighting.add(hemisphere, keyLight, fillLight, rimLight);
    this.sketchContext.renderOrder = 0; // Child semantic orders own sketch layering.
    this.scene.add(this.environment, this.lighting, this.content, this.helpers, this.sketchContext, this.sketchGrid.object);
    this.sketchGrid.object.visible = false;

    const navigationPicker = new NavigationPicker(
      this.camera,
      this.renderer.domElement,
      () => this.content.children,
    );
    this.navigation = new NavigationController(this.camera, () => ({
      width: this.renderer.domElement.clientWidth,
      height: this.renderer.domElement.clientHeight,
    }), () => {
      this.updateCameraClipping();
      this.invalidate();
    }, navigationPicker, () => this.visibleContentCenter(), "default",
    import.meta.env.DEV && import.meta.env.VITE_INPUT_DEBUG === "true",
      () => this.visibleContentBounds(), () => this.fit(), () => Boolean(this.activeSketchID));
    this.viewTransition = new ViewTransition(this.camera, this.navigation.target);
    for (const event of ["pointerdown", "wheel", "keydown"])
      this.host.addEventListener(event, this.interruptViewTransition, true);
    this.navigationHUD = new NavigationHUD();
    this.tools = new ToolManager({ viewport: this.toolViewportPort() });
    this.tools.register(new SelectTool());
    this.tools.register(new AssemblyMoveTool());
    for (const kind of ["fix", "rigid", "fix_together", "contact", "coincident", "concentric", "angle", "parallel", "perpendicular", "distance"] as const)
      this.tools.register(new AssemblyConstraintTool(kind));
    this.tools.register(new PointSketchTool());
    this.tools.register(new ProjectExternalGeometrySketchTool());
    this.tools.register(new LineSketchTool());
    this.tools.register(new CircleSketchTool());
    this.tools.register(new ArcSketchTool());
    this.tools.register(new PolylineSketchTool());
    this.tools.register(new SplineSketchTool());
    this.tools.register(new ControlSplineSketchTool());
    this.tools.register(new RectangleSketchTool());
    this.tools.register(new RegularPolygonSketchTool());
    this.tools.register(new RegularPolygonSketchTool("CIRCUMSCRIBED"));
    this.tools.register(new CenterRectangleSketchTool());
    this.tools.register(new OrientedRectangleSketchTool());
    this.tools.register(new ThreePointCircleSketchTool());
    this.tools.register(new ThreePointArcSketchTool());
    this.tools.register(new EllipseSketchTool());
    this.tools.register(new EllipticalArcSketchTool());
    for(const kind of ["delete","copy","move","mirror","split","trim","fillet","chamfer","extend","complement","close","offset","spline_insert","spline_delete","spline_close","spline_control","construction"] as const)this.tools.register(new SketchEditTool(kind));
    this.tools.register(new LinearDimensionSketchTool());
    for (const kind of ["COINCIDENT","PARALLEL","COLLINEAR","FIXED","HORIZONTAL","VERTICAL","PERPENDICULAR","TANGENT","EQUAL","DISTANCE","HORIZONTAL_DISTANCE","VERTICAL_DISTANCE","LENGTH","RADIUS","MAJOR_RADIUS","MINOR_RADIUS","ANGLE","CONCENTRIC","POINT_ON_OBJECT","MIDPOINT","SYMMETRY"] as const)
      this.tools.register(new ConstraintSketchTool(kind));
    this.tools.activate("select");
    this.selectionController = new SelectionController(
      (x, y, additive) => this.pick(x, y, additive),
      (x, y) => this.preselectAt(x, y),
      () => { this.preselect(null, true); this.clearSnapPreview(); },
      { enabled:()=>Boolean(this.activeSketchID&&this.activeToolID==="select"&&!this.featureSelection),
        preview:(from,to)=>this.showSketchMarquee(from,to),clear:()=>this.clearSketchMarquee(),select:(from,to,additive)=>this.selectSketchMarquee(from,to,additive) },
    );
    this.interaction = new InteractionRouter(this.tools, this.selectionController, this.navigation);
    this.sketchModal=new SketchModalInputController(this.interaction,this.navigation,{
      blocked:()=>Boolean(this.sketchCommitPending||this.sketchReceipt||this.activeSketchID&&this.pendingVisualSnapshot),
      pick:(request,event)=>request.featureId!==this.activeSketchID?undefined:this.sketchReferenceAt(event.x,event.y,request.pick,request.retained?{target:"ENTITY",entityId:request.retained.id,subElement:"WHOLE"}:undefined,request.allowed)??undefined,
      highlight:(request,reference)=>{if(reference)this.showReferencePreview(reference,request.references,request.slot,request.hoverOverridesRetained);else this.clearReferencePreview();},
    });
    this.input = new InputManager(this.renderer.domElement, this.sketchModal,{keyDown:this.sketchModal.modalKeyDown,keyUp:this.sketchModal.modalKeyUp});
    this.input.subscribe(() => this.emitDebugState());
    this.tools.subscribe((toolID) => {
      this.activeToolID = toolID ?? "select";
      this.selectionMode = selectionModeForTool(this.activeToolID);
      this.preselect(null, true);
      if (this.activeToolID !== "assembly.move") this.moveManipulator.detach();
      else {
        const selection=this.selected.length===1?this.selected[0]:undefined;
        const group=selection?.instanceId?this.instanceGroups.get(selection.instanceId):undefined;
        if(group&&selection?.kind!=="instance"){
          this.selected=[refreshOccurrenceSelection(this.view!,group.userData as SelectionItem)];
          this.callbacks.selectionsChanged(this.selected);
        }
        this.attachMoveManipulator();
        const picked=this.lastManipulatorPick;
        if(group&&picked&&picked.selection.instanceId===selection?.instanceId&&picked.revisionId===this.view?.document.versionId){
          const anchor=this.manipulatorAnchorFromIntersection(picked.hit,picked.selection);
          this.moveManipulator.attach(anchor.position,anchor.orientation??this.moveManipulator.frameQuaternion());this.updateManipulatorPivot(anchor);
        }
      }
      this.host.classList.toggle("drawing", Boolean(toolID?.startsWith("sketch.")) && Boolean(this.sketchPlane));
      this.callbacks.activeToolChanged(this.activeToolID as import("../state/workbench-store").WorkbenchToolID);
      this.emitDebugState();
    });
    this.navigation.subscribe((action, profile, snapshot) => {
      this.navigationProfile = profile;
      this.renderer.domElement.style.cursor = navigationCursor(snapshot);
      this.host.classList.toggle("navigating", action !== "none" || Boolean(snapshot.catia?.hudVisible));
      if (action !== "none") this.clearSnapPreview();
      this.updateNavigationHUD(snapshot);
      this.invalidate();
      this.emitDebugState();
    });

    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(host);
    this.resize();
    this.invalidate();
  }

  private sketchView(): DocumentDescriptor | undefined {
    return sketchInteractionView(this.view, this.editContext);
  }

  private visuals = new VisualRepository();
  private visualGeneration = 0;
  private pendingVisualSnapshot = false;
  private visualError?: HTMLDivElement;
  private geometrySignature(view: DocumentDescriptor, editContext?: ViewportEditContext): string {
    const part = (value?: DocumentDescriptor) => value?.part?.bodies.map((body) => [body.id, body.geometryKey, body.displayFallback, body.consumed]);
    return JSON.stringify([view.document.id, view.document.type, part(view),
      view.resolvedInstances?.map((resolved) => [resolved.occurrencePath,resolved.bodyId, resolved.geometryKey, resolved.displayFallback, resolved.translation, resolved.rotation,
        resolved.ownedSketchIds]),
      view.product?.instances.map((instance) => [instance.id, instance.translation, instance.rotation]),
      view.contextVariants?.map((variant) => [variant.owningInstancePath.canonical, variant.variantKey]),
      editContext?.occurrencePath, editContext?.translation, editContext?.rotation, part(editContext?.view)]);
  }
  render(view: DocumentDescriptor, editContext?: ViewportEditContext): void {
    const motionView=editContext?.view??view;
    if(this.motionPresentation&&(this.motionPresentation.documentId!==motionView.document.id||this.motionPresentation.revisionId!==motionView.document.versionId||this.motionPresentation.ownerOccurrence!==(editContext?.occurrencePath??"")))this.showRemainingMotion();
    // The same owning-Product evidence may remain valid while its outer rigid
    // occurrence moves. Reproject the temporary guides, not their local data.
    if(this.motionMarkers){
      this.motionMarkers.position.fromArray(editContext?.occurrencePath?(editContext.translation??[0,0,0]):[0,0,0]);
      this.motionMarkers.quaternion.fromArray(editContext?.occurrencePath?(editContext.rotation??[0,0,0,1]):[0,0,0,1]);
    }
    if(this.view?.document.versionId!==view.document.versionId){this.manipulatorSnapCache?.clear();this.snapLock=undefined;}
    if(this.view&&(this.view.document.id!==view.document.id || (this.editContext?.view.document.id??this.view.document.id)!==(editContext?.view.document.id??view.document.id) || this.editContext?.occurrencePath!==editContext?.occurrencePath))this.cancelMovePreviewGesture("editing context changed");
    else this.moveInteraction.invalidate(view.document.versionId);
    const signature = this.geometrySignature(view, editContext);
    const generation = ++this.visualGeneration;
    if (this.renderGeometrySignature === signature && !this.pendingVisualSnapshot && this.view) {
      this.editContext=editContext;
      this.updateDisplayProjection(view);
      return;
    }
    const scope = (display?: DocumentDescriptor, editing?: ViewportEditContext) => JSON.stringify([
      display?.document.id, display?.document.versionId,
      editing?.view.document.id, editing?.view.document.versionId, editing?.occurrencePath,
    ]);
    this.pendingVisualSnapshot = scope(this.view, this.editContext) !== scope(view, editContext);
    if (this.view?.document.id !== view.document.id) {
      this.visuals.clearReusablePreview();
      this.navigation.cancel();
      this.viewTransition.cancel();
      this.transforms.stopAll();
      this.moveManipulator.detach();
      this.clearInsertPatternPreview();
      this.clearCommandPreview();
      this.clearInteractionState();
      this.disposeGroup(this.content);
      this.disposeGroup(this.helpers);
      this.disposeGroup(this.sketchContext);
      this.solidBindings.clear();
      this.instanceGroups.clear();
      this.selectable.clear();
      this.selectionIndex.clear();
      this.view = undefined;
      this.editContext = undefined;
      this.invalidate();
    }
    void Promise.all([
      this.visuals.hydrate(view),
      editContext ? this.visuals.hydrate(editContext.view) : Promise.resolve(undefined),
    ]).then(([display, editing]) => {
      if (generation !== this.visualGeneration) return;
      this.visualError?.remove();
      this.visualError = undefined;
      this.visuals.retain([view, ...(editContext ? [editContext.view] : [])]);
      this.renderReady(display, editContext && editing ? { ...editContext, view: editing } : undefined);
      this.renderGeometrySignature = signature;
      this.pendingVisualSnapshot = false;
    }).catch(error => {
      if (generation !== this.visualGeneration) return;
      this.visualError?.remove();
      const message = document.createElement("div");
      message.setAttribute("role", "alert");
      message.textContent = `几何显示加载失败：${error instanceof Error ? error.message : String(error)}`;
      Object.assign(message.style, {
        position: "absolute", top: "48px", left: "12px", zIndex: "20",
        background: "white", padding: "8px", color: "#b42318",
      });
      this.host.append(message);
      this.visualError = message;
    });
  }

  private renderReady(view: DocumentView, editContext?: Omit<ViewportEditContext,"view"> & {view:DocumentView}): void {
    this.clearInsertPatternPreview();
    this.navigation.cancel();
    const previousDocumentID = this.view?.document.id;
    if (previousDocumentID !== view.document.id) {
      this.viewTransition.cancel();
      this.sketchReturnView = undefined;
      this.sketchPlane = undefined; this.activeSketchID = undefined;
      this.disposeGroup(this.sketchContext);
    }
    const previousInstancePoses = new Map([...this.instanceGroups].map(([id, group]) => [id, snapshotTransform(group)]));
    const retainedSelections = previousDocumentID === view.document.id ? [...this.selected] : [];
    const retainedMoveSelection = this.activeToolID === "assembly.move" ? [...this.selected] : [];
	this.clearCommandPreview(false);
	this.transforms.stopAll();
	this.clearInteractionState();
    this.view = view;
    this.visibilityResolver = visibilityResolverForView(view);
    this.editContext = editContext;
    this.moveManipulator.detach();
    this.moveTarget = undefined;
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.selectionIndex.clear();
    this.solidBindings.clear();
    this.instanceGroups.clear();
    this.assemblyConstraintReferences.clear();
    this.assemblyConstraintMarkers.clear();
    this.screenStableReferences.clear();
    if (view.document.type === "PART") this.renderPart(view);
    else this.renderProduct(view);
    if (view.document.type === "PRODUCT" && editContext?.view.document.type === "PART") {
      const consumedSketches = new Set((editContext.view.part?.features ?? []).flatMap((feature) => feature.profile ? [feature.profile] : []));
      for (const feature of editContext.view.part?.features ?? []) {
        if (feature.sketch) this.addSketch(feature, false, editContext.view, {
          documentId: editContext.view.document.id, bodyId:feature.bodyId, geometryKey:editContext.view.part?.bodies.find(b=>b.id===feature.bodyId)?.geometryKey??"",
          occurrencePath: editContext.occurrencePath ?? "", treeNodeId:view.resolvedInstances?.find(r=>r.occurrencePath===editContext.occurrencePath && r.bodyId===feature.bodyId)?.bodyTreeNodeId??"",
        }, editContext.translation, editContext.rotation, !consumedSketches.has(feature.id));
      }
    }
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.setDatumPreview(this.datumPreview);
    this.refreshContentBounds();
    if (previousDocumentID === view.document.id && view.document.type === "PRODUCT") {
      const starts: Array<{ object: THREE.Group; target: TransformPose }> = [];
      const targets: Array<{ object: THREE.Group; target: TransformPose; frame: () => void }> = [];
      for (const [id, group] of this.instanceGroups) {
        const previous = previousInstancePoses.get(id);
        if (!previous) continue;
        const target = snapshotTransform(group);
        starts.push({ object: group, target: previous });
        targets.push({ object: group, target, frame: () => this.syncAttachedManipulatorPosition(group) });
      }
      this.transforms.applyBatch(starts, "immediate");
      this.transforms.applyBatch(targets, "reconcile");
    }
    const validMoveSelection = retainedMoveSelection.filter((selection) => selection.kind === "instance" &&
      this.instanceGroups.has(selection.instanceId ?? selection.id));
    this.selectMany(this.activeToolID === "assembly.move" ? validMoveSelection : retainedSelections, false);
    if (previousDocumentID !== view.document.id) {
      standardView(this.camera, this.navigation.target, "ISO");
      this.frameContent();
    }
    this.emitDebugState();
    this.invalidate();
  }

  clear(): void {
    this.clearInsertPatternPreview();
	this.clearCommandPreview(false);
	this.transforms.stopAll();
	this.clearInteractionState();
    this.view = undefined;
    this.sketchReturnView = undefined;
    this.sketchPlane = undefined; this.activeSketchID = undefined;
    this.moveManipulator.detach();
    this.moveTarget = undefined;
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.instanceGroups.clear();
    this.contentBounds.makeEmpty();
    this.invalidate();
  }

  setDisplaySettings(references: ReferenceVisibility, display: SolidDisplaySettings): void {
    this.referenceVisibility = { ...references };
    this.solidDisplay = { ...display };
    this.applyTreeVisibility();
    for (const { group } of this.solidBindings.values()) applySolidDisplaySettings(group, display);
    this.preselect(null, true);
    this.invalidate();
  }

  setTreeVisibilityOverrides(overrides: TreeVisibilityOverrides): void {
    this.treeVisibilityOverrides = { ...overrides };
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.invalidate();
  }

  updateDisplayProjection(view: DocumentDescriptor): void {
    if (!this.view || this.view.document.id !== view.document.id) return;
    this.view = {...this.view,document:view.document,structureTree:view.structureTree,part:view.part,product:view.product,
      contextVariants:view.contextVariants,constraintDisplayScopes:view.constraintDisplayScopes,resolvedInstances:view.resolvedInstances};
    this.selectionIndex.setSemanticProjection(selection=>this.view?refreshOccurrenceSelection(this.view,selection):selection);
    // Geometry reuse must not reuse the old semantic snapshot. Pick closures
    // share the binding context; update it without replacing GPU resources.
    for(const binding of this.solidBindings.values()){
      const path=occurrenceSnapshot(view,binding.context.occurrencePath);
      if(path){binding.context.instancePath=path;binding.context.versionId=path.segments.at(-1)?.resolvedVersionId;
        binding.context.documentId=path.segments.at(-1)?.referencedDocumentId??binding.context.documentId;
        binding.context.contextVariantKey=view.contextVariants?.find(variant=>variant.owningInstancePath.canonical===binding.context.occurrencePath)?.variantKey;}
      binding.group.userData=refreshOccurrenceSelection(view,binding.group.userData as SelectionItem);
    }
    for(const group of this.instanceGroups.values()){
      const selection=group.userData as SelectionItem;
      group.userData=refreshOccurrenceSelection(view,selection);
      if(group.userData.kind)this.selectionIndex.register(group.userData as SelectionItem,group);
    }
    if(this.moveTarget&&!view.product?.instances.some(instance=>instance.id===this.moveTarget!.group.userData.id)){
      this.cancelMovePreviewGesture("运动组件已删除或替换");this.moveManipulator.detach();this.moveTarget=undefined;
    }
    this.selected=this.selected.map(selection=>refreshOccurrenceSelection(view,selection));
    if(this.preselected)this.preselected=refreshOccurrenceSelection(view,this.preselected);
    this.addAssemblyConstraintMarkers(this.view);
    this.visibilityResolver = visibilityResolverForView(view);
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.invalidate();
  }

  private editingSketchScope(): import("../cad/interaction/visibility-resolver").EditingSketchScope | undefined {
    if(this.activeSketchID)return {id:this.activeSketchID,occurrencePath:this.editContext?.occurrencePath??""};
    const session=this.featureSelection;
    if(!session||!["profile","axis","seam","point","geometry"].includes(session.role)||!session.sketchIds?.length)return;
    return {id:session.sketchIds[0],ids:session.sketchIds,documentId:session.documentId,occurrencePath:session.occurrencePath??""};
  }

  private semanticVisibilityAddress(entry: Partial<SelectionItem>): DisplayAddress | undefined {
    const documentId = entry.documentId;
    const occurrencePath = entry.occurrencePath ?? "";
    if (!documentId) return undefined;
    if (entry.kind === "body" && entry.bodyId) return {documentId, occurrencePath, kind:"BODY", entityId:entry.bodyId};
    if (entry.kind === "sketch" && entry.id) return {documentId, occurrencePath, kind:"SKETCH", entityId:entry.id, bodyId:entry.bodyId};
    if (entry.kind === "visual" && entry.featureId && entry.entityId) return {documentId, occurrencePath,
      kind:"SKETCH_ENTITY", entityId:entry.entityId, ownerEntityId:entry.featureId, bodyId:entry.bodyId};
    if (entry.kind === "sketch-constraint" && entry.featureId) return {documentId, occurrencePath,
      kind:"SKETCH", entityId:entry.featureId, bodyId:entry.bodyId};
    if (entry.kind === "instance" && entry.instanceId) {
      const node = this.view?.structureTree?.children?.find((candidate) => candidate.kind === "INSTANCE" && candidate.entityId === entry.instanceId);
      return {documentId: node?.ownerDocumentId ?? this.view?.document.id ?? documentId,
        occurrencePath: entry.occurrencePath ?? entry.instanceId, kind:"INSTANCE", entityId:entry.instanceId};
    }
    return undefined;
  }

  private applyTreeVisibility(): void {
    for (const root of [this.content, this.helpers]) root.traverse((object) => {
      const entry = object.userData as Partial<SelectionItem> & {visualizationPrimitive?: boolean; sketchFeatureID?: string};
      const semanticKey = typeof entry.kind === "string" && typeof entry.id === "string"
        ? selectionKey(entry as SelectionItem) : undefined;
      const legacyKey = typeof entry.treeNodeId === "string" ? entry.treeNodeId : undefined;
      const hidden = treeVisibilityOverride(semanticKey, this.treeVisibilityOverrides) ??
        treeVisibilityOverride(legacyKey, this.treeVisibilityOverrides);
      const address = this.semanticVisibilityAddress(entry);
      const semanticVisible = address ? this.visibilityResolver?.resolve(address, this.editingSketchScope()).effectiveVisible : undefined;
      const editingOverlayReplacesPrimitive = Boolean(this.editContext && this.activeSketchID && entry.visualizationPrimitive &&
        entry.sketchFeatureID === this.activeSketchID && entry.occurrencePath === this.editContext.occurrencePath);
      const category = referenceCategory(object.userData.kind, object.userData.axis);
      const featureReference = this.featureSelection && !this.featureSelection.localSketchId &&
        (entry.ownerDocumentId ?? entry.documentId) === this.featureSelection.documentId &&
        (entry.occurrencePath ?? "") === (this.featureSelection.occurrencePath ?? "") &&
        (this.featureSelection.role === "plane" && entry.kind === "plane" || this.featureSelection.role === "axis" && ["axis","axis-system"].includes(entry.kind??"") || this.featureSelection.role === "point" && entry.kind === "axis-system");
      if(object.userData.patternCenterMarker){ object.visible=this.featureSelection?.role==="point"&&!this.featureSelection.localSketchId;return; }
      if (category) object.visible = (this.referenceVisibility[category] || !!featureReference) &&
        !(root === this.helpers && this.sketchPlane && !featureReference) && hidden !== false;
      else if (editingOverlayReplacesPrimitive) object.visible = false;
      else if (semanticVisible !== undefined) object.visible = semanticVisible;
      else if (hidden === false) object.visible = false;
    });
    this.refreshInteractionHighlights();
  }

  private objectVisible(object: THREE.Object3D): boolean {
    for (let current: THREE.Object3D | null = object; current; current = current.parent) if (!current.visible) return false;
    return true;
  }

  private animatePlaneView(focus: THREE.Vector3, normal: THREE.Vector3, up: THREE.Vector3): void {
    const destination = this.camera.clone();
    const target = this.navigation.target.clone();
    orientPlaneView(destination, target, focus, normal, up);
    this.viewTransition.start(saveView(destination, target), performance.now());
    this.invalidate();
  }

  beginSketch(sketchID: string, plane: SketchPlane): void {
    const entering = this.activeSketchID !== sketchID;
    if (entering && !this.sketchReturnView) this.sketchReturnView = saveView(this.camera, this.navigation.target);
    this.navigation.cancel();
    this.activeSketchID = sketchID;
    this.sketchPlane = plane;
    this.moveManipulator.detach();
    if (entering) this.select(null);
    this.navigation.setEnabled(true);
    if (entering) {
      const frame = planeFrame(plane);
      const focus = viewFocus(this.camera, this.navigation.target);
      // Keep the region being inspected, projected onto the support plane.
      focus.addScaledVector(frame.normal, -focus.clone().sub(frame.origin).dot(frame.normal));
      this.animatePlaneView(focus, frame.normal, frame.v);
    }
    this.buildSketchContext();
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.callbacks.toolPromptChanged("选择：选择草图元素，或从工具栏启动创建命令");
    this.invalidate();
  }

  normalToPlane(selection: SelectionItem, plane?: NormalViewPlane): boolean {
    // Datum meshes already carry their local frame; only their parent placement
    // belongs here. Topology descriptors are in the solid's local coordinates.
    let placement: THREE.Matrix4 | undefined;
    if (selection.kind === "plane") {
      const object = this.selectionIndex.objectsFor(selection).find(value=>value.userData.kind === "plane");
      plane ??= object?.userData.datumPlane as NormalViewPlane | undefined;
      object?.updateWorldMatrix(true,false);
      placement = object?.parent?.matrixWorld;
    } else if (selection.kind === "face") {
      const binding = [...this.solidBindings.values()].find(value=>
        value.artifact.geometryKey === selection.geometryKey &&
        (value.context.occurrencePath ?? "") === (selection.occurrencePath ?? ""));
      binding?.mesh.updateWorldMatrix(true,false);
      placement = binding?.mesh.matrixWorld;
    }
    if (!placement || !plane) return false;
    const frame = normalViewFrame(plane,placement);
    if (!frame) return false;
    this.navigation.cancel();
    const focus = viewFocus(this.camera,this.navigation.target);
    focus.addScaledVector(frame.normal,-focus.clone().sub(frame.origin).dot(frame.normal));
    this.animatePlaneView(focus, frame.normal, frame.up);
    this.invalidate();
    return true;
  }

  normalToSketch(): void {
    if (!this.sketchPlane) return;
    this.navigation.cancel();
    const frame = planeFrame(this.sketchPlane);
    const focus = viewFocus(this.camera, this.navigation.target);
    focus.addScaledVector(frame.normal, -focus.clone().sub(frame.origin).dot(frame.normal));
    this.animatePlaneView(focus, frame.normal, frame.v);
  }

  endSketch(): void {
    if (!this.activeSketchID && !this.sketchReturnView) return;
    this.navigation.cancel();
	this.preselect(null, false);
    this.clearReferencePreview();
    this.sketchPlane = undefined;
    this.activeSketchID = undefined;
    this.host.classList.remove("drawing");
    this.tools.cancel();
    this.clearSnapPreview();
    this.navigation.setEnabled(true);
    this.disposeGroup(this.sketchContext);
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.refreshInteractionHighlights();
    this.callbacks.toolPromptChanged("");
    if (this.sketchReturnView) {
      this.viewTransition.start(this.sketchReturnView, performance.now());
      this.sketchReturnView = undefined;
      this.navigation.syncCamera(false);
    }
    this.invalidate();
  }

  setSketchDialogOpen(open:boolean):void {
    if(open)this.viewTransition.cancel(); // Freeze the current view, never orient or fit a dimension panel.
    this.sketchModal.setOpen(open);if(!open){this.clearDimensionDefinitionPreview();this.clearReferencePreview();}
  }
  private dimensionDefinitionPreview?:THREE.Group;
  private dimensionPreviewHidden=new Set<THREE.Object3D>();
  private dimensionPreviewGeneration=0;
  clearDimensionDefinitionPreview(restoreHighlights=true):void {
    const hadPreview=!!this.dimensionDefinitionPreview;
    this.dimensionPreviewGeneration++;
    if(this.dimensionDefinitionPreview){this.dimensionDefinitionPreview.removeFromParent();this.disposeRenderable(this.dimensionDefinitionPreview);this.dimensionDefinitionPreview=undefined;}
    for(const root of this.dimensionPreviewHidden)root.visible=true;
    this.dimensionPreviewHidden.clear();if(hadPreview&&restoreHighlights)this.refreshInteractionHighlights();this.invalidate();
  }
  async previewDimensionDefinition(featureId:string,operations:SketchOperation[],signal:AbortSignal):Promise<void> {
    this.clearDimensionDefinitionPreview();
    if(!operations.length)return;
    const generation=this.dimensionPreviewGeneration,scope=this.dimensionGestureScope();
    const current=()=>!signal.aborted&&generation===this.dimensionPreviewGeneration&&scope===this.dimensionGestureScope()&&this.sketchModal.visible;
    const result=await this.toolViewportPort().previewSketchOperations!(operations,signal);
    if(!current()||featureId!==this.activeSketchID||!this.sketchPlane)return;
    const feature=this.sketchView()?.part?.features.find(feature=>feature.id===featureId);
    if(!feature?.sketch)return;
    const visible=new Set(this.visibleSketchReferenceEntities().map(entity=>entity.id));
    const group=new THREE.Group();
    for(const entity of result.entities.filter(entity=>visible.has(entity.id)&&!entity.suppressed)){
      const points=sampleSketchEntity(entity).map(point=>localToWorld(this.sketchPlane!,point));if(!points.length)continue;
      const primitive=entity.kind==="POINT"?new THREE.Points(new THREE.BufferGeometry().setFromPoints(points),this.materials.point(CATIA_VISUAL_THEME.preview,9,false)):makeSketchOverlayLine(points,CATIA_VISUAL_THEME.preview,entity.role==="CONSTRUCTION"?2:2.5,entity.role==="CONSTRUCTION");
      primitive.renderOrder=entity.role==="CONSTRUCTION"?SKETCH_FEEDBACK_ORDER.construction:SKETCH_FEEDBACK_ORDER.geometry;
      updateHighlightLineResolution(primitive,this.renderer.domElement.clientWidth,this.renderer.domElement.clientHeight);group.add(primitive);
    }
    const replacements=new Map(operations.flatMap(operation=>operation.type==="UPDATE_CONSTRAINT"?[[operation.constraintId,operation.constraint] as const]:operation.type==="ADD_CONSTRAINT"?[[operation.constraint.id,operation.constraint] as const]:[]));
    const deleted=new Set(operations.flatMap(operation=>operation.type==="DELETE_CONSTRAINT"?[operation.constraintId]:[]));
    const constraints=[...feature.sketch.constraints.filter(c=>!replacements.has(c.id)&&!deleted.has(c.id)),...replacements.values()];
    const entities=[...result.entities,...sketchReferenceEntities(feature).filter(e=>!result.entities.some(candidate=>candidate.id===e.id))];
    for(const original of constraints){
      if(original.suppressed)continue;
      const measured=isDimensionConstraintKind(original.kind)?measureSketchDimension(original.kind,original.references,entities):undefined;
      const constraint=isDimensionConstraintKind(original.kind)?{...original,value:measured}:original;
      group.add(makeSketchConstraintRenderable(this.displaySketchConstraint(this.sketchView()!.document.id,featureId,constraint),entities,point=>localToWorld(this.sketchPlane!,point),this.materials,{width:this.renderer.domElement.clientWidth,height:this.renderer.domElement.clientHeight}));
    }
    // Hide only the active occurrence's normal display, never its pick index.
    // Reference replacement still picks the frozen authoritative model.
    this.helpers.traverse(root=>{const selection=root.userData;
      if(root.visible&&selection.featureId===featureId&&(selection.occurrencePath??"")===(this.editContext?.occurrencePath??"")&&(selection.kind==="visual"||selection.kind==="sketch-constraint")){
        root.visible=false;this.dimensionPreviewHidden.add(root);
      }
    });
    this.replaceTopologyOverlays("selected",[]);this.replaceTopologyOverlays("preselected",[]);
    this.dimensionDefinitionPreview=group;this.attachSketchPreview(group);this.invalidate();
  }
  beginSketchReferenceSelection(request:SketchReferenceSelectionRequest):()=>void {
    if(request.featureId!==this.activeSketchID)throw new Error("引用选择不属于当前草图");
    return this.sketchModal.begin(request);
  }
  highlightSketchReference(featureId:string,reference:SketchGeometryRef|undefined,slot?:number):void {
    if(featureId!==this.activeSketchID)return;
    if(reference)this.showReferencePreview(reference,[],slot);else this.clearReferencePreview();
  }
  locateSketchReference(featureId:string,reference:SketchGeometryRef,slot?:number):void {
    if(featureId!==this.activeSketchID||!this.sketchPlane)return;
    const point=this.sketchReferenceAnchor(reference);if(!point)return;
    const anchor=localToWorld(this.sketchPlane,point),offset=this.camera.position.clone().sub(this.navigation.target);
    this.navigation.target.copy(anchor);this.camera.position.copy(anchor).add(offset);this.camera.lookAt(anchor);this.navigation.syncCamera(false);this.showReferencePreview(reference,[],slot);this.invalidate();
  }
  private sketchReferenceAnchor(reference:SketchGeometryRef):Vec2|undefined {
    if(reference.target==="SKETCH_ORIGIN")return [0,0];
    if(reference.target==="SKETCH_X_AXIS")return [1,0];if(reference.target==="SKETCH_Y_AXIS")return [0,1];
    const entity=this.visibleSketchReferenceEntities().find(e=>e.id===reference.entityId);if(!entity)return;
    if(reference.subElement==="CONTROL"&&entity.kind==="SPLINE")return splineReferencePoint(entity,reference.controlPointIndex??0,reference.controlPointId);
    if(["START","END","CENTER","POINT"].includes(reference.subElement))return sketchEntityPoint(entity,reference.subElement as "START"|"END"|"CENTER"|"POINT")??undefined;
    const sample=sampleSketchEntity(entity);return sample[Math.floor(sample.length/2)];
  }
  commitDimensionOperations(featureId:string,operations:SketchOperation[],intent?:SketchCommitIntent):Promise<SketchCommitResult> {
    if(featureId!==this.activeSketchID)return Promise.reject(new Error("草图编辑会话已变化"));
    return this.commitSketchOperations(operations,intent);
  }
  private sketchCommandState?:SketchCommandState;
  private sketchLengthUnit: "mm"|"cm"|"m"|"in"="mm";
  private selectionSource:import("../cad/input/input-types").SelectionInputSource="selection";
  setSketchLengthUnit(unit:"mm"|"cm"|"m"|"in"):void {this.sketchLengthUnit=unit;}
  private sketchCommitPending?:Promise<SketchCommitResult>;
  private sketchReceipt?:SketchCommitReceipt;
  private deferredToolFinish?:{exit?:boolean;toolId:string};
  async retrySketchReceipt():Promise<void> {
    const receipt=this.sketchReceipt;if(!receipt||receipt.status!=="unknown"||!this.callbacks.sketchReceiptCheck)return;
    const scope={owner:this.sketchView()?.document.id,feature:this.activeSketchID,occurrence:this.editContext?.occurrencePath};
    const current=()=>scope.owner===receipt.ownerDocumentId&&scope.owner===this.sketchView()?.document.id&&scope.feature===this.activeSketchID&&scope.occurrence===this.editContext?.occurrencePath;
    receipt.status="committing";this.callbacks.sketchReceiptChanged?.({...receipt});
    let confirmed=false;
    try {
      const updated=await this.callbacks.sketchReceiptCheck(receipt);confirmed=true;
      this.sketchReceipt=undefined;this.callbacks.sketchReceiptChanged?.(undefined);
      if(updated&&current()) {
        this.pendingVisualSnapshot=true;
        const hydrated=await this.visuals.hydrate(updated);
        if(current()) {
          this.visualGeneration++;
          if(this.editContext&&this.view&&this.view.document.id!==updated.document.id)this.renderReady(this.view,{...this.editContext,view:hydrated});else this.renderReady(hydrated);
          this.pendingVisualSnapshot=false;
        }
      }
      if(current())this.tools.activate("select");
    } catch(error) {
      if(confirmed) {
        this.callbacks.toolPromptChanged("原请求已确认，几何显示加载失败，请重新读取；不会重复提交模型");this.callbacks.operationFailed?.(error);
        if(current())this.tools.activate("select");
      } else if(sketchCommitResultUnknown(error)) {
        receipt.status="unknown";receipt.error=error instanceof Error?error.message:String(error);this.callbacks.sketchReceiptChanged?.({...receipt});
      } else {
        this.sketchReceipt=undefined;this.callbacks.sketchReceiptChanged?.(undefined);this.callbacks.toolPromptChanged(`原请求明确失败：${error instanceof Error?error.message:String(error)}`);this.callbacks.operationFailed?.(error);
        if(current())this.tools.activate("select");
      }
    }
  }

  commandAction(action:SketchCommandAction):void { this.tools.commandAction(action); }
  private setSketchCommandState(state:SketchCommandState|undefined):void {
    if(JSON.stringify(this.sketchCommandState)===JSON.stringify(state))return;
    this.sketchCommandState=state;this.updateSketchParameterVisibility();
    this.callbacks.sketchCommandChanged?.(state);
    if(state) {
      const selections=state.selectedIds.flatMap(id=>{const entity=this.visibleSketchReferenceEntities().find(entity=>entity.id===id);return entity?[this.sketchEntitySelection(entity)]:[];});
      this.selectMany(selections,true,"command");
    }
  }
  private async commitSketchOperations(operations:SketchOperation[],intent?:SketchCommitIntent):Promise<SketchCommitResult> {
    if(this.sketchCommitPending)throw new Error("当前草图操作仍在提交，请等待结果");
    if(this.sketchReceipt?.status==="committing")throw new Error("正在确认原草图请求结果，请等待回执");
    if(this.pendingVisualSnapshot)throw new Error("正在加载已确认草图结果，请等待显示更新");
    if(this.sketchReceipt?.status==="unknown"&&intent?.requestId!==this.sketchReceipt.intent.requestId)throw new Error("先确认上一草图请求结果，避免重复编辑");
    const featureId=this.activeSketchID,owner=this.sketchView()?.document.id,occurrence=this.editContext?.occurrencePath,toolId=this.tools.activeToolID;
    if(!featureId||!owner)throw new Error("草图编辑会话已结束");
    if(!operations.length)throw new Error("操作为空，没有产生有效编辑");
    this.clearSnapPreview();
    const receipt:SketchCommitReceipt={ownerDocumentId:owner,featureId,operations:structuredClone(operations),intent:intent?structuredClone(intent):{requestId:randomUUID(),baseVersionId:this.sketchView()?.document.versionId},status:"committing"};
    this.sketchReceipt=receipt;this.callbacks.sketchReceiptChanged?.(receipt);
    const pending=Promise.resolve().then(()=>this.callbacks.sketchOperations(featureId,receipt.operations,receipt.intent));
    this.sketchCommitPending=pending;
    let confirmed:SketchCommitResult;
    try {
      const updated=await pending;confirmed=updated;
      if(updated&&this.activeSketchID===featureId&&this.sketchView()?.document.id===owner&&this.editContext?.occurrencePath===occurrence) {
        // The next gesture uses the accepted model only after its visual data is
        // ready. This does not reinterpret a failed request as a successful one.
        this.pendingVisualSnapshot=true;
        const display=await this.visuals.hydrate(updated);
        if(this.activeSketchID===featureId&&this.sketchView()?.document.id===owner&&this.editContext?.occurrencePath===occurrence) {
          this.visualGeneration++;
          if(this.editContext&&this.view&&this.view.document.id!==owner)this.renderReady(this.view,{...this.editContext,view:display});
          else this.renderReady(display);
          this.pendingVisualSnapshot=false;
        }
      }
      this.sketchReceipt=undefined;this.callbacks.sketchReceiptChanged?.(undefined);
      const finish=this.deferredToolFinish;this.deferredToolFinish=undefined;
      if(finish&&this.tools.activeToolID===finish.toolId){if(finish.exit)this.tools.activate("select");else this.callbacks.toolUseCompleted();}
      return updated;
    } catch(error) {
      if(confirmed){
        this.sketchReceipt=undefined;this.callbacks.sketchReceiptChanged?.(undefined);
        this.callbacks.toolPromptChanged("操作已确认，几何显示加载失败，请重新读取；不会重复提交模型");this.callbacks.operationFailed?.(error);
        const finish=this.deferredToolFinish;this.deferredToolFinish=undefined;if(finish&&this.tools.activeToolID===finish.toolId)this.callbacks.toolUseCompleted();
        return confirmed;
      }
      this.deferredToolFinish=undefined;
      if(sketchCommitResultUnknown(error)){receipt.status="unknown";receipt.error=error instanceof Error?error.message:String(error);this.callbacks.sketchReceiptChanged?.({...receipt});}
      else {this.sketchReceipt=undefined;this.callbacks.sketchReceiptChanged?.(undefined);this.callbacks.toolPromptChanged(`操作失败：${error instanceof Error?error.message:String(error)}`);this.callbacks.operationFailed?.(error);}
      throw error;
    } finally { if(this.sketchCommitPending===pending)this.sketchCommitPending=undefined; }
  }
  private sketchEntitySelection(entity:SketchEntity):SelectionItem {
    const view=this.sketchView()!,feature=view.part?.features.find(feature=>feature.id===this.activeSketchID);
    return {kind:"visual",id:`${this.editContext?.occurrencePath||"root"}:${this.activeSketchID}:${entity.id}`,visualType:entity.kind==="POINT"?"POINT":"CURVE",featureId:this.activeSketchID!,entityId:entity.id,documentId:view.document.id,ownerDocumentId:view.document.id,versionId:view.document.versionId,occurrencePath:this.editContext?.occurrencePath,bodyId:feature?.bodyId,role:entity.role};
  }
  private sketchEntitySelectionAt(x:number,y:number):SelectionItem|null {
    const ref=this.sketchReferenceAt(x,y,"ENTITY");
    const entity=ref?.target==="ENTITY"&&this.visibleSketchReferenceEntities().find(entity=>entity.id===ref.entityId);
    return entity?this.sketchEntitySelection(entity):null;
  }
  private visibleSketchDeletionEntities():SketchEntity[] {
    const members=this.sketchView()?.sketchPatternMembers?.[this.activeSketchID??""]??[];
    return [...this.visibleSketchReferenceEntities(),...members.filter(e=>{
      const selection=this.sketchEntitySelection(e),object=this.selectable.get(`visual:${selection.id}`);
      return !!object&&this.objectVisible(object);
    })];
  }
  private visibleSketchReferenceEntities():SketchEntity[] {
    const view=this.sketchView(),feature=view?.part?.features.find(feature=>feature.id===this.activeSketchID);
    if(!view||!feature?.sketch)return [];
    return sketchReferenceEntities(feature).filter(entity=>{
      if(entity.suppressed)return false;
      const address:DisplayAddress={documentId:view.document.id,occurrencePath:this.editContext?.occurrencePath??"",kind:"SKETCH_ENTITY",entityId:entity.id,ownerEntityId:feature.id,bodyId:feature.bodyId};
      const selection=this.sketchEntitySelection(entity),semantic=selectionKey(selection);
      return treeVisibilityOverride(semantic,this.treeVisibilityOverrides)!==false&&treeVisibilityOverride(selection.treeNodeId,this.treeVisibilityOverrides)!==false&&(this.visibilityResolver?.resolve(address,this.editingSketchScope()).effectiveVisible??true);
    });
  }
  captureToolSelections(selections: readonly SelectionItem[]): boolean {
    const result=this.tools.selectionInput(selections);
    if(result===SelectionInputResult.Unhandled)this.selectionSource="selection";
    return result!==SelectionInputResult.Unhandled;
  }

  setActiveTool(toolID: import("../state/workbench-store").WorkbenchToolID): void {
    const generation=++this.toolActivationGeneration;
    if(this.moveFinalization||this.moveCommitPending||this.moveInteraction.hasUncommittedFinal){
      void this.settleAssemblyInteraction().then(()=>{
        if(!this.disposed&&generation===this.toolActivationGeneration)this.tools.activate(toolID);
      }).catch(error=>{if(!this.disposed&&generation===this.toolActivationGeneration)this.callbacks.operationFailed?.(error);});
      return;
    }
    this.tools.activate(toolID);
  }

  beginExternalReconnect(externalID: string): void {
    this.reconnectExternalID = externalID;
    this.tools.activate("sketch.project");
  }

  setCatiaRotationSphereVisible(visible: boolean): void {
    this.navigation.setCatiaRotationSphereVisible(visible);
  }

  setNavigationProfile(profile: NavigationProfileID): void {
    this.navigation.setProfile(profile);
  }

  setCaptureSettings(settings: CaptureSettings): void {
    this.captureSettings = settings;
    this.preselect(null, true);
    this.clearSnapPreview();
  }

  fit(): void {
    this.frameContent();
  }

  private previewInputMaterials:Array<{object:THREE.Mesh;original:THREE.Material|THREE.Material[];temporary:THREE.Material[]}>=[];
  private previewVisualGeneration = 0;
  private previewVisuals?: VisualRepository;
  private previewSketchHidden:THREE.Object3D[]=[];
  previewArtifact(descriptor: ArtifactDescriptor, operation: FeaturePreviewOperation = "NEW_BODY", sketchFeatureId?:string): void {
    this.clearCommandPreview();
    const generation = this.previewVisualGeneration;
    if (descriptor.representationKind !== "TRANSIENT_PREVIEW" || !this.view) return;
    const view = { ...this.view, artifact: descriptor, artifacts: {} };
    this.previewVisuals = this.visuals.previewRepository();
    void this.previewVisuals.hydrate(view).then(display => {
      if (generation !== this.previewVisualGeneration || this.disposed || !display.artifact) return;
      if(sketchFeatureId!==undefined)this.showSketchPatternPreview(display.artifact,sketchFeatureId);
      else this.showPreviewArtifact(display.artifact, operation);
    }).catch(error => {
      if (generation !== this.previewVisualGeneration || this.disposed) return;
      this.callbacks.toolPromptChanged(`预览显示加载失败：${error instanceof Error ? error.message : String(error)}`);
    });
  }

  private showSketchPatternPreview(artifact:Artifact,featureId:string):void {
    const existing=new Set(this.sketchView()?.part?.features.map(feature=>feature.id)??[]);
    const group=new THREE.Group();
    for(const primitive of artifact.visualization?.primitives??[]) {
      if(primitive.entityType==="REFERENCE_POINT")continue;
      if(!(featureId?(primitive.patternId??primitive.featureId)===featureId:!existing.has(primitive.patternId??primitive.featureId))||!["SKETCH_CURVE","SKETCH_POINT"].includes(primitive.semantic))continue;
      const geometry=new THREE.BufferGeometry().setFromPoints(primitive.positions.map(position=>new THREE.Vector3().fromArray(position)));
      const object=primitive.kind==="POINTS"?new THREE.Points(geometry,new THREE.PointsMaterial({color:CATIA_VISUAL_THEME.selected,size:5,depthTest:false})):new THREE.Line(geometry,new THREE.LineBasicMaterial({color:CATIA_VISUAL_THEME.selected,depthTest:false}));
      group.add(object);
    }
    if(this.editContext?.translation)group.position.fromArray(this.editContext.translation);
    if(this.editContext?.rotation)group.quaternion.fromArray(this.editContext.rotation);
    for(const root of [this.helpers,this.content])root.traverse(object=>{
      if(!object.visible)return;
      const data=object.userData;
      if(data.patternId!==featureId&&data.featureId!==featureId&&data.sketchFeatureID!==featureId)return;
      if(this.activeSketchID===featureId&&!data.patternMember)return;
      if((data.occurrencePath??"")!==(this.editContext?.occurrencePath??""))return;
      this.previewSketchHidden.push(object);object.visible=false;
    });
    this.scene.add(group);this.commandPreview=group;this.updateCameraClipping();this.invalidate();
  }

  private showPreviewArtifact(artifact: Artifact, operation: FeaturePreviewOperation): void {
    if (!artifact.mesh.vertices.length || !artifact.mesh.triangles.length) return;
    const binding = this.solidBindings.get(`${this.editContext?.occurrencePath || "root"}/body:${artifact.bodyId}`);
    const geometry = makeGeometry(artifact);
    const group = makeFeaturePreview(geometry, operation, binding?.mesh.geometry.clone());
    if (binding) {
      this.previewBody = { group: binding.group, visible: binding.group.visible };
      binding.group.visible = !!this.featureSelection;
      if(this.featureSelection)binding.group.traverse(child=>{
        const object=child as THREE.Mesh;
        if(!object.material)return;
        const original=object.material;
        const temporary=(Array.isArray(original)?original:[original]).map(material=>{
          const copy=material.clone();copy.visible=false;return copy;
        });
        object.material=Array.isArray(original)?temporary:temporary[0];
        this.previewInputMaterials.push({object,original,temporary});
      });
    }
    if (this.editContext?.translation) group.position.fromArray(this.editContext.translation);
    if (this.editContext?.rotation) group.quaternion.fromArray(this.editContext.rotation);
    this.scene.add(group); this.commandPreview = group;
    this.host.dataset.featurePreview = operation;
    this.updateCameraClipping(); this.invalidate();
  }

  clearCommandPreview(restore = true): void {
    this.previewVisualGeneration++;
    this.previewVisuals?.dispose();
    this.previewVisuals = undefined;
    delete this.host.dataset.featurePreview;
    for(const {object,original,temporary} of this.previewInputMaterials){object.material=original;for(const material of temporary)material.dispose();}
    this.previewInputMaterials=[];
    for(const object of this.previewSketchHidden)object.visible=true;
    this.previewSketchHidden=[];
    if (this.previewBody) {
      this.previewBody.group.visible = this.previewBody.visible;
      this.previewBody = undefined;
    }
    if (this.commandPreview) {
      this.scene.remove(this.commandPreview); this.disposeRenderable(this.commandPreview);
      this.commandPreview = undefined;
    }
    if (this.assemblyPosePreview) {
      const targets = [];
      for (const [id, pose] of this.assemblyPosePreview) {
        const group = this.instanceGroups.get(id);
        if (!group) continue;
        if (restore) targets.push({ object: group, target: this.transformPose(pose.position, pose.rotation),
          frame: () => this.syncAttachedManipulatorPosition(group) });
        else this.transforms.stop(group);
      }
      if (restore) this.transforms.applyBatch(targets, "rollback");
      this.assemblyPosePreview = undefined;
    }
    this.invalidate();
  }

  clearInsertPatternPreview(): void {
    const preview=this.insertPatternPreview;
    if (!preview) return;
    this.scene.remove(preview);
    preview.traverse((object) => {
      const renderable=object as THREE.Mesh;
      const materials=Array.isArray(renderable.material)?renderable.material:[renderable.material];
      materials.forEach((material) => material?.dispose()); // Geometry and textures belong to the live instance.
    });
    this.insertPatternPreview=undefined;
    this.invalidate();
  }

  previewInsertPattern(input?: InstancePatternPreview): void {
    this.clearInsertPatternPreview();
    if (!input) return;
    let source: THREE.Group | undefined;
    if (input.parentOccurrencePath) {
      const prefix = `${input.parentOccurrencePath}/${input.sourceInstanceId}`;
      const assembly = new THREE.Group();
      this.content.updateMatrixWorld(true);
      for (const root of this.instanceGroups.values()) for (const child of root.children) {
        const path = child.userData.occurrencePath as string | undefined;
        if (path !== prefix && !path?.startsWith(`${prefix}/`)) continue;
        const copy = child.clone(true);
        child.matrixWorld.decompose(copy.position, copy.quaternion, copy.scale);
        assembly.add(copy);
      }
      if (assembly.children.length > 0) source = assembly;
    } else source = this.instanceGroups.get(input.sourceInstanceId);
    if (!source) return;
    const offsets=instancePatternOffsets(input);
    if (offsets.length===0) return;
    const preview=new THREE.Group();
    preview.name="instance-pattern-preview";
    for (const offset of offsets) {
      const ghost=source.clone(true);
      const displacement = new THREE.Vector3().fromArray(offset);
      if (input.parentRotation) displacement.applyQuaternion(new THREE.Quaternion().fromArray(input.parentRotation));
      ghost.position.add(displacement);
      ghost.traverse((object) => {
        object.userData={};
        const renderable=object as THREE.Mesh;
        if (!renderable.material) return;
        const decorate=(material:THREE.Material) => {
          const copy=material.clone();copy.transparent=true;copy.opacity=.38;copy.depthWrite=false;
          return copy;
        };
        renderable.material=Array.isArray(renderable.material)?renderable.material.map(decorate):decorate(renderable.material);
      });
      preview.add(ghost);
    }
    this.scene.add(preview);
    this.insertPatternPreview=preview;
    this.invalidate();
  }

  previewAssemblyPoses(poses: Array<{instanceId:string;translation:Vec3;rotation:[number,number,number,number]}>): void {
    if (!this.assemblyPosePreview) {
      this.assemblyPosePreview = new Map([...this.instanceGroups].map(([id, group]) =>
        [id, { position: group.position.clone(), rotation: group.quaternion.clone() }]));
    }
    const targets = [];
    for (const pose of poses) {
      const group = this.instanceGroups.get(pose.instanceId);
      if (!group) continue;
      targets.push({ object: group, target: this.transformPose(pose.translation, pose.rotation),
        frame: () => this.syncAttachedManipulatorPosition(group) });
    }
    this.transforms.applyBatch(targets, "settle");
  }

  setStandardView(view: "TOP" | "FRONT" | "RIGHT" | "ISO"): void {
    this.viewTransition.cancel();
    this.navigation.cancel();
    standardView(this.camera, this.navigation.target, view);
    if (view === "ISO") this.frameContent();
    else this.navigation.syncCamera(false);
  }

  select(selection: Selection, notify = true): void {
    this.selectMany(selection ? [selection] : [], notify);
  }

  requestDimensionEdit(selection: Extract<SelectionItem, { kind: "sketch-constraint" }>, x?: number, y?: number): boolean {
    const sketchView = this.sketchView();
    if (!sketchView) return false;
    if ((selection.ownerDocumentId??selection.documentId)!==sketchView.document.id ||
        (selection.occurrencePath??"")!==(this.editContext?.occurrencePath??"") ||
        selection.versionId&&selection.versionId!==sketchView.document.versionId ||
        this.activeSketchID&&selection.featureId!==this.activeSketchID) {
      this.callbacks.toolPromptChanged("尺寸选择不属于当前文档、草图或 occurrence，请重新选择");return false;
    }
    const feature = sketchView.part?.features.find((candidate) => candidate.id === selection.featureId);
    const constraint = feature?.sketch?.constraints.find((candidate) => candidate.id === selection.constraintId);
    if(!constraint)return false;
    if(constraint.internal||["MIRROR","SAME_SUPPORT"].includes(constraint.kind)){this.callbacks.toolPromptChanged("生成操作的内部关系须通过对应几何编辑显式解除，不能直接改写");return false;}
    this.selectMany([selection]);
    this.callbacks.dimensionEditRequested({ mode: "edit", featureId: selection.featureId, constraintId: constraint.id,
      value: constraint.value, unit: constraint.unit==="deg"?"deg":"mm", x: x ?? this.renderer.domElement.clientWidth / 2,
      y: y ?? this.renderer.domElement.clientHeight / 2 });
    return true;
  }

  selectMany(selections: readonly SelectionItem[], notify = true, source:import("../cad/input/input-types").SelectionInputSource="selection"): void {
    if(notify)this.selectionSource=source;
    const projected=selections.map(selection=>this.view?refreshOccurrenceSelection(this.view,selection):selection);
    const unique = [...new Map(projected.map((selection) => [selectionKey(selection), selection])).values()];
    if (sameSelections(this.selected, unique) && !this.preselected) {
      if (notify) this.callbacks.selectionsChanged(unique);
      if(this.activeToolID==="assembly.move"&&(!this.moveManipulator.isAttached()||this.pendingManipulatorAnchor))this.attachMoveManipulator();
      return;
    }
    if(!sameSelections(this.selected,unique)){
      const motion=this.motionPresentation,path=motion?[motion.ownerOccurrence,motion.bodyId].filter(Boolean).join("/"):undefined;
      if(path&&!unique.some(s=>s.occurrencePath===path||s.occurrencePath?.startsWith(`${path}/`)))this.showRemainingMotion();
      if(!this.moveFinalization&&!this.moveInteraction.hasUncommittedFinal&&!this.moveCommitPending)this.cancelMovePreviewGesture("操纵选择已变化");
    }
    this.selected = unique;
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.preselected = null;
    this.moveManipulator.detach();
    this.refreshInteractionHighlights();
    if (this.activeToolID === "assembly.move" && unique.length === 1 && unique[0].kind === "instance") {
      this.attachMoveManipulator();
    }
    if (notify) this.callbacks.selectionsChanged(unique);
    this.emitDebugState();
    this.invalidate();
  }

  private attachMoveManipulator(): void {
    if(this.moveFinalization||this.moveInteraction.hasUncommittedFinal||this.moveCommitPending)return;
    if (this.selected.length !== 1 || this.selected[0].kind !== "instance") return;
    const object = this.selectable.get(`instance:${this.selected[0].instanceId ?? this.selected[0].id}`);
    if (!(object instanceof THREE.Group)) return;
    const instanceId = this.selected[0].instanceId ?? this.selected[0].id;
    const storedLocalPivot = this.manipulatorPivots.get(instanceId);
    const picked=this.pendingManipulatorAnchor?.instanceId===instanceId?this.pendingManipulatorAnchor.anchor:undefined;
    this.pendingManipulatorAnchor=undefined;
    const center = picked?.position??(storedLocalPivot ? object.localToWorld(storedLocalPivot.clone())
      : new THREE.Box3().setFromObject(object).getCenter(new THREE.Vector3()));
    const localPivot = object.worldToLocal(center.clone());
    this.manipulatorPivots.set(instanceId, localPivot.clone());
    const objectRotation=object.getWorldQuaternion(new THREE.Quaternion());
    const orientation=picked?.orientation??objectRotation.clone().multiply(this.manipulatorFrames.get(instanceId)??new THREE.Quaternion());
    this.manipulatorFrames.set(instanceId,objectRotation.clone().invert().multiply(orientation));
    this.moveManipulator.attach(center,orientation);
    this.moveTarget = { group: object, startPosition: object.position.clone(), startQuaternion: object.quaternion.clone(),
      startPivot: center.clone(), localPivot };
    this.desiredMovePose = { translation: object.position.toArray(), rotation: object.quaternion.toArray() };
    this.acceptedMovePose = { translation: object.position.toArray(), rotation: object.quaternion.toArray() };
  }

  private updateManipulatorPivot(anchor:ManipulatorAnchor): void {
    const target = this.moveTarget;
    if (!target) return;
    const position=anchor.position;
    target.startPivot.copy(position);
    target.localPivot.copy(target.group.worldToLocal(position.clone()));
    const instanceId = target.group.userData.id as string | undefined;
    if (instanceId){
      this.manipulatorPivots.set(instanceId, target.localPivot.clone());
      if(anchor.orientation)this.manipulatorFrames.set(instanceId,target.group.getWorldQuaternion(new THREE.Quaternion()).invert().multiply(anchor.orientation));
    }
    this.invalidate();
  }

  private snapManipulatorPivot(x: number, y: number): ManipulatorAnchor | undefined {
    if (this.pendingVisualSnapshot) return undefined;
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const worldPerPixel = worldUnitsPerCssPixel(this.camera, this.moveManipulator.object.getWorldPosition(new THREE.Vector3()),
      viewportMetrics(this.renderer));
    this.raycaster.params.Line = { threshold: worldPerPixel * 8 };
    this.raycaster.params.Points = { threshold: worldPerPixel * 11 };
    this.datumAxisPickToleranceWorld = worldPerPixel * 1.75;
    const hit=this.selectionIndex.pickWithIntersection(this.raycaster,selection=>Boolean(assemblyGeometryRef(selection)));
    if(!hit.selection||!hit.intersection){this.snapLock=undefined;return;}
    return this.manipulatorAnchorFromIntersection(hit.intersection,hit.selection);
  }

  private manipulatorAnchorFromIntersection(hit:THREE.Intersection,selection?:SelectionItem):ManipulatorAnchor{
    // Mesh is permitted as approximate pointer grab position, never as the
    // authority for a persistent/exact geometric direction objective.
    const fallback={position:hit.point.clone()};
    if(!selection||!this.view||!this.callbacks.inspectAssemblySupports)return fallback;
    const current=refreshOccurrenceSelection(this.view,selection),reference=assemblyGeometryRef(current);
    if(!reference)return fallback;
    const view=this.view,key=JSON.stringify([view.document.id,view.document.versionId,reference]);
    this.snapInput={key,hit};
    const controlled=this.moveTarget?.group.userData.id??current.instanceId;
    const frame=this.moveManipulator.frameQuaternion();
    const choose=(inspection:SupportInspection):ManipulatorAnchor=>{
      const support=inspection.supports[0],motionId=reference.instancePath?.segments[0]?.instanceId??reference.instanceId;
      const group=this.instanceGroups.get(motionId);
      if(inspection.versionId!==view.document.versionId||support?.status!=="RESOLVED"||!support.descriptor||!group)return fallback;
      group.updateWorldMatrix(true,false);
      const point=this.snapInput?.key===key?this.snapInput.hit.point:hit.point;
      const candidates=exactSnapCandidates(support.descriptor,support.snapHints,group.matrixWorld,point,frame);
      const pixels=(position:THREE.Vector3)=>{
        const a=position.clone().project(this.camera),b=point.clone().project(this.camera),metrics=viewportMetrics(this.renderer);
        return Math.hypot((a.x-b.x)*metrics.cssWidth/2,(a.y-b.y)*metrics.cssHeight/2);
      };
      const locked=this.snapLock?.key===key?this.snapLock:undefined;
      if(locked&&pixels(locked.anchor.position)<22)return locked.anchor;
      candidates.sort((a,b)=>pixels(a.position)-pixels(b.position));
      const candidate=candidates.find(c=>c.role!=="surface"&&pixels(c.position)<14)??
        (support.descriptor.Kind==="CIRCLE"?candidates.find(c=>c.role==="center"):undefined)??candidates.find(c=>c.role==="surface");
      const anchor=candidate??{...fallback,orientation:candidates[0]?.orientation};
      this.snapLock={key,anchor};return anchor;
    };
    const cached=this.manipulatorSnapCache.get(key);
    if(cached)return choose(cached);
    this.manipulatorSnapCache.request(key,signal=>this.callbacks.inspectAssemblySupports!(view.document.id,[reference],signal),inspection=>{
      if(this.snapInput?.key!==key||this.activeToolID!=="assembly.move"||this.view?.document.versionId!==view.document.versionId||this.view.document.id!==view.document.id||
        this.moveTarget?.group.userData.id!==controlled||this.moveManipulator.isDragging()&&!this.moveManipulator.isPivotDragging())return;
      const anchor=choose(inspection);
      this.moveManipulator.attach(anchor.position,anchor.orientation??frame);this.updateManipulatorPivot(anchor);
    });
    return fallback;
  }
  setAssemblyMoveDirection(instanceId:string,direction:Vec3,kind:"line"|"plane"):boolean{
    if(this.moveCommitPending||this.moveManipulator.isDragging())return false;
    const group=this.instanceGroups.get(instanceId);if(!group)return false;
    const axis=new THREE.Vector3(...direction);
    if(!Number.isFinite(axis.lengthSq())||axis.lengthSq()<1e-20)return false;
    axis.normalize().applyQuaternion(group.quaternion);
    this.manipulatorFrames.set(instanceId,group.getWorldQuaternion(new THREE.Quaternion()).invert().multiply(manipulatorFrame(axis,kind)));
    this.select({kind:"instance",id:instanceId,instanceId},true);
    this.setActiveTool("assembly.move");this.attachMoveManipulator();this.invalidate();return true;
  }
  cancelAssemblyInteraction():void{this.cancelMovePreviewGesture("用户取消操纵");}
  retryAssemblyMoveCommit():void{if(this.moveCommitPending)void this.submitMoveCommit();else if(this.moveInteraction.hasUncommittedFinal)this.finishMovePreviewGesture(true);}

  private updateMoveTarget(): void {
    if (!this.moveTarget || !this.moveManipulator.isAttached() || !this.moveManipulator.isDragging()) return;
    const pivot = this.moveManipulator.candidatePose();
    const transformed=transformAroundWorldPivot(this.moveTarget.startPosition,this.moveTarget.startQuaternion,
      this.moveTarget.startPivot,pivot.position,pivot.rotation);
    const position=transformed.position;
    const rotation=transformed.rotation.toArray();
    this.desiredMovePose={translation:position.toArray(),rotation};
    const components=this.moveManipulator.components(),frame=this.moveFrameRotation;
    if(components&&frame)this.moveInteraction.target({localGrabPoint:this.moveTarget.localPivot.toArray(),targetPose:this.desiredMovePose,frameRotation:frame,...components});
  }

  private beginMovePreviewGesture():void{
    this.manipulatorSnapCache?.cancelPending();
    if(this.moveCommitPending)return;
    this.showRemainingMotion();
    const target=this.moveTarget;
    if(target&&this.view){
      const instanceId=target.group.userData.id as string;
      const path=occurrenceSnapshot(this.view,instanceId);
      if(!path||!this.view.product?.instances.some(instance=>instance.id===instanceId)){
        this.cancelMovePreviewGesture("操纵对象已变化，请重新选择组件");this.moveManipulator.detach();return;
      }
      if(this.moveNominalScene)this.cancelMovePreviewGesture("开始新的操纵，先恢复未提交的权威基线");
      this.moveGestureGeneration++;
      this.moveNominalBase={documentId:this.view.document.id,revisionId:this.view.document.versionId};
      this.moveNominalScene=new Map([...this.instanceGroups].map(([id,group])=>[id,snapshotTransform(group)]));
      this.transforms.stop(target.group);
      target.startPosition.copy(target.group.position);
      target.startQuaternion.copy(target.group.quaternion);
      target.startPivot.copy(this.moveManipulator.object.getWorldPosition(new THREE.Vector3()));
      target.localPivot.copy(target.group.worldToLocal(target.startPivot.clone()));
      this.acceptedMovePose={translation:target.startPosition.toArray(),rotation:target.startQuaternion.toArray()};
      this.moveFrameRotation=this.moveManipulator.frameQuaternion().toArray();
      this.moveSessionDocumentId=this.view.document.id;
      this.moveInteraction.begin({baseRevisionId:this.view.document.versionId,instanceId,
        occurrencePath:path,localGrabPoint:target.localPivot.toArray(),frameRotation:this.moveFrameRotation});
    }
  }
  private cancelMovePreviewGesture(reason="cancelled"):void{
    this.manipulatorSnapCache?.cancelPending();this.snapInput=undefined;
    if(this.moveInteraction.state==="committing")return; // receipt recovery owns a submitted command
    this.moveGestureGeneration++;
    this.moveInteraction.cancel(reason);
    this.restoreMoveNominalScene();
  }
  private restoreMoveNominalScene():void{
    if(this.moveNominalBase&&(this.pendingVisualSnapshot||this.view?.document.id!==this.moveNominalBase.documentId||this.view.document.versionId!==this.moveNominalBase.revisionId)){
      // An external/current Head is authoritative. Never rollback an old
      // session baseline over a newer rendered revision or incoming snapshot.
      this.moveNominalScene=undefined;this.moveNominalBase=undefined;return;
    }
    if(this.moveNominalScene){
      const batch=[...this.moveNominalScene].flatMap(([id,pose])=>{const object=this.instanceGroups.get(id);return object?[{object,target:pose}]:[];});
      this.transforms.applyBatch(batch,"immediate");this.moveNominalScene=undefined;this.moveNominalBase=undefined;
      if(this.moveTarget)this.syncMoveManipulatorToRenderedPose(this.moveTarget);
      if(this.view)this.addAssemblyConstraintMarkers(this.view);
      this.refreshContentBounds();this.invalidate();
    }
  }
  private finishMovePreviewGesture(retry=false):void{
    if(this.moveFinalization||this.moveCommitPending)return;
    const documentId=this.moveSessionDocumentId;
    const generation=this.moveGestureGeneration;
    const operation=(retry?this.moveInteraction.retryFinal():this.moveInteraction.finish()).then(async candidate=>{
      if(generation!==this.moveGestureGeneration)return;
      if(!candidate){
        if(this.moveInteraction.hasUncommittedFinal&&this.moveInteraction.state==="blocked"){
          this.callbacks.assemblyInteractionState?.("blocked","移动尚未保存；请重试确认当前目标或取消。");return;
        }
        // Budget/Failed/invalidated never commit an older frame, and never
        // become the next gesture's nominal baseline.
        const state=this.moveInteraction.state;
        this.moveInteraction.cancel(`${this.moveInteraction.reason??state}；最终目标没有合格候选，已恢复未提交的权威基线`,state);
        this.restoreMoveNominalScene();return;
      }
      if(candidate.unchanged){this.moveInteraction.committed();this.moveNominalScene=undefined;return;}
      this.moveInteraction.committing();
      this.moveCommitPending={documentId,candidate};
      await this.submitMoveCommit();
    });
    this.moveFinalization=operation;
    void operation.finally(()=>{if(this.moveFinalization===operation)this.moveFinalization=undefined;});
  }
  private async submitMoveCommit():Promise<void>{
      const pending=this.moveCommitPending;if(!pending||this.moveCommitInFlight)return;
      this.moveCommitInFlight=true;
      try{
        await this.callbacks.instanceMoved(pending.documentId,pending.candidate);
        if(this.moveCommitPending!==pending)return;
        this.moveCommitPending=undefined;
        this.moveInteraction.committed();this.moveNominalScene=undefined;
        const target=this.moveTarget;
        if(target){target.startPosition.copy(target.group.position);target.startQuaternion.copy(target.group.quaternion);
          target.startPivot.copy(target.group.localToWorld(target.localPivot.clone()));
          this.moveManipulator.commitPreviewFrame();this.moveManipulator.setAuthoritativePose(target.startPivot);}
      }catch(error){
        const code=(error as {code?:string})?.code;
        if(!code||["CONNECTION_CLOSED","TIMEOUT","REALTIME_UNAVAILABLE","REALTIME_BUSY","DATABASE_BUSY"].includes(code)){
          // Submission may already have committed. Keep exact request/token for
          // receipt recovery; neither restore nominal nor generate a new MOVE.
          this.callbacks.assemblyInteractionState?.("committing",`提交结果未知；可重试同一请求回执。${String(error)}`);
        }else{
          this.callbacks.operationFailed?.(error);
          this.moveCommitPending=undefined;
          const state=assemblyInteractionFailureState(error);
          this.moveInteraction.cancel(String(error),state);
          this.restoreMoveNominalScene();
        }
      }finally{this.moveCommitInFlight=false;}
  }
  private applyAcceptedMoveFrame(frame:AssemblyInteractionFrame):void{
    const target=this.moveTarget;
    const batch=frame.instancePoses.flatMap(pose=>{const object=this.instanceGroups.get(pose.instanceId);
      return object?[{object,target:this.transformPose(pose.translation,pose.rotation)}]:[];});
    // Authoritative assembly states are atomic snapshots. Independent pose
    // interpolation can leave the hard manifold even if both endpoints satisfy it.
    this.transforms.applyBatch(batch,"immediate");
    if(target){
      const driven=frame.instancePoses.find(p=>p.instanceId===target.group.userData.id);
      if(driven)this.acceptedMovePose={translation:driven.translation,rotation:driven.rotation,previewId:frame.previewId};
      this.syncMoveManipulatorToRenderedPose(target);
    }
    this.content.updateMatrixWorld(true);
    if(this.view)this.addAssemblyConstraintMarkers(this.view);
    this.refreshContentBounds();this.invalidate();
  }

  preselect(selection: Selection, notify = false): void {
    if (sameSelection(this.preselected, selection)) return;
    this.preselected = selection;
    this.refreshInteractionHighlights();
    if (notify) this.callbacks.preselectionChanged(selection);
    this.invalidate();
  }

  private refreshInteractionHighlights(): void {
    if(this.featureSelection?.localSketchId){
      const retained=this.featureSelection.selections.flatMap(s=>s.sketchReference?[s.sketchReference]:[]);
      const hovered=this.preselected?.sketchReference;
      if(hovered??retained[0])this.showReferencePreview(hovered??retained[0],retained,undefined,!!hovered);else this.clearReferenceHover();
    }
    for (const object of this.highlightedRoots) this.applyHighlight(object, "default");
    this.highlightedRoots.clear();
    const withAssemblyReferences = (selections: readonly SelectionItem[]) => selections.filter(s=>!this.featureSelection?.localSketchId||!s.sketchReference).flatMap((selection) =>
      selection.kind === "assembly-constraint" ? [selection, ...(this.assemblyConstraintReferences.get(this.assemblyMarkerKey(selection.documentId ?? "",selection.occurrencePath ?? "",selection.constraintId)) ?? [])]
        : selection.kind === "publication" && selection.highlightTarget ? [selection.highlightTarget] : [selection]);
    for (const object of this.selectionIndex.objectsForMany(this.featureSelection?.contextSelections ?? [])) {
      if (!this.objectVisible(object)) continue;
      this.applyHighlight(object, "context"); this.highlightedRoots.add(object);
    }
    this.replaceTopologyOverlays("selected", withAssemblyReferences(this.featureSelection?.selections ?? this.selected));
    for (const object of this.selectionIndex.objectsForMany(withAssemblyReferences(this.featureSelection?.selections ?? this.selected))) {
      if (!this.objectVisible(object)) continue;
      this.applyHighlight(object, "selected"); this.highlightedRoots.add(object);
    }
    this.replaceTopologyOverlays("preselected", withAssemblyReferences(this.preselected ? [this.preselected] : []));
    if (this.preselected && !(this.featureSelection?.localSketchId&&this.preselected.sketchReference)) {
      for (const object of this.selectionIndex.objectsFor(this.preselected)) {
        if (!this.objectVisible(object)) continue;
        this.applyHighlight(object, "hover"); this.highlightedRoots.add(object);
      }
    }
  }

  private clearInteractionState(): void {
    this.clearDimensionDefinitionPreview(false);
	for (const object of this.highlightedRoots) this.applyHighlight(object, "default");
	this.highlightedRoots.clear();
	this.replaceTopologyOverlays("preselected", []);
	this.replaceTopologyOverlays("selected", []);
	this.clearReferencePreview();
	this.selected = [];
	this.preselected = null;
  }

  dispose(): void {
    disposeMotionMarkers(this.motionMarkers);
    this.cancelMovePreviewGesture("viewport interaction reset");
    this.clearSketchMarquee();this.clearDimensionDefinitionPreview(false);
    this.visualGeneration++;
    this.visuals.dispose();
    this.visualError?.remove();
    this.clearInsertPatternPreview();
    if (this.disposed) return;
    this.disposed = true;
    this.viewTransition.cancel();
    for (const event of ["pointerdown", "wheel", "keydown"])
      this.host.removeEventListener(event, this.interruptViewTransition, true);
    cancelAnimationFrame(this.animationFrame);
    this.resizeObserver.disconnect();
    this.input.dispose();
    this.clearPreview();
    this.clearReferencePreview();
    this.clearSnapPreview();
    this.clearCommandPreview(false);
    this.transforms.stopAll();
    this.moveManipulator.dispose();
    this.sketchManipulator?.dispose();
    this.navigationHUD.dispose();
    this.background.dispose();
    this.groundGrid.dispose();
    this.sketchGrid.dispose();
    this.disposeGroup(this.environment);
    this.disposeGroup(this.lighting);
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.disposeGroup(this.sketchContext);
    this.scene.environment = null;
    this.studioEnvironment.dispose();
    this.renderer.dispose();
    this.renderer.domElement.remove();
  }

  private transformPose(position: Vec3 | THREE.Vector3, rotation: [number, number, number, number] | THREE.Quaternion,
    scale = new THREE.Vector3(1, 1, 1)): TransformPose {
    return {
      position: position instanceof THREE.Vector3 ? position.clone() : new THREE.Vector3().fromArray(position),
      rotation: rotation instanceof THREE.Quaternion ? rotation.clone().normalize()
        : new THREE.Quaternion().fromArray(rotation).normalize(),
      scale: scale.clone(),
    };
  }

  private syncAttachedManipulatorPosition(group: THREE.Group): void {
    const target = this.moveTarget;
    if (!target || target.group !== group || !this.moveManipulator.isAttached() || this.moveManipulator.isDragging()) return;
    this.moveManipulator.setAuthoritativePose(group.localToWorld(target.localPivot.clone()));
  }

  private syncMoveManipulatorToRenderedPose(target: { group: THREE.Group; startPosition: THREE.Vector3;
    startQuaternion: THREE.Quaternion; startPivot: THREE.Vector3; localPivot: THREE.Vector3 }): void {
    if (this.moveTarget !== target || !this.moveManipulator.isAttached()) return;
    const gestureDelta = target.group.quaternion.clone().multiply(target.startQuaternion.clone().invert()).normalize();
    const orientation = gestureDelta.multiply(this.moveManipulator.frameQuaternion()).normalize();
    this.moveManipulator.setPreviewPose(target.group.localToWorld(target.localPivot.clone()), orientation);
  }

  private renderPart(view: DocumentView): void {
    const rootPath = `document:${view.document.id}`;
    for (const datum of view.datumPlanes ?? []) this.addDatumPlane(datum, this.helpers, true, {
      documentId: view.document.id, versionId: view.document.versionId,
      geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/plane:${datum.id}`,
    });
    for (const axis of view.axisSystems ?? []) this.addAxisSystem(axis, this.helpers, {
      documentId: view.document.id, versionId: view.document.versionId,
      geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/axis:${axis.id}`,
    });
    for (const axis of view.datumAxes ?? view.part?.datumAxes ?? []) this.addDatumAxis(axis, this.helpers, {
      documentId: view.document.id, versionId: view.document.versionId,
      geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/datum-axis:${axis.id}`,
    });
    for (const body of view.part?.bodies ?? []) {
      if (body.consumed) continue;
      const displayKey = body.geometryKey || body.displayFallback?.geometryKey;
      const artifact = displayKey ? view.artifacts?.[displayKey] : undefined;
      if (!artifact) continue;
      const bodyTreeNodeId = `${rootPath}/body:${body.id}`;
      const context: SolidContext = { bodyId:body.id, documentId:view.document.id,versionId:view.document.versionId,
        geometryKey:artifact.geometryKey,occurrencePath:"",treeNodeId:bodyTreeNodeId,displayStageFeatureId:artifact.displayStageFeatureId };
      if (body.geometryKey) this.addVisualPrimitives(artifact.visualization, this.helpers, context, true,
        new Set((view.part?.features??[]).filter(feature=>feature.type==="SKETCH_PATTERN"&&feature.bodyId===body.id&&!feature.suppressed).map(feature=>feature.id)));
      if (artifact.mesh.triangles.length) {
        const solid=body.geometryKey ? this.makeSolid(artifact,CATIA_VISUAL_THEME.surface,context) : this.makeFailedBody(artifact,body.visible,context);
        this.content.add(solid);
      }
    }
    const consumedSketches = new Set((view.part?.features ?? []).flatMap((feature) => feature.profile ? [feature.profile] : []));
    for (const feature of view.part?.features ?? []) {
      if (feature.sketch) {
        this.addSketch(feature, false, view, undefined, undefined, undefined, !consumedSketches.has(feature.id));
      }
    }
  }

  private renderProduct(view: DocumentView): void {
    const rootName = view.document.name;
    for (const instance of view.product?.instances ?? []) {
      const group = new THREE.Group();
      group.position.fromArray(instance.translation);
      const instanceRotation = new THREE.Quaternion().fromArray(instance.rotation ?? [0, 0, 0, 1]);
      group.quaternion.copy(instanceRotation);
      const instanceSelection = {
        kind: "instance" as const, id: instance.id,
        treeNodeId: `document:${view.document.id}/instance:${instance.id}`, documentId: instance.documentId,
        versionId: instance.versionId, occurrencePath: instance.id, instanceId: instance.id,
        instancePath: view.resolvedInstances?.find((resolved) => resolved.occurrencePath === instance.id)?.instancePath ??
          view.structureTree?.children?.find((node) => node.kind === "INSTANCE" && node.entityId === instance.id)?.instancePath,
        contextVariantKey: view.contextVariants?.find((variant) => variant.owningInstancePath.canonical === instance.id)?.variantKey,
      };
      group.userData = instanceSelection;
      this.content.add(group);
      this.instanceGroups.set(instance.id, group);
      this.selectable.set(`instance:${instance.id}`, group);
      this.selectionIndex.register(instanceSelection, group);
      const prefix = `${rootName}/${instance.id}`;
      const referencedOccurrences = new Set<string>();
      for (const resolved of view.resolvedInstances ?? []) {
        if (!resolved.id.startsWith(prefix)) continue;
        const artifact = view.artifacts?.[resolved.geometryKey || resolved.displayFallback?.geometryKey || ""];
        if (!artifact) continue;
        const resolvedGroup = new THREE.Group();
        resolvedGroup.userData = { occurrencePath: resolved.occurrencePath };
        resolvedGroup.position.fromArray(resolved.translation).sub(new THREE.Vector3().fromArray(instance.translation))
          .applyQuaternion(instanceRotation.clone().invert());
        const resolvedRotation = new THREE.Quaternion().fromArray(resolved.rotation ?? [0, 0, 0, 1]);
        resolvedGroup.quaternion.copy(instanceRotation.clone().invert().multiply(resolvedRotation));
        if (artifact.mesh.triangles.length > 0) {
          const context: SolidContext = {
            bodyId:resolved.bodyId, documentId: resolved.documentId, versionId: resolved.instancePath.segments.at(-1)?.resolvedVersionId, geometryKey: artifact.geometryKey,
            contextVariantKey: view.contextVariants?.find((variant) => variant.owningInstancePath.canonical === resolved.occurrencePath)?.variantKey,
            instancePath: resolved.instancePath, occurrencePath: resolved.occurrencePath, treeNodeId: resolved.bodyTreeNodeId, instanceId: instance.id
          };
          const solid = resolved.geometryKey ? this.makeSolid(artifact, CATIA_VISUAL_THEME.productSurface, context) : this.makeFailedBody(artifact,resolved.bodyVisible,context);
          resolvedGroup.add(solid);
        }
        const visualContext = {
          bodyId:resolved.bodyId, documentId: resolved.documentId, versionId: resolved.instancePath.segments.at(-1)?.resolvedVersionId,
          contextVariantKey: view.contextVariants?.find((variant) => variant.owningInstancePath.canonical === resolved.occurrencePath)?.variantKey,
          geometryKey: artifact.geometryKey, occurrencePath: resolved.occurrencePath,
          instancePath: resolved.instancePath, treeNodeId: resolved.bodyTreeNodeId, instanceId: instance.id,
        };
        if (resolved.geometryKey && !referencedOccurrences.has(resolved.occurrencePath)) { this.addReferenceGeometry(artifact.visualization.referenceGeometry, resolvedGroup, visualContext); referencedOccurrences.add(resolved.occurrencePath); }
        if (resolved.geometryKey) this.addVisualPrimitives(artifact.visualization, resolvedGroup, visualContext, true,
          new Set(resolved.ownedSketchIds ?? []));
        if (resolvedGroup.children.length > 0) group.add(resolvedGroup);
      }
      if (group.children.length === 0) {
        const placeholder = new THREE.Mesh(
          new THREE.BoxGeometry(20, 20, 20),
          this.materials.surface(CATIA_VISUAL_THEME.placeholder),
        );
        placeholder.userData = { kind: "instance", id: instance.id };
        markNavigationPickable(placeholder);
        group.add(placeholder);
      }
    }
    this.addAssemblyConstraintMarkers(view);
  }

  private addAssemblyConstraintMarkers(view: DocumentView): void {
    // A separate projection from geometry: never hydrate GLB or rebuild bodies.
    this.assemblyConstraintMarkers ??= new Map();
    const live=new Set<string>();
    const scopes:Array<{view:DocumentDescriptor;occurrence:string;treeNodeId:string;instancePath?:import("../types").InstancePath}>=[{view,occurrence:"",treeNodeId:`document:${view.document.id}`},...(view.constraintDisplayScopes??[]).map(scope=>({
      view:{document:{...view.document,id:scope.documentId,versionId:scope.versionId},product:{instances:[],constraints:scope.constraints}},
      occurrence:scope.instancePath.canonical,treeNodeId:scope.treeNodeId,instancePath:scope.instancePath}))];
    // Only the explicitly editable occurrence follows its authoritative draft
    // document. Other occurrences keep their accepted immutable projections.
    const editing=this.editContext;
    if(editing?.liveConstraintProjection && editing.view.document.type==="PRODUCT") {
      const scope=scopes.find(scope=>scope.occurrence===editing.occurrencePath && scope.view.document.id===editing.view.document.id);
      if(scope)scope.view=editing.view;
    }
    for(const scope of scopes)this.reconcileAssemblyConstraintMarkers(scope.view,scope.occurrence,live,scope.treeNodeId,scope.instancePath);
    for(const [key,entry] of this.assemblyConstraintMarkers)if(!live.has(key))this.removeAssemblyConstraintMarker(key,entry);
    this.refreshInteractionHighlights();
  }

  private assemblyMarkerKey(documentId:string,occurrence:string,constraintId:string) {
    return JSON.stringify([documentId,occurrence,constraintId]);
  }

  private removeAssemblyConstraintMarker(key:string,entry:{group:THREE.Group;selection:SelectionItem},deleted=true) {
    this.selectionIndex.unregister(entry.selection,entry.group);
    entry.group.removeFromParent();this.disposeRenderable(entry.group);
    this.assemblyConstraintMarkers.delete(key);this.assemblyConstraintReferences.delete(key);
    const matches=(s:Selection)=>s?.kind==="assembly-constraint" && this.assemblyMarkerKey(s.documentId??"",s.occurrencePath??"",s.constraintId)===key;
    if(deleted){const priorCount=this.selected.length;this.selected=this.selected.filter(s=>!matches(s));
      if(this.selected.length!==priorCount)this.callbacks?.selectionsChanged(this.selected);
      if(matches(this.preselected)){this.preselected=null;this.callbacks?.preselectionChanged(null);}}
  }

  private reconcileAssemblyConstraintMarkers(view: DocumentDescriptor,occurrence:string,live:Set<string>,treePath:string,instancePath?:import("../types").InstancePath): void {
    const glyphs: Record<import("../types").AssemblyConstraint["kind"], number> = {
      FIX: 2, RIGID: 7, FIX_TOGETHER:7, CONTACT:6, COINCIDENT: 0, CONCENTRIC: 13, ANGLE: 12, DISTANCE: 8,
    };
    const statusColors: Record<import("../types").AssemblyConstraint["evaluationStatus"], number> = {
      VERIFIED: CATIA_VISUAL_THEME.constraint, NOT_UPDATED: CATIA_VISUAL_THEME.selected, IMPOSSIBLE: CATIA_VISUAL_THEME.sketchRedundant, BROKEN: CATIA_VISUAL_THEME.sketchInvalid,
    };
    for (const constraint of view.product?.constraints ?? []) {
      const key=this.assemblyMarkerKey(view.document.id,occurrence,constraint.id);live.add(key);
      const references = assemblyConstraintReferences(constraint, view.product?.constraints ?? []);
      const located = references.map((reference) => {
        const exact = this.resolveAssemblyConstraintReference(reference,occurrence);
        if (exact) return { reference, ...exact, exact: true };
        const instance = this.instanceGroups.get(occurrence.split("/")[0] || reference.instanceId);
        if (!instance) return undefined;
        const selection: SelectionItem = { kind: "instance", id: reference.instanceId, instanceId: reference.instanceId,
          occurrencePath: reference.instanceId, visualKey: `occurrence:${reference.instanceId}` };
        return { reference, selection, object: instance, anchor: new THREE.Box3().setFromObject(instance).getCenter(new THREE.Vector3()), exact: false };
      }).filter((value): value is NonNullable<typeof value> => Boolean(value));
      if (!located.length) {const old=this.assemblyConstraintMarkers.get(key);if(old)this.removeAssemblyConstraintMarker(key,old);continue;}
      const signature=JSON.stringify([constraint,located.map(l=>l.anchor.toArray())]);
      const prior=this.assemblyConstraintMarkers.get(key);
      if(prior?.signature===signature)continue;
      if(prior)this.removeAssemblyConstraintMarker(key,prior,false);
      const markerPosition = located.reduce((sum, value) => sum.add(value.anchor), new THREE.Vector3())
        .multiplyScalar(1 / located.length);
      const span = located.length > 1 ? located[0].anchor.distanceTo(located[1].anchor) : 0;
      markerPosition.z += Math.max(4, span * 0.08);
      const treeNodeId = `${treePath ?? `document:${view.document.id}`}/assembly-constraints/constraint:${constraint.id}`;
      const selection: SelectionItem = { kind: "assembly-constraint", id: constraint.id, constraintId: constraint.id,
        constraintType: constraint.kind, documentId: view.document.id, occurrencePath:occurrence, instancePath, rootDocumentId:occurrence ? this.view?.document.id : undefined, treeNodeId };
      const group = new THREE.Group(); group.position.copy(markerPosition); group.userData = selection;
      const pointGeometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3()]);
      const glyph = new THREE.Points(pointGeometry, this.materials.constraintGlyph(
        assemblyConstraintGlyph(constraint.evaluationStatus, glyphs[constraint.kind]), constraint.suppressed ? 0x808080 : statusColors[constraint.evaluationStatus], 22));
      glyph.renderOrder = 92;
      group.add(glyph);
      if (located.some((value) => !value.anchor.equals(markerPosition))) {
        const leaders = makeOcclusionVisibleSegments(located.map((value) => [value.anchor.clone().sub(markerPosition), new THREE.Vector3()]),
          constraint.suppressed ? 0x808080 : statusColors[constraint.evaluationStatus], 1.75);
        leaders.renderOrder = 90; group.add(leaders);
      }
      this.helpers.add(group);
      this.selectionIndex.register(selection, group);
      this.selectionIndex.registerPick(glyph, () => selection, 90);
      this.assemblyConstraintMarkers.set(key,{signature,group,selection});
      this.assemblyConstraintReferences.set(key, located.map((value) => value.selection));
      for (const value of located) {
        // Topology references are highlighted by exact face/edge/vertex overlays. Associating
        // their owning mesh group would incorrectly highlight the complete occurrence.
        if (!value.exact || !['FACE', 'EDGE', 'VERTEX'].includes(value.reference.kind))
          this.selectionIndex.associate(selection, value.object);
        this.selectionIndex.associate(value.selection, group);
      }
    }
  }

  measureAssemblyConstraint(kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]): number {
    if (references.length < 2) return 0;
    const resolved = references.slice(0, 2).map((reference) => this.resolveAssemblyConstraintReference(reference));
    if (!resolved[0] || !resolved[1]) return 0;
    const direction = (reference: AssemblyGeometryRef, value: NonNullable<typeof resolved[number]>): THREE.Vector3 | undefined => {
      if (reference.kind === 'PLANE') {
        const datum = (value.selection as Extract<SelectionItem, { kind: 'plane' }>).datumPlane;
        return datum ? new THREE.Vector3().fromArray(datum.normal).transformDirection(value.object.matrixWorld) : undefined;
      }
      if (reference.kind === 'FACE') {
        const binding = [...this.solidBindings.values()].find((candidate) => candidate.context.instanceId === reference.instanceId &&
      (!reference.instancePath || candidate.context.occurrencePath === reference.instancePath.canonical) &&
          candidate.artifact.geometryKey === reference.geometryKey);
        if (!binding || !reference.topologyId) return undefined;
        const normal = new THREE.Vector3(); let count = 0;
        binding.artifact.mesh.triangles.forEach((triangle, index) => {
          if ((binding.artifact.mesh.faceIds[index] ?? -1) !== reference.topologyId) return;
          const a = new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[triangle[0]]);
          const b = new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[triangle[1]]);
          const c = new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[triangle[2]]);
          normal.add(b.sub(a).cross(c.sub(a))); count++;
        });
        return count && normal.lengthSq() > 0 ? normal.transformDirection(binding.mesh.matrixWorld) : undefined;
      }
      if (reference.kind === 'AXIS' || reference.kind === 'EDGE' || reference.kind === 'CYLINDER') {
        if (reference.kind === 'EDGE') {
          const binding = [...this.solidBindings.values()].find((candidate) => candidate.context.instanceId === reference.instanceId &&
      (!reference.instancePath || candidate.context.occurrencePath === reference.instancePath.canonical) &&
            candidate.artifact.geometryKey === reference.geometryKey);
          const edge = binding?.artifact.mesh.edges?.find((candidate) => candidate.localId === reference.topologyId);
          if (binding && edge && edge.points.length >= 2) {
            return new THREE.Vector3().fromArray(edge.points[edge.points.length - 1])
              .sub(new THREE.Vector3().fromArray(edge.points[0])).transformDirection(binding.mesh.matrixWorld);
          }
        }
        const geometry = (value.object as THREE.Line).geometry as THREE.BufferGeometry | undefined;
        const positions = geometry?.getAttribute('position');
        if (positions && positions.count >= 2) {
          const first = new THREE.Vector3().fromBufferAttribute(positions, 0);
          const second = new THREE.Vector3().fromBufferAttribute(positions, positions.count - 1);
          return second.sub(first).transformDirection(value.object.matrixWorld);
        }
      }
      return undefined;
    };
    const firstDirection = direction(references[0], resolved[0]);
    const secondDirection = direction(references[1], resolved[1]);
    if (kind === 'angle') {
      if (!firstDirection || !secondDirection) return 0;
      return Math.atan2(firstDirection.clone().cross(secondDirection).length(),
        THREE.MathUtils.clamp(firstDirection.dot(secondDirection), -1, 1)) * 180 / Math.PI;
    }
    if (kind !== 'distance') return 0;
    const firstPlane = references[0].kind === 'PLANE' || references[0].kind === 'FACE';
    const secondPlane = references[1].kind === 'PLANE' || references[1].kind === 'FACE';
    if (firstPlane && secondPlane) {
      if (!firstDirection || !secondDirection || Math.abs(firstDirection.dot(secondDirection)) < 1 - 1e-6) return 0;
      return Math.abs(resolved[0].anchor.clone().sub(resolved[1].anchor).dot(secondDirection));
    }
    if (firstPlane && firstDirection)
      return Math.abs(resolved[1].anchor.clone().sub(resolved[0].anchor).dot(firstDirection));
    if (secondPlane && secondDirection)
      return Math.abs(resolved[0].anchor.clone().sub(resolved[1].anchor).dot(secondDirection));
    const firstAxis = references[0].kind === 'AXIS' || references[0].kind === 'EDGE' || references[0].kind === 'CYLINDER';
    const secondAxis = references[1].kind === 'AXIS' || references[1].kind === 'EDGE' || references[1].kind === 'CYLINDER';
    if (firstAxis && secondAxis && firstDirection && secondDirection) {
      const cross = firstDirection.clone().cross(secondDirection);
      const delta = resolved[0].anchor.clone().sub(resolved[1].anchor);
      return cross.lengthSq() < 1e-12 ? delta.cross(secondDirection).length() : Math.abs(delta.dot(cross.normalize()));
    }
    if (firstAxis && firstDirection) return resolved[1].anchor.clone().sub(resolved[0].anchor).cross(firstDirection).length();
    if (secondAxis && secondDirection) return resolved[0].anchor.clone().sub(resolved[1].anchor).cross(secondDirection).length();
    return resolved[0].anchor.distanceTo(resolved[1].anchor);
  }

  focusAssemblyReference(reference: AssemblyGeometryRef,ownerOccurrence=""): boolean {
    const resolved = this.resolveAssemblyConstraintReference(reference,ownerOccurrence);
    const instance = this.instanceGroups.get(reference.instanceId);
    const anchor = resolved?.anchor ?? (instance ? new THREE.Box3().setFromObject(instance).getCenter(new THREE.Vector3()) : undefined);
    if (!anchor) return false;
    this.viewTransition.cancel();
    const offset = this.camera.position.clone().sub(this.navigation.target);
    this.navigation.target.copy(anchor);
    this.camera.position.copy(anchor).add(offset);
    this.navigation.syncCamera();
    this.invalidate();
    return true;
  }

  assemblyAngleReferenceDirection(references: AssemblyGeometryRef[]): Vec3 | undefined {
    if (references.length < 2) return undefined;
    const resolved = references.slice(0, 2).map((reference) => this.resolveAssemblyConstraintReference(reference));
    if (!resolved[0] || !resolved[1]) return undefined;
    const planeNormal = (reference: AssemblyGeometryRef, value: NonNullable<typeof resolved[number]>): THREE.Vector3 | undefined => {
      if (reference.kind === "PLANE") {
        const datum = (value.selection as Extract<SelectionItem, {kind:"plane"}>).datumPlane;
        return datum ? new THREE.Vector3().fromArray(datum.normal).transformDirection(value.object.matrixWorld) : undefined;
      }
      if (reference.kind !== "FACE") return undefined;
      const binding=[...this.solidBindings.values()].find((candidate)=>candidate.context.instanceId===reference.instanceId&&candidate.artifact.geometryKey===reference.geometryKey);
      if(!binding||!reference.topologyId)return undefined;const normal=new THREE.Vector3();const samples:THREE.Vector3[]=[];
      binding.artifact.mesh.triangles.forEach((triangle,index)=>{if((binding.artifact.mesh.faceIds[index]??-1)!==reference.topologyId)return;
        const a=new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[triangle[0]]),b=new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[triangle[1]]),c=new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[triangle[2]]);
        const sample=b.sub(a).cross(c.sub(a));if(sample.lengthSq()>1e-12){sample.normalize();samples.push(sample);normal.add(sample);}});
      if(!samples.length||normal.lengthSq()<1e-12)return undefined;normal.normalize();
      if(samples.some((sample)=>Math.abs(sample.dot(normal))<0.9999))return undefined;
      return normal.transformDirection(binding.mesh.matrixWorld);
    };
    const first=planeNormal(references[0],resolved[0]),second=planeNormal(references[1],resolved[1]);
    if(!first||!second)return undefined;
    const worldReference=first.clone().cross(second);
    const secondInstance=this.instanceGroups.get(references[1].instanceId);
    if(!secondInstance)return undefined;
    if(worldReference.lengthSq()<1e-10){
      const candidates=[new THREE.Vector3(1,0,0),new THREE.Vector3(0,1,0),new THREE.Vector3(0,0,1)]
        .map((axis)=>axis.applyQuaternion(secondInstance.getWorldQuaternion(new THREE.Quaternion())));
      const candidate=candidates.sort((a,b)=>Math.abs(a.dot(second))-Math.abs(b.dot(second)))[0];
      worldReference.copy(second).cross(candidate);
    }
    if(worldReference.lengthSq()<1e-12)return undefined;
    const inverse=secondInstance.getWorldQuaternion(new THREE.Quaternion()).invert();
    return worldReference.normalize().applyQuaternion(inverse).toArray();
  }

  private resolveAssemblyConstraintReference(reference: AssemblyGeometryRef,ownerOccurrence=""):
    { selection: SelectionItem; object: THREE.Object3D; anchor: THREE.Vector3 } | undefined {
    const scopedPath=ownerOccurrence ? `${ownerOccurrence}/${reference.instancePath?.canonical ?? reference.instanceId}` : reference.instancePath?.canonical;
    const instance = this.instanceGroups.get(ownerOccurrence.split("/")[0] || reference.instanceId);
    if (!instance) return undefined;
    const instanceSelection: SelectionItem = { kind: "instance", id: reference.instanceId, instanceId: reference.instanceId,
      occurrencePath: reference.instanceId, visualKey: `occurrence:${reference.instanceId}` };
    const instanceCenter = () => new THREE.Box3().setFromObject(instance).getCenter(new THREE.Vector3());
    const resolvedTopology = reference.resolution?.result.status === "RESOLVED" ? reference.resolution.result.candidates?.[0] : undefined;
    const geometryKey = resolvedTopology?.geometryKey ?? reference.geometryKey;
    const topologyId = resolvedTopology?.localId ?? reference.topologyId;
    const binding = [...this.solidBindings.values()].find((candidate) => (ownerOccurrence || candidate.context.instanceId === reference.instanceId) &&
      (!scopedPath || candidate.context.occurrencePath === scopedPath) &&
      (!geometryKey || candidate.artifact.geometryKey === geometryKey));
    if (reference.kind === "BODY") return {selection:binding ? binding.group.userData as SelectionItem : instanceSelection,object:binding?.group ?? instance,
      anchor:binding ? new THREE.Box3().setFromObject(binding.group).getCenter(new THREE.Vector3()) : instanceCenter()};
    if (reference.kind === "FACE" && binding && topologyId) {
      const anchor = new THREE.Vector3(); let count = 0;
      binding.artifact.mesh.triangles.forEach((triangle, index) => {
        if ((binding.artifact.mesh.faceIds[index] ?? -1) !== topologyId) return;
        for (const vertexIndex of triangle) { anchor.add(binding.mesh.localToWorld(new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[vertexIndex]))); count++; }
      });
      const selection: SelectionItem = { kind: "face", id: `${binding.context.occurrencePath}:${binding.artifact.geometryKey}:face:${topologyId}`,
        topologyId, ...binding.context };
      return { selection, object: binding.group, anchor: count ? anchor.multiplyScalar(1 / count) : instanceCenter() };
    }
    if (reference.kind === "EDGE" && binding && topologyId) {
      const edge=binding.artifact.mesh.edges?.find((candidate)=>candidate.localId===topologyId);
      const anchor=new THREE.Vector3();for(const point of edge?.points??[])anchor.add(binding.mesh.localToWorld(new THREE.Vector3().fromArray(point)));
      const selection:SelectionItem={kind:"edge",id:`${binding.context.occurrencePath}:${binding.artifact.geometryKey}:edge:${topologyId}`,
        topologyId,...binding.context};
      return {selection,object:binding.group,anchor:edge?.points.length?anchor.multiplyScalar(1/edge.points.length):instanceCenter()};
    }
    if (reference.kind === "VERTEX" && binding && topologyId) {
      const vertex=binding.artifact.mesh.topologyVertices?.find((candidate)=>candidate.localId===topologyId);
      const selection:SelectionItem={kind:"vertex",id:`${binding.context.occurrencePath}:${binding.artifact.geometryKey}:vertex:${topologyId}`,
        topologyId,...binding.context};
      return {selection,object:binding.group,anchor:vertex?binding.mesh.localToWorld(new THREE.Vector3().fromArray(vertex.point)):instanceCenter()};
    }
    let matched: THREE.Object3D | undefined;
    instance.traverse((object) => {
      if (matched || object.userData.entityId !== reference.geometryId ||
        (scopedPath && object.userData.occurrencePath !== scopedPath)) return;
      if (reference.kind === "PLANE" && object.userData.kind === "plane") matched = object;
      else if (reference.kind === "AXIS" && object.userData.kind === "axis" && (!reference.axis || object.userData.axis === reference.axis)) matched = object;
      else if (reference.kind === "POINT" && object.userData.kind === "axis-system") matched = object;
    });
    const selection = matched?.userData as SelectionItem | undefined;
    return selection && matched ? { selection, object: matched, anchor: new THREE.Box3().setFromObject(matched).getCenter(new THREE.Vector3()) }
      : undefined;
  }

  private addDatumPlane(datum: DatumPlane, parent: THREE.Group, selectable: boolean, context?: SolidContext): void {
    const { id, plane } = datum;
    const geometry = new THREE.PlaneGeometry(0.72, 0.72);
    if (plane !== "CUSTOM") geometry.translate(0.54, 0.54, 0);
    const material = this.materials.datumPlane(planeColors[plane]);
    const mesh = new THREE.Mesh(geometry, material);
    mesh.position.fromArray(datum.origin);
    const normal = new THREE.Vector3().fromArray(datum.normal).normalize();
    const u = new THREE.Vector3().fromArray(datum.uDirection).normalize();
    mesh.quaternion.setFromRotationMatrix(new THREE.Matrix4().makeBasis(u, normal.clone().cross(u).normalize(), normal));
    mesh.renderOrder = 90;
    const selection = {
      kind: "plane" as const, id: `${context?.occurrencePath || "root"}:${id}`, entityId: id, plane, datumPlane: datum,
      treeNodeId: context?.treeNodeId, documentId: context?.documentId, occurrencePath: context?.occurrencePath,
      versionId: context?.versionId,
      geometryKey: context?.geometryKey, instancePath: context?.instancePath, instanceId: context?.instanceId
    };
    mesh.userData = selection;
    parent.add(mesh);
    this.screenStableReferences.set(mesh, 46);
    if (selectable || context) {
      this.selectable.set(`plane:${selection.id}`, mesh);
      this.selectionIndex.register(selection, mesh);
      this.selectionIndex.registerPick(mesh, () => selection, 200, 10);
    }
  }

  private addDatumAxis(axis: DatumAxis, parent: THREE.Group, context?: SolidContext): void {
    const reference = makeDatumAxisReference(axis);
    updateHighlightLineResolution(reference, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
    const pickLine = new THREE.Line(new THREE.BufferGeometry().setFromPoints([
      new THREE.Vector3(-0.65, 0, 0), new THREE.Vector3(1, 0, 0),
    ]), new THREE.LineBasicMaterial({ transparent: true, opacity: 0, depthTest: false, depthWrite: false }));
    const selection = { kind: "axis" as const, axis: "DATUM" as const,
      id: `${context?.occurrencePath || "root"}:${axis.id}`, entityId: axis.id, treeNodeId: context?.treeNodeId,
      documentId: context?.documentId, versionId: context?.versionId, occurrencePath: context?.occurrencePath, geometryKey: context?.geometryKey,
      instancePath: context?.instancePath, instanceId: context?.instanceId };
    pickLine.raycast = raycastDatumAxis;
    reference.userData = selection;
    pickLine.userData = selection;
    reference.add(pickLine);
    parent.add(reference);
    this.screenStableReferences.set(reference, 54);
    this.selectionIndex.register(selection, reference); this.selectionIndex.registerPick(pickLine, (hit) =>
      datumAxisHitAccepted(this.raycaster.ray.distanceToPoint(hit.point), this.datumAxisPickToleranceWorld) ? selection : null, 200, 10);
  }

  private addAxisSystem(axis: AxisSystem, parent: THREE.Group, context?: SolidContext): void {
    const origin = new THREE.Vector3().fromArray(axis.origin);
    const system = new THREE.Group();
    system.position.copy(origin);
    this.screenStableReferences.set(system, 54);
    const systemSelection = {
      kind: "axis-system" as const, id: `${context?.occurrencePath || "root"}:${axis.id}`,
      entityId: axis.id,
      treeNodeId: context?.treeNodeId, documentId: context?.documentId, occurrencePath: context?.occurrencePath,
      geometryKey: context?.geometryKey, instancePath: context?.instancePath, instanceId: context?.instanceId
    };
    const definitions = [["X", axis.xDirection, CATIA_VISUAL_THEME.axisX], ["Y", axis.yDirection, CATIA_VISUAL_THEME.axisY], ["Z", axis.zDirection, CATIA_VISUAL_THEME.axisZ]] as const;
    for (const [name, direction, color] of definitions) {
      const axisReference = new THREE.Group();
      const points = [new THREE.Vector3(), new THREE.Vector3().fromArray(direction)];
      const visibleLine = makeDatumReferenceLine(points, color);
      updateHighlightLineResolution(visibleLine, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
      const pickLine = new THREE.Line(new THREE.BufferGeometry().setFromPoints(points),
        new THREE.LineBasicMaterial({ transparent: true, opacity: 0, depthTest: false, depthWrite: false }));
      const selection = {
        ...systemSelection, kind: "axis" as const, axis: name, id: `${systemSelection.id}:${name}`,
        treeNodeId: context?.treeNodeId ? `${context.treeNodeId}/${name.toLowerCase()}` : undefined
      };
      pickLine.raycast = raycastDatumAxis;
      axisReference.userData = selection;
      pickLine.userData = selection;
      axisReference.add(visibleLine, pickLine);
      system.add(axisReference);
      this.selectionIndex.register(selection, axisReference, context?.treeNodeId);
      this.selectionIndex.registerPick(pickLine, (hit) =>
        datumAxisHitAccepted(this.raycaster.ray.distanceToPoint(hit.point), this.datumAxisPickToleranceWorld) ? selection : null, 200, 10);
    }
    const originPoint = new THREE.Points(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3()]), this.materials.point(CATIA_VISUAL_THEME.vertex, 8, false));
    originPoint.userData = {...systemSelection,patternCenterMarker:true};
    originPoint.visible = false;
    system.add(originPoint);
    this.selectionIndex.registerPick(originPoint, () => this.featureSelection?.role === "point" ? systemSelection : null, 210);
    system.userData = systemSelection; parent.add(system);
    this.selectionIndex.register(systemSelection, system);
  }

  private addReferenceGeometry(reference: ReferenceGeometry | undefined, parent: THREE.Group, context: SolidContext): void {
    if (!reference) return;
    for (const datum of reference.datumPlanes ?? []) this.addDatumPlane(datum, parent, false, {
      ...context, treeNodeId: context.treeNodeId.replace(/\/body$/, `/origin/plane:${datum.id}`),
    });
    for (const axis of reference.axisSystems ?? []) this.addAxisSystem(axis, parent, {
      ...context, treeNodeId: context.treeNodeId.replace(/\/body$/, `/origin/axis:${axis.id}`),
    });
    for (const axis of reference.datumAxes ?? []) this.addDatumAxis(axis, parent, {
      ...context, treeNodeId: context.treeNodeId.replace(/\/body$/, `/origin/datum-axis:${axis.id}`),
    });
  }

  private featureTreeNode(context: SolidContext, featureID: string): string | undefined {
    const visit = (node: DocumentStructureNode | undefined): string | undefined => {
      if (!node) return undefined;
      if (node.entityId === featureID && node.id.startsWith(context.treeNodeId)) return node.id;
      for (const child of node.children ?? []) {
        const found = visit(child);
        if (found) return found;
      }
      return undefined;
    };
    return visit(this.view?.structureTree);
  }

  private addVisualPrimitives(visualization: VisualizationManifest | undefined, parent: THREE.Group, context: SolidContext,
    includeSketch = true, ownedSketchIds?: ReadonlySet<string>): void {
    if (!visualization || (visualization.schemaVersion !== 1 && visualization.schemaVersion !== 2)) return;
    const sketchEntityObjects = new Map<string, THREE.Object3D>();
    const sketchGroups = new Map<string, THREE.Group>();
    const featureTreeNodes = new Map<string, string | undefined>();
    const constraintAssociations: Array<{ selection: Extract<SelectionItem, { kind: "sketch-constraint" }>; featureID: string; entityIDs: string[] }> = [];
    for (const primitive of visualization.primitives ?? []) {
      const isSketch = primitive.semantic.startsWith("SKETCH_");
      if (isSketch && (!includeSketch || ownedSketchIds && !ownedSketchIds.has(primitive.patternId??primitive.featureId))) continue;
      if (primitive.positions.length === 0) continue;
      const construction = primitive.role === "CONSTRUCTION";
      const color = primitive.semantic === "SKETCH_CONSTRAINT" ? CATIA_VISUAL_THEME.constraint
        : sketchDiagnosticColor(primitive.status, construction);
      const points = primitive.positions.map((position) => new THREE.Vector3().fromArray(position));
      const geometry = new THREE.BufferGeometry().setFromPoints(points);
      let object: THREE.Object3D;
      const type = visualType(primitive);
      if (primitive.semantic === "SKETCH_CONSTRAINT" && primitive.kind === "POINTS") {
        object = new THREE.Points(geometry,
          this.materials.constraintGlyph(constraintSymbolCode(primitive.entityType as ConstraintKind)));
        object.renderOrder = SKETCH_FEEDBACK_ORDER.glyph;
      } else if (primitive.semantic === "SKETCH_CONSTRAINT" && primitive.kind === "LINE_SEGMENTS") {
        geometry.dispose();
        const group = new THREE.Group();
        const segments: Array<[THREE.Vector3, THREE.Vector3]> = [];
        for (let index = 0; index + 1 < points.length; index += 2) segments.push([points[index], points[index + 1]]);
        const leaders = makeOcclusionVisibleSegments(segments, CATIA_VISUAL_THEME.constraint, 1.25);
        leaders.renderOrder = SKETCH_FEEDBACK_ORDER.leader;
        updateHighlightLineResolution(leaders, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
        group.add(leaders);
        if (primitive.label && primitive.labelPosition) {
          const ownerView=context.documentId===this.editContext?.view.document.id?this.editContext.view:this.view;
          const feature=ownerView?.part?.features.find(feature=>feature.id===primitive.featureId);
          const support=feature?.sketch?.support;
          const datum=ownerView?.datumPlanes?.find(plane=>plane.id===support?.datumPlaneId)??ownerView?.part?.datumPlanes?.find(plane=>plane.id===support?.datumPlaneId);
          const plane:PlaneName|SketchPlane=support?.origin&&support.normal&&support.xDirection?{datumPlaneId:support.datumPlaneId??feature!.id,plane:"CUSTOM",origin:support.origin,normal:support.normal,uDirection:support.xDirection}:datum?{datumPlaneId:datum.id,plane:datum.plane,origin:datum.origin,normal:datum.normal,uDirection:datum.uDirection}:feature?.plane!=="CUSTOM"?feature?.plane??"XY":"XY";
          const definition=feature?.sketch?.constraints.find(constraint=>constraint.id===primitive.id);
          const label = makeConstraintDimensionLabel(definition?sketchDimensionText(definition):primitive.label,point=>localToWorld(plane,point));
          label.position.fromArray(primitive.labelPosition);
          label.renderOrder = SKETCH_FEEDBACK_ORDER.label;
          group.add(label);
        }
        object = group;
      } else if (primitive.kind === "POINTS") {
        object = new THREE.Points(geometry, this.materials.point(color, 9, false));
      } else if (primitive.kind === "POLYLINE") {
        geometry.dispose();
        object = makeSketchOverlayLine(points, color, 2.25, construction);
        updateHighlightLineResolution(object, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
      } else if (primitive.kind === "LINE_SEGMENTS") {
        object = new THREE.LineSegments(geometry, new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.constraint, depthTest: false }));
      } else {
        if (!primitive.indices || primitive.indices.length < 3) { geometry.dispose(); continue; }
        geometry.setIndex(primitive.indices);
        geometry.computeVertexNormals();
        object = new THREE.Mesh(geometry, new THREE.MeshBasicMaterial({
          color, transparent: true, opacity: 0.58, side: THREE.DoubleSide, depthWrite: false,
        }));
      }
      if (primitive.semantic !== "SKETCH_CONSTRAINT") object.renderOrder = primitive.kind === "POINTS" ? construction ? SKETCH_FEEDBACK_ORDER.constructionEndpoint : SKETCH_FEEDBACK_ORDER.endpoint : construction ? SKETCH_FEEDBACK_ORDER.construction : SKETCH_FEEDBACK_ORDER.geometry;
      if (!featureTreeNodes.has(primitive.featureId)) featureTreeNodes.set(primitive.featureId, this.featureTreeNode(context, primitive.featureId));
      const featureTreeNode = featureTreeNodes.get(primitive.featureId);
      let objectParent = parent;
      if (isSketch && ownedSketchIds) {
        let group = sketchGroups.get(primitive.featureId);
        if (!group) {
          group = new THREE.Group();
          const sketchSelection = {kind:"sketch" as const, patternId:primitive.patternId,patternMemberSlot:primitive.patternMemberSlot,id:primitive.featureId, entityId:primitive.featureId,
            documentId:context.documentId, bodyId:context.bodyId, versionId:context.versionId,
            contextVariantKey:context.contextVariantKey, instancePath:context.instancePath,
            occurrencePath:context.occurrencePath, treeNodeId:featureTreeNode};
          group.userData = {...sketchSelection, sketchFeatureID:primitive.featureId, visualizationPrimitive:true};
          parent.add(group);
          this.selectionIndex.register(sketchSelection, group, featureTreeNode);
          sketchGroups.set(primitive.featureId, group);
        }
        objectParent = group;
      }
      const selection = visualSelection(primitive, {
        treeNodeId: featureTreeNode ? primitive.semantic === "SKETCH_CONSTRAINT"
          ? constraintTreeNodeID(featureTreeNode, primitive.entityType as ConstraintKind, primitive.id) : `${featureTreeNode}/geometry/entity:${primitive.id}`
          : context.treeNodeId,
        documentId: context.documentId, versionId: context.versionId, bodyId: context.bodyId,
        contextVariantKey: context.contextVariantKey,
        instancePath: context.instancePath, occurrencePath: context.occurrencePath,
        geometryKey: context.geometryKey, instanceId: context.instanceId,
      });
      object.userData = { ...selection, entityId: primitive.displayEntityId ?? ("entityId" in selection ? selection.entityId : undefined),
        sketchFeatureID: primitive.featureId, visualizationPrimitive: true };
      object.traverse((child) => {
        child.userData = { ...child.userData, ...selection, entityId: primitive.displayEntityId ?? ("entityId" in selection ? selection.entityId : undefined),
          sketchFeatureID: primitive.featureId, visualizationPrimitive: true };
      });
      objectParent.add(object);
      if (primitive.semantic === "SKETCH_POINT" || primitive.semantic === "SKETCH_CURVE") {
        sketchEntityObjects.set(`${primitive.featureId}:${primitive.id}`, object);
      }
      if (primitive.selectable) {
        this.selectable.set(`visual:${selection.id}`, object);
        this.selectionIndex.register(selection, object, selection.treeNodeId);
        const pickables = object.children.length > 0 ? object.children : [object];
        for (const pickable of pickables) {
          this.selectionIndex.registerPick(pickable, () => selection, type === "POINT" ? 75 : type === "CURVE" ? 70 : 30);
        }
        if (primitive.semantic === "SKETCH_CONSTRAINT") {
          constraintAssociations.push({ selection: selection as Extract<SelectionItem, { kind: "sketch-constraint" }>,
            featureID: primitive.featureId, entityIDs: primitive.relatedEntityIds ?? [] });
        }
      }
    }
    for (const association of constraintAssociations) {
      for (const entityID of association.entityIDs) {
        const related = sketchEntityObjects.get(`${association.featureID}:${entityID}`);
        if (related) this.selectionIndex.associate(association.selection, related);
      }
    }
  }

  private buildSketchContext(): void {
    this.disposeGroup(this.sketchContext);
    if (!this.sketchPlane) return;
    const anchor = localToWorld(this.sketchPlane, [0, 0]);
    for (const [endpoint, color] of [[[1, 0], CATIA_VISUAL_THEME.axisX], [[0, 1], CATIA_VISUAL_THEME.axisY]] as const) {
      const direction = localToWorld(this.sketchPlane, [...endpoint]).sub(anchor);
      const muted = new THREE.Color(color).lerp(new THREE.Color(CATIA_VISUAL_THEME.backgroundBottom), 0.28);
      this.sketchContext.add(makeSketchReferenceAxis(anchor, direction, muted));
    }
    const origin = new THREE.Points(
      new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane, [0, 0])]),
      this.materials.point(CATIA_VISUAL_THEME.sketchProfile, 11, false),
    );
    origin.renderOrder = 18;
    this.sketchContext.add(origin);
  }

  private updateSketchContextVisibility(): void {
    const editing = Boolean(this.sketchPlane && this.activeSketchID);
    const inContext = Boolean(this.editContext?.occurrencePath);
    const layers = sketchContextLayerVisibility(editing ? this.activeSketchID : undefined, inContext);
    this.environment.visible = layers.environment;
    this.lighting.visible = layers.lighting;
    this.content.visible = layers.body;
    this.sketchContext.visible = layers.sketch;
    for (const child of this.helpers.children) {
      if (child.userData.visualizationPrimitive) {
        child.visible = !editing;
      } else if (child.userData.sketchEditOverlay) {
        const address = this.semanticVisibilityAddress(child.userData as Partial<SelectionItem>);
        child.visible = address ? this.visibilityResolver?.resolve(address, this.editingSketchScope()).effectiveVisible ?? true : true;
        for (const sketchChild of child.children) {
          if (sketchChild.userData.sketchEntityOverlay) {
            const entityAddress = this.semanticVisibilityAddress(sketchChild.userData as Partial<SelectionItem>);
            sketchChild.visible = Boolean(child.visible && (entityAddress ? this.visibilityResolver?.resolve(entityAddress, this.editingSketchScope()).effectiveVisible : true));
          }
        }
      } else child.visible = !editing || child.userData.sketchFeatureID === this.activeSketchID;
    }
  }

  private addSketch(feature: Feature, _includeEntities = true, sourceView = this.view,
    sourceContext?: SolidContext, translation?: Vec3, rotation?: [number, number, number, number], visibleOutsideSketchEdit = true): void {
    const support = feature.sketch?.support;
    const datum = sourceView?.datumPlanes?.find((candidate) => candidate.id === support?.datumPlaneId)
      ?? sourceView?.artifact?.visualization.referenceGeometry.datumPlanes?.find((candidate) => candidate.id === support?.datumPlaneId);
    const facePlane: SketchPlane | undefined = support?.type === "PLANAR_FACE" && support.status !== "FAILED_SUPPORT" &&
      support.origin && support.normal && support.xDirection
      ? { datumPlaneId: `face:${support.persistentSelection?.anchor.featureId ?? feature.id}`, plane: "CUSTOM",
          origin: support.origin, normal: support.normal, uDirection: support.xDirection }
      : undefined;
    const plane: PlaneName | SketchPlane = facePlane ?? (datum ? { datumPlaneId: datum.id, plane: datum.plane, origin: datum.origin,
      normal: datum.normal, uDirection: datum.uDirection } : (support?.plane as PlaneName ?? feature.plane ?? "XY"));
    const group = new THREE.Group();
    if (translation) group.position.fromArray(translation);
    if (rotation) group.quaternion.fromArray(rotation);
    group.userData.sketchFeatureID = feature.id;
    const documentId = sourceView?.document.id ?? "";
    const context = sourceContext ?? { documentId, bodyId:feature.bodyId, geometryKey: sourceView?.part?.bodies.find(b=>b.id===feature.bodyId)?.geometryKey ?? "",
      occurrencePath: "", versionId: sourceView?.document.versionId,
      treeNodeId: `document:${documentId}/body:${feature.bodyId}` };
    const featureTreeNode = this.featureTreeNode(context, feature.id)
      ?? `${context.treeNodeId}/sketch:${feature.id}`;
    const sketchSelection = { kind: "sketch" as const, id: feature.id, documentId,
      bodyId: feature.bodyId, versionId: context.versionId, instancePath: context.instancePath,
      contextVariantKey: context.contextVariantKey,
      occurrencePath: context.occurrencePath, treeNodeId: featureTreeNode };
    const sketchEntityObjects = new Map<string, THREE.Object3D>();
    const activeConstraints=(feature.sketch?.constraints??[]).filter((constraint)=>!constraint.suppressed);
    const conflicting=new Set(feature.sketch?.solve.conflictingConstraintIds??[]);
    const conflictEntities=new Set<string>();
    let changed=true;while(changed){changed=false;for(const constraint of activeConstraints){
      const ids=constraint.references.flatMap((reference)=>reference.entityId?[reference.entityId]:[]);
      if(conflicting.has(constraint.id)||ids.some((id)=>conflictEntities.has(id))){
        if(!conflicting.has(constraint.id)){conflicting.add(constraint.id);changed=true;}
        for(const id of ids)if(!conflictEntities.has(id)){conflictEntities.add(id);changed=true;}
      }
    }}
    const patternMembers=sourceView?.sketchPatternMembers?.[feature.id]??[];
    const patternMemberIDs=new Set(patternMembers.map(entity=>entity.id));
    const patternSeeds=new Set(feature.sketch?.patterns?.filter(pattern=>!pattern.suppressed).flatMap(pattern=>pattern.entityIds)??[]);
    for (const entity of [...feature.sketch?.entities ?? [],...patternMembers]) {
      if (entity.suppressed || this.activeSketchID!==feature.id && patternSeeds.has(entity.id)) continue;
      const type = entity.kind === "POINT" ? "POINT" as const : "CURVE" as const;
      const entitySelection = { kind: "visual" as const, id: `${context.occurrencePath || "root"}:${feature.id}:${entity.id}`, visualType: type,
        featureId: feature.id, entityId: entity.id, role: entity.role, patternId:patternMemberIDs.has(entity.id)?entity.createdByOperationId:undefined,associatedSourceEntityId:patternMemberIDs.has(entity.id)?entity.sourceEntityId:undefined,sketchReference:entity.kind==="POINT"?{target:"ENTITY" as const,entityId:entity.id,subElement:"POINT" as const}:undefined,documentId, ownerDocumentId:documentId,
        bodyId: feature.bodyId, versionId: context.versionId, instancePath: context.instancePath,
        contextVariantKey: context.contextVariantKey,
        occurrencePath: context.occurrencePath,
        treeNodeId: this.featureTreeNode(context,entity.id)??`${featureTreeNode}/geometry/entity:${entity.id}` };
      let object: THREE.Object3D | undefined;
	  const diagnosticStatus=patternEntityStatus(feature.sketch!,entity);
      const entityColor=conflictEntities.has(entity.sourceEntityId??entity.id)?CATIA_VISUAL_THEME.sketchInvalid:sketchDiagnosticColor(diagnosticStatus, entity.role === "CONSTRUCTION");
      if (entity.kind === "POINT" && entity.point) {
        object = new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(plane, [entity.point.x, entity.point.y])]),
          this.materials.point(entityColor, 9, false));
        object.renderOrder = entity.role === "CONSTRUCTION" ? SKETCH_FEEDBACK_ORDER.constructionEndpoint : SKETCH_FEEDBACK_ORDER.endpoint;
      } else {
        const sampled=sampleSketchEntity(entity);
        if(sampled.length<2)continue;
        const positions=sampled.map((point)=>localToWorld(plane,point));
        object = makeSketchOverlayLine(positions, entityColor, entity.role === "CONSTRUCTION" ? 2 : 2.5, entity.role === "CONSTRUCTION");
        updateHighlightLineResolution(object, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
        object.renderOrder = entity.role === "CONSTRUCTION" ? SKETCH_FEEDBACK_ORDER.construction : SKETCH_FEEDBACK_ORDER.geometry;
        if (entity.kind === "SPLINE") group.add(makeSplineControlFeedback(
          splineEditablePoints(entity).map(p => [p.x, p.y]), entity.mode ?? "FIT",
          point => localToWorld(plane, point), this.materials,
          {width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight}, CATIA_VISUAL_THEME.vertex));
        const markerPoints:Vec2[] = entity.kind==="SPLINE"?[]
          :["CIRCLE","ELLIPSE"].includes(entity.kind)?[]:[worldToLocal(plane,positions[0]),worldToLocal(plane,positions.at(-1)!)];
        if(entity.center&&["CIRCLE","ARC","ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind))markerPoints.push([entity.center.x,entity.center.y]);
        if(entity.center&&["ELLIPSE","ELLIPTICAL_ARC"].includes(entity.kind)){
          const focal=Math.sqrt(entity.majorRadius!**2-entity.minorRadius!**2),c=Math.cos(entity.rotation??0),s=Math.sin(entity.rotation??0);
          markerPoints.push([entity.center.x+focal*c,entity.center.y+focal*s],[entity.center.x-focal*c,entity.center.y-focal*s]);
          for(const angle of [0,Math.PI/2]){const point=ellipsePoint(entity,angle);if(point)markerPoints.push(point);}
        }
        const markers=markerPoints.map(point=>localToWorld(plane,point));
        const endpointMarkers = new THREE.Points(new THREE.BufferGeometry().setFromPoints(markers),
          this.materials.point(CATIA_VISUAL_THEME.vertex, 8, false));
        endpointMarkers.userData = { ...entitySelection, sketchEntityOverlay: true,patternMember:patternMemberIDs.has(entity.id) }; endpointMarkers.renderOrder = entity.role === "CONSTRUCTION" ? SKETCH_FEEDBACK_ORDER.constructionEndpoint : SKETCH_FEEDBACK_ORDER.endpoint; group.add(endpointMarkers);
        this.selectionIndex.registerPick(endpointMarkers,hit=>{
          if(this.featureSelection?.role!=="point")return null;
          const index=hit.index??0;
          const subElement=(["LINE","ARC","ELLIPTICAL_ARC"].includes(entity.kind)&&index<2)?(index===0?"START":"END"):entity.center?"CENTER":undefined;
          if(!subElement||patternMemberIDs.has(entity.id))return null;
          return {...entitySelection,visualType:"POINT",sketchReference:{target:"ENTITY",entityId:entity.id,subElement}};
        },80);
      }
      if (!object) continue;
      object.userData = { ...entitySelection, sketchEntityOverlay: true,patternMember:patternMemberIDs.has(entity.id) }; group.add(object);
      sketchEntityObjects.set(entity.id, object);
      // Associated members are rendered from the accepted server projection;
      // only their seed participates in solver-driven editing and drag inputs.
      if(patternMemberIDs.has(entity.id)){
        object.renderOrder-=1;
        this.selectionIndex.associate({kind:"feature",id:entity.createdByOperationId!,entityId:entity.createdByOperationId,documentId,ownerDocumentId:documentId,bodyId:feature.bodyId,versionId:context.versionId,occurrencePath:context.occurrencePath},object);
      }
      this.selectable.set(`visual:${entitySelection.id}`, object);
      this.selectionIndex.register(entitySelection, object);
      this.selectionIndex.registerPick(object, () => entitySelection, (type === "POINT" ? 75 : 70)+sketchGeometryPickPriority(entity.role));
    }
    // Locations are computed by the server using the production Profile
    // graph. Displaying them never creates or guesses endpoint connectivity.
    if (this.activeSketchID === feature.id) {
      const feedback = sketchProfileFeedback(sourceView?.sketchAnalyses?.[feature.id]);
      const locations = feedback.locations.map(position => localToWorld(plane, position));
      if (locations.length) {
        const markers = new THREE.Points(new THREE.BufferGeometry().setFromPoints(locations), this.materials.point(CATIA_VISUAL_THEME.sketchInvalid, 14, false));
        markers.renderOrder = 85; markers.userData.sketchAnalysisIssue = true; group.add(markers);
      }
    }
    const externalEntities = (feature.sketch?.externalGeometry ?? []).flatMap((external) => {
      const snapshot = external.snapshot;
      if (!snapshot) return [];
      const entity: SketchEntity = { id: external.id, kind: snapshot.kind, role: "CONSTRUCTION",
        point: snapshot.point, start: snapshot.start, end: snapshot.end, center: snapshot.center, radius: snapshot.radius };
      const type = snapshot.kind === "POINT" ? "POINT" as const : "CURVE" as const;
      const selection = { kind: "visual" as const, id: `${context.occurrencePath || "root"}:${feature.id}:${external.id}`, visualType: type,
        featureId: feature.id, entityId: external.id, role: "CONSTRUCTION" as const, documentId,
        bodyId: feature.bodyId, versionId: context.versionId, instancePath: context.instancePath,
        contextVariantKey: context.contextVariantKey,
        occurrencePath: context.occurrencePath,
        treeNodeId: `${featureTreeNode}/external-geometry/external:${external.id}` };
      let object: THREE.Object3D | undefined;
      const color = external.status === "CONNECTED" ? CATIA_VISUAL_THEME.sketchExternal : CATIA_VISUAL_THEME.sketchInvalid;
      if (snapshot.kind === "POINT" && snapshot.point) {
        object = new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(plane, [snapshot.point.x, snapshot.point.y])]),
          this.materials.point(color, 10, false));
      } else {
        const sampled = sampleSketchEntity(entity);
        if (sampled.length >= 2) {
          object = makeSketchOverlayLine(sampled.map((point) => localToWorld(plane, point)), color, 2.75, true);
          updateHighlightLineResolution(object, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
        }
      }
      if (object) {
        object.renderOrder = SKETCH_FEEDBACK_ORDER.construction; object.userData = { ...selection, sketchEntityOverlay: true }; group.add(object);
        sketchEntityObjects.set(external.id, object); this.selectable.set(`visual:${selection.id}`, object);
        this.selectionIndex.register(selection, object); this.selectionIndex.registerPick(object, () => selection, type === "POINT" ? 65 : 60);
      }
      return [entity];
    });
    for (const constraint of feature.sketch?.constraints ?? []) {
      if (constraint.suppressed) continue;
      if (this.dimensionDrag?.selection.featureId === feature.id && this.dimensionDrag.constraint.id === constraint.id) continue;
      const constraintSelection = { kind: "sketch-constraint" as const,
        id: `${context.occurrencePath || "root"}:${feature.id}:constraint:${constraint.id}`, featureId: feature.id,
        constraintId: constraint.id, constraintType: constraint.kind, documentId,
        bodyId: feature.bodyId, versionId: context.versionId, instancePath: context.instancePath,
        contextVariantKey: context.contextVariantKey,
        occurrencePath: context.occurrencePath,
        treeNodeId: constraintTreeNodeID(featureTreeNode, constraint.kind, constraint.id) };
      const constraintGroup = makeSketchConstraintRenderable(this.displaySketchConstraint(documentId,feature.id,constraint), [...(feature.sketch?.entities ?? []), ...externalEntities],
        (point) => localToWorld(plane, point), this.materials,
        { width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight },
        feature.sketch?.solve.conflictingConstraintIds?.includes(constraint.id) ? CATIA_VISUAL_THEME.sketchInvalid
          : feature.sketch?.solve.redundantConstraintIds?.includes(constraint.id) ? CATIA_VISUAL_THEME.sketchRedundant : undefined);
      if (constraintGroup.children.length === 0) continue;
      constraintGroup.userData = constraintSelection;
      constraintGroup.traverse((child) => { child.userData = {...child.userData,...constraintSelection};
        if (child !== constraintGroup && (!isDimensionConstraintKind(constraint.kind) || child.userData.sketchDimensionLabel || (child as THREE.Points).isPoints)) this.selectionIndex.registerPick(child, () => constraintSelection, 90); });
      group.add(constraintGroup);
      this.selectable.set(`sketch-constraint:${constraintSelection.id}`, constraintGroup);
      this.selectionIndex.register(constraintSelection, constraintGroup);
      for (const reference of constraint.references) {
        if (!reference.entityId) continue;
        const related = sketchEntityObjects.get(reference.entityId);
        if (related) this.selectionIndex.associate(constraintSelection, related);
      }
    }
    group.userData = { ...sketchSelection, sketchFeatureID: feature.id, sketchEditOverlay: true, visibleOutsideSketchEdit };
    this.helpers.add(group);
    this.selectable.set(`sketch:${feature.id}`, group);
    this.selectionIndex.register(sketchSelection, group);
  }

  private makeFailedBody(artifact: Artifact, visible: boolean, context: SolidContext): THREE.Group {
    const group = new THREE.Group(); group.visible = visible;
    group.userData = { ...context, kind: "body", id: context.occurrencePath ? `${context.occurrencePath}:body:${context.bodyId}` : context.bodyId, displayFallback: true };
    const mesh = new THREE.Mesh(makeGeometry(artifact), this.materials.surface(0xb86151));
    mesh.material.transparent = true; mesh.material.opacity = .6;
    mesh.material.depthWrite = false; mesh.material.roughness = .85;
    // Camera navigation may use the display, modeling picks cannot.
    mesh.raycast = acceleratedRaycast; markNavigationPickable(mesh);
    group.add(mesh); return group;
  }

  private topologyAssociationContext(artifact:Artifact,context:SolidContext,kind:string,id:number):Pick<SelectionItem,"associatedFeatureIds"|"sourceFeatureIds"|"treeNodeId"|"patternId"|"patternMemberSlot"> {
    const association=topologyFeatureAssociation(artifact.visualization?.featureAssociations,kind,id);
    if(!association)return {};
    const primary=association.primary??[];
    const member=association.members?.length===1?association.members[0]:undefined;
    return {associatedFeatureIds:primary,sourceFeatureIds:association.origins,treeNodeId:primary.length===1?this.featureTreeNode(context,primary[0])??context.treeNodeId:context.treeNodeId,
      patternId:member?.patternId,patternMemberSlot:member?.slot};
  }

  private makeSolid(artifact: Artifact, color: number, context: SolidContext): THREE.Group {
    context={...context,displayStageFeatureId:artifact.displayStageFeatureId};
    const geometry = makeGeometry(artifact);
    geometry.userData.navigationFaceIds = artifact.mesh.faceIds;
    const group = new THREE.Group();
    const mesh = new THREE.Mesh(geometry, this.materials.surface(color));
    mesh.raycast = acceleratedRaycast;
    markNavigationPickable(mesh);
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    const bodySelection = { kind: "body" as const, id: context.occurrencePath ? `${context.occurrencePath}:body:${context.bodyId}` : context.bodyId ?? "body", ...context };
    group.userData = bodySelection;
    this.selectionIndex.register(bodySelection, group);
    this.selectionIndex.registerVisualKey(`body:${bodySelection.id}`, group);
    const occurrenceParts = context.occurrencePath.split("/").filter(Boolean);
    for (let length = 1; length <= occurrenceParts.length; length++) {
      this.selectionIndex.registerVisualKey(`occurrence:${occurrenceParts.slice(0, length).join("/")}`, group);
    }
    this.selectable.set(`body:${bodySelection.id}`, group);
    if(!artifact.historicalPreview)this.selectionIndex.registerPick(mesh, (hit) => {
      const triangle = hit.faceIndex ?? -1;
      const localID = triangle >= 0 ? (artifact.mesh.faceIds[triangle] ?? 0) : 0;
      return localID > 0 ? {
        kind: "face", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:face:${localID}`,
        topologyId: localID, ...context, ...this.topologyAssociationContext(artifact,context,"face",localID)
      } : bodySelection;
    }, 20);
    group.add(mesh);
    const edgePositions: number[] = [];
    const edgeIDs: number[] = [];
    for (const edge of artifact.mesh.edges ?? []) {
      for (let index = 1; index < edge.points.length; index++) {
        edgePositions.push(...edge.points[index - 1], ...edge.points[index]); edgeIDs.push(edge.localId);
      }
    }
    if (edgePositions.length > 0) {
      const edgeGeometry = new THREE.BufferGeometry();
      edgeGeometry.setAttribute("position", new THREE.Float32BufferAttribute(edgePositions, 3));
      edgeGeometry.userData.navigationEdgeIds = edgeIDs;
      const edges = new THREE.LineSegments(edgeGeometry, this.materials.edge());
      markNavigationPickable(edges);
      if(!artifact.historicalPreview)this.selectionIndex.registerPick(edges, (hit) => {
        const segmentIndex = ((hit.index ?? 0) / 2) | 0;
        const localID = edgeIDs[segmentIndex] ?? 0;
        return {
          kind: "edge", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:edge:${localID}`,
          topologyId: localID, ...context, ...this.topologyAssociationContext(artifact,context,"edge",localID)
        };
      }, 40);
      group.add(edges);
    } else {
      group.add(new THREE.LineSegments(makeFeatureEdges(geometry), this.materials.edge()));
    }
    const topologyVertices = artifact.mesh.topologyVertices ?? [];
    if (topologyVertices.length > 0) {
      const pointGeometry = new THREE.BufferGeometry();
      pointGeometry.setAttribute("position", new THREE.Float32BufferAttribute(topologyVertices.flatMap((item) => item.point), 3));
      const points = new THREE.Points(pointGeometry, this.materials.point(CATIA_VISUAL_THEME.vertex, 7));
      markNavigationPickable(points);
      if(!artifact.historicalPreview)this.selectionIndex.registerPick(points, (hit) => {
        const localID = topologyVertices[hit.index ?? 0]?.localId ?? 0;
        return {
          kind: "vertex", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:vertex:${localID}`,
          topologyId: localID, ...context, ...this.topologyAssociationContext(artifact,context,"vertex",localID)
        };
      }, 50);
      group.add(points);
    }
    applySolidDisplaySettings(group, this.solidDisplay);
    this.solidBindings.set(`${context.occurrencePath || "root"}/body:${context.bodyId}`, { group, mesh, artifact, context });
    return group;
  }

  setDatumPreview(preview?: DatumPreview): void {
    this.datumPreview = preview;
    if (this.datumPreviewGroup) {
      this.datumPreviewGroup.traverse(child => this.screenStableReferences.delete(child));
      this.disposeGroup(this.datumPreviewGroup); this.datumPreviewGroup.removeFromParent();
      this.datumPreviewGroup = undefined;
    }
    if (preview) {
      const placement = new THREE.Group();
      placement.position.fromArray(preview.translation ?? [0,0,0]);
      placement.quaternion.fromArray(preview.rotation ?? [0,0,0,1]);
      let reference: THREE.Group | THREE.Mesh | undefined;
      if (preview.axis) reference = makeDatumAxisReference(preview.axis, CATIA_VISUAL_THEME.commandPreview);
      if (preview.plane) {
        const p = preview.plane, normal = new THREE.Vector3().fromArray(p.normal), u = new THREE.Vector3().fromArray(p.uDirection);
        reference = new THREE.Mesh(new THREE.PlaneGeometry(1.5,1.5), this.materials.datumPlane(CATIA_VISUAL_THEME.commandPreview));
        reference.position.fromArray(p.origin);
        reference.quaternion.setFromRotationMatrix(new THREE.Matrix4().makeBasis(u, normal.clone().cross(u), normal));
        reference.renderOrder = 95;
      }
      if (reference) { placement.add(reference); this.helpers.add(placement); this.screenStableReferences.set(reference, 90); this.datumPreviewGroup = placement; }
      updateHighlightLineResolution(placement,this.renderer.domElement.clientWidth,this.renderer.domElement.clientHeight);
    }
    this.emitDebugState(); this.invalidate();
  }

  setFeatureSelection(session?: FeatureSelectionSession): void {
    if(this.releaseFeatureSketchPick){this.releaseFeatureSketchPick();this.releaseFeatureSketchPick=undefined;this.sketchModal.setOpen(false);}

    if(this.featureSelection?.localSketchId&&!session?.localSketchId)this.clearReferenceHover();
    this.featureSelection = session;
    if(this.loftConnections){this.loftConnections.removeFromParent();this.disposeGroup(this.loftConnections);this.loftConnections=undefined;}
    if(session?.connectionLines?.length){
      const group=new THREE.Group();
      // Points belong to the preview Part frame, including occurrence placement.
      if(session.occurrencePath && session.occurrencePath===this.editContext?.occurrencePath){
        group.position.fromArray(this.editContext.translation??[0,0,0]);
        group.quaternion.fromArray(this.editContext.rotation??[0,0,0,1]);
      }
      for(const line of session.connectionLines){
        const points=line.map(p=>new THREE.Vector3(...p));
        group.add(makeSketchOverlayLine(points,0x418de8,1.5,true));
      }
      this.helpers.add(group);this.loftConnections=group;
      updateHighlightLineResolution(group,this.renderer.domElement.clientWidth,this.renderer.domElement.clientHeight);
    }

    this.preselect(null);
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.refreshInteractionHighlights();
    if(session?.localSketchId){
      const selection=(reference:SketchGeometryRef):SelectionItem=>({kind:"visual",id:`${session.localSketchId}:${reference.entityId??reference.target}:${reference.subElement}`,featureId:session.localSketchId!,entityId:reference.entityId??reference.target,visualType:session.role==="point"?"POINT":"CURVE",sketchReference:reference,documentId:session.documentId,versionId:session.versionId,occurrencePath:session.occurrencePath});
      this.releaseFeatureSketchPick=this.sketchModal.begin({featureId:session.localSketchId,slot:0,pick:session.role==="point"?"POINT":session.role==="axis"?"LINE":"ENTITY",references:session.selections.flatMap(s=>s.sketchReference?[s.sketchReference]:[]),hoverOverridesRetained:true,
        allowed:reference=>session.role==="geometry"?reference.target==="ENTITY":session.role!=="point"||["POINT","START","END","CENTER"].includes(reference.subElement),
        onCandidate:reference=>{session.onPick(selection(reference));this.preselect(null,true);return false;},
        onPreview:reference=>this.preselect(reference?selection(reference):null,true),onCancel:()=>this.preselect(null,true)});
    }
    this.refreshInteractionHighlights();
    this.emitDebugState();
    this.invalidate();
  }

  private pick(x: number, y: number, additive: boolean): void {
    if (this.moveManipulator.isDragging()) return;
    const hit = this.hitTest(x, y,true);
    if (this.featureSelection) { if (hit) this.featureSelection.onPick(hit); return; }
    if (!hit) { if (!additive) this.selectMany([]); return; }
    if (!additive) { this.selectMany([hit]); return; }
    const key = selectionKey(hit);
    this.selectMany(this.selected.some((selection) => selectionKey(selection) === key)
      ? this.selected.filter((selection) => selectionKey(selection) !== key) : [...this.selected, hit]);
  }

  private sketchMarquee?:HTMLDivElement;
  private clearSketchMarquee():void {this.sketchMarquee?.remove();this.sketchMarquee=undefined;}
  private showSketchMarquee(from:SketchScreenPoint,to:SketchScreenPoint):void {
    const rectangle=this.sketchMarquee??=document.createElement("div");
    rectangle.setAttribute("aria-hidden","true");rectangle.dataset.sketchMarquee="true";
    Object.assign(rectangle.style,{position:"absolute",pointerEvents:"none",zIndex:"5",border:`1px ${to.x<from.x?"dashed":"solid"} #368bd6`,background:"rgba(54,139,214,0.08)",left:`${Math.min(from.x,to.x)}px`,top:`${Math.min(from.y,to.y)}px`,width:`${Math.abs(to.x-from.x)}px`,height:`${Math.abs(to.y-from.y)}px`});
    if(!rectangle.parentElement)this.host.append(rectangle);
  }
  private selectSketchMarquee(from:SketchScreenPoint,to:SketchScreenPoint,additive:boolean):void {
    if(!this.sketchPlane||!this.activeSketchID||this.pendingVisualSnapshot||this.activeToolID!=="select")return;
    this.camera.updateMatrixWorld();const metrics=viewportMetrics(this.renderer);
    const selections=this.visibleSketchDeletionEntities().filter(entity=>{
      const selection=this.sketchEntitySelection(entity);
      if(!allowsSelectionInContext(this.captureSettings,selection,this.activeSketchID))return false;
      const points=sampleSketchEntity(entity).map(point=>{
        const screen=localToWorld(this.sketchPlane!,point).project(this.camera);
        return [(screen.x+1)*metrics.cssWidth/2,(1-screen.y)*metrics.cssHeight/2] as Vec2;
      });
      return sketchMarqueeContains(points,from,to);
    }).map(entity=>this.sketchEntitySelection(entity));
    this.selectMany(additive?[...this.selected,...selections]:selections,true,"selection");
  }

  private preselectAt(x: number, y: number): void {
    if (this.moveManipulator.isDragging() || this.navigation.activeAction !== "none") return;
    if (this.activeToolID === "select") this.clearSnapPreview();
    this.preselect(this.hitTest(x, y), true);
  }

  private hitTest(x: number, y: number, captureManipulatorAnchor = false): Selection {
    if (this.pendingVisualSnapshot) return null;
    this.updateScreenStableReferences();
    const metrics = viewportMetrics(this.renderer);
    updateScreenLines(this.scene, this.camera, metrics.cssWidth, metrics.cssHeight);
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const worldPerPixel = worldUnitsPerCssPixel(this.camera, this.navigation.target, viewportMetrics(this.renderer));
    this.raycaster.params.Line = { threshold: worldPerPixel * 5 };
    this.raycaster.params.Points = { threshold: worldPerPixel * 7 };
    this.datumAxisPickToleranceWorld = worldPerPixel * 1.75;
    if(this.activeSketchID&&this.featureSelection&&(this.featureSelection.role==="point"||this.featureSelection.role==="axis")){
      const role=this.featureSelection.role,ref=this.sketchReferenceAt(x,y,role==="point"?"POINT":"LINE");
      const view=this.sketchView();
      if(ref&&view)return {kind:"visual",id:`${this.activeSketchID}:${ref.entityId??ref.target}:${ref.subElement}`,featureId:this.activeSketchID,entityId:ref.entityId??ref.target,visualType:role==="point"?"POINT":"CURVE",sketchReference:ref,documentId:view.document.id,versionId:view.document.versionId,occurrencePath:this.editContext?.occurrencePath};
      if(this.featureSelection.localSketchId)return null;
    }
    if(this.activeSketchID&&this.featureSelection?.localSketchId&&this.featureSelection.role==="geometry"){
      return featureSelectionHit(this.sketchEntitySelectionAt(x,y),this.featureSelection);
    }
    if(this.activeSketchID&&!this.featureSelection&&this.activeToolID!=="sketch.project"){
      const view=this.sketchView(),feature=view?.part?.features.find(feature=>feature.id===this.activeSketchID);
      const current=(selection:SelectionItem)=>"featureId" in selection&&selection.featureId===this.activeSketchID&&(selection.ownerDocumentId??selection.documentId)===view?.document.id&&(selection.occurrencePath??"")===(this.editContext?.occurrencePath??"")&&(!selection.versionId||selection.versionId===view?.document.versionId)&&(!selection.bodyId||selection.bodyId===feature?.bodyId)&&allowsSelectionInContext(this.captureSettings,selection,this.activeSketchID);
      const marker=this.selectionIndex.pick(this.raycaster,selection=>selection.kind==="sketch-constraint"&&current(selection));
      if(marker)return marker;
      const entity=this.sketchEntitySelectionAt(x,y);
      if(entity&&current(entity))return entity;
      return this.selectionIndex.pick(this.raycaster,selection=>!!selection.associatedSourceEntityId&&current(selection));
    }
    const hit = this.selectionIndex.pickWithIntersection(this.raycaster, (selection) =>
      this.featureSelection ? !!featureSelectionHit(selection, this.featureSelection) :
      this.activeToolID === "sketch.project" && (selection.kind === "edge" || selection.kind === "vertex")
        ? allowsSelection(this.captureSettings, selection)
        : allowsSelectionInContext(this.captureSettings, selection, this.activeSketchID));
    const raw=hit.selection && bindPublicationSelection(hit.selection, this.view?.structureTree);
    if(captureManipulatorAnchor&&raw&&hit.intersection&&this.view)
      this.lastManipulatorPick={selection:raw,hit:hit.intersection,revisionId:this.view.document.versionId};
    if(captureManipulatorAnchor&&this.activeToolID==="assembly.move"&&raw?.instanceId&&hit.intersection){
      this.pendingManipulatorAnchor={instanceId:raw.instanceId,anchor:this.manipulatorAnchorFromIntersection(hit.intersection,raw)};
    }
    return this.featureSelection ? featureSelectionHit(raw, this.featureSelection) : this.selectionMode.project(projectSketchFeatureSelection(raw, this.activeSketchID));
  }

  private dimensionConstraintAt(x: number, y: number) {
    const selection = this.hitTest(x, y);
    const sketchView = this.sketchView();
    if (!selection || selection.kind !== "sketch-constraint" || !sketchView) return undefined;
    const feature = sketchView.part?.features.find((candidate) => candidate.id === selection.featureId);
    const constraint = feature?.sketch?.constraints.find((candidate) => candidate.id === selection.constraintId);
    if (!constraint || !isDimensionConstraintKind(constraint.kind)) return undefined;
    return { selection, constraint, feature };
  }

  private rawSketchPoint(x: number, y: number): Vec2 | undefined {
    if (!this.sketchPlane) return undefined;
    this.updatePointer(x, y); this.raycaster.setFromCamera(this.pointer, this.camera);
    const world = this.raycaster.ray.intersectPlane(rayPlane(this.sketchPlane), new THREE.Vector3());
    return world ? worldToLocal(this.sketchPlane, world) : undefined;
  }

  private displaySketchConstraint(documentId:string,sketchId:string,constraint:SketchConstraint):SketchConstraint {
    const position=useUIPreferences.getState().sketchLabelPositions[sketchLabelPositionKey(documentId,sketchId,constraint.id)];
    return position?{...constraint,labelPosition:position}:constraint;
  }
  private dimensionGestureScope():string {const view=this.sketchView();return JSON.stringify([view?.document.id,view?.document.versionId,this.activeSketchID,this.editContext?.occurrencePath]);}

  private beginDimensionDrag(x: number, y: number): boolean {
    if (!this.sketchPlane || !this.activeSketchID) return false;
    const hit = this.dimensionConstraintAt(x, y);
    if (!hit || hit.selection.featureId !== this.activeSketchID) return false;
    this.selectMany([hit.selection]);
    const root = this.selectable.get(`sketch-constraint:${hit.selection.id}`);
    this.dimensionDrag = { selection: hit.selection, constraint: hit.constraint, root, startX: x, startY: y,scopeKey:this.dimensionGestureScope() };
    return true;
  }

  private updateDimensionDrag(x: number, y: number): void {
    const sketchView = this.sketchView();
    if (!this.dimensionDrag || !this.sketchPlane || !sketchView) return;
    if(this.dimensionDrag.scopeKey!==this.dimensionGestureScope()){this.cancelDimensionDrag();return;}
    if (Math.hypot(x - this.dimensionDrag.startX, y - this.dimensionDrag.startY) < 3 && !this.dimensionDrag.position) return;
    const position = this.rawSketchPoint(x, y); if (!position||!position.every(Number.isFinite)){this.cancelDimensionDrag();return;}
    if (!this.dimensionDrag.position) {
      this.selectMany([]);
      const root = this.dimensionDrag.root;
      const parent = root?.parent;
      if (root && parent) {
        this.dimensionDrag.rootParent = parent;
        this.dimensionDrag.rootIndex = parent.children.indexOf(root);
        parent.remove(root);
      }
    }
    this.dimensionDrag.position = position;
    this.clearReferencePreview();
    const feature = sketchView.part?.features.find((candidate) => candidate.id === this.dimensionDrag!.selection.featureId);
    if (!feature?.sketch) return;
    const previewConstraint = { ...this.dimensionDrag.constraint, labelPosition: { x: position[0], y: position[1] } };
    this.referencePreview = makeSketchConstraintRenderable(previewConstraint, sketchReferenceEntities(feature),
      (point) => localToWorld(this.sketchPlane!, point), this.materials,
      { width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight });
    this.scene.add(this.referencePreview); this.invalidate();
  }

  private finishDimensionDrag(): void {
    if(this.dimensionDrag&&this.dimensionDrag.scopeKey!==this.dimensionGestureScope()){this.cancelDimensionDrag();return;}
    const drag = this.dimensionDrag; this.dimensionDrag = undefined;
    if (!drag) return;
    this.clearReferencePreview();
    if (drag.position) {
      const view=this.sketchView(),feature=view?.part?.features.find(feature=>feature.id===drag.selection.featureId);
      if(view&&feature?.sketch&&this.sketchPlane){
        useUIPreferences.getState().setSketchLabelPosition(view.document.id,feature.id,drag.constraint.id,{x:drag.position[0],y:drag.position[1]});
        const replacement=makeSketchConstraintRenderable(this.displaySketchConstraint(view.document.id,feature.id,drag.constraint),sketchReferenceEntities(feature),point=>localToWorld(this.sketchPlane!,point),this.materials,{width:this.renderer.domElement.clientWidth,height:this.renderer.domElement.clientHeight},
          feature.sketch.solve.conflictingConstraintIds?.includes(drag.constraint.id)?CATIA_VISUAL_THEME.sketchInvalid:feature.sketch.solve.redundantConstraintIds?.includes(drag.constraint.id)?CATIA_VISUAL_THEME.sketchRedundant:undefined);
        replacement.userData=drag.selection;
        const related=this.selectionIndex.objectsFor(drag.selection).filter(object=>object!==drag.root);
        if(drag.root)this.selectionIndex.unregister(drag.selection,drag.root);
        replacement.traverse(child=>{
          child.userData={...child.userData,...drag.selection};
          if(child!==replacement&&(child.userData.sketchDimensionLabel||(child as THREE.Points).isPoints))this.selectionIndex.registerPick(child,()=>drag.selection,90);
        });
        this.selectionIndex.register(drag.selection,replacement);
        for(const object of related)this.selectionIndex.associate(drag.selection,object);
        const parent=drag.rootParent??drag.root?.parent;
        parent?.add(replacement);
        this.selectable.set(`sketch-constraint:${drag.selection.id}`,replacement);
        if(drag.root){drag.root.removeFromParent();this.disposeRenderable(drag.root);}
      }else if(drag.root&&drag.rootParent)drag.rootParent.add(drag.root);
    }
    this.invalidate();
  }

  private cancelDimensionDrag(): void {
    const drag = this.dimensionDrag;
    if (drag?.root && drag.rootParent && !drag.root.parent) {
      drag.rootParent.add(drag.root);
      const currentIndex = drag.rootParent.children.indexOf(drag.root);
      const targetIndex = Math.max(0, Math.min(drag.rootIndex ?? currentIndex, drag.rootParent.children.length - 1));
      drag.rootParent.children.splice(currentIndex, 1);
      drag.rootParent.children.splice(targetIndex, 0, drag.root);
    }
    this.dimensionDrag = undefined; this.clearReferencePreview(); this.invalidate();
  }

  private editDimensionAt(x: number, y: number): boolean {
    const hit = this.dimensionConstraintAt(x, y);
    if (!hit || hit.selection.featureId !== this.activeSketchID ||
      (hit.constraint.unit !== "mm" && hit.constraint.unit !== "deg")) return false;
    return this.requestDimensionEdit(hit.selection, x, y);
  }

  private replaceTopologyOverlays(layer: "selected" | "preselected", selections: readonly SelectionItem[]): void {
    const property = layer === "selected" ? "selectedOverlays" : "preselectedOverlays";
    for (const previous of this[property]) {
      previous.parent?.remove(previous);
      this.disposeRenderable(previous);
    }
    this[property] = [];
    for (const selection of selections) this.addTopologyOverlay(layer, selection);
  }

  private addTopologyOverlay(layer: "selected" | "preselected", selection: SelectionItem, ids?:ReadonlySet<number>): void {
    if(["pad","feature","import"].includes(selection.kind)) {
      const binding=[...this.solidBindings.values()].find(b=>sameDisplayContext(selection,b.context));
      if(!binding)return;
      const contribution=featureContribution(binding.artifact.visualization?.featureAssociations,selection.entityId??selection.id);
      for(const kind of ["face","edge"] as const) {
        const locators=new Set(contribution.elements.filter(e=>e.kind===kind.toUpperCase()).map(e=>e.localId));
        if(locators.size)this.addTopologyOverlay(layer,{...selection,kind,topologyId:[...locators][0]},locators);
      }
      return;
    }
    if(selection.kind==="visual"&&this.activeSketchID&&selection.featureId===this.activeSketchID){
      const owner=this.sketchView()?.document;
      const feature=this.sketchView()?.part?.features.find(feature=>feature.id===this.activeSketchID);
      if(selection.bodyId&&selection.bodyId!==feature?.bodyId)return;
      if((selection.ownerDocumentId??selection.documentId)!==owner?.id||(selection.occurrencePath??"")!==(this.editContext?.occurrencePath??"")||selection.versionId&&selection.versionId!==owner?.versionId)return;
      const entity=this.visibleSketchReferenceEntities().find(candidate=>candidate.id===selection.entityId);
      if(!entity)return;
      const overlay=this.makeReferencePreview({target:"ENTITY",entityId:entity.id,subElement:"WHOLE"},layer==="selected"?CATIA_VISUAL_THEME.selected:CATIA_VISUAL_THEME.hover);
      if(!overlay)return;
      overlay.traverse(child=>child.renderOrder=layer==="selected"?SKETCH_FEEDBACK_ORDER.selected:SKETCH_FEEDBACK_ORDER.hover);
      this.scene.add(overlay);(layer==="selected"?this.selectedOverlays:this.preselectedOverlays).push(overlay);return;
    }
    if(selection.kind==="sketch-constraint"){
      const constraint=this.sketchView()?.part?.features.find((feature)=>feature.id===selection.featureId)?.sketch?.constraints
        .find((candidate)=>candidate.id===selection.constraintId);
      const color=layer==="selected"?CATIA_VISUAL_THEME.selected:CATIA_VISUAL_THEME.hover;
      for(const reference of constraint?.references??[]){
        if(reference.target!=="SKETCH_X_AXIS"&&reference.target!=="SKETCH_Y_AXIS")continue;
        const overlay=this.makeReferencePreview(reference,color);if(!overlay)continue;
        overlay.renderOrder=layer==="selected"?102:101;this.scene.add(overlay);
        (layer==="selected"?this.selectedOverlays:this.preselectedOverlays).push(overlay);
      }
      return;
    }
    if (selection.kind !== "face" && selection.kind !== "edge" && selection.kind !== "vertex") return;
    const binding = [...this.solidBindings.values()].find(b => sameDisplayContext(selection,b.context));
    if (!binding || !selection.topologyId) return;
    const selectedIDs=ids??new Set([selection.topologyId]);
    const color = layer === "selected" ? CATIA_VISUAL_THEME.selected : CATIA_VISUAL_THEME.hover;
    let overlay: THREE.Object3D | undefined;
    if (selection.kind === "face") {
      const positions: number[] = [];
      binding.artifact.mesh.triangles.forEach((triangle, index) => {
        if (!selectedIDs.has(binding.artifact.mesh.faceIds[index] ?? -1)) return;
        for (const vertex of triangle) positions.push(...binding.artifact.mesh.vertices[vertex]);
      });
      if (positions.length > 0) {
        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute("position", new THREE.Float32BufferAttribute(positions, 3)); geometry.computeVertexNormals();
        overlay = new THREE.Mesh(geometry, new THREE.MeshBasicMaterial({
          color, transparent: true,
          opacity: layer === "selected" ? 0.48 : 0.3, side: THREE.DoubleSide, depthWrite: false,
          depthTest: false,
          polygonOffset: true, polygonOffsetFactor: -2, polygonOffsetUnits: -2
        }));
      }
    } else if (selection.kind === "edge") {
      const group=new THREE.Group();
      for(const edge of binding.artifact.mesh.edges??[])if(selectedIDs.has(edge.localId))group.add(makeOcclusionVisibleHighlightLine(
        edge.points.map(point=>new THREE.Vector3().fromArray(point)),color,layer==="selected"?5:4));
      if(group.children.length===1){overlay=group.children[0];group.remove(overlay);}else if(group.children.length)overlay=group;
    } else {
      const vertex = (binding.artifact.mesh.topologyVertices ?? []).find((item) => item.localId === selection.topologyId);
      if (vertex) {
        const geometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3().fromArray(vertex.point)]);
        overlay = new THREE.Points(geometry, this.materials.point(color, layer === "selected" ? 11 : 9, false));
      }
    }
    if (!overlay) return;
    overlay.renderOrder = layer === "selected" ? 102 : 101;
    updateHighlightLineResolution(overlay, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
    binding.group.add(overlay);
    (layer === "selected" ? this.selectedOverlays : this.preselectedOverlays).push(overlay);
  }

  private sketchPoint(x: number, y: number, geometryOnly=false): Vec2 | null {
    if (!this.sketchPlane) return null;
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const point = this.raycaster.ray.intersectPlane(rayPlane(this.sketchPlane), new THREE.Vector3());
    if (!point) { this.clearSnapPreview(); return null; }
    const raw = worldToLocal(this.sketchPlane, point);
    return this.snapSketchLocalPoint(raw, geometryOnly);
  }

  private snapSketchLocalPoint(raw: Vec2, geometryOnly=false, excludedEntityId?:string): Vec2 {
    if (!this.sketchPlane) return raw;
    const snapEntities = this.visibleSketchReferenceEntities().filter(entity=>entity.id!==excludedEntityId);
    const screen = (local: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, local).project(this.camera);
      return [(projected.x + 1) * this.renderer.domElement.clientWidth / 2,
        (1 - projected.y) * this.renderer.domElement.clientHeight / 2] as Vec2;
    };
    const first = screen(raw), second = screen([raw[0] + 1, raw[1]]);
    const pixelsPerUnit = Math.max(Math.hypot(second[0] - first[0], second[1] - first[1]), 1.0e-6);
    const snap = this.captureSettings.enabled
      ? resolveSketchSnap(raw, snapEntities, pixelsPerUnit, geometryOnly?1:adaptiveGridSpacing(this.camera, this.renderer.domElement.clientHeight),
        SKETCH_INPUT_POLICY.snapThresholdPixels, geometryOnly?this.captureSettings.sketch.filter(kind=>kind!=="GRID"&&kind!=="ORIGIN"):this.captureSettings.sketch, screen) : undefined;
    if(!geometryOnly)this.lastSketchSnap = snap;
    if (snap) this.showSnapPreview(snap, 8 / pixelsPerUnit); else this.clearSnapPreview();
    return snap?.point ?? raw;
  }

  private showSnapPreview(snap: SketchSnapResult, markerRadius: number): void {
    this.clearSnapPreview();
    if (!this.sketchPlane) return;
    const center = localToWorld(this.sketchPlane, snap.point);
    const group = new THREE.Group();
    const marker = new THREE.Points(new THREE.BufferGeometry().setFromPoints([center]),
      this.materials.point(CATIA_VISUAL_THEME.snap, snap.kind === "GRID" ? 19 : 16, false));
    marker.renderOrder = SKETCH_FEEDBACK_ORDER.snap;
    const ringPoints = Array.from({ length: 33 }, (_, index) => {
      const angle = index / 32 * Math.PI * 2;
      const radius = markerRadius * (snap.kind === "GRID" ? 1.15 : 1);
      return localToWorld(this.sketchPlane!, [snap.point[0] + Math.cos(angle) * radius, snap.point[1] + Math.sin(angle) * radius]);
    });
    const ring = new THREE.Line(new THREE.BufferGeometry().setFromPoints(ringPoints),
      new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.snap, depthTest: false, transparent: true, opacity: 0.92 }));
    ring.renderOrder = SKETCH_FEEDBACK_ORDER.snap;
    group.userData.snapKind = snap.kind;
    group.add(ring, marker); this.attachSketchPreview(group); this.snapPreview = group; this.invalidate();
  }

  private clearSnapPreview(): void {
    if (!this.snapPreview) return;
    this.snapPreview.removeFromParent(); this.disposeRenderable(this.snapPreview);
    this.snapPreview = undefined; this.invalidate();
  }

  private updatePointer(x: number, y: number): void {
    const width = Math.max(this.renderer.domElement.clientWidth, 1);
    const height = Math.max(this.renderer.domElement.clientHeight, 1);
    this.pointer.set((x / width) * 2 - 1, -(y / height) * 2 + 1);
  }

  private drawPreview(points2: Vec2[], closed: boolean, plane: PlaneName | SketchPlane): void {
    this.clearPreview();
    const localPoints = closed && points2.length > 0 ? [...points2, points2[0]] : points2;
    const points = localPoints.map((point) => localToWorld(plane, point));
    const group=new THREE.Group();
    const line = makeSketchOverlayLine(points,CATIA_VISUAL_THEME.preview,2.5);
    updateHighlightLineResolution(line,this.renderer.domElement.clientWidth,this.renderer.domElement.clientHeight);
    line.renderOrder=SKETCH_FEEDBACK_ORDER.preview;group.add(line);this.preview=group;this.attachSketchPreview(group);
    this.invalidate();
  }

  private drawPointPreview(point: Vec2, plane: PlaneName | SketchPlane): void {
    this.clearPreview();
    const group=new THREE.Group();const marker = new THREE.Points(
      new THREE.BufferGeometry().setFromPoints([localToWorld(plane, point)]),
      this.materials.point(CATIA_VISUAL_THEME.preview, 11, false),
    );
    marker.renderOrder = SKETCH_FEEDBACK_ORDER.preview;group.add(marker);this.preview=group;this.attachSketchPreview(group);
    this.invalidate();
  }

  private showSketchEditCandidate(candidate:SketchEditCandidatePreview):void {
    this.clearPreview();if(!this.sketchPlane)return;
    const group=new THREE.Group();
    const draw=(entities:SketchEntity[],color:number,dashed:boolean)=>{for(const entity of entities){
      const points=sampleSketchEntity(entity).map(point=>localToWorld(this.sketchPlane!,point));if(!points.length)continue;
      const line=points.length===1?new THREE.Points(new THREE.BufferGeometry().setFromPoints(points),this.materials.point(color,12,false)):makeSketchOverlayLine(points,color,dashed?4:2.75,dashed);
      line.renderOrder=dashed?SKETCH_FEEDBACK_ORDER.trim:SKETCH_FEEDBACK_ORDER.preview;
      updateHighlightLineResolution(line,this.renderer.domElement.clientWidth,this.renderer.domElement.clientHeight);group.add(line);
      if (!dashed && entity.kind === "SPLINE") group.add(makeSplineControlFeedback(
        splineEditablePoints(entity).map(p => [p.x, p.y]), entity.mode ?? "FIT",
        point => localToWorld(this.sketchPlane!, point), this.materials,
        {width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight}, color, SKETCH_FEEDBACK_ORDER.preview + 3));
    }};
    draw(candidate.entities,CATIA_VISUAL_THEME.preview,false);
    draw(candidate.hitEntities,0xe05252,true);
    (candidate.dimensions??[]).forEach((dimension,index)=>{
      const renderable=makeSketchConstraintRenderable(dimension,[...candidate.entities,...candidate.dimensionEntities??[],...this.visibleSketchReferenceEntities()],point=>localToWorld(this.sketchPlane!,point),this.materials,{width:this.renderer.domElement.clientWidth,height:this.renderer.domElement.clientHeight});
      renderable.traverse(child=>{if(child.userData.sketchDimensionLabel)child.userData.dimensionInputField=index;});group.add(renderable);
    });
    this.preview=group;this.attachSketchPreview(group);this.updateSketchParameterVisibility();this.invalidate();
  }
  private clearPreview(): void {
    if (!this.preview) return;
    this.preview.removeFromParent();
    this.disposeRenderable(this.preview);
    this.preview = undefined;
    this.invalidate();
  }

  private sketchParameterEditing=false;
  setSketchParameterEditing(editing:boolean):void {this.sketchParameterEditing=editing;this.updateSketchParameterVisibility();this.invalidate();}
  private updateSketchParameterVisibility():void {
    const field=this.sketchCommandState?.input?.fieldIndex??0;
    for(const root of [this.preview,this.referencePreview])root?.traverse(child=>{if(child.userData.sketchDimensionLabel)child.visible=!(this.sketchParameterEditing&&(child.userData.dimensionInputField??0)===field);});
  }
  private clearReferenceHover():void {
    if(!this.referenceHover)return;this.referenceHover.removeFromParent();this.disposeRenderable(this.referenceHover);this.referenceHover=undefined;this.invalidate();
  }
  private clearConstraintPreview(): void {
    if (!this.referencePreview) return;
    this.referencePreview.removeFromParent();
    this.disposeRenderable(this.referencePreview);
    this.referencePreview = undefined;
    this.invalidate();
  }

  private clearReferencePreview():void {this.clearReferenceHover();this.clearConstraintPreview();}

  private makeReferencePreview(reference: SketchGeometryRef, color: number): THREE.Object3D | undefined {
    if (!this.sketchPlane) return undefined;
    if (reference.target === "SKETCH_ORIGIN") {
      return new THREE.Points(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane, [0, 0])]),
        this.materials.point(color, 15, false),
      );
    }
    if (reference.target === "SKETCH_X_AXIS" || reference.target === "SKETCH_Y_AXIS") {
      const origin = localToWorld(this.sketchPlane, [0, 0]);
      const direction = localToWorld(this.sketchPlane, reference.target === "SKETCH_X_AXIS" ? [1, 0] : [0, 1]).sub(origin);
      const line = makeSketchReferenceAxis(origin, direction, new THREE.Color(color), 4);
      updateHighlightLineResolution(line, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
      return line;
    }
    const sketchView = this.sketchView();
    if (!sketchView || !reference.entityId) return undefined;
    const entity = sketchReferenceEntities(sketchView.part?.features.find((feature) => feature.id === this.activeSketchID))
      .find((candidate) => candidate.id === reference.entityId);
    if (!entity) return undefined;
    if (entity.kind === "POINT" && entity.point) {
      return new THREE.Points(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane, [entity.point.x, entity.point.y])]),
        this.materials.point(color, 15, false),
      );
    }
    if (["POINT", "START", "END", "CENTER"].includes(reference.subElement)) {
      const point = sketchEntityPoint(entity, reference.subElement as "POINT" | "START" | "END" | "CENTER");
      if (point) return new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane, point)]),
        this.materials.point(color, 15, false));
    }
    if(reference.subElement==="CONTROL"&&reference.controlPointIndex!==undefined&&entity.kind==="SPLINE"){
      const point=splineReferencePoint(entity,reference.controlPointIndex,reference.controlPointId);if(point)return new THREE.Points(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane,point)]),this.materials.point(color,15,false));
    }
    const sampled = sampleSketchEntity(entity);
    if (sampled.length < 2) return undefined;
    const line = makeOcclusionVisibleHighlightLine(sampled.map((point) => localToWorld(this.sketchPlane!, point)), color, 4);
    updateHighlightLineResolution(line, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
    return line;
  }

  private showReferencePreview(reference: SketchGeometryRef, retained: readonly SketchGeometryRef[] = [], slot?:number, hoverOverridesRetained=false): void {
    this.clearReferenceHover();
    if(hoverOverridesRetained)retained=retained.filter(item=>!(item.target===reference.target&&item.entityId===reference.entityId&&(item.subElement===reference.subElement||item.subElement==="WHOLE"||reference.subElement==="WHOLE")));
    const group = new THREE.Group();
    const same=retained.some((item)=>item.target===reference.target&&item.entityId===reference.entityId&&item.subElement===reference.subElement);
    for(const item of retained){const selected=this.makeReferencePreview(item,CATIA_VISUAL_THEME.selected);if(selected){selected.traverse(child=>child.renderOrder=SKETCH_FEEDBACK_ORDER.selected);group.add(selected);}}
    if(!same){const candidate=this.makeReferencePreview(reference,CATIA_VISUAL_THEME.hover);if(candidate){candidate.traverse(child=>child.renderOrder=SKETCH_FEEDBACK_ORDER.hover);group.add(candidate);}}

    if (group.children.length === 0) return;
    this.referenceHover = group;
    this.referenceHover.renderOrder = 0;
    this.attachSketchPreview(this.referenceHover);
    this.invalidate();
  }

  private showConstraintPreview(kind: ConstraintKind, references: readonly SketchGeometryRef[], value?: number, labelPosition?: Vec2): void {
    this.clearConstraintPreview();
    const sketchView = this.sketchView();
    if (!this.sketchPlane || !sketchView) return;
    const feature = sketchView.part?.features.find((candidate) => candidate.id === this.activeSketchID);
    if (!feature?.sketch) return;
    const group = new THREE.Group();
    for (const reference of references) {
      const highlight = this.makeReferencePreview(reference, CATIA_VISUAL_THEME.selected);
      if (highlight){highlight.traverse(child=>child.renderOrder=SKETCH_FEEDBACK_ORDER.selected);group.add(highlight);}
    }
    const constraint: SketchConstraint = {
      id: "constraint-preview", kind, references: [...references],
      ...(value === undefined ? {} : { value, unit: kind === "ANGLE" ? "deg" : "mm" }),
      ...(labelPosition ? { labelPosition: { x: labelPosition[0], y: labelPosition[1] } } : {}),
    };
    group.add(makeSketchConstraintRenderable(constraint, sketchReferenceEntities(feature),
      (point) => localToWorld(this.sketchPlane!, point), this.materials,
      { width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight }));
    this.referencePreview = group;
    this.attachSketchPreview(group);this.updateSketchParameterVisibility();
    this.invalidate();
  }

  private showSketchManipulator(origin:Vec2,mode:"translate"|"rotate"|"both",changed:(value:{translation:Vec2;angle:number;origin:Vec2;finish?:boolean})=>void):void {
    if(!this.sketchPlane)return;
    if(!this.sketchManipulator){
      const notify=(finish?:boolean)=>{
        if(!this.sketchPlane||!this.sketchManipulator)return;
        const candidate=this.sketchManipulator.candidatePose(),position=worldToLocal(this.sketchPlane,candidate.position);
        const normal=localToWorld(this.sketchPlane,[1,0]).sub(localToWorld(this.sketchPlane,[0,0])).cross(localToWorld(this.sketchPlane,[0,1]).sub(localToWorld(this.sketchPlane,[0,0]))).normalize();
        const angle=2*Math.atan2(new THREE.Vector3(candidate.rotation.x,candidate.rotation.y,candidate.rotation.z).dot(normal),candidate.rotation.w);
        this.sketchManipulatorLastValue={origin:[...this.sketchManipulatorOrigin],translation:[position[0]-this.sketchManipulatorOrigin[0],position[1]-this.sketchManipulatorOrigin[1]],angle};
        this.sketchManipulator.setPreviewPose(candidate.position,candidate.rotation.clone().multiply(this.sketchManipulator.frameQuaternion()));
        this.sketchManipulatorChanged?.({...this.sketchManipulatorLastValue,finish});
      };
      this.sketchManipulator=new AssemblyManipulator(this.shaders,{
        poseChanged:()=>notify(),visualChanged:()=>this.invalidate(),dragStarted:()=>{},
        dragFinished:commit=>{if(commit&&this.sketchManipulatorLastValue)this.sketchManipulatorChanged?.({...this.sketchManipulatorLastValue,finish:true});},
        snapPivot:(x,y)=>{const point=this.sketchPoint(x,y,true);return point&&this.sketchPlane?{position:localToWorld(this.sketchPlane,point)}:undefined;},
        pivotChanged:anchor=>{if(this.sketchPlane)this.sketchManipulatorOrigin=worldToLocal(this.sketchPlane,anchor.position);},
      },true);
      this.attachSketchPreview(this.sketchManipulator.root);
    }
    this.sketchManipulatorOrigin=[...origin];this.sketchManipulatorLastValue=undefined;this.sketchManipulatorChanged=changed;
    const zero=localToWorld(this.sketchPlane,[0,0]),u=localToWorld(this.sketchPlane,[1,0]).sub(zero).normalize(),v=localToWorld(this.sketchPlane,[0,1]).sub(zero).normalize();
    this.sketchManipulator.attach(localToWorld(this.sketchPlane,origin),new THREE.Quaternion().setFromRotationMatrix(new THREE.Matrix4().makeBasis(u,v,u.clone().cross(v))));
    this.sketchManipulator.setPlanarMode(mode);this.invalidate();
  }
  private clearSketchManipulator():void {this.clearSnapPreview();this.sketchManipulatorChanged=undefined;this.sketchManipulator?.detach();this.invalidate();}
  private toolViewportPort(): ToolViewportPort {
    return {
      showSketchManipulator:(origin,mode,changed)=>this.showSketchManipulator(origin,mode,changed),
      clearSketchManipulator:()=>this.clearSketchManipulator(),
      sketchManipulatorPointerDown:(id,x,y)=>{this.sketchManipulator?.updateScale(this.camera,viewportMetrics(this.renderer));return this.sketchManipulator?.pointerDown(id,x,y,this.camera,this.renderer.domElement)??false;},
      sketchManipulatorPointerMove:(id,x,y)=>this.sketchManipulator?.pointerMove(id,x,y,this.camera,this.renderer.domElement)??false,
      sketchManipulatorPointerUp:(id,commit)=>this.sketchManipulator?.pointerUp(id,commit)??false,
      sketchPoint: (x, y) => this.sketchPoint(x, y),
      snapSketchEditPoint: (point, entityId) => this.snapSketchLocalPoint(point, true, entityId),
      sketchSnapReference: () => {
        const snap=this.lastSketchSnap;
        if(!snap)return undefined;
        if(snap.kind==="ORIGIN")return {target:"SKETCH_ORIGIN",subElement:"POINT"};
        if(snap.entityId && (snap.subElement==="POINT"||snap.subElement==="START"||snap.subElement==="END"||snap.subElement==="CENTER"||snap.subElement==="CONTROL")) {
          const external = this.sketchView()?.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch?.externalGeometry
            ?.some((candidate) => candidate.id === snap.entityId);
          return {target:external?"EXTERNAL":"ENTITY",entityId:snap.entityId,subElement:snap.subElement,...(snap.subElement==="CONTROL"?{controlPointIndex:snap.controlPointIndex,controlPointId:snap.controlPointId}:{})};
        }
        return undefined;
      },
      sketchPlacementPoint: (x, y) => this.rawSketchPoint(x, y) ?? null,
      showPolylinePreview: (points, closed = false) => {
        if (this.sketchPlane) this.drawPreview(points, closed, this.sketchPlane);
      },
      showSplineControlPreview: (points, mode) => {
        if (!this.preview || !this.sketchPlane) return;
        this.preview.add(makeSplineControlFeedback(points, mode, point => localToWorld(this.sketchPlane!, point),
          this.materials, {width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight},
          CATIA_VISUAL_THEME.preview, SKETCH_FEEDBACK_ORDER.preview + 3));
        this.invalidate();
      },
      showPointPreview: (point) => {
        if (this.sketchPlane) this.drawPointPreview(point, this.sketchPlane);
      },
      showReferenceDimensions: (geometry) => {
        if(!this.preview||!this.sketchPlane)return;
        for(const dimension of sketchReferenceDimensions(geometry)){const sprite=makeConstraintDimensionLabel(dimension.text,point=>localToWorld(this.sketchPlane!,point));
          sprite.position.copy(localToWorld(this.sketchPlane,dimension.position));sprite.renderOrder=31;this.preview.add(sprite);}
        this.invalidate();
      },
      clearToolPreview: () => { this.clearPreview(); this.clearSnapPreview(); },
      commitSketchOperations: (operations,intent) => {const result=this.commitSketchOperations(operations,intent);void result.catch(()=>{});return result;},
      setSketchCommandState: state=>this.setSketchCommandState(state),
      isFeatureSelectionActive:()=>!!this.featureSelection,
      sketchEntityAt:(x,y)=>this.sketchEntitySelectionAt(x,y),
      hasActiveSketch: () => Boolean(this.sketchPlane && this.activeSketchID),
      sketchReferenceAt: (x, y, kind, retained,allowed) => this.sketchReferenceAt(x, y, kind, retained,allowed),
      showReferencePreview: (reference, retained) => this.showReferencePreview(reference, retained),
      showConstraintPreview: (kind, references, value, labelPosition) => this.showConstraintPreview(kind, references, value, labelPosition),
      measureDimension: (kind, references) => {
        const sketchView = this.sketchView();
        const sketch = sketchView?.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch;
        const feature = sketchView?.part?.features.find((candidate) => candidate.id === this.activeSketchID);
        return sketch && feature ? measureSketchDimension(kind, references, sketchReferenceEntities(feature)) : undefined;
      },
      requestDimensionCreation: (kind, references, value, unit, labelPosition, x, y) => {
        if (!this.activeSketchID || !isDimensionConstraintKind(kind)) return;
        this.callbacks.dimensionCreateRequested({ mode: "create", featureId: this.activeSketchID, kind,
          references: [...references], labelPosition, value, unit, x, y });
      },
      beginDimensionDrag: (x, y) => this.beginDimensionDrag(x, y),
      updateDimensionDrag: (x, y) => this.updateDimensionDrag(x, y),
      finishDimensionDrag: () => this.finishDimensionDrag(),
      cancelDimensionDrag: () => this.cancelDimensionDrag(),
      editDimensionAt: (x, y) => this.editDimensionAt(x, y),
      clearReferencePreview: () => this.clearReferencePreview(),
      clearReferenceHover:()=>this.clearReferenceHover(),
      setToolPrompt: (prompt) => this.callbacks.toolPromptChanged(prompt),
      finishToolUse: (exit) => {
        if(exit){this.tools.activate("select");return;}
        if(this.sketchCommitPending){this.deferredToolFinish={toolId:this.tools.activeToolID??"select"};return;}
        this.callbacks.toolUseCompleted();
      },
      selectionAt: (x, y) => this.hitTest(x, y, true),
      commitExternalProjection: async (selection) => {
        if (!this.activeSketchID || !selection.geometryKey || !selection.versionId) return;
        const externalID = this.reconnectExternalID ?? randomUUID();
        const type = this.reconnectExternalID ? "RECONNECT_EXTERNAL_GEOMETRY" as const : "ADD_EXTERNAL_GEOMETRY" as const;
        const result=await this.commitSketchOperations( [{ type, externalId: externalID, geometryKey: selection.geometryKey,
          topologyId: selection.topologyId, topologyKind: selection.kind.toUpperCase() as "EDGE"|"VERTEX", sourceVersionId: selection.versionId }]);
        if(this.reconnectExternalID===externalID)this.reconnectExternalID=undefined;
        return result;
      },
      currentSketchReferenceEntities:()=>this.visibleSketchReferenceEntities(),
      previewSketchOperations:async(operations,signal)=>{
        const featureId=this.activeSketchID,scope=this.sketchView()?.document;
        if(!featureId||!scope||!this.callbacks.sketchPreview)throw new Error("草图权威预览不可用");
        const preview=await this.callbacks.sketchPreview(featureId,operations,signal);
        const candidate=preview.sketchCandidates?.find(candidate=>candidate.featureId===featureId);
        if(preview.baseVersionId!==scope.versionId||!candidate)throw new Error("草图候选已失效或无法求值");
        return {featureId,versionId:preview.baseVersionId,entities:candidate.entities};
      },
      currentSketchConstraints: () => this.sketchView()?.part?.features.find(feature=>feature.id===this.activeSketchID)?.sketch?.constraints ?? [],
      currentSketchDeletableEntities:()=>this.visibleSketchDeletionEntities().filter(e=>!this.sketchView()?.part?.features.find(f=>f.id===this.activeSketchID)?.sketch?.externalGeometry?.some(external=>external.id===e.id)),
      currentSketchEntities: () => this.visibleSketchReferenceEntities().filter(entity=>!this.sketchView()?.part?.features.find(feature=>feature.id===this.activeSketchID)?.sketch?.externalGeometry?.some(external=>external.id===entity.id)),
      currentSketchIdentity: () => {const view=this.sketchView();return view&&this.activeSketchID?{documentId:view.document.id,versionId:view.document.versionId,sketchId:this.activeSketchID,occurrencePath:this.editContext?.occurrencePath}:undefined;},
      showSketchEditCandidate:candidate=>this.showSketchEditCandidate(candidate),
      showSketchEntityPreview: (entities) => {
        this.clearPreview();if(!this.sketchPlane)return;
        const group=new THREE.Group();
        for(const entity of entities){const points=sampleSketchEntity(entity).map(point=>localToWorld(this.sketchPlane!,point));
          if(!points.length)continue;
          const primitive=entity.kind==="POINT"?new THREE.Points(new THREE.BufferGeometry().setFromPoints(points),this.materials.point(CATIA_VISUAL_THEME.preview,11,false)):
            makeSketchOverlayLine(points,CATIA_VISUAL_THEME.preview,2.75,entity.role==="CONSTRUCTION");
          primitive.renderOrder=SKETCH_FEEDBACK_ORDER.preview;updateHighlightLineResolution(primitive,this.renderer.domElement.clientWidth,this.renderer.domElement.clientHeight);group.add(primitive);
          if (entity.kind === "SPLINE") group.add(makeSplineControlFeedback(
            splineEditablePoints(entity).map(p => [p.x, p.y]), entity.mode ?? "FIT", point => localToWorld(this.sketchPlane!, point),
            this.materials, {width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight},
            CATIA_VISUAL_THEME.preview, SKETCH_FEEDBACK_ORDER.preview + 3));
        }
        this.preview=group;this.attachSketchPreview(group);this.invalidate();
      },
      currentSelections: () => [...this.selected],
      retainSelections: (selections) => this.selectMany(selections.map(selection=>{
        const scope=this.sketchView()?.document;
        if(selection.kind!=="visual"||selection.featureId!==this.activeSketchID||(selection.ownerDocumentId??selection.documentId??scope?.id)!==scope?.id||(selection.occurrencePath??"")!==(this.editContext?.occurrencePath??""))return selection;
        const entity=this.visibleSketchReferenceEntities().find(entity=>entity.id===selection.entityId);
        return entity?this.sketchEntitySelection(entity):selection;
      }),true,"command"),
      currentSelectionSource:()=>this.selectionSource,
      consumeActivationSelection:()=>{this.selectionSource="command";},
      currentLengthUnit:()=>this.sketchLengthUnit,
      projectSketchPoint:point=>{
        if(!this.sketchPlane)return undefined;
        const p=localToWorld(this.sketchPlane,point).project(this.camera);
        return [(p.x+1)*this.renderer.domElement.clientWidth/2,(1-p.y)*this.renderer.domElement.clientHeight/2];
      },
      requestAssemblyConstraint: (kind, references) => this.callbacks.assemblyConstraintRequested(kind, references),
      moveManipulatorPointerDown: (pointerId, x, y) => !this.moveFinalization&&!this.moveCommitPending&&!this.moveInteraction.hasUncommittedFinal&&this.moveManipulator.pointerDown(pointerId, x, y, this.camera, this.renderer.domElement),
      moveManipulatorPointerMove: (pointerId, x, y) => this.moveManipulator.pointerMove(pointerId, x, y, this.camera, this.renderer.domElement),
      moveManipulatorPointerUp: (pointerId, commit) => {
        this.manipulatorSnapCache.cancelPending();this.snapInput=undefined;
        const handled=this.moveManipulator.pointerUp(pointerId,commit);
        if(!commit&&!handled)this.cancelMovePreviewGesture(); // Esc/blur after pointerup while final solve is pending
        return handled;
      },
    };
  }

  private sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind, retained?: SketchGeometryRef,allowed?:(reference:SketchGeometryRef)=>boolean) {
    const sketchView = this.sketchView();
    if (!this.sketchPlane || !sketchView) return null;
    const width = Math.max(this.renderer.domElement.clientWidth, 1);
    const height = Math.max(this.renderer.domElement.clientHeight, 1);
    const screen = (point: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, point).project(this.camera);
      return { x: (projected.x + 1) * width / 2, y: (1 - projected.y) * height / 2 };
    };
    const feature = sketchView.part?.features.find((candidate) => candidate.id === this.activeSketchID);
    const entities = this.visibleSketchReferenceEntities();
    const origin = localToWorld(this.sketchPlane, [0, 0]);
    const extents = ([[1, 0], [0, 1]] as Vec2[]).map(axis => {
      const ends = sketchAxisEndpoints(this.camera, origin, localToWorld(this.sketchPlane!, axis).sub(origin), width, height);
      return ends ? ends[1].distanceTo(origin) : 0;
    }) as Vec2;
    const reference = resolveSketchReference({ x, y }, entities, screen, kind, 12, extents, retained,allowed);
    if (!reference) return null;
    if (reference.entityId && feature?.sketch?.externalGeometry?.some((external) => external.id === reference.entityId)) reference.target = "EXTERNAL";
    const captureKind = reference.target === "SKETCH_ORIGIN" ? "ORIGIN"
      : reference.subElement === "START" || reference.subElement === "END" ? "ENDPOINT"
      : reference.subElement === "CENTER" ? "CENTER" : reference.subElement === "POINT" ? "POINT" : "CURVE";
    // Explicit role picking is independent of automatic snapping preferences.
    return reference;
  }

  private emitDebugState(): void {
    if (this.disposed) return;
    this.callbacks.debugStateChanged?.({
      input: this.input.getState(), activeTool: this.activeToolID,
      navigationProfile: this.navigationProfile, navigationAction: this.navigation.activeAction,
      selectionKeys: (this.featureSelection?.selections??this.selected).map(selectionKey),
      datumPreview: this.datumPreview?.axis ? {kind:"axis",origin:this.datumPreview.axis.origin,direction:this.datumPreview.axis.direction} : this.datumPreview?.plane ? {kind:"plane",origin:this.datumPreview.plane.origin,direction:this.datumPreview.plane.normal} : undefined,
      featureSelection: this.featureSelection?{role:this.featureSelection.role,count:this.featureSelection.selections.length,overlays:this.selectedOverlays.length}:undefined,
      highlightedVisible: [...this.highlightedRoots].filter((root) => {
        for (let object: THREE.Object3D | null = root; object; object = object.parent) if (!object.visible) return false;
        return true;
      }).length,
      navigation: this.navigation.snapshot, hudScreen: this.navigationHUD.screenPosition
    });
  }

  private updateNavigationHUD(snapshot = this.navigation.snapshot): void {
    this.navigationHUD.update(
      snapshot,
      this.camera,
      this.renderer.domElement.clientWidth,
      this.renderer.domElement.clientHeight,
    );
  }

  private applyHighlight(object: THREE.Object3D, state: "default" | "hover" | "selected" | "context"): void {
    this.materials.setInteractionState(object, state);
    object.traverse((child) => {
      const material = (child as THREE.Mesh).material;
      if (material instanceof THREE.MeshBasicMaterial) {
        // Text is an opaque glyph over a transparent mask, not a translucent face.
        material.opacity = child.userData.sketchDimensionLabel ? 1 : state === "selected" ? 0.28 : state === "hover" ? 0.18 : 0.075;
      } else if (material instanceof THREE.LineBasicMaterial) {
        material.userData.baseColor ??= material.color.getHex();
        material.color.setHex(state === "selected" ? CATIA_VISUAL_THEME.selected : state === "hover" ? CATIA_VISUAL_THEME.hover : Number(material.userData.baseColor));
      } else if (material instanceof THREE.PointsMaterial) {
        material.userData.baseColor ??= material.color.getHex();
        material.color.setHex(state === "selected" ? CATIA_VISUAL_THEME.selected : state === "hover" ? CATIA_VISUAL_THEME.hover : Number(material.userData.baseColor));
      }
    });
  }

  private frameContent(): void {
    this.viewTransition.cancel();
    this.navigation.cancel();
    const box = this.visibleContentBounds();
    if (box.isEmpty()) box.setFromCenterAndSize(new THREE.Vector3(), new THREE.Vector3(180, 180, 100));
    fitOrthographicView(this.camera, this.navigation.target, box);
    this.navigation.syncCamera(false);
  }

  private updateCameraClipping(box?: THREE.Box3): void {
    const bounds = (box ?? this.contentBounds).clone();
    // A sketch can exist without a solid. Include its geometry and fixed-pixel
    // references, otherwise zooming produces a far plane in front of the sketch.
    for (const root of [this.helpers, this.sketchContext, this.preview, this.referencePreview, this.referenceHover, this.snapPreview, this.dimensionDefinitionPreview]) {
      if (!root) continue;
      root.updateMatrixWorld(true);
      root.traverseVisible(object => {
        const geometry = (object as THREE.Mesh).geometry;
        if (!geometry) return;
        if (!geometry.boundingBox) geometry.computeBoundingBox();
        if (geometry.boundingBox) bounds.union(geometry.boundingBox.clone().applyMatrix4(object.matrixWorld));
      });
    }
    if (this.sketchPlane) bounds.expandByPoint(localToWorld(this.sketchPlane, [0, 0]));
    if (bounds.isEmpty()) bounds.expandByPoint(this.navigation.target);
    if (this.commandPreview) bounds.union(new THREE.Box3().setFromObject(this.commandPreview));
    updateOrthographicClipping(this.camera, bounds);
  }

  private resize(): void {
    const width = Math.max(this.host.clientWidth, 1);
    const height = Math.max(this.host.clientHeight, 1);
    this.renderer.setSize(width, height, false);
    const halfHeight = (this.camera.top - this.camera.bottom) / 2;
    this.camera.left = -halfHeight * width / height;
    this.camera.right = halfHeight * width / height;
    this.camera.updateProjectionMatrix();
    for (const overlay of [...this.preselectedOverlays, ...this.selectedOverlays]) {
      updateHighlightLineResolution(overlay, width, height);
    }
    for(const root of [this.helpers,this.preview,this.referencePreview,this.referenceHover,this.dimensionDefinitionPreview])if(root)updateHighlightLineResolution(root,width,height);
    this.updateNavigationHUD();
    this.invalidate();
  }

  private refreshContentBounds(): void {
    this.contentBounds.setFromObject(this.content);
  }

  private visibleContentBounds(): THREE.Box3 {
    const bounds = new THREE.Box3();
    this.content.updateMatrixWorld(true);
    this.content.traverseVisible((object) => {
      if (!(object instanceof THREE.Mesh || object instanceof THREE.Line || object instanceof THREE.Points)) return;
      const geometry = object.geometry;
      if (!geometry.boundingBox) geometry.computeBoundingBox();
      if (geometry.boundingBox) bounds.union(geometry.boundingBox.clone().applyMatrix4(object.matrixWorld));
    });
    if (this.commandPreview) bounds.union(new THREE.Box3().setFromObject(this.commandPreview));
    return bounds;
  }

  private visibleContentCenter(): THREE.Vector3 {
    const bounds = this.visibleContentBounds();
    return bounds.isEmpty() ? this.navigation.target.clone() : bounds.getCenter(new THREE.Vector3());
  }

  private disposeGroup(group: THREE.Group): void {
    for (const child of [...group.children]) {
      group.remove(child);
      this.disposeRenderable(child);
    }
  }

  private disposeRenderable(root: THREE.Object3D): void {
    root.traverse((object) => {
      const renderable = object as THREE.Mesh;
      renderable.geometry?.dispose();
      const materials = Array.isArray(renderable.material) ? renderable.material : [renderable.material];
      materials.forEach((material) => {
        if (material?.userData.ownedTexture instanceof THREE.Texture) material.userData.ownedTexture.dispose();
        material?.dispose();
      });
    });
  }

  private invalidate(): void {
    if (this.disposed || this.animationFrame) return;
    this.animationFrame = requestAnimationFrame((now) => {
      this.animationFrame = 0;
      if (!this.disposed) {
        if (this.viewTransition.update(now)) this.navigation.syncCamera(false);
        this.updateNavigationHUD();
        this.moveManipulator.updateScale(this.camera, viewportMetrics(this.renderer));
        this.sketchManipulator?.updateScale(this.camera,viewportMetrics(this.renderer));
        this.updateScreenStableReferences();
        const metrics=viewportMetrics(this.renderer);
        updateScreenLines(this.scene, this.camera, metrics.cssWidth, metrics.cssHeight);
        this.updateSketchParameterAnchor();
        this.updateCameraClipping();
        this.scene.traverse((object)=>{
          const material=(object as THREE.Points).material;
          if(material instanceof THREE.ShaderMaterial&&material.uniforms.uPointSize&&typeof material.userData.cssPointSize==="number")
            material.uniforms.uPointSize.value=material.userData.cssPointSize*metrics.devicePixelRatio;
        });
        this.sketchGrid.object.visible = false;
        if (this.sketchPlane) this.sketchGrid.update(this.camera, this.renderer.domElement.clientHeight, planeFrame(this.sketchPlane));
        this.renderer.clear(true, true, true);
        this.background.render(this.renderer, this.camera);
        if (this.environment.visible) this.groundGrid.render(this.renderer, this.camera);
        this.renderer.clearDepth();
        this.renderer.render(this.scene, this.camera);
        if(this.analysisScene.children.length){
          updateAnalysisGuides(this.analysisScene,this.camera,metrics);
          this.renderer.clearDepth();
          this.renderer.render(this.analysisScene,this.camera);
        }
        this.navigationHUD.render(this.renderer);
        if (this.viewTransition.active) this.invalidate();
      }
    });
  }

  private updateSketchParameterAnchor():void {
    const state=this.sketchCommandState,point=state?.input?.modelAnchor;
    if(!state?.input||!point||!this.sketchPlane)return;
    const anchor=this.toolViewportPort().projectSketchPoint?.(point);if(!anchor)return;
    const previous=state.input.anchor;if(previous&&Math.hypot(anchor[0]-previous[0],anchor[1]-previous[1])<1)return;
    this.sketchCommandState={...state,input:{...state.input,anchor}};
    this.callbacks.sketchCommandChanged?.(this.sketchCommandState);
  }

  private updateScreenStableReferences(): void {
    if (!this.screenStableReferences.size) return;
    this.camera.updateMatrixWorld();
    this.camera.matrixWorldInverse.copy(this.camera.matrixWorld).invert();
    this.scene.updateMatrixWorld(true);
    const metrics = viewportMetrics(this.renderer);
    const worldPosition = new THREE.Vector3();
    for (const [object, cssPixels] of this.screenStableReferences) {
      object.getWorldPosition(worldPosition);
      const scale = worldUnitsPerCssPixel(this.camera, worldPosition, metrics) * cssPixels;
      if (Number.isFinite(scale) && scale > 0) object.scale.setScalar(scale);
    }
    this.scene.updateMatrixWorld(true);
  }
}
