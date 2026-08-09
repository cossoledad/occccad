# occccad v0.0.6：协作与访问控制基础

> 状态：已实现并通过真实 PostgreSQL 权限隔离测试  
> 实现日期：2026-08-09

> 历史说明：本页记录 v0.0.6 当时的 Header 身份适配器；该入口和固定 Demo 用户已在 v0.0.7 删除，
> 当前账号与会话规则以 [v0.0.7 文档](occccad_v0.0.7_Distributed_Artifact_Pipeline.md)为准。

v0.0.6 在 v0.0.5 文档组织模型上建立统一 Request Principal、User/Team、ACL、共享和访问审计。
本版本的目标不是实现账号密码系统，而是确保每一次文档读取、建模命令和管理操作都有明确主体，
并且后端能够独立拒绝越权请求。

## 1. 范围与安全边界

实现范围：

- User、Team 和 Team Member 数据模型；
- Owner、Editor、Viewer 三档资源权限；
- Document 直接授权；
- Folder 授权沿祖先路径继承到子 Folder 和 Document；
- 用户与团队共享、修改授权和撤销授权；
- “与我共享”文档和共享根文件夹视图；
- 所有 API 请求绑定 Principal；
- 成功的写请求记录 Actor、Route、Resource、Request ID 和 Trace ID；
- 前端按照有效权限隐藏或禁用命令，但后端始终再次授权。

本版本使用 `X-OCCCCAD-User-ID` 作为**本地开发身份适配器**，并预置 Owner、Editor、Viewer 三个
本地用户。该 Header 便于内网 Demo 验收，不是生产认证协议。公网部署前必须在网关使用 OIDC/OAuth2
验证 Token，由可信中间件写入 Principal，并禁止客户端直接覆盖身份 Header。密码、注册、邮件邀请、
MFA 和外部身份提供商不属于 CAD 领域服务。

## 2. 权限模型

```text
OWNER  (30)  管理资源、共享、编辑、查看
EDITOR (20)  编辑、创建版本、移动、Trash、查看
VIEWER (10)  查看、历史、导出、复制到自己可写的位置
NONE    (0)  不出现在列表，资源详情返回 403
```

Document 的有效权限取以下来源的最大值：

1. `documents.owner_user_id`；
2. Document 直接 User/Team Grant；
3. 所在 Folder 以及全部祖先 Folder 的 User/Team Grant；
4. 祖先 Folder Owner 权限。

Folder 的有效权限使用同一祖先链规则。Grant 只允许 `VIEWER` 和 `EDITOR`；所有权不通过普通 Share
静默转移。未来如果支持 Ownership Transfer，必须有独立命令、目标用户确认和不可删审计事件。

授权采用“最大权限合并”，v0.0.6 不支持 Deny。这样可以避免用户同时通过 Team 和直接授权进入时，
因为顺序不同得到不确定结果。需要 Deny 时必须先定义其优先级、继承截断点和 UI 可解释性。

## 3. 数据模型

迁移 `0006_collaboration_acl.sql` 新增：

```text
users
teams
team_members
resource_grants
access_audit_events

documents.owner_user_id -> users.id
folders.owner_user_id   -> users.id
```

`resource_grants` 使用 `(resource_type, resource_id)` 表示 Document 或 Folder，并通过两个部分唯一索引
保证同一资源对同一 User/Team 只有一条直接授权。更新共享是 Upsert，不会积累相互冲突的权限行。

历史数据在迁移时归属本地默认 Owner；新资源必须显式写入当前 Principal。权限计算由 PostgreSQL
稳定函数完成，因此分页的 `total` 和结果集使用完全相同的可见性条件，不会通过数量泄露隐藏文档。

## 4. 请求授权边界

| API 类别 | 最低权限 |
|---|---|
| 文档列表、Folder 列表 | SQL 只返回有效权限 >= Viewer 的资源 |
| 文档详情、历史、STEP 导出 | Viewer |
| 建模命令、Undo/Redo、版本、导入、改名、移动、Trash | Editor |
| 读取或修改 Share | Owner |
| 在 Folder 内新建或移动进入 | 目标 Folder Editor |
| Product 插入引用 | Product Editor + 被引用 Document Viewer |

复制需要源 Document Viewer；如果调用者不能写源 Folder，前端把副本创建到个人根目录。直接调用 API
指定目标 Folder 时，目标 Folder 同样必须具备 Editor 权限。

## 5. HTTP API

```text
GET /api/session
GET /api/users
GET /api/teams
GET /api/teams/{teamId}/members

GET    /api/documents/{id}/shares
POST   /api/documents/{id}/shares
DELETE /api/documents/{id}/shares/{grantId}

GET    /api/folders/{id}/shares
POST   /api/folders/{id}/shares
DELETE /api/folders/{id}/shares/{grantId}

GET /api/documents?shared=true&allFolders=true
GET /api/folders?shared=true
GET /api/audit?documentId={id}&limit=50
```

共享写入请求：

```json
{"subjectType":"USER","subjectId":"...","role":"VIEWER"}
```

## 6. 前端行为

- Shell Bar 提供 Owner、Editor、Viewer 本地身份切换；
- 文档列表显示当前有效权限；
- Owner 可从文档行、Folder Card 或工作台打开共享面板；
- 共享面板支持用户、团队、Viewer/Editor 和撤销授权；
- 左侧增加“与我共享”；
- Viewer 仍可查看模型、历史和导出 STEP，但不能执行建模、Undo/Redo、版本或管理命令；
- 权限切换会清空旧身份的 Tab 和内存文档，避免 UI 暂存泄露；
- API 返回 403 时显示统一错误，不把前端禁用按钮当作安全边界。

## 7. 审计边界

所有成功的 POST、PATCH、DELETE API 请求在统一中间件记录：

- Actor User；
- HTTP Method 与匹配后的 Route Pattern；
- Document/Folder/Team Resource ID；
- Request ID 与 Trace ID；
- Path 和 Response Status。

审计与 CAD `commands` / `document_changes` 分层：前者回答“谁访问或管理了资源”，后者回答“模型状态
如何变化”。读取审计至少需要该 Document 的 Viewer 权限。后续合规版本应把审计写入追加型独立存储，
并定义保留期；v0.0.6 数据库管理员仍可删除审计行。

## 8. 验收结果

- 0001–0006 可连续迁移，已应用迁移不可修改且有 checksum；
- 未授权 Viewer 获取 Document 返回 403；
- Folder Viewer Grant 自动使子 Document 显示为 Viewer；
- Viewer 可打开但 PATCH 和 Share API 返回 403；
- Folder Grant 提升为 Editor 后，继承的 Document PATCH 成功；
- 撤销 Folder Grant 后，Document 再次返回 403；
- 写操作产生可按 Document 查询的审计事件；
- Go、TypeScript/Vite、C++/OCCT 测试通过。

## 9. v0.0.7 后续实现

后续范围已经由 [v0.0.7 账号管理、本地制品与持久任务](occccad_v0.0.7_Distributed_Artifact_Pipeline.md)
实现：旧 Header 身份切换被数据库会话替代，增加注册审批与管理后台，并以 PostgreSQL Job Queue、
本地 ArtifactStore 和异步 STEP 建立可恢复计算闭环。S3、Redis 和 OIDC 均保留为未来适配器。
