# occccad v0.0.7：账号管理、本地制品与持久任务

> 状态：已实现并完成独立环境验收  
> 实现日期：2026-08-09  
> 前置版本：[v0.0.6 协作与访问控制](occccad_v0.0.6_Collaboration_and_Access_Control.md)

v0.0.7 结束“可切换 Demo 用户”的开发模式，建立可长期演进的账号、制品和后台任务边界。
本版本不部署 Redis、S3 或 MinIO：PostgreSQL 保存业务状态和任务状态，API 启动目录下的
`data/` 保存大字节制品。未来对象存储通过 `ArtifactStore` 接口替换，不改变 Document、Job
或 Geometry 的领域语义。

## 1. 交付范围

已交付：

- 唯一初始管理员，由启动配置引导并在数据库中保存 bcrypt 哈希；
- 用户自助注册，初始状态为 `PENDING`，不能直接登录；
- 管理后台支持查询、添加、审批、启用/禁用、平台角色分配和密码重置；
- `ADMIN/MEMBER` 平台角色与文档 `OWNER/EDITOR/VIEWER` 权限分层；
- 数据库会话、HttpOnly Cookie、SameSite Cookie 和写请求 CSRF 校验；
- 删除客户端可伪造的 `X-OCCCCAD-User-ID` 身份入口和三用户切换控件；
- 历史 Demo 用户和 Demo Team 清理，原有资源所有权转移给初始管理员；
- 本地内容寻址制品库、SHA-256 去重、原子写入和路径穿越防护；
- PostgreSQL Job Queue、`SKIP LOCKED` Claim、Lease、Retry 和 Attempt 记录；
- 独立 `occccad-jobs` 进程；
- STEP Import/Export 异步执行，浏览器轮询任务，导出完成后下载制品；
- 新几何同时写入 PostgreSQL 与本地 B-Rep/GLB 制品，状态为 `DUAL`；
- Part Thumbnail 由持久任务生成 SVG 制品，并显示在文档中心；
- 迁移 `0007_accounts_and_sessions.sql` 与 `0008_local_artifacts_and_jobs.sql`。

预留但本版本不启用：

- `ArtifactStore` 的 S3-compatible 实现；
- Redis 通知、缓存或 Worker Registry；
- OIDC/企业 SSO Identity Provider 适配器；
- 任务取消 UI、管理员任务诊断页；
- B-Rep/GLB 从 PostgreSQL `bytea` 到文件制品的在线回填。

交互式 Sketch、Pad、Undo、Redo 仍为同步命令。只有文件交换、预览、回填等可恢复的长任务进入
Job Queue。

## 2. 运行架构

```text
Browser
  | password session cookie + CSRF
  v
Go API / Control Plane
  |-- PostgreSQL: account / ACL / document / version / job / artifact metadata
  |-- ./data: immutable local artifact bytes
  `-- enqueue durable jobs
             |
             v
       occccad-jobs (one or more)
             |
             v
       Geometry Worker (OCCT/gRPC)
```

边界约束：

- PostgreSQL 是账号、权限、文档、版本、任务和对象引用的唯一状态真相；
- 文件系统只保存可校验的不可变字节，不保存 Document Head；
- API 完成 ACL 判断后才创建 Job；Job Worker 只执行已固化输入；
- Geometry Worker 不连接 PostgreSQL，也不接收浏览器身份；
- Redis 未来只能加速唤醒或缓存，不能成为 Job 唯一记录；
- S3 后端必须维持相同的相对 Object Key 和 SHA-256 语义。

## 3. 账号与管理后台

### 3.1 生命周期

```text
注册 -> PENDING --管理员审批--> ACTIVE --管理员禁用--> DISABLED
                         ^                         |
                         `--------重新启用---------'
```

- `PENDING` 与 `DISABLED` 均不能创建会话；
- 管理员创建的账号可以直接为 `ACTIVE`；开发阶段不强制首次登录修改密码；
- 管理员重置密码会撤销目标账号的全部现有会话；
- 禁用账号会立即撤销现有会话；
- 当前管理员不能禁用自己或移除自己的管理员角色；
- 系统禁止移除最后一个有效管理员。

平台角色只控制系统管理能力：

| 平台角色 | 能力 |
|---|---|
| `ADMIN` | 用户审批、账号维护、角色分配、全局统计 |
| `MEMBER` | 使用文档中心和 CAD 工作台 |

文档权限继续使用 `OWNER/EDITOR/VIEWER`。成为平台管理员不会自动改变某个文档的 ACL；只有
管理接口和任务诊断可以使用平台管理员越权策略。

### 3.2 唯一初始管理员

首次启动前必须配置：

```dotenv
OCCCCAD_ADMIN_EMAIL=admin@occccad.local
OCCCCAD_ADMIN_DISPLAY_NAME=Administrator
OCCCCAD_ADMIN_PASSWORD=replace-with-a-strong-password
```

固定管理员记录由迁移保留，启动时只在 `password_hash` 为空时使用配置密码生成哈希，并要求首次
登录立即改密。后续修改环境变量不会覆盖已经设置的密码，避免每次重启重置管理员凭据。

### 3.3 会话安全边界

- 密码使用 bcrypt；明文不写数据库和日志；
- 连续 5 次失败后锁定 15 分钟；
- 会话 Token 和 CSRF Token 在数据库中只保存 SHA-256 摘要；
- 会话 Cookie 为 HttpOnly、SameSite=Lax；CSRF Cookie 可由前端读取并复制到
  `X-CSRF-Token`；
- 除 GET/HEAD 外的已认证 API 必须同时通过 Cookie 与 CSRF 校验；
- 生产 HTTPS 部署设置 `OCCCCAD_SECURE_COOKIES=true`；
- API 完全忽略旧 `X-OCCCCAD-User-ID`，不能由浏览器指定 Principal。

