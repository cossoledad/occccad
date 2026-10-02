# 装配六类约束：数学语义、组合能力与生命周期

> 当前装配实现合同；能力覆盖与实际验证由唯一可执行目录和派生报告区分，不能仅凭本文宣称全部场景通过。编排见[Product](product-assembly.md)，数学实现见[算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md)，后续方向见[后续工作](../../../plans/README.md)。CATIA 是语义参照，不要求复制其私有模型或所有限制。

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

六类入口保持 Coincidence、Contact、Offset、Angle、Fix、Fix Together。下述系统合同优先于参照页面；来源差异记录为设计决策，不阻塞实现。目录区分实现覆盖与实际验证，组合数学证据不替代完整产品验收。任意曲面接触、摩擦/碰撞动力学、任意运动耦合与 Mechanism 时间积分不属于当前支持范围。

本合同的机器可读目录与定向执行约定见[可执行合同](../../../tests/assembly-contract/README.md)。目录固定用户族/内部类型映射、具体几何和参数区域、秩/退化及命令阶段失败规则；测试预期以本合同为依据，未知几何子类或未冻结秩保留显式问题，不能从 solver 的一次输出反向生成预期。实现覆盖与实际结果仍由 current/派生报告负责，本页不维护支持状态表。

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

| 用户类型 | 必须保存的定义和行为 | 内部方程边界 |
|---|---|---|
| Coincidence（重合） | 两个支持元素、适用组合上的方向意图；点重合/点在线面上/共线/同轴/共面/坐标系重合 | 可复用 `COINCIDENT`、`CONCENTRIC` 适用方程；Concentric 属于重合族，不作为第七种用户类型 |
| Contact（接触） | 有定向几何支撑、材料侧、接触分支及证据；按无限数学支撑求解 | 零距离不自动等于 Contact；必须满足接触几何及分支合同 |
| Offset（偏移） | 支持元素、距离数量、方向/符号规则、Driving/Measure | 可复用 `DISTANCE` 方程，但必须满足完整几何、符号及测量合同 |
| Angle（角度） | 无轴0–360°空间角、指定轴0–360°投影角、平行、垂直；仅指定轴模式需要稳定参考轴 | ANGLE/DirectedAngle 按几何、方向及参考轴合同消费，不以数值类型代替用户合同 |
| Fix（固定） | 默认 Fix in space，另支持相对 Fix；固定基准/捕获 pose、可编辑位置参数 | 空间与相对基准显式区分，不能用临时 interaction driver 替代持久定义 |
| Fix Together（固联） | 稳定组 identity、名称、至少两个成员、组/成员关系及激活状态；允许已有组加入 | 双体 `RIGID` 相对刚性方程不能代替多成员组与分阶段更新 |

用户类型、solver typed definition 和 descriptor 分开。公共 UI/目录/查询/编辑采用六类；旧实验命令与调用方按当前未发布规则收敛到唯一模型，不永久保留两套语义。内部允许复用 Rigid、Concentric、Distance、Parallel、Perpendicular 方程；工程连接是这些约束之上的后续组合层。

## 支持几何与参数门

### Coincidence

参考 `cfyugasmrf0102.htm` 的表及附注：

