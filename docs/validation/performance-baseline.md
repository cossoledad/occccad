# 本机性能基线

测量日期：2026-10-16 UTC；源码 HEAD：`2a8c5941194f477e0eee7d22480c35f5f95f295c`。本文汇总当前仓库已有性能入口；详细样本、阶段和环境见[机器可读数据](performance-baseline.json)。先读本文，再按需读取 JSON。

本机 Release 实测是本仓库唯一性能基准。后续性能比较以本页的同机工作负载、构建身份和样本为起点；具体源码与二进制摘要见机器可读数据。

## 环境与测量口径

- AMD Ryzen 9 9950X 16-Core Processor；32 逻辑 CPU；`Linux-6.18.54.1-microsoft-standard-WSL2-x86_64-with-glibc2.43`。
- `go version go1.26.5 linux/amd64`；`c++ (Ubuntu 15.2.0-16ubuntu1) 15.2.0`；OCCT 7.9.1。
- 通过 Invoke/Conan 重新配置并检查当前 Release 构建，实际为 `-O3 -DNDEBUG`、ASAN/UBSAN 关闭；本机已建制品与当前源码一致，本轮构建报告 `ninja: no work to do`。二进制与源码 SHA-256 保存在 JSON；Go benchmark 另记录本次运行时间与源码摘要。
- Native → Router/Session → 长 ID → Go benchmark → 连续建模 → 增量运行时 → 建模 fixture → 圆角 → Go RSS 串行执行，没有并行构建或测试。已有应用/开发工具继续运行，未绑核、锁频或消除后台负载。
- 时间均为 wall time。kernel、RPC、Session、命令提交与测试整例是不同边界；阶段可能包含，不能相加。Native 与 Router 的初值/目标/profile 不完全相同，不能相减推断传输开销。
- 正常服务测量复用已配置的专用 SQLite 测试库；建模 fixture 使用独立临时 SQLite/ArtifactStore。没有重置开发业务数据。

## 覆盖与结果

| 入口 | 本次范围 | 结果 |
|---|---|---|
| 装配 Native | 12 场景 × 20 次；另 8192 字符几何 ID 静态链 × 20 次 | 通过 |
| 真实 Router / Session | 9 个 Router 场景、普通/嵌套 Session 更新各 20 次；Begin 各一次 | 通过 |
| Go benchmark | 全仓 10 项，各 5 组、500ms/组、benchmem、包串行 | 通过；无 Skip |
| 连续建模 | 3 个独立 100 步序列，共 300 次正式修改与 3 次末态冷等价检查 | 通过 |
| 增量运行时 | 正常预算/1 字节淘汰各 5 次，含真实 Worker 重启，共 110 个预览计时 | 通过 |
| 既有建模 fixture | Bottom Support / Fan / Loft 投影各 3 次 | 通过 |
| 有限面域圆角 | 5 个 CTest 场景各串行重复 3 次 | 通过；预期拒绝仍拒绝 |
| Go 进程 RSS | 预先编译的 Workspace/Geometry benchmark，各 3 次 | 通过；不含编译器 |

## 装配 Native

单位 ms。静态链只提供 20 次平均与最后样本阶段值；其余为 20 次中位数。预期失败场景通过表示正确拒绝提交，并非成功求解。

| 场景 | 统计 | 总 ms | 提交资格 |
|---|---|---|---|
| plane5 | 平均 | 0.175 | 通过既有断言 |
| plane15 | 平均 | 3.861 | 通过既有断言 |
| plane30 | 平均 | 76.485 | 通过既有断言 |
| warm-rotation30 | 中位数 | 0.580 | 通过既有断言 |
| single | 中位数 | 0.039 | 通过既有断言 |
| connected50-200 | 中位数 | 309.764 | 通过既有断言 |
| independent50 | 中位数 | 0.185 | 通过既有断言 |
| group-contact | 中位数 | 0.093 | 通过既有断言 |
| grounded-failure | 中位数 | 0.005 | 不可提交（预期） |
| cancelled | 中位数 | 0.102 | 不可提交（预期） |
| budget | 中位数 | 306.616 | 不可提交（预期） |
| invalid-input | 中位数 | 0.017 | 不可提交（预期） |

