# 装配主线：六类约束与自由度组合、激活状态与实时操纵

> 状态：待验收（M4/M5 第一阶段实现及自动收口已完成，等待维护者实机验收）；六类主要能力及最近候选/图标修复已有维护者反馈，不能扩展为本轮验收。已交付六类、Frame、Contact、组及自由度解释不重新领取。实现、实际验证及性能限制见[当前架构](../docs/architecture/current/product-assembly.md#m4m5-第一阶段连续操纵与局部解释)，返回[统一路线](README.md)。

## 交付目标与当前差距

先把 **Coincidence、Contact、Offset、Angle、Fix、Fix Together** 六种用户约束完整落到模型、求解、参数编辑和状态，再交付尊重约束的连续三维操纵，随后强化冲突解释与 Engineering Connections。六类的准确合同、几何矩阵及本机来源页面统一见[六类约束合同](../docs/architecture/target/assembly-constraints.md)，本页只维护工作分解、依赖与验收。

本轮公共 v2 定义已统一为六族；Concentric/Distance/Parallel/Perpendicular/Rigid 保留为编译原语或明确快捷入口，不形成另一套活跃用户模型。真正的多成员固联及解析 Contact、有限精确 Curve/Surface/Frame 支持和来源/生命周期链已实现，不再列为从零实施的缺口。实现、自动执行证据和人工验收分开记录：目录/报告反映逐项实际结果，人工待验收不等于代码未实现。

代码核对入口：`assembly_interaction.go`（版本化 Session/最终候选）、`assembly_conflict.go`（只读诊断）、`assembly_solve.go`（六类编译）、`workers/geometry/src/main.cpp`（纯值协议）与 `kernel/assembly/src/solver.cpp`（分层目标/方程）。新交互不注入 interaction-driver Fix；普通非 Session MOVE 保留既有兼容语义，不作为 M4 入口。

## 六类约束与生命周期补齐

本轮九个 CONSTRAINT-* 的实施内容已归入[当前实现](../docs/architecture/current/product-assembly.md#六类公共定义精确支持与生命周期)与[可执行目录](../tests/assembly-contract/README.md)。以下仅保留稳定标识、现有锚点及待验收出口，不复制架构或另建手工覆盖矩阵。若自动报告暴露真实失败、必需测试缺失或环境阻塞，应按 capability/case 修复并重跑，不能用代码存在或拒绝测试通过替代验收。

状态：**待验收（未覆盖场景与本轮新增交互）**。六类已实现及既有自动证据见[当前记录](../docs/architecture/current/product-assembly.md#constraint-composition-本轮收口)，不再领取已完成代码任务。维护者反馈主要能力可用不代替逐项、工业或本轮 M4/M5 验收；新增交互实机验证单列。

### CONSTRAINT-ACTIVATION：激活/取消激活（抑制）完整闭环

人工退出门：单项/批量停用恢复、Measured→停用→恢复 Measured、Fix 释放自由度、组停用保留独立内部约束、Broken 重新解析、空活动集合、Undo/Redo 与旧 Release 状态可读。对应自动证据由目录驱动，不再领取状态模型重建任务。

### CONSTRAINT-GEOMETRY：支持元素和精确 descriptor

人工退出门：真实面/边/点、Datum、Publication 与稳定派生支持能定位/高亮；精确类型和单位可读；共享 Part、多 CAD Body 及 nested occurrence 不串源；来源变化/删除/歧义与显式 Reconnect 可解释。有限类型和参数域按目标合同，不宣称任意曲线/曲面支持。

### CONSTRAINT-COINCIDENCE：重合族与 Undefined 分支

人工退出门：基础点/线/面、同轴、有限精确 Curve/Surface 和 Frame–Frame 创建/替换/编辑可用，Undefined 意图与求值分支可区分，交换与反向不静默翻解；Frame 子元素按显式角色选择。同轴不错误要求圆柱半径相等。连续拖拽分支运输另属 M4。

### CONSTRAINT-OFFSET：完整偏移与测量模式

此前首切片与维护者反馈保留在[当前 Product](../docs/architecture/current/product-assembly.md#offset-有符号纵向切片)。平行 EDGE 零目标/交线与 Axis–Plane 法向对齐后 Driving 恢复的已知数值问题已进入本轮修复和专用回归，不再默认留为下一轮开发任务；实际验证按报告，不把几何满足替代偏好收敛。

人工退出门：六种组合创建/编辑/Preview 的精确支持、正负/零值、第一法向和方向独立性、稳定 Quantity 表达式、Measured 不驱动/不可测清旧值、来源重连、恢复原模式、符号回填及取消/快速切换正确；历史旧符号不被重新解释。维护者此前的通过反馈不自动认证这些新增交互。

### CONSTRAINT-ANGLE：角度参数族

人工退出门：FREE/DIRECTED/Parallel/Perpendicular 的参数、模式与支持替换；独立第三 occurrence 参考轴、反轴/交换与稳定表达式；端点/退化诊断、Measured 和 Undo/Redo 正确。无轴空间角、投影角与空间平行/垂直不可混用。静态分支与 session winding 分开；359°→1°连续拖拽和圈数运输归 ASSEMBLY-BRANCH，不作为本轮已实现交互。

### CONSTRAINT-FIX：空间固定与相对固定

人工退出门：SPACE/RELATIVE 的 owning Product 捕获、位置/姿态编辑、外层 Product 移动与显式内部移动/update、Preview 取消、抑制恢复和 Undo/Redo；普通求值或测量不能改写 SPACE 基准。本轮 Session 仅在授权的显式 Move 最终提交更新 RELATIVE 基准。

### CONSTRAINT-FIX-TOGETHER：多成员固联组

人工退出门：稳定组 ID/名称、2/3/N 成员及增删/重排、已有组参与、嵌套/重叠/循环诊断、整组激活、树/Inspector/高亮、内部编辑后外部整组运动和解散；失败候选不能覆盖捕获关系，组停用不自动删除内部独立约束。组内先解/组外后解已实施，普通 pair Rigid 仍不能作为组生命周期验收证据。

### CONSTRAINT-CONTACT：完整解析接触矩阵

人工退出门：目标 11 个解析分支的创建/编辑/Preview/取消、材料侧与 Internal/External、交换与分支、半径/锥角不兼容的可读拒绝、来源 Reconnect、激活和历史状态。球球 face 不替代为外切、整圆支撑不冒充普通零距离。任意 NURBS 接触、动力学和碰撞响应不在本轮范围。

### CONSTRAINT-COMPOSITION：组合能力与六类纵向验收门

自动退出门已通过：现有完整目录的 baseline/gaps 与严格 composition 检查各 617 PASS，必需层有专用证据、无未解释失败/缺测/环境阻塞；独立几何、Jacobian/秩、0–6 及关节有限运动、组内外阶段、来源/生命周期、冷重放与 Release 按具体 case 认证。准确结果读取 `build/constraint-composition/final-composition/report.json`，报告是实际执行派生物，不另维护一张手工全矩阵。后续目录变化继续运行同一门防退化；此处只保留下述人工退出门。

人工退出门：按[可复现场景与检查步骤](../tests/assembly-contract/README.md)确认六族、组、取消/迟到响应及来源恢复的真实交互，维护者反馈逐批记录，不由 Agent 代签。自动门通过与人工待验收可以并存；CONSTRAINT-COMPOSITION 不等于 M4 最近可行连续拖拽交付。

## Assembly M4：尊重约束的连续实时操纵

### ASSEMBLY-SESSION：版本化 session 与 branch snapshot

第一阶段实现已归入[当前架构](../docs/architecture/current/product-assembly.md#m4m5-第一阶段连续操纵与局部解释)，本节仅保留稳定任务标识与实机退出门。确认取消/过期、外部 Head/参数/模式/激活变化失效，冻结 baseline 与 warm start 不混用，迟到结果不能覆盖新状态。

TREE-03 的 Workbench `EditSession` 只负责宿主与编辑目标，不是这里的约束求解 Session；CAD Body 的历史/求值归属也不等于 solver body 或运动学刚体。已有 Preview、null-space、自由度显示不构成 M4 完成证据。

实机退出门：正常 pointerup 与异常 capture 丢失区分；取消不写历史、不用旧 baseline 覆盖外部 Head；断线后区分未提交手势和提交结果未知。

### ASSEMBLY-DRAG：约束流形上的拖拽目标

实机退出门：单一手柄的轴向/平面平移、绕轴旋转与自动精确几何吸附/定向可用，中心重定位不移动零件或写历史，当前已接纳硬约束保持，不可达目标直接表现为受限可行运动；非原点抓取、组和整体自由运动不被隐藏 Fix 限制。局部最优不宣称全局最近点。

尊重 Fix 模式、Fix Together 分阶段关系和 suppressed/Measure 的非驱动语义。不可达目标只产生受限结果，不使整个操作随机翻转 branch。若提供 CATIA 式不尊重约束的自由操纵选项，必须明确为自由预览并经正常 update/commit 验证；不能自动取消约束或持久提交声称 Verified 的非法姿态。

### ASSEMBLY-FEEDBACK：连续反馈与请求控制

- 浏览器手柄/鼠标反馈保持连续；任何近似本地预测必须区分于权威 accepted pose，不进入 Revision。
- 合并中间 pointermove，每个 session 最多一个在途求解和一个最新待处理目标；不中途反复取消造成反馈饥饿。Session generation、sequence 与 base digest 拒绝迟到响应，pointerup 的最终目标不能被丢弃。
- Instance、同组成员与手柄应用同一权威帧；正常受限不弹警告或显示常驻算法 HUD，内部状态保留。真实故障有统一反馈，网络延迟/断线保持最后确认帧和诊断，未知提交可查询原回执。
- pointerup 等待配对的合格最终结果后提交一次 Move；Esc、pointercancel、异常 lost capture、blur 取消临时覆盖，不覆盖外部新 Head。无可行变化不写空 Revision，结果未知按 receipt/requestId 确认。

验收：长拖拽不堆积请求，快速反向、旋转跨角边界、网断重连、重复/乱序/迟到响应、约束中途停用/激活、多人 Head 改变、整组移动、完全固定和欠约束闭环均有自动场景及真实浏览器验收。

### ASSEMBLY-LATENCY：测量与性能门

第一阶段已建立单体、真实50 bodies/200活动约束单连通闭环、独立组件及组/接触的固定内核测量，观察基线见当前架构与既有 baseline；不从零重领。待验收边界：优化内核连通场景P95约1.25秒，未达100ms；Debug超出Session RPC预算。排队/传输分量、输入至显示延迟、浏览器帧时间和实机取消响应仍需真实测量，不把内核数据替代它们。

候选目标：约定本机基准上普通 50-body 场景的输入至权威显示 P95 ≤100 ms，界面目标 60 Hz、主线程不因远程求解阻塞；慢/病态案例必须有有限预算、取消与诚实的 pending/受限反馈。先实测冻结预算，再以回归门约束退化；上述数值不是当前性能承诺，不以减少约束或降低残差正确性达标。优先复用冻结解析、affected component、warm start 与求解预算，不把 M7 稀疏后端或独立 Worker 强行变成前置。

### ASSEMBLY-BRANCH：六类参与的 M4 收口

静态 Revision 保存 branch intent，continuous angle/winding 留在 session。复用 CONSTRAINT-ANGLE 的轴/方向定义，覆盖 0/π/2π、不可达目标、内部固联更新、Contact 分支和模式/激活切换；所有新几何族重跑相关 drag corpus。最终门是六类全覆盖 + 实时操纵 + 激活/抑制组合验收，不是只在旧六个枚举上演示拖拽。

## Assembly M5：局部冲突解释

第一阶段只读分析、三值 oracle、有预算删除过滤和正式修复入口已归入当前架构。本节保留实机退出门；一般非线性未收敛只能 UNKNOWN/嫌疑，不当成 UNSAT，可靠矛盾才可验证不可约，不承诺最小基数。

- **CONFLICT-CORPUS**：实机核对当前未满足、可靠不可满足、验证不可约、局部嫌疑、冗余/退化及预算未知的不同标签；不提供最小基数标签，不把数值未收敛当UNSAT。
- **CONFLICT-SEARCH**：按 changed constraint/activation/group 的 incidence graph 构造活动邻域，结合 rank/branch evidence 与有预算的删除过滤/QuickXplain；不把 inactive 定义算作活动冲突，不伪造 MUS。
- **CONFLICT-UX**：按稳定 Constraint/Group/Equation ID 高亮树与视口，复用已有 Activate/Deactivate、Measure、Reconnect 命令供用户修复。此阶段增强解释，不再延后抑制基本功能。

## Assembly M6：Engineering Connections 扩展

状态：**候选；明确延期**。不设恢复日期，也不是 M4/M5 验收后的自动下一项。已有 Frame、Contact、Fix Together 和自由度解释保留；有限运动测试不新增 Connection 实体。

- **CONNECTION-FRAME**：在已支持 Frame descriptor 上增加 Connector 接口类别、对称性、极性、允许连接、名义间隙/尺寸与属性。
- **CONNECTION-BASIC**：组合既有六类方程，形成 Rigid/Revolute/Prismatic/Cylindrical/Planar Connection，以实际 freedom 验证声明类型。Fix Together 不是这一步才交付，也不等于声明一个 Rigid Connection。
- **CONNECTION-LIMITS**：Distance/Angle/Joint limits 的 active-set/branch 和交互诊断。gear/rack、任意 NURBS 接触与动力学另属候选。

原 CONNECTION-CONSTRAINTS 的 Offset/Parallel/Perpendicular、CONNECTION-CONTACT 和 CONNECTION-DESCRIPTORS 的 Circle/Sphere/Cone 已前移至六类能力批次；不重复实现、不把六类完整性依赖隐藏在 M6。

## 执行依赖与验证

当前执行路线：M4/M5 第一阶段实现完成 → 维护者实机验证 → 按反馈修复和整理。自动证据见当前架构与完整合同报告，不代替人工或性能目标验收。后续主线转向 Part Sketch/Feature/Naming，本轮不实施；M6 明确延期。六族、组和 Session 等已实现能力不重新排为从零开发批次。

`9a22fbe` 基线的快照/手柄/工程分析交互整改已实现并通过定向自动验证，见[收口记录](../docs/architecture/current/product-assembly.md#m4m5-交互收口快照统一手柄与工程分析)。剩余为维护者实机确认：删除/停用后不重选直接拖动、自动吸附稳定性与方向、受限运动安静反馈、命令 Tab 工程概览/定位修复、未知提交回执恢复；不能把此人工出口重新排为从零开发，或将自动通过当成人工通过。

`da0a0d7` 后的 M4/M5-03 收口新增显式运动意图保持、累计轨迹及未确认最终目标恢复，分析改为紧凑分页/按需详情与工作台公共浮层。自动证据与限制见[当前记录](../docs/architecture/current/product-assembly.md#m4m5-03意图保持连续恢复与紧凑分析)。本轮交互仍等待维护者实机验收，不将界面 1000 行 fixture 当作千组件求解验收；M6 继续延期。

SESSION/延迟基准可基于明确合同推进；最终 M4 验收依赖 CONSTRAINT-COMPOSITION 自动门通过，并覆盖六族/组/生命周期组合。人工交互待验收保持单列，不能虚报通过或无限保留已完成代码待办；新发现缺陷按证据回归修复。Feature/Revolve、Sketch Projection、大模型及工程维护是独立队列，不是装配主线的默认前置；Kinematics/M7/跨主机和完整多用户协作仍按候选进入条件处理。

每批按具体测试 → assembly/workspace/web 受影响域 → 定向真实集成升级，共享 Proto/模型/参数/迁移变化增加对应构建及兼容检查；实际执行和未执行范围如实记录，不以 Mock/源码检查代替真实几何或数据库。本轮未运行浏览器自动测试与无差别全量单测，人工由维护者后续执行；不清理应用数据库/制品。完成事实归入当前架构，计划删除已完成实施待办并保留未通过的验收出口。
