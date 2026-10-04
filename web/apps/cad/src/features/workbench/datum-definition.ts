import type { DatumPlane, DocumentView, SelectionItem, TopologyElementProperties, Vec3 } from "../../types";
export const finiteVector = (v: unknown): v is Vec3 => Array.isArray(v) && v.length === 3 && v.every(Number.isFinite);
export function unitVector(v: Vec3): Vec3 {
  if (!finiteVector(v) || Math.hypot(...v) < 1e-9) throw new Error("方向需要三个有限数值，且不能为零向量");
  const length = Math.hypot(...v);
  return v.map(x => x / length) as Vec3;
}
export function planeFrame(origin: Vec3, normal: Vec3, preferredU?: Vec3): Pick<DatumPlane, "origin" | "normal" | "uDirection"> {
  if (!finiteVector(origin)) throw new Error("中心需要三个有限数值");
  const n = unitVector(normal);
  const preferredDot = preferredU?.reduce((sum, x, i) => sum + x * n[i], 0) ?? 0;
  const usable = preferredU && Math.hypot(...preferredU.map((x, i) => x - preferredDot * n[i])) > 1e-9;
  const basis = usable ? preferredU : (Math.abs(n[0]) < .9 ? [1, 0, 0] : [0, 1, 0]);
  const dot = basis.reduce((sum, x, i) => sum + x * n[i], 0);
  const u = unitVector(basis.map((x, i) => x - dot * n[i]) as Vec3);
  return { origin, normal: n, uDirection: u };
}
export function planarFaceFrame(properties: TopologyElementProperties) {
  const p = properties.properties;
  if (properties.geometryType !== "PLANE" || !finiteVector(p.snapCenter) || !finiteVector(p.normal) || !finiteVector(p.xDirection)) throw new Error("请选择实体上的平面");
  return planeFrame(p.snapCenter, p.normal, p.xDirection);
}
export function datumAxisDefinition(view: DocumentView, selection: SelectionItem) {
  if (selection.kind !== "axis") return;
  if (selection.axis === "DATUM") return (view.datumAxes ?? view.part?.datumAxes)?.find(a => a.id === selection.entityId);
  const system = (view.axisSystems ?? view.part?.axisSystems)?.find(a => a.id === selection.entityId);
  if (!system) return;
  return { name: `${system.name} ${selection.axis}`, origin: system.origin,
    direction: selection.axis === "X" ? system.xDirection : selection.axis === "Y" ? system.yDirection : system.zDirection };
}
