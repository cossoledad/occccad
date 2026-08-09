# occccad v0.0.4：文档中心与专业 CAD 工作台

> 状态：已实现，等待交互验收  
> 实现日期：2026-08-09

> 文档中心组织、Folder、Recent、Copy 和分页的当前行为以
> [v0.0.5](occccad_v0.0.5_Document_Organization.md) 为准。

v0.0.4 把 Demo03 的功能垂直切片整理为日常可使用的产品入口。首页不再自动打开数据库中的
第一个 Part，也不再把“文档库”作为特征树下方的附属区域；用户先在文档中心管理设计数据，
然后显式打开某个文档进入 CAD 工作台。

## 1. 设计依据与本次边界

Onshape 的 Documents Page 是登录后的主要入口，提供筛选、搜索、列表/网格、重命名和 Trash；
打开文档后，Logo 返回 Documents Page，Document Toolbar 显示当前文档及 Workspace；Part Studio、
Assembly 等编辑上下文通过 Tab 切换。

occccad v0.0.4 采用这些信息架构原则，但不复制 Onshape 的账号、权限和企业功能：

- 首页是 Document Center，工作台是独立页面状态；
- 单击选择文档，双击或按 Enter 打开，避免单击即切换建模上下文；
- Part 与 Product 是当前两种文档类型；
- 删除首先进入 Trash，恢复不会改变 CAD 模型和 Version；
- 工作台顶部显示文档上下文，Tab 保存本次浏览器会话中打开的文档；
- Toolbar 按“创建、交换、装配、历史、视图”划分，不把所有命令铺成一组；
- 左栏只呈现当前文档的 Feature/Instance Tree，文档库不再混入结构树；
- 右栏使用真正的“属性/版本历史”互斥 Tab，减少同时显示产生的视觉噪声。

本次不实现 Folder、Label、Share/ACL、永久删除、缩略图服务、最近打开记录和多人 Presence。
这些能力需要身份与权限模型，不能只增加几个前端按钮。

## 2. 页面与路由

```text
GET /
  Document Center
  ├─ Active / Parts / Products / Trash filters
  ├─ Search + type filter
  ├─ Create / Edit / Move to Trash / Restore
  └─ double click -> /documents/{documentId}

GET /documents/{documentId}
  CAD Workbench
  ├─ grouped command toolbar
  ├─ session document tabs
  ├─ feature or instance tree
  ├─ graphics viewport
  └─ properties / version history inspector
```

前端使用 History API，浏览器前进/后退可以在文档中心与工作台之间导航；Go 静态文件 Handler
对无扩展名路径回退到 `index.html`，因此直接刷新 `/documents/{id}` 仍能恢复工作台。打开的 Tab ID
写入 `sessionStorage`，关闭浏览器会话后不制造服务器端用户状态。

## 3. 文档领域模型

`documents` 在 v0.0.4 增加：

| 字段 | 含义 |
|---|---|
| `description` | 最长 500 字符的文档说明，不参与 GeometryKey |
| `deleted_at` | `NULL` 表示活动文档，非空表示在 Trash |

名称和说明属于 Document Metadata，不属于 Part/Product 的参数化模型：

- 修改元数据会更新 `documents.updated_at`；
- 写入 `commands` 和追加式 `document_changes`，形成可追踪的业务操作；
- 不创建新的 `document_versions`，因为几何与 Feature/Instance Snapshot 没有变化；
- 不改变 `head_version_id`，也不进入 CAD Undo/Redo 游标；
- Trash 仍保留 Version、Command、Artifact 和引用边；恢复只是清除 `deleted_at`；
- Trash 文档不能直接打开或修改，现存 Product 引用仍可解析，避免软删除导致装配瞬间损坏；
- 永久删除尚未开放，未来必须先做引用影响分析、保留策略和权限检查。

这一区分非常重要：CAD Undo/Redo 只处理建模命令，文档中心的改名或 Trash 生命周期不能冒充
一个几何版本。

## 4. HTTP API

```text
GET    /api/documents?scope=active|trash|all&q=...&type=PART|PRODUCT
POST   /api/documents
GET    /api/documents/{id}
PATCH  /api/documents/{id}
DELETE /api/documents/{id}
POST   /api/documents/{id}/restore
```

