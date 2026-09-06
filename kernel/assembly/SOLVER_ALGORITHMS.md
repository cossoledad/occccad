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

所有 active constraint 的残差块按稳定约束顺序拼接为向量 `r(x)`。M2.5 的几何恢复最小化
`||r(x)||²/2`；满足硬几何语义后，按词典序最小化 `(E_ref(x), E_all(x))`。
运动目标不与几何残差加权混合，定义与收敛条件见第 4、7 节。

长度残差除以 `length_scale`，角度和方向残差除以 `angle_scale`，使不同量纲可以进入同一个范数。两者必须由调用方按
模型单位和容差策略显式设置，当前默认值都是 `1.0`。

RPC 使用 `AssemblySolverProfile schema_version=2` 传递求解策略；普通零值字段沿用 kernel 默认值；optional `max_preference_iterations` 显式为零表示不给偏好迭代预算。主要默认阈值为：length
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

## 4. M2.5 从动层级优化与名义位姿

Product ADD/EDIT/preview/commit 共用 `assemblyConstraintSolveIntent()`：第一选择 moving、第二选择 reference。
约束本身不增加持久主从方向。`Body.initial_pose` 是本次操作前冻结的名义位姿与分支基线，`initial_guess` 仅初始化数值变量；
Rigid 成员提供的多个 seed 必须符合捕获的刚性关系，未选择的 affected component 不应用 seed。

每个连通分量严格保留 Fix/Rigid 和全部有效几何方程，按以下顺序选择解：

1. 恢复几何可行性；
2. 在可行流形上最小化 reference occurrence 的名义位姿变化平方和；
3. 在 reference 的局部最优子空间内最小化全部 occurrence 的名义变化平方和。

运动度量是 `||t-t0||²/L² + ||Log(R0^T R)||²/A²`，L/A 为独立运动尺度，不是几何容差。
目标按 occurrence 而非 cluster 代表原点计算，包含刚性成员力臂。旋转 Log Jacobian 与左旋转增量一致，Debug 有中央差分 oracle；
π 处是旋转坐标图切口，oracle 只排除跨切口的旋转行，不将它们当普通光滑导数。

无物理 ground 且只有一个 reference cluster、其 reference 名义位姿与捕获刚性关系一致时，仍可等价消去全局 gauge；
否则保留 reference 变量。第一元素 Fix、间接接地或部分受限时，第二元素始终在同一问题内承担必要运动。
多个 reference 联合优化，不固定列表中第一个。原 moving/neutral/reference 弱权重已删除，未增加临时 Fix 重试路径。

`preference.status` 与几何 `SolveStatus` 独立：可行但偏好预算耗尽/停滞仍返回可行 Pose 和 `PREFERENCE_NOT_CONVERGED` 诊断，
Product 拒绝将它当作从动成功提交；不会误报为几何冲突。响应包含每体角色及平移/旋转变化、两层目标值、最终投影梯度、
迭代数和尺度。MOVE_INSTANCE 的 `interaction-driver` 临时 Fix 仍属于另一条 M4 待替换路径。

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

## 7. 可行性恢复与流形上的层级迭代

几何恢复沿用 M2 增广 `ColPivHouseholderQR` 阻尼最小二乘，不形成正规方程，也不再加入弱运动权重。
候选必须降低真实几何残差，通过长度/角度容差检查；small-step 还检查几何梯度，避免大 damping 伪造驻点。
方向验收改用 `atan2(||a×b||, a·b)`，消除 `acos(dot)` 在对齐附近的浮点精度底限。

几何可行后，M2.5 在无量纲切空间用同一 SVD 阈值构造正交零空间及最小范数校正。
二级目标使用投影 BFGS 曲率更新和有界回溯；trust region 只限制步长，不与几何约束竞争。
零空间步只有一阶可行，候选需经有界几何恢复，再检查真实容差、上级目标固定上界及当前层改善。
可行性校正比最终显示容差更严格，以免曲率误差掩盖最后的目标下降。

