import {formatDisplayNumber} from "../../utils/display-number";
import {CadNumberInput as InputNumber} from "../../cad/overlay/cad-number-input";
import { defaultSketchToolMode } from "../../cad/sketch/sketch-inline-parameter-input";
import type { SketchCommandState, SketchCommitIntent, SketchCommitReceipt } from "../../cad/tool/sketch-command-session";
import { selectionNamingIssue, topologyNamingIssue } from "./topology-naming-capability";
import { openDocumentTab, registerDocumentTab, updateOpenDocumentSummary } from "./open-document-tab";
import { EditActivationGate, prepareOccurrenceEditSession, rootEditSession, pinnedReferenceInPath,
  withWorkingBody, type EditSession } from "./edit-session";
import { canDiscardNewSketch, defaultSolidReversed, type NewSketchSession } from "./sketch-session-policy";
import { assemblyConstraintEntry, invalidateAngleReferenceDirection, angleRelationSupportsMeasured } from "../../cad/assembly/assembly-angle";
import { offsetInitialFields } from "../../cad/assembly/assembly-offset";
import { assemblyQuantityInitialFields } from "../../cad/assembly/assembly-quantity";
import { assemblyPublicInitialFields, assemblyPublicEditDraft } from "../../cad/assembly/assembly-public";
import { assemblyEditIntent, assemblyIntentKey, AssemblyCandidateBinding, AssemblyDialogLifecycle } from "../../cad/assembly/assembly-edit-intent";
import { assemblyTargetSupportsEligible, contactRelationOptions, derivedSupportOptions, matchingAssemblyCapabilities } from "../../cad/assembly/assembly-capability";
import { exactNormalViewPlane } from "../../cad/navigation/normal-view";
import { AssemblyAngleParameters, angleAxisCandidateError } from "./assembly-angle-parameters";
import { fixedPoseAngles } from "../../cad/assembly/assembly-fixed-pose";
import type { AssemblyInteractionCommit } from "../../cad/assembly/assembly-interaction";
import { AssemblyConflictResponseGate,assemblyConflictCurrent,type AssemblyConflictReport,type AssemblyConflictMember,type AssemblyConflictRepair } from "../../cad/assembly/assembly-conflict";
import { AssemblyConflictPanel } from "./assembly-conflict-panel";
import {useOperationFeedback} from "../../cad/command/operation-feedback";
import { FeaturePreviewLegend } from "./feature-preview-legend";
import { InsertDocumentDialog } from "./insert-document-dialog";
import { InstancePatternDialog } from "./instance-pattern-dialog";
import type { InstancePatternPreview } from "./instance-pattern";
import "./workbench.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert, App, Button, Divider, Empty, Form, Input,
  Select, Space, Spin, Switch, Tag, Typography,
} from "antd";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api, isMockMode } from "../../api/client";
import { ApiError } from "../../api";
import { realtime, RealtimeError } from "../../api/realtime-client";
import { randomUUID } from "../../utils/random-uuid";
import { queryKeys } from "../../app/query-keys";
import { ShareDialog, type ShareResource } from "../../components/share-dialog";
import { CommandProvider } from "../../cad/command/command-context";
import { CommandRegistry } from "../../cad/command/command-registry";
import { selectionKey, selectionSetToken } from "../../cad/interaction/selection-identity";
import { treeVisibilityOverride } from "../../cad/interaction/tree-visibility";
import { visibilityResolverForView, type DisplayKind } from "../../cad/interaction/visibility-resolver";
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
import { followedDocumentIDs, staleProductDocumentIDs, followProductUpdates } from "./product-edit-context";
import { createAssemblyPreviewActor } from "./assembly-preview-machine";
import { isLengthParameter, linearExtrudeLengthInput, linearExtrudeLengthEditInput, parameterDisplayValue, parameterSourceText, parameterEditSource, parseParameterSource } from "./parameter-editor";
import { WorkbenchInspectorPanel } from "./workbench-inspector-panel";
import { deletableTreeNodesForSelections, ancestorHintKeysForSelections, findStructureEntity, findStructureOccurrenceEntity, isSolidFeature, selectedFeature, structureSelection, treeData, treeKeyForSelection, treeKeysForSelections } from "./workbench-tree-model";
import { ASSEMBLY_CONSTRAINT_STATUS, assemblyStatusAfterPreviewFailure, assemblySupportPresentation,
  firstDisconnectedSupport, validateReconnectCandidate } from "../../cad/assembly/assembly-constraint-ux";
import { describeAssemblyReference } from "../../cad/assembly/assembly-reference-presentation";
import { ProductReleaseCenter } from "./product-release-center";

const CadViewport = lazy(() => import("../../viewport/cad-viewport").then((module) => ({ default: module.CadViewport })));

const sketchToolCommands:WorkbenchToolID[]=["sketch.edit.delete","sketch.edit.copy","sketch.edit.move","sketch.edit.mirror","sketch.edit.split","sketch.edit.trim","sketch.edit.fillet","sketch.edit.chamfer","sketch.edit.extend","sketch.edit.complement","sketch.edit.close","sketch.edit.offset","sketch.edit.spline_insert","sketch.edit.spline_delete","sketch.edit.spline_close","sketch.edit.spline_control","sketch.edit.construction","sketch.project","sketch.ellipse","sketch.elliptical_arc","sketch.rectangle","sketch.rectangle.center","sketch.rectangle.oriented","sketch.circle.three_point","sketch.arc.three_point","sketch.polygon","sketch.polygon.circumscribed","sketch.point","sketch.line","sketch.circle","sketch.arc","sketch.polyline","sketch.spline","sketch.spline.control",
  "sketch.constraint.coincident","sketch.constraint.parallel","sketch.constraint.collinear","sketch.constraint.fixed","sketch.constraint.horizontal","sketch.constraint.vertical",
  "sketch.constraint.perpendicular","sketch.constraint.tangent","sketch.constraint.equal","sketch.dimension.linear",
  "sketch.constraint.horizontal_distance","sketch.constraint.vertical_distance","sketch.constraint.radius","sketch.constraint.major_radius","sketch.constraint.minor_radius","sketch.constraint.angle","sketch.constraint.concentric","sketch.constraint.point_on_object","sketch.constraint.midpoint","sketch.constraint.symmetry"];

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

type AssemblyConstraintUIDefinition = { supports: 0 | 1 | 2; direction: boolean; distanceDirection: boolean; value?: "angle" | "distance" };
const assemblyConstraintUI: Record<AssemblyConstraint["kind"], AssemblyConstraintUIDefinition> = {
  FIX: { supports: 1, direction: false, distanceDirection: false },
  RIGID: { supports: 2, direction: false, distanceDirection: false },
  CONTACT: { supports: 2, direction: false, distanceDirection: false },
  FIX_TOGETHER: { supports: 0, direction: false, distanceDirection: false },
  COINCIDENT: { supports: 2, direction: true, distanceDirection: false },
  CONCENTRIC: { supports: 2, direction: false, distanceDirection: false },
  ANGLE: { supports: 2, direction: false, distanceDirection: false, value: "angle" },
  DISTANCE: { supports: 2, direction: true, distanceDirection: true, value: "distance" },
};

function AssemblyConstraintFields({ kind, references, exactTypes, sourceTypes, contactCapabilities, view, lengthUnit, constraint, previewEvaluation, dirty, replacing, onReplace, onDerive, onLocate, onRefresh, onValueCommit, angleRelation }: {
  kind: keyof typeof assemblyConstraintUI; references: Array<AssemblyGeometryRef | undefined>;
  exactTypes?:string[];sourceTypes?:string[];contactCapabilities?:Array<{subtype:string}>;
  view?: DocumentView;
  lengthUnit: string;
  constraint?: AssemblyConstraint; dirty?: boolean; replacing?: 0 | 1 | 2;
  previewEvaluation?: CommandPreview["constraintEvaluation"];
  onReplace: (index: 0 | 1) => void; onLocate: (reference: AssemblyGeometryRef) => void;
  onDerive: (index:0|1,role:string)=>void;
  onRefresh?: () => void;
  onValueCommit: () => void; angleRelation?:AssemblyConstraint["angleRelation"];
}) {
  const definition = assemblyConstraintUI[kind];
  const evaluationStatus = previewEvaluation?.status ?? (dirty ? "NOT_UPDATED" : constraint?.evaluationStatus ?? "NOT_UPDATED");
  const status = ASSEMBLY_CONSTRAINT_STATUS[evaluationStatus];
  const planePair = exactTypes?.length===2 && exactTypes.every(kind=>kind==="PLANE");
  const distanceRelation = Form.useWatch("distanceRelation");
  const quantityExpression = Form.useWatch("quantityExpression");
  const expressionDriven = (kind === "DISTANCE" && !!quantityExpression?.trim()) || (kind === "ANGLE" && !!quantityExpression?.trim());
  const minimumValue = kind === "DISTANCE" && distanceRelation && distanceRelation !== "UNSIGNED" ? undefined : 0;
  const relation = angleRelation ?? constraint?.angleRelation;
  const relationOnly = kind === "ANGLE" && !angleRelationSupportsMeasured(relation);
  const directionApplicable = relation === "PARALLEL" || relation === "PERPENDICULAR" || (kind !== "ANGLE" && definition.direction && planePair);
  const distanceDirectionApplicable = definition.distanceDirection && !!exactTypes?.includes("PLANE");
  return <>
    {kind !== "FIX_TOGETHER" && <Typography.Text type="secondary">精确支持：{exactTypes?.join(" → ") || "待服务器解析；拾取面/边不代表平面/直线"}</Typography.Text>}
    {kind === "CONTACT" && <>
      <Form.Item name="contactKind" label="接触集合" rules={[{required:true}]}><Select onChange={onValueCommit} options={[...new Set(contactCapabilities?.map(c=>c.subtype.split("-").at(-1)?.toUpperCase()))].filter(Boolean).map(value=>({value,label:value}))}/></Form.Item>
      <Form.Item name="contactSide" label="材料侧" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:"EXTERNAL",label:"External（外法向相反）"},{value:"INTERNAL",label:"Internal（外法向相同）"}]}/></Form.Item>
      <Form.Item name="contactBranch" label="定向分支" rules={[{required:true}]}><Select onChange={onValueCommit} options={[{value:1,label:"正分支"},{value:-1,label:"负分支"}]}/></Form.Item>
      {!contactCapabilities?.length && <Alert type="warning" message="此精确支持组合不在 Contact 合同内"/>}
    </>}
    {kind === "FIX_TOGETHER" && <>
      <Form.Item name="groupName" label="固联组名称"><Input onBlur={onValueCommit}/></Form.Item>
      <Form.Item name="groupMembers" label="成员（组件或已有组）" rules={[{required:true,type:"array",min:2}]}><Select mode="multiple" onChange={onValueCommit} options={[
        ...(view?.product?.instances??[]).map(i=>({value:`instance:${i.id}`,label:i.name||i.id})),
        ...(view?.product?.constraints??[]).filter(c=>c.kind==="FIX_TOGETHER"&&c.id!==constraint?.id).map(c=>({value:`group:${c.id}`,label:`组：${c.name||c.id}`})),
      ]}/></Form.Item>
      <Typography.Text type="secondary">先求解有效组内约束，再让组整体参与外部约束；停用组不删除组内独立约束。</Typography.Text>
    </>}
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
    {definition.supports > 0 && <div className="assembly-support-list"><strong>支持元素</strong>{references.slice(0, definition.supports).map((reference,index)=><div className="assembly-support-row" key={index}>
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
        {reference && derivedSupportOptions(sourceTypes?.[index]).length>0 && <Select aria-label={`支持元素 ${index+1} 的精确子元素`}
          value={reference.derivedRole??""} options={derivedSupportOptions(sourceTypes?.[index])} onChange={role=>onDerive(index as 0|1,role)}/>}
        <Button size="small" type={replacing===index?"primary":"default"} onClick={()=>onReplace(index as 0|1)}>
          {support.status === "NOT_CONNECTED" ? "Reconnect" : "更换"}</Button>
      </>; })()}</div>)}</div>}
    {directionApplicable && <Form.Item name="directionRelation" label="方向"><Select onChange={onValueCommit} options={relation === "PERPENDICULAR" ? [{value:"SAME",label:"正向（90°）"},{value:"OPPOSITE",label:"反向（270°）"}] : [
      {value:"UNORIENTED",label:"未定义"},{value:"SAME",label:"同向"},{value:"OPPOSITE",label:"反向"}]} /></Form.Item>}
    {distanceDirectionApplicable && <Form.Item name="distanceRelation" label="距离方向"><Select onChange={onValueCommit} options={[
      {value:"UNSIGNED",label:"无符号"},{value:"SELECTED_PLANE_NORMAL_V1",label:"第一元素法向（双平面）/所选平面法向"},
      ...(constraint?.distanceRelation === "ALONG_SECOND_NORMAL" || constraint?.distanceRelation === "OPPOSITE_SECOND_NORMAL" ?
        [{value:constraint.distanceRelation,label:"历史定义：第二法向约定（保留）"}] : [])]} /></Form.Item>}
    {kind === "DISTANCE" && <>
      <Typography.Text type="secondary">偏移 = 所选法向 ·（第一位置 − 第二位置）。双平面取第一法向；同向/反向不改变输入正负。无平面仅无符号无限支撑距离。</Typography.Text>
      <Form.Item name="quantityKey" label="偏移参数标识"><Input onBlur={onValueCommit} /></Form.Item>
      <Form.Item name="quantityExpression" label="长度表达式（空白使用数值）"><Input placeholder="例如 Offset_base + 5 mm" onBlur={onValueCommit} onPressEnter={event=>event.currentTarget.blur()} /></Form.Item>
      <Typography.Text type="secondary">可引用：{view?.product?.constraints?.flatMap(c=>{const p=c.quantityParameter;return p?[p.key]:[];}).join(", ") || "暂无；表达式须带长度单位"}</Typography.Text>
      <Form.Item name="constraintMode" label="求值模式"><Select onChange={onValueCommit} options={[{value:"DRIVING",label:"驱动"},{value:"MEASURED",label:"只测量（保留驱动定义）"}]} /></Form.Item>
      {constraint?.mode === "MEASURED" && <Typography.Text>测量：{constraint.measuredValue === undefined ? "不可测" : `${constraint.measuredValue} mm`}</Typography.Text>}
    </>}
    {kind === "ANGLE" && !relationOnly && <>
      <Form.Item name="quantityKey" label="角度参数标识"><Input onBlur={onValueCommit}/></Form.Item>
      <Form.Item name="quantityExpression" label="角度表达式（空白使用数值）"><Input placeholder="例如 Tilt_base + 15 deg" onBlur={onValueCommit} onPressEnter={event=>event.currentTarget.blur()}/></Form.Item>
      <Form.Item name="constraintMode" label="求值模式"><Select onChange={onValueCommit} options={[{value:"DRIVING",label:"驱动"},{value:"MEASURED",label:"只测量（保留驱动定义）"}]}/></Form.Item>
      {constraint?.mode === "MEASURED" && <Typography.Text>测量：{constraint.measuredValue === undefined ? "不可测" : `${constraint.measuredValue*180/Math.PI} deg`}</Typography.Text>}
    </>}
    {definition.value && !relationOnly && <Form.Item name="value" label={definition.value === "angle" ? "角度（deg）" : `偏移（${lengthUnit}）`}
      rules={expressionDriven ? [] : [{required:true},{type:"number",min:minimumValue,max:definition.value === "angle"?360:undefined}]}>
      <InputNumber disabled={expressionDriven} min={minimumValue} max={definition.value === "angle"?360:undefined} precision={2} style={{width:"100%"}}
        onBlur={onValueCommit} onPressEnter={(event)=>event.currentTarget.blur()} /></Form.Item>}
  </>;
}


