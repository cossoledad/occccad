# Go Control Plane Agent Guide

## Scope

`services/` 是 Go 模块化控制面：API/Auth、Workspace/Model Core、Geometry client/router、Jobs、Artifact 和数据库迁移。`internal/*` 是同一部署族内的包，不因领域名词自动变成微服务。

## Invariants

- Command handler 产生确定、无 I/O 的候选模型；昂贵求值在事务外，最终短事务 CAS 提交。
- evaluator/solver 规范化后，ChangeSet 必须从最终持久 Revision 重建。新增 PropertySlot 同时贯通 current/read/write、补偿、canonical digest、依赖与 Undo/Redo 测试。
- capability 与 Undo/Redo 执行使用同一 action-log 折叠语义；依赖图提交时验证全部 edge 两端。
- PostgreSQL 使用 `occccad.<table>` 或显式 `search_path`。至少一次消息/任务按效果幂等，迟到 compute 结果不得推进已变化的 Head。
- 原始 B-Rep bytes 保留在 ArtifactStore；控制面不从 Three.js mesh 猜测精确拓扑。

## Context route

先从所属 `internal/<package>` 的入口与 `_test.go` 搜符号。以下大文件只读命中范围：`internal/workspace/service.go`、`model_core.go`、`service_test.go`、`internal/api/server.go`、`internal/geometry/client.go`。按 Domain Command/handler、evaluation、history、projection/API adapter 的实际调用链扩展，不从文件开头顺读。

公共协议先读 `../proto/`，不先读 `gen/`；生成代码仅在 codegen/API discrepancy 时打开。进程运行和配置看 `cmd/*/README.md`。跨稳定身份、Revision/history、数据库模型或公共协议时，由 `../docs/README.md` 路由到架构章节。

## Validation

- 单包优先：在本目录运行 `go test ./internal/<owner>`，可用 `-run '<name>'` 精确筛选。
- 稳定入口也支持 `invoke check --scope workspace --match '<test regex>'` 或 services scope；match 不包含跨包 conformance。
- Workspace/Model Core：`invoke check --scope workspace`
- 其他控制面：`invoke check --scope services`
- Proto、迁移、共享 Router/build 边界：`invoke check --scope all`
- 数据语义变化按根指南从空 schema 验证；清库不是 Undo/Redo、重试或状态转换测试的替代品。
