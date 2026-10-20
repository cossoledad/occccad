# Assistant Tool Host

本包将 [Automation](../automation/README.md) 适配为模型无关的 Tool/Call/Result 和有界 ContextPacket。尚无模型客户端、自然语言规划循环、密钥或网络模型请求。

应用以用户、文档和写权限构造 Host；这些范围不进入模型可改写参数。发现与完整合同分开，任务读取返回检查点，业务写入由 Automation 执行，模型不能发出显示确认。

`Context` 组装规则、快照、持久检查点与少量合同，限制字节数并明确报告超额。供应商适配器未来只翻译数据结构，继续经过相同校验与权限门；DeepSeek 为后续测试候选。

在 `services/` 运行 `go test ./internal/automation -run 'TestAutomation.*(Catalog|Bounded)'`，包外场景验证实际 Host 分派、只读限制、目标防伪和上下文预算。详细限制见[当前合同](../../../docs/architecture/current/automation.md)。
