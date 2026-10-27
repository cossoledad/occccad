# occccad-dev

一次性本地开发 CLI，公开任务由根 mise 配置和 Usage file tasks 提供。职责分包文件位于 [internal/devtool](../../internal/devtool/)：依赖身份与原生调用、变化选择、验证证据、诊断、环境检查和安全清理。它不拥有 CMake 依赖图，不管理应用进程；生命周期由 Control 拥有。

独立准备：`mise run setup --component tooling --jobs 4`，输出 `build/tools/occccad-dev`。不需要 OCCT、Worker、数据库或前端；工具更新后重做这一步。帮助/计划不隐式重建。

`mise tasks ls` 发现公开任务；`mise run <task> --help` 查询参数。`mise run --quiet plan --changed --level unit --json` 只读选择。注册表在 [dev/validation.json](../../../dev/validation.json)，只能引用叶任务与结构化参数，不循环调用 check。构建本身不加载业务环境。

[环境](../../../docs/development/environment.md)说明工具、锁、构建、资源与运行；[Agent 工作流](../../../docs/development/agent-workflow.md)说明选择/状态/报告/诊断；资源 checker 仍复用当前配置，不迁移或清库。工具源码/fixture 的测试不是产品集成证据。
