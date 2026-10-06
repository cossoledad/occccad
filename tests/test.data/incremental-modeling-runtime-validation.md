# 增量建模运行时定向验证

2026-10-06；Debug OCCT 7.9.1，真实本机 Router/Worker，临时 SQLite 与临时 ArtifactStore。未连接、清理或修改用户文档；未进行浏览器自动测试或全量单测。实现职责、预算及冷恢复合同见[当前架构](../../docs/architecture/current/jobs-artifacts.md#part-增量运行时)。

## 实现边界

- Go：既有 DependencyGraph 增补 Profile/投影/放样对应节点，基准与草图按图解析；跨请求复用冻结准备，请求内复用 Body 前缀，EvaluationManifest.runtime 记录实际执行。
- OCCT：Shape 仓库保存精确阶段、Naming、工具及范围起点；追加/编辑从最长有效前缀续算。继续使用原有几何算子、持久引用和粗粒度 Body RPC。
- Worker/Router：内部 exact-only 阶段省略 GLB；按需复用 Naming/显示缓冲；字节预算、LRU、Body 亲和及安全取消。Worker 消失时由定义和冻结制品恢复。
- Web/realtime：稳定编辑会话，一个当前计算加一个可替换的最新输入；候选提升、CAS、幂等及未知回执恢复保持原合同。

## 性能证据

修改前记录同一组三条生产测试：Bottom Support 2.00s、风扇 Feature 生命周期 10.96s、放样直边投影 2.41s，合计约 15.38s。一次定向性能运行中同三条分别 2.23s、6.90s、2.40s，合计约 11.53s（约下降 25%）；风扇链约下降 37%。最终九条生产回归期间同三条为 2.50s、8.45s、2.41s（合计 13.36s）；存在系统负载和并行构建检查带来的波动。这是本机单次 fixture 观测，包含进程启动、命令和制品开销，不是所有模型的性能承诺。

下表来自上述定向性能运行的 `TestIncrementalRuntimeProductionThroughRouter`。冷请求绕过共享准备、阶段/响应/显示/Naming/输入缓存、已登记结果，并清除 Worker 驻留 Shape/Topology；冷请求自身的相同准备只构建一次。不是重建 Go Service 模拟冷求值。

| 场景 | 实际执行/复用阶段（热→冷） | 服务耗时 ms（热→冷） | exact ms（热→冷） | Naming ms（热→冷） |
|---|---:|---:|---:|---:|
| 追加第三个拉伸 | 1/2 → 3/0 | 319.82 → 518.91 | 104.91 → 213.75 | 109.04 → 174.60 |
| 编辑中间拉伸 | 2/1 → 3/0 | 458.05 → 521.82 | 191.21 → 215.46 | 159.26 → 176.20 |
| 连续放样第二次预览 | 1/3 → 4/0 | 320.64 → 753.63 | 124.49 → 337.41 | 91.70 → 254.22 |
| Worker 真实重启后的放样 | 4/0 → 4/0 | 733.31 → 750.18 | 337.74 → 340.85 | 251.50 → 246.75 |

- 追加/中间编辑的热 Profile 构建均为 0；对应冷请求为 3。放样第一次只新增一次对应准备，第二次固定截面 Profile 与对应准备均为 0；冷请求为 5 次 Profile、1 次对应准备。
- 放样第二次预览 mesh/encoding 为 0；对应冷请求分别 12.84/0.61ms。其排队约 0.11/0.12ms、制品 I/O 6.88/6.89ms。队列没有高负载压力基准，不能据此宣称并发容量提升。
- 名称、Body 可见性预览约 34.65/35.62ms，几何算子、离散与编码均为 0。无 Worker 调用时日志 RSS=0 表示没有测量，不能解释为没有内存使用。
- 本次放样热/冷 Worker 生命周期 RSS 高水位约 76.22/76.35MiB；阶段缓存约 10.1MiB。RSS 是进程高水位，预算是保留数据的估算计费，两者不是同一指标。修改前 `/usr/bin/time` 的 409.75MiB 包含 Go 构建/测试进程，不能作为 Worker 内存优化对照，也不据此宣称内存下降。

增量/冷重建核对真实精确 GeometryID、BREP 存在、体积、包围盒尺寸及完整 Naming 字节。不可变键的 BREP/NAMING 摘要冲突直接失败，数据库不能静默返回旧制品掩盖差异。内核另核对历史 evidence digest、语义输出与候选不修改已接受形体；现有风扇、投影、Boolean 和 Product 测试核对实际引用/成员语义。

## 定向验证

- C++：6 条几何场景通过，涵盖增量追加/中间编辑、阶段淘汰、阶段边界取消与失败恢复、冷 Naming、Boolean 历史、跨 Body 冻结来源、切除阵列与镜像。
- Go：Dependency/Evaluation/准备缓存/预览相关定向测试通过；准备结果所有权、冷请求隔离、同输入合并、失败恢复和最新待处理 runner 的 race 检查通过。
- 真实 Router/Worker：最终九条主测试通过（91.284s）；热/冷阶段验证、1 字节预算淘汰、Worker 进程重启、名称/显示、参数→基准→投影→草图→实体、风扇阵列后加工、放样直边和放样切除阵列/镜像通过。独立 SQLite 中的跨 Body Boolean、共享参数阵列、Product occurrence 与补偿历史通过。
- TypeScript 类型检查、生产构建及四条受影响 Node 场景通过：最新输入队列、预览显示、命令/候选身份、realtime 控制。

复现入口（从仓库根运行 CTest，Go 从 services/ 运行；跨 Body/Product 另指定一次性 SQLite 的 `OCCCCAD_TEST_DATABASE_URL=sqlite:/绝对路径/runtime.db`）：

```sh
ctest --test-dir build/cmake/debug -R 'GeometryExchange\.(IncrementalStages|PatternCut|CrossBodyBoolean|LoftCutCircular|MirrorLoftAdditive|TopologyHistoryComposesBoolean)' --output-on-failure
OCCCCAD_TEST_GEOMETRY_WORKER=/home/ganjb/project/occccad/build/cmake/debug/workers/geometry/occccad_geometry_worker go test ./internal/control -run 'TestIncrementalRuntime(Production|Eviction)ThroughRouter' -count=1 -v
go test -race ./internal/workspace ./internal/api -run 'TestPreparedRuntime|TestPreviewRunner' -count=1
```

## 限制与人工验收

原生共享状态仍串行，取消不能打断正在运行的 OCCT 算子；阶段检查点易失，重启或缺失后冷恢复；预算不等于 OCCT 分配器/RSS 硬上限，单个超预算活动形体仍能计算。参数与便宜框架仍进行校验；请求仍需准备轻量输入/Naming 解析，不承诺所有 CPU/I/O 均消失。候选产生的共享内容寻址制品可能留待既有生命周期清理；没有新增持久 GC、跨机调度或任意特征组重执行能力。

人工重建并重启相关服务（不 reset-data），打开已有验证模型：追加末尾特征、编辑中间尺寸、连续调放样参数并取消/确认；观察最终预览、历史编辑上下游、阵列成员与面/边引用。再检查共享参数更新、跨 Body Boolean、Product 多 occurrence 与 Undo/Redo；名称/可见性改变不应产生几何执行。观察 Worker `part_runtime` 和 Revision runtime 证据。浏览器/WebGL 实机操作、长会话大模型内存及高负载排队尚未人工验收。