- 列表查询最多返回 500 条；当前规模下足够，进入多人/大数据阶段前改为游标分页；
- `q` 在名称和说明中做大小写不敏感搜索；
- `DELETE` 是幂等边界明确的软删除入口，重复删除返回验证错误；
- CORS 允许 `PATCH` 和 `DELETE`；所有修改继续带 Request ID 和 Trace Context；
- 元数据写操作与 Command/Change Audit 在同一个 PostgreSQL 事务提交。

## 5. 前端组件边界

当前仍采用 TypeScript DOM + Three.js，避免在几何交互稳定前引入大型 UI Runtime。v0.0.4 已把
页面按可复用组件语义组织，而不是继续堆叠 Demo 按钮：

- `Shellbar`：品牌、页面返回、文档 Breadcrumb、版本和健康状态；
- `LibraryNavigation`：活动文档、类型和回收站过滤；
- `DocumentBrowser`：搜索、类型过滤、可选择表格和行级操作；
- `DocumentDialog`：创建/编辑的统一字段与验证规则；
- `CommandToolbar`：按工作流分组并由 Selection/Mode 驱动 Enabled State；
- `DocumentTabs`：打开、激活、关闭和会话恢复；
- `FeatureTree`：Part 的顺序 Feature 或 Product 的 Instance；
- `InspectorTabs`：属性与历史互斥显示；
- `CadView`：只负责 WebGL、选择、草图和相机，不承担文档列表 UI。

下一轮如需要快捷菜单、虚拟滚动、拖拽布局和无障碍组件，可引入 Lit/Web Components 或经过评估的
Headless UI；不建议仅为了按钮外观迁移到一个重量级 SPA 框架。

## 6. 数据迁移与部署

新增迁移 `0004_document_management.sql`，只添加字段和索引，不修改已经固定的 0001–0003。
Server 启动或 `go run ./cmd/occccad-migrate` 自动应用；Advisory Lock、事务与 Checksum 规则不变。

升级步骤：

```bash
invoke web.build
cd services && go run ./cmd/occccad-migrate
invoke run.worker
invoke run.server
```

旧文档的 `description` 自动为空字符串，`deleted_at` 为 `NULL`，不会改变现有 Head 或几何制品。

## 7. 清理内容

v0.0.4 删除了已经被 Workspace/Feature Chain 完全替代的运行时代码：

- `/api/demo` 与 `/api/demo/seed`；
- `services/internal/demo` 固定 100×60×40 盒体 Seed Service 及其测试；
- 首页自动打开第一个 Part；
- 工作台左栏中的第二套“文档库”；
- `Demo 01` 出现在任务说明、Worker 协议注释和 GLB Generator 名称中的运行时标记。

旧 Demo 文档不删除，它们记录架构演进，但运行手册明确标记 Seed API 已退役。旧 Snapshot 的
矩形兼容字段继续保留，否则清理代码会破坏既有数据库数据。

## 8. v0.0.4 验收标准

1. `/` 默认进入文档中心，不自动打开任何 Part；
2. 可以创建 Part/Product，并填写名称与说明；
3. 搜索和类型筛选工作，单击只选择，双击/Enter 打开；
4. 修改名称/说明后列表、Tab、Breadcrumb 同步；
5. 删除进入 Trash，活动列表消失，Restore 后完整恢复；
6. 被 Trash 文档的历史和 Artifact 不被删除，已有 Product 引用仍能解析；
7. 工作台可以打开、切换和关闭多个文档 Tab；
8. 浏览器前进/后退及直接刷新文档 URL 工作；
9. Feature Tree、多草图/多拉伸、STEP、Product 实例、Undo/Redo 均无回归；
10. 空数据库能连续自动执行 0001–0004。

## 9. v0.0.5 建议

后续优先级建议为：Folder/最近打开与缩略图任务、用户/团队/ACL、文档复制、分页与全文搜索，
随后再做可持久化面板布局和命令面板。CAD 领域侧应并行推进 Face Support/Persistent Naming，
否则继续增加高级 Feature 会建立在不稳定的面引用上。
