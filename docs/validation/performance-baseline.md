# 本机性能基线

测量日期：2026-10-16 UTC；源码 HEAD：`a8dd5345a35c5f547466ef3ba45455d9cd61b6d6`，含工作区改动；准确源码摘要、二进制、命令、资源身份和完整样本见[机器可读数据](performance-baseline.json)。这些样本仅描述指定源码与环境。

## 环境与计时范围

- AMD Ryzen 9 9950X 16-Core Processor；32 逻辑 CPU；`Linux-6.18.54.1-microsoft-standard-WSL2-x86_64-with-glibc2.43`。
- `go version go1.26.5 linux/amd64`；`c++ (Ubuntu 15.2.0-16ubuntu1) 15.2.0`；OCCT 7.9.1。Release/优化参数与 sanitizer 状态见 JSON 中 assembly 构建记录。
- 人工应用、Agent 真实集成和Router/建模样本均使用同一 `.env` 的数据库、LOCAL/S3 后端、暂存目录和 Worker。测量期间没有重置数据库，使用已有可丢弃开发数据；终态规模见 JSON 的 databaseStateAtFinish。
- 各入口串行测量，测量时应用未启动；没有并行构建或测试。未绑核、锁频或控制其他后台负载。
- kernel、RPC、Session、命令提交、序列和整例测试耗时边界不同。阶段有包含关系，不能相加；Native/Router 工作负载不完全相同，不能相减推断网络成本。

## 覆盖与结果

| 入口 | 范围 | 结果 |
|---|---|---|
| Native | 12 场景各 20 次；另长 ID 静态链 | 通过既有数值断言，预期失败仍拒绝 |
| Router / Session | 9 个 Router、普通/嵌套更新各 20 次；Begin 各一次 | 通过，无 Skip |
| Go benchmark | 10 项各 5 组，500ms/组、benchmem、包串行 | 通过 |
| 连续建模 | 3×100 步正式修改，3 次末态冷等价检查 | 通过 |
| 增量运行时 | 正常缓存/1 字节预算各 5 次，共 110 个预览样本 | 通过 |
| 建模 fixture | Bottom Support / Fan / Loft 投影各 3 次 | 通过 |
| 有限面域圆角 | 5 个 CTest 场景各 3 次 | 通过，保留预期拒绝 |
| Go 进程 RSS | 预编译 Workspace/Geometry benchmark 各 3 次 | 不含编译器 |

## Native 装配

单位 ms。静态链报告均值及末样本阶段；其他场景报告 20 次中位数。

| 场景 | 统计 | 总 ms | 提交资格 |
|---|---|---|---|
| plane5 | mean | 0.186 | 通过既有断言 |
| plane15 | mean | 3.895 | 通过既有断言 |
| plane30 | mean | 75.416 | 通过既有断言 |
| warm-rotation30 | median | 0.554 | 通过既有断言 |
| single | median | 0.039 | 通过既有断言 |
| connected50-200 | median | 306.994 | 通过既有断言 |
| independent50 | median | 0.177 | 通过既有断言 |
| group-contact | median | 0.092 | 通过既有断言 |
| grounded-failure | median | 0.005 | 预期拒绝 |
| cancelled | median | 0.100 | 预期拒绝 |
| budget | median | 306.239 | 预期拒绝 |
| invalid-input | median | 0.023 | 预期拒绝 |

阶段与 factorization 有包含关系，完整值在 JSON 中；不得相加归因。

## Router 与 Session

每行 20 次的中位数，首请求包含在内。`cold` 只标记该 fixture 首请求，不代表系统冷启动。marshal/response JSON 是独立校准。

