import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type { CaptureSettings } from "../cad/interaction/capture-settings";
import { InputDebugOverlay, type InputDebugSnapshot } from "../cad/overlay/input-debug-overlay";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";
import type { WorkbenchToolID } from "../state/workbench-store";
import { randomUUID } from "../utils/random-uuid";
import type { Artifact, AssemblyGeometryRef, DocumentView, Selection, SelectionItem, SketchGeometryRef, SketchOperation, SketchPlane, Vec2, Vec3 } from "../types";
import type { AssemblyConstraintToolKind } from "../cad/tool/cad-tool";
import { CadViewportEngine } from "./cad-viewport-engine";
import { formatSketchDimensionValue, normalizeSketchDimensionValue } from "../cad/sketch/sketch-input-policy";

export type CadViewportHandle = {
  fit: () => void;
  setStandardView: (view: "TOP" | "FRONT" | "RIGHT" | "ISO") => void;
  previewArtifact: (artifact: Artifact) => void;
  clearCommandPreview: () => void;
  editDimension: (selection: Extract<SelectionItem, { kind: "sketch-constraint" }>) => void;
};

type Props = {
  view: DocumentView;
  editingView?: DocumentView;
  activeInstancePath?: string;
  activeInstanceTranslation?: Vec3;
  activeInstanceRotation?: [number, number, number, number];
  activeBodyTreeNodeId?: string;
  selections: SelectionItem[];
  preselection: Selection;
  sketchPlane?: SketchPlane;
  activeSketchID?: string;
  activeToolID: WorkbenchToolID;
  navigationProfile: NavigationProfileID;
  captureSettings: CaptureSettings;
  hiddenTreeKeys: string[];
  onSelectionsChange: (selections: SelectionItem[]) => void;
  onPreselectionChange: (selection: Selection) => void;
  onSketchOperations: (featureID: string, operations: SketchOperation[]) => void;
  onToolUseComplete: () => void;
  onActiveToolChange: (toolID: WorkbenchToolID) => void;
  onInstanceMoved: (instanceID: string, translation: Vec3) => void;
  onAssemblyConstraint: (kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]) => void;
};

