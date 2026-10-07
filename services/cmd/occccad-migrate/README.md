# occccad-migrate

occccad-migrate 是一次性 PostgreSQL/SQLite 迁移进程，适用于部署前置任务或人工升级。

## 行为

- 按后端从嵌入 Go 二进制的 `internal/database/postgres_migrations/*.sql` 或 `internal/database/sqlite_migrations/*.sql` 读取迁移；
- PostgreSQL 使用 Advisory Lock，SQLite 使用 BEGIN IMMEDIATE 防止并发迁移；
- 每个迁移在事务内执行；
- 在 `occccad.schema_migrations` 记录版本与 SHA-256 校验值；
- 两条迁移链必须同名、同编号、同数量；已应用 checksum 改变、旧文件缺失或应用记录缺号时拒绝继续；
- 成功后退出 0，失败后退出非 0。

当前两边各有一个 `0001_baseline.sql`，直接建立最终结构和必要管理员种子。旧开发迁移记录需要显式重建，默认迁移不会自动清库。

默认模式不启动 HTTP/gRPC 服务，也不迁移 ArtifactStore 中的大对象。

## 配置与运行

数据库变量与 [occccad-server](../occccad-server/README.md) 相同，优先读取 `OCCCCAD_DATABASE_URL`；SQLite 使用 `sqlite:/absolute/path/local.db`。两个数据库后端均支持统一 reset；SQLite 文件必须专用于本应用。

```bash
cd services
go run ./cmd/occccad-migrate
```

当前 `occccad-server` 与 `occccad-jobs` 启动时也会自动调用相同迁移器。本地开发因此通常不必单独执行；生产部署应先运行本进程，成功后再滚动应用进程，以便显式控制升级失败。

当前未发布开发阶段还提供受保护的破坏性模式。它按配置删除 PostgreSQL `occccad` schema 或 SQLite 专用数据库的全部用户表、视图与迁移记录，清空并重建 `OCCCCAD_DATA_DIR`；S3 模式先清空 `OCCCCAD_S3_BUCKET` 的对象、所有版本、删除标记和未完成分片（保留桶），然后执行全部当前迁移：

```bash
cd services
OCCCCAD_ALLOW_DEV_RESET=1 go run ./cmd/occccad-migrate --reset-development-data
```

通常应使用仓库入口 `invoke data.reset --yes` 或 `invoke run.app --reset-data`，避免手工设置保护变量。制品目录为空、指向文件系统根、仓库/服务工作目录或符号链接时命令会拒绝执行；数据库凭据不会写入日志。重置不是备份或生产迁移工具，执行前必须停止使用目标数据库/目录的 API、Jobs 等进程。SQLite 文件位于制品目录内时，会先关闭连接再清理目录，重新打开后执行迁移；位于目录外时原位重建表结构。

## 运维规则

1. 当前未发布迁移可以重写并通过空库重建验证；一旦建立发布基线，迁移只追加、不修改、不重排。
2. 当前阶段的破坏性 Schema 变更直接更新唯一基线并重建；发布后才使用 expand/migrate/contract，多版本兼容后再删除旧列。
3. 发布后的数据回填若可能超过部署超时，应作为可恢复后台任务，而不是长事务迁移。
4. 普通发布迁移执行前备份 PostgreSQL；开发重置本身不提供备份或回滚。

## 验证

```bash
cd services
go test ./internal/database -count=1
```

配置从根 `.env` 读取，已导出的环境变量优先。删除前报告 PostgreSQL 数据库/schema 或 SQLite 文件、本地目录和当前配置 S3 桶；只清理这一桶，不遍历其他桶。S3 清理失败时不继续删除本地目录或数据库表，但已删除对象无法回滚；清库与存储不是跨服务事务，可修复后重复执行。测试使用隔离桶，不清空实际开发桶。
