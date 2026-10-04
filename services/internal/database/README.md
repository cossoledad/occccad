# 数据库访问与调度

应用服务依赖 `database.DB`、`Tx`、`Rows`、`Row`、`Result` 和 `Batch`；接口不暴露 pgx、database/sql 或 SQLite 类型。`database.Open` 按连接串选择 PostgreSQL 或 SQLite 实现，`Pool` 只负责有界调度。迁移、连接统计、错误转换与事务实现归后端所有。

## 选择模式

默认沿用 `OCCCCAD_POSTGRES_*`；显式 `OCCCCAD_DATABASE_URL` 优先：

```dotenv
# PostgreSQL（继续支持原有 pgx 连接参数）
OCCCCAD_DATABASE_URL=postgres://user:password@localhost:5432/occccad

# SQLite Local Mode；二选一，必须使用绝对路径
# OCCCCAD_DATABASE_URL=sqlite:/absolute/path/occccad-local.db
```

SQLite 使用 modernc.org/sqlite（纯 Go、BSD-3-Clause），不需要数据库服务器或 CGO。API、Jobs、Control 启动拓扑不变，API 与 Jobs 使用同一个本机数据库文件及 ArtifactStore 配置。不要把 SQLite 文件放在网络共享文件系统；Local Mode 是本机持久化模式，不是浏览器离线同步，也不自动复制或转换现有 PostgreSQL 数据。切换连接串选择另一份独立数据。

SQLite 每进程一个物理连接、一个执行名额；每次连接启用 WAL、foreign_keys、synchronous=FULL 和 2 秒 busy_timeout。写事务使用 BEGIN IMMEDIATE，在读取待修改状态前取得写入资格；跨进程竞争由 SQLite 文件锁协调。等待超时或锁忙通过 `ErrBusy` 表达，业务写入不自动重试。几何求值仍在事务外，因此多个计算任务可以并发，而最终写事务串行。

## 执行与一致性

- 普通 SQL、结果流或完整事务占用一个执行名额。等待队列有界且可取消；满时返回 `ErrBusy`，超时返回可识别的 admission 错误。API 使用 503/DATABASE_BUSY，WebSocket 使用 retryable 错误。
- `Begin/BeginTx` 从 BEGIN 到 COMMIT/ROLLBACK 持有名额；事务内不重新进入 Pool 排队。Workspace 锁/CAS、Revision、ChangeSet、Outbox 原子边界保持不变。
- `Query` 必须遍历完成或 Close，`QueryRow` 必须 Scan，`SendBatch` 必须 Close，事务必须结束；不要持有未消费的 Rows 再发起另一个 Pool 操作。
- PostgreSQL 批次保留 pgx pipeline；SQLite 顺序执行同一事务内的语句。`ExecBatch` 最多每组 128 条，无中间提交。语句失败后不能提交前面的成功写入；savepoint 回滚可恢复外层事务。
- `ErrNoRows`、`ErrUniqueViolation`、`ErrBusy` 是供应商无关的错误。SQL 参数不进入调度日志。
- 登录的会话与账号状态，以及 Jobs 状态、attempt 与 Outbox 均通过显式事务写入。PostgreSQL 领取保留 FOR UPDATE SKIP LOCKED，SQLite 通过事务的写锁保证唯一领取。
- `Background(ctx)` 为监控与 Outbox 轮询提供独立等待预算；各进程分别限流。

## SQL 与扩展边界

当前服务保留已有 SQL 查询词汇；PostgreSQL 直接执行，SQLite 方言层按 token 处理 schema、占位符、显式类型、JSON、聚合、时间和锁。字符串字面量不会被文本替换。ACL 的数据库函数在 SQLite 中展开为同一快照内的关联查询，不回调数据库池。复杂多表写入使用 Go 显式事务，不模拟任意 PostgreSQL writable CTE。

这不是通用 SQL 兼容引擎。新增 SQL 必须同时通过两种后端的查询和业务合同测试；不支持的表达式应显式扩展方言或改成共用事务操作。新增 MySQL 等后端需实现内部 backend、SQL 方言、迁移和同等合同验证，并在 Open 注册路由；应用服务接口无需更换。当前未提供 MySQL 实现。

## 迁移

PostgreSQL 使用 `migrations/` 与连接级 advisory lock；SQLite 使用 `sqlite_migrations/` 和 BEGIN IMMEDIATE。两者记录 SHA-256 校验值，拒绝已应用迁移被修改。SQLite 的独立空库基线覆盖 PostgreSQL 0001–0038 的当前表、外键、唯一约束、检查约束与 UI 目录；JSON、UUID、布尔值有显式检查。以后数据库变更必须同步两条迁移链。

SQLite 迁移不读取 PostgreSQL，也不需要联网。`invoke data.reset --yes` 与 `invoke run.app --reset-data` 按环境配置选择后端：PostgreSQL 删除 `occccad` schema，SQLite 在事务内删除当前专用数据库的全部用户表、视图及迁移记录，再执行迁移恢复表结构与种子数据。SQLite 文件应专用于本应用；文件位于制品目录内时，清理目录前先关闭连接，再重新打开数据库迁移。执行前须停止 API、Jobs 等占用进程。

## 配置

| 配置 | 默认值 | 含义 |
|---|---|---|
| PostgreSQL `pool_max_conns` | pgx 默认值 | 每进程物理连接上限 |
| `OCCCCAD_DB_CONCURRENCY` | PG: pool_max_conns；SQLite: 1 | SQLite 必须省略或设为 1 |
| `OCCCCAD_DB_QUEUE_CAPACITY` | 64 | 前台等待容量，0 表示不排队 |
| `OCCCCAD_DB_BACKGROUND_CONCURRENCY` | max(1, CONCURRENCY/4) | 不得超过总并发 |
| `OCCCCAD_DB_BACKGROUND_QUEUE_CAPACITY` | 16 | 后台等待容量 |
| `OCCCCAD_DB_QUEUE_TIMEOUT_MS` | 2000 | 调度等待时限 |

总并发为 1 时无法预留前台连接。HTTP phases_ms 记录 db-queue-wait、db-query、db-transaction 等；pgx 另有 db-pool-wait/db-batch。事务计时包含内部查询，不可简单相加。监控 snapshot 提供 active/waiting/rejected 与连接占用。

## 验证

- `go test -race ./internal/database ./internal/jobs ./internal/authn ./internal/access ./internal/workspace`
- SQLite 测试使用临时文件：空库迁移/重开、外键、唯一性、批量失败回滚、取消、savepoint、独立连接池与实际子进程竞争、Jobs/登录故障注入，以及真实文档命令、Undo/Redo、ACL 和冷读。
- `TestSQLiteApplicationQueriesCompile` 在实际 schema 上编译静态业务查询；动态拼接与运行语义由业务测试补充，不能以查询编译代替功能验收。
- 设置 `OCCCCAD_TEST_SCHEMA_PARITY_URL` 指向已迁移的 PostgreSQL 后运行 `go test ./internal/database -run TestSQLitePostgresSchemaAndCatalogParity`：只读比较 36 张业务表的列与 UI 目录，SQLite 侧使用临时空文件。
- 设置 `OCCCCAD_TEST_DATABASE_URL` 后运行 `go test ./internal/database -run TestPoolPostgres -count=1`：只使用连接临时表，不迁移业务数据。
- 配置独立 SQLite 文件与 `OCCCCAD_TEST_GEOMETRY_WORKER` 后，可运行 `go test ./internal/control -run TestSolidBooleanLifecycleThroughRouter -count=1`，验证真实 Router/Worker/ArtifactStore 与持久化链。
