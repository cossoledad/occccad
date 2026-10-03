# Sketch Solver

此目录负责同进程 PlaneGCS 适配和外部几何在支撑面上的计算投影，
不拥有持久模型、参数表达式、Revision 或用户操作事务。
PlaneGCS 固定为 FreeCAD commit `256fc7eff3379911ab5daf88e10182c509aa8052`，
源码与 SHA256 清单见 `CMakeLists.txt`，不依赖上游最新接口。

## 两线距离合同

`DISTANCE` 的两个 whole/direction 直线引用表示无限支撑线的非负平行间距，
不是有限线段最近距离。允许本地直线和内置轴；同一支撑实体不能对自身标注间距。
控制面须持久化可见的 `PARALLEL` 关系；已具有相同 H/V 关系或各自平行同一内置轴时
无需重复添加。Worker 依据这些明确关系验证，不根据初始坐标猜测平行，也不增加 Fix。
Go 操作负责关系与尺寸的原子创建、Parameter/单位以及历史。

非零间距使用锁定 PlaneGCS 的 `addConstraintP2LDistance`（绝对值距离）加已有平行关系；
零间距使用 `addConstraintPointOnLine` 加已有平行关系，避免绝对值在零点的不可导性。
反转端点和交换选择次序不改变尺寸含义。非零间距不保留有向侧别：求解以初始几何作
数值初值，不宣称支持持久化侧向意图。与角度冲突由 PlaneGCS 按正式关系 ID 诊断。
不存在显式平行关系、负数/非有限数或重复实体引用时拒绝模型。

同一几何的欠约束、冗余和冲突仍分别返回；两条自由线只添加 Parallel 和 Distance
时保留 6 DOF。外部几何的只读性及通过固定参考参与求解由控制面合同保障。

## 验证

- 定向：`invoke check --scope sketch --match ParallelLineDistance`
- 领域：`invoke check --scope sketch`

对应 `tests/solver_scenarios.cpp` 中 `ParallelLineDistance*`：实际残差、DOF、
非原点倾斜线、反向/选择顺序、零/重合初值、尺度变化、内置轴、已有 H/V、
角度冲突以及非法引用/负数。C++ 测试不代表参数 UI、Undo 或 Router 集成已经验收。

## 椭圆与椭圆弧

领域表示为中心、主半轴、次半轴及旋转角（radian）；椭圆弧另有有向的起止参数，
支持反向和跨周期接缝。Worker 原生编译为 PlaneGCS `Ellipse` 的 center/focus1/radmin，
椭圆弧使用 `ArcOfEllipse` 与原生 `ArcOfEllipseRules`。求解结果转换回领域半轴及旋转。
要求 `major_radius > minor_radius > 0`；圆退化使用 Circle，不使用零焦距椭圆猜测方向。

支持组合：

| 关系 | 几何角色与含义 |
|---|---|
| Fixed | 椭圆整体五参数；椭圆弧额外固定起止参数 |
| Coincident / FixedPoint / Symmetry | 椭圆中心；椭圆弧中心、真实参数端点 |
| Concentric | Circle/Arc/Ellipse/EllipticalArc 任意中心组合 |
| Equal | 两个 Ellipse/EllipticalArc 的主、次半轴同时相等 |
| PointOnObject | 点在真实椭圆；椭圆弧使用 CurveValue 辅助参数并校验参数范围 |
| Tangent | Line × Ellipse/EllipticalArc；椭圆弧校验解析切点在弧范围内 |
| MAJOR_RADIUS / MINOR_RADIUS | mm 半轴尺寸，单独驱动主/次半轴 |

主半轴尺寸使用锁定版本的 `ConstraintEqualMajorAxesConic` 和不可变常量参考椭圆；
次半轴使用原生标量 Equal。参考椭圆不进入未知量，不固定业务几何，也不产生自由度。
没有手写曲线残差或有限采样约束。Ellipse × Circle 或 Ellipse × Ellipse 的 Tangent
没有相应适配，明确拒绝。椭圆弧范围外的点/切点、圆退化和负/零半轴不能当成可行结果；
有限参数分支没有全局穷举保证，范围门禁失败返回数值失败诊断，不宣称已证明无解。

测试入口：`invoke check --scope sketch --match 'Ellipse|Elliptical'`。
`Ellipse*` / `EllipticalArc*` 场景覆盖五/七 DOF、真实半轴尺寸、解析椭圆残差、
Line tangent、端点连接、Equal/Concentric、反向跨周期弧、弧范围门禁和退化拒绝。

## 样条表示与关联镜像

历史字段 `control_points` 的语义保留为 FIT 拟合点；不把它改名成极点。
`mode=CONTROL` 编辑 `poles`，对应真实有理 B-Spline 的 degree、唯一递增 knots、
multiplicities、positive weights 和 parameter_start/end。FIT 求解后由 OCCT Interpolate
产生这些 canonical 字段。周期输入原生转换为同参数域内的非周期 clamped canonical；
closed 表达几何闭合，periodic 表达基函数周期性，两者不同。
Profile 直接构造 canonical B-Spline，不使用旧拟合点再次拟合。
精确求值、D1、交点、重叠区间与 Segment 裁剪入口见 kernel.hpp 的 `*_sketch_curve*`。
裁剪生成 CONTROL 子曲线；FIT 原曲线的拟合方式与拟合点继续保留。

