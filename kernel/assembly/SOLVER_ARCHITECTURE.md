# Three-dimensional assembly constraint solver evolution

## 1. Purpose and scope

This document defines the next evolution of `kernel/assembly` from the current
standalone numerical slice into a geometry-based three-dimensional assembly
constraint manager. CATIA Assembly Design is the reference for user-visible
engineering semantics and D-Cubed 3D DCM is a reference for constraint-manager
capabilities such as rigid sets, freedoms, diagnostics and partitioning. This does
not imply copying either product's UI, data format or private implementation.

The target is not merely a nonlinear optimizer with more residual kinds. A usable
CAD assembly solver must preserve design intent across edits, distinguish valid
under-constrained models from failures, explain conflicts and redundant
constraints, keep discrete solution branches stable, and solve only the affected
part of a large occurrence graph.

This document is subordinate to the platform invariants and Product/Assembly
domain model in [`docs/TARGET_ARCHITECTURE.md`](../../docs/TARGET_ARCHITECTURE.md).
It focuses on the internal solver architecture, implementation order and
verification gates.

## 2. Current verified baseline

The current M2.5 implementation is already a graph-aware reference solver rather
than the original single-problem A1 prototype. It provides:

- immutable value types for rigid bodies and body-local `Point`, `Axis`, `Plane`
  and `Cylinder` descriptors;
- `Fix`, `Rigid`, `Coincident`, `Concentric`, `Angle` and `Distance` constraints,
  stable connection/constraint/equation identities and four constraint modes;
- rigid-cluster compilation, physical-ground elimination, deterministic connected
  components and affected-component selection;
- right-handed `SE(3)` local increments, forward analytic differentiation for the
  current descriptor matrix, augmented dense QR and bounded backtracking;
- frozen direction, distance-side and angle branches, including projected directed
  angles and request-carried wrapped/unwrapped/winding state;
- column-normalized SVD rank and numeric null-space basis, relative/gauge DOF
  counts, per-constraint declared/effective/incremental rank, semantic equation
  provenance, geometric satisfaction tests and bounded conflict probes;
- a control-plane adapter that resolves direct Part datums and revision-bound exact
  topology descriptors, then calls `SolveAssembly` through the formal Router path;
- atomic Product command submission in which the constraint and all solved poses
  are recorded in the final Revision and reconciled ChangeSet.

The remaining boundary is equally important:

- public transport constraint kinds remain enum/string based; the compatibility
  boundary compiles them into typed internal equation definitions;
- each component still uses dense matrices, although the production step no longer
  forms normal equations;
- numeric null-space and interpreted instantaneous freedom subspaces are returned;
  durable freedom/Engineering Connection identities remain later work;
- hierarchical pose selection is local to a frozen branch; no global nonconvex
  optimum or finite-motion reachability is claimed;
- Product input is limited to direct Part instances and revision-local topology IDs;
  it does not yet freeze typed InstancePaths, Publications/PersistentSelections or
  nested/flexible expansion in an immutable solve manifest;
- MOVE preview still models its target as a temporary `Fix`, so it cannot return the
  nearest feasible point and blocked directions for an unreachable drag target;
- conflict probes identify suspects, not a verified IIS/MUS, and solver profile,
  branch snapshots and build provenance are not yet durable solve inputs/results;
- cancellation, sparse/incremental factorization and representative large-assembly
  performance gates are not implemented.

M3 and later milestones close the remaining gaps in dependency order. They must not be
collapsed into a larger enum surface or a backend replacement that leaves Product
identity, branch intent and diagnostics unresolved.

## 3. Lessons from CATIA and D-Cubed

### 3.1 Engineering connections are above scalar equations

CATIA represents an Engineering Connection as one or more assembly constraints
between components and guides the user toward compatible functional geometry
combinations. Connection types such as rigid, revolute or prismatic express
mechanical intent and remaining freedom; they are not aliases for a single
residual. CATIA also exposes constrained manipulation, constraint-network
diagnosis and flexible subassemblies.

occcad should therefore use this hierarchy:

```text
EngineeringConnection
  -> AssemblyConstraint
       -> compiled semantic equations
            -> backend residual and Jacobian blocks
```

For example, a declared revolute connection can compile to axis coincidence and
axial positioning constraints, but is valid only if the solved connection really
retains one rotational DOF. The declared type must be verified against the
Jacobian; it cannot be trusted as a UI label.

Public CATIA references:

