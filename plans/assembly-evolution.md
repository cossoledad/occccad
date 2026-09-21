# 装配主线

> 状态：待实施。M3 manifest/replay 已有实现；Product 真实浏览器基线尚待[验收](product-acceptance.md)。返回[统一路线](README.md)。

## Assembly M4 分支稳定的约束流形交互

### ASSEMBLY-SESSION：版本化 preview session 与 branch snapshot

Session 绑定 base Workspace sequence 和 SolveManifest digest，保存 accepted pose、branch 和 warm-start snapshot；它是可丢弃的交互状态，不进入 Revision。

### ASSEMBLY-DRAG：null-space drag objective

用 M2/M2.5 null-space 在硬约束可行流形上最小化拖拽目标，替换临时 `interaction-driver` Fix；返回最近可行 pose。

### ASSEMBLY-FEEDBACK：allowed/blocked direction 与 Product UX

贯通平移/旋转手柄、受限方向反馈、迟到响应丢弃、最终 accepted frame 和一次 pointerup commit；保留同一选择与手柄生命周期。

### ASSEMBLY-BRANCH：DirectedAngle 与 M4 验收

加入显式 Datum/Publication axis 和 sense；静态 Revision 保存 branch intent，unwrapped/winding 留在 Interaction/Kinematics；完成闭环、0/π/2π 邻域和不可达拖拽 corpus。

## Assembly M5 局部冲突解释

### CONFLICT-CORPUS：确定性冲突 corpus 与 evidence contract

建立多约束冗余、矛盾、branch、容差和退化场景；定义 proven minimal、irreducible、localized suspect、unsatisfied 和 budget exhausted。

### CONFLICT-SEARCH：局部邻域与 bounded conflict search

从 changed constraint 和 incidence graph 构造邻域，组合 rank/branch evidence 与 bounded deletion filtering 或 QuickXplain；耗尽预算返回部分证据，不伪造 MUS。

### CONFLICT-UX：诊断与修复 UX

按稳定 Connection/Constraint/Equation ID 高亮结构树和 viewport，提供 suppress/measure/reconnect 候选动作；任何修复都由显式 Domain Command 提交。

## Assembly M6 Engineering Connections

### CONNECTION-CONSTRAINTS：Offset、Parallel、Perpendicular

在现有 Point/Axis/Plane/Cylinder descriptor 上增加 typed definitions、解析 Jacobian、branch、DOF 和诊断 corpus。

### CONNECTION-FRAME：Frame/Connector Publication

Connector 合同增加接口种类、frame、对称性、极性、允许 Connection、名义间隙/尺寸和自定义属性。

### CONNECTION-BASIC：基础 Engineering Connections

实现并用实际 M2.5 freedom 验证 Rigid、Revolute、Prismatic、Cylindrical、Planar Connection；声明类型不能替代方程和 DOF 验证。

### CONNECTION-CONTACT：受控定位 Contact

先实现解析 plane/plane 和 cylinder 定位 Contact 子集，明确接触侧、branch、退化和多解诊断；不把 mesh 碰撞或任意最近点冒充权威定位约束。

### CONNECTION-DESCRIPTORS：Circle、Sphere 与 Cone descriptor

逐类增加 descriptor、Publication resolution、适用约束、解析 Jacobian、symmetry 和 corpus；一次实现保持在这三个有界解析几何族内。

### CONNECTION-LIMITS：Distance、Angle 与 Joint limits

增加有界距离、角度和 joint limit 的 active-set/branch 语义、状态与交互诊断。任意 NURBS-NURBS 接触、gear/rack 和复杂曲面接触另立后续研究计划。
## 依赖与验证

M4：SESSION → DRAG → FEEDBACK → BRANCH；M5：CORPUS → SEARCH → UX；M6：CONSTRAINTS → FRAME → BASIC → CONTACT → DESCRIPTORS → LIMITS。阶段之间沿 M4 → M5 → M6 进入产品交付；允许独立算法 spike，但不能因此跳过上游产品验收。

DirectedAngle 数值内核已经存在；ASSEMBLY-BRANCH 交付的是显式 Datum/Publication 轴、交互 session 和跨 branch 验收，不重新发明已有角度方程。当前拖拽仍有 `interaction-driver` Fix，不能以已有预览或 null-space 输出宣称 M4 完成。

每项先跑匹配 corpus，再跑 assembly/workspace/web 受影响域；Proto/Router 公共合同变化升级全仓。交互必须另做真实浏览器验收。算法门的详细定义见[Solver Architecture](../kernel/assembly/SOLVER_ARCHITECTURE.md)，这里负责产品依赖与下一步工作。