connected50-200 的阶段中位数：preference 146.794 ms、DOF 160.658 ms、factorization 303.515 ms。factorization 横跨阶段，retraction 属于 preference，不能相加归因。

## Router 与 Session

下表每行 20 次，列为各自中位数；首请求包含在内，第一条的 `cold` 标记只表示该 fixture 首请求，不是进程/系统冷启动。Session Begin 各一次，单独记录，不视为分布。响应 JSON 与请求 marshal 是独立校准，不加入总时间；`—` 表示该入口没有该项测量。

| 类型/场景 | 总 ms | RPC ms | kernel ms | base check ms |
|---|---|---|---|---|
| plane5 | 2.579 | 2.545 | 1.405 | — |
| plane15 | 7.679 | 7.614 | 5.963 | — |
| plane30 | 73.910 | 73.840 | 71.392 | — |
| single | 1.781 | 1.772 | 0.954 | — |
| connected50-200 | 319.602 | 319.458 | 315.936 | — |
| independent50 | 10.072 | 10.003 | 8.728 | — |
| group-contact | 2.233 | 2.213 | 1.281 | — |
| grounded-failure | 1.119 | 1.107 | 0.264 | — |
| budget | 176.311 | 176.149 | 173.175 | — |
| Session nested=False | 2.117 | 1.723 | 0.907 | 0.110 |
| Session nested=True | 5.114 | 2.087 | 1.004 | 2.633 |

普通/嵌套 Session Begin 分别为 1.787 / 3.901 ms（各 n=1）。预览 Head 不变、无关体保持和嵌套 root 变化后拒绝继续的断言均通过。

## Go 热路径与分配

各项 5 组指标的中位数。`500ms` 是 Go 校准目标，不是固定迭代次数；表中分配值也取各组中位数。

| benchmark | ms/op | B/op | allocs/op |
|---|---|---|---|
| AssemblyFreeze | 0.725904 | 570589 | 9166 |
| AssemblyDigest | 0.589429 | 454057 | 8616 |
| AssemblyReferenceKey | 0.001292 | 1856 | 2 |
| ProfileBuilder200Circles | 3.549305 | 710923 | 2838 |
| VisualizationManifest200Circles | 0.947632 | 2118629 | 3247 |
| AssemblyRPC | 0.051987 | 64105 | 442 |
| AssemblyReplay | 0.172586 | 141260 | 2317 |
| CatalogFamily | 0.000584 | 1488 | 13 |
| CatalogParse | 0.306965 | 57632 | 805 |
| ThumbnailRing | 47.028947 | 19229378 | 200 |

装配 RPC 压力输入仍为 4,246 B，数值 replay JSON 为 11,151 B；不是所有日常模型的固定大小。预编译 benchmark 进程 RSS 中位数：Workspace 29460 KiB、Geometry 26648 KiB；各 n=3，不是常驻 API/Worker 的峰值或内存上限。

## 建模与增量求值

连续建模的 100 步命令累计时间依次为 4.965 / 5.106 / 4.894 秒；每轮固定同一工作负载并逐步推进 Revision，全部末态与强制冷重建的 BREP/NAMING 字节等价。完整说明见[连续建模](continuous-modeling-100-baseline.md)，热/冷表见[增量运行时](incremental-modeling-runtime-validation.md)。

| 整例 fixture | n | 中位数 s | 全部 s |
|---|---|---|---|
| BottomSupportParametricDatumLifecycleThroughRouter | 3 | 1.390 | 1.500, 1.390, 1.390 |
| FeatureAssociationFanLifecycleThroughRouter | 3 | 2.010 | 2.010, 2.010, 1.790 |
| LoftLinearBoundaryProjectionThroughRouter | 3 | 1.250 | 1.250, 1.350, 1.240 |

整例时间包括该测试的准备、迁移、Worker 生命周期和断言，不是单个几何算子时间。冷/热预览比较同轮同定义，各阶段按实际边界分别记录。

