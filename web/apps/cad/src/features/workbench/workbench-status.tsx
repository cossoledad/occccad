import { DisplaySettingsButton } from "../../cad/overlay/display-settings-button";
import { LoadingOutlined } from "@ant-design/icons";
import { CaptureSettingsButton } from "../../cad/overlay/capture-settings-button";
import { useUIPreferences } from "../../state/ui-preferences";

export function WorkbenchStatus({ busy, canEdit, selectionCount, toolName, lengthUnit, continuous }: {
  busy: boolean; canEdit: boolean; selectionCount: number; toolName: string; lengthUnit: string; continuous: boolean;
}) {
  const preferences = useUIPreferences();
  return <footer className="workbench-status" aria-label="工作台状态">
    <span className="workbench-operation" role="status">{busy ? <><LoadingOutlined /> 正在提交…</> : <><i />{canEdit ? "可编辑" : "只读"}</>}</span>
    <span className="workbench-current-tool">{toolName}{continuous ? " · 连续执行" : ""}</span>
    <span className="workbench-selection-count">{selectionCount ? `已选择 ${selectionCount} 项` : "未选择对象"}</span>
    <span className="workbench-status-spacer" />
    <DisplaySettingsButton references={preferences.referenceVisibility} mode={preferences.renderMode}
      onReferences={preferences.setReferenceVisibility} onMode={preferences.setRenderMode} />
    <CaptureSettingsButton settings={preferences.captureSettings} onEnabledChange={preferences.setCaptureEnabled}
      onSelectionToggle={preferences.toggleSelectionCapture} onSketchToggle={preferences.toggleSketchSnap}
      onAll={preferences.captureAll} onPointsOnly={preferences.capturePointsOnly} />
    <span>长度 · {lengthUnit}</span>
    <span className="workbench-navigation-label">{preferences.navigationProfile === "catia" ? "3DEXPERIENCE CATIA" : preferences.navigationProfile === "solidworks" ? "SOLIDWORKS" : "默认"} 导航</span>
  </footer>;
}
