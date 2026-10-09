# 三维装配求解性能记录

最新复测见 [2026-10-09 Release 内核对比](#2026-10-09一键命令与-release-内核复测)。以下第一轮统计保留原测量条件与结论。

## 第一轮：2026-10-08

测量日期：2026-10-08。基线 HEAD：`e94f3695a0460ed5a1ad036b2977f467e1b95246`。
本轮交付了可重复运行的串行测量、轻量阶段计时、等价复用、Session 并发回归和侧栏最后请求耗时。
下面是这台机器的有限样本结果，不代表稳定 P95、工业容量或 60 Hz。

## 环境与边界

- Intel i7-4900MQ @ 2.80 GHz，8 个逻辑 CPU，WSL2 Linux 6.6.114.1，GNU C++ 15.2、Go 1.26.5。
- 项目 C++ 编译参数为 `-m64 -O3 -DNDEBUG -g0`；ASAN/UBSAN/TSAN 关闭。复用已安装的 Debug Conan 依赖，CMake 配置名仍为 Debug。**这是项目优化构建，不是完整 Release 依赖构建**。Release 配置需要另建依赖，本轮未完成。
- 修改前先加入相同的观测与测量入口，保留原算法与服务调用路径；然后修改实现，在相同目录与配置重建。每轮串行运行 native → Router/Session → allocation probes；性能采样期间没有并行构建或测试。没有绑定 CPU 或控制频率。
- 内核和真实 Go Client → Router → C++ Worker 分别测量。Session 测量包含真实 PostgreSQL 上下文读取、冻结输入、求解、模型更新与响应对象构造。没有 HTTP 网关、浏览器绘制或鼠标输入延迟。
- 每场景 5 次。静态平面链入口输出 5 次平均，其阶段指标属于最后一次；其余表格输出 5 次中位数。阶段中位数不必来自同一次请求，不做加和。
- Router 日志 `cold=true` 表示该场景第一次、没有 initial guess；之后 4 次使用上次解。Worker 已在 fixture 中启动，后续场景第一次也不是进程冷启动。Session 第一帧和后续帧同理。另有真实新 Worker 冷回放回归，但没有测量进程启动耗时。
- 使用配置的专用测试 PostgreSQL、测试 fixture 临时 ArtifactStore 与现有解析几何 fixture；没有重置应用数据库、S3 或其他开发数据。
- 原始输出留在 `build/performance/assembly-round1/{before,after}/`，包括环境、source SHA256、全样本、阶段和分配信息。修改前 instrumented 源码留在该目录的 `source-before/`；这些运行产物不提交为静态 fixture。下文保留本次各场景全部总时间样本。
- Before solver SHA256：`a1427911cf4f13a08ac04b6f0e66e0b26e09a9c9cae089496bb9ae44c89498d5`；After：`3070dc782d845b2729001531c0a5c0cc70ed7c255463b8035a1531a5fba97698`。

## 计时定义

`SolveMetrics` 默认只保留请求级标量和计数，不保存矩阵、逐迭代轨迹或额外几何。Go `performance.Start` 无 recorder 时直接返回空操作；有 recorder 时复用现有记录器。没有改 Proto，Worker 仍通过已有 summary 回传总求解、编译、hard、preference、retraction；新增细分指标由 native 入口输出。

| 指标 | 包含范围与重叠 |
| --- | --- |
| kernel / solve | 编译、求解和诊断的总 wall time；不是 CPU 时间 |
| input_compile | 验证、索引、cluster、branch、端点、连通分量；After 也覆盖抛异常，Before 的失败编译计时缺失 |
| hard_feasibility | 主硬约束恢复循环；不包含后续 preference 中的 retraction |
| preference | 完整运动优先级阶段，**包含**内部 retraction、分解、BFGS、pose 与 residual/Jacobian |
| retraction | 子阶段累计，通常属于 preference；不得再加到 preference |
| dof_analysis | 最终 Jacobian、rank/null space、自由度解释和分类输出；不是只有 SVD |
| redundancy | 各约束 rank／冗余分析，包含重复分解；拖动已有路径跳过该阶段 |
| factorization | SVD/QR/LDLT、秩与零空间 helper 及局部基构造，包含 helper 中缩放和基运算；横跨 hard/preference/diagnostics，**不是互斥阶段**，计数也包括空矩阵 helper |
| BFGS / pose | BFGS 更新表达式、全局或组件 pose 构造的子计时；已属于上层阶段 |
| Go request-compile / response-decode | 领域值到 Proto 请求、Proto 结果到 Go 结果；不把它们称为完整序列化成本 |
| Go assembly-rpc | Client RPC 调用的 wall time，包含编码、传输、Router 调度、Worker 与解码；已有 assembly-worker 是它的外层，不能相加 |
| Go base-check / edit-context | 两次 Head/Workspace 和 EditContext 校验；edit-context 属于 base-check；数据库计时属于这些阶段 |
| Go update-prepare / publish | 输入准备、结果合成与最终候选持久化；copy、nominal、JSON 等属于 publish |
| marshal / response JSON probe | 测量之后独立执行的 Proto.Marshal 或 frame JSON 校准；并未加到 RPC／Session 总时间 |

已核实原拖动实现跳过重型约束冗余、方向探索和冲突 probes。本轮没有再次削减诊断：最终 rank/DOF、受影响完整连通分量、硬约束与目标资格仍计算。

## 实际优化与失效规则

1. **重复物理零空间**：一个组件的 `optimize_motion` 内保留最近一次 physical kernel；所有 pose 标量严格相等才复用。pose 改变立即重新计算；约束、分支、缩放与 options 在该调用内不可变。缓存在函数结束销毁，不跨组件、求解或 RPC，不依据几何近似猜测。四类拖动每次命中 5 次，静态链命中 2 次。DOF 分析使用已有最终 Jacobian，但仍计算自己需要的 scaled kernel，避免改变 rank 阈值和尺度语义。
2. **组件 pose**：CompiledAssembly 保存不可变初始 body poses，组件求值复制这组纯值并只 compose 自己的自由 cluster。最终完整解仍构造所有已求解 poses。没有引入跨请求 Worker 数值缓存；每次模型、branch 或 profile 变化都重新编译。独立 50 体 pose 子成本下降明显，仍有 O(N) 值复制和分配。
3. **EditContext**：校验从 `GetProductDesignSession` 的两次展开／publication catalog 加第三次展开，改为一次 `productContextRoot` 的完整快照、路径和 owner 校验。该次校验的只读 row cache 按 document/revision 去重；相同 Part/Product 的重复 occurrence 复用读取。每次 RPC 前后重新创建 cache，Head、版本、删除状态仍重新读取；不跨帧缓存 mutable Head。PINNED 路径继续拒绝。
4. **Session 数据**：Begin 深拷贝 caller 输入，保留不可变 snapshot 与 nominal/constraint/geometry 索引；取消、过期、Head/context 变化后 Session 失效。模型仅复制写入的 Instances/Constraints 值 slice；嵌套定义与路径只读；branch 与 measured 值替换为新值。全模型响应、JSON/hash 和正式校验保留，没有 delta 协议。
5. **锁**：`inFlight` 保证同一 Session 单一 update 写者；全局锁保护生命周期、序列和 token，不包围 DB、RPC、digest/JSON 或模型合成。发布与 commit 在锁内再次检查 membership；最终持久化后重新检查 base/cancellation。不同 Session 可以取消，迟到结果不生成 token；Revision/CAS 与补偿历史路径未改。
6. **BFGS**：常规静态链／四类拖动根本没有执行更新。补充 warm rotation 30 体场景实际执行 60 次，Before median BFGS 约 0.055 ms，占其 6.990 ms 总耗时不足 1%。本轮保留原一般稠密矩阵乘法，没有为理论复杂度改变高风险更新公式。

前端复用有界 200 条 performance samples，在侧栏底部显示“后端请求：N ms”。HTTP 控制请求计时到响应正文处理结束，WebSocket 请求计时包含连接等待至完整响应；成功、失败、取消都更新，多个并发请求按最后完成顺序显示。命令多个阶段分别覆盖最后值，不累加；初始化无请求显示 `—`。这是浏览器观察的粗略 wall time，不是服务端 CPU 指标；资源加载和渲染不计入这个控制请求读数。

## 本次性能对比

单位 ms。变化列为耗时变化，负数表示改善。Before/After 使用相同场景与判定。

### Native 静态链与 BFGS 补充

| Bodies | 约束 | 组件 | rank | hard / preference 迭代 | Before 平均 | After 平均 | 变化 | residual（两轮相同） | 有效 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 5 | 5 | 1 | 12 | 3 / 2 | 0.743 | 1.270 | +70.9% | 4.50718e-25 | true / true |
| 15 | 15 | 1 | 42 | 4 / 2 | 15.074 | 9.430 | -37.4% | 3.96079e-23 | true / true |
| 30 | 30 | 1 | 87 | 5 / 2 | 277.647 | 192.799 | -30.6% | 3.12402e-22 | true / true |

5 体平均本次退化 **70.9%**，不能宣称所有静态场景都提速。该入口没有输出逐样本总时间，最后样本子计时不足以解释平均值变化，本轮保留该观察、不据此归因。15/30 体分别改善 37.4% / 30.6%。warm rotation 30 体（0 约束、30 组件、每次 60 BFGS 更新）5 次 median 为 6.990 → 1.423；全部 nominal objective < 1e-12，属于补充场景。

静态 30 体**最后样本**的 phase，不是上表平均的拆分：

| 阶段 | Before | After |
| --- | --- | --- |
| compile_ms | 0.1067 | 0.0903 |
| hard_ms | 18.7735 | 13.2118 |
| preference_ms | 70.4865 | 27.6751 |
| retraction_ms | 17.3008 | 13.1611 |
| dof_ms | 33.0721 | 26.4168 |
| redundancy_ms | 156.935 | 121.071 |
| factorization_ms | 269.926 | 183.008 |
| pose_ms | 0.406197 | 0.2663 |

### Native 四类交互和失败路径

| 场景 | Bodies / active约束 | 组件 / 更新 | rank | hard / intent迭代 | Before median | After median | 变化 | 提交资格 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| single | 1 / 0 | 1 / 1 | 0 | 0 / 4 | 0.088 | 0.075 | -15.3% | true |
| connected50-200 | 50 / 200 | 1 / 1 | 294 | 0 / 4 | 1506.020 | 687.463 | -54.4% | true |
| independent50 | 50 / 0 | 50 / 1 | 0 | 0 / 4 | 1.453 | 0.568 | -60.9% | true |
| group-contact | 4 / 4 | 1 / 1 | 1 | 0 / 4 | 0.282 | 0.230 | -18.5% | true |
| grounded-failure | 2 / 3 | 1 / 1 | 0 | 0 / 0 | 0.029 | 0.028 | -0.7% | false |
| cancelled | 50 / 200 | 0 / 0 | 0 | 0 / 0 | 0.387 | 0.381 | -1.7% | false |
| budget | 50 / 200 | 1 / 1 | 294 | 0 / 1 | 678.443 | 677.458 | -0.1% | false |
| invalid-input | 50 / 200 | 0 / 0 | 0 | 0 / 0 | 0.083 | 0.084 | +0.5% | false |

四类正常交互全部 valid、hard feasible；grounded-failure 是明确 grounded contradiction，不可提交；cancelled 在编译后退出，invalid-input 是未知几何输入错误；budget 硬可行且全局 solve status 为 Converged，但交互为 Budget、不可提交。`valid=true` 在失败场景表示**按预期拒绝**，不是得到可提交解。取消／输入错误没有最终 component 报告，不能把 0 报告当成真实模型没有组件。

Native 子阶段 median（Before → After，不可加和）：

| 场景 | 编译 | hard | preference | retraction | DOF | 分解 helpers | pose |
| --- | --- | --- | --- | --- | --- | --- | --- |
| connected50-200 | 0.380 → 0.365 | 1.780 → 1.581 | 1169.170 → 337.616 | 0.642 → 0.647 | 334.356 → 347.451 | 1469.900 → 671.441 | 1.589 → 1.452 |
| independent50 | 0.077 → 0.082 | 0.006 → 0.002 | 0.324 → 0.071 | 0.028 → 0.003 | 0.977 → 0.349 | 0.046 → 0.047 | 1.073 → 0.270 |
| group-contact | 0.017 → 0.018 | 0.009 → 0.009 | 0.143 → 0.101 | 0.014 → 0.013 | 0.088 → 0.080 | 0.099 → 0.085 | 0.025 → 0.015 |
| budget | 0.380 → 0.365 | 1.761 → 1.633 | 337.067 → 340.959 | 0.650 → 0.632 | 338.744 → 336.418 | 661.818 → 662.912 | 1.398 → 1.418 |

connected 的 factorization helper 调用 465 → 460、cache hit 5，减少的是重复**大物理 kernel**；remaining canonical freedoms 仍产生大量小分解。independent50 pose 构造次数 268 → 247，median 子时间 1.073 → 0.270；不是全模型值复制完全消失。拖动 redundancy/BFGS 子时间均为 0，原路径已跳过冗余、这些目标未触发 BFGS。invalid-input 的 After 编译 median 0.058 ms；Before 计时只在成功构造末尾写入，失败值 0 代表未采集。

### 真实 Client/Router/Worker 和 Session

| 场景 | Before median | After median | 变化 | 第一请求 | 后续4次 median |
| --- | --- | --- | --- | --- | --- |
| plane5 | 5.277 | 5.134 | -2.7% | 8.474 → 8.046 | 5.257 → 5.127 |
| plane15 | 20.488 | 19.237 | -6.1% | 26.024 → 21.871 | 20.340 → 18.685 |
| plane30 | 250.352 | 216.412 | -13.6% | 288.173 → 254.398 | 249.962 → 215.723 |
| single | 3.742 | 3.882 | +3.7% | 4.355 → 4.720 | 3.737 → 3.881 |
| connected50-200 | 1522.329 | 687.409 | -54.8% | 1521.747 → 685.491 | 1788.280 → 788.058 |
| independent50 | 18.493 | 18.763 | +1.5% | 18.838 → 18.023 | 18.249 → 19.020 |
| group-contact | 4.868 | 4.768 | -2.1% | 4.868 → 5.279 | 4.957 → 4.724 |
| grounded-failure | 2.879 | 2.812 | -2.3% | 3.071 → 2.812 | 2.792 → 2.740 |
| budget | 684.916 | 697.201 | +1.8% | 686.963 → 691.051 | 683.886 → 700.773 |
| session-False | 9.024 | 9.327 | +3.4% | 9.415 → 9.462 | 8.792 → 8.681 |
| session-True | 965.832 | 35.557 | -96.3% | 965.832 → 36.889 | 930.518 → 35.153 |

Router 规模、rank/DOF、资格与 native 对应；static rank/DOF 为 12/12、42/42、87/87，第一次 hard 迭代 3/4/5，后续为 0，preference 均 2；正常交互 hard 0、preference 4，budget 为 1。single DOF 6，connected 6，independent 300，group-contact 5，grounded-failure 0；两轮一致。

Session Product 有 50 个 occurrence、无约束，数值 manifest 选中完整的一个独立组件（1 body、rank 0、DOF 6），每帧 intent 4 次；响应仍包含全部 50 个 pose，约 6.6 KB。nested 场景有三层 Product、两个重复子装配路径，展开含 root 共 105 个 occurrence、4 个不同 document/revision。两轮有效且未改变 Head。Begin 单次无上下文 6.768 → 11.930；有上下文 496.947 → 20.355。Begin 只有一次，不解释为稳定启动分布。

小场景的服务总时间有退化：single +3.7%、independent50 +1.5%、无上下文 Session +3.4%、budget +1.8%。本轮不把 native 改善扩展为所有端到端场景提速。

| 场景 | 观测阶段 | Before median | After median |
| --- | --- | --- | --- |
| connected50-200 | assembly-request-compile | 0.261 | 0.325 |
| connected50-200 | assembly-rpc | 1521.593 | 686.943 |
| connected50-200 | assembly-kernel | 1512.639 | 678.038 |
| connected50-200 | assembly-response-decode | 0.141 | 0.130 |
| independent50 | assembly-request-compile | 0.019 | 0.017 |
| independent50 | assembly-rpc | 18.391 | 18.622 |
| independent50 | assembly-kernel | 14.427 | 14.309 |
| independent50 | assembly-response-decode | 0.073 | 0.094 |
| session-True | assembly-request-compile | 0.004 | 0.004 |
| session-True | assembly-rpc | 3.972 | 4.795 |
| session-True | assembly-kernel | 1.392 | 1.348 |
| session-True | assembly-response-decode | 0.009 | 0.010 |
| session-True | assembly-edit-context | 957.284 | 26.213 |
| session-True | assembly-model-copy | 0.331 | 0.010 |
| session-True | assembly-nominal-compare | 0.029 | 0.016 |
| session-True | assembly-model-serialize | 0.053 | 0.073 |
| session-True | db-query | 928.492 | 22.159 |

Go Proto.Marshal 校准 median：connected 0.229 → 0.235（第一请求约 20.7 KB）；independent 0.032 → 0.034；nested frame JSON 0.062 → 0.064。RPC 是包含 Worker 的区间，不能把 RPC 减掉独立测量的 native median 当作纯网络成本。

### 分配 probes

这些既有微基准每项 3 × 200 ms，与真实 Router 延迟分开，使用同一输入；RPC probe 是序列化/编译测试，不是网络 RPC。原实现未优化的 Freeze/Digest 仍是明显分配来源。

| probe | median ms/op | median B/op | median allocs/op |
| --- | --- | --- | --- |
| Freeze | 2.078 → 2.062 | 567030 → 564753 | 9165 → 9165 |
| Digest | 1.712 → 1.790 | 453183 → 451176 | 8616 → 8615 |
| ReferenceKey | 0.007 → 0.007 | 1856 → 1856 | 2 → 2 |
| RPC | 0.148 → 0.147 | 64104 → 64104 | 442 → 442 |
| Replay | 0.499 → 0.492 | 139921 → 139863 | 2317 → 2317 |

### 总时间样本存档

以下数组均按采样顺序，单位 ms。静态链入口只输出上述平均；其余完整时间样本保留在这里，避免后续把 5 次测量当成稳定 P95。

| 场景 | Before 5次 | After 5次 |
| --- | --- | --- |
| native single | 0.214, 0.099, 0.081, 0.088, 0.087 | 4.186, 0.075, 0.067, 0.074, 0.082 |
| native connected50-200 | 1506.020, 1500.640, 1473.890, 2039.840, 2056.460 | 558.537, 687.463, 662.611, 873.830, 920.718 |
| native independent50 | 1.453, 1.470, 1.433, 1.438, 1.456 | 0.561, 0.603, 0.729, 0.568, 0.548 |
| native group-contact | 0.312, 0.282, 0.278, 0.273, 0.306 | 2.420, 0.238, 0.230, 0.226, 0.224 |
| native grounded-failure | 0.037, 0.029, 0.034, 0.025, 0.022 | 0.036, 0.028, 0.034, 0.025, 0.022 |
| native cancelled | 0.384, 0.376, 0.398, 0.387, 0.416 | 0.396, 0.381, 0.380, 0.377, 0.390 |
| native budget | 669.183, 678.443, 665.987, 864.603, 922.278 | 670.269, 677.458, 654.508, 885.751, 918.255 |
| native invalid-input | 0.173, 0.089, 0.083, 0.079, 0.078 | 2.524, 0.093, 0.084, 0.080, 0.078 |
| service plane5 | 8.474, 5.051, 5.302, 5.237, 5.277 | 8.046, 5.120, 4.767, 5.134, 5.358 |
| service plane15 | 26.024, 20.643, 19.829, 20.191, 20.488 | 21.871, 19.237, 19.368, 17.182, 18.133 |
| service plane30 | 288.173, 249.572, 254.539, 250.352, 248.423 | 254.398, 214.174, 216.412, 217.127, 215.035 |
| service single | 4.355, 4.313, 3.742, 3.579, 3.732 | 4.720, 3.926, 3.880, 3.882, 3.813 |
| service connected50-200 | 1521.747, 1518.970, 1522.329, 2054.231, 2066.791 | 685.491, 687.409, 673.652, 888.706, 943.565 |
| service independent50 | 18.838, 19.311, 18.005, 17.678, 18.493 | 18.023, 18.444, 20.593, 19.276, 18.763 |
| service group-contact | 4.868, 4.536, 4.447, 5.379, 6.738 | 5.279, 5.105, 4.768, 4.481, 4.679 |
| service grounded-failure | 3.071, 2.879, 2.705, 2.543, 2.898 | 2.812, 2.892, 2.653, 2.659, 2.820 |
| service budget | 686.963, 684.916, 667.968, 682.857, 861.632 | 691.051, 697.201, 683.238, 704.346, 883.504 |
| service session-False | 9.415, 8.560, 10.267, 9.024, 7.819 | 9.462, 9.327, 9.628, 7.612, 8.034 |
| service session-True | 965.832, 886.025, 967.643, 893.394, 975.792 | 36.889, 34.750, 31.630, 35.557, 41.291 |
| native warm rotation | 10.211, 6.990, 7.038, 6.981, 6.939 | 1.476, 1.376, 1.816, 1.423, 1.329 |

## 验证与限制

- 优化构建 assembly scenarios + corpus：230/230 通过；最终 source 再跑同一 CTest 范围通过。
- workspace 整包与 performance 包通过；真实 Router/Worker 装配回归在较宽 control 检查中通过。额外四项 `-race`：累计拖动意图、取消时最终持久化、嵌套上下文失效、提交／Undo／Redo／Release／冷回放通过。
- 新上下文回归覆盖 root/active Revision、typed path、PINNED、caller 指针修改、删除子文档；新并发回归让持久化即使忽略取消仍延迟完成，验证两 Session 取消及时、无迟到 token、不能提交、Head 不变。PINNED 补充后还做了正常串行重跑。
- 最终性能 fixture 全部通过独立几何断言、未选体保持、失败资格、预算资格与预览 Head 不变；嵌套 root 变更后的继续帧被拒绝。
- Web 9 个受影响 scenarios、TypeScript、production build 通过；重启 Playwright 页面后的侧栏真实 HTTP 计时用例通过。没有做真实装配拖动的 WebGL／人工流畅性验收。
- 较宽 Go 检查有一个 Shell 子场景失败：`TestCppWorkerShellAfterFilletAndMixedLoft/offset-rectangle-larger-circle`，`SHELL_OFFSET_TOLERANCE_EXCEEDED`。Shell 路径未改，未对原 HEAD 做该失败的复现，不能宣称整个 Go scope 通过或已定位该失败根因。
- 同次较宽检查中在仍会启动 Worker 时重建了同一路径，导致 15 个其他 control 测试 `permission denied`。这是验证安排错误；停止构建后串行重跑这 15 项与两项新增回归，全部通过（187.273 s）。性能采样未受该并行重建影响。
- `invoke check --scope web` 被 `m4.snapshot.current-render-bindings` contract summary lock 阻断。该 fixture、contract 生产代码、catalog、evidence 与 baseline 文件已逐一核对与本轮修改前 HEAD 字节一致，本轮没有改锁来绕过失败。
- 额外扩大 `workbench` 路径过滤时，`feature-association.scenario.mjs:38` 的 generator source 断言得到 `[]` 而期望 `[blade]`。该测试及 feature-picking、feature-association、feature-selection、viewport engine、tree projection 生产文件均与 HEAD 字节一致；没有扩大本轮范围修改实体 Feature 行为或宣称所有 workbench scenarios 通过。
- 本轮不能给出稳定尾延迟或并发吞吐量；取消竞态回归也不等于并发容量测试。测试数据库 cache、机器频率、第三方 Debug 库与小样本波动仍影响数据。

## 复现

先按仓库开发环境加载既有配置和 fixture；下面 runner 会复用 `tasks` 的 env 加载，不要把含特殊字符的 env 文件直接 shell source。

```sh
cmake -S . -B build/cmake/performance -G Ninja \
  -DCMAKE_BUILD_TYPE=Debug -DCMAKE_CXX_FLAGS_DEBUG='-O3 -DNDEBUG -g0' \
  -DCMAKE_TOOLCHAIN_FILE="$PWD/build/cmake/debug/build/Debug/generators/conan_toolchain.cmake"
cmake --build build/cmake/performance --target \
  occcad_assembly_solver_scenarios occcad_assembly_solver_corpus \
  occcad_assembly_solver_benchmark occcad_assembly_interaction_benchmark \
  occccad_geometry_worker -j 2
python kernel/assembly/tests/run_performance.py --label after --samples 5
ctest --test-dir build/cmake/performance -R '^(assembly/|assembly-corpus/)' --output-on-failure
invoke check --scope workspace --match 'TestAssemblyInteraction|TestAssemblyManifest|TestInteractionCandidate'
pnpm --dir web/apps/cad test -- request-time realtime-control realtime-lifecycle \
  workbench-command-model workbench-visual workbench-more-commands \
  workbench-inspector workbench-registration-lifecycle latest-preview-queue
pnpm --dir web/apps/cad typecheck
pnpm --dir web/apps/cad build
pnpm --dir web/apps/cad exec playwright test browser/request-time.spec.ts
invoke context-audit
git diff --check
```

回归的真实 Worker 必须指向 `build/cmake/performance/workers/geometry/occccad_geometry_worker`，使用 `OCCCCAD_TEST_GEOMETRY_WORKER` 覆盖配置；Go 命令从 `services/` 执行。`go test ./internal/control -run '^TestAssemblyInteraction(CumulativeIntentTrace|CancellationDuringFinalPersistence|NestedContextInvalidation|RouterCommitHistory)$' -race -count=1 -timeout=5m` 为本轮四项 race 复现入口。

测量修改前使用同一配置、尚未优化的 instrumented 源码及 runner 的 `--label before`，完成后才改代码。`--label` 只是输出目录标签，**不会自动还原源码**；不要用优化后源码覆盖已记录的 before。重新比较需在独立 checkout 保留待比较版本和一致 instrumentation/fixture，然后先完成构建再串行采样。`--stages static interaction` 可只测 native；完整结果必须同时包含 service。配置 env 可由 `OCCCCAD_ENV_FILE` 指定。不要在仍有测试启动该 Worker 时重建它。

## 剩余热点

连接 50 体／200 约束的稠密 rank/null-space 与自由度解释仍是主成本；预算退出同样保留最终 DOF，而这个用例没有缓存命中，不能指望相同收益。静态 30 链的冗余分析仍重，拖动原本已跳过它。独立 50 体的 native 改善没有转化为相同服务端收益，真实 Worker summary 本身仍约 14 ms，需要进一步对齐 Worker 运行上下文再归因，不能全部算作网络。

嵌套校验还有两次 fresh DB 读取与 JSON 解码／路径构造，但没有跨帧缓存 Head。全模型复制的主要 JSON round-trip 已移除，完整响应、序列化、canonical hash/digest 仍保留。allocation probes 的 Freeze/Digest 分配数量基本没变。BFGS 大自由度用例、真正进程冷启动、长时间尾延迟、并发容量及真实拖动验收留待后续。稀疏后端、机构仿真、DMU 和部署拆分未纳入本轮。

<!-- Measured tables below are generated from this run's retained logs. -->

## 2026-10-09：一键命令与 Release 内核复测

本次新增并实际运行 `invoke performance.assembly`。默认只测三维求解器 Native，不编译项目、不访问数据库、不启动应用进程；复用维护者刚完成的 Release 构建。旧的第一轮结果保留在上文，本节与第一轮 **After** 比较。

### 复现命令与证据

```sh
invoke performance.assembly
invoke performance.assembly --samples=20
# 可选：扩展到 Router/Session 与 Go 分配 probes，需要专用测试 PostgreSQL 和解析几何 fixture。
invoke performance.assembly --stages=static,interaction,service,allocations
```

- 默认每场景 5 次，串行运行静态链/BFGS → 交互/失败路径；本次使用默认命令，未运行可选 service/allocations。
- 原始日志和机器可读统计：`build/performance/assembly/20261009T140739937053Z/{environment.json,static.txt,interaction.txt,summary.json}`；这些本地制品不作为静态 fixture 提交。每次生成唯一 UTC 时间戳目录，显式重用 label 会失败，不覆盖旧统计。
- 测量开始：`2026-10-09T14:07:39.976071+00:00`（UTC）；机器：Intel(R) Core(TM) i7-4900MQ CPU @ 2.80GHz，8 个逻辑 CPU；系统：`Linux-6.6.114.1-microsoft-standard-WSL2-x86_64-with-glibc2.43`。
- HEAD：`1882d398c336366ff4c36973fe51fc26991ae82c`；测量时未提交改动仅为命令与 Python runner。未修改或重建 C++ solver、benchmark 或 Worker。
- CMake 配置：`Release`；编译器：`c++ (Ubuntu 15.2.0-16ubuntu1) 15.2.0`；实际参数：`-m64 -O3 -DNDEBUG`；ASAN/UBSAN/TSAN 均为 OFF。
- Solver SHA256：`3070dc782d845b2729001531c0a5c0cc70ed7c255463b8035a1531a5fba97698`，与第一轮 After 一致。第三方依赖此次由维护者完整 Release 重建；第一轮则使用 Debug Conan 依赖和手动优化的项目编译。
- static binary SHA256：`4a81044a6156ab3af1d277c428e68b19d99488b83d59bfac13a3e3b6840098e8`；interaction binary SHA256：`d757b50f96eea0f43c06520c678b868ed2f9d17c61f8e58314f5f31de36722b6`。
- 采样期间没有由本任务启动的并行构建、测试；已有应用/开发工具继续运行。没有绑定 CPU、控制频率或消除系统背景负载。

### 与第一轮 After 对比

单位 ms。静态 plane5/15/30 使用 5 次平均，其余使用 5 次中位数；变化按保存的上一轮原始值计算，表中时间四舍五入。阶段包含关系仍遵循上文，不能相加。

| 场景 | 统计量 | 第一轮 After | 本次 Release | 耗时变化 | 提交资格 / 判定 |
| --- | --- | --- | --- | --- | --- |
| plane5 | mean | 1.270 | 0.516 | -59.4% | 正常求解 |
| plane15 | mean | 9.430 | 10.200 | +8.2% | 正常求解 |
| plane30 | mean | 192.799 | 190.107 | -1.4% | 正常求解 |
| warm-rotation30 | median | 1.423 | 1.461 | +2.7% | 正常求解 |
| single | median | 0.075 | 0.076 | +0.9% | 可提交 |
| connected50-200 | median | 687.463 | 677.770 | -1.4% | 可提交 |
| independent50 | median | 0.568 | 0.561 | -1.2% | 可提交 |
| group-contact | median | 0.230 | 0.238 | +3.7% | 可提交 |
| grounded-failure | median | 0.028 | 0.030 | +6.0% | 按预期不可提交 |
| cancelled | median | 0.381 | 0.390 | +2.3% | 按预期不可提交 |
| budget | median | 677.458 | 684.701 | +1.1% | 按预期不可提交 |
| invalid-input | median | 0.084 | 0.080 | -3.7% | 按预期不可提交 |

50 体／200 约束拖动中位数为 677.770 ms，较上一轮 687.463 ms 减少 1.4%；30 体静态链平均减少 1.4%，5 体静态链平均减少 59.4%。15 体静态链增加 8.2%，warm rotation 增加 2.7%，group-contact 增加 3.7%，小场景及失败路径也有正负波动。本次不作“所有场景提速”或“Release 普遍显著提速”的结论。

本次静态链最后一次结果保持 rank 12/42/87、hard 迭代 3/4/5、preference 迭代 2；residual 分别为 `4.50718e-25`、`3.96079e-23`、`3.12402e-22`，与上文一致。正常交互单体/connected50-200/independent50/group-contact 的 physical rank 为 0/294/0/1，组件数为 1/1/50/1，均 hard feasible、有效且可提交。grounded-failure、cancelled、budget、invalid-input 均有效地按预期拒绝提交；budget hard feasible 不等于可提交。

### connected50-200 阶段中位数

| 阶段 | 第一轮 After | 本次 Release |
| --- | --- | --- |
| compile_ms | 0.365 | 0.373 |
| hard_ms | 1.581 | 1.600 |
| preference_ms | 337.616 | 339.874 |
| retraction_ms | 0.647 | 0.639 |
| dof_ms | 347.451 | 335.286 |
| factorization_ms | 671.441 | 660.875 |
| pose_ms | 1.452 | 1.477 |

preference 和 DOF 仍占据主要时间；factorization 横跨多个阶段，retraction 包含于 preference。本次只记录现状，没有据此削减诊断或改变数值算法。

### 本次全样本

按采样顺序，单位 ms；静态链程序只输出平均，没有逐次总时间，本节不补造其样本。

| 场景 | 5 次总时间 |
| --- | --- |
| warm-rotation30 | 3.965770, 1.416590, 1.494790, 1.431190, 1.460990 |
| single | 0.137198, 0.080299, 0.075699, 0.074300, 0.073500 |
| connected50-200 | 526.326000, 616.672000, 677.770000, 887.039000, 941.384000 |
| independent50 | 0.584395, 0.561496, 0.606895, 0.553795, 0.556196 |
| group-contact | 0.265998, 0.238398, 0.231398, 0.225598, 0.251798 |
| grounded-failure | 0.036900, 0.030000, 0.035200, 0.026000, 0.023099 |
| cancelled | 0.416196, 0.388396, 0.389697, 0.430596, 0.384796 |
| budget | 684.701000, 681.237000, 671.036000, 878.961000, 930.219000 |
| invalid-input | 3.007870, 0.099199, 0.075999, 0.079999, 0.080399 |

### 本次验证与限制

- 新命令成功完成 12 个 Native 场景，每场景 5 次；benchmark 既有的求解状态及交互几何/资格断言通过，汇总还检查预期场景集合和采样数，失败或样本缺失直接报错。
- 命令与统计测试覆盖 samples/build 路由、平均与中位数及 ns→ms 换算、预期失败的提交资格；已有 Python 工具测试一并运行。文档上下文审计与差异检查通过。
- 没有运行本轮 Router/Session、分配 probes、全量 Assembly CTest/corpus、全量 Go/Web 或浏览器验收。上一轮相应统计仅为历史记录，本轮没有更新这些端到端结论。
- 这是同机器、相同 solver.cpp SHA 的历史复测，但构建/依赖配置与运行时环境不同，不是严格控制变量的 A/B 实验。只有 5 次采样；原程序的 kernel_p95_ms 是小样本分位数，本报告不把它解释为稳定 P95、交互流畅性或吞吐容量。
