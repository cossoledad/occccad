import { Alert, Button, Card, Input, Select, Space, Typography } from "antd";
import type { SketchCommandAction, SketchCommandState } from "../tool/sketch-command-session";

export function SketchCommandPanel({ state, prompt, onAction }: {
  state?: SketchCommandState; prompt: string; onAction: (action: SketchCommandAction) => void;
}) {
  if (!prompt && !state) return null;
  const pending = state?.phase === "committing", unknown = state?.phase === "unknown";
  return <Card size="small" className="sketch-command-panel" data-cad-command-panel="true"
    style={{ position: "absolute", top: 12, right: 12, width: 330, maxHeight: "calc(100% - 24px)", overflow: "auto", zIndex: 10 }}
    title={state?.operation ?? "草图工具"}
    onKeyDownCapture={event => {
      if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229 || event.repeat) return;
      if(event.key === "Enter" && (event.target as HTMLElement).closest('[role="combobox"],textarea')) return;
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); onAction({ type: "cancel" }); }
      else if (event.key === "Enter" && state && state.canConfirm && !pending && !unknown) {
        event.preventDefault(); event.stopPropagation(); onAction({ type: "confirm" });
      }
    }}>
    <Space orientation="vertical" style={{ width: "100%" }}>
      {state && <Typography.Text strong>{state.role} · 已选 {state.selectedIds.length} 个对象</Typography.Text>}
      <Typography.Text>{prompt}</Typography.Text>
      {state?.error && <Alert type="error" title={state.error} />}
      {state?.fields.map((field, index) => <label key={index} style={{ display: "block" }}>
        <Typography.Text>{field.label}</Typography.Text>
        <Input value={field.value} placeholder={field.placeholder} disabled={pending || unknown}
          onChange={event => onAction({ type: "field", index, value: event.target.value })} />
      </label>)}
      {state?.options.map(option => <label key={option.name} style={{ display: "block" }}>
        <Typography.Text>{option.label}</Typography.Text>
        <Select style={{ width: "100%" }} value={option.value} options={option.choices} disabled={pending || unknown}
          onChange={value => onAction({ type: "option", name: option.name, value })} />
      </label>)}
      {state?.preview === "approximate" && <Typography.Text type="secondary">预览为近似候选，提交后权威验证</Typography.Text>}
      {state?.preview === "authoritative" && <Typography.Text type="secondary">权威候选已验证；确认后提交</Typography.Text>}
      {state?.preview === "pending" && <Typography.Text type="secondary">正在计算权威候选</Typography.Text>}
      {state?.preview === "unavailable" && <Button onClick={()=>onAction({type:"retry"})}>重新预览</Button>}
      {state && <Space wrap>
        <Button type="primary" loading={pending} disabled={!state.canConfirm || unknown}
          onClick={() => onAction({ type: "confirm" })}>{state.phase === "selection" ? "下一步" : "确认"}</Button>
        {state.phase !== "selection" && <Button disabled={pending || unknown} onClick={() => onAction({ type: "back" })}>上一步</Button>}
        {unknown && <Button onClick={() => onAction({ type: "retry" })}>确认原请求结果</Button>}
        {["sketch.edit.trim", "sketch.edit.quick_trim", "sketch.edit.split", "sketch.edit.extend", "sketch.edit.complement", "sketch.edit.close"].includes(state.toolId) &&
          <Button disabled={pending || unknown} onClick={() => onAction({ type: "release" })}>查看/切换关系解除</Button>}
        {["sketch.edit.fillet", "sketch.edit.chamfer"].includes(state.toolId) &&
          <Button disabled={pending || unknown} onClick={() => onAction({ type: "add-batch" })}>加入角点批次</Button>}
        <Button onClick={() => onAction({ type: "cancel" })}>{pending ? "退出等待界面" : "取消（Esc）"}</Button>
      </Space>}
    </Space>
  </Card>;
}
