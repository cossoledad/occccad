import { Alert, Input, InputNumber, Segmented } from "antd";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import type { FeatureSelectionSession } from "../../cad/interaction/feature-selection";
import type { DatumPreview } from "../../cad/rendering/datum-reference";
import type { DocumentView, SelectionItem, Vec3 } from "../../types";
import { displayLengthToMillimeters, millimetersToDisplayLength } from "../../state/ui-preferences";
import { datumAxisDefinition, finiteVector, planarFaceFrame, planeFrame, unitVector } from "./datum-definition";
import { FeaturePickField } from "./feature-pick-field";

export function DatumEditor({ kind, view, seed, unit, occurrencePath, placement, onClose, onApply, onPreview, onSelectionSession }: {
  kind: "plane" | "axis"; view: DocumentView; seed: SelectionItem[]; unit: "mm" | "cm" | "m" | "in";
  occurrencePath?: string; placement?: Pick<DatumPreview, "translation" | "rotation">;
  onClose: () => void; onApply: (input: Record<string, unknown>) => Promise<unknown>;
  onPreview: (preview?: DatumPreview) => void; onSelectionSession: (session?: FeatureSelectionSession) => void;
}) {
  const [mode, setMode] = useState("REFERENCE"), [name, setName] = useState(kind === "plane" ? "Plane" : "Axis");
  const [origin, setOrigin] = useState<Vec3>([0, 0, 0]), [direction, setDirection] = useState<Vec3>([0, 0, 1]);
  const [offset, setOffset] = useState<number | null>(0), [reference, setReference] = useState<SelectionItem>();
  const [referenceName, setReferenceName] = useState<string>(), [picking, setPicking] = useState(true), [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>(), [loading, setLoading] = useState(false);
  const preferredU = useRef<Vec3 | undefined>(undefined), alive = useRef(true), generation = useRef(0), base = useRef(view.document.versionId);
  const callbacks = useRef({ onPreview, onSelectionSession }); callbacks.current = { onPreview, onSelectionSession };
  const validRevision = base.current === view.document.versionId;
  const pick = async (selection: SelectionItem) => {
    if ((selection.ownerDocumentId ?? selection.documentId) !== view.document.id || (selection.occurrencePath ?? "") !== (occurrencePath ?? "") || selection.versionId && selection.versionId !== base.current) return;
    const request = ++generation.current;
    setLoading(true); setError(undefined);
    try {
      let center: Vec3, vector: Vec3, label: string, referenceU: Vec3 | undefined;
      if (kind === "plane") {
        const plane = selection.kind === "plane" ? (view.datumPlanes ?? view.part?.datumPlanes)?.find(p => p.id === selection.entityId) : undefined;
        const frame = plane ?? (selection.kind === "face" && selection.geometryKey
          ? planarFaceFrame(await api.getTopologyProperties(view.document.id, selection.geometryKey, "FACE", selection.topologyId, base.current)) : undefined);
        if (!frame) throw new Error("请选择标准面、基准面或实体上的平面");
        center = frame.origin; vector = frame.normal; referenceU = frame.uDirection;
        label = plane?.name ?? "实体平面中心";
      } else {
        const axis = datumAxisDefinition(view, selection);
        if (!axis) throw new Error("请选择标准轴或基准轴");
        center = axis.origin; vector = axis.direction; label = axis.name;
      }
      if (!alive.current || request !== generation.current) return;
      preferredU.current = referenceU;
      setOrigin(center.map(v => millimetersToDisplayLength(v, unit)) as Vec3); setDirection(vector);
      setOffset(0); setReference(selection); setReferenceName(label); setPicking(false);
    } catch (cause) { if (alive.current && request === generation.current) setError(String(cause)); }
    finally { if (alive.current && request === generation.current) setLoading(false); }
  };
  const pickRef = useRef(pick); pickRef.current = pick;
  useEffect(() => {
    alive.current = true;
    const initial = seed.find(s => kind === "axis" ? s.kind === "axis" : s.kind === "plane" || s.kind === "face");
    if (initial) void pickRef.current(initial);
    else { setMode("CUSTOM"); setPicking(false); }
    return () => { alive.current = false; generation.current++; callbacks.current.onPreview(); callbacks.current.onSelectionSession(); };
  }, []);
  useEffect(() => {
    callbacks.current.onSelectionSession(mode === "REFERENCE" && picking && validRevision ? {
      role: kind, documentId: view.document.id, versionId: base.current, occurrencePath,
      selections: reference ? [reference] : [], onPick: s => { void pickRef.current(s); },
    } : undefined);
  }, [mode, picking, reference, validRevision]);
  let definition: { origin: Vec3; direction: Vec3; normal?: Vec3; uDirection?: Vec3 } | undefined, parameterError: string | undefined;
  try {
    if (!validRevision) throw new Error("模型已改变，请关闭并重新打开命令");
    if (loading || mode === "REFERENCE" && !reference) throw new Error("请选择复制参考");
    if (!finiteVector(origin) || offset === null || !Number.isFinite(offset)) throw new Error("位置需要有限数值");
    const n = unitVector(direction), displacement = displayLengthToMillimeters(offset, unit);
    const center = origin.map((v, i) => displayLengthToMillimeters(v, unit) + n[i] * displacement) as Vec3;
    definition = kind === "plane" ? { ...planeFrame(center, n, preferredU.current), direction: n } : { origin: center, direction: n };
  } catch (cause) { parameterError = String(cause); }
  const token = JSON.stringify(definition);
  useEffect(() => {
    callbacks.current.onPreview(definition ? { ...placement, ...(kind === "plane" ? { plane: { id: "datum-preview", name, plane: "CUSTOM", origin: definition.origin, normal: definition.normal!, uDirection: definition.uDirection!, size: 180 } } : { axis: { id: "datum-preview", name, origin: definition.origin, direction: definition.direction } }) } : undefined);
  }, [token, name, placement?.translation, placement?.rotation]);
  const vectorFields = (label: string, values: Vec3, change: (v: Vec3) => void) => <fieldset className="datum-vector-fields"><legend>{label}</legend>
    {(["X", "Y", "Z"] as const).map((axis, i) => <label key={axis}>{axis}<InputNumber aria-label={`${label} ${axis}`} value={Number.isFinite(values[i]) ? values[i] : null} onChange={v => change(values.map((x, index) => index === i ? v ?? NaN : x) as Vec3)} /></label>)}</fieldset>;
  return <CommandDialog id={`datum-${kind}`} open size="S" title={kind === "plane" ? "创建基准面" : "创建基准轴"} onClose={onClose}
    confirmLoading={busy} confirmDisabled={!definition || !name.trim() || loading} onConfirm={async () => {
      if (!definition || busy) return; setBusy(true);
      try { await onApply({ type: kind === "plane" ? "CREATE_DATUM_PLANE" : "CREATE_DATUM_AXIS", name: name.trim(), ...definition }); onClose(); }
      catch (cause) { setError(String(cause)); setBusy(false); }
    }}>
    <label>名称<Input value={name} onChange={e => setName(e.target.value)} /></label>
    <Segmented block options={[{ label: "自由设置", value: "CUSTOM" }, { label: "按参考复制", value: "REFERENCE" }]} value={mode} onChange={v => { setMode(v); setPicking(v === "REFERENCE"); }} />
    {mode === "REFERENCE" && <FeaturePickField label="复制参考" value={referenceName ?? (kind === "plane" ? "选择基准面或实体平面" : "选择标准轴或基准轴")} active={picking} onActivate={() => setPicking(true)} />}
    {vectorFields(`${kind === "plane" ? "中心" : "原点"}（${unit}）`, origin, setOrigin)}
    {vectorFields(kind === "plane" ? "法向" : "方向", direction, setDirection)}
    <label>{kind === "plane" ? "沿法向偏置" : "沿轴位移"}（{unit}）<InputNumber value={offset} onChange={setOffset} /></label>
    <small role="status">{loading ? "正在读取参考几何…" : "位置和方向修改后立即预览；复制使用参考的当前几何。"}</small>
    {(error ?? parameterError) && <Alert type={error ? "error" : "info"} title={error ?? parameterError} />}
  </CommandDialog>;
}
