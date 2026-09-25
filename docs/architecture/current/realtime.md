# CAD realtime 控制面

返回[当前架构](../../CURRENT_ARCHITECTURE.md)。沿用 `occccad.realtime.v1` envelope、Workspace sequence、事务 Outbox 和现有 session/CSRF 鉴权，没有另建消息总线或增量同步模型。

## 接口边界

| 通道 | 当前职责 |
|---|---|
| WebSocket `/api/realtime` | CAD Domain Command、Preview request/cancel、Workspace 提交事件、Job 状态/进度提示 |
| HTTP 普通资源 | 登录/权限、文档目录与搜索、文件夹/管理、历史与属性查询、权威业务快照 |
| HTTP / ArtifactStore | 导入上传、导出下载、BREP/GLB/Naming 等文件数据 |

旧 `POST /api/documents/{id}/commands` 和 `command-previews` 路由已删除；Web 的既有 `api.command` / `api.previewCommand` 门面统一进入 realtime client。管理操作继续保留 HTTP，不把文件或所有查询改成 WebSocket。STEP/XDE/AP242 未改造。

## Envelope 与消息

首条消息仍为 `connection.initialize.v1`，验证 session cookie、CSRF 和 Origin；Vite `/api` proxy 必须保留 `ws: true` 和原始 Host/Origin。envelope 的 `id` 是一次传输尝试身份，响应通过 `correlationId` 对应；业务 `command.requestId` 是跨连接稳定幂等键，不因重试更换。

| 请求/事件 | 载荷与语义 |
|---|---|
| `document.subscribe.v1` / `document.unsubscribe.v1` | 按 Document 注册/撤销；订阅返回 workspaceId、versionId、sequence 元数据 |
| `workspace.command.execute.v1` | documentId + 现有 CommandRequest，必须带 requestId |
| `workspace.command.completed.v1` | requestId、documentId、已提交 versionId、transaction sequence；可附带不超过 64 KiB 且与该提交完全一致的轻量 DocumentView |
| `workspace.preview.request.v1` | documentId、interactionId、递增 previewSequence，以及同一 CommandRequest 模型 |
| `workspace.preview.ready.v1` | 对应 interaction/sequence、baseVersionId/baseSequence、previewId、求解诊断、Pose 或轻量 Artifact 引用 |
| `workspace.preview.cancel.v1` / `workspace.preview.canceled.v1` | 取消该连接内指定 interaction 的该序号或更早工作；旧取消不能取消更新序号 |
| `workspace.preview.failed.v1` | 保留 code、phase、message、retryable，错误不发布候选结果 |
| `workspace.transaction.committed.v1` / `stream.ack.v1` | 沿用持久 Workspace Outbox、单调序号与客户端 acknowledgement |
| `job.state.changed.v1` | Job 摘要；完整 payload/大型导入结果列表留在 Jobs HTTP 查询 |

保持 **1 MiB** 单消息上限，客户端发送和服务端回复/广播均执行边界，不因大模型提高上限。响应过大返回关联的 `MESSAGE_TOO_LARGE`；超大广播导致连接恢复并重新查询，不发送无界数据。错误体不携带几何文件。128 条发送队列满时断开慢消费者。

## 命令一致性与恢复

正式命令仍执行同一 adapter、typed handler、求值、短事务 CAS、ChangeSet、Revision 和 Outbox。actor、requestId 和 intent digest 必须匹配已提交记录；同一 requestId 改变 intent 不会重新执行。重连重试与原请求并发时，在 Workspace 锁内再次检查已提交记录；若适配/求值因并发 Head 变化失败，也只恢复已经存在的同 actor、同 digest 提交结果，不自动 rebase 未提交意图。

客户端对连接丢失、超时或调度繁忙最多自动重试一次，沿用业务 requestId，使用新的 envelope id。错误保留结构化 code/phase/retryable，验证失败不当作网络失败重试；显式停止不会重新连接。断线不是事务回滚证明，未知提交结果须经幂等记录恢复。

服务端读循环只处理 envelope、ack 和预览控制；普通请求进入每连接 16 条有界串行队列，从入队开始计时的单次操作有 2 分钟 deadline。耗时命令不阻塞 Ping/Pong 或 Preview Cancel。队列满明确返回 `REALTIME_BUSY`，不报告提交成功。

## 权威快照与 sequence

订阅通过 WebSocket 通知快照身份。小型命令完成结果可内联业务快照，读取后核对当前 Head/sequence 与提交回执完全一致；超过 64 KiB、并发提交或投影失败时仅返回回执，客户端走 HTTP 获取。客户端拒绝落后于已观察 sequence 的内联结果。已知大快照的客户端以 `inlineSnapshot=false` 跳过内联投影，避免每次提交重复构建大文档；服务端始终独立执行字节上限检查。`GET /api/documents/{id}/realtime-snapshot` 返回轻量 DocumentView（无 Mesh/Naming 大载荷）及 workspaceId、versionId、sequence；读取前后核对 Head/sequence，并要求 view.versionId 相同，避免给旧 View 配上新 sequence。持续变化时有界重试后返回可重试错误。

