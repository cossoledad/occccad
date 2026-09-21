# 装配主线：六类约束与自由度组合、激活状态与实时操纵

> 状态：实施中；2026-09-21 按自洽语义与自由度控制目标更新。Product/M3 基线已[完成验收](../docs/architecture/current/product-assembly.md#accept-product-完成记录)。返回[统一路线](README.md)。

## 交付目标与当前差距

先把 **Coincidence、Contact、Offset、Angle、Fix、Fix Together** 六种用户约束完整落到模型、求解、参数编辑和状态，再交付尊重约束的连续三维操纵，随后强化冲突解释与 Engineering Connections。六类的准确合同、几何矩阵及本机来源页面统一见[六类约束合同](../docs/architecture/target/assembly-constraints.md)，本页只维护工作分解、依赖与验收。

当前 `COINCIDENT/CONCENTRIC/DISTANCE/ANGLE/FIX/RIGID` 不是 CATIA 六类的等价集合。Concentric 应归入 Coincidence；Distance 只能复用为 Offset 的部分方程；Rigid 不等于多成员、组内先解的 Fix Together。Contact、多成员固联、公共六类语义收敛和部分支持几何仍有缺口。现有类型的 Web → Domain Command → 引用解析 → active set → Release 激活/停用基础链已进入代码；完整六类与组生命周期的退出门尚未满足。

代码核对入口：`services/internal/workspace/assembly_solve.go`（能力/branch 规范化与临时 Fix）、`model.go`（约束及 mode）、`workers/geometry/src/main.cpp`（mode adapter）、`kernel/assembly/include/occccad/assembly/solver.hpp` 与 `src/solver.cpp`（模式/方程）。当前 MOVE 仍注入 `interaction-driver` Fix；已有 null-space 和预览不能算最近可行拖拽。

## 六类约束与生命周期补齐

首批已落地独立激活状态、批量命令、原模式恢复、空活动集合 manifest、预览证据提交与 Undo/Redo 求解证据；Angle 显式稳定参考轴及平行/垂直、点线/线面偏移、Fix 基准与六参数编辑、0–6 阶及常见关节有限运动测试也已进入代码。事实与测试入口见[当前实现](../docs/architecture/current/product-assembly.md#六类约束与生命周期的首批实现)。**本阶段尚未完成，不作完成标记。** 下列批次保留的是完整退出门，不能用首批通过替代六类全矩阵验收。

### CONSTRAINT-CONTRACT：六类能力与参照 corpus 冻结

- 建立六类 user definition → typed solver definition → exact descriptor → 参数可用性矩阵，覆盖创建、编辑、类型切换及选择顺序；菜单/Toolbar/属性/query 共用能力来源，未完成组合明确禁用或返回 unsupported。
- 收敛当前 Concentric/Distance/Rigid 用户入口与命令语义，保留可复用数值方程，不做永久双模型；当前未发布 schema 按唯一实现修改并空库验证。
- 为本系统每个允许组合/参数登记正常、交换选择、退化、不可行和上游失效案例；记录 CATIA 来源与主动设计差异，外部行为不是正确性 oracle。
- 采用目标合同的既定决策：球锥环相切及交换对称、Frame 显式子元素、统一 0–360°有向角。建立 `3 + 2 + 1` 位姿构造、0–6 独立秩及常见关节 corpus，验证数量、方向与有限运动；来源文档疑点不再作为阻塞。
- 同时冻结抑制与 mode 的正交表示、Fix 模式基准及 Fix Together 组内/外求解边界，再进入持久模型实现。

退出门：一份机器可执行或可直接落成参数化测试的能力矩阵，每行对应本系统数学定义、参数域、预期秩和验证案例，附来源或设计理由；版本化类型、依赖、失败提交和验证策略明确。不是只调整按钮名称。

### CONSTRAINT-ACTIVATION：激活/取消激活（抑制）完整闭环

- 支持单个和批量 Activate/Deactivate、Fix Together 整组切换；保留 ID、定义、原驱动/测量模式，Undo/Redo 精确恢复。
- 贯通模型字段/PropertySlot、typed command、服务端 validation、active-set 编译、Solver、Tree/Properties/glyph、分析计数和快捷上下文菜单。
- 抑制项不参与硬方程、rigid cluster、DOF/rank 或活动冲突；Broken 停用项不阻塞其余组件。重新激活重新解析并求解，不沿用旧 Verified。
- 模式或激活状态变化使当前 preview session、SolveManifest 和 Release candidate 失效；已发布 Release 保持冻结状态。处理“全部抑制/只有 Measure”时的空活动集合，不能触发当前 manifest 的“至少一条求解约束”假设或伪造成功求解证据。

验收：六类逐项停用/恢复、Measure→停用→恢复 Measure、Fix 释放自由度、组停用保留独立内部约束、来源删除后停用/恢复、批量原子性、CAS/幂等、连续 Undo/Redo、刷新/冷重建、旧 Release replay。新类型尚未交付时先用已有类型验证基础链，各类型交付后必须补入同一矩阵。

### CONSTRAINT-GEOMETRY：支持元素和精确 descriptor

- 复用 Point/Axis/Plane/Cylinder，补齐 Circle/Sphere/Cone/Frame 与球心、锥顶、锥轴等稳定派生引用。
- 补齐 Coincidence 表中的 Point–Curve/Surface 和坐标系特例；明确 underlying geometry 范围、支撑参数/branch 和求解查询边界。需要精确曲线/面数据时保持受限、不可变、带 provenance 的值协议，不泄漏 OCCT 类型。
- 支持解析 Contact 的有向面、内外侧、半径/锥角及环接触证据；Publication/PersistentSelection 冷解析与 Update/Reconnect 同步扩展。

验收：类型正确、单位/容差显式、frame 转换和 nested occurrence 正确，相关 source edit/delete/ambiguity 与冷重建可解释；不能靠显示网格推断精确支持。

### CONSTRAINT-COINCIDENCE：重合族与 Undefined 分支

覆盖 Point/Line/Plane 基础组合、同轴、允许的 Curve/Surface/AxisSystem 组合及方向。Undefined 意图与 resolved branch 分离，不能提交时强制改成 Same；保持 session 连续性，换源、编辑、重放时不静默跳解。当前 Concentric 能力复用为此族，不新增第七个用户类型。

### CONSTRAINT-OFFSET：完整偏移与测量模式

实现 Point/Line/Plane 的完整六种无序组合，补齐点线与线面。实现有平面时的有符号偏移、首选平面法向规则及 Undefined/Same/Opposite；无平面组合不显示无意义的 signed 控件。贯通 Quantity/表达式、Driving/Measure 和不可测诊断；非平行双平面 Measure 不显示过期值。编辑、参数绑定、抑制恢复与历史使用同一合同。

### CONSTRAINT-ANGLE：角度参数族

交付统一 0–360°有向绕轴角与独立 Parallel/Perpendicular 关系；使用稳定方向和参考轴，不引入 CATIA 的 ≤90°输入/sector 四分法。投影角只控制一个方位自由度，平行控制两个转动自由度；平面内角/铰链角由明确的基础关系组合。Angle 支持 Measure，几何含义与投影退化按目标合同处理。复用经测试符合合同的 DirectedAngle 核心。

验收：0/90/180/270/360°、359°→1°及反向连续移动、交换元素/反转轴、投影退化、平行与投影零角的区别、`3 + 2 + 1` 构造及正确 Jacobian 秩、参数修改/Undo/冷重放。静态 360°归一为 0°，session 保留连续圈数；ASSEMBLY-BRANCH 完成全链路拖拽验证。

### CONSTRAINT-FIX：空间固定与相对固定

实现默认 Fix in space、位置/姿态参数编辑及相对 Fix。用同一组件“显式移动 → update”验证：空间固定返回捕获位置，相对固定保留移动后的基准并带动相关组件；嵌套 Product 的基准属于 owning Product。无论自由预览或尊重约束预览，都不能悄悄更新空间固定定义。覆盖抑制、Undo/Redo、外层 Product 运动和不同欠约束状态。

### CONSTRAINT-FIX-TOGETHER：多成员固联组

实现稳定组 ID、命名、成员增删、已有组参与、整体 Activate/Deactivate 与用户选择反馈。按 CATIA 先解组内约束，再求组外约束；调整 rigid-cluster 编译时机，防止合法内部约束被提前合并锁死。明确重叠/嵌套组依赖与拒绝循环的诊断。

验收：2/3/N 成员、已有组并入、成员重排不改身份、内部约束修改、外部约束带动全组、删除/抑制独立约束、组停用/恢复、已约束组件加入不被直接误判 overconstrained；普通 pair Rigid 不能代替此验收。

### CONSTRAINT-CONTACT：完整解析接触矩阵

逐行实现[目标合同](../docs/architecture/target/assembly-constraints.md)中的面、线、点与环接触，不再仅把 plane/plane 和 cylinder 子集留到 M6。包括 Plane–Cylinder/Sphere、Cylinder–Cylinder、Sphere–Sphere/Cone/Circle、Cone–Cone/Circle 及相等半径/锥角限制；不适用组合明确拒绝。

验收：材料侧与线接触 Internal/External、交换选择、接触分支、无限支撑在裁剪面外的合法解、退化/不可能输入、正确 rank/DOF、来源编辑/重连、抑制/恢复及 deterministic replay。Contact 是位置关系，不能用 mesh 碰撞或动力学算法替代。

### CONSTRAINT-COMPOSITION：组合能力与六类纵向验收门

六类每一行都贯通 Web → command/history → immutable resolution → 正式 Router/Worker → solver → manifest/replay/Release；覆盖所有参数与几何组合，以及 inactive/Measure/不可能/断链状态。人工验收用可重放 fixture，检查本系统合同并记录参照差异。必须通过 0–6 独立秩、球铰/平面副/圆柱副/转动副/移动副/固定关系的运动方向和有限运动验证，以及奇异、冗余、冲突和抑制恢复案例。数学构造不替代端到端验收；存在未实现合同组合或仅有隐藏按钮时不得标完成。

## Assembly M4：尊重约束的连续实时操纵

### ASSEMBLY-SESSION：版本化 session 与 branch snapshot

Session 绑定 Workspace Head/sequence、M3 digest、完整激活集合/模式、accepted pose、branch 和 warm start。开始手势冻结 nominal baseline，取消/到期可丢弃；其他用户提交、参数/模式或激活变化使 session 失效，不能把旧响应应用到新约束集。

### ASSEMBLY-DRAG：约束流形上的拖拽目标

利用 M2/M2.5 null-space，以活动硬约束可行为首要门，再在允许流形上逼近鼠标目标，替换临时 `interaction-driver` Fix。支持轴向/平面平移、绕轴旋转、几何指定方向/轴；输出最近可行 pose、残差及 allowed/blocked direction。

尊重 Fix 模式、Fix Together 分阶段关系和 suppressed/Measure 的非驱动语义。不可达目标只产生受限结果，不使整个操作随机翻转 branch。若提供 CATIA 式不尊重约束的自由操纵选项，必须明确为自由预览并经正常 update/commit 验证；不能自动取消约束或持久提交声称 Verified 的非法姿态。

### ASSEMBLY-FEEDBACK：连续反馈与请求控制

- 浏览器手柄/鼠标反馈保持连续；任何近似本地预测必须区分于权威 accepted pose，不进入 Revision。
- 合并中间 pointermove，每个 session 最多一个在途求解和一个最新待处理目标；新目标取消过期计算，request sequence 与 base digest 双重拒绝迟到响应。pointerup 的最终目标不能被节流丢弃。
- Instance、同组成员与手柄应用同一权威帧；展示允许/阻塞方向、计算中、受限及失败，不靠颜色猜语义。网络延迟/断线保持最后确认帧和诊断。
- pointerup 等待最终权威结果后提交一次 Move；Esc、pointercancel、lost capture、blur 恢复 baseline，不产生历史。无可行变化不写空 Revision。

验收：长拖拽不堆积请求，快速反向、旋转跨角边界、网断重连、重复/乱序/迟到响应、约束中途停用/激活、多人 Head 改变、整组移动、完全固定和欠约束闭环均有自动场景及真实浏览器验收。

### ASSEMBLY-LATENCY：测量与性能门

建立固定的单组件、50 bodies/200 constraints 连通组件、多个独立组件，以及接触/固联/病态闭环 corpus。记录硬件/build、总量/活动量、transport/排队/求解/显示耗时、P50/P95、求解次数、取消响应和浏览器帧时间。

候选目标：约定本机基准上普通 50-body 场景的输入至权威显示 P95 ≤100 ms，界面目标 60 Hz、主线程不因远程求解阻塞；慢/病态案例必须有有限预算、取消与诚实的 pending/受限反馈。先实测冻结预算，再以回归门约束退化；上述数值不是当前性能承诺，不以减少约束或降低残差正确性达标。优先复用冻结解析、affected component、warm start 与求解预算，不把 M7 稀疏后端或独立 Worker 强行变成前置。

### ASSEMBLY-BRANCH：六类参与的 M4 收口

静态 Revision 保存 branch intent，continuous angle/winding 留在 session。复用 CONSTRAINT-ANGLE 的轴/方向定义，覆盖 0/π/2π、不可达目标、内部固联更新、Contact 分支和模式/激活切换；所有新几何族重跑相关 drag corpus。最终门是六类全覆盖 + 实时操纵 + 激活/抑制组合验收，不是只在旧六个枚举上演示拖拽。

## Assembly M5：局部冲突解释

- **CONFLICT-CORPUS**：六类及内部/外部组关系、冗余、矛盾、branch、容差与退化，区分 proven minimal、irreducible、localized suspect、unsatisfied、budget exhausted。
- **CONFLICT-SEARCH**：按 changed constraint/activation/group 的 incidence graph 构造活动邻域，结合 rank/branch evidence 与有预算的删除过滤/QuickXplain；不把 inactive 定义算作活动冲突，不伪造 MUS。
- **CONFLICT-UX**：按稳定 Constraint/Group/Equation ID 高亮树与视口，复用已有 Activate/Deactivate、Measure、Reconnect 命令供用户修复。此阶段增强解释，不再延后抑制基本功能。

## Assembly M6：Engineering Connections 扩展

- **CONNECTION-FRAME**：在已支持 Frame descriptor 上增加 Connector 接口类别、对称性、极性、允许连接、名义间隙/尺寸与属性。
- **CONNECTION-BASIC**：组合既有六类方程，形成 Rigid/Revolute/Prismatic/Cylindrical/Planar Connection，以实际 freedom 验证声明类型。Fix Together 不是这一步才交付，也不等于声明一个 Rigid Connection。
- **CONNECTION-LIMITS**：Distance/Angle/Joint limits 的 active-set/branch 和交互诊断。gear/rack、任意 NURBS 接触与动力学另属候选。

原 CONNECTION-CONSTRAINTS 的 Offset/Parallel/Perpendicular、CONNECTION-CONTACT 和 CONNECTION-DESCRIPTORS 的 Circle/Sphere/Cone 已前移至六类能力批次；不重复实现、不把六类完整性依赖隐藏在 M6。

## 执行依赖与验证

推荐串行顺序：CONTRACT → ACTIVATION → GEOMETRY → COINCIDENCE → OFFSET → ANGLE → FIX → FIX-TOGETHER → CONTACT → COMPOSITION → SESSION → DRAG → FEEDBACK → LATENCY → BRANCH → M5 → M6。

这是默认领取顺序，不是所有算法的硬依赖：CONTRACT 完成后 SESSION/延迟基准可与六类补齐并行；几何族可按消费方分批贯通，Fix/activation 不必等全部曲面 descriptor；但 CONSTRAINT-COMPOSITION 必须等六类与生命周期全部通过，M4 最终验收必须等 COMPOSITION。Feature/Revolve 和 Sketch 投影不是这条主线的强制前置。

每批按具体测试 → assembly/workspace/web 受影响域 → 集成升级，公共 Proto/迁移/Router 修改运行全仓并验证空开发 schema。必须覆盖正常/退化/失败、方向/选择顺序、单位、确定性、Jacobian/rank/DOF、Undo/Redo、冷重放和迟到结果；交互另做重启后的真实浏览器验收。完成事实归入当前架构，计划删除已完成条目。
