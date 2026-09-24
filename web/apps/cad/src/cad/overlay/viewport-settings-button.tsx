import { Button, Checkbox, Popover, Radio, Tabs } from "antd";
import { SettingOutlined } from "@ant-design/icons";
import { useEffect, useRef, useState } from "react";
import type { ReferenceVisibility, SolidDisplaySettings } from "../rendering/display-settings";
import { CaptureSettingsPanel } from "./capture-settings-panel";
import type { ComponentProps } from "react";
import { useUIHelp } from "../help/ui-help-context";

export function ViewportSettingsButton({ references, display, onReferences, onDisplay, capture }: {
  references: ReferenceVisibility; display: SolidDisplaySettings;
  onReferences: (value: ReferenceVisibility) => void;
  onDisplay: (value: SolidDisplaySettings) => void;
  capture: ComponentProps<typeof CaptureSettingsPanel>;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLSpanElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const help = useUIHelp();
  useEffect(() => {
    if (!open) return;
    // Viewport pointer handlers stop bubbling. Observe the capture phase instead.
    const outside = (event: PointerEvent) => {
      const path = event.composedPath();
      if (!path.includes(trigger.current!) && !path.includes(content.current!)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    document.addEventListener("pointerdown", outside, true);
    document.addEventListener("keydown", escape, true);
    return () => {
      document.removeEventListener("pointerdown", outside, true);
      document.removeEventListener("keydown", escape, true);
    };
  }, [open]);
  const displayPanel = <div className="cad-capture-panel">
    <section><strong>基准元素</strong><div className="cad-capture-grid">
      {([["planes", "基准面"], ["axes", "基准轴"], ["coordinateSystems", "坐标系"]] as const).map(([key, label]) =>
        <Checkbox key={key} checked={references[key]} onChange={event =>
          onReferences({ ...references, [key]: event.target.checked })}>{label}</Checkbox>)}
    </div></section>
    <section><strong>实体</strong><Radio.Group value={display.mode}
      onChange={event => onDisplay({ ...display, mode: event.target.value })}
      options={[{ value: "shaded", label: "正常显示" }, { value: "wireframe", label: "线框显示" }]} />
      <div className="cad-capture-grid">
        <Checkbox disabled={display.mode === "wireframe"} checked={display.mode === "wireframe" || display.edges} onChange={event => onDisplay({ ...display, edges: event.target.checked })}>显示边线</Checkbox>
        <Checkbox checked={display.vertices} onChange={event => onDisplay({ ...display, vertices: event.target.checked })}>显示交点</Checkbox>
      </div>
      <small>边线开关控制正常显示时的轮廓；线框显示始终保留边线。</small>
    </section>
  </div>;
  return <Popover open={open} trigger="click" placement="topRight" onOpenChange={next => {
    if (next && help.active) {
      help.explain({ toolbarName: "", commandName: "视区设置", helpText: "设置显示方式、基准元素显隐和捕获类型。" });
      return;
    }
    setOpen(next);
  }} content={<div ref={content} style={{ maxHeight: "65vh", overflowY: "auto" }}>
    <Tabs size="small" items={[{ key: "display", label: "显示", children: displayPanel },
      { key: "capture", label: "捕获", children: <CaptureSettingsPanel {...capture} /> }]} />
  </div>}>
    <span ref={trigger}><Button className="cad-tool-button" aria-label="视区设置" title="显示与捕获设置" icon={<SettingOutlined />} /></span>
  </Popover>;
}
