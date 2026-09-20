/** Shared product palette. CSS, Ant Design and WebGL consume these semantic roles. */
export const palette = {
  primary: "#3868df", primaryHover: "#2855c5", primarySoft: "#eaf0ff",
  chrome: "#172333", chromeRaised: "#24364b", chromeText: "#e4edf8",
  canvas: "#f1f4f8", surface: "#ffffff", subtle: "#f7f9fc", border: "#dce3ed",
  text: "#24334a", muted: "#687b93", success: "#25846a", warning: "#a56a19", danger: "#c44658",
  viewportTop: "#293744", viewportBottom: "#526372", solid: "#c1cbd5", productSolid: "#b9c7d4",
  edge: "#172a3e", vertex: "#edf3fa", sketch: "#f0f5fc", construction: "#8facca", external: "#80cfe6",
  selected: "#f4ba62", hover: "#68d7e6", preview: "#aab9ff", snap: "#ffe099",
  solved: "#7edbb1", invalid: "#ff8490", redundant: "#d6a0ef",
  gridMinor: "#708495", gridMajor: "#8fa4b6", axisX: "#f28388", axisY: "#79cda6", axisZ: "#83adff",
} as const;

export const colorNumber = (color: string): number => Number.parseInt(color.slice(1), 16);
export function installVisualTokens(root: HTMLElement): void {
  for (const [name, value] of Object.entries(palette)) root.style.setProperty(`--color-${name.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)}`, value);
}
