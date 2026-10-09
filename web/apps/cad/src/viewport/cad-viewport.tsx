import { featureInputDisplay } from "./feature-input-display";
import type { DatumPreview } from "../cad/rendering/datum-reference";
import type { FeatureSelectionSession } from "../cad/interaction/feature-selection";
import type { SketchCommandState, SketchCommandAction, SketchCommitIntent, SketchCommitResult, SketchCommitReceipt } from "../cad/tool/sketch-command-session";
import { SketchInlineParameterInput } from "../cad/sketch/sketch-inline-parameter-input";
import { SketchCommandPanel } from "../cad/sketch/sketch-command-panel";
import type { GridVisibility, ReferenceVisibility, SolidDisplaySettings } from "../cad/rendering/display-settings";
import type { InstancePatternPreview } from "../features/workbench/instance-pattern";
import type { NormalViewPlane } from "../cad/navigation/normal-view";
import type { FeaturePreviewOperation } from "../cad/rendering/feature-preview";
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type { CaptureSettings } from "../cad/interaction/capture-settings";
import { InputDebugOverlay, type InputDebugSnapshot } from "../cad/overlay/input-debug-overlay";
import type { NavigationProfileID } from "../cad/navigation/navigation-profile";
import type { WorkbenchToolID } from "../state/workbench-store";
import type { TreeVisibilityOverrides } from "../cad/interaction/tree-visibility";
import { SketchDimensionEditor, type DimensionRequest } from "../cad/sketch/sketch-dimension-editor";
import type { Artifact, AssemblyGeometryRef, DocumentView, Selection, SelectionItem, SketchGeometryRef, SketchOperation, SketchPlane, Vec2, Vec3 } from "../types";
import type { AssemblyConstraintToolKind } from "../cad/tool/cad-tool";
import { CadViewportEngine } from "./cad-viewport-engine";
import type { DocumentSessions } from "../cad/document/document-session";

import type { AssemblyInteractionBegin, AssemblyInteractionSession, AssemblyInteractionUpdate, AssemblyInteractionFrame, AssemblyInteractionCommit, AssemblyInteractionState } from "../cad/assembly/assembly-interaction";

export type CadViewportHandle = {
  sketchCommandAction:(action:SketchCommandAction)=>void;
  retrySketchReceipt:()=>Promise<void>;
  showRemainingMotion: (motion?:import("../cad/assembly/motion-presentation").MotionPresentation)=>void;
  cancelAssemblyInteraction:()=>void;
  settleAssemblyInteraction:()=>Promise<void>;
  retryAssemblyMoveCommit:()=>void;
  setAssemblyMoveDirection:(instanceId:string,direction:Vec3,kind:"line"|"plane")=>boolean;
  captureToolSelections: (selections: readonly SelectionItem[]) => boolean;
  fit: () => void;
  normalToSketch: () => void;
  normalToPlane: (selection:SelectionItem,plane?:NormalViewPlane) => boolean;
  setStandardView: (view: "TOP" | "FRONT" | "RIGHT" | "ISO") => void;
  previewArtifact: (artifact: Artifact, operation?: FeaturePreviewOperation, sketchFeatureId?:string) => void;
  clearCommandPreview: (restore?: boolean) => void;
  editDimension: (selection: Extract<SelectionItem, { kind: "sketch-constraint" }>) => void;
  measureAssemblyConstraint: (kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]) => number;
  previewAssemblyPoses: (poses: Array<{instanceId:string;translation:Vec3;rotation:[number,number,number,number]}>) => void;
  previewInsertPattern: (input?: InstancePatternPreview) => void;
  assemblyAngleReferenceDirection: (references: AssemblyGeometryRef[]) => Vec3 | undefined;
  focusAssemblyReference: (reference: AssemblyGeometryRef,ownerOccurrence?:string) => boolean;
  beginExternalReconnect: (externalID:string) => void;
};