reference 最优残差为零时，其目标保持子空间是 `null(A_ref Z)`；残差非零时不能冻结整个残差向量。
当前 dense reference 后端对解析 Lagrangian 梯度做 Richardson 中央差分，获得包含约束曲率的 reduced Hessian，
用其零空间保留整个局部 reference 最优集合。球面上“reference 到名义原点距离恒定”的回归验证总目标仍可沿球面优化。
这一步是二阶曲率计算，不是将生产几何 Jacobian 降级为数值差分；后续稀疏/解析二阶优化必须与该参考结果对照。

终止检查两层在最终位姿上的投影梯度，默认 `preference_tolerance=1e-8`，每层最多 100 次。
reference 目标上界固定为第一层终值加 `objective_tolerance`（默认 `1e-12`），不逐步累计放宽。
当目标差接近机器精度时，只在非累积能量误差界内且投影梯度进一步下降时接受步骤，不以浮点停滞冒充最优。
`Converged` 是冻结 branch 下的局部一阶最优性证据，不证明非凸全局最优或任意有限运动可达性。

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
原始 basis 仍作为数值证据保留。M2.5 另在无量纲 metric 中构造每个 occurrence 的正交 allowed/blocked 子空间，
按纯平移子空间与 angular image 分解，返回平移方向、转轴点/方向/pitch、linearization pose 和 rank threshold。
无 ground 时这些解释相对稳定 body ID 的基准 occurrence 计算，整体六维 gauge 单独报告；固定坐标规约不冒充物理接地。
规范自由度按子空间关系识别，无法确定为标准族时返回 Coupled。rank 是局部
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
当前算法还不具备：最小冲突集、全局多分支枚举、拖拽流形投影、一般曲面接触和大规模稀疏图优化。
M2.5 已提供局部层级运动优化和规范化瞬时自由度解释，不能由此推断全局最优或有限运动可达性。

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
2. **M2.5 自由度与解选择（已实现）**：a 从动层级优化、b Product 贯通验收及 c 自由度解释均已落地。将数值 null-space 在稳定 cluster tangent 顺序下解释为平移、旋转和耦合瞬时自由度；以子空间而非原始 SVD 列进行确定性验证。在可行流形内使用层级优化依次最小化 reference motion 与总 nominal change，已替换 M1.7 弱权重策略。
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

### M2.5 验证记录（2026-09-06）

重新编译 Debug 内核与 Geometry Worker 后，assembly/assembly-corpus 共 **75/75** 项通过。
新增回归覆盖从动、非零 reference 最优解族、初值与 nominal 分离、单位/世界坐标变换、刚性成员代表选择、
自由度子空间及偏好预算失败。Go 全仓测试通过；正式 Router 测试调用真实 Worker，并在隔离的临时 PostgreSQL
上验证空库迁移及重复迁移、Product preview/commit、刷新、连续两次 Undo/Redo 和 capability。
Web 场景测试及生产构建通过；浏览器真实入口验证约束面板的求解证据、取消、提交和刷新。
预览 actor 按 effect 生命周期创建，避免 StrictMode 重挂载复用已停止 actor 而丢失响应。

当前显式 moving/reference 的平面链 Debug 基准（每组 3 次）如下；该场景中 reference 必须平移 1：

| Body 数 | 平均耗时 | 几何/偏好迭代 | 归一化残差 | reference 目标/平移 |
|---|---|---|---|---|
| 5 | 53.1 ms | 3 / 2 | 1.68e-11 | 1 / 1 |
| 15 | 541 ms | 3 / 2 | 3.28e-8 | 1 / 1 |
| 30 | 6.63 s | 4 / 2 | 1.77e-9 | 1 / 1 |

总目标在整个可行切空间已满足驻点阈值时，直接使用该更强证据，省去不必要的 reference 二阶切空间计算。
以上是 dense Debug 参考后端的规模证据；场景和策略已变化，不与旧 M2 时延作等价对比，也不代表生产容量承诺。
