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

// The mock API stands in for the server during isolated frontend development. Production never imports this fixture.
export const mockToolbarCatalog: ToolbarCatalog = { schemaVersion: 1, toolbars: [
  toolbar("part-design", "Part Design", "PART_DESIGN", "top-left", "part", 10, [
    ["tool.select", "选择", "select"], ["capture.settings", "捕捉", "capture"], ["sketch.start", "草图", "sketch"],
    ["part.pad", "拉伸", "pad"], ["part.pocket", "切除", "pocket"], ["part.revolve", "旋转", "revolve"],
    ["part.datum-plane", "基准面", "datum-plane"], ["part.datum-axis", "基准轴", "datum-axis"],
  ]),
  toolbar("sketch-geometry", "草图几何", "SKETCHER", "top-left", "sketch", 20, [
    ["tool.select", "选择", "select"], ["capture.settings", "捕捉", "capture"], ["sketch.point", "点", "point", undefined, true],
    ["sketch.line", "直线", "line", undefined, true], ["sketch.arc", "圆弧", "arc", undefined, true],
    ["sketch.polyline", "多段线", "polyline", undefined, true], ["sketch.spline", "过点曲线", "spline", undefined, true], ["sketch.finish", "退出草图", "finish"],
  ]),
  toolbar("sketch-geometric-constraints", "几何约束", "SKETCHER", "top-left", "sketch", 30, [
    ["sketch.constraint.coincident", "重合", "coincident", undefined, true], ["sketch.constraint.parallel", "平行", "parallel", undefined, true],
    ["sketch.constraint.fixed", "固定", "fixed", undefined, true], ["sketch.constraint.horizontal", "水平", "horizontal", undefined, true],
    ["sketch.constraint.vertical", "垂直", "vertical", undefined, true], ["sketch.constraint.perpendicular", "垂直相交", "perpendicular", undefined, true],
    ["sketch.constraint.tangent", "相切", "tangent", undefined, true], ["sketch.constraint.equal", "相等", "equal", undefined, true],
    ["sketch.constraint.concentric", "同心", "concentric", undefined, true], ["sketch.constraint.point_on_object", "点在对象上", "point-on-object", undefined, true],
    ["sketch.constraint.midpoint", "中点", "midpoint", undefined, true], ["sketch.constraint.symmetry", "对称", "symmetry", undefined, true],
  ]),
  toolbar("sketch-dimensional-constraints", "尺寸约束", "SKETCHER", "top-left", "sketch", 40, [
    ["sketch.dimension.linear", "线性尺寸", "distance", undefined, true], ["sketch.constraint.radius", "半径", "radius", undefined, true],
    ["sketch.constraint.angle", "角度", "angle", undefined, true],
  ]),
  toolbar("sketch-aggregates", "草图常用图形", "SKETCHER", "top-left", "sketch", 50, [
    ["sketch.rectangle", "矩形", "rectangle", undefined, true], ["sketch.polygon", "正六边形", "polygon", undefined, true],
    ["sketch.circle", "圆", "circle", undefined, true],
  ]),
  toolbar("assembly-design", "Assembly Design", "ASSEMBLY_DESIGN", "top-left", "assembly", 60, [
    ["tool.select", "选择", "select"], ["capture.settings", "捕捉", "capture"], ["product.insert", "插入", "insert"],
    ["product.reference.toggle", "引用模式", "reference"], ["assembly.move", "移动组件", "move"], ["assembly.fix", "固定", "fixed"],
    ["assembly.coincident", "重合", "coincident"], ["assembly.concentric", "同心", "concentric"],
    ["assembly.angle", "角度", "angle"], ["assembly.distance", "距离", "distance"],
  ]),
  toolbar("common-edit", "编辑", "ALL", "top-center", "standard", 70, [
    ["edit.undo", "撤销", "undo"], ["edit.redo", "重做", "redo"], ["history.version", "创建版本", "version"], ["document.share", "共享", "share"],
  ]),
  toolbar("view", "视图", "ALL", "top-right", "standard", 80, [
    ["navigation.profile.toggle", "导航模式", "navigation"], ["view.fit", "适合窗口", "fit"], ["view.iso", "等轴测", "isometric"],
  ]),
  toolbar("debug", "Debug", "ALL", "bottom-right", "debug", 90, [["debug.download", "下载诊断包", "debug"]]),
] };
