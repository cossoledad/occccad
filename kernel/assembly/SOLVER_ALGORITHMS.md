# 当前三维装配约束求解算法

本文记录 `kernel/assembly` 当前已经实现的算法基线，用于代码审查、数值回归和后续替换求解后端。它描述的是可由
[`src/solver.cpp`](src/solver.cpp) 验证的事实，不代表 CATIA 或 DCM 的内部实现，也不把
[`SOLVER_ARCHITECTURE.md`](SOLVER_ARCHITECTURE.md) 中的目标能力描述为已经交付。

## 1. 输入、状态与目标函数

每个 Body 的位姿是从 body-local 坐标到世界坐标的刚体变换

\[
p_w = R p_l + t,\qquad T=(R,t)\in SE(3).
\]

Point、Axis、Plane 和 Cylinder 都以 body-local 不可变值描述；求解时才通过 Body 位姿变换到世界坐标。一个自由刚性
cluster 使用六维切空间增量

\[
\Delta x=[\Delta t_x,\Delta t_y,\Delta t_z,
           \Delta \theta_x,\Delta \theta_y,\Delta \theta_z]^T.
\]

平移直接相加，旋转采用旋转向量的指数映射左乘当前四元数：

\[
t' = t+\Delta t,\qquad R'=\operatorname{Exp}(\Delta\theta)R.
\]

所有 active constraint 的残差块按稳定约束顺序拼接为向量 `r(x)`。M2 以几何约束为主目标，并以弱运动偏好选择可行解：

\[
\min_x \frac{1}{2}\lVert r(x)\rVert_2^2
+\frac{1}{2}\lVert W_m\log(T_0^{-1}T)\rVert_2^2.
\]

长度残差除以 `length_scale`，角度和方向残差除以 `angle_scale`，使不同量纲可以进入同一个范数。两者必须由调用方按
模型单位和容差策略显式设置，当前默认值都是 `1.0`。

RPC 使用 `AssemblySolverProfile schema_version=1` 传递求解策略；零值字段沿用 kernel 默认值。主要默认阈值为：length
convergence/classification `10⁻⁷`、angle convergence/classification `10⁻⁸`、translation step `10⁻⁹`、rotation step
`10⁻¹⁰`、degeneracy `10⁻⁸`、translation/rotation finite difference `10⁻⁶/10⁻⁷`、initial damping `10⁻⁴`、
rank absolute/relative `10⁻¹⁰/10⁻⁸`。classification 可由
调用方独立设置，不参与迭代停止，但不得严于对应 convergence tolerance，否则模型请求无效。

## 2. 总体处理流水线

```mermaid
flowchart TD
    A[校验并规范化输入] --> B[active Rigid 并查集合并]
    B --> C[传播 cluster 内相对位姿并检查刚性环]
    C --> D[将 active Fix 转换为 cluster ground pose]
    D --> E[建立 cluster/constraint 连通分量]
    E --> F[按 affected bodies 选择分量]
    F --> G[解析并冻结 ConstraintBranchState]
    G --> H[消元 ground 或 reference gauge]
    H --> I[typed equations、连续残差与解析 Jacobian]
    I --> J[augmented QR 阻尼最小二乘]
    J --> K[SVD rank、数值 null-space、DOF 与诊断]
    K --> L[恢复 Body 位姿、branch 与方程 provenance]
```

输入校验包括稳定 ID 唯一性、引用完整性、有限数、单位方向、合法半径、距离非负、unsigned Angle 位于 `[0, π]`、
directed Angle 位于 `[0, 2π]`、SolveIntent body 存在且 moving/reference 在 body 与 rigid-cluster 层均不冲突。无效模型返回
`InvalidModel`，不会让异常越过公开求解接口。

## 3. 图编译

### 3.1 Rigid cluster

`Driving` 或 `Controlled` 的 `Rigid` 约束先通过并查集合并 Body。每条 Rigid 边保存创建约束时捕获的相对位姿；从按
Body ID 排序后选出的 cluster root 做广度优先传播，得到 `root_to_body`。若闭环通过不同路径计算出的相对位姿误差超过
`length_tolerance`、`angle_tolerance`，模型被判为不一致，而不是静默采用某一条路径。

