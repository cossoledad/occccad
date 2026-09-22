import { openDocumentTab } from "./open-document-tab";
import { canDiscardNewSketch, defaultSolidReversed, type NewSketchSession } from "./sketch-session-policy";
import { assemblyConstraintEntry } from "../../cad/assembly/assembly-angle";
import { exactNormalViewPlane } from "../../cad/navigation/normal-view";
import { AssemblyAngleParameters, angleAxisCandidateError } from "./assembly-angle-parameters";
import { fixedPoseAngles, fixedPoseFromParameters } from "../../cad/assembly/assembly-fixed-pose";
import { FeaturePreviewLegend } from "./feature-preview-legend";
import { InsertDocumentDialog } from "./insert-document-dialog";
import "./workbench.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert, App, Button, Divider, Empty, Form, Input, InputNumber,
  Select, Space, Spin, Switch, Tag, Typography,
} from "antd";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api, isMockMode } from "../../api/client";
import { ApiError } from "../../api";
import { realtime } from "../../api/realtime-client";
import { randomUUID } from "../../utils/random-uuid";
import { queryKeys } from "../../app/query-keys";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { CommandProvider } from "../../cad/command/command-context";
import { CommandRegistry } from "../../cad/command/command-registry";
import { selectionKey, selectionSetToken } from "../../cad/interaction/selection-identity";
import { sketchTreeVisible, treeVisibilityOverride } from "../../cad/interaction/tree-visibility";
import { assemblyGeometryRef, type AssemblyConstraintToolKind } from "../../cad/tool/cad-tool";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { resolveCadWorkbench } from "../../cad/workbench/cad-workbench";
import { useWorkbenchStore, type WorkbenchToolID } from "../../state/workbench-store";
import { displayLengthToMillimeters, effectiveLengthUnit, millimetersToDisplayLength,
  useUIPreferences } from "../../state/ui-preferences";
import { useApplicationContext } from "../../state/application-context";
import type { AssemblyConstraint, AssemblyGeometryRef, CommandPreview, DatumPlane, DocumentView, Feature, ParameterDefinition, ProductRelease, Selection, SketchOperation, SketchPlane, Vec3 } from "../../types";
import { WorkbenchLayout } from "./workbench-layout";
import { WorkbenchCommands, WorkbenchViewControls } from "./workbench-commands";
import { WorkbenchStatus } from "./workbench-status";
import { contextualToolbars } from "./workbench-command-model";
import type { CadViewportHandle } from "../../viewport/cad-viewport";
import { SpecificationTree, type SpecificationTreeNode } from "./specification-tree";
import { followedDocumentIDs, staleProductDocumentIDs } from "./product-edit-context";
import { createAssemblyPreviewActor } from "./assembly-preview-machine";
import { isLengthParameter, linearExtrudeLengthInput, parameterDisplayValue, parameterSourceText, parseParameterSource } from "./parameter-editor";
import { WorkbenchInspectorPanel } from "./workbench-inspector-panel";
import { findStructureEntity, isSolidFeature, selectedFeature, structureSelection, treeData, treeKeyForSelection, treeKeysForSelections } from "./workbench-tree-model";
import { ASSEMBLY_CONSTRAINT_STATUS, assemblyStatusAfterPreviewFailure, assemblySupportPresentation,
  firstDisconnectedSupport, validateReconnectCandidate } from "../../cad/assembly/assembly-constraint-ux";
import { describeAssemblyReference } from "../../cad/assembly/assembly-reference-presentation";
import { ProductReleaseCenter } from "./product-release-center";

const CadViewport = lazy(() => import("../../viewport/cad-viewport").then((module) => ({ default: module.CadViewport })));

const sketchToolCommands:WorkbenchToolID[]=["sketch.project","sketch.rectangle","sketch.polygon","sketch.slot","sketch.point","sketch.line","sketch.circle","sketch.arc","sketch.polyline","sketch.spline",
  "sketch.constraint.coincident","sketch.constraint.parallel","sketch.constraint.fixed","sketch.constraint.horizontal","sketch.constraint.vertical",
  "sketch.constraint.perpendicular","sketch.constraint.tangent","sketch.constraint.equal","sketch.dimension.linear",
  "sketch.constraint.radius","sketch.constraint.angle","sketch.constraint.concentric","sketch.constraint.point_on_object","sketch.constraint.midpoint","sketch.constraint.symmetry"];

function sketchPlane(datum: DatumPlane): SketchPlane {
  return { datumPlaneId: datum.id, plane: datum.plane, origin: datum.origin, normal: datum.normal, uDirection: datum.uDirection };
}

function featureSketchPlane(view: DocumentView, feature: Feature): SketchPlane | undefined {
	const support = feature.sketch?.support;
	if (support?.type === "PLANAR_FACE" && support.status !== "FAILED_SUPPORT" && support.origin && support.normal && support.xDirection) {
		return { datumPlaneId: `face:${support.persistentSelection?.anchor.featureId ?? feature.id}`,
			plane: "CUSTOM", origin: support.origin, normal: support.normal, uDirection: support.xDirection };
	}
  const datum = view.datumPlanes?.find((candidate) => candidate.id === feature.sketch?.support.datumPlaneId)
    ?? view.part?.datumPlanes.find((candidate) => candidate.id === feature.sketch?.support.datumPlaneId);
  return datum ? sketchPlane(datum) : undefined;
}

function occurrenceSketchPlane(plane: SketchPlane, translation?: Vec3, rotation: [number,number,number,number] = [0,0,0,1]): SketchPlane {
  const rotate = (value: Vec3): Vec3 => {
    const [x,y,z,w]=rotation,[vx,vy,vz]=value;const dot=x*vx+y*vy+z*vz,uu=x*x+y*y+z*z;
    const cross:[number,number,number]=[y*vz-z*vy,z*vx-x*vz,x*vy-y*vx];
    return [2*dot*x+(w*w-uu)*vx+2*w*cross[0],2*dot*y+(w*w-uu)*vy+2*w*cross[1],2*dot*z+(w*w-uu)*vz+2*w*cross[2]];
  };
  const origin=rotate(plane.origin);return {...plane,origin:origin.map((value,index)=>value+(translation?.[index]??0)) as Vec3,
    normal:rotate(plane.normal),uDirection:rotate(plane.uDirection)};
}

type AssemblyConstraintUIDefinition = { supports: 1 | 2; direction: boolean; distanceDirection: boolean; value?: "angle" | "distance" };
const assemblyConstraintUI: Record<AssemblyConstraint["kind"], AssemblyConstraintUIDefinition> = {
  FIX: { supports: 1, direction: false, distanceDirection: false },
  RIGID: { supports: 2, direction: false, distanceDirection: false },
  COINCIDENT: { supports: 2, direction: true, distanceDirection: false },
  CONCENTRIC: { supports: 2, direction: false, distanceDirection: false },
  ANGLE: { supports: 2, direction: false, distanceDirection: false, value: "angle" },
  DISTANCE: { supports: 2, direction: true, distanceDirection: true, value: "distance" },
};

function AssemblyConstraintFields({ kind, references, view, lengthUnit, constraint, previewEvaluation, dirty, replacing, onReplace, onLocate, onRefresh, onValueCommit, angleRelation }: {
  kind: keyof typeof assemblyConstraintUI; references: Array<AssemblyGeometryRef | undefined>;
  view?: DocumentView;
  lengthUnit: string;
  constraint?: AssemblyConstraint; dirty?: boolean; replacing?: 0 | 1 | 2;
  previewEvaluation?: CommandPreview["constraintEvaluation"];
  onReplace: (index: 0 | 1) => void; onLocate: (reference: AssemblyGeometryRef) => void;
  onRefresh?: () => void;
  onValueCommit: () => void; angleRelation?:AssemblyConstraint["angleRelation"];
}) {
  const definition = assemblyConstraintUI[kind];
  const evaluationStatus = previewEvaluation?.status ?? (dirty ? "NOT_UPDATED" : constraint?.evaluationStatus ?? "NOT_UPDATED");
  const status = ASSEMBLY_CONSTRAINT_STATUS[evaluationStatus];
  const planePair = references.slice(0, 2).every((reference) => reference && ["PLANE", "FACE"].includes(reference.kind));
  const distanceRelation = Form.useWatch("distanceRelation");
  const minimumValue = kind === "DISTANCE" && distanceRelation && distanceRelation !== "UNSIGNED" ? undefined : 0;
  const relation = angleRelation ?? constraint?.angleRelation;
  const relationOnly = kind === "ANGLE" && (relation === "PARALLEL" || relation === "PERPENDICULAR");
  const directionApplicable = relation === "PARALLEL" || relation === "PERPENDICULAR" || (kind !== "ANGLE" && definition.direction && planePair);
  const distanceDirectionApplicable = definition.distanceDirection && references.slice(0, 2)
    .some((reference) => reference && ["PLANE", "FACE"].includes(reference.kind));
  return <>
    {kind === "FIX" && constraint && <>
      <Typography.Text type="secondary">所属装配坐标系；依次绕 X、Y、Z 轴旋转。</Typography.Text>
      {["X","Y","Z"].map((axis,index)=><Form.Item key={axis} name={["fixedTranslation",index]} label={`${axis}（${lengthUnit}）`} rules={[{required:true,type:"number"}]}>
        <InputNumber style={{width:"100%"}} onBlur={onValueCommit} onPressEnter={event=>event.currentTarget.blur()} /></Form.Item>)}
      {["X","Y","Z"].map((axis,index)=><Form.Item key={axis} name={["fixedAngles",index]} label={`绕 ${axis} 旋转（deg）`} rules={[{required:true,type:"number"}]}>
        <InputNumber style={{width:"100%"}} onBlur={onValueCommit} onPressEnter={event=>event.currentTarget.blur()} /></Form.Item>)}
    </>}
    <div className={`assembly-constraint-status status-${evaluationStatus.toLowerCase()}`} role="status"
      aria-label={`Constraint status ${status.label}`}>
      <span className="assembly-status-light" style={{ background: status.color }} />
      <span><strong>{status.label}</strong><small>{previewEvaluation?.summary ?? (dirty ? "定义已修改，等待权威预览。" : constraint?.evaluationSummary ?? status.description)}</small></span>
      {constraint && evaluationStatus !== "VERIFIED" && !dirty && <Button size="small" onClick={onRefresh}>重新计算</Button>}
    </div>
    <div className="assembly-support-list"><strong>支持元素</strong>{references.slice(0, definition.supports).map((reference,index)=><div className="assembly-support-row" key={index}>
      {(() => { const base = assemblySupportPresentation(reference); const preview = index === 0 ? previewEvaluation?.first : previewEvaluation?.second;
        const support = preview ? { ...base, status: preview.status, label: preview.status === "CONNECTED" ? "Connected" as const : "NotConnected" as const,
          diagnosticCode: preview.diagnosticCode, diagnostic: preview.diagnostic } : base; return <>
        <span className={`assembly-support-index status-${support.status.toLowerCase()}`}>{index+1}</span>
        {(() => { const presentation = describeAssemblyReference(reference, view); return <span className="assembly-support-detail">
          <strong>{presentation.primary}</strong><small>{presentation.secondary}</small>
          <small><Tag color={support.status === "CONNECTED" ? "success" : "error"}>{support.label}</Tag>
            {presentation.publication && <Tag color="blue">Publication</Tag>}
            {support.diagnosticCode && <span>{support.diagnosticCode}</span>}</small>
          {support.diagnostic && <small title={support.evidenceDigest}>{support.diagnostic}</small>}
        </span>; })()}
        {reference && <Button size="small" onClick={()=>onLocate(reference)}>定位</Button>}
        <Button size="small" type={replacing===index?"primary":"default"} onClick={()=>onReplace(index as 0|1)}>
          {support.status === "NOT_CONNECTED" ? "Reconnect" : "更换"}</Button>
      </>; })()}</div>)}</div>
    {directionApplicable && <Form.Item name="directionRelation" label="方向"><Select onChange={onValueCommit} options={relation === "PERPENDICULAR" ? [{value:"SAME",label:"正向（90°）"},{value:"OPPOSITE",label:"反向（270°）"}] : [
      {value:"UNORIENTED",label:"未定义"},{value:"SAME",label:"同向"},{value:"OPPOSITE",label:"反向"}]} /></Form.Item>}
    {distanceDirectionApplicable && <Form.Item name="distanceRelation" label="距离方向"><Select onChange={onValueCommit} options={[
      {value:"UNSIGNED",label:"无符号"},{value:"ALONG_SECOND_NORMAL",label:"沿第二元素法向"},{value:"OPPOSITE_SECOND_NORMAL",label:"逆第二元素法向"}]} /></Form.Item>}
    {definition.value && !relationOnly && <Form.Item name="value" label={definition.value === "angle" ? "角度（deg）" : `距离（${lengthUnit}）`}
      rules={[{required:true},{type:"number",min:minimumValue,max:definition.value === "angle"?360:undefined}]}>
      <InputNumber min={minimumValue} max={definition.value === "angle"?360:undefined} precision={3} style={{width:"100%"}}
        onBlur={onValueCommit} onPressEnter={(event)=>event.currentTarget.blur()} /></Form.Item>}
  </>;
}