export const CadViewport = forwardRef<CadViewportHandle, Props>(function CadViewport(props, ref) {
  const host = useRef<HTMLDivElement>(null);
  const engine = useRef<CadViewportEngine | undefined>(undefined);
  const callbacks = useRef(props);
  const [debug, setDebug] = useState<InputDebugSnapshot>();
  const [dimensionEditor, setDimensionEditor] = useState<
    { mode: "edit"; featureId: string; constraintId: string; value: number; unit: "mm" | "deg"; x: number; y: number }
    | { mode: "create"; featureId: string; kind: "DISTANCE"|"LENGTH"|"RADIUS"|"DIAMETER"|"ANGLE";
      references: SketchGeometryRef[]; labelPosition: Vec2; value: number; unit: "mm"|"deg"; x: number; y: number }>();
  callbacks.current = props;

  useEffect(() => {
    if (!host.current) return;
    const instance = new CadViewportEngine(host.current, {
      selectionsChanged: (selections) => callbacks.current.onSelectionsChange(selections),
      preselectionChanged: (selection) => callbacks.current.onPreselectionChange(selection),
      sketchOperations: (featureID, operations) => callbacks.current.onSketchOperations(featureID, operations),
      toolUseCompleted: () => callbacks.current.onToolUseComplete(),
      dimensionEditRequested: (request) => setDimensionEditor(request),
      dimensionCreateRequested: (request) => setDimensionEditor(request),
      activeToolChanged: (toolID) => callbacks.current.onActiveToolChange(toolID),
      toolPromptChanged: () => {},
      instanceMoved: (instanceID, translation) => callbacks.current.onInstanceMoved(instanceID, translation),
      assemblyConstraintRequested: (kind, references) => callbacks.current.onAssemblyConstraint(kind, references),
      debugStateChanged: import.meta.env.DEV && import.meta.env.VITE_INPUT_DEBUG === "true" ? setDebug : undefined,
    });
    engine.current = instance;
    instance.render(callbacks.current.view, callbacks.current.editingView ? {
      view: callbacks.current.editingView, occurrencePath: callbacks.current.activeInstancePath,
      translation: callbacks.current.activeInstanceTranslation, bodyTreeNodeId: callbacks.current.activeBodyTreeNodeId,
      rotation: callbacks.current.activeInstanceRotation,
    } : undefined);
    instance.selectMany(callbacks.current.selections, false);
    instance.preselect(callbacks.current.preselection, false);
    if (callbacks.current.sketchPlane && callbacks.current.activeSketchID) {
      instance.beginSketch(callbacks.current.activeSketchID, callbacks.current.sketchPlane);
    }
    instance.setActiveTool(callbacks.current.activeToolID);
    instance.setNavigationProfile(callbacks.current.navigationProfile);
    instance.setCaptureSettings(callbacks.current.captureSettings);
    instance.setHiddenTreeKeys(callbacks.current.hiddenTreeKeys);
    return () => { instance.dispose(); engine.current = undefined; };
  }, [CadViewportEngine]);

  useEffect(() => {
    const instance = engine.current; if (!instance) return;
    instance.render(props.view, props.editingView ? { view: props.editingView, occurrencePath: props.activeInstancePath,
      translation: props.activeInstanceTranslation, rotation: props.activeInstanceRotation,
      bodyTreeNodeId: props.activeBodyTreeNodeId } : undefined);
    instance.selectMany(callbacks.current.selections, false);
    instance.preselect(callbacks.current.preselection, false);
  }, [props.view, props.editingView, props.activeInstancePath, props.activeInstanceTranslation, props.activeInstanceRotation, props.activeBodyTreeNodeId]);
  useEffect(() => { engine.current?.selectMany(props.selections, false); }, [props.selections]);
  useEffect(() => { engine.current?.preselect(props.preselection, false); }, [props.preselection]);
  useEffect(() => {
    if (props.sketchPlane && props.activeSketchID) engine.current?.beginSketch(props.activeSketchID, props.sketchPlane);
    else engine.current?.endSketch();
  }, [props.sketchPlane, props.activeSketchID]);
  useEffect(() => { engine.current?.setActiveTool(props.activeToolID); }, [props.activeToolID]);
  useEffect(() => { engine.current?.setNavigationProfile(props.navigationProfile); }, [props.navigationProfile]);
  useEffect(() => { engine.current?.setCaptureSettings(props.captureSettings); }, [props.captureSettings]);
  useEffect(() => { engine.current?.setHiddenTreeKeys(props.hiddenTreeKeys); }, [props.hiddenTreeKeys]);

  useImperativeHandle(ref, () => ({
    fit: () => engine.current?.fit(),
    setStandardView: (view) => engine.current?.setStandardView(view),
    previewArtifact: (artifact) => engine.current?.previewArtifact(artifact),
    clearCommandPreview: () => engine.current?.clearCommandPreview(),
    editDimension: (selection) => engine.current?.requestDimensionEdit(selection),
  }), []);

  return <><div ref={host} className="cad-viewport-canvas" />
    {dimensionEditor && <input key={`${dimensionEditor.mode}:${dimensionEditor.mode === "edit" ? dimensionEditor.constraintId : dimensionEditor.kind}`} className="sketch-dimension-editor" autoFocus
      defaultValue={formatSketchDimensionValue(dimensionEditor.value, dimensionEditor.unit)}
      inputMode="decimal" step={dimensionEditor.unit === "deg" ? 0.1 : 0.01} aria-label={`编辑尺寸 (${dimensionEditor.unit})`}
      style={{ left: dimensionEditor.x, top: dimensionEditor.y }}
      onBlur={() => setDimensionEditor(undefined)}
      onKeyDown={(event) => {
        if (event.key === "Escape") { setDimensionEditor(undefined); return; }
        if (event.key !== "Enter") return;
        const value = normalizeSketchDimensionValue(Number(event.currentTarget.value), dimensionEditor.unit);
        if (Number.isFinite(value) && value > 0) callbacks.current.onSketchOperations(dimensionEditor.featureId,
          dimensionEditor.mode === "edit"
            ? [{ type: "UPDATE_CONSTRAINT_VALUE", constraintId: dimensionEditor.constraintId, value }]
            : [{ type: "ADD_CONSTRAINT", constraint: { id: randomUUID(), kind: dimensionEditor.kind,
              references: dimensionEditor.references, value, unit: dimensionEditor.unit,
              labelPosition: { x: dimensionEditor.labelPosition[0], y: dimensionEditor.labelPosition[1] } } }]);
        setDimensionEditor(undefined);
      }} />}
    {debug && <InputDebugOverlay snapshot={debug} />}</>;
});
