# Agent 上下文与验证工作流

目标是减少一次正确完成任务的无关读取、环境排查和返工。平台不变量归[根指南](../../AGENTS.md)，领域语义归[项目地图](../README.md)所路由的 Current/Target，工程工具不另建领域真相。

## 最小上下文

```text
根 AGENTS → 最近局部 AGENTS → 符号/调用方/邻近测试 → 命中架构或证据
首次了解项目：根 AGENTS → docs/README.md
```

按 Search → Read → Expand → Architecture 探索。所属模块、当前行为、实现/测试、最小变更和验证入口明确后停止。40 KB 是先查符号的信号，80/120 KB 是职责审查候选，不自动拆文件。

开始任务先明确目标、验收、不变项和分析/实现阶段。高频任务可直接取得带源码位置和验证义务的上下文：

```bash
invoke task --recipe property
invoke task --recipe selection --symbol AssemblyDialogLifecycle --read --json
```

| recipe | 完成边界 |
|---|---|
| property | 同一 PropertySlot 的写、读、补偿、digest、依赖及两步 Undo/Redo |
| rpc | Proto、生成接口、server/client/Router、capability 与真实请求 |
| selection | Document/Revision/InstancePath、手势取消、迟到响应与浏览器 |
| feature | 精确形状/Naming、Preview、CAS commit、历史和冷读 |
| diagnostic | 失败时不可变输入、阶段、版本、实际可重放范围 |

[任务配方](../../tools/task_recipes.py)只拥有工程路由与义务，生产目录、协议和模型仍由所属模块拥有。配方路径/符号由 `context-audit` 校验；改变实现归属时同步路由。默认只给位置，`--read` 最多返回两段各 5 KB 的源码，不加载全仓索引。

## 同一开发环境

人工应用与 Agent 使用同一 `.env`，规则见[环境准备](environment.md)。不要为通过测试另设连接、测试桶或 Worker 路径。先检查现有资源：

```bash
invoke doctor
invoke doctor --scope web --browser
invoke doctor --live
```

`doctor` 检查工具、当前构建类型、Worker/fixture 生产者、Ninja 待构建状态及配置资源。它不迁移或删除业务数据，不下载依赖。`--live` 另核对 `invoke run` 的源码/制品/资源身份与实际进程；源码或配置变化后重启应用。

## 渐进验证 API

```bash
invoke check --scope workspace --match '^TestHistoryCapabilitiesAcrossTwoUndoAndRedoSteps$'
invoke check --scope web --match assembly-edit-intent
invoke check --scope <assembly|geometry|sketch|workspace|services|web>
invoke check --base HEAD~1 --head HEAD --plan --json
invoke check --staged --plan
invoke check
invoke check --scope all
```

`--match` 只接受一个非 all 的显式 scope：C++ 使用所属 CTest 前缀内 regex，Go 使用 `-run`，Web 使用路径 substring。零匹配失败，选中的必需测试跳过也不能通过。精确入口跳过跨层集成与 production build；行为完成后升级领域 scope → 受影响集成 → all。

省略 scope 时按路径保守选域：Assembly、OCCT、Worker sketch、Workspace/Model Core、其他 Go、Web 分别映射对应 scope；Proto、迁移、共享 Worker/kernel API/构建/工具及未知路径升级 all。Markdown 不触发可执行测试，知识入口仍运行 `invoke context-audit`。准确实现见[tasks.py](../../tasks.py)。

Git 选择明确区分：默认比较 HEAD 与工作区并包含未跟踪文件；`--base` 改变比较起点；同时指定 `--head` 只选分支提交差异；`--staged` 只选暂存区。跨域重命名同时考虑原路径与新路径。干净工作区表示未选择验证，不表示某个提交已通过。

`--plan --json` 返回路径、scope、升级理由、构建类型、cwd 与命令，不执行测试。`invoke test` 使用同一全量执行与判定工具；公共协议、迁移和共享构建变化必须升级 all。

## 结论、日志与版本边界

- PASS：已实际执行的断言通过；不是能力覆盖率或浏览器保证。
- PASS_WITH_SKIPS：适用测试通过，另有明确 `NOT_APPLICABLE` 的可选/其他后端测试；报告保留每项跳过原因。
- ENVIRONMENT_BLOCKED：必需测试因缺环境跳过，包括父测试 PASS、子测试 skip 的情况。
- FAIL：断言、构建、进程或零匹配失败。
- NOT_RUN：无选择或前序失败后未执行。
- SOURCE_CHANGED：验证期间自有代码/构建输入发生变化，需要在稳定源码上重跑。

Go 使用结构化终态和 `-count=1`，真实跨包验证串行运行；外部 Worker 更新不能靠 Go 测试缓存证明。CTest 使用当前运行新生成的 JUnit；Node 场景保留零匹配失败。装配合同执行器与通用入口共用[判定工具](../../tools/validation.py)，专用 capability 验收仍要求其全部必需层。

每轮完整 stdout/stderr、复现命令和 JSON 报告位于 `build/agent-check/<run>/`。默认终端只输出步骤与耗时，失败展示相关测试/编译的高信号输出；`--verbose` 流式显示。报告绑定实际构建类型、制品哈希、资源身份及运行前后源码摘要；不保存密码。源码摘要不替代具体 corpus/外部输入的 provenance。

## 从诊断到复现

```bash
invoke diagnose --document <documentId>
invoke diagnose --reference 'CAD_DIAGNOSTIC <documentId>/<recordId>' --json
invoke diagnose --reference 'CAD_DIAGNOSTIC <documentId>/<recordId>' --attempt 0 --replay
invoke diagnose --job <jobId>
```

命令从当前日志根目录定位归档，或用 `--file` 读取明确提供的文件；检查路径、schema 与字节预算。已有数值输入导出到 `build/diagnostics/`，经同一生产 Worker 重放，不覆盖原文件或推进 Head。进程执行成功与求解结果分别报告。没有记录数值请求时明确返回该边界，不用新 Head 或几何猜测补造输入。

归档范围与字段归[可观测性](../architecture/current/validation.md)，装配数值选项归[重放 CLI](../../services/cmd/occccad-3dreplay/README.md)。完整领域事务的通用离线重放不属于已有数值重放能力。

## 测量与维护

`context-audit` 检查指南尺寸、导航可达性、链接/锚点、目录归属、任务源码位置与大文本热点。字节是静态代理，既不是 tokenizer 计数，也不证明端到端开发更快。

任务包记录本次搜索次数、返回源码字节和定位耗时；验证报告记录执行时间、输出字节和测试终态。真实模型 token 未取得时保留空值。工程基线应使用相同任务与验收范围，分别记录环境准备、定位、验证、返工和人工介入；生成结果保存在 build/，不累计为长期过程文档。

`invoke workflow-baseline` 串行测量五个配方的定位包和定向验证，输出 `build/agent-workflow/<run>/baseline.json`。每类仅一个样本；它不包含实现、返工或人工介入时间，也不代表端到端速度或稳定分位数。重测前先确保选定 Worker 已构建。

长期指南只保留规则。临时任务状态与原始报告放会话或 `build/`，CAD 任务顺序只归[主线](../../plans/cad-core-roadmap.md)，已知问题归所属主题限制。
