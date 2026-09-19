import { ControlOutlined } from "@ant-design/icons";
import { Button, Divider, Drawer, Segmented, Select, Typography } from "antd";
import { useState } from "react";
import { CaptureSettingsPanel } from "../../cad/overlay/capture-settings-button";
import { effectiveLengthUnit, useUIPreferences, type DisplayLengthUnit } from "../../state/ui-preferences";

const unitOptions = [
  { value: "mm", label: "毫米 (mm)" },
  { value: "cm", label: "厘米 (cm)" },
  { value: "m", label: "米 (m)" },
  { value: "in", label: "英寸 (in)" },
] satisfies Array<{ value: DisplayLengthUnit; label: string }>;

export function UserPreferencesCenter({ activeDocumentID }: { activeDocumentID?: string }) {
  const [open, setOpen] = useState(false);
  const preferences = useUIPreferences();
  const effectiveUnit = effectiveLengthUnit(preferences.displayLengthUnit, preferences.documentLengthUnits, activeDocumentID);
  const documentUnit = activeDocumentID ? preferences.documentLengthUnits[activeDocumentID] : undefined;

  return <>
    <Button ghost aria-label="用户偏好" title="用户偏好" icon={<ControlOutlined />} onClick={() => setOpen(true)} />
    <Drawer className="user-preferences-drawer" title="用户偏好" placement="right" width={410}
      open={open} onClose={() => setOpen(false)}>
      <section className="preference-section">
        <Typography.Title level={5}>鼠标操作</Typography.Title>
        <Typography.Paragraph type="secondary">选择工作台中的平移、旋转和缩放手势。</Typography.Paragraph>
        <Segmented block value={preferences.navigationProfile} onChange={(value) => preferences.setNavigationProfile(value as "default" | "catia")}
          options={[{ value: "default", label: "occccad" }, { value: "catia", label: "CATIA V5" }]} />
      </section>
      <Divider />
      <section className="preference-section">
        <Typography.Title level={5}>单位</Typography.Title>
        <label className="preference-field"><span>用户默认长度单位</span><Select value={preferences.displayLengthUnit}
          options={unitOptions} onChange={preferences.setDisplayLengthUnit} /></label>
        {activeDocumentID && <label className="preference-field"><span>当前文档长度单位</span><Select
          value={documentUnit ?? "INHERIT"}
          options={[{ value: "INHERIT", label: `继承用户设置 (${preferences.displayLengthUnit})` }, ...unitOptions]}
          onChange={(value) => preferences.setDocumentLengthUnit(activeDocumentID,
            value === "INHERIT" ? undefined : value as DisplayLengthUnit)} /></label>}
        <Typography.Paragraph type="secondary" className="preference-note">
          当前有效单位：{effectiveUnit}。单位偏好控制显示和新建数值输入；内核几何仍使用规范毫米值，已有参数表达式不会被改写。
        </Typography.Paragraph>
      </section>
      <Divider />
      <section className="preference-section">
        <CaptureSettingsPanel settings={preferences.captureSettings}
          onEnabledChange={preferences.setCaptureEnabled}
          onSelectionToggle={preferences.toggleSelectionCapture}
          onSketchToggle={preferences.toggleSketchSnap}
          onAll={preferences.captureAll} onPointsOnly={preferences.capturePointsOnly} />
      </section>
    </Drawer>
  </>;
}