- Point–Point、Point–Line、Point–Plane、Line–Line、Line–Plane、Plane–Plane；Point 可来自精确点、球心、锥顶，Line 可来自直线、圆柱轴、锥轴，Plane 可来自平面支撑。
- Point–Curve 与 Point–Surface 是允许组合；Curve 附注限定为有效 underlying geometry，必须把可识别种类/退化和选定 branch 转为权威 descriptor/证据，不能任意把显示折线当精确曲线。本系统以可精确求值且参数域/分支可验证的支撑为准，在能力合同测试中明确，不依赖未解释的文档附注。
- 当前精确 Curve 子类为无限 Line、Circle；修剪 Arc 必须显式选择 underlying-circle 语义，保留原修剪范围与 provenance。Surface 子类为 Plane、Cylinder、Sphere、所选叶 Cone，不声明任意 NURBS。Point–Cylinder 曲面入射是一条半径关系，不等于 Point–Cylinder-axis 的两条横向位置关系；圆上点为平面入射与半径两条独立关系。球/圆柱/圆锥曲面一般点入射各一秩，锥顶属于奇异构型，不用瞬时零空间认证有限运动。
- Frame–Frame 定义为原点与完整朝向重合，控制六个相对自由度；Frame 必须为右手正交刚体坐标系，不把反射当旋转。Frame 与其他元素组合时必须显式选择其原点、轴或基准平面，再编译为点/线/面关系。例如“原点在平面上”控制一个自由度，“基准平面重合”控制三个；不提供含义不明的 Frame–Plane 隐式转换。这是对 CATIA 表/正文差异的本系统决策。
- Same/Opposite 仅在几何可定向时显示；Undefined 是用户意图，选定的离散分支是求值证据。禁止把 Undefined 永久改写为 Same；preview 内冻结 branch 防跳解，重新求值按显式策略选择并报告分支。

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

合同未列出的组合返回 unsupported，不将其称为数学上不可能；后续可按同一合同扩展。Sphere–Cone 统一定义为环相切：球心位于锥轴、球面与所选锥叶的母线相切，接触环半径必须非零，材料侧与锥叶显式。球锥不能因转置图标而解释为整片曲面重合。Sphere–Circle/Cone–Circle 的环接触表示整条所选圆位于对应支撑上，明确区别于两个曲面的相切。

每个接触分支冻结解析定义、参数域、独立秩及退化处理。几何对是无序的，交换选择只变换定向参数/证据，不能改变可行位姿集合。相等半径/锥角导致接触类型或秩变化时显式切换定义或报告退化，不能继续使用通用距离残差并谎报自由度。以上规则直接解决来源图标疑点，不以获取 CATIA 运行环境为前置。

解析 descriptor 长度一律毫米、角度弧度；Quantity 源值保持 SI，在 Quantity 编译边界转换。Plane 法向已经是变换后的材料外法向，不再乘第二次材料符号；Cylinder/Sphere/Cone 的规范径向法向乘 `materialSide=±1`。External 要求相触外法向相反，Internal 要求相同；该侧别与 Coincidence 的 Same/Opposite、OFFSET 正负无关。Cone 使用有效叶轴 `A=leaf·axis`，`leaf=±1`，半角 `0<α<π/2`，Circle 半径严格正。

以下秩为一个参考运动单元固定、另一个可动的非退化构型；方程条数可以超过独立秩。轴平行/重合初态必须可通过显式分支初值进入合法线接触，不能仅因初值奇异永久拒绝合法目标。

| 分支 | 解析关系 | 一般独立秩 |
|---|---|---|
| Plane–Plane face | 法向按材料侧关系平行，平面位置相同 | 3 |
| Plane–Cylinder line | 柱轴方向平行平面，轴到平面的定向距离为分支半径 | 2 |
| Plane–Sphere point | 球心到平面的定向距离为分支半径 | 1 |
| Cylinder–Cylinder line | 轴平行，轴间距按材料侧取半径和/差；零差转入 face/退化诊断 | 3 |
| Cylinder–Cylinder face | 半径相等、同轴、材料侧兼容 | 4 |
| Sphere–Sphere face | 半径相等、球心相同、材料侧兼容；不是球球外切 | 3 |
| Sphere–Cone ring | 球心在有效叶轴上，高度 `R/sin α`，非零接触环半径 `R cos α` | 3 |
| Sphere–Circle ring | 圆心相对球心沿圆法轴的高度 `branch·sqrt(R²-r²)`；`r>R` 不可行，`r=R` 高度零仍三秩 | 3 |
| Cone–Cone line | 叶轴夹角按材料侧为半角和/差，共同母线 `g·Aᵢ=cos αᵢ`，锥顶差平行母线 | 3 |
| Cone–Cone face | 半角相等、锥顶相同、有效叶轴相同、材料侧兼容 | 5 |
| Cone–Circle ring | 圆法轴平行有效叶轴、圆心在轴上，高度 `r/tan α` | 5 |

