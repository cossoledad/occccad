import * as THREE from "three";
import { TransformControls } from "three/examples/jsm/controls/TransformControls.js";
import { mergeVertices } from "three/examples/jsm/utils/BufferGeometryUtils.js";
import { acceleratedRaycast, computeBoundsTree, disposeBoundsTree } from "three-mesh-bvh";
import { CommandRegistry } from "../cad/command/command-registry";
import { InputManager } from "../cad/input/input-manager";
import type { InputState } from "../cad/input/input-types";
import { InteractionRouter } from "../cad/interaction/interaction-router";
import { SelectionController } from "../cad/interaction/selection-controller";
import { sameSelection, SelectionIndex } from "../cad/interaction/selection-index";
import { resolveSketchReference, type SketchReferencePickKind } from "../cad/interaction/sketch-reference-pick";
import { resolveSketchSnap, type SketchSnapResult } from "../cad/interaction/sketch-snap";
import { NavigationController, type NavigationSnapshot } from "../cad/navigation/navigation-controller";
import { CatiaNavigationHUD } from "../cad/navigation/hud/catia-navigation-hud";
import { CAD_GEOMETRY_LAYER, markNavigationPickable, NavigationPicker } from "../cad/navigation/navigation-picker";
import type { NavigationAction, NavigationProfileID } from "../cad/navigation/navigation-profile";
import { CadBackground } from "../cad/rendering/cad-background";
import { CadMaterialFactory } from "../cad/rendering/cad-material-factory";
import { visualSelection, visualType } from "../cad/rendering/visualization-render-model";
import { CATIA_VISUAL_THEME } from "../cad/rendering/cad-visual-theme";
import { sampleSketchEntity, sketchEntityPoint } from "../cad/sketch/sketch-geometry";
import { CadShaderLibrary } from "../cad/rendering/shader/cad-shader-library";
import { ShortcutManager } from "../cad/shortcut/shortcut-manager";
import { ArcSketchTool, CircleSketchTool, ConstraintSketchTool, LineSketchTool, PointSketchTool, PolylineSketchTool, RectangleSketchTool, RegularPolygonSketchTool, SelectTool, SlotSketchTool, SplineSketchTool, type ToolViewportPort } from "../cad/tool/cad-tool";
import { ToolManager } from "../cad/tool/tool-manager";
import type {
  Artifact, AxisSystem, DatumPlane, DocumentStructureNode, DocumentView, Feature, PlaneName, ReferenceGeometry, Selection, SketchGeometryRef, SketchOperation, Vec2, Vec3, VisualizationManifest,
} from "../types";

type Callbacks = {
  selectionChanged: (selection: Selection) => void;
  preselectionChanged: (selection: Selection) => void;
  sketchOperations: (operations: SketchOperation[]) => void;
  toolPromptChanged: (prompt: string) => void;
  toolUseCompleted: () => void;
  instanceMoved: (instanceId: string, translation: Vec3) => void;
  debugStateChanged?: (state: ViewportDebugState) => void;
};

type SolidContext = {
  documentId: string; geometryKey: string; occurrencePath: string; treeNodeId: string; instanceId?: string;
};

type SolidBinding = { group: THREE.Group; mesh: THREE.Mesh; artifact: Artifact; context: SolidContext };

export type ViewportDebugState = {
  input: InputState;
  activeTool: string;
  navigationProfile: NavigationProfileID;
  navigationAction: NavigationAction;
  navigation?: NavigationSnapshot;
  hudScreen?: { x: number; y: number };
};

const planeColors: Record<PlaneName, number> = { XY: CATIA_VISUAL_THEME.axisZ, XZ: CATIA_VISUAL_THEME.axisY, YZ: CATIA_VISUAL_THEME.axisX };

function localToWorld(plane: PlaneName, point: Vec2): THREE.Vector3 {
  if (plane === "XY") return new THREE.Vector3(point[0], point[1], 0);
  if (plane === "XZ") return new THREE.Vector3(point[0], 0, point[1]);
  return new THREE.Vector3(0, point[0], point[1]);
}

function worldToLocal(plane: PlaneName, point: THREE.Vector3): Vec2 {
  if (plane === "XY") return [point.x, point.y];
  if (plane === "XZ") return [point.x, point.z];
  return [point.y, point.z];
}

