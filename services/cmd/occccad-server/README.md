# occccad-server

occccad-server 是当前系统的 HTTP/WebSocket API 与业务编排进程。它是模块化单体，不是多个虚构微服务的集合。

## 当前职责

- 初始化管理员账号和数据库会话认证；
- 提供用户、团队、文档、文件夹、版本、历史、分享、ACL 与审计 API；
- 提供版本化 WebSocket 消息、文档订阅、命令请求响应和 Workspace Outbox 实时事件；
- 验证访问权限，把 HTTP transport DTO 转换为版本化 Domain Command，并由 typed handler 应用到显式 Part/Product Workspace；
- 在数据库事务外通过 gRPC 求值候选模型，再以 Workspace Head/sequence CAS 原子持久化 Transaction、ChangeSet、Revision、EvaluationManifest、依赖投影和 outbox；
- 将 STEP/BREP 文档交换与缩略图工作写入 PostgreSQL 持久任务队列；
- 通过 ArtifactStore 读写本地内容寻址制品；
- 暴露健康检查并传播 HTTP/gRPC Trace Context。

它不提供静态前端，不负责消费后台任务，也不直接执行 OCCT 几何算法。

## 依赖与调用关系

```mermaid
flowchart LR
    Browser["CAD Web / HTTP"] --> Server["occccad-server"]
    Server --> DB[(PostgreSQL)]
    Server --> Store["Local ArtifactStore"]
    Server --> Router["Geometry Worker gRPC endpoint"]
    Server -. "enqueue" .-> Jobs[(jobs table)]
```

启动时会自动执行数据库迁移，并要求 Geometry gRPC 地址可连接。当前 ArtifactStore 固定为本地文件系统。

## 接口

- 默认监听：`0.0.0.0:8080`
- 健康检查：`GET /api/health`
- API 前缀：`/api/`
- 实时入口：`GET /api/realtime`，子协议 `occccad.realtime.v1`
- 根路径返回服务说明，不提供 Web 静态文件。

主要资源包括 `/api/auth/*`、`/api/session`、`/api/documents`、`/api/folders`、`/api/jobs`、`/api/teams`、`/api/users`、`/api/admin/*` 与 `/api/audit`。具体契约当前以 `services/internal/api/server.go` 为准；仓库尚未发布稳定的外部 OpenAPI。

文档交换使用独立资源：`POST /api/exchange/imports?format=STEP|BREP&fileName=...` 把原始 request body 流式写入 ArtifactStore，限制 128 MiB；`POST /api/exchange/exports` 提交 `{documentId, format}`；`GET /api/jobs` 恢复当前用户最近 100 条可见任务，`POST /api/jobs/{jobID}/cancel|retry` 执行发起者或管理员动作，任务完成后从 `GET /api/jobs/{jobID}/download` 流式下载。导入不要求先创建 Part，不使用 multipart，也不让大文件经过 WebSocket 或 gRPC bytes。

`GET|POST /api/documents/{documentID}/workspaces` 用于列出 Workspace 或从所属 Revision 创建 Branch。`POST /api/documents/{documentID}/commands` 是保留的 HTTP transport；Web 使用 `workspace.command.execute.v1` WebSocket 消息。二者进入同一 Workspace handler。`POST /api/documents/{documentID}/command-previews` 在当前 Head 上运行相同 command adapter、typed handler、Sketch Solver 与 Part evaluator，返回 base Revision 和精确 Artifact，但不创建 Revision、历史、Outbox 或推进 Workspace；请求受 Editor ACL、HTTP cancellation 和 15 秒 deadline 约束。`SET_PARAMETER_VALUE` 接受 `parameterId/value/unit`，`SET_PARAMETER_EXPRESSION` 接受 `parameterId/expression`。表达式在服务端绑定稳定 ParameterId，Worker 不解析用户 source text。

`POST /api/documents/{documentID}/diagnostic-bundles` 为具有文档读取权限的用户生成不可缓存的 `occccad.cad-diagnostic-bundle.v1` JSON 下载。Web 在草图求解命令失败时自动提交失败命令和客户端环境，也允许通过 Debug Toolbar 手动导出当前状态；服务端补充当前文档、草图、Workspace、历史、最近事务/命令错误及 evaluator provenance。该接口不导出 Cookie、密码、其他文档日志或 B-Rep 原始字节。

WebSocket 首条消息必须是携带 CSRF token 的 `connection.initialize.v1`。之后可发送 `document.subscribe.v1`、`document.unsubscribe.v1`、`workspace.command.execute.v1` 与 `stream.ack.v1`；服务端返回 correlation response/error，并从事务 Outbox 发布 `workspace.transaction.committed.v1`。单消息限制 1 MiB，大制品仍走 HTTP/ArtifactStore。

后台 Exchange 到达最终状态时发布用户级 `job.state.changed.v1`。事件只发送到 `requested_by_user_id` 对应的连接；没有在线连接时保持在 Outbox，不能因一次空广播丢失完成通知。前端提交后立即返回 Document Center，导出结果只在用户点击通知中的下载动作时走流式 HTTP。