`MIRROR` 的正式引用顺序为 source WHOLE、稳定 axis DIRECTION、target WHOLE，
源和目标须同类型。Point/Line 反射对应点；Circle 反射圆心并保持半径；
Arc 另反射真实起止端点；Ellipse 反射圆心和焦点并保持次半轴；EllipticalArc 另反射端点。
编译使用原生 Symmetry、Equal 和曲线规则，不维护求解后的第二套坐标修补机制。
同一关系的内部代数冗余可被容忍，真正的冲突仍通过 MIRROR 正式 ID 返回。

FIT 镜像反射全部对应拟合点，凭插值方程的反射等变性约束整条曲线；
CONTROL 镜像要求相同 degree/knots/multiplicities/weights/domain/periodic，反射全部对应极点。
不同表示或基函数不静默重拟合。Construction 与实体身份由控制面保留；
同实体 self mirror 以显式 `self_mirror_mode` 保留关系：Line/Spline 使用 ON_AXIS 或 PAIRED，
Ellipse/EllipticalArc 使用 MAJOR_PARALLEL 或 MAJOR_PERPENDICULAR。Point/Circle 的中心在轴上；
Arc/EllipticalArc 起终点成对反射；CONTROL 自映射要求反序 knots/multiplicities/weights/domain
对称，拒绝不匹配基函数。控制面负责去重和轴端点正式 Coincident、删除/修剪关系及历史。
PlaneGCS 原生对称的 Perpendicular 以两条线长度归一，合法轴上自映射会产生零 chord；
适配仅改该原语的 rescale 为轴长归一，保留原生精确 residual 与 analytic gradient。
`ReflectionNormalResidual*` 对零/非零 chord 全部参数进行中央差分导数验证。

验证入口：`invoke check --scope sketch --match LinkedMirror` 与
`ctest --test-dir build/cmake/debug -R '^geometry/ExactSketchCurve' --output-on-failure`。
其中 kernel 测试独立检查 rational curve 解析值/端部 D1、FIT/canonical 值和 D1、
周期转非周期、exact split、镜像整曲线等变性及 canonical Profile Shape gate。

## 坐标投影尺寸与共线

`HORIZONTAL_DISTANCE` / `VERTICAL_DISTANCE` 使用两个明确的点引用，持久值是
`references[1] − references[0]` 的有向 X/Y 投影（mm），允许负值和零。
使用锁定 PlaneGCS 的 Difference 原语，不取绝对值，也不依据相机或数组端点重排符号。
原点、独立点、实体中心、真实端点及相应表示的 CONTROL 引用均沿统一点解析入口。
选择次序属于正式定义；互换引用须同时反转数值，不能只改变画面标签。

`COLLINEAR` 对两条直线/本地线与内置轴编译 Parallel + PointOnLine。
已具有正式 Parallel 或相同 H/V/内置轴平行关系时不重复添加平行方程。
不新增方程种类，不固定几何；两条自由线共线保留 6 DOF。
同一支撑对的非平行 Angle 与 Collinear 数学矛盾在原生数值求解前明确返回双方 ID；
其他数值不收敛继续是 failed，不能据此宣称冲突。
测试入口：`invoke check --scope sketch --match 'ProjectedDistance|Collinear'`。

## 支撑关系与精确端部相切

内部 `SAME_SUPPORT` 保留裁剪子段与原支撑几何关系：Line 使用 Parallel/PointOnLine，
Circle/Arc 使用中心相等和半径相等，Ellipse/EllipticalArc 使用中心、焦点、次半轴相等。
起止参数仍独立可变，不重新分配原端点身份；Spline 不伪造不同基函数的同支撑关系。

Line × Arc/EllipticalArc 指定 START/END 时使用真实端点接触和原生精确切向/法向方程；
已有 Coincident 到 Line 端点时去重 PointOnLine。CONTROL 开放 clamped 非周期 Spline
端部切向以首/末极点 chord 编译 Parallel：有理 B-Spline 端部 D1 是该 chord 的正标量倍数。
拒绝奇异零导数、FIT 点冒充极点、周期/未 clamped 的该端部组合。FIT 可显式转换 CONTROL。
入口：`invoke check --scope sketch --match 'SameSupport|SelfMirror|Endpoint|ReflectionNormal'`。

精确 Profile 分类由 `classify_sketch_profile` 执行：真正参数曲线 self-intersection、
非相邻接触、边重叠、环间交点先拒绝，再用原生 wire/face、面积与 BRepClass 确定孔/岛。
只允许模型拓扑已经遍历出的相邻接点；坐标公差验证接点，不新增连接。
对应 kernel `ExactSketchCurve.Profile*` 测试直接检查拉伸 Shape/体积。

固定实体的真实端点 Coincident 若已由两侧正式 whole Fixed 保证，使用锁定 PlaneGCS
`getFinePrecision()` 校验其残差后不再重复编译标量；模型连接及其 REDUNDANT ID 保留。
这避免圆角半径更新时原生 reduced-system 诊断将已固定的相同端点误判冲突；
不建立新连接、不处理未固定坐标、不同固定端点及冲突尺寸仍必须报告 CONFLICTING。
冗余成功结果与普通成功结果统一通过 applySolution 和有限弧/端部导数门禁；
未收敛结果保留 FAILED，不以冗余标签接受旧坐标。测试：`LineArcAndArcArcCorner*`、
`FixedEndpointCoincidence*`、`RedundantDisconnectedComponent*`。