| 场景 | 总 ms | RPC ms | kernel ms | base check ms |
|---|---|---|---|---|
| plane5 | 2.436 | 2.415 | 1.339 | — |
| plane15 | 8.198 | 8.151 | 6.323 | — |
| plane30 | 73.209 | 73.085 | 71.087 | — |
| single | 1.749 | 1.740 | 0.879 | — |
| connected50-200 | 311.642 | 311.514 | 308.552 | — |
| independent50 | 9.847 | 9.802 | 8.491 | — |
| group-contact | 2.107 | 2.090 | 1.203 | — |
| grounded-failure | 1.013 | 1.002 | 0.246 | — |
| budget | 175.034 | 174.545 | 171.911 | — |
| Session nested=False | 2.149 | 1.737 | 0.881 | 0.115 |
| Session nested=True | 4.251 | 1.817 | 0.896 | 2.114 |

Begin（各 n=1）：nested=False 1.058 ms；nested=True 2.184 ms。

## Go 热路径与分配

每项 5 组的中位数；500ms 是 Go 校准目标，迭代次数不同。

| benchmark | ms/op | B/op | allocs/op |
|---|---|---|---|
| AssemblyFreeze | 0.748359 | 571165 | 9166 |
| AssemblyDigest | 0.600732 | 454396 | 8616 |
| AssemblyReferenceKey | 0.001348 | 1856 | 2 |
| ProfileBuilder200Circles | 3.672334 | 710924 | 2838 |
| VisualizationManifest200Circles | 0.916535 | 2118629 | 3247 |
| AssemblyRPC | 0.051753 | 64105 | 442 |
| AssemblyReplay | 0.180242 | 141224 | 2317 |
| CatalogFamily | 0.000646 | 1488 | 13 |
| CatalogParse | 0.305907 | 57632 | 805 |
| ThumbnailRing | 46.017931 | 19229377 | 200 |

预编译 benchmark 进程 RSS 中位数：workspace 30264 KiB；geometry 27140 KiB，各 n=3；不是常驻 API/Worker 峰值或内存上限。

## 建模与几何

100 步命令累计时间（3 个独立序列）：11.286 s / 11.171 s / 12.027 s。全部末态 BREP/NAMING 与强制冷重建逐字节等价。详情见[连续建模](continuous-modeling-100-baseline.md)与[增量运行时](incremental-modeling-runtime-validation.md)。

| 整例 fixture | n | 中位数 s |
|---|---|---|
| BottomSupportParametricDatumLifecycleThroughRouter | 3 | 2.130 |
| FeatureAssociationFanLifecycleThroughRouter | 3 | 2.330 |
| LoftLinearBoundaryProjectionThroughRouter | 3 | 1.560 |

整例包含准备、数据库、Worker 生命周期和断言，不是单个算子时间。

| 长 ID 静态链 | 均值 ms | 末样本 RSS KiB |
|---|---|---|
| plane5 | 0.194 | 33680 |
| plane15 | 3.954 | 33680 |
| plane30 | 75.355 | 33680 |

长/短 ID 来自顺序运行的不同进程；不把小差异视为稳定因果效应。热循环字符串计数与累计高水位见 JSON。

| 圆角 CTest | n | 整例中位数 s |
|---|---|---|
| TargetSupportEvidence | 3 | 0.030 |
| OriginalFiniteDomainReproductions | 3 | 0.460 |
| PanelFeatureHistoryAndParameterRoundTrip | 3 | 0.990 |
| CylindricalSupportBeyondAxialLimit | 3 | 0.020 |
| RejectsNonTangentConsumedCornerPatches | 3 | 0.150 |

圆角内部计时包含多个输入及嵌套阶段，不能混成一个整件平均值。

## 复现与限制

先完成当前 Release 构建，再串行运行：

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

RSS 先 `go test -c` 编译，再对生成的 `.test` 执行 `/usr/bin/time -v ... -test.run=^$ -test.bench=BenchmarkAssembly -test.benchmem -test.benchtime=500ms -test.count=1`；各包重复 3 次。准确命令与原始日志路径在 JSON，不把编译器算进 RSS。

本次没有浏览器输入至权威显示、并发吞吐、长会话或工业大模型测量，不承诺稳定 P95/P99、60 Hz 或容量。性能入口通过也不代表全仓回归通过：未修改的 Shell 偏移场景在全量 CTest/Worker 检查中失败，另见[工作流验收](agent-workflow-baseline.md)。
