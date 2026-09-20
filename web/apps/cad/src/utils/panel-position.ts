export type PanelPosition = { x: number; y: number };
export function normalizePanelPosition(value: unknown): PanelPosition | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { x, y } = value as Partial<PanelPosition>;
  return typeof x === "number" && Number.isFinite(x) && typeof y === "number" && Number.isFinite(y) ? { x, y } : undefined;
}
export function clampPanelPosition(position: PanelPosition, panel: { width: number; height: number },
  container: { width: number; height: number }): PanelPosition {
  return {
    x: Math.max(8, Math.min(position.x, container.width - panel.width - 8)),
    y: Math.max(8, Math.min(position.y, container.height - panel.height - 8)),
  };
}
