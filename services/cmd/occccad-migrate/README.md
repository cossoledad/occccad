# occccad-migrate

occccad-migrate 是一次性 PostgreSQL Schema 迁移进程，适用于部署前置任务或人工升级。

## 行为

- 从嵌入 Go 二进制的 `services/internal/database/migrations/*.sql` 读取迁移；
- 使用 PostgreSQL Advisory Lock 防止并发迁移；
- 每个迁移在事务内执行；
- 在 `occccad.schema_migrations` 记录版本与 SHA-256 校验值；
- 已执行文件内容发生变化时拒绝继续，防止静默篡改历史；
- 成功后退出 0，失败后退出非 0。

它不启动 HTTP/gRPC 服务，也不迁移 ArtifactStore 中的大对象。

## 配置与运行

数据库变量与 [occccad-server](../occccad-server/README.md) 相同，优先读取 `OCCCCAD_DATABASE_URL`。

```bash
cd services
go run ./cmd/occccad-migrate
```

当前 `occccad-server` 与 `occccad-jobs` 启动时也会自动调用相同迁移器。本地开发因此通常不必单独执行；生产部署应先运行本进程，成功后再滚动应用进程，以便显式控制升级失败。

## 运维规则

1. 已发布迁移只追加、不修改、不重排。
2. 破坏性 Schema 变更使用 expand/migrate/contract，多版本兼容后再删除旧列。
3. 数据回填若可能超过部署超时，应作为可恢复后台任务，而不是长事务迁移。
4. 执行前备份 PostgreSQL；该进程没有自动回滚到旧 Schema 的能力。

## 验证

```bash
cd services
go test ./internal/database/... ./...
```
