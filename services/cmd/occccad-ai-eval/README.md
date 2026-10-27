# Explicit live-provider evaluation

该命令只在开发者显式运行时发起真实模型调用，不属于单测、`mise run check` 或 CI 默认流程。它通过当前 CAD HTTP/session 与正式 realtime Preview/Command API 创建独立评测文档，使用已有账号保存在服务端的 API Key；不读取、回显、更换或删除 Key，也不修改模型、思考度和账号预算。

从仓库根目录启动当前默认 DeepSeek 配置的服务后，在 `services/` 执行：

```bash
go run ./cmd/occccad-ai-eval --case box --token-budget 20000 --timeout 5m
go run ./cmd/occccad-ai-eval --case box-fillet --token-budget 20000 --timeout 5m
go run ./cmd/occccad-ai-eval --case shaft-hole --token-budget 20000 --timeout 5m
```

`--base-url` 默认当前 `.env` 的 CAD API 地址；`--email`/`--password` 默认当前环境的管理员配置。优先使用环境载入密码，避免将密码放入 shell 历史。支持 `--case all`，每个用例都是独立 Run，并分别受显式小额累计 token 预算约束。`--timeout` 只限制评测等待；到时按最新版本发送 `stop`，保留已应用 CAD 结果、Run 和用量账本，不改变生产 Run 的持续运行语义。停止请求结果未知时如实记录，不能声称已经停止。

输出默认写入仓库 `build/acceptance/ai-eval.json`（可用 `--output` 指定），按用例保存真实 Run ID、模型/思考度/设置版本、实际每次调用与已报告用量、缓存/思考细分缺失、unknown attempts、修订事件、运行/独立语义验收结果、耗时和生成文档 ID。生成的 CAD 文档保留供检查，不清理共享数据。

没有已配置 Key 时输出 `NOT_RUN`，不注册测试凭证。服务端必须报告可验证的非本机 HTTPS `providerOrigin`，否则记录 `LIVE_PROVIDER_ORIGIN_NOT_ATTESTED`，不把本地 fixture 冒充 `LIVE_PROVIDER`。此检查确认服务端配置来源，不是对远端服务身份或模型质量的密码学证明。真实 Provider 的计划仍必须通过应用/内核检查；HTTP 成功、完成文案或固定 Provider 计划均不算验收通过。

案例入口为 [`tasks/ai-eval-cases.json`](../../../tasks/ai-eval-cases.json)：矩形拉伸、同一完整目标中的全部边 R2、普通参数化轴/套筒的同轴和底端面定位并保留一个旋转自由度。轴孔 fixture 的圈草图和实体由正式命令确定生成；模型需要自行观察、选择、建配合和完成目标，fixture 生成本身不是模型规划能力证据。