整圆支撑分支不额外发明 Circle 材料法向接触门。曲面 face 与 Sphere–Cone 同规范法向位置关系要求曲面材料符号：External 乘积为负，Internal 为正。Plane 与曲面距离分支使用曲面材料符号和侧别计算，选择交换只交换角色，不改变几何可行集合。不可行常量条件与可恢复初态退化分别诊断；允许保存失败定义不意味着采用失败候选姿态。

分支符号以支持本身而非任意世界轴定义。Plane–Cylinder/Sphere 令 `h=n_plane·(C_curved-O_plane)`，必须 `h=branch·R`，其中 External 的 `branch=m_curved`，Internal 为 `-m_curved`；材料侧、支持交换与距离符号不是同一操作。Sphere–Cone 为 `C_s-O_c=(R/sinα)(leaf·axis)`；给定锥叶后高度唯一，通用 ±branch 不制造另一个非法叶上的环。Sphere–Circle 为 `C_circle-C_s=branch·sqrt(R²-r²)n_circle`，反圆法向须同时变换 branch 才保留可行集合。Cone–Circle 为 `C_circle-O_c=(r/tanα)(leaf·axis)`，圆法向可同向或反向，不把它误作材料接触侧。

Cylinder/Cone line 的有效外侧由 `effectiveExternal=(side==External)==(m₁m₂>0)` 决定，分别选择半径和/差或半角和/差；选定锥叶先转为有效轴再比较夹角。Cone–Cone line 的共同单位母线满足 `g·A₁=cosα₁`、`g·A₂=cosα₂`，锥顶差与 g 平行，在两个选定叶上用有限点/切平面检查接触；轴平行初态是可恢复初值退化，等半角差零则是线分支退化为 face，不能混淆两者。Circle/Sphere 正半径和 Cone 开区间半角是前置条件，端点/奇异构型不由一般秩或一次线性零空间认证有限运动。

属性页 `cfyugasmrf0501.htm` 的线接触方向图 `images/rf020NLS.gif` / `rf021NLS.gif` 明确为 Internal/External；它们不是 Coincidence 的 Same/Opposite。点接触方向由默认规则确定。Contact 对定向支撑的内部/外部侧有要求；普通无定向 Surface 不能冒充实体面。Circle 按表中指定的环接触特例验证支撑来源。持久定义保存工程支撑、材料侧与 branch，精确 descriptor 提取须带 provenance。

接触作用于无限数学支撑，结果可位于可见修剪区域之外。不能偷偷增加有限面重叠条件，也不能用 mesh collision、最近三角形或力学接触替代。每个适用组合都覆盖选择顺序、内外侧、多解、相等半径/锥角、退化与不可能输入。

### Offset 与 Measure

Point/Line/Plane 的全部六种无序组合均须支持。至少一方为平面时才能定义正负号，实体面以材料外法向为正；双平面以第一选择的法向定义正号，selection order 属于持久意图。无平面的点点、点线、线线距离不能借任意世界轴制造 signed offset。

明确符号函数：在 owning Product 坐标中，`d = n · (p_first − p_second)`。双平面 `n = n_first`；只有一个平面时取该平面定向法向，不因为它在第二位置就交换支持。`p` 分别是精确点位置、无限直线支撑原点或平面支撑原点。Line–Plane 驱动要求直线平行平面；Plane–Plane 驱动要求平面平行（Same/Opposite 指定法向关系，Undefined 保留二者的并集）。这些前置关系成立时距离不依赖支撑原点的任意切向选择；测量时不成立则不可测，不能取有限面/边最近距离。Datum 使用持久定向法向，实体面使用经 occurrence 刚体旋转变换的材料外法向，位置同时接受旋转和平移。

