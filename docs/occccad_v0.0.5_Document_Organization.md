# occccad v0.0.5：文档组织与设计复用

> 状态：已实现并通过真实 PostgreSQL 端到端验证  
> 实现日期：2026-08-09
> 后续权限模型以 [v0.0.6](occccad_v0.0.6_Collaboration_and_Access_Control.md) 为准。

v0.0.5 在 v0.0.4 文档中心之上增加可扩展的组织层：层级 Folder、Breadcrumb、最近打开、
文档移动、Main Workspace 复制和服务端分页。这个版本不改变 Part Feature、Product Reference
或 Geometry Worker 协议，而是让已有 CAD 能力可以管理更大的设计集合。

## 1. 设计范围

本次采用 Onshape Documents Page 的几个稳定原则：Folder 是文档容器；搜索与筛选是文档中心
能力；打开文件夹通过 Breadcrumb 返回上层；复制 Workspace 创建独立文档；最近打开与修改时间
是不同维度。

v0.0.5 明确不实现：

- User/Team/Company、Share 与 ACL：需要统一身份和授权中间件；
- 缩略图异步渲染：需要制品任务队列与对象存储；
- 永久删除文档：需要引用影响分析和保留策略；
- Folder 权限继承：必须建立在 ACL 模型之上；
- 跨 Folder 拖拽：当前先用可验证的 Move Dialog，避免拖拽误操作。

## 2. 数据模型

```text
Folder (adjacency list)
├─ Folder
│  ├─ Part Document
│  └─ Product Document
└─ Document

Document
├─ folder_id                   organization only
├─ last_opened_at              navigation activity only
├─ copied_from_document_id     provenance only
├─ updated_at                  metadata/model change
└─ head_version_id             CAD workspace state
```

`folders.parent_id` 使用邻接表，路径通过递归 CTE 读取。当前操作不会移动 Folder 本身，因此没有
形成环的入口；未来加入 Folder Move 时必须在事务内用递归 CTE 拒绝把父目录移入其后代。

文档名称唯一性从全库 `(document_type,name)` 改为活动文档在
`(folder_id,document_type,lower(name))` 内唯一；不同 Folder 可以使用相同名称，Trash 不阻止创建
新文档。恢复或移动发生重名时返回业务验证错误。

## 3. 时间与版本边界

- 打开文档只更新 `last_opened_at`，不更新 `updated_at`；
- 创建、改名、移动、Trash 和 Restore 继续进入 Command/Change Audit；
- Folder CRUD 不产生 CAD Version；
- 文档移动只改变 `folder_id`，不改变 `head_version_id`；
- Copy 读取源文档当前 Main Head，创建一个新 Document 和 Version #1；
- Copy 不复制源历史线和命名 Version，否则新文档会伪装成同一设计身份；
- Part Copy 复用不可变 Geometry Artifact；
- Product Copy 同时复制当前 Snapshot 的 `product_instances` 投影，引用模式与引用 Version 保持不变；
- 后续修改源文档和副本互不影响，但 FOLLOW_HEAD 引用仍按每条 Product 引用边正常解析。

## 4. Folder 生命周期

v0.0.5 的 Folder 删除只允许空 Folder，随后物理删除 Folder 行。Folder 本身不承载 CAD 数据，
而拒绝删除非空 Folder 可以避免隐式地批量 Trash 文档。文档仍使用 v0.0.4 的软删除语义。

删除含活动文档、Trash 文档或子 Folder 的 Folder 均返回 `400`；用户必须先移动或恢复其中的文档，
并处理子 Folder。未来如需要 Onshape 风格整树 Trash，
应增加 Folder Trash Snapshot 与原路径恢复数据，而不是递归修改后无法准确恢复。

## 5. HTTP API

```text
GET    /api/folders?parentId={id}
POST   /api/folders
PATCH  /api/folders/{id}
DELETE /api/folders/{id}
GET    /api/folders/{id}/breadcrumbs

GET    /api/documents?folderId={id}&recent=true&sort=recent
       &limit=25&offset=0&allFolders=false
POST   /api/documents/{id}/move
POST   /api/documents/{id}/copy
```

文档列表响应统一为：

```json
{"documents": [], "total": 0, "limit": 25, "offset": 0}
```

服务端允许每页 1–200 条；前端默认 25 条。普通 Folder 浏览只读取当前 Folder；全文搜索、Recent
和 Trash 使用 `allFolders=true`。排序支持 `updated`、`name`、`created`、`recent`，排序列由服务端
白名单选择，不接受任意 SQL 表达式。

## 6. 前端交互

- 左侧增加“最近打开”；
- 文档浏览器顶部增加 Folder Breadcrumb；
- 当前 Folder 的子 Folder 以 Card 显示，双击进入；
- Folder 支持创建、编辑和删除空 Folder；
- 新建文档自动落入当前 Folder；
- 文档行增加复制、移动；
- Move Dialog 列出完整 Folder 路径，也可以移动到根目录；
- Copy Dialog 默认生成 `原名称 Copy`，成功后直接打开副本；
- 列表显示服务端总数，并提供上一页/下一页；
- Search 在当前 Folder 外执行全局搜索，避免用户必须记住文件位置。

## 7. 数据迁移

`0005_document_organization.sql`：

1. 创建 `folders` 与父子唯一索引；
2. 添加 `documents.folder_id`、`last_opened_at`、`copied_from_document_id`；
3. 删除旧的全局文档名称约束；
4. 增加 Folder 内活动文档名称唯一索引；
5. 增加 Folder、Recent 查询索引。

迁移只新增 `0005`，不修改已经应用的 0001–0004。

## 8. 验收结果

- 空数据库连续执行 0001–0005；
- 创建根 Folder 与子 Folder，Breadcrumb 返回两级路径；
- 在子 Folder 创建 Part/Product；
- Product 插入 Part 后复制，副本有一个独立 Version 和完整 Instance 投影；
- 副本移动到根目录不改变其 Version；
- 打开 Part 后出现在 Recent；
- `limit=1` 返回正确 `total`；
- 删除非空 Folder 返回 400；
- Go、TypeScript/Vite 和 C++/OCCT 测试通过。

## 9. v0.0.6 建议

下一阶段建议先建立 User/Team/ACL 和 Request Principal，再实现 Share、Folder 权限继承和操作审计。
缩略图应作为 Artifact 派生任务写入对象存储；不要在文档列表请求中同步启动 Three.js 或 OCCT
渲染。CAD 领域继续以 Persistent Topology Naming 和 Face Support 为最高优先级。