## 圆角与长 ID

| Native 静态链 | 短 ID 平均 ms | 长 ID 平均 ms | 长 ID 末样本 RSS KiB |
|---|---|---|---|
| plane5 | 0.175 | 0.257 | 29020 |
| plane15 | 3.861 | 4.375 | 29020 |
| plane30 | 76.485 | 76.037 | 29020 |

长 ID 末样本的热循环字符串查询/处理字节均为 0；RSS 为同一进程截至该案例的累计高水位。短/长 ID 是顺序运行的两个进程，小差异不能视为稳定因果效应。

| 圆角 CTest | n | 整例中位数 s |
|---|---|---|
| TargetSupportEvidence | 3 | 0.030 |
| OriginalFiniteDomainReproductions | 3 | 0.470 |
| PanelFeatureHistoryAndParameterRoundTrip | 3 | 1.000 |
| CylindricalSupportBeyondAxialLimit | 3 | 0.020 |
| RejectsNonTangentConsumedCornerPatches | 3 | 0.150 |

圆角内部阶段的全部计时另存 JSON，它们覆盖多个几何输入和嵌套阶段，不能混成一个代表整件模型的平均值。明确拒绝非相切角区的回归通过，不表示目标 Solar/Main Fan 的完整几何等效已完成。

## 复现与证据限制

先完成 Release 构建，再串行执行下列入口；不要在测量同时构建或运行其他测试。Go TestMain 自动读取根 `.env`，仍须确认实际执行而非 Skip。

```bash
invoke configure --build-type=Release
invoke build --build-type=Release
invoke performance.assembly --samples=20 --stages=static,interaction,service
build/cmake/release/kernel/assembly/tests/occcad_assembly_solver_benchmark 20 --long-ids
go -C services test ./internal/workspace ./internal/geometry ./internal/assemblycontract ./internal/thumbnail -run '^$' -bench . -benchmem -benchtime=500ms -count=5 -p=1
go -C services test ./internal/control -run '^TestContinuousComplexModeling100BaselineThroughRouter$' -count=3 -v -timeout=20m
go -C services test ./internal/control -run '^TestIncrementalRuntime(Production|Eviction)ThroughRouter$' -count=5 -v -timeout=20m
go -C services test ./internal/control -run '^Test(BottomSupportParametricDatumLifecycle|FeatureAssociationFanLifecycle|LoftLinearBoundaryProjection)ThroughRouter$' -count=3 -v -timeout=20m
ctest --test-dir build/cmake/release -R '^geometry/FilletBoundary\.' --repeat until-fail:3 -V
```

既有默认输出 `build/performance/go-workspace.txt` 与 `build/performance/assembly-solver.txt` 已用本轮对应真实样本刷新，完整日志、3 轮建模 JSON、Go 进程 RSS 和构建记录在 `build/performance/refresh-20261016T145917Z/`，装配原始样本在 `build/performance/assembly/refresh-20261016T145917Z/`；日志摘要与 SHA-256 在机器可读数据中。完整第三轮 100 步数据更新于 [JSON 基线](../../tests/test.data/continuous-modeling-100-baseline.json)。

本轮没有浏览器/WebGL 输入至权威显示、HTTP/WebSocket 传输、并发吞吐、长会话、工业大模型或跨主机基准。没有稳定尾延迟/60 Hz/容量承诺；100 步序列的样本分位数只描述该序列。执行测试的整例耗时不是通用性能基准。

Go RSS 复现时先编译完成，再计量 benchmark 进程，避免把 Go 编译器算进峰值：

```bash
go -C services test -c -o "$PWD/build/performance/workspace-bench.test" ./internal/workspace
go -C services test -c -o "$PWD/build/performance/geometry-bench.test" ./internal/geometry
for owner in workspace geometry; do
  for sample in 1 2 3; do
    /usr/bin/time -v "build/performance/$owner-bench.test" -test.run='^$' -test.bench=BenchmarkAssembly -test.benchmem -test.benchtime=500ms -test.count=1
  done
done
```
