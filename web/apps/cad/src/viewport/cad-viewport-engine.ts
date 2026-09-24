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
import { acceleratedRaycast, computeBoundsTree, disposeBoundsTree } from "three-mesh-bvh";
import { InputManager } from "../cad/input/input-manager";
import { snapshotTransform, TransformTransitionSystem, type TransformPose } from "../cad/animation/transform-transition";
import type { InputState } from "../cad/input/input-types";
import { InteractionRouter } from "../cad/interaction/interaction-router";
import { SelectionController } from "../cad/interaction/selection-controller";
import { SelectionIndex } from "../cad/interaction/selection-index";
import { AssemblyManipulator, type ManipulatorAnchor } from "../cad/interaction/assembly-manipulator";
import { projectSketchFeatureSelection, selectionModeForTool, sketchContextLayerVisibility, type SelectionMode } from "../cad/interaction/selection-mode";
import { sketchTreeVisible, treeVisibilityOverride, type TreeVisibilityOverrides } from "../cad/interaction/tree-visibility";
import { sameSelection, sameSelections, selectionKey } from "../cad/interaction/selection-identity";
import { resultBodyFeatureTreeNode } from "../cad/interaction/selection-hierarchy";
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
import { makeDatumReferenceLine, makeOcclusionVisibleHighlightLine, makeOcclusionVisibleSegments,
  makeSketchOverlayLine, updateHighlightLineResolution } from "../cad/rendering/interaction-highlight";
import { constraintSymbolCode, makeConstraintDimensionLabel, makeSketchConstraintRenderable } from "../cad/rendering/sketch-constraint-renderer";
import { isDimensionConstraintKind, type ConstraintKind } from "../cad/sketch/sketch-constraint-definition";
import { measureSketchDimension } from "../cad/sketch/sketch-constraint-layout";
import { sketchReferenceDimensions, SKETCH_INPUT_POLICY } from "../cad/sketch/sketch-input-policy";
import { sampleSketchEntity, sketchEntityPoint } from "../cad/sketch/sketch-geometry";
import { CadShaderLibrary } from "../cad/rendering/shader/cad-shader-library";
import { manipulatorFrame, transformAroundWorldPivot, viewportMetrics, worldUnitsPerCssPixel } from "../cad/rendering/viewport-metrics";
import { randomUUID } from "../utils/random-uuid";
import { assemblyConstraintGlyph } from "../cad/assembly/assembly-constraint-ux";
import { ArcSketchTool, AssemblyConstraintTool, AssemblyMoveTool, CircleSketchTool, ConstraintSketchTool, LineSketchTool, LinearDimensionSketchTool, PointSketchTool, PolylineSketchTool, ProjectExternalGeometrySketchTool, RectangleSketchTool, RegularPolygonSketchTool, SelectTool, SlotSketchTool, SplineSketchTool, type AssemblyConstraintToolKind, type ToolViewportPort } from "../cad/tool/cad-tool";
import { ToolManager } from "../cad/tool/tool-manager";
import type {
  Artifact, AssemblyGeometryRef, AxisSystem, DatumAxis, DatumPlane, DocumentStructureNode, DocumentView, Feature, PlaneName, Publication, ReferenceGeometry, Selection, SelectionItem, SketchConstraint, SketchEntity, SketchGeometryRef, SketchOperation, SketchPlane, Vec2, Vec3, VisualizationManifest,
} from "../types";

type Callbacks = {
  selectionsChanged: (selections: SelectionItem[]) => void;
  preselectionChanged: (selection: Selection) => void;
  sketchOperations: (featureID: string, operations: SketchOperation[]) => void;
  toolPromptChanged: (prompt: string) => void;
  toolUseCompleted: () => void;
  dimensionEditRequested: (request: { mode: "edit"; featureId: string; constraintId: string; value: number; unit: "mm" | "deg"; x: number; y: number }) => void;
  dimensionCreateRequested: (request: { mode: "create"; featureId: string; kind: "DISTANCE"|"LENGTH"|"RADIUS"|"DIAMETER"|"ANGLE";
    references: SketchGeometryRef[]; labelPosition: Vec2; value: number; unit: "mm"|"deg"; x: number; y: number }) => void;
  activeToolChanged: (toolID: import("../state/workbench-store").WorkbenchToolID) => void;
  instanceMoved: (instanceId: string, translation: Vec3, rotation:[number,number,number,number], previewId?:string) => void;
	instanceMovePreview: (instanceId:string,translation:Vec3,rotation:[number,number,number,number],interactionId:string,previewSequence:number)=>Promise<{
		poses:Array<{instanceId:string;translation:Vec3;rotation:[number,number,number,number]}>;constraintLimited:boolean;previewId:string}>;
  assemblyConstraintRequested: (kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]) => void;
  debugStateChanged?: (state: ViewportDebugState) => void;
};

type SolidContext = {
  instancePath?: SelectionItem["instancePath"];
  documentId: string; versionId?: string; geometryKey: string; occurrencePath: string; treeNodeId: string; instanceId?: string;
};

type SolidBinding = { group: THREE.Group; mesh: THREE.Mesh; artifact: Artifact; context: SolidContext };
export type ViewportEditContext = { view: DocumentView; occurrencePath?: string; translation?: Vec3;
  rotation?: [number, number, number, number]; bodyTreeNodeId?: string };

// A Product viewport owns the assembly scene, while sketch interaction belongs
// to the active occurrence's reference document. Keeping this decision in one
// place prevents hit testing and constraint tools from accidentally reading the
// root Product (which intentionally has no Part feature collection).
export function sketchInteractionView(view?: DocumentView, editContext?: ViewportEditContext): DocumentView | undefined {
  if (editContext?.view.document.type === "PART") return editContext.view;
  return view?.document.type === "PART" ? view : undefined;
}

export function datumAxisHitAccepted(distanceToRay: number | undefined, tolerance: number): boolean {
  return distanceToRay !== undefined && distanceToRay <= tolerance;
}

