# 数据库访问与调度

`database.Open` 返回统一的 `database.Pool`，API、Workspace、Access、Authn、Artifact、Jobs 使用同一入口。底层复用现有 pgx/v5 与 golang.org/x/sync/semaphore，不引入新服务或持久队列。迁移和开发 reset 仅在本包内使用原始连接，保持 advisory lock 的连接归属。

## 执行与一致性

- 每个普通 SQL、结果流或完整事务占用一个执行名额。没有名额时，调用 goroutine 可取消地等待；等待队列有界，满时返回 `ErrBusy`，超时返回可识别的 admission 错误。API 命令/预览/权限入口以 503 + DATABASE_BUSY 表达过载，WebSocket 命令使用 retryable 错误。
- 调用接口仍等待真实查询/提交结果；没有提前成功、write-behind、离线操作或自动重试。写入结果不明时由原 Domain Command request ID 幂等处理，数据库层不重放写入。
- `Begin/BeginTx` 从 BEGIN 到 COMMIT/ROLLBACK 持有名额。事务内语句、批次和 savepoint 直接使用同一 pgx.Tx，不能重新进入 Pool 排队。现有 Workspace 锁/CAS、Revision、ChangeSet、Outbox 原子边界保持不变。
- `Query` 必须遍历完成或 Close；`QueryRow` 必须 Scan；`SendBatch` 必须 Close；事务必须结束。所有错误路径同样释放资源。不要在持有未消费的 Rows 时发起另一个 Pool 操作；先收集结果并关闭，或使用同一事务连接。
- `ExecBatch` 在调用方事务内按最多 128 条分批发送无返回值的写入，保持顺序，任意批次失败由调用方回滚整个事务。不得向此 helper 传入结果回调。
- API 的监控与 Outbox 轮询使用 `database.Background(ctx)`，有独立等待预算和较小执行上限。Jobs 是独立进程，有独立池与预算；部署总连接数需合并计算，当前不是跨进程全局限流。

## 配置

| 配置 | 默认值 | 含义 |
|---|---|---|
| 连接串 `pool_max_conns` | pgx 默认值 | 每进程物理连接上限 |
| `OCCCCAD_DB_CONCURRENCY` | `pool_max_conns` | 活跃数据库单元上限，不得超过物理连接上限 |
| `OCCCCAD_DB_QUEUE_CAPACITY` | 64 | 前台最大等待数量，0 表示无等待缓冲 |
| `OCCCCAD_DB_BACKGROUND_CONCURRENCY` | `max(1, CONCURRENCY/4)` | 后台活跃上限，不得超过总上限 |
| `OCCCCAD_DB_BACKGROUND_QUEUE_CAPACITY` | 16 | 后台最大等待数量 |
| `OCCCCAD_DB_QUEUE_TIMEOUT_MS` | 2000 | 仅限制调度等待；运行中的事务仍受请求和 PostgreSQL 超时约束 |

总并发为 1 时不能预留前台连接；所有名额均被前台占用时后台也会等待。队列不能提高数据库本身的吞吐能力，不应以放大队列应对持续过载。

HTTP phases_ms 的 `db-queue-wait`、`db-pool-wait`、`db-query`、`db-batch`、`db-transaction` 分别记录调度等待、底层连接获取、SQL、批次和事务生存期。同名值累计，事务与查询存在包含关系，不可简单相加。计时不记录 SQL 参数或凭据。内部监控 snapshot 的 `database` 字段提供 active/waiting/rejected、并发上限及连接占用。

## 验证

- `go test -race ./internal/database`
- 设置 `OCCCCAD_TEST_DATABASE_URL` 后运行 `go test ./internal/database -run TestPoolPostgres -count=1`：仅使用连接临时表，验证多批次失败整体回滚、取消不执行、结果流释放；不迁移或改动业务数据。
- 同一配置下 `go test ./internal/workspace -run TestHistoryCapabilityBatchMatchesSinglePostgres -count=1`：只读比较文档列表批量历史能力与单文档结果，需要已有管理员和文档。