cluster 的自由变量始终只有一个 `SE(3)` 位姿，因此含 N 个 Body 的刚性子装配从 `6N` 个切空间变量降为 6 个。最终
Body 位姿由下式恢复：

\[
T_{body}=T_{cluster}\,T_{root\rightarrow body}.
\]

### 3.2 Ground 消元

active `Fix` 的目标是显式 `fixed_pose`，未提供时使用 Body 初始位姿。若 Fix 施加在非 root Body 上，先反算 cluster root
目标：

\[
T_{root}^{*}=T_{body}^{*}\left(T_{root\rightarrow body}\right)^{-1}.
\]

grounded cluster 不进入数值变量。一个 cluster 上的多个 Fix 若不能导出同一 root pose，则直接报告无效模型。

### 3.3 Connected component 与局部求解

除 Fix/Rigid 外的 active constraints 在 cluster 之间建立无向边，并查集形成 connected components。空的
`affected_body_ids` 选择全部 component；否则只更新包含指定 Body 的 component，其他 component 保持名义状态并返回
`COMPONENT_NOT_SOLVED` 诊断。排序和 ID 生成以稳定 Body/cluster ID 为依据，避免依赖哈希遍历顺序。

`Measured` 约束不进入图和目标函数，但在最终位姿上计算残差；`Suppressed` 完全跳过；`Driving` 与 `Controlled` 当前采用
相同的数值驱动语义。

## 4. M1.7 moving/reference 规约与运动偏好

`SolveIntent` 是单次请求的解选择策略，不持久改变约束方向。Product 创建二元约束时将第一选择作为 moving、第二选择
作为 reference。

无物理 ground 的连通分量存在六维整体刚体运动自由度。若策略是 `MoveFirstMinimizeReference`，求解器按请求顺序找到
该 component 中第一个 reference body 所属 cluster，将它从数值变量中移除并保持初始位姿。这样第二选择成为 gauge
anchor，第一选择承担相对运动，但不会生成物理 Fix。

该消元只是选择同一相对解族的世界坐标规约，因此仍报告 `gauge_dof = 6`。每个自由 cluster 还会相对 solve 初始位姿
加入弱二次正则，权重满足 reference > neutral > moving；已接地的多解系统也会优先让 moving cluster 承担变化。它不是
严格词典序优化，权重不得牺牲几何可满足性。Rigid 合并后在 cluster 层分配角色；同一 cluster 同时含 moving/reference
会返回 `InvalidModel`。

## 5. 当前约束残差

记世界点为 `p`，轴为 `(o,d)`，平面为 `(o,n)`，其中方向均为单位向量。编译阶段从 nominal/warm-start pose 把
`Unoriented` 固定为 `Same` 或 `Opposite`，并把无符号距离固定到初始侧；迭代中不再换支。Angle 的
`Unoriented` 保留完整 `[0,π]` 语义。

| 约束与几何对 | 当前残差块 | 标量行数 |
|---|---|---:|
| Fix | 平移差 + 四元数相对旋转的 rotation vector | 6 |
| Rigid | 当前相对位姿与捕获相对位姿之差 | 6 |
| Coincident Point–Point | `(p₁-p₂)/L` | 3 |
| Coincident Point–Axis/Cylinder | `((p-o)×d)/L` | 3 |
| Coincident Point–Plane | `((p-o)·n)/L` | 1 |
| Coincident / Concentric Axis-like pair | 方向差 + `((o₁-o₂)×d₂)/L` | 6 |
| Coincident Cylinder–Cylinder | Axis-like 残差 + 半径差 | 7 |
| Coincident Plane–Plane | 法向差 + 有符号法向距离 | 4 |
| Angle，普通 `[0,π]` 目标 | `(atan2(‖d₁×d₂‖, d₁·d₂)-target)/A`（应用冻结方向支） | 1 |
| Angle，含 0/π 端点 | 冻结局部转轴后的周期标量角误差 | 1 |
| Distance Point–Point | `(‖p₁-p₂‖-target)/L` | 1 |
| Distance Point–Plane | 显式 signed/unsigned side 的法向距离差 | 1 |
| Distance Axis-like pair | 两条无限直线的最短距离差 | 1 |
| Distance Plane–Plane | 法向平行残差 + 显式 side 的距离差 | 4 |

