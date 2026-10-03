/** Shared product palette. CSS, Ant Design and WebGL consume these semantic roles. */
export const palette = {
  primary: "#3666db", primaryHover: "#2855c5", primarySoft: "#eaf0ff", primaryBorder: "#b8c9f4",
  chrome: "#172333", chromeRaised: "#24364b", chromeText: "#e4edf8",
  canvas: "#f1f4f8", surface: "#ffffff", subtle: "#f7f9fc", border: "#dce3ed", ribbonDivider: "#8a9fbe",
  text: "#24334a", textSecondary: "#5f6f82", textDisabled: "#9aa7b6", muted: "#5f6f82", success: "#25846a", warning: "#a56a19", danger: "#c44658",
  viewportTop: "#293744", viewportBottom: "#526372", solid: "#98a8b8", productSolid: "#94a6b8",
  edge: "#172a3e", vertex: "#edf3fa", sketch: "#f0f5fc", construction: "#8facca", external: "#80cfe6",
  selected: "#f4ba62", hover: "#68d7e6", preview: "#aab9ff", snap: "#ffe099",
  sketchGrid: "#8dc7de", previewNew: "#8bb8e8", previewAdd: "#70c9ab", previewRemove: "#ebaa84",
  previewIntersect: "#b1a1df", previewReference: "#e1b896",
  dimensionText: "#f4e6bc", dimensionReference: "#cfdbe7", viewportText: "#eef3f8",
  solved: "#7edbb1", invalid: "#ff8490", redundant: "#d6a0ef",
  gridMinor: "#708495", gridMajor: "#8fa4b6", axisX: "#f28388", axisY: "#79cda6", axisZ: "#83adff",
} as const;

/** Logical CSS pixels. Canvas/renderer adapters handle DPR separately. */
export const uiTokens = {
  fontFamily: '"Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", system-ui, sans-serif',
  fontSize: 13, fontSmall: 12, fontTitle: 14, fontDimension: 14, fontInline: 15,
  fontWeight: 400, fontGroupWeight: 500, fontTitleWeight: 600,
  controlHeight: 32, ribbonHeight: 30, statusActionHeight: 24, statusHeight: 32,
  categoryHeight: 36, commandAreaHeight: 88, toolWidth: 88, splitWidth: 112, arrowWidth: 24,
  iconToolbar: 18, iconMenu: 18, iconAction: 16,
  radius: 4, radiusPanel: 8, borderWidth: 1, space4: 4, space8: 8, space12: 12, space16: 16, space24: 24,
  shadowPopup: "0 6px 20px rgba(23,35,51,0.14)",
  shadowPanel: "0 12px 36px rgba(23,35,51,0.20)",
  shadowInline: "0 2px 8px rgba(23,35,51,0.16)",
  motion: "140ms", layerViewport: 0, layerControls: 20, layerInline: 30, layerPanel: 40,
  layerPopup: 600, layerModal: 1000, layerNotification: 1400,
} as const;
export const commandPanelWidths = { S: 380, M: 520, L: 960 } as const;
export type CommandPanelSize = keyof typeof commandPanelWidths;

export const colorNumber = (color: string): number => Number.parseInt(color.slice(1), 16);
export function installVisualTokens(root: HTMLElement): void {
  for (const [name, value] of Object.entries(palette)) root.style.setProperty(`--color-${name.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)}`, value);
  for (const [name, value] of Object.entries(uiTokens)) {
    const key = name.replace(/[A-Z]/g, letter => `-${letter.toLowerCase()}`).replace(/([a-z])(\d)/g, "$1-$2");
    const unit = typeof value === "number" && !name.includes("Weight") && !name.startsWith("layer") ? "px" : "";
    root.style.setProperty(`--ui-${key}`, `${value}${unit}`);
  }
}