export function Workbench() {
  const { documentID = "" } = useParams();
  const navigate = useNavigate();
  const newSketchSession = useRef<NewSketchSession | undefined>(undefined);
  const client = useQueryClient();
  const { message } = App.useApp();
  const commandRegistry = useMemo(() => new CommandRegistry(), []);
  const viewport = useRef<CadViewportHandle>(null);
  const normalViewRequest = useRef(0);
  const [padOpen, setPadOpen] = useState(false);
  const [padGenerator, setPadGenerator] = useState<"LINEAR_EXTRUDE" | "REVOLVE">("LINEAR_EXTRUDE");
  const [padSketchID, setPadSketchID] = useState<string>();
  const [padPreviewPending, setPadPreviewPending] = useState(false);
	const [editingExtrude, setEditingExtrude] = useState<{ feature: Feature; digest: string }>();
	const [featurePreviewPending, setFeaturePreviewPending] = useState(false);
	const [featurePreviewError, setFeaturePreviewError] = useState<string>();
	const featurePreviewAbort = useRef<AbortController | undefined>(undefined);
	const featurePreviewSequence = useRef(0);
	const featurePreviewID = useRef<string | undefined>(undefined);
	const featureInteractionID = useRef<string | undefined>(undefined);
  const padPreviewAbort = useRef<AbortController | undefined>(undefined);
  const padPreviewSequence = useRef(0);
  const padIntentRequestID = useRef<string | undefined>(undefined);
	const padPreviewID = useRef<string | undefined>(undefined);
  const latestDocumentVersion = useRef<string | undefined>(undefined);
  const automaticUpdateSignature = useRef("");
  const automaticUpdateRunning = useRef(false);
  const [activeDocumentID, setActiveDocumentID] = useState(documentID);
  const [activeInstancePath, setActiveInstancePath] = useState<string>();
  const [definitionContextPath, setDefinitionContextPath] = useState<string>();
  const [insertOpen, setInsertOpen] = useState(false);
  const [newPartTarget, setNewPartTarget] = useState<SpecificationTreeNode>();
  const [versionOpen, setVersionOpen] = useState(false);
  const [releaseOpen, setReleaseOpen] = useState(false);
  const [datumPlaneOpen, setDatumPlaneOpen] = useState(false);
  const [datumAxisOpen, setDatumAxisOpen] = useState(false);
  const [parameterManagerOpen, setParameterManagerOpen] = useState(false);
  const [publicationManagerOpen, setPublicationManagerOpen] = useState(false);
  const [externalParameterID, setExternalParameterID] = useState<string>();
  const [editingParameterID, setEditingParameterID] = useState<string>();
  const [pendingAssemblyConstraint, setPendingAssemblyConstraint] = useState<{ kind: AssemblyConstraintToolKind; references: AssemblyGeometryRef[]; angleRelation?:AssemblyConstraint["angleRelation"]; angleAxis?:AssemblyGeometryRef; reverseAngleAxis?:boolean; angleReferenceDirection?: Vec3 }>();
  const [editingAssemblyConstraint, setEditingAssemblyConstraint] = useState<AssemblyConstraint>();
  const [replacingAssemblyReference, setReplacingAssemblyReference] = useState<0 | 1 | 2>();
  const [assemblyDefinitionDirty, setAssemblyDefinitionDirty] = useState(false);
  const [reconnectError, setReconnectError] = useState<string>();
  const [assemblyPreviewEvaluation, setAssemblyPreviewEvaluation] = useState<CommandPreview["constraintEvaluation"]>();
  const [assemblyPreviewCommit, setAssemblyPreviewCommit] = useState(0);
  const assemblyPreviewAbort = useRef<AbortController | undefined>(undefined);
  const assemblyPreviewSequence = useRef(0);
	const assemblyPreviewID = useRef<string | undefined>(undefined);
	const assemblyInteractionID = useRef<string>(randomUUID());
  const assemblyPreviewActor = useRef<ReturnType<typeof createAssemblyPreviewActor> | undefined>(undefined);
  const [assemblyPreviewSnapshot, setAssemblyPreviewSnapshot] = useState(() => createAssemblyPreviewActor().getSnapshot());
  const inspectorOpen = useUIPreferences((state) => state.inspectorOpen);
  const setInspectorOpen = useUIPreferences((state) => state.setInspectorOpen);
  const treeVisibilityOverrides = useUIPreferences((state) => state.treeVisibilityOverrides);
  const setTreeVisibility = useUIPreferences((state) => state.setTreeVisibility);
  const catiaRotationSphereVisible = useUIPreferences((state) => state.catiaRotationSphereVisible);
  const navigationProfile = useUIPreferences((state) => state.navigationProfile);
  const captureSettings = useUIPreferences((state) => state.captureSettings);
  const displayLengthUnit = useUIPreferences((state) => state.displayLengthUnit);
  const documentLengthUnits = useUIPreferences((state) => state.documentLengthUnits);
  const setShellActiveDocumentID = useApplicationContext((state) => state.setActiveDocumentID);
  const [shareResource, setShareResource] = useState<ShareResource>();
  const [padForm] = Form.useForm<{ generator: "LINEAR_EXTRUDE" | "REVOLVE"; operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
    lengthSource: string; angle: number; axisEntityId?: string; reversed: boolean }>();
	const [featureForm] = Form.useForm<{ lengthText: string }>();
  const [newPartForm] = Form.useForm<{ name?: string; description?: string }>();
  const [versionForm] = Form.useForm<{ name: string; description: string }>();
  const [datumPlaneForm] = Form.useForm<{ name: string; offset: number }>();
  const [datumAxisForm] = Form.useForm<{ name: string; ox: number; oy: number; oz: number; dx: number; dy: number; dz: number }>();
  const [parameterForm] = Form.useForm<{ key: string; source: string }>();
  const [publicationForm] = Form.useForm<{ name: string; semanticPurpose: string }>();
  const [contextReferenceForm] = Form.useForm<{ name:string;catalogKey:string;publicationType:string;targetId:string }>();
  const [replacementForm] = Form.useForm<{referencedDocumentId:string}>();
  const [externalParameterForm] = Form.useForm<{ catalogKey: string }>();
  const [assemblyConstraintForm] = Form.useForm<{ value: number; directionRelation: string; distanceRelation: string; fixedTranslation:Vec3; fixedAngles:Vec3 }>();
  const padOperation = Form.useWatch("operation", padForm) ?? "NEW_BODY";
  const contextPublicationType = Form.useWatch("publicationType", contextReferenceForm) ?? "PLANE";
  const assemblyDirection = Form.useWatch("directionRelation", assemblyConstraintForm);
  const assemblyDistance = Form.useWatch("distanceRelation", assemblyConstraintForm);
  const store = useWorkbenchStore();
  const document = useQuery({ queryKey: queryKeys.document(documentID), queryFn: () => api.getDocument(documentID), enabled: Boolean(documentID) });
	useEffect(() => { setActiveDocumentID(documentID); setActiveInstancePath(undefined); setDefinitionContextPath(undefined); store.endSketch(); store.setSelection(null); }, [documentID]);
  useEffect(() => {
    // A stopped XState actor cannot be restarted. Own one actor per effect
    // lifetime so StrictMode's setup/cleanup/setup cycle retains live previews.
    const actor = createAssemblyPreviewActor();
    assemblyPreviewActor.current = actor;
    const subscription = actor.subscribe(setAssemblyPreviewSnapshot);
    actor.start();
    return () => { assemblyPreviewActor.current = undefined; subscription.unsubscribe(); actor.stop(); };
  }, [assemblyPreviewActor]);
  const activeDocument = useQuery({ queryKey: queryKeys.document(activeDocumentID), queryFn: () => api.getDocument(activeDocumentID),
    enabled: Boolean(activeDocumentID && activeDocumentID !== documentID) });
	const activeID = activeDocumentID || documentID;
	const lengthUnit = effectiveLengthUnit(displayLengthUnit, documentLengthUnits, activeID);
	useEffect(() => {
	  setShellActiveDocumentID(activeID);
	  return () => setShellActiveDocumentID(undefined);
	}, [activeID, setShellActiveDocumentID]);
	const toolbarCatalog = useQuery({ queryKey: ["ui", "toolbars"], queryFn: api.toolbarCatalog, staleTime: 5 * 60_000 });
  const catalog = useQuery({ queryKey: queryKeys.documents({ workbench: true }), queryFn: () => api.listDocuments({ limit: 100, allFolders: true }), enabled: publicationManagerOpen });

  useEffect(() => {
    if (document.data) void client.invalidateQueries({ queryKey: queryKeys.openDocuments });
  }, [client, document.data]);
  const refresh = useCallback(async (view?: DocumentView) => {
    if (view) client.setQueryData(queryKeys.document(view.document.id), view);
    const changedID = view?.document.id ?? activeID;
    await Promise.all([view ? (changedID === documentID ? Promise.resolve() : client.invalidateQueries({ queryKey: queryKeys.document(documentID) }))
      : client.invalidateQueries({ queryKey: queryKeys.document(changedID) }),
    client.invalidateQueries({ queryKey: queryKeys.history(changedID), refetchType: "active" }),
    client.invalidateQueries({ queryKey: queryKeys.documentProperties(changedID), refetchType: "active" }),
    client.invalidateQueries({ queryKey: ["product-design-session", documentID] }),
    client.invalidateQueries({ queryKey: ["context-catalog", documentID] }),
    client.invalidateQueries({ queryKey: ["product-update-plan", documentID] }),
    client.invalidateQueries({ queryKey: ["documents"] }),
    client.invalidateQueries({ queryKey: queryKeys.openDocuments })]);
  }, [activeID, client, documentID]);
  useEffect(() => {
    if (!documentID || isMockMode) return;
    let disposed = false;
    let unsubscribe: (() => void) | undefined;
    void realtime.subscribe(documentID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(documentID), snapshot.view);
      }
      void Promise.all([
        event.type === "document.snapshot.v1" ? Promise.resolve() : client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
        client.invalidateQueries({ queryKey: queryKeys.history(documentID), refetchType: "active" }),
        client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID), refetchType: "active" }),
        client.invalidateQueries({ queryKey: ["product-update-plan", documentID] }),
        client.invalidateQueries({ queryKey: ["documents"] }),
        client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
      ]);
    }).then((dispose) => {
      if (disposed) dispose(); else unsubscribe = dispose;
    }).catch((error: Error) => {
      if (!disposed) message.error(`实时连接失败：${error.message}`);
    });
    return () => { disposed = true; unsubscribe?.(); };
  }, [client, documentID, message]);
  const command = useMutation({
    mutationFn: (operation: () => Promise<DocumentView>) => operation(),
    onSuccess: (updated) => { store.setSelection(null); void refresh(updated); }, onError: (error) => message.error(error.message)
  });
  const moveCommand = useMutation({mutationFn:(operation:()=>Promise<DocumentView>)=>operation(),
    onSuccess:(updated)=>{void refresh(updated);},onError:(error)=>{message.error(error.message);void refresh();}});
  const view = document.data;
  const editingView = activeDocumentID === documentID ? view : activeDocument.data;
  const discardNewSketch = () => {
    const session = newSketchSession.current;
    if (!session) return;
    newSketchSession.current = undefined;
    const latest = client.getQueryData<DocumentView>(queryKeys.document(session.documentId));
    if (!canDiscardNewSketch(session, latest)) return;
    // Use the owning document, even when exit was caused by switching tabs or
    // activating another occurrence. Do not clear the new context's selection.
    void api.deleteNodes(session.documentId, [{ targetKind: "FEATURE", targetId: session.sketchId }])
      .then((updated) => refresh(updated))
      .catch((error: Error) => message.error(`放弃空草图失败：${error.message}`));
  };
  const discardNewSketchOnUnmount = useRef(discardNewSketch);
  discardNewSketchOnUnmount.current = discardNewSketch;
  useEffect(() => () => discardNewSketchOnUnmount.current(), []);
  useEffect(() => {
    const session = newSketchSession.current;
    if (session && (store.activeSketchID !== session.sketchId || activeID !== session.documentId)) discardNewSketch();
  }, [store.activeSketchID, activeID, client, refresh, message]);
  const activeResolvedInstance = activeInstancePath
    ? view?.resolvedInstances?.find((instance) => instance.instancePath?.canonical === activeInstancePath) : undefined;
  const designSession = useQuery({ queryKey: queryKeys.productDesignSession(documentID, activeInstancePath ?? ""),
    queryFn: () => api.getProductDesignSession(documentID, activeInstancePath ?? ""),
    enabled: Boolean(view?.document.type === "PRODUCT") });
  const contextCatalog = useQuery({ queryKey: queryKeys.contextCatalog(documentID, activeInstancePath ?? "", contextPublicationType),
    queryFn: () => api.getContextCatalog(documentID, activeInstancePath ?? "", contextPublicationType),
    enabled: Boolean(view?.document.type === "PRODUCT" && activeInstancePath && publicationManagerOpen) });
  const parameterContextCatalog = useQuery({ queryKey: queryKeys.contextCatalog(documentID, activeInstancePath ?? "", "PARAMETER"),
    queryFn: () => api.getContextCatalog(documentID, activeInstancePath ?? "", "PARAMETER"),
    enabled: Boolean(view?.document.type === "PRODUCT" && activeInstancePath && externalParameterID) });
  const productUpdatePlan = useQuery({ queryKey: ["product-update-plan", documentID],
    queryFn: () => api.getProductUpdatePlan(documentID), enabled: Boolean(view?.document.type === "PRODUCT") });
  const productReleases = useQuery({ queryKey: ["product-releases", documentID],
    queryFn: () => api.listProductReleases(documentID), enabled: Boolean(view?.document.type === "PRODUCT" && releaseOpen) });
  latestDocumentVersion.current = editingView?.document.versionId;
  const followedIDs = useMemo(() => [...new Set([
    ...followedDocumentIDs(view?.structureTree), ...(view?.referenceUpdates ?? []).map((item) => item.sourceDocumentId),
    ...(activeID !== documentID ? [activeID] : []),
  ])].filter((id) => id !== documentID), [activeID, documentID, view?.structureTree, view?.referenceUpdates]);
  useEffect(() => {
    if (isMockMode || !view || followedIDs.length === 0) return;
    let disposed = false; const unsubscribers: Array<() => void> = [];
    for (const dependencyID of followedIDs) void realtime.subscribe(dependencyID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(dependencyID), snapshot.view);
        return;
      }
      // A fresh root projection will drive the serialized leaf-to-root auto-update effect.
      void client.invalidateQueries({ queryKey: queryKeys.document(dependencyID) });
      void client.invalidateQueries({ queryKey: queryKeys.document(documentID) });
      void client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID), refetchType: "active" });
      void client.invalidateQueries({ queryKey: ["product-update-plan", documentID] });
    }).then((unsubscribe) => { if (disposed) unsubscribe(); else unsubscribers.push(unsubscribe); })
      .catch((error: Error) => { if (!disposed) message.error(`引用文档实时连接失败：${error.message}`); });
    return () => { disposed = true; unsubscribers.forEach((unsubscribe) => unsubscribe()); };
  }, [client, documentID, followedIDs.join("|"), message, view]);
  const treeNodes = useMemo(() => {
    const consumedSketches = new Set((editingView?.part?.features ?? []).flatMap((feature) => feature.profile ? [feature.profile] : []));
    const decorate = (node: SpecificationTreeNode, parentVisible = true): SpecificationTreeNode => {
      const ownVisible = node.kind === "SKETCH" && node.entityId
        ? sketchTreeVisible({ featureID: node.entityId, treeKey: node.key, activeSketchID: store.activeSketchID,
          defaultVisible: !consumedSketches.has(node.entityId), overrides: treeVisibilityOverrides })
        : treeVisibilityOverride(node.key, treeVisibilityOverrides) ?? true;
      const visible = parentVisible && ownVisible;
      return { ...node, hidden: !visible, children: node.children?.map((child) => decorate(child, visible)) };
    };
    return view ? treeData(view, editingView).map((node) => decorate(node)) : [];
  }, [view, editingView, store.activeSketchID, treeVisibilityOverrides]);
  useEffect(()=>{
    normalViewRequest.current+=1;
    return ()=>{normalViewRequest.current+=1;};
  },[store.selection,view?.document.versionId,documentID]);
  const normalToSelection = async () => {
    const selection = store.selection;
    if (!selection || !view) return;
    const generation = ++normalViewRequest.current;
    try {
      const plane = selection.kind === "plane"
        ? selection.datumPlane ?? view.datumPlanes?.find(value=>value.id===(selection.entityId ?? selection.id))
        : selection.kind === "face" && selection.geometryKey
          ? exactNormalViewPlane(await api.getTopologyProperties(selection.documentId ?? view.document.id,
              selection.geometryKey,"FACE",selection.topologyId,selection.versionId)) : undefined;
      if (generation!==normalViewRequest.current) return;
      if (!plane && selection.kind !== "plane") { message.info("请选择基准平面或实体的平面面；曲面没有唯一法线视图。"); return; }
      if (!viewport.current?.normalToPlane(selection,plane)) message.info("所选平面当前不可用，请重新选择。");
    } catch (error) {
      if (generation===normalViewRequest.current) message.error(error instanceof Error ? error.message : String(error));
    }
  };
  const canEdit = editingView?.document.permission === "OWNER" || editingView?.document.permission === "EDITOR";
  const canEditRoot = view?.document.permission === "OWNER" || view?.document.permission === "EDITOR";
  const activeWorkbench = resolveCadWorkbench(editingView?.document.type ?? "PART", Boolean(store.sketchPlane));

  useEffect(() => {
    if (!view || view.document.type !== "PRODUCT" || !canEditRoot || automaticUpdateRunning.current) return;
    const targets = staleProductDocumentIDs(view.structureTree);
    if (!targets.length) { automaticUpdateSignature.current = ""; return; }
    const signature = `${view.document.versionId}:${targets.join("|")}`;
    if (automaticUpdateSignature.current === signature) return;
    automaticUpdateSignature.current = signature;
    automaticUpdateRunning.current = true;
    let disposed = false;
    void (async () => {
      try {
        for (const productID of targets) {
          const plan = await api.getProductUpdatePlan(productID);
          if (!plan.hasUpdates) continue;
          if (!plan.canAccept) throw new Error(plan.entries.find((entry) => entry.kind !== "ASSEMBLY_SOLVE" && entry.diagnostic)?.diagnostic ?? "Product 自动更新被上游求值阻塞");
          const updated = await api.acceptProductUpdatePlan(productID, plan.digest);
          client.setQueryData(queryKeys.document(productID), updated);
        }
        if (!disposed) await Promise.all([
          client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
          client.invalidateQueries({ queryKey: ["product-update-plan", documentID] }),
        ]);
      } catch (cause) {
        if (!disposed) message.error(`自动跟随最新版本失败：${cause instanceof Error ? cause.message : String(cause)}`);
      } finally {
        automaticUpdateRunning.current = false;
      }
    })();
    return () => { disposed = true; };
  }, [canEditRoot, client, documentID, message, view]);

  useEffect(() => {
    if (replacingAssemblyReference === undefined || !store.selection) return;
    const reference = assemblyGeometryRef(store.selection);
    if (!reference) {
      setReconnectError("请选择点、轴、平面、面、边、顶点或实体。");
      return;
    }
    if (replacingAssemblyReference === 2) {
      const second = editingAssemblyConstraint?.second ?? pendingAssemblyConstraint?.references[1];
      const error = angleAxisCandidateError(reference,second);
      if (error) { setReconnectError(error); return; }
      if (editingAssemblyConstraint) setEditingAssemblyConstraint({...editingAssemblyConstraint,angleAxis:reference,angleRelation:"DIRECTED"});
      if (pendingAssemblyConstraint) setPendingAssemblyConstraint({...pendingAssemblyConstraint,angleAxis:reference,angleRelation:"DIRECTED"});
    } else if (editingAssemblyConstraint) {
      const other = replacingAssemblyReference === 0 ? editingAssemblyConstraint.second : editingAssemblyConstraint.first;
      const validation = validateReconnectCandidate(reference, other);
      if (validation) { setReconnectError(validation); return; }
      setEditingAssemblyConstraint((current) => {
        if (!current) return current;
        const next = { ...current, ...(replacingAssemblyReference === 0 ? { first: reference } : { second: reference }) };
        return { ...next, angleAxis: replacingAssemblyReference === 1 ? undefined : next.angleAxis,
          angleReferenceDirection: undefined };
      });
    } else if (pendingAssemblyConstraint) {
      const references = [...pendingAssemblyConstraint.references];
      const other = references[replacingAssemblyReference === 0 ? 1 : 0];
      const validation = validateReconnectCandidate(reference, other);
      if (validation) { setReconnectError(validation); return; }
      references[replacingAssemblyReference] = reference;
      setPendingAssemblyConstraint({ ...pendingAssemblyConstraint, references,
        angleAxis: replacingAssemblyReference === 1 ? undefined : pendingAssemblyConstraint.angleAxis, angleReferenceDirection: undefined });
    }
    setAssemblyDefinitionDirty(true);
    setAssemblyPreviewEvaluation(undefined);
    setReconnectError(undefined);
    setReplacingAssemblyReference(undefined);
  }, [store.selection, replacingAssemblyReference, editingAssemblyConstraint, pendingAssemblyConstraint]);

  useEffect(() => {
    const constraint = editingAssemblyConstraint;
    const pending = pendingAssemblyConstraint;
    const references = constraint ? [constraint.first, constraint.second].filter((value): value is AssemblyGeometryRef => Boolean(value))
      : pending?.references ?? [];
    const kind = (constraint?.kind.toLowerCase() ?? pending?.kind) as AssemblyConstraintToolKind | undefined;
    const missingAngleAxis = kind === "angle" && (constraint?.angleRelation ?? pending?.angleRelation ?? "FREE") === "DIRECTED" && !(constraint?.angleAxis ?? pending?.angleAxis);
    if (missingAngleAxis || !editingView || !kind || references.length < (kind === "fix" ? 1 : 2) || replacingAssemblyReference !== undefined) {
	  assemblyPreviewID.current=undefined;
      assemblyPreviewActor.current?.send({ type: "RESET" });
      return;
    }
	const sequence=++assemblyPreviewSequence.current;
	assemblyPreviewID.current=undefined;
    setAssemblyPreviewEvaluation(undefined);
    assemblyPreviewActor.current?.send({ type: "REQUEST", sequence });
    assemblyPreviewAbort.current?.abort();
    const controller=new AbortController();assemblyPreviewAbort.current=controller;
    const timer = window.setTimeout(() => {
      const rawValue=Number(assemblyConstraintForm.getFieldValue("value")??0);
      const value = kind === "angle" ? rawValue * Math.PI / 180
        : kind === "distance" ? displayLengthToMillimeters(rawValue, lengthUnit) : rawValue;
      const commandInput = constraint ? {
        type: "EDIT_ASSEMBLY_CONSTRAINT", targetId: constraint.id, value,
        fixedPose: constraint.kind === "FIX" ? fixedPoseFromParameters(
          (assemblyConstraintForm.getFieldValue("fixedTranslation") as Vec3).map(v=>displayLengthToMillimeters(v,lengthUnit)) as Vec3,
          assemblyConstraintForm.getFieldValue("fixedAngles")) : undefined, angleRelation:constraint.angleRelation,fixMode:constraint.fixMode,
        directionRelation: assemblyDirection ?? "UNORIENTED", distanceRelation: assemblyDistance ?? "UNSIGNED",
        firstAssemblyRef: references[0], secondAssemblyRef: references[1],
        angleAxis:constraint.angleAxis,reverseAngleAxis:constraint.reverseAngleAxis,angleReferenceDirection: constraint.angleReferenceDirection,
      } : {
        type: "ADD_ASSEMBLY_CONSTRAINT", constraintKind: kind.toUpperCase(), value,
        directionRelation: assemblyDirection ?? "UNORIENTED", distanceRelation: assemblyDistance ?? "UNSIGNED",
        firstAssemblyRef: references[0], secondAssemblyRef: references[1],
        angleRelation:pending?.angleRelation,angleAxis:pending?.angleAxis,reverseAngleAxis:pending?.reverseAngleAxis,
      };
	  void api.previewCommand(editingView.document.id, {...commandInput,interactionId:assemblyInteractionID.current,previewSequence:sequence},controller.signal).then((preview) => {
		if (sequence===assemblyPreviewSequence.current&&preview.baseVersionId === editingView.document.versionId) {
		  assemblyPreviewID.current=preview.previewId;
          setAssemblyPreviewEvaluation(preview.constraintEvaluation);
          if (preview.instancePoses) viewport.current?.previewAssemblyPoses(preview.instancePoses);
          assemblyPreviewActor.current?.send({ type: "RESOLVE", sequence, components: preview.assemblyComponents });
        }
      }).catch((cause: unknown) => {
        if (controller.signal.aborted) {
          assemblyPreviewActor.current?.send({ type: "CANCEL", sequence });
          return;
        }
        const error = cause instanceof Error ? cause : new Error(String(cause));
        const apiError = cause instanceof ApiError ? cause : undefined;
		if (sequence === assemblyPreviewSequence.current) {
		  const supports = references.map((reference) => assemblySupportPresentation(reference));
		  setAssemblyPreviewEvaluation({
		    constraintId: constraint?.id ?? "preview",
		    status: assemblyStatusAfterPreviewFailure(apiError?.phase, Boolean(apiError?.retryable), supports),
		    summary: error.message,
		    first: { status: supports[0]?.status ?? "NOT_CONNECTED", diagnosticCode: supports[0]?.diagnosticCode, diagnostic: supports[0]?.diagnostic },
		    ...(supports[1] ? { second: { status: supports[1].status, diagnosticCode: supports[1].diagnosticCode, diagnostic: supports[1].diagnostic } } : {}),
		  });
		}
        assemblyPreviewActor.current?.send({ type: "REJECT", sequence, error: error.message,
          errorCode: apiError?.code, phase: apiError?.phase, retryable: apiError?.retryable });
      });
    }, 140);
    return () => {window.clearTimeout(timer);controller.abort();assemblyPreviewActor.current?.send({type:"CANCEL",sequence});};
  }, [editingView, editingAssemblyConstraint, pendingAssemblyConstraint, replacingAssemblyReference,
    assemblyDirection, assemblyDistance, assemblyPreviewCommit, assemblyConstraintForm, assemblyPreviewActor, lengthUnit]);

  const editSketch = (featureID: string, operations: SketchOperation[]) => {
    if (operations.length && newSketchSession.current?.sketchId === featureID) newSketchSession.current.edited = true;
    if (!editingView) return;
    command.mutate(() => api.editSketch(editingView.document.id, featureID, operations));
  };
  const moveInstance = (instanceID: string, translation: Vec3, rotation:[number,number,number,number], previewId?:string) => {
	if (editingView?.document.type === "PRODUCT" && canEdit) moveCommand.mutate(() => api.move(editingView.document.id, instanceID, translation,rotation,previewId));
  };
  const executeHistory = (direction: "undo" | "redo") => {
    if (!editingView) return; command.mutate(() => direction === "undo" ? api.undo(editingView.document.id) : api.redo(editingView.document.id));
  };
  const openAssemblyConstraintEditor = (constraint: AssemblyConstraint, reconnect = false) => {
    assemblyInteractionID.current = randomUUID();
    assemblyPreviewActor.current?.send({ type: "START" });
    setEditingAssemblyConstraint({ ...constraint, angleRelation:constraint.kind === "ANGLE" ? constraint.angleRelation ?? "FREE" : undefined });
    setAssemblyDefinitionDirty(false);
    setAssemblyPreviewEvaluation(undefined);
    setReconnectError(undefined);
    setReplacingAssemblyReference(reconnect ? firstDisconnectedSupport(constraint) : undefined);
    if (reconnect) { store.setSelection(null); store.setActiveTool("select", "once"); }
    const instance = editingView?.product?.instances.find(value=>value.id===constraint.first.instanceId);
    const fixedPose = constraint.fixedPose ?? {translation:instance?.translation ?? [0,0,0],rotation:instance?.rotation ?? [0,0,0,1]};
    assemblyConstraintForm.setFieldsValue({
      fixedTranslation: fixedPose.translation.map(v=>millimetersToDisplayLength(v,lengthUnit)),
      fixedAngles: fixedPoseAngles(fixedPose),
      value: constraint.kind === "ANGLE" ? (constraint.value ?? 0) * 180 / Math.PI
        : constraint.kind === "DISTANCE" ? millimetersToDisplayLength(constraint.value ?? 0, lengthUnit) : constraint.value ?? 0,
      directionRelation: constraint.kind === "ANGLE" && constraint.angleReferenceDirection ? "SAME"
        : constraint.directionRelation && constraint.directionRelation !== "UNORIENTED" ? constraint.directionRelation
        : constraint.second && [constraint.first, constraint.second].every((reference) => ["PLANE", "FACE"].includes(reference.kind))
          ? (viewport.current?.measureAssemblyConstraint("angle", [constraint.first, constraint.second]) ?? 0) > 90 ? "OPPOSITE" : "SAME"
          : "UNORIENTED",
      distanceRelation: constraint.distanceRelation ?? "UNSIGNED",
    });
  };
  const refreshAssemblyConstraint = (constraintID?: string) => {
    if (editingView?.document.type !== "PRODUCT") return;
    const operation = productUpdatePlan.data && editingView.document.id === documentID
      ? () => api.acceptProductUpdatePlan(documentID, productUpdatePlan.data!.digest)
      : () => api.updateReferences(editingView.document.id);
    command.mutate(operation, { onSuccess: (updated) => {
      if (!constraintID) return;
      const refreshed = updated.product?.constraints?.find((constraint) => constraint.id === constraintID);
      if (refreshed) { setEditingAssemblyConstraint({ ...refreshed }); setAssemblyDefinitionDirty(false); }
    }});
  };
  const selectFeature = (sourceView: DocumentView, featureID: string) => {
    const node = findStructureEntity(sourceView.structureTree, featureID);
    if (!node) return;
    const selection = structureSelection(node, sourceView);
    if (!selection) return;
    if (!activeInstancePath || !activeResolvedInstance) {
      store.setSelection(selection);
      return;
    }
    const bodyMarker = node.id.indexOf("/body/");
    store.setSelection({ ...selection,
      documentId: sourceView.document.id,
      occurrencePath: activeInstancePath,
      instancePath: activeResolvedInstance.instancePath,
      instanceId: activeInstancePath.split("/")[0],
      geometryKey: sourceView.artifact?.geometryKey,
      treeNodeId: bodyMarker >= 0 ? `${activeResolvedInstance.bodyTreeNodeId}${node.id.slice(bodyMarker + 5)}` : selection.treeNodeId,
      visualKey: selection.kind === "sketch" ? undefined : `body:${activeInstancePath}:body`,
    });
  };
  const finishSketch = () => {
    if (command.isPending) return;
    const sketchID = store.activeSketchID;
    const discard = newSketchSession.current && canDiscardNewSketch(newSketchSession.current, editingView);
    store.endSketch();
    if (discard) store.setSelection(null);
    else if (sketchID && editingView) selectFeature(editingView, sketchID);
  };
  const startSketch = () => {
    if (!editingView || !store.selection) return;
    if (store.selection.kind === "sketch") {
      const feature = editingView.part?.features.find((candidate) => candidate.id === store.selection!.id);
      const localPlane = feature ? featureSketchPlane(editingView, feature) : undefined;
      const plane = localPlane ? occurrenceSketchPlane(localPlane, activeResolvedInstance?.translation, activeResolvedInstance?.rotation) : undefined;
      if (feature && plane) store.beginSketch(feature.id, plane);
      return;
    }
    if (store.selection.kind === "face") {
	  const selection = store.selection;
	  if (!selection.geometryKey || !selection.topologyId) return;
	  command.mutate(() => api.createSketch(editingView.document.id, { targetKind: "FACE", geometryKey: selection.geometryKey,
		  topologyId: selection.topologyId, versionId: selection.versionId ?? editingView.document.versionId }), { onSuccess: (updated) => {
		const sketch = [...(updated.part?.features ?? [])].reverse().find((candidate) => candidate.type.toUpperCase() === "SKETCH");
		const localPlane = sketch ? featureSketchPlane(updated, sketch) : undefined;
		if (sketch && localPlane) {
          newSketchSession.current = { documentId: updated.document.id, sketchId: sketch.id, edited: false };
          store.beginSketch(sketch.id, occurrenceSketchPlane(localPlane, activeResolvedInstance?.translation, activeResolvedInstance?.rotation));
        }
	  }});
	  return;
	}
    if (store.selection.kind !== "plane") return;
    const datum = store.selection.datumPlane ?? editingView.datumPlanes?.find((candidate) => store.selection?.id.endsWith(candidate.id));
    if (!datum) return;
    const plane = occurrenceSketchPlane(sketchPlane(datum), activeResolvedInstance?.translation, activeResolvedInstance?.rotation);
    command.mutate(() => api.createSketch(editingView.document.id, { plane: datum.plane, datumPlaneId: datum.id }), { onSuccess: (updated) => {
      const sketch = [...(updated.part?.features ?? [])].reverse().find((feature) => feature.type.toUpperCase() === "SKETCH");
      if (sketch) {
        newSketchSession.current = { documentId: updated.document.id, sketchId: sketch.id, edited: false };
        store.beginSketch(sketch.id, plane);
      }
    }});
  };
  const padSketch = (values: { generator: "LINEAR_EXTRUDE" | "REVOLVE"; operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
    lengthSource: string; angle: number; axisEntityId?: string; reversed: boolean }) => {
    if (!editingView || !padSketchID) return;
    padPreviewAbort.current?.abort();
    viewport.current?.clearCommandPreview();
    const generator = values.generator ?? padGenerator;
    const lengthInput = generator === "LINEAR_EXTRUDE" ? linearExtrudeLengthInput(values.lengthSource, lengthUnit) : {};
    command.mutate(() => api.createSolidFeature(editingView.document.id, { sketchId: padSketchID, generator,
      operation: values.operation, ...lengthInput,
      angle: generator === "REVOLVE" ? values.angle : undefined,
	  axisEntityId: generator === "REVOLVE" ? values.axisEntityId : undefined, reversed: values.reversed,
	  previewId: padPreviewID.current }, padIntentRequestID.current), { onSuccess: (updated) => {
        const feature = [...(updated.part?.features ?? [])].reverse().find((candidate) =>
          isSolidFeature(candidate) && candidate.profile === padSketchID);
        if (feature) selectFeature(updated, feature.id);
      }});
	setPadOpen(false); setPadSketchID(undefined); padIntentRequestID.current = undefined; padPreviewID.current=undefined;
  };
  const closePad = () => {
    padPreviewAbort.current?.abort(); padPreviewSequence.current += 1; setPadPreviewPending(false);
	viewport.current?.clearCommandPreview(); setPadOpen(false); setPadSketchID(undefined); padIntentRequestID.current = undefined; padPreviewID.current=undefined;
  };
	const closeFeatureEditor = () => {
		featurePreviewAbort.current?.abort(); featurePreviewSequence.current += 1;
		viewport.current?.clearCommandPreview(); featurePreviewID.current=undefined; featureInteractionID.current=undefined;
		setFeaturePreviewPending(false); setFeaturePreviewError(undefined); setEditingExtrude(undefined);
	};
	const openFeatureEditor = (node: SpecificationTreeNode) => {
		if (!editingView || !node.entityId || !node.definitionDigest || !node.capabilities?.includes("EDIT")) return;
		const feature=editingView.part?.features.find((candidate)=>candidate.id===node.entityId);
		if (!feature || !["PAD","LINEAR_EXTRUDE"].includes(feature.type.toUpperCase())) return;
		const parameter=editingView.part?.parameters?.find((candidate)=>candidate.parameterId===`parameter:${feature.id}:length`);
		featureForm.setFieldsValue({lengthText:parameter?parameterSourceText(parameter, lengthUnit):String(millimetersToDisplayLength(feature.length??0, lengthUnit))}); featurePreviewID.current=undefined;
		featureInteractionID.current=randomUUID();
		setFeaturePreviewError(undefined); setEditingExtrude({feature,digest:node.definitionDigest});
	};
	const requestFeaturePreview = async () => {
		if (!editingView || !editingExtrude) return;
		let lengthInput: {length?:number;lengthExpression?:string};
		try {
			const values=await featureForm.validateFields(); lengthInput=linearExtrudeLengthInput(values.lengthText, lengthUnit);
		} catch { return; }
		featurePreviewAbort.current?.abort(); const abort=new AbortController(); featurePreviewAbort.current=abort;
		const sequence=++featurePreviewSequence.current, baseVersionID=editingView.document.versionId;
		featurePreviewID.current=undefined; viewport.current?.clearCommandPreview(); setFeaturePreviewError(undefined); setFeaturePreviewPending(true);
		try {
			const preview=await api.previewCommand(editingView.document.id,{type:"EDIT_FEATURE",targetId:editingExtrude.feature.id,
				expectedFeatureDigest:editingExtrude.digest,...lengthInput,interactionId:featureInteractionID.current,
				previewSequence:sequence},abort.signal);
			if(sequence!==featurePreviewSequence.current||preview.baseVersionId!==baseVersionID||preview.baseVersionId!==latestDocumentVersion.current||!preview.artifact)return;
			featurePreviewID.current=preview.previewId; viewport.current?.previewArtifact(preview.artifact, editingExtrude.feature.operation);
		} catch(cause) {
			if(abort.signal.aborted)return; const error=cause instanceof Error?cause:new Error(String(cause));
			setFeaturePreviewError(error.message);
		} finally { if(sequence===featurePreviewSequence.current)setFeaturePreviewPending(false); }
	};
	const commitFeatureEdit = async () => {
		if(!editingView||!editingExtrude)return;
		let lengthInput: {length?:number;lengthExpression?:string};
		try {
			const values=await featureForm.validateFields(); lengthInput=linearExtrudeLengthInput(values.lengthText, lengthUnit);
		} catch { return; }
		command.mutate(()=>api.editFeature(editingView.document.id,{featureId:editingExtrude.feature.id,
			expectedFeatureDigest:editingExtrude.digest,...lengthInput,previewId:featurePreviewID.current}),{
			onSuccess:(updated)=>{selectFeature(updated,editingExtrude.feature.id);viewport.current?.clearCommandPreview(false);featurePreviewID.current=undefined;featureInteractionID.current=undefined;setEditingExtrude(undefined);},
			onError:(cause)=>setFeaturePreviewError(cause instanceof Error?cause.message:String(cause))});
	};
  const requestPadPreview = async (sketchID: string, generatorOverride?: "LINEAR_EXTRUDE" | "REVOLVE") => {
    if (!editingView) return;
    const values = padForm.getFieldsValue(); values.generator = generatorOverride ?? values.generator ?? padGenerator;
    let lengthInput: { length?: number; lengthExpression?: string } = {};
    if (values.generator === "LINEAR_EXTRUDE") {
      try { lengthInput = linearExtrudeLengthInput(values.lengthSource, lengthUnit); } catch { return; }
    }
    if (values.generator === "REVOLVE" && (!Number.isFinite(values.angle) || values.angle <= 0 || !values.axisEntityId)) return;
    padPreviewAbort.current?.abort();
    const abort = new AbortController(); padPreviewAbort.current = abort;
	const sequence = ++padPreviewSequence.current; const baseVersionID = editingView.document.versionId;
	padPreviewID.current=undefined;
    viewport.current?.clearCommandPreview();
    setPadPreviewPending(true);
    try {
      const preview = await api.previewCommand(editingView.document.id, { type: "CREATE_SOLID_FEATURE", sketchId: sketchID,
        generator: values.generator, operation: values.operation, ...lengthInput, angle: values.angle,
        axisEntityId: values.axisEntityId, reversed: values.reversed,
        ...(padIntentRequestID.current ? { requestId: padIntentRequestID.current } : {}) }, abort.signal);
	  if (sequence !== padPreviewSequence.current || preview.baseVersionId !== baseVersionID ||
		preview.baseVersionId !== latestDocumentVersion.current || !preview.artifact) return;
	  padPreviewID.current=preview.previewId;
      viewport.current?.previewArtifact(preview.artifact, values.operation);
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) message.error(`预览失败：${(error as Error).message}`);
    } finally {
      if (sequence === padPreviewSequence.current) setPadPreviewPending(false);
    }
  };
  const previewPad = () => {
    if (padSketchID) void requestPadPreview(padSketchID);
  };
  const openSolidFeature = (generator: "LINEAR_EXTRUDE" | "REVOLVE", operation?: "NEW_BODY" | "ADD" | "REMOVE") => {
    if (store.selection?.kind !== "sketch") return;
    const hasBody = Boolean(editingView?.part?.features.some((feature) => feature.type === "IMPORT_BODY" ||
      ["PAD", "LINEAR_EXTRUDE", "REVOLVE"].includes(feature.type.toUpperCase())));
    const selectedOperation = operation ?? (hasBody ? "ADD" : "NEW_BODY");
    const sketchID = store.selection.id;
	padIntentRequestID.current = randomUUID(); padPreviewID.current=undefined; setPadSketchID(sketchID); setPadGenerator(generator);
    padForm.setFieldsValue({ generator, operation: selectedOperation, lengthSource: "40", angle: 360,
      axisEntityId: undefined, reversed: defaultSolidReversed(generator, selectedOperation) });
    setPadOpen(true);
  };
  useEffect(() => {
    if (!padOpen || !padSketchID) return;
    void requestPadPreview(padSketchID, padGenerator);
  }, [padOpen, padSketchID, padGenerator]);
  useEffect(() => {
    if (!padOpen || padGenerator !== "REVOLVE" || !padSketchID || !store.selection) return;
    const selection = store.selection;
    let reference: string | undefined;
    if (selection.kind === "axis") {
      const parts = selection.id.split(":");
      reference = selection.axis === "DATUM"
        ? `DATUM_AXIS:${parts.at(-1)}` : `AXIS_SYSTEM:${parts.at(-2)}:${selection.axis}`;
    } else if (selection.kind === "visual") {
      const selectedSketch = editingView?.part?.features.find((feature) => feature.id === selection.featureId)?.sketch;
      const entity = selectedSketch?.entities.find((candidate) => candidate.id === selection.entityId);
      if (entity?.kind === "LINE") reference = `SKETCH_LINE:${selection.featureId}:${entity.id}`;
    }
    if (!reference) return;
    padForm.setFieldValue("axisEntityId", reference);
    void requestPadPreview(padSketchID, "REVOLVE");
  }, [padOpen, padGenerator, padSketchID, store.selection, editingView]);
  const createPartComponent = (values: { name?: string; description?: string }) => {
    if (view?.document.type !== "PRODUCT" || !newPartTarget) return;
    command.mutate(() => api.createPartComponent(view.document.id, { name: values.name?.trim() || undefined,
      description: values.description?.trim() || undefined,
      targetProductInstancePath: newPartTarget.instancePath }), {
      onSuccess: () => { setNewPartTarget(undefined); newPartForm.resetFields(); message.success("零件已创建并插入 Product"); },
    });
  };
  const deleteTreeNodes = (nodes: SpecificationTreeNode[]) => {
    if (!editingView || !canEdit || command.isPending) return;
    const candidates = nodes.filter((node) => node.entityId && node.kind && node.capabilities?.includes("DELETE"));
    const selectedFeatures = new Set(candidates.filter((node) => !["SKETCH_ENTITY", "SKETCH_CONSTRAINT", "ASSEMBLY_CONSTRAINT", "INSTANCE"].includes(node.kind!))
      .map((node) => node.entityId));
    const featureOrder = new Map((editingView.part?.features ?? []).map((feature, index) => [feature.id, index]));
    const targets = candidates.filter((node) => !node.ownerEntityId || !selectedFeatures.has(node.ownerEntityId)).sort((left, right) => {
      const rank = (node: SpecificationTreeNode) => node.kind === "SKETCH_CONSTRAINT" ? 0 : node.kind === "SKETCH_ENTITY" ? 1 : 2;
      const difference = rank(left) - rank(right); if (difference) return difference;
      return (featureOrder.get(right.entityId!) ?? 0) - (featureOrder.get(left.entityId!) ?? 0);
    });
    if (!targets.length) return;
    command.mutate(() => api.deleteNodes(editingView.document.id, targets.map((node) => ({
      targetKind: ["SKETCH_ENTITY", "SKETCH_CONSTRAINT", "ASSEMBLY_CONSTRAINT", "INSTANCE"].includes(node.kind!) ? node.kind! : "FEATURE",
      targetId: node.entityId!, ownerEntityId: node.ownerEntityId,
    }))));
  };
  const createVersion = async (values: { name: string; description: string }) => {
    if (!editingView) return; await api.createVersion(editingView.document.id, values.name, values.description); setVersionOpen(false);
    await client.invalidateQueries({ queryKey: queryKeys.history(editingView.document.id) }); message.success("版本已创建");
  };
  const createRelease = async (name: string) => {
    const release: ProductRelease = await api.createProductRelease(documentID, name);
    await client.invalidateQueries({ queryKey: ["product-releases", documentID] });
    message.success(`产品版本 ${release.name} 已冻结`);
  };
  useEffect(() => {
    const disposers = [
      commandRegistry.register({ id: "tool.select", execute: () => store.setActiveTool("select", "once"),
        isActive: () => store.activeToolID === "select" }),
      commandRegistry.register({ id: "assembly.move", execute: () => store.setActiveTool("assembly.move", "continuous"),
        isVisible: () => editingView?.document.type === "PRODUCT", isEnabled: () => Boolean(canEdit), isActive: () => store.activeToolID === "assembly.move" }),
      commandRegistry.register({ id: "sketch.start", execute: startSketch,
		isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit && (["plane", "sketch", "face"].includes(store.selection?.kind ?? ""))) }),
      commandRegistry.register({ id: "view.normal", execute: normalToSelection,
        isEnabled: () => Boolean(store.selection && ["plane","face"].includes(store.selection.kind)) }),
      commandRegistry.register({ id: "sketch.normal", execute: () => viewport.current?.normalToSketch(),
        isVisible: () => Boolean(store.sketchPlane), isEnabled: () => Boolean(store.sketchPlane) }),
      commandRegistry.register({ id: "sketch.finish", execute: finishSketch,
        isVisible: () => Boolean(store.sketchPlane), isEnabled: () => Boolean(canEdit && !command.isPending) }),
      ...sketchToolCommands.map((toolID)=>commandRegistry.register({id:toolID,execute:(invocation)=>store.setActiveTool(toolID,invocation?.continuous?"continuous":"once"),
        isVisible:()=>Boolean(store.sketchPlane),isEnabled:()=>Boolean(canEdit&&store.sketchPlane),isActive:()=>store.activeToolID===toolID})),
      commandRegistry.register({ id: "part.pad", execute: () => openSolidFeature("LINEAR_EXTRUDE"), isVisible: () => editingView?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch") }),
      commandRegistry.register({ id: "part.pocket", execute: () => openSolidFeature("LINEAR_EXTRUDE", "REMOVE"), isVisible: () => editingView?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch" && editingView?.part?.features.some((feature) => isSolidFeature(feature))) }),
      commandRegistry.register({ id: "part.revolve", execute: () => openSolidFeature("REVOLVE"), isVisible: () => editingView?.document.type === "PART",
        isEnabled: () => Boolean(canEdit && store.selection?.kind === "sketch") }),
      commandRegistry.register({ id: "part.parameters", execute: () => setParameterManagerOpen(true),
        isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(editingView?.part) }),
      commandRegistry.register({ id: "part.publications", execute: () => setPublicationManagerOpen(true),
		isVisible: () => Boolean(editingView), isEnabled: () => Boolean(editingView?.part || editingView?.product) }),
      commandRegistry.register({ id: "product.publications", execute: () => setPublicationManagerOpen(true),
		isVisible: () => editingView?.document.type === "PRODUCT", isEnabled: () => Boolean(editingView?.product) }),
      commandRegistry.register({ id: "part.datum-plane", execute: () => { datumPlaneForm.setFieldsValue({ name: "Plane", offset: 10 }); setDatumPlaneOpen(true); },
        isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit && store.selection?.kind === "plane") }),
      commandRegistry.register({ id: "part.datum-axis", execute: () => { datumAxisForm.setFieldsValue({ name: "Axis", ox: 0, oy: 0, oz: 0, dx: 0, dy: 0, dz: 1 }); setDatumAxisOpen(true); },
        isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "product.insert", execute: () => setInsertOpen(true), isVisible: () => editingView?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "product.release", execute: () => setReleaseOpen(true), isVisible: () => view?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEditRoot) }),
      ...(["fix", "rigid", "coincident", "concentric", "angle", "parallel", "perpendicular", "distance"] as const).map((constraint) => commandRegistry.register({
        id: `assembly.${constraint}`,
        execute: (invocation) => store.setActiveTool(`assembly.${constraint}`, invocation?.continuous ? "continuous" : "once"),
        isVisible: () => editingView?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEdit),
        isActive: () => store.activeToolID === `assembly.${constraint}`,
      })),
      commandRegistry.register({ id: "history.version", execute: () => setVersionOpen(true), isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "document.share", execute: () => editingView && setShareResource({ type: "documents", id: editingView.document.id, name: editingView.document.name }),
        isEnabled: () => editingView?.document.permission === "OWNER" }),
      commandRegistry.register({ id: "edit.undo", execute: () => executeHistory("undo"),
        isEnabled: () => Boolean(canEdit && editingView?.document.canUndo && !command.isPending) }),
      commandRegistry.register({ id: "edit.redo", execute: () => executeHistory("redo"),
        isEnabled: () => Boolean(canEdit && editingView?.document.canRedo && !command.isPending) }),
      commandRegistry.register({ id: "view.fit", execute: () => viewport.current?.fit() }),
      commandRegistry.register({ id: "view.top", execute: () => viewport.current?.setStandardView("TOP") }),
      commandRegistry.register({ id: "view.front", execute: () => viewport.current?.setStandardView("FRONT") }),
      commandRegistry.register({ id: "view.right", execute: () => viewport.current?.setStandardView("RIGHT") }),
      commandRegistry.register({ id: "view.iso", execute: () => viewport.current?.setStandardView("ISO") }),
	  commandRegistry.register({ id: "debug.download", execute: () => editingView && (editingView.product ? api.downloadAssemblyReplay(editingView.document.id) : api.downloadDiagnosticBundle(editingView.document.id)),
		isVisible: () => !isMockMode, isEnabled: () => Boolean(editingView) }),
    ];
    return () => { for (const dispose of disposers.reverse()) dispose(); };
  }, [commandRegistry, editingView, view, canEdit, canEditRoot, store.selection, store.sketchPlane, store.activeToolID, lengthUnit,
    command.isPending, assemblyConstraintForm]);

  useEffect(() => { commandRegistry.notifyStateChanged(); }, [commandRegistry, editingView, store.selection, store.sketchPlane,
    store.activeToolID, command.isPending]);

  const selected = selectedFeature(editingView ?? {} as DocumentView, store.selection);
  const publicationTarget = () => {
    const selection = store.selection;
    if (!selection || !editingView?.part) return undefined;
    if (selection.kind === "plane" && selection.entityId) return { publicationType: "PLANE", targetKind: "PLANE", targetId: selection.entityId };
    if (selection.kind === "axis-system" && selection.entityId) return { publicationType: "FRAME", targetKind: "AXIS_SYSTEM", targetId: selection.entityId };
    if (selection.kind === "axis" && selection.entityId) return { publicationType: "AXIS", targetKind: selection.axis === "DATUM" ? "DATUM_AXIS" : "AXIS",
      targetId: selection.entityId, axis: selection.axis === "DATUM" ? undefined : selection.axis };
    if ((selection.kind === "face" || selection.kind === "edge") && selection.geometryKey) return {
      publicationType: selection.kind === "face" ? "SURFACE" : "CURVE", targetKind: selection.kind.toUpperCase(),
      geometryKey: selection.geometryKey, topologyId: selection.topologyId, versionId: selection.versionId ?? editingView.document.versionId,
    };
    if (selection.kind === "body") {
      const feature = [...editingView.part.features].reverse().find(isSolidFeature);
      if (feature) return { publicationType: "BODY", targetKind: "BODY", targetId: feature.id };
    }
    return undefined;
  };
  const createSelectedPublication = async () => {
    if (!editingView) return;
    const target = publicationTarget();
    if (!target) { message.warning("请先选择基准、面、边或 PartBody"); return; }
    const values = await publicationForm.validateFields();
    command.mutate(() => api.createPublication(editingView.document.id, { ...target, ...values }),
      { onSuccess: () => publicationForm.resetFields() });
  };
  const forwardSelectedPublication = async () => {
    if (!editingView?.product || !store.selection?.publication || !store.selection.instanceId) {
      message.warning("请在 Product 的引用树中选择一个子 Part Publication"); return;
    }
    const values = await publicationForm.validateFields();
    command.mutate(() => api.createProductPublication(editingView.document.id, store.selection!.instanceId!,
      store.selection!.publicationId!, values.name, values.semanticPurpose, store.selection!.instancePath));
  };
  const createContextBinding = async () => {
    if (!editingView?.part || !activeResolvedInstance?.instancePath || view?.document.type !== "PRODUCT") {
      message.warning("请先在 Product 中激活要编辑的 Part occurrence"); return;
    }
    const values = await contextReferenceForm.validateFields();
    const entry = contextCatalog.data?.publications.find((item) =>
      `${item.instancePath.canonical}::${item.publication.id}` === values.catalogKey);
    if (!entry) { message.warning("请选择当前 Product 上下文内的 Publication"); return; }
    const targetKind = values.publicationType === "PARAMETER" ? "PARAMETER"
      : values.publicationType === "CURVE" ? "SKETCH_EXTERNAL_GEOMETRY"
      : ["PLANE", "AXIS"].includes(values.publicationType) ? "DATUM" : "FEATURE_INPUT";
    command.mutate(() => api.createProductContextBinding(documentID, { name: values.name,
      contextInputName: `${values.name}Input`, owningInstancePath: activeResolvedInstance.instancePath!,
      sourceInstancePath: entry.instancePath, publicationId: entry.publication.id,
      publicationType: entry.publication.type, targetKind, targetId: values.targetId,
      referenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT" }), { onSuccess: () => {
        contextReferenceForm.resetFields();
        void client.invalidateQueries({ queryKey: queryKeys.document(editingView.document.id) });
      } });
  };
  const bindExternalParameter = async () => {
    if (!editingView?.part || !externalParameterID || !activeResolvedInstance?.instancePath || view?.document.type !== "PRODUCT") {
      message.warning("跨文档参数只能在 Product 上下文中的激活 Part 内绑定"); return;
    }
    const values = await externalParameterForm.validateFields();
    const entry = parameterContextCatalog.data?.publications.find((item) =>
      `${item.instancePath.canonical}::${item.publication.id}` === values.catalogKey);
    if (!entry) { message.warning("请选择当前 Product 上下文内的参数 Publication"); return; }
    const parameter = editingView.part.parameters?.find((item) => item.parameterId === externalParameterID);
    command.mutate(() => api.createProductContextBinding(documentID, { name: `${parameter?.key ?? "Parameter"}Binding`,
      contextInputName: `${parameter?.key ?? "Parameter"}Input`, owningInstancePath: activeResolvedInstance.instancePath!,
      sourceInstancePath: entry.instancePath, publicationId: entry.publication.id, publicationType: "PARAMETER",
      targetKind: "PARAMETER", targetId: externalParameterID, referenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT" }),
    { onSuccess: () => { setExternalParameterID(undefined); externalParameterForm.resetFields();
      void client.invalidateQueries({ queryKey: queryKeys.document(editingView.document.id) }); } });
  };
  const replaceSelectedInstance = async () => {
    if (!editingView?.product || store.selection?.kind !== "instance" || !store.selection.instanceId) {
      message.warning("请先选择要替换的 Instance"); return;
    }
    const values=await replacementForm.validateFields();
    command.mutate(()=>api.replaceInstance(editingView.document.id,store.selection!.instanceId!,values.referencedDocumentId));
  };
  const publishParameter = (parameter: ParameterDefinition) => {
    if (!editingView) return;
    command.mutate(() => api.createPublication(editingView.document.id, { name: parameter.key,
      semanticPurpose: parameter.label, publicationType: "PARAMETER", targetKind: "PARAMETER", targetId: parameter.parameterId }));
  };
  const redirectPublicationToSelection = (publicationId: string, publicationType: string) => {
    if (!editingView) return;
    const target = publicationTarget();
    if (!target || target.publicationType !== publicationType) {
      message.warning(`请选择与 ${publicationType} 合同兼容的目标`); return;
    }
    command.mutate(() => api.redirectPublication(editingView.document.id, publicationId, target));
  };
  const openParameterEditor = (parameterID: string) => {
	const parameter = editingView?.part?.parameters?.find((candidate) => candidate.parameterId === parameterID);
	if (!parameter) return;
	parameterForm.setFieldsValue({key:parameter.key,source:parameterSourceText(parameter, isLengthParameter(parameter) ? lengthUnit : parameter.displayUnit)}); setEditingParameterID(parameterID);
  };
  const commitParameterEdit = async () => {
	if (!editingView || !editingParameterID) return;
	const values = await parameterForm.validateFields();
	const current = editingView.part?.parameters?.find((candidate) => candidate.parameterId === editingParameterID);
	command.mutate(async () => {
		let updated = editingView;
		if (current && current.key !== values.key) updated = await api.renameParameter(editingView.document.id, editingParameterID, values.key);
		const source = parseParameterSource(values.source, current && isLengthParameter(current) ? lengthUnit : current?.displayUnit);
		updated = source.kind === "LITERAL"
			? await api.setParameterValue(editingView.document.id, editingParameterID, source.value, source.unit)
			: await api.setParameterExpression(editingView.document.id, editingParameterID, source.expression);
		return updated;
	}, {onSuccess:()=>setEditingParameterID(undefined)});
  };
  if (document.isLoading) return <div className="workbench-loading"><Spin size="large" /></div>;
  if (!view) return <Empty description="无法打开文档" />;

  const assemblyPreviewPending = assemblyPreviewSnapshot.matches("pending");
  const assemblyPreviewFailed = assemblyPreviewSnapshot.matches("failed");
  const motionComponents = assemblyPreviewSnapshot.matches("succeeded") ? assemblyPreviewSnapshot.context.components : undefined;
  const freedomNames = ["固定", "转动", "滑动", "圆柱", "平面", "球面", "自由", "耦合"];
  const assemblyPreviewEvidence = assemblyPreviewFailed ? <Alert type="error" showIcon
    message="约束预览求解失败"
    description={`${assemblyPreviewSnapshot.context.errorCode ? `[${assemblyPreviewSnapshot.context.errorCode}${assemblyPreviewSnapshot.context.phase ? ` @ ${assemblyPreviewSnapshot.context.phase}` : ""}] ` : ""}${assemblyPreviewSnapshot.context.error ?? "无法生成约束预览"}`}
  /> : motionComponents?.length ? <div aria-label="装配求解结果">
    {motionComponents.map(component => <div key={component.componentId}>
      <Typography.Text type="secondary">剩余相对自由度：{component.relativeDof}；整体自由度：{component.gaugeDof}</Typography.Text>
      {component.preference.bodies.filter(body => body.role === 1).map(body => <div key={body.bodyId}>
        第二元素变化：{body.translation.toPrecision(3)} mm / {(body.rotation * 180 / Math.PI).toPrecision(3)}°
      </div>)}
      {component.freedoms.filter(freedom => !freedom.relativeToBodyId || freedom.bodyId !== freedom.relativeToBodyId).map(freedom => <div key={freedom.bodyId}>
        {editingView?.product?.instances.find(instance => instance.id === freedom.bodyId)?.name ?? freedom.bodyId}：
        {freedomNames[freedom.kind]}（{freedom.translationDof} 平移 / {freedom.rotationDof} 转动）
        {freedom.relativeToBodyId && `，相对 ${editingView?.product?.instances.find(instance => instance.id === freedom.relativeToBodyId)?.name ?? freedom.relativeToBodyId}`}
      </div>)}
    </div>)}
  </div> : undefined;
  const assemblyPreviewFeedback = <>
    {assemblyPreviewEvidence}
    {(assemblyPreviewFailed || motionComponents?.length) ? <Button type="link" size="small"
      onClick={() => { if (editingView) void api.downloadAssemblyReplay(editingView.document.id).catch(error => message.error(String(error))); }}>
      下载 3dreplay
    </Button> : null}
  </>;


  const visibleToolbars = contextualToolbars(toolbarCatalog.data?.toolbars ?? [], activeWorkbench);
  const activeToolName = visibleToolbars.flatMap((toolbar) => toolbar.items)
    .find((item) => item.commandId === store.activeToolID)?.name ?? "选择";

  return <CommandProvider registry={commandRegistry}><section className="cad-workbench">
    <WorkbenchLayout documentName={editingView?.document.name ?? view.document.name}
      inspectorOpen={inspectorOpen} onInspectorChange={setInspectorOpen}
      commands={<WorkbenchCommands key={activeWorkbench} toolbars={visibleToolbars} workbench={activeWorkbench} />}
      status={<WorkbenchStatus busy={command.isPending} canEdit={canEdit} selectionCount={store.selections.length}
        toolName={activeToolName} lengthUnit={lengthUnit} continuous={store.activeToolMode === "continuous"} />}
      tree={<SpecificationTree key={documentID} nodes={treeNodes} selectedKeys={treeKeysForSelections(treeNodes, store.selections)}
            selectedIdentityKeys={store.selections.map(selectionKey)}
            selectionToken={selectionSetToken(store.selections)}
            highlightedKey={treeKeyForSelection(treeNodes, store.preselection)}
            activeDocumentId={activeID}
            activeInstancePath={activeInstancePath}
            onSelect={(nodes) => store.setSelections(nodes.flatMap((node) => node.selection ? [node.selection] : []))}
            onOpenDocumentTab={(node) => {
              if (node.kind === "INSTANCE" && node.documentId) void openDocumentTab(node.documentId, client, api.getDocument, navigate)
                .catch((error: Error) => message.error(`打开文档失败：${error.message}`));
            }}
            onActivate={(node) => {
              if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) {
                const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
                if (constraint) openAssemblyConstraintEditor(constraint);
                return;
              }
			  if (node.capabilities?.includes("EDIT")) { openFeatureEditor(node); return; }
              if (node.documentId && ["PART", "PRODUCT", "INSTANCE"].includes(node.kind ?? "")) {
                setActiveDocumentID(node.documentId); setActiveInstancePath(node.instancePath?.canonical);
                setDefinitionContextPath(undefined);
                store.endSketch(); store.setSelection(null); return;
              }
              if (!canEdit || !node.selection || !editingView) return;
              if (node.selection.kind === "sketch") {
                const feature=editingView.part?.features.find((candidate)=>candidate.id===node.selection!.id);
                const localPlane=feature?featureSketchPlane(editingView,feature):undefined;
                const plane=localPlane?occurrenceSketchPlane(localPlane,activeResolvedInstance?.translation,activeResolvedInstance?.rotation):undefined;
                if(feature&&plane)store.beginSketch(feature.id,plane);
              } else if (node.selection.kind === "sketch-constraint") viewport.current?.editDimension(node.selection);
            }}
			onEdit={(node) => {
              if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) {
                const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
                if (constraint) openAssemblyConstraintEditor(constraint);
              } else openFeatureEditor(node);
            }}
			onCreatePart={(node) => {
			  if (node.documentType !== "PRODUCT") return;
			  newPartForm.resetFields(); setNewPartTarget(node);
			}}
			onReferenceMode={(node, mode) => {
			  const segment = node.instancePath?.segments.at(-1);
			  if (!segment) return;
			  command.mutate(() => api.setReferenceMode(segment.ownerDocumentId, segment.instanceId, mode), {
			    onSuccess: async (updated) => {
			      client.setQueryData(queryKeys.document(updated.document.id), updated);
			      await client.invalidateQueries({ queryKey: queryKeys.document(documentID) });
			      message.success(mode === "PINNED" ? "已固定当前引用版本" : "已恢复跟随最新版本");
			    },
			  });
			}}
			onReconnect={(node) => {
              if (node.kind === "SKETCH_EXTERNAL_GEOMETRY" && node.entityId && node.ownerEntityId && editingView) {
                const feature = editingView.part?.features.find((candidate) => candidate.id === node.ownerEntityId);
                const localPlane = feature ? featureSketchPlane(editingView, feature) : undefined;
                const plane = localPlane ? occurrenceSketchPlane(localPlane, activeResolvedInstance?.translation, activeResolvedInstance?.rotation) : undefined;
                if (plane) store.beginSketch(node.ownerEntityId, plane);
                store.setActiveTool("sketch.project", "once");
                viewport.current?.beginExternalReconnect(node.entityId);
                return;
              }
              const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
              if (constraint) openAssemblyConstraintEditor(constraint, true);
            }}
			onDetach={(node) => {
              if (node.kind === "SKETCH_EXTERNAL_GEOMETRY" && node.ownerEntityId && node.entityId)
                editSketch(node.ownerEntityId, [{type:"DETACH_EXTERNAL_GEOMETRY", externalId:node.entityId}]);
            }}
            onRefresh={(node) => refreshAssemblyConstraint(node.entityId)}
            onHover={(node) => store.setPreselection(node?.selection ?? null)} onDelete={deleteTreeNodes}
            onToggleVisibility={(node)=>{
              if (!node.hidden && node.kind === "SKETCH" && node.entityId === store.activeSketchID) store.endSketch();
              setTreeVisibility(node.key, Boolean(node.hidden));
            }}
            onToggleSuppression={(node)=>{
              if(node.kind==="ASSEMBLY_CONSTRAINT"||node.kind==="ASSEMBLY_CONSTRAINT_SET") {
                const constraints=node.kind==="ASSEMBLY_CONSTRAINT"?[node]:node.children??[];
                if(!node.documentId)return;
                assemblyPreviewActor.current?.send({type:"CANCEL",sequence:assemblyPreviewSequence.current});
                assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined;
                command.mutate(()=>api.command(node.documentId!,{type:"SET_ASSEMBLY_CONSTRAINT_STATE",constraintIds:constraints.flatMap(c=>c.entityId?[c.entityId]:[]),suppressed:!constraints.every(c=>c.suppressed)}));
                return;
              }
              const leaves:SpecificationTreeNode[]=[];const visit=(item:SpecificationTreeNode)=>{if(item.kind==="SKETCH_ENTITY"||item.kind==="SKETCH_CONSTRAINT")leaves.push(item);else item.children?.forEach(visit);};visit(node);
              const targetState=!leaves.every((item)=>item.suppressed);const bySketch=new Map<string,SketchOperation[]>();
              for(const item of leaves){if(!item.ownerEntityId||!item.entityId)continue;const operations=bySketch.get(item.ownerEntityId)??[];
                operations.push(item.kind==="SKETCH_ENTITY"?{type:"UPDATE_ENTITY_SUPPRESSION",entityId:item.entityId,suppressed:targetState}
                  :{type:"UPDATE_CONSTRAINT_SUPPRESSION",constraintId:item.entityId,suppressed:targetState});bySketch.set(item.ownerEntityId,operations);}
              for(const [sketchID,operations] of bySketch)editSketch(sketchID,operations);
            }}
            onToggleConstruction={(node)=>{
              if(!node.ownerEntityId||!node.entityId)return;
              editSketch(node.ownerEntityId,[{type:"UPDATE_ENTITY_ROLE",entityId:node.entityId,
                role:node.role==="CONSTRUCTION"?"PROFILE":"CONSTRUCTION"}]);
            }} />}
      inspector={<WorkbenchInspectorPanel documentID={activeID} view={editingView ?? view} selection={store.selection}
        feature={selected} workbench={activeWorkbench} sketchPlane={store.sketchPlane} activeTool={store.activeToolID}
        navigationProfile={navigationProfile} onEditParameter={openParameterEditor} canRestore={canEdit && !command.isPending}
        onRestore={(entry) => command.mutate(() => api.restore(activeID, entry.versionId))} />}>
        {view.document.type === "PRODUCT" && (activeInstancePath || activeDocumentID !== documentID) && <div style={{position:"absolute",zIndex:12,top:12,left:"50%",transform:"translateX(-50%)",
          padding:"6px 10px",borderRadius:6,background:"rgba(22,27,34,.88)",color:"white"}}>
          <Space size="small"><Typography.Text style={{color:"white"}}>
            {activeInstancePath ? `上下文编辑 · ${activeResolvedInstance?.instancePath?.display ?? activeInstancePath}`
              : `定义编辑 · ${editingView?.document.name??activeDocumentID}`}
          </Typography.Text>
          {activeInstancePath && <Button size="small" onClick={() => { setDefinitionContextPath(activeInstancePath); setActiveInstancePath(undefined); store.endSketch(); store.setSelection(null); }}>
            打开定义</Button>}
          {!activeInstancePath&&definitionContextPath&&<Button size="small" onClick={()=>{setActiveInstancePath(definitionContextPath);setDefinitionContextPath(undefined);store.endSketch();store.setSelection(null);}}>在此上下文打开</Button>}
          {designSession.isError && <Tag color="error">上下文失效</Tag>}</Space>
        </div>}
        {view.document.type === "PRODUCT" && productUpdatePlan.data?.hasUpdates && !productUpdatePlan.data.canAccept && <Alert
          style={{position:"absolute",zIndex:12,top:56,left:"50%",transform:"translateX(-50%)",minWidth:420}}
          type="error" showIcon message="自动跟随最新版本被阻塞"
          description={productUpdatePlan.data.entries.find((entry)=>entry.kind !== "ASSEMBLY_SOLVE" && entry.diagnostic)?.diagnostic ?? "更新计划被上游解析或求值失败阻塞。"} />}
        <Suspense fallback={<div className="viewport-loading"><Spin size="large" /></div>}><CadViewport ref={viewport} view={view}
          editingView={editingView} activeInstancePath={activeInstancePath} activeInstanceTranslation={activeResolvedInstance?.translation}
          activeInstanceRotation={activeResolvedInstance?.rotation}
          activeBodyTreeNodeId={activeResolvedInstance?.bodyTreeNodeId}
          selections={store.selections}
          preselection={store.preselection}
          treeVisibilityOverrides={treeVisibilityOverrides}
          sketchPlane={store.sketchPlane} activeSketchID={store.activeSketchID} activeToolID={store.activeToolID} navigationProfile={navigationProfile} catiaRotationSphereVisible={catiaRotationSphereVisible}
          captureSettings={captureSettings} onSelectionsChange={store.setSelections} onPreselectionChange={store.setPreselection} onSketchOperations={editSketch}
          onToolUseComplete={store.completeToolUse} onActiveToolChange={store.setActiveTool}
		  onAssemblyConstraint={(toolKind, references) => {
            const { kind, angleRelation } = assemblyConstraintEntry(toolKind);
			if (!editingView) return;
			assemblyInteractionID.current=randomUUID();
			assemblyPreviewActor.current?.send({type:"START"});
            setAssemblyDefinitionDirty(true); setReconnectError(undefined); setReplacingAssemblyReference(undefined);
            const measuredValue = kind === "angle" || kind === "distance" ? viewport.current?.measureAssemblyConstraint(kind, references) ?? 0 : 0;
            const value = kind === "distance" ? millimetersToDisplayLength(measuredValue, lengthUnit) : measuredValue;
            const planePair=references.length===2&&references.every((reference)=>["PLANE","FACE"].includes(reference.kind));
            assemblyConstraintForm.setFieldsValue({ value, directionRelation: kind === "angle" ? "SAME" : planePair
              ? (viewport.current?.measureAssemblyConstraint("angle",references)??0)>90?"OPPOSITE":"SAME" : "UNORIENTED", distanceRelation: "UNSIGNED" });
            setPendingAssemblyConstraint({ kind, references,
              angleRelation });
          }}
		  onInstanceMovePreview={async(instanceId,translation,rotation,interactionId,previewSequence)=>{
			if(editingView?.document.type!=="PRODUCT")return{poses:[],constraintLimited:true,previewId:""};const preview=await api.previewCommand(editingView.document.id,{type:"MOVE_INSTANCE",interactionId,previewSequence,instanceId,translation,rotation});return{poses:preview.instancePoses??[],constraintLimited:Boolean(preview.constraintLimited),previewId:preview.previewId};
          }}
          onInstanceMoved={moveInstance} /></Suspense>
      <WorkbenchViewControls toolbars={visibleToolbars} />
    </WorkbenchLayout>
    <CommandDialog id="assembly-constraint-edit" open={Boolean(editingAssemblyConstraint)} title="约束定义" width={390}
      onClose={() => { assemblyPreviewActor.current?.send({type:"CANCEL",sequence:assemblyPreviewSequence.current});assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined; setAssemblyPreviewEvaluation(undefined); setReplacingAssemblyReference(undefined); setReconnectError(undefined); setAssemblyDefinitionDirty(false); setEditingAssemblyConstraint(undefined); }}
      confirmLoading={command.isPending || assemblyPreviewPending} confirmDisabled={assemblyPreviewFailed || replacingAssemblyReference !== undefined ||
        Boolean(editingAssemblyConstraint?.kind === "ANGLE" && editingAssemblyConstraint.angleRelation === "DIRECTED" && !editingAssemblyConstraint.angleAxis)} onConfirm={async()=>{
        if(!editingView||!editingAssemblyConstraint)return;const values=await assemblyConstraintForm.validateFields();const constraint=editingAssemblyConstraint;
		assemblyPreviewActor.current?.send({type:"CONFIRM"});
        command.mutate(()=>api.editAssemblyConstraint(editingView.document.id,constraint.id,{value:constraint.kind==="ANGLE"?values.value*Math.PI/180
          :constraint.kind==="DISTANCE"?displayLengthToMillimeters(values.value,lengthUnit):values.value,
		  directionRelation:values.directionRelation,distanceRelation:values.distanceRelation,
          fixedPose: constraint.kind === "FIX" ? fixedPoseFromParameters(
            (values.fixedTranslation as Vec3).map(v=>displayLengthToMillimeters(v,lengthUnit)) as Vec3,values.fixedAngles) : undefined,
		  firstAssemblyRef:constraint.first,secondAssemblyRef:constraint.second,angleAxis:constraint.angleAxis,reverseAngleAxis:constraint.reverseAngleAxis,angleReferenceDirection:constraint.angleReferenceDirection,angleRelation:constraint.angleRelation,fixMode:constraint.fixMode,
		  previewId:assemblyPreviewID.current}),{onSuccess:(updated)=>{assemblyPreviewActor.current?.send({type:"COMMIT_SUCCESS"});viewport.current?.clearCommandPreview(false);assemblyPreviewID.current=undefined;setAssemblyDefinitionDirty(false);setReconnectError(undefined);setEditingAssemblyConstraint(undefined);store.setSelection({kind:"assembly-constraint",id:constraint.id,constraintId:constraint.id,constraintType:constraint.kind,documentId:updated.document.id,treeNodeId:`document:${updated.document.id}/assembly-constraints/constraint:${constraint.id}`});},
		  onError:(cause)=>assemblyPreviewActor.current?.send({type:"COMMIT_FAILURE",error:String(cause)})});
      }}>
      <Form form={assemblyConstraintForm} layout="vertical">
        {editingAssemblyConstraint?.kind === "FIX" && <Select aria-label="固定基准" value={editingAssemblyConstraint.fixMode ?? "SPACE"}
          options={[{value:"SPACE",label:"空间固定"},{value:"RELATIVE",label:"相对固定"}]}
          onChange={(fixMode)=>{setEditingAssemblyConstraint({...editingAssemblyConstraint,fixMode});setAssemblyDefinitionDirty(true);assemblyPreviewActor.current?.send({type:"CHANGE"});setAssemblyPreviewCommit(v=>v+1);}} />}
        {editingAssemblyConstraint?.kind === "ANGLE" && <AssemblyAngleParameters value={editingAssemblyConstraint} view={editingView}
          onChange={value=>{if(value.angleRelation === "PERPENDICULAR") assemblyConstraintForm.setFieldValue("directionRelation", assemblyConstraintForm.getFieldValue("directionRelation") === "OPPOSITE" ? "OPPOSITE" : "SAME");setEditingAssemblyConstraint({...editingAssemblyConstraint,...value});setAssemblyDefinitionDirty(true);assemblyPreviewActor.current?.send({type:"CHANGE"});}}
          onPick={()=>{store.setSelection(null);setReplacingAssemblyReference(2);store.setActiveTool("select","once");}} />}
        {editingAssemblyConstraint && <Space>
          <Button disabled={command.isPending} onClick={() => {
            if (!editingView) return;
            const constraint = editingAssemblyConstraint;
            assemblyPreviewActor.current?.send({type:"CANCEL", sequence:assemblyPreviewSequence.current});
            assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined;
            command.mutate(() => api.command(editingView.document.id, {type:"SET_ASSEMBLY_CONSTRAINT_STATE",
              constraintIds:[constraint.id], suppressed:!constraint.suppressed}), {onSuccess:()=>setEditingAssemblyConstraint(undefined)});
          }}>{editingAssemblyConstraint.suppressed ? "激活约束" : "停用约束"}</Button>
          {["ANGLE","DISTANCE"].includes(editingAssemblyConstraint.kind) && (!editingAssemblyConstraint.angleRelation || editingAssemblyConstraint.angleRelation === "DIRECTED" || editingAssemblyConstraint.angleRelation === "FREE") && <Button disabled={command.isPending} onClick={() => {
            if (!editingView) return;
            const constraint = editingAssemblyConstraint;
            assemblyPreviewActor.current?.send({type:"CANCEL", sequence:assemblyPreviewSequence.current});
            assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined;
            command.mutate(() => api.command(editingView.document.id, {type:"SET_ASSEMBLY_CONSTRAINT_STATE",
              constraintIds:[constraint.id], constraintMode:constraint.mode === "MEASURED" ? "DRIVING" : "MEASURED"}),
              {onSuccess:()=>setEditingAssemblyConstraint(undefined)});
          }}>{editingAssemblyConstraint.mode === "MEASURED" ? "改为驱动" : "改为测量"}</Button>}
        </Space>}

        {editingAssemblyConstraint && <AssemblyConstraintFields kind={editingAssemblyConstraint.kind} view={editingView} lengthUnit={lengthUnit}
          references={[editingAssemblyConstraint.first, editingAssemblyConstraint.second]} replacing={replacingAssemblyReference}
          constraint={editingAssemblyConstraint} previewEvaluation={assemblyPreviewEvaluation} dirty={assemblyDefinitionDirty}

		  onValueCommit={()=>{setAssemblyDefinitionDirty(true);assemblyPreviewActor.current?.send({type:"CHANGE"});setAssemblyPreviewCommit((value)=>value+1);}}
          onLocate={(reference)=>{if(!viewport.current?.focusAssemblyReference(reference))setReconnectError("当前支持元素无法在视图区定位。");}}
		  onRefresh={()=>refreshAssemblyConstraint(editingAssemblyConstraint.id)}
		  onReplace={(index)=>{store.setSelection(null);setReconnectError(undefined);setReplacingAssemblyReference(index);store.setActiveTool("select","once");}} />}
        {replacingAssemblyReference !== undefined && <Alert type="info" showIcon message={`Reconnect 支持元素 ${replacingAssemblyReference + 1}`}
          description="在视图区选择新的几何元素；Esc 取消整个编辑会话。" />}
        {reconnectError && <Alert type="error" showIcon message="无法使用该支持元素" description={reconnectError} />}
        {assemblyPreviewFeedback}
      </Form>
    </CommandDialog>
    <CommandDialog id="assembly-constraint-value" open={Boolean(pendingAssemblyConstraint)} title="约束定义" width={390}
      onClose={() => { assemblyPreviewActor.current?.send({type:"CANCEL",sequence:assemblyPreviewSequence.current});assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined; setAssemblyPreviewEvaluation(undefined); setReplacingAssemblyReference(undefined); setReconnectError(undefined); setAssemblyDefinitionDirty(false); setPendingAssemblyConstraint(undefined); }}
      confirmLoading={command.isPending || assemblyPreviewPending} confirmDisabled={assemblyPreviewFailed || replacingAssemblyReference !== undefined ||
        Boolean(pendingAssemblyConstraint?.kind === "angle" && pendingAssemblyConstraint.angleRelation === "DIRECTED" && !pendingAssemblyConstraint.angleAxis)}
      onConfirm={async () => {
        if (!editingView || !pendingAssemblyConstraint) return;
        const values = await assemblyConstraintForm.validateFields();
        const pending = pendingAssemblyConstraint;
		assemblyPreviewActor.current?.send({type:"CONFIRM"});
        command.mutate(() => api.addAssemblyConstraint(editingView.document.id, {
          constraintKind: pending.kind.toUpperCase(), firstAssemblyRef: pending.references[0], secondAssemblyRef: pending.references[1],
          value: pending.kind === "angle" ? values.value * Math.PI / 180
            : pending.kind === "distance" ? displayLengthToMillimeters(values.value, lengthUnit) : values.value,
          directionRelation: values.directionRelation, distanceRelation: values.distanceRelation,
		  angleRelation:pending.angleRelation,angleAxis:pending.angleAxis,reverseAngleAxis:pending.reverseAngleAxis,
		  previewId:assemblyPreviewID.current,
		}), { onSuccess: () => { assemblyPreviewActor.current?.send({type:"COMMIT_SUCCESS"});viewport.current?.clearCommandPreview(false); assemblyPreviewID.current=undefined; setReconnectError(undefined); setAssemblyDefinitionDirty(false); setPendingAssemblyConstraint(undefined); },
		  onError:(cause)=>assemblyPreviewActor.current?.send({type:"COMMIT_FAILURE",error:String(cause)}) });
      }}>
      <Form form={assemblyConstraintForm} layout="vertical">
        {pendingAssemblyConstraint?.kind === "angle" && <AssemblyAngleParameters value={pendingAssemblyConstraint} view={editingView}
          onChange={value=>{if(value.angleRelation === "PERPENDICULAR") assemblyConstraintForm.setFieldValue("directionRelation", assemblyConstraintForm.getFieldValue("directionRelation") === "OPPOSITE" ? "OPPOSITE" : "SAME");setPendingAssemblyConstraint({...pendingAssemblyConstraint,...value});setAssemblyDefinitionDirty(true);assemblyPreviewActor.current?.send({type:"CHANGE"});}}
          onPick={()=>{store.setSelection(null);setReplacingAssemblyReference(2);store.setActiveTool("select","once");}} />}
        {pendingAssemblyConstraint && <AssemblyConstraintFields kind={pendingAssemblyConstraint.kind.toUpperCase() as keyof typeof assemblyConstraintUI} view={editingView} lengthUnit={lengthUnit}
          references={pendingAssemblyConstraint.references} replacing={replacingAssemblyReference}
          previewEvaluation={assemblyPreviewEvaluation} dirty
          angleRelation={pendingAssemblyConstraint.angleRelation}
		  onValueCommit={()=>{assemblyPreviewActor.current?.send({type:"CHANGE"});setAssemblyPreviewCommit((value)=>value+1);}}
          onLocate={(reference)=>{if(!viewport.current?.focusAssemblyReference(reference))setReconnectError("当前支持元素无法在视图区定位。");}}
		  onReplace={(index)=>{store.setSelection(null);setReconnectError(undefined);setReplacingAssemblyReference(index);store.setActiveTool("select","once");}} />}
        {replacingAssemblyReference !== undefined && <Alert type="info" showIcon message={`重新选择支持元素 ${replacingAssemblyReference + 1}`}
          description="在视图区选择新的几何元素；Esc 取消整个创建会话。" />}
        {reconnectError && <Alert type="error" showIcon message="无法使用该支持元素" description={reconnectError} />}
        {assemblyPreviewFeedback}
      </Form>
    </CommandDialog>
    <CommandDialog id="solid-generator" open={padOpen} title="实体特征" onClose={closePad} confirmLoading={command.isPending}
      onConfirm={async () => padSketch(await padForm.validateFields())}>
      <Form form={padForm} layout="vertical"><Form.Item name="generator" hidden><Input /></Form.Item>
        <Form.Item name="operation" label="Body 操作" rules={[{ required: true }]}>
        <Select onChange={(operation) => {
          padForm.setFieldValue("reversed", defaultSolidReversed(padGenerator, operation));
          previewPad();
        }} options={[{ value: "NEW_BODY", label: "新建实体" }, { value: "ADD", label: "添加材料" },
          { value: "REMOVE", label: "移除材料" }, { value: "INTERSECT", label: "保留交集" }]} /></Form.Item>
        <Form.Item noStyle shouldUpdate={(before, after) => before.generator !== after.generator}>{({ getFieldValue }) => getFieldValue("generator") === "REVOLVE" ? <>
          <Form.Item name="axisEntityId" label="旋转轴" rules={[{ required: true }]}><Select onChange={previewPad}
            placeholder="在视图区或结构树选择直线/轴"
            options={[...(editingView?.part?.features ?? []).flatMap((feature) => (feature.sketch?.entities ?? [])
              .filter((entity) => entity.kind === "LINE")
              .map((entity, index) => ({ value: `SKETCH_LINE:${feature.id}:${entity.id}`, label: `${feature.name ?? feature.id} · 直线 ${index + 1}` }))),
              ...(editingView?.axisSystems ?? []).flatMap((axis) => (["X","Y","Z"] as const).map((direction) => ({
                value: `AXIS_SYSTEM:${axis.id}:${direction}`, label: `${axis.name} · ${direction}` }))),
              ...(editingView?.datumAxes ?? []).map((axis) => ({ value: `DATUM_AXIS:${axis.id}`, label: axis.name }))]} /></Form.Item>
          <Form.Item name="angle" label="旋转角度（deg）" rules={[{ required: true }, { type: "number", min: 0.1, max: 360 }]}>
            <InputNumber min={0.1} max={360} precision={2} style={{ width: "100%" }} onBlur={previewPad} onPressEnter={previewPad} /></Form.Item>
        </> : <>
          <Form.Item name="lengthSource" label={`拉伸长度（${lengthUnit}）`} rules={[{ required: true }, { validator: async (_, value) => {
            try { linearExtrudeLengthInput(String(value ?? ""), lengthUnit); } catch (cause) { throw cause; }
          } }]}>
            <Input data-quantity-input="true" placeholder="40 或参数表达式" onBlur={previewPad} onPressEnter={previewPad} />
          </Form.Item>
          <Form.Item label="引用已有长度参数">
            <Select allowClear showSearch optionFilterProp="label" placeholder="选择后绑定其稳定 ParameterId"
              options={(editingView?.part?.parameters ?? []).filter(isLengthParameter).map((parameter) => ({
                value: parameter.key, label: `${parameter.key} · ${parameterDisplayValue(parameter, lengthUnit)}`,
              }))}
              onChange={(key) => { if (key) padForm.setFieldValue("lengthSource", key); previewPad(); }} />
          </Form.Item>
        </>}</Form.Item>
        <Form.Item name="reversed" label="反向" valuePropName="checked"><Switch onChange={previewPad} /></Form.Item>
        <FeaturePreviewLegend operation={padOperation} />
        <small className="cad-command-hint">{padPreviewPending ? "后端正在求值预览…" : "输入后按 Enter 或点击视口可刷新后端瞬态预览；预览不会创建 Revision。"}</small></Form>
    </CommandDialog>
	<CommandDialog id="linear-extrude-edit" open={Boolean(editingExtrude)} title="编辑线性拉伸" onClose={closeFeatureEditor}
		confirmLoading={command.isPending || featurePreviewPending} confirmDisabled={Boolean(featurePreviewError)} onConfirm={commitFeatureEdit}>
		<Form form={featureForm} layout="vertical"><Form.Item name="lengthText" label={`拉伸长度（${lengthUnit}）`}
			rules={[{required:true},{validator:async(_,value)=>{try{linearExtrudeLengthInput(String(value??""), lengthUnit);}catch(cause){throw cause;}}}]}>
			<Input data-quantity-input="true" autoFocus placeholder="40 或参数表达式"
			onBlur={()=>void requestFeaturePreview()} onPressEnter={(event)=>{event.preventDefault();void commitFeatureEdit();}} /></Form.Item>
		<FeaturePreviewLegend operation={editingExtrude?.feature.operation ?? "NEW_BODY"} />
		{featurePreviewError&&<Alert type="error" showIcon message="编辑预览失败" description={featurePreviewError}/>}
		<small className="cad-command-hint">{featurePreviewPending?"后端正在求值预览…":"离开输入框刷新瞬态预览；按 Enter 或确定提交一个 Revision。"}</small></Form>
	</CommandDialog>
	<CommandDialog id="parameter-manager" open={parameterManagerOpen} title="参数" width={680}
		onClose={() => setParameterManagerOpen(false)} onConfirm={() => setParameterManagerOpen(false)} confirmText="完成">
		<div className="parameter-manager" aria-label="文档参数">
			<div className="parameter-manager-header"><span>别名 / 稳定身份</span><span>来源</span><span>计算值</span><span /></div>
			{(editingView?.part?.parameters ?? []).length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Part 尚无参数" />
				: (editingView?.part?.parameters ?? []).map((parameter: ParameterDefinition) => <div className="parameter-manager-row" key={parameter.parameterId}>
					<span><strong>{parameter.key}</strong><Typography.Text type="secondary" copyable={{text:parameter.parameterId}}>{parameter.parameterId}</Typography.Text></span>
					<Space direction="vertical" size={0}><Typography.Text ellipsis={{tooltip:parameterSourceText(parameter)}}>{parameterSourceText(parameter)}</Typography.Text>
						{editingView?.referenceUpdates?.find((item) => item.consumerKind === "EXTERNAL_PARAMETER" && item.consumerId === parameter.parameterId) && ((update) =>
							<Tag color={update.status === "CURRENT" ? "success" : update.status === "UPDATE_AVAILABLE" ? "processing" : "error"}
								title={update.diagnostic}>{update.status}</Tag>)(editingView.referenceUpdates.find((item) => item.consumerKind === "EXTERNAL_PARAMETER" && item.consumerId === parameter.parameterId)!)}</Space>
					<Typography.Text>{parameterDisplayValue(parameter, lengthUnit)}</Typography.Text>
					<Space><Button size="small" disabled={!canEdit} onClick={() => publishParameter(parameter)}>发布</Button>
					<Button size="small" disabled={!canEdit} onClick={() => { externalParameterForm.resetFields(); setExternalParameterID(parameter.parameterId); }}>引用</Button>
					<Button size="small" disabled={!canEdit} onClick={() => openParameterEditor(parameter.parameterId)}>编辑</Button></Space>
				</div>)}
			<small className="cad-command-hint">表达式使用可读别名输入，提交后绑定稳定 ParameterId；重命名别名不会断开已有引用。</small>
		</div>
	</CommandDialog>
	<CommandDialog id="publication-manager" open={publicationManagerOpen} title="Publications" width={760}
		onClose={() => setPublicationManagerOpen(false)} onConfirm={() => setPublicationManagerOpen(false)} confirmText="完成">
		<Form form={publicationForm} layout="inline" initialValues={{ name: "Published Element", semanticPurpose: "" }}>
			<Form.Item name="name" rules={[{ required: true }]}><Input placeholder="发布名称" /></Form.Item>
			<Form.Item name="semanticPurpose"><Input placeholder="语义用途" /></Form.Item>
			<Form.Item><Button type="primary" disabled={!canEdit || (editingView?.document.type === "PART" ? !publicationTarget() : !store.selection?.publicationId)} loading={command.isPending}
				onClick={() => void (editingView?.document.type === "PRODUCT" ? forwardSelectedPublication() : createSelectedPublication())}>
				{editingView?.document.type === "PRODUCT" ? "转发所选子 Publication" : "发布当前选择"}</Button></Form.Item>
		</Form>
		<div className="parameter-manager" aria-label="文档 Publications">
			{editingView?.document.type === "PRODUCT" ? ((editingView.product?.publications ?? []).length === 0
				? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Product 尚无转发 Publication" />
				: (editingView.product?.publications ?? []).map((publication) => <div className="parameter-manager-row" key={publication.id}>
					<span><Typography.Text strong editable={canEdit ? {onChange:(name)=>command.mutate(()=>api.editProductPublication(editingView.document.id,publication.id,name,publication.semanticPurpose))}:false}>{publication.name}</Typography.Text><Typography.Text type="secondary" copyable={{text:publication.id}}>{publication.id}</Typography.Text></span>
					<span>{publication.type} · {publication.target.instancePath.display}</span>
					<Tag color={publication.resolution.status === "CONNECTED" ? "success" : "error"}>{publication.resolution.status}</Tag>
					<Button danger size="small" disabled={!canEdit} onClick={() => command.mutate(() => api.deleteProductPublication(editingView.document.id, publication.id))}>删除</Button>
				</div>)) : (editingView?.part?.publications ?? []).length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Part 尚无 Publication" />
				: (editingView?.part?.publications ?? []).map((publication) => <div className="parameter-manager-row" key={publication.id}>
					<span><Typography.Text strong editable={canEdit ? {onChange:(name)=>command.mutate(()=>api.editPublication(editingView.document.id,publication.id,{name,semanticPurpose:publication.semanticPurpose}))}:false}>{publication.name}</Typography.Text><Typography.Text type="secondary" copyable={{text:publication.id}}>{publication.id}</Typography.Text></span>
					<span>{publication.type} · {publication.target.kind}</span>
					<Tag color={publication.resolution.status === "CONNECTED" ? "success" : "error"}>{publication.resolution.status}</Tag>
					<Space><Button size="small" disabled={!canEdit} onClick={() => redirectPublicationToSelection(publication.id, publication.type)}>重定向</Button>
					<Button danger size="small" disabled={!canEdit} onClick={() => editingView && command.mutate(() => api.deletePublication(editingView.document.id, publication.id))}>删除</Button></Space>
				</div>)}
			<small className="cad-command-hint">PublicationId 在兼容重定向时保持不变；断开的目标会以 BROKEN_PUBLICATION 保存在 Revision 中。</small>
		</div>
		{editingView?.part && <><Divider>Product Context Bindings</Divider>
			<Form form={contextReferenceForm} layout="vertical" initialValues={{name:"Context Binding",publicationType:"PLANE"}}>
				<Space align="start" wrap>
					<Form.Item name="name" label="名称" rules={[{required:true}]}><Input /></Form.Item>
					<Form.Item name="publicationType" label="合同类型" rules={[{required:true}]}><Select style={{width:130}}
						onChange={()=>{contextReferenceForm.setFieldsValue({catalogKey:undefined,targetId:undefined});}}
						options={["PLANE","AXIS","CURVE","SURFACE","BODY","PARAMETER"].map((value)=>({value,label:value}))}/></Form.Item>
					<Form.Item name="catalogKey" label="当前 Product 中的来源" rules={[{required:true}]}><Select style={{width:310}} showSearch optionFilterProp="label"
						loading={contextCatalog.isLoading} placeholder={activeInstancePath?"按 occurrence / 发布名称选择":"请先激活 Product 中的 Part"}
						options={(contextCatalog.data?.publications??[]).map((item)=>({value:`${item.instancePath.canonical}::${item.publication.id}`,
							label:`${item.displayPath} · ${item.publication.type}`,disabled:!item.selectable}))}/></Form.Item>
					<Form.Item name="targetId" label="本地目标" rules={[{required:true}]}><Select style={{width:220}} showSearch optionFilterProp="label" options={
						contextPublicationType==="PARAMETER"?(editingView.part.parameters??[]).map((item)=>({value:item.parameterId,label:item.key}))
						:contextPublicationType==="CURVE"?editingView.part.features.filter((item)=>item.sketch).map((item)=>({value:item.id,label:item.name??item.id}))
						:contextPublicationType==="PLANE"?(editingView.datumPlanes??editingView.part.datumPlanes).map((item)=>({value:item.id,label:item.name}))
						:contextPublicationType==="AXIS"?(editingView.datumAxes??[]).map((item)=>({value:item.id,label:item.name}))
						:editingView.part.features.map((item)=>({value:item.id,label:item.name??item.id}))}/></Form.Item>
					<Form.Item label=" "><Button disabled={!canEdit||!activeInstancePath} onClick={()=>void createContextBinding()}>创建绑定</Button></Form.Item>
				</Space>
			</Form>
			{(editingView.part.contextInputs??[]).map((input)=><div className="parameter-manager-row" key={input.id}>
				<span><Typography.Text strong editable={canEdit?{onChange:(name)=>command.mutate(()=>api.editContextInput(editingView.document.id,input.id,name,Boolean(input.required)))}:false}>{input.name}</Typography.Text>
				<Typography.Text type="secondary" copyable={{text:input.id}}>{input.id}</Typography.Text></span>
				<span>{input.type} · {input.target.kind}</span><Tag color={input.required?"blue":"default"}>{input.required?"REQUIRED":"OPTIONAL"}</Tag><span />
			</div>)}
			{(view.product?.contextBindings??[]).filter((binding)=>binding.owningInstancePath.canonical===activeInstancePath).map((binding)=><div className="parameter-manager-row" key={binding.id}>
				<span><strong>{binding.name}</strong><Typography.Text type="secondary">{binding.contextInputId}</Typography.Text></span>
				<span>{binding.sourceInstancePath.display} / {binding.publication.expectedType}</span>
				<Tag color={binding.resolution.status==="CONNECTED"?"success":"error"}>{binding.accepted.status}</Tag>
				<Typography.Text type="secondary">{binding.referenceMode}</Typography.Text>
			</div>)}
			{view.document.type!=="PRODUCT"&&<Alert type="info" showIcon message="跨文档关联从 Product 设计会话创建" description="请在 Product 中激活该 Part，再从当前产品上下文目录选择 Publication。" />}</>}
		{editingView?.product && <><Divider>兼容替换</Divider><Form form={replacementForm} layout="inline">
			<Form.Item name="referencedDocumentId" rules={[{required:true}]}><Select style={{width:240}} placeholder="选择替换 Part" showSearch optionFilterProp="label"
				options={(catalog.data?.documents??[]).filter((item)=>item.type==="PART").map((item)=>({value:item.id,label:item.name}))}/></Form.Item>
			<Form.Item><Button disabled={!canEdit||store.selection?.kind!=="instance"} onClick={()=>void replaceSelectedInstance()}>替换所选 Instance</Button></Form.Item>
			<small className="cad-command-hint">替换前按已使用 Publication contract 解析；兼容接口自动重连，不兼容接口保留为 Broken 供 Reconnect。</small>
		</Form><Divider>Instance Names</Divider>
			{editingView.product.instances.map((instance)=><div className="parameter-manager-row" key={instance.id}>
				<span><Typography.Text strong editable={canEdit?{onChange:(name)=>command.mutate(()=>api.renameInstance(editingView.document.id,instance.id,name))}:false}>{instance.name}</Typography.Text>
				<Typography.Text type="secondary" copyable={{text:instance.id}}>{instance.id}</Typography.Text></span>
				<span>{instance.documentId}</span><Tag>{instance.referenceMode??"FOLLOW_HEAD"}</Tag><span />
			</div>)}</>}
	</CommandDialog>
	<CommandDialog id="external-parameter" open={Boolean(externalParameterID)} title="引用外部参数 Publication"
		onClose={() => setExternalParameterID(undefined)} confirmLoading={command.isPending} onConfirm={async () => {
			await bindExternalParameter();
		}}><Form form={externalParameterForm} layout="vertical">
			<Form.Item name="catalogKey" label="当前 Product 中的参数 Publication" rules={[{required:true}]}><Select showSearch optionFilterProp="label"
				loading={parameterContextCatalog.isLoading} placeholder={activeInstancePath?"按 occurrence / 发布名称选择":"请先在 Product 中激活 Part"}
				options={(parameterContextCatalog.data?.publications??[]).map((item)=>({value:`${item.instancePath.canonical}::${item.publication.id}`,
					label:`${item.displayPath} · ${item.publication.name}`,disabled:!item.selectable}))} /></Form.Item>
			<small className="cad-command-hint">来源限定在当前 Product occurrence 图中；提交会原子创建 Part ContextInput、Product ContextBinding 与接受快照。</small>
		</Form>
	</CommandDialog>
	<CommandDialog id="parameter-edit" open={Boolean(editingParameterID)} title="编辑参数" onClose={() => setEditingParameterID(undefined)}
		confirmLoading={command.isPending} onConfirm={commitParameterEdit}>
		<Form form={parameterForm} layout="vertical">
			<Form.Item name="key" label="可读别名" rules={[{required:true,pattern:/^[A-Za-z_][A-Za-z0-9_]*$/,
				message:"请输入 ASCII 标识符"}]}><Input /></Form.Item>
			<Form.Item name="source" label="值或表达式" rules={[{required:true}]}><Input data-quantity-input="true" placeholder="40 或 base_width / 2" /></Form.Item>
			<small className="cad-command-hint">表达式按当前 Part 的参数别名编辑；提交后 AST 绑定稳定 ParameterId，后续重命名不会破坏引用。</small>
		</Form>
	</CommandDialog>
    {insertOpen && editingView?.document.type === "PRODUCT" && <InsertDocumentDialog key={activeID}
      targetID={activeID} rootID={documentID} busy={command.isPending} onClose={() => setInsertOpen(false)}
      onInsert={(referencedID) => command.mutateAsync(() => api.insert(activeID, referencedID))} />}
    <CommandDialog id="new-part-component" open={Boolean(newPartTarget)} title="新建零件"
      onClose={() => setNewPartTarget(undefined)} confirmLoading={command.isPending}
      onConfirm={async () => createPartComponent(await newPartForm.validateFields())}>
      <Form form={newPartForm} layout="vertical">
        <Form.Item name="name" label="零件名称" rules={[{ max: 120 }]}><Input placeholder="留空自动分配 Part1、Part2…" /></Form.Item>
        <Form.Item name="description" label="说明" rules={[{ max: 500 }]}><Input.TextArea rows={2} /></Form.Item>
        <small className="cad-command-hint">新 Part 与 occurrence 会原子创建；默认位于所选 Product 原点，实例名按“零件名.序号”分配。</small>
      </Form>
    </CommandDialog>
    <CommandDialog id="datum-plane" open={datumPlaneOpen} title="创建基准面" onClose={() => setDatumPlaneOpen(false)}
      confirmLoading={command.isPending} onConfirm={async () => {
        const values = await datumPlaneForm.validateFields(); const selectedPlane = store.selection?.kind === "plane" ? store.selection.datumPlane : undefined;
        if (!selectedPlane) return; const offset = displayLengthToMillimeters(values.offset, lengthUnit);
        const origin = selectedPlane.origin.map((value, index) => value + selectedPlane.normal[index] * offset) as Vec3;
        command.mutate(() => api.createDatumPlane(activeID, { name: values.name, origin, normal: selectedPlane.normal, uDirection: selectedPlane.uDirection }),
          { onSuccess: () => setDatumPlaneOpen(false) });
      }}><Form form={datumPlaneForm} layout="vertical"><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="offset" label={`偏置（${lengthUnit}）`} rules={[{ required: true }, { type: "number" }]}><InputNumber style={{ width: "100%" }} /></Form.Item></Form>
    </CommandDialog>
    <CommandDialog id="datum-axis" open={datumAxisOpen} title="创建基准轴" onClose={() => setDatumAxisOpen(false)}
      confirmLoading={command.isPending} onConfirm={async () => { const v = await datumAxisForm.validateFields();
        command.mutate(() => api.createDatumAxis(activeID, { name: v.name,
          origin: [displayLengthToMillimeters(v.ox,lengthUnit),displayLengthToMillimeters(v.oy,lengthUnit),displayLengthToMillimeters(v.oz,lengthUnit)],
          direction: [v.dx,v.dy,v.dz] }),
          { onSuccess: () => setDatumAxisOpen(false) }); }}><Form form={datumAxisForm} layout="vertical"><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Space><Form.Item name="ox" label={`原点 X（${lengthUnit}）`}><InputNumber /></Form.Item><Form.Item name="oy" label="Y"><InputNumber /></Form.Item><Form.Item name="oz" label="Z"><InputNumber /></Form.Item></Space>
        <Space><Form.Item name="dx" label="方向 X"><InputNumber /></Form.Item><Form.Item name="dy" label="Y"><InputNumber /></Form.Item><Form.Item name="dz" label="Z"><InputNumber /></Form.Item></Space></Form>
    </CommandDialog>
    <CommandDialog id="version" open={versionOpen} title="创建命名版本" onClose={() => setVersionOpen(false)}
      onConfirm={async () => createVersion(await versionForm.validateFields())}>
      <Form form={versionForm} layout="vertical"><Form.Item name="name" label="版本名称" rules={[{ required: true }]}><Input placeholder="V1 - Initial concept" /></Form.Item>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item></Form>
    </CommandDialog>
    <ProductReleaseCenter open={releaseOpen} releases={productReleases.data ?? []} loading={productReleases.isLoading}
      creating={command.isPending} onClose={() => setReleaseOpen(false)} onCreate={createRelease}
      onReplay={async (release) => { const result = await api.replayProductRelease(documentID, release.id);
        message.success(`版本重放验证：${result.status}`); }} />
    <ShareDialog resource={shareResource} onClose={() => setShareResource(undefined)} />
  </section></CommandProvider>;
}