- [Assembly Design capabilities](https://3dswym.3dexperience.3ds.com/wiki/catia-user-community/catia-assembly-design-2-asd_Lx2jW-oPR9qcb3gZNgrKhw)
- [Creating Engineering Connections](https://3dswym.3dexperience.3ds.com/post/3dexperience-edu-students/creating-assemblies-with-catia-3dexperience-r2022x_3AhyqEsmTOueqlaoNXes2A)
- [Constraints in a mechanism](https://help-3dexperience.aesvietnam.com/English/KimUserMap/kim-c-EngConnections-Constraints.htm)

### 3.2 Rigid sets, freedoms and partitions are first-class

D-Cubed 3D DCM publicly describes rigid sets representing parts, reporting the
remaining translational or rotational freedoms of an under-defined set, and
partitioning regions whose changes cannot affect each other. Its published design
discussion also emphasizes explicit support for known geometry types: geometric
special cases enable more predictable behavior, more applicable constraint
combinations and analytic methods that can be faster and more accurate than a
single general formulation.

Consequences for occccad are:

- rigid clustering and graph partitioning precede numeric optimization;
- under-constrained is a successful model state, not a convergence error;
- DOF must include directions and axes, not just a count;
- analytic geometry equations remain explicit rather than being reduced early to
  generic sampled surfaces or closest-distance functions;
- diagnostics must map back to application geometry, connections, constraints and
  individual semantic equations.

Public D-Cubed references:

- [3D DCM version 55: rigid-set freedoms, diagnostics and partitioning](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-dcm-version-55-0/)
- [3D DCM version 56: enhanced partitioning](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-dcm-version-56-0/)
- [3D DCM version 60: sets with transforms and under-defined DOF](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-components-release-highlights/)
- [Why geometry-specific solving matters](https://blogs.sw.siemens.com/news/geometric-constraint-solving-its-all-about-special-cases/)
- [Bounded dimensions and inequality relationships](https://blogs.sw.siemens.com/plm-components/d-cubed-3d-dcm-v50-a-closer-look-at-bounded-constraints/)

## 4. Architectural principles

### 4.1 Separate domain semantics from the numerical backend

The public kernel API owns:

- stable connection, constraint, endpoint and equation identities;
- typed geometry descriptors and their symmetry;
- constraint mode, units, branch intent and limits;
- component DOF, conflict/redundancy and diagnostic contracts;
- solver profile and provenance.

A numeric backend owns only local variable blocks, residual/Jacobian evaluation,
linearization and nonlinear iteration. Ceres, Eigen or a future custom backend must
not leak types into Product models, Proto or persisted commands.

### 4.2 Use three distinct graphs

The architecture contains three related but non-interchangeable graphs:

| Graph | Nodes and edges | Responsibility |
|---|---|---|
| Product occurrence graph | Product references and typed instance paths | hierarchy, configuration, permissions and expansion |
| Constraint incidence graph | rigid clusters, constraints and semantic equations | components, grounding, DOF, conflicts and solve scope |
| Revision dependency graph | revisions, publications and geometry inputs | dirty closure, cache invalidation and replay |

Within one solve component, the incidence graph becomes a factor graph between
`SE(3)` pose variable blocks and equation blocks. Graph optimization here means
preserving and exploiting this block sparsity; it does not mean treating Product
structure as a pose graph or storing numerical factors as business state.

### 4.3 Residual rows are not independent DOF restrictions

An implementation may use a three-component cross product to enforce two
independent directional equations. Similarly, unit-vector difference residuals
contain structural dependence. Each compiled constraint must therefore declare:

- its semantic equation kinds;
- expected generic rank;
- geometry-dependent degeneracy conditions;
- physical units and normalization;
- branch variables;
- the mapping from every Jacobian row to
  `(connection_id, constraint_id, equation_id)`.

DOF and redundancy must use rank, not `variable_count - residual_count`.

### 4.4 Hard constraints and interaction objectives are different

Driving constraints define the feasible manifold. Measured constraints only
evaluate values. A drag target and preference for nominal poses are secondary
objectives and must not be encoded as infinite-weight or temporary hard
constraints.

Conceptually, constrained drag solves:

```text
minimize  drag_error(selected_pose, target)
          + nominal_change(other_poses)
subject to all active driving constraints
```

When the target is unreachable, the result should be the nearest feasible pose
plus the blocked directions, without weakening assembly constraints.

## 5. Recommended module boundaries

The existing library should evolve incrementally toward:

```text
kernel/assembly/
  api/                 descriptors, connections, constraints, result, diagnostics
  compile/             validation, capability matrix, equation compilation
  graph/               rigid clusters, components, grounding, affected subgraph
  equations/           geometry-specific residuals and Jacobians
  diagnostics/         rank, DOF basis, redundancy, conflicts, degeneracy
  solve/               initialization, candidates, continuation, orchestration
  backends/
    eigen-reference/   small deterministic reference solver
    ceres/             optional nonlinear backend after evaluation
```

This is a dependency direction, not a requirement to create every directory
immediately. New files should be extracted only when the corresponding boundary
has behavior and conformance tests.

## 6. Solver pipeline

### 6.1 Freeze an immutable solve manifest

Before entering the solver, the control plane resolves:

- root Product Revision and configuration/resolution snapshot;
- typed occurrence paths and occurrence-local nominal poses;
- Publication or PersistentSelection targets;
- immutable local geometry descriptors and symmetry;
- Engineering Connections and atomic constraints;
- tolerance profile, solver build, interaction target and deadline.

The Assembly Solver Worker receives this immutable manifest and never queries the
business database, follows a moving workspace head or reads a B-Rep directly.

### 6.2 Compile constraints into semantic equations

Compilation performs:

- endpoint and geometry-kind compatibility checks;
- units, finite-value, range and degeneracy validation;
- branch completeness validation;
- conversion to normalized equation blocks;
- analytic or automatic Jacobian binding;
- provenance assignment for every equation row.

Compatibility belongs in a server-authoritative capability registry shared by
command validation and solver compilation. The UI may use the registry for
guidance but is never the authority.

### 6.3 Collapse rigid clusters

Use rigid connections to union occurrences into clusters. Keep each member's
fixed transform relative to the cluster frame. A cluster contributes one six-DOF
pose; a grounded cluster contributes no free variable.

Constraints entirely inside one rigid cluster are still evaluated. A violated
internal constraint is an explainable `INTRA_RIGID_CONFLICT`, not an ignored
factor.

### 6.4 Build and partition the incidence graph

Build the bipartite cluster/constraint graph, remove suppressed and measured
constraints from the driving equation set, and find connected components. Only
components affected by an edit, drag target or explicit request are solved.
Independent components may run concurrently and can reuse their compiled graph
signature, sparsity pattern and warm start.

This partitioning must be implemented before attempting to solve the entire
assembly with a more sophisticated sparse library.

### 6.5 Grounding, gauge and structural analysis

For each component:

- eliminate grounded clusters from the variable vector;
- if there is no ground, report the six global rigid-body gauge freedoms
  separately;
- use structural matching to identify candidate under-, well- and
  over-constrained regions;
- use rank-revealing QR, with SVD for small suspect blocks, for numeric
  classification;
- use a versioned rank tolerance from `ToleranceProfile`.

The reported DOF is:

```text
remaining_dof = dimension(free tangent variables) - rank(active Jacobian)
```

The result also contains a null-space basis interpreted as translational
directions, rotational axes and coupled screw-like instantaneous motions.

### 6.6 Generate and preserve branch candidates

Known geometry combinations should provide analytic initialization candidates for
orientation, angle sector, contact side and axial direction. Candidate selection
uses this priority:

1. satisfy persisted branch intent;
2. preserve continuity with the preceding accepted solution;
3. minimize change from nominal poses;
4. return `AMBIGUOUS_BRANCH` if distinct candidates remain equivalent.

The solver must never silently select the first topology or geometric solution.

### 6.7 Refine on `SE(3)`

The first backend evolution can remain Eigen-based, but it must consume one
compiled component at a time and preserve block sparsity. Required improvements
are:

- ground and rigid-cluster elimination;
- analytic Jacobians for Point/Axis/Plane/Cylinder equations;
- finite differences retained as a Jacobian conformance oracle, not production
  evaluation;
- trust-region damping with physical residual checks;
- rank-revealing linear solves for small and rank-deficient components;
- sparse QR/Cholesky evaluation only after representative benchmarks;
- deadline, cancellation, finite-pose and resource checks.

Ceres is a reasonable optional backend experiment because it provides manifolds,
automatic differentiation and sparse nonlinear least-squares solvers. It is not a
constraint manager: branch selection, rigid clustering, CAD geometry semantics,
DOF and conflict explanations remain occccad responsibilities. Ceres also warns
that forming normal equations squares the condition number and recommends
analytic or automatic differentiation over numeric differentiation:

- [Ceres modeling and manifolds](https://ceres-solver.readthedocs.io/latest/nnls_modeling.html)
- [Ceres derivative guidance](https://ceres-solver.readthedocs.io/latest/modeling_faqs.html)
- [Ceres sparse and dense linear solvers](https://ceres-solver.readthedocs.io/latest/nnls_solving.html)

### 6.8 Validate and diagnose the result

Numerical convergence is necessary but insufficient. Post-solve validation checks:

- each constraint's physical length/angular error against its tolerance;
- connection limits and branch intent;
- finite, canonical rigid poses;
- declared Connection type against actual remaining DOF;
- rank changes caused by geometric degeneracy;
- conflict and redundant equation explanations.

The public status set must distinguish at least:

- `SOLVED_FULLY`;
- `SOLVED_UNDER_CONSTRAINED`;
- `REDUNDANT`;
- `CONFLICTING`;
- `BROKEN_REFERENCE`;
- `AMBIGUOUS_BRANCH`;
- `NON_CONVERGENT`;
- `LIMIT_VIOLATION`;
- `RESOURCE_LIMIT`.

Under-constrained is normally committable. Conflicting, ambiguous or broken input
must not modify the Workspace.

## 7. Diagnostics strategy

Initial diagnostics should be useful and deterministic before attempting globally
minimal explanations:

1. verify that the previous model solves;
2. when a new constraint fails, start from its connected graph neighborhood;
3. use incremental rank contribution to identify redundant equation blocks;
4. apply deletion filtering or a QuickXplain-style search to the small conflict
   neighborhood;
5. return a small explanatory set containing connection, constraint and equation
   IDs plus geometry evidence;
6. label the result as minimal only if minimality was actually verified.

Diagnostics are domain data, not an English string from the numeric backend. Each
diagnostic needs a stable code, severity, affected identities, numeric evidence,
tolerance/profile information and a localizable message key.

## 8. Delivery plan

### M0: corpus and baseline observability (implemented)

Create `tests/assembly-corpus/` before structural rewrites. Cover at least:

- one free body: 6 DOF;
- one fixed body: 0 DOF;
- plane coincidence: expected 3 remaining relative DOF;
- concentric axes: expected 2 remaining relative DOF;
- revolute: 1R;
- prismatic: 1T;
- cylindrical: 1R + 1T;
- two ungrounded bodies: relative freedoms plus six gauge freedoms;
- duplicate constraints;
- conflicting offsets and angles;
- exact and near-degenerate axes/planes;
- branch preservation after dimension edits;
- permutation invariance of body and constraint order;
- semantic equivalence of cold solve and valid warm start.

Golden checks compare satisfied invariants, DOF, branch and diagnostics within a
tolerance profile, not byte-identical poses.

The executable baseline is maintained in
[`tests/assembly-corpus`](../../tests/assembly-corpus). Freedom cases retain
canonical feasible and blocked rigid motions as black-box conformance checks and
also assert the M1 numeric rank, relative DOF, gauge and structured classification.

### M1: graph kernel and result contract (implemented baseline)

The baseline now provides:

- introduce stable connection/equation IDs and constraint modes;
- build rigid clusters and connected components;
- eliminate grounded variables and report gauge freedoms;
- solve only affected components;
- extend results with component DOF, redundancy/conflict fields and structured
  diagnostics;
- keep the present numerical method as a reference backend.

At the M1 milestone, rank and whole-constraint redundancy analysis used the
finite-difference reference Jacobian; current M2 uses analytic derivatives. Interpreted
null-space bases are M2.5, and localized/minimal conflict explanations are M5;
complex surface constraints were deliberately not added in M1.

### M1.5: request-scoped moving/reference intent (implemented baseline)

Selection order is an interaction intent, not an intrinsic property of a geometric
constraint. For a binary constraint the Product adapter sends the first selected
occurrence as `moving` and the second as `reference`, using the request-scoped
`MoveFirstMinimizeReference` policy. The persisted constraint, equation identities
and residuals remain symmetric; replay does not permanently turn one occurrence
into a driver and the other into a dependent object.

For an ungrounded connected component the six global rigid motions are a gauge, so
M1.5 deterministically removes the selected reference cluster from the numerical
variables and holds its nominal pose exactly. The first selection therefore moves
to satisfy the constraint while the second stays unchanged, without synthesizing a
physical `Fix`. Result reporting still uses the logical pre-gauge variable count
and reports the component's six gauge DOF.

This is deliberately narrower than a general priority solver. If a component is
already physically grounded but retains several feasible internal motions, M1.5
alone provided no priority optimization; M2 added M1.7 weak regularization. Strict
lexicographic minimization of reference motion and weighted body groups require
M2.5 null-space/hierarchical optimization, now implemented; drag projection uses that result in
M4. The
`Driving`/`Measured`/`Controlled`/`Suppressed` mode remains orthogonal: mode decides
whether an equation drives the solve, while solve intent decides how equivalent
pose solutions are distributed.

### M1.6: residual semantics and robustness gate (implemented baseline)

M1.6 stabilizes the mathematical problem before analytic Jacobians are written.
It does not add new constraint families or a sparse backend. The accepted baseline:

- exposes a versioned `AssemblySolverProfile` across Proto, Worker and Go with
  independent length/angle convergence acceptance, classification, translation/
  rotation step, degeneracy, finite-difference, damping and rank thresholds;
- replaces `acos(dot)` with `atan2(norm(cross), dot)` for regular unsigned angles;
  exact zero/pi targets use a branch-preserving cross-vector plus dot residual so
  finite differences can converge through the endpoint cusp;
- chooses and freezes direction and unsigned distance-side branches once per solve from
  explicit request intent where available, otherwise the nominal/warm-start pose;
- handles parallel and nearly parallel axis distance without switching through an
  ill-conditioned generic formula;
- keeps a stationary candidate as `Unsatisfied`, uses `Inconsistent` only when a
  zero-variable component exceeds classification tolerance, and reserves
  `NonConvergent` for iteration exhaustion or numerical failure;
- reports unsatisfied constraint IDs separately from proven conflicting IDs;
- covers frozen orientation/side, convergence-versus-classification tolerance,
  zero/pi endpoint convergence and a deterministic near-parallel axis-distance
  sweep in the focused corpus;
- keeps dense Eigen and the finite-difference reference backend during this gate.

Persisted branch intent and directed-angle session state remain M4 work; M1.6 freezes a
deterministic request-local branch only. The current Product integration adds one
narrow directed plane-angle slice: a reference direction stored in the second
body's local frame distinguishes the two sectors within one turn. Explicitly
selectable datum-axis/sense semantics and multi-turn session state remain M4 work.

Warm start at this stage means that the caller supplies the previously accepted
poses as the next nominal poses. It is not yet an incremental factorization cache
or a persistent solver session.

### M1.7: continuity, motion preference and stable local diagnostics (implemented baseline)

M1.7 preserves the M1 compiler/component/LM architecture while separating geometry,
discrete branch state and solution preference. Directed angles project both endpoint
directions onto the reference-axis normal plane and reject a degenerate projection;
all Angle constraints remain one scalar equation at zero and pi and use a wrapped
periodic error. `AngleBranchState` carries wrapped, unwrapped and winding values
between calls without making multi-turn state part of the static assembly definition.

Direction, unsigned-distance side and angle branch data are compiled into an internal
`ConstraintBranchState` and remain fixed during central-difference perturbations.
At M1.7, SolveIntent assigned weak cluster-level motion weights in addition to the existing
ungrounded reference gauge; a rigid cluster containing both roles is invalid. This is
a weighted secondary objective, not yet lexicographic null-space optimization.

Translation and rotation use separate central-difference steps. Rank analysis column-
normalizes the Jacobian and uses SVD with absolute plus relative thresholds. Per-
constraint diagnostics expose equation count, effective rank and chosen-basis
incremental rank. Failed feasible-variable components receive bounded single-
constraint removal probes and report suspected conflicts without claiming an IIS/MUS.
Satisfaction uses grouped geometric norms for the overcomplete residual blocks.

### M2: compiled equations and differential correctness (implemented baseline)

M2 is a kernel-only correctness gate. It does not add new Product
constraint kinds, replace Eigen or claim large-assembly scalability.

- introduce internal typed constraint definitions and an equation registry whose
  entries declare supported descriptor pairs, semantic equation IDs, generic rank,
  physical dimension, normalization, symmetry and degeneracy policy;
- split unsigned and directed angles into distinct compiled definitions; preserve a
  compatibility adapter for the current public `ConstraintKind` until all callers
  switch atomically;
- implement left-trivialized analytic Jacobian blocks for every currently supported
  Point/Axis/Plane/Cylinder pair, including support-point motion under rotation;
- retain central differences solely as a differential oracle and compare every
  analytic block over regular, near-degenerate and branch-boundary samples;
- solve the damped linearized problem without forming `J^T J` (augmented QR for the
  reference backend, SVD only for rank/null-space inspection and suspect blocks);
- return the numeric null-space basis in a deterministic cluster tangent ordering,
  with singular values, thresholds and equation provenance.

M2 acceptance gates:

- analytic-versus-central-difference error satisfies scale-aware tolerances for the
  full current capability matrix and randomized rigid transforms;
- analytic and reference-derivative backends produce equivalent classification,
  branch, rank and physical residual results on the corpus;
- exact 0/pi angles, near-parallel axes and off-origin supports remain finite and
  continuous; a derivative mismatch fails the test rather than silently falling
  back in production;
- existing Product create/edit/preview/Undo/Redo and Router tests remain unchanged;
- benchmarks record component size, residual/Jacobian evaluations, factorization
  time and condition evidence before any sparse-library decision.

### M2.5: explainable freedoms and hierarchical pose selection (implemented baseline)

M2.5 preserves the raw numeric basis and adds a separate instantaneous geometric
interpretation and hierarchical pose policy. The delivered layer:

- canonicalize bases deterministically despite sign, repeated singular values and
  equivalent subspace rotations; compare subspaces, never raw SVD columns;
- map cluster tangent bases to translation directions, rotation axes and coupled
  screw-like instantaneous freedoms, retaining a raw basis when interpretation is
  ambiguous;
- verify declared canonical motion families (revolute, prismatic, cylindrical and
  planar) against rank and interpreted freedoms without yet persisting them as
  Engineering Connections;
- implement lexicographic/null-space secondary optimization: constraint feasibility
  first, then reference-motion policy, then total nominal change;
- return blocked/allowed instantaneous directions with tolerance and linearization
  provenance.

The gate is semantic stability under body/constraint permutation, global rigid
motion, unit scaling and valid warm starts with the same frozen nominal poses and
branch. The delivered slices are M2.5a pose policy, M2.5b Product integration,
and M2.5c user-facing freedom interpretation. Numeric null-space algebra is needed
for M2.5a; a UI classification of freedoms is not its prerequisite. The weak
policy has been removed; there is no silent weighted or temporary-Fix fallback.

#### M2.5a：从动算法语义与实现（已实现）

这里的“从动”指第二选择 reference 在满足全部约束的前提下尽量保持操作前位姿；第一选择 moving 优先承担变化。
它不是机械动力学的主从传播，也不按“约束条数多”决定固定谁。是否必须移动由整个连通分量的可行空间决定。
第一元素已 Fix、通过 Rigid 间接接地、仅某些方向受限或被闭环约束时，第二元素仍参与同一求解问题，
自然承担不可避免的运动。禁止“临时固定第二元素 → 失败 → 去掉固定重试”，也不能靠增大 reference 权重实现优先级。

令 q 为物理 ground 消元后的 cluster 位姿，q0 为本次操作前冻结的名义位姿，F 为当前 branch 下全部有效几何约束的可行集合。
求解目标按词典序排列：

1. 满足 F；所有 Driving/Controlled、Fix/Rigid、距离侧与角度分支仍是硬语义。
2. 在 F 内最小化 E_ref(q)，即第二选择所属 occurrence 相对 q0 的位姿变化。
3. 在上一级最优集合内最小化 E_all(q)，减少其余 occurrence 的总名义变化。

M2.5 首片采用 reference 优先、其余元素同级的总位移目标；不沿用旧 neutral > moving 权重作为隐含的第三层优先级。
若以后需要“先移动第一元素，再移动其他非 reference 元素”的额外层级，必须显式扩展 policy 与验收，不能暗加权重。
固定分支内只承诺有收敛证据的局部层级最优，不声称任意非凸装配的全局最小位移；跨离散分支搜索仍属于后续里程碑。

位姿度量以 occurrence 的固定局部原点为平移测量点，采用
`||t_i(q)-t_i(q0)||² / L² + ||Log(R_i(q0)^T R_i(q))||² / A²`。
L/A 是冻结的运动度量尺度，与几何可满足性容差分离；单位换算必须同时换算 L。reference 集合按 occurrence ID 去重，
E_all 对选定连通分量全部 occurrence 求和。Rigid 只压缩变量，目标仍从成员 occurrence 位姿计算，避免更换 cluster 代表原点改变偏好。
旋转 Log 在 π 附近需确定性 branch/连续化与导数 oracle；必须与当前 `t += dt, R = Exp(dw) R` 更新约定一致。
不得直接复用当前近似 preference gradient 作为精确目标 Jacobian。

具体在现有 Eigen dense reference backend 内实现，不新增外部优化库：

- 将冻结 nominal pose 与数值 initial guess 分开。warm start、奇异点 seed、可行性恢复均不得重置 q0。
- 用几何 Jacobian J 求可行性恢复步 Δp，并构造其数值零空间 Z；局部步写成 `Δ = Δp + Z y`。
  当前实现先恢复可行性，再以投影 BFGS 优化 reference。reference 残差为零时用 `null(A_ref Z)`；非零时
  使用 reduced Lagrangian Hessian 的零空间，保留目标值最优而残差向量仍可变化的解族，再优化总名义变化。
  二阶曲率通过解析梯度的 Richardson 差分获得；几何 Jacobian 仍是解析微分。
  阻尼和 trust region 只控制步长、稳定分解，不能变成与硬约束竞争的运动权重。
- 当前导出的 null-space 经列反缩放后仅逐列归一，不能假设正交，更不能直接用 `Z Z^T` 作投影。
  在统一无量纲 tangent 度量下重新 QR/SVD 正交化，投影与梯度使用同一尺度，并复用明确的 rank threshold。
- 非线性层使用有界 trust-region/回溯与可行性恢复。初始不可行时优先恢复约束；进入可行带后，候选需同时通过真实几何容差、
  上级目标不劣化和当前层目标改善检查。零空间步只是一阶可行，必须重新求值并校正曲率误差，校正后再次检查全部层级。
  用配置化 objective tolerance 定义数值上的“不劣化”，不得逐步累计放宽上级最优值。
- 删除“几何满足即结束”的单一退出条件。分开记录 geometric feasibility 与 preference optimality；后者检查投影梯度/KKT 残差、
  步长、目标改善和预算。可行但偏好未收敛必须显式报告，不能冒充已完成从动最优化，也不能误报模型约束冲突。
- 仅有一个 reference cluster 且整个 component 无 ground 时，可保留现有 reference gauge 消元作为等价优化：整体刚体变换可令它
  恢复 q0，因此 E_ref 最优值可为零。存在 ground 时 reference 必须保留为自由变量（除非它本身物理固定）；多个 reference cluster
  不得按列表顺序固定第一个，应保留整体 gauge 供目标优化，或证明采用的规约不改变目标最优值。
- rank 改变、退化或 branch 边界必须有诊断与有界恢复，禁止随机重启或以局部 stationary 宣称第二元素全局上“不得不移动”。

#### M2.5b：调用链与验收（已实现）

复用 `assemblyConstraintSolveIntent()`，让 ADD、EDIT、preview、commit 对相同基础 Revision 冻结相同 q0、branch 和 policy。
内核/Proto/Worker/Go/Router 同步贯通运动尺度、偏好收敛状态及每个角色的平移/旋转变化证据；字段使用 typed 契约，
不塞入自由格式 diagnostic 字符串。计算策略变化更新 solver profile/build 与求值缓存身份；当前未发布阶段直接统一调用方，
不维护实验数据兼容路径。所有结果 Pose 与约束继续原子提交并从最终 Revision 重建 ChangeSet。
MOVE 的最近可行目标投影仍由 M4 实现，本门不以现有 `interaction-driver` Fix 路径代替二元约束从动算法。

| 已覆盖的场景 | 验收结果 |
|---|---|
| 两个自由元素，单 reference | 第二元素零变化（运动容差内），第一元素满足约束，仍报告 6 gauge DOF |
| 第一元素 Fix 或经 Rigid 接地 | 第一元素严格不动，第二元素承担必要变化，不能因 reference 偏好报无解 |
| 第三元素接地，第一元素仍可独自完成约束 | 第二元素保持 q0，避免“component 有 ground 就只能近似不动” |
| 第一元素沿 x 滑动，第二元素沿 y 滑动，局部原点初值分别 (0,2,0)/(3,0,0)，新增点重合 | 解为 (3,2,0)，第二元素只作必要的 y 位移 2，第一元素作 x 位移 3 |
| 闭环/多解中第二元素必须动但仍有剩余自由度 | 与小型解析最优解比较 E_ref，再比较 E_all；可行不等于最优 |
| 几何已满足但数值初值偏离冻结 q0 | 继续二级优化，不因初始可行立即退出 |
| 两元素都固定且约束冲突 | 保留真实冲突；不得释放 Fix 或牺牲几何容差 |
| 多 reference、Rigid 成员换代表、输入排列 | 相同目标与解语义；不得依赖第一个 reference 或 cluster 原点 |
| 相邻角度、π 附近、秩变化、长力臂、单位缩放 | 残差/运动连续或明确分支诊断，目标导数 oracle 通过 |
| 同一 q0/branch 的冷启动与热启动 | 比较可行性、层级目标与已知局部解族；改变 q0 不冒充 warm-start 等价测试 |
| 正式 Router 与 Product 生命周期 | 创建/编辑一致，preview 不写历史，提交/刷新、连续两次 Undo/Redo 与 capability 一致 |

每层目标输出尺度、最终值与最优性残差；失败 fixture 固定输入与 branch，不只断言 `Converged`。
在相同构建配置下记录 5/15/30-body 耗时、迭代数、可行性残差和 reference 位移；性能不能以降低上述优先级换取。
#### M2.5c：瞬时自由度解释（已实现）

每体 allowed 子空间来自 component 的几何零空间经 occurrence 力臂映射；blocked 是相同无量纲度量下的正交补。
规范化通过子空间投影与确定性正交化完成，测试比较 projector，不依赖原始 SVD 列。
纯平移空间和 angular image 分别给出平移维数/方向、旋转维数/轴点/pitch；转动、滑动、圆柱、平面及球面族需满足对应
子空间关系（球面要求转轴共享中心），其他组合明确标为 Coupled。无 ground 时相对稳定 occurrence 解释并单列全局 gauge，
输出 relative-to ID、linearization pose、rank threshold 和运动尺度；它不声称存在有限的自由行程。

Worker/Go/Router 携带 typed freedom/preference enums 与完整证据，Product 预览通过现有 preview actor 展示第二元素变化和自由度，
REQUEST/RESET/CANCEL 清理旧证据，旧 sequence 的响应不覆盖新状态。提交不持久化额外求解状态机；最终 Pose 仍进入同一
Revision/ChangeSet，连续 Undo/Redo 由原历史语义处理。当前 `solver_build=assembly-m2.5-hierarchy-v2`，profile schema 为 2。
装配求解当前没有结果缓存；无需通过修改无关的 Part evaluator 来假装失效装配缓存。持久 solve manifest/provenance 是 M3。


### M3: replayable Product solve manifest

Stable input precedes richer interaction. The control plane freezes a versioned,
content-digested manifest containing:

- root Product Revision, typed relative `InstancePath`, immutable
  `ResolutionSnapshot` and complete occurrence-local nominal poses;
- Publication or PersistentSelection endpoints plus resolution evidence;
- descriptor kind, local frame, symmetry, source geometry identity and provenance;
- connection/constraint definitions, parameter values, branch intent, tolerance
  profile, solver build and requested affected scope;
- explicit rigid-subassembly expansion; flexible expansion remains a later Product
  capability but must fit the same manifest.

The solver never queries Product, PostgreSQL or B-Rep. Broken, ambiguous or
incompatible references fail during manifest construction with stable diagnostics.
Current direct-Part datum and `geometryKey + topology local ID` references are
development-only inputs and are removed when the M3 path is complete; the project
is still pre-release, so no permanent dual model is introduced.

M3 is accepted only when replaying the same manifest after workspace-head changes
produces a semantically equivalent result, nested rigid Product occurrences solve
correctly, and changing a referenced Part either resolves the same publication or
returns an explicit broken/ambiguous-reference result without silent rebinding.

### M4: branch-stable constrained manipulation

- persist static discrete branch intent in Product while keeping iteration branch
  state immutable inside one solve;
- use explicit Publication/Datum axis and sense for `DirectedAngle`; static assembly
  stores modulo `2*pi`, while unwrapped/winding state belongs to a versioned preview,
  kinematics or simulation session;
- create a short-lived preview session bound to base workspace sequence and solve-
  manifest digest, carrying accepted pose and branch snapshots as warm starts;
- replace the temporary MOVE `Fix` with a secondary drag objective projected onto
  the driving-constraint manifold using the M2/M2.5 null space;
- return nearest feasible pose plus allowed/blocked directions when the target is
  unreachable; cancellation, deadline and stale-sequence checks run inside solve
  iteration as well as at RPC boundaries;
- incrementally recompile only when graph/definition digests change; pose-only drag
  iterations reuse the immutable compiled problem, never a hidden business state.

Only pointer-up commits one Domain Transaction. Preview sessions are disposable,
request-scoped acceleration and continuity state, not Revision authority.

### M5: localized diagnostics and repair guidance

- construct conflict neighborhoods from the incidence graph and changed constraint;
- combine structural rank evidence, branch candidates and bounded deletion filtering
  or QuickXplain-style search;
- distinguish proven minimal, irreducible, localized-suspect and merely unsatisfied
  results in the type contract;
- return geometry/branch/tolerance evidence and stable connection, constraint and
  equation IDs suitable for tree/viewport highlighting;
- propose suppress/measure/reconnect actions, but never mutate the Product or select
  an alternative branch without an explicit command.

M5 is gated by deterministic multi-constraint conflict corpora and fault budgets;
an exhausted diagnostic budget returns partial evidence rather than a false MUS.

### M6: Engineering Connections and constraint coverage

Add capabilities in dependency order, each gated by equation, Jacobian, freedom,
branch and diagnostic corpus:

1. typed `Offset`, `Parallel` and `Perpendicular` definitions over existing geometry;
2. Frame/Connector Publications and declared rigid, revolute, prismatic,
   cylindrical and planar Connections, verified against M2.5 freedoms;
3. analytic plane/plane and cylinder subsets of positioning `Contact`;
4. Circle, Sphere and Cone descriptors;
5. bounded distance/angle and joint limits;
6. spherical, universal and screw Connections;
7. only then evaluate point/curve, gear, rack and general curve/surface contact.

Arbitrary NURBS-to-NURBS contact is not a near-term positioning primitive. Datum
and Connector Publications should express stable engineering intent first.

### M7: scale, scheduling and backend decision

- establish representative connected-component benchmarks before adopting sparse
  QR/Cholesky, Ceres or another backend;
- add block-sparse storage, symbolic-pattern caching and parallel independent
  components only where measurements justify them;
- enforce body/equation/iteration/time/memory limits and cooperative cancellation;
- version solver provenance and run deterministic shadow comparisons before changing
  the authoritative backend;
- split an Assembly Solver Worker from the Geometry Worker only when workload,
  isolation or scaling evidence requires a deployment boundary; keep one coarse-
  grained immutable-manifest RPC.

M7 does not distribute one connected component across RPC calls. Mechanism time
integration, flexible-subassembly overrides and dynamics remain Product/Kinematics
milestones built on the same equation and identity contracts, not extensions of the
static placement solve loop.

## 9. Next vertical slice and acceptance criteria

The M2.5 baseline now includes hierarchy, Product integration and instantaneous
freedom interpretation. The next slice is M3: replayable solve inputs and stable
Product references, reusing PersistentSelection/Publication rather than inventing
a second topology identity. Keep the M2.5 motion, subspace and Router/history corpus
as its compatibility gate. MOVE projection is still M4; sparse/incremental execution
remains benchmark-led M7 work.

## 10. Explicit non-goals for the next milestone

- copying CATIA UI or proprietary formats;
- treating Ceres or another optimizer as the domain architecture;
- arbitrary freeform surface contact;
- mechanism time simulation, collision dynamics or force propagation;
- distributing one connected constraint component across RPC calls;
- persisting OCCT local topology IDs as stable assembly identity;
- claiming CATIA-level capability from visual placement alone.

The immediate objective is a small, explainable and well-tested constraint-manager
core. Broader geometry and mechanism coverage should be built on that foundation.
