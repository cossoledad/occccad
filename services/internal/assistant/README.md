# Conversational Modeling Assistant

本包拥有 DeepSeek/OpenAI 格式客户端、按账号加密的 API Key、有界对话规划循环和模型无关的 Tool/Call/Result。几何写入仍由 [Automation](../automation/README.md) 与原有 Workspace 执行。

`provider.go` 用标准库调用 Chat Completions；`credentials.go` 用 AES-256-GCM 保存账号凭证；`conversation.go` 保存请求、回复、usage 和任务引用；`budget.go` 统一低/中/高的 Token、调用次数、上下文、输出与时间边界；`tools.go/context.go` 绑定用户/文档、查询与合同。规划器只读取事实、生成计划，不能调用 advance 或伪造显示确认。应用将局部步骤标签转换为稳定 UUID，绑定 requestId 和冻结 Head。

用户在下边栏打开建模助手，在连接设置中填写 Key；验证成功后不会回显。默认地址 `https://api.deepseek.com`、模型 `deepseek-flash`，使用原生工具调用；模型列表由当前账号 `/models` 提供，支持账号保存模型及关闭/低/高/最高思考度。调用预算为低 8 万、中 20 万、高 50 万 Token，默认中档；每次需求独立计算并冻结实际设置，公开对话显示已报告用量和模型调用次数。工具循环保留私有思考上下文，公开对话不保存思考原文。服务端可覆盖 `OCCCCAD_AI_BASE_URL/MODEL/KEY_FILE`，无需模型 SDK。默认加密主密钥自动建立于 `<OCCCCAD_DATA_DIR>/assistant/credentials.key`；数据库与此文件共同恢复，多 API 进程共用同一文件。丢失后需重新填写 Key，不回退明文存储。

在 `services/` 运行 `go test -p 1 ./internal/assistant ./internal/automation ./internal/api -run '^TestAssistant|^TestAutomation'`。协议测试使用本地受控服务，CAD 集成复用 `.env` 的数据库、制品与原生 Worker；这不证明线上模型质量。详细预算、恢复、UI 与验收入口见[当前合同](../../../docs/architecture/current/automation.md)。
