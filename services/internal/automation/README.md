# CAD Automation

本包提供供应商无关的操作目录、快照查询和持久任务，复用 Workspace、ACL、预览、求值与 CAS。当前范围见[当前合同](../../../docs/architecture/current/automation.md)。

- `catalog.go/commands.go`：公开领域命令与工作台命令目录、渐进类型 schema、发现和编译；覆盖测试防止遗漏。
- `query.go/topology.go/selection_resolution.go`：不可变定义分页与 Revision/Body 范围的精确拓扑集合；旧选择经正式 Naming 重定位，不把第一页当全集或只改 Revision。
- `service.go`：授权、资源额度、预览/提交与回执恢复。
- `tasks.go/execution_control.go`：单段 1..32 步、版本化账本、短锁/租约和人工控制；Assistant Run 可持续创建新段，不限制整个目标的总步数。
- `bindings.go`：声明允许的实体、AssemblyRef、InstancePath 输入；多输出按稳定 ID 或唯一 role 选择，禁止数组位置和任意 JSON 路径。
- `receipt_reconciliation.go/run_receipts.go`：停止或旧代际仍能只读恢复原请求回执；Run 与单段回执标记在同一短事务内记账，重放不重计。
- `part_verification.go`：显式对象和实际快照绑定的尺寸、特征、完整全部边/正式选边、保护几何与权威制品检查。Assistant 按需调用，不编译历史自然语言或阻止结束当前请求；模型回复不作为几何证据。

`AUTOMATIC` 在真实 Preview → evaluator/CAS 提交后记为 `APPLIED`，后台可继续，不依赖浏览器逐步推进。`CONFIRM`（独立 API 默认）在 `WAIT_CONFIRM` 等待绑定 Run/代际/步骤/版本/基线/精确候选的确认；候选失效后重新预览、重新确认。`presentation` 独立记录 `NOT_PRESENTED/PRESENTED/FAILED`，只接受精确已应用 Revision 的真实视口回报，显示失败不重复建模。

输出绑定或合同校验失败也会持久保存 `FAILED` 步骤和 `PAUSED` Task；Run 先结束旧阶段并归档原请求的精确回执，再把诊断交回 Agent 决定修复或如实结束本次请求。重复失败保留语义摘要和原反馈，不自动拦截模型的下一次规划；权限拒绝、用户取消和未知提交结果不自动重放。

`ui.*` 先持久 handoff，再进入 `WAIT_UI`。`UI_OPENED` 仅表示已打开；完成需原工具携带预注册 `domainRequestId` 提交，应用核对精确回执、输入基线和已声明的领域效果。未知结果不重放，取消不视为成功。尚无完成效果合同的界面工具保留人工接续，不借任意新 Head 宣称完成。

HTTP 提供查询、任务定义和 Part 验证；独立工具可用既有鉴权 realtime，持久 Run 在服务器调用同一执行器。应用构造 Service 并传入真实 Workspace/Authorizer。没有独立服务、模型 SDK 或新增启动配置。

属性检查器的[只读查询调试](../../../docs/development/cad-query-debugging.md)经同一查询和工具处理链展示服务原页/模型可见结果，固定文档、Revision、Body/选区，沿用 Viewer 权限；无需模型或 Run。人工属性补充不等于模型已知事实。

在 `services/` 执行 `go test -tags=integration -p 1 ./internal/automation ./internal/api -run '^TestAutomation' -count=1`。真实链路复用 `.env` 的数据库、制品和原生 Worker；合成查询使用测试拥有的短期对象。迁移或共享边界改变还需全量路由检查。
