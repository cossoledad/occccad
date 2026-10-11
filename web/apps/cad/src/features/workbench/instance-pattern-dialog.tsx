import {CadNumberInput as InputNumber} from "../../cad/overlay/cad-number-input";
import { Alert, Button, Segmented, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import {owningInstanceSelection} from "../../cad/assembly/instance-selection";
import type {FeatureSelectionSession} from "../../cad/interaction/feature-selection";
import type {DocumentView,SelectionItem } from "../../types";
import { instancePatternOffsets, type InstancePatternPreview, type PatternAxis } from "./instance-pattern";

/** CATIA's Define Multi-Instantiation flow: select a component, define new copies, preview, apply. */
export function InstancePatternDialog({ view, seed, onSelectionSession, parentOccurrencePath, parentRotation, busy, onClose, onApply, onPreview }: {
  view:DocumentView;seed:SelectionItem|null;onSelectionSession:(session?:FeatureSelectionSession)=>void; parentOccurrencePath?: string; parentRotation?: [number, number, number, number];
  busy: boolean; onClose: () => void; onApply: (input: InstancePatternPreview) => Promise<unknown>;
  onPreview: (input?: InstancePatternPreview) => void;
}) {
  const [source,setSource]=useState(()=>seed?owningInstanceSelection(view,seed,parentOccurrencePath):null);
  const sourceInstance=view.product?.instances.find(v=>v.id===source?.instanceId);
  useEffect(()=>{
    onSelectionSession({role:"instance",documentId:view.document.id,versionId:view.document.versionId,
      project:s=>owningInstanceSelection(view,s,parentOccurrencePath),accept:s=>Boolean(s.instanceId),selections:source?[source]:[],onPick:setSource});
    return()=>onSelectionSession();
  },[view.document.id,view.document.versionId,parentOccurrencePath,source,onSelectionSession]);
  const [axis, setAxis] = useState<PatternAxis>("X");
  const [newCount, setNewCount] = useState<number | null>(1);
  const [spacing, setSpacing] = useState<number | null>(10);
  const [reversed, setReversed] = useState(false);
  const [error, setError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const input: InstancePatternPreview | undefined = sourceInstance && newCount !== null && spacing !== null
    ? { sourceInstanceId: sourceInstance.id, axis, count: newCount + 1, spacing, reversed, parentOccurrencePath, parentRotation }
    : undefined;
  const valid = input && instancePatternOffsets(input).length === newCount;
  useEffect(() => {
    onPreview(valid ? input : undefined);
    return () => onPreview(undefined);
  }, [sourceInstance?.id, axis, newCount, spacing, reversed, parentOccurrencePath, parentRotation, onPreview]);
  const apply = async (close: boolean) => {
    if (!input || !valid || busy || submittingRef.current) return;
    submittingRef.current = true; setSubmitting(true); setError(undefined);
    try { await onApply(input); if (close) onClose(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)); }
    finally { submittingRef.current = false; setSubmitting(false); }
  };
  return <CommandDialog id="product-pattern" open title="多实例化" size="S"
    onClose={onClose} onConfirm={() => apply(true)} confirmText="确定" confirmLoading={busy || submitting} confirmDisabled={!valid}>
    <div className="instance-pattern-fields">
      <label>源组件<Button aria-label="源组件">{sourceInstance?.name??"选择组件"}</Button><Button size="small" onClick={()=>setSource(null)}>清除</Button></label>
      <label>新增实例数量<InputNumber min={1} max={127} precision={0} value={newCount} disabled={busy} onChange={setNewCount} /></label>
      <label>相邻间距（mm）<InputNumber min={0.001} max={1e6} value={spacing} disabled={busy} onChange={setSpacing} /></label>
      <label>方向轴<Segmented value={axis} options={["X", "Y", "Z"]} disabled={busy}
        onChange={(value) => setAxis(value as PatternAxis)} /></label>
      <label>反向<Switch checked={reversed} disabled={busy} onChange={setReversed} /></label>
      {error && <Alert type="error" showIcon title="创建失败" description={error} />}
      <Button disabled={!valid || busy || submitting} onClick={() => void apply(false)}>应用并继续</Button>
    </div>
  </CommandDialog>;
}
