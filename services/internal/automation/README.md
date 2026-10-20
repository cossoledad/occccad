# CAD Automation

本包提供供应商无关的操作目录、快照查询和持久任务，复用 Workspace、ACL、预览、求值与 CAS。当前范围见[当前合同](../../../docs/architecture/current/automation.md)。

- `catalog.go`：操作字段、schema、发现与命令编译。
- `query.go`：本级不可变定义的过滤、投影与分页。
- `service.go`：授权、资源额度、预览/提交与回执恢复。
- `tasks.go`：版本化账本、步骤绑定、短锁/租约和人工控制。

HTTP 提供查询和任务定义；操作推进走现有鉴权 realtime。应用构造 Service 并传入真实 Workspace/Authorizer。没有独立服务、模型 SDK 或新增启动配置。

在 `services/` 执行 `go test -p 1 ./internal/automation ./internal/api -run '^TestAutomation' -count=1`。真实链路复用 `.env` 的数据库、制品和原生 Worker；合成查询使用测试拥有的短期对象。迁移或共享边界改变还需全量路由检查。