交换选择：单平面 `d_new = -d`；双平面令 `n_second = s n_first`，则 `d_new = -s d`，Same 的 `s=+1`，Opposite 的 `s=-1`。Undefined 需要已解析分支证据，不猜测统一取负。反转定义法向使对应符号取反并变换定向关系；仅修改 Same/Opposite 不静默修改用户距离。等价变换比较可行集合，不要求 reference/moving 偏好选中相同位姿。无平面距离非负、交换不变，直线均为精确无限支撑。

双平面的 Undefined/Same/Opposite 与偏移符号是不同参数。Measure 模式只测现有 placement，不产生驱动方程；两非平行平面不可测时返回稳定无效诊断/无数值，不显示旧值或伪零（CATIA 显示 `(##)`）。切换模式、表达式数量、重命名、Undo/Redo、刷新和 Release/replay 必须保持定义与测量结果的边界。

驱动 Quantity/表达式必须求值为有限长度，稳定参数引用不因显示标识重命名断裂；非法维度、循环及无符号负值原子拒绝。Measured 保留驱动 Quantity/表达式，输出独立测量值，恢复 Driving 不把测量值写回目标。用户单位只在明确边界转换，支持顺序、符号合同版本、参数源与评价证据随历史冻结；旧记录不能在读取时被悄悄重释。

### Angle：空间角、指定轴角与方向关系

角度数量统一支持用户输入 0–360°；静态姿态的 360°与 0°等价，内部规范化为 `[0, 2π)`。不复制 CATIA 的 ≤90°输入加四个 sector 的表达方式。Angle 与 Offset 提供 Driving/Measured 切换；平行/垂直是关系类型，不冒充角度数量测量。停用恢复须保留原模式。

**无轴空间角**只约束两有向支持的夹角：`α = atan2(||a×b||, a·b)`。目标 θ≤180° 使用正向分支，θ>180° 使用反向分支；不冻结叉积方向，不把任意辅助轴作为几何定义。正常构型仅限制一个转动自由度，法向可绕另一法向沿圆锥运动，允许与相容的线重合等约束组合。0°/180°/360° 的可行集合为平行/反平行，驱动方程按方向对齐处理，秩为二。

仅凭两个方向的点积无法区分正反解，因此无轴角还保存由初始姿态确定、随接受姿态运输的隐式扇区方向 `s`。角度使用 `atan2(sign(s·(a×b)) ||a×b||, a·b)`，不将 `s·(a×b)` 的大小当作正弦，不添加绕 s 的对齐约束。90°和270°必须选择相反姿态；已有正交构型切换方向时需要改变位姿，不能仅改标签。分支独立于数值 warm start，随定义/求值证据进入 manifest 和补偿历史；共线初始构型用第二支撑局部框架中的确定方向播种。求解后的分支运输允许连续运动而不将初始化方向固定成旋转轴。分界处按选定局部扇区计算导数，几何夹角及约束组合仍须独立验证。

**有向绕轴角**使用两条有向支持 `a,b` 和显式单位参考轴 `k`。令 `u = a − (a·k)k`、`v = b − (b·k)k`，在两个投影都非零时定义：

```text
θ = atan2(k · (u × v), u · v) mod 2π
```

参考轴和方向来自稳定 Datum/Publication/PersistentSelection，持久化来源、局部方向及正反意图；无向边/线经显式定向后也可使用。不能从相机或每帧 `a×b` 猜轴。交换 `a,b` 或反转 `k` 应按合同变换目标角；无有效轴或投影退化时返回明确诊断。残差使用周期一致的局部角差与 branch 管理，不能简单相减导致 359°→1°跳成 −358°。

