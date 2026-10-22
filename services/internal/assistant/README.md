# 持续对话建模 Assistant

本包负责账号凭证、DeepSeek/OpenAI 格式 Provider、持久 Run、用户输入、调用账本及后台规划调度。几何写入只经 [Automation](../automation/README.md) 与既有 Workspace Preview/Command/CAS；模型不能执行步骤或伪造显示。

入口：`run_store.go`（身份/分页）、`run_scheduler.go`（租约/后台/公平并发）、`run_planner.go`（有界工作集/下一片段）、`run_stages.go`（执行观察/精确回执）、`run_scope.go`（冻结选择/目标来源）、`run_goal_targets.go/run_verification.go`（要求与正式输出绑定/当前目标证据）、`run_progress.go`（公开简述/精简检查点）、`run_questions.go`（独立只读问答）、`call_ledger.go`（逐尝试用量）、`call_trace.go`（实际公开请求/回复/工具反馈及执行版本）、`provider.go`、`credentials.go`、`budget.go`（单次请求资源）。`conversation.go` 仅定义公共数据合同，无旧同步规划运行时。

`tool_result.go` 统一工具内容的 32 KiB 边界，规范结果同时用于在线消息、账本、恢复与 trace。`query_debug.go` 提供无凭证/Run/Provider 的只读对照适配，复用 Host 与 Automation，原始服务页和人工补充不回灌模型。操作见[查询调试](../../../docs/development/cad-query-debugging.md)。

默认地址 `https://api.deepseek.com`，模型 `deepseek-flash`，支持账号 `/models` 发现、关闭/低/高/最高思考度；实际配置逐调用冻结。默认累计 Token 预算为 `null`，没有 Run 总调用/步骤/阶段/整轮时限；用户主动设置预算后可提高或清空并继续同一 Run。账号默认预算与活动 Run 预算分别保存。单次上下文、输出、网络与工具执行仍有资源保护。

Key 用 AES-256-GCM、账号 AAD 加密；主密钥默认 `<OCCCCAD_DATA_DIR>/assistant/credentials.key`（0600），与数据库共同备份。可覆盖 `OCCCCAD_AI_BASE_URL/MODEL/KEY_FILE`。多 API 进程须使用相同主密钥及 Provider 配置；调度认领由主密钥与 Provider origin 的非秘密 digest 隔离。设置、事件、账本不回显 Key 或私有思考。在线工具回合私有思考仅在内存，重启/压缩后从公开事实重新构造新请求，不回放缺少思考的旧 transcript。

在 `services/` 运行 `go test -p 1 ./internal/assistant ./internal/automation ./internal/api -run '^TestAssistant|^TestAutomation'`；共享开发 SQLite 验证串行运行。百万用量测试使用合成 usage，不调用付费 Provider。小规模线上入口见 [ai-eval](../../cmd/occccad-ai-eval/README.md)。完整运行、恢复、选择及显示合同见[当前架构](../../../docs/architecture/current/automation.md)。
