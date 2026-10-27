# Geometry Worker

当前唯一的 C++ 网络计算服务，通过粗粒度 gRPC 承载 OCCT、PlaneGCS 与独立 Assembly 算法库。Worker 消费冻结纯值和制品，不拥有 Document、Revision、ACL 或业务数据库。先读 [AGENTS](AGENTS.md)，再按 RPC 名搜 `src/main.cpp`。

## 接口与模块入口

协议权威是 [geometry_worker.proto](../../proto/occccad/worker/v1/geometry_worker.proto)，包名 `occcad.worker.v1`。下表指当前 server 实现，不把 Proto 声明当作能力证明。

| RPC | 职责与详细合同 |
|---|---|
| Ping | Worker 身份、OCCT 版本、驻留数量 |
| SolveSketch / ComputeSketchCurves | 约束求解 / 精确曲线计算；[Sketch adapter](sketch/README.md) |
| ProjectExternalGeometry | 已解析支持到草图平面的权威投影；[Part/Sketch](../../docs/architecture/current/part-sketch.md) |
| EvaluatePart / ResolveLoftCorrespondence / AnalyzeBooleanRegions | 每 Body 的冻结链/阶段续算、截面对应及独立精确布尔分区；[实体 Feature](../../docs/architecture/current/solid-features.md) |
| SolveAssembly | body-local 几何与纯值方程；[算法](../../kernel/assembly/SOLVER_ALGORITHMS.md)，Product/Session 仍由 Go 管理 |
| AnalyzeInterference | 不可变 BREP/Pose 的底层分析；[exact 输入与限制](../../docs/architecture/current/geometry-representations.md#独立几何分析的-exact-输入)，不代表已交付干涉应用 |
| InspectExchange / ImportExchange / ExportExchange | ArtifactReference 上的 STEP/BREP/XDE；[交换](../../docs/architecture/current/jobs-artifacts.md) |
| GetTopology | 冷恢复/缓存精确面边点及提示；[几何制品](../../docs/architecture/current/geometry-representations.md) |

Proto 中的 LoadGeometry、UnloadGeometry、Tessellate、CreateChamfer、CreateFillet 当前没有 server override，调用返回 UNIMPLEMENTED。局部圆角/倒角由 EvaluatePart 承载。

`kernel/api` 定义 OCCT-free 值类型，`kernel/occt` 实现精确几何，`kernel/assembly` 只求纯数值刚体约束，`sketch/` 隔离 PlaneGCS 类型。源码、RPC 代理、结果采用与 CAS 的完整检查归[Worker/Router 合同](../../docs/architecture/current/worker-contracts.md)。

## 配置、制品与缓存

- `OCCCCAD_GEOMETRY_WORKER_LISTEN`：默认 `127.0.0.1:51001`。托管启动使用控制面分配的 `OCCCCAD_GEOMETRY_WORKER_ID`，Ping 就绪检查核对本次身份；禁用 SO_REUSEPORT，不接管已有进程。
- unary gRPC 发送/接收均为 128 MiB，与 Go `internal/geometryrpc` 一致；文件大小上限独立。大 BREP/GLB/STEP 经 ArtifactReference，不进入 unary bytes。
- 持久存储支持 LOCAL/S3；Go 将远端输入验证并 materialize 到 LOCAL scratch，Worker 输出由 Adopt 上传。API/Jobs/Worker 仍需共享 `OCCCCAD_DATA_DIR`，object key 是根目录内 opaque 相对键。
- 请求携带 request_id、geometry_key，Trace Context 用 gRPC metadata。每个 Body 有独立 GeometryId 与 BREP/Visual/Naming；相同 Part occurrence 复用几何，仅施加 InstancePath/Transform。
- 首次 GetTopology 可从不可变 BREP 冷恢复，Router 先保留 owner；后续精确拓扑查询过滤同一缓存。完整显示载荷唯一来源为 GLB，格式归 [Visual/Naming](../../docs/architecture/current/visual-naming-artifacts.md)，schema 与 Naming policy 版本独立。
- EvaluatePart 的 `exact_only` 省略 GLB，`force_cold` 绕过热缓存并冷重建，`runtime_affinity` 只是路由提示；`runtime_stats` 记录实际执行/复用。`OCCCCAD_STAGE_CACHE_BYTES` 默认 134217728，0 关闭，最大 2147483648，超预算项不缓存。完整键与恢复语义归[增量运行时](../../docs/architecture/current/jobs-artifacts.md#part-增量运行时)。

## 日志

Worker 使用 spdlog 1.15.3 同时输出控制台与滚动文件。托管启动时 stdout/stderr 直接透传到终端，不再被 Go control 包装成 `service output message="..."`；Worker 生命周期日志仍由 control 记录。`OCCCCAD_LOG_LEVEL` 控制两类 sink 的级别；`OCCCCAD_LOG_DIR` 控制文件目录，`occccad-control` 默认把相对路径解析为 `services/logs/`。每个监听地址使用独立 `occccad-geometry-<address>.log`，单文件达到 10 MiB 后轮转并保留 5 个，避免多个 Worker 争写同一文件。每个 Worker 的共享内核仍串行调度，OCCT 内部至多 4 路并行。求值及可视化缓存各采用 64 MiB 字节预算与 LRU；阶段、Shape、Naming 与输入制品预算见上文增量 Part 求值；命中复用完整字节/映射，超限结果不缓存。`visual_snapshot` 记录剖分、面/边提取、法线、编码时间和数据量；曲面抽壳的 wall_offsets/wall_partition/wall_assembly 阶段计时输出到 stderr。日志包含启动、RPC、拓扑缓存命中、耗时和错误上下文，但不记录模型内容、凭据或制品 URL。

## 构建与运行

```bash
mise run cpp:deps --profile release
mise run cpp:build --profile release --target=occccad_geometry_worker
mise exec -- build/cmake/linux-gcc15-x86_64/release/workers/geometry/occccad_geometry_worker --profile release
```

Geometry 测试邻近位于 `kernel/occt/tests`，PlaneGCS 测试邻近位于 `workers/geometry/sketch/tests`。可运行全部测试：

```bash
mise run check --scope all --level full --profile release
```

局部开发优先使用 `mise run check --scope geometry` 或 `mise run check --scope sketch`；公共 Proto、通用 Worker/RPC 和 Router 边界使用 `mise run check --scope all`。这些命令成功时保持摘要输出，失败时展开底层诊断。

只验证几何交换回归（包括 `tests/test.data/` 中的真实 STEP 语料和共享/嵌套 XDE Product round-trip）：

```bash
mise exec -- build/cmake/linux-gcc15-x86_64/release/workers/geometry/occccad_geometry_worker --profile release
```

Worker `main` 仅负责启动 gRPC 服务，不包含 `--smoke` 或测试专用分支。

依赖版本以根 `conanfile.py` 为准，当前为 C++17、OCCT 7.9.1、gRPC 1.71.0、spdlog 1.15.3、Eigen 3.4.0 和 header-only Boost 1.86.0。PlaneGCS 锁定 FreeCAD 1.0.2 的不可变 commit，CMake 仅下载带逐文件 SHA-256 的官方核心源码清单，并构建为独立 shared library；不链接完整 FreeCAD。构建产物旁的 `LICENSE.FreeCAD-PlaneGCS` 必须随该库分发。

## 资源与故障语义

- resident geometry 只存在于进程内；Worker 退出后必须能从持久 B-Rep 或参数模型重建；
- GeometryId 基于内容，不得包含 Worker 地址；
- 同一个 Worker 内的 OCCT 操作当前由互斥边界保护，扩展吞吐优先增加进程而不是假设所有 OCCT 路径线程安全；
- 输入 STEP/BREP 是不可信复杂文件；当前 Worker 拒绝空对象、越界 object key 和超过 `OCCCCAD_EXCHANGE_MAX_BYTES`（默认 16 GiB）的制品，生产环境仍需要更严格的时间、内存、实体数量和进程级隔离；
- 计算请求必须可重试，调用方不能把 Worker 局部状态当成唯一副本。

## 设计与验证边界

二维草图与 Part 求值同进程，三维装配由本 Worker 调用独立算法库；负载或隔离证据才触发独立 Worker，见[计算边界约定](../../docs/architecture/target/system.md#计算制品与安全)。

数值 profile、build policy、Naming evaluator 的准确版本看 Proto、源码常量与锁定依赖，不在 README 重复历史版本。`SolveAssemblyResponse.effective_solver_profile` 冻结实际数值默认值供 [3dreplay](../../services/cmd/occccad-3dreplay/README.md)重放；classification tolerance 不得严于 convergence tolerance，nominal 与 warm start 各有职责。

AnalyzeInterference 在隔离内核/深拷贝上分析，不修改源形体；最多 128 实例、8192 pair、256 MiB 不同 BREP，保留取消及部分失败诊断。它是精确分析 RPC，不代表已交付完整干涉应用或连续碰撞。

正式 Router/Worker 集成必须实际执行，直连、Proto 声明、Mock 或测试入口存在均不能代替通过证据。