该角一般贡献一个独立标量方程，仅控制投影后的方位角，**不隐含两方向都垂直于参考轴，也不意味着角为 0°时两空间方向平行**。需要平面内角时组合方向与轴垂直的关系；需要铰链角时先用同轴关系控制倾斜，再以绕轴角控制余下转动。UI 应显示“绕所选轴的角”与参考方向，不将其误标为无参考轴的空间夹角。

- Parallel：两方向平行，正常构型控制两个转动自由度；Same/Opposite 指定朝向，Undefined 保存意图及求值分支。不能用一个退化的 `dot=±1` 标量梯度代表两个独立约束。
- Perpendicular：两方向点积为零，正常构型控制一个转动自由度；界面提供正向90°/反向270°，保存方向意图，正反向须选到相反姿态，编译为带扇区分支的空间角而不增加轴。Parallel 与 Perpendicular 提供独立工具栏入口。与“投影绕轴角等于 90°”明确区分。
- 平面以法向作为方向支持；线面关系必须注明“线与法向”或通过明确的组合表达“线在平面内”，不静默互余转换。空间角和指定轴投影角均可作为驱动或测量数量，界面明确区分。

静态 Revision 保存规范角、轴和方向意图；连续角、累计圈数与 warm start 属于 preview session，跨 0/180/360°保持连续。不引入持久多圈传动约束。参数范围扩大本身不改变独立约束秩；自由度覆盖由前述构造及 conformance corpus 证明。CATIA 的角度输入域疑点据此关闭为设计差异，其 1 μm/1 μrad 阈值不自动改写平台容差。

### Fix 与 Fix Together

空间 Fix 在 owning Product 的坐标框架固定捕获 pose，嵌套 Product 移动不把子件错误钉到根世界坐标；提供位置/姿态参数编辑，显示角参数与内部 quaternion 转换必须可重放。

相对 Fix 以用户显式移动后的组件位置作为后续装配 update 的保留基准，使相关组件随之求解；空间 Fix 则在 update 回到固定位置。相对 Fix 既不是“无约束”，也不能偷换为两体 Rigid 或临时弱权重。用 CATIA `ut0305` 的“移动 → update”双模式样例冻结基准更新时间、手势确认、Undo/Redo 与欠约束解选择。

Fix Together 支持多成员、增删成员、命名及已有组参与；组保存稳定成员关系，不以数组位置为身份。根据 `ut0306`，组内已有约束先求解，再将更新后的组作为整体求组外约束；不能提前硬合并 rigid cluster 导致合法组内约束被误判冲突。重叠/嵌套组的环、重复成员与层次顺序必须明确定义和验证。用户操作一个成员时按尊重约束策略移动关联组；显式自由移动的行为与提示另行定义，不隐式解除组关系。

成员按稳定 occurrence/组身份规范化，声明顺序不改变组身份；嵌套组形成有向无环依赖，循环拒绝。重叠组的传递闭包合并为一个确定性内部求解阶段，各组仍保留自己的 ID、名称与成员定义。内部关系要求所有参与运动单元都属于该阶段，包含指定轴投影角的独立第三参考轴；轴在组外时该关系属于外部，不能向仅含组成员的求解请求发送缺失的轴。内部参考轴的精确 descriptor 也进入捕获输入身份。

首次建立、成员或内部定义显式变化时，先求解有效内部约束，仅成功结果可生成新的相对关系；普通读取和输入未变的更新不重新捕获。模式切换等合法内部输入变化可以改变捕获输入 digest，不意味着物理关系必须漂移。内部或外部失败都不能采用失败姿态或覆盖既有捕获；组停用释放组关系，不改变独立内部约束的激活状态。正常固联的相对限制为 `6·(N−1)`，与已有内部限制重合时按独立 Jacobian 秩处理，不重复扣除自由度。

## 激活、模式、连接和求值状态正交

Activate/Deactivate（抑制）控制该定义是否参与更新，作用于六类约束以及 Fix Together 整组。抑制不是删除、隐藏或 Measure：

