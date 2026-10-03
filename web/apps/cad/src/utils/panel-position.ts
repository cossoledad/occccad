export type PanelPosition = { x: number; y: number };
export function normalizePanelPosition(value: unknown): PanelPosition | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { x, y } = value as Partial<PanelPosition>;
  return typeof x === "number" && Number.isFinite(x) && typeof y === "number" && Number.isFinite(y) ? { x, y } : undefined;
}
export function clampPanelPosition(position: PanelPosition, panel: { width: number; height: number },
  container: { width: number; height: number }): PanelPosition {
  return {
    x: Math.max(16, Math.min(position.x, container.width - panel.width - 16)),
    y: Math.max(16, Math.min(position.y, container.height - panel.height - 16)),
  };
}

export function initialCommandPanelPosition(panel:{width:number;height:number},host:{width:number;height:number},viewport:{left:number;top:number;width:number;height:number},large=false):PanelPosition {
 return clampPanelPosition({x:large?(host.width-panel.width)/2:viewport.left+viewport.width-panel.width-16,y:viewport.top+16},panel,host);
}
