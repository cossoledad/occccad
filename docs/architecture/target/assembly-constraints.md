# 装配六类约束：数学语义、组合能力与生命周期

> 目标设计，尚未完整交付。用户约束分类以本机 CATIA B33 Assembly Design 为参照；实现基线见[当前 Product](../current/product-assembly.md)，唯一实施顺序见[装配主线](../../../plans/assembly-evolution.md)。本系统以自洽的数学语义、刚体自由度控制及可组合性为验收依据；CATIA 的分类与流程是参考，不要求逐项复制其参数、限制或文档疑点。

## 参照范围与证据

2026-09-21 从 `control/AsmEnglishC2.viewdoc`、`control/CfyEnglishC2.viewdoc` 定位，并读取用户指定的 `online/CATIAfr_C2/asmugCATIAfrs.htm` frameset、Assembly 菜单及下列任务/参考页。绝对根目录为 `/mnt/s/tools/DS/Catia/B33doc/English/`；本页编号仅为来源定位，不是开发批次。

| 来源 | 内容 |
|---|---|
| `online/asmug_C2/asmugwd0300.htm`；`online/cfyugasm_C2/cfyugasmwd0100.htm` | Insert 菜单与 Constraints Toolbar：六种用户约束，Quick Constraint 是创建辅助工具 |
| `online/cfyugasm_C2/cfyugasmut0301.htm`、`cfyugasmrf0102.htm` | Coincidence，支持几何组合和 Undefined/Same/Opposite |
| `online/cfyugasm_C2/cfyugasmut0302.htm`、`cfyugasmrf0103.htm` | Contact，定向支撑、面/线/点/环接触及兼容矩阵 |
| `online/cfyugasm_C2/cfyugasmut0303.htm`、`cfyugasmrf0104.htm` | Offset，符号、选择顺序、Measure 模式 |
| `online/cfyugasm_C2/cfyugasmut0304.htm`、`cfyugasmrf0105.htm` | Angle、Parallelism、Perpendicularity、Planar angle、四种 sector |
| `online/cfyugasm_C2/cfyugasmut0305.htm` | Fix in space、相对 Fix、坐标编辑 |
| `online/cfyugasm_C2/cfyugasmut0306.htm` | 多成员 Fix Together、组内先解、组外后解、组激活与停用 |
| `online/cfyugasm_C2/cfyugasmrf0501.htm`、`cfyugasmut0311.htm` | 属性、Measure 适用范围、线接触 Internal/External、编辑/Reconnect |
| `online/cfyugasm_C2/cfyugasmut0309.htm`、`cfyugasmut1500.htm` | Activate/Deactivate；Deactivated 与求值状态、Measure Mode 独立统计 |
| `online/cfyugasm_C2/cfyugasmut0403.htm` | Manipulate 轴向/平面平移、绕轴转动、几何指定方向、With respect to constraints |

六类入口保持 Coincidence、Contact、Offset、Angle、Fix、Fix Together。下述系统合同优先于参照页面；来源差异记录为设计决策，不阻塞实现。所有列入本合同的几何组合仍需完整交付，不能以组合能力证明替代产品功能验收。任意曲面接触、摩擦/碰撞动力学、任意运动耦合与 Mechanism 时间积分不属于本轮范围。

## 自由度控制与组合能力

这里的“完备”限于刚体装配中三个平移、三个转动方向的控制及常见关节的表达，不声称可以表达任意非线性运动规律，也不保证任意约束网络有解、唯一解或全局数值收敛。

固定一个参考刚体后，另一刚体的相对位姿属于 `SE(3)`，有六个自由度。存在如下构造：

1. 两个锚点重合控制三个位置自由度，剩余绕该点的三个转动自由度。
2. 两个有向轴对齐控制其中两个转动自由度，剩余绕该轴的转动。
3. 选择不平行于该轴的稳定参考方向，以有向绕轴角控制最后一个自由度。