type Props = {
  motionDisplay?:import("../cad/assembly/motion-study").MotionPlayback;
  documentSessions?: DocumentSessions;
  datumPreview?:DatumPreview;
  featureSelection?:FeatureSelectionSession;
  featureInputArtifacts?:Artifact[];
  preferredLengthUnit?: "mm"|"cm"|"m"|"in";
  onSketchCommandStateChange?:(state:SketchCommandState|undefined)=>void;
  onSketchReceiptChange?:(receipt:SketchCommitReceipt|undefined)=>void;
  view: DocumentView;
  editingView?: DocumentView;
  liveDefinitionProjection?: boolean;
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
  catiaRotationSphereVisible: boolean;
  captureSettings: CaptureSettings;
  gridVisibility: GridVisibility;
  referenceVisibility: ReferenceVisibility;
  solidDisplay: SolidDisplaySettings;
  treeVisibilityOverrides: TreeVisibilityOverrides;
  onSelectionsChange: (selections: SelectionItem[]) => void;
  onPreselectionChange: (selection: Selection) => void;
  onSketchOperations: (featureID: string, operations: SketchOperation[],intent?:SketchCommitIntent) => Promise<SketchCommitResult>;
  onDimensionOperations: (featureID:string,operations:SketchOperation[],intent?:SketchCommitIntent)=>Promise<unknown>;
  onSketchReceiptCheck?:(receipt:SketchCommitReceipt)=>Promise<SketchCommitResult>;
  onSketchPreview?:(featureId:string,operations:SketchOperation[],signal?:AbortSignal)=>Promise<import("../types").CommandPreview>;
  onToolUseComplete: () => void;
  onActiveToolChange: (toolID: WorkbenchToolID) => void;
  onInstanceMoved: (documentId:string,candidate:AssemblyInteractionCommit)=>Promise<void>;
  onAssemblyInteractionBegin:(documentId:string,input:AssemblyInteractionBegin,signal:AbortSignal)=>Promise<AssemblyInteractionSession>;
  onAssemblyInteractionUpdate:(documentId:string,input:AssemblyInteractionUpdate,signal:AbortSignal)=>Promise<AssemblyInteractionFrame>;
  onAssemblyInteractionCancel:(documentId:string,sessionId:string)=>Promise<unknown>;
  onAssemblyInteractionState?:(state:AssemblyInteractionState,reason?:string)=>void;
  onOperationFailed?:(error:unknown)=>void;
  onInspectAssemblySupports?:(documentId:string,references:AssemblyGeometryRef[],signal:AbortSignal)=>Promise<import("../cad/interaction/manipulator-snap").SupportInspection>;
  onAssemblyConstraint: (kind: AssemblyConstraintToolKind, references: AssemblyGeometryRef[]) => void;
};

