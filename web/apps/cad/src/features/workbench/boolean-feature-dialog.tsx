import {CommandField,CommandStatus} from "../../cad/overlay/command-form";
import {recordDeveloperEvidence} from "../../cad/command/developer-evidence";
import {engineeringProblem} from "../../cad/command/operation-feedback";

import { Alert, Button, Segmented, Switch, Tooltip } from "antd";
import {booleanRoleColors} from "../../cad/rendering/boolean-region-style";
import {EyeOutlined} from "@ant-design/icons";
import {selectBooleanRegion,matchBooleanRegion,type BooleanRegionRef,type BooleanRegionsAnalysis,type BooleanRegionCell} from "./boolean-region-model";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { api } from "../../api/client";
import type { Artifact, DocumentView, Feature, SelectionItem } from "../../types";
import type { FeatureSelectionSession } from "../../cad/interaction/feature-selection";
import { booleanInputStages, booleanDefinitionReady } from "./solid-feature-model";
import { bodyStage } from "./feature-picking";
import { FeaturePickField } from "./feature-pick-field";
import { useFeaturePreview } from "./use-feature-preview";
import { randomUUID } from "../../utils/random-uuid";
function roleName(name:string,role:keyof typeof booleanRoleColors){return <span className="boolean-role-name" title={name}><i aria-hidden style={{background:booleanRoleColors[role]}}/><span>{name}</span></span>}
export function BooleanFeatureDialog({ view, initial, digest, bodyId, seed, occurrencePath, occurrenceContext, onClose, onApply, onPreview, onSelectionSession, onInputArtifacts }: {
    view: DocumentView;
    initial?: Feature;
    digest?: string;
    bodyId?: string;
    seed: SelectionItem[];
    occurrencePath?: string;
    occurrenceContext?: Pick<SelectionItem, "instancePath" | "contextVariantKey" | "rootDocumentId">;
    onClose: () => void;
    onApply: (input: Record<string, unknown>) => Promise<unknown>;
    onPreview: (artifact?: Artifact, operation?: Feature["operation"]) => void;
    onSelectionSession: (session?: FeatureSelectionSession) => void;
    onInputArtifacts: (artifacts?: Artifact[]) => void;
}) {
    const stages = booleanInputStages(view.part!.features, initial?.id);
    const [target, setTarget] = useState(initial?.bodyId ?? bodyId), [operation, setOperation] = useState<Feature["operation"]>(initial?.operation ?? "REMOVE");
    const [tools, setTools] = useState(initial?.tools ?? []), [keep, setKeep] = useState(initial?.keepTools ?? false), [suppressed, setSuppressed] = useState(initial?.suppressed ?? false);
    const [role, setRole] = useState<"target" | "tools" | "regions" | undefined>(initial?.booleanResult?.mode==='EXPLICIT'?'regions':'tools'), [error, setError] = useState<string>(), [committing, setCommitting] = useState(false);
    const [replacing,setReplacing]=useState<string>();
    const [resultMode,setResultMode]=useState<'ALL'|'EXPLICIT'>(initial?.booleanResult?.mode??'ALL');
    const [retained,setRetained]=useState<BooleanRegionRef[]>(initial?.booleanResult?.regions??[]);
    const [analysisState,setAnalysisState]=useState<{key:string;value:BooleanRegionsAnalysis}>();
    const [analysisPending,setAnalysisPending]=useState(false),[analysisError,setAnalysisError]=useState<string>(),[analysisRetry,setAnalysisRetry]=useState(0);
    const [replacingRegion,setReplacingRegion]=useState<number>(),[focusId,setFocusId]=useState<string>(),[isolateId,setIsolateId]=useState<string>(),[focusRequest,setFocusRequest]=useState(0);
    const initializeAll=useRef(false),originalInputs=useRef<Artifact[]|undefined>(undefined),modeRef=useRef(resultMode),operationRef=useRef(operation);modeRef.current=resultMode;operationRef.current=operation;
    const analysisKey=JSON.stringify([view.document.id,view.document.versionId,initial?.id,target,tools]);
    const analysis=analysisState?.key===analysisKey?{...analysisState.value,regions:analysisState.value.regions.map(cell=>({...cell,eligible:cell.operations.includes(operation!)}))}:undefined;
    const resolved=retained.map(ref=>matchBooleanRegion(ref,analysis?.regions??[]));
    const failedRegions=analysis?resolved.flatMap((r,index)=>r.status!=='RESOLVED'?[index]:[]):[];
    const toggleRegion=(cell:BooleanRegionCell)=>{
        if(!cell.eligible)return;
        setRetained(previous=>selectBooleanRegion(previous,analysis?.regions??[],cell,replacingRegion));
        setFocusId(cell.reference.cellDigest);setReplacingRegion(undefined);
    };
    useEffect(()=>{
        if(resultMode!=='EXPLICIT'||tools.length!==1||!target||!booleanDefinitionReady(target,operation,tools,stages)){setAnalysisPending(false);setAnalysisError(undefined);return;}
        if(analysisState?.key===analysisKey){setAnalysisPending(false);setAnalysisError(undefined);callbacks.current.onInputArtifacts(analysisState.value.inputs);return;}
        let alive=true;const controller=new AbortController();setAnalysisPending(true);setAnalysisError(undefined);
        setFocusId(undefined);setIsolateId(undefined);
        void api.analyzeBooleanRegions(view.document.id,{versionId:view.document.versionId,featureId:initial?.id,bodyId:target,tool:tools[0],operation:operation!},controller.signal).then(value=>{
            if(!alive||value.versionId!==view.document.versionId)return;
            recordDeveloperEvidence("布尔结果区域",{inputs:{versionId:view.document.versionId,target,tools,operation},analysis:value});
            setAnalysisState({key:analysisKey,value});
            callbacks.current.onInputArtifacts(value.inputs);
            if(initializeAll.current){initializeAll.current=false;setRetained(value.regions.filter(c=>c.operations.includes(operationRef.current!)).map(c=>c.reference));}
        }).catch(cause=>{if(alive)setAnalysisError(engineeringProblem(cause))}).finally(()=>{if(alive)setAnalysisPending(false)});
        return()=>{alive=false;controller.abort()};
    },[resultMode,analysisKey,analysisRetry]);
    const cancelPick=()=>{if(!role)return false;setRole(undefined);setReplacing(undefined);setReplacingRegion(undefined);setFocusId(undefined);setIsolateId(undefined);return true};
    const [loading, setLoading] = useState(!!initial);
    const version = useRef(view.document.versionId), intent = useRef(randomUUID());
    const callbacks = useRef({ onSelectionSession, onInputArtifacts });
    callbacks.current = { onSelectionSession, onInputArtifacts };
    useEffect(() => () => { callbacks.current.onSelectionSession(); callbacks.current.onInputArtifacts(); }, []);
    useEffect(() => {
        if (!initial)
            return;
        const controller = new AbortController();
        let alive = true;
        void api.getFeatureInput(view.document.id, { versionId: version.current, featureId: initial.id }, controller.signal).then(context => {
            if(alive){originalInputs.current=context.artifacts??[context.artifact];if(modeRef.current==='ALL')callbacks.current.onInputArtifacts(originalInputs.current);}
        }).catch(cause => { if (alive)
            setError(engineeringProblem(cause)); }).finally(() => { if (alive)
            setLoading(false); });
        return () => { alive = false; controller.abort(); };
    }, []);
    const pick = (s: SelectionItem) => {
        if (!role || committing || version.current !== view.document.versionId || (s.ownerDocumentId ?? s.documentId) !== view.document.id || (s.occurrencePath ?? "") !== (occurrencePath ?? ""))
            return;
        if(role==='regions'){
            if(s.kind==='boolean-region'&&analysis){const cell=analysis.regions.find(c=>c.reference.cellDigest===s.regionId&&c.reference.inputDigest===s.analysisId);if(cell)toggleRegion(cell);}
            return;
        }
        const stage = bodyStage(s, stages);
        if (!stage)
            return;
        setError(undefined);
        if (role === "target") {
            setTarget(stage.bodyId);
            setTools(previous => previous.filter(t => t.bodyId !== stage.bodyId));
            setRole(undefined);
        }
        else if (stage.bodyId !== target) {
            if(replacing){if(tools.some(t=>t.bodyId===stage.bodyId&&t.bodyId!==replacing)){setError("该实体已在工具列表中");return;}setTools(previous=>previous.map(t=>t.bodyId===replacing?stage:t));setReplacing(undefined);setRole(undefined);return;}
            setTools(previous => previous.some(t => t.bodyId === stage.bodyId) ? previous.filter(t => t.bodyId !== stage.bodyId) : operation === "INTERSECT" ? [stage] : [...previous, stage]);
        }
    };
    const pickRef = useRef(pick);
    pickRef.current = pick;
    const seedOnce = useRef(false);
    useEffect(() => {
        if (seedOnce.current || initial)
            return;
        seedOnce.current = true;
        const picked = seed.filter(s => (s.ownerDocumentId ?? s.documentId) === view.document.id && (s.occurrencePath ?? "") === (occurrencePath ?? "")).map(s => bodyStage(s, stages)).filter((s): s is NonNullable<typeof s> => !!s);
        if (!picked.length)
            return;
        const selectedTarget = picked[0].bodyId;
        setTarget(selectedTarget);
        setTools([...new Map(picked.slice(1).filter(t => t.bodyId !== selectedTarget).map(t => [t.bodyId, t])).values()]);
    }, []);
    const selectedBodies = [target, ...tools.map(t => t.bodyId)].filter((id): id is string => !!id);
    const token=JSON.stringify([selectedBodies,role,replacing,analysis,retained,focusId,isolateId,focusRequest]);
    useEffect(()=>{
        const regions=resultMode==='EXPLICIT'&&analysis&&target&&tools[0]?{
            inputs:analysis.inputs,cells:analysis.regions.map(cell=>({...cell,retained:resolved.some(r=>r.cell?.reference.cellDigest===cell.reference.cellDigest)})),focusId,isolateId,focusRequest,targetBodyId:target,toolBodyId:tools[0].bodyId,
        }:undefined;
        if(!role&&!regions){callbacks.current.onSelectionSession();return;}
        callbacks.current.onSelectionSession({role:role==='regions'?'boolean-region':'body',...(role==='regions'?occurrenceContext:undefined),documentId:view.document.id,versionId:view.document.versionId,occurrencePath,
            booleanRegions:regions,
            accept:!role?()=>false:undefined,
            selections:role==='regions'?(focusId&&analysis?[{...occurrenceContext,kind:'boolean-region',id:`boolean-region:${focusId}`,regionId:focusId,analysisId:analysis.regions[0]?.reference.inputDigest??'',documentId:view.document.id,versionId:view.document.versionId,occurrencePath}]:[]):selectedBodies.map(id=>({...occurrenceContext,kind:'body',id,bodyId:id,documentId:view.document.id,versionId:view.document.versionId,occurrencePath})),
            onPick:s=>pickRef.current(s)});
    },[token,resultMode]);
    const valid = !loading && version.current === view.document.versionId && booleanDefinitionReady(target, operation, tools, stages);
    const definition: Feature = { ...initial, id: initial?.id ?? "", type: "BOOLEAN", bodyId: target, operation, tools, keepTools: keep, suppressed };
    const resultReady=resultMode==='ALL'||!!analysis&&!analysisPending&&!failedRegions.length&&retained.length>0;
    definition.booleanResult=resultMode==='EXPLICIT'?{mode:'EXPLICIT',regions:[...new Map(resolved.flatMap(r=>r.cell?[r.cell.reference]:[]).map(ref=>[ref.cellDigest,ref])).values()]}:undefined;
    const input: Record<string, unknown> | undefined = valid && resultReady ? initial ? { type: "EDIT_FEATURE", targetId: initial.id, expectedFeatureDigest: digest, feature: definition, requestId: intent.current } : { type: "CREATE_BOOLEAN_FEATURE", bodyId: target, operation, tools, keepTools: keep, booleanResult:definition.booleanResult, requestId: intent.current } : undefined;
    const preview = useFeaturePreview(view.document.id, view.document.versionId, input, operation, (artifact,op)=>{if(resultMode==='ALL')onPreview(artifact,op);else onPreview();});
    const apply = async () => { if (!input || !preview.previewId || preview.pending || committing)
        return; setCommitting(true); try {
        await onApply({ ...input, previewId: preview.previewId });
        onClose();
    }
    catch (cause) {
        setError(engineeringProblem(cause));
        setCommitting(false);
    } };
    return <CommandDialog id="boolean-feature" title={initial ? "编辑布尔" : "布尔运算"} open size="S" onClose={onClose} onCancelPick={cancelPick} commandStatus={<CommandStatus pending={loading||analysisPending||preview.pending} error={analysisError??preview.error} onRetry={analysisError?()=>setAnalysisRetry(v=>v+1):preview.retry}/>} onConfirm={apply} confirmDisabled={!input || !preview.previewId || preview.pending || committing} confirmLoading={committing}>
  <fieldset disabled={committing} className="instance-pattern-fields feature-input-fields">
   <FeaturePickField label="目标实体" value={target?roleName(view.part?.bodies.find(b => b.id === target)?.name??"实体已丢失",'target'):undefined} selected={!!target} active={role === "target"} onActivate={() => {setReplacingRegion(undefined);setReplacing(undefined);setRole("target")}} onClear={target?()=>setTarget(undefined):undefined}/>
   <CommandField  label={<>运算</>} width="content"><Segmented value={operation} options={[{ value: "ADD", label: "并集" }, { value: "REMOVE", label: "差集" }, { value: "INTERSECT", label: "交集" }]} onChange={value => { setOperation(value as Feature["operation"]); if (value === "INTERSECT")
        setTools(previous => previous.slice(0, 1)); }}/></CommandField>
   <FeaturePickField mode="multiple" label="工具实体" active={role==='tools'&&!replacing} onActivate={()=>{setReplacingRegion(undefined);setReplacing(undefined);setRole('tools')}} onClear={tools.length?()=>setTools([]):undefined} items={tools.map((tool,i)=>({key:tool.bodyId,actionLabel:`工具 ${i+1}`,value:roleName(view.part?.bodies.find(b=>b.id===tool.bodyId)?.name??'实体已丢失','tool'),searchText:view.part?.bodies.find(b=>b.id===tool.bodyId)?.name,error:!view.part?.bodies.some(b=>b.id===tool.bodyId),active:role==='tools'&&replacing===tool.bodyId,onActivate:()=>{setReplacingRegion(undefined);setReplacing(tool.bodyId);setRole('tools')},onClear:()=>setTools(previous=>previous.filter(t=>t.bodyId!==tool.bodyId)),selections:[{...occurrenceContext,kind:'body' as const,id:tool.bodyId,bodyId:tool.bodyId,documentId:view.document.id,versionId:version.current,occurrencePath}]}))}/>

   <CommandField label="结果区域" width="content" error={resultMode==='EXPLICIT'&&tools.length!==1?'选择区域需要一个工具实体':undefined}><Segmented aria-label="结果区域" value={resultMode} options={[{value:'ALL',label:'全部'},{value:'EXPLICIT',label:'选择区域',disabled:tools.length!==1}]} onChange={value=>{
       const mode=value as 'ALL'|'EXPLICIT';setResultMode(mode);onPreview();
       setReplacingRegion(undefined);if(mode==='EXPLICIT'){initializeAll.current=!analysis;if(analysis)setRetained(analysis.regions.filter(c=>c.eligible).map(c=>c.reference));setRole('regions')}else{initializeAll.current=false;setRole(undefined);callbacks.current.onInputArtifacts(originalInputs.current);}
   }}/></CommandField>
   {resultMode==='EXPLICIT'&&<FeaturePickField label="结果区域" mode="multiple" active={role==='regions'} onActivate={()=>{setReplacing(undefined);setReplacingRegion(undefined);setRole('regions')}} onClear={retained.length?()=>setRetained([]):undefined}
      error={!analysisPending&&analysis&&!retained.length?'至少保留一个结果区域':undefined}
      items={[
        ...failedRegions.map(index=>({key:`missing:${index}`,value:`失效区域 ${index+1}`,error:true,detail:resolved[index].status==='AMBIGUOUS'?'无法唯一对应':analysis?.regions.some(c=>matchBooleanRegion(retained[index],[{...c,eligible:true}]).status==='RESOLVED')?'不属于当前运算结果':'原区域已不存在',active:replacingRegion===index,onActivate:()=>{setReplacing(undefined);setReplacingRegion(index);setRole('regions')},onClear:()=>setRetained(previous=>previous.filter((_,i)=>i!==index))})),
        ...(analysis?.regions??[]).filter(c=>c.eligible).map((cell,index)=>{
           const id=cell.reference.cellDigest,selected=resolved.some(r=>r.cell?.reference.cellDigest===id),name=`区域 ${index+1}`;
           return {key:id,actionLabel:name,value:roleName(name,selected?'retained':'excluded'),searchText:name,onSelect:()=>setFocusId(id),state:selected?'retained':'excluded',active:focusId===id,
             onLocate:()=>{setFocusId(id);setFocusRequest(n=>n+1)},
             actions:<><Tooltip title={isolateId===id?'显示全部区域':'单独显示'}><Button size="small" type="text" aria-label={`单独显示${name}`} aria-pressed={isolateId===id} icon={<EyeOutlined/>} onClick={()=>{setFocusId(id);setIsolateId(previous=>previous===id?undefined:id)}}/></Tooltip><Switch size="small" aria-label={`保留${name}`} checked={selected} checkedChildren="保留" unCheckedChildren="排除" onChange={()=>toggleRegion(cell)}/></>};
        })]}/>}
   <CommandField  label={<>保留工具实体</>} width="option"><Switch checked={keep} onChange={setKeep}/></CommandField>
   {initial && <CommandField  label={<>抑制</>} width="option"><Switch checked={suppressed} onChange={setSuppressed}/></CommandField>}

   {error&&<Alert type="error" title={error}/>}

  </fieldset>

</CommandDialog>;
}