表中 `L=length_scale`、`A=angle_scale`。某些向量残差含代数相关行，例如单位方向的三分量差并不总有三维独立 rank；
因此 DOF 不能用“残差行数相减”估计，而必须在求解点计算 Jacobian rank。Cylinder–Cylinder `Coincident` 检查半径相等，
`Concentric` 刻意不约束半径。

Directed Angle 先把两个方向投影到 reference axis 的法平面，再用投影单位向量计算
`atan2(k·(a×b),a·b)`；投影退化会明确拒绝模型。所有 Angle 使用 `atan2(sin(delta),cos(delta))` 的最短周期误差。
`AngleBranchState` 保存 wrapped/unwrapped/winding，输入上一状态时返回最邻近的等价角；跨请求保存由调用者负责。

## 6. 解析 Jacobian 与差分 oracle

当前 Point/Axis/Plane/Cylinder 能力矩阵由内部 typed equation registry 编译为带语义 equation kind、declared generic rank
和稳定 provenance 的残差行。生产 Jacobian 使用前向解析微分值类型，在同一次方程计算中传播值及其对稳定自由 cluster
切空间顺序的导数。点的旋转导数包含相对 cluster 原点的力臂，因此支持点偏离原点时不会漏掉旋转引起的平移。

中央有限差分只作为可配置的 differential oracle：Debug 默认启用，逐列对比解析矩阵并使用 scale-aware tolerance；差异超限
直接失败，不静默切回数值微分。周期 Angle 行在 oracle 中先计算最短周期差，避免跨 `2π` 把等价残差误判为导数错误。

对每个自由 cluster 的六个切空间变量分别施加正负扰动：

\[
J_{:,j}\approx\frac{r(x+h_j e_j)-r(x-h_j e_j)}{2h_j}.
\]

平移和旋转使用独立默认步长 `10^-6` 与 `10^-7`。正负扰动复用已冻结 branch，残差维数必须一致且全部有限，否则返回
`NumericalFailure`。兼容字段 `finite_difference_step` 非零时仍可覆盖两者，新调用方应使用分离字段。

## 7. 阻尼最小二乘迭代

每轮直接构造增广最小二乘系统

\[
\begin{bmatrix}J\\ \sqrt{\lambda}I\\ W_m\end{bmatrix}\Delta x
=
\begin{bmatrix}-r\\ 0\\ -W_m d_{nominal}\end{bmatrix},
\]

并使用 Eigen `ColPivHouseholderQR` 求解，避免显式形成会平方条件数的 `J^T J`；同一增广系统包含弱运动正则。
small-step 只有在 `||J^T r||` 也低于 `gradient_tolerance` 时才认定驻点，避免大 damping 伪造 stationary。SVD 只用于
rank/null-space 分析：

- 初始 `λ = initial_damping`，默认 `10⁻⁴`；
- 每个 component 对完整候选步执行最多 12 次确定性二分回溯；候选残差平方范数严格下降时接受步骤，并令 `λ=max(0.25λ,10⁻¹²)`；
- 否则拒绝步骤，并令 `λ=min(10λ,10¹²)`；
- 每个长度、角度方程分别满足 `length_tolerance`、`angle_tolerance` 时收敛；
- 平移、旋转增量分别低于 `translation_step_tolerance`、`rotation_step_tolerance` 但方程仍超限时返回 `Unsatisfied`；
- 默认最多 100 次迭代；非有限线性解返回 `NumericalFailure`。

若 component 没有自由变量且超过 convergence tolerance：同时超过 classification tolerance 时返回 `Inconsistent`，否则
返回 `Unsatisfied`。真正耗尽迭代预算才返回 `MaxIterations`。

## 8. Rank、DOF 与 gauge

收敛后重新计算 `J`，先对非零参数列归一化，再使用 Eigen `JacobiSVD`；阈值为
`max(rank_absolute_tolerance, rank_relative_tolerance*sigma_max)`。设参与计算的自由
变量数为 `n`：

\[
nullity = \max(n-\rho,0).
\]

