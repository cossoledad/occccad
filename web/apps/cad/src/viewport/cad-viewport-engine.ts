import * as THREE from "three";
import { mergeVertices } from "three/examples/jsm/utils/BufferGeometryUtils.js";
import { acceleratedRaycast, computeBoundsTree, disposeBoundsTree } from "three-mesh-bvh";
import { InputManager } from "../cad/input/input-manager";
import type { InputState } from "../cad/input/input-types";
import { InteractionRouter } from "../cad/interaction/interaction-router";
import { SelectionController } from "../cad/interaction/selection-controller";
import { SelectionIndex } from "../cad/interaction/selection-index";
import { AssemblyManipulator } from "../cad/interaction/assembly-manipulator";
import { selectionModeForTool, type SelectionMode } from "../cad/interaction/selection-mode";
import { sameSelection, sameSelections, selectionKey } from "../cad/interaction/selection-identity";
import { resultBodyFeatureTreeNode } from "../cad/interaction/selection-hierarchy";
import { resolveSketchReference, type SketchReferencePickKind } from "../cad/interaction/sketch-reference-pick";
import { resolveSketchSnap, type SketchSnapResult } from "../cad/interaction/sketch-snap";
import { allowsSelectionInContext, DEFAULT_CAPTURE_SETTINGS, type CaptureSettings } from "../cad/interaction/capture-settings";
import { NavigationController, type NavigationSnapshot } from "../cad/navigation/navigation-controller";
import { CatiaNavigationHUD } from "../cad/navigation/hud/catia-navigation-hud";
import { CAD_GEOMETRY_LAYER, markNavigationPickable, NavigationPicker } from "../cad/navigation/navigation-picker";
import type { NavigationAction, NavigationProfileID } from "../cad/navigation/navigation-profile";
import { CadBackground } from "../cad/rendering/cad-background";
import { CadMaterialFactory } from "../cad/rendering/cad-material-factory";
import { visualSelection, visualType } from "../cad/rendering/visualization-render-model";
import { CATIA_VISUAL_THEME } from "../cad/rendering/cad-visual-theme";
import { makeOcclusionVisibleHighlightLine, makeOcclusionVisibleSegments, updateHighlightLineResolution } from "../cad/rendering/interaction-highlight";
import { constraintSymbolCode, makeConstraintDimensionLabel, makeSketchConstraintRenderable } from "../cad/rendering/sketch-constraint-renderer";
import { isDimensionConstraintKind, type ConstraintKind } from "../cad/sketch/sketch-constraint-definition";
import { measureSketchDimension } from "../cad/sketch/sketch-constraint-layout";
import { sketchReferenceDimensions, SKETCH_INPUT_POLICY } from "../cad/sketch/sketch-input-policy";
import { sampleSketchEntity, sketchEntityPoint } from "../cad/sketch/sketch-geometry";
import { CadShaderLibrary } from "../cad/rendering/shader/cad-shader-library";
import { viewportMetrics } from "../cad/rendering/viewport-metrics";
import { ArcSketchTool, AssemblyConstraintTool, AssemblyMoveTool, CircleSketchTool, ConstraintSketchTool, LineSketchTool, LinearDimensionSketchTool, PointSketchTool, PolylineSketchTool, RectangleSketchTool, RegularPolygonSketchTool, SelectTool, SlotSketchTool, SplineSketchTool, type AssemblyConstraintToolKind, type ToolViewportPort } from "../cad/tool/cad-tool";
import { ToolManager } from "../cad/tool/tool-manager";
import type {
  Artifact, AssemblyGeometryRef, AxisSystem, DatumAxis, DatumPlane, DocumentStructureNode, DocumentView, Feature, PlaneName, ReferenceGeometry, Selection, SelectionItem, SketchConstraint, SketchGeometryRef, SketchOperation, SketchPlane, Vec2, Vec3, VisualizationManifest,
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
  instanceMoved: (instanceId: string, translation: Vec3, rotation:[number,number,number,number]) => void;
  instanceMovePreview: (instanceId:string,translation:Vec3,rotation:[number,number,number,number])=>Promise<{
    poses:Array<{instanceId:string;translation:Vec3;rotation:[number,number,number,number]}>;constraintLimited:boolean}>;
  assemblyConstraintRequested: (kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]) => void;
  debugStateChanged?: (state: ViewportDebugState) => void;
};

type SolidContext = {
  documentId: string; geometryKey: string; occurrencePath: string; treeNodeId: string; instanceId?: string;
};

type SolidBinding = { group: THREE.Group; mesh: THREE.Mesh; artifact: Artifact; context: SolidContext };
export type ViewportEditContext = { view: DocumentView; occurrencePath?: string; translation?: Vec3;
  rotation?: [number, number, number, number]; bodyTreeNodeId?: string };

export type ViewportDebugState = {
  input: InputState;
  activeTool: string;
  navigationProfile: NavigationProfileID;
  navigationAction: NavigationAction;
  navigation?: NavigationSnapshot;
  hudScreen?: { x: number; y: number };
};

const planeColors: Record<PlaneName | "CUSTOM", number> = { XY: CATIA_VISUAL_THEME.axisZ, XZ: CATIA_VISUAL_THEME.axisY, YZ: CATIA_VISUAL_THEME.axisX, CUSTOM: 0x42a5c6 };

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

function makeFeatureEdges(geometry: THREE.BufferGeometry): THREE.EdgesGeometry {
  // OCCT tessellation can repeat the same vertex for adjacent triangles/faces.
  // EdgesGeometry interprets those repetitions as open triangle boundaries and
  // makes a shaded solid look like a wireframe. Weld position-only geometry
  // before extracting display edges; keep the original mesh untouched.
  const edgeSource = new THREE.BufferGeometry();
  edgeSource.setAttribute("position", geometry.getAttribute("position").clone());
  if (geometry.index) edgeSource.setIndex(geometry.index.clone());
  const welded = mergeVertices(edgeSource, 1.0e-4);
  const edges = new THREE.EdgesGeometry(welded, 32);
  edgeSource.dispose();
  welded.dispose();
  return edges;
}

