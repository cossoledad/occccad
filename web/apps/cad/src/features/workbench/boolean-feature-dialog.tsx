import { Alert, Button, Select, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { api } from "../../api/client";
import type { Artifact, DocumentView, Feature } from "../../types";
import { booleanInputStages, booleanDefinitionReady } from "./solid-feature-model";
import { randomUUID } from "../../utils/random-uuid";
export function BooleanFeatureDialog({ view, initial, digest, bodyId, onClose, onApply, onPreview }: {
    view: DocumentView;
    initial?: Feature;
    digest?: string;
    bodyId?: string;
    onClose: () => void;
    onApply: (input: Record<string, unknown>) => Promise<unknown>;
    onPreview: (artifact?: Artifact, operation?: Feature["operation"]) => void;
}) {
    const [target, setTarget] = useState(initial?.bodyId ?? bodyId);
    const [operation, setOperation] = useState<Feature["operation"]>(initial?.operation ?? "REMOVE");
    const [tools, setTools] = useState(initial?.tools ?? []);
    const [keep, setKeep] = useState(initial?.keepTools ?? false);
    const [suppressed, setSuppressed] = useState(initial?.suppressed ?? false);
    const [error, setError] = useState<string>();
    const [busy, setBusy] = useState(false);
    const generation = useRef(0), abort = useRef<AbortController>(undefined), preview = useRef<string>(undefined);
    const intent = useRef(randomUUID());
    const features = view.part?.features ?? [];
    const stages = booleanInputStages(features, initial?.id);
    const targets = (view.part?.bodies ?? []).filter(body => stages.some(feature => feature.bodyId === body.id));
    const valid = booleanDefinitionReady(target, operation, tools, stages);
    const definition: Feature = { ...initial, id: initial?.id ?? "", type: "BOOLEAN", bodyId: target, operation, tools, keepTools: keep, suppressed };
    const input: Record<string, unknown> = initial ? { type: "EDIT_FEATURE", targetId: initial.id, expectedFeatureDigest: digest, feature: definition } : { type: "CREATE_BOOLEAN_FEATURE", bodyId: target, operation, tools, keepTools: keep };
    const changed = () => { generation.current++; abort.current?.abort(); preview.current = undefined; onPreview(); setError(undefined); setBusy(false); };
    useEffect(() => () => { generation.current++; abort.current?.abort(); onPreview(); }, []);
    useEffect(() => { changed(); }, [view.document.versionId]);
    const requestPreview = async () => {
        changed();
        const sequence = generation.current;
        const controller = new AbortController();
        abort.current = controller;
        setBusy(true);
        try {
            const result = await api.previewCommand(view.document.id, { ...input, requestId: intent.current }, controller.signal);
            if (sequence !== generation.current || result.baseVersionId !== view.document.versionId)
                return;
            preview.current = result.previewId;
            onPreview(result.artifact, operation);
        }
        catch (cause) {
            if (!controller.signal.aborted)
                setError(String(cause));
        }
        finally {
            if (sequence === generation.current)
                setBusy(false);
        }
    };
    const apply = async () => { if (!valid || busy)
        return; setBusy(true); setError(undefined); try {
        await onApply({ ...input, requestId: intent.current, previewId: preview.current });
        onClose();
    }
    catch (cause) {
        setError(String(cause));
    }
    finally {
        setBusy(false);
    } };
    return <CommandDialog id="boolean-feature" title={initial ? "编辑布尔" : "布尔运算"} open size="S" onClose={onClose} onConfirm={apply} confirmDisabled={!valid || busy} confirmLoading={busy}>
    <div className="instance-pattern-fields">
      <label>目标 Body<Select value={target} disabled={busy} options={targets.map(b => ({ value: b.id, label: b.name }))} onChange={value => { changed(); setTarget(value); setTools(tools.filter(t => t.bodyId !== value)); }}/></label>
      <label>运算<Select value={operation} disabled={busy} options={[{ value: "ADD", label: "并集" }, { value: "REMOVE", label: "差集" }, { value: "INTERSECT", label: "交集" }]} onChange={value => { changed(); setOperation(value); if (value === "INTERSECT")
        setTools(tools.slice(0, 1)); }}/></label>
      <label>工具输出阶段<Select mode="multiple" value={tools.map(t => t.featureId)} disabled={busy} options={stages.filter(f => f.bodyId !== target).map(f => ({ value: f.id, label: `${view.part?.bodies.find(b => b.id === f.bodyId)?.name} / ${f.name ?? f.id}` }))} onChange={ids => { changed(); const selected = ids.map(id => stages.find(f => f.id === id)!); setTools(selected.map(f => ({ bodyId: f.bodyId!, featureId: f.id }))); }}/></label>
      <label>保留工具结果<Switch checked={keep} disabled={busy} onChange={value => { changed(); setKeep(value); }}/></label>
      {initial && <label>抑制<Switch checked={suppressed} disabled={busy} onChange={value => { changed(); setSuppressed(value); }}/></label>}
      <Button onClick={() => void requestPreview()} disabled={!valid || busy}>预览</Button>
      {error && <Alert type="error" title="布尔求值失败" description={error}/>}
    </div>
  </CommandDialog>;
}