- 保留稳定 ID、完整参数、支持元素、组成员和原 Driving/Measured/Controlled 模式。持久激活状态与求解模式分开表达；若内部仍使用 `SUPPRESSED` 枚举，adapter 必须无损保存恢复前模式，不能激活后一律 Driving。
- 单个/批量切换通过 versioned Domain Command、原子 ChangeSet、CAS、幂等和 Undo/Redo；不触发零件 Feature 重算，只使受影响装配 component、DOF、manifest/结果证据失效。
- 非激活约束不进入硬方程、rigid-cluster 合并、rank/DOF 限制或活动冲突集合。组停用释放其组关系，但不删除或自动抑制组内独立定义的约束。
- 停用对象保留可检查的引用状态；来源缺失仍可显示 NotConnected，但不以其 Broken 阻止其余活动约束求解。激活前重新解析当前快照，不能复用停用前 Verified。恢复时重新检查支持与兼容性；允许保存 Broken/Impossible/NotUpdated 定义，失败不写入候选姿态，后续抑制、重连或编辑可恢复。
- Deactivated、Connected/NotConnected、NotUpdated/Broken/Impossible/Verified、Measure 是不同维度。树、属性、视口符号和分析计数都需显示停用；停用标记不抹去诊断证据。
- Manifest/Release 冻结全部定义与激活状态，参与 solver 的 active set 可重建。Release gate 对活动约束要求 Verified，对停用定义显式记录排除理由；停用不伪造 Verified，也不使旧 Release 随新激活状态变化。

## 尊重约束的连续三维操纵

对标 `ut0403` 的轴向平移、平面平移、轴向旋转和由几何指定方向/轴。默认受约束编辑只在所有活动硬约束允许的流形上移动，固定/固联/抑制/测量模式都参与正确的自由度解释。

实时不是每个 pointermove 提交 Revision。浏览器响应手势，服务端用不可变 Manifest 输入和 session branch/warm start 连续求解；请求合并、取消、背压和过期响应门保持有效。不可达目标返回最近可行 pose 与 blocked feedback；基础设施失败保留最后确认帧，不显示假成功。pointerup 等待最终确认，只形成一次版本化移动，Esc/cancel 不提交。具体交互性能预算与测量场景由[装配主线](../../../plans/README.md)维护。

约束定义的合法性与求解可满足性分离：Impossible 仅用于几何与单个约束定义不兼容；组合冲突、过约束或数值未收敛使用 NotUpdated 并保留求解证据。结构树允许保存这些定义，抑制与激活可用于修复；旧失败状态不得永久阻止重新参与求值。无向关系允许两个方向分支，不能以首次添加时的姿态冻结其可行解集。

已求解约束集合与待接纳定义分离。添加或修改导致组合冲突时保留原已解集合，仅将不能接纳的定义置为 NotUpdated 并排除出运动方程；已有可满足的冗余方程无需仅因秩冗余而失败。拖动不承担重试隔离定义的职责；显式重算、定义修改、删除、抑制/激活后按确定顺序重新尝试接纳。用户 Suppressed 与求值隔离是两个维度，不通过偷偷抑制来实现过滤。

### 连续操纵与局部诊断合同

求解 Session 是有容量、TTL、取消与并发保护的瞬态计算上下文，不是 工作台 EditSession 或持久连接模型。它冻结 actor、Workspace/Head/sequence、owning Product/完整 occurrence 与编辑上下文、已接受引用、定义/模式/激活集合、精确支持、组捕获、solver policy/profile/build 和手势 nominal baseline。accepted pose/branch 仅用于后续 initial guess；失效、取消、未接纳或迟到结果不能运输它们。外部 Head/相关输入或上下文变化要求重新开始，不能自动追随新 Head。