function sketchDiagnosticColor(status: string | undefined, construction = false): number {
  if (construction) return CATIA_VISUAL_THEME.sketchConstruction;
  switch (status) {
  case "SOLVED": return CATIA_VISUAL_THEME.sketchSolved;
  case "CONFLICTING":
  case "FAILED":
  case "INVALID_MODEL": return CATIA_VISUAL_THEME.sketchInvalid;
  case "REDUNDANT": return CATIA_VISUAL_THEME.sketchRedundant;
  default: return CATIA_VISUAL_THEME.sketchProfile;
  }
}

function rayPlane(plane: PlaneName): THREE.Plane {
  if (plane === "XY") return new THREE.Plane(new THREE.Vector3(0, 0, 1), 0);
  if (plane === "XZ") return new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
  return new THREE.Plane(new THREE.Vector3(1, 0, 0), 0);
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
  private readonly transform: TransformControls;
  private readonly navigation: NavigationController;
  private readonly navigationHUD: CatiaNavigationHUD;
  private readonly shortcuts: ShortcutManager;
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
  private selected: Selection = null;
  private preselected: Selection = null;
  private selectedOverlay?: THREE.Object3D;
  private preselectedOverlay?: THREE.Object3D;
  private view?: DocumentView;
  private sketchPlane?: PlaneName;
  private activeSketchID?: string;
  private preview?: THREE.Object3D;
  private referencePreview?: THREE.Object3D;
  private snapPreview?: THREE.Object3D;
  private commandPreview?: THREE.Object3D;
  private suppressNextSelection = false;
  private activeToolID = "select";
  private navigationProfile: NavigationProfileID = "default";
  private sketchShortcutDispose?: () => void;
  private toolShortcutDispose?: () => void;
  private readonly resizeObserver: ResizeObserver;
  private animationFrame = 0;
  private disposed = false;

  constructor(private readonly host: HTMLElement, private readonly callbacks: Callbacks, commandRegistry: CommandRegistry) {
    this.scene.background = null;
    this.camera.position.set(310, -360, 270);
    this.camera.up.set(0, 0, 1);
    this.camera.layers.enable(CAD_GEOMETRY_LAYER);
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 1.5));
    this.renderer.shadowMap.enabled = false;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.autoClear = false;
    host.appendChild(this.renderer.domElement);

    this.transform = new TransformControls(this.camera, this.renderer.domElement);
    this.transform.setMode("translate");
    this.transform.setSpace("world");
    this.scene.add(this.transform.getHelper());
    this.transform.addEventListener("dragging-changed", (event) => {
      this.navigation.setEnabled(!Boolean(event.value));
      if (event.value) this.suppressNextSelection = true;
    });
    this.transform.addEventListener("mouseUp", () => {
      this.commitTransform();
      window.setTimeout(() => { this.suppressNextSelection = false; }, 0);
    });
    this.transform.addEventListener("change", () => this.invalidate());

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
    this.shortcuts = new ShortcutManager((commandID) => commandRegistry.execute(commandID));
    this.tools = new ToolManager({ viewport: this.toolViewportPort() });
    this.tools.register(new SelectTool());
    this.tools.register(new PointSketchTool());
    this.tools.register(new LineSketchTool());
    this.tools.register(new CircleSketchTool());
    this.tools.register(new ArcSketchTool());
    this.tools.register(new PolylineSketchTool());
    this.tools.register(new SplineSketchTool());
    this.tools.register(new RectangleSketchTool());
    this.tools.register(new RegularPolygonSketchTool());
    this.tools.register(new SlotSketchTool());
    for (const kind of ["COINCIDENT","PARALLEL","FIXED","HORIZONTAL","VERTICAL","PERPENDICULAR","TANGENT","EQUAL","DISTANCE","LENGTH","RADIUS","DIAMETER","ANGLE","CONCENTRIC","POINT_ON_OBJECT","MIDPOINT"] as const)
      this.tools.register(new ConstraintSketchTool(kind));
    this.tools.activate("select");
    this.selectionController = new SelectionController(
      (x, y) => this.pick(x, y),
      (x, y) => this.preselectAt(x, y),
      () => this.preselect(null, true),
    );
    this.interaction = new InteractionRouter(this.tools, this.selectionController, this.navigation, this.shortcuts);
    this.input = new InputManager(this.renderer.domElement, this.interaction);
    this.shortcuts.pushContext("Global", [
      { key: "z", primary: true, shift: true, command: "edit.redo" },
      { key: "y", primary: true, shift: false, command: "edit.redo" },
      { key: "z", primary: true, shift: false, command: "edit.undo" },
    ]);
    this.shortcuts.pushContext("Viewport", [{ key: "f", primary: false, command: "view.fit" }]);
    this.input.subscribe(() => this.emitDebugState());
    this.tools.subscribe((toolID) => {
      this.activeToolID = toolID ?? "select";
      this.host.classList.toggle("drawing", Boolean(toolID?.startsWith("sketch.")) && Boolean(this.sketchPlane));
      this.refreshShortcutContexts();
      this.emitDebugState();
    });
    this.navigation.subscribe((action, profile, snapshot) => {
      this.navigationProfile = profile;
      this.host.classList.toggle("navigating", action !== "none" || Boolean(snapshot.catia?.hudVisible));
      this.updateNavigationHUD(snapshot);
      this.invalidate();
      this.emitDebugState();
    });

    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(host);
    this.invalidate();
  }

  render(view: DocumentView): void {
    this.clearCommandPreview();
    this.view = view;
    this.transform.detach();
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.selectionIndex.clear();
    this.solidBindings.clear();
    this.instanceGroups.clear();
    this.selected = null;
    this.preselected = null;
    if (view.document.type === "PART") this.renderPart(view);
    else this.renderProduct(view);
    this.updateSketchContextVisibility();
    this.refreshContentBounds();
    // this.frameContent();
    this.invalidate();
  }

  clear(): void {
    this.clearCommandPreview();
    this.view = undefined;
    this.transform.detach();
    this.disposeGroup(this.content);
    this.disposeGroup(this.helpers);
    this.selectable.clear();
    this.instanceGroups.clear();
    this.contentBounds.makeEmpty();
    this.selected = null;
    this.invalidate();
  }

  beginSketch(sketchID: string, plane: PlaneName): void {
    this.activeSketchID = sketchID;
    this.sketchPlane = plane;
    this.transform.detach();
    this.select({ kind: "plane", id: `datum-${plane.toLowerCase()}`, plane });
    this.navigation.setEnabled(true);
    if (plane === "XY") this.camera.position.set(0, 0, 420);
    else if (plane === "XZ") this.camera.position.set(0, -420, 0);
    else this.camera.position.set(420, 0, 0);
    this.navigation.target.set(0, 0, 0);
    if (plane === "XY") this.camera.up.set(0, 1, 0);
    else this.camera.up.set(0, 0, 1);
    this.navigation.syncCamera();
    this.buildSketchContext();
    this.updateSketchContextVisibility();
    this.callbacks.toolPromptChanged("选择：选择草图元素，或从工具栏启动创建命令");
    this.refreshShortcutContexts();
    this.invalidate();
  }

  endSketch(): void {
    this.sketchPlane = undefined;
    this.activeSketchID = undefined;
    this.host.classList.remove("drawing");
    this.tools.cancel();
    this.clearSnapPreview();
    this.navigation.setEnabled(true);
    this.disposeGroup(this.sketchContext);
    this.updateSketchContextVisibility();
    this.callbacks.toolPromptChanged("");
    this.refreshShortcutContexts();
    this.frameContent();
  }

  setActiveTool(toolID: import("../state/workbench-store").WorkbenchToolID): void {
    this.tools.activate(toolID);
  }

  setNavigationProfile(profile: NavigationProfileID): void {
    this.navigation.setProfile(profile);
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
    this.scene.add(group); this.commandPreview = group; this.invalidate();
  }

  clearCommandPreview(): void {
    if (!this.commandPreview) return;
    this.scene.remove(this.commandPreview); this.disposeRenderable(this.commandPreview);
    this.commandPreview = undefined; this.invalidate();
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
    this.selected = selection;
    this.transform.detach();
    for (const object of this.selectable.values()) this.applyHighlight(object, "default");
    for (const object of this.selectionIndex.objectsFor(selection)) this.applyHighlight(object, "selected");
    this.replaceTopologyOverlay("selected", selection);
    if (!sameSelection(this.preselected, selection)) {
      for (const object of this.selectionIndex.objectsFor(this.preselected)) this.applyHighlight(object, "hover");
      this.replaceTopologyOverlay("preselected", this.preselected);
    } else this.replaceTopologyOverlay("preselected", null);
    if (selection) {
      const object = this.selectable.get(`${selection.kind}:${selection.id}`);
      if (object) {
        this.applyHighlight(object, "selected");
        if (selection.kind === "instance" && object instanceof THREE.Group) this.transform.attach(object);
      }
    }
    if (notify) this.callbacks.selectionChanged(selection);
    this.invalidate();
  }

  preselect(selection: Selection, notify = false): void {
    if (sameSelection(this.preselected, selection)) return;
    for (const object of this.selectionIndex.objectsFor(this.preselected)) {
      if (!this.selectionIndex.objectsFor(this.selected).includes(object)) this.applyHighlight(object, "default");
    }
    this.preselected = selection;
    for (const object of this.selectionIndex.objectsFor(selection)) {
      if (!this.selectionIndex.objectsFor(this.selected).includes(object)) this.applyHighlight(object, "hover");
    }
    this.replaceTopologyOverlay("preselected", sameSelection(selection, this.selected) ? null : selection);
    if (notify) this.callbacks.preselectionChanged(selection);
    this.invalidate();
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    cancelAnimationFrame(this.animationFrame);
    this.resizeObserver.disconnect();
    this.input.dispose();
    this.sketchShortcutDispose?.();
    this.toolShortcutDispose?.();
    this.clearPreview();
    this.clearReferencePreview();
    this.clearSnapPreview();
    this.clearCommandPreview();
    this.transform.detach();
    this.transform.dispose();
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
    for (const datum of view.datumPlanes ?? []) this.addDatumPlane(datum, this.helpers, true, {
      documentId: view.document.id, geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/plane:${datum.id}`,
    });
    for (const axis of view.axisSystems ?? []) this.addAxisSystem(axis, this.helpers, {
      documentId: view.document.id, geometryKey: view.artifact?.geometryKey ?? "", occurrencePath: "",
      treeNodeId: `${rootPath}/origin/axis:${axis.id}`,
    });
    if (view.artifact) this.addVisualPrimitives(view.artifact.visualization, this.helpers, {
      documentId: view.document.id, geometryKey: view.artifact.geometryKey, occurrencePath: "", treeNodeId: `${rootPath}/body`,
    });
    for (const feature of view.part?.features ?? []) {
      if (feature.type.toUpperCase().includes("SKETCH")) this.addSketch(feature, false);
    }
    if (view.artifact && view.artifact.mesh.triangles.length > 0) {
      const solid = this.makeSolid(view.artifact, CATIA_VISUAL_THEME.surface, {
        documentId: view.document.id, geometryKey: view.artifact.geometryKey, occurrencePath: "", treeNodeId: `${rootPath}/body`,
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
        resolvedGroup.position.set(
          resolved.translation[0] - instance.translation[0],
          resolved.translation[1] - instance.translation[1],
          resolved.translation[2] - instance.translation[2],
        );
        if (artifact.mesh.triangles.length > 0) {
          const context: SolidContext = {
            documentId: resolved.documentId, geometryKey: artifact.geometryKey,
            occurrencePath: resolved.occurrencePath, treeNodeId: resolved.bodyTreeNodeId, instanceId: instance.id
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
        this.addVisualPrimitives(artifact.visualization, resolvedGroup, visualContext);
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
  }

  private addDatumPlane(datum: DatumPlane, parent: THREE.Group, selectable: boolean, context?: SolidContext): void {
    const { id, plane } = datum;
    const geometry = new THREE.PlaneGeometry(datum.size || 180, datum.size || 180);
    if (plane === "XZ") geometry.rotateX(Math.PI / 2);
    if (plane === "YZ") geometry.rotateY(Math.PI / 2);
    const material = new THREE.MeshBasicMaterial({
      color: planeColors[plane], transparent: true, opacity: 0.075,
      side: THREE.DoubleSide, depthWrite: false,
    });
    const mesh = new THREE.Mesh(geometry, material);
    mesh.position.fromArray(datum.origin);
    const selection = {
      kind: "plane" as const, id: `${context?.occurrencePath || "root"}:${id}`, plane,
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

  private addAxisSystem(axis: AxisSystem, parent: THREE.Group, context?: SolidContext): void {
    const length = 38;
    const origin = new THREE.Vector3().fromArray(axis.origin);
    const system = new THREE.Group();
    const systemSelection = {
      kind: "axis-system" as const, id: `${context?.occurrencePath || "root"}:${axis.id}`,
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

  private addVisualPrimitives(visualization: VisualizationManifest | undefined, parent: THREE.Group, context: SolidContext): void {
    if (!visualization || visualization.schemaVersion !== 1) return;
    for (const primitive of visualization.primitives ?? []) {
      if (primitive.positions.length === 0) continue;
      const construction = primitive.role === "CONSTRUCTION";
      const color = primitive.semantic === "SKETCH_CONSTRAINT" ? CATIA_VISUAL_THEME.constraint
        : sketchDiagnosticColor(primitive.status, construction);
      const geometry = new THREE.BufferGeometry().setFromPoints(
        primitive.positions.map((position) => new THREE.Vector3().fromArray(position)),
      );
      let object: THREE.Object3D;
      const type = visualType(primitive);
      if (primitive.kind === "POINTS") {
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
      object.renderOrder = primitive.kind === "POINTS" ? 22 : 20;
      const featureTreeNode = this.featureTreeNode(context, primitive.featureId);
      const selection = visualSelection(primitive, {
        treeNodeId: featureTreeNode ? primitive.semantic === "SKETCH_CONSTRAINT"
          ? `${featureTreeNode}/constraints/constraint:${primitive.id}` : `${featureTreeNode}/geometry/entity:${primitive.id}` : undefined,
        documentId: context.documentId,
        occurrencePath: context.occurrencePath, geometryKey: context.geometryKey, instanceId: context.instanceId,
      });
      object.userData = { ...selection, sketchFeatureID: primitive.featureId };
      parent.add(object);
      if (primitive.selectable) {
        this.selectable.set(`visual:${selection.id}`, object);
        this.selectionIndex.register(selection, object, selection.treeNodeId);
        this.selectionIndex.registerPick(object, () => selection, type === "POINT" ? 75 : type === "CURVE" ? 70 : 30);
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
      const line = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane!, first), localToWorld(this.sketchPlane!, second)]),
        new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.9, depthTest: false }),
      );
      line.renderOrder = 16;
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
    this.environment.visible = !editing;
    this.content.visible = !editing;
    this.sketchContext.visible = editing;
    for (const child of this.helpers.children) {
      if (child.userData.sketchEditOverlay) {
        const active = child.userData.sketchFeatureID === this.activeSketchID;
        child.visible = editing && active;
        for (const sketchChild of child.children) {
          if (sketchChild.userData.sketchEntityOverlay) sketchChild.visible = editing && active;
        }
      } else child.visible = !editing || child.userData.sketchFeatureID === this.activeSketchID;
    }
  }

  private addSketch(feature: Feature, _includeEntities = true): void {
    const plane = feature.sketch?.support.plane ?? feature.plane ?? "XY";
    const group = new THREE.Group();
    group.userData.sketchFeatureID = feature.id;
    const entities = new Map((feature.sketch?.entities ?? []).map((entity) => [entity.id, entity]));
    const documentId = this.view?.document.id ?? "";
    const featureTreeNode = this.featureTreeNode({ documentId, geometryKey: this.view?.artifact?.geometryKey ?? "",
      occurrencePath: "", treeNodeId: `document:${documentId}/body` }, feature.id)
      ?? `document:${documentId}/body/sketch:${feature.id}`;
    const referencedPoint = (reference: SketchGeometryRef): Vec2 | undefined => {
      if (reference.target === "SKETCH_ORIGIN") return [0, 0];
      const entity = reference.entityId ? entities.get(reference.entityId) : undefined;
      if (!entity || !["POINT","START","END","CENTER"].includes(reference.subElement)) return undefined;
      return sketchEntityPoint(entity,reference.subElement as "POINT"|"START"|"END"|"CENTER");
    };
    const sketchSelection = { kind: "sketch" as const, id: feature.id, documentId, treeNodeId: featureTreeNode };
    for (const entity of feature.sketch?.entities ?? []) {
      const type = entity.kind === "POINT" ? "POINT" as const : "CURVE" as const;
      const entitySelection = { kind: "visual" as const, id: `root:${feature.id}:${entity.id}`, visualType: type,
        featureId: feature.id, entityId: entity.id, role: entity.role, documentId,
        treeNodeId: `${featureTreeNode}/geometry/entity:${entity.id}` };
      let object: THREE.Object3D | undefined;
      if (entity.kind === "POINT" && entity.point) {
        object = new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(plane, [entity.point.x, entity.point.y])]),
          this.materials.point(sketchDiagnosticColor(feature.sketch?.solve.status, entity.role === "CONSTRUCTION"), 9, false));
        object.renderOrder = 22;
      } else {
        const sampled=sampleSketchEntity(entity);
        if(sampled.length<2)continue;
        const positions=sampled.map((point)=>localToWorld(plane,point));
        object = new THREE.Line(new THREE.BufferGeometry().setFromPoints(positions), new THREE.LineBasicMaterial({ color: sketchDiagnosticColor(feature.sketch?.solve.status, entity.role === "CONSTRUCTION"), depthTest: false }));
        object.renderOrder = 20;
        const markers=entity.kind==="CIRCLE"&&entity.center?[localToWorld(plane,[entity.center.x,entity.center.y])]:[positions[0],positions.at(-1)!];
        const endpointMarkers = new THREE.Points(new THREE.BufferGeometry().setFromPoints(markers),
          this.materials.point(CATIA_VISUAL_THEME.vertex, 8, false));
        endpointMarkers.userData = { sketchEntityOverlay: true }; endpointMarkers.renderOrder = 21; group.add(endpointMarkers);
      }
      if (!object) continue;
      object.userData = { ...entitySelection, sketchEntityOverlay: true }; group.add(object);
      this.selectable.set(`visual:${entitySelection.id}`, object);
      this.selectionIndex.register(entitySelection, object);
      this.selectionIndex.registerPick(object, () => entitySelection, type === "POINT" ? 75 : 70);
    }
    for (const constraint of feature.sketch?.constraints ?? []) {
      const constraintSelection = { kind: "sketch-constraint" as const,
        id: `root:${feature.id}:constraint:${constraint.id}`, featureId: feature.id,
        constraintId: constraint.id, constraintType: constraint.kind, documentId,
        treeNodeId: `${featureTreeNode}/constraints/constraint:${constraint.id}` };
      const constraintGroup = new THREE.Group();
      if (constraint.kind === "COINCIDENT") {
        const point = referencedPoint(constraint.references[0]);
        if (point) constraintGroup.add(new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(plane, point)]),
          this.materials.point(CATIA_VISUAL_THEME.constraint, 8, false)));
      }
      if (constraint.kind === "PARALLEL") {
        for (const reference of constraint.references.filter((item) => item.target === "ENTITY")) {
          const entity = reference.entityId ? entities.get(reference.entityId) : undefined;
          if (entity?.kind !== "LINE" || !entity.start || !entity.end) continue;
          const dx=entity.end.x-entity.start.x, dy=entity.end.y-entity.start.y, length=Math.hypot(dx,dy);
          if (length===0) continue;
          const middle:Vec2=[(entity.start.x+entity.end.x)/2,(entity.start.y+entity.end.y)/2];
          const perpendicular:Vec2=[-dy/length,dx/length];
          const ticks: THREE.Vector3[] = [];
          for (const offset of [-2,2]) {
            const center:Vec2=[middle[0]+dx/length*offset,middle[1]+dy/length*offset];
            ticks.push(localToWorld(plane,[center[0]-perpendicular[0]*2,center[1]-perpendicular[1]*2]),localToWorld(plane,[center[0]+perpendicular[0]*2,center[1]+perpendicular[1]*2]));
          }
          constraintGroup.add(new THREE.LineSegments(new THREE.BufferGeometry().setFromPoints(ticks),
            new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.constraint, depthTest: false })));
        }
      }
      if (constraint.kind === "FIXED_POINT") {
        const point = constraint.fixedPoint ? [constraint.fixedPoint.x, constraint.fixedPoint.y] as Vec2
          : referencedPoint(constraint.references[0]);
        if (point) constraintGroup.add(new THREE.Points(new THREE.BufferGeometry().setFromPoints([localToWorld(plane, point)]),
          this.materials.point(CATIA_VISUAL_THEME.constraint, 11, false)));
      }
      if (constraintGroup.children.length === 0) continue;
      constraintGroup.userData = constraintSelection;
      constraintGroup.traverse((child) => { child.userData = constraintSelection; child.renderOrder = 24;
        if (child !== constraintGroup) this.selectionIndex.registerPick(child, () => constraintSelection, 80); });
      group.add(constraintGroup);
      this.selectable.set(`sketch-constraint:${constraintSelection.id}`, constraintGroup);
      this.selectionIndex.register(constraintSelection, constraintGroup);
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

  private pick(x: number, y: number): void {
    if (this.suppressNextSelection) { this.suppressNextSelection = false; return; }
    if (this.transform.dragging) return;
    this.select(this.hitTest(x, y));
  }

  private preselectAt(x: number, y: number): void {
    if (this.transform.dragging || this.navigation.activeAction !== "none") return;
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
    return this.selectionIndex.pick(this.raycaster);
  }

  private replaceTopologyOverlay(layer: "selected" | "preselected", selection: Selection): void {
    const property = layer === "selected" ? "selectedOverlay" : "preselectedOverlay";
    const previous = this[property];
    if (previous) {
      previous.parent?.remove(previous);
      this.disposeRenderable(previous);
      this[property] = undefined;
    }
    if (!selection || (selection.kind !== "face" && selection.kind !== "edge" && selection.kind !== "vertex")) return;
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
          polygonOffset: true, polygonOffsetFactor: -2, polygonOffsetUnits: -2
        }));
      }
    } else if (selection.kind === "edge") {
      const edge = (binding.artifact.mesh.edges ?? []).find((item) => item.localId === selection.topologyId);
      if (edge) overlay = new THREE.Line(new THREE.BufferGeometry().setFromPoints(edge.points.map((point) => new THREE.Vector3().fromArray(point))),
        new THREE.LineBasicMaterial({ color, depthTest: true }));
    } else {
      const vertex = (binding.artifact.mesh.topologyVertices ?? []).find((item) => item.localId === selection.topologyId);
      if (vertex) {
        const geometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3().fromArray(vertex.point)]);
        overlay = new THREE.Points(geometry, this.materials.point(color, layer === "selected" ? 11 : 9, false));
      }
    }
    if (!overlay) return;
    overlay.renderOrder = layer === "selected" ? 102 : 101;
    binding.group.add(overlay); this[property] = overlay;
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
    const snap = resolveSketchSnap(raw, active?.entities ?? [], pixelsPerUnit);
    if (snap) this.showSnapPreview(snap); else this.clearSnapPreview();
    return snap?.point ?? raw;
  }

  private showSnapPreview(snap: SketchSnapResult): void {
    this.clearSnapPreview();
    if (!this.sketchPlane) return;
    const center = localToWorld(this.sketchPlane, snap.point);
    const group = new THREE.Group();
    const marker = new THREE.Points(new THREE.BufferGeometry().setFromPoints([center]),
      this.materials.point(CATIA_VISUAL_THEME.snap, 15, false));
    marker.renderOrder = 35;
    group.userData.snapKind = snap.kind;
    group.add(marker); this.scene.add(group); this.snapPreview = group; this.invalidate();
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

  private drawPreview(points2: Vec2[], closed: boolean, plane: PlaneName): void {
    this.clearPreview();
    const localPoints = closed && points2.length > 0 ? [...points2, points2[0]] : points2;
    const points = localPoints.map((point) => localToWorld(plane, point));
    this.preview = new THREE.Line(
      new THREE.BufferGeometry().setFromPoints(points),
      new THREE.LineBasicMaterial({ color: CATIA_VISUAL_THEME.preview, depthTest: false }),
    );
    this.scene.add(this.preview);
    this.invalidate();
  }

  private drawPointPreview(point: Vec2, plane: PlaneName): void {
    this.clearPreview();
    this.preview = new THREE.Points(
      new THREE.BufferGeometry().setFromPoints([localToWorld(plane, point)]),
      this.materials.point(CATIA_VISUAL_THEME.preview, 11, false),
    );
    this.preview.renderOrder = 28;
    this.scene.add(this.preview);
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
      return new THREE.Line(
        new THREE.BufferGeometry().setFromPoints(points.map((point) => localToWorld(this.sketchPlane!, point))),
        new THREE.LineBasicMaterial({ color, depthTest: false }),
      );
    }
    if (!this.view || !reference.entityId) return undefined;
    const entity = this.view.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch?.entities
      .find((candidate) => candidate.id === reference.entityId);
    if (!entity) return undefined;
    if (entity.kind === "POINT" && entity.point && reference.subElement === "POINT") {
      return new THREE.Points(
        new THREE.BufferGeometry().setFromPoints([localToWorld(this.sketchPlane, [entity.point.x, entity.point.y])]),
        this.materials.point(color, 15, false),
      );
    }
    if (entity.kind !== "LINE" || !entity.start || !entity.end) return undefined;
    const start = localToWorld(this.sketchPlane, [entity.start.x, entity.start.y]);
    const end = localToWorld(this.sketchPlane, [entity.end.x, entity.end.y]);
    if (reference.subElement === "DIRECTION") {
      return new THREE.Line(new THREE.BufferGeometry().setFromPoints([start, end]),
        new THREE.LineBasicMaterial({ color, depthTest: false }));
    }
    return new THREE.Points(
      new THREE.BufferGeometry().setFromPoints([reference.subElement === "START" ? start : end]),
      this.materials.point(color, 15, false),
    );
  }

  private showReferencePreview(reference: SketchGeometryRef, retained?: SketchGeometryRef): void {
    this.clearReferencePreview();
    const group = new THREE.Group();
    const same = retained && retained.target === reference.target && retained.entityId === reference.entityId &&
      retained.subElement === reference.subElement;
    if (retained && !same) {
      const first = this.makeReferencePreview(retained, CATIA_VISUAL_THEME.selected); if (first) group.add(first);
    }
    const candidate = this.makeReferencePreview(reference, CATIA_VISUAL_THEME.snap); if (candidate) group.add(candidate);
    if (group.children.length === 0) return;
    this.referencePreview = group;
    this.referencePreview.renderOrder = 30;
    this.referencePreview.traverse((child) => { child.renderOrder = 30; });
    this.scene.add(this.referencePreview);
    this.invalidate();
  }

  private toolViewportPort(): ToolViewportPort {
    return {
      sketchPoint: (x, y) => this.sketchPoint(x, y),
      showPolylinePreview: (points, closed = false) => {
        if (this.sketchPlane) this.drawPreview(points, closed, this.sketchPlane);
      },
      showPointPreview: (point) => {
        if (this.sketchPlane) this.drawPointPreview(point, this.sketchPlane);
      },
      clearToolPreview: () => { this.clearPreview(); this.clearSnapPreview(); },
      commitSketchOperations: (operations) => { this.clearSnapPreview(); this.callbacks.sketchOperations(operations); },
      hasActiveSketch: () => Boolean(this.sketchPlane && this.activeSketchID),
      sketchReferenceAt: (x, y, kind) => this.sketchReferenceAt(x, y, kind),
      showReferencePreview: (reference, retained) => this.showReferencePreview(reference, retained),
      clearReferencePreview: () => this.clearReferencePreview(),
      setToolPrompt: (prompt) => this.callbacks.toolPromptChanged(prompt),
      finishToolUse: () => this.callbacks.toolUseCompleted(),
    };
  }

  private sketchReferenceAt(x: number, y: number, kind: SketchReferencePickKind) {
    if (!this.sketchPlane || !this.view) return null;
    const width = Math.max(this.renderer.domElement.clientWidth, 1);
    const height = Math.max(this.renderer.domElement.clientHeight, 1);
    const screen = (point: Vec2) => {
      const projected = localToWorld(this.sketchPlane!, point).project(this.camera);
      return { x: (projected.x + 1) * width / 2, y: (1 - projected.y) * height / 2 };
    };
    const entities = this.view.part?.features.find((feature) => feature.id === this.activeSketchID)?.sketch?.entities ?? [];
    return resolveSketchReference({ x, y }, entities, screen, kind);
  }

  private refreshShortcutContexts(): void {
    this.sketchShortcutDispose?.();
    this.toolShortcutDispose?.();
    this.sketchShortcutDispose = this.sketchPlane ? this.shortcuts.pushContext("Sketch", [
      { key: "p", primary: false, command: "sketch.point" },
      { key: "l", primary: false, command: "sketch.line" },
      { key: "r", primary: false, command: "sketch.rectangle" },
    ]) : undefined;
    this.toolShortcutDispose = this.sketchPlane && this.activeToolID !== "select"
      ? this.shortcuts.pushContext("SketchTool", [{ key: "Escape", primary: false, command: "tool.select" }]) : undefined;
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
    const object = this.transform.object;
    if (!object?.userData.id) return;
    this.callbacks.instanceMoved(object.userData.id as string,
      [object.position.x, object.position.y, object.position.z]);
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
      materials.forEach((material) => material?.dispose());
    });
  }

  private invalidate(): void {
    if (this.disposed || this.animationFrame) return;
    this.animationFrame = requestAnimationFrame(() => {
      this.animationFrame = 0;
      if (!this.disposed) {
        this.updateNavigationHUD();
        this.renderer.clear(true, true, true);
        this.background.render(this.renderer);
        this.renderer.clearDepth();
        this.renderer.render(this.scene, this.camera);
        this.navigationHUD.render(this.renderer);
      }
    });
  }
}