export class CadViewportEngine {
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(42, 1, 0.1, 5000);
  private readonly renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: "high-performance" });
  private readonly shaders = new CadShaderLibrary();
  private readonly materials = new CadMaterialFactory(this.shaders);
  private readonly background = new CadBackground(this.shaders);
  private readonly moveManipulator: AssemblyManipulator;
  private moveTarget?: { group: THREE.Group; startPosition: THREE.Vector3; startQuaternion: THREE.Quaternion; startPivot: THREE.Vector3 };
  private readonly navigation: NavigationController;
  private readonly navigationHUD: CatiaNavigationHUD;
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
  private readonly contentBounds = new THREE.Box3();
  private readonly selectable = new Map<string, THREE.Object3D>();
  private readonly selectionIndex = new SelectionIndex();
  private readonly solidBindings = new Map<string, SolidBinding>();
  private readonly instanceGroups = new Map<string, THREE.Group>();
  private readonly assemblyConstraintReferences = new Map<string, SelectionItem[]>();
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
  private assemblyPosePreview?: Map<string, { position: THREE.Vector3; rotation: THREE.Quaternion }>;
  private dimensionDrag?: { selection: Extract<SelectionItem, { kind: "sketch-constraint" }>; constraint: SketchConstraint;
    root?: THREE.Object3D; rootParent?: THREE.Object3D; rootIndex?: number; startX: number; startY: number; position?: Vec2 };
  private movePreviewGeneration=0;private movePreviewInFlight=false;
  private pendingMovePreview?:{generation:number;instanceId:string;translation:Vec3;rotation:[number,number,number,number]};
  private desiredMovePose?:{translation:Vec3;rotation:[number,number,number,number]};
  private acceptedMovePose?:{translation:Vec3;rotation:[number,number,number,number]};
  private activeToolID = "select";
  private selectionMode: SelectionMode = selectionModeForTool("select");
  private navigationProfile: NavigationProfileID = "default";
  private captureSettings: CaptureSettings = DEFAULT_CAPTURE_SETTINGS;
  private hiddenTreeKeys = new Set<string>();
  private readonly resizeObserver: ResizeObserver;
  private animationFrame = 0;
  private disposed = false;

  constructor(private readonly host: HTMLElement, private readonly callbacks: Callbacks) {
    this.scene.background = null;
    this.camera.position.set(310, -360, 270);
    this.camera.up.set(0, 0, 1);
    this.camera.layers.enable(CAD_GEOMETRY_LAYER);
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 1.5));
    this.renderer.shadowMap.enabled = false;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.autoClear = false;
    host.appendChild(this.renderer.domElement);

    this.moveManipulator = new AssemblyManipulator(this.shaders, {
      dragStarted: () => { this.navigation.setEnabled(false); this.beginMovePreviewGesture(); },
      changed: () => { this.updateMoveTarget(); this.invalidate(); },
      dragFinished: (commit) => {
        this.navigation.setEnabled(true); this.endMovePreviewGesture();
        if (commit) this.commitTransform();
        else if (this.moveTarget) {
          this.moveTarget.group.position.copy(this.moveTarget.startPosition);
          this.moveTarget.group.quaternion.copy(this.moveTarget.startQuaternion);
          this.attachMoveManipulator(); this.invalidate();
        }
      },
    });
    this.scene.add(this.moveManipulator.root);

    const groundGrid = new THREE.GridHelper(1200, 60);
    groundGrid.rotation.x = Math.PI / 2;
    groundGrid.position.z = -0.02;
    (groundGrid.material as THREE.Material).dispose();
    const groundMaterial = this.materials.edge(0x5c7281);
    groundMaterial.uniforms.uOpacity.value = 0.24;
    groundMaterial.depthWrite = false;
    groundGrid.material = groundMaterial as unknown as THREE.LineBasicMaterial;
    groundGrid.renderOrder = -10;
    this.environment.add(groundGrid);
    const hemisphere = new THREE.HemisphereLight(0xf4f7f8, 0x405261, 2.25);
    const keyLight = new THREE.DirectionalLight(0xffffff, 2.8);
    keyLight.position.set(-3, -4, 7);
    const fillLight = new THREE.DirectionalLight(0xadc9d8, 1.25);
    fillLight.position.set(5, 2, 3);
    this.environment.add(hemisphere, keyLight, fillLight);
    this.sketchContext.renderOrder = 15;
    this.scene.add(this.environment, this.content, this.helpers, this.sketchContext);

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
    }, navigationPicker, "default", import.meta.env.DEV && import.meta.env.VITE_INPUT_DEBUG === "true");
    this.navigationHUD = new CatiaNavigationHUD(this.shaders);
    this.tools = new ToolManager({ viewport: this.toolViewportPort() });
    this.tools.register(new SelectTool());
    this.tools.register(new AssemblyMoveTool());
    for (const kind of ["fix", "rigid", "coincident", "concentric", "angle", "distance"] as const)
      this.tools.register(new AssemblyConstraintTool(kind));
    this.tools.register(new PointSketchTool());
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
      this.host.classList.toggle("navigating", action !== "none" || Boolean(snapshot.catia?.hudVisible));
      if (action !== "none") this.clearSnapPreview();
      this.updateNavigationHUD(snapshot);
      this.invalidate();
      this.emitDebugState();
    });

    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(host);
    this.invalidate();
  }

  render(view: DocumentView, editContext?: ViewportEditContext): void {
    const retainedMoveSelection = this.activeToolID === "assembly.move" ? [...this.selected] : [];
    this.clearCommandPreview();
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
    if (view.document.type === "PART") this.renderPart(view);
    else this.renderProduct(view);
    if (view.document.type === "PRODUCT" && editContext?.view.document.type === "PART") {
      for (const feature of editContext.view.part?.features ?? []) {
        if (feature.type.toUpperCase().includes("SKETCH")) this.addSketch(feature, false, editContext.view, {
          documentId: editContext.view.document.id, geometryKey: editContext.view.artifact?.geometryKey ?? "",
          occurrencePath: editContext.occurrencePath ?? "", treeNodeId: editContext.bodyTreeNodeId ?? "",
        }, editContext.translation, editContext.rotation);
      }
    }
    this.updateSketchContextVisibility();
    this.applyTreeVisibility();
    this.refreshContentBounds();
    const validMoveSelection = retainedMoveSelection.filter((selection) => selection.kind === "instance" &&
      this.instanceGroups.has(selection.instanceId ?? selection.id));
    if (validMoveSelection.length === 1) this.selectMany(validMoveSelection, false);
    // this.frameContent();
    this.invalidate();
  }

  clear(): void {
    this.clearCommandPreview();
	this.clearInteractionState();
    this.view = undefined;
    this.moveManipulator.detach();
    this.moveTarget = undefined;
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.instanceGroups.clear();
    this.contentBounds.makeEmpty();
    this.invalidate();
  }

  setHiddenTreeKeys(keys: readonly string[]): void {
    this.hiddenTreeKeys = new Set(keys);
    if (this.view) this.render(this.view);
  }

  private applyTreeVisibility(): void {
    for (const root of [this.content, this.helpers]) root.traverse((object) => {
      const key = typeof object.userData.treeNodeId === "string" ? object.userData.treeNodeId : undefined;
      if (key && [...this.hiddenTreeKeys].some((hidden) => key === hidden || key.startsWith(`${hidden}/`))) object.visible = false;
    });
  }

  beginSketch(sketchID: string, plane: SketchPlane): void {
    this.activeSketchID = sketchID;
    this.sketchPlane = plane;
    this.moveManipulator.detach();
    this.select({ kind: "plane", id: plane.datumPlaneId, plane: plane.plane });
    this.navigation.setEnabled(true);
    const frame = planeFrame(plane);
    this.navigation.target.copy(frame.origin);
    this.camera.position.copy(frame.origin).addScaledVector(frame.normal, 420);
    this.camera.up.copy(frame.v);
    this.navigation.syncCamera();
    this.buildSketchContext();
    this.updateSketchContextVisibility();
    this.callbacks.toolPromptChanged("选择：选择草图元素，或从工具栏启动创建命令");
    this.invalidate();
  }

  endSketch(): void {
	this.clearInteractionState();
    this.sketchPlane = undefined;
    this.activeSketchID = undefined;
    this.host.classList.remove("drawing");
    this.tools.cancel();
    this.clearSnapPreview();
    this.navigation.setEnabled(true);
    this.disposeGroup(this.sketchContext);
    this.updateSketchContextVisibility();
    this.callbacks.toolPromptChanged("");
    this.frameContent();
  }

  setActiveTool(toolID: import("../state/workbench-store").WorkbenchToolID): void {
    this.tools.activate(toolID);
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

  previewArtifact(artifact: Artifact): void {
    this.clearCommandPreview();
    if (!artifact.mesh.vertices.length || !artifact.mesh.triangles.length) return;
    const geometry = makeGeometry(artifact);
    const group = new THREE.Group();
    const solid = new THREE.Mesh(geometry, new THREE.MeshPhongMaterial({ color: CATIA_VISUAL_THEME.commandPreview,
      transparent: true, opacity: 0.34, depthWrite: false, side: THREE.DoubleSide }));
    const edges = new THREE.LineSegments(makeFeatureEdges(geometry),
      new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.preview, transparent: true, opacity: 0.9, depthTest: false }));
    solid.renderOrder = 90; edges.renderOrder = 91; group.add(solid, edges);
    if (this.editContext?.translation) group.position.fromArray(this.editContext.translation);
    if (this.editContext?.rotation) group.quaternion.fromArray(this.editContext.rotation);
    this.scene.add(group); this.commandPreview = group; this.invalidate();
  }

  clearCommandPreview(): void {
    if (this.commandPreview) {
      this.scene.remove(this.commandPreview); this.disposeRenderable(this.commandPreview);
      this.commandPreview = undefined;
    }
    if (this.assemblyPosePreview) {
      for (const [id, pose] of this.assemblyPosePreview) {
        const group = this.instanceGroups.get(id);
        if (!group) continue;
        group.position.copy(pose.position); group.quaternion.copy(pose.rotation); group.updateMatrixWorld(true);
      }
      this.assemblyPosePreview = undefined;
    }
    this.invalidate();
  }

  previewAssemblyPoses(poses: Array<{instanceId:string;translation:Vec3;rotation:[number,number,number,number]}>): void {
    if (!this.assemblyPosePreview) {
      this.assemblyPosePreview = new Map([...this.instanceGroups].map(([id, group]) =>
        [id, { position: group.position.clone(), rotation: group.quaternion.clone() }]));
    }
    for (const pose of poses) {
      const group = this.instanceGroups.get(pose.instanceId);
      if (!group) continue;
      group.position.fromArray(pose.translation); group.quaternion.fromArray(pose.rotation); group.updateMatrixWorld(true);
    }
    this.invalidate();
  }

  setStandardView(view: "TOP" | "FRONT" | "RIGHT" | "ISO"): void {
    const box = new THREE.Box3().setFromObject(this.content);
    if (box.isEmpty()) box.setFromCenterAndSize(new THREE.Vector3(), new THREE.Vector3(180, 180, 100));
    const center = box.getCenter(new THREE.Vector3());
    const distance = this.fitDistance(box);
    const directions = {
      TOP: new THREE.Vector3(0, 0, 1).multiplyScalar(distance),
      FRONT: new THREE.Vector3(0, -1, 0).multiplyScalar(distance),
      RIGHT: new THREE.Vector3(1, 0, 0).multiplyScalar(distance),
      ISO: new THREE.Vector3(1, -1, 1).normalize().multiplyScalar(distance),
    };
    this.camera.position.copy(center).add(directions[view]);
    this.camera.up.set(0, view === "TOP" ? 1 : 0, view === "TOP" ? 0 : 1);
    this.navigation.target.copy(center);
    this.updateCameraClipping(box);
    this.navigation.syncCamera();
  }

  select(selection: Selection, notify = true): void {
    this.selectMany(selection ? [selection] : [], notify);
  }

  requestDimensionEdit(selection: Extract<SelectionItem, { kind: "sketch-constraint" }>, x?: number, y?: number): boolean {
    if (!this.view) return false;
    const feature = this.view.part?.features.find((candidate) => candidate.id === selection.featureId);
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
      if(this.activeToolID==="assembly.move"&&!this.moveManipulator.isAttached())this.attachMoveManipulator();
      return;
    }
    this.selected = unique;
    this.preselected = null;
    this.moveManipulator.detach();
    this.refreshInteractionHighlights();
    if (this.activeToolID === "assembly.move" && unique.length === 1 && unique[0].kind === "instance") {
      this.attachMoveManipulator();
    }
    if (notify) this.callbacks.selectionsChanged(unique);
    this.invalidate();
  }

  private attachMoveManipulator(): void {
    if (this.selected.length !== 1 || this.selected[0].kind !== "instance") return;
    const object = this.selectable.get(`instance:${this.selected[0].instanceId ?? this.selected[0].id}`);
    if (!(object instanceof THREE.Group)) return;
    const center = new THREE.Box3().setFromObject(object).getCenter(new THREE.Vector3());
    this.moveManipulator.attach(center);
    this.moveTarget = { group: object, startPosition: object.position.clone(), startQuaternion: object.quaternion.clone(), startPivot: center.clone() };
    this.desiredMovePose = { translation: object.position.toArray(), rotation: object.quaternion.toArray() };
    this.acceptedMovePose = { translation: object.position.toArray(), rotation: object.quaternion.toArray() };
  }

  private updateMoveTarget(): void {
    if (!this.moveTarget || !this.moveManipulator.isAttached()) return;
    const pivot = this.moveManipulator.candidatePose();
    const offset=this.moveTarget.startPosition.clone().sub(this.moveTarget.startPivot).applyQuaternion(pivot.rotation);
    const position=pivot.position.clone().add(offset);
    const rotation=pivot.rotation.clone().multiply(this.moveTarget.startQuaternion).toArray();
    const id=this.moveTarget.group.userData.id as string;
    this.desiredMovePose={translation:position.toArray(),rotation};
    this.pendingMovePreview={generation:this.movePreviewGeneration,instanceId:id,translation:this.desiredMovePose.translation,rotation};
    this.drainMovePreview();
  }

  private beginMovePreviewGesture():void{this.movePreviewGeneration+=1;this.pendingMovePreview=undefined;}
  private endMovePreviewGesture():void{this.movePreviewGeneration+=1;this.pendingMovePreview=undefined;}
  private drainMovePreview():void{
    if(this.movePreviewInFlight||!this.pendingMovePreview)return;const request=this.pendingMovePreview;this.pendingMovePreview=undefined;this.movePreviewInFlight=true;
    void this.callbacks.instanceMovePreview(request.instanceId,request.translation,request.rotation).then(({poses,constraintLimited})=>{
      if(request.generation!==this.movePreviewGeneration)return;
      if(constraintLimited){this.invalidate();return;}
      for(const pose of poses){const group=this.instanceGroups.get(pose.instanceId);if(group){group.position.fromArray(pose.translation);group.quaternion.fromArray(pose.rotation);group.updateMatrix();group.updateMatrixWorld(true);}}
      const driven=poses.find((pose)=>pose.instanceId===request.instanceId),target=this.moveTarget;
      if(driven&&target){
        this.acceptedMovePose={translation:driven.translation,rotation:driven.rotation};
        const center=new THREE.Box3().setFromObject(target.group).getCenter(new THREE.Vector3());
        const handleRotation=target.group.quaternion.clone().multiply(target.startQuaternion.clone().invert());
        this.moveManipulator.setAuthoritativePose(center,handleRotation);
      }
      this.invalidate();
    }).catch(()=>{}).finally(()=>{this.movePreviewInFlight=false;if(this.pendingMovePreview?.generation===this.movePreviewGeneration)this.drainMovePreview();});
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
    if (this.disposed) return;
    this.disposed = true;
    cancelAnimationFrame(this.animationFrame);
    this.resizeObserver.disconnect();
    this.input.dispose();
    this.clearPreview();
    this.clearReferencePreview();
    this.clearSnapPreview();
    this.clearCommandPreview();
    this.moveManipulator.dispose();
    this.navigationHUD.dispose();
    this.background.dispose();
    this.disposeGroup(this.environment);
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.disposeGroup(this.sketchContext);
    this.renderer.dispose();
    this.renderer.domElement.remove();
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
    }, view.artifact.mesh.triangles.length === 0);
    const consumedSketches = new Set((view.part?.features ?? []).flatMap((feature) => feature.profile ? [feature.profile] : []));
    for (const feature of view.part?.features ?? []) {
      if (feature.type.toUpperCase().includes("SKETCH") && (!consumedSketches.has(feature.id) || feature.id === this.activeSketchID)) this.addSketch(feature, false, view);
    }
    if (view.artifact && view.artifact.mesh.triangles.length > 0) {
      const solid = this.makeSolid(view.artifact, CATIA_VISUAL_THEME.surface, {
        documentId: view.document.id, geometryKey: view.artifact.geometryKey, occurrencePath: "",
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
        resolvedGroup.position.fromArray(resolved.translation).sub(new THREE.Vector3().fromArray(instance.translation))
          .applyQuaternion(instanceRotation.clone().invert());
        const resolvedRotation = new THREE.Quaternion().fromArray(resolved.rotation ?? [0, 0, 0, 1]);
        resolvedGroup.quaternion.copy(instanceRotation.clone().invert().multiply(resolvedRotation));
        if (artifact.mesh.triangles.length > 0) {
          const resultTreeNodeId = resultBodyFeatureTreeNode(this.view?.structureTree, resolved.bodyTreeNodeId) ?? resolved.bodyTreeNodeId;
          const context: SolidContext = {
            documentId: resolved.documentId, geometryKey: artifact.geometryKey,
            occurrencePath: resolved.occurrencePath, treeNodeId: resultTreeNodeId, instanceId: instance.id
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
    for (const constraint of view.product?.constraints ?? []) {
      const references = [constraint.first, constraint.second].filter((value): value is AssemblyGeometryRef => Boolean(value));
      const resolved = references.map((reference) => this.resolveAssemblyConstraintReference(reference)).filter(
        (value): value is { selection: SelectionItem; object: THREE.Object3D; anchor: THREE.Vector3 } => Boolean(value));
      if (!resolved.length) continue;
      const markerPosition = resolved.reduce((sum, value) => sum.add(value.anchor), new THREE.Vector3())
        .multiplyScalar(1 / resolved.length);
      const span = resolved.length > 1 ? resolved[0].anchor.distanceTo(resolved[1].anchor) : 0;
      markerPosition.z += Math.max(4, span * 0.08);
      const treeNodeId = `document:${view.document.id}/assembly-constraints/constraint:${constraint.id}`;
      const selection: SelectionItem = { kind: "assembly-constraint", id: constraint.id, constraintId: constraint.id,
        constraintType: constraint.kind, documentId: view.document.id, treeNodeId };
      const group = new THREE.Group(); group.position.copy(markerPosition); group.userData = selection;
      const pointGeometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3()]);
      const glyph = new THREE.Points(pointGeometry, this.materials.constraintGlyph(glyphs[constraint.kind], CATIA_VISUAL_THEME.constraint, 22));
      glyph.renderOrder = 92;
      group.add(glyph);
      if (resolved.some((value) => !value.anchor.equals(markerPosition))) {
        const leaders = makeOcclusionVisibleSegments(resolved.map((value) => [value.anchor.clone().sub(markerPosition), new THREE.Vector3()]),
          CATIA_VISUAL_THEME.constraint, 1.75);
        leaders.renderOrder = 90; group.add(leaders);
      }
      this.helpers.add(group);
      this.selectionIndex.register(selection, group);
      this.selectionIndex.registerPick(glyph, () => selection, 90);
      this.assemblyConstraintReferences.set(constraint.id, resolved.map((value) => value.selection));
      for (const [index, reference] of resolved.entries()) {
        // Topology references are highlighted by exact face/edge/vertex overlays. Associating
        // their owning mesh group would incorrectly highlight the complete occurrence.
        if (!['FACE', 'EDGE', 'VERTEX'].includes(references[index]?.kind ?? ''))
          this.selectionIndex.associate(selection, reference.object);
        this.selectionIndex.associate(reference.selection, group);
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
    const binding = [...this.solidBindings.values()].find((candidate) => candidate.context.instanceId === reference.instanceId &&
      (!reference.geometryKey || candidate.artifact.geometryKey === reference.geometryKey));
    if (reference.kind === "FACE" && binding && reference.topologyId) {
      const anchor = new THREE.Vector3(); let count = 0;
      binding.artifact.mesh.triangles.forEach((triangle, index) => {
        if ((binding.artifact.mesh.faceIds[index] ?? -1) + 1 !== reference.topologyId) return;
        for (const vertexIndex of triangle) { anchor.add(binding.mesh.localToWorld(new THREE.Vector3().fromArray(binding.artifact.mesh.vertices[vertexIndex]))); count++; }
      });
      const selection: SelectionItem = { kind: "face", id: `${binding.context.occurrencePath}:${binding.artifact.geometryKey}:face:${reference.topologyId}`,
        topologyId: reference.topologyId, ...binding.context };
      return { selection, object: binding.group, anchor: count ? anchor.multiplyScalar(1 / count) : instanceCenter() };
    }
    if (reference.kind === "EDGE" && binding && reference.topologyId) {
      const edge=binding.artifact.mesh.edges?.find((candidate)=>candidate.localId===reference.topologyId);
      const anchor=new THREE.Vector3();for(const point of edge?.points??[])anchor.add(binding.mesh.localToWorld(new THREE.Vector3().fromArray(point)));
      const selection:SelectionItem={kind:"edge",id:`${binding.context.occurrencePath}:${binding.artifact.geometryKey}:edge:${reference.topologyId}`,
        topologyId:reference.topologyId,...binding.context};
      return {selection,object:binding.group,anchor:edge?.points.length?anchor.multiplyScalar(1/edge.points.length):instanceCenter()};
    }
    if (reference.kind === "VERTEX" && binding && reference.topologyId) {
      const vertex=binding.artifact.mesh.topologyVertices?.find((candidate)=>candidate.localId===reference.topologyId);
      const selection:SelectionItem={kind:"vertex",id:`${binding.context.occurrencePath}:${binding.artifact.geometryKey}:vertex:${reference.topologyId}`,
        topologyId:reference.topologyId,...binding.context};
      return {selection,object:binding.group,anchor:vertex?binding.mesh.localToWorld(new THREE.Vector3().fromArray(vertex.point)):instanceCenter()};
    }
    let matched: THREE.Object3D | undefined;
    instance.traverse((object) => {
      if (matched || object.userData.entityId !== reference.geometryId) return;
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
    const geometry = new THREE.PlaneGeometry(datum.size || 180, datum.size || 180);
    const material = new THREE.MeshBasicMaterial({
      color: planeColors[plane], transparent: true, opacity: 0.075,
      side: THREE.DoubleSide, depthWrite: false,
    });
    const mesh = new THREE.Mesh(geometry, material);
    mesh.position.fromArray(datum.origin);
    mesh.quaternion.setFromUnitVectors(new THREE.Vector3(0, 0, 1), new THREE.Vector3().fromArray(datum.normal).normalize());
    const selection = {
      kind: "plane" as const, id: `${context?.occurrencePath || "root"}:${id}`, entityId: id, plane, datumPlane: datum,
      treeNodeId: context?.treeNodeId, documentId: context?.documentId, occurrencePath: context?.occurrencePath,
      geometryKey: context?.geometryKey, instanceId: context?.instanceId
    };
    mesh.userData = selection;
    const edge = new THREE.LineSegments(
      new THREE.EdgesGeometry(geometry),
      new THREE.LineBasicMaterial({ color: planeColors[plane], transparent: true, opacity: 0.5 }),
    );
    mesh.add(edge);
    parent.add(mesh);
    if (selectable || context) {
      this.selectable.set(`plane:${selection.id}`, mesh);
      this.selectionIndex.register(selection, mesh);
      this.selectionIndex.registerPick(mesh, () => selection, 8);
    }
  }

  private addDatumAxis(axis: DatumAxis, parent: THREE.Group, context?: SolidContext): void {
    const origin = new THREE.Vector3().fromArray(axis.origin);
    const direction = new THREE.Vector3().fromArray(axis.direction).normalize();
    const half = 90;
    const line = new THREE.Line(new THREE.BufferGeometry().setFromPoints([
      origin.clone().addScaledVector(direction, -half), origin.clone().addScaledVector(direction, half),
    ]), new THREE.LineDashedMaterial({ color: 0xd89422, dashSize: 8, gapSize: 5 }));
    line.computeLineDistances();
    const selection = { kind: "axis" as const, axis: "DATUM" as const,
      id: `${context?.occurrencePath || "root"}:${axis.id}`, entityId: axis.id, treeNodeId: context?.treeNodeId,
      documentId: context?.documentId, occurrencePath: context?.occurrencePath, geometryKey: context?.geometryKey,
      instanceId: context?.instanceId };
    line.userData = selection; parent.add(line);
    this.selectionIndex.register(selection, line); this.selectionIndex.registerPick(line, () => selection, 48);
  }

  private addAxisSystem(axis: AxisSystem, parent: THREE.Group, context?: SolidContext): void {
    const length = 38;
    const origin = new THREE.Vector3().fromArray(axis.origin);
    const system = new THREE.Group();
    const systemSelection = {
      kind: "axis-system" as const, id: `${context?.occurrencePath || "root"}:${axis.id}`,
      entityId: axis.id,
      treeNodeId: context?.treeNodeId, documentId: context?.documentId, occurrencePath: context?.occurrencePath,
      geometryKey: context?.geometryKey, instanceId: context?.instanceId
    };
    const definitions = [["X", axis.xDirection, 0xe62e24], ["Y", axis.yDirection, 0x29b849], ["Z", axis.zDirection, 0x3478e5]] as const;
    for (const [name, direction, color] of definitions) {
      const geometry = new THREE.BufferGeometry().setFromPoints([origin,
        origin.clone().add(new THREE.Vector3().fromArray(direction).multiplyScalar(length))]);
      const line = new THREE.Line(geometry, new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.95 }));
      const selection = {
        ...systemSelection, kind: "axis" as const, axis: name, id: `${systemSelection.id}:${name}`,
        treeNodeId: context?.treeNodeId ? `${context.treeNodeId}/${name.toLowerCase()}` : undefined
      };
      line.userData = selection; system.add(line);
      this.selectionIndex.register(selection, line, context?.treeNodeId);
      this.selectionIndex.registerPick(line, () => selection, 45);
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
        object = new THREE.Line(geometry, new THREE.LineBasicMaterial({ color, depthTest: false }));
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
    const minorGridPositions: number[] = [];
    const majorGridPositions: number[] = [];
    for (let coordinate = -100; coordinate <= 100; coordinate += 10) {
      for (const [first, second] of [
        [[coordinate, -100], [coordinate, 100]], [[-100, coordinate], [100, coordinate]],
      ] as Array<[Vec2, Vec2]>) {
        for (const point of [first, second]) {
          const world = localToWorld(this.sketchPlane, point);
          const target = coordinate % 50 === 0 ? majorGridPositions : minorGridPositions;
          target.push(world.x, world.y, world.z);
        }
      }
    }
    const grid = (positions: number[], color: number, opacity: number) => {
      const lines = new THREE.LineSegments(
        new THREE.BufferGeometry().setAttribute("position", new THREE.Float32BufferAttribute(positions, 3)),
        new THREE.LineBasicMaterial({ color, transparent: true, opacity, depthTest: false }));
      lines.renderOrder = 14; this.sketchContext.add(lines);
    };
    grid(minorGridPositions, CATIA_VISUAL_THEME.gridMinor, 0.16);
    grid(majorGridPositions, CATIA_VISUAL_THEME.gridMajor, 0.3);
    const axis = (first: Vec2, second: Vec2, color: number) => {
      const muted=new THREE.Color(color).lerp(new THREE.Color(CATIA_VISUAL_THEME.backgroundBottom),0.28);
      const line = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane!, first), localToWorld(this.sketchPlane!, second)]),
        new THREE.LineBasicMaterial({ color:muted, depthTest:false, depthWrite:false }),
      );
      // Transparent axes are deferred until after opaque sketch curves and can
      // cover coincident highlighted geometry. Keep the baseline in the early
      // opaque pass; explicit axis hover/selection uses a separate overlay.
      line.renderOrder = 12;
      this.sketchContext.add(line);
    };
    axis([-110, 0], [110, 0], CATIA_VISUAL_THEME.axisX);
    axis([0, -110], [0, 110], CATIA_VISUAL_THEME.axisY);
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
    this.environment.visible = !editing || inContext;
    this.content.visible = !editing || inContext;
    this.sketchContext.visible = editing;
    for (const child of this.helpers.children) {
      if (child.userData.visualizationPrimitive) {
        child.visible = !editing;
      } else if (child.userData.sketchEditOverlay) {
        const active = child.userData.sketchFeatureID === this.activeSketchID;
        child.visible = editing && active;
        for (const sketchChild of child.children) {
          if (sketchChild.userData.sketchEntityOverlay) sketchChild.visible = editing && active;
        }
      } else child.visible = !editing || child.userData.sketchFeatureID === this.activeSketchID;
    }
  }

  private addSketch(feature: Feature, _includeEntities = true, sourceView = this.view,
    sourceContext?: SolidContext, translation?: Vec3, rotation?: [number, number, number, number]): void {
    const support = feature.sketch?.support;
    const datum = sourceView?.datumPlanes?.find((candidate) => candidate.id === support?.datumPlaneId)
      ?? sourceView?.artifact?.visualization.referenceGeometry.datumPlanes?.find((candidate) => candidate.id === support?.datumPlaneId);
    const plane: PlaneName | SketchPlane = datum ? { datumPlaneId: datum.id, plane: datum.plane, origin: datum.origin,
      normal: datum.normal, uDirection: datum.uDirection } : (support?.plane as PlaneName ?? feature.plane ?? "XY");
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
        object = new THREE.Line(new THREE.BufferGeometry().setFromPoints(positions), new THREE.LineBasicMaterial({ color: entityColor, depthTest: false }));
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
    for (const constraint of feature.sketch?.constraints ?? []) {
      if (constraint.suppressed) continue;
      if (this.dimensionDrag?.selection.featureId === feature.id && this.dimensionDrag.constraint.id === constraint.id) continue;
      const constraintSelection = { kind: "sketch-constraint" as const,
        id: `${context.occurrencePath || "root"}:${feature.id}:constraint:${constraint.id}`, featureId: feature.id,
        constraintId: constraint.id, constraintType: constraint.kind, documentId, occurrencePath: context.occurrencePath,
        treeNodeId: constraintTreeNodeID(featureTreeNode, constraint.kind, constraint.id) };
      const constraintGroup = makeSketchConstraintRenderable(constraint, feature.sketch?.entities ?? [],
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
    group.userData = { ...sketchSelection, sketchFeatureID: feature.id, sketchEditOverlay: true };
    this.helpers.add(group);
    this.selectable.set(`sketch:${feature.id}`, group);
    this.selectionIndex.register(sketchSelection, group);
  }

  private makeSolid(artifact: Artifact, color: number, context: SolidContext): THREE.Group {
    const geometry = makeGeometry(artifact);
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
      const edges = new THREE.LineSegments(edgeGeometry, this.materials.edge());
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
      this.selectionIndex.registerPick(points, (hit) => {
        const localID = topologyVertices[hit.index ?? 0]?.localId ?? 0;
        return {
          kind: "vertex", id: `${context.occurrencePath || "root"}:${artifact.geometryKey}:vertex:${localID}`,
          topologyId: localID, ...context
        };
      }, 50);
      group.add(points);
    }
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

  private hitTest(x: number, y: number): Selection {
    this.updatePointer(x, y);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const distance = Math.max(this.camera.position.distanceTo(this.navigation.target), 1);
    const worldPerPixel = 2 * distance * Math.tan(THREE.MathUtils.degToRad(this.camera.fov / 2)) /
      Math.max(this.renderer.domElement.clientHeight, 1);
    this.raycaster.params.Line = { threshold: worldPerPixel * 5 };
    this.raycaster.params.Points = { threshold: worldPerPixel * 7 };
    const raw = this.selectionIndex.pick(this.raycaster,
      (selection) => allowsSelectionInContext(this.captureSettings, selection, this.activeSketchID));
    return this.selectionMode.project(raw);
  }

  private dimensionConstraintAt(x: number, y: number) {
    const selection = this.hitTest(x, y);
    if (!selection || selection.kind !== "sketch-constraint" || !this.view) return undefined;
    const feature = this.view.part?.features.find((candidate) => candidate.id === selection.featureId);
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
    if (!this.dimensionDrag || !this.sketchPlane || !this.view) return;
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
    const feature = this.view.part?.features.find((candidate) => candidate.id === this.dimensionDrag!.selection.featureId);
    if (!feature?.sketch) return;
    const previewConstraint = { ...this.dimensionDrag.constraint, labelPosition: { x: position[0], y: position[1] } };
    this.referencePreview = makeSketchConstraintRenderable(previewConstraint, feature.sketch.entities,
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
      const constraint=this.view?.part?.features.find((feature)=>feature.id===selection.featureId)?.sketch?.constraints
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
    const active = this.view?.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch;
    const screen = (local: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, local).project(this.camera);
      return [(projected.x + 1) * this.renderer.domElement.clientWidth / 2,
        (1 - projected.y) * this.renderer.domElement.clientHeight / 2] as Vec2;
    };
    const first = screen(raw), second = screen([raw[0] + 1, raw[1]]);
    const pixelsPerUnit = Math.max(Math.hypot(second[0] - first[0], second[1] - first[1]), 1.0e-6);
    const snap = this.captureSettings.enabled
      ? resolveSketchSnap(raw, active?.entities ?? [], pixelsPerUnit, SKETCH_INPUT_POLICY.gridSpacing,
        SKETCH_INPUT_POLICY.snapThresholdPixels, this.captureSettings.sketch) : undefined;
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
      const points: [Vec2, Vec2] = reference.target === "SKETCH_X_AXIS"
        ? [[-110, 0], [110, 0]] : [[0, -110], [0, 110]];
      const line = makeOcclusionVisibleHighlightLine(points.map((point) => localToWorld(this.sketchPlane!, point)), color, 4);
      updateHighlightLineResolution(line, this.renderer.domElement.clientWidth, this.renderer.domElement.clientHeight);
      return line;
    }
    if (!this.view || !reference.entityId) return undefined;
    const entity = this.view.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch?.entities
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
    if (!this.sketchPlane || !this.view) return;
    const feature = this.view.part?.features.find((candidate) => candidate.id === this.activeSketchID);
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
    group.add(makeSketchConstraintRenderable(constraint, feature.sketch.entities,
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
        if(snap.entityId && (snap.subElement==="POINT"||snap.subElement==="START"||snap.subElement==="END"||snap.subElement==="CENTER"))
          return {target:"ENTITY",entityId:snap.entityId,subElement:snap.subElement};
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
        const sketch = this.view?.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch;
        return sketch ? measureSketchDimension(kind, references, sketch.entities) : undefined;
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
      selectionAt: (x, y) => this.hitTest(x, y),
      retainSelections: (selections) => this.selectMany(selections),
      requestAssemblyConstraint: (kind, references) => this.callbacks.assemblyConstraintRequested(kind, references),
      moveManipulatorPointerDown: (pointerId, x, y) => this.moveManipulator.pointerDown(pointerId, x, y, this.camera, this.renderer.domElement),
      moveManipulatorPointerMove: (pointerId, x, y) => this.moveManipulator.pointerMove(pointerId, x, y, this.camera, this.renderer.domElement),
      moveManipulatorPointerUp: (pointerId, commit) => this.moveManipulator.pointerUp(pointerId, commit),
    };
  }

  private sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind, retained?: SketchGeometryRef) {
    if (!this.sketchPlane || !this.view || !this.captureSettings.enabled) return null;
    const width = Math.max(this.renderer.domElement.clientWidth, 1);
    const height = Math.max(this.renderer.domElement.clientHeight, 1);
    const screen = (point: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, point).project(this.camera);
      return { x: (projected.x + 1) * width / 2, y: (1 - projected.y) * height / 2 };
    };
    const entities = this.view.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch?.entities ?? [];
    const reference = resolveSketchReference({ x, y }, entities, screen, kind, 12, 110, retained);
    if (!reference) return null;
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
      navigation: this.navigation.snapshot, hudScreen: this.navigationHUD.screenPosition
    });
  }

  private updateNavigationHUD(snapshot = this.navigation.snapshot): void {
    this.navigationHUD.update(
      snapshot.profile === "catia" ? snapshot.catia : undefined,
      this.camera,
      this.renderer.domElement.clientWidth,
      this.renderer.domElement.clientHeight,
    );
  }

  private commitTransform(): void {
    const object = this.moveTarget?.group,pose=this.acceptedMovePose;
    if (!object?.userData.id || !pose) return;
    this.callbacks.instanceMoved(object.userData.id as string,pose.translation,pose.rotation);
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
    const box = this.contentBounds.clone();
    if (box.isEmpty()) {
      box.setFromCenterAndSize(new THREE.Vector3(), new THREE.Vector3(180, 180, 100));
    }

    const newCenter = box.getCenter(new THREE.Vector3());
    const distance = this.fitDistance(box);

    // Preserve the user's current viewing direction when fitting new content.
    const viewDir = this.camera.getWorldDirection(new THREE.Vector3()).negate().normalize();

    // 如果相机恰好在原 target 点上导致 viewDir 为零，给定一个默认方向 fallback
    if (viewDir.lengthSq() === 0) {
      viewDir.set(1, -1.2, 0.8).normalize();
    }

    this.navigation.target.copy(newCenter);

    // 3. 沿原视线方向拉远/拉近相机，将位置移动到新中心偏移 distance 的地方
    this.camera.position.copy(newCenter).addScaledVector(viewDir, distance);

    this.updateCameraClipping(box);
    this.navigation.syncCamera();
  }

  private fitDistance(box: THREE.Box3): number {
    const sphere = box.getBoundingSphere(new THREE.Sphere());
    const radius = Math.max(sphere.radius, 1);
    const verticalFov = THREE.MathUtils.degToRad(this.camera.fov);
    const horizontalFov = 2 * Math.atan(Math.tan(verticalFov / 2) * Math.max(this.camera.aspect, 0.01));
    return radius / Math.sin(Math.min(verticalFov, horizontalFov) / 2) * 1.15;
  }

  private updateCameraClipping(box?: THREE.Box3): void {
    // Geometry bounds are cached when the document changes. High-frequency
    // navigation therefore never traverses a large Product scene per move.
    const bounds = box ?? this.contentBounds;
    const sphere = bounds.isEmpty()
      ? new THREE.Sphere(this.navigation.target.clone(), 100)
      : bounds.getBoundingSphere(new THREE.Sphere());
    const distance = this.camera.position.distanceTo(this.navigation.target);
    const radius = Math.max(sphere.radius, 1.0e-3);
    this.camera.near = Math.max(1.0e-4, Math.min(radius * 1.0e-3, distance * 0.1));
    this.camera.far = Math.max(this.camera.near * 1000, distance + radius * 20, 1000);
    this.camera.updateProjectionMatrix();
  }

  private resize(): void {
    const width = Math.max(this.host.clientWidth, 1);
    const height = Math.max(this.host.clientHeight, 1);
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
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
    this.animationFrame = requestAnimationFrame(() => {
      this.animationFrame = 0;
      if (!this.disposed) {
        this.updateNavigationHUD();
        this.moveManipulator.updateScale(this.camera, viewportMetrics(this.renderer));
        const metrics=viewportMetrics(this.renderer);
        this.scene.traverse((object)=>{
          const material=(object as THREE.Points).material;
          if(material instanceof THREE.ShaderMaterial&&material.uniforms.uPointSize&&typeof material.userData.cssPointSize==="number")
            material.uniforms.uPointSize.value=material.userData.cssPointSize*metrics.devicePixelRatio;
        });
        this.renderer.clear(true, true, true);
        this.background.render(this.renderer);
        this.renderer.clearDepth();
        this.renderer.render(this.scene, this.camera);
        this.navigationHUD.render(this.renderer);
      }
    });
  }
}
