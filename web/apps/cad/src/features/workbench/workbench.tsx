import {useGlobalOptions} from "../preferences/global-options-center";
import {sceneBackgroundIsLight} from "../../cad/rendering/scene-settings";
import {AssemblyConstraintFields} from "./assembly-constraint-fields";
import {ObjectPropertiesDialog} from "./object-properties-dialog";
import {propertyEditorTarget,refreshPropertyTargets,type PropertyEditorTarget} from "./property-target";
import {withMotionResults} from './motion-result-tree';
import {structureCommandIDs,type StructureActions,type StructureInvocation} from "./commands/structure-commands";
import {useWorkbenchCommandRegistration} from "./workbench-command-registration";
import {ParameterReferenceInput} from "./parameter-reference-input";
import {useFeaturePreview} from "./use-feature-preview";
import {DiagnosticCopy,diagnosticReference} from "../../cad/command/diagnostic-copy";
import { DatumEditor } from "./datum-editor";
import type { DatumPreview } from "../../cad/rendering/datum-reference";
import {assemblyGroupMember,groupMemberForInstance,groupMemberToken,groupMemberSelection} from "../../cad/assembly/group-selection";
import { pickFeatureSelection, type FeatureSelectionSession } from "../../cad/interaction/feature-selection";
import {formatDisplayNumber} from "../../utils/display-number";
import {CadNumberInput as InputNumber} from "../../cad/overlay/cad-number-input";
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
import { assemblyConstraintReferences,assemblyTargetSupportsEligible, contactRelationOptions, matchingAssemblyCapabilities } from "../../cad/assembly/assembly-capability";
import { exactNormalViewPlane } from "../../cad/navigation/normal-view";
import { AssemblyAngleParameters, angleAxisCandidateError } from "./assembly-angle-parameters";
import { fixedPoseAngles } from "../../cad/assembly/assembly-fixed-pose";
import type { AssemblyInteractionCommit } from "../../cad/assembly/assembly-interaction";
import { AssemblyConflictResponseGate,assemblyConflictCurrent,type AssemblyConflictReport,type AssemblyConflictMember,type AssemblyConflictRepair } from "../../cad/assembly/assembly-conflict";
import { AssemblyConflictPanel } from "./assembly-conflict-panel";
import {useOperationFeedback} from "../../cad/command/operation-feedback";
import { InsertDocumentDialog } from "./insert-document-dialog";
import { SolidFeatureEditor } from "./solid-feature-editor";
import { BooleanFeatureDialog } from "./boolean-feature-dialog";
import { InstancePatternDialog } from "./instance-pattern-dialog";
import type { InstancePatternPreview } from "./instance-pattern";
import "./workbench.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert, App, Button, Checkbox, Divider, Empty, Form, Input,
  Select, Space, Spin, Switch, Tag, Typography,
} from "antd";
import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useDocumentSessions } from "../../cad/document/document-session-context";
import { documentEditingState, restoreDocumentEditingState } from "./document-editing-state";
import { treeVisibilityAction } from "./tree-visibility-action";
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
import { DISPLAY_KINDS, visibilityResolverForView, type DisplayKind } from "../../cad/interaction/visibility-resolver";
import { assemblyGeometryRef, type AssemblyConstraintToolKind } from "../../cad/tool/cad-tool";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import {contextTabs} from "../../cad/command/workbench-catalog";
import { useWorkbenchStore, type WorkbenchToolID } from "../../state/workbench-store";
import { displayLengthToMillimeters, effectiveLengthUnit, millimetersToDisplayLength,
  useUIPreferences } from "../../state/ui-preferences";
import { useApplicationContext } from "../../state/application-context";
import type { AssemblyConstraint, AssemblyGeometryRef, AssemblyGroupMember, Artifact, CommandPreview, DatumPlane, DocumentView, Feature, ParameterDefinition, ProductRelease, Selection, SketchOperation, SketchPlane, Vec3 } from "../../types";
import { WorkbenchLayout } from "./workbench-layout";
import { WorkbenchCommands, WorkbenchViewControls } from "./workbench-commands";
import { WorkbenchStatus } from "./workbench-status";
import { contextualToolbars } from "./workbench-command-model";
import type { CadViewportHandle } from "../../viewport/cad-viewport";
import { SpecificationTree, type SpecificationTreeNode } from "./specification-tree";
import { followedDocumentIDs, staleProductDocumentIDs, followProductUpdates } from "./product-edit-context";
import { assemblyAutomaticPreviewAllowed, createAssemblyPreviewActor } from "./assembly-preview-machine";
import { isLengthParameter, parameterDisplayValue, parameterSourceText, parameterEditSource, parseParameterSource } from "./parameter-editor";
import {kinematicKinds,motionSelectionTargets,qualifyMotionSelections,deleteMotionObjects,renameMotionObject} from "../../cad/assembly/motion-definitions";
import {MotionStudyPanel,type MotionPanelHandle} from "./motion-study-panel";
import {applicationDisplay,restoredApplicationSources} from "../../cad/assembly/application-display";
import type {MotionPlayback} from "../../cad/assembly/motion-study";
import { WorkbenchInspectorPanel } from "./workbench-inspector-panel";
import { associatedTreeKeyForSelection, deletableTreeNodesForSelections, ancestorHintKeysForSelections, findStructureEntity, findStructureOccurrenceEntity, isSolidFeature, selectedFeature, structureSelection, treeData, treeKeyForSelection, treeKeysForSelections } from "./workbench-tree-model";
import { assemblyStatusAfterPreviewFailure, assemblySupportPresentation,
  firstDisconnectedSupport, validateReconnectCandidate } from "../../cad/assembly/assembly-constraint-ux";
import { ProductReleaseCenter } from "./product-release-center";

const CadViewport = lazy(() => import("../../viewport/cad-viewport").then((module) => ({ default: module.CadViewport })));


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

