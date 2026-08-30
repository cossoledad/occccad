import { Button, Checkbox, Popover, Switch } from "antd";
import type { CaptureSettings, SelectionCaptureKind, SketchSnapCaptureKind } from "../interaction/capture-settings";
import { CadIcon } from "./cad-icons";
import { useUIHelp } from "../help/ui-help-context";
import { useState } from "react";

const selectionLabels: Record<SelectionCaptureKind, string> = {
  POINT: "点 / 顶点", CURVE: "曲线 / 边", SURFACE: "曲面 / 面", BODY: "实体 / 特征",
  SKETCH: "草图", CONSTRAINT: "草图约束", DATUM_PLANE: "基准面", DATUM_AXIS: "基准轴 / 坐标系", INSTANCE: "装配实例",
};
const sketchLabels: Record<SketchSnapCaptureKind, string> = {
  GRID: "网格点", ORIGIN: "草图原点", POINT: "独立点", ENDPOINT: "端点", CENTER: "圆心 / 圆弧中心",
  MIDPOINT: "中点", CURVE: "曲线投影",
};

export function CaptureSettingsButton({ settings, onEnabledChange, onSelectionToggle, onSketchToggle, onAll, onPointsOnly }: {
  settings: CaptureSettings;
  onEnabledChange: (enabled: boolean) => void;
  onSelectionToggle: (kind: SelectionCaptureKind) => void;
  onSketchToggle: (kind: SketchSnapCaptureKind) => void;
  onAll: () => void;
  onPointsOnly: () => void;
}) {
	const uiHelp = useUIHelp();
  const [open, setOpen] = useState(false);
  const content = <div className="cad-capture-panel">
    <div className="cad-capture-heading"><strong>捕获设置</strong><Switch size="small" checked={settings.enabled}
      onChange={onEnabledChange} aria-label="启用捕获" /></div>
    <div className="cad-capture-presets"><Button size="small" onClick={onAll}>全部</Button>
      <Button size="small" onClick={onPointsOnly}>仅点</Button></div>
    <section><strong>三维选择过滤</strong><div className="cad-capture-grid">
      {Object.entries(selectionLabels).map(([kind, label]) => <Checkbox key={kind}
        checked={settings.selection.includes(kind as SelectionCaptureKind)} disabled={!settings.enabled}
        onChange={() => onSelectionToggle(kind as SelectionCaptureKind)}>{label}</Checkbox>)}
    </div></section>
    <section><strong>草图吸附</strong><div className="cad-capture-grid">
      {Object.entries(sketchLabels).map(([kind, label]) => <Checkbox key={kind}
        checked={settings.sketch.includes(kind as SketchSnapCaptureKind)} disabled={!settings.enabled}
        onChange={() => onSketchToggle(kind as SketchSnapCaptureKind)}>{label}</Checkbox>)}
    </div></section>
    <small>过滤器只影响下一次捕获；已有选择保持不变。</small>
  </div>;
  const activeCount = settings.selection.length + settings.sketch.length;
  return <Popover open={open} content={content} trigger="click" placement="bottomLeft" onOpenChange={(nextOpen) => {
	if (nextOpen && uiHelp.active) {
      setOpen(false);
      uiHelp.explain({ toolbarName: "", commandName: "捕捉", helpText: "设置三维选择过滤和草图吸附类型。" });
      return;
    }
    setOpen(nextOpen);
  }}>
    <Button className={`cad-tool-button cad-capture-button ${settings.enabled ? "active" : ""}`}
      type={settings.enabled ? "primary" : "default"} icon={<CadIcon name="capture" />}
      aria-label="捕获设置" aria-pressed={settings.enabled}
	  data-active-count={activeCount} />
  </Popover>;
}
