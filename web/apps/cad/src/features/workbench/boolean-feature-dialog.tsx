import {DiagnosticCopy,diagnosticReference} from "../../cad/command/diagnostic-copy";
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
    const [role, setRole] = useState<"target" | "tools">("tools"), [error, setError] = useState<string>(), [committing, setCommitting] = useState(false);
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
            setError(String(cause)); }).finally(() => { if (alive)
            setLoading(false); });
        return () => { alive = false; controller.abort(); };
    }, []);
    const pick = (s: SelectionItem) => {
        if (committing || version.current !== view.document.versionId || (s.ownerDocumentId ?? s.documentId) !== view.document.id || (s.occurrencePath ?? "") !== (occurrencePath ?? ""))
            return;
        const stage = bodyStage(s, stages);
        if (!stage)
            return;
        setError(undefined);
        if (role === "target") {
            setTarget(stage.bodyId);
            setTools(previous => previous.filter(t => t.bodyId !== stage.bodyId));
            setRole("tools");
        }
        else if (stage.bodyId !== target)
            setTools(previous => previous.some(t => t.bodyId === stage.bodyId) ? previous.filter(t => t.bodyId !== stage.bodyId) : operation === "INTERSECT" ? [stage] : [...previous, stage]);
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
        callbacks.current.onSelectionSession({ role: "body", documentId: view.document.id, versionId: version.current, occurrencePath,
            selections: selectedBodies.map(id => ({ ...occurrenceContext, kind: "body", id, bodyId: id, documentId: view.document.id, versionId: version.current, occurrencePath })), onPick: s => pickRef.current(s) });
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
        setError(String(cause));
        setCommitting(false);
    } };
    return <CommandDialog id="boolean-feature" title={initial ? "编辑布尔" : "布尔运算"} open size="S" onClose={onClose} onConfirm={apply} confirmDisabled={!input || !preview.previewId || preview.pending || committing} confirmLoading={committing}>
  <fieldset disabled={committing} className="instance-pattern-fields feature-input-fields">
   <FeaturePickField label="目标实体" value={view.part?.bodies.find(b => b.id === target)?.name ?? "请在视图区选择实体"} active={role === "target"} onActivate={() => setRole("target")}/>
   <label>运算<Segmented value={operation} options={[{ value: "ADD", label: "并集" }, { value: "REMOVE", label: "差集" }, { value: "INTERSECT", label: "交集" }]} onChange={value => { setOperation(value as Feature["operation"]); if (value === "INTERSECT")
        setTools(previous => previous.slice(0, 1)); }}/></label>
   <FeaturePickField label="工具实体" value={`已选择 ${tools.length} 个实体`} active={role === "tools"} onActivate={() => setRole("tools")} onClear={tools.length ? () => setTools([]) : undefined}/>
   <label>保留工具结果<Switch checked={keep} onChange={setKeep}/></label>
   {initial && <label>抑制<Switch checked={suppressed} onChange={setSuppressed}/></label>}
   <small role="status">{loading ? "正在恢复上游实体…" : preview.pending ? "正在预览…" : "在视图区点击实体添加工具，再次点击取消。"}</small>
   {(error ?? preview.error) && <Alert type="error" title="布尔求值失败" description={error ?? preview.error}/>}
  </fieldset>
 {!!preview.failure && <DiagnosticCopy text={diagnosticReference(preview.failure,{documentId:view.document.id,versionId:view.document.versionId})}/>}
</CommandDialog>;
}
