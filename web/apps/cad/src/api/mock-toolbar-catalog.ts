import type { ToolbarCatalog, ToolbarCatalogEntry, ToolbarCatalogItem } from "../types";

type ItemSeed = [commandId: string, name: string, iconKey: string, helpText?: string, repeatable?: boolean, groupKey?: string];

const items = (seeds: ItemSeed[]): ToolbarCatalogItem[] => seeds.map(([commandId, name, iconKey, helpText, repeatable, groupKey], index) => ({
  commandId, name, iconKey, helpText: helpText ?? `${name}命令。`, repeatable: repeatable ?? false,
  groupKey: groupKey ?? "primary", sortOrder: (index + 1) * 10,
}));

const toolbar = (id: string, name: string, workbench: ToolbarCatalogEntry["workbench"],
  position: ToolbarCatalogEntry["position"], styleKey: ToolbarCatalogEntry["styleKey"], sortOrder: number,
  seeds: ItemSeed[]): ToolbarCatalogEntry => ({
  id, name, workbench, position, styleKey, sortOrder, orientation: "horizontal", items: items(seeds),
});

// Keep this isolated frontend fixture aligned with migrations 0023–0026. Production
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
    ["sketch.normal", "正对草图平面", "top-view", "恢复活动草图平面的正视方向，保留当前缩放和关注区域。"],
  ]),
  toolbar("sketch-projection", "外部几何", "SKETCHER", "top-left", "sketch", 20, [
    ["sketch.project", "投影", "project", undefined, true],
  ]),
  toolbar("sketch-primitives", "草图基本元素", "SKETCHER", "top-left", "sketch", 30, [
    ["sketch.point", "点", "point", undefined, true], ["sketch.line", "直线", "line", undefined, true],
    ["sketch.arc", "圆弧", "arc", undefined, true], ["sketch.polyline", "多段线", "polyline", undefined, true],
    ["sketch.spline", "拟合点样条", "spline", "采集拟合点并创建必须经过这些点的插值样条；双击或 Enter 完成。", true],
    ["sketch.spline.control", "控制点样条", "spline", "创建控制点 B-Spline，曲线不要求经过控制点；闭合缝不保证切向连续。", true, "more"],
  ]),
  toolbar("sketch-profiles", "草图轮廓", "SKETCHER", "top-left", "sketch", 40, [
    ["sketch.rectangle", "矩形", "rectangle", undefined, true], ["sketch.polygon", "正六边形", "polygon", undefined, true],
    ["sketch.circle", "圆", "circle", undefined, true],
    ["sketch.circle.three_point", "三点圆", "circle", "依次选择三个圆周点创建真实圆。", true, "more"],
    ["sketch.arc.three_point", "三点圆弧", "arc", "依次选择起点、经过点和终点，方向由经过点确定。", true, "more"],
    ["sketch.rectangle.center", "中心矩形", "rectangle", "选择中心与角点；中心由辅助对角线及中点约束保持。", true, "more"],
    ["sketch.rectangle.oriented", "定向矩形", "rectangle", "选择底边两点及高度创建任意方向矩形。", true, "more"],
    ["sketch.ellipse", "椭圆", "circle", "选择中心、主轴及次轴；圆形请使用圆工具。", true, "more"],
    ["sketch.elliptical_arc", "椭圆弧", "arc", "定义椭圆后选择弧范围；R 切换方向。", true, "more"],
  ]),
  toolbar("sketch-edit", "草图编辑", "SKETCHER", "top-left", "sketch", 45, [
    ["sketch.edit.delete", "删除几何", "delete", "删除选定本地几何及明确受影响的约束。"],
    ["sketch.edit.copy", "复制几何", "copy", "生成独立几何副本；I 可选择只复制所选集合内部约束。"],
    ["sketch.edit.move", "移动几何", "move", "保留现有约束；选择基点及目标点，或输入 X/Y 位移。"],
    ["sketch.edit.rotate", "旋转几何", "rotate", "选择中心、基准点及目标点，或输入角度。"],
    ["sketch.edit.scale", "统一缩放", "scale", "选择中心及两参考点，或输入正比例；驱动尺寸冲突时拒绝。"],
    ["sketch.edit.mirror", "镜像", "mirror", "默认关联镜像；M 切换独立副本，选择直线或按 X/Y 使用内置轴。"],
    ["sketch.edit.spline_insert", "样条插入点/结点", "point", "FIT 插入拟合点；CONTROL 精确插入结点，保持曲线形状。", false, "more"],
    ["sketch.edit.spline_delete", "样条删除点/结点", "delete", "FIT 删除明确拟合点；CONTROL 仅允许精确移除结点，不任意删 pole。", false, "more"],
    ["sketch.edit.spline_close", "样条开/闭合", "spline", "明确切换样条开闭；闭合缝不保证高阶连续。", false, "more"],
    ["sketch.edit.spline_control", "转控制点样条", "spline", "将 FIT 的已求解 canonical 曲线精确转为 CONTROL，先说明点引用影响。", false, "more"],
    ["sketch.edit.extend", "延伸", "line", "明确端点、边界及交点分支；外部边界只读，权威内核计算交点。"],
    ["sketch.edit.offset", "二维偏移", "reference", "线段、圆、圆弧及连续链的有符号独立偏移；M 切换斜接/圆角。"],
    ["sketch.edit.complement", "圆弧补弧", "arc", "改为另一补弧；U 显式解除受影响整体/端点关系。", false, "more"],
    ["sketch.edit.close", "闭合曲线", "circle", "圆弧/椭圆弧转完整周期曲线；旧端点引用不静默转绑。", false, "more"],
    ["sketch.edit.fillet", "草图圆角", "arc", "选择两条线/圆弧的裁切端部与分支，输入半径；B 可加入原子多角点批次。"],
    ["sketch.edit.chamfer", "草图倒角", "line", "两条线段；M 切换等长/双长度/长度加角度，K 保留或修剪支撑，B 批次。"],
    ["sketch.edit.trim", "修剪范围", "reference", "输入真实参数域中保留范围；交点与合法曲线子段由内核计算。"],
    ["sketch.edit.quick_trim", "快速修剪", "reference", "选边界及命中段；Q 切换删除、保留或打断，权威内核计算交点。"],
    ["sketch.edit.split", "分割几何", "reference", "输入真实参数域的比例位置；约束引用由控制面映射。", false, "more"],
    ["sketch.edit.construction", "标准/辅助转换", "reference", "原子切换选定几何的轮廓角色。", false, "more"],
  ]),
  toolbar("sketch-geometric-constraints", "几何约束", "SKETCHER", "top-left", "sketch", 50, [
    ["sketch.constraint.coincident", "重合", "coincident", undefined, true], ["sketch.constraint.parallel", "平行", "parallel", undefined, true],
    ["sketch.constraint.collinear", "共线", "parallel", undefined, true],
    ["sketch.constraint.fixed", "固定", "fixed", undefined, true], ["sketch.constraint.horizontal", "水平", "horizontal", undefined, true],
    ["sketch.constraint.vertical", "垂直", "vertical", undefined, true], ["sketch.constraint.perpendicular", "垂直相交", "perpendicular", undefined, true],
    ["sketch.constraint.tangent", "相切", "tangent", undefined, true], ["sketch.constraint.equal", "相等", "equal", undefined, true],
    ["sketch.constraint.concentric", "同心", "concentric", undefined, true], ["sketch.constraint.point_on_object", "点在对象上", "point-on-object", undefined, true],
    ["sketch.constraint.midpoint", "中点", "midpoint", undefined, true], ["sketch.constraint.symmetry", "对称", "symmetry", undefined, true],
  ]),
  toolbar("sketch-dimensional-constraints", "尺寸约束", "SKETCHER", "top-left", "sketch", 60, [
    ["sketch.dimension.linear", "线性尺寸", "distance", undefined, true], ["sketch.constraint.radius", "半径", "radius", undefined, true],
    ["sketch.constraint.horizontal_distance", "水平距离", "distance", "目标 X − 基准 X，允许负值和零。", true],
    ["sketch.constraint.vertical_distance", "竖直距离", "distance", "目标 Y − 基准 Y，允许负值和零。", true],
    ["sketch.constraint.major_radius", "长半轴", "radius", "设置椭圆或椭圆弧的长半轴长度 a。", true],
    ["sketch.constraint.minor_radius", "短半轴", "radius", "设置椭圆或椭圆弧的短半轴长度 b。", true],
    ["sketch.constraint.angle", "角度", "angle", undefined, true],
  ]),
  toolbar("product-structure", "产品结构", "ASSEMBLY_DESIGN", "top-left", "assembly", 10, [
    ["product.insert", "插入", "insert"], ["product.pattern", "多实例化", "pattern", "沿坐标轴重复所选组件并实时预览。"],
  ]),
  toolbar("product-interface", "产品接口", "ASSEMBLY_DESIGN", "top-left", "assembly", 20, [
    ["product.publications", "发布", "publication", "转发子 Part Publication。"],
    ["product.release", "产品版本", "release", "打开不可变产品版本中心。"],
  ]),
  toolbar("assembly-positioning", "组件定位", "ASSEMBLY_DESIGN", "top-left", "assembly", 30, [
    ["assembly.move", "移动组件", "move"],
    ["assembly.analyze", "装配约束分析", "measure"],
    ["assembly.move-receipt", "确认移动结果", "move"],
  ]),
  toolbar("assembly-constraints", "装配约束", "ASSEMBLY_DESIGN", "top-left", "assembly", 40, [
    ["assembly.coincident", "重合", "coincident"], ["assembly.contact", "接触", "coincident"], ["assembly.distance", "偏移", "distance"],
    ["assembly.angle", "角度", "angle"], ["assembly.fix", "固定", "fixed"], ["assembly.fix_together", "固联组", "link"],
    ["assembly.concentric", "同轴（重合）", "concentric"], ["assembly.parallel", "平行（角度）", "parallel"], ["assembly.perpendicular", "垂直（角度）", "perpendicular"],
  ]),
  toolbar("history", "历史", "ALL", "top-center", "standard", 70, [
    ["edit.undo", "撤销", "undo"], ["edit.redo", "重做", "redo"], ["history.version", "创建版本", "version"],
  ]),
  toolbar("collaboration", "协作", "ALL", "top-center", "standard", 80, [["document.share", "共享", "share"]]),
  toolbar("view-navigation", "视图", "ALL", "top-right", "standard", 90, [
    ["view.fit", "适合窗口", "fit"], ["view.top", "顶视图", "top-view"], ["view.front", "前视图", "front-view"],
    ["view.right", "右视图", "right-view"], ["view.iso", "等轴测", "isometric"],
    ["view.normal", "法线视图", "top-view", "选择基准面或实体平面后正对该面，保留显示比例。"],
  ]),
  toolbar("debug", "诊断", "ALL", "bottom-right", "debug", 100, [["debug.download", "下载诊断包", "debug"]]),
] };
