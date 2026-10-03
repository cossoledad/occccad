import { Alert, Button, Input, Select, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { api } from "../../api/client";
import type { Artifact, DocumentView, Feature, Selection } from "../../types";
import { parameterSourceText } from "./parameter-editor";
import { solidParameterEdit, solidFeatureNames } from "./solid-feature-model";
import { randomUUID } from "../../utils/random-uuid";
export function SolidFeatureEditor({ view, feature, digest, unit, selection, onClose, onApply, onPreview }: {
    view: DocumentView;
    feature: Feature;
    digest?: string;
    unit: string;
    selection?: Selection;
    onClose: () => void;
    onApply: (input: Record<string, unknown>) => Promise<unknown>;
    onPreview: (artifact?: Artifact, operation?: Feature["operation"]) => void;
}) {
    const loft = feature.type === "LOFT";
    const modifier = ["FILLET", "CHAMFER", "DRAFT", "SHELL"].includes(feature.type);
    const angular = feature.type === "REVOLVE" || feature.type === "DRAFT";
    const [draft, setDraft] = useState({ ...feature });
    const initial = (slot: string, fallback: number) => { const p = view.part?.parameters?.find(p => p.ownerFeatureId === feature.id && p.propertySlot === slot); return p ? parameterSourceText(p, slot === "angle" ? "deg" : unit, true) : String(slot === "angle" ? fallback : fallback / (({ mm: 1, cm: 10, m: 1000, in: 25.4 } as Record<string, number>)[unit] ?? 1)); };
    const initialText = useRef({ length: initial("length", feature.length ?? 10), length2: feature.extent === "TWO_SIDED" ? initial("length2", feature.length2 ?? 10) : "", angle: initial("angle", feature.angle ?? 360) });
    const [length, setLength] = useState(initial("length", feature.length ?? 10)), [second, setSecond] = useState(initial("length2", feature.length2 ?? 10)), [angle, setAngle] = useState(initial("angle", feature.angle ?? 360));
    const [busy, setBusy] = useState(false), [error, setError] = useState<string>();
    const generation = useRef(0), abort = useRef<AbortController>(undefined), preview = useRef<string>(undefined), intent = useRef(randomUUID());
    const changed = () => { generation.current++; abort.current?.abort(); preview.current = undefined; onPreview(); setError(undefined); setBusy(false); };
    useEffect(() => () => { generation.current++; abort.current?.abort(); onPreview(); }, []);
    useEffect(() => { changed(); }, [view.document.versionId]);
    const update = (patch: Partial<Feature>) => { changed(); setDraft({ ...draft, ...patch }); };
    const command = () => {
        const value = { ...draft }, expressions: Record<string, string> = {};
        const read = (slot: "length" | "length2" | "angle", text: string) => {
            const edit = solidParameterEdit(slot, text, initialText.current[slot], unit);
            if (edit.value !== undefined)
                value[slot] = edit.value;
            if (edit.expression !== undefined)
                expressions[slot] = edit.expression;
        };
        if (!loft) {
            if (angular)
                read("angle", angle);
            else {
                read("length", length);
                if (draft.extent === "TWO_SIDED")
                    read("length2", second);
            }
        }
        return { type: feature.id ? "EDIT_FEATURE" : loft ? "CREATE_SOLID_FEATURE" : "CREATE_MODIFY_FEATURE", targetId: feature.id, expectedFeatureDigest: digest, feature: value, parameterExpressions: expressions, requestId: intent.current };
    };
    const run = async (commit: boolean) => {
        if (busy)
            return;
        setError(undefined);
        let input;
        try {
            input = command();
        }
        catch (cause) {
            setError(String(cause));
            return;
        }
        const sequence = ++generation.current;
        setBusy(true);
        try {
            if (commit) {
                await onApply({ ...input, previewId: preview.current });
                onClose();
            }
            else {
                abort.current?.abort();
                const controller = new AbortController();
                abort.current = controller;
                const result = await api.previewCommand(view.document.id, input, controller.signal);
                if (sequence === generation.current && result.baseVersionId === view.document.versionId) {
                    preview.current = result.previewId;
                    onPreview(result.artifact, draft.operation);
                }
            }
        }
        catch (cause) {
            if (sequence === generation.current)
                setError(String(cause));
        }
        finally {
            if (sequence === generation.current)
                setBusy(false);
        }
    };
    const addSelection = async () => {
        if (!selection || !(selection.kind === "edge" || selection.kind === "face") || selection.bodyId !== draft.bodyId || selection.documentId && selection.documentId !== view.document.id)
            return;
        const kind = feature.type === "FILLET" || feature.type === "CHAMFER" ? "EDGE" : "FACE";
        if (selection.kind.toUpperCase() !== kind)
            return;
        const seq = ++generation.current;
        try {
            const bound = await api.getTopologyProperties(view.document.id, selection.geometryKey!, kind, selection.topologyId, selection.versionId ?? view.document.versionId);
            if (seq !== generation.current)
                return;
            if (!bound.persistentSelection)
                throw new Error("所选拓扑没有可用的持久引用");
            const picks = [...(draft.selections ?? [])];
            if (!picks.some(p => JSON.stringify(p.selection.anchor) === JSON.stringify(bound.persistentSelection!.anchor)))
                picks.push({ selection: bound.persistentSelection, sourceVersionId: selection.versionId ?? view.document.versionId });
            update({ selections: picks });
        }
        catch (cause) {
            setError(String(cause));
        }
    };
    const index = feature.id ? (view.part?.features.findIndex(f => f.id === feature.id) ?? 0) : (view.part?.features.length ?? 0);
    const upstream = view.part?.features.slice(0, index) ?? [];
    const definitionReady = loft ? (draft.sections?.length ?? 0) >= 2 : modifier ? (draft.selections?.length ?? 0) > 0 && !!draft.bodyId && (!angular || !!draft.neutralPlaneId) : !!draft.profile;
    return <CommandDialog id="solid-feature-editor" open title={`${feature.id ? "编辑" : "创建"} ${feature.name ?? solidFeatureNames[feature.type] ?? feature.type}`} size="S" onClose={onClose} onConfirm={() => run(true)} confirmLoading={busy} confirmDisabled={busy || !definitionReady}>
  <div className="instance-pattern-fields">
   {!modifier && <>{!loft && <label>轮廓<Select value={draft.profile} disabled={busy} options={upstream.filter(f => f.sketch).map(f => ({ value: f.id, label: f.name ?? f.id }))} onChange={profile => update({ profile })}/></label>}
   <label>运算<Select value={draft.operation} disabled={busy} options={[...(!feature.id ? [{ value: "NEW_BODY", label: "新建 Body" }] : []), { value: "ADD", label: "添加材料" }, { value: "REMOVE", label: "移除材料" }, { value: "INTERSECT", label: "交集" }]} onChange={operation => update({ operation })}/></label></>}
   {loft && <>
    <label>截面（按添加顺序）<Select disabled={busy} mode="multiple" value={draft.sections?.map(s => s.sketchId)} options={upstream.filter(f => f.sketch).map(f => ({ value: f.id, label: f.name ?? f.id }))} onChange={ids => update({ sections: ids.map(sketchId => draft.sections?.find(s => s.sketchId === sketchId) ?? { sketchId }) })}/></label>
    {(draft.sections ?? []).map((section, i) => <div key={section.sketchId}>
     <span>截面 {i + 1}</span><Button size="small" disabled={busy || !i} onClick={() => { const sections = [...draft.sections!]; [sections[i - 1], sections[i]] = [sections[i], sections[i - 1]]; update({ sections }); }}>上移</Button>
     <label>反向<Switch disabled={busy} checked={section.reversed} onChange={reversed => update({ sections: draft.sections?.map((s, j) => i === j ? { ...s, reversed } : s) })}/></label>
     {upstream.find(f => f.id === section.sketchId)?.sketch?.entities.some(e => e.kind === "CIRCLE") && <label>圆的闭合点角度（deg）<Input disabled={busy} type="number" value={section.seamAngle ?? 0} onChange={e => update({ sections: draft.sections?.map((s, j) => i === j ? { ...s, seamAngle: Number(e.target.value) } : s) })}/></label>}
     <label>闭合起始边<Select disabled={busy} allowClear value={section.seamEntityId} options={upstream.find(f => f.id === section.sketchId)?.sketch?.entities.filter(e => e.role !== "CONSTRUCTION" && e.kind !== "POINT").map(e => ({ value: e.id, label: e.id }))} onChange={seamEntityId => update({ sections: draft.sections?.map((s, j) => i === j ? { ...s, seamEntityId } : s) })}/></label>
    </div>)}
    <label>直纹<Switch disabled={busy} checked={draft.ruled} onChange={ruled => update({ ruled })}/></label>
   </>}
   {modifier && <>
    <Button disabled={busy} onClick={() => void addSelection()}>添加当前选择的{feature.type === "FILLET" || feature.type === "CHAMFER" ? "边" : "面"}</Button>
    {(draft.selections ?? []).map((pick, i) => <div key={JSON.stringify(pick.selection.anchor)}>选择 {i + 1}<Button size="small" disabled={busy} onClick={() => update({ selections: draft.selections?.filter((_, j) => j !== i) })}>移除</Button></div>)}
    {feature.type === "DRAFT" && <label>中性平面<Select disabled={busy} value={draft.neutralPlaneId} options={view.part?.datumPlanes.map(p => ({ value: p.id, label: p.name }))} onChange={neutralPlaneId => update({ neutralPlaneId })}/></label>}
   </>}
   {!loft && (angular ? <>

    <label>角度（deg）<Input value={angle} disabled={busy} onChange={e => { changed(); setAngle(e.target.value); }}/></label>
    {feature.type === "REVOLVE" && <label>轴<Select value={draft.axisEntityId} disabled={busy} options={[...upstream.flatMap(f => (f.sketch?.entities ?? []).filter(e => e.kind === "LINE").map(e => ({ value: `SKETCH_LINE:${f.id}:${e.id}`, label: `${f.name} / ${e.id}` }))), ...(view.part?.datumAxes ?? []).map(a => ({ value: `DATUM_AXIS:${a.id}`, label: a.name })), ...(view.part?.axisSystems ?? []).flatMap(a => ["X", "Y", "Z"].map(axis => ({ value: `AXIS_SYSTEM:${a.id}:${axis}`, label: `${a.name} ${axis}` })))]} onChange={axisEntityId => update({ axisEntityId })}/></label>}
   </> : <>
    {!modifier && <label>范围<Select value={draft.extent ?? "FINITE"} disabled={busy} options={[{ value: "FINITE", label: "有限长度" }, { value: "TWO_SIDED", label: "双侧" }, { value: "SYMMETRIC", label: "对称（总长）" }, ...(draft.operation === "REMOVE" ? [{ value: "THROUGH_ALL", label: "贯穿切除" }] : [])]} onChange={extent => update({ extent })}/></label>}
    {draft.extent !== "THROUGH_ALL" && <label>{feature.type === "FILLET" ? "半径" : feature.type === "SHELL" ? "厚度" : feature.type === "CHAMFER" ? "等距距离" : "长度"}（{unit}）<Input value={length} disabled={busy} onChange={e => { changed(); setLength(e.target.value); }}/></label>}
    {draft.extent === "TWO_SIDED" && <label>反向{feature.type === "FILLET" ? "半径" : feature.type === "SHELL" ? "厚度" : feature.type === "CHAMFER" ? "等距距离" : "长度"}（{unit}）<Input value={second} disabled={busy} onChange={e => { changed(); setSecond(e.target.value); }}/></label>}
   </>)}
   <label>反向<Switch checked={draft.reversed} disabled={busy} onChange={reversed => update({ reversed })}/></label>
   <label>抑制<Switch checked={draft.suppressed} disabled={busy} onChange={suppressed => update({ suppressed })}/></label>
   <Button disabled={busy || !definitionReady} onClick={() => void run(false)}>预览</Button>
   {error && <Alert type="error" title="特征求值失败" description={error}/>}
  </div>
 </CommandDialog>;
}
