# 可观测性与验证

> 返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。代码和测试定义当前事实；验证范围见[验证说明](validation.md)。

## 可观测性、构建与测试

Web 另有独立 `pnpm test:browser` 入口，Playwright 自动启动端口 5174 的 Mock Vite，并在 Chromium/SwiftShader 上回归上下文命令、树筛选/键盘、面板布局、多文档与草图/装配切换，以及插入浏览器的分页/嵌套文件夹/检索/失败重试和撤销重做快捷键。失败保留截图和 trace；该入口不替代真实后端 B-Rep 与复杂拾取验收。

- Go HTTP/gRPC 使用结构化日志和 OpenTelemetry Trace Context；
- 配置 OTLP 端点时导出 Trace，未配置时仍生成关联 ID；
- C++ Worker 记录 RPC、request ID 和 traceparent；
- 每个 API 请求建立有界、低基数的性能 Recorder；命令路径分解为 `command-prepare / command-apply / candidate-promote / sketch-solve / assembly-solve / geometry-evaluate / commit`，预览、DocumentView、Artifact 和拓扑查询也记录各自阶段。阶段同时进入结构化日志的 `phases_ms` 和响应 `Server-Timing`，浏览器保留最近 200 条 API 总耗时/Server-Timing 样本并随诊断包导出；不得把 DocumentId/FeatureId 作为阶段名或指标 label。
- `invoke performance-baseline` 对 Profile Builder 与 VisualizationManifest 热路径执行多样本、带 allocation 的邻近 Go benchmark，结果写到 `build/performance/go-workspace.txt`，可交给 `benchstat` 比较。正确性测试与性能基准分开，慢机器只影响绝对时间，不影响前后版本同机对比。

- C++ Geometry Worker 使用 Conan 固定的 spdlog 1.15.3，同时写彩色控制台和按 Worker 地址隔离的滚动文件；默认文件位于 `services/logs/`，单文件 10 MiB、保留 5 个，级别复用 `OCCCCAD_LOG_LEVEL`。

测试资产现在由被测模块拥有，而不是按语言堆在仓库根目录：C++ 场景位于对应 library 的 `tests/` 并由局部 CMake 注册；Web 场景位于 `src/**/testing/*.scenario.mjs`，统一 runner 自动发现后为每个场景启动独立进程；Go 遵循工具链，将 package 白盒测试保留为邻近 `_test.go`，只有跨 package、跨进程的公共契约测试进入 `tests/go`。`tests/test.data/` 统一保存 STEP/BREP、数值重放、草图输入、测试合同基线与验收记录；测试代码和执行入口仍邻近实现，根 `tests/` 不再作为语言分类目录。`invoke test` 保持构建并运行 CTest、`services/` Go package tests、独立 `tests/go` module 和 Web 场景的全量入口。

`invoke check` 是面向局部开发与 Agent 的稳定验证 API，显式支持 `assembly / geometry / sketch / workspace / services / web / all` scope；省略 scope 时合并 tracked 与 untracked Git 工作区路径并作保守映射。公共 Proto、数据库迁移、通用 Geometry Worker/`kernel/api`、共享 build/validation 入口和未知路径升级为 `all`，纯 Markdown 不触发可执行测试。单个非 all scope 可用 `--match` 进入 Level 1：C++ 组合领域 CTest 前缀，Go 使用 `-run`，Web 使用场景 substring，并跳过跨层集成/production build。`--plan` 只展示 scope、升级理由、cwd 和底层命令。每个底层命令默认捕获 stdout/stderr，成功只报告步骤、耗时和总计；失败在终端展示有界高信号内容，把完整 stdout/stderr 与命令写入 `build/agent-logs/`，并给出复现命令；`--verbose` 恢复流式执行。全量 `invoke test` 与 `check --scope all` 都运行全部 `tests/python` 单测，包含配置命令、环境加载、routing/output/context-audit。该层只改变开发命令输出，不削弱运行时 observability 或失败诊断。

`invoke context-audit` 检查根/local guide 尺寸、必需知识、全仓自有 Markdown 本地路径/锚点及 fenced block、旧 token prompt、`tasks.py` 阈值与大文本。它报告 root + docs/README + 一个 local guide 的字节范围，非 tokenizer 估算；`--verbose` 列出热点。`python tools/documentation_audit.py --baseline <commit>` 可重复测量删除/新增去向及前后字节规模，不关闭断链规则迁就整理。

