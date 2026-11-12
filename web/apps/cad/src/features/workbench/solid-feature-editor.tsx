import {bindGeometricInput} from './geometric-input-selection';
import {geometryInputName} from './geometric-feature-model';
import {commandForView,viewContextKey} from "../../api/context-edit-scope";
import {boundContextParameter,ContextInputSource} from './context-input-source';
import {FeaturePickScope} from "../../cad/interaction/feature-selection";
import {CommandInputError} from "../../cad/command/operation-failure";
import {CommandField,CommandStatus,fieldErrorText} from "../../cad/overlay/command-form";
import { pickedPatternCenter } from "./pattern-selection";
import { PatternFeatureEditor } from "./pattern-feature-editor";
import { Button, Input, Segmented, Select, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { api } from "../../api/client";
import type { Artifact, DocumentView, Feature, FeatureInputResolution, SelectionItem } from "../../types";
import type { FeaturePickRole, FeatureSelectionSession } from "../../cad/interaction/feature-selection";
import { selectionKey } from "../../cad/interaction/selection-identity";
import { parameterDisplayValue,parameterSourceText } from "./parameter-editor";
import {ParameterReferenceInput} from './parameter-reference-input';
import type {DisplayLengthUnit} from '../../state/ui-preferences';
import type {SolidFeatureDraft,SolidQuantitySlot} from './solid-sketch-workflow';
import { booleanInputStages, solidParameterEdit, solidFeatureNames, pickLoftSection } from "./solid-feature-model";
import { selectedAxis, selectedSketch, sketchPick, profileSelectionIDs } from "./feature-picking";
import { FeaturePickField } from "./feature-pick-field";
import { useFeaturePreview } from "./use-feature-preview";
import {useFeatureDialogFeedback} from "./use-feature-dialog-feedback";
import {profileRegionCandidates,profileRegionPick} from "./profile-region-selection";
import { randomUUID } from "../../utils/random-uuid";
import type { FeatureInsertionContext } from "./historical-creation";
export type { FeatureInsertionContext } from "./historical-creation";
type BoundPick = {
    id: string;
    resolution: FeatureInputResolution;
    definition: NonNullable<Feature["selections"]>[number];
    visual?: SelectionItem;
};
const inputStatusText = (state: FeatureInputResolution) => ({RESOLVED:"有效",MISSING:"原对象已缺失",AMBIGUOUS:"引用存在歧义",TYPE_MISMATCH:"对象类型不匹配",OUTSIDE_CURRENT_TIP:"原对象已不在上游结果中",SOURCE_UNAVAILABLE:"引用来源不可用",CONTRACT_MISMATCH:"引用与来源不一致"})[state.status];
export function SolidFeatureEditor(props: Parameters<typeof SolidFeatureDefinitionEditor>[0]) {
    const unit=useRef(props.resume?.unit??props.unit).current;
    return props.feature.pattern ? <PatternFeatureEditor {...props} unit={unit}/> : <SolidFeatureDefinitionEditor {...props} unit={unit}/>;
}
function SolidFeatureDefinitionEditor({ view, feature, digest, unit, occurrencePath, occurrenceContext, intentRequestId, insertion, resume, onSketch, onClose, onApply, onPreview, onSelectionSession, onInputArtifact }: {
    view: DocumentView;
    feature: Feature;
    digest?: string;
    unit: DisplayLengthUnit;
    occurrencePath?: string;
    occurrenceContext?: Pick<SelectionItem, "instancePath" | "contextVariantKey" | "rootDocumentId">;
    intentRequestId?:string;
    insertion?:FeatureInsertionContext;
    resume?:SolidFeatureDraft;
    onSketch?:(draft:SolidFeatureDraft,sketchId?:string)=>void;
    onClose: () => void;
    onApply: (input: Record<string, unknown>) => Promise<unknown>;
    onPreview: (artifact?: Artifact, operation?: Feature["operation"], sketchFeatureId?:string) => void;
    onSelectionSession: (session?: FeatureSelectionSession) => void;
    onInputArtifact: (artifact?: Artifact | Artifact[]) => void;
}) {
    const boundSlot=(slot:string)=>boundContextParameter(view,view.part?.parameters?.find(p=>p.ownerFeatureId===feature.id&&p.propertySlot===slot)?.parameterId);
    const [loftEnd,setLoftEnd]=useState<'loftStart'|'loftEnd'>('loftStart');
    const [loftRows,setLoftRows]=useState<Record<'loftStart'|'loftEnd',BoundPick[]>>({loftStart:[],loftEnd:[]});
    const [loftReplacement,setLoftReplacement]=useState<string>();
    const [holeReplacement,setHoleReplacement]=useState<string>();
    const [sweepPickingSketch,setSweepPickingSketch]=useState(false);
    const [sweepReplacement,setSweepReplacement]=useState<string>();
    const [thinReplacement,setThinReplacement]=useState<string>();
    const hole=feature.type==="HOLE",sweep=feature.type==="SWEEP";
    const loft = feature.type === "LOFT", modifier = ["FILLET", "CHAMFER", "DRAFT", "SHELL"].includes(feature.type);
    const angular = feature.type === "REVOLVE" || feature.type === "DRAFT";
    const pickRole: FeaturePickRole = hole ? "sketch-point" : modifier ? (feature.type === "FILLET" || feature.type === "CHAMFER" ? "edge" : "face") : "profile";
    const [showConnections,setShowConnections]=useState(false);
    const acceptedSections=useRef<Feature["sections"]>(undefined);
    const [draft, setDraft] = useState<Feature>({ ...(resume?.feature??feature), profileSelection:(resume?.feature??feature).profileSelection??((resume?.feature??feature).thinWall?.input!=="CHAINS"&&!feature.id&&!modifier&&!loft&&!hole?{mode:"REGIONS",regionIds:[]}:undefined) });
    const terminating=!modifier&&!loft&&(draft.extent==="TO_PLANE"||draft.extent==="TO_SURFACE");
    const associated=terminating||draft.extent==="TO_NEXT",surface=draft.extent==="TO_SURFACE";
    const linear=["PAD","LINEAR_EXTRUDE"].includes(feature.type);
    const thinChains=draft.thinWall?.input==="CHAINS";
    const editSweep=(patch:Partial<NonNullable<Feature["sweep"]>>)=>{if(draft.sweep)edit({sweep:{...draft.sweep,...patch}})};
    const explicitRegions = !modifier&&!loft&&!hole&&!thinChains&&draft.profileSelection?.mode==="REGIONS";
    const profileRole:FeaturePickRole=hole?"sketch-point":explicitRegions?"profile-region":"profile";
    const [regionReplacement,setRegionReplacement]=useState<string>();
    const [sectionReplacement,setSectionReplacement]=useState<number>();
    const [role, setRoleState] = useState<FeaturePickRole|undefined>(!feature.id&&!modifier&&!loft&&!hole||feature.profileSelection?.mode==="REGIONS"?"profile-region":pickRole), [seamIndex, setSeamIndex] = useState<number>();
    const [pointResolution,setPointResolution]=useState<FeatureInputResolution>();
    // The initial lookup describes the saved point, never a replacement picked meanwhile.
    const pointReplaced=useRef(false);
    const [terminationVisual,setTerminationVisual]=useState<SelectionItem>();
    const [terminationLoadError,setTerminationLoadError]=useState<unknown>();
    const [terminationResolution,setTerminationResolution]=useState<FeatureInputResolution>();
    const [planeInputs,setPlaneInputs]=useState<Artifact[]>([]),[planeErrors,setPlaneErrors]=useState<string[]>([]),[planeLoadAttempt,setPlaneLoadAttempt]=useState(0);
    const [neutralVisual, setNeutralVisual] = useState<SelectionItem>();
    const [neutralResolution, setNeutralResolution] = useState<FeatureInputResolution>();
    const [replacement, setReplacement] = useState<string>();
    const [inputError, setInputError] = useState<unknown>();
    const [picks, setPicks] = useState<BoundPick[]>([]), [axisVisual, setAxisVisual] = useState<SelectionItem>();
    const [error, setError] = useState<unknown>(), [committing, setCommitting] = useState(false), [binding, setBinding] = useState(0);
    const [inputContext, setInputContext] = useState<Awaited<ReturnType<typeof api.getFeatureInput>>>();
    const [inputAttempt,setInputAttempt]=useState(0);
    const [loadingInput, setLoadingInput] = useState(!!insertion || !!feature.id);
    const persisted = (slot: string, fallback: number) => {
        const p = view.part?.parameters?.find(p => p.ownerFeatureId === feature.id && p.propertySlot === slot);
        return p ? parameterSourceText(p, slot.startsWith("loft:")?"1":(slot === "angle"||slot==="countersinkAngle"||slot==="drillTipAngle") ? "deg" : unit, true) : String((slot.startsWith("loft:")||slot === "angle"||slot==="countersinkAngle"||slot==="drillTipAngle") ? fallback : fallback / (({ mm: 1, cm: 10, m: 1000, in: 25.4 } as Record<string, number>)[unit] ?? 1));
    };
    const original = useRef({"loft:startMagnitude":feature.id&&feature.loftStart?persisted("loft:startMagnitude",feature.loftStart.magnitude):"","loft:endMagnitude":feature.id&&feature.loftEnd?persisted("loft:endMagnitude",feature.loftEnd.magnitude):"",drillTipAngle:feature.id&&feature.drillTipAngle?persisted("drillTipAngle",feature.drillTipAngle):"","thread:majorDiameter":feature.id&&feature.thread?persisted("thread:majorDiameter",feature.thread.majorDiameter):"","thread:pitch":feature.id&&feature.thread?persisted("thread:pitch",feature.thread.pitch):"","thread:depth":feature.id&&feature.thread?persisted("thread:depth",feature.thread.depth):"",recessDiameter:feature.id&&feature.holeKind&&feature.holeKind!=="SIMPLE"?persisted("recessDiameter",feature.recessDiameter??10):"",recessDepth:feature.id&&feature.holeKind==="COUNTERBORE"?persisted("recessDepth",feature.recessDepth??3):"",countersinkAngle:feature.id&&feature.holeKind==="COUNTERSINK"?persisted("countersinkAngle",feature.countersinkAngle??90):"",diameter:feature.id?persisted("diameter",feature.diameter??5):"",depth:feature.id?persisted("depth",feature.depth??10):"", length: feature.id ? persisted("length", feature.length ?? 10) : "", length2: feature.id && feature.extent === "TWO_SIDED" ? persisted("length2", feature.length2 ?? 10) : "", angle: feature.id ? persisted("angle", feature.angle ?? 360) : "",startOffset:feature.id?persisted("startOffset",feature.startOffset??0):"",endOffset:feature.id?persisted("endOffset",feature.endOffset??0):"","thin:thickness":feature.id&&feature.thinWall?persisted("thin:thickness",feature.thinWall.thickness):"","thin:thickness2":feature.id&&feature.thinWall?.mode==="TWO_SIDED"?persisted("thin:thickness2",feature.thinWall.thickness2??1):"" });
    const initial=(slot:SolidQuantitySlot,fallback:number)=>resume?.quantities[slot]??persisted(slot,fallback);
    const [recessDiameter,setRecessDiameter]=useState(initial("recessDiameter",feature.recessDiameter??10)),[recessDepth,setRecessDepth]=useState(initial("recessDepth",feature.recessDepth??3)),[countersinkAngle,setCountersinkAngle]=useState(initial("countersinkAngle",feature.countersinkAngle??90));
    const [startMagnitude,setStartMagnitude]=useState(initial("loft:startMagnitude",feature.loftStart?.magnitude??1)),[endMagnitude,setEndMagnitude]=useState(initial("loft:endMagnitude",feature.loftEnd?.magnitude??1));
    const [tipAngle,setTipAngle]=useState(initial("drillTipAngle",feature.drillTipAngle??118)),[threadDiameter,setThreadDiameter]=useState(initial("thread:majorDiameter",feature.thread?.majorDiameter??6)),[threadPitch,setThreadPitch]=useState(initial("thread:pitch",feature.thread?.pitch??1)),[threadDepth,setThreadDepth]=useState(initial("thread:depth",feature.thread?.depth??5));
    const [diameter,setDiameter]=useState(initial("diameter",feature.diameter??5)),[depth,setDepth]=useState(initial("depth",feature.depth??10));
    const [length, setLength] = useState(initial("length", feature.length ?? 10)), [second, setSecond] = useState(initial("length2", feature.length2 ?? 10)), [angle, setAngle] = useState(initial("angle", feature.angle ?? 360));
    const [startOffset,setStartOffset]=useState(initial("startOffset",feature.startOffset??0)),[endOffset,setEndOffset]=useState(initial("endOffset",feature.endOffset??0));
    const [thinThickness,setThinThickness]=useState(initial("thin:thickness",feature.thinWall?.thickness??1)),[thinSecond,setThinSecond]=useState(initial("thin:thickness2",feature.thinWall?.thickness2??1));
    const frozen=useRef(commandForView(view,{})),scopeKey=useRef(viewContextKey(view));
    const intent = useRef(intentRequestId??resume?.intentId??insertion?.intentId??randomUUID()), version = useRef(view.document.versionId), alive = useRef(true), queue = useRef(Promise.resolve());
    const pickOwner=useRef(new FeaturePickScope()),pickContext=JSON.stringify([viewContextKey(view),replacement,regionReplacement,seamIndex,sectionReplacement,draft.extent,draft.thinWall?.input,loftEnd,loftReplacement]);
    pickOwner.current.activate(role,pickContext);
    const setRole=(next:FeaturePickRole|undefined)=>{pickOwner.current.cancel();pickOwner.current.activate(next,pickContext);setRoleState(next);setBinding(0)};
    const cancelPick=()=>{if(!role)return false;pickOwner.current.cancel();setRoleState(undefined);setBinding(0);setReplacement(undefined);setRegionReplacement(undefined);setSectionReplacement(undefined);setSweepReplacement(undefined);setThinReplacement(undefined);setSweepPickingSketch(false);return true};
    const callbacks = useRef({ onSelectionSession, onInputArtifact });
    callbacks.current = { onSelectionSession, onInputArtifact };
    const index = feature.id ? view.part!.features.findIndex(f => f.id === feature.id) : insertion ? view.part!.features.findIndex(f=>f.id===insertion.afterFeatureId)+1 : view.part!.features.length;
    const upstream = view.part!.features.slice(0, Math.max(0, index));
    const profileUpstream=upstream;
    const edit = (patch: Partial<Feature>) => { setError(undefined); setDraft(current => ({ ...current, ...patch })); };
    const editBoundary=(end:'loftStart'|'loftEnd',patch:Partial<NonNullable<Feature['loftStart']>>)=>{if(draft[end])edit({[end]:{...draft[end],...patch}})};
    const editThin=(patch:Partial<NonNullable<Feature["thinWall"]>>)=>{if(draft.thinWall)edit({thinWall:{...draft.thinWall,...patch,...(patch.entityIds?{directions:draft.thinWall.directions?.filter(d=>patch.entityIds!.includes(d.entityId))}:{})}})};
    const validRevision = scopeKey.current===viewContextKey(view) && version.current === view.document.versionId && (!insertion || insertion.versionId===view.document.versionId);
    useEffect(() => { alive.current = true; pickOwner.current.activate(role,pickContext); return () => { alive.current = false; pickOwner.current.cancel(); callbacks.current.onSelectionSession(); callbacks.current.onInputArtifact(); }; }, []);
    useEffect(() => {
        if (!feature.id && !insertion)
            return;
        let current = true;
        const controller = new AbortController();
        setLoadingInput(true);setInputError(undefined);setTerminationLoadError(undefined);
        void api.getFeatureInput(view.document.id, { ...frozen.current,versionId: view.document.versionId, featureId: feature.id||undefined,afterFeatureId:insertion?.afterFeatureId,bodyId:insertion?.bodyId }, controller.signal).then(context => {
            if (!current)
                return;
            setInputContext(context);
            if(loft){const rows=(end:'loftStart'|'loftEnd')=>(feature[end]?.supports??[]).map((definition,i)=>{const r=context[end==='loftStart'?'loftStartPicks':'loftEndPicks']?.[i]??{status:'SOURCE_UNAVAILABLE' as const};return {id:`original/${end}/${i}`,definition,resolution:r,visual:r.status==='RESOLVED'&&'localId' in r&&r.localId?{...occurrenceContext,kind:'face' as const,id:`loft-input:${end}:${i}`,documentId:view.document.id,bodyId:r.bodyId,geometryKey:r.geometryKey,topologyId:r.localId,versionId:version.current,occurrencePath,displayStageFeatureId:r.displayStageFeatureId}:undefined}});setLoftRows({loftStart:rows('loftStart'),loftEnd:rows('loftEnd')});}
            if(!pointReplaced.current)setPointResolution(context.pointPick);
            if(context.terminationPick){const p=context.terminationPick;setTerminationResolution(p);if(p.status==="RESOLVED"&&p.localId)setTerminationVisual({...occurrenceContext,kind:"face",id:`input:termination:${p.localId}`,documentId:view.document.id,bodyId:p.bodyId,versionId:context.versionId,geometryKey:p.geometryKey,displayStageFeatureId:p.displayStageFeatureId,topologyId:p.localId,occurrencePath});}
            if (context.neutralPick) {
                const p = context.neutralPick;
                setNeutralResolution(p);
                if (p.status === "RESOLVED" && p.localId) setNeutralVisual({ ...occurrenceContext, kind:"face", id:`input:neutral:${p.localId}`, documentId:view.document.id, bodyId:p.bodyId, versionId:context.versionId, geometryKey:p.geometryKey, displayStageFeatureId:p.displayStageFeatureId, topologyId:p.localId, occurrencePath });
            }
            setPicks(context.picks.map(p => ({ id:`original/${p.index}`, definition: feature.selections![p.index], resolution:p, visual: p.status === "RESOLVED" && p.localId ? { ...occurrenceContext, kind: p.kind.toLowerCase() as "edge" | "face", id: `input:${p.kind}:${p.localId}`, documentId: view.document.id, bodyId: feature.bodyId, displayStageFeatureId:context.sourceFeatureId, versionId: context.versionId, geometryKey: context.artifact.geometryKey, topologyId: p.localId, occurrencePath } : undefined })));
        }).catch(cause => { if (current && !controller.signal.aborted)
            if(["TO_PLANE","TO_SURFACE"].includes(feature.extent??""))setTerminationLoadError(cause);else setInputError(cause); }).finally(() => { if (current)
            setLoadingInput(false); });
        return () => { current = false; controller.abort(); };
    }, [inputAttempt]);
    useEffect(()=>{
        if(!(feature.id||insertion)||!(terminating||loft||feature.type==="DRAFT")){setPlaneInputs([]);setPlaneErrors([]);return;}
        const controller=new AbortController();let active=true;
        const sourceBodies=new Set(booleanInputStages(upstream).map(f=>f.bodyId));
        const bodies=view.part!.bodies.filter(b=>sourceBodies.has(b.id));
        void Promise.allSettled(bodies.map(b=>api.getFeatureInput(view.document.id,{...frozen.current,versionId:version.current,featureId:feature.id||undefined,afterFeatureId:insertion?.afterFeatureId,bodyId:insertion?.bodyId??b.id,referenceBodyId:insertion?b.id:undefined},controller.signal))).then(results=>{
            if(!active)return;const inputs:Artifact[]=[],errors:string[]=[];
            results.forEach((r,i)=>{if(r.status==="fulfilled"){if(r.value.sourceFeatureId)inputs.push(r.value.artifact);}else errors.push(bodies[i].name)});
            setPlaneInputs(inputs);setPlaneErrors(errors);
        });
        return()=>{active=false;controller.abort()};
    },[terminating,loft,planeLoadAttempt]);
    useEffect(()=>{
        if(!inputContext&&!planeInputs.length)return;
        const artifacts=[...(inputContext?(inputContext.artifacts??[inputContext.artifact]):[]),...planeInputs];
        callbacks.current.onInputArtifact([...new Map(artifacts.map(a=>[`${a.bodyId}/${a.geometryKey}`,a])).values()]);
    },[inputContext,planeInputs]);
    const current = useRef({ draft, role, inputContext, upstream, committing, validRevision, picks });
    current.current = { draft, role, inputContext, upstream, committing, validRevision, picks };
    const bindPick = (selection: SelectionItem, replaceID?: string) => {
        const ticket=pickOwner.current.capture();
        setBinding(n => n + 1);
        queue.current = queue.current.then(async () => {
            if (!alive.current || !current.current.validRevision || !pickOwner.current.current(ticket) || current.current.committing)
                return;
            const state = current.current;
            if ((selection.kind !== "edge" && selection.kind !== "face") || selection.kind !== pickRole || selection.bodyId !== state.draft.bodyId || !selection.geometryKey)
                return;
            let definition: NonNullable<Feature["selections"]>[number];
            if (feature.id || insertion) {
                if (!state.inputContext || selection.geometryKey !== state.inputContext.artifact.geometryKey)
                    return;
                const result = await api.getFeatureInput(view.document.id, { ...frozen.current,versionId: version.current, featureId: feature.id||undefined, afterFeatureId:insertion?.afterFeatureId,bodyId:insertion?.bodyId, geometryKey: selection.geometryKey, kind: selection.kind.toUpperCase(), localId: selection.topologyId });
                if (!result.selection)
                    throw new CommandInputError("所选边面没有可用的持久引用");
                definition = result.selection;
            }
            else {
                const result = await api.getTopologyProperties(view.document.id, selection.geometryKey, selection.kind.toUpperCase() as "EDGE" | "FACE", selection.topologyId, selection.versionId ?? version.current);
                if (!result.persistentSelection)
                    throw new CommandInputError("所选边面没有可用的持久引用");
                definition = { selection: result.persistentSelection, sourceVersionId: selection.versionId ?? version.current };
            }
            if (!alive.current || !current.current.validRevision || !pickOwner.current.current(ticket))
                return;
            const key = selectionKey(selection);
            if (replaceID && current.current.picks.some(p=>p.id!==replaceID && p.visual && selectionKey(p.visual)===key)) {
                setError("该对象已在输入中，请选择其他对象或明确移除失效项");
                return;
            }
            setPicks(previous => {
                const bound: BoundPick = {id:replaceID??randomUUID(),definition,visual:selection,resolution:{status:"RESOLVED"}};
                if (replaceID) return previous.map(p=>p.id===replaceID?bound:p);
                return previous.some(p=>p.visual && selectionKey(p.visual)===key) ? previous.filter(p=>!p.visual || selectionKey(p.visual)!==key) : [...previous,bound];
            });
            setReplacement(previous=>previous===replaceID?undefined:previous);if(replaceID)setRole(undefined);
            setError(undefined);
        }).catch(cause => { if (alive.current&&pickOwner.current.current(ticket))
            setError(cause); }).finally(() => { if (alive.current && pickOwner.current.current(ticket))
            setBinding(n => Math.max(0,n - 1)); });
    };
    const bindNeutralPlane = (selection: SelectionItem, termination=false) => {
        const ticket=pickOwner.current.latest();
        if (selection.kind !== "face" || !selection.geometryKey) return;
        setBinding(1);
        queue.current = queue.current.then(async () => {
            if (!alive.current || !current.current.validRevision || !pickOwner.current.current(ticket) || current.current.committing) return;
            let definition: NonNullable<Feature["selections"]>[number];
            if (feature.id || insertion) {
                const result = await api.getFeatureInput(view.document.id, { ...frozen.current,versionId:version.current, featureId:feature.id||undefined,afterFeatureId:insertion?.afterFeatureId, bodyId:insertion?.bodyId??selection.bodyId,referenceBodyId:insertion?selection.bodyId:undefined, geometryKey:selection.geometryKey, kind:"FACE", localId:selection.topologyId });
                if (!result.selection || !(termination&&current.current.draft.extent==="TO_SURFACE")&&result.selection.selection.creationEvidence.geometryType !== "PLANE") throw new CommandInputError(termination?"终止目标必须是平面":"中性平面必须是平面");
                definition = result.selection;
            } else {
                const result = await api.getTopologyProperties(view.document.id, selection.geometryKey!, "FACE", selection.topologyId, version.current);
                if (!(termination&&current.current.draft.extent==="TO_SURFACE")&&result.geometryType !== "PLANE" || !result.persistentSelection) throw new CommandInputError("请选择可绑定的实体平面");
                definition = { selection:result.persistentSelection, sourceVersionId:version.current };
            }
            if (!alive.current || !current.current.validRevision || !pickOwner.current.current(ticket)) return;
            if(termination){const extent=current.current.draft.extent;if(extent!=="TO_PLANE"&&extent!=="TO_SURFACE")return;edit({terminationPlaneId:undefined,terminationPlane:extent==="TO_PLANE"?definition:undefined,terminationSurface:extent==="TO_SURFACE"?definition:undefined});setTerminationResolution({status:"RESOLVED"});setTerminationLoadError(undefined);setTerminationVisual(selection);setRole(undefined);}
            else {edit({neutralPlaneId:undefined, neutralPlane:definition}); setNeutralResolution({status:"RESOLVED"}); setNeutralVisual(selection); setRole(undefined);}
        }).catch(cause => { if (alive.current&&pickOwner.current.current(ticket)) setError(cause); }).finally(()=>{if(alive.current&&pickOwner.current.current(ticket))setBinding(n=>Math.max(0,n-1));});
    };
    const bindLoftSupport=(selection:SelectionItem,end:'loftStart'|'loftEnd')=>{
     const ticket=pickOwner.current.capture();if(selection.kind!=='face'||!selection.geometryKey)return;setBinding(n=>n+1);
     queue.current=queue.current.then(async()=>{if(!alive.current||!current.current.validRevision||!pickOwner.current.current(ticket))return;
      let definition:NonNullable<Feature['selections']>[number];
      if(feature.id||insertion){const r=await api.getFeatureInput(view.document.id,{...frozen.current,versionId:version.current,featureId:feature.id||undefined,afterFeatureId:insertion?.afterFeatureId,bodyId:insertion?.bodyId??selection.bodyId,referenceBodyId:insertion?selection.bodyId:undefined,geometryKey:selection.geometryKey,kind:'FACE',localId:selection.topologyId});if(!r.selection)throw new CommandInputError('所选面不在合法上游阶段');definition=r.selection;}
      else {const r=await api.getTopologyProperties(view.document.id,selection.geometryKey!,'FACE',selection.topologyId,selection.versionId??version.current);if(!r.persistentSelection)throw new CommandInputError('所选面缺少持久身份');definition={selection:r.persistentSelection,sourceVersionId:version.current};}
      if(!alive.current||!current.current.validRevision||!pickOwner.current.current(ticket))return;
      const key=JSON.stringify(definition.selection.anchor),rows=loftRows[end];
      if(rows.some(p=>p.id!==loftReplacement&&JSON.stringify(p.definition.selection.anchor)===key)){setError('该支撑面已在列表中');return;}
      const row:BoundPick={id:loftReplacement??randomUUID(),definition,resolution:{status:'RESOLVED'},visual:selection};const next=loftReplacement?rows.map(p=>p.id===loftReplacement?row:p):[...rows,row];
      setLoftRows(r=>({...r,[end]:next}));editBoundary(end,{supports:next.map(p=>p.definition)});setLoftReplacement(undefined);if(loftReplacement)setRole(undefined);setError(undefined);
     }).catch(cause=>{if(alive.current&&pickOwner.current.current(ticket))setError(cause)}).finally(()=>{if(alive.current&&pickOwner.current.current(ticket))setBinding(n=>Math.max(0,n-1))});
    };
    const pick = (selection: SelectionItem, chosenRole: FeaturePickRole|undefined = current.current.role) => {
		if(chosenRole==='profile-region'&&selection.kind==='sketch')chosenRole='profile';
        if (!chosenRole || (selection.ownerDocumentId ?? selection.documentId) !== view.document.id || (selection.occurrencePath ?? "") !== (occurrencePath ?? "") || selection.versionId && selection.versionId !== version.current || committing || !validRevision)
            return;
        if(chosenRole==='geometry'&&thinChains){
            if(selection.kind!=='visual'||selection.visualType!=='CURVE')return;
            const source=upstream.find(f=>f.id===selection.featureId&&f.sketch&&!f.suppressed),entity=source?.sketch?.entities.find(e=>e.id===selection.entityId&&!e.suppressed&&['LINE','ARC','CIRCLE'].includes(e.kind));
            if(!source||!entity)return;
            const selected=source.id===draft.profile?draft.thinWall?.entityIds??[]:[];
            if(thinReplacement&&(source.id!==draft.profile||selected.includes(entity.id)&&entity.id!==thinReplacement)){setError('请选择同一草图中尚未使用的曲线');return;}
            const next=thinReplacement?selected.map(id=>id===thinReplacement?entity.id:id):selected.includes(entity.id)?selected.filter(id=>id!==entity.id):[...selected,entity.id];
            edit({profile:source.id,profileMemberSlot:undefined,profileSelection:undefined,thinWall:{...draft.thinWall!,directions:source.id===draft.profile?draft.thinWall?.directions?.filter(d=>next.includes(d.entityId)&&d.entityId!==entity.id):undefined,entityIds:next}});
            if(thinReplacement){setThinReplacement(undefined);setRole(undefined)}return;
        }
        if(hole&&chosenRole==="sketch-point") {
            if(selection.kind!=="visual")return;
            const sketch=upstream.find(f=>f.id===selection.featureId&&f.type==="SKETCH"&&!f.suppressed);
            const entity=sketch?.sketch?.entities.find(e=>e.id===selection.entityId&&e.kind==="POINT"&&!e.sourceEntityId&&!e.suppressed);
            if(selection.kind!=="visual"||selection.visualType!=="POINT"||!sketch||!entity||["FAILED_SUPPORT","BLOCKED"].includes(sketch.sketch!.support.status??""))return;
            const ref={sketchId:sketch.id,reference:{target:"ENTITY" as const,entityId:entity.id,subElement:"POINT" as const}};
            pointReplaced.current=true;
            if(draft.holePoints){const key=(p:NonNullable<Feature['holePoint']>)=>p.sketchId+'/'+p.reference.entityId,k=key(ref),items=draft.holePoints;
             if(holeReplacement&&items.some(p=>key(p)===k&&key(p)!==holeReplacement)){setError('该孔位已在孔组中');return;}
             edit({holePoints:holeReplacement?items.map(p=>key(p)===holeReplacement?ref:p):items.some(p=>key(p)===k)?items.filter(p=>key(p)!==k):[...items,ref]});setHoleReplacement(undefined);if(holeReplacement)setRole(undefined);
            }else {edit({holePoint:ref});setRole(undefined);}
            setPointResolution({status:"RESOLVED"});return;
        }
        if(loft&&chosenRole==="face"){bindLoftSupport(selection,loftEnd);return;}
 if(loft&&chosenRole==="axis"){const axis=selectedAxis(view,selection,upstream);if(axis){editBoundary(loftEnd,{directionEntityId:axis});setRole(undefined)}else setError("请选择基准轴或上游草图直线");return;}
 if(sweep&&chosenRole==='geometry'&&(selection.kind==='edge'||upstream.some(f=>f.id===(selection.result?.featureId??selection.entityId??selection.id)&&f.result?.domain==='WIRE_SET'))){
   const ticket=pickOwner.current.capture();setBinding(n=>n+1);
   void bindGeometricInput(view,selection,version.current,upstream).then(geometry=>{if(alive.current&&pickOwner.current.current(ticket)){editSweep({path:{sketchId:'',entityIds:[],geometry}});setRole(undefined)}}).catch(error=>{if(alive.current&&pickOwner.current.current(ticket))setError(error)}).finally(()=>{if(alive.current)setBinding(n=>Math.max(0,n-1))});return;
 }
 if(sweep&&chosenRole==='geometry'&&selection.kind==='visual'&&selection.visualType==='CURVE') {
  const source=upstream.find(f=>f.id===selection.featureId&&f.sketch&&!f.suppressed),entity=source?.sketch?.entities.find(e=>e.id===selection.entityId&&!e.suppressed&&e.kind!=='POINT');
  if(!source||!entity)return;
  const path=draft.sweep!.path,same=path.sketchId===source.id,ids=same?path.entityIds:[];
  if(sweepReplacement&&(!same||ids.includes(entity.id)&&entity.id!==sweepReplacement)){setError('请选择同一路径草图中尚未使用的曲线');return;}
  const next=sweepReplacement?ids.map(id=>id===sweepReplacement?entity.id:id):ids.includes(entity.id)?ids.filter(id=>id!==entity.id):[...ids,entity.id];
  editSweep({path:{...path,geometry:undefined,sketchId:source.id,entityIds:next,direction:same&&next.includes(path.direction?.entityId??'')?path.direction:undefined}});
  if(sweepReplacement){setSweepReplacement(undefined);setRole(undefined)}return;
 }
 if(sweep&&chosenRole==='profile'&&sweepPickingSketch){
  const source=selectedSketch(view,selection,upstream);if(!source?.sketch)return;
  editSweep({path:source.id===draft.sweep!.path.sketchId?draft.sweep!.path:{sketchId:source.id,entityIds:[]}});setSweepPickingSketch(false);setSweepReplacement(undefined);setRole('geometry');return;
 }
 if (chosenRole === "edge" || chosenRole === "face") {
            bindPick(selection, replacement);
            return;
        }
        if (chosenRole === "point" && loft) {
            const point=pickedPatternCenter(selection);
            if(!point || point.sketchId && !upstream.some(f=>f.id===point.sketchId))return;
            const section={sketchId:point.sketchId??"",point};
            edit({sections:sectionReplacement===undefined?[...(draft.sections??[]).filter(s=>JSON.stringify(s.point)!==JSON.stringify(point)),section]:(draft.sections??[]).map((s,i)=>i===sectionReplacement?section:s)});
            setSectionReplacement(undefined);setRole(undefined);
            return;
        }
        if(chosenRole === "profile-region" && selection.kind==="profile-region") {
            const candidate=profileRegionCandidates(view,upstream,draft.profile).find(c=>c.sketchId===selection.featureId);
            if(!candidate?.regions.some(r=>r.id===selection.regionId))return;
            const ids=draft.profileSelection?.regionIds??[];
            if(regionReplacement&&ids.includes(selection.regionId)&&regionReplacement!==selection.regionId){setError("该区域已在输入中，请选择其他区域或明确移除失效项");return;}
            const next=regionReplacement?ids.map(id=>id===regionReplacement?selection.regionId:id):ids.includes(selection.regionId)?ids.filter(id=>id!==selection.regionId):[...ids,selection.regionId];
            edit({profile:selection.featureId,profileMemberSlot:undefined,profileSelection:{mode:"REGIONS",regionIds:next}});setRegionReplacement(undefined);if(regionReplacement)setRole(undefined);
            return;
        }
        if (chosenRole === "profile") {
            const sketch = selectedSketch(view, selection, upstream);
            if (!sketch)
                return;
            if (loft) {
                if(sectionReplacement===undefined)setDraft(previous=>({...previous,sections:pickLoftSection(previous.sections??[],sketch)}));
                else {if(draft.sections?.some((s,i)=>i!==sectionReplacement&&s.sketchId===sketch.id&&s.memberSlot===sketch.profileMemberSlot)){setError("该截面已在列表中");return;}edit({sections:draft.sections?.map((s,i)=>i===sectionReplacement?{sketchId:sketch.id,memberSlot:sketch.profileMemberSlot}:s)});setSectionReplacement(undefined);setRole(undefined);}
            }
            else {
                edit({ ...(thinChains&&sketch.id!==draft.profile?{thinWall:{...draft.thinWall!,entityIds:[],directions:undefined}}:{}),profile: sketch.id, profileMemberSlot:sketch.type==="SKETCH_PATTERN"?sketch.profileMemberSlot:undefined, profileSelection:explicitRegions?{mode:"REGIONS",regionIds:[...(view.sketchAnalyses?.[sketch.id]?.regionIds??[])]}:draft.profileSelection });
                if (feature.type === "REVOLVE" && !draft.axisEntityId)
                    setRole("axis");
                else setRole(undefined);
            }
        }
        else if (chosenRole === "axis") {
            const axis = selectedAxis(view, selection, upstream);
            if (!axis) {
                setError("请选择草图中的直线或基准轴");
                return;
            }
            edit({ axisEntityId: axis });
            setAxisVisual(selection);setRole(undefined);
        }
        else if (chosenRole === "plane" && selection.kind === "plane") {
            const plane = view.part?.datumPlanes.find(p => p.id === selection.datumPlane?.id || p.id === selection.entityId || p.id === selection.id);
            if (plane) {
                if(terminating){if(surface)return;edit({terminationPlaneId:plane.id,terminationPlane:undefined,terminationSurface:undefined});setTerminationResolution({status:"RESOLVED"});setTerminationLoadError(undefined);setTerminationVisual(selection);setRole(undefined);return;}
                edit({ neutralPlaneId: plane.id, neutralPlane: undefined });
                setNeutralResolution({status:"RESOLVED"});
                setNeutralVisual(selection);
                setRole(undefined);
            }
        }
        else if (chosenRole === "plane" && selection.kind === "face") { bindNeutralPlane(selection,terminating); }
        else if (chosenRole === "seam" && selection.kind === "visual" && seamIndex !== undefined) {
            edit({ sections: (acceptedSections.current??draft.sections)?.map((s, i) => i === seamIndex && (s.sketchId === selection.featureId || s.sketchId===selection.patternId&&s.memberSlot===selection.patternMemberSlot) ? { ...s, correspondenceResolved:true, seamEntityId: selection.entityId } : s) });
            setRole(undefined);
        }
        else if (chosenRole === "body" && selection.bodyId) {
            edit({ bodyId: selection.bodyId });
            setRole(undefined);
        }
    };
    const pickRef = useRef(pick);
    pickRef.current = pick;
    const axisParts = draft.axisEntityId?.split(":");
    const restoredAxis: SelectionItem | undefined = axisParts?.[0] === "SKETCH_LINE" ? {
        kind: "visual", visualType: "CURVE", id: `${occurrencePath || "root"}:${axisParts[1]}:${axisParts[2]}`, featureId: axisParts[1], entityId: axisParts[2], bodyId: upstream.find(f => f.id === axisParts[1])?.bodyId, documentId: view.document.id, versionId: version.current, occurrencePath
    } : undefined;
    const holeRefs=draft.holePoints??(draft.holePoint?[draft.holePoint]:[]);
    const holeKey=(p:NonNullable<Feature['holePoint']>)=>p.sketchId+'/'+p.reference.entityId;
    const validHoleRef=(p:NonNullable<Feature['holePoint']>)=>{const s=upstream.find(f=>f.id===p.sketchId&&!f.suppressed);return !!s?.sketch&&!['FAILED_SUPPORT','BLOCKED'].includes(s.sketch.support.status??'')&&s.sketch.entities.some(e=>e.id===p.reference.entityId&&e.kind==='POINT'&&!e.sourceEntityId&&!e.suppressed)};
    const holeReady=!!holeRefs.length&&holeRefs.every(validHoleRef)&&(!pointResolution||pointResolution.status==='RESOLVED');
    const highlights: SelectionItem[] = hole ? holeRefs.filter(validHoleRef).map(p=>({kind:'visual',visualType:'POINT',id:`${occurrencePath||'root'}:${p.sketchId}:${p.reference.entityId}`,featureId:p.sketchId,entityId:p.reference.entityId,sketchReference:p.reference,bodyId:upstream.find(f=>f.id===p.sketchId)?.bodyId,documentId:view.document.id,versionId:version.current,occurrencePath} as SelectionItem)) : modifier ? picks.flatMap(p => p.visual ? [p.visual] : []) : loft ? (draft.sections ?? []).map(s => s.point?.sketchId ? ({kind:"visual",visualType:"POINT",id:`loft-point:${s.sketchId}:${JSON.stringify(s.point.reference)}`,featureId:s.sketchId,entityId:s.point.reference.entityId??s.point.reference.target,sketchReference:s.point.reference,documentId:view.document.id,versionId:version.current,occurrencePath} as SelectionItem) : s.point?.axisEntityId ? ({kind:"axis-system",id:s.point.axisEntityId.split(":")[1],entityId:s.point.axisEntityId.split(":")[1],documentId:view.document.id,versionId:version.current,occurrencePath} as SelectionItem) : sketchPick(view, s.sketchId, occurrencePath, s.memberSlot)) : draft.profile ? explicitRegions ? (draft.profileSelection?.regionIds??[]).map(id=>profileRegionPick(view,draft.profile!,id,occurrencePath)) : [sketchPick(view, draft.profile, occurrencePath, draft.profileMemberSlot)] : [];
    if(sweep&&draft.sweep)for(const id of draft.sweep.path.entityIds)highlights.push({...occurrenceContext,kind:"visual",visualType:"CURVE",id:`sweep:${draft.sweep.path.sketchId}:${id}`,featureId:draft.sweep.path.sketchId,entityId:id,documentId:view.document.id,versionId:version.current,occurrencePath});
    if(thinChains&&draft.profile)for(const id of draft.thinWall?.entityIds??[])if(upstream.find(f=>f.id===draft.profile)?.sketch?.entities.some(e=>e.id===id&&!e.suppressed))highlights.push({...occurrenceContext,kind:"visual",visualType:"CURVE",id:`thin:${draft.profile}:${id}`,featureId:draft.profile!,entityId:id,bodyId:upstream.find(f=>f.id===draft.profile)?.bodyId,documentId:view.document.id,versionId:version.current,occurrencePath});
    if (axisVisual ?? restoredAxis)
        highlights.push((axisVisual ?? restoredAxis)!);
    if(loft)for(const end of ["loftStart","loftEnd"] as const)highlights.push(...loftRows[end].flatMap(p=>p.visual?[p.visual]:[]));
    if (neutralVisual) highlights.push(neutralVisual);
    if(terminating&&terminationVisual)highlights.push(terminationVisual);
    const contextSelections = feature.type === "REVOLVE" && role === "axis" ? highlights.filter(s => s.kind === "sketch"||s.kind==="profile-region") : [];
    const selections = contextSelections.length ? highlights.filter(s => s.kind !== "sketch"&&s.kind!=="profile-region") : highlights;
    const highlightsToken = JSON.stringify([selections, contextSelections]);
    const seamSection=seamIndex!==undefined?draft.sections?.[seamIndex]:undefined;
    const sketchIds = (thinChains||sweep&&role==="geometry")?upstream.filter(f=>f.sketch&&!f.suppressed).map(f=>f.id):hole ? upstream.filter(f=>f.type==="SKETCH"&&f.sketch&&!f.suppressed&&!["FAILED_SUPPORT","BLOCKED"].includes(f.sketch.support.status??"")).map(f=>f.id) : role === "seam" && seamSection ? seamSection.memberSlot!==undefined?profileSelectionIDs(view,upstream,seamSection.sketchId,seamSection.memberSlot):[seamSection.sketchId] : profileSelectionIDs(view,upstream);
    const regionCandidates=!modifier&&!loft&&!hole?profileRegionCandidates(view,profileUpstream,explicitRegions?draft.profile:undefined):[];
    const regionIDs=draft.profileSelection?.regionIds??[];
    const availableRegionIDs=regionCandidates.find(c=>c.sketchId===draft.profile)?.regions.map(r=>r.id)??[];
    const profileAnalysis=draft.profile?view.sketchAnalyses?.[draft.profile]:undefined;
    const profileAnalysisError=explicitRegions&&draft.profile&&(!profileAnalysis?.geometryVerified||!profileAnalysis.regionIds?.length) ? profileAnalysis?.issues.map(issue=>issue.message).join("；")||"草图尚无可用的闭合区域" : undefined;
    const missingRegions=explicitRegions?regionIDs.filter(id=>!availableRegionIDs.includes(id)):[];
    const sketchIdsToken = JSON.stringify(sketchIds);
    let input: Record<string, unknown> | undefined, parameterError: string | undefined;
    const unresolved = picks.filter(p=>p.resolution.status!=="RESOLVED");
    const sweepReady=!sweep||!!draft.sweep?.path.geometry||!!draft.sweep?.path.entityIds.length&&upstream.some(f=>f.id===draft.sweep?.path.sketchId&&f.sketch&&!f.suppressed);
    const loftBoundaryReady=!loft||(["loftStart","loftEnd"] as const).every(end=>!draft[end]||((draft[end]!.condition!=="DIRECTION"||!!draft[end]!.directionEntityId)&&(!["G1","G2"].includes(draft[end]!.condition)||!!loftRows[end].length&&loftRows[end].every(p=>p.resolution.status==="RESOLVED"))));
    const ready = loftBoundaryReady && sweepReady && (!(hole||modifier)&&draft.operation!=="NEW_BODY"?!!draft.bodyId:true) && (!terminating||!terminationLoadError&&!!(draft.terminationPlaneId||draft.terminationPlane||draft.terminationSurface)&&(!terminationResolution||terminationResolution.status==="RESOLVED")) && (!explicitRegions||regionIDs.length>0&&!missingRegions.length) && (!thinChains||!!draft.thinWall?.entityIds?.length) && validRevision && !loadingInput && !binding && !inputError && (!(modifier||hole) || !feature.id && !insertion || !!inputContext) && !unresolved.length && (!neutralResolution || neutralResolution.status==="RESOLVED") && (hole ? holeReady&&!!draft.bodyId : loft ? (draft.sections?.length ?? 0) >= 2 && !!draft.sections?.some(s=>!s.point) && !draft.sections?.some((s,i)=>s.point&&i!==0&&i!==draft.sections!.length-1) : modifier ? picks.length > 0 && !!draft.bodyId && (feature.type !== "DRAFT" || !!draft.neutralPlaneId || !!draft.neutralPlane) : !!draft.profile && (feature.type !== "REVOLVE" || !!draft.axisEntityId));
    try {
        const value = { ...draft,loftStart:draft.loftStart?{...draft.loftStart}:undefined,loftEnd:draft.loftEnd?{...draft.loftEnd}:undefined,thread:draft.thread?{...draft.thread}:undefined,thinWall:draft.thinWall?{...draft.thinWall}:undefined, selections: modifier ? picks.map(p => p.definition) : draft.selections }, expressions: Record<string, string> = {};
        const read = (slot: SolidQuantitySlot, text: string) => { const parsed = solidParameterEdit(slot, text, original.current[slot], unit); if (parsed.value !== undefined) {
            if(slot==='loft:startMagnitude'||slot==='loft:endMagnitude'){const end=slot==='loft:startMagnitude'?'loftStart':'loftEnd';if(value[end])value[end]!.magnitude=parsed.value;}
            else if((slot==='thread:majorDiameter'||slot==='thread:pitch'||slot==='thread:depth')){if(value.thread)value.thread[slot.slice(7) as 'pitch'|'depth'|'majorDiameter']=parsed.value;}
            else if(slot==='thin:thickness'||slot==='thin:thickness2'){if(value.thinWall)value.thinWall[slot==='thin:thickness'?'thickness':'thickness2']=parsed.value;}
            else value[slot] = parsed.value;
        } if (parsed.expression !== undefined)
            expressions[slot] = parsed.expression; };
        if(hole){
 read("diameter",diameter);if(draft.extent==="BLIND")read("depth",depth);
 if(draft.drillTipAngle&&draft.extent==="BLIND")read("drillTipAngle",tipAngle);else {delete value.drillTipAngle;delete value.depthConvention;}
 if(value.thread){read("thread:majorDiameter",threadDiameter);read("thread:pitch",threadPitch);read("thread:depth",threadDepth);}
 value.holeKind=draft.holeKind??"SIMPLE";
 if(value.holeKind!=="SIMPLE")read("recessDiameter",recessDiameter);else delete value.recessDiameter;
 if(value.holeKind==="COUNTERBORE")read("recessDepth",recessDepth);else delete value.recessDepth;
 if(value.holeKind==="COUNTERSINK")read("countersinkAngle",countersinkAngle);else delete value.countersinkAngle;
}
        else if (!loft&&!sweep) {
            if (angular)
                read("angle", angle);
            else if (draft.extent !== "THROUGH_ALL" && !associated) {
                read("length", length);
                if (draft.extent === "TWO_SIDED")
                    read("length2", second);
            }
        }
        if(value.loftStart)read("loft:startMagnitude",startMagnitude);if(value.loftEnd)read("loft:endMagnitude",endMagnitude);
        if(linear&&draft.extent!=="THROUGH_ALL")read("startOffset",startOffset);else delete value.startOffset;
        if(linear&&associated)read("endOffset",endOffset);else delete value.endOffset;
        if(value.thinWall){read("thin:thickness",thinThickness);if(value.thinWall.mode==="TWO_SIDED")read("thin:thickness2",thinSecond);else delete value.thinWall.thickness2;}
        const common = { ...frozen.current,requestId: intent.current, parameterExpressions: expressions };
        if (ready && (!insertion || !!inputContext))
            input = feature.id ? { ...common, type: "EDIT_FEATURE", targetId: feature.id, expectedFeatureDigest: digest, feature: value } : modifier ? { ...common, type: "CREATE_MODIFY_FEATURE", feature: value,afterFeatureId:insertion?.afterFeatureId,versionId:insertion?.versionId,geometryKey:insertion?inputContext?.artifact.geometryKey:undefined } : loft||hole||sweep ? { ...common, type: "CREATE_SOLID_FEATURE", feature: value,afterFeatureId:insertion?.afterFeatureId,versionId:insertion?.versionId,geometryKey:insertion?inputContext?.artifact.geometryKey:undefined } : { ...common, type: "CREATE_SOLID_FEATURE", afterFeatureId:insertion?.afterFeatureId, versionId:insertion?.versionId, geometryKey:insertion?inputContext?.artifact.geometryKey:undefined, terminationPlaneId:value.terminationPlaneId,terminationPlane:value.terminationPlane,terminationSurface:value.terminationSurface,startOffset:value.startOffset,endOffset:value.endOffset,thinWall:value.thinWall,sketchId: value.profile, profileSelection:value.profileSelection, profileMemberSlot:value.profileMemberSlot, generator: value.type, bodyId: value.bodyId, operation: value.operation, length: value.length ?? 1, length2: value.length2, angle: value.angle ?? 360, extent: value.extent, axisEntityId: value.axisEntityId, reversed: value.reversed };
    }
    catch (cause) {
        parameterError = fieldErrorText(cause);
    }
    const planeGeometryKeys=feature.id||insertion?[...new Set([...planeInputs.map(a=>a.geometryKey),...(inputContext?[inputContext.artifact.geometryKey]:[])])]:view.part!.bodies.filter(b=>b.geometryKey).map(b=>b.geometryKey!);
    const preview = useFeaturePreview(view.document.id, view.document.versionId, input, draft.operation, onPreview);
    const referenceProblem=!loadingInput&&(unresolved.length||missingRegions.length||holeRefs.length&&!holeReady||terminationResolution&&terminationResolution.status!=='RESOLVED'||neutralResolution&&neutralResolution.status!=='RESOLVED')?'输入引用已失效，请重新选择':undefined;
    const feedback=useFeatureDialogFeedback(error??inputError??terminationLoadError??profileAnalysisError??preview.failure??(!validRevision?'模型已改变，请重新打开命令':planeErrors.length?`参考阶段加载失败：${planeErrors.join('、')}`:referenceProblem),solidFeatureNames[feature.type]??'实体特征',parameterError,preview.repairRequired,inputError||terminationLoadError?()=>setInputAttempt(n=>n+1):planeErrors.length?()=>setPlaneLoadAttempt(n=>n+1):preview.failure?preview.retry:undefined);
    const candidateQuantities=preview.parameterCandidates?.filter(p=>feature.id?p.ownerFeatureId===feature.id:!view.part?.parameters?.some(old=>old.parameterId===p.parameterId));
    const bore=candidateQuantities?.find(p=>p.propertySlot==='diameter'),recess=candidateQuantities?.find(p=>p.propertySlot==='recessDiameter'),cone=candidateQuantities?.find(p=>p.propertySlot==='countersinkAngle');
    const coneDepth=bore?.evaluatedValue&&recess?.evaluatedValue&&cone?.evaluatedValue?parameterDisplayValue({...bore,displayUnit:unit,evaluatedValue:{...bore.evaluatedValue,siValue:(recess.evaluatedValue.siValue-bore.evaluatedValue.siValue)/(2*Math.tan(cone.evaluatedValue.siValue/2))}}):'—';
    acceptedSections.current=preview.loftSections;
    useEffect(() => {
        callbacks.current.onSelectionSession(role?{ seedEnabled:!feature.id&&!insertion&&!resume,seedSlot:role,seedMultiple:modifier||loft||!!draft.holePoints||role==='profile-region'||role==='geometry',...occurrenceContext,role,topologyPick:sweep&&role==='geometry'?'EDGE':undefined,accept:sweep&&role==='geometry'?s=>(s.ownerDocumentId??s.documentId)===view.document.id&&(s.occurrencePath??'')===(occurrencePath??'')&&(!s.versionId||s.versionId===version.current)&&(s.kind==='edge'||s.kind==='visual'&&s.visualType==='CURVE'&&upstream.some(f=>f.id===s.featureId&&f.sketch)||upstream.some(f=>f.id===(s.result?.featureId??s.entityId??s.id)&&f.result?.domain==='WIRE_SET')):undefined,project:surface?s=>s.kind==='plane'?null:s:undefined,sketchPoints:hole?upstream.filter(f=>sketchIds.includes(f.id)).flatMap(f=>f.sketch?.entities.filter(e=>e.kind==="POINT"&&!e.sourceEntityId&&!e.suppressed).map(e=>({sketchId:f.id,entityId:e.id}))??[]):undefined,planeGeometryKeys:terminating||feature.type==="DRAFT"?planeGeometryKeys:undefined, profileRegions:explicitRegions?regionCandidates:undefined, connectionLines:showConnections?preview.loftConnections:undefined, documentId: view.document.id, versionId: version.current, occurrencePath, bodyId: modifier ? draft.bodyId : undefined, geometryKey:modifier && (feature.id || insertion) ? inputContext?.artifact.geometryKey : undefined, sketchIds, contextSelections: contextSelections.map(s => ({ ...occurrenceContext, ...s })), selections: selections.map(s => ({ ...occurrenceContext, ...s })), onPick: s => pickRef.current(s) }:undefined);
    }, [role, highlightsToken, sketchIdsToken, draft.bodyId,inputContext?.artifact.geometryKey,showConnections,JSON.stringify(preview.loftConnections),explicitRegions,draft.profile,terminating,surface,JSON.stringify(planeGeometryKeys)]);
    const apply = async () => { if (!input || !preview.previewId || preview.pending || committing)
        {feedback.reportValidation();return;} setCommitting(true); try {
        await onApply({ ...input, previewId: preview.previewId });
        onClose();
    }
    catch (cause) {
        setError(cause);
        setCommitting(false);
    } };
    const name = (id?: string) => upstream.find(f => f.id === id)?.name ?? (id?"引用草图已丢失":"未选择");
    const sketchDraft=():SolidFeatureDraft=>({feature:draft,unit,intentId:intent.current,quantities:{"loft:startMagnitude":startMagnitude,"loft:endMagnitude":endMagnitude,drillTipAngle:tipAngle,"thread:majorDiameter":threadDiameter,"thread:pitch":threadPitch,"thread:depth":threadDepth,length,length2:second,startOffset,endOffset,"thin:thickness":thinThickness,"thin:thickness2":thinSecond,angle,diameter,depth,recessDiameter,recessDepth,countersinkAngle}});
    return <CommandDialog id="solid-feature-editor" open title={`${feature.id ? "编辑" : "创建"} ${solidFeatureNames[feature.type] ?? feature.type}`} size="S" onClose={onClose} onCancelPick={cancelPick} commandStatus={<CommandStatus pending={!!binding||loadingInput||preview.pending}/>} onConfirm={apply} confirmLoading={committing} confirmDisabled={!input || !preview.previewId || preview.pending || committing}>
  <fieldset disabled={committing} onBlurCapture={feedback.onBlurCapture} className="instance-pattern-fields feature-input-fields">
   {!modifier && !loft && !hole && <FeaturePickField mode={explicitRegions?"multiple":"single"} label="轮廓" value={draft.profile ? name(draft.profile) : explicitRegions?"选择闭合区域":"选择草图"} selected={!!draft.profile} selections={highlights.filter(s=>s.kind==='sketch'||s.kind==='profile-region')} items={explicitRegions?regionIDs.map((id,i)=>({key:id,actionLabel:`区域 ${i+1}`,value:`${name(draft.profile!)} / 区域 ${i+1}`,state:availableRegionIDs.includes(id)?'RESOLVED':'MISSING',error:!availableRegionIDs.includes(id),detail:availableRegionIDs.includes(id)?undefined:profileAnalysisError?'上游草图不可用':'区域已失效',active:role==='profile-region'&&regionReplacement===id,selections:highlights.filter(s=>s.kind==='profile-region'&&s.regionId===id),onActivate:()=>{setRegionReplacement(id);setRole('profile-region')},onClear:()=>{edit({profileSelection:{mode:'REGIONS',regionIds:regionIDs.filter(r=>r!==id)}});setRegionReplacement(undefined)}})):undefined} active={role === profileRole&&!regionReplacement} onActivate={() => {setSweepPickingSketch(false);setRole(profileRole)}} onClear={draft.profile ? () => {edit({ profile: undefined,profileMemberSlot:undefined,profileSelection:explicitRegions?{mode:"REGIONS",regionIds:[]}:draft.profileSelection });setRegionReplacement(undefined);} : undefined}/>}
   {!modifier&&!loft&&!hole&&<>
    {onSketch&&<CommandField label="输入草图" width="content"><Button size="small" disabled={!draft.profile||!upstream.some(f=>f.id===draft.profile&&f.sketch)||committing||!!binding} onClick={()=>onSketch(sketchDraft(),draft.profile)}>编辑输入草图</Button>{<Button size="small" disabled={committing||!!binding} onClick={()=>onSketch(sketchDraft())}>创建输入草图</Button>}</CommandField>}
    <CommandField  label={<>轮廓范围</>} width="content"><Segmented value={explicitRegions?"REGIONS":"WHOLE_SKETCH"} options={[{value:"REGIONS",label:"选定区域"},{value:"WHOLE_SKETCH",label:"整张草图"}]} onChange={mode=>{edit({profileSelection:mode==="REGIONS"?{mode:"REGIONS",regionIds:[]}:{mode:"WHOLE_SKETCH"}});setRegionReplacement(undefined);setRole(mode==="REGIONS"?"profile-region":"profile");}}/></CommandField>
    {explicitRegions&&<>
     <Button size="small" disabled={!draft.profile||!availableRegionIDs.length} onClick={()=>{edit({profileSelection:{mode:"REGIONS",regionIds:[...availableRegionIDs]}});setRegionReplacement(undefined);}}>全选区域</Button>
    </>}
   </>}
   {linear&&<CommandField label="薄壁" width="option"><Switch aria-label="启用薄壁" checked={!!draft.thinWall} onChange={enabled=>{edit({thinWall:enabled?{input:"REGIONS",mode:"ONE_SIDED",side:"LEFT",join:"MITER",thickness:1}:undefined,profileSelection:draft.profileSelection??{mode:"REGIONS",regionIds:[...(view.sketchAnalyses?.[draft.profile??""]?.regionIds??[])]}});setRole("profile-region");}}/></CommandField>}
   {draft.thinWall&&<>
    <CommandField label="薄壁来源" width="content"><Segmented value={draft.thinWall.input} options={[{value:"REGIONS",label:"区域边界"},{value:"CHAINS",label:"选择线链"}]} onChange={value=>{edit({thinWall:{...draft.thinWall!,input:value as 'REGIONS'|'CHAINS',entityIds:[],directions:undefined},profileSelection:value==='CHAINS'?undefined:{mode:'REGIONS',regionIds:[...(view.sketchAnalyses?.[draft.profile??'']?.regionIds??[])]}});setRole(value==='CHAINS'?'profile':'profile-region');}}/></CommandField>
    <CommandField label="厚度方式" width="content"><Segmented value={draft.thinWall.mode} options={[{value:"ONE_SIDED",label:"单侧"},{value:"TWO_SIDED",label:"双侧"},{value:"SYMMETRIC",label:"对称总厚度"}]} onChange={value=>editThin({mode:value as NonNullable<Feature['thinWall']>['mode'],thickness2:value==='TWO_SIDED'?draft.thinWall?.thickness2??1:undefined})}/></CommandField>
    <CommandField label={<>薄壁厚度（{unit}）</>} width="expression" actions={<ContextInputSource view={view} binding={boundSlot('thin:thickness')}/>}><ParameterReferenceInput value={thinThickness} disabled={committing||!!boundSlot('thin:thickness')} onChange={setThinThickness} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>
    {draft.thinWall.mode==='TWO_SIDED'&&<CommandField label={<>第二侧厚度（{unit}）</>} width="expression" actions={<ContextInputSource view={view} binding={boundSlot('thin:thickness2')}/>}><ParameterReferenceInput value={thinSecond} disabled={committing||!!boundSlot('thin:thickness2')} onChange={setThinSecond} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
    {draft.thinWall.mode!=='SYMMETRIC'&&<CommandField label="厚度主侧" width="content"><Segmented value={draft.thinWall.side} options={[{value:'LEFT',label:'左侧'},{value:'RIGHT',label:'右侧'}]} onChange={value=>editThin({side:value as 'LEFT'|'RIGHT'})}/></CommandField>}
    <CommandField label="薄壁转角" width="content"><Segmented value={draft.thinWall.join} options={[{value:'MITER',label:'相交'},{value:'ROUND',label:'圆接'}]} onChange={value=>editThin({join:value as 'MITER'|'ROUND'})}/></CommandField>
    <CommandField label="反转线链方向" width="option"><Switch checked={draft.thinWall.reversed} onChange={reversed=>editThin({reversed})}/></CommandField>
    {thinChains&&<FeaturePickField label="薄壁线链" mode="multiple" active={role==='geometry'&&!thinReplacement} onActivate={()=>{setThinReplacement(undefined);setRole('geometry')}} onClear={draft.thinWall.entityIds?.length?()=>editThin({entityIds:[],directions:undefined}):undefined} items={(draft.thinWall.entityIds??[]).map((id,i)=>{
     const entity=upstream.find(f=>f.id===draft.profile&&!f.suppressed)?.sketch?.entities.find(e=>e.id===id&&!e.suppressed);
     return {key:id,actionLabel:`线链曲线 ${i+1}`,value:entity?.name??`曲线 ${i+1}`,error:!entity,state:entity?'RESOLVED':'MISSING',active:role==='geometry'&&thinReplacement===id,selections:highlights.filter(s=>s.kind==='visual'&&s.entityId===id),onActivate:()=>{setThinReplacement(id);setRole('geometry')},onClear:()=>editThin({entityIds:draft.thinWall!.entityIds!.filter(value=>value!==id),directions:draft.thinWall!.directions?.filter(value=>value.entityId!==id)})};
    })}/>}
    {thinChains&&<CommandField label="线链方向" width="content"><Button size="small" onClick={()=>editThin({directions:undefined})}>重设线链方向</Button></CommandField>}
   </>}
   {sweep&&draft.sweep&&<>
    {!draft.sweep.path.geometry&&<FeaturePickField label="路径草图" value={draft.sweep.path.sketchId?name(draft.sweep.path.sketchId):'选择草图'} selected={!!draft.sweep.path.sketchId} active={role==='profile'&&sweepPickingSketch} onActivate={()=>{setSweepPickingSketch(true);setSweepReplacement(undefined);setRole('profile')}} onClear={draft.sweep.path.sketchId?()=>editSweep({path:{sketchId:'',entityIds:[]}}):undefined}/>}
    <FeaturePickField label={draft.sweep.path.geometry?"路径":"路径曲线"} mode={draft.sweep.path.geometry?"single":"multiple"} value={draft.sweep.path.geometry?geometryInputName(view,draft.sweep.path.geometry):`${draft.sweep.path.entityIds.length} 条曲线`} selected={!!draft.sweep.path.geometry||!!draft.sweep.path.entityIds.length} active={role==='geometry'&&!sweepReplacement} onActivate={()=>{setSweepPickingSketch(false);setSweepReplacement(undefined);setRole('geometry')}} onClear={draft.sweep.path.geometry||draft.sweep.path.entityIds.length?()=>editSweep({path:{sketchId:"",entityIds:[]}}):undefined} items={draft.sweep.path.geometry?undefined:draft.sweep.path.entityIds.map((id,i)=>{
     const entity=upstream.find(f=>f.id===draft.sweep!.path.sketchId&&!f.suppressed)?.sketch?.entities.find(e=>e.id===id&&!e.suppressed);
     return {key:id,actionLabel:`路径曲线 ${i+1}`,value:entity?.name??`曲线 ${i+1}`,state:entity?'RESOLVED':'MISSING',error:!entity,active:role==='geometry'&&sweepReplacement===id,selections:highlights.filter(s=>s.kind==='visual'&&s.entityId===id),onActivate:()=>{setSweepReplacement(id);setRole('geometry')},onClear:()=>editSweep({path:{...draft.sweep!.path,entityIds:draft.sweep!.path.entityIds.filter(value=>value!==id),direction:draft.sweep!.path.direction?.entityId===id?undefined:draft.sweep!.path.direction}})};
    })}/>
    <CommandField label="路径方向" width="content"><Switch aria-label="反转扫掠路径" checked={draft.sweep.path.reversed} onChange={reversed=>editSweep({path:{...draft.sweep!.path,reversed}})}/>{!draft.sweep.path.geometry&&<Button size="small" onClick={()=>editSweep({path:{...draft.sweep!.path,direction:undefined}})}>重设路径起点</Button>}</CommandField>
    <CommandField label="截面姿态" width="content"><Segmented value={draft.sweep.frameMode} options={[{value:'CORRECTED_FRENET',label:'修正标架'},{value:'FRENET',label:'曲率标架'}]} onChange={value=>editSweep({frameMode:value as NonNullable<Feature['sweep']>['frameMode']})}/></CommandField>
    {!draft.sweep.path.geometry&&<CommandField label="路径转角" width="content"><Segmented value={draft.sweep.cornerMode} options={[{value:'MITER',label:'直线斜接'},{value:'G1_ONLY',label:'仅光滑连接'}]} onChange={value=>editSweep({cornerMode:value as NonNullable<Feature['sweep']>['cornerMode']})}/></CommandField>}
    {onSketch&&!draft.sweep.path.geometry&&<CommandField label="路径输入" width="content"><Button size="small" disabled={!draft.sweep.path.sketchId||committing} onClick={()=>onSketch({...sketchDraft(),inputRole:'SWEEP_PATH'},draft.sweep!.path.sketchId)}>编辑路径草图</Button><Button size="small" disabled={committing} onClick={()=>onSketch({...sketchDraft(),inputRole:'SWEEP_PATH'})}>创建路径草图</Button></CommandField>}
   </>}
   {insertion&&<CommandField label="插入位置" width="expression"><span>{insertion.label} 之后</span></CommandField>}
   {hole&&<>
    <CommandField label="多点孔组" width="option"><Switch aria-label="多点孔组" checked={!!draft.holePoints} disabled={committing||!!draft.holePoints&&draft.holePoints.length>1} onChange={group=>{edit(group?{holePoints:draft.holePoint?[draft.holePoint]:[],holePoint:undefined}:{holePoint:draft.holePoints?.[0],holePoints:undefined});setPointResolution(undefined);setRole('sketch-point')}}/></CommandField>
    <FeaturePickField label="定位点" mode={draft.holePoints?'multiple':'single'} value={draft.holePoints?`${holeRefs.length} 个显式点`:draft.holePoint?holeReady?`${name(draft.holePoint.sketchId)} · 已选择显式点`:pointResolution?inputStatusText(pointResolution):'原定位点已失效':'选择上游普通草图的显式点'} selected={!!holeRefs.length} selections={highlights.filter(s=>s.kind==='visual'&&s.visualType==='POINT')} active={role==='sketch-point'&&!holeReplacement} onActivate={()=>{setHoleReplacement(undefined);setRole('sketch-point')}} onClear={holeRefs.length?()=>{pointReplaced.current=true;edit(draft.holePoints?{holePoints:[]}:{holePoint:undefined});setPointResolution(undefined)}:undefined}
      items={draft.holePoints?.map((p,i)=>({key:holeKey(p),actionLabel:`孔位 ${i+1}`,value:`${name(p.sketchId)} / 孔位 ${i+1}`,state:validHoleRef(p)?'RESOLVED':'MISSING',error:!validHoleRef(p),active:role==='sketch-point'&&holeReplacement===holeKey(p),onActivate:()=>{setHoleReplacement(holeKey(p));setRole('sketch-point')},onClear:()=>{pointReplaced.current=true;edit({holePoints:draft.holePoints!.filter(q=>holeKey(q)!==holeKey(p))});setPointResolution(undefined)}}))}/>
    <FeaturePickField label="目标实体" value={view.part?.bodies.find(b=>b.id===draft.bodyId)?.name??"选择实体"} disabled={!!feature.id} selected={!!draft.bodyId} active={role==="body"} onActivate={()=>setRole("body")} onClear={!feature.id&&draft.bodyId?()=>edit({bodyId:undefined}):undefined}/>
    <CommandField label="孔型" width="content"><Segmented value={draft.holeKind??'SIMPLE'} options={[{value:'SIMPLE',label:'普通孔'},{value:'COUNTERBORE',label:'圆柱沉孔'},{value:'COUNTERSINK',label:'锥沉孔'}]} onChange={kind=>edit({holeKind:kind as Feature['holeKind']})}/></CommandField>
    <CommandField actions={<ContextInputSource view={view} binding={boundSlot("diameter")}/>}  label={<>直径（{unit}）</>} width="expression"><ParameterReferenceInput value={diameter} disabled={committing||!!boundSlot("diameter")} onChange={setDiameter} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>
    <CommandField  label={<>终止</>} width="content"><Segmented value={draft.extent} options={[{value:"BLIND",label:"平底盲孔"},{value:"THROUGH_ALL",label:"单向贯穿"},{value:"TO_PLANE",label:"到平面"}]} onChange={extent=>{edit({extent:extent as Feature["extent"],terminationPlane:undefined,terminationPlaneId:undefined});setTerminationResolution(undefined);setTerminationLoadError(undefined);setTerminationVisual(undefined);setRole(extent==="TO_PLANE"?"plane":undefined);}}/></CommandField>
    {draft.extent==="BLIND"&&<CommandField actions={<ContextInputSource view={view} binding={boundSlot("depth")}/>}  label={<>深度（{unit}）</>} width="expression"><ParameterReferenceInput value={depth} disabled={committing||!!boundSlot("depth")} onChange={setDepth} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
    {draft.extent==='BLIND'&&<>
     <CommandField label="钻尖" width="option"><Switch aria-label="锥形钻尖" checked={!!draft.drillTipAngle} onChange={enabled=>edit({drillTipAngle:enabled?118:undefined,depthConvention:enabled?'CYLINDER':undefined})}/></CommandField>
     {!!draft.drillTipAngle&&<><CommandField label="钻尖夹角（°）" width="expression"><ParameterReferenceInput value={tipAngle} onChange={setTipAngle} disabled={!!boundSlot('drillTipAngle')} parameters={view.part?.parameters??[]} onConfirm={()=>void apply()}/></CommandField><CommandField label="盲孔深度定义" width="content"><Segmented value={draft.depthConvention} options={[{value:'CYLINDER',label:'完整圆柱深度'},{value:'TIP',label:'到钻尖总深度'}]} onChange={value=>edit({depthConvention:value as 'CYLINDER'|'TIP'})}/></CommandField></>}
    </>}
    <CommandField label="螺纹工程标注" width="option"><Switch aria-label="螺纹工程标注" checked={!!draft.thread} onChange={enabled=>edit({thread:enabled?{mode:'ANNOTATION',designation:'M6x1',hand:'RIGHT',majorDiameter:6,pitch:1,depth:5}:undefined})}/></CommandField>
    {draft.thread&&<>
     <CommandField label="螺纹规格标记" width="expression"><Input aria-label="螺纹规格标记" value={draft.thread.designation} maxLength={128} onChange={e=>edit({thread:{...draft.thread!,designation:e.target.value}})}/></CommandField>
     <CommandField label="螺纹旋向" width="content"><Segmented value={draft.thread.hand} options={[{value:'RIGHT',label:'右旋'},{value:'LEFT',label:'左旋'}]} onChange={value=>edit({thread:{...draft.thread!,hand:value as 'RIGHT'|'LEFT'}})}/></CommandField>
     {([['thread:majorDiameter','公称螺纹直径',threadDiameter,setThreadDiameter],['thread:pitch','螺距',threadPitch,setThreadPitch],['thread:depth','螺纹标注深度',threadDepth,setThreadDepth]] as const).map(([slot,label,value,set])=><CommandField key={slot} label={`${label}（${unit}）`} width="expression"><ParameterReferenceInput value={value} onChange={set} parameters={view.part?.parameters??[]} unit={unit} disabled={!!boundSlot(slot)} onConfirm={()=>void apply()}/></CommandField>)}
    </>}
    {draft.holeKind&&draft.holeKind!=='SIMPLE'&&<CommandField actions={<ContextInputSource view={view} binding={boundSlot("recessDiameter")}/>} label={<>沉孔直径（{unit}）</>} width="expression"><ParameterReferenceInput value={recessDiameter} disabled={committing||!!boundSlot("recessDiameter")} onChange={setRecessDiameter} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
    {draft.holeKind==='COUNTERBORE'&&<CommandField actions={<ContextInputSource view={view} binding={boundSlot("recessDepth")}/>} label={<>沉孔深度（{unit}）</>} width="expression"><ParameterReferenceInput value={recessDepth} disabled={committing||!!boundSlot("recessDepth")} onChange={setRecessDepth} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
    {draft.holeKind==='COUNTERSINK'&&<><CommandField actions={<ContextInputSource view={view} binding={boundSlot("countersinkAngle")}/>} label="锥面夹角（°）" width="expression"><ParameterReferenceInput value={countersinkAngle} disabled={committing||!!boundSlot("countersinkAngle")} onChange={setCountersinkAngle} parameters={view.part?.parameters??[]} unit={undefined} onConfirm={()=>void apply()}/></CommandField><CommandField label="锥段深度" width="content"><span>{coneDepth}</span></CommandField></>}
   </>}
   {draft.profile && upstream.find(f=>f.id===draft.profile)?.type==="SKETCH_PATTERN" && <CommandField  label={<>草图成员槽位</>} width="expression"><Input type="number" min={0} step={1} value={draft.profileMemberSlot} onChange={e=>edit({profileMemberSlot:e.target.value===""?undefined:Number(e.target.value)})}/></CommandField>}
   {feature.type === "REVOLVE" && <FeaturePickField label="旋转轴" value={draft.axisEntityId ? "已选择 1 条轴线" : "选择直线或轴"} selected={!!draft.axisEntityId} selections={axisVisual?[axisVisual]:restoredAxis?[restoredAxis]:[]} active={role === "axis"} onActivate={() => setRole("axis")} onClear={draft.axisEntityId ? () => { edit({ axisEntityId: undefined }); setAxisVisual(undefined); } : undefined}/>}
   {modifier&&<FeaturePickField mode="multiple" label={pickRole==='edge'?'边集':'面集'} detail={loadingInput?'正在恢复选择…':undefined} active={role===pickRole&&!replacement} disabled={loadingInput||!!inputError||!!binding} onActivate={()=>{setReplacement(undefined);setRole(pickRole)}} onClear={picks.length&&!binding?()=>{setPicks([]);setReplacement(undefined)}:undefined} items={picks.map((p,i)=>({key:p.id,actionLabel:`${pickRole==='edge'?'边':'面'} ${i+1}`,value:`${view.part?.features.find(f=>f.id===p.definition.sourceFeatureId)?.name??'上游几何'} / ${pickRole==='edge'?'边':'面'} ${i+1}`,state:p.resolution.status,error:p.resolution.status!=='RESOLVED',detail:p.resolution.status==='RESOLVED'?undefined:inputStatusText(p.resolution),active:role===pickRole&&replacement===p.id,selections:p.visual?[p.visual]:[],onActivate:()=>{setReplacement(p.id);setRole(pickRole)},onClear:()=>{setPicks(previous=>previous.filter(item=>item.id!==p.id));setReplacement(undefined)}}))}/>}
   {terminating&&<>
    <FeaturePickField label={surface?"终止曲面":"终止平面"} value={terminationLoadError?"原目标解析受阻，请重新选择":terminationResolution&&terminationResolution.status!=="RESOLVED"?inputStatusText(terminationResolution):draft.terminationSurface?"已选择上游实体曲面":draft.terminationPlane?"已选择上游实体平面":view.part?.datumPlanes.find(p=>p.id===draft.terminationPlaneId)?.name??"选择基准平面或上游实体平面"} selected={!!(draft.terminationPlane||draft.terminationPlaneId||draft.terminationSurface)} selections={terminationVisual?[terminationVisual]:[]} active={role==="plane"} disabled={loadingInput||!!binding} onActivate={()=>setRole("plane")} onClear={draft.terminationPlane||draft.terminationPlaneId||draft.terminationSurface?()=>{edit({terminationPlane:undefined,terminationPlaneId:undefined,terminationSurface:undefined});setTerminationResolution(undefined);setTerminationLoadError(undefined);setTerminationVisual(undefined);}:undefined}/>
   </>}
   {feature.type === "DRAFT" && <>
    <FeaturePickField label="中性平面" value={neutralResolution && neutralResolution.status!=="RESOLVED" ? inputStatusText(neutralResolution) : draft.neutralPlane ? "已选择实体平面" : view.part?.datumPlanes.find(p => p.id === draft.neutralPlaneId)?.name ?? "选择基准面或实体平面"} selected={!!(draft.neutralPlane||draft.neutralPlaneId)} selections={neutralVisual?[neutralVisual]:[]} active={role === "plane"} disabled={loadingInput || !!inputError || !!binding} onActivate={() => {setReplacement(undefined);setRole("plane");}} onClear={(draft.neutralPlane || draft.neutralPlaneId) && !binding ? ()=>{edit({neutralPlane:undefined,neutralPlaneId:undefined});setNeutralVisual(undefined);setNeutralResolution(undefined);} : undefined}/>
   </>}
   {loft && <>
    <FeaturePickField label="点端" value="选择草图点或基准原点" active={role === "point"&&sectionReplacement===undefined} onActivate={()=>{setSectionReplacement(undefined);setRole("point");}}/>
    <CommandField  label={<>显示连接线</>} width="option"><Switch checked={showConnections} onChange={setShowConnections}/></CommandField>
    <FeaturePickField mode="ordered" label="截面" active={role==='profile'&&sectionReplacement===undefined} onActivate={()=>{setSectionReplacement(undefined);setRole('profile')}} onClear={draft.sections?.length?()=>edit({sections:[]}):undefined} items={(preview.loftSections??draft.sections??[]).map((section,i)=>({
     key:`${section.sketchId}/${section.memberSlot??''}/${JSON.stringify(section.point??'')}/${i}`,actionLabel:`截面 ${i+1}`,value:section.point?`点端 · ${section.point.axisEntityId?view.part?.axisSystems.find(a=>a.id===section.point?.axisEntityId?.split(':')[1])?.name??'引用原点已丢失':name(section.sketchId)}`:name(section.sketchId),selections:highlights[i]?[highlights[i]]:[],active:sectionReplacement===i&&(role==='profile'||role==='point'),onActivate:()=>{setSectionReplacement(i);setRole(section.point?'point':'profile')},onClear:()=>edit({sections:draft.sections?.filter((_,j)=>j!==i)}),
     actions:<><Button size="small" aria-label={`上移截面 ${i+1}`} disabled={!i} onClick={()=>{const sections=[...draft.sections!];[sections[i-1],sections[i]]=[sections[i],sections[i-1]];edit({sections});}}>↑</Button><Button size="small" aria-label={`下移截面 ${i+1}`} disabled={i===(draft.sections?.length??0)-1} onClick={()=>{const sections=[...draft.sections!];[sections[i+1],sections[i]]=[sections[i],sections[i+1]];edit({sections});}}>↓</Button></>,
     settings:section.point?undefined:<>
     {upstream.find(f=>f.id===section.sketchId)?.type==="SKETCH_PATTERN"&&<CommandField  label={<>成员槽位</>} width="expression"><Input type="number" min={0} step={1} value={section.memberSlot} onChange={e=>edit({sections:(acceptedSections.current??draft.sections)?.map((s,j)=>i===j?{...s,memberSlot:e.target.value===""?undefined:Number(e.target.value)}:s)})}/></CommandField>}
     {!section.point && <><Button size="small" onClick={()=>edit({sections:(acceptedSections.current??draft.sections)?.map((s,j)=>i===j?{...s,correspondenceResolved:false,seamEntityId:undefined,seamAngle:0,reversed:false}:s)})}>自动对应</Button>
     <CommandField  label={<>反向</>} width="option"><Switch checked={section.reversed} onChange={reversed => edit({ sections: (acceptedSections.current??draft.sections)?.map((s, j) => i === j ? { ...s, correspondenceResolved:true, reversed } : s) })}/></CommandField>
     <Button size="small" aria-pressed={role === "seam" && seamIndex === i} onClick={() => { setSeamIndex(i); setRole("seam"); }}>选择闭合起始边</Button>
     {upstream.find(f => f.id === section.sketchId)?.sketch?.entities.some(e => e.kind === "CIRCLE") && <CommandField  label={<>闭合点角度（deg）</>} width="expression"><Input type="number" value={section.seamAngle ?? 0} onChange={e => edit({ sections: (acceptedSections.current??draft.sections)?.map((s, j) => i === j ? { ...s, correspondenceResolved:true, seamAngle: Number(e.target.value) } : s) })}/></CommandField>}
     </>}
     </>
    }))}/>
    {(['loftStart','loftEnd'] as const).map(end=>{const b=draft[end],label=end==='loftStart'?'起端':'末端',rows=loftRows[end],slot=end==='loftStart'?'loft:startMagnitude':'loft:endMagnitude';return <div key={end}>
     <CommandField label={`${label}条件`} width="expression"><Select aria-label={`${label}条件`} value={b?.condition??'FREE'} options={[{value:'FREE',label:'自由'},{value:'NORMAL',label:'垂直截面'},{value:'DIRECTION',label:'指定端部方向'},{value:'G1',label:'匹配切向 G1'},{value:'G2',label:'匹配曲率 G2'}]} onChange={condition=>{edit({ruled:false,[end]:condition==='FREE'?undefined:{condition,magnitude:b?.magnitude??1,supports:[]}});setLoftRows(r=>({...r,[end]:[]}));setLoftEnd(end);setLoftReplacement(undefined);setRole(['G1','G2'].includes(condition)?'face':condition==='DIRECTION'?'axis':undefined)}}/></CommandField>
     {b&&<>
      <CommandField label={`${label}影响系数`} width="expression"><ParameterReferenceInput value={end==='loftStart'?startMagnitude:endMagnitude} onChange={end==='loftStart'?setStartMagnitude:setEndMagnitude} parameters={view.part?.parameters??[]} disabled={!!boundSlot(slot)} onConfirm={()=>void apply()}/></CommandField>
      {b.condition!=='NORMAL'&&<FeaturePickField label={`${label}方向`} value={b.directionEntityId??'使用截面法向'} selected={!!b.directionEntityId} active={role==='axis'&&loftEnd===end} onActivate={()=>{setLoftEnd(end);setRole('axis')}} onClear={b.directionEntityId?()=>editBoundary(end,{directionEntityId:undefined}):undefined}/>}
      <CommandField label={`${label}方向反转`} width="option"><Switch checked={b.reversed} onChange={reversed=>editBoundary(end,{reversed})}/></CommandField>
      {['G1','G2'].includes(b.condition)&&<FeaturePickField mode="multiple" label={`${label}连续性支撑`} active={role==='face'&&loftEnd===end&&!loftReplacement} onActivate={()=>{setLoftEnd(end);setLoftReplacement(undefined);setRole('face')}} onClear={()=>{setLoftRows(r=>({...r,[end]:[]}));editBoundary(end,{supports:[]})}} items={rows.map((p,i)=>({key:p.id,actionLabel:`${label}支撑 ${i+1}`,value:`支撑面 ${i+1}`,state:p.resolution.status,error:p.resolution.status!=='RESOLVED',detail:inputStatusText(p.resolution),selections:p.visual?[p.visual]:[],active:role==='face'&&loftEnd===end&&loftReplacement===p.id,onActivate:()=>{setLoftEnd(end);setLoftReplacement(p.id);setRole('face')},onClear:()=>{const next=rows.filter(r=>r.id!==p.id);setLoftRows(r=>({...r,[end]:next}));editBoundary(end,{supports:next.map(p=>p.definition)})}}))}/>}
     </>}
    </div>})}
    <CommandField  label={<>截面连接</>} width="content"><Segmented value={draft.ruled ? "ruled" : "smooth"} options={[{ value: "smooth", label: "平滑" }, { value: "ruled", label: "直纹" }]} onChange={v => edit({ ruled: v === "ruled",...(v==='ruled'?{loftStart:undefined,loftEnd:undefined}:{}) })}/></CommandField>
   </>}
   {!modifier && !hole && <>
    <CommandField  label={<>材料</>} width="content"><Segmented value={draft.operation} options={[...(!feature.id&&!insertion ? [{ value: "NEW_BODY", label: "新实体" }] : []), { value: "ADD", label: "添加" }, { value: "REMOVE", label: "切除" }, { value: "INTERSECT", label: "交集" }]} onChange={operation => edit({ operation: operation as Feature["operation"], extent: operation !== "REMOVE" && draft.extent === "THROUGH_ALL" || operation==="INTERSECT"&&associated||operation==="NEW_BODY"&&draft.extent==="TO_NEXT" ? "FINITE" : draft.extent, ...(operation==="INTERSECT"?{terminationPlane:undefined,terminationPlaneId:undefined,terminationSurface:undefined,endOffset:0}:{}) })}/></CommandField>
    {draft.operation !== "NEW_BODY" && <FeaturePickField label="目标实体" value={view.part?.bodies.find(b => b.id === draft.bodyId)?.name ?? "选择实体"} disabled={!!feature.id||!!insertion} selected={!!draft.bodyId} active={role === "body"} onActivate={() => setRole("body")} onClear={!feature.id&&!insertion&&draft.bodyId?()=>edit({bodyId:undefined}):undefined}/>}
   </>}
   {linear&&draft.extent!=="THROUGH_ALL"&&<CommandField label={<>起始偏移（{unit}）</>} width="expression" actions={<ContextInputSource view={view} binding={boundSlot("startOffset")}/>}><ParameterReferenceInput value={startOffset} disabled={committing||!!boundSlot("startOffset")} onChange={setStartOffset} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
   {linear&&associated&&<><CommandField label={<>终止偏移（{unit}）</>} width="expression" actions={<ContextInputSource view={view} binding={boundSlot("endOffset")}/>}><ParameterReferenceInput value={endOffset} disabled={committing||!!boundSlot("endOffset")} onChange={setEndOffset} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField></>}
   {!loft && !hole && !sweep && (angular ? <CommandField actions={<ContextInputSource view={view} binding={boundSlot("angle")}/>}  label={<>角度（deg）</>} width="expression"><ParameterReferenceInput value={angle} disabled={committing||!!boundSlot("angle")} onChange={setAngle} parameters={view.part?.parameters??[]} unit={undefined} onConfirm={()=>void apply()}/></CommandField> : <>
    {!modifier && <CommandField  label={<>范围</>} width="content"><Segmented value={draft.extent ?? "FINITE"} options={[{ value: "FINITE", label: "长度" }, { value: "TWO_SIDED", label: "双侧" }, { value: "SYMMETRIC", label: "对称" }, ...(["PAD","LINEAR_EXTRUDE"].includes(feature.type)&&["ADD","REMOVE","NEW_BODY"].includes(draft.operation??"")?[{value:"TO_PLANE",label:"到平面"},{value:"TO_SURFACE",label:"到曲面"},...(draft.operation!=="NEW_BODY"?[{value:"TO_NEXT",label:"到下一面"}]:[])]:[]), ...(draft.operation === "REMOVE" ? [{ value: "THROUGH_ALL", label: "贯穿" }] : [])]} onChange={extent => {edit({ extent: extent as Feature["extent"],terminationPlane:undefined,terminationPlaneId:undefined,terminationSurface:undefined,startOffset:extent==="THROUGH_ALL"?0:draft.startOffset,endOffset:0 });setTerminationResolution(undefined);setTerminationLoadError(undefined);setTerminationVisual(undefined);if(extent==="TO_PLANE"||extent==="TO_SURFACE")setRole("plane");else setRole(undefined);}}/></CommandField>}
    {draft.extent !== "THROUGH_ALL" && !associated && <CommandField actions={<ContextInputSource view={view} binding={boundSlot("length")}/>}  label={<>{feature.type === "FILLET" ? "半径" : feature.type === "CHAMFER" ? "距离" : feature.type === "SHELL" ? "厚度" : "长度"}（{unit}）</>} width="expression"><ParameterReferenceInput value={length} disabled={committing||!!boundSlot("length")} onChange={setLength} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
    {draft.extent === "TWO_SIDED" && <CommandField actions={<ContextInputSource view={view} binding={boundSlot("length2")}/>}  label={<>反向长度（{unit}）</>} width="expression"><ParameterReferenceInput value={second} disabled={committing||!!boundSlot("length2")} onChange={setSecond} parameters={view.part?.parameters??[]} unit={unit} onConfirm={()=>void apply()}/></CommandField>}
   </>)}
   {!loft && !sweep && <CommandField  label={<>反向</>} width="option"><Switch checked={draft.reversed} disabled={committing} onChange={reversed => edit({ reversed })}/></CommandField>}
   {feature.id && <CommandField  label={<>抑制</>} width="option"><Switch checked={draft.suppressed} disabled={committing} onChange={suppressed => edit({ suppressed })}/></CommandField>}
  </fieldset>
 </CommandDialog>;
}