## 4. 本地 ArtifactStore

默认配置：

```dotenv
OCCCCAD_DATA_DIR=./data
```

相对路径按进程启动目录解析，因此执行 `invoke run.server` 与 `invoke run.jobs` 时，两者都在
`services/` 启动并共享 `services/data/`。正式部署时应给两个进程配置同一个绝对路径或共享挂载。

```text
data/
└── artifacts/
    ├── .staging/
    └── sha256/
        └── {hash[0:2]}/{sha256}/
            ├── source.step
            ├── export.step
            ├── shape.brep
            ├── mesh.glb
            └── preview.svg
```

写入流程：

1. 写到 `.staging` 临时文件，同时计算 SHA-256；
2. `fsync` 并关闭临时文件；
3. 根据摘要生成相对 Object Key；
4. 以不覆盖现有文件的原子硬链接发布到最终路径；
5. 若相同对象已存在，校验大小并复用；
6. 在 `artifact_objects` 中 Upsert 元数据并标记 `READY`。

业务层只接收相对 Object Key，`LocalStore` 拒绝绝对路径、反斜线和 `..` 越界。未来 S3 实现
`Store.Put/Open/Delete` 后可以替换本地后端；领域服务不拼接 Bucket URL，也不感知宿主机路径。

## 5. 持久任务

状态机：

```text
QUEUED -> RUNNING -> SUCCEEDED
             |
             +-> RETRY_WAIT -> RUNNING
             `-> FAILED
```

`jobs` 保存类型、文档、提交版本、请求人、输入/结果对象、幂等键、租约、重试次数、进度和错误。
`job_attempts` 保存每一次 Worker 尝试。

Worker 使用短事务和 `FOR UPDATE SKIP LOCKED` 原子领取一条任务。领取后增加 Attempt 并设置
`lease_owner/lease_expires_at`；崩溃导致租约过期时，其他 Worker 可重新领取。失败采用有限退避，
超过 `max_attempts` 进入 `FAILED`。

STEP Import/Export 固化提交时的 `version_id`。执行前若 Document Head 已变化，任务失败而不是把旧
结果覆盖到新设计上。该规则使异步任务不会静默跨越文档更新边界。

API：

```text
POST /api/documents/{id}/import-step   -> 202 Job
POST /api/documents/{id}/export-step   -> 202 Job
GET  /api/jobs/{id}                    -> Job state
GET  /api/jobs/{id}/download           -> completed export bytes
GET  /api/jobs                         -> current user's recent jobs
```

Job 只允许提交者、具备文档查看权限的用户或平台管理员查询；下载还必须处于 `SUCCEEDED` 且存在
`result_object_id`。

## 6. 数据库迁移与兼容

`0007`：

- 扩展 `users` 的账号状态、平台角色、密码与审批字段；
- 把 Demo Editor/Viewer 拥有的 Folder/Document 转移给初始管理员；
- 删除固定 Demo 用户、Demo Team 及其级联授权；
- 创建 `user_sessions` 和 `account_audit_events`。

`0008`：

- 创建 `artifact_objects`、`jobs`、`job_attempts`、`document_previews`；
- 给 `geometry_artifacts` 增加对象引用与 `DATABASE/DUAL/OBJECT` 状态。

现有 B-Rep、GLB 和 `mesh_json` 不删除。v0.0.7 对新求值几何进行数据库与本地制品双写并标记
`DUAL`；升级前的几何仍为 `DATABASE`。历史制品回填将在后续以 `ARTIFACT_BACKFILL` Job 完成，
最终读取切换仍经过 `DATABASE -> DUAL -> OBJECT` 的可回滚过程。

## 7. 配置与启动

`.env` 不再包含 Redis 或 S3 参数。开发启动顺序：

```bash
invoke web.build
invoke run.worker
invoke run.jobs
invoke run.server
```

生产部署至少需要：共享的持久 `OCCCCAD_DATA_DIR`、HTTPS、Secure Cookie、强管理员密码、数据库备份
和独立运行的 API/Job Worker。多个 `occccad-jobs` 可以共享 PostgreSQL 与数据目录横向扩展。

## 8. 验收结果

2026-08-09 使用独立 PostgreSQL 数据库和独立 API 端口完成：

- 从空库执行全部 8 份迁移；
- 管理员引导、登录、Session 查询；
- 新用户注册为 `PENDING`，管理接口可查询；
- 无 CSRF 的写请求返回 403；
- 创建 Part、Rectangle Sketch、Pad；
- 新 Pad 的 B-Rep/GLB 同时落入内容寻址文件并关联 `geometry_artifacts`；
- Thumbnail Job 进入 `SUCCEEDED`，`document_previews` 为 `READY`，授权接口返回 779 字节 SVG；
- 创建 STEP Export Job，独立 Worker Claim 并执行；
- `data/artifacts/sha256/.../export.step` 生成 15,444 字节制品；
- Job 进入 `SUCCEEDED` 并通过授权下载接口返回相同大小文件；
- Go 全量测试与 TypeScript/Vite 生产构建通过。

验收使用的数据库、进程和文件目录均为临时资源，未修改正在运行的现有服务。

## 9. 后续边界

建议 v0.0.8 继续完成：

1. B-Rep/GLB `DATABASE -> DUAL -> OBJECT` 历史回填与读回退；
2. 任务中心、取消、手动重试和管理员诊断 UI；
3. Team 创建/成员管理与邀请；
4. 当单机/共享目录成为真实容量或可用性瓶颈后，再增加 S3 Store；
5. 只有出现明确的通知延迟或缓存压力后，再评估 Redis。