- 有物理 ground：`gauge_dof=0`，`relative_dof=nullity`；
- 无物理 ground 且没有显式 reference gauge 消元：从 nullity 中最多扣除 6 个整体刚体 gauge；
- 使用 M1.5 reference gauge 消元：数值变量已不含这 6 维，故 `relative_dof=nullity`，但逻辑变量数和
  `gauge_dof=6` 仍单独报告。

结果同时返回按自由 cluster tangent 排序的数值 null-space basis、参与该排序的 cluster IDs、奇异值和实际 rank threshold。
这些原始向量用于数值证据与后续 M2.5 输入；当前尚不把它们稳定解释为用户可见的平移轴、转轴或螺旋自由度。rank 是局部
线性化结论，会受尺度、姿态、退化几何和阈值影响。

## 9. 冗余、冲突与分类

冗余检测按 chosen basis 顺序增量拼接 Jacobian block，并为每个 constraint 返回 equation count、effective rank、
incremental rank 与 Independent/PartiallyRedundant/FullyRedundant。旧 ID 列表仅包含 incremental rank 为零者；归因仍随
basis 顺序变化，不声称唯一冗余来源。

求解结束后，所有非 Suppressed 约束都在最终 Body 位姿上按独立 classification tolerance 重新计算。`Unsatisfied`
component 的超差约束进入 `unsatisfied_constraint_ids`；只有零变量且超过 classification tolerance 的 `Inconsistent`
component 才写入 `conflicting_constraint_ids`。一般 `Unsatisfied` component 会在有界预算内逐个临时 Suppress active
constraint；若可解性或残差显著改善，则写入 `suspected_conflicting_constraint_ids` 与 `LIKELY_INCONSISTENT`。这是探针，
不是 IIS/MUS 证明。

最终分类优先级为：

1. 数值失败或超过迭代条件：`NonConvergent`；
2. 零变量 component 超过 classification tolerance：`Inconsistent`；
3. 一般驻点仍超过 convergence tolerance：`Unsatisfied`；
4. 存在 whole-constraint 冗余：`Redundant`；
5. 存在 relative DOF：`SolvedUnderConstrained`；
6. 否则：`SolvedFully`。

每个残差标量生成稳定的 `constraint-id/equation/semantic-kind`，并携带 Connection、Constraint 与 declared generic rank
provenance。同一 block 内 semantic kind 必须唯一；当前“稳定”指同一规范输入及残差定义下可重放，未来修改残差分解时
必须考虑 equation identity 的版本化。

零变量违反证明的是当前冻结分支、ground 和给定约束条件不可相容；一般非线性驻点只标记 `Unsatisfied`。当前
`conflicting_constraint_ids` 仍不是 MUS：它是已证明不一致 component 中超差的约束邻域。

## 10. 确定性、复杂度和已知边界

cluster、component 及冗余/冲突 ID 集合会显式排序；Body 与方程结果保留规范输入顺序。相同规范输入、选项和初始位姿
应得到语义等价结果。该保证不意味着不同 CPU/Eigen 版本下浮点位完全一致，也不意味着未规范化的 constraint 排列会给出
完全相同的冗余归因对象。

设一个 component 有 `b` 个自由 cluster、`n=6b` 个变量、`m` 个残差标量。生产解析微分一次构造 dense `m×n`
Jacobian，augmented QR 的成本仍随 component 大小快速增长；Debug differential oracle 额外需要约 `2n` 次残差计算。
connected-component 分解和 ground/rigid
消元是当前最主要的规模控制手段，尚未使用稀疏 Jacobian、增量因子分解或并行 component 求解。

M1.6 还为近平行直线距离引入以 `degeneracy_tolerance` 为尺度的 blended 退化极限；除极小的
`kDirectionEpsilon` 保护分支外，它在 skew 与 parallel 公式之间连续过渡。该表达是工程正则化而非无限直线距离的唯一解析
延拓，仍需用容差边界 sweep 验证 bias、Jacobian 和 rank。
当前算法还不具备：可解释且规范化的 null-space 自由度、最小冲突集、全局多分支枚举、严格层级最小位移、
拖拽流形投影、一般曲面接触和大规模稀疏图优化。上述能力的引入顺序见架构路线图，不能从本文的 M1/M1.5 基线推断为
已经实现。

## 11. 已确定的后续升级流程

后续工作先修正残差和状态语义，再替换微分与线性代数实现，避免为不稳定方程编写解析 Jacobian：

