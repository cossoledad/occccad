# Artifact storage administration

业务文件经 `artifact.Store` 写 LOCAL 或 S3；几何计算使用本地临时文件。日志和调试 replay 仍是本地运维数据。API、Jobs、control 和本命令从根 `.env` 读取配置（已导出变量优先）。

S3 配置：`OCCCCAD_ARTIFACT_BACKEND=S3`，`OCCCCAD_S3_ENDPOINT=host:port`（无协议前缀），`OCCCCAD_S3_BUCKET`、`OCCCCAD_S3_REGION`、`OCCCCAD_S3_ACCESS_KEY`、`OCCCCAD_S3_SECRET_KEY`、`OCCCCAD_S3_SECURE`。HTTP MinIO 使用 `false`，HTTPS 使用 `true`。凭据不写文档或版本库。服务启动要求桶已存在且可访问，不自动创建基础设施。

从 `services/` 执行：

```sh
go run ./cmd/occccad-artifacts --init-bucket
go run ./cmd/occccad-artifacts --migrate-local
go run ./cmd/occccad-artifacts --verify-target
```

初始化仅在配置桶不存在时创建。迁移仅处理已登记的 LOCAL Artifact；数据库不再存储 bytea 几何或完整 Mesh/Naming。上传成功并校验后切换元数据，不改对象 ID 或历史 Revision。重复运行安全；中途失败可重新执行整个命令，已完成对象会跳过。不删除迁移源文件。旧实验 schema 不提供兼容迁移，升级本轮数据结构需要停止服务后执行 `invoke data.reset --yes`。正常服务使用配置后端写新对象；迁移期间可继续读旧 LOCAL 对象。迁移前让运行服务使用相同后端，避免旧配置进程继续写 LOCAL。

`OCCCCAD_DATA_DIR` 仍需是 API、Jobs 和受管理本机 Worker 共同可见的目录，供上传 hash spool、远端输入下载和几何输出使用。需要足够临时盘，S3 不消除 OCCT 和 GLB 组装的内存工作集。`OCCCCAD_EXCHANGE_MAX_BYTES` 默认 17179869184（16 GiB），API capabilities 和 Worker 使用同一配置。

对象存储成功后才发布数据库引用；失败不声明成功。每次分片上传独立，不续传、不自动重试。取消时对本次 upload ID 尝试清理，异常断电或服务持续不可达可能留下孤儿分片/对象；自动 GC 未交付。`invoke data.reset --yes` 在 S3 模式清空当前配置专用桶中的全部对象、历史版本、删除标记和未完成分片，保留桶；先停止写入进程，命令会报告清理目标。清理按每批最多 1000 个对象版本执行；列表、删除或分片撤销失败会报错，不能保证跨存储原子性，修复后可重新执行。

定向验证（真实 S3 测试只删除本次随机生成的测试对象）：

```sh
go test ./internal/artifact ./internal/geometry -run 'TestLocalStore|TestS3Spool|TestS3Multipart|TestArtifactStaging'
OCCCCAD_TEST_S3=1 OCCCCAD_TEST_S3_BYTES=1073741824 go test ./internal/artifact -run TestS3IntegrationStreaming -count=1 -v
OCCCCAD_TEST_S3=1 OCCCCAD_TEST_GEOMETRY_WORKER=/absolute/path/occccad_geometry_worker go test ./internal/geometry -run TestS3WorkerExchangeRoundTrip -count=1 -v
```

字节传输验收不等于 1 GiB STEP/BREP 已完成几何求值或浏览器显示验收。