export function Workbench() {
  const { documentID = "" } = useParams();
  const navigate = useNavigate();
  const newSketchSession = useRef<NewSketchSession | undefined>(undefined);
  const client = useQueryClient();
  const { message,modal } = App.useApp();
  const operationFeedback=useOperationFeedback();
  const commandRegistry = useMemo(() => new CommandRegistry(), []);
  const viewport = useRef<CadViewportHandle>(null);
  const [sketchCommandState,setSketchCommandState]=useState<SketchCommandState>();
  const [sketchReceipt,setSketchReceipt]=useState<SketchCommitReceipt>();
  const normalViewRequest = useRef(0);
  const [padOpen, setPadOpen] = useState(false);
  const receiptPrompt=useRef(false);
  const [moveReceiptPending,setMoveReceiptPending]=useState(false);
  const [conflictOpen,setConflictOpen]=useState(false);
  const [conflictReport,setConflictReport]=useState<AssemblyConflictReport>();
  const [conflictContextCurrent,setConflictContextCurrent]=useState(false);
  const [conflictPending,setConflictPending]=useState(false);
  const [conflictError,setConflictError]=useState<string>();
  const conflictGate=useRef(new AssemblyConflictResponseGate());
  const conflictAbort=useRef<AbortController|undefined>(undefined);
  const [renameTarget, setRenameTarget] = useState<SpecificationTreeNode>();
  const [padGenerator, setPadGenerator] = useState<"LINEAR_EXTRUDE" | "REVOLVE">("LINEAR_EXTRUDE");
  const [padSketchID, setPadSketchID] = useState<string>();
  const [padPreviewPending, setPadPreviewPending] = useState(false);
  const [padPreviewPlacement, setPadPreviewPlacement] = useState<{
    bodyId: string; bodyName: string; assignment: CommandPreview["bodyAssignment"];
  }>();
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
  const latestHostVersion=useRef<string|undefined>(undefined);
  const automaticUpdateSignature = useRef("");
  const automaticUpdateRunning = useRef(false);
  const [automaticUpdateEpoch, setAutomaticUpdateEpoch] = useState(0);
  const [productUpdateFailure, setProductUpdateFailure] = useState<string>();
  const [editSession, setEditSession] = useState<EditSession>();
  const activationGate = useRef(new EditActivationGate());
  const [insertOpen, setInsertOpen] = useState(false);
  const [patternOpen, setPatternOpen] = useState(false);
  const previewInsertPattern = useCallback((input?: InstancePatternPreview) => viewport.current?.previewInsertPattern(input), []);
  const showAnalysisMotion = useCallback((motion?: Parameters<NonNullable<CadViewportHandle["showRemainingMotion"]>>[0]) => viewport.current?.showRemainingMotion(motion), []);
  const [newPartTarget, setNewPartTarget] = useState<SpecificationTreeNode>();
  const [versionOpen, setVersionOpen] = useState(false);
  const [releaseOpen, setReleaseOpen] = useState(false);
  const [datumPlaneOpen, setDatumPlaneOpen] = useState(false);
  const [datumAxisOpen, setDatumAxisOpen] = useState(false);
  const [parameterManagerOpen, setParameterManagerOpen] = useState(false);
  const [publicationManagerOpen, setPublicationManagerOpen] = useState(false);
  const [externalParameterID, setExternalParameterID] = useState<string>();
  const [editingParameterID, setEditingParameterID] = useState<string>();
  const [editingPublication, setEditingPublication] = useState<{id:string;kind:"PART"|"PRODUCT"}>();
  const [pendingAssemblyConstraint, setPendingAssemblyConstraint] = useState<{ kind: AssemblyConstraintToolKind; references: AssemblyGeometryRef[]; fixMode?:AssemblyConstraint["fixMode"]; angleRelation?:AssemblyConstraint["angleRelation"]; angleAxis?:AssemblyGeometryRef; reverseAngleAxis?:boolean; angleReferenceDirection?: Vec3 }>();
  const [editingAssemblyConstraint, setEditingAssemblyConstraint] = useState<AssemblyConstraint>();
  const [replacingAssemblyReference, setReplacingAssemblyReference] = useState<0 | 1 | 2>();
  const [assemblyDefinitionDirty, setAssemblyDefinitionDirty] = useState(false);
  const [reconnectError, setReconnectError] = useState<string>();
  const [assemblyPreviewEvaluation, setAssemblyPreviewEvaluation] = useState<CommandPreview["constraintEvaluation"]>();
  const [assemblyPreviewCommit, setAssemblyPreviewCommit] = useState(0);
  const assemblyPreviewAbort = useRef<AbortController | undefined>(undefined);
  const assemblyPreviewSequence = useRef(0);
	const assemblyPreviewID = useRef<string | undefined>(undefined);
  const assemblyCandidate = useRef(new AssemblyCandidateBinding());
  const assemblyDraftBase = useRef<{documentId:string;revision:string}|undefined>(undefined);
  const assemblyDialogLifecycle = useRef(new AssemblyDialogLifecycle());
	const assemblyInteractionID = useRef<string>(randomUUID());
  const assemblyPreviewActor = useRef<ReturnType<typeof createAssemblyPreviewActor> | undefined>(undefined);
  const [assemblyPreviewSnapshot, setAssemblyPreviewSnapshot] = useState(() => createAssemblyPreviewActor().getSnapshot());
  const inspectorOpen = useUIPreferences((state) => state.inspectorOpen);
  const setInspectorOpen = useUIPreferences((state) => state.setInspectorOpen);
  const treeVisibilityOverrides = useUIPreferences((state) => state.treeVisibilityOverrides);
  const setTreeVisibility = useUIPreferences((state) => state.setTreeVisibility);
  const catiaRotationSphereVisible = useUIPreferences((state) => state.catiaRotationSphereVisible);
  const navigationProfile = useUIPreferences((state) => state.navigationProfile);
  const referenceVisibility = useUIPreferences((state) => state.referenceVisibility);
  const solidDisplay = useUIPreferences((state) => state.solidDisplay);
  const captureSettings = useUIPreferences((state) => state.captureSettings);
  const displayLengthUnit = useUIPreferences((state) => state.displayLengthUnit);
  const documentLengthUnits = useUIPreferences((state) => state.documentLengthUnits);
  const setShellActiveDocumentID = useApplicationContext((state) => state.setActiveDocumentID);
  const [shareResource, setShareResource] = useState<ShareResource>();
  const [padForm] = Form.useForm<{ generator: "LINEAR_EXTRUDE" | "REVOLVE"; operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
    bodyId: string; lengthSource: string; angle: number; axisEntityId?: string; reversed: boolean }>();
	const [featureForm] = Form.useForm<{ lengthText: string }>();
  const [renameForm] = Form.useForm<{ name: string }>();
  const [newPartForm] = Form.useForm<{ name?: string; description?: string }>();
  const [versionForm] = Form.useForm<{ name: string; description: string }>();
  const [datumPlaneForm] = Form.useForm<{ name: string; offset: number }>();
  const [datumAxisForm] = Form.useForm<{ name: string; ox: number; oy: number; oz: number; dx: number; dy: number; dz: number }>();
  const [parameterForm] = Form.useForm<{ key: string; source: string }>();
  const [createUserParameterForm] = Form.useForm<{name?:string;value:number;unit:string}>();
  const [publicationForm] = Form.useForm<{ name: string; semanticPurpose: string }>();
  const [publicationEditForm] = Form.useForm<{name:string;semanticPurpose:string}>();
  const [contextReferenceForm] = Form.useForm<{ name:string;catalogKey:string;publicationType:string;targetId:string }>();
  const [replacementForm] = Form.useForm<{referencedDocumentId:string}>();
  const [externalParameterForm] = Form.useForm<{ catalogKey: string }>();
  const [assemblyConstraintForm] = Form.useForm<{ value: number; directionRelation: string; distanceRelation: string; quantityExpression?:string;quantityKey?:string;contactKind?:string;contactSide?:string;contactBranch?:number;groupName?:string;groupMembers?:string[];constraintMode?:string; fixedTranslation:Vec3; fixedAngles:Vec3 }>();
  const padOperation = Form.useWatch("operation", padForm) ?? "ADD";
  const contextPublicationType = Form.useWatch("publicationType", contextReferenceForm) ?? "PLANE";
  const assemblyDirection = Form.useWatch("directionRelation", assemblyConstraintForm);
  const assemblyDistance = Form.useWatch("distanceRelation", assemblyConstraintForm);
  const store = useWorkbenchStore();
  const document = useQuery({ queryKey: queryKeys.document(documentID), queryFn: () => api.getDocument(documentID), enabled: Boolean(documentID) });
	useEffect(() => {
	  activationGate.current.invalidate(); setEditSession(undefined); store.endSketch(); store.setSelection(null);
	  if (!documentID) return;
	  void registerDocumentTab(documentID, client, api.openDocument)
	    .catch((error: Error) => message.error(`打开工作空间失败：${error.message}`));
	// Route changes are the explicit open lifecycle. Interaction state changes
	// must never register, reorder, or reopen a tab.
	// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [client, documentID]);
  useEffect(() => { setPatternOpen(false); previewInsertPattern(); }, [editSession?.editTarget.documentId,
    editSession?.editTarget.instancePath?.canonical, previewInsertPattern]);
  useEffect(() => {
    // A stopped XState actor cannot be restarted. Own one actor per effect
    // lifetime so StrictMode's setup/cleanup/setup cycle retains live previews.
    const actor = createAssemblyPreviewActor();
    assemblyPreviewActor.current = actor;
    const subscription = actor.subscribe(setAssemblyPreviewSnapshot);
    actor.start();
    return () => { assemblyPreviewActor.current = undefined; subscription.unsubscribe(); actor.stop(); };
  }, [assemblyPreviewActor]);
	const activeID = editSession?.editTarget.documentId ?? documentID;
	const activeInstancePath = editSession?.editTarget.instancePath?.canonical;
  useEffect(()=>()=>assemblyDialogLifecycle.current.invalidate(),[documentID,activeID,activeInstancePath]);
  const activeDocument = useQuery({ queryKey: queryKeys.document(activeID), queryFn: () => api.getDocument(activeID),
    enabled: Boolean(editSession && activeID !== documentID) });
	const lengthUnit = effectiveLengthUnit(displayLengthUnit, documentLengthUnits, activeID);
	useEffect(() => {
	  setShellActiveDocumentID(activeID);
	  return () => setShellActiveDocumentID(undefined);
	}, [activeID, setShellActiveDocumentID]);
	const toolbarCatalog = useQuery({ queryKey: ["ui", "toolbars"], queryFn: api.toolbarCatalog, staleTime: 5 * 60_000 });
  const catalog = useQuery({ queryKey: queryKeys.documents({ workbench: true }), queryFn: () => api.listDocuments({ limit: 100, allFolders: true }), enabled: publicationManagerOpen });

  const refresh = useCallback(async (view?: DocumentView) => {
    if (view) {
      client.setQueryData(queryKeys.document(view.document.id), view);
      updateOpenDocumentSummary(client, view.document);
      setEditSession((current) => current && current.hostDocumentId === documentID ? {
        ...current,
        snapshot: {
          ...current.snapshot,
          ...(view.document.id === current.hostDocumentId ? { hostRevisionId: view.document.versionId } : {}),
          ...(view.document.id === current.editTarget.documentId ? { targetRevisionId: view.document.versionId } : {}),
        },
      } : current);
    }
    const changedID = view?.document.id ?? activeID;
    await Promise.all([view ? (changedID === documentID ? Promise.resolve() : client.invalidateQueries({ queryKey: queryKeys.document(documentID) }))
      : client.invalidateQueries({ queryKey: queryKeys.document(changedID) }),
    client.invalidateQueries({ queryKey: queryKeys.history(changedID), refetchType: "active" }),
    client.invalidateQueries({ queryKey: queryKeys.documentProperties(changedID), refetchType: "active" }),
    client.invalidateQueries({ queryKey: ["product-design-session", documentID] }),
    client.invalidateQueries({ queryKey: ["context-catalog", documentID] }),
    client.invalidateQueries({ queryKey: ["product-update-plan", documentID] }),
    client.invalidateQueries({ queryKey: ["documents"] })]);
  }, [activeID, client, documentID]);
  useEffect(() => {
    if (!documentID || isMockMode) return;
    let disposed = false;
    let unsubscribe: (() => void) | undefined;
    void realtime.subscribe(documentID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(documentID), snapshot.view);
        updateOpenDocumentSummary(client, snapshot.view.document);
      }
      void Promise.all([
        event.type === "document.snapshot.v1" ? Promise.resolve() : client.invalidateQueries({ queryKey: queryKeys.document(documentID) })
          .then(() => { const fresh = client.getQueryData<DocumentView>(queryKeys.document(documentID));
            if (fresh) updateOpenDocumentSummary(client, fresh.document); }),
        client.invalidateQueries({ queryKey: queryKeys.history(documentID), refetchType: "active" }),
        client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID), refetchType: "active" }),
        client.invalidateQueries({ queryKey: ["product-update-plan", documentID] }),
        client.invalidateQueries({ queryKey: ["documents"] }),
      ]);
    }).then((dispose) => {
      if (disposed) dispose(); else unsubscribe = dispose;
    }).catch((error: Error) => {
      if (!disposed) message.error(`实时连接失败：${error.message}`);
    });
    return () => { disposed = true; unsubscribe?.(); };
  }, [client, documentID, message]);
  const command = useMutation({
    onMutate:async()=>{await viewport.current?.settleAssemblyInteraction();},
    mutationFn: (operation: () => Promise<DocumentView>) => operation(),
    onSuccess: (updated) => { store.setSelection(null); void refresh(updated); }, onError: (error) => operationFeedback(error,"命令")
  });
  const moveCommand = useMutation({mutationFn:(operation:()=>Promise<DocumentView>)=>operation(),
    onSuccess:(updated)=>{void refresh(updated);},onError:()=>{void refresh();}});
  const view = document.data;
  useEffect(() => {
    if (!view || editSession?.hostDocumentId === view.document.id) return;
    setEditSession(rootEditSession(view, activationGate.current.begin()));
  }, [editSession?.hostDocumentId, view]);
  const editingView = activeID === documentID ? view : activeDocument.data;
  const engineeringEvidence=useQuery({queryKey:["assembly-engineering-evidence",editingView?.document.id,editingView?.document.versionId],
    queryFn:({signal})=>api.getAssemblyEngineeringEvidence(editingView!.document.id,editingView!.document.versionId,signal),
    enabled:conflictOpen&&Boolean(editingView?.product),retry:false});
  useEffect(()=>{if(engineeringEvidence.error)operationFeedback(engineeringEvidence.error,"读取装配状态");},[engineeringEvidence.error,operationFeedback]);
  latestHostVersion.current=view?.document.versionId;
  useEffect(()=>{
    conflictGate.current.invalidate();conflictAbort.current?.abort();setConflictPending(false);setConflictContextCurrent(false);
    return ()=>{conflictGate.current.invalidate();conflictAbort.current?.abort();};
  },[editingView?.document.id,editingView?.document.versionId,documentID,view?.document.versionId,activeInstancePath]);
  useEffect(()=>{
    if(!editingAssemblyConstraint || !editingView)return;
    const authoritative=editingView.product?.constraints?.find(c=>c.id===editingAssemblyConstraint.id);
    if(!authoritative || authoritative.suppressed!==editingAssemblyConstraint.suppressed) {
      assemblyDialogLifecycle.current.invalidate();
      assemblyPreviewSequence.current++;assemblyPreviewAbort.current?.abort();assemblyPreviewID.current=undefined;
      assemblyCandidate.current.invalidate();assemblyPreviewActor.current?.send({type:"RESET"});
      viewport.current?.clearCommandPreview();setEditingAssemblyConstraint(undefined);
    }
  },[editingView,editingAssemblyConstraint]);
  const assemblyGroupMembers = Form.useWatch("groupMembers",assemblyConstraintForm) as string[]|undefined;
  const assemblyContactKind = Form.useWatch("contactKind",assemblyConstraintForm) as string|undefined;
  const editingGroup = editingAssemblyConstraint?.kind==="FIX_TOGETHER" || pendingAssemblyConstraint?.kind==="fix_together";
  const assemblyReferences = editingAssemblyConstraint ? [editingAssemblyConstraint.first,editingAssemblyConstraint.second].filter((r):r is AssemblyGeometryRef=>!!r&&!!r.instanceId) : pendingAssemblyConstraint?.references ?? [];
  const assemblyInspectionReferences = [...assemblyReferences.map(reference=>({...reference,derivedRole:undefined})),...assemblyReferences];
  const assemblySupportQuery = useQuery({queryKey:["assembly-support-inspection",editingView?.document.id,editingView?.document.versionId,assemblyInspectionReferences],
    queryFn:({signal})=>api.inspectAssemblySupports(editingView!.document.id,assemblyInspectionReferences,signal),enabled:!!editingView&&!editingGroup&&assemblyReferences.length>0,retry:false});
  const assemblyCapabilityQuery = useQuery({queryKey:["assembly-capabilities"],queryFn:()=>api.assemblyCapabilities(),enabled:!!editingAssemblyConstraint||!!pendingAssemblyConstraint,staleTime:Infinity,retry:false});
  const assemblyInspection = assemblySupportQuery.data?.versionId===editingView?.document.versionId ? assemblySupportQuery.data : undefined;
  const assemblyExactTypes = useMemo(()=>assemblyInspection?.supports.slice(assemblyReferences.length).map(s=>s.exactType??""),[assemblyInspection,assemblyReferences.length]);
  const assemblySourceTypes = useMemo(()=>assemblyInspection?.supports.slice(0,assemblyReferences.length).map(s=>s.exactType??""),[assemblyInspection,assemblyReferences.length]);
  const assemblyTargetSupports = assemblyInspection?.supports.slice(assemblyReferences.length);
  const resolvedAssemblyReferences = assemblyInspection?.supports.slice(assemblyReferences.length).map(s=>s.reference)??assemblyReferences;
  const contactCapabilities = matchingAssemblyCapabilities(assemblyCapabilityQuery.data?.capabilities,"Contact",assemblyExactTypes);
  const editingContact = editingAssemblyConstraint?.kind==="CONTACT"||pendingAssemblyConstraint?.kind==="contact";
  const publicKind = editingAssemblyConstraint?.kind ?? pendingAssemblyConstraint?.kind.toUpperCase();
  const publicFamily = publicKind === "DISTANCE" ? "Offset" : publicKind === "ANGLE" ? "Angle" : publicKind === "FIX" ? "Fix" : publicKind === "CONTACT" ? "Contact" : "Coincidence";
  const applicableCapabilities = matchingAssemblyCapabilities(assemblyCapabilityQuery.data?.capabilities,publicFamily,assemblyExactTypes);
  const angleRelation = editingAssemblyConstraint?.angleRelation ?? pendingAssemblyConstraint?.angleRelation ?? "FREE";
  const angleAxis = editingAssemblyConstraint?.angleAxis ?? pendingAssemblyConstraint?.angleAxis;
  const angleAxisQuery = useQuery({queryKey:["assembly-angle-axis-inspection",editingView?.document.id,editingView?.document.versionId,angleAxis],
    queryFn:({signal})=>api.inspectAssemblySupports(editingView!.document.id,[{...angleAxis!,derivedRole:undefined},angleAxis!],signal),
    enabled:!!editingView&&publicKind==="ANGLE"&&angleRelation==="DIRECTED"&&!!angleAxis,retry:false});
  const axisInspection = angleAxisQuery.data?.versionId===editingView?.document.versionId?angleAxisQuery.data:undefined;
  const resolvedAngleAxis = axisInspection?.supports[1]?.reference ?? angleAxis;
  const axisExactType = axisInspection?.supports[1]?.exactType;
  const axisReady = publicKind!=="ANGLE"||angleRelation!=="DIRECTED"||assemblyTargetSupportsEligible(axisInspection?.supports.slice(1))&&
    matchingAssemblyCapabilities(assemblyCapabilityQuery.data?.capabilities,"Angle",[axisExactType??"",axisExactType??""]).length>0;
  const assemblySupportsReady = editingGroup ? (assemblyGroupMembers?.length??0)>=2&&
    matchingAssemblyCapabilities(assemblyCapabilityQuery.data?.capabilities,"FixTogether",["BODY"]).some(capability=>capability.subtype==="multi-member") : !!assemblyExactTypes?.length&&assemblyTargetSupportsEligible(assemblyTargetSupports)&&
    applicableCapabilities.length>0&&(!editingContact||contactRelationOptions(contactCapabilities).includes(assemblyContactKind??""))&&
    (publicKind!=="ANGLE"||applicableCapabilities.some(capability=>capability.subtype===angleRelation))&&axisReady;
  useEffect(()=>{
    if(pendingAssemblyConstraint?.kind==="distance"&&assemblyExactTypes?.length){
      assemblyConstraintForm.setFieldValue("distanceRelation",offsetInitialFields(undefined,[],assemblyExactTypes).distanceRelation);
    }
  },[assemblyExactTypes,pendingAssemblyConstraint?.kind,assemblyConstraintForm]);
  useEffect(()=>{
    if(pendingAssemblyConstraint?.kind==="contact"&&!assemblyContactKind){
      const first=contactRelationOptions(contactCapabilities)[0];if(first)assemblyConstraintForm.setFieldValue("contactKind",first);
    }
  },[assemblyExactTypes,pendingAssemblyConstraint?.kind,assemblyContactKind,assemblyConstraintForm,assemblyCapabilityQuery.data]);
  const invalidateAssemblyDefinition = () => {
    assemblyDialogLifecycle.current.invalidate();
    assemblyCandidate.current.invalidate();
    assemblyPreviewSequence.current+=1;assemblyPreviewAbort.current?.abort();assemblyPreviewID.current=undefined;
    viewport.current?.clearCommandPreview();setAssemblyPreviewEvaluation(undefined);setAssemblyDefinitionDirty(true);
    assemblyPreviewActor.current?.send({type:"CHANGE"});setAssemblyPreviewCommit(value=>value+1);
  };
  const deriveAssemblyReference = (index:0|1,role:string) => {
    const reference=assemblyReferences[index];if(!reference)return;
    const next={...reference,derivedRole:role||undefined};invalidateAssemblyDefinition();
    if(editingAssemblyConstraint)setEditingAssemblyConstraint({...editingAssemblyConstraint,...(index===0?{first:next}:{second:next})});
    else if(pendingAssemblyConstraint)setPendingAssemblyConstraint({...pendingAssemblyConstraint,references:pendingAssemblyConstraint.references.map((r,i)=>i===index?next:r)});
  };
  const deriveAngleAxis = (role:string) => {
    if(!angleAxis)return;invalidateAssemblyDefinition();const next={...angleAxis,derivedRole:role||undefined};
    if(editingAssemblyConstraint)setEditingAssemblyConstraint({...editingAssemblyConstraint,angleAxis:next});
    else if(pendingAssemblyConstraint)setPendingAssemblyConstraint({...pendingAssemblyConstraint,angleAxis:next});
  };
  const workingBodyID = editingView?.part?.bodies.some((body) => body.id === editSession?.workingBodyId)
    ? editSession?.workingBodyId : editingView?.part?.activeBodyId;
  useEffect(() => {
    if (!view || !editSession || editSession.hostDocumentId !== view.document.id ||
        editSession.snapshot.hostRevisionId === view.document.versionId) return;
    if (!editSession.editTarget.instancePath?.canonical) {
      setEditSession((current) => current?.hostDocumentId === view.document.id && !current.editTarget.instancePath?.canonical
        ? { ...current, snapshot: { ...current.snapshot, hostRevisionId: view.document.versionId,
          targetRevisionId: view.document.versionId } } : current);
      return;
    }
    const target = editSession.editTarget;
    const generation = activationGate.current.begin();
    void prepareOccurrenceEditSession({ host: view,
      node: { kind: target.documentType, documentId: target.documentId, documentType: target.documentType,
        instancePath: target.instancePath }, activationGeneration: generation,
      getDesignSession: api.getProductDesignSession,
      getDocument: (id) => client.fetchQuery({ queryKey: queryKeys.document(id), queryFn: () => api.getDocument(id) }),
    }).then((prepared) => {
      if (!activationGate.current.isCurrent(generation)) return;
      client.setQueryData(queryKeys.document(prepared.targetView.document.id), prepared.targetView);
      setEditSession((current) => current && current.editTarget.documentId === target.documentId &&
        current.editTarget.instancePath?.canonical === target.instancePath?.canonical
        ? { ...prepared.session, workingBodyId: current.workingBodyId } : current);
    }).catch((error) => {
      if (activationGate.current.isCurrent(generation)) setProductUpdateFailure(
        `编辑上下文快照无法同步：${error instanceof Error ? error.message : String(error)}`);
    });
  // Re-resolve only when the host snapshot changes; target changes have their own activation flow.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client, documentID, view?.document.versionId]);
  const selectedNamingIssue = selectionNamingIssue(store.selection, editingView, view);
  const repairableImport = editingView?.part?.features.find((feature) => feature.type === "IMPORT_BODY" && !feature.importDefinitionId);
  const activeNamingIssue = Object.values(editingView?.artifacts??{}).find(a=>a.topology.faces>0 && a.naming && !a.naming.canBind)?.naming;
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
    ...followedDocumentIDs(view), ...(view?.referenceUpdates ?? []).map((item) => item.sourceDocumentId),
    ...(activeID !== documentID ? [activeID] : []),
  ])].filter((id) => id !== documentID), [activeID, documentID, view?.followedDocumentIds, view?.product?.instances, view?.referenceUpdates]);
  useEffect(() => {
    if (isMockMode || !view || followedIDs.length === 0) return;
    let disposed = false; const unsubscribers: Array<() => void> = [];
    for (const dependencyID of followedIDs) void realtime.subscribe(dependencyID, (event) => {
      if (event.type === "document.snapshot.v1") {
        const snapshot = event.payload as { view: DocumentView };
        client.setQueryData(queryKeys.document(dependencyID), snapshot.view);
        updateOpenDocumentSummary(client, snapshot.view.document);
        return;
      }
      // A fresh root projection will drive the serialized leaf-to-root auto-update effect.
      void client.invalidateQueries({ queryKey: queryKeys.document(dependencyID) }).then(() => {
        const fresh = client.getQueryData<DocumentView>(queryKeys.document(dependencyID));
        if (fresh) updateOpenDocumentSummary(client, fresh.document);
      });
      void client.invalidateQueries({ queryKey: queryKeys.document(documentID) });
      void client.invalidateQueries({ queryKey: queryKeys.documentProperties(documentID), refetchType: "active" });
      void client.invalidateQueries({ queryKey: ["product-update-plan", documentID] });
    }).then((unsubscribe) => { if (disposed) unsubscribe(); else unsubscribers.push(unsubscribe); })
      .catch((error: Error) => { if (!disposed) message.error(`引用文档实时连接失败：${error.message}`); });
    return () => { disposed = true; unsubscribers.forEach((unsubscribe) => unsubscribe()); };
  }, [client, documentID, followedIDs.join("|"), message, view]);
  const treeNodes = useMemo(() => {
    const resolver = visibilityResolverForView(view);
    const decorate = (node: SpecificationTreeNode): SpecificationTreeNode => {
      const visibilityKey = node.selection ? selectionKey(node.selection) : node.key;
      const kind = node.kind === "SKETCH_INPUT_REFERENCE" ? "SKETCH" : node.kind;
      const semantic = ["INSTANCE", "PART", "BODY", "SKETCH", "SKETCH_ENTITY"].includes(kind ?? "") && node.selection?.entityRef
        ? resolver.resolve({documentId: node.selection.entityRef.documentId,
          occurrencePath: node.instancePath?.canonical ?? "", kind: kind as DisplayKind,
          entityId: node.selection.entityRef.entityId, ownerEntityId: node.ownerEntityId, bodyId: node.bodyId},
          store.activeSketchID ? {id:store.activeSketchID, occurrencePath:activeInstancePath ?? ""} : undefined)
        : undefined;
      const ownVisible = semantic?.effectiveVisible ?? treeVisibilityOverride(visibilityKey, treeVisibilityOverrides) ?? true;
      const actionOwner = node.kind === "INSTANCE" ? node.ownerDocumentId : node.documentId;
      const ownerEditable = Boolean(editingView && actionOwner === editingView.document.id &&
        ["OWNER", "EDITOR"].includes(editingView.document.permission ?? ""));
      return { ...node, hidden: !ownVisible, localVisible: semantic?.localVisible ?? node.localVisible,
        capabilities: ownerEditable ? node.capabilities : node.capabilities?.filter((capability) => capability !== "EDIT" && capability !== "DELETE"),
        visibilityMode: semantic?.mode ?? node.visibilityMode,
        hiddenByAncestor: Boolean(semantic?.blockedBy && (semantic.blockedBy.kind !== kind || semantic.blockedBy.entityId !== node.entityId)),
        visibilityBlocker: semantic?.blockedBy ? `${semantic.blockedBy.kind} · ${semantic.blockedBy.entityId}` : undefined,
        children: node.children?.map(decorate) };
    };
    return view ? treeData(view, editingView).map((node) => decorate(node)) : [];
  }, [view, editingView, store.activeSketchID, activeInstancePath, treeVisibilityOverrides]);
  useEffect(()=>{
    normalViewRequest.current+=1;
    return ()=>{normalViewRequest.current+=1;};
  },[store.selection,store.sketchPlane,store.activeSketchID,view?.document.versionId,documentID]);
  const normalToSelection = async () => {
    const generation = ++normalViewRequest.current;
    if(store.sketchPlane){viewport.current?.normalToSketch();return;}
    const selection = store.selection;
    if (!selection || !view) return;
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
    if (!productUpdatePlan.dataUpdatedAt && !staleProductDocumentIDs(view).length) return;
    const signature = `${view.document.versionId}:${productUpdatePlan.dataUpdatedAt}`;
    if (automaticUpdateSignature.current === signature) return;
    automaticUpdateSignature.current = signature;
    automaticUpdateRunning.current = true;
    setProductUpdateFailure(undefined);
    let disposed = false;
    void (async () => {
      try {
        const updatedViews = await followProductUpdates(view, api);
        if (!disposed) for (const updated of updatedViews) {
          client.setQueryData(queryKeys.document(updated.document.id), updated);
          updateOpenDocumentSummary(client, updated.document);
        }
        if (!disposed && updatedViews.length) await Promise.all([
          client.invalidateQueries({ queryKey: queryKeys.document(documentID) }),
          client.invalidateQueries({ queryKey: ["product-update-plan", documentID] }),
        ]);
      } catch (cause) {
        if (!disposed) {
          const diagnostic = cause instanceof Error ? cause.message : String(cause);
          setProductUpdateFailure(diagnostic);
          message.error(`自动跟随最新版本失败：${diagnostic}`);
        }
      } finally {
        automaticUpdateRunning.current = false;
        // A new view/document may have arrived while the previous wave ran.
        // Reconsider it after releasing the single-flight guard.
        setAutomaticUpdateEpoch((value) => value + 1);
        if (disposed) void client.invalidateQueries({ queryKey: queryKeys.document(documentID) });
      }
    })();
    return () => { disposed = true; };
  }, [canEditRoot, client, documentID, message, view, productUpdatePlan.dataUpdatedAt, automaticUpdateEpoch]);

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
        return invalidateAngleReferenceDirection(next);
      });
    } else if (pendingAssemblyConstraint) {
      const references = [...pendingAssemblyConstraint.references];
      const other = references[replacingAssemblyReference === 0 ? 1 : 0];
      const validation = validateReconnectCandidate(reference, other);
      if (validation) { setReconnectError(validation); return; }
      references[replacingAssemblyReference] = reference;
      setPendingAssemblyConstraint(invalidateAngleReferenceDirection({ ...pendingAssemblyConstraint, references }));
    }
    invalidateAssemblyDefinition();
    setAssemblyDefinitionDirty(true);
    setAssemblyPreviewEvaluation(undefined);
    setReconnectError(undefined);
    setReplacingAssemblyReference(undefined);
  }, [store.selection, replacingAssemblyReference, editingAssemblyConstraint, pendingAssemblyConstraint]);

  useEffect(() => {
    const constraint = editingAssemblyConstraint;
    const pending = pendingAssemblyConstraint;
    const references = resolvedAssemblyReferences;
    const kind = (constraint?.kind.toLowerCase() ?? pending?.kind) as AssemblyConstraintToolKind | undefined;
    const missingAngleAxis = kind === "angle" && (constraint?.angleRelation ?? pending?.angleRelation ?? "FREE") === "DIRECTED" && !(constraint?.angleAxis ?? pending?.angleAxis);
    if(editingView && kind && assemblyDraftBase.current &&
      (assemblyDraftBase.current.documentId!==editingView.document.id || assemblyDraftBase.current.revision!==editingView.document.versionId)) {
      const sequence=++assemblyPreviewSequence.current;assemblyPreviewAbort.current?.abort();assemblyPreviewID.current=undefined;
      assemblyCandidate.current.invalidate();viewport.current?.clearCommandPreview();
      assemblyPreviewActor.current?.send({type:"REQUEST",sequence});
      assemblyPreviewActor.current?.send({type:"REJECT",sequence,error:"STALE_PREVIEW_BASE：文档基线已变化；草稿已保留，请确认当前基线后重新预览。"});return;
    }
    if (missingAngleAxis || !assemblySupportsReady || !editingView || !kind || (!editingGroup&&references.length < (kind === "fix" ? 1 : 2)) || replacingAssemblyReference !== undefined) {
	  assemblyPreviewSequence.current+=1;assemblyPreviewAbort.current?.abort();viewport.current?.clearCommandPreview();
	  assemblyPreviewID.current=undefined;
      assemblyPreviewActor.current?.send({ type: "RESET" });
      return;
    }
    const sequence=++assemblyPreviewSequence.current;
    const bindingGeneration=assemblyCandidate.current.begin();
	assemblyPreviewID.current=undefined;
    setAssemblyPreviewEvaluation(undefined);
    assemblyPreviewActor.current?.send({ type: "REQUEST", sequence });
    assemblyPreviewAbort.current?.abort();
    const controller=new AbortController();assemblyPreviewAbort.current=controller;
    const timer = window.setTimeout(() => {
      const commandInput = assemblyEditIntent(kind.toUpperCase(),assemblyConstraintForm.getFieldsValue(true),references,lengthUnit,
        constraint && {...constraint,angleAxis:resolvedAngleAxis}, {angleRelation:pending?.angleRelation,angleAxis:resolvedAngleAxis,
          reverseAngleAxis:pending?.reverseAngleAxis,angleReferenceDirection:pending?.angleReferenceDirection,fixMode:pending?.fixMode});
      const key=assemblyIntentKey(commandInput,editingView.document.id,editingView.document.versionId,activeInstancePath);
	  void api.previewCommand(editingView.document.id, {...commandInput,
        interactionId:assemblyInteractionID.current,previewSequence:sequence},controller.signal).then((preview) => {
		if (!controller.signal.aborted&&sequence===assemblyPreviewSequence.current) {
          if(preview.baseVersionId!==editingView.document.versionId) {
            assemblyPreviewActor.current?.send({type:"REJECT",sequence,error:"STALE_PREVIEW_BASE：预览基线已变化；草稿已保留，请重新预览。"});return;
          }
		  assemblyPreviewID.current=preview.previewId;
          if(!assemblyCandidate.current.resolve(bindingGeneration,key,preview.previewId,commandInput)) {
            assemblyPreviewActor.current?.send({type:"REJECT",sequence,error:"预览没有可提交候选，请修正输入并重新预览。"});return;
          }
          setAssemblyPreviewEvaluation(preview.constraintEvaluation);
          if (preview.instancePoses) viewport.current?.previewAssemblyPoses(preview.instancePoses);
          assemblyPreviewActor.current?.send({ type: "RESOLVE", sequence, components: preview.assemblyComponents, definitionOnly:preview.evaluationOutcome==="DEFINITION_ONLY" });
        }
      }).catch((cause: unknown) => {
        if (controller.signal.aborted) {
          assemblyPreviewActor.current?.send({ type: "CANCEL", sequence });
          return;
        }
        const error = cause instanceof Error ? cause : new Error(String(cause));
        const apiError = cause instanceof ApiError || cause instanceof RealtimeError ? cause : undefined;
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
        if (sequence === assemblyPreviewSequence.current) assemblyPreviewActor.current?.send({ type: "REJECT", sequence, error: error.message,
          errorCode: apiError?.code, phase: apiError?.phase, retryable: apiError?.retryable });
      });
    }, 140);
    return () => {window.clearTimeout(timer);controller.abort();assemblyCandidate.current.invalidate();assemblyPreviewActor.current?.send({type:"CANCEL",sequence});};
  }, [editingView, editingAssemblyConstraint, pendingAssemblyConstraint, replacingAssemblyReference,
    assemblyDirection, assemblyDistance, assemblyPreviewCommit, assemblyConstraintForm, assemblyPreviewActor, lengthUnit,assemblySupportsReady,editingGroup,activeInstancePath,assemblyInspection,axisInspection]);

  const editSketch = async (featureID: string, operations: SketchOperation[],intent?:SketchCommitIntent):Promise<DocumentView> => {
    const owner=editingView;
    if(!owner||store.activeSketchID!==featureID)throw new Error("草图编辑会话已结束");
    if(!intent?.retryReceipt&&intent?.baseVersionId&&intent.baseVersionId!==owner.document.versionId)throw new Error("草图版本已变化，请重新确认操作");
    const issue=operations.flatMap(operation=>operation.type==="ADD_EXTERNAL_GEOMETRY"||operation.type==="RECONNECT_EXTERNAL_GEOMETRY"?[topologyNamingIssue(operation.geometryKey,owner,view)].filter(Boolean):[])[0];
    if(issue)throw new Error(issue.diagnostic);
    if(!operations.length)throw new Error("没有有效的草图编辑");
    const updated=await command.mutateAsync(()=>api.command(owner.document.id,{type:"EDIT_SKETCH",sketchId:featureID,operations,requestId:intent?.requestId??randomUUID()}));
    if(newSketchSession.current?.sketchId===featureID)newSketchSession.current.edited=true;
    await refresh(updated);
    return updated;
  };

  const moveInstance = async (ownerDocumentId:string,candidate:AssemblyInteractionCommit) => {
    if(!candidate.commitCommand)throw new Error("装配操纵缺少完整候选提交身份");
    // Server binds the exact final target/token/requestId. Do not reconstruct a
    // superficially equivalent MOVE or lose its idempotent receipt identity.
    await moveCommand.mutateAsync(()=>api.command(ownerDocumentId,candidate.commitCommand!));
  };
  const executeHistory = (direction: "undo" | "redo") => {
    if (!editingView) return; command.mutate(() => direction === "undo" ? api.undo(editingView.document.id) : api.redo(editingView.document.id));
  };
  const openAssemblyConstraintEditor = (constraint: AssemblyConstraint, reconnect = false) => {
    assemblyDialogLifecycle.current.invalidate();
    assemblyCandidate.current.invalidate();assemblyPreviewSequence.current++;assemblyPreviewAbort.current?.abort();assemblyPreviewID.current=undefined;
    assemblyDraftBase.current=editingView ? {documentId:editingView.document.id,revision:editingView.document.versionId} : undefined;
    constraint = assemblyPublicEditDraft(constraint);
    assemblyInteractionID.current = randomUUID();
    assemblyPreviewActor.current?.send({ type: "START" });
    setEditingAssemblyConstraint({ ...constraint, angleRelation:constraint.kind === "ANGLE" ? constraint.angleRelation ?? "FREE" : undefined });
    setAssemblyDefinitionDirty(false);
    setAssemblyPreviewEvaluation(undefined);
    setReconnectError(undefined);
    setReplacingAssemblyReference(reconnect ? firstDisconnectedSupport(constraint) : undefined);
    if (reconnect) { store.setSelection(null); store.setActiveTool("select", "once"); }
    const instance = editingView?.product?.instances.find(value=>value.id===constraint.first?.instanceId);
    const fixedPose = constraint.fixedPose ?? {translation:instance?.translation ?? [0,0,0],rotation:instance?.rotation ?? [0,0,0,1]};
    assemblyConstraintForm.setFieldsValue({
      ...assemblyPublicInitialFields(constraint.kind,[constraint.first,...(constraint.second?[constraint.second]:[])],constraint),
      fixedTranslation: fixedPose.translation.map(v=>millimetersToDisplayLength(v,lengthUnit)),
      fixedAngles: fixedPoseAngles(fixedPose),
      value: constraint.kind === "ANGLE" ? (constraint.value ?? 0) * 180 / Math.PI
        : constraint.kind === "DISTANCE" ? millimetersToDisplayLength(constraint.value ?? 0, lengthUnit) : constraint.value ?? 0,
      directionRelation: constraint.directionRelation ?? "UNORIENTED",
      distanceRelation: constraint.distanceRelation ?? "UNSIGNED",
      ...(constraint.kind === "DISTANCE" ? offsetInitialFields(constraint) : {}),
      ...(constraint.kind === "ANGLE" ? assemblyQuantityInitialFields(constraint) : {}),
    });
  };
  const stopConflictAnalysis=()=>{
    conflictGate.current.invalidate();conflictAbort.current?.abort();
    if(conflictPending)setConflictError("分析已取消；未完成的探测不形成数值结论。");
    setConflictPending(false);
  };
  const analyzeConflicts=async(ids?:string[])=>{
    if(!editingView?.product)return;
    conflictAbort.current?.abort();
    const abort=new AbortController();conflictAbort.current=abort;
    const owner=editingView.document.id,revision=editingView.document.versionId;
    const generation=conflictGate.current.begin(owner,revision);
    const selectedIds=ids??store.selections.flatMap(s=>s.kind==="assembly-constraint"?[s.constraintId]:[]);
    setConflictPending(true);setConflictError(undefined);setConflictContextCurrent(false);
    try{
      const report=await api.analyzeAssemblyConflicts(owner,{analysisId:randomUUID(),baseRevisionId:revision,
        ...(selectedIds.length?{targetConstraintIds:selectedIds}:{})},abort.signal);
      if(conflictGate.current.accepts(generation,report)){setConflictReport(report);setConflictContextCurrent(true);}
      else if(!abort.signal.aborted)setConflictError("响应版本已失效，请重新分析当前装配。");
    }catch(error){if(!abort.signal.aborted){setConflictError(error instanceof Error?error.message:String(error));operationFeedback(error,"装配检查");}}
    finally{if(conflictAbort.current===abort)setConflictPending(false);}
  };
  const locateConflictMember=(member:AssemblyConflictMember,revisionId:string)=>{
    if(!editingView||revisionId!==editingView.document.versionId)return;
    const definition=editingView.product?.constraints?.find(c=>c.id===member.constraintId);
    if(!definition)return;
    store.setSelections([{kind:"assembly-constraint",id:definition.id,constraintId:definition.id,
      constraintType:definition.kind,documentId:editingView.document.id,occurrencePath:activeInstancePath}]);
    viewport.current?.focusAssemblyReference(member.first,activeInstancePath);
  };
  const repairConflictMember=(member:AssemblyConflictMember,action:AssemblyConflictRepair,revisionId:string)=>{
    if(!canEdit||!editingView||revisionId!==editingView.document.versionId)return;
    const definition=editingView.product?.constraints?.find(c=>c.id===member.constraintId);if(!definition)return;
    if(action==="EDIT"||action==="RECONNECT"){openAssemblyConstraintEditor(definition,action==="RECONNECT");return;}
    command.mutate(()=>api.command(editingView.document.id,{type:"SET_ASSEMBLY_CONSTRAINT_STATE",constraintIds:[definition.id],
      ...(action==="SUPPRESS"?{suppressed:true}:{constraintMode:"MEASURED"})}));
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
    const sourceNode = findStructureEntity(sourceView.structureTree, featureID);
    const node = activeInstancePath ? findStructureOccurrenceEntity(view?.structureTree, featureID,
      activeInstancePath, sourceNode?.bodyId) : sourceNode;
    if (!node) return;
    const selection = structureSelection(node, activeInstancePath && view ? view : sourceView);
    if (!selection) return;
    store.setSelection(selection);
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
    if (selectedNamingIssue) { message.warning(selectedNamingIssue.diagnostic); return; }
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
		  topologyId: selection.topologyId, versionId: selection.versionId ?? editingView.document.versionId,
          bodyId: workingBodyID ?? selection.bodyId }), { onSuccess: (updated) => {
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
    command.mutate(() => api.createSketch(editingView.document.id, { plane: datum.plane, datumPlaneId: datum.id,
      bodyId: workingBodyID }), { onSuccess: (updated) => {
      const sketch = [...(updated.part?.features ?? [])].reverse().find((feature) => feature.type.toUpperCase() === "SKETCH");
      if (sketch) {
        newSketchSession.current = { documentId: updated.document.id, sketchId: sketch.id, edited: false };
        store.beginSketch(sketch.id, plane);
      }
    }});
  };
  const padSketch = (values: { generator: "LINEAR_EXTRUDE" | "REVOLVE"; operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT";
    bodyId: string; lengthSource: string; angle: number; axisEntityId?: string; reversed: boolean }) => {
    if (!editingView || !padSketchID) return;
    padPreviewAbort.current?.abort();
    viewport.current?.clearCommandPreview();
    const generator = values.generator ?? padGenerator;
    const lengthInput = generator === "LINEAR_EXTRUDE" ? linearExtrudeLengthInput(values.lengthSource, lengthUnit) : {};
    command.mutate(() => api.createSolidFeature(editingView.document.id, { sketchId: padSketchID, generator,
      operation: values.operation, bodyId: values.bodyId, ...lengthInput,
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
	setPadPreviewPlacement(undefined);
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
		featureForm.setFieldsValue({lengthText:parameter?parameterSourceText(parameter, lengthUnit):formatDisplayNumber(millimetersToDisplayLength(feature.length??0, lengthUnit))}); featurePreviewID.current=undefined;
		featureInteractionID.current=randomUUID();
		setFeaturePreviewError(undefined); setEditingExtrude({feature,digest:node.definitionDigest});
	};
	const requestFeaturePreview = async () => {
		if (!editingView || !editingExtrude) return;
		let lengthInput: {length?:number;lengthExpression?:string};
		try {
			const values=await featureForm.validateFields(); lengthInput=linearExtrudeLengthEditInput(values.lengthText,lengthUnit,editingExtrude.feature.length??0,editingView.part?.parameters?.find(parameter=>parameter.parameterId===`parameter:${editingExtrude.feature.id}:length`));
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
			const values=await featureForm.validateFields(); lengthInput=linearExtrudeLengthEditInput(values.lengthText,lengthUnit,editingExtrude.feature.length??0,editingView.part?.parameters?.find(parameter=>parameter.parameterId===`parameter:${editingExtrude.feature.id}:length`));
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
    setPadPreviewPlacement(undefined);
    viewport.current?.clearCommandPreview();
    setPadPreviewPending(true);
    try {
      const preview = await api.previewCommand(editingView.document.id, { type: "CREATE_SOLID_FEATURE", sketchId: sketchID,
        generator: values.generator, operation: values.operation, bodyId: values.bodyId,
        ...lengthInput, angle: values.angle,
        axisEntityId: values.axisEntityId, reversed: values.reversed,
        ...(padIntentRequestID.current ? { requestId: padIntentRequestID.current } : {}) }, abort.signal);
	  if (sequence !== padPreviewSequence.current || preview.baseVersionId !== baseVersionID ||
		preview.baseVersionId !== latestDocumentVersion.current || !preview.artifact) return;
	  padPreviewID.current=preview.previewId;
      setPadPreviewPlacement({ bodyId: preview.resultBodyId ?? preview.artifact.bodyId ?? "",
        bodyName: preview.resultBodyName ?? "", assignment: preview.bodyAssignment });
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
    const selectedOperation = operation ?? "ADD";
    const sketchID = store.selection.id;
	const sketchBodyID = editingView?.part?.features.find((feature) => feature.id === sketchID && feature.sketch)?.bodyId;
	padIntentRequestID.current = randomUUID(); padPreviewID.current=undefined; setPadSketchID(sketchID); setPadGenerator(generator);
    padForm.setFieldsValue({ generator, operation: selectedOperation, bodyId: sketchBodyID ?? workingBodyID ?? "", lengthSource: "40", angle: 360,
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
    const candidates = [...new Map(nodes.filter((node) => node.entityId && node.kind && node.capabilities?.includes("DELETE") &&
      node.presentationRole !== "INPUT_REFERENCE").map((node) => [`${node.documentId}:${node.kind}:${node.entityId}`, node])).values()];
    const selectedFeatures = new Set(candidates.filter((node) => !["SKETCH_ENTITY", "SKETCH_CONSTRAINT", "ASSEMBLY_CONSTRAINT", "INSTANCE"].includes(node.kind!))
      .map((node) => node.entityId));
    const featureOrder = new Map((editingView.part?.features ?? []).map((feature, index) => [feature.id, index]));
    const targets = candidates.filter((node) => !node.ownerEntityId || !selectedFeatures.has(node.ownerEntityId)).sort((left, right) => {
      const rank = (node: SpecificationTreeNode) => node.kind === "SKETCH_CONSTRAINT" ? 0 : node.kind === "SKETCH_ENTITY" ? 1 : 2;
      const difference = rank(left) - rank(right); if (difference) return difference;
      return (featureOrder.get(right.entityId!) ?? 0) - (featureOrder.get(left.entityId!) ?? 0);
    });
    if (!targets.length) return;
    const ownerIDs = new Set(targets.map((node) => node.kind === "INSTANCE" ? node.ownerDocumentId : node.documentId));
    if (ownerIDs.size !== 1 || !ownerIDs.has(editingView.document.id)) {
      message.warning("请先激活同一所有者文档，再删除这些对象。");
      return;
    }
    if (targets.length === 1) {
      const node = targets[0], id = node.entityId!, documentId = editingView.document.id;
      if (node.kind === "PUBLICATION") { command.mutate(() => api.deletePublication(documentId, id)); return; }
      if (node.kind === "PARAMETER") { command.mutate(() => api.command(documentId, {type:"DELETE_PARAMETER",parameterId:id})); return; }
      if (node.kind === "PRODUCT_PUBLICATION") { command.mutate(() => api.deleteProductPublication(documentId, id)); return; }
      if (node.kind === "CONTEXT_INPUT") { command.mutate(() => api.deleteContextInput(documentId, id)); return; }
      if (node.kind === "CONTEXT_BINDING") { command.mutate(() => api.deleteContextBinding(documentId, id)); return; }
    }
    if (targets.some((node) => ["PUBLICATION", "PRODUCT_PUBLICATION", "PARAMETER", "CONTEXT_INPUT", "CONTEXT_BINDING"].includes(node.kind ?? ""))) {
      message.warning("请分别删除 Publication、上下文输入和绑定。");
      return;
    }
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
    const deletionNodes=deletableTreeNodesForSelections(treeNodes,store.selections);
    const disposers = [
      commandRegistry.register({id:"edit.delete",execute:()=>deleteTreeNodes(deletionNodes),isEnabled:()=>Boolean(canEdit&&!command.isPending&&store.activeToolID==="select"&&deletionNodes.some(node=>node.capabilities?.includes("DELETE")))}),
      commandRegistry.register({ id: "tool.select", execute: () => store.setActiveTool("select", "once"),
        isActive: () => store.activeToolID === "select" }),
      commandRegistry.register({ id: "assembly.move", execute: () => store.setActiveTool("assembly.move", "continuous"),
        isVisible: () => view?.document.type === "PRODUCT", isEnabled: () => Boolean(canEditRoot), isActive: () => store.activeToolID === "assembly.move" }),
      commandRegistry.register({id:"assembly.analyze",execute:()=>setConflictOpen(true),
        isVisible:()=>view?.document.type==="PRODUCT",isEnabled:()=>Boolean(editingView?.product),isActive:()=>conflictOpen}),
      commandRegistry.register({id:"assembly.move-receipt",execute:()=>viewport.current?.retryAssemblyMoveCommit(),
        isVisible:()=>Boolean(moveReceiptPending),isEnabled:()=>Boolean(moveReceiptPending)}),
      commandRegistry.register({ id: "sketch.start", execute: startSketch,
		isVisible: () => editingView?.document.type === "PART", isEnabled: () => Boolean(canEdit && !selectedNamingIssue && (["plane", "sketch", "face"].includes(store.selection?.kind ?? ""))) }),
      commandRegistry.register({ id: "view.normal", execute: normalToSelection,
        isEnabled: () => Boolean(store.sketchPlane || store.selection && ["plane","face"].includes(store.selection.kind)) }),
      commandRegistry.register({ id: "sketch.finish", execute: finishSketch,
        isVisible: () => Boolean(store.sketchPlane), isEnabled: () => Boolean(canEdit && !command.isPending) }),
      ...sketchToolCommands.map((toolID)=>commandRegistry.register({id:toolID,execute:(invocation)=>store.setActiveTool(toolID,defaultSketchToolMode(toolID,invocation?.continuous)),
        isVisible:()=>Boolean(store.sketchPlane),isEnabled:()=>Boolean(canEdit&&store.sketchPlane&&(toolID!=="sketch.project"||!selectedNamingIssue)),isActive:()=>store.activeToolID===toolID})),
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
      commandRegistry.register({ id: "product.pattern", execute: () => setPatternOpen(true),
        isVisible: () => editingView?.document.type === "PRODUCT", isEnabled: () => Boolean(canEdit) }),
      commandRegistry.register({ id: "product.release", execute: () => setReleaseOpen(true), isVisible: () => view?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEditRoot) }),
      ...(["fix", "rigid", "coincident", "concentric", "angle", "parallel", "perpendicular", "distance"] as const).map((constraint) => commandRegistry.register({
        id: `assembly.${constraint}`,
        execute: (invocation) => store.setActiveTool(`assembly.${constraint}`, invocation?.continuous ? "continuous" : "once"),
        isVisible: () => editingView?.document.type === "PRODUCT",
        isEnabled: () => Boolean(canEdit && (["fix", "rigid"].includes(constraint) || !selectedNamingIssue)),
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
    command.isPending, assemblyConstraintForm, selectedNamingIssue,conflictOpen,moveReceiptPending,treeNodes,store.selections]);

  useEffect(() => { commandRegistry.notifyStateChanged(); }, [commandRegistry, editingView, store.selection, store.sketchPlane,
    store.activeToolID, command.isPending]);

  const selected = selectedFeature(editingView ?? {} as DocumentView, store.selection);
  const publicationTarget = () => {
    if (selectedNamingIssue) return;
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
      const bodyId = selection.bodyId ?? selection.entityId;
      if (bodyId && editingView.part.bodies.some((body) => body.id === bodyId))
        return { publicationType: "BODY", targetKind: "BODY", targetId: bodyId };
    }
    if ((selection.kind === "pad" || selection.kind === "import") && selection.entityId &&
      editingView.part.features.some((feature) => feature.id === selection.entityId && isSolidFeature(feature)))
      return {publicationType:"BODY",targetKind:"FEATURE_OUTPUT",targetId:selection.entityId};
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
    command.mutate(() => api.createPublication(editingView.document.id, {
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
  const redirectProductPublicationToSelection = (publicationId: string) => {
    const source = store.selection;
    if (!editingView?.product || source?.kind !== "publication" || !source.publicationId || !source.instancePath?.segments.length) {
      message.warning("请先选择当前 Product 实例中的来源 Publication"); return;
    }
    command.mutate(() => api.command(editingView.document.id, {type:"REDIRECT_PRODUCT_PUBLICATION",
      publicationId, targetId:source.publicationId, instancePath:source.instancePath}));
  };
  const openPublicationEditor = (publicationID: string, kind: "PART" | "PRODUCT") => {
    const publication = kind === "PART" ? editingView?.part?.publications?.find((item) => item.id === publicationID)
      : editingView?.product?.publications?.find((item) => item.id === publicationID);
    if (!publication) return;
    publicationEditForm.setFieldsValue({name:publication.name,semanticPurpose:publication.semanticPurpose ?? ""});
    setEditingPublication({id:publicationID,kind});
  };
  const commitPublicationEdit = async () => {
    if (!editingView || !editingPublication) return;
    const values = await publicationEditForm.validateFields();
    command.mutate(() => editingPublication.kind === "PART"
      ? api.editPublication(editingView.document.id, editingPublication.id, values)
      : api.editProductPublication(editingView.document.id, editingPublication.id, values.name, values.semanticPurpose),
      {onSuccess:()=>setEditingPublication(undefined)});
  };
  const openParameterEditor = (parameterID: string) => {
	const parameter = editingView?.part?.parameters?.find((candidate) => candidate.parameterId === parameterID);
	if (!parameter) return;
	parameterForm.setFieldsValue({key:parameter.displayAlias ?? "",source:parameterSourceText(parameter, isLengthParameter(parameter) ? lengthUnit : parameter.displayUnit)}); setEditingParameterID(parameterID);
  };
  const commitParameterEdit = async () => {
	if (!editingView || !editingParameterID) return;
	const values = await parameterForm.validateFields();
	const current = editingView.part?.parameters?.find((candidate) => candidate.parameterId === editingParameterID);
    if(current?.role==="MEASURED"||current?.source.external){
      await command.mutateAsync(()=>api.command(editingView.document.id,{type:"RENAME_PARAMETER",parameterId:editingParameterID,name:values.key.trim()||current.key}));
      setEditingParameterID(undefined);return;
    }
    if(current&&values.source===parameterSourceText(current,isLengthParameter(current)?lengthUnit:current.displayUnit)){
      const name=values.key.trim()||current.key;
      if(name!==(current.displayAlias??current.key))await command.mutateAsync(()=>api.command(editingView.document.id,{type:"RENAME_PARAMETER",parameterId:editingParameterID,name}));
      setEditingParameterID(undefined);return;
    }
	const source = parseParameterSource(current?parameterEditSource(current,values.source,isLengthParameter(current)?lengthUnit:current.displayUnit):values.source, current && isLengthParameter(current) ? lengthUnit : current?.displayUnit);
	command.mutate(() => api.command(editingView.document.id, {type: "EDIT_PARAMETER", parameterId: editingParameterID,
		name: values.key.trim() || current?.key, ...(source.kind === "LITERAL" ? { value: source.value, unit: source.unit } : { expression: source.expression })}),
		{onSuccess:()=>setEditingParameterID(undefined)});
  };
  const editingParameterDefinition=editingView?.part?.parameters?.find(parameter=>parameter.parameterId===editingParameterID);
  const parameterSourceReadonly=editingParameterDefinition?.role==="MEASURED"||Boolean(editingParameterDefinition?.source.external);
  const endInteractionForActivation = () => {
    viewport.current?.cancelAssemblyInteraction();stopConflictAnalysis();
    assemblyDialogLifecycle.current.invalidate();
    padPreviewAbort.current?.abort(); padPreviewSequence.current += 1; padPreviewID.current = undefined;
    featurePreviewAbort.current?.abort(); featurePreviewSequence.current += 1; featurePreviewID.current = undefined;
    assemblyPreviewActor.current?.send({ type: "CANCEL", sequence: assemblyPreviewSequence.current });
    assemblyPreviewAbort.current?.abort(); assemblyPreviewSequence.current += 1; assemblyPreviewID.current = undefined;
    viewport.current?.clearCommandPreview();
    store.endSketch(); store.setSelection(null);
  };
  const activateDocumentNode = async (node: SpecificationTreeNode) => {
    if (!view || !node.documentId || !node.documentType ||
        !["PART", "PRODUCT", "INSTANCE"].includes(node.kind ?? "")) return;
    const generation = activationGate.current.begin();
    if (!node.instancePath?.canonical && node.documentId === view.document.id &&
        (node.kind === "PART" || node.kind === "PRODUCT")) {
      endInteractionForActivation();
      activationGate.current.commit(generation, rootEditSession(view, generation), setEditSession);
      return;
    }
    try {
      const prepared = await prepareOccurrenceEditSession({
        host: view,
        node: { kind: node.kind as "PART" | "PRODUCT" | "INSTANCE", documentId: node.documentId,
          documentType: node.documentType as "PART" | "PRODUCT", instancePath: node.instancePath },
        activationGeneration: generation,
        getDesignSession: api.getProductDesignSession,
        getDocument: (id) => client.fetchQuery({ queryKey: queryKeys.document(id), queryFn: () => api.getDocument(id) }),
      });
      if (!activationGate.current.isCurrent(generation)) return;
      client.setQueryData(queryKeys.document(prepared.targetView.document.id), prepared.targetView);
      endInteractionForActivation();
      activationGate.current.commit(generation, prepared.session, setEditSession);
    } catch (error) {
      if (activationGate.current.isCurrent(generation)) {
        message.error(`无法进入上下文编辑：${error instanceof Error ? error.message : String(error)}`);
      }
    }
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
  /> : assemblyPreviewSnapshot.matches("definitionReady") ? <Alert type="warning" showIcon message="计算未完成，可确认保存为待更新" /> : motionComponents?.length ? <div aria-label="装配求解结果">
    {motionComponents.map(component => <div key={component.componentId}>
      <Typography.Text type="secondary">剩余相对自由度：{component.relativeDof}；整体自由度：{component.gaugeDof}</Typography.Text>
      {component.preference.bodies.filter(body => body.role === 1).map(body => <div key={body.bodyId}>
        第二元素变化：{formatDisplayNumber(body.translation)} mm / {formatDisplayNumber(body.rotation * 180 / Math.PI)}°
      </div>)}
      {component.freedoms.filter(freedom => !freedom.relativeToBodyId || freedom.bodyId !== freedom.relativeToBodyId).map(freedom => <div key={freedom.bodyId}>
        {editingView?.product?.instances.find(instance => instance.id === freedom.bodyId)?.name ?? freedom.bodyId}：
        {freedomNames[freedom.kind]}（{freedom.translationDof} 平移 / {freedom.rotationDof} 转动）
        {freedom.relativeToBodyId && `，相对 ${editingView?.product?.instances.find(instance => instance.id === freedom.relativeToBodyId)?.name ?? freedom.relativeToBodyId}`}
      </div>)}
    </div>)}
  </div> : undefined;
  const assemblyPreviewFeedback = <>
    {publicKind==="ANGLE"&&angleRelation==="DIRECTED"&&angleAxisQuery.isPending&&angleAxis&&<Alert type="info" message="正在解析参考轴的精确方向"/>}
    {publicKind==="ANGLE"&&angleRelation==="DIRECTED"&&angleAxisQuery.error&&<Alert type="error" message="参考轴查询失败" description={String(angleAxisQuery.error)}/>}
    {publicKind==="ANGLE"&&angleRelation==="DIRECTED"&&axisInspection?.supports.slice(1).filter(s=>s.status==="RESOLVED"&&s.constraintEligible!==true).map((support,index)=><Alert key={`axis-contract-${index}`} type="warning" message={support.constraintDiagnosticCode ?? "参考轴的约束适用性尚未确认"} description={support.constraintDiagnostic ?? "请选择适用的精确工程子元素。"}/>)}
    {!editingGroup && (assemblySupportQuery.isPending || assemblyCapabilityQuery.isPending) && <Alert type="info" message="正在解析精确支持及约束能力" />}
    {!editingGroup && (assemblySupportQuery.error || assemblyCapabilityQuery.error) && <Alert type="error" message="精确支持查询失败" description={String(assemblySupportQuery.error ?? assemblyCapabilityQuery.error)} />}
    {!editingGroup && assemblyInspection?.supports.filter(s=>s.status!=="RESOLVED").map((support,index)=><Alert key={index} type="error" message={support.diagnosticCode ?? "支持元素无法解析"} description={support.diagnostic} />)}
    {!editingGroup && assemblyTargetSupports?.filter(s=>s.status==="RESOLVED"&&s.constraintEligible!==true).map((support,index)=><Alert key={`contract-${index}`} type="warning" message={support.constraintDiagnosticCode ?? "精确支持的约束适用性尚未确认"} description={support.constraintDiagnostic ?? "请明确选择 Underlying Circle 或适用的工程子元素；查询未确认时不能预览或提交。"}/>)}
    {!editingGroup && assemblyExactTypes?.length && !assemblySupportsReady && assemblyInspection?.supports.every(s=>s.status==="RESOLVED") && <Alert type="warning" message="当前精确支持与关系不兼容" description="请选择能力目录允许的关系，或明确选择工程子元素；服务端仍将验证最终提交。" />}
    {assemblyPreviewEvidence}
    {assemblyPreviewFailed && <Button onClick={async()=>{
      const sequence=assemblyPreviewSequence.current;
      await refresh();
      if(sequence!==assemblyPreviewSequence.current)return;
      const current=client.getQueryData<DocumentView>(queryKeys.document(activeID));
      if(!current)return;
      assemblyDraftBase.current={documentId:current.document.id,revision:current.document.versionId};
      invalidateAssemblyDefinition();
    }}>使用当前基线重新预览（保留草稿）</Button>}
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
        sketchReceipt={sketchReceipt} onSketchReceiptCheck={()=>void viewport.current?.retrySketchReceipt()} sketchCommand={store.activeSketchID?sketchCommandState:undefined} onSketchAction={action=>viewport.current?.sketchCommandAction(action)}
        toolName={activeToolName} lengthUnit={lengthUnit} continuous={store.activeToolMode === "continuous"} />}
      tree={<SpecificationTree key={documentID} nodes={treeNodes} selectedKeys={treeKeysForSelections(treeNodes, store.selections)}
            ancestorHintKeys={ancestorHintKeysForSelections(treeNodes, store.selections)}
            selectionToken={selectionSetToken(store.selections)}
            highlightedKey={treeKeyForSelection(treeNodes, store.preselection)}
            editSession={editSession}
            activeDocumentId={activeID}
            activeInstancePath={activeInstancePath}
            workingBodyId={workingBodyID}
            onSelect={(nodes) => {
              const selections = [...new Map(nodes.flatMap((node) => node.selection ? [[selectionKey(node.selection), node.selection] as const] : [])).values()];
              if (!viewport.current?.captureToolSelections(selections)) store.setSelections(selections);
            }}
            onOpenDocumentTab={(node) => {
              const targetDocumentId = node.kind === "INSTANCE" ? node.documentId : node.sourceDocumentId;
              if (targetDocumentId) void openDocumentTab(targetDocumentId, client, api.openDocument, navigate)
                .catch((error: Error) => message.error(`打开文档失败：${error.message}`));
            }}
            onActivate={(node) => {
              if (node.kind === "BODY" && node.documentId === editingView?.document.id && node.bodyId) {
                setEditSession((current) => current ? withWorkingBody(current, node.bodyId!) : current);
                return;
              }
              if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) {
                const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
                if (constraint) openAssemblyConstraintEditor(constraint);
                return;
              }
              if (node.capabilities?.includes("EDIT")) { openFeatureEditor(node); return; }
              if (node.documentId && ["PART", "PRODUCT", "INSTANCE"].includes(node.kind ?? "")) {
                void activateDocumentNode(node); return;
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
              } else if (node.selection?.kind === "sketch-constraint") viewport.current?.editDimension(node.selection);
              else if (node.kind === "PARAMETER" && node.entityId) openParameterEditor(node.entityId);
              else if (node.kind === "PUBLICATION" && node.entityId) openPublicationEditor(node.entityId,"PART");
              else if (node.kind === "PRODUCT_PUBLICATION" && node.entityId) openPublicationEditor(node.entityId,"PRODUCT");
              else openFeatureEditor(node);
            }}
            onRename={(node) => {
              if (!canEdit || node.documentId !== editingView?.document.id || !node.entityId) return;
              const name = node.kind === "BODY" ? editingView.part?.bodies.find((body) => body.id === node.entityId)?.name
                : editingView.part?.features.find((feature) => feature.id === node.entityId)?.name;
              renameForm.setFieldsValue({name:name ?? ""}); setRenameTarget(node);
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
              if (node.kind === "CONTEXT_REFERENCE" && node.entityId && node.documentId) {
                command.mutate(() => api.detachContextReference(node.documentId!, node.entityId!));
                return;
              }
              if (node.kind === "SKETCH_EXTERNAL_GEOMETRY" && node.ownerEntityId && node.entityId)
                editSketch(node.ownerEntityId, [{type:"DETACH_EXTERNAL_GEOMETRY", externalId:node.entityId}]);
            }}
            onRefresh={(node) => {
              if (node.kind === "CONTEXT_REFERENCE" && node.documentId) {
                command.mutate(() => api.updateReferences(node.documentId!));
                return;
              }
              refreshAssemblyConstraint(node.entityId);
            }}
            onHover={(node) => store.setPreselection(node?.selection ?? null)} onDelete={deleteTreeNodes}
            onToggleVisibility={(node,scope,mode)=>{
              if (scope === "DEFINITION" && node.documentId && node.entityId) {
                command.mutate(() => api.command(node.documentId!, {type:"SET_DEFINITION_VISIBILITY",targetKind:node.kind,
                  targetId:node.entityId,ownerEntityId:node.ownerEntityId,visible:!node.localVisible}));
                return;
              }
              if (scope === "OCCURRENCE" && view?.document.type === "PRODUCT" && node.instancePath && node.entityId) {
                command.mutate(() => api.command(view.document.id, {type:"SET_OCCURRENCE_VISIBILITY",instancePath:node.instancePath,
                  targetKind:node.kind,targetId:node.entityId,visibilityMode:mode ?? (node.visibilityMode === "HIDE" ||
                    node.visibilityMode !== "SHOW" && node.localVisible === false ? "SHOW" : "HIDE")}));
                return;
              }
              setTreeVisibility(node.selection ? selectionKey(node.selection) : node.key, Boolean(node.hidden));
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
        onEditParameter={openParameterEditor} onEditPublication={openPublicationEditor}
        navigationProfile={navigationProfile} canRestore={canEdit && !command.isPending}
        onRestore={(entry) => command.mutate(() => api.restore(activeID, entry.versionId))} />}>
        <AssemblyConflictPanel open={conflictOpen} pending={conflictPending} report={conflictReport}
          current={Boolean(conflictContextCurrent&&editingView&&assemblyConflictCurrent(conflictReport,editingView.document.id,editingView.document.versionId))}
          error={conflictError} canEdit={canEdit&&!command.isPending} view={editingView} evidence={engineeringEvidence.data}
          onMotion={showAnalysisMotion}
          ownerOccurrence={activeInstancePath} lengthUnit={lengthUnit}
          onLocateInstance={id=>{if(!editingView?.product?.instances.some(instance=>instance.id===id))return;
            store.setSelections([{kind:"instance",id,instanceId:id,documentId:editingView.document.id,occurrencePath:activeInstancePath?[activeInstancePath,id].join("/"):id}]);
            viewport.current?.focusAssemblyReference({instanceId:id,kind:"BODY"},activeInstancePath);}}
          onAnalyze={ids=>void analyzeConflicts(ids)} onStop={stopConflictAnalysis}
          onClose={()=>{viewport.current?.showRemainingMotion();stopConflictAnalysis();setConflictOpen(false);}} onLocate={locateConflictMember} onRepair={repairConflictMember}/>
        {view.document.type === "PRODUCT" && (productUpdateFailure || productUpdatePlan.data?.hasUpdates && !productUpdatePlan.data.canAccept) && <Alert
          style={{position:"absolute",zIndex:12,top:12,left:"50%",transform:"translateX(-50%)",minWidth:420}}
          type="error" showIcon message="自动跟随最新版本被阻塞"
          description={productUpdateFailure ?? productUpdatePlan.data?.entries.find((entry)=>entry.kind !== "ASSEMBLY_SOLVE" && entry.diagnostic)?.diagnostic ?? "更新计划被上游解析或求值失败阻塞。"}
          action={<Button size="small" onClick={() => { setProductUpdateFailure(undefined); automaticUpdateSignature.current = "";
            void productUpdatePlan.refetch().finally(() => setAutomaticUpdateEpoch((value) => value + 1)); }}>重试</Button>} />}
        {(selectedNamingIssue ?? activeNamingIssue) && <Alert
          style={{position:"absolute",zIndex:12,bottom:12,left:12,maxWidth:520}}
          type="warning" showIcon message="当前几何暂不支持持久拓扑引用"
          description={(selectedNamingIssue ?? activeNamingIssue)?.diagnostic}
          action={repairableImport && canEdit ? <Button size="small" loading={command.isPending} onClick={() => command.mutate(() => api.command(editingView!.document.id, {type:"REPAIR_IMPORT_NAMING",targetId:repairableImport.id}))}>建立导入命名</Button> : undefined} />}
        <Suspense fallback={<div className="viewport-loading"><Spin size="large" /></div>}><CadViewport ref={viewport} view={view}
          editingView={editingView} activeInstancePath={activeInstancePath} activeInstanceTranslation={activeResolvedInstance?.translation}
          liveConstraintProjection={Boolean(canEdit && editSession?.hostDocumentId===view.document.id && activeInstancePath &&
            !pinnedReferenceInPath(view.structureTree,activeInstancePath))}
          activeInstanceRotation={activeResolvedInstance?.rotation}
          activeBodyTreeNodeId={activeResolvedInstance?.bodyTreeNodeId}
          selections={store.selections}
          preselection={store.preselection}
          treeVisibilityOverrides={treeVisibilityOverrides}
          sketchPlane={store.sketchPlane} activeSketchID={store.activeSketchID} activeToolID={store.activeToolID} navigationProfile={navigationProfile} catiaRotationSphereVisible={catiaRotationSphereVisible}
          referenceVisibility={referenceVisibility} solidDisplay={solidDisplay}
          preferredLengthUnit={lengthUnit} captureSettings={captureSettings} onSelectionsChange={store.setSelections} onPreselectionChange={store.setPreselection} onSketchOperations={editSketch} onDimensionOperations={editSketch} onSketchPreview={(featureId,operations,signal)=>{if(!editingView||store.activeSketchID!==featureId)return Promise.reject(new Error("草图预览上下文已结束"));return api.previewCommand(editingView.document.id,{type:"EDIT_SKETCH",sketchId:featureId,operations},signal);}} onSketchReceiptCheck={async receipt=>{const updated=await command.mutateAsync(()=>api.command(receipt.ownerDocumentId,{type:"EDIT_SKETCH",sketchId:receipt.featureId,operations:receipt.operations,requestId:receipt.intent.requestId}));await refresh(updated);return updated;}}
          onSketchReceiptChange={setSketchReceipt} onSketchCommandStateChange={setSketchCommandState} onToolUseComplete={store.completeToolUse} onActiveToolChange={store.setActiveTool}
		  onAssemblyConstraint={(toolKind, references) => {
            const issue = references.flatMap((reference) => {
              if (!["FACE", "EDGE", "VERTEX"].includes(reference.kind)) return [];
              const issue = topologyNamingIssue(reference.geometryKey, editingView, view);
              return issue ? [issue] : [];
            })[0];
            if (issue) { message.warning(issue.diagnostic); return; }
            const { kind, angleRelation } = assemblyConstraintEntry(toolKind);
			if (!editingView) return;
			assemblyDialogLifecycle.current.invalidate();
			assemblyCandidate.current.invalidate();assemblyPreviewSequence.current++;assemblyPreviewAbort.current?.abort();assemblyPreviewID.current=undefined;
			assemblyDraftBase.current={documentId:editingView.document.id,revision:editingView.document.versionId};
			assemblyInteractionID.current=randomUUID();
			assemblyPreviewActor.current?.send({type:"START"});
            setAssemblyDefinitionDirty(true); setReconnectError(undefined); setReplacingAssemblyReference(undefined);
            // Mesh/display measurements are not authoritative parameter or
            // branch defaults. Wait for exact server inspection for sign rules.
            assemblyConstraintForm.setFieldsValue({ value:0, directionRelation:"UNORIENTED", distanceRelation: "UNSIGNED",
              ...(kind === "distance" ? offsetInitialFields(undefined,references) : {}) });
            assemblyConstraintForm.setFieldsValue({...assemblyPublicInitialFields(kind.toUpperCase(),references),...(kind==="angle"?assemblyQuantityInitialFields():{})});
            setPendingAssemblyConstraint({ kind, references,
              angleRelation });
          }}
          onAssemblyInteractionBegin={(ownerDocumentId,input,signal)=>{
            if(!canEditRoot||!editingView)throw new Error("运动所属 Product 不可编辑");
            return api.beginAssemblyInteraction(ownerDocumentId,{...input,editContext:{
              rootDocumentId:view.document.id,rootRevisionId:view.document.versionId,
              activeDocumentId:editingView.document.id,activeRevisionId:editingView.document.versionId,
              instancePath:editSession?.editTarget.instancePath}},signal);
          }}
          onAssemblyInteractionUpdate={(ownerDocumentId,input,signal)=>api.updateAssemblyInteraction(ownerDocumentId,input,signal)}
          onAssemblyInteractionCancel={(ownerDocumentId,sessionId)=>api.cancelAssemblyInteraction(ownerDocumentId,sessionId)}
          onInspectAssemblySupports={(owner,references,signal)=>api.inspectAssemblySupports(owner,references,signal)}
          onOperationFailed={error=>operationFeedback(error,"装配操纵")}
          onAssemblyInteractionState={(state,reason)=>{
            if(state==="blocked"&&reason?.startsWith("移动尚未保存"))setMoveReceiptPending(true);
            if(state==="committing"&&reason?.startsWith("提交结果未知")&&!receiptPrompt.current){
              setMoveReceiptPending(true);
              receiptPrompt.current=true;modal.confirm({title:"移动提交结果待确认",content:"连接中断时移动可能已经保存。请查询原请求回执，勿重复创建移动。",
                okText:"查询 / 重试原提交",cancelText:"稍后",onOk:()=>{receiptPrompt.current=false;viewport.current?.retryAssemblyMoveCommit();},onCancel:()=>{receiptPrompt.current=false;}});
            }
            if(state==="committed"||state==="idle"||state==="failed"||state==="invalidated"){receiptPrompt.current=false;setMoveReceiptPending(false);}
          }}
          onInstanceMoved={moveInstance} /></Suspense>
      <WorkbenchViewControls toolbars={visibleToolbars} />
    </WorkbenchLayout>
    <CommandDialog id="assembly-constraint-edit" open={Boolean(editingAssemblyConstraint)} title="约束定义" size="M"
      onClose={() => { assemblyDialogLifecycle.current.invalidate();assemblyPreviewActor.current?.send({type:"CANCEL",sequence:assemblyPreviewSequence.current});assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined; setAssemblyPreviewEvaluation(undefined); setReplacingAssemblyReference(undefined); setReconnectError(undefined); setAssemblyDefinitionDirty(false); setEditingAssemblyConstraint(undefined); }}
      confirmLoading={command.isPending || assemblyPreviewPending} confirmDisabled={!assemblySupportsReady || !(assemblyPreviewSnapshot.matches("succeeded") || assemblyPreviewSnapshot.matches("definitionReady")) || replacingAssemblyReference !== undefined ||
        Boolean(editingAssemblyConstraint?.kind === "ANGLE" && editingAssemblyConstraint.angleRelation === "DIRECTED" && !editingAssemblyConstraint.angleAxis)} onConfirm={async()=>{
        if(!editingView||!editingAssemblyConstraint)return;
        const constraint=editingAssemblyConstraint, target=editingView.document;
        const intent=assemblyEditIntent(constraint.kind,assemblyConstraintForm.getFieldsValue(true),resolvedAssemblyReferences,lengthUnit,{...constraint,angleAxis:resolvedAngleAxis});
        const input=assemblyCandidate.current.freeze(assemblyIntentKey(intent,target.id,target.versionId,activeInstancePath));
        if(!input){invalidateAssemblyDefinition();return;}
        const sequence=assemblyPreviewSequence.current;
        const dialogGeneration=assemblyDialogLifecycle.current.capture();
        await assemblyConstraintForm.validateFields();
        if(sequence!==assemblyPreviewSequence.current || !(assemblyPreviewActor.current?.getSnapshot().matches("succeeded") || assemblyPreviewActor.current?.getSnapshot().matches("definitionReady")))return;
		assemblyPreviewActor.current?.send({type:"CONFIRM"});
        command.mutate(()=>api.command(target.id,input),{onSuccess:(updated)=>{if(!assemblyDialogLifecycle.current.isCurrent(dialogGeneration))return;assemblyPreviewActor.current?.send({type:"COMMIT_SUCCESS"});if(assemblyPreviewSnapshot.matches("definitionReady"))message.info("约束已保存，等待更新");viewport.current?.clearCommandPreview(false);assemblyPreviewID.current=undefined;setAssemblyDefinitionDirty(false);setReconnectError(undefined);setEditingAssemblyConstraint(undefined);store.setSelection({kind:"assembly-constraint",id:constraint.id,constraintId:constraint.id,constraintType:constraint.kind,documentId:updated.document.id,
          occurrencePath:activeInstancePath??"",instancePath:editSession?.editTarget.instancePath,rootDocumentId:activeInstancePath?view?.document.id:undefined,
          treeNodeId:`${view?.constraintDisplayScopes?.find(scope=>scope.documentId===target.id && scope.instancePath.canonical===activeInstancePath)?.treeNodeId??`document:${updated.document.id}`}/assembly-constraints/constraint:${constraint.id}`});},
		  onError:(cause)=>{if(assemblyDialogLifecycle.current.isCurrent(dialogGeneration))assemblyPreviewActor.current?.send({type:"COMMIT_FAILURE",error:String(cause)});}});
      }}>
      <Form form={assemblyConstraintForm} layout="vertical" onValuesChange={invalidateAssemblyDefinition}>
        {editingAssemblyConstraint?.kind === "FIX" && <Select aria-label="固定基准" value={editingAssemblyConstraint.fixMode ?? "SPACE"}
          options={[{value:"SPACE",label:"空间固定"},{value:"RELATIVE",label:"相对固定"}]}
          onChange={(fixMode)=>{invalidateAssemblyDefinition();setEditingAssemblyConstraint({...editingAssemblyConstraint,fixMode});}} />}
        {editingAssemblyConstraint?.kind === "ANGLE" && <AssemblyAngleParameters value={editingAssemblyConstraint} view={editingView}
          axisSourceType={axisInspection?.supports[0]?.exactType} axisExactType={axisInspection?.supports[1]?.exactType} onDerive={deriveAngleAxis}
          onChange={value=>{invalidateAssemblyDefinition();if(!angleRelationSupportsMeasured(value.angleRelation))assemblyConstraintForm.setFieldValue("constraintMode","DRIVING");if(value.angleRelation === "PERPENDICULAR") assemblyConstraintForm.setFieldValue("directionRelation", assemblyConstraintForm.getFieldValue("directionRelation") === "OPPOSITE" ? "OPPOSITE" : "SAME");setEditingAssemblyConstraint({...editingAssemblyConstraint,...value});}}
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
          {editingAssemblyConstraint.kind === "ANGLE" && (!editingAssemblyConstraint.angleRelation || editingAssemblyConstraint.angleRelation === "DIRECTED" || editingAssemblyConstraint.angleRelation === "FREE") && <Button disabled={command.isPending} onClick={() => {
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
          exactTypes={assemblyExactTypes} sourceTypes={assemblySourceTypes} contactCapabilities={contactCapabilities} onDerive={deriveAssemblyReference}
          references={[editingAssemblyConstraint.first, editingAssemblyConstraint.second]} replacing={replacingAssemblyReference}
          constraint={editingAssemblyConstraint} previewEvaluation={assemblyPreviewEvaluation} dirty={assemblyDefinitionDirty}

		  onValueCommit={invalidateAssemblyDefinition}
          onLocate={(reference)=>{if(!viewport.current?.focusAssemblyReference(reference))setReconnectError("当前支持元素无法在视图区定位。");}}
		  onRefresh={()=>refreshAssemblyConstraint(editingAssemblyConstraint.id)}
		  onReplace={(index)=>{store.setSelection(null);setReconnectError(undefined);setReplacingAssemblyReference(index);store.setActiveTool("select","once");}} />}
        {replacingAssemblyReference !== undefined && <Alert type="info" showIcon message={`Reconnect 支持元素 ${replacingAssemblyReference + 1}`}
          description="在视图区选择新的几何元素；Esc 取消整个编辑会话。" />}
        {reconnectError && <Alert type="error" showIcon message="无法使用该支持元素" description={reconnectError} />}
        {assemblyPreviewFeedback}
      </Form>
    </CommandDialog>
    <CommandDialog id="assembly-constraint-value" open={Boolean(pendingAssemblyConstraint)} title="约束定义" size="M"
      onClose={() => { assemblyDialogLifecycle.current.invalidate();assemblyPreviewActor.current?.send({type:"CANCEL",sequence:assemblyPreviewSequence.current});assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;viewport.current?.clearCommandPreview();assemblyPreviewID.current=undefined; setAssemblyPreviewEvaluation(undefined); setReplacingAssemblyReference(undefined); setReconnectError(undefined); setAssemblyDefinitionDirty(false); setPendingAssemblyConstraint(undefined); }}
      confirmLoading={command.isPending || assemblyPreviewPending} confirmDisabled={!assemblySupportsReady || !(assemblyPreviewSnapshot.matches("succeeded") || assemblyPreviewSnapshot.matches("definitionReady")) || replacingAssemblyReference !== undefined ||
        Boolean(pendingAssemblyConstraint?.kind === "angle" && pendingAssemblyConstraint.angleRelation === "DIRECTED" && !pendingAssemblyConstraint.angleAxis)}
      onConfirm={async () => {
        if (!editingView || !pendingAssemblyConstraint) return;
        const pending = pendingAssemblyConstraint;
        const target=editingView.document;
        const intent=assemblyEditIntent(pending.kind.toUpperCase(),assemblyConstraintForm.getFieldsValue(true),resolvedAssemblyReferences,lengthUnit,undefined,
          {angleRelation:pending.angleRelation,angleAxis:resolvedAngleAxis,reverseAngleAxis:pending.reverseAngleAxis,angleReferenceDirection:pending.angleReferenceDirection,fixMode:pending.fixMode});
        const input=assemblyCandidate.current.freeze(assemblyIntentKey(intent,target.id,target.versionId,activeInstancePath));
        if(!input){invalidateAssemblyDefinition();return;}
        const sequence=assemblyPreviewSequence.current;
        const dialogGeneration=assemblyDialogLifecycle.current.capture();
        await assemblyConstraintForm.validateFields();
        if(sequence!==assemblyPreviewSequence.current || !(assemblyPreviewActor.current?.getSnapshot().matches("succeeded") || assemblyPreviewActor.current?.getSnapshot().matches("definitionReady")))return;
		assemblyPreviewActor.current?.send({type:"CONFIRM"});
        command.mutate(() => api.command(target.id,input), { onSuccess: () => {if(!assemblyDialogLifecycle.current.isCurrent(dialogGeneration))return; assemblyPreviewActor.current?.send({type:"COMMIT_SUCCESS"});if(assemblyPreviewSnapshot.matches("definitionReady"))message.info("约束已保存，等待更新");viewport.current?.clearCommandPreview(false); assemblyPreviewID.current=undefined; setReconnectError(undefined); setAssemblyDefinitionDirty(false); setPendingAssemblyConstraint(undefined); },
		  onError:(cause)=>{if(assemblyDialogLifecycle.current.isCurrent(dialogGeneration))assemblyPreviewActor.current?.send({type:"COMMIT_FAILURE",error:String(cause)});} });
      }}>
      <Form form={assemblyConstraintForm} layout="vertical" onValuesChange={invalidateAssemblyDefinition}>
        {pendingAssemblyConstraint?.kind === "fix" && <Select aria-label="创建固定基准" value={pendingAssemblyConstraint.fixMode ?? "SPACE"}
          options={[{value:"SPACE",label:"空间固定（所属 Product 坐标系）"},{value:"RELATIVE",label:"相对固定（显式移动后更新基准）"}]}
          onChange={fixMode=>{invalidateAssemblyDefinition();setPendingAssemblyConstraint({...pendingAssemblyConstraint,fixMode});}} />}
        {pendingAssemblyConstraint?.kind === "angle" && <AssemblyAngleParameters value={pendingAssemblyConstraint} view={editingView}
          axisSourceType={axisInspection?.supports[0]?.exactType} axisExactType={axisInspection?.supports[1]?.exactType} onDerive={deriveAngleAxis}
          onChange={value=>{invalidateAssemblyDefinition();if(!angleRelationSupportsMeasured(value.angleRelation))assemblyConstraintForm.setFieldValue("constraintMode","DRIVING");if(value.angleRelation === "PERPENDICULAR") assemblyConstraintForm.setFieldValue("directionRelation", assemblyConstraintForm.getFieldValue("directionRelation") === "OPPOSITE" ? "OPPOSITE" : "SAME");setPendingAssemblyConstraint({...pendingAssemblyConstraint,...value});}}
          onPick={()=>{store.setSelection(null);setReplacingAssemblyReference(2);store.setActiveTool("select","once");}} />}
        {pendingAssemblyConstraint && <AssemblyConstraintFields kind={pendingAssemblyConstraint.kind.toUpperCase() as keyof typeof assemblyConstraintUI} view={editingView} lengthUnit={lengthUnit}
          exactTypes={assemblyExactTypes} sourceTypes={assemblySourceTypes} contactCapabilities={contactCapabilities} onDerive={deriveAssemblyReference}
          references={pendingAssemblyConstraint.references} replacing={replacingAssemblyReference}
          previewEvaluation={assemblyPreviewEvaluation} dirty
          angleRelation={pendingAssemblyConstraint.angleRelation}
		  onValueCommit={invalidateAssemblyDefinition}
          onLocate={(reference)=>{if(!viewport.current?.focusAssemblyReference(reference))setReconnectError("当前支持元素无法在视图区定位。");}}
		  onReplace={(index)=>{store.setSelection(null);setReconnectError(undefined);setReplacingAssemblyReference(index);store.setActiveTool("select","once");}} />}
        {replacingAssemblyReference !== undefined && <Alert type="info" showIcon message={`重新选择支持元素 ${replacingAssemblyReference + 1}`}
          description="在视图区选择新的几何元素；Esc 取消整个创建会话。" />}
        {reconnectError && <Alert type="error" showIcon message="无法使用该支持元素" description={reconnectError} />}
        {assemblyPreviewFeedback}
      </Form>
    </CommandDialog>
    <CommandDialog size="S" id="rename-model-object" open={Boolean(renameTarget)} title="重命名建模对象"
      onClose={() => setRenameTarget(undefined)} confirmLoading={command.isPending}
      onConfirm={async () => {
        const values = await renameForm.validateFields();
        if (!renameTarget?.entityId || !editingView || renameTarget.documentId !== editingView.document.id) return;
        const target = renameTarget;
        command.mutate(() => api.command(editingView.document.id, target.kind === "BODY"
          ? {type:"RENAME_BODY",bodyId:target.entityId,name:values.name.trim()}
          : {type:"RENAME_FEATURE",targetId:target.entityId,name:values.name.trim()}),
          {onSuccess:() => setRenameTarget(undefined)});
      }}>
      <Form form={renameForm} layout="vertical"><Form.Item name="name" label="名称" rules={[{required:true,whitespace:true}]}>
        <Input maxLength={160} /></Form.Item></Form>
    </CommandDialog>
    <CommandDialog id="solid-generator" open={padOpen} title="实体特征" onClose={closePad} confirmLoading={command.isPending}
      onConfirm={async () => padSketch(await padForm.validateFields())}>
      <Form form={padForm} layout="vertical"><Form.Item name="generator" hidden><Input /></Form.Item>
        <Form.Item name="operation" label="Body 操作" rules={[{ required: true }]}>
        <Select onChange={(operation) => {
          padForm.setFieldValue("reversed", defaultSolidReversed(padGenerator, operation));
          previewPad();
        }} options={[{ value: "NEW_BODY", label: "新建 Body" }, { value: "ADD", label: "添加材料" },
          { value: "REMOVE", label: "移除材料" }, { value: "INTERSECT", label: "保留交集" }]} /></Form.Item>
        <Form.Item noStyle shouldUpdate={(before, after) => before.operation !== after.operation}>{({ getFieldValue }) =>
          <Form.Item name="bodyId" label="目标 Body" rules={getFieldValue("operation") === "NEW_BODY" ? [] : [{ required: true, message: "请选择目标 Body" }]}>
            <Select disabled={getFieldValue("operation") === "NEW_BODY"} onChange={previewPad}
              options={(editingView?.part?.bodies ?? []).map((body) => ({ value: body.id, label: body.name }))} />
          </Form.Item>}
        </Form.Item>
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
				value: parameter.key, label: `${parameter.qualifiedDisplayPath ?? parameter.displayName ?? parameter.label} · ${parameterDisplayValue(parameter, lengthUnit)}`,
              }))}
              onChange={(key) => { if (key) padForm.setFieldValue("lengthSource", key); previewPad(); }} />
          </Form.Item>
        </>}</Form.Item>
        <Form.Item name="reversed" label="反向" valuePropName="checked"><Switch onChange={previewPad} /></Form.Item>
        <FeaturePreviewLegend operation={padOperation} />
        {padPreviewPlacement?.bodyId && <small className="cad-command-hint">最终归属：{padPreviewPlacement.bodyName || padPreviewPlacement.bodyId}
          {padPreviewPlacement.assignment === "EXPLICIT_NEW_BODY" ? "（显式新建 Body）" : "（现有目标 Body）"}</small>}
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
	<CommandDialog id="parameter-manager" open={parameterManagerOpen} title="参数" size="M"
		onClose={() => setParameterManagerOpen(false)} onConfirm={() => setParameterManagerOpen(false)} confirmText="完成">
		<div className="parameter-manager" aria-label="文档参数">
			<Form form={createUserParameterForm} layout="inline" initialValues={{value:0,unit:"mm"}}>
				<Form.Item name="name"><Input placeholder="别名（可选）" /></Form.Item>
				<Form.Item name="value" rules={[{required:true}]}><InputNumber placeholder="初始值" /></Form.Item>
				<Form.Item name="unit"><Select style={{width:105}} options={[{value:"mm",label:"mm"},{value:"deg",label:"deg"},{value:"",label:"无量纲"}]} /></Form.Item>
				<Form.Item><Button disabled={!canEdit || !editingView?.part} onClick={async()=>{
					const values=await createUserParameterForm.validateFields(); if(!editingView)return;
					command.mutate(()=>api.command(editingView.document.id,{type:"CREATE_PARAMETER",name:values.name?.trim(),
						value:values.value,unit:values.unit}),{onSuccess:()=>createUserParameterForm.resetFields()});
				}}>新建参数</Button></Form.Item>
			</Form>
			<div className="parameter-manager-header"><span>参数 / 别名</span><span>来源</span><span>计算值</span><span /></div>
			{(editingView?.part?.parameters ?? []).length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Part 尚无参数" />
				: (editingView?.part?.parameters ?? []).map((parameter: ParameterDefinition) => <div className="parameter-manager-row" key={parameter.parameterId}>
					<span><strong>{parameter.qualifiedDisplayPath ?? parameter.displayName ?? parameter.label}</strong>
						{parameter.displayAlias && <Typography.Text type="secondary">{parameter.displayAlias}</Typography.Text>}
						<Typography.Text type="secondary" copyable={{text:parameter.parameterId}} title={parameter.parameterId}>技术详情</Typography.Text></span>
					<Space direction="vertical" size={0}><Typography.Text ellipsis={{tooltip:parameterSourceText(parameter)}}>{parameterSourceText(parameter)}</Typography.Text>
						{editingView?.referenceUpdates?.find((item) => item.consumerKind === "EXTERNAL_PARAMETER" && item.consumerId === parameter.parameterId) && ((update) =>
							<Tag color={update.status === "CURRENT" ? "success" : update.status === "UPDATE_AVAILABLE" ? "processing" : "error"}
								title={update.diagnostic}>{update.status}</Tag>)(editingView.referenceUpdates.find((item) => item.consumerKind === "EXTERNAL_PARAMETER" && item.consumerId === parameter.parameterId)!)}</Space>
					<Typography.Text>{parameterDisplayValue(parameter, lengthUnit)}</Typography.Text>
					<Space><Button size="small" disabled={!canEdit} onClick={() => publishParameter(parameter)}>发布</Button>
					<Button size="small" disabled={!canEdit} onClick={() => { externalParameterForm.resetFields(); setExternalParameterID(parameter.parameterId); }}>引用</Button>
					<Button size="small" disabled={!canEdit} onClick={() => openParameterEditor(parameter.parameterId)}>编辑</Button>
					<Button size="small" danger disabled={!canEdit || parameter.lifecycle !== "USER"}
						title={parameter.lifecycle === "SKETCH_DIMENSION" ? "请通过尺寸约束删除" : parameter.lifecycle !== "USER" ? "Feature 必需参数不能独立删除" : undefined}
						onClick={() => command.mutate(() => api.command(editingView!.document.id, {type:"DELETE_PARAMETER",parameterId:parameter.parameterId}))}>删除</Button></Space>
				</div>)}
			<small className="cad-command-hint">表达式使用可读别名输入，提交后绑定稳定 ParameterId；重命名别名不会断开已有引用。</small>
		</div>
	</CommandDialog>
	<CommandDialog id="publication-manager" open={publicationManagerOpen} title="Publications" size="L"
		onClose={() => setPublicationManagerOpen(false)} onConfirm={() => setPublicationManagerOpen(false)} confirmText="完成">
		<Form form={publicationForm} layout="inline" initialValues={{ name: "", semanticPurpose: "" }}>
			<Form.Item name="name"><Input placeholder="自动命名（可选）" /></Form.Item>
			<Form.Item name="semanticPurpose"><Input placeholder="语义用途" /></Form.Item>
			<Form.Item><Button type="primary" disabled={!canEdit || (editingView?.document.type === "PART" ? !publicationTarget() : !store.selection?.publicationId)} loading={command.isPending}
				onClick={() => void (editingView?.document.type === "PRODUCT" ? forwardSelectedPublication() : createSelectedPublication())}>
				{editingView?.document.type === "PRODUCT" ? "转发所选子 Publication" : "发布当前选择"}</Button></Form.Item>
		</Form>
		<div className="parameter-manager" aria-label="文档 Publications">
			{editingView?.document.type === "PRODUCT" ? ((editingView.product?.publications ?? []).length === 0
				? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Product 尚无转发 Publication" />
				: (editingView.product?.publications ?? []).map((publication) => <div className="parameter-manager-row" key={publication.id}>
					<span><Typography.Text strong>{publication.name}</Typography.Text><Typography.Text type="secondary" copyable={{text:publication.id}}>{publication.id}</Typography.Text></span>
					<span>{publication.type} · {publication.target.instancePath.display}</span>
					<Tag color={publication.resolution.status === "CONNECTED" ? "success" : "error"}>{publication.resolution.status}</Tag>
					<Space><Button size="small" disabled={!canEdit} onClick={() => openPublicationEditor(publication.id,"PRODUCT")}>编辑</Button>
					<Button size="small" disabled={!canEdit} onClick={() => redirectProductPublicationToSelection(publication.id)}>重定向</Button>
					<Button danger size="small" disabled={!canEdit} onClick={() => command.mutate(() => api.deleteProductPublication(editingView.document.id, publication.id))}>删除</Button></Space>
				</div>)) : (editingView?.part?.publications ?? []).length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Part 尚无 Publication" />
				: (editingView?.part?.publications ?? []).map((publication) => <div className="parameter-manager-row" key={publication.id}>
					<span><Typography.Text strong>{publication.name}</Typography.Text><Typography.Text type="secondary" copyable={{text:publication.id}}>{publication.id}</Typography.Text></span>
					<span>{publication.type} · {publication.target.kind}</span>
					<Tag color={publication.resolution.status === "CONNECTED" ? "success" : "error"}>{publication.resolution.status}</Tag>
					<Space><Button size="small" disabled={!canEdit} onClick={() => openPublicationEditor(publication.id,"PART")}>编辑</Button>
					<Button size="small" disabled={!canEdit} onClick={() => redirectPublicationToSelection(publication.id, publication.type)}>重定向</Button>
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
	<CommandDialog size="S" id="parameter-edit" open={Boolean(editingParameterID)} title="编辑参数" onClose={() => setEditingParameterID(undefined)}
		confirmLoading={command.isPending} onConfirm={commitParameterEdit}>
		<Form form={parameterForm} layout="vertical">
			<Form.Item name="key" label="可读别名（可选）" rules={[{pattern:/^$|^[A-Za-z_][A-Za-z0-9_]*$/,
				message:"请输入 ASCII 标识符"}]}><Input /></Form.Item>
			<Form.Item name="source" label="值或表达式" rules={[{required:!parameterSourceReadonly}]}><Input disabled={parameterSourceReadonly} data-quantity-input="true" placeholder="40 或 base_width / 2" /></Form.Item>
            {parameterSourceReadonly&&<small className="cad-command-hint">{editingParameterDefinition?.role==="MEASURED"?"参考尺寸只测量；请在尺寸定义编辑中显式恢复驱动后再改原来源。":"外部来源只读，当前仅编辑名称。"}</small>}
			<small className="cad-command-hint">表达式按当前 Part 的参数别名编辑；提交后 AST 绑定稳定 ParameterId，后续重命名不会破坏引用。</small>
		</Form>
	</CommandDialog>
	<CommandDialog size="S" id="publication-edit" open={Boolean(editingPublication)} title="编辑 Publication"
		onClose={() => setEditingPublication(undefined)} confirmLoading={command.isPending} onConfirm={commitPublicationEdit}>
		<Form form={publicationEditForm} layout="vertical">
			<Form.Item name="name" label="名称" rules={[{required:true}]}><Input /></Form.Item>
			<Form.Item name="semanticPurpose" label="用途"><Input /></Form.Item>
		</Form>
	</CommandDialog>
    {insertOpen && editingView?.document.type === "PRODUCT" && <InsertDocumentDialog key={activeID}
      targetID={activeID} rootID={documentID} busy={command.isPending}
      onClose={() => setInsertOpen(false)}
      onInsertDocuments={(ids) => command.mutateAsync(() => api.insertMany(activeID, ids))} />}
    {patternOpen && editingView?.document.type === "PRODUCT" && <InstancePatternDialog
      sourceInstance={editingView.product?.instances.find((instance) => instance.id ===
        (store.selection?.kind === "instance" ? store.selection.instanceId : undefined))}
      parentOccurrencePath={activeInstancePath} parentRotation={activeResolvedInstance?.rotation}
      busy={command.isPending} onPreview={previewInsertPattern}
      onClose={() => { previewInsertPattern(); setPatternOpen(false); }}
      onApply={(input) => command.mutateAsync(() => api.patternInstances(activeID, input))} />}
    <CommandDialog size="S" id="new-part-component" open={Boolean(newPartTarget)} title="新建零件"
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
    <CommandDialog size="S" id="datum-axis" open={datumAxisOpen} title="创建基准轴" onClose={() => setDatumAxisOpen(false)}
      confirmLoading={command.isPending} onConfirm={async () => { const v = await datumAxisForm.validateFields();
        command.mutate(() => api.createDatumAxis(activeID, { name: v.name,
          origin: [displayLengthToMillimeters(v.ox,lengthUnit),displayLengthToMillimeters(v.oy,lengthUnit),displayLengthToMillimeters(v.oz,lengthUnit)],
          direction: [v.dx,v.dy,v.dz] }),
          { onSuccess: () => setDatumAxisOpen(false) }); }}><Form form={datumAxisForm} layout="vertical"><Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Space><Form.Item name="ox" label={`原点 X（${lengthUnit}）`}><InputNumber /></Form.Item><Form.Item name="oy" label="Y"><InputNumber /></Form.Item><Form.Item name="oz" label="Z"><InputNumber /></Form.Item></Space>
        <Space><Form.Item name="dx" label="方向 X"><InputNumber /></Form.Item><Form.Item name="dy" label="Y"><InputNumber /></Form.Item><Form.Item name="dz" label="Z"><InputNumber /></Form.Item></Space></Form>
    </CommandDialog>
    <CommandDialog size="S" id="version" open={versionOpen} title="创建命名版本" onClose={() => setVersionOpen(false)}
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
