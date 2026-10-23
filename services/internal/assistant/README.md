# 持续对话建模 Assistant

本包拥有账号凭证、Provider、持久 Run/用户输入、调用账本和后台规划调度。模型只提出有界片段；几何写入经 [Automation](../automation/README.md) 与既有 Workspace Preview/Command/CAS。运行、恢复、用量、选择、导出及显示的唯一合同见[自动化主题](../../../docs/architecture/current/automation.md)。

## 源码路由

| 职责 | 入口 |
|---|---|
| 身份/分页、后台租约、规划 | `run_store.go`、`run_scheduler.go`、`run_planner.go` |
| 阶段执行、精确回执、当前请求结束 | `run_stages.go`、`run_completion.go`、`run_targets.go` |
| 冻结选择、查询引用与有界工作集 | `run_scope.go`、`run_query_bindings.go`、`run_context_projection.go`、`run_stage_context.go` |
| 公开简述、独立只读问答 | `run_progress.go`、`run_questions.go` |
| 逐尝试账本、轮次用量、完整导出 | `call_ledger.go`、`turn_usage.go`、`session_export.go`、`call_trace.go` |
| Provider、凭证、单请求资源 | `provider.go`、`credentials.go`、`budget.go` |
| 规范工具结果、无模型查询调试 | `tool_result.go`、`query_debug.go`；[调试操作](../../../docs/development/cad-query-debugging.md) |

## 配置与验证

默认地址、模型及单次保护由 `OCCCCAD_AI_BASE_URL/MODEL` 和账号设置控制，具体默认值与协议边界见[Provider 合同](../../../docs/architecture/current/automation.md#资源与-provider-用量)。累计预算默认可空；无 Run 总调用/步骤/阶段/整轮时限，单次资源保护继续有效。

主密钥默认 `<OCCCCAD_DATA_DIR>/assistant/credentials.key`（0600），可覆盖 `OCCCCAD_AI_KEY_FILE`，须与数据库共同备份。多 API 进程使用相同主密钥与 Provider 配置；调度认领由其非秘密 digest 隔离。设置、事件和账本不回显 Key 或私有思考。

在 `services/` 运行 `go test -p 1 ./internal/assistant ./internal/automation ./internal/api -run '^TestAssistant|^TestAutomation'`。真实集成沿用当前开发资源，故障注入使用测试临时对象；合成百万 usage 不调用付费 Provider。[证据与当前限制](../../../docs/architecture/current/automation.md#当前限制)不等于线上推理质量保证；明确小预算的线上入口见 [ai-eval](../../cmd/occccad-ai-eval/README.md)。
