# 可观测性与验证

> 2026-09-21 文档核对基线。返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。这里只记录实现事实；测试存在不等于本轮已经运行，验证缺口见[统一路线](../../../plans/README.md)。

## 可观测性、构建与测试

Web 另有独立 `pnpm test:browser` 入口，Playwright 自动启动端口 5174 的 Mock Vite，并在 Chromium/SwiftShader 上回归上下文命令、树筛选/键盘、面板布局、多文档与草图/装配切换，以及插入浏览器的分页/嵌套文件夹/检索/失败重试和撤销重做快捷键。失败保留截图和 trace；该入口不替代真实后端 B-Rep 与复杂拾取验收。

- Go HTTP/gRPC 使用结构化日志和 OpenTelemetry Trace Context；
- 配置 OTLP 端点时导出 Trace，未配置时仍生成关联 ID；
- C++ Worker 记录 RPC、request ID 和 traceparent；
- 每个 API 请求建立有界、低基数的性能 Recorder；命令路径分解为 `command-prepare / command-apply / candidate-promote / sketch-solve / assembly-solve / geometry-evaluate / commit`，预览、DocumentView、Artifact 和拓扑查询也记录各自阶段。阶段同时进入结构化日志的 `phases_ms` 和响应 `Server-Timing`，浏览器保留最近 200 条 API 总耗时/Server-Timing 样本并随诊断包导出；不得把 DocumentId/FeatureId 作为阶段名或指标 label。
- `invoke performance-baseline` 对 Profile Builder 与 VisualizationManifest 热路径执行多样本、带 allocation 的邻近 Go benchmark，结果写到 `build/performance/go-workspace.txt`，可交给 `benchstat` 比较。正确性测试与性能基准分开，慢机器只影响绝对时间，不影响前后版本同机对比。

- C++ Geometry Worker 使用 Conan 固定的 spdlog 1.15.3，同时写彩色控制台和按 Worker 地址隔离的滚动文件；默认文件位于 `services/logs/`，单文件 10 MiB、保留 5 个，级别复用 `OCCCCAD_LOG_LEVEL`。

测试资产现在由被测模块拥有，而不是按语言堆在仓库根目录：C++ 场景位于对应 library 的 `tests/` 并由局部 CMake 注册；Web 场景位于 `src/**/testing/*.scenario.mjs`，统一 runner 自动发现后为每个场景启动独立进程；Go 遵循工具链，将 package 白盒测试保留为邻近 `_test.go`，只有跨 package、跨进程的公共契约测试进入 `tests/go`。`models/` 只保存可被多个实现复用的 STEP/BREP 回归语料，根 `tests/` 不再作为语言分类目录。`invoke test` 保持构建并运行 CTest、`services/` Go package tests、独立 `tests/go` module 和 Web 场景的全量入口。

`invoke check` 是面向局部开发与 Agent 的稳定验证 API，显式支持 `assembly / geometry / sketch / workspace / services / web / all` scope；省略 scope 时合并 tracked 与 untracked Git 工作区路径并作保守映射。公共 Proto、数据库迁移、通用 Geometry Worker/`kernel/api`、共享 build/validation 入口和未知路径升级为 `all`，纯 Markdown 不触发可执行测试。单个非 all scope 可用 `--match` 进入 Level 1：C++ 组合领域 CTest 前缀，Go 使用 `-run`，Web 使用场景 substring，并跳过跨层集成/production build。`--plan` 只展示 scope、升级理由、cwd 和底层命令。每个底层命令默认捕获 stdout/stderr，成功只报告步骤、耗时和总计；失败在终端展示有界高信号内容，把完整 stdout/stderr 与命令写入 `build/agent-logs/`，并给出复现命令；`--verbose` 恢复流式执行。全量 `invoke test` 与 `check --scope all` 都运行 routing/output/context-audit Python 单测。该层只改变开发命令输出，不削弱运行时 observability 或失败诊断。

`invoke context-audit` 当前检查根/local guide 尺寸、必需 focused knowledge、Markdown 本地断链、旧 token prompt 残留、`tasks.py` 自身阈值，并统计 Git 已跟踪及未忽略的大文本；正常成功只报告 large/strong candidate 数，`--verbose` 才列出文件。

Web scenario runner 支持一个或多个路径/文件名片段的 OR 筛选、`--list` 和 `--verbose`。默认每个子进程输出被缓冲，全部成功时只输出场景计数，失败时仅展开失败场景的 stdout/stderr。Web 当前使用 Vite SSR 加载真实 Tool/状态模块，覆盖完整 pointer 手势、操作批次、约束选择、尺寸输入和实时生命周期；浏览器布局、WebGL 拾取及真实后端组合 E2E 仍待补充。

Agent 上下文按根 repository router、五个高频 local `AGENTS.md` 和 `docs/README.md` 架构知识路由渐进加载。生成代码、corpus、锁文件、制品与超过阈值的大型源文件仍可按需访问，但不再是默认探索对象；当前与目标架构目录分别路由到事实分册和长期契约分册。

## 实现与验证入口

- [验证入口](../../../tasks.py)
- [验证路由测试](../../../tests/python/test_validation_routing.py)
- [跨模块验证路由](../../../tests/README.md)