可在整套应用运行时设置 `OCCCCAD_REALTIME_TEST_URL`、`OCCCCAD_ADMIN_PASSWORD` 与 `OCCCCAD_EXCHANGE_TEST_STEP`，运行 `go test ./internal/api -run TestRealtimeExchangeImportNotification -count=1 -v`，验证“HTTP 提交立即返回、后台导入、用户级 WebSocket 终态通知”这一真实入口；测试本身不查询 Job 状态。

RFC 6455 Upgrade、frame、Ping/Pong 由 `github.com/gorilla/websocket` v1.5.3 提供；该依赖使用两条款 BSD 许可。它只位于 HTTP transport adapter，领域 Envelope、Command、Outbox 和恢复语义不依赖其类型，未来可以替换实现而不改变持久模型。

## 配置

| 变量 | 默认值 | 说明 |
|---|---|---|
| `OCCCCAD_DATABASE_URL` | 由 PostgreSQL 分项变量生成 | 完整连接串，优先级最高 |
| `OCCCCAD_POSTGRES_HOST` | `127.0.0.1` | PostgreSQL 主机 |
| `OCCCCAD_POSTGRES_PORT` | `5432` | PostgreSQL 端口 |
| `OCCCCAD_POSTGRES_USER` | `occccad` | 数据库用户 |
| `OCCCCAD_POSTGRES_PASSWORD` | 空 | 数据库密码 |
| `OCCCCAD_POSTGRES_DB` | `occccad` | 数据库名 |
| `OCCCCAD_SERVER_LISTEN` | `0.0.0.0:8080` | HTTP 监听地址 |
| `OCCCCAD_GEOMETRY_WORKER_ADDRESS` | `127.0.0.1:51001` | Geometry gRPC 地址，可指向 Router |
| `OCCCCAD_DATA_DIR` | `./data` | 本地制品根目录，相对启动目录 |
| `OCCCCAD_ADMIN_EMAIL` | `admin@occccad.local` | 初始管理员邮箱 |
| `OCCCCAD_ADMIN_DISPLAY_NAME` | `Administrator` | 初始管理员显示名 |
| `OCCCCAD_ADMIN_PASSWORD` | 无 | 首次及每次启动均必须非空；不会覆盖已有密码 |
| `OCCCCAD_SESSION_DURATION` | `12h` | 会话有效期 |
| `OCCCCAD_SECURE_COOKIES` | `false` | 生产 HTTPS 环境必须为 `true` |
| `OCCCCAD_ALLOWED_ORIGINS` | 空 | 逗号分隔的跨域 Origin；同源代理无需设置 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | OTLP 导出端点；为空时仍记录 Trace ID |

## 运行

在仓库根目录配置 `.env` 后：

```bash
invoke run.server
```

完整本地拓扑通常通过 `invoke run.app` 由 occccad-control 管理，此时 API 实际监听内部地址并由控制进程代理。

## 一致性与故障语义

- PostgreSQL 是业务真相；Geometry Worker 返回的是可重建的计算结果。
- 写操作经过身份和 ACL 校验；昂贵求值不占用数据库事务，最终提交以 Head CAS 防止迟到结果覆盖新 Revision。
- Undo/Redo/Restore 都追加 Transaction 与 Revision；Revert/Reapply 围绕稳定根 Transaction 折叠，支持连续多步 Undo/Redo。`canUndo/canRedo` 由同一 actor history fold 计算并返回 Web；字段 digest 或结构依赖不匹配时返回冲突。
- DocumentView 的 Specification Tree 节点携带服务端计算的 capability。`DELETE_NODE` transport 意图只可适配为受支持的 Part Feature/Sketch child 或 Product Instance typed command；未列出的节点类型（包括基准面和轴）默认拒绝。Sketch Entity 删除与引用约束清理属于同一原子 `sketch.model` 变更。
- 当前是无生产数据的新项目阶段；C0–C4 schema 变更要求重建开发数据库，不提供旧 cursor/history adapter。
- 后台任务具有幂等键；API 返回任务后不保证任务已完成。
- WebSocket 事件按 Workspace sequence 去重；断线、gap 或慢消费者被断开后，浏览器重新订阅并获取权威快照。当前 Hub 只覆盖单个 API 进程，多实例需要事件总线扇出。
- HTTP header 超时为 5 秒；为支持受限的 128 MiB 流式上传下载，body 读写超时为 15 分钟。几何解析等长操作仍必须进入任务系统，不能占用 HTTP 请求。
- 进程重启会丢失内存中的“已打开文档”注册表，但不会丢失持久文档。

## 验证与扩展边界

```bash
cd services
go test ./...
```

扩展业务能力优先增加 `services/internal` 模块，只有出现独立扩缩容、故障隔离、安全边界或发布节奏需求时才拆分网络服务。长期演进见项目[目标架构](../../../docs/TARGET_ARCHITECTURE.md)。
