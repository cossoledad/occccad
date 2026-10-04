import { PatternFeatureEditor } from "./pattern-feature-editor";
import { Alert, Button, Input, Segmented, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { api } from "../../api/client";
import type { Artifact, DocumentView, Feature, SelectionItem } from "../../types";
import type { FeaturePickRole, FeatureSelectionSession } from "../../cad/interaction/feature-selection";
import { selectionKey } from "../../cad/interaction/selection-identity";
import { parameterSourceText } from "./parameter-editor";
import { solidParameterEdit, solidFeatureNames, pickLoftSection } from "./solid-feature-model";
import { selectedAxis, selectedSketch, sketchPick, profileSelectionIDs } from "./feature-picking";
import { FeaturePickField } from "./feature-pick-field";
import { useFeaturePreview } from "./use-feature-preview";
import { randomUUID } from "../../utils/random-uuid";
type BoundPick = {
    definition: NonNullable<Feature["selections"]>[number];
    visual: SelectionItem;
};
export function SolidFeatureEditor(props: Parameters<typeof SolidFeatureDefinitionEditor>[0]) {
    return props.feature.pattern ? <PatternFeatureEditor {...props}/> : <SolidFeatureDefinitionEditor {...props}/>;
}
function SolidFeatureDefinitionEditor({ view, feature, digest, unit, seed, occurrencePath, occurrenceContext, onClose, onApply, onPreview, onSelectionSession, onInputArtifact }: {
    view: DocumentView;
    feature: Feature;
    digest?: string;
    unit: string;
    seed: SelectionItem[];
    occurrencePath?: string;
    occurrenceContext?: Pick<SelectionItem, "instancePath" | "contextVariantKey" | "rootDocumentId">;
    onClose: () => void;
    onApply: (input: Record<string, unknown>) => Promise<unknown>;
    onPreview: (artifact?: Artifact, operation?: Feature["operation"], sketchFeatureId?:string) => void;
    onSelectionSession: (session?: FeatureSelectionSession) => void;
    onInputArtifact: (artifact?: Artifact | Artifact[]) => void;
}) {
    const loft = feature.type === "LOFT", modifier = ["FILLET", "CHAMFER", "DRAFT", "SHELL"].includes(feature.type);
    const angular = feature.type === "REVOLVE" || feature.type === "DRAFT";
    const pickRole: FeaturePickRole = modifier ? (feature.type === "FILLET" || feature.type === "CHAMFER" ? "edge" : "face") : "profile";
    const [draft, setDraft] = useState({ ...feature });
    const [role, setRole] = useState<FeaturePickRole>(pickRole), [seamIndex, setSeamIndex] = useState<number>();
    const [neutralVisual, setNeutralVisual] = useState<SelectionItem>();
    const [picks, setPicks] = useState<BoundPick[]>([]), [axisVisual, setAxisVisual] = useState<SelectionItem>();
    const [error, setError] = useState<string>(), [committing, setCommitting] = useState(false), [binding, setBinding] = useState(0);
    const [inputContext, setInputContext] = useState<Awaited<ReturnType<typeof api.getFeatureInput>>>();
    const [loadingInput, setLoadingInput] = useState(modifier && !!feature.id);
    const initial = (slot: string, fallback: number) => {
        const p = view.part?.parameters?.find(p => p.ownerFeatureId === feature.id && p.propertySlot === slot);
        return p ? parameterSourceText(p, slot === "angle" ? "deg" : unit, true) : String(slot === "angle" ? fallback : fallback / (({ mm: 1, cm: 10, m: 1000, in: 25.4 } as Record<string, number>)[unit] ?? 1));
    };
    const original = useRef({ length: feature.id ? initial("length", feature.length ?? 10) : "", length2: feature.id && feature.extent === "TWO_SIDED" ? initial("length2", feature.length2 ?? 10) : "", angle: feature.id ? initial("angle", feature.angle ?? 360) : "" });
    const [length, setLength] = useState(initial("length", feature.length ?? 10)), [second, setSecond] = useState(initial("length2", feature.length2 ?? 10)), [angle, setAngle] = useState(initial("angle", feature.angle ?? 360));
    const intent = useRef(randomUUID()), version = useRef(view.document.versionId), alive = useRef(true), queue = useRef(Promise.resolve());
    const callbacks = useRef({ onSelectionSession, onInputArtifact });
    callbacks.current = { onSelectionSession, onInputArtifact };
    const index = feature.id ? view.part!.features.findIndex(f => f.id === feature.id) : view.part!.features.length;
    const upstream = view.part!.features.slice(0, Math.max(0, index));
    const edit = (patch: Partial<Feature>) => { setError(undefined); setDraft(current => ({ ...current, ...patch })); };
    const validRevision = version.current === view.document.versionId;
    useEffect(() => { alive.current = true; return () => { alive.current = false; callbacks.current.onSelectionSession(); callbacks.current.onInputArtifact(); }; }, []);
    useEffect(() => {
        if (!modifier || !feature.id)
            return;
        let current = true;
        const controller = new AbortController();
        setLoadingInput(true);
        void api.getFeatureInput(view.document.id, { versionId: view.document.versionId, featureId: feature.id }, controller.signal).then(context => {
            if (!current)
                return;
            setInputContext(context);
            callbacks.current.onInputArtifact(context.artifacts ?? context.artifact);
            if (context.neutralPick) { const p = context.neutralPick; setNeutralVisual({ ...occurrenceContext, kind:"face", id:`input:neutral:${p.localId}`, documentId:view.document.id, bodyId:p.bodyId, versionId:context.versionId, geometryKey:p.geometryKey, topologyId:p.localId, occurrencePath }); }
            setPicks(context.picks.map(p => ({ definition: feature.selections![p.index], visual: { ...occurrenceContext, kind: p.kind.toLowerCase() as "edge" | "face", id: `input:${p.kind}:${p.localId}`, documentId: view.document.id, bodyId: feature.bodyId, versionId: context.versionId, geometryKey: context.artifact.geometryKey, topologyId: p.localId, occurrencePath } })));
        }).catch(cause => { if (current && !controller.signal.aborted)
            setError(String(cause)); }).finally(() => { if (current)
            setLoadingInput(false); });
        return () => { current = false; controller.abort(); };
    }, []);
    const current = useRef({ draft, role, inputContext, upstream, committing, validRevision });
    current.current = { draft, role, inputContext, upstream, committing, validRevision };
    const bindPick = (selection: SelectionItem) => {
        setBinding(n => n + 1);
        queue.current = queue.current.then(async () => {
            if (!alive.current || !current.current.validRevision || current.current.committing)
                return;
            const state = current.current;
            if ((selection.kind !== "edge" && selection.kind !== "face") || selection.kind !== pickRole || selection.bodyId !== state.draft.bodyId || !selection.geometryKey)
                return;
            let definition: NonNullable<Feature["selections"]>[number];
            if (feature.id) {
                if (!state.inputContext || selection.geometryKey !== state.inputContext.artifact.geometryKey)
                    return;
                const result = await api.getFeatureInput(view.document.id, { versionId: version.current, featureId: feature.id, geometryKey: selection.geometryKey, kind: selection.kind.toUpperCase(), localId: selection.topologyId });
                if (!result.selection)
                    throw new Error("所选边面没有可用的持久引用");
                definition = result.selection;
            }
            else {
                const result = await api.getTopologyProperties(view.document.id, selection.geometryKey, selection.kind.toUpperCase() as "EDGE" | "FACE", selection.topologyId, selection.versionId ?? version.current);
                if (!result.persistentSelection)
                    throw new Error("所选边面没有可用的持久引用");
                definition = { selection: result.persistentSelection, sourceVersionId: selection.versionId ?? version.current };
            }
            if (!alive.current || !current.current.validRevision)
                return;
            const key = selectionKey(selection);
            setPicks(previous => previous.some(p => selectionKey(p.visual) === key) ? previous.filter(p => selectionKey(p.visual) !== key) : [...previous, { definition, visual: selection }]);
            setError(undefined);
        }).catch(cause => { if (alive.current)
            setError(String(cause)); }).finally(() => { if (alive.current)
            setBinding(n => n - 1); });
    };
    const bindNeutralPlane = (selection: SelectionItem) => {
        if (selection.kind !== "face" || !selection.geometryKey) return;
        setBinding(n => n+1);
        queue.current = queue.current.then(async () => {
            if (!alive.current || !current.current.validRevision || current.current.committing) return;
            let definition: NonNullable<Feature["selections"]>[number];
            if (feature.id) {
                const result = await api.getFeatureInput(view.document.id, { versionId:version.current, featureId:feature.id, bodyId:selection.bodyId, geometryKey:selection.geometryKey, kind:"FACE", localId:selection.topologyId });
                if (!result.selection || result.selection.selection.creationEvidence.geometryType !== "PLANE") throw new Error("中性平面必须是平面");
                definition = result.selection;
            } else {
                const result = await api.getTopologyProperties(view.document.id, selection.geometryKey!, "FACE", selection.topologyId, version.current);
                if (result.geometryType !== "PLANE" || !result.persistentSelection) throw new Error("请选择可绑定的实体平面");
                definition = { selection:result.persistentSelection, sourceVersionId:version.current };
            }
            if (!alive.current || !current.current.validRevision) return;
            edit({neutralPlaneId:undefined, neutralPlane:definition}); setNeutralVisual(selection); setRole(pickRole);
        }).catch(cause => { if (alive.current) setError(String(cause)); }).finally(()=>{if(alive.current)setBinding(n=>n-1);});
    };
    const pick = (selection: SelectionItem, chosenRole: FeaturePickRole = current.current.role) => {
        if ((selection.ownerDocumentId ?? selection.documentId) !== view.document.id || (selection.occurrencePath ?? "") !== (occurrencePath ?? "") || selection.versionId && selection.versionId !== version.current || committing || !validRevision)
            return;
        if (chosenRole === "edge" || chosenRole === "face") {
            bindPick(selection);
            return;
        }
        if (chosenRole === "profile") {
            const sketch = selectedSketch(view, selection, upstream);
            if (!sketch)
                return;
            if (loft)
                setDraft(previous => ({ ...previous, sections: pickLoftSection(previous.sections ?? [], sketch) }));
            else {
                edit({ profile: sketch.id, profileMemberSlot:sketch.type==="SKETCH_PATTERN"?sketch.profileMemberSlot:undefined });
                if (feature.type === "REVOLVE" && !draft.axisEntityId)
                    setRole("axis");
            }
        }
        else if (chosenRole === "axis") {
            const axis = selectedAxis(view, selection, upstream);
            if (!axis) {
                setError("请选择草图中的直线或基准轴");
                return;
            }
            edit({ axisEntityId: axis });
            setAxisVisual(selection);
        }
        else if (chosenRole === "plane" && selection.kind === "plane") {
            const plane = view.part?.datumPlanes.find(p => p.id === selection.datumPlane?.id || p.id === selection.entityId || p.id === selection.id);
            if (plane) {
                edit({ neutralPlaneId: plane.id, neutralPlane: undefined });
                setNeutralVisual(selection);
                setRole(pickRole);
            }
        }
        else if (chosenRole === "plane" && selection.kind === "face") { bindNeutralPlane(selection); }
        else if (chosenRole === "seam" && selection.kind === "visual" && seamIndex !== undefined) {
            edit({ sections: draft.sections?.map((s, i) => i === seamIndex && (s.sketchId === selection.featureId || s.sketchId===selection.patternId&&s.memberSlot===selection.patternMemberSlot) ? { ...s, seamEntityId: selection.entityId } : s) });
            setRole("profile");
        }
        else if (chosenRole === "body" && selection.bodyId) {
            edit({ bodyId: selection.bodyId });
            setRole("profile");
        }
    };
    const seedOnce = useRef(false);
    useEffect(() => {
        if (seedOnce.current || feature.id)
            return;
        seedOnce.current = true;
        if (modifier)
            seed.forEach(s => pick(s, pickRole));
        else if (loft)
            seed.forEach(s => pick(s, "profile"));
        else {
            const sketch = seed.map(s => selectedSketch(view, s, upstream)).find(Boolean);
            if (sketch)
                edit({ profile: sketch.id, profileMemberSlot:sketch.type==="SKETCH_PATTERN"?sketch.profileMemberSlot:undefined });
            const axis = seed.find(s => selectedAxis(view, s, upstream));
            if (axis)
                pick(axis, "axis");
            else if (sketch && feature.type === "REVOLVE")
                setRole("axis");
        }
    }, []);
    const pickRef = useRef(pick);
    pickRef.current = pick;
    const axisParts = draft.axisEntityId?.split(":");
    const restoredAxis: SelectionItem | undefined = axisParts?.[0] === "SKETCH_LINE" ? {
        kind: "visual", visualType: "CURVE", id: `${occurrencePath || "root"}:${axisParts[1]}:${axisParts[2]}`, featureId: axisParts[1], entityId: axisParts[2], bodyId: upstream.find(f => f.id === axisParts[1])?.bodyId, documentId: view.document.id, versionId: version.current, occurrencePath
    } : undefined;
    const highlights: SelectionItem[] = modifier ? picks.map(p => p.visual) : loft ? (draft.sections ?? []).map(s => sketchPick(view, s.sketchId, occurrencePath, s.memberSlot)) : draft.profile ? [sketchPick(view, draft.profile, occurrencePath, draft.profileMemberSlot)] : [];
    if (axisVisual ?? restoredAxis)
        highlights.push((axisVisual ?? restoredAxis)!);
    if (neutralVisual) highlights.push(neutralVisual);
    const contextSelections = feature.type === "REVOLVE" && role === "axis" ? highlights.filter(s => s.kind === "sketch") : [];
    const selections = contextSelections.length ? highlights.filter(s => s.kind !== "sketch") : highlights;
    const highlightsToken = JSON.stringify([selections, contextSelections]);
    const seamSection=seamIndex!==undefined?draft.sections?.[seamIndex]:undefined;
    const sketchIds = role === "seam" && seamSection ? seamSection.memberSlot!==undefined?profileSelectionIDs(view,upstream,seamSection.sketchId,seamSection.memberSlot):[seamSection.sketchId] : profileSelectionIDs(view,upstream);
    const sketchIdsToken = JSON.stringify(sketchIds);
    useEffect(() => {
        callbacks.current.onSelectionSession({ role, documentId: view.document.id, versionId: version.current, occurrencePath, bodyId: modifier ? draft.bodyId : undefined, sketchIds, contextSelections: contextSelections.map(s => ({ ...occurrenceContext, ...s })), selections: selections.map(s => ({ ...occurrenceContext, ...s })), onPick: s => pickRef.current(s) });
    }, [role, highlightsToken, sketchIdsToken, draft.bodyId]);
    let input: Record<string, unknown> | undefined, parameterError: string | undefined;
    const ready = validRevision && !loadingInput && !binding && (loft ? (draft.sections?.length ?? 0) >= 2 : modifier ? picks.length > 0 && !!draft.bodyId && (feature.type !== "DRAFT" || !!draft.neutralPlaneId || !!draft.neutralPlane) : !!draft.profile && (feature.type !== "REVOLVE" || !!draft.axisEntityId));
    try {
        const value = { ...draft, selections: modifier ? picks.map(p => p.definition) : draft.selections }, expressions: Record<string, string> = {};
        const read = (slot: "length" | "length2" | "angle", text: string) => { const parsed = solidParameterEdit(slot, text, original.current[slot], unit); if (parsed.value !== undefined)
            value[slot] = parsed.value; if (parsed.expression !== undefined)
            expressions[slot] = parsed.expression; };
        if (!loft) {
            if (angular)
                read("angle", angle);
            else if (draft.extent !== "THROUGH_ALL") {
                read("length", length);
                if (draft.extent === "TWO_SIDED")
                    read("length2", second);
            }
        }
        const common = { requestId: intent.current, parameterExpressions: expressions };
        if (ready)
            input = feature.id ? { ...common, type: "EDIT_FEATURE", targetId: feature.id, expectedFeatureDigest: digest, feature: value } : modifier ? { ...common, type: "CREATE_MODIFY_FEATURE", feature: value } : loft ? { ...common, type: "CREATE_SOLID_FEATURE", feature: value } : { ...common, type: "CREATE_SOLID_FEATURE", sketchId: value.profile, profileMemberSlot:value.profileMemberSlot, generator: value.type, bodyId: value.bodyId, operation: value.operation, length: value.length ?? 1, length2: value.length2, angle: value.angle ?? 360, extent: value.extent, axisEntityId: value.axisEntityId, reversed: value.reversed };
    }
    catch (cause) {
        parameterError = String(cause);
    }
    const preview = useFeaturePreview(view.document.id, view.document.versionId, input, draft.operation, onPreview);
    const apply = async () => { if (!input || !preview.previewId || preview.pending || committing)
        return; setCommitting(true); try {
        await onApply({ ...input, previewId: preview.previewId });
        onClose();
    }
    catch (cause) {
        setError(String(cause));
        setCommitting(false);
    } };
    const name = (id?: string) => upstream.find(f => f.id === id)?.name ?? "已选择";
    return <CommandDialog id="solid-feature-editor" open title={`${feature.id ? "编辑" : "创建"} ${solidFeatureNames[feature.type] ?? feature.type}`} size="S" onClose={onClose} onConfirm={apply} confirmLoading={committing} confirmDisabled={!input || !preview.previewId || preview.pending || committing}>
  <fieldset disabled={committing} className="instance-pattern-fields feature-input-fields">
   {!modifier && !loft && <FeaturePickField label="轮廓" value={draft.profile ? name(draft.profile) : "请在视图区选择草图"} active={role === "profile"} onActivate={() => setRole("profile")} onClear={draft.profile ? () => edit({ profile: undefined }) : undefined}/>}
   {draft.profile && upstream.find(f=>f.id===draft.profile)?.type==="SKETCH_PATTERN" && <label>草图成员槽位<Input type="number" min={0} step={1} value={draft.profileMemberSlot} onChange={e=>edit({profileMemberSlot:e.target.value===""?undefined:Number(e.target.value)})}/></label>}
   {feature.type === "REVOLVE" && <FeaturePickField label="旋转轴" value={draft.axisEntityId ? "已选择 1 条轴线" : "请在视图区选择直线或轴"} active={role === "axis"} onActivate={() => setRole("axis")} onClear={draft.axisEntityId ? () => { edit({ axisEntityId: undefined }); setAxisVisual(undefined); } : undefined}/>}
   {modifier && <FeaturePickField label={pickRole === "edge" ? "边集" : "面集"} value={loadingInput ? "正在恢复选择…" : `已选择 ${picks.length} ${pickRole === "edge" ? "条边" : "个面"}`} active={role === pickRole} onActivate={() => setRole(pickRole)} onClear={picks.length ? () => setPicks([]) : undefined}/>}
   {feature.type === "DRAFT" && <FeaturePickField label="中性平面" value={draft.neutralPlane ? "已选择实体平面" : view.part?.datumPlanes.find(p => p.id === draft.neutralPlaneId)?.name ?? "选择基准面或实体平面"} active={role === "plane"} onActivate={() => setRole("plane")}/>}
   {loft && <>
    <FeaturePickField label="截面" value={`已选择 ${draft.sections?.length ?? 0} 个草图`} active={role === "profile"} onActivate={() => setRole("profile")} onClear={draft.sections?.length ? () => edit({ sections: [] }) : undefined}/>
    {(draft.sections ?? []).map((section, i) => <div className="feature-section-row" key={`${section.sketchId}/${i}`}>
     <span>{i + 1}. {name(section.sketchId)}</span>
     {upstream.find(f=>f.id===section.sketchId)?.type==="SKETCH_PATTERN"&&<label>成员槽位<Input type="number" min={0} step={1} value={section.memberSlot} onChange={e=>edit({sections:draft.sections?.map((s,j)=>i===j?{...s,memberSlot:e.target.value===""?undefined:Number(e.target.value)}:s)})}/></label>}
     <Button size="small" aria-label={`上移截面 ${i + 1}`} disabled={!i} onClick={() => { const sections = [...draft.sections!]; [sections[i - 1], sections[i]] = [sections[i], sections[i - 1]]; edit({ sections }); }}>↑</Button>
     <Button size="small" aria-label={`移除截面 ${i + 1}`} onClick={() => edit({ sections: draft.sections?.filter((_, j) => j !== i) })}>移除</Button>
     <label>反向<Switch checked={section.reversed} onChange={reversed => edit({ sections: draft.sections?.map((s, j) => i === j ? { ...s, reversed } : s) })}/></label>
     <Button size="small" aria-pressed={role === "seam" && seamIndex === i} onClick={() => { setSeamIndex(i); setRole("seam"); }}>选择闭合起始边</Button>
     {upstream.find(f => f.id === section.sketchId)?.sketch?.entities.some(e => e.kind === "CIRCLE") && <label>闭合点角度（deg）<Input type="number" value={section.seamAngle ?? 0} onChange={e => edit({ sections: draft.sections?.map((s, j) => i === j ? { ...s, seamAngle: Number(e.target.value) } : s) })}/></label>}
    </div>)}
    <label>截面连接<Segmented value={draft.ruled ? "ruled" : "smooth"} options={[{ value: "smooth", label: "平滑" }, { value: "ruled", label: "直纹" }]} onChange={v => edit({ ruled: v === "ruled" })}/></label>
   </>}
   {!modifier && <>
    <label>材料<Segmented value={draft.operation} options={[...(!feature.id ? [{ value: "NEW_BODY", label: "新实体" }] : []), { value: "ADD", label: "添加" }, { value: "REMOVE", label: "切除" }, { value: "INTERSECT", label: "交集" }]} onChange={operation => edit({ operation: operation as Feature["operation"], extent: operation !== "REMOVE" && draft.extent === "THROUGH_ALL" ? "FINITE" : draft.extent })}/></label>
    {draft.operation !== "NEW_BODY" && <FeaturePickField label="目标实体" value={view.part?.bodies.find(b => b.id === draft.bodyId)?.name ?? "请在视图区选择实体"} disabled={!!feature.id} active={role === "body"} onActivate={() => setRole("body")}/>}
   </>}
   {!loft && (angular ? <label>角度（deg）<Input value={angle} disabled={committing} onChange={e => setAngle(e.target.value)}/></label> : <>
    {!modifier && <label>范围<Segmented value={draft.extent ?? "FINITE"} options={[{ value: "FINITE", label: "长度" }, { value: "TWO_SIDED", label: "双侧" }, { value: "SYMMETRIC", label: "对称" }, ...(draft.operation === "REMOVE" ? [{ value: "THROUGH_ALL", label: "贯穿" }] : [])]} onChange={extent => edit({ extent: extent as Feature["extent"] })}/></label>}
    {draft.extent !== "THROUGH_ALL" && <label>{feature.type === "FILLET" ? "半径" : feature.type === "CHAMFER" ? "距离" : feature.type === "SHELL" ? "厚度" : "长度"}（{unit}）<Input value={length} disabled={committing} onChange={e => setLength(e.target.value)}/></label>}
    {draft.extent === "TWO_SIDED" && <label>反向长度（{unit}）<Input value={second} disabled={committing} onChange={e => setSecond(e.target.value)}/></label>}
   </>)}
   {!loft && <label>反向<Switch checked={draft.reversed} disabled={committing} onChange={reversed => edit({ reversed })}/></label>}
   {feature.id && <label>抑制<Switch checked={draft.suppressed} disabled={committing} onChange={suppressed => edit({ suppressed })}/></label>}
   <small role="status">{!validRevision ? "模型已改变，请关闭并重新打开此命令" : binding || loadingInput ? "正在恢复精确选择…" : preview.pending ? "正在预览…" : "在视图区点击添加选择，再次点击取消；参数修改后自动预览。"}</small>
   {(error ?? parameterError ?? preview.error) && <Alert type="error" title="特征求值失败" description={error ?? parameterError ?? preview.error}/>}
   {preview.error && <Button onClick={preview.retry}>重新预览</Button>}
  </fieldset>
 </CommandDialog>;
}
