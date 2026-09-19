import type { ToolbarCatalog, ToolbarCatalogEntry, ToolbarCatalogItem } from "../types";

type ItemSeed = [commandId: string, name: string, iconKey: string, helpText?: string, repeatable?: boolean];

const items = (seeds: ItemSeed[]): ToolbarCatalogItem[] => seeds.map(([commandId, name, iconKey, helpText, repeatable], index) => ({
  commandId, name, iconKey, helpText: helpText ?? `${name}命令。`, repeatable: repeatable ?? false,
  groupKey: "primary", sortOrder: (index + 1) * 10,
}));

const toolbar = (id: string, name: string, workbench: ToolbarCatalogEntry["workbench"],
  position: ToolbarCatalogEntry["position"], styleKey: ToolbarCatalogEntry["styleKey"], sortOrder: number,
  seeds: ItemSeed[]): ToolbarCatalogEntry => ({
  id, name, workbench, position, styleKey, sortOrder, orientation: "horizontal", items: items(seeds),
});

// Keep this isolated frontend fixture aligned with migration 0023. Production
// receives the same presentation catalog from the server.
export const mockToolbarCatalog: ToolbarCatalog = { schemaVersion: 1, toolbars: [
  toolbar("selection", "选择", "ALL", "top-left", "standard", 0, [
    ["tool.select", "选择", "select", "选择视图区或结构树中的对象。"],
  ]),
  toolbar("part-sketch", "草图入口", "PART_DESIGN", "top-left", "part", 10, [
    ["sketch.start", "草图", "sketch", "选择基准面创建草图，或选择已有草图进入编辑。"],
  ]),
  toolbar("part-features", "实体特征", "PART_DESIGN", "top-left", "part", 20, [
    ["part.pad", "拉伸", "pad"], ["part.pocket", "切除", "pocket"], ["part.revolve", "旋转", "revolve"],
  ]),
  toolbar("part-knowledge", "参数与接口", "PART_DESIGN", "top-left", "part", 30, [
    ["part.parameters", "参数", "parameters", "集中查看、编辑并复用当前 Part 的参数。"],
    ["part.publications", "发布", "publication", "管理稳定 Publication 契约。"],
  ]),
  toolbar("part-datums", "基准", "PART_DESIGN", "top-left", "part", 40, [
    ["part.datum-plane", "基准面", "datum-plane"], ["part.datum-axis", "基准轴", "datum-axis"],
  ]),
  toolbar("sketch-lifecycle", "草图会话", "SKETCHER", "top-left", "sketch", 10, [
    ["sketch.finish", "退出草图", "finish"],
  ]),
  toolbar("sketch-projection", "外部几何", "SKETCHER", "top-left", "sketch", 20, [
    ["sketch.project", "投影", "project", undefined, true],
  ]),
  toolbar("sketch-primitives", "草图基本元素", "SKETCHER", "top-left", "sketch", 30, [
    ["sketch.point", "点", "point", undefined, true], ["sketch.line", "直线", "line", undefined, true],
    ["sketch.arc", "圆弧", "arc", undefined, true], ["sketch.polyline", "多段线", "polyline", undefined, true],
    ["sketch.spline", "过点曲线", "spline", undefined, true],
  ]),
  toolbar("sketch-profiles", "草图轮廓", "SKETCHER", "top-left", "sketch", 40, [
    ["sketch.rectangle", "矩形", "rectangle", undefined, true], ["sketch.polygon", "正六边形", "polygon", undefined, true],
    ["sketch.circle", "圆", "circle", undefined, true], ["sketch.slot", "长圆孔", "slot", undefined, true],
  ]),
  toolbar("sketch-geometric-constraints", "几何约束", "SKETCHER", "top-left", "sketch", 50, [
    ["sketch.constraint.coincident", "重合", "coincident", undefined, true], ["sketch.constraint.parallel", "平行", "parallel", undefined, true],
    ["sketch.constraint.fixed", "固定", "fixed", undefined, true], ["sketch.constraint.horizontal", "水平", "horizontal", undefined, true],
    ["sketch.constraint.vertical", "垂直", "vertical", undefined, true], ["sketch.constraint.perpendicular", "垂直相交", "perpendicular", undefined, true],
    ["sketch.constraint.tangent", "相切", "tangent", undefined, true], ["sketch.constraint.equal", "相等", "equal", undefined, true],
    ["sketch.constraint.concentric", "同心", "concentric", undefined, true], ["sketch.constraint.point_on_object", "点在对象上", "point-on-object", undefined, true],
    ["sketch.constraint.midpoint", "中点", "midpoint", undefined, true], ["sketch.constraint.symmetry", "对称", "symmetry", undefined, true],
  ]),
  toolbar("sketch-dimensional-constraints", "尺寸约束", "SKETCHER", "top-left", "sketch", 60, [
    ["sketch.dimension.linear", "线性尺寸", "distance", undefined, true], ["sketch.constraint.radius", "半径", "radius", undefined, true],
    ["sketch.constraint.angle", "角度", "angle", undefined, true],
  ]),
  toolbar("product-structure", "产品结构", "ASSEMBLY_DESIGN", "top-left", "assembly", 10, [
    ["product.insert", "插入", "insert"],
  ]),
  toolbar("product-interface", "产品接口", "ASSEMBLY_DESIGN", "top-left", "assembly", 20, [
    ["product.publications", "发布", "publication", "转发子 Part Publication。"],
    ["product.release", "产品版本", "release", "打开不可变产品版本中心。"],
  ]),
  toolbar("assembly-positioning", "组件定位", "ASSEMBLY_DESIGN", "top-left", "assembly", 30, [
    ["assembly.move", "移动组件", "move"],
  ]),
  toolbar("assembly-constraints", "装配约束", "ASSEMBLY_DESIGN", "top-left", "assembly", 40, [
    ["assembly.fix", "固定", "fixed"], ["assembly.rigid", "固连", "link"], ["assembly.coincident", "重合", "coincident"],
    ["assembly.concentric", "同心", "concentric"], ["assembly.angle", "角度", "angle"], ["assembly.distance", "距离", "distance"],
  ]),
  toolbar("history", "历史", "ALL", "top-center", "standard", 70, [
    ["edit.undo", "撤销", "undo"], ["edit.redo", "重做", "redo"], ["history.version", "创建版本", "version"],
  ]),
  toolbar("collaboration", "协作", "ALL", "top-center", "standard", 80, [["document.share", "共享", "share"]]),
  toolbar("view-navigation", "视图", "ALL", "top-right", "standard", 90, [
    ["view.fit", "适合窗口", "fit"], ["view.top", "顶视图", "top-view"], ["view.front", "前视图", "front-view"],
    ["view.right", "右视图", "right-view"], ["view.iso", "等轴测", "isometric"],
  ]),
  toolbar("debug", "诊断", "ALL", "bottom-right", "debug", 100, [["debug.download", "下载诊断包", "debug"]]),
] };