DragTarget 的 pose、抓取点目标与参考帧均属于 owning Product（长度毫米、角度弧度）。抓取点为 `R * localGrabPoint + t`；目标为 `R_target * localGrabPoint + t_target`。冻结 frame 的驱动/保持 mask 互斥，未落入两组者完全自由；保持是交互偏好，不升级为物理方程。层级为：实际接纳硬约束与分支可行 → 用户驱动目标的局部最优 → 保持未要求改变的姿态/侧向抓取点 → 剩余 total nominal motion。可行自由平移应保持完整姿态，但机构必要的耦合旋转不能被禁止。旋转目标包含绕 pivot 的位姿轨迹。统一尺度中的零空间必须正交化；每个切步做真实硬约束恢复和上层复验。无 Ground 时保留物理整体运动，不把数值 gauge 写成 Fix。静态 ADD/EDIT 和旧策略仍按原 reference/total 层级解释。

用户目标始终来自冻结 baseline 与累计输入；已接纳结果只作 continuation guess。可行显示、合格 checkpoint、warm state 和最终候选分开。短暂预算不足不丢弃整个手势；最终目标可有界续算，但目标身份与传输 attempt 分离，同一目标身份不得改变参数。最终仍未确认时保留明确未保存的预览并提供重试/取消，禁止静默提交早期目标。开始其他命令/目标先终结或取消未提交状态；外部版本失效恢复当前权威状态而不是旧 nominal。

SPACE Fix 保留捕获基准。显式 Move 可编辑被驱动运动单元及其既有刚性组成员的 RELATIVE 基准：开始时冻结授权 Fix ID 集合，交互编译明确把这些基准视为本次编辑参数，其他硬约束不变；合格最终帧一次原子写入新基准和所有受影响姿态。不是 admission 隔离或用户 suppression，不在数值迭代中重捕获；普通求值、测量、失败、取消不能改变持久基准。组捕获关系在整个手势中不变。

反馈分别报告硬约束可行、目标到达/受限局部最优、优化预算/未知、数值失败与版本失效。“最近可行”只限定于本分支局部收敛证据；瞬时自由度不是有限运动证明。预算帧可反馈最后可行状态，但不可作为合格最终候选。最终候选绑定最新目标、完整冻结上下文、实际证据与 policy；中间帧零 Revision，无变化零提交，有变化一段手势原子提交一次 Move。CAS、receipt 重试、Undo/Redo 和冻结 Manifest 沿原体系；提交结果未知先确认 receipt，不能用恢复本地 baseline 冒充业务撤销。

每 Session 一个在途求解和一个最新未发目标。连续输入合并而不反复取消在途工作；属于有效 Session 的顺序递增可行帧仍可反馈，不因存在更晚待发目标饥饿。pointerup 必须另行 flush 最终目标。异常 capture 丢失、Esc、取消、blur/切换取消；正常 pointerup 的 release 不触发异常取消。实例、组、glyph、手柄按同一权威帧投影，不重新生成几何。

局部诊断分析冻结 Revision/输入、目标隔离定义与完整相关 incidence 邻域（含物理 Ground、闭环、第三轴及组生成关系）；probe 固定方程集，禁止使用 admission 自动缩减后的成功证明原集合 SAT。分析只读、预算与取消有界，不修改生产 warm start、branch、基准、激活或历史。Suppressed/Measured 不进入活动硬冲突；来源、参数及基础设施故障单独解释。

oracle 是三值：SAT 需要独立几何见证；UNSAT 需要范围明确的解析/结构不可满足证据；未收敛、驻点、证书未覆盖或预算不足为 UNKNOWN。当前未满足、冗余、退化、localized suspect、解析不兼容与 verified irreducible 分开。只有原集合可靠 UNSAT 且必要删除集都有 SAT 见证，才声明背景/分支范围内不可约；不是最小基数。结果提供 Constraint/Group/Equation 与支持来源、版本和合法的既有修复命令，用户决定是否编辑、停用、测量或重连。修复后旧版本诊断失效，鼠标不可达目标不触发冲突搜索。

Engineering Connections 暂缓；上述数学有限运动和 Frame/Contact/组不创建 Connector、Joint limits、gear/rack 或动力学实体。