M1.6 鲁棒性门已经落地：Angle 使用 unsigned `atan2` 和端点对齐残差，一次 solve 内冻结方向和适用的距离侧分支；
版本化 SolverProfile 已贯穿 Proto/Worker/Go；`Unsatisfied`、`Inconsistent` 与 `NonConvergent` 拥有不同证据边界。

M1.7 已把该切片修正为严格的绕轴角：先投影两个端点方向，再计算有向角；Angle 在所有目标值都保持单标量语义，周期误差
跨 0/2π 连续。分支结构可接收/返回 winding，但静态装配 Revision 仍只保存 modulo `2π` 的几何目标，多圈累计属于交互或
运动状态。
所有 component 对候选步使用固定上限次数的二分回溯线搜索。旋转不仅改变支持方向，也会改变离 cluster 原点较远的
支持点世界位置，因此 Plane-Plane Coincident 等普通约束同样可能拒绝完整 LM 步但接受较小下降步。文档实例 FACE 5
回归正是这一类平移/旋转强耦合问题；统一回溯后无需随机 perturb 或增加迭代预算即可收敛。
对于显式 Same/Opposite 的 Plane-Plane Coincident，若当前法向恰好处于目标 branch 的反点，法向差目标存在零梯度鞍点。
初始化阶段会选择与法向最不平行的规范世界轴，构造绕第一支持平面原点的确定性半周 seed，并补偿法向距离；该 seed 只决定
离散 branch 初值，其他约束仍由同一 component 的数值求解统一满足。

1. **M2 方程与微分正确性（已完成）**：内部 typed equation registry 覆盖当前 Point/Axis/Plane/Cylinder 能力矩阵；前向解析微分提供左增量 Jacobian，中央有限差分作为 differential oracle。参考后端使用 augmented QR，SVD 专用于 rank、奇异值和数值 null-space。M2 没有增加 Product 约束类型。
2. **M2.5 自由度与解选择**：将数值 null-space 在稳定 cluster tangent 顺序下解释为平移、旋转和耦合瞬时自由度；以子空间而非原始 SVD 列进行确定性验证。在可行流形内使用层级优化依次最小化 reference motion 与总 nominal change，验收后替换 M1.7 弱权重策略。
3. **M3 可重放输入**：由控制面冻结包含 typed InstancePath、ResolutionSnapshot、Publication/PersistentSelection、descriptor symmetry/provenance、branch intent、tolerance 和 solver build 的不可变 solve manifest。当前 direct Part 与 revision-local topology ID 路径在开发期直接收敛到唯一新模型。
4. **M4 稳定分支与交互**：把当前自动 reference direction 平面切片升级为可选择 axis/sense 的完整 `DirectedAngle`；静态 Product 只持久化 modulo `2π` branch intent，preview/kinematics session 承担 winding。利用 M2/M2.5 null space 把 Drag 目标作为二级目标投影到约束流形，返回最近可行 Pose 与 blocked directions，不再注入临时 `Fix`。
5. **M5 以后**：先完成图局部化、带预算且证据分级的冲突解释，再扩展 Engineering Connection/几何覆盖；最后依据大装配 benchmark 决定 block-sparse、增量 factorization、可选后端与独立 Worker 部署。详细阶段门见 `SOLVER_ARCHITECTURE.md`。

静态装配只保存 modulo `2π` 的姿态分支，多圈累计角属于 Interaction、Kinematics 或 Simulation 状态。MUS/minimal
conflict set 不在 M2 关键路径上；当前优先保证方程、解析微分、数值子空间和可重放输入的正确性，再建设分支交互和诊断搜索。

## 12. 回归验证

邻近场景测试覆盖具体残差行为；[`tests/assembly-corpus`](../../tests/assembly-corpus) 覆盖 canonical DOF、刚性聚类、
ground 消元、gauge、重复/冲突/退化输入、排列不变性、冷/热启动语义以及 M1.5 moving/reference。运行：

```sh
cmake --build build/cmake/debug \
  --target occcad_assembly_solver_scenarios occcad_assembly_solver_corpus
ctest --test-dir build/cmake/debug \
  -R '^(assembly|assembly-corpus)/' --output-on-failure
```