export const CadViewport = forwardRef<CadViewportHandle, Props>(function CadViewport(props, ref) {
  const host = useRef<HTMLDivElement>(null);
  const engine = useRef<CadViewportEngine | undefined>(undefined);
  const patternPreview = useRef<InstancePatternPreview | undefined>(undefined);
  const callbacks = useRef(props);
  const [toolPrompt,setToolPrompt]=useState("");
  const [sketchCommand,setSketchCommand]=useState<SketchCommandState>();
  const [debug, setDebug] = useState<InputDebugSnapshot>();
  const [dimensionEditor, setDimensionEditor] = useState<DimensionRequest & {ownerDocumentID:string;ownerRevisionID:string;epoch:number}>();
  const dimensionEpoch=useRef(0);
  const openDimension=(request:DimensionRequest)=>{
    const owner=(callbacks.current.editingView??callbacks.current.view).document;
    engine.current?.setSketchDialogOpen(true);
    setDimensionEditor({...request,ownerDocumentID:owner.id,ownerRevisionID:owner.versionId,epoch:++dimensionEpoch.current});
  };
  callbacks.current = props;
  const dimensionOwner=(props.editingView??props.view).document;
  useEffect(()=>{setDimensionEditor(undefined);engine.current?.setSketchDialogOpen(false);},[dimensionOwner.id,dimensionOwner.versionId,props.activeSketchID,props.activeInstancePath]);
  useEffect(()=>engine.current?.setSketchDialogOpen(Boolean(dimensionEditor)),[dimensionEditor]);
  useEffect(()=>{setSketchCommand(undefined);callbacks.current.onSketchCommandStateChange?.(undefined);},[dimensionOwner.id,props.activeSketchID,props.activeInstancePath]);


  useEffect(() => {
    if (!host.current) return;
    const instance = new CadViewportEngine(host.current, {
      selectionsChanged: (selections) => callbacks.current.onSelectionsChange(selections),
      preselectionChanged: (selection) => callbacks.current.onPreselectionChange(selection),
      sketchOperations: (featureID, operations,intent) => callbacks.current.onSketchOperations(featureID, operations,intent),
      sketchCommandChanged:state=>{setSketchCommand(state);callbacks.current.onSketchCommandStateChange?.(state);},
      sketchReceiptChanged:receipt=>callbacks.current.onSketchReceiptChange?.(receipt),
      sketchPreview:(featureId,operations,signal)=>callbacks.current.onSketchPreview?.(featureId,operations,signal)??Promise.reject(new Error("草图权威预览不可用")),
      sketchReceiptCheck:receipt=>callbacks.current.onSketchReceiptCheck?.(receipt)??Promise.reject(new Error("原请求查询不可用")),
      toolUseCompleted: () => callbacks.current.onToolUseComplete(),
      dimensionEditRequested: (request) => openDimension(request),
      dimensionCreateRequested: (request) => openDimension(request),
      activeToolChanged: (toolID) => callbacks.current.onActiveToolChange(toolID),
      toolPromptChanged: setToolPrompt,
      instanceMoved:(documentId,candidate)=>callbacks.current.onInstanceMoved(documentId,candidate),
      assemblyInteractionBegin:(documentId,input,signal)=>callbacks.current.onAssemblyInteractionBegin(documentId,input,signal),
      assemblyInteractionUpdate:(documentId,input,signal)=>callbacks.current.onAssemblyInteractionUpdate(documentId,input,signal),
      assemblyInteractionCancel:(documentId,sessionId)=>callbacks.current.onAssemblyInteractionCancel(documentId,sessionId),
      assemblyInteractionState:(state,reason)=>callbacks.current.onAssemblyInteractionState?.(state,reason),
      operationFailed:error=>callbacks.current.onOperationFailed?.(error),
      inspectAssemblySupports:(documentId,references,signal)=>callbacks.current.onInspectAssemblySupports!(documentId,references,signal),
      assemblyConstraintRequested: (kind, references) => callbacks.current.onAssemblyConstraint(kind, references),
      documentViewReady: (documentId, restoredCamera) => {
        const current = callbacks.current;
        if (current.view.document.id !== documentId) return;
        if (current.activeSketchID && current.sketchPlane)
          engine.current?.beginSketch(current.activeSketchID, current.sketchPlane, restoredCamera);
        else engine.current?.endSketch();
      },
      debugStateChanged: import.meta.env.DEV && import.meta.env.VITE_INPUT_DEBUG === "true" ? setDebug : undefined,
    }, callbacks.current.documentSessions);
    engine.current = instance;
    instance.render(callbacks.current.view, callbacks.current.editingView ? {
      view: callbacks.current.editingView, occurrencePath: callbacks.current.activeInstancePath,
      translation: callbacks.current.activeInstanceTranslation, bodyTreeNodeId: callbacks.current.activeBodyTreeNodeId,
      rotation: callbacks.current.activeInstanceRotation,
      liveDefinitionProjection: callbacks.current.liveDefinitionProjection,
    } : undefined);
    instance.setMotionDisplay(callbacks.current.motionDisplay);
    instance.previewInsertPattern(patternPreview.current);
    instance.selectMany(callbacks.current.selections, false);
    instance.preselect(callbacks.current.preselection, false);
    instance.setSketchLengthUnit(callbacks.current.preferredLengthUnit??"mm");
    instance.setActiveTool(callbacks.current.activeToolID);
    instance.setNavigationProfile(callbacks.current.navigationProfile);
    instance.setCatiaRotationSphereVisible(callbacks.current.catiaRotationSphereVisible);
    instance.setGridVisibility(callbacks.current.gridVisibility);
    instance.setDisplaySettings(callbacks.current.referenceVisibility, callbacks.current.solidDisplay);
    instance.setCaptureSettings(callbacks.current.captureSettings);
    instance.setTreeVisibilityOverrides(callbacks.current.treeVisibilityOverrides);
    return () => { instance.dispose(); engine.current = undefined; };
  }, [CadViewportEngine]);

  useEffect(() => {
    const instance = engine.current; if (!instance) return;
    const inputs = props.featureInputArtifacts;
    const ownerId=(props.editingView??props.view).document.id;
    const replaceInput = (view:DocumentView) => featureInputDisplay(view,inputs,ownerId,view.part?undefined:props.activeInstancePath);
    instance.render(replaceInput(props.view), props.editingView ? { view: replaceInput(props.editingView), occurrencePath: props.activeInstancePath,
      translation: props.activeInstanceTranslation, rotation: props.activeInstanceRotation,
      bodyTreeNodeId: props.activeBodyTreeNodeId, liveDefinitionProjection:props.liveDefinitionProjection } : undefined);
    instance.previewInsertPattern(patternPreview.current);
    instance.selectMany(callbacks.current.selections, false);
    instance.preselect(callbacks.current.preselection, false);
  }, [props.view, props.editingView, props.activeInstancePath, props.activeInstanceTranslation, props.activeInstanceRotation, props.activeBodyTreeNodeId, props.liveDefinitionProjection, props.featureInputArtifacts]);
  useEffect(()=>{engine.current?.setMotionDisplay(props.motionDisplay)},[props.motionDisplay]);
  useEffect(() => { engine.current?.setDatumPreview(props.datumPreview); }, [props.datumPreview]);
  useEffect(() => { engine.current?.setFeatureSelection(props.featureSelection); }, [props.featureSelection]);
  useEffect(() => { engine.current?.selectMany(props.selections, false); }, [props.selections]);
  useEffect(() => { engine.current?.preselect(props.preselection, false); }, [props.preselection]);
  useEffect(() => {
    const instance = engine.current;
    if (!instance?.isDocumentReady(props.view.document.id)) return;
    if (props.sketchPlane && props.activeSketchID) instance.beginSketch(props.activeSketchID, props.sketchPlane);
    else instance.endSketch();
  }, [props.view.document.id, props.sketchPlane, props.activeSketchID]);
  useEffect(() => { engine.current?.setSketchLengthUnit(props.preferredLengthUnit??"mm"); }, [props.preferredLengthUnit]);
  useEffect(() => { engine.current?.setActiveTool(props.activeToolID); }, [props.activeToolID]);
  useEffect(() => { engine.current?.setNavigationProfile(props.navigationProfile); }, [props.navigationProfile]);
  useEffect(() => { engine.current?.setCatiaRotationSphereVisible(props.catiaRotationSphereVisible); }, [props.catiaRotationSphereVisible]);
  useEffect(() => { engine.current?.setGridVisibility(props.gridVisibility); }, [props.gridVisibility]);
  useEffect(() => { engine.current?.setDisplaySettings(props.referenceVisibility, props.solidDisplay); }, [props.referenceVisibility, props.solidDisplay]);
  useEffect(() => { engine.current?.setCaptureSettings(props.captureSettings); }, [props.captureSettings]);
  useEffect(() => { engine.current?.setTreeVisibilityOverrides(props.treeVisibilityOverrides); }, [props.treeVisibilityOverrides]);

  useImperativeHandle(ref, () => ({
    cancelAssemblyInteraction:()=>engine.current?.cancelAssemblyInteraction(),
    settleAssemblyInteraction:async()=>{await engine.current?.settleAssemblyInteraction();},
    retryAssemblyMoveCommit:()=>engine.current?.retryAssemblyMoveCommit(),
    setAssemblyMoveDirection:(id,direction,kind)=>engine.current?.setAssemblyMoveDirection(id,direction,kind)??false,
    captureToolSelections: selections => engine.current?.captureToolSelections(selections) ?? false,
    fit: () => engine.current?.fit(),
    normalToSketch: () => engine.current?.normalToSketch(),
    normalToPlane: (selection,plane) => engine.current?.normalToPlane(selection,plane) ?? false,
    setStandardView: (view) => engine.current?.setStandardView(view),
    previewArtifact: (artifact, operation, sketchFeatureId) => engine.current?.previewArtifact(artifact, operation, sketchFeatureId),
    clearCommandPreview: (restore) => engine.current?.clearCommandPreview(restore),
    editDimension: (selection) => engine.current?.requestDimensionEdit(selection),
    measureAssemblyConstraint: (kind, references) => engine.current?.measureAssemblyConstraint(kind, references) ?? 0,
    previewAssemblyPoses: (poses) => engine.current?.previewAssemblyPoses(poses),
    previewInsertPattern: (input) => { patternPreview.current = input; engine.current?.previewInsertPattern(input); },
    assemblyAngleReferenceDirection: (references) => engine.current?.assemblyAngleReferenceDirection(references),
    focusAssemblyReference: (reference,ownerOccurrence) => engine.current?.focusAssemblyReference(reference,ownerOccurrence) ?? false,
    showRemainingMotion: motion=>engine.current?.showRemainingMotion(motion),
    sketchCommandAction:action=>engine.current?.commandAction(action),
    retrySketchReceipt:async()=>{await engine.current?.retrySketchReceipt();},
    beginExternalReconnect: (externalID) => engine.current?.beginExternalReconnect(externalID),
  }), []);

  const failedDisplay = props.view.part?.bodies.some(b => b.displayFallback) || props.view.resolvedInstances?.some(r => r.displayFallback);
  return <><div ref={host} className="cad-viewport-canvas" />
    {failedDisplay && <div className="cad-failed-body-notice" role="status">部分特征计算失败：红色半透明实体为上次成功结果，请修复结构树中标记的特征。</div>}
    {props.activeSketchID&&!dimensionEditor&&sketchCommand?.presentation==="advanced"&&<SketchCommandPanel state={sketchCommand} prompt={toolPrompt} onAction={action=>engine.current?.commandAction(action)} />}
    {props.activeSketchID&&!dimensionEditor&&sketchCommand?.presentation==="inline"&&<SketchInlineParameterInput state={sketchCommand} lengthUnit={props.preferredLengthUnit} onEditing={editing=>engine.current?.setSketchParameterEditing(editing)} onAction={action=>engine.current?.commandAction(action)} />}
    {dimensionEditor && <SketchDimensionEditor key={`${dimensionEditor.epoch}:${dimensionEditor.featureId}:${dimensionEditor.mode === "edit" ? dimensionEditor.constraintId : dimensionEditor.kind}`}
      preferredLengthUnit={props.preferredLengthUnit} request={dimensionEditor} view={props.editingView??props.view} onClose={()=>setDimensionEditor(current=>current?.epoch===dimensionEditor.epoch?undefined:current)}
      onPreview={(operations,signal)=>engine.current?.previewDimensionDefinition(dimensionEditor.featureId,operations,signal)??Promise.resolve()}
      onSelectReference={request=>engine.current?.beginSketchReferenceSelection(request)??(()=>{})}
      onHighlightReference={(featureId,reference,slot)=>engine.current?.highlightSketchReference(featureId,reference,slot)}
      onLocateReference={(featureId,reference,slot)=>engine.current?.locateSketchReference(featureId,reference,slot)}
      onSubmit={async (operations,intent)=>{
        const owner=(callbacks.current.editingView??callbacks.current.view).document;
        if(owner.id!==dimensionEditor.ownerDocumentID||(!intent?.retryReceipt&&owner.versionId!==dimensionEditor.ownerRevisionID)||callbacks.current.activeSketchID!==dimensionEditor.featureId)throw new Error("草图版本或编辑会话已改变，请重新打开尺寸编辑");
        return engine.current?.commitDimensionOperations(dimensionEditor.featureId,operations,intent)??callbacks.current.onDimensionOperations(dimensionEditor.featureId,operations,intent);
      }} />}

    {debug && <InputDebugOverlay snapshot={debug} />}</>;
});
