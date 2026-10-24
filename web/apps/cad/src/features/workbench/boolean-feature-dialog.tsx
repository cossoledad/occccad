import {CommandField,CommandStatus} from "../../cad/overlay/command-form";
import {engineeringProblem} from "../../cad/command/operation-feedback";

import { Alert, Segmented, Switch } from "antd";
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
    const [role, setRole] = useState<"target" | "tools"|undefined>("tools"), [error, setError] = useState<string>(), [committing, setCommitting] = useState(false);
    const [replacing,setReplacing]=useState<string>();
    const cancelPick=()=>{if(!role)return false;setRole(undefined);setReplacing(undefined);return true};
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
            if (alive)
                callbacks.current.onInputArtifacts(context.artifacts ?? [context.artifact]);
        }).catch(cause => { if (alive)
            setError(engineeringProblem(cause)); }).finally(() => { if (alive)
            setLoading(false); });
        return () => { alive = false; controller.abort(); };
    }, []);
    const pick = (s: SelectionItem) => {
        if (!role || committing || version.current !== view.document.versionId || (s.ownerDocumentId ?? s.documentId) !== view.document.id || (s.occurrencePath ?? "") !== (occurrencePath ?? ""))
            return;
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
    const token = JSON.stringify(selectedBodies);
    useEffect(() => {
        callbacks.current.onSelectionSession(role?{ role: "body", documentId: view.document.id, versionId: version.current, occurrencePath,
            selections: selectedBodies.map(id => ({ ...occurrenceContext, kind: "body", id, bodyId: id, documentId: view.document.id, versionId: version.current, occurrencePath })), onPick: s => pickRef.current(s) }:undefined);
    }, [token, role]);
    const valid = !loading && version.current === view.document.versionId && booleanDefinitionReady(target, operation, tools, stages);
    const definition: Feature = { ...initial, id: initial?.id ?? "", type: "BOOLEAN", bodyId: target, operation, tools, keepTools: keep, suppressed };
    const input: Record<string, unknown> | undefined = valid ? initial ? { type: "EDIT_FEATURE", targetId: initial.id, expectedFeatureDigest: digest, feature: definition, requestId: intent.current } : { type: "CREATE_BOOLEAN_FEATURE", bodyId: target, operation, tools, keepTools: keep, requestId: intent.current } : undefined;
    const preview = useFeaturePreview(view.document.id, view.document.versionId, input, operation, onPreview);
    const apply = async () => { if (!input || !preview.previewId || preview.pending || committing)
        return; setCommitting(true); try {
        await onApply({ ...input, previewId: preview.previewId });
        onClose();
    }
    catch (cause) {
        setError(engineeringProblem(cause));
        setCommitting(false);
    } };
    return <CommandDialog id="boolean-feature" title={initial ? "编辑布尔" : "布尔运算"} open size="S" onClose={onClose} onCancelPick={cancelPick} commandStatus={<CommandStatus pending={loading||preview.pending} error={preview.error} onRetry={preview.retry}/>} onConfirm={apply} confirmDisabled={!input || !preview.previewId || preview.pending || committing} confirmLoading={committing}>
  <fieldset disabled={committing} className="instance-pattern-fields feature-input-fields">
   <FeaturePickField label="目标实体" value={view.part?.bodies.find(b => b.id === target)?.name ?? "请在视图区选择实体"} selected={!!target} active={role === "target"} onActivate={() => setRole("target")} onClear={target?()=>setTarget(undefined):undefined}/>
   <CommandField  label={<>运算</>} width="enum"><Segmented value={operation} options={[{ value: "ADD", label: "并集" }, { value: "REMOVE", label: "差集" }, { value: "INTERSECT", label: "交集" }]} onChange={value => { setOperation(value as Feature["operation"]); if (value === "INTERSECT")
        setTools(previous => previous.slice(0, 1)); }}/></CommandField>
   <FeaturePickField mode="multiple" label="工具实体" value={`已选择 ${tools.length} 个实体`} selected={tools.length>0} active={role === "tools"&&!replacing} onActivate={() => {setReplacing(undefined);setRole("tools");}} onClear={tools.length ? () => setTools([]) : undefined}/>
   <CommandField  label={<>保留工具结果</>} width="option"><Switch checked={keep} onChange={setKeep}/></CommandField>
   {initial && <CommandField  label={<>抑制</>} width="option"><Switch checked={suppressed} onChange={setSuppressed}/></CommandField>}
   {tools.map((tool,i)=><FeaturePickField key={tool.bodyId} label={`工具 ${i+1}`} value={view.part?.bodies.find(b=>b.id===tool.bodyId)?.name??"实体已丢失"} selected active={role==="tools"&&replacing===tool.bodyId} onActivate={()=>{setReplacing(tool.bodyId);setRole("tools");}} onClear={()=>setTools(previous=>previous.filter(t=>t.bodyId!==tool.bodyId))} selections={[{...occurrenceContext,kind:"body",id:tool.bodyId,bodyId:tool.bodyId,documentId:view.document.id,versionId:version.current,occurrencePath}]}/>)}
   {error&&<Alert type="error" title={error}/>}

  </fieldset>

</CommandDialog>;
}