export function Workbench() {
  const { documentID = "" } = useParams();
  const documentSessions = useDocumentSessions();
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
  const receiptPrompt=useRef(false);
  const [moveReceiptPending,setMoveReceiptPending]=useState(false);
 const motionPanel=useRef<MotionPanelHandle>(null);
 const motionGestures=useRef(new Map<string,{owner:import('../../cad/assembly/mechanism-motion-session').MechanismMotionSession;bodyId:string;initialValues:Record<string,import("../../cad/assembly/motion-study").MotionQuantity>;completed?:boolean}>());
 const [motionDraft,setMotionDraft]=useState<import('../../cad/assembly/motion-study').MotionStudy>();
 const [motionSession,setMotionSession]=useState<string>();
 const motionOpen=motionSession===documentID;
 useEffect(()=>{setMotionSession(undefined)},[documentID]);
 const [motionStatus,setMotionStatus]=useState("");
 const [mechanismDefinition,setMechanismDefinition]=useState<import("../../cad/assembly/motion-study").Mechanism>();
 const [motionCommandActive,setMotionCommandActive]=useState(false);
  const [motionDisplay,setMotionDisplay]=useState<MotionPlayback>();
 const motionSources=useRef<string[]>([]);
 const [motionAcceptedFrame,setMotionAcceptedFrame]=useState<Array<import("../../cad/assembly/motion-study").MotionPose&{instanceId:string}>>();
 const [motionPosePreview,setMotionPosePreview]=useState<Array<import("../../cad/assembly/motion-study").MotionPose&{instanceId:string}>>();
  const [conflictOpen,setConflictOpen]=useState(false);
  const [conflictReport,setConflictReport]=useState<AssemblyConflictReport>();
  const [conflictContextCurrent,setConflictContextCurrent]=useState(false);
  const [conflictPending,setConflictPending]=useState(false);
  const [conflictError,setConflictError]=useState<string>();
  const conflictGate=useRef(new AssemblyConflictResponseGate());
  const conflictAbort=useRef<AbortController|undefined>(undefined);
  const [propertyTargets,setPropertyTargets]=useState<PropertyEditorTarget[]>();
  const globalOptions=useGlobalOptions();
  const optionsDraft=globalOptions?.draft;
  const persistedScene=useUIPreferences(s=>s.sceneSettings);
  const sceneSettings=optionsDraft?.sceneSettings??persistedScene;
  useEffect(()=>()=>globalOptions?.setMetrics(undefined),[globalOptions?.setMetrics]);
  const [objectPreview,previewObjectProperties]=useState<import("../../types").PropertyPreview>();
  const requestObjectPreview=useCallback((properties:import("../../types").PropertyUpdateRequest,signal:AbortSignal)=>{
    if(!propertyTargets)throw new Error("属性编辑上下文已关闭");
    return api.previewCommand(propertyTargets[0].documentId,{type:"SET_PROPERTIES",versionId:propertyTargets[0].versionId,properties},signal);
  },[propertyTargets]);
  const latestDocumentVersion = useRef<string | undefined>(undefined);
  const latestHostVersion=useRef<string|undefined>(undefined);
  const automaticUpdateSignature = useRef("");
  const automaticUpdateRunning = useRef(false);
  const [automaticUpdateEpoch, setAutomaticUpdateEpoch] = useState(0);
  const [productUpdateFailure, setProductUpdateFailure] = useState<string>();
  const [editSessionState, setEditSession] = useState<EditSession>();
  const editSession = editSessionState?.hostDocumentId === documentID ? editSessionState : undefined;
  const latestEditSession = useRef(editSessionState); latestEditSession.current = editSessionState;
  const activationGate = useRef(new EditActivationGate());
  const [insertOpen, setInsertOpen] = useState(false);
  const [solidEditor,setSolidEditor]=useState<{feature:Feature;digest?:string}>();
  const [featureSelection,setFeatureSelection]=useState<FeatureSelectionSession>();
  const [featureInputs,setFeatureInputs]=useState<Artifact[]>();
  const [historyResult,setHistoryResult]=useState<string>();
  const historyGeneration=useRef(0);
  const endHistoryResult=()=>{historyGeneration.current++;setHistoryResult(undefined);setFeatureInputs(undefined);};
  const featureInputChanged=useCallback((artifact?:Artifact|Artifact[])=>setFeatureInputs(artifact ? Array.isArray(artifact) ? artifact : [artifact] : undefined),[]);
  const [booleanDialog, setBooleanDialog] = useState<{feature?:Feature;digest?:string}>();
  const [patternOpen, setPatternOpen] = useState(false);
  const previewInsertPattern = useCallback((input?: InstancePatternPreview) => viewport.current?.previewInsertPattern(input), []);
  const showAnalysisMotion = useCallback((motion?: Parameters<NonNullable<CadViewportHandle["showRemainingMotion"]>>[0]) => viewport.current?.showRemainingMotion(motion), []);
  const [newPartTarget, setNewPartTarget] = useState<SpecificationTreeNode>();
  const [versionOpen, setVersionOpen] = useState(false);
  const [releaseOpen, setReleaseOpen] = useState(false);
  const [datumEditor, setDatumEditor] = useState<"plane" | "axis">();
 const [editingDatumId,setEditingDatumId]=useState<string>();
 const [parameterSearch,setParameterSearch]=useState("");
  const [datumPreview, setDatumPreview] = useState<DatumPreview>();
  const [parameterManagerOpen, setParameterManagerOpen] = useState(false);
  const [publicationManagerOpen, setPublicationManagerOpen] = useState(false);
  const [externalParameterID, setExternalParameterID] = useState<string>();
  const [editingParameterID, setEditingParameterID] = useState<string>();
  const [editingPublication, setEditingPublication] = useState<{id:string;kind:"PART"|"PRODUCT"}>();
  const [editingContextInput,setEditingContextInput]=useState<{id:string;name:string;required:boolean}>();
  const [pendingAssemblyConstraint, setPendingAssemblyConstraint] = useState<{ kind: AssemblyConstraintToolKind; references: AssemblyGeometryRef[]; angleRelation?:AssemblyConstraint["angleRelation"]; angleAxis?:AssemblyGeometryRef; reverseAngleAxis?:boolean; angleReferenceDirection?: Vec3 }>();
  const [editingAssemblyConstraint, setEditingAssemblyConstraint] = useState<AssemblyConstraint>();
  const [replacingAssemblyReference, setReplacingAssemblyReference] = useState<0 | 1 | 2>();
  const [assemblyDefinitionDirty, setAssemblyDefinitionDirty] = useState(false);
  const [reconnectError, setReconnectError] = useState<string>();
  const [assemblyPreviewEvaluation, setAssemblyPreviewEvaluation] = useState<CommandPreview["constraintEvaluation"]>();
  const [assemblyPreviewCommit, setAssemblyPreviewCommit] = useState(0);
  const assemblyPreviewAbort = useRef<AbortController | undefined>(undefined);
  // Draft epoch fences UI callbacks; RealtimeClient owns monotonic wire sequences,
  // including confirmation refreshes of the same draft.
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
  const saved_catiaRotationSphereVisible = useUIPreferences((state) => state.catiaRotationSphereVisible);
  const catiaRotationSphereVisible=optionsDraft?.catiaRotationSphereVisible??saved_catiaRotationSphereVisible;
  const saved_navigationProfile = useUIPreferences((state) => state.navigationProfile);
  const navigationProfile=optionsDraft?.navigationProfile??saved_navigationProfile;
  const saved_gridVisibility = useUIPreferences(state=>state.gridVisibility);
  const gridVisibility=optionsDraft?.gridVisibility??saved_gridVisibility;
  const saved_referenceVisibility = useUIPreferences((state) => state.referenceVisibility);
  const referenceVisibility=optionsDraft?.referenceVisibility??saved_referenceVisibility;
  const saved_solidDisplay = useUIPreferences((state) => state.solidDisplay);
  const solidDisplay=optionsDraft?.solidDisplay??saved_solidDisplay;
  const saved_captureSettings = useUIPreferences((state) => state.captureSettings);
  const captureSettings=optionsDraft?.captureSettings??saved_captureSettings;
  const saved_displayLengthUnit = useUIPreferences((state) => state.displayLengthUnit);
  const displayLengthUnit=optionsDraft?.displayLengthUnit??saved_displayLengthUnit;
  const saved_documentLengthUnits = useUIPreferences((state) => state.documentLengthUnits);
  const documentLengthUnits=optionsDraft?.documentLengthUnits??saved_documentLengthUnits;
  const setShellActiveDocumentID = useApplicationContext((state) => state.setActiveDocumentID);
  const [shareResource, setShareResource] = useState<ShareResource>();

  const [newPartForm] = Form.useForm<{ name?: string; description?: string }>();
  const [versionForm] = Form.useForm<{ name: string; description: string }>();
  const [parameterForm] = Form.useForm<{ key: string; source: string }>();
  const [createUserParameterForm] = Form.useForm<{name?:string;value:number;unit:string}>();
  const [publicationForm] = Form.useForm<{ name: string; semanticPurpose: string }>();
  const [publicationEditForm] = Form.useForm<{name:string;semanticPurpose:string}>();
  const [contextReferenceForm] = Form.useForm<{ name:string;catalogKey:string;publicationType:string;targetId:string }>();
  const [replacementForm] = Form.useForm<{referencedDocumentId:string}>();
  const [externalParameterForm] = Form.useForm<{ catalogKey: string }>();
  const [assemblyConstraintForm] = Form.useForm<{ value: number; directionRelation: string; distanceRelation: string; quantityExpression?:string;quantityKey?:string;contactKind?:string;contactSide?:string;contactBranch?:number;groupMembers?:string[];fixMode?:AssemblyConstraint["fixMode"];constraintMode?:string; fixedTranslation:Vec3; fixedAngles:Vec3 }>();
  const contextPublicationType = Form.useWatch("publicationType", contextReferenceForm) ?? "PLANE";
  const assemblyDirection = Form.useWatch("directionRelation", assemblyConstraintForm);
  const assemblyDistance = Form.useWatch("distanceRelation", assemblyConstraintForm);
  const store = useWorkbenchStore();
  const document = useQuery({ queryKey: queryKeys.document(documentID), queryFn: () => api.getDocument(documentID), enabled: Boolean(documentID) });
	useLayoutEffect(() => {
	  documentSessions.open(documentID);
	  activationGate.current.invalidate(); setEditSession(undefined); store.endSketch(); store.setSelection(null);
	  if (!documentID) return;
	  void registerDocumentTab(documentID, client, api.openDocument)
	    .catch((error: Error) => message.error(`打开工作空间失败：${error.message}`));
	  return () => {
        const session = latestEditSession.current;
        if (session?.hostDocumentId === documentID) documentSessions.write(documentID, documentEditingState,
          { session, sketchId: useWorkbenchStore.getState().activeSketchID, newSketchSession: newSketchSession.current });
        newSketchSession.current = undefined; activationGate.current.invalidate();
      };
	// Route changes are the explicit open lifecycle. Interaction state changes
	// must never register, reorder, or reopen a tab.
	// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [client, documentID, documentSessions]);
  useLayoutEffect(() => {
    if (editSession) documentSessions.write(documentID, documentEditingState,
      { session: editSession, sketchId: store.activeSketchID, newSketchSession: newSketchSession.current });
  }, [documentID, documentSessions, editSession, store.activeSketchID]);
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
      const selectionState=useWorkbenchStore.getState();
      if(view.product&&selectionState.selections.some(s=>s.kind==='kinematic-object'&&(s.ownerDocumentId??s.documentId)===view.document.id))selectionState.setSelections(qualifyMotionSelections(view,selectionState.selections));
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
  const motionMutation=useMutation({mutationFn:(operation:()=>Promise<DocumentView>)=>operation(),onError:(error)=>operationFeedback(error,"机械装置")});
  const moveCommand = useMutation({mutationFn:(operation:()=>Promise<DocumentView>)=>operation(),
    onSuccess:(updated)=>{void refresh(updated);},onError:()=>{void refresh();}});
  const view = document.data;
  const motionJobs=useQuery({queryKey:['motion-jobs',documentID],queryFn:async()=> (await api.listJobs()).filter(j=>j.type==='MOTION_STUDY'&&j.documentId===documentID),enabled:Boolean(view?.product),refetchInterval:5000});

  const featureOccurrenceContext = activeInstancePath ? {
    instancePath: editSession?.editTarget.instancePath, rootDocumentId: documentID,
    contextVariantKey: view?.contextVariants?.find(v => v.owningInstancePath.canonical === activeInstancePath)?.variantKey,
  } : undefined;
  useEffect(() => {
    if (!view || editSession?.hostDocumentId === view.document.id) return;
    if (document.isFetching && documentSessions.read(documentID, documentEditingState)) return;
    const generation = activationGate.current.begin();
    void restoreDocumentEditingState(view, documentSessions.read(documentID, documentEditingState), generation,
      api.getProductDesignSession, id => client.fetchQuery({ queryKey: queryKeys.document(id), queryFn: () => api.getDocument(id), staleTime: 0 }))
      .then(prepared => {
        if (!activationGate.current.isCurrent(generation)) return;
        client.setQueryData(queryKeys.document(prepared.targetView.document.id), prepared.targetView);
        activationGate.current.commit(generation, prepared.session, setEditSession);
        if (prepared.sketchId) {
          const feature = prepared.targetView.part?.features.find(feature => feature.id === prepared.sketchId);
          const plane = feature && featureSketchPlane(prepared.targetView, feature);
          if (plane) {
            const occurrence = view.resolvedInstances?.find(instance => instance.occurrencePath === prepared.session.editTarget.instancePath?.canonical);
            newSketchSession.current = prepared.newSketchSession;
            store.beginSketch(prepared.sketchId, occurrenceSketchPlane(plane, occurrence?.translation, occurrence?.rotation));
          }
        }
      }).catch(error => {
        if (!activationGate.current.isCurrent(generation)) return;
        activationGate.current.commit(generation, rootEditSession(view, generation), setEditSession);
        message.info(`原编辑目标当前不可恢复，已返回宿主文档：${String(error)}`);
      });
    return () => { if (activationGate.current.isCurrent(generation)) activationGate.current.invalidate(); };
  }, [documentID, documentSessions, client, editSession?.hostDocumentId, view, document.isFetching]);
  const editingView = activeID === documentID ? view : activeDocument.data;
  useEffect(()=>{setMotionDisplay(undefined);if(activeInstancePath||store.activeSketchID)setMotionOpen(false)},[view?.document.id,view?.document.versionId,activeInstancePath,store.activeSketchID]);
  useEffect(() => {
    setSolidEditor(undefined);
    setBooleanDialog(undefined);
    setDatumEditor(undefined); setDatumPreview(undefined);
    historyGeneration.current++;setHistoryResult(undefined);setFeatureInputs(undefined);setFeatureSelection(undefined);
  }, [editingView?.document.id,editingView?.document.versionId,activeInstancePath]);
  useEffect(()=>{if(solidEditor||booleanDialog||datumEditor||store.activeSketchID){historyGeneration.current++;setHistoryResult(undefined);setFeatureInputs(undefined);}},[solidEditor,booleanDialog,datumEditor,store.activeSketchID]);
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
  const [replacingGroupMember,setReplacingGroupMember] = useState<string>();
  const assemblyGroupMembers = Form.useWatch("groupMembers",assemblyConstraintForm) as string[]|undefined;
  useEffect(()=>{if(replacingGroupMember&&!assemblyGroupMembers?.includes(replacingGroupMember))setReplacingGroupMember(undefined);},[assemblyGroupMembers,replacingGroupMember]);
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
  const assemblySupportsReady = editingGroup ? assemblyConstraintReferences({id:'draft',kind:'FIX_TOGETHER',first:{kind:'BODY',instanceId:''},evaluationStatus:'NOT_UPDATED',groupMembers:(assemblyGroupMembers??[]).map(token=>{const [kind,id]=token.split(':');return kind==='group'?{groupId:id}:{instanceId:id}})},editingView?.product?.constraints??[]).length>=2&&
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
  useEffect(()=>{
    if(!editingGroup||!editingView)return;
    const owner=activeInstancePath??'';
    const members=(assemblyGroupMembers??[]).map(token=>{const [kind,id]=token.split(':');return kind==='group'?{groupId:id}:{instanceId:id}});
    const session:FeatureSelectionSession={role:'instance',documentId:editingView.document.id,versionId:editingView.document.versionId,
      project:item=>{const member=assemblyGroupMember(editingView,item,owner,editingAssemblyConstraint?.id);return member?groupMemberSelection(editingView,member,owner):null},
      accept:item=>Boolean(assemblyGroupMember(editingView,item,owner,editingAssemblyConstraint?.id)),
      selections:members.flatMap(member=>{const selection=groupMemberSelection(editingView,member,owner);return selection?[selection]:[]}),
      onPick:item=>{const member=assemblyGroupMember(editingView,item,owner,editingAssemblyConstraint?.id);if(!member)return;const token=groupMemberToken(member),current:string[]=assemblyConstraintForm.getFieldValue('groupMembers')??[];assemblyConstraintForm.setFieldValue('groupMembers',replacingGroupMember?[...new Set(current.map(value=>value===replacingGroupMember?token:value))]:current.includes(token)?current.filter(value=>value!==token):[...current,token]);setReplacingGroupMember(undefined);invalidateAssemblyDefinition()}
    };
    setFeatureSelection(session);return()=>setFeatureSelection(current=>current===session?undefined:current);
  },[editingGroup,editingView,editingAssemblyConstraint?.id,activeInstancePath,assemblyGroupMembers,assemblyConstraintForm,replacingGroupMember]);
  const locateAssemblyMember=(member:AssemblyGroupMember)=>{
    if(!editingView)return;
    const selection=groupMemberSelection(editingView,member,activeInstancePath??'');
    if(selection)store.setSelection(selection);
    const group=editingView.product?.constraints?.find(c=>c.id===member.groupId);
    const reference=member.instanceId?{kind:'BODY' as const,instanceId:member.instanceId}:group?assemblyConstraintReferences(group,editingView.product?.constraints??[])[0]:undefined;
    if(reference)viewport.current?.focusAssemblyReference(reference,activeInstancePath??'');
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
    // Explicit exit or target activation discards an unedited new sketch. Tab
    // switches suspend it in the document session instead of deleting it.
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
    const displayView=motionDisplay?.view??view;
    const resolver = visibilityResolverForView(displayView, !motionDisplay&&editingView ? { view: editingView, occurrencePath: activeInstancePath,
      liveDefinitionProjection: Boolean(editSession && ["OWNER", "EDITOR"].includes(editingView.document.permission ?? "")) } : undefined);
    const decorate = (node: SpecificationTreeNode): SpecificationTreeNode => {
      const visibilityKey = node.selection ? selectionKey(node.selection) : node.key;
      const kind = node.kind === "SKETCH_INPUT_REFERENCE" ? "SKETCH" : node.kind;
      const semantic = (DISPLAY_KINDS as readonly string[]).includes(kind ?? "") && node.selection?.entityRef
        ? resolver.resolve({documentId: node.selection.entityRef.documentId,
          occurrencePath: node.instancePath?.canonical ?? "", kind: kind as DisplayKind,
          axis:node.axis, entityId: node.selection.entityRef.entityId, ownerEntityId: node.ownerEntityId, bodyId: node.bodyId},
          store.activeSketchID ? {id:store.activeSketchID, occurrencePath:activeInstancePath ?? ""} : undefined)
        : undefined;
      const ownVisible = semantic?.effectiveVisible ?? treeVisibilityOverride(visibilityKey, treeVisibilityOverrides) ?? node.localVisible ?? true;
      const actionOwner = node.kind === "INSTANCE" ? node.ownerDocumentId : node.documentId;
      const ownerEditable = node.kind==="MOTION_RESULT"?Boolean(!motionDisplay&&!motionCommandActive&&node.documentId===view?.document.id):Boolean(!motionDisplay && (!motionOpen||!motionCommandActive) && (!motionOpen||kinematicKinds.includes(node.kind??"")||node.kind==="JOINT_EXPANSION"||node.kind==="ASSEMBLY_GROUP_MEMBER_REFERENCE"&&Boolean(node.selection?.mechanismId)) && editingView && actionOwner === editingView.document.id &&
        ["OWNER", "EDITOR"].includes(editingView.document.permission ?? ""));
      return { ...node, hidden: !ownVisible, definitionVisible: semantic?.definitionVisible ?? node.definitionVisible, localVisible: semantic?.localVisible ?? node.localVisible,
        capabilities: ownerEditable ? node.capabilities : node.capabilities?.filter((capability) => capability !== "EDIT" && capability !== "DELETE" && capability !== "SUPPRESS" && capability !== "RENAME"),
        visibilityMode: semantic?.mode ?? node.visibilityMode,
        hiddenByAncestor: Boolean(semantic?.blockedBy && (semantic.blockedBy.kind !== kind || semantic.blockedBy.entityId !== node.entityId)),
        visibilityBlocker: semantic?.blockedBy ? `${semantic.blockedBy.kind} · ${semantic.blockedBy.entityId}` : undefined,
        children: node.children?.map(decorate) };
    };
    return displayView ? withMotionResults(treeData(displayView,motionDisplay?undefined:editingView),displayView,motionJobs.data??[],motionDraft).map((node) => decorate(node)) : [];
  }, [view, motionDraft, motionJobs.data, motionDisplay, motionOpen, motionCommandActive, editingView, editSession, store.activeSketchID, activeInstancePath, treeVisibilityOverrides]);
  useEffect(()=>{
    normalViewRequest.current+=1;
    return ()=>{normalViewRequest.current+=1;};
  },[store.selection,store.sketchPlane,store.activeSketchID,view?.document.versionId,documentID]);
  const normalToSelection = async (signal?:AbortSignal) => {
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
      if (signal?.aborted||generation!==normalViewRequest.current) return;
      if (!plane && selection.kind !== "plane") { message.info("请选择基准平面或实体的平面面；曲面没有唯一法线视图。"); return; }
      if (!viewport.current?.normalToPlane(selection,plane)) message.info("所选平面当前不可用，请重新选择。");
    } catch (error) {
      if (!signal?.aborted&&generation===normalViewRequest.current) message.error(error instanceof Error ? error.message : String(error));
    }
  };
  const canEdit = !motionOpen && !motionDisplay && Boolean(editSession) && (editingView?.document.permission === "OWNER" || editingView?.document.permission === "EDITOR");

  const liveMotionOwner=motionPanel.current?.motionSession();
  const liveMotionCanPause=Boolean(liveMotionOwner?.active&&liveMotionOwner.checkpoint?.qualified&&liveMotionOwner.identity?.baseRevisionId===view?.document.versionId);
  const canEditRoot = (!motionDisplay||liveMotionCanPause) && (view?.document.permission === "OWNER" || view?.document.permission === "EDITOR");
  const canDeleteMotionResult=!motionDisplay&&!motionCommandActive&&store.selections.length>0&&store.selections.every(s=>s.kind==="kinematic-object"&&s.entityKind==="MOTION_RESULT"&&motionJobs.data?.some(j=>j.id===s.id&&["SUCCEEDED","FAILED","CANCELED"].includes(j.state)));
  // Route rendering precedes the layout cleanup of the previous tab. A sketch
  // belongs to the active edit session; a previous Part's store state must not
  // select an impossible Product/Sketcher catalog context during that render.
  const sketchContextActive=Boolean(editSession&&editingView?.part&&store.sketchPlane&&store.activeSketchID);
  const commandFacts={canDeleteMotionResult,rootTarget:Boolean(view&&editingView?.document.id===view.document.id),motionActive:Boolean(motionOpen&&view&&editingView?.document.id===view.document.id&&!sketchContextActive),isMock:isMockMode,moveReceiptPending,hostType:view?.document.type??"PART",targetType:editingView?.document.type??"PART",sketchActive:sketchContextActive,canEdit,rootCanEdit:canEditRoot,busy:command.isPending,selectionKind:store.selection?.kind??"",selectionCount:store.selections.length,hasWorkingBody:Boolean(workingBodyID)};
  const activeTabs=toolbarCatalog.data?contextTabs(toolbarCatalog.data,commandFacts):[];
 const activeWorkbench=activeTabs[0]?.workbench??"";
 const setMotionOpen=(open:boolean)=>{const selected=view?.product?.kinematics?.mechanisms.find(m=>store.selection?.kind==='kinematic-object'&&store.selection.id===m.id)?.id;commandRegistry.cancel();closeCommandPanels();store.setActiveTool("select","once");store.setSelections([]);setMotionSession(open?documentID:undefined);if(open)void motionPanel.current?.execute("enter",selected);};
 const restoredMotionDocument=useRef<string|undefined>(undefined);
 useEffect(()=>{
  if(!view?.product||document.isLoading||!editSession||!motionPanel.current||restoredMotionDocument.current===documentID)return;
  const params=new URLSearchParams(window.location.search),id=params.get('mechanismId');
  const m=view.product.kinematics?.mechanisms.find(m=>m.id===id);
  // Activate the panel before restoring its application. Its inactive cleanup
  // (including React's initial effect replay) must not invalidate that restore.
  if(params.get('application')==='mechanism'&&m&&!motionOpen){setMotionSession(documentID);return;}
  restoredMotionDocument.current=documentID;
  if(params.get('application')==='mechanism'&&m){motionSources.current=restoredApplicationSources(view,m,params);void motionPanel.current.execute('resume',m.id)}
  else if(params.get('application')==='mechanism'){const url=new URL(window.location.href);for(const key of ['application','mechanismId','mechanismSources','applicationRevision'])url.searchParams.delete(key);window.history.replaceState(null,'',url);message.warning('所选机械装置已不存在，已返回装配设计');}
 },[documentID,view?.product,document.isLoading,editSession,motionOpen]);
 const previousMotionOpen=useRef(motionOpen);
 useEffect(()=>{if(previousMotionOpen.current&&!motionOpen){commandRegistry.cancel();store.setActiveTool("select","once");store.setSelections([])}previousMotionOpen.current=motionOpen;},[motionOpen,commandRegistry]);

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
    const previewState = assemblyPreviewActor.current?.getSnapshot().value;
    if (previewState && !assemblyAutomaticPreviewAllowed(previewState)) return;
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
          reverseAngleAxis:pending?.reverseAngleAxis,angleReferenceDirection:pending?.angleReferenceDirection});
      const key=assemblyIntentKey(commandInput,editingView.document.id,editingView.document.versionId,activeInstancePath);
	  void api.previewCommand(editingView.document.id, {...commandInput,
        interactionId:assemblyInteractionID.current},controller.signal).then((preview) => {
		if (!controller.signal.aborted&&sequence===assemblyPreviewSequence.current) {
          if(preview.baseVersionId!==editingView.document.versionId) {
            assemblyPreviewActor.current?.send({type:"REJECT",sequence,error:"STALE_PREVIEW_BASE：预览基线已变化；草稿已保留，请重新预览。"});return;
          }
		  assemblyPreviewID.current=preview.previewId;
          if(!assemblyCandidate.current.resolve(bindingGeneration,key,preview.previewId,commandInput,preview.expiresAt)) {
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
    return () => {
      window.clearTimeout(timer);controller.abort();
      const state=assemblyPreviewActor.current?.getSnapshot().value;
      if(state && !assemblyAutomaticPreviewAllowed(state))return;
      assemblyCandidate.current.invalidate();assemblyPreviewActor.current?.send({type:"CANCEL",sequence});
    };
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
    await moveCommand.mutateAsync(()=>candidate.commitCommand!.type==='SAVE_KINEMATICS'&&motionPanel.current
      ?motionPanel.current.commitManipulation(()=>api.command(ownerDocumentId,candidate.commitCommand!))
      :api.command(ownerDocumentId,candidate.commitCommand!));
  };
  const executeHistory = (direction: "undo" | "redo") => {
    if (!editingView) return; command.mutate(() => direction === "undo" ? api.undo(editingView.document.id) : api.redo(editingView.document.id));
  };
  const openAssemblyConstraintEditor = (constraint: AssemblyConstraint, reconnect = false) => {
    assemblyDialogLifecycle.current.invalidate();
    assemblyCandidate.current.invalidate();assemblyPreviewSequence.current++;assemblyPreviewAbort.current?.abort();assemblyPreviewID.current=undefined;
    assemblyDraftBase.current=editingView ? {documentId:editingView.document.id,revision:editingView.document.versionId} : undefined;
    setPendingAssemblyConstraint(undefined);setReplacingGroupMember(undefined);
    store.setActiveTool("select","once");
    constraint = assemblyPublicEditDraft(constraint);
    if(editingView&&constraint.kind==="FIX_TOGETHER")constraint={...constraint,groupMembers:[...new Map((constraint.groupMembers??[]).map(member=>{const resolved=groupMemberForInstance(editingView,member,constraint.id);return [groupMemberToken(resolved),resolved]})).values()]};
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
      fixMode:constraint.fixMode??"SPACE",
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
  const openFeatureEditor = (node: SpecificationTreeNode) => {
    if (!editingView || !node.entityId || !node.capabilities?.includes("EDIT")) return;
    endHistoryResult();
    if(node.kind==="SKETCH_PATTERN_DEFINITION"&&node.ownerEntityId){
      const owner=editingView.part?.features.find(f=>f.id===node.ownerEntityId),pattern=owner?.sketch?.patterns?.find(p=>p.id===node.entityId);
      if(owner&&pattern){const plane=featureSketchPlane(editingView,owner);if(plane)store.beginSketch(owner.id,occurrenceSketchPlane(plane,activeResolvedInstance?.translation,activeResolvedInstance?.rotation));setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature:{id:"",type:"SKETCH",profile:owner.id,bodyId:owner.bodyId,pattern:{...pattern,sourceKind:"SKETCH_FRAME",source:{bodyId:owner.bodyId!,featureId:owner.id}}}});}return;
    }
    if(!node.definitionDigest)return;
    const feature = editingView.part?.features.find(candidate => candidate.id === node.entityId);
    if (feature && ["SOLID_PATTERN", "SKETCH_PATTERN", "PAD", "LINEAR_EXTRUDE", "REVOLVE", "FILLET", "CHAMFER", "DRAFT", "SHELL", "LOFT"].includes(feature.type.toUpperCase())) {
      setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature, digest: node.definitionDigest});
    } else if (feature?.type === "BOOLEAN") {
      setDatumEditor(undefined);setSolidEditor(undefined);store.setActiveTool("select","once");setBooleanDialog({feature, digest: node.definitionDigest});
    }
  };
  const openSolidFeature = (generator: "LINEAR_EXTRUDE" | "REVOLVE", operation: "NEW_BODY" | "ADD" | "REMOVE" = "ADD") => {
    setDatumEditor(undefined);setBooleanDialog(undefined); store.setActiveTool("select","once");
    const sketch = store.selections.find(s=>s.kind==="sketch");
    setSolidEditor({feature:{id:"",type:generator,bodyId:sketch?.bodyId??workingBodyID,operation,length:40,length2:10,angle:360,extent:"FINITE",reversed:defaultSolidReversed(generator,operation)}});
  };
  const createPartComponent = (values: { name?: string; description?: string }) => {
    if (view?.document.type !== "PRODUCT" || !newPartTarget) return;
    command.mutate(() => api.createPartComponent(view.document.id, { name: values.name?.trim() || undefined,
      description: values.description?.trim() || undefined,
      targetProductInstancePath: newPartTarget.instancePath }), {
      onSuccess: () => { setNewPartTarget(undefined); newPartForm.resetFields(); message.success("零件已创建并插入 Product"); },
    });
  };
  const deleteTreeNodes = (nodes: SpecificationTreeNode[]) => {
    if(command.isPending)return;
    const candidates = [...new Map(nodes.filter((node) => node.entityId && node.kind && node.capabilities?.includes("DELETE") &&
      node.presentationRole !== "INPUT_REFERENCE").map((node) => [`${node.documentId}:${node.kind}:${node.entityId}`, node])).values()];
    if(candidates.some(node=>node.kind==='MOTION_RESULT')){
      if(candidates.some(node=>node.kind!=='MOTION_RESULT'||node.documentId!==view?.document.id))return;
      void (async()=>{
        try{for(const node of candidates){await api.deleteMotionResult(node.entityId!);motionPanel.current?.removeResult(node.entityId!);}
          const ids=new Set(candidates.map(node=>node.entityId));
          store.setSelections(useWorkbenchStore.getState().selections.filter(s=>s.kind!=="kinematic-object"||s.entityKind!=="MOTION_RESULT"||!ids.has(s.id)));
        }catch(error){operationFeedback(error,'删除模拟结果')}
        finally{await Promise.all([client.invalidateQueries({queryKey:['motion-jobs',documentID]}),client.invalidateQueries({queryKey:queryKeys.jobs})])}
      })();return;
    }
    if (!editingView || !(canEdit||motionOpen&&canEditRoot&&!motionDisplay&&nodes.length>0&&nodes.every(node=>kinematicKinds.includes(node.kind??""))) || command.isPending) return;
    if(candidates.some(node=>kinematicKinds.includes(node.kind??""))){
 if(!view?.product?.kinematics||candidates.some(node=>!kinematicKinds.includes(node.kind??"")))return;
 command.mutate(async()=>{
  let base=view;
  if(mechanismDefinition&&candidates.some(n=>n.kind==='MECHANISM'&&motionSources.current.includes(n.entityId!))){
   base=await api.command(view.document.id,{type:'EXIT_MECHANISM',versionId:view.document.versionId,mechanismApplication:{baseRevisionId:view.document.versionId,sourceMechanismId:mechanismDefinition.id,sourceMechanismIds:motionSources.current}});
   motionPanel.current?.adoptProduct(base);
   setMotionAcceptedFrame((base.product?.instances??[]).map(v=>({instanceId:v.id,translation:v.translation,rotation:v.rotation??[0,0,0,1]})));setMechanismDefinition(undefined);setMotionPosePreview(undefined);setMotionDisplay(undefined);motionSources.current=[];
   const url=new URL(window.location.href);for(const key of ['application','mechanismId','mechanismSources','applicationRevision'])url.searchParams.delete(key);window.history.replaceState(null,'',url);await refresh(base);
  }
  const updated=await api.command(base.document.id,{type:"SAVE_KINEMATICS",versionId:base.document.versionId,kinematics:deleteMotionObjects(base.product!.kinematics!,candidates.map(n=>n.entityId!))});motionPanel.current?.adoptDefinition(updated);return updated;
 });return;
 }
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
      targetKind: ["SKETCH_PATTERN_DEFINITION", "SKETCH_ENTITY", "SKETCH_CONSTRAINT", "ASSEMBLY_CONSTRAINT", "INSTANCE", "DATUM_AXIS"].includes(node.kind!) ? node.kind! : node.kind === "PLANE" ? "DATUM_PLANE" : "FEATURE",
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
  const currentAssemblyCandidate = async (target:{id:string;versionId:string}, key:string, sequence:number, force=false) => {
    return assemblyCandidate.current.ensureCurrent(key, async input => {
      const controller=new AbortController();assemblyPreviewAbort.current?.abort();assemblyPreviewAbort.current=controller;
      assemblyPreviewActor.current?.send({type:assemblyPreviewActor.current.getSnapshot().matches("committing")?"REFRESH":"REQUEST",sequence});
      try {
        const preview=await api.previewCommand(target.id,{...input,interactionId:assemblyInteractionID.current},controller.signal);
        if(controller.signal.aborted || sequence!==assemblyPreviewSequence.current || preview.baseVersionId!==target.versionId)throw new Error("预览上下文已变化，请重新预览。");
        setAssemblyPreviewEvaluation(preview.constraintEvaluation);
        assemblyPreviewID.current=preview.previewId;
        if(preview.instancePoses)viewport.current?.previewAssemblyPoses(preview.instancePoses);
        assemblyPreviewActor.current?.send({type:"RESOLVE",sequence,components:preview.assemblyComponents,definitionOnly:preview.evaluationOutcome==="DEFINITION_ONLY"});
        return preview;
      } catch(cause) {
        if(sequence===assemblyPreviewSequence.current && !controller.signal.aborted)assemblyPreviewActor.current?.send({type:"REJECT",sequence,error:String(cause)});
        throw cause;
      }
    },force);
  };
  const commitAssemblyCandidate = async (target:{id:string;versionId:string},key:string,input:Record<string,unknown>,sequence:number) => {
    try {return await api.command(target.id,input);} catch(cause) {
      const failure=cause as {details?:string;message?:string};
      // Expiry/eviction may race the freshness check. Re-evaluate this exact
      // intent once; a changed base or mismatched payload still fails normally.
      if(!/candidate_expired|candidate_missing_or_consumed/.test(`${failure.details??""} ${failure.message??""}`) || sequence!==assemblyPreviewSequence.current)throw cause;
      const refreshed=await currentAssemblyCandidate(target,key,sequence,true);
      if(!refreshed || sequence!==assemblyPreviewSequence.current)throw cause;
      assemblyPreviewActor.current?.send({type:"CONFIRM"});
      return api.command(target.id,refreshed);
    }
  };

  const closeAssemblyCandidate=()=>{
    assemblyDialogLifecycle.current.invalidate();assemblyCandidate.current.invalidate();
    setReplacingGroupMember(undefined);setFeatureSelection(undefined);store.setActiveTool("select","once");
    assemblyPreviewActor.current?.send({type:"CANCEL",sequence:assemblyPreviewSequence.current});
    assemblyPreviewAbort.current?.abort();assemblyPreviewSequence.current+=1;assemblyPreviewID.current=undefined;
    viewport.current?.clearCommandPreview();setAssemblyPreviewEvaluation(undefined);setReplacingAssemblyReference(undefined);
    setReconnectError(undefined);setAssemblyDefinitionDirty(false);setEditingAssemblyConstraint(undefined);setPendingAssemblyConstraint(undefined);
  };
  const closeCommandPanels=(nextCommand?:string)=>{
    closeAssemblyCandidate();stopConflictAnalysis();viewport.current?.showRemainingMotion();
    if(nextCommand!=="dmu.apply"||!motionDisplay)motionPanel.current?.closeCommand();
    setConflictOpen(false);setSolidEditor(undefined);setBooleanDialog(undefined);setDatumEditor(undefined);setDatumPreview(undefined);
    setFeatureSelection(undefined);setNewPartTarget(undefined);
    setPropertyTargets(undefined);
    setInsertOpen(false);setPatternOpen(false);setReleaseOpen(false);setVersionOpen(false);setShareResource(undefined);
    setParameterManagerOpen(false);setPublicationManagerOpen(false);setExternalParameterID(undefined);
    setEditingParameterID(undefined);setEditingPublication(undefined);
    setEditingContextInput(undefined);
  };
  const openObjectProperties=(selections?:import("../../types").SelectionItem[])=>{
    if(!view)return;
    const selected=selections??useWorkbenchStore.getState().selections;
    setPropertyTargets((selected.length?selected:[undefined]).map(selection=>propertyEditorTarget(view,editingView,selection,activeInstancePath,canEdit,canEditRoot)));
  };
  const treeActions:StructureActions={
onProperties:node=>{
  const selected=useWorkbenchStore.getState().selections;
  const selections=node.selection&&selected.some(s=>selectionKey(s)===selectionKey(node.selection!))?selected:node.selection?[node.selection]:undefined;
  openObjectProperties(selections);
},
onViewResult:async(node,operation)=>{
              if(!editingView?.part||node.documentId!==editingView.document.id||(node.selection?.occurrencePath??"")!==(activeInstancePath??"")||!node.entityId||solidEditor||booleanDialog||datumEditor||store.activeSketchID||command.isPending)return;
              const generation=++historyGeneration.current;
              await api.getFeatureInput(node.documentId,{versionId:editingView.document.versionId,featureId:node.entityId,resultStage:true}).then(result=>{
                if(historyGeneration.current!==generation||(operation&&!operation.current))return;
                viewport.current?.clearCommandPreview();setFeatureInputs([result.artifact]);setHistoryResult(String(node.entityId));
              }).catch(error=>{if(historyGeneration.current===generation&&(!operation||operation.current))operationFeedback(error,"查看步骤结果");});
            },
onOpenDocumentTab:async(node,operation) => {
              const targetDocumentId = node.kind === "INSTANCE" ? node.documentId : node.sourceDocumentId;
              if (targetDocumentId) await openDocumentTab(targetDocumentId, client, api.openDocument, path=>{if(!operation||operation.current)navigate(path);})
                .catch((error: Error) => {if(!operation||operation.current)message.error(`打开文档失败：${error.message}`);});
            },
onActivate:async(node,operation) => {
 if(node.kind==='MOTION_RESULT'){if(motionCommandActive||motionDisplay)return;setMotionSession(documentID);await motionPanel.current?.execute('result',node.entityId);return}
 if(node.kind==="JOINT_EXPANSION"||node.kind==="ASSEMBLY_GROUP_MEMBER_REFERENCE"){if(!motionDisplay)await commandRegistry.execute("tree.edit",{payload:{node}});return}
 if(node.kind==="MECHANISM"||node.kind==="MECHANISM_REFERENCE"){setMotionSession(documentID);motionPanel.current?.execute("enter",node.entityId);return}
 if(kinematicKinds.includes(node.kind??"")){if(!motionDisplay)await commandRegistry.execute("tree.edit",{payload:{node}});return}
              if (node.kind === "BODY" && node.documentId === editingView?.document.id && node.bodyId) {
                setEditSession((current) => current ? withWorkingBody(current, node.bodyId!) : current);
                return;
              }
              if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) {
                const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
                if (constraint) openAssemblyConstraintEditor(constraint);
                return;
              }
              if (node.capabilities?.includes("EDIT")) { await commandRegistry.execute("tree.edit",{payload:{node}}); return; }
              if (node.documentId && ["PART", "PRODUCT", "INSTANCE"].includes(node.kind ?? "")) {
                await activateDocumentNode(node,operation); return;
              }
              if (!canEdit || !node.selection || !editingView) return;
              if (node.selection.kind === "sketch") {
                const feature=editingView.part?.features.find((candidate)=>candidate.id===node.selection!.id);
                const localPlane=feature?featureSketchPlane(editingView,feature):undefined;
                const plane=localPlane?occurrenceSketchPlane(localPlane,activeResolvedInstance?.translation,activeResolvedInstance?.rotation):undefined;
                if(feature&&plane)store.beginSketch(feature.id,plane);
              } else if (node.selection.kind === "sketch-constraint") viewport.current?.editDimension(node.selection);
            },
onEdit:(node,operation) => {
 if((node.kind==="MECHANISM"||node.kind==="MECHANISM_REFERENCE")&&node.entityId){
  setMotionSession(documentID);return motionPanel.current?.execute("enter",node.entityId,operation);
 }
 if(node.kind==="SKETCH"&&node.entityId){
  const feature=editingView?.part?.features.find(candidate=>candidate.id===node.entityId);
  const localPlane=feature&&editingView?featureSketchPlane(editingView,feature):undefined;
  if(localPlane)store.beginSketch(node.entityId,occurrenceSketchPlane(localPlane,activeResolvedInstance?.translation,activeResolvedInstance?.rotation));
  return;
 }
 if(node.kind==="CONTEXT_INPUT"&&node.entityId){
  const input=editingView?.part?.contextInputs?.find(input=>input.id===node.entityId);
  if(input)setEditingContextInput({id:input.id,name:input.name,required:Boolean(input.required)});
  return;
 }
 if(node.kind==='ASSEMBLY_GROUP_MEMBER_REFERENCE'){
  if(node.selection?.mechanismId){setMotionSession(documentID);void motionPanel.current?.execute('edit-relation',node.selection.ownerEntityId,operation)}else{const group=editingView?.product?.constraints?.find(c=>c.id===node.selection?.ownerEntityId);if(group)openAssemblyConstraintEditor(group)}
  return;
 }

 if(node.kind==="JOINT_EXPANSION"){setMotionSession(documentID);void motionPanel.current?.execute("edit-relation",node.entityId,operation);return}
 if(kinematicKinds.includes(node.kind??"")){setMotionSession(documentID);motionPanel.current?.execute("edit",node.entityId,operation);return}
              if (node.kind === "ASSEMBLY_CONSTRAINT" && node.entityId) {
                const constraint = editingView?.product?.constraints?.find((candidate) => candidate.id === node.entityId);
                if (constraint) openAssemblyConstraintEditor(constraint);
              } else if (node.selection?.kind === "sketch-constraint") viewport.current?.editDimension(node.selection);
              else if (node.kind === "PARAMETER" && node.entityId) openParameterEditor(node.entityId);
 else if((node.kind==="PLANE"||node.kind==="DATUM_AXIS")&&node.entityId){setEditingDatumId(node.entityId);setDatumEditor(node.kind==="PLANE"?"plane":"axis");}
              else if (node.kind === "PUBLICATION" && node.entityId) openPublicationEditor(node.entityId,"PART");
              else if (node.kind === "PRODUCT_PUBLICATION" && node.entityId) openPublicationEditor(node.entityId,"PRODUCT");
              else openFeatureEditor(node);
            },
onCreatePart:(node) => {
			  if (node.documentType !== "PRODUCT") return;
			  newPartForm.resetFields(); setNewPartTarget(node);
			},
onReferenceMode:(node, mode) => {
			  const segment = node.instancePath?.segments.at(-1);
			  if (!segment) return;
			  command.mutate(() => api.setReferenceMode(segment.ownerDocumentId, segment.instanceId, mode), {
			    onSuccess: async (updated) => {
			      client.setQueryData(queryKeys.document(updated.document.id), updated);
			      await client.invalidateQueries({ queryKey: queryKeys.document(documentID) });
			      message.success(mode === "PINNED" ? "已固定当前引用版本" : "已恢复跟随最新版本");
			    },
			  });
			},
onReconnect:(node) => {
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
            },
onDetach:(node) => {
              if (node.kind === "CONTEXT_REFERENCE" && node.entityId && node.documentId) {
                command.mutate(() => api.detachContextReference(node.documentId!, node.entityId!));
                return;
              }
              if (node.kind === "SKETCH_EXTERNAL_GEOMETRY" && node.ownerEntityId && node.entityId)
                editSketch(node.ownerEntityId, [{type:"DETACH_EXTERNAL_GEOMETRY", externalId:node.entityId}]);
            },
onRefresh:(node) => {
              if (node.kind === "CONTEXT_REFERENCE" && node.documentId) {
                command.mutate(() => api.updateReferences(node.documentId!));
                return;
              }
              refreshAssemblyConstraint(node.entityId);
            },
onDelete:deleteTreeNodes,
onToggleVisibility:(node,scope)=>{
              const action = treeVisibilityAction(node, editSession);
              if (!action || action.scope !== scope) throw new Error("显示上下文已变化，请重新选择。");
              if (scope === "DEFINITION" && node.documentId && node.entityId) {
                command.mutate(() => api.command(node.documentId!, {type:"SET_DEFINITION_VISIBILITY",targetKind:node.kind,
                  targetId:node.entityId,axis:node.axis,ownerEntityId:node.ownerEntityId,visible:!(node.definitionVisible ?? node.localVisible ?? true)}));
                return;
              }
              if (scope === "OCCURRENCE" && view?.document.type === "PRODUCT" && node.instancePath && node.entityId) {
                command.mutate(() => api.command(view.document.id, {type:"SET_OCCURRENCE_VISIBILITY",instancePath:node.instancePath,
                  targetKind:node.kind,targetId:node.entityId,visibilityMode:action.mode}));
                return;
              }
              throw new Error("显示目标当前不可用，请重新选择。");
            },
onToggleSuppression:(node)=>{
              if(node.kind==="SKETCH_PATTERN_DEFINITION"&&node.documentId===editingView?.document.id&&node.ownerEntityId){
                const pattern=editingView?.part?.features.find(f=>f.id===node.ownerEntityId)?.sketch?.patterns?.find(p=>p.id===node.entityId);
                if(pattern)editSketch(node.ownerEntityId,[{type:"EDIT_PATTERN",patternId:pattern.id,pattern:{...pattern,suppressed:!pattern.suppressed}}]);return;
              }
              if (node.documentId === editingView?.document.id && node.entityId && ["PAD","REVOLVE","FEATURE","SKETCH"].includes(node.kind ?? "") && node.definitionDigest) {
                command.mutate(()=>api.command(node.documentId!,{type:"SET_FEATURE_SUPPRESSION",targetId:node.entityId,expectedFeatureDigest:node.definitionDigest,suppressed:!node.suppressed}));
                return;
              }
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
            },
onToggleConstruction:(node)=>{
              if(!node.ownerEntityId||!node.entityId)return;
              editSketch(node.ownerEntityId,[{type:"UPDATE_ENTITY_ROLE",entityId:node.entityId,
                role:node.role==="CONSTRUCTION"?"PROFILE":"CONSTRUCTION"}]);
            }
  };
  const executeTree=(action:keyof StructureActions,payload:StructureInvocation)=>{void commandRegistry.execute(structureCommandIDs[action],{payload}).catch(error=>operationFeedback(error,"命令"));};
  const configuredToolbars=useWorkbenchCommandRegistration(commandRegistry,{openProperties:()=>openObjectProperties(),openOptions:globalOptions?.open,prepareAssemblyMove:async()=>{const owner=motionPanel.current?.motionSession();if(owner)await owner.prepareSpatial()},setMotionOpen,motionOpen,motionCommandEnabled:action=>motionPanel.current?.enabled(action)??false,onMotionCommand:(action,operation)=>{motionPanel.current?.execute(action,undefined,operation)},treeActions,closePanels:closeCommandPanels,toolContinuation:Boolean(pendingAssemblyConstraint||editingAssemblyConstraint),
    formActive:Boolean(motionCommandActive||conflictOpen||solidEditor||booleanDialog||datumEditor||propertyTargets||newPartTarget||insertOpen||patternOpen||releaseOpen||versionOpen||shareResource||parameterManagerOpen||publicationManagerOpen||externalParameterID||editingParameterID||editingPublication||editingContextInput||pendingAssemblyConstraint||editingAssemblyConstraint),store,editingView,view,canEdit,canEditRoot,command,selectedNamingIssue,conflictOpen,moveReceiptPending,workingBodyID,treeNodes,viewport,startSketch,finishSketch,normalToSelection,openSolidFeature,deleteTreeNodes,executeHistory,setConflictOpen,setParameterManagerOpen,setPublicationManagerOpen,setInsertOpen,setPatternOpen,setReleaseOpen,setVersionOpen,setShareResource,setSolidEditor,setBooleanDialog,setDatumEditor,setEditingDatumId,showMessage:text=>{void message.info(text);}},toolbarCatalog.data,
    commandFacts,
    JSON.stringify([documentID,activeID,editSession?.activationGeneration,store.activeSketchID,workingBodyID]));


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
  const parameterDraftSource=Form.useWatch("source",parameterForm),parameterDraftKey=Form.useWatch("key",parameterForm);
 const previewParameter=editingView?.part?.parameters?.find(p=>p.parameterId===editingParameterID);
 let parameterPreviewInput:Record<string,unknown>|undefined;
 try{if(editingParameterID&&editingView&&previewParameter&&previewParameter.role!=="MEASURED"&&!previewParameter.source.external&&typeof parameterDraftSource==="string"){
 const parsed=parseParameterSource(parameterEditSource(previewParameter,parameterDraftSource,isLengthParameter(previewParameter)?lengthUnit:previewParameter.displayUnit),isLengthParameter(previewParameter)?lengthUnit:previewParameter.displayUnit);
 if(parsed.kind==="LITERAL"||parsed.expression)parameterPreviewInput={type:"EDIT_PARAMETER",parameterId:editingParameterID,name:parameterDraftKey?.trim()||previewParameter.key,...(parsed.kind==="LITERAL"?{value:parsed.value,unit:parsed.unit}:{expression:parsed.expression})};
 }}catch{}
 const parameterInputPreview=useFeaturePreview(editingView?.document.id??"",editingView?.document.versionId??"",parameterPreviewInput,undefined,()=>{});
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
	if(!parameterInputPreview.previewId||parameterInputPreview.pending){message.warning("请等待表达式检查成功");return;}
 command.mutate(() => api.command(editingView.document.id, {previewId:parameterInputPreview.previewId,type: "EDIT_PARAMETER", parameterId: editingParameterID,
		name: values.key.trim() || current?.key, ...(source.kind === "LITERAL" ? { value: source.value, unit: source.unit } : { expression: source.expression })}),
		{onSuccess:()=>setEditingParameterID(undefined)});
  };
  const editingParameterDefinition=editingView?.part?.parameters?.find(parameter=>parameter.parameterId===editingParameterID);
  const parameterSourceReadonly=editingParameterDefinition?.role==="MEASURED"||Boolean(editingParameterDefinition?.source.external);
  const endInteractionForActivation = () => {
    viewport.current?.cancelAssemblyInteraction();stopConflictAnalysis();
    assemblyDialogLifecycle.current.invalidate();
    setDatumEditor(undefined);setSolidEditor(undefined); setBooleanDialog(undefined);
    assemblyPreviewActor.current?.send({ type: "CANCEL", sequence: assemblyPreviewSequence.current });
    assemblyPreviewAbort.current?.abort(); assemblyPreviewSequence.current += 1; assemblyPreviewID.current = undefined;
    viewport.current?.clearCommandPreview();
    store.endSketch(); store.setSelection(null);
  };
  const activateDocumentNode = async (node: SpecificationTreeNode,operation?:import("../../cad/command/command-registry").CommandOperation) => {
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
      if (!activationGate.current.isCurrent(generation)||(operation&&!operation.current)) return;
      client.setQueryData(queryKeys.document(prepared.targetView.document.id), prepared.targetView);
      endInteractionForActivation();
      activationGate.current.commit(generation, prepared.session, setEditSession);
    } catch (error) {
      if (activationGate.current.isCurrent(generation)&&(!operation||operation.current)) {
        message.error(`无法进入上下文编辑：${error instanceof Error ? error.message : String(error)}`);
      }
    }
  };
  if (document.isLoading || view && !editSession) return <div className="workbench-loading"><Spin size="large" /></div>;
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

  </>;


  const activeTabIDs=new Set(activeTabs.map(tab=>tab.id));
  const visibleToolbars = contextualToolbars(configuredToolbars, activeWorkbench).filter(toolbar=>toolbar.tabIds?.some(id=>activeTabIDs.has(id)));
  const activeToolName = visibleToolbars.flatMap((toolbar) => toolbar.items)
    .find((item) => item.commandId === store.activeToolID)?.name ?? "选择";

  const applicationParams=new URLSearchParams(window.location.search);
  const restoringMechanism=!mechanismDefinition&&!motionAcceptedFrame&&applicationParams.get('application')==='mechanism'?view.product?.kinematics?.mechanisms.find(m=>m.id===applicationParams.get('mechanismId')):undefined;
  const restoringPoses=restoringMechanism?Object.entries(Object.assign({},...restoredApplicationSources(view,restoringMechanism,applicationParams).map(id=>view.product?.kinematics?.mechanisms.find(m=>m.id===id)?.poses??{})) as Record<string,import("../../cad/assembly/motion-study").MotionPose>).map(([instanceId,p])=>({instanceId,...p})):undefined;
  return <CommandProvider registry={commandRegistry} catalog={toolbarCatalog.data}><section className="cad-workbench">

    {historyResult&&<Alert type="info" title="正在查看历史步骤结果" action={<Button onClick={endHistoryResult}>恢复当前结果</Button>}/> }
    {view.product&&<MotionStudyPanel ref={motionPanel} sourceMechanismIds={motionSources.current} onDraft={setMotionDraft} captureViewpoint={()=>viewport.current?.captureMotionViewpoint()} previewViewpoint={(v,documentId)=>{if(!documentId||documentId===view.document.id)viewport.current?.previewMotionViewpoint(v)}} onCommit={async command=>{const updated=await motionMutation.mutateAsync(()=>api.command(view.document.id,command));await refresh(updated);return updated}} active={motionOpen} selection={store.selection} view={view} canEdit={["OWNER","EDITOR"].includes(view.document.permission??"")} onCommandState={setMotionCommandActive} onMechanism={m=>{setMechanismDefinition(m);if(m&&motionOpen&&motionSources.current.at(-1)===m.id){const url=new URL(window.location.href);url.searchParams.set("application","mechanism");url.searchParams.set("mechanismId",m.id);url.searchParams.set("applicationRevision",view.document.versionId);url.searchParams.set("mechanismSources",motionSources.current.join(','));window.history.replaceState(null,"",url)}}} onStatus={setMotionStatus} onSelectionSession={setFeatureSelection} onPreview={setMotionPosePreview} onSave={async (k,baseRevisionId)=>{if(motionMutation.isPending)throw new Error("当前机械装置命令尚未完成");const updated=await motionMutation.mutateAsync(()=>api.command(view.document.id,{type:"SAVE_KINEMATICS",versionId:baseRevisionId??view.document.versionId,kinematics:k}));const created=updated.product?.kinematics?.mechanisms.find(m=>!view.product?.kinematics?.mechanisms.some(v=>v.id===m.id));const acceptedId=created?.id??mechanismDefinition?.id;if(acceptedId)motionSources.current=[...motionSources.current.filter(id=>id!==acceptedId&&updated.product?.kinematics?.mechanisms.some(m=>m.id===id)),acceptedId];setMechanismDefinition(m=>updated.product?.kinematics?.mechanisms.find(v=>v.id===m?.id));setMotionPosePreview(undefined);await refresh(updated);return updated}} onSwitch={async input=>{
      if(motionMutation.isPending)throw new Error("当前机械装置命令尚未完成");
      input={...input,sourceMechanismIds:input.sourceMechanismIds??motionSources.current.filter(id=>view.product?.kinematics?.mechanisms.some(m=>m.id===id))};
      if(!input.mechanismId&&!["OWNER","EDITOR"].includes(view.document.permission??""))input={...input,sourceMechanismId:undefined,sourceMechanismIds:[]};
      const prepared=await api.prepareMechanismApplication(view.document.id,input);
      if(!["OWNER","EDITOR"].includes(view.document.permission??"")){motionSources.current=input.mechanismId?[...new Set([...(input.sourceMechanismIds??[]),input.mechanismId])]:[];setMotionAcceptedFrame(prepared.instancePoses);return {...prepared,view}}
      const updated=await motionMutation.mutateAsync(()=>api.command(view.document.id,{type:input.mechanismId?"ACTIVATE_MECHANISM":"EXIT_MECHANISM",versionId:input.baseRevisionId,mechanismApplication:input}));
      const m=updated.product?.kinematics?.mechanisms.find(v=>v.id===input.mechanismId);motionSources.current=m?[...new Set([...(input.sourceMechanismIds??[]),m.id])]:[];const poses=m?prepared.instancePoses.map(v=>({instanceId:v.instanceId,...(m.poses?.[v.instanceId]??{translation:v.translation,rotation:v.rotation})})):(updated.product?.instances??[]).map(v=>({instanceId:v.id,translation:v.translation,rotation:v.rotation??[0,0,0,1] as [number,number,number,number]}));setMotionAcceptedFrame(poses);setMechanismDefinition(m);setMotionPosePreview(undefined);setMotionDisplay(undefined);await refresh(updated);
      return {view:updated,baseRevisionId:input.baseRevisionId,mechanism:m,instancePoses:poses}
    }} onApply={async motionApply=>{const updated=await motionMutation.mutateAsync(()=>api.command(view.document.id,{type:"APPLY_MOTION_FRAME",versionId:view.document.versionId,motionApply}));setMotionAcceptedFrame((updated.product?.instances??[]).map(v=>({instanceId:v.id,translation:v.translation,rotation:v.rotation??[0,0,0,1]})));setMechanismDefinition(undefined);setMotionPosePreview(undefined);setMotionDisplay(undefined);store.setSelections([]);await refresh(updated)}} onPlayback={setMotionDisplay} onHighlight={store.setSelections} onClose={()=>{const url=new URL(window.location.href);url.searchParams.delete("application");url.searchParams.delete("mechanismId");url.searchParams.delete("mechanismSources");url.searchParams.delete("applicationRevision");motionSources.current=[];setMotionAcceptedFrame(undefined);window.history.replaceState(null,"",url);setMotionSession(undefined);setMechanismDefinition(undefined);setMotionPosePreview(undefined);setMotionDisplay(undefined);store.setSelections([])}}/>}
    <WorkbenchLayout
      inspectorOpen={inspectorOpen} onInspectorChange={setInspectorOpen} lightScene={sceneBackgroundIsLight(sceneSettings)}

      commands={<WorkbenchCommands documentSessions={documentSessions} hostDocumentId={documentID} toolbars={visibleToolbars} workbench={activeWorkbench} catalog={toolbarCatalog.data} tabs={activeTabs} />}
      status={<WorkbenchStatus busy={command.isPending} canEdit={motionOpen&&!motionDisplay?canEditRoot:canEdit} selectionCount={store.selections.length}
        sketchReceipt={sketchReceipt} onSketchReceiptCheck={()=>void viewport.current?.retrySketchReceipt()} sketchCommand={store.activeSketchID?sketchCommandState:undefined} onSketchAction={action=>viewport.current?.sketchCommandAction(action)}
        toolName={activeToolName} prompt={motionOpen?motionStatus:undefined} lengthUnit={lengthUnit} continuous={store.activeToolMode === "continuous"} />}
      tree={<SpecificationTree key={documentID} documentSessions={documentSessions} hostDocumentId={documentID} nodes={treeNodes} selectedKeys={treeKeysForSelections(treeNodes, store.selections)}
            ancestorHintKeys={ancestorHintKeysForSelections(treeNodes, store.selections)}
            selectionToken={selectionSetToken(store.selections)}
            highlightedKey={treeKeyForSelection(treeNodes, store.preselection)??associatedTreeKeyForSelection(treeNodes,store.preselection)}
            editSession={editSession}
            activeDocumentId={activeID}
            activeInstancePath={activeInstancePath}
            workingBodyId={workingBodyID}
            onViewResult={node=>executeTree("onViewResult",{node})}
            onSelect={(nodes) => {
              const selections = [...new Map(nodes.flatMap((node) => node.selection ? [[selectionKey(node.selection), node.selection] as const] : [])).values()];
              if(featureSelection){for(const selection of selections)pickFeatureSelection(selection,featureSelection);return;}
              if (!viewport.current?.captureToolSelections(selections)) store.setSelections(selections);
            }}
            onOpenDocumentTab={node=>executeTree("onOpenDocumentTab",{node})}
            onActivate={node=>executeTree("onActivate",{node})}
			onEdit={node=>executeTree("onEdit",{node})}
            onProperties={node=>executeTree("onProperties",{node})}
			onCreatePart={node=>executeTree("onCreatePart",{node})}
			onReferenceMode={(node,mode)=>executeTree("onReferenceMode",{node,referenceMode:mode})}
			onReconnect={node=>executeTree("onReconnect",{node})}
			onDetach={node=>executeTree("onDetach",{node})}
            onRefresh={node=>executeTree("onRefresh",{node})}
            onHover={(node) => store.setPreselection(node?.selection ?? null)} onDelete={nodes=>executeTree("onDelete",{nodes})}
            onToggleVisibility={(node,scope,mode)=>executeTree("onToggleVisibility",{node,scope,visibilityMode:mode})}
            onToggleSuppression={node=>executeTree("onToggleSuppression",{node})}
            onToggleConstruction={node=>executeTree("onToggleConstruction",{node})} />}
      inspector={<WorkbenchInspectorPanel documentID={motionDisplay?documentID:activeID} view={motionDisplay?.view??editingView??view} selection={store.selection}
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
        {view.document.type==="PRODUCT"&&productUpdatePlan.data?.parameterUpdates?.some(node=>node.needsUpdate)&&!productUpdateFailure&&<Alert
          style={{position:"absolute",zIndex:12,top:12,left:"50%",transform:"translateX(-50%)"}}
          type="info" showIcon message="共享参数有待接受的更新"
          action={<Button size="small" disabled={!canEditRoot||command.isPending||!productUpdatePlan.data.canAccept} onClick={()=>command.mutate(()=>api.acceptProductUpdatePlan(documentID,productUpdatePlan.data!.digest))}>更新关联零件</Button>}/>}
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
        <Suspense fallback={<div className="viewport-loading"><Spin size="large" /></div>}><CadViewport ref={viewport} onSceneMetrics={globalOptions?.setMetrics} applicationDisplay={motionOpen||restoringMechanism?applicationDisplay(motionDisplay?.view??view,mechanismDefinition??restoringMechanism,motionDisplay,motionPosePreview,motionAcceptedFrame??restoringPoses):undefined} view={motionDisplay?.view??view} motionDisplay={motionDisplay} mechanism={motionOpen?mechanismDefinition:undefined} mechanismSelection={store.selection?.kind==="kinematic-object"?store.selection.id:undefined} documentSessions={documentSessions}
          editingView={motionDisplay?undefined:editingView} activeInstancePath={motionDisplay?undefined:activeInstancePath} activeInstanceTranslation={activeResolvedInstance?.translation}
          liveDefinitionProjection={Boolean(canEdit && editSession?.hostDocumentId===view.document.id && activeInstancePath &&
            !pinnedReferenceInPath(view.structureTree,activeInstancePath))}
          activeInstanceRotation={activeResolvedInstance?.rotation}
          activeBodyTreeNodeId={activeResolvedInstance?.bodyTreeNodeId}
          selections={motionSelectionTargets(motionDisplay?.view??view,store.selections)}
          preselection={store.preselection}
          treeVisibilityOverrides={treeVisibilityOverrides}
          sketchPlane={motionDisplay?undefined:store.sketchPlane} activeSketchID={motionDisplay?undefined:store.activeSketchID} activeToolID={motionDisplay?"select":store.activeToolID} navigationProfile={navigationProfile} catiaRotationSphereVisible={catiaRotationSphereVisible}
          propertyPreview={objectPreview} sceneSettings={sceneSettings} gridVisibility={{...gridVisibility,scene:true}} referenceVisibility={referenceVisibility} solidDisplay={solidDisplay}
          datumPreview={datumPreview} featureSelection={featureSelection} featureInputArtifacts={featureInputs} preferredLengthUnit={lengthUnit} captureSettings={captureSettings} onSelectionsChange={store.setSelections} onPreselectionChange={store.setPreselection} onSketchOperations={editSketch} onDimensionOperations={editSketch} onSketchPreview={(featureId,operations,signal)=>{if(!editingView||store.activeSketchID!==featureId)return Promise.reject(new Error("草图预览上下文已结束"));return api.previewCommand(editingView.document.id,{type:"EDIT_SKETCH",sketchId:featureId,operations},signal);}} onSketchReceiptCheck={async receipt=>{const updated=await command.mutateAsync(()=>api.command(receipt.ownerDocumentId,{type:"EDIT_SKETCH",sketchId:receipt.featureId,operations:receipt.operations,requestId:receipt.intent.requestId}));await refresh(updated);return updated;}}
          onSketchReceiptChange={setSketchReceipt} onSketchCommandStateChange={setSketchCommandState} onToolUseComplete={store.completeToolUse} onActiveToolChange={store.setActiveTool}
		  onAssemblyConstraint={(toolKind, references,groupMembers) => {
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
            setEditingAssemblyConstraint(undefined);setReplacingGroupMember(undefined);
            setAssemblyDefinitionDirty(true); setReconnectError(undefined); setReplacingAssemblyReference(undefined);
            // Mesh/display measurements are not authoritative parameter or
            // branch defaults. Wait for exact server inspection for sign rules.
            assemblyConstraintForm.setFieldsValue({ value:0, directionRelation:"UNORIENTED", distanceRelation: "UNSIGNED",
              ...(kind === "distance" ? offsetInitialFields(undefined,references) : {}) });
            assemblyConstraintForm.setFieldsValue({...assemblyPublicInitialFields(kind.toUpperCase(),references),...(groupMembers?{groupMembers:[...new Set(groupMembers.map(member=>groupMemberToken(groupMemberForInstance(editingView,member))))]}:{}),...(kind==="angle"?assemblyQuantityInitialFields():{})});
            const instance=editingView.product?.instances.find(instance=>instance.id===references[0]?.instanceId);
            const pose={translation:instance?.translation??[0,0,0] as Vec3,rotation:instance?.rotation??[0,0,0,1] as [number,number,number,number]};
            assemblyConstraintForm.setFieldsValue({fixMode:"SPACE",fixedTranslation:pose.translation.map(v=>millimetersToDisplayLength(v,lengthUnit)) as Vec3,fixedAngles:fixedPoseAngles(pose)});
            setPendingAssemblyConstraint({ kind, references,
              angleRelation });
          }}
          onAssemblyInteractionBegin={(ownerDocumentId,input,signal)=>{
            if(!canEditRoot||!editingView)throw new Error("运动所属 Product 不可编辑");
            const owner=motionPanel.current?.motionSession();
            if(owner){
              if(!owner.active)throw new Error('机构会话正在准备或已结束');
              return owner.prepareSpatial().then(()=>{const identity=owner.identity!,sessionId='mechanism-gesture-'+randomUUID();motionGestures.current.set(sessionId,{owner,bodyId:input.instanceId,initialValues:Object.fromEntries(owner.checkpoint!.activeDriverIds.map(id=>{const driver=identity.drivers.find(d=>d.id===id)!,coordinate=editingView.product?.kinematics?.mechanisms.find(m=>m.id===driver.mechanismId)?.joints.flatMap(j=>j.coordinates??[]).find(c=>c.id===driver.coordinateId);return [id,{value:owner.checkpoint!.frame.commandValues[id],unit:coordinate?.dimension==='ANGLE'?'rad':'mm'}]}))});
                return {mode:'COMMAND_TRIAL' as const,sessionId,bodyId:input.instanceId,baseRevisionId:identity.baseRevisionId,inputDigest:identity.inputDigest,nominalPoses:Object.entries(owner.checkpoint!.frame.unitPoses).map(([instanceId,pose])=>({instanceId,...pose}))};});
            }
            return api.beginAssemblyInteraction(ownerDocumentId,{...input,...(input.mechanismId?{sourceMechanismIds:motionSources.current}:{}),editContext:{
              rootDocumentId:view.document.id,rootRevisionId:view.document.versionId,
              activeDocumentId:editingView.document.id,activeRevisionId:editingView.document.versionId,
              instancePath:editSession?.editTarget.instancePath}},signal);
          }}
          onAssemblyInteractionUpdate={async(ownerDocumentId,input,signal)=>{
            const gesture=motionGestures.current.get(input.sessionId);if(!gesture)return api.updateAssemblyInteraction(ownerDocumentId,input,signal);
            if(signal?.aborted)throw new DOMException('Canceled','AbortError');
            const response=await gesture.owner.request({spatialTarget:{...input.target,bodyId:gesture.bodyId}});
            if(!response||signal?.aborted)throw new DOMException('Canceled','AbortError');
            if(input.final)gesture.completed=true;
            return {sessionOutcome:response.accepted?'ACCEPTED':'RETAINED',previewId:'',requestId:response.sessionId+'/'+response.sequence,sessionId:input.sessionId,inputDigest:response.inputDigest,sequence:input.sequence,goalSequence:input.goalSequence,
              instancePoses:Object.entries(response.frame.unitPoses).map(([instanceId,pose])=>({instanceId,...pose})),interaction:{...response.interaction,status:response.accepted?(response.interaction?.status??'CONSTRAINED'):'FAILED',hardFeasible:response.frame.kinematicValid,targetConverged:response.accepted&&Boolean(response.interaction?.targetConverged),eligibleForCommit:false,targetError:response.interaction?.targetError??Number.NaN,targetOptimality:response.interaction?.targetOptimality??Number.NaN,targetSequence:input.sequence,diagnostic:response.failure?.message}};
          }}
          onAssemblyInteractionCancel={(ownerDocumentId,sessionId)=>{const gesture=motionGestures.current.get(sessionId);if(gesture){motionGestures.current.delete(sessionId);if(!gesture.completed&&gesture.owner.active)return gesture.owner.request({values:gesture.initialValues},false).then(()=>undefined);return Promise.resolve()}return api.cancelAssemblyInteraction(ownerDocumentId,sessionId)}}
          onInspectAssemblySupports={(owner,references,signal)=>api.inspectAssemblySupports(owner,references,signal)}
          onOperationFailed={error=>operationFeedback(error,"装配操纵")}
          onAssemblyInteractionState={(state,reason)=>{
            if(state==="blocked"&&reason?.startsWith("移动尚未保存"))setMoveReceiptPending(true);
            if(state==="committing"&&reason?.startsWith("提交结果未知")&&!receiptPrompt.current){
              setMoveReceiptPending(true);
              receiptPrompt.current=true;modal.confirm({title:"移动提交结果待确认",content:"连接中断时移动可能已经保存。请查询原请求回执，勿重复创建移动。",
                okText:"确定",cancelText:"取消",onOk:()=>{receiptPrompt.current=false;viewport.current?.retryAssemblyMoveCommit();},onCancel:()=>{receiptPrompt.current=false;}});
            }
            if(state==="committed"||state==="idle"||state==="failed"||state==="invalidated"){receiptPrompt.current=false;setMoveReceiptPending(false);}
          }}
          onInstanceMoved={moveInstance} /></Suspense>
      <WorkbenchViewControls toolbars={visibleToolbars} />
    </WorkbenchLayout>
    <CommandDialog id="assembly-constraint-edit" open={Boolean(editingAssemblyConstraint)} title="约束定义" size="M"
      onClose={closeAssemblyCandidate}
      confirmLoading={command.isPending || assemblyPreviewPending} confirmDisabled={!assemblySupportsReady || !(assemblyPreviewSnapshot.matches("succeeded") || assemblyPreviewSnapshot.matches("definitionReady")) || replacingAssemblyReference !== undefined || !!replacingGroupMember ||
        Boolean(editingAssemblyConstraint?.kind === "ANGLE" && editingAssemblyConstraint.angleRelation === "DIRECTED" && !editingAssemblyConstraint.angleAxis)} onConfirm={async()=>{
        if(!editingView||!editingAssemblyConstraint)return;
        const constraint=editingAssemblyConstraint, target=editingView.document;
        const intent=assemblyEditIntent(constraint.kind,assemblyConstraintForm.getFieldsValue(true),resolvedAssemblyReferences,lengthUnit,{...constraint,angleAxis:resolvedAngleAxis});
        const key=assemblyIntentKey(intent,target.id,target.versionId,activeInstancePath);
        const sequence=assemblyPreviewSequence.current;
        const dialogGeneration=assemblyDialogLifecycle.current.capture();
        await assemblyConstraintForm.validateFields();
        const input=await currentAssemblyCandidate(target,key,sequence);
        if(!input){invalidateAssemblyDefinition();return;}
        if(sequence!==assemblyPreviewSequence.current || !(assemblyPreviewActor.current?.getSnapshot().matches("succeeded") || assemblyPreviewActor.current?.getSnapshot().matches("definitionReady")))return;
		assemblyPreviewActor.current?.send({type:"CONFIRM"});
        command.mutate(()=>commitAssemblyCandidate(target,key,input,sequence),assemblyDialogLifecycle.current.bindCallbacks<DocumentView>(dialogGeneration,{onSuccess:(updated)=>{assemblyPreviewActor.current?.send({type:"COMMIT_SUCCESS"});if(updated.product?.constraints?.find(c=>c.id === constraint.id)?.evaluationStatus === "NOT_UPDATED")message.info("约束已保存，等待更新");viewport.current?.clearCommandPreview(false);assemblyPreviewID.current=undefined;setAssemblyDefinitionDirty(false);setReconnectError(undefined);setEditingAssemblyConstraint(undefined);store.setSelection({kind:"assembly-constraint",id:constraint.id,constraintId:constraint.id,constraintType:constraint.kind,documentId:updated.document.id,
          occurrencePath:activeInstancePath??"",instancePath:editSession?.editTarget.instancePath,rootDocumentId:activeInstancePath?view?.document.id:undefined,
          treeNodeId:`${view?.constraintDisplayScopes?.find(scope=>scope.documentId===target.id && scope.instancePath.canonical===activeInstancePath)?.treeNodeId??`document:${updated.document.id}`}/assembly-constraints/constraint:${constraint.id}`});},
		  onError:(cause)=>{assemblyPreviewActor.current?.send({type:"COMMIT_FAILURE",error:String(cause)});}}));
      }}>
      <Form form={assemblyConstraintForm} layout="vertical" onValuesChange={invalidateAssemblyDefinition}>
        {editingAssemblyConstraint?.kind === "ANGLE" && <AssemblyAngleParameters value={editingAssemblyConstraint} view={editingView}
          axisSourceType={axisInspection?.supports[0]?.exactType} axisExactType={axisInspection?.supports[1]?.exactType} onDerive={deriveAngleAxis}
          onChange={value=>{invalidateAssemblyDefinition();if(!angleRelationSupportsMeasured(value.angleRelation))assemblyConstraintForm.setFieldValue("constraintMode","DRIVING");if(value.angleRelation === "PERPENDICULAR") assemblyConstraintForm.setFieldValue("directionRelation", assemblyConstraintForm.getFieldValue("directionRelation") === "OPPOSITE" ? "OPPOSITE" : "SAME");setEditingAssemblyConstraint({...editingAssemblyConstraint,...value});}}
          onPick={()=>{store.setSelection(null);setReplacingAssemblyReference(2);store.setActiveTool("select","once");}} />}


        {editingAssemblyConstraint && <AssemblyConstraintFields kind={editingAssemblyConstraint.kind} view={editingView} lengthUnit={lengthUnit}
          replacingMember={replacingGroupMember} onReplaceMember={token=>{setReplacingGroupMember(current=>current===token?undefined:token);store.setSelection(null);}}
          onLocateMember={locateAssemblyMember}
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
        {editingAssemblyConstraint && <Space className="assembly-constraint-actions">
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
      </Form>
    </CommandDialog>
    <CommandDialog id="assembly-constraint-value" open={Boolean(pendingAssemblyConstraint)} title="约束定义" size="M"
      onClose={closeAssemblyCandidate}
      confirmLoading={command.isPending || assemblyPreviewPending} confirmDisabled={!assemblySupportsReady || !(assemblyPreviewSnapshot.matches("succeeded") || assemblyPreviewSnapshot.matches("definitionReady")) || replacingAssemblyReference !== undefined || !!replacingGroupMember ||
        Boolean(pendingAssemblyConstraint?.kind === "angle" && pendingAssemblyConstraint.angleRelation === "DIRECTED" && !pendingAssemblyConstraint.angleAxis)}
      onConfirm={async () => {
        if (!editingView || !pendingAssemblyConstraint) return;
        const pending = pendingAssemblyConstraint;
        const target=editingView.document;
        const intent=assemblyEditIntent(pending.kind.toUpperCase(),assemblyConstraintForm.getFieldsValue(true),resolvedAssemblyReferences,lengthUnit,undefined,
          {angleRelation:pending.angleRelation,angleAxis:resolvedAngleAxis,reverseAngleAxis:pending.reverseAngleAxis,angleReferenceDirection:pending.angleReferenceDirection});
        const key=assemblyIntentKey(intent,target.id,target.versionId,activeInstancePath);
        const sequence=assemblyPreviewSequence.current;
        const dialogGeneration=assemblyDialogLifecycle.current.capture();
        await assemblyConstraintForm.validateFields();
        const input=await currentAssemblyCandidate(target,key,sequence);
        if(!input){invalidateAssemblyDefinition();return;}
        if(sequence!==assemblyPreviewSequence.current || !(assemblyPreviewActor.current?.getSnapshot().matches("succeeded") || assemblyPreviewActor.current?.getSnapshot().matches("definitionReady")))return;
		assemblyPreviewActor.current?.send({type:"CONFIRM"});
        command.mutate(() => commitAssemblyCandidate(target,key,input,sequence), assemblyDialogLifecycle.current.bindCallbacks<DocumentView>(dialogGeneration,{ onSuccess: (updated) => { assemblyPreviewActor.current?.send({type:"COMMIT_SUCCESS"});if(updated.product?.constraints?.at(-1)?.evaluationStatus === "NOT_UPDATED")message.info("约束已保存，等待更新");viewport.current?.clearCommandPreview(false); assemblyPreviewID.current=undefined; setReconnectError(undefined); setAssemblyDefinitionDirty(false); setPendingAssemblyConstraint(undefined); },
		  onError:(cause)=>{assemblyPreviewActor.current?.send({type:"COMMIT_FAILURE",error:String(cause)});} }));
      }}>
      <Form form={assemblyConstraintForm} layout="vertical" onValuesChange={invalidateAssemblyDefinition}>
        {pendingAssemblyConstraint?.kind === "angle" && <AssemblyAngleParameters value={pendingAssemblyConstraint} view={editingView}
          axisSourceType={axisInspection?.supports[0]?.exactType} axisExactType={axisInspection?.supports[1]?.exactType} onDerive={deriveAngleAxis}
          onChange={value=>{invalidateAssemblyDefinition();if(!angleRelationSupportsMeasured(value.angleRelation))assemblyConstraintForm.setFieldValue("constraintMode","DRIVING");if(value.angleRelation === "PERPENDICULAR") assemblyConstraintForm.setFieldValue("directionRelation", assemblyConstraintForm.getFieldValue("directionRelation") === "OPPOSITE" ? "OPPOSITE" : "SAME");setPendingAssemblyConstraint({...pendingAssemblyConstraint,...value});}}
          onPick={()=>{store.setSelection(null);setReplacingAssemblyReference(2);store.setActiveTool("select","once");}} />}
        {pendingAssemblyConstraint && <AssemblyConstraintFields kind={pendingAssemblyConstraint.kind.toUpperCase() as AssemblyConstraint["kind"]} view={editingView} lengthUnit={lengthUnit}
          replacingMember={replacingGroupMember} onReplaceMember={token=>{setReplacingGroupMember(current=>current===token?undefined:token);store.setSelection(null);}}
          onLocateMember={locateAssemblyMember}
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
    {propertyTargets&&<ObjectPropertiesDialog targets={propertyTargets} pending={command.isPending}
      onRefresh={async()=>{const latest=await api.getDocument(view.document.id);const editing=editingView&&editingView.document.id!==view.document.id?await api.getDocument(editingView.document.id):latest;const targets=refreshPropertyTargets(propertyTargets,latest,editing,activeInstancePath,canEdit,canEditRoot);await refresh(latest);setPropertyTargets(targets)}}
      onClose={()=>{if(!command.isPending)setPropertyTargets(undefined)}} onPreview={previewObjectProperties} onPreviewRequest={requestObjectPreview}
      onApply={async properties=>{const target=propertyTargets[0];await command.mutateAsync(async()=>{
        if(target.target.kind==="KINEMATIC_OBJECT"){
          if(!view.product?.kinematics||!properties.name)throw new Error("机构对象或名称已变化");
          const updated=await api.command(target.documentId,{type:"SAVE_KINEMATICS",versionId:target.versionId,kinematics:renameMotionObject(view.product.kinematics,target.target.id,properties.name)});motionPanel.current?.adoptDefinition(updated);return updated;
        }
        return api.command(target.documentId,{type:"SET_PROPERTIES",versionId:target.versionId,properties});
      });setPropertyTargets(undefined)}}
      onOpenOriginal={id=>{setPropertyTargets(undefined);void openDocumentTab(id,client,api.openDocument,navigate)}}/>}
	<CommandDialog id="parameter-manager" open={parameterManagerOpen} title="参数" size="L"
		onClose={() => setParameterManagerOpen(false)} onConfirm={() => setParameterManagerOpen(false)}>
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
			<Input.Search aria-label="搜索参数" placeholder="搜索参数、别名或所属对象" value={parameterSearch} onChange={e=>setParameterSearch(e.target.value)}/><div className="parameter-manager-header"><span>参数 / 别名</span><span>来源</span><span>计算值</span><span /></div>
			{(editingView?.part?.parameters ?? []).length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Part 尚无参数" />
				: (editingView?.part?.parameters ?? []).filter(p=>[p.key,p.displayAlias,p.qualifiedDisplayPath,p.label].some(text=>text?.toLowerCase().includes(parameterSearch.trim().toLowerCase()))).map((parameter: ParameterDefinition) => <div className="parameter-manager-row" key={parameter.parameterId}>
					<span><strong>{parameter.qualifiedDisplayPath ?? parameter.displayName ?? parameter.label}</strong>
						{parameter.displayAlias && <Typography.Text type="secondary">{parameter.displayAlias}</Typography.Text>}
						</span>
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
		onClose={() => setPublicationManagerOpen(false)} onConfirm={() => setPublicationManagerOpen(false)}>
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
	<CommandDialog id="context-input-edit" open={Boolean(editingContextInput)} title="编辑上下文输入" size="S"
    onClose={()=>setEditingContextInput(undefined)} confirmLoading={command.isPending}
    confirmDisabled={!canEdit||!editingContextInput?.name.trim()}
    onConfirm={()=>{if(editingView&&editingContextInput)command.mutate(()=>api.editContextInput(editingView.document.id,editingContextInput.id,editingContextInput.name.trim(),editingContextInput.required),{onSuccess:()=>setEditingContextInput(undefined)});}}>
    <label className="cad-property-row"><span>名称</span><Input aria-label="上下文输入名称" value={editingContextInput?.name??""} onChange={event=>setEditingContextInput(current=>current?{...current,name:event.target.value}:current)}/></label>
    <Checkbox checked={editingContextInput?.required??false} onChange={event=>setEditingContextInput(current=>current?{...current,required:event.target.checked}:current)}>必需输入</Checkbox>
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
		confirmLoading={command.isPending} confirmDisabled={!parameterSourceReadonly&&parameterInputPreview.pending||Boolean(parameterInputPreview.error)} onConfirm={commitParameterEdit}>
		<Form form={parameterForm} layout="vertical">
			<Form.Item name="key" label="可读别名（可选）" rules={[{pattern:/^$|^[A-Za-z_][A-Za-z0-9_]*$/,
				message:"请输入 ASCII 标识符"}]}><Input /></Form.Item>
			<Form.Item name="source" label="值或表达式" rules={[{required:!parameterSourceReadonly}]}><ParameterReferenceInput disabled={parameterSourceReadonly} parameters={editingView?.part?.parameters??[]} excludeId={editingParameterID} unit={lengthUnit} placeholder="40 或 r_1 + 5"/></Form.Item>
            {parameterSourceReadonly&&<small className="cad-command-hint">{editingParameterDefinition?.role==="MEASURED"?"参考尺寸只测量；请在尺寸定义编辑中显式恢复驱动后再改原来源。":"外部来源只读，当前仅编辑名称。"}</small>}
			{parameterInputPreview.pending&&<Typography.Text type="secondary">正在检查表达式…</Typography.Text>}{parameterInputPreview.error&&<Alert type="error" message={parameterInputPreview.error}/>}{parameterInputPreview.parameterCandidates?.find(p=>p.parameterId===editingParameterID)&&<Typography.Text>计算值：{parameterDisplayValue(parameterInputPreview.parameterCandidates.find(p=>p.parameterId===editingParameterID)!,lengthUnit)}</Typography.Text>}
 {editingView&&Boolean(parameterInputPreview.failure)&&<DiagnosticCopy text={diagnosticReference(parameterInputPreview.failure,{documentId:editingView.document.id,versionId:editingView.document.versionId})}/>}
 <small className="cad-command-hint">长度增量请显式输入单位，如 r_1 + 5 mm。表达式按当前 Part 的参数别名编辑；提交后 AST 绑定稳定 ParameterId，后续重命名不会破坏引用。</small>
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
    {solidEditor && editingView?.part && <SolidFeatureEditor key={editingView.document.id+solidEditor.feature.id+solidEditor.feature.type} view={editingView} feature={solidEditor.feature} digest={solidEditor.digest} unit={lengthUnit} seed={store.selections} occurrencePath={activeInstancePath} occurrenceContext={featureOccurrenceContext} onSelectionSession={setFeatureSelection} onInputArtifact={featureInputChanged}
      onClose={()=>{viewport.current?.clearCommandPreview();setSolidEditor(undefined);}}
      onPreview={(artifact,operation,sketchFeatureId)=>{if(artifact)viewport.current?.previewArtifact(artifact,operation,sketchFeatureId);else viewport.current?.clearCommandPreview();}}
      onApply={input=>command.mutateAsync(()=>api.command(editingView.document.id,input))} />}
    {booleanDialog && editingView?.part && <BooleanFeatureDialog key={editingView.document.id+":"+(booleanDialog.feature?.id??"new")} view={editingView} initial={booleanDialog.feature} digest={booleanDialog.digest} bodyId={workingBodyID} seed={store.selections} occurrencePath={activeInstancePath} occurrenceContext={featureOccurrenceContext} onSelectionSession={setFeatureSelection} onInputArtifacts={setFeatureInputs}
      onClose={()=>{viewport.current?.clearCommandPreview();setBooleanDialog(undefined);}}
      onPreview={(artifact,operation)=>{if(artifact)viewport.current?.previewArtifact(artifact,operation);else viewport.current?.clearCommandPreview();}}
      onApply={input=>command.mutateAsync(()=>api.command(editingView.document.id,input))} />}
    {patternOpen && editingView?.document.type === "PRODUCT" && <InstancePatternDialog
      view={editingView} seed={store.selection} onSelectionSession={setFeatureSelection}
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
    {datumEditor && editingView?.part && <DatumEditor key={editingView.document.id+datumEditor+(editingDatumId??"")+(activeInstancePath ?? "")} kind={datumEditor} datumId={editingDatumId} view={editingView} seed={store.selections} unit={lengthUnit}
      occurrencePath={activeInstancePath} placement={{translation:activeResolvedInstance?.translation,rotation:activeResolvedInstance?.rotation}}
      onClose={()=>setDatumEditor(undefined)} onSelectionSession={setFeatureSelection} onPreview={setDatumPreview}
      onApply={input=>command.mutateAsync(()=>api.command(editingView.document.id,input))} />}
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
