-- Local two-dimensional sketch workflow catalog. Keep previously applied
-- migrations immutable; this updates the single server-owned production catalog.
-- Slot is removed as a product command, without adding a data compatibility path.
DELETE FROM occccad.ui_toolbar_items
WHERE toolbar_id = 'sketch-profiles' AND command_id = 'sketch.slot';

INSERT INTO occccad.ui_toolbars(id,name,workbench,position,style_key,sort_order) VALUES
('sketch-edit','草图编辑','SKETCHER','top-left','sketch',45)
ON CONFLICT (id) DO UPDATE SET
name=EXCLUDED.name, workbench=EXCLUDED.workbench, position=EXCLUDED.position,
style_key=EXCLUDED.style_key, sort_order=EXCLUDED.sort_order;

-- Parent toolbar exists before child items. Removing Slot first frees its old
-- (toolbar_id,sort_order) unique position for the three-point circle command.
INSERT INTO occccad.ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable) VALUES
('sketch-primitives','sketch.point','点','在活动草图中连续创建点；C 切换辅助几何，Esc 或右键结束。','point','primary',10,true),
('sketch-primitives','sketch.line','直线','连续创建两点直线；长度/角度数值输入，Tab 切换，Enter 确认。','line','primary',20,true),
('sketch-primitives','sketch.polyline','多段线','连续创建相接直线和相切圆弧；T 切换，双击或 Enter 完成。','polyline','primary',40,true),
('sketch-primitives','sketch.spline','拟合点样条','采集拟合点并创建必须经过这些点的插值样条；双击或 Enter 完成。','spline','primary',50,true),
('sketch-primitives','sketch.spline.control','控制点样条','创建控制点 B-Spline；不要求经过控制点，闭合缝不保证切向连续。','spline','more',60,true),
('sketch-profiles','sketch.circle.three_point','三点圆','通过三个不共线点创建圆。','circle','more',40,true),
('sketch-profiles','sketch.arc.three_point','三点圆弧','通过起点、中间点和终点创建有向圆弧。','arc','more',50,true),
('sketch-profiles','sketch.rectangle.center','中心矩形','从中心和角点创建带正式中心关系的矩形。','rectangle','more',60,true),
('sketch-profiles','sketch.rectangle.oriented','定向矩形','从底边和第三点创建带正式方向约束的矩形。','rectangle','more',70,true),
('sketch-profiles','sketch.ellipse','椭圆','指定中心、长半轴与短半轴创建真实椭圆。','circle','more',80,true),
('sketch-profiles','sketch.elliptical_arc','椭圆弧','指定椭圆与起止参数范围创建有向椭圆弧。','arc','more',90,true),
('sketch-edit','sketch.edit.delete','删除几何','删除选定本地几何；约束影响由原子领域操作验证。','delete','primary',10,false),
('sketch-edit','sketch.edit.copy','复制几何','创建副本；默认仅几何（连接与尺寸未复制），I 切换内部约束复制，公式依赖拒绝。','copy','primary',20,false),
('sketch-edit','sketch.edit.move','移动几何','保留约束；选基点与目标点或输入 X/Y 位移，冲突明确拒绝。','move','primary',30,false),
('sketch-edit','sketch.edit.rotate','旋转几何','选中心、基准点和目标点，或输入角度；Shift+C 切换带复制。','rotate','primary',40,false),
('sketch-edit','sketch.edit.scale','统一缩放','选中心及两参考点，或输入正比例；固定、公式和尺寸冲突不静默删除。','scale','primary',50,false),
('sketch-edit','sketch.edit.mirror','镜像','默认关联镜像；M 切换独立副本，选直线或 X/Y 轴；独立模式默认不复制连接/尺寸，I 切换。','mirror','primary',60,false),
('sketch-edit','sketch.edit.spline_insert','样条插入点/结点','FIT 插入拟合点；CONTROL 精确插入结点，保持曲线形状。','point','more',83,false),
('sketch-edit','sketch.edit.spline_delete','样条删除点/结点','FIT 删除明确拟合点；CONTROL 只允许精确移除结点，不任意删 pole。','delete','more',84,false),
('sketch-edit','sketch.edit.spline_close','样条开/闭合','明确切换样条开闭；闭合缝不保证高阶连续。','spline','more',85,false),
('sketch-edit','sketch.edit.spline_control','转控制点样条','精确转换已求解 FIT canonical 曲线；U 明确释放受影响点引用。','spline','more',86,false),
('sketch-edit','sketch.edit.extend','延伸','明确要延伸的端点、只读参考边界和目标交点分支；权威内核计算。','line','primary',61,false),
('sketch-edit','sketch.edit.offset','二维偏移','线段、圆、圆弧及连续链独立偏移；有符号距离，M 斜接/圆角，退化和自交拒绝。','reference','primary',62,false),
('sketch-edit','sketch.edit.complement','圆弧补弧','改为另一补弧；U 显式解除整体/端点关系，不按最近端点重连。','arc','more',81,false),
('sketch-edit','sketch.edit.close','闭合曲线','圆弧/椭圆弧转完整周期曲线；原端点关系需显式解除。','circle','more',82,false),
('sketch-edit','sketch.edit.fillet','草图圆角','选择两条线/圆弧裁切端部与分支；半径圆角，K 保留或修剪，B 原子多角点批次。','arc','primary',63,false),
('sketch-edit','sketch.edit.chamfer','草图倒角','两条线段；M 切换等长/双长度/长度加角度，K 保留或修剪，B 原子批次。','line','primary',64,false),
('sketch-edit','sketch.edit.trim','修剪范围','输入曲线真实参数域的保留范围；U 显式查看整体关系释放影响。','reference','primary',65,false),
('sketch-edit','sketch.edit.quick_trim','快速修剪','选择本地/外部只读边界及命中段；Q 切换删除、保留或打断，权威内核计算交点。','reference','primary',66,false),
('sketch-edit','sketch.edit.split','分割几何','指定真实参数域内的位置；稳定端点和约束引用由控制面映射。','reference','more',70,false),
('sketch-edit','sketch.edit.construction','标准/辅助转换','原子切换选定几何的轮廓角色；辅助几何不参与实体轮廓。','reference','more',80,false),
('sketch-geometric-constraints','sketch.constraint.collinear','共线','使两条直线支撑共线；可使用内置草图轴。','line','more',15,true),
('sketch-dimensional-constraints','sketch.constraint.horizontal_distance','水平距离','两点沿草图 U 轴的有向投影距离；按第二点减第一点，允许负值与零。','distance','more',15,true),
('sketch-dimensional-constraints','sketch.constraint.vertical_distance','垂直距离','两点沿草图 V 轴的有向投影距离；按第二点减第一点，允许负值与零。','distance','more',16,true),
('sketch-dimensional-constraints','sketch.constraint.major_radius','长半轴','标注椭圆或椭圆弧的长半轴尺寸。','radius','more',70,true),
('sketch-dimensional-constraints','sketch.constraint.minor_radius','短半轴','标注椭圆或椭圆弧的短半轴尺寸。','radius','more',80,true)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
name=EXCLUDED.name, help_text=EXCLUDED.help_text, icon_key=EXCLUDED.icon_key,
group_key=EXCLUDED.group_key, sort_order=EXCLUDED.sort_order,
repeatable=EXCLUDED.repeatable;