Web scenario runner 支持一个或多个路径/文件名片段的 OR 筛选、`--list` 和 `--verbose`。默认每个子进程输出被缓冲，全部成功时只输出场景计数，失败时仅展开失败场景的 stdout/stderr。Web 当前使用 Vite SSR 加载真实 Tool/状态模块，覆盖完整 pointer 手势、操作批次、约束选择、尺寸输入和实时生命周期；浏览器布局、WebGL 拾取及真实后端组合 E2E 仍待补充。

Agent 上下文按根 repository router、五个高频 local `AGENTS.md` 和 `docs/README.md` 架构知识路由渐进加载。生成代码、corpus、锁文件、制品与超过阈值的大型源文件仍可按需访问，但不再是默认探索对象；当前与目标架构目录分别路由到事实分册和长期契约分册。

## 实现与验证入口

- [验证入口](../../../tasks.py)
- [验证路由测试](../../../tests/python/test_validation_routing.py)
- [跨模块验证路由](../../../tests/README.md)
- [文档审计及规模测量](../../../tools/documentation_audit.py)、[审计负例](../../../tests/python/test_documentation_audit.py)

## 最近定向基线

核对基线为 `e5f1304` 加剩余运动/文档工作区变更。实际执行包含 Web 运动投影/分页/Three.js 标记和共享交互场景、TypeScript 检查与生产构建、Go Manifest 回归、专用 `occccad_offset_contract_test` 与匹配 Worker 的快照/只读证据测试、合同设施和导航审计。三个已审查源码锁只因推断规则纠正及增强断言更新，新投影是共享 UI 证据，不冒充具体几何组合验收；数值预期和 requiredLayers 不变。可复跑命令见[装配验证](../../../tests/assembly-contract/README.md)。

维护者反馈当前使用场景下拖拽与约束较为稳定；剩余运动呈现尚待实机确认。本次未运行浏览器自动验收、工业容量、性能基准或无差别全仓测试，也未改算法或清理应用数据。历史执行不复制为当前全部通过。

## 操作失败诊断快照

API 初始化 `debug/cad-diagnostics` 的现有 `debugartifact.Store`，按文档最多 50 份、7 天保存操作失败；读取时也执行过期清理。`PreviewCommand` 和普通 Domain Command 应用失败记录原请求、可用的规范命令、失败阶段、基准 Revision/模型、已构造的未提交候选及基准 Body 的 BREP/Naming/GLB 引用、evaluator/Naming policy 与制品内的 kernel/Worker 版本。适配前失败可能没有候选；用户取消不归档。单份上限 8 MiB，归档失败记录日志并保留原业务错误，不改变提交语义。

错误通过 HTTP/WebSocket 携带 `diagnosticId`（`documentId/recordId`）。`GET /api/documents/{documentID}/diagnostics/{diagnosticID}` 经当前文档 Viewer 权限返回失败时的不可变 JSON；不拼入查询时的新 Head，缺失/过期返回 404。服务重开仍可读取本机日志目录的记录，不创建业务 Revision或提交候选，不收集凭据或无关日志。历史手动诊断包下载入口仍可用。

Web 操作诊断和实体/阵列/布尔预览提供“复制诊断”，复制 `CAD_DIAGNOSTIC <documentId>/<recordId> <code>`；剪贴板不可用或拒绝时显示可选择的文本，不虚报成功。没有服务端记录时复制已有阶段、操作请求 ID、错误及可用文档/Revision 上下文。普通命令与预览共享结构化错误，预览保留原错误对象并清除旧成功候选。此入口当前覆盖返回错误的预览及普通 Domain Command；已接受 FAILED Revision、历史补偿、后台 Job 和离线失败的自动归档尚未贯通，不应声称全部错误途径均已统一。

三维装配求解器通过 `invoke performance.assembly` 使用现有优化制品串行测量，默认 Native 静态链/BFGS 与交互/失败路径各 5 次；显式 stages 可扩展到真实 Router/Session 和分配 probes。每轮独立保存环境、二进制哈希、原始输出与 Native 汇总，不重新构建或覆盖历史结果。性能证据与语义回归、浏览器端到端验收分别记录，见 [性能记录](../../../tests/test.data/assembly-solver-performance.md)。