export function bindPublicationSelection(selection: SelectionItem, root?: DocumentStructureNode): SelectionItem {
  if (!root || selection.publicationId) return selection;
  const matches: Array<{ id: string; publication: Publication }> = [];
  const visit = (node: DocumentStructureNode) => {
    const source = node.publication;
    const forwarded = node.productPublication;
    const resolution = source?.resolution ?? forwarded?.resolution;
    const occurrencePath = source ? node.instancePath?.canonical ?? "" : forwarded?.target.instancePath.canonical ?? "";
    const topologyMatches = resolution?.topologyKind?.toLowerCase() === selection.kind && "topologyId" in selection &&
      resolution.localId === selection.topologyId && (!resolution.geometryKey || !selection.geometryKey || resolution.geometryKey === selection.geometryKey);
    const datumID = source?.target.datumId ?? resolution?.geometryId;
    const datumMatches = (["plane", "axis", "axis-system"].includes(selection.kind) && datumID && selection.entityId === datumID);
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
  navigationProfile: NavigationProfileID;
  navigationAction: NavigationAction;
  navigation?: NavigationSnapshot;
  hudScreen?: { x: number; y: number };
};

function sketchReferenceEntities(feature?: Feature): SketchEntity[] {
  return [...(feature?.sketch?.entities ?? []), ...(feature?.sketch?.externalGeometry ?? []).flatMap((external) => external.snapshot ? [{
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
  geometry.computeVertexNormals();
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
  private readonly scene = new THREE.Scene();
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
  private readonly navigation: NavigationController;
  private readonly navigationHUD: NavigationHUD;
  private readonly tools: ToolManager;
  private readonly selectionController: SelectionController;
  private readonly interaction: InteractionRouter;
  private readonly input: InputManager;
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();
  private readonly content = new THREE.Group();
  private readonly helpers = new THREE.Group();
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
  private editContext?: ViewportEditContext;
  private sketchPlane?: SketchPlane;
  private activeSketchID?: string;
  private preview?: THREE.Object3D;
  private referencePreview?: THREE.Object3D;
  private snapPreview?: THREE.Object3D;
  private lastSketchSnap?: SketchSnapResult;
  private commandPreview?: THREE.Object3D;
  private previewBody?: { group: THREE.Group; visible: boolean };
  private assemblyPosePreview?: Map<string, { position: THREE.Vector3; rotation: THREE.Quaternion }>;
  private dimensionDrag?: { selection: Extract<SelectionItem, { kind: "sketch-constraint" }>; constraint: SketchConstraint;
    root?: THREE.Object3D; rootParent?: THREE.Object3D; rootIndex?: number; startX: number; startY: number; position?: Vec2 };
	private movePreviewGeneration=0;private movePreviewInFlight=false;
	private moveInteractionId="";private movePreviewSequence=0;
  private moveCommitPending=false;
  private pendingMovePreview?:{generation:number;instanceId:string;translation:Vec3;rotation:[number,number,number,number]};
  private desiredMovePose?:{translation:Vec3;rotation:[number,number,number,number]};
	private acceptedMovePose?:{translation:Vec3;rotation:[number,number,number,number];previewId?:string};
  private activeToolID = "select";
  private reconnectExternalID?: string;
  private selectionMode: SelectionMode = selectionModeForTool("select");
  private sketchReturnView?: SavedView;
  private navigationProfile: NavigationProfileID = "default";
  private captureSettings: CaptureSettings = DEFAULT_CAPTURE_SETTINGS;
  private treeVisibilityOverrides: TreeVisibilityOverrides = {};
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

    this.moveManipulator = new AssemblyManipulator(this.shaders, {
      dragStarted: () => { this.navigation.setEnabled(false); this.beginMovePreviewGesture(); },
      snapPivot: (x, y) => this.snapManipulatorPivot(x, y),
      pivotChanged: (anchor) => this.updateManipulatorPivot(anchor),
      poseChanged: () => { this.updateMoveTarget(); this.invalidate(); },
      visualChanged: () => this.invalidate(),
      dragFinished: (commit) => {
        this.navigation.setEnabled(true);
        if (commit) this.finishMovePreviewGesture();
        else if (this.moveTarget) {
          this.cancelMovePreviewGesture();
          const target = this.moveTarget;
          this.transforms.apply(target.group, this.transformPose(target.startPosition, target.startQuaternion), "rollback",
            () => this.syncMoveManipulatorToRenderedPose(target));
        }
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
    this.sketchContext.renderOrder = 15;
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
    for (const kind of ["fix", "rigid", "coincident", "concentric", "angle", "parallel", "perpendicular", "distance"] as const)
      this.tools.register(new AssemblyConstraintTool(kind));
    this.tools.register(new PointSketchTool());
    this.tools.register(new ProjectExternalGeometrySketchTool());
    this.tools.register(new LineSketchTool());
    this.tools.register(new CircleSketchTool());
    this.tools.register(new ArcSketchTool());
    this.tools.register(new PolylineSketchTool());
    this.tools.register(new SplineSketchTool());
    this.tools.register(new RectangleSketchTool());
    this.tools.register(new RegularPolygonSketchTool());
    this.tools.register(new SlotSketchTool());
    this.tools.register(new LinearDimensionSketchTool());
    for (const kind of ["COINCIDENT","PARALLEL","FIXED","HORIZONTAL","VERTICAL","PERPENDICULAR","TANGENT","EQUAL","DISTANCE","LENGTH","RADIUS","ANGLE","CONCENTRIC","POINT_ON_OBJECT","MIDPOINT","SYMMETRY"] as const)
      this.tools.register(new ConstraintSketchTool(kind));
    this.tools.activate("select");
    this.selectionController = new SelectionController(
      (x, y, additive) => this.pick(x, y, additive),
      (x, y) => this.preselectAt(x, y),
      () => { this.preselect(null, true); this.clearSnapPreview(); },
    );
    this.interaction = new InteractionRouter(this.tools, this.selectionController, this.navigation);
    this.input = new InputManager(this.renderer.domElement, this.interaction);
    this.input.subscribe(() => this.emitDebugState());
    this.tools.subscribe((toolID) => {
      this.activeToolID = toolID ?? "select";
      this.selectionMode = selectionModeForTool(this.activeToolID);
      this.preselect(null, true);
      if (this.activeToolID !== "assembly.move") this.moveManipulator.detach();
      else this.attachMoveManipulator();
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

  private sketchView(): DocumentView | undefined {
    return sketchInteractionView(this.view, this.editContext);
  }

  render(view: DocumentView, editContext?: ViewportEditContext): void {
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
    this.screenStableReferences.clear();
    if (view.document.type === "PART") this.renderPart(view);
    else this.renderProduct(view);
    if (view.document.type === "PRODUCT" && editContext?.view.document.type === "PART") {
      const consumedSketches = new Set((editContext.view.part?.features ?? []).flatMap((feature) => feature.profile ? [feature.profile] : []));
      for (const feature of editContext.view.part?.features ?? []) {
        if (feature.type.toUpperCase().includes("SKETCH")) this.addSketch(feature, false, editContext.view, {
          documentId: editContext.view.document.id, geometryKey: editContext.view.artifact?.geometryKey ?? "",
          occurrencePath: editContext.occurrencePath ?? "", treeNodeId: editContext.bodyTreeNodeId ?? "",
        }, editContext.translation, editContext.rotation, !consumedSketches.has(feature.id));
      }
    }
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
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
    if (this.view) this.render(this.view);
  }

  private applyTreeVisibility(): void {
    const activeSketchKey = this.helpers.children.find((object) => object.userData.sketchEditOverlay === true &&
      object.userData.sketchFeatureID === this.activeSketchID)?.userData.treeNodeId as string | undefined;
    const activeSketchOverrides = activeSketchKey ? Object.fromEntries(Object.entries(this.treeVisibilityOverrides)
      .filter(([candidate]) => candidate.startsWith(`${activeSketchKey}/`))) : {};
    for (const root of [this.content, this.helpers]) root.traverse((object) => {
      const key = typeof object.userData.treeNodeId === "string" ? object.userData.treeNodeId : undefined;
      const activeSketchSubtree = Boolean(key && activeSketchKey && (key === activeSketchKey || key.startsWith(`${activeSketchKey}/`)));
      const overrides = activeSketchSubtree ? activeSketchOverrides : this.treeVisibilityOverrides;
      const category = referenceCategory(object.userData.kind, object.userData.axis);
      if (category) object.visible = this.referenceVisibility[category] &&
        !(root === this.helpers && this.sketchPlane) && treeVisibilityOverride(key, overrides) !== false;
      if (treeVisibilityOverride(key, overrides) === false) object.visible = false;
    });
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
    if (entering) this.select({ kind: "plane", id: plane.datumPlaneId, plane: plane.plane });
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

  captureToolSelections(selections: readonly SelectionItem[]): boolean {
    return this.tools.selectionInput(selections);
  }

  setActiveTool(toolID: import("../state/workbench-store").WorkbenchToolID): void {
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

  previewArtifact(artifact: Artifact, operation: FeaturePreviewOperation = "NEW_BODY"): void {
    this.clearCommandPreview();
    if (!artifact.mesh.vertices.length || !artifact.mesh.triangles.length) return;
    const binding = this.solidBindings.get(this.editContext?.occurrencePath || "root");
    const geometry = makeGeometry(artifact);
    const group = makeFeaturePreview(geometry, operation, binding?.mesh.geometry.clone());
    if (binding) {
      this.previewBody = { group: binding.group, visible: binding.group.visible };
      binding.group.visible = false;
    }
    if (this.editContext?.translation) group.position.fromArray(this.editContext.translation);
    if (this.editContext?.rotation) group.quaternion.fromArray(this.editContext.rotation);
    this.scene.add(group); this.commandPreview = group;
    this.host.dataset.featurePreview = operation;
    this.updateCameraClipping(); this.invalidate();
  }

  clearCommandPreview(restore = true): void {
    delete this.host.dataset.featurePreview;
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
    const feature = sketchView.part?.features.find((candidate) => candidate.id === selection.featureId);
    const constraint = feature?.sketch?.constraints.find((candidate) => candidate.id === selection.constraintId);
    if (!constraint || constraint.value === undefined || (constraint.unit !== "mm" && constraint.unit !== "deg") ||
      !isDimensionConstraintKind(constraint.kind)) return false;
    this.selectMany([selection]);
    this.callbacks.dimensionEditRequested({ mode: "edit", featureId: selection.featureId, constraintId: constraint.id,
      value: constraint.value, unit: constraint.unit, x: x ?? this.renderer.domElement.clientWidth / 2,
      y: y ?? this.renderer.domElement.clientHeight / 2 });
    return true;
  }

  selectMany(selections: readonly SelectionItem[], notify = true): void {
    const unique = [...new Map(selections.map((selection) => [selectionKey(selection), selection])).values()];
    if (sameSelections(this.selected, unique) && !this.preselected) {
      if (notify) this.callbacks.selectionsChanged(unique);
      if(this.activeToolID==="assembly.move"&&(!this.moveManipulator.isAttached()||this.pendingManipulatorAnchor))this.attachMoveManipulator();
      return;
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
    const orientation=picked?.orientation??this.manipulatorFrames.get(instanceId)??new THREE.Quaternion();
    this.manipulatorFrames.set(instanceId,orientation.clone());
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
      if(anchor.orientation)this.manipulatorFrames.set(instanceId,anchor.orientation.clone());
    }
    this.invalidate();
  }

  private snapManipulatorPivot(x: number, y: number): ManipulatorAnchor | undefined {
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const worldPerPixel = worldUnitsPerCssPixel(this.camera, this.moveManipulator.object.getWorldPosition(new THREE.Vector3()),
      viewportMetrics(this.renderer));
    this.raycaster.params.Line = { threshold: worldPerPixel * 8 };
    this.raycaster.params.Points = { threshold: worldPerPixel * 11 };
    const roots = [...this.solidBindings.values()].map((binding) => binding.group);
    const hits = this.raycaster.intersectObjects(roots, true).filter((hit) => hit.object.visible);
    if (hits.length === 0) return undefined;
    const near = hits.filter((hit) => hit.distance <= hits[0].distance + worldPerPixel * 12);
    const priority = (hit: THREE.Intersection) => hit.object instanceof THREE.Points ? 2
      : hit.object instanceof THREE.LineSegments ? 1 : 0;
    near.sort((left, right) => priority(right) - priority(left));
    return this.manipulatorAnchorFromIntersection(near[0]);
  }

  private manipulatorAnchorFromIntersection(hit:THREE.Intersection):ManipulatorAnchor{
    let direction:THREE.Vector3|undefined,kind:"line"|"plane"|undefined;
    if(hit.object instanceof THREE.LineSegments){
      const position=hit.object.geometry.getAttribute("position"),index=((hit.index??0)/2|0)*2;
      if(position&&index+1<position.count){
        const first=hit.object.localToWorld(new THREE.Vector3().fromBufferAttribute(position,index));
        const second=hit.object.localToWorld(new THREE.Vector3().fromBufferAttribute(position,index+1));
        direction=second.sub(first).normalize();kind="line";
      }
    }else if(hit.object instanceof THREE.Mesh&&hit.face){
      direction=hit.face.normal.clone().transformDirection(hit.object.matrixWorld);kind="plane";
    }
    return {position:hit.point.clone(),orientation:direction?manipulatorFrame(direction,kind!):undefined};
  }

  private updateMoveTarget(): void {
    if (!this.moveTarget || !this.moveManipulator.isAttached() || !this.moveManipulator.isDragging()) return;
    const pivot = this.moveManipulator.candidatePose();
    const transformed=transformAroundWorldPivot(this.moveTarget.startPosition,this.moveTarget.startQuaternion,
      this.moveTarget.startPivot,pivot.position,pivot.rotation);
    const position=transformed.position;
    const rotation=transformed.rotation.toArray();
    const id=this.moveTarget.group.userData.id as string;
    this.desiredMovePose={translation:position.toArray(),rotation};
    this.pendingMovePreview={generation:this.movePreviewGeneration,instanceId:id,translation:this.desiredMovePose.translation,rotation};
    this.drainMovePreview();
  }

  private beginMovePreviewGesture():void{
	this.movePreviewGeneration+=1;this.pendingMovePreview=undefined;this.moveCommitPending=false;
	this.moveInteractionId=randomUUID();this.movePreviewSequence=0;
    const target=this.moveTarget;
    if(target){
      this.transforms.stop(target.group);
      target.startPosition.copy(target.group.position);
      target.startQuaternion.copy(target.group.quaternion);
      target.startPivot.copy(this.moveManipulator.object.getWorldPosition(new THREE.Vector3()));
      target.localPivot.copy(target.group.worldToLocal(target.startPivot.clone()));
		this.acceptedMovePose={translation:target.startPosition.toArray(),rotation:target.startQuaternion.toArray()};
    }
  }
  private cancelMovePreviewGesture():void{
    this.movePreviewGeneration+=1;this.pendingMovePreview=undefined;this.moveCommitPending=false;
  }
  private finishMovePreviewGesture():void{
    this.moveCommitPending=true;
    if(!this.movePreviewInFlight&&!this.pendingMovePreview)this.finishAcceptedMove();
  }
  private finishAcceptedMove():void{
    if(!this.moveCommitPending)return;
    this.moveCommitPending=false;
    const target=this.moveTarget;
    const pose=this.acceptedMovePose;
    if(target&&pose){
      this.transforms.finish(target.group);
      this.transforms.apply(target.group, this.transformPose(pose.translation, pose.rotation), "immediate");
      target.startPosition.copy(target.group.position);
      target.startQuaternion.copy(target.group.quaternion);
      const center=target.group.localToWorld(target.localPivot.clone());
      target.startPivot.copy(center);
      this.moveManipulator.commitPreviewFrame();
      this.moveManipulator.setAuthoritativePose(center);
    }
	this.commitTransform(pose?.previewId);
  }
  private drainMovePreview():void{
    if(this.movePreviewInFlight||!this.pendingMovePreview)return;const request=this.pendingMovePreview;this.pendingMovePreview=undefined;this.movePreviewInFlight=true;
	const previewSequence=++this.movePreviewSequence;
	void this.callbacks.instanceMovePreview(request.instanceId,request.translation,request.rotation,this.moveInteractionId,previewSequence).then(({poses,constraintLimited,previewId})=>{
      if(request.generation!==this.movePreviewGeneration)return;
      if(constraintLimited){this.invalidate();return;}
      const driven=poses.find((pose)=>pose.instanceId===request.instanceId),target=this.moveTarget;
      const transitions=[];
      for(const pose of poses){
        const group=this.instanceGroups.get(pose.instanceId);if(!group)continue;
        transitions.push({object:group,target:this.transformPose(pose.translation,pose.rotation),
          frame:driven&&target&&group===target.group?()=>this.syncMoveManipulatorToRenderedPose(target):undefined});
      }
      this.transforms.applyBatch(transitions,"preview");
      if(driven&&target){
		this.acceptedMovePose={translation:driven.translation,rotation:driven.rotation,previewId};
      }
    }).catch(()=>{}).finally(()=>{
      this.movePreviewInFlight=false;
      if(this.pendingMovePreview?.generation===this.movePreviewGeneration)this.drainMovePreview();
      else if(this.moveCommitPending)this.finishAcceptedMove();
    });
  }

  preselect(selection: Selection, notify = false): void {
    if (sameSelection(this.preselected, selection)) return;
    this.preselected = selection;
    this.refreshInteractionHighlights();
    if (notify) this.callbacks.preselectionChanged(selection);
    this.invalidate();
  }

  private refreshInteractionHighlights(): void {
    for (const object of this.highlightedRoots) this.applyHighlight(object, "default");
    this.highlightedRoots.clear();
    const withAssemblyReferences = (selections: readonly SelectionItem[]) => selections.flatMap((selection) =>
      selection.kind === "assembly-constraint" ? [selection, ...(this.assemblyConstraintReferences.get(selection.constraintId) ?? [])] : [selection]);
    this.replaceTopologyOverlays("preselected", withAssemblyReferences(this.preselected ? [this.preselected] : []));
    if (this.preselected) {
      for (const object of this.selectionIndex.objectsFor(this.preselected)) {
        this.applyHighlight(object, "hover"); this.highlightedRoots.add(object);
      }
    }
    this.replaceTopologyOverlays("selected", withAssemblyReferences(this.selected));
    for (const object of this.selectionIndex.objectsForMany(this.selected)) {
      this.applyHighlight(object, "selected"); this.highlightedRoots.add(object);
    }
  }

  private clearInteractionState(): void {
	for (const object of this.highlightedRoots) this.applyHighlight(object, "default");
	this.highlightedRoots.clear();
	this.replaceTopologyOverlays("preselected", []);
	this.replaceTopologyOverlays("selected", []);
	this.clearReferencePreview();
	this.selected = [];
	this.preselected = null;
  }

  dispose(): void {
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
    const bodyTreeNodeId = `${rootPath}/body`;
    for (const datum of view.datumPlanes ?? []) this.addDatumPlane(datum, this.helpers, true, {
      documentId: view.document.id, geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/plane:${datum.id}`,
    });
    for (const axis of view.axisSystems ?? []) this.addAxisSystem(axis, this.helpers, {
      documentId: view.document.id, geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/axis:${axis.id}`,
    });
    for (const axis of view.datumAxes ?? view.part?.datumAxes ?? []) this.addDatumAxis(axis, this.helpers, {
      documentId: view.document.id, geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/datum-axis:${axis.id}`,
    });
    if (view.artifact) this.addVisualPrimitives(view.artifact.visualization, this.helpers, {
      documentId: view.document.id, geometryKey: view.artifact.geometryKey, occurrencePath: "", treeNodeId: `${rootPath}/body`,
    }, false);
    const consumedSketches = new Set((view.part?.features ?? []).flatMap((feature) => feature.profile ? [feature.profile] : []));
    for (const feature of view.part?.features ?? []) {
      if (feature.type.toUpperCase().includes("SKETCH")) {
        this.addSketch(feature, false, view, undefined, undefined, undefined, !consumedSketches.has(feature.id));
      }
    }
    if (view.artifact && view.artifact.mesh.triangles.length > 0) {
      const solid = this.makeSolid(view.artifact, CATIA_VISUAL_THEME.surface, {
        documentId: view.document.id, versionId: view.document.versionId, geometryKey: view.artifact.geometryKey, occurrencePath: "",
        treeNodeId: resultBodyFeatureTreeNode(this.view?.structureTree, bodyTreeNodeId) ?? bodyTreeNodeId,
      });
      solid.userData = { kind: "body", id: "body-1" };
      this.content.add(solid);
      this.selectable.set("body:body-1", solid);
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
        occurrencePath: instance.id, instanceId: instance.id
      };
      group.userData = instanceSelection;
      this.content.add(group);
      this.instanceGroups.set(instance.id, group);
      this.selectable.set(`instance:${instance.id}`, group);
      this.selectionIndex.register(instanceSelection, group);
      const prefix = `${rootName}/${instance.id}`;
      for (const resolved of view.resolvedInstances ?? []) {
        if (!resolved.id.startsWith(prefix)) continue;
        const artifact = view.artifacts?.[resolved.geometryKey];
        if (!artifact) continue;
        const resolvedGroup = new THREE.Group();
        resolvedGroup.userData = { occurrencePath: resolved.occurrencePath };
        resolvedGroup.position.fromArray(resolved.translation).sub(new THREE.Vector3().fromArray(instance.translation))
          .applyQuaternion(instanceRotation.clone().invert());
        const resolvedRotation = new THREE.Quaternion().fromArray(resolved.rotation ?? [0, 0, 0, 1]);
        resolvedGroup.quaternion.copy(instanceRotation.clone().invert().multiply(resolvedRotation));
        if (artifact.mesh.triangles.length > 0) {
          const resultTreeNodeId = resultBodyFeatureTreeNode(this.view?.structureTree, resolved.bodyTreeNodeId) ?? resolved.bodyTreeNodeId;
          const context: SolidContext = {
            documentId: resolved.documentId, versionId: resolved.instancePath.segments.at(-1)?.resolvedVersionId, geometryKey: artifact.geometryKey,
            instancePath: resolved.instancePath, occurrencePath: resolved.occurrencePath, treeNodeId: resultTreeNodeId, instanceId: instance.id
          };
          const solid = this.makeSolid(artifact, CATIA_VISUAL_THEME.productSurface, context);
          solid.userData = { kind: "instance", id: instance.id };
          resolvedGroup.add(solid);
        }
        const visualContext = {
          documentId: resolved.documentId, geometryKey: artifact.geometryKey, occurrencePath: resolved.occurrencePath,
          treeNodeId: resolved.bodyTreeNodeId, instanceId: instance.id,
        };
        this.addReferenceGeometry(artifact.visualization.referenceGeometry, resolvedGroup, visualContext);
        this.addVisualPrimitives(artifact.visualization, resolvedGroup, visualContext, false);
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
    const glyphs: Record<import("../types").AssemblyConstraint["kind"], number> = {
      FIX: 2, RIGID: 7, COINCIDENT: 0, CONCENTRIC: 13, ANGLE: 12, DISTANCE: 8,
    };
    const statusColors: Record<import("../types").AssemblyConstraint["evaluationStatus"], number> = {
      VERIFIED: CATIA_VISUAL_THEME.constraint, NOT_UPDATED: CATIA_VISUAL_THEME.selected, IMPOSSIBLE: CATIA_VISUAL_THEME.sketchRedundant, BROKEN: CATIA_VISUAL_THEME.sketchInvalid,
    };
    for (const constraint of view.product?.constraints ?? []) {
      const references = [constraint.first, constraint.second].filter((value): value is AssemblyGeometryRef => Boolean(value));
      const located = references.map((reference) => {
        const exact = this.resolveAssemblyConstraintReference(reference);
        if (exact) return { reference, ...exact, exact: true };
        const instance = this.instanceGroups.get(reference.instanceId);
        if (!instance) return undefined;
        const selection: SelectionItem = { kind: "instance", id: reference.instanceId, instanceId: reference.instanceId,
          occurrencePath: reference.instanceId, visualKey: `occurrence:${reference.instanceId}` };
        return { reference, selection, object: instance, anchor: new THREE.Box3().setFromObject(instance).getCenter(new THREE.Vector3()), exact: false };
      }).filter((value): value is NonNullable<typeof value> => Boolean(value));
      if (!located.length) continue;
      const markerPosition = located.reduce((sum, value) => sum.add(value.anchor), new THREE.Vector3())
        .multiplyScalar(1 / located.length);
      const span = located.length > 1 ? located[0].anchor.distanceTo(located[1].anchor) : 0;
      markerPosition.z += Math.max(4, span * 0.08);
      const treeNodeId = `document:${view.document.id}/assembly-constraints/constraint:${constraint.id}`;
      const selection: SelectionItem = { kind: "assembly-constraint", id: constraint.id, constraintId: constraint.id,
        constraintType: constraint.kind, documentId: view.document.id, treeNodeId };
      const group = new THREE.Group(); group.position.copy(markerPosition); group.userData = selection;
      const pointGeometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3()]);
      const glyph = new THREE.Points(pointGeometry, this.materials.constraintGlyph(
        assemblyConstraintGlyph(constraint.evaluationStatus, glyphs[constraint.kind]), constraint.suppressed ? 0x808080 : statusColors[constraint.evaluationStatus], 22));
      glyph.renderOrder = 92;
      group.add(glyph);
      if (located.some((value) => !value.anchor.equals(markerPosition))) {
        const leaders = makeOcclusionVisibleSegments(located.map((value) => [value.anchor.clone().sub(markerPosition), new THREE.Vector3()]),
          statusColors[constraint.evaluationStatus], 1.75);
        leaders.renderOrder = 90; group.add(leaders);
      }
      this.helpers.add(group);
      this.selectionIndex.register(selection, group);
      this.selectionIndex.registerPick(glyph, () => selection, 90);
      this.assemblyConstraintReferences.set(constraint.id, located.map((value) => value.selection));
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
          if ((binding.artifact.mesh.faceIds[index] ?? -1) + 1 !== reference.topologyId) return;
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

  focusAssemblyReference(reference: AssemblyGeometryRef): boolean {
    const resolved = this.resolveAssemblyConstraintReference(reference);
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
      binding.artifact.mesh.triangles.forEach((triangle,index)=>{if((binding.artifact.mesh.faceIds[index]??-1)+1!==reference.topologyId)return;
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

  private resolveAssemblyConstraintReference(reference: AssemblyGeometryRef):
    { selection: SelectionItem; object: THREE.Object3D; anchor: THREE.Vector3 } | undefined {
    const instance = this.instanceGroups.get(reference.instanceId);
    if (!instance) return undefined;
    const instanceSelection: SelectionItem = { kind: "instance", id: reference.instanceId, instanceId: reference.instanceId,
      occurrencePath: reference.instanceId, visualKey: `occurrence:${reference.instanceId}` };
    const instanceCenter = () => new THREE.Box3().setFromObject(instance).getCenter(new THREE.Vector3());
    if (reference.kind === "BODY") return { selection: instanceSelection, object: instance, anchor: instanceCenter() };
    const resolvedTopology = reference.resolution?.result.status === "RESOLVED" ? reference.resolution.result.candidates?.[0] : undefined;
    const geometryKey = resolvedTopology?.geometryKey ?? reference.geometryKey;
    const topologyId = resolvedTopology?.localId ?? reference.topologyId;
    const binding = [...this.solidBindings.values()].find((candidate) => candidate.context.instanceId === reference.instanceId &&
      (!reference.instancePath || candidate.context.occurrencePath === reference.instancePath.canonical) &&
      (!geometryKey || candidate.artifact.geometryKey === geometryKey));
    if (reference.kind === "FACE" && binding && topologyId) {
      const anchor = new THREE.Vector3(); let count = 0;
      binding.artifact.mesh.triangles.forEach((triangle, index) => {
        if ((binding.artifact.mesh.faceIds[index] ?? -1) + 1 !== topologyId) return;
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
        (reference.instancePath && object.userData.occurrencePath !== reference.instancePath.canonical)) return;
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
    geometry.translate(0.54, 0.54, 0);
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
    const origin = new THREE.Vector3().fromArray(axis.origin);
    const direction = new THREE.Vector3().fromArray(axis.direction).normalize();
    const reference = new THREE.Group();
    reference.position.copy(origin);
    reference.quaternion.setFromUnitVectors(new THREE.Vector3(1, 0, 0), direction);
    const visibleLine = makeDatumReferenceLine([
      new THREE.Vector3(), new THREE.Vector3(1, 0, 0),
    ], 0xd89422, true);
    updateHighlightLineResolution(visibleLine, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
    const pickLine = new THREE.Line(new THREE.BufferGeometry().setFromPoints([
      new THREE.Vector3(), new THREE.Vector3(1, 0, 0),
    ]), new THREE.LineBasicMaterial({ transparent: true, opacity: 0, depthTest: false, depthWrite: false }));
    const selection = { kind: "axis" as const, axis: "DATUM" as const,
      id: `${context?.occurrencePath || "root"}:${axis.id}`, entityId: axis.id, treeNodeId: context?.treeNodeId,
      documentId: context?.documentId, occurrencePath: context?.occurrencePath, geometryKey: context?.geometryKey,
      instancePath: context?.instancePath, instanceId: context?.instanceId };
    pickLine.raycast = raycastDatumAxis;
    reference.userData = selection;
    pickLine.userData = selection;
    reference.add(visibleLine, pickLine);
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

  private addVisualPrimitives(visualization: VisualizationManifest | undefined, parent: THREE.Group, context: SolidContext, includeSketch = true): void {
    if (!visualization || visualization.schemaVersion !== 1) return;
    const sketchEntityObjects = new Map<string, THREE.Object3D>();
    const constraintAssociations: Array<{ selection: Extract<SelectionItem, { kind: "sketch-constraint" }>; featureID: string; entityIDs: string[] }> = [];
    for (const primitive of visualization.primitives ?? []) {
      if (!includeSketch && primitive.semantic.startsWith("SKETCH_")) continue;
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
        object.renderOrder = 84;
      } else if (primitive.semantic === "SKETCH_CONSTRAINT" && primitive.kind === "LINE_SEGMENTS") {
        geometry.dispose();
        const group = new THREE.Group();
        const segments: Array<[THREE.Vector3, THREE.Vector3]> = [];
        for (let index = 0; index + 1 < points.length; index += 2) segments.push([points[index], points[index + 1]]);
        const leaders = makeOcclusionVisibleSegments(segments, CATIA_VISUAL_THEME.constraint, 1.25);
        leaders.renderOrder = 82;
        updateHighlightLineResolution(leaders, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
        group.add(leaders);
        if (primitive.label && primitive.labelPosition) {
          const label = makeConstraintDimensionLabel(primitive.label);
          label.position.fromArray(primitive.labelPosition);
          label.renderOrder = 86;
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
      if (primitive.semantic !== "SKETCH_CONSTRAINT") object.renderOrder = primitive.kind === "POINTS" ? 22 : 20;
      const featureTreeNode = this.featureTreeNode(context, primitive.featureId);
      const selection = visualSelection(primitive, {
        treeNodeId: featureTreeNode ? primitive.semantic === "SKETCH_CONSTRAINT"
          ? constraintTreeNodeID(featureTreeNode, primitive.entityType as ConstraintKind, primitive.id) : `${featureTreeNode}/geometry/entity:${primitive.id}`
          : context.treeNodeId,
        documentId: context.documentId,
        occurrencePath: context.occurrencePath, geometryKey: context.geometryKey, instanceId: context.instanceId,
      });
      object.userData = { ...selection, sketchFeatureID: primitive.featureId, visualizationPrimitive: true };
      object.traverse((child) => {
        child.userData = { ...child.userData, ...selection, sketchFeatureID: primitive.featureId, visualizationPrimitive: true };
      });
      parent.add(object);
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
        child.visible = sketchTreeVisible({ selected: this.selected.some((selection) => selection.kind === "sketch" &&
          selection.id === child.userData.sketchFeatureID && (!selection.occurrencePath || selection.occurrencePath === child.userData.occurrencePath)), featureID: child.userData.sketchFeatureID, treeKey: child.userData.treeNodeId,
          activeSketchID: this.activeSketchID, defaultVisible: child.userData.visibleOutsideSketchEdit === true,
          overrides: this.treeVisibilityOverrides });
        for (const sketchChild of child.children) {
          if (sketchChild.userData.sketchEntityOverlay) sketchChild.visible = child.visible;
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
    const context = sourceContext ?? { documentId, geometryKey: sourceView?.artifact?.geometryKey ?? "",
      occurrencePath: "", treeNodeId: `document:${documentId}/body` };
    const featureTreeNode = this.featureTreeNode(context, feature.id)
      ?? `${context.treeNodeId}/sketch:${feature.id}`;
    const sketchSelection = { kind: "sketch" as const, id: feature.id, documentId,
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
    for (const entity of feature.sketch?.entities ?? []) {
      if (entity.suppressed) continue;
      const type = entity.kind === "POINT" ? "POINT" as const : "CURVE" as const;
      const entitySelection = { kind: "visual" as const, id: `${context.occurrencePath || "root"}:${feature.id}:${entity.id}`, visualType: type,
        featureId: feature.id, entityId: entity.id, role: entity.role, documentId, occurrencePath: context.occurrencePath,
        treeNodeId: `${featureTreeNode}/geometry/entity:${entity.id}` };
      let object: THREE.Object3D | undefined;
	  const component=feature.sketch?.solve.components?.find((candidate)=>candidate.entityIds.includes(entity.id));
	  const diagnosticStatus=component?.definitionStatus??feature.sketch?.solve.definitionStatus??component?.status??feature.sketch?.solve.status;
      const entityColor=conflictEntities.has(entity.id)?CATIA_VISUAL_THEME.sketchInvalid:sketchDiagnosticColor(diagnosticStatus, entity.role === "CONSTRUCTION");
      if (entity.kind === "POINT" && entity.point) {
        object = new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(plane, [entity.point.x, entity.point.y])]),
          this.materials.point(entityColor, 9, false));
        object.renderOrder = 22;
      } else {
        const sampled=sampleSketchEntity(entity);
        if(sampled.length<2)continue;
        const positions=sampled.map((point)=>localToWorld(plane,point));
        object = makeSketchOverlayLine(positions, entityColor, entity.role === "CONSTRUCTION" ? 2 : 2.5, entity.role === "CONSTRUCTION");
        updateHighlightLineResolution(object, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
        object.renderOrder = 20;
        const markers=entity.kind==="CIRCLE"&&entity.center?[localToWorld(plane,[entity.center.x,entity.center.y])]
          :entity.kind==="SPLINE"?(entity.controlPoints??[]).map((point)=>localToWorld(plane,[point.x,point.y]))
            :[positions[0],positions.at(-1)!];
        const endpointMarkers = new THREE.Points(new THREE.BufferGeometry().setFromPoints(markers),
          this.materials.point(CATIA_VISUAL_THEME.vertex, 8, false));
        endpointMarkers.userData = { sketchEntityOverlay: true }; endpointMarkers.renderOrder = 21; group.add(endpointMarkers);
      }
      if (!object) continue;
      object.userData = { ...entitySelection, sketchEntityOverlay: true }; group.add(object);
      sketchEntityObjects.set(entity.id, object);
      this.selectable.set(`visual:${entitySelection.id}`, object);
      this.selectionIndex.register(entitySelection, object);
      this.selectionIndex.registerPick(object, () => entitySelection, type === "POINT" ? 75 : 70);
    }
    const externalEntities = (feature.sketch?.externalGeometry ?? []).flatMap((external) => {
      const snapshot = external.snapshot;
      if (!snapshot) return [];
      const entity: SketchEntity = { id: external.id, kind: snapshot.kind, role: "CONSTRUCTION",
        point: snapshot.point, start: snapshot.start, end: snapshot.end, center: snapshot.center, radius: snapshot.radius };
      const type = snapshot.kind === "POINT" ? "POINT" as const : "CURVE" as const;
      const selection = { kind: "visual" as const, id: `${context.occurrencePath || "root"}:${feature.id}:${external.id}`, visualType: type,
        featureId: feature.id, entityId: external.id, role: "CONSTRUCTION" as const, documentId, occurrencePath: context.occurrencePath,
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
        object.renderOrder = 23; object.userData = { ...selection, sketchEntityOverlay: true }; group.add(object);
        sketchEntityObjects.set(external.id, object); this.selectable.set(`visual:${selection.id}`, object);
        this.selectionIndex.register(selection, object); this.selectionIndex.registerPick(object, () => selection, type === "POINT" ? 77 : 72);
      }
      return [entity];
    });
    for (const constraint of feature.sketch?.constraints ?? []) {
      if (constraint.suppressed) continue;
      if (this.dimensionDrag?.selection.featureId === feature.id && this.dimensionDrag.constraint.id === constraint.id) continue;
      const constraintSelection = { kind: "sketch-constraint" as const,
        id: `${context.occurrencePath || "root"}:${feature.id}:constraint:${constraint.id}`, featureId: feature.id,
        constraintId: constraint.id, constraintType: constraint.kind, documentId, occurrencePath: context.occurrencePath,
        treeNodeId: constraintTreeNodeID(featureTreeNode, constraint.kind, constraint.id) };
      const constraintGroup = makeSketchConstraintRenderable(constraint, [...(feature.sketch?.entities ?? []), ...externalEntities],
        (point) => localToWorld(plane, point), this.materials,
        { width: this.renderer.domElement.clientWidth, height: this.renderer.domElement.clientHeight },
        feature.sketch?.solve.conflictingConstraintIds?.includes(constraint.id) ? CATIA_VISUAL_THEME.sketchInvalid
          : feature.sketch?.solve.redundantConstraintIds?.includes(constraint.id) ? CATIA_VISUAL_THEME.sketchRedundant : undefined);
      if (constraintGroup.children.length === 0) continue;
      constraintGroup.userData = constraintSelection;
      constraintGroup.traverse((child) => { child.userData = constraintSelection;
        if (child !== constraintGroup) this.selectionIndex.registerPick(child, () => constraintSelection, 80); });
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

  private makeSolid(artifact: Artifact, color: number, context: SolidContext): THREE.Group {
    const geometry = makeGeometry(artifact);
    geometry.userData.navigationFaceIds = artifact.mesh.faceIds;
    const group = new THREE.Group();
    const mesh = new THREE.Mesh(geometry, this.materials.surface(color));
    mesh.raycast = acceleratedRaycast;
    markNavigationPickable(mesh);
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    const bodySelection = { kind: "body" as const, id: `${context.occurrencePath || "root"}:body`, ...context };
    this.selectionIndex.register(bodySelection, group);
    this.selectionIndex.registerVisualKey(`body:${bodySelection.id}`, group);
    const occurrenceParts = context.occurrencePath.split("/").filter(Boolean);
    for (let length = 1; length <= occurrenceParts.length; length++) {
      this.selectionIndex.registerVisualKey(`occurrence:${occurrenceParts.slice(0, length).join("/")}`, group);
    }
    this.selectable.set(`body:${bodySelection.id}`, group);
    this.selectionIndex.registerPick(mesh, (hit) => {
      const triangle = hit.faceIndex ?? -1;
      const localID = triangle >= 0 ? (artifact.mesh.faceIds[triangle] ?? 0) + 1 : 0;
      return localID > 0 ? {
        kind: "face", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:face:${localID}`,
        topologyId: localID, ...context
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
      this.selectionIndex.registerPick(edges, (hit) => {
        const segmentIndex = ((hit.index ?? 0) / 2) | 0;
        const localID = edgeIDs[segmentIndex] ?? 0;
        return {
          kind: "edge", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:edge:${localID}`,
          topologyId: localID, ...context
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
      this.selectionIndex.registerPick(points, (hit) => {
        const localID = topologyVertices[hit.index ?? 0]?.localId ?? 0;
        return {
          kind: "vertex", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:vertex:${localID}`,
          topologyId: localID, ...context
        };
      }, 50);
      group.add(points);
    }
    applySolidDisplaySettings(group, this.solidDisplay);
    this.solidBindings.set(context.occurrencePath || "root", { group, mesh, artifact, context });
    return group;
  }

  private pick(x: number, y: number, additive: boolean): void {
    if (this.moveManipulator.isDragging()) return;
    const hit = this.hitTest(x, y);
    if (!hit) { if (!additive) this.selectMany([]); return; }
    if (!additive) { this.selectMany([hit]); return; }
    const key = selectionKey(hit);
    this.selectMany(this.selected.some((selection) => selectionKey(selection) === key)
      ? this.selected.filter((selection) => selectionKey(selection) !== key) : [...this.selected, hit]);
  }

  private preselectAt(x: number, y: number): void {
    if (this.moveManipulator.isDragging() || this.navigation.activeAction !== "none") return;
    if (this.activeToolID === "select") this.clearSnapPreview();
    this.preselect(this.hitTest(x, y), true);
  }

  private hitTest(x: number, y: number, captureManipulatorAnchor = false): Selection {
    this.updateScreenStableReferences();
    const metrics = viewportMetrics(this.renderer);
    updateScreenLines(this.scene, this.camera, metrics.cssWidth, metrics.cssHeight);
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const worldPerPixel = worldUnitsPerCssPixel(this.camera, this.navigation.target, viewportMetrics(this.renderer));
    this.raycaster.params.Line = { threshold: worldPerPixel * 5 };
    this.raycaster.params.Points = { threshold: worldPerPixel * 7 };
    this.datumAxisPickToleranceWorld = worldPerPixel * 1.75;
    const hit = this.selectionIndex.pickWithIntersection(this.raycaster, (selection) =>
      this.activeToolID === "sketch.project" && (selection.kind === "edge" || selection.kind === "vertex")
        ? allowsSelection(this.captureSettings, selection)
        : allowsSelectionInContext(this.captureSettings, selection, this.activeSketchID));
    const raw=hit.selection && bindPublicationSelection(hit.selection, this.view?.structureTree);
    if(captureManipulatorAnchor&&this.activeToolID==="assembly.move"&&raw?.instanceId&&hit.intersection){
      this.pendingManipulatorAnchor={instanceId:raw.instanceId,anchor:this.manipulatorAnchorFromIntersection(hit.intersection)};
    }
    return this.selectionMode.project(projectSketchFeatureSelection(raw, this.activeSketchID));
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

  private beginDimensionDrag(x: number, y: number): boolean {
    if (!this.sketchPlane || !this.activeSketchID) return false;
    const hit = this.dimensionConstraintAt(x, y);
    if (!hit || hit.selection.featureId !== this.activeSketchID) return false;
    this.selectMany([hit.selection]);
    const root = this.selectable.get(`sketch-constraint:${hit.selection.id}`);
    this.dimensionDrag = { selection: hit.selection, constraint: hit.constraint, root, startX: x, startY: y };
    return true;
  }

  private updateDimensionDrag(x: number, y: number): void {
    const sketchView = this.sketchView();
    if (!this.dimensionDrag || !this.sketchPlane || !sketchView) return;
    if (Math.hypot(x - this.dimensionDrag.startX, y - this.dimensionDrag.startY) < 3 && !this.dimensionDrag.position) return;
    const position = this.rawSketchPoint(x, y); if (!position) return;
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
    const drag = this.dimensionDrag; this.dimensionDrag = undefined;
    if (!drag) return;
    this.clearReferencePreview();
    if (drag.position) {
      this.callbacks.sketchOperations(drag.selection.featureId, [{ type: "UPDATE_CONSTRAINT_PLACEMENT",
        constraintId: drag.constraint.id, labelPosition: { x: drag.position[0], y: drag.position[1] } }]);
      if (drag.root && !drag.root.parent) this.disposeRenderable(drag.root);
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
    if (!hit || hit.selection.featureId !== this.activeSketchID || hit.constraint.value === undefined ||
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

  private addTopologyOverlay(layer: "selected" | "preselected", selection: SelectionItem): void {
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
    const binding = this.solidBindings.get(selection.occurrencePath || "root");
    if (!binding || !selection.topologyId) return;
    const color = layer === "selected" ? CATIA_VISUAL_THEME.selected : CATIA_VISUAL_THEME.hover;
    let overlay: THREE.Object3D | undefined;
    if (selection.kind === "face") {
      const positions: number[] = [];
      binding.artifact.mesh.triangles.forEach((triangle, index) => {
        if ((binding.artifact.mesh.faceIds[index] ?? -1) + 1 !== selection.topologyId) return;
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
      const edge = (binding.artifact.mesh.edges ?? []).find((item) => item.localId === selection.topologyId);
      if (edge) overlay = makeOcclusionVisibleHighlightLine(
        edge.points.map((point) => new THREE.Vector3().fromArray(point)), color, layer === "selected" ? 5 : 4,
      );
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

  private sketchPoint(x: number, y: number): Vec2 | null {
    if (!this.sketchPlane) return null;
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const point = this.raycaster.ray.intersectPlane(rayPlane(this.sketchPlane), new THREE.Vector3());
    if (!point) { this.clearSnapPreview(); return null; }
    const raw = worldToLocal(this.sketchPlane, point);
    const activeFeature = this.sketchView()?.part?.features.find((feature) => feature.id === this.activeSketchID);
    const screen = (local: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, local).project(this.camera);
      return [(projected.x + 1) * this.renderer.domElement.clientWidth / 2,
        (1 - projected.y) * this.renderer.domElement.clientHeight / 2] as Vec2;
    };
    const first = screen(raw), second = screen([raw[0] + 1, raw[1]]);
    const pixelsPerUnit = Math.max(Math.hypot(second[0] - first[0], second[1] - first[1]), 1.0e-6);
    const snap = this.captureSettings.enabled
      ? resolveSketchSnap(raw, sketchReferenceEntities(activeFeature), pixelsPerUnit, adaptiveGridSpacing(this.camera, this.renderer.domElement.clientHeight),
        SKETCH_INPUT_POLICY.snapThresholdPixels, this.captureSettings.sketch, screen) : undefined;
    this.lastSketchSnap = snap;
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
    marker.renderOrder = 36;
    const ringPoints = Array.from({ length: 33 }, (_, index) => {
      const angle = index / 32 * Math.PI * 2;
      const radius = markerRadius * (snap.kind === "GRID" ? 1.15 : 1);
      return localToWorld(this.sketchPlane!, [snap.point[0] + Math.cos(angle) * radius, snap.point[1] + Math.sin(angle) * radius]);
    });
    const ring = new THREE.Line(new THREE.BufferGeometry().setFromPoints(ringPoints),
      new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.snap, depthTest: false, transparent: true, opacity: 0.92 }));
    ring.renderOrder = 35;
    group.userData.snapKind = snap.kind;
    group.add(ring, marker); this.scene.add(group); this.snapPreview = group; this.invalidate();
  }

  private clearSnapPreview(): void {
    if (!this.snapPreview) return;
    this.scene.remove(this.snapPreview); this.disposeRenderable(this.snapPreview);
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
    const line = new THREE.Line(
      new THREE.BufferGeometry().setFromPoints(points),
      new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.preview, depthTest: false }),
    );
    line.renderOrder=28;group.add(line);this.preview=group;this.scene.add(group);
    this.invalidate();
  }

  private drawPointPreview(point: Vec2, plane: PlaneName | SketchPlane): void {
    this.clearPreview();
    const group=new THREE.Group();const marker = new THREE.Points(
      new THREE.BufferGeometry().setFromPoints([localToWorld(plane, point)]),
      this.materials.point(CATIA_VISUAL_THEME.preview, 11, false),
    );
    marker.renderOrder = 28;group.add(marker);this.preview=group;this.scene.add(group);
    this.invalidate();
  }

  private clearPreview(): void {
    if (!this.preview) return;
    this.scene.remove(this.preview);
    this.disposeRenderable(this.preview);
    this.preview = undefined;
    this.invalidate();
  }

  private clearReferencePreview(): void {
    if (!this.referencePreview) return;
    this.scene.remove(this.referencePreview);
    this.disposeRenderable(this.referencePreview);
    this.referencePreview = undefined;
    this.invalidate();
  }

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
      const point=entity.controlPoints?.[reference.controlPointIndex];if(point)return new THREE.Points(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane,[point.x,point.y])]),this.materials.point(color,15,false));
    }
    const sampled = sampleSketchEntity(entity);
    if (sampled.length < 2) return undefined;
    const line = makeOcclusionVisibleHighlightLine(sampled.map((point) => localToWorld(this.sketchPlane!, point)), color, 4);
    updateHighlightLineResolution(line, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
    return line;
  }

  private showReferencePreview(reference: SketchGeometryRef, retained: readonly SketchGeometryRef[] = []): void {
    this.clearReferencePreview();
    const group = new THREE.Group();
    const same=retained.some((item)=>item.target===reference.target&&item.entityId===reference.entityId&&item.subElement===reference.subElement);
    for(const item of retained){const selected=this.makeReferencePreview(item,CATIA_VISUAL_THEME.selected);if(selected)group.add(selected);}
    if(!same){const candidate=this.makeReferencePreview(reference,CATIA_VISUAL_THEME.snap);if(candidate)group.add(candidate);}
    if (group.children.length === 0) return;
    this.referencePreview = group;
    this.referencePreview.renderOrder = 30;
    this.referencePreview.traverse((child) => { child.renderOrder = 30; });
    this.scene.add(this.referencePreview);
    this.invalidate();
  }

  private showConstraintPreview(kind: ConstraintKind, references: readonly SketchGeometryRef[], value?: number, labelPosition?: Vec2): void {
    this.clearReferencePreview();
    const sketchView = this.sketchView();
    if (!this.sketchPlane || !sketchView) return;
    const feature = sketchView.part?.features.find((candidate) => candidate.id === this.activeSketchID);
    if (!feature?.sketch) return;
    const group = new THREE.Group();
    for (const reference of references) {
      const highlight = this.makeReferencePreview(reference, CATIA_VISUAL_THEME.selected);
      if (highlight) group.add(highlight);
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
    this.scene.add(group);
    this.invalidate();
  }

  private toolViewportPort(): ToolViewportPort {
    return {
      sketchPoint: (x, y) => this.sketchPoint(x, y),
      sketchSnapReference: () => {
        const snap=this.lastSketchSnap;
        if(!snap)return undefined;
        if(snap.kind==="ORIGIN")return {target:"SKETCH_ORIGIN",subElement:"POINT"};
        if(snap.entityId && (snap.subElement==="POINT"||snap.subElement==="START"||snap.subElement==="END"||snap.subElement==="CENTER")) {
          const external = this.sketchView()?.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch?.externalGeometry
            ?.some((candidate) => candidate.id === snap.entityId);
          return {target:external?"EXTERNAL":"ENTITY",entityId:snap.entityId,subElement:snap.subElement};
        }
        return undefined;
      },
      sketchPlacementPoint: (x, y) => this.rawSketchPoint(x, y) ?? null,
      showPolylinePreview: (points, closed = false) => {
        if (this.sketchPlane) this.drawPreview(points, closed, this.sketchPlane);
      },
      showPointPreview: (point) => {
        if (this.sketchPlane) this.drawPointPreview(point, this.sketchPlane);
      },
      showReferenceDimensions: (geometry) => {
        if(!this.preview||!this.sketchPlane)return;
        for(const dimension of sketchReferenceDimensions(geometry)){const sprite=makeConstraintDimensionLabel(dimension.text);
          sprite.position.copy(localToWorld(this.sketchPlane,dimension.position));sprite.renderOrder=31;this.preview.add(sprite);}
        this.invalidate();
      },
      clearToolPreview: () => { this.clearPreview(); this.clearSnapPreview(); },
      commitSketchOperations: (operations) => { this.clearSnapPreview(); if (this.activeSketchID) this.callbacks.sketchOperations(this.activeSketchID, operations); },
      hasActiveSketch: () => Boolean(this.sketchPlane && this.activeSketchID),
      sketchReferenceAt: (x, y, kind, retained) => this.sketchReferenceAt(x, y, kind, retained),
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
      setToolPrompt: (prompt) => this.callbacks.toolPromptChanged(prompt),
      finishToolUse: () => this.callbacks.toolUseCompleted(),
      selectionAt: (x, y) => this.hitTest(x, y, true),
      commitExternalProjection: (selection) => {
        if (!this.activeSketchID || !selection.geometryKey || !selection.versionId) return;
        const externalID = this.reconnectExternalID ?? randomUUID();
        const type = this.reconnectExternalID ? "RECONNECT_EXTERNAL_GEOMETRY" as const : "ADD_EXTERNAL_GEOMETRY" as const;
        this.reconnectExternalID = undefined;
        this.callbacks.sketchOperations(this.activeSketchID, [{ type, externalId: externalID, geometryKey: selection.geometryKey,
          topologyId: selection.topologyId, topologyKind: selection.kind.toUpperCase() as "EDGE"|"VERTEX", sourceVersionId: selection.versionId }]);
      },
      currentSelections: () => [...this.selected],
      retainSelections: (selections) => this.selectMany(selections),
      requestAssemblyConstraint: (kind, references) => this.callbacks.assemblyConstraintRequested(kind, references),
      moveManipulatorPointerDown: (pointerId, x, y) => this.moveManipulator.pointerDown(pointerId, x, y, this.camera, this.renderer.domElement),
      moveManipulatorPointerMove: (pointerId, x, y) => this.moveManipulator.pointerMove(pointerId, x, y, this.camera, this.renderer.domElement),
      moveManipulatorPointerUp: (pointerId, commit) => this.moveManipulator.pointerUp(pointerId, commit),
    };
  }

  private sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind, retained?: SketchGeometryRef) {
    const sketchView = this.sketchView();
    if (!this.sketchPlane || !sketchView || !this.captureSettings.enabled) return null;
    const width = Math.max(this.renderer.domElement.clientWidth, 1);
    const height = Math.max(this.renderer.domElement.clientHeight, 1);
    const screen = (point: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, point).project(this.camera);
      return { x: (projected.x + 1) * width / 2, y: (1 - projected.y) * height / 2 };
    };
    const feature = sketchView.part?.features.find((candidate) => candidate.id === this.activeSketchID);
    const entities = sketchReferenceEntities(feature);
    const origin = localToWorld(this.sketchPlane, [0, 0]);
    const extents = ([[1, 0], [0, 1]] as Vec2[]).map(axis => {
      const ends = sketchAxisEndpoints(this.camera, origin, localToWorld(this.sketchPlane!, axis).sub(origin), width, height);
      return ends ? ends[1].distanceTo(origin) : 0;
    }) as Vec2;
    const reference = resolveSketchReference({ x, y }, entities, screen, kind, 12, extents, retained);
    if (!reference) return null;
    if (reference.entityId && feature?.sketch?.externalGeometry?.some((external) => external.id === reference.entityId)) reference.target = "EXTERNAL";
    const captureKind = reference.target === "SKETCH_ORIGIN" ? "ORIGIN"
      : reference.subElement === "START" || reference.subElement === "END" ? "ENDPOINT"
      : reference.subElement === "CENTER" ? "CENTER" : reference.subElement === "POINT" ? "POINT" : "CURVE";
    return this.captureSettings.sketch.includes(captureKind) ? reference : null;
  }

  private emitDebugState(): void {
    if (this.disposed) return;
    this.callbacks.debugStateChanged?.({
      input: this.input.getState(), activeTool: this.activeToolID,
      navigationProfile: this.navigationProfile, navigationAction: this.navigation.activeAction,
      selectionKeys: this.selected.map(selectionKey),
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

  private commitTransform(previewId?:string): void {
    const object = this.moveTarget?.group,pose=this.acceptedMovePose;
    if (!object?.userData.id || !pose) return;
	this.callbacks.instanceMoved(object.userData.id as string,pose.translation,pose.rotation,previewId);
    this.refreshContentBounds();
  }

  private applyHighlight(object: THREE.Object3D, state: "default" | "hover" | "selected"): void {
    this.materials.setInteractionState(object, state);
    object.traverse((child) => {
      const material = (child as THREE.Mesh).material;
      if (material instanceof THREE.MeshBasicMaterial) {
        material.opacity = state === "selected" ? 0.28 : state === "hover" ? 0.18 : 0.075;
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
    for (const root of [this.helpers, this.sketchContext, this.preview, this.referencePreview, this.snapPreview]) {
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
    updateHighlightLineResolution(this.helpers, width, height);
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
      child.traverse((object) => {
        const mesh = object as THREE.Mesh;
        mesh.geometry?.dispose();
        const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material];
        materials.forEach((material) => material?.dispose());
      });
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
        this.updateScreenStableReferences();
        const metrics=viewportMetrics(this.renderer);
        updateScreenLines(this.scene, this.camera, metrics.cssWidth, metrics.cssHeight);
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
        this.navigationHUD.render(this.renderer);
        if (this.viewTransition.active) this.invalidate();
      }
    });
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