订阅先登记再读取 Head；客户端随后获取权威快照，丢弃落后于已观察事件序号的快照。重复/乱序事件按 sequence 去重；gap 自动触发快照恢复，不依赖 UI 仅失效缓存。重连重新订阅所有仍需关注的文档，再获取权威快照；失败重试通过重连完成，权限撤销明确报告 unavailable。旧连接消息和旧连接的 HTTP 快照不能覆盖新会话。

普通连续提交事件仍驱动既有 Query 刷新；未引入复杂实体 delta/补丁同步。Hub 仍是单 API 进程，跨 API 实例广播与共享 transient candidate 尚未实现。

## Preview 生命周期及文件授权

Preview 保持不创建 Revision、不推进 Head、不写历史/提交 Outbox。相同 interaction 的新序号取消旧计算并撤销旧候选；同号/更旧请求拒绝。取消、退订、断线和到期后的迟到结果被丢弃。求值完成再次核对 base Head；前端还以 interaction sequence、已观察 Workspace sequence 和视口加载 generation 隔离迟到结果。

每连接最多 4 个尚未返回的 preview evaluator 和 64 个短期 interaction 记录；被取消的 evaluator 在实际返回前仍占额度，不能靠连发取消绕过资源限制。Preview deadline 为 15 秒；客户端 20 秒超时后发送 cancel。取消向 Go/Worker context 传播，但不能保证 OCCT 内所有算法立刻停止；迟到结果不会发布。暂存输出路径属于独立求值 attempt，稳定业务 requestId 不再造成并发文件覆盖。

沿用 45 秒、一次性 verified candidate：提交仅在 actor、document、Head/sequence、typed payload 完全一致时提升候选，否则正常重算。候选缓存丢失不会成为成功提交的前提。Part 和装配继续共用这条门，不创建平行 Preview 命令体系。

Preview Artifact 标记 `TRANSIENT_PREVIEW`，不含 `previewMesh`；完整显示仅来自 GLB。`representations.VISUAL.url` 使用既有文件路由和 `previewId`，文件读取同时校验文档权限、actor、候选存活期和对象归属。取消、断线、候选消费或过期后该授权失效；正式 Revision 的对象按原 Artifact 授权读取。

视口用独立可取消的 GLB 加载器显示预览，清除预览时取消下载并释放工作集；不会把临时 Mesh 放回 Query 状态。当前求值仍可生成内容寻址 Artifact 对象；取消只撤销候选/授权，不删除可能共享的对象，孤儿对象 GC 属于后续生命周期工作。

## Job 与验证

排队、领取、进度、重试等待、主动重试、取消请求与终态都写入 Job Outbox；状态和 hint 在同一 SQL statement 或同一领取事务提交；API 按 Job 合并待发提示，向发起用户推送当前摘要，不复制完整 Job payload。继续沿用同一 Outbox。重连后客户端刷新 Jobs 权威列表；消息中心保留活动任务的 HTTP 兜底刷新。

定向入口：

- `go test ./internal/api ./internal/workspace -run 'TestRealtime|TestInteractionCandidate|TestPreviewAccess' -race`；设置隔离测试 `OCCCCAD_TEST_DATABASE_URL` 后覆盖真实 session/CSRF WebSocket、候选与 GLB 授权、丢失回执重试、覆盖/取消、Job 进度；`OCCCCAD_TEST_VITE_REALTIME=1` 额外启动临时 Vite 验证代理 Upgrade，无浏览器。
- `go test ./internal/jobs -run TestJobLifecycleRealtimeOutboxDatabase`：隔离数据库验证排队、领取、进度、重试、取消和完成的状态/Outbox 原子写入。
- `pnpm test -- realtime-control`：客户端 requestId 重试、correlation、gap/快照、Preview 覆盖/取消/超时、结构化错误、消息上限。
- `mesh-glb`、`assembly-preview-machine`、`command-preview-identity` 场景及 TypeScript 检查，分别覆盖显示、交互状态和候选身份。

本阶段未做浏览器验收、全量单测或 STEP/XDE 重构；无需数据迁移，前后端需一起更新协议调用边界。

## 交互关键路径

命令成功回执在事务提交及快照校验后发送，不等待缩略图任务入队；入队仍在原有有界执行器内完成，不额外启动无界后台任务。Preview 下载保留用户/文档权限检查，其对象归属由已验证候选中的精确 Visual objectId 判断，取消、消费及过期均撤销该授权，免除重复的 geometry_representations 查询。

Viewport 只共享最近一次完整下载并校验成功的 Preview GLB 解码结果，以 objectId + digest 匹配提交显示。未完成下载仍由各自 Preview 独立取消；历史 Preview 不累积，切换文档清理复用槽。复用的是显示数据，不提升候选或替代服务端权限、事务校验。

超过 250 ms 的 Command/Preview 控制操作输出 `slow realtime operation`，包含 correlation_id、duration_ms、独立 phases_ms；命令计时止于回执排队，不包含后续缩略图入队。HTTP 文件与快照耗时仍由现有 HTTP 日志记录。