该 `3 + 2 + 1` 构造在参考方向有效时确定完整相对位姿。位置也可用锚点到三个独立参考平面的有符号偏移控制。Frame–Frame 完整重合提供直接的六自由度固定关系，使用正交旋转/局部旋转增量求值，不依赖有万向节锁的全局欧拉角参数化。以上是设计层的构造依据，实现仍须验证残差、Jacobian 和实际运动。

| 独立标量约束数（正常构型） | 构造 | 剩余相对运动 |
|---|---|---|
| 0 | 无约束 | 3 平移 + 3 转动 |
| 1 | 点在平面上 | 2 平移 + 3 转动 |
| 2 | 有向轴平行 | 3 平移 + 1 转动 |
| 3 | 点重合；或平面重合 | 球铰的 3 转动；或平面副的 2 平移 + 1 转动 |
| 4 | 同轴 | 沿轴平移 + 绕轴转动 |
| 5 | 同轴 + 轴向偏移；或同轴 + 绕轴角 | 转动副；或移动副 |
| 6 | 同轴 + 轴向偏移 + 绕轴角；或 Frame 重合 | 固定 |

对未被固定/消元的变量 `q`，在正则解邻域以 `dim(q) − rank(J_active)` 判断局部自由度；冗余方程不重复扣减。无世界参考的孤立刚体组还包含整体刚体运动，必须分开报告整体运动与组内相对自由度。奇异点的 Jacobian 零空间仅是瞬时线性结果，不能直接宣称存在同维的有限运动；应报告奇异性，并用解析案例、有限扰动/连续求解检查可运动方向。理论依据见 Modern Robotics 的[构型与速度约束](https://modernrobotics.northwestern.edu/nu-gm-book-resource/2-4-configuration-and-velocity-constraints/)；本表及 `3 + 2 + 1` 为本系统的设计构造。

验收必须同时检查自由度数量、运动方向及有限运动后的约束满足性，覆盖 0–6 阶、闭环、冗余、矛盾、退化、抑制恢复。仅有正确计数或完整六按钮不构成通过。

## 六种用户定义与内部方程分层

| 用户类型 | 必须保存的定义和行为 | 当前对应及缺口 |
|---|---|---|
| Coincidence（重合） | 两个支持元素、适用组合上的方向意图；点重合/点在线面上/共线/同轴/共面/坐标系重合 | 复用 `COINCIDENT`、`CONCENTRIC` 的适用方程；Concentric 是重合族中的几何语义，不作为第七种并列用户类型；补齐下述组合 |
| Contact（接触） | 有定向几何支撑、材料侧、接触分支及证据；按无限数学支撑求解 | 当前缺少独立产品 Contact 全链路；零距离不自动等于 Contact |
| Offset（偏移） | 支持元素、距离数量、方向/符号规则、Driving/Measure | 当前 `DISTANCE` 仅为算法基础；补齐点线、线面、带方向/符号和测量语义，不能只改 UI 标签 |
| Angle（角度） | 无轴0–360°空间角、指定轴0–360°投影角、平行、垂直；仅指定轴模式需要稳定参考轴 | 当前 ANGLE/DirectedAngle 数学基础可复用；前后端尚需贯通本系统方向/参考轴和几何可用性合同 |
| Fix（固定） | 默认 Fix in space，另支持相对 Fix；固定基准/捕获 pose、可编辑位置参数 | 当前 `FIX` 是捕获空间 pose，不能宣称相对 Fix 已支持 |
| Fix Together（固联） | 稳定组 identity、名称、至少两个成员、组/成员关系及激活状态；允许已有组加入 | 当前双体 `RIGID` 只覆盖相对刚性关系，不能代替多成员组与分阶段更新 |

用户类型、solver typed definition 和 descriptor 分开。公共 UI/目录/查询/编辑采用六类；旧实验命令与调用方按当前未发布规则收敛到唯一模型，不永久保留两套语义。内部允许复用 Rigid、Concentric、Distance、Parallel、Perpendicular 方程；工程连接是这些约束之上的后续组合层。

## 支持几何与参数门

### Coincidence

参考 `cfyugasmrf0102.htm` 的表及附注：

- Point–Point、Point–Line、Point–Plane、Line–Line、Line–Plane、Plane–Plane；Point 可来自精确点、球心、锥顶，Line 可来自直线、圆柱轴、锥轴，Plane 可来自平面支撑。
- Point–Curve 与 Point–Surface 是允许组合；Curve 附注限定为有效 underlying geometry，必须把可识别种类/退化和选定 branch 转为权威 descriptor/证据，不能任意把显示折线当精确曲线。本系统以可精确求值且参数域/分支可验证的支撑为准，在能力合同测试中明确，不依赖未解释的文档附注。
- Frame–Frame 定义为原点与完整朝向重合，控制六个相对自由度；Frame 必须为右手正交刚体坐标系，不把反射当旋转。Frame 与其他元素组合时必须显式选择其原点、轴或基准平面，再编译为点/线/面关系。例如“原点在平面上”控制一个自由度，“基准平面重合”控制三个；不提供含义不明的 Frame–Plane 隐式转换。这是对 CATIA 表/正文差异的本系统决策。
- Same/Opposite 仅在几何可定向时显示；Undefined 是用户意图，选定的离散分支是求值证据。不能像当前路径那样直接把 Undefined 永久改写为 Same；preview 内冻结 branch 防跳解，重新求值按显式策略选择并报告分支。

### Contact

`cfyugasmrf0103.htm` 使用图标表达接触类别；核对时必须读取图标与脚注，不能仅提取文字后遗漏允许组合。

| 无序几何对 | 本系统必须覆盖的接触分支 |
|---|---|
| Plane–Plane | 面接触 |
| Plane–Cylinder | 线接触 |
| Plane–Sphere | 点接触 |
| Cylinder–Cylinder | 线接触；半径相等才可面接触 |
| Sphere–Sphere | 半径相等、球心重合的面接触 |
| Sphere–Cone、Sphere–Circle | 环接触，按脚注 (4) |
| Cone–Cone | 线接触；锥角相等才可面接触 |
| Cone–Circle | 环接触 |

本轮未列出的组合返回 unsupported，不将其称为数学上不可能；后续可按同一合同扩展。Sphere–Cone 统一定义为环相切：球心位于锥轴、球面与所选锥叶的母线相切，接触环半径必须非零，材料侧与锥叶显式。球锥不能因转置图标而解释为整片曲面重合。Sphere–Circle/Cone–Circle 的环接触表示整条所选圆位于对应支撑上，明确区别于两个曲面的相切。

每个接触分支冻结解析定义、参数域、独立秩及退化处理。几何对是无序的，交换选择只变换定向参数/证据，不能改变可行位姿集合。相等半径/锥角导致接触类型或秩变化时显式切换定义或报告退化，不能继续使用通用距离残差并谎报自由度。以上规则直接解决来源图标疑点，不以获取 CATIA 运行环境为前置。

属性页 `cfyugasmrf0501.htm` 的线接触方向图 `images/rf020NLS.gif` / `rf021NLS.gif` 明确为 Internal/External（本轮已查看图像）；它们不是 Coincidence 的 Same/Opposite。点接触方向由默认规则确定。Contact 对定向支撑的内部/外部侧有要求；普通无定向 Surface 不能冒充实体面。Circle 按表中指定的环接触特例验证支撑来源。持久定义保存工程支撑、材料侧与 branch，精确 descriptor 提取须带 provenance。

接触作用于无限数学支撑，结果可位于可见修剪区域之外。不能偷偷增加有限面重叠条件，也不能用 mesh collision、最近三角形或力学接触替代。每个适用组合都覆盖选择顺序、内外侧、多解、相等半径/锥角、退化与不可能输入。

### Offset 与 Measure

Point/Line/Plane 的全部六种无序组合均须支持。至少一方为平面时才能定义正负号，实体面以材料外法向为正；双平面以第一选择的法向定义正号，selection order 属于持久意图。无平面的点点、点线、线线距离不能借任意世界轴制造 signed offset。

双平面的 Undefined/Same/Opposite 与偏移符号是不同参数。Measure 模式只测现有 placement，不产生驱动方程；两非平行平面不可测时返回稳定无效诊断/无数值，不显示旧值或伪零（CATIA 显示 `(##)`）。切换模式、表达式数量、重命名、Undo/Redo、刷新和 Release/replay 必须保持定义与测量结果的边界。

### Angle：空间角、指定轴角与方向关系

角度数量统一支持用户输入 0–360°；静态姿态的 360°与 0°等价，内部规范化为 `[0, 2π)`。不复制 CATIA 的 ≤90°输入加四个 sector 的表达方式。Angle 与 Offset 提供 Driving/Measured 切换；平行/垂直是关系类型，不冒充角度数量测量。停用恢复须保留原模式。

**无轴空间角**只约束两有向支持的夹角：`α = atan2(||a×b||, a·b)`。目标 θ≤180° 使用 α，θ>180° 使用反角 360°−α；不冻结叉积方向，不把任意辅助轴作为几何定义。正常构型仅限制一个转动自由度，法向可绕另一法向沿圆锥运动，允许与相容的线重合等约束组合。0°/180°/360° 的可行集合为平行/反平行，驱动方程按方向对齐处理，秩为二。

没有第三个参考方向时，θ与360°−θ具有同一可行姿态集合；正反角是保留的数量/分支意图，不能凭它定义全局唯一转向。自由度不足以唯一定位时，以既有名义位姿和运动偏好选解。需要物理上可区分的绕轴方向时，使用下面的显式轴模式。

**有向绕轴角**使用两条有向支持 `a,b` 和显式单位参考轴 `k`。令 `u = a − (a·k)k`、`v = b − (b·k)k`，在两个投影都非零时定义：

```text
θ = atan2(k · (u × v), u · v) mod 2π
```

参考轴和方向来自稳定 Datum/Publication/PersistentSelection，持久化来源、局部方向及正反意图；无向边/线经显式定向后也可使用。不能从相机或每帧 `a×b` 猜轴。交换 `a,b` 或反转 `k` 应按合同变换目标角；无有效轴或投影退化时返回明确诊断。残差使用周期一致的局部角差与 branch 管理，不能简单相减导致 359°→1°跳成 −358°。

该角一般贡献一个独立标量方程，仅控制投影后的方位角，**不隐含两方向都垂直于参考轴，也不意味着角为 0°时两空间方向平行**。需要平面内角时组合方向与轴垂直的关系；需要铰链角时先用同轴关系控制倾斜，再以绕轴角控制余下转动。UI 应显示“绕所选轴的角”与参考方向，不将其误标为无参考轴的空间夹角。

- Parallel：两方向平行，正常构型控制两个转动自由度；Same/Opposite 指定朝向，Undefined 保存意图及求值分支。不能用一个退化的 `dot=±1` 标量梯度代表两个独立约束。
- Perpendicular：两方向点积为零，正常构型控制一个转动自由度；界面提供正向90°/反向270°，保存方向意图，两者具有同一可行集合而不增加轴。Parallel 与 Perpendicular 提供独立工具栏入口。与“投影绕轴角等于 90°”明确区分。
- 平面以法向作为方向支持；线面关系必须注明“线与法向”或通过明确的组合表达“线在平面内”，不静默互余转换。空间角和指定轴投影角均可作为驱动或测量数量，界面明确区分。

静态 Revision 保存规范角、轴和方向意图；连续角、累计圈数与 warm start 属于 preview session，跨 0/180/360°保持连续。本轮不引入持久多圈传动约束。参数范围扩大本身不改变独立约束秩；自由度覆盖由前述构造及 conformance corpus 证明。CATIA 的角度输入域疑点据此关闭为设计差异，其 1 μm/1 μrad 阈值不自动改写平台容差。

### Fix 与 Fix Together

空间 Fix 在 owning Product 的坐标框架固定捕获 pose，嵌套 Product 移动不把子件错误钉到根世界坐标；提供位置/姿态参数编辑，显示角参数与内部 quaternion 转换必须可重放。

相对 Fix 以用户显式移动后的组件位置作为后续装配 update 的保留基准，使相关组件随之求解；空间 Fix 则在 update 回到固定位置。相对 Fix 既不是“无约束”，也不能偷换为两体 Rigid 或临时弱权重。用 CATIA `ut0305` 的“移动 → update”双模式样例冻结基准更新时间、手势确认、Undo/Redo 与欠约束解选择。

Fix Together 支持多成员、增删成员、命名及已有组参与；组保存稳定成员关系，不以数组位置为身份。根据 `ut0306`，组内已有约束先求解，再将更新后的组作为整体求组外约束；不能提前硬合并 rigid cluster 导致合法组内约束被误判冲突。重叠/嵌套组的环、重复成员与层次顺序必须明确定义和验证。用户操作一个成员时按尊重约束策略移动关联组；显式自由移动的行为与提示另行定义，不隐式解除组关系。

## 激活、模式、连接和求值状态正交

Activate/Deactivate（抑制）控制该定义是否参与更新，作用于六类约束以及 Fix Together 整组。抑制不是删除、隐藏或 Measure：

- 保留稳定 ID、完整参数、支持元素、组成员和原 Driving/Measured/Controlled 模式。持久激活状态与求解模式分开表达；若内部仍使用 `SUPPRESSED` 枚举，adapter 必须无损保存恢复前模式，不能激活后一律 Driving。
- 单个/批量切换通过 versioned Domain Command、原子 ChangeSet、CAS、幂等和 Undo/Redo；不触发零件 Feature 重算，只使受影响装配 component、DOF、manifest/结果证据失效。
- 非激活约束不进入硬方程、rigid-cluster 合并、rank/DOF 限制或活动冲突集合。组停用释放其组关系，但不删除或自动抑制组内独立定义的约束。
- 停用对象保留可检查的引用状态；来源缺失仍可显示 NotConnected，但不以其 Broken 阻止其余活动约束求解。激活前重新解析当前快照，不能复用停用前 Verified。恢复时的失败遵循现有命令/失败 Revision 合同，清楚报告 Broken/Impossible，不假报成功。
- Deactivated、Connected/NotConnected、NotUpdated/Broken/Impossible/Verified、Measure 是不同维度。树、属性、视口符号和分析计数都需显示停用；停用标记不抹去诊断证据。
- M3/Release 冻结全部定义与激活状态，参与 solver 的 active set 可重建。Release gate 对活动约束要求 Verified，对停用定义显式记录排除理由；停用不伪造 Verified，也不使旧 Release 随新激活状态变化。

## 尊重约束的连续三维操纵

对标 `ut0403` 的轴向平移、平面平移、轴向旋转和由几何指定方向/轴。默认受约束编辑只在所有活动硬约束允许的流形上移动，固定/固联/抑制/测量模式都参与正确的自由度解释。

实时不是每个 pointermove 提交 Revision。浏览器响应手势，服务端用不可变 M3 输入和 session branch/warm start 连续求解；请求合并、取消、背压和过期响应门保持有效。不可达目标返回最近可行 pose 与 blocked feedback；基础设施失败保留最后确认帧，不显示假成功。pointerup 等待最终确认，只形成一次版本化移动，Esc/cancel 不提交。具体交互性能预算与测量场景由[装配主线](../../../plans/assembly-evolution.md)维护。
