# TREE-03：Product 编辑会话、标签与菜单

> TREE-03 实现基线：`ebbf37a9eb2ca1d3d9777624ec1a6967b753e8a5`。2026-09-30 文档核对基线：`main`，HEAD 同为该提交。结构树领域身份与显隐见 [TREE-02](tree02-model-display.md)，长期 Product 上下文合同见[目标架构](../target/product-context.md)。

## 编辑会话

一个 Workbench 路由只拥有一个宿主文档。Product 路由的宿主始终是根 Product；编辑子 Part/Product 不改变 URL、活动标签或根场景。前端 `EditSession` 一次保存宿主 DocumentId、唯一编辑目标（DocumentId、类型和完整 InstancePath）、宿主/目标 Revision 基线、root snapshot digest、会话工作 Body 及 activation generation。工作台、属性、历史、Undo/Redo 和普通 Part/Product 命令读取编辑目标；Instance 引用策略、移动和替换仍按 InstancePath 最后一段的 owner Product 路由。

Product 内只有 occurrence 上下文中的唯一编辑目标，取消定义编辑/上下文编辑双模式切换及其居中提示。双击 Part Instance 与其下的 Part 根解析为同一上下文目标；双击 Product Instance 与其下的 Product 根解析为同一嵌套 Product 目标；双击根 Product 返回根装配编辑。Instance 只是快捷入口，绿色文档编辑态只标记类型、DocumentId 和完整 occurrence 路径均匹配的 Part/Product 根；同一共享 Part 的其他 occurrence、Instance 行和工作 Body 不复用该标记。Body 激活只更新会话工作 Body。

Occurrence 激活先用 Product Design Session 校验 root Revision、完整路径、目标身份和 resolved Revision，再读取目标文档。空 Part 不依赖 `ResolvedInstance` 或任意 Body/Artifact，也可以激活。路径中存在 `PINNED` 引用时保留冻结快照供 Product 查看，但拒绝进入修改并要求先显式恢复跟随；非固定引用若 resolved Revision 与目标 HEAD 不一致也拒绝激活，等待 UpdatePlan 收敛。验证成功后才结束旧工具、取消 Preview、清理选择，并一次提交新会话；activation generation 丢弃旧查询和迟到回调。失败保留原有效会话。

普通 Sketch、Feature、Body、Parameter 与 Part Publication 修改共享 Part 定义，不复制 Part，也不创建 occurrence 专属文档。Occurrence 只提供 placement、外部引用、权限和快照验证。显式 ContextBinding/ContextVariant 继续使用原有合同。独立打开 Part 会建立另一个 Part 宿主工作空间。

## 标签与读取边界

`GET /api/documents/{id}`、realtime snapshot、依赖加载、自动更新和 Query refetch 都是资源读取，只更新文档缓存及已存在标签的摘要。它们不再注册、打开、激活或重排标签。用户从路由、文档中心、新建结果或“在新标签页中打开”进入时，才调用 `POST /api/open-documents/{id}` 执行显式打开生命周期。

打开列表按 `user + X-OCCCCAD-Workspace-ID` 隔离；浏览器在 `sessionStorage` 为当前窗口保存 workspace id 和标签顺序。服务端 `Open` 对已有条目原位更新，对新条目追加；`Update` 只更新已经打开的摘要。前端同样只对显式打开追加条目，普通摘要更新不会重新加入已关闭标签。重复挂载和重复打开幂等。

进入 Product 工作空间时，当前宿主 Product 在显示顺序中固定首位；其他标签保持窗口内保存的相对顺序。宿主编辑期间，依赖广播、Part 提交、Product 自动更新和快照恢复不会改变该顺序。关闭活动标签按当时可见顺序选择相邻标签，不依赖后台列表返回顺序。关闭独立 Part 标签只结束该标签生命周期；Product 自己的依赖读取和 realtime 订阅继续存在。

## 更新失败与右键菜单

非 PINNED Product 引用继续通过服务端 `followedProductIds` 与 UpdatePlan 按叶到根自动接受。失败保留已提交的子 Part 和原合法 Product 快照，不切换宿主或编辑目标；工作台显示诊断和显式重试入口。PINNED 路径不进入自动接受。

结构树右键菜单的每个可操作项都使用公共 `ContextMenuIcon` 槽。有图标时在固定 16 px 槽内显示；无图标时保留 `aria-hidden` 空槽。禁用、危险、多选和普通操作共用相同文字起点；空槽不产生可访问文本。分隔线或纯标题无需图标槽。点击、键盘和命令行为仍由 Ant Design Menu 负责。

## 验证与限制

本轮交互整改已完成。维护者明确反馈：“现在已经进行了一轮修改，并且验证通过。”据此记录为**维护者反馈 TREE-03 本轮使用验证通过**，是本轮整改的维护者验收依据，不是本次 Agent 重新运行浏览器或测试所得，也不是下列功能逐项单独验收的记录。

实现基线及本次文档核对 HEAD 均为 `ebbf37a9eb2ca1d3d9777624ec1a6967b753e8a5`；反馈未提供人工测试日期、准确测试版本、逐项场景、截图、trace 或性能数据，因此不将代码核对基线冒充准确人工测试版本。后续 HEAD 的未知改动不自动继承此反馈。

代码核对入口：`edit-session.ts` 与 `workbench.tsx`（宿主/目标及激活）、`specification-tree.tsx`（根节点编辑态及菜单）、`open-document-tab.ts`、`document-tab-order.ts` 与 `document-tabs.tsx`（显式打开/稳定顺序）、`services/internal/api/open_documents.go`（读取与窗口作用域）、`ContextMenuIcon` 与样式（固定图标槽）。这些支持上文完成事实；测试入口存在与测试已执行分别记录。

仓库已有定向测试入口：

- Go `TestOpenDocumentRegistryPreservesOrderAndWorkspaceScope`：窗口隔离、原位摘要更新、关闭范围；
- `edit-session.scenario.mjs`：Instance/定义根同目标、唯一编辑态、重复 occurrence、嵌套 Product、空 Part、PINNED 和迟到激活；
- `open-document-tab.scenario.mjs` 与 `document-tab-order.scenario.mjs`：显式打开、后台摘要不新增、Product 宿主首位、稳定排序和关闭后不复活；
- `context-menu-icon.scenario.mjs`：图标/空槽及可访问名称；
- 既有 Product Update、TREE-02 多 Body/显隐/参数/Publication 场景用于回归。

本次文档核对只检查代码、测试入口和文档，不执行上述测试。此前本文仅列出定向入口，没有逐项执行结果；不据此补记这些测试已通过。维护者反馈与既有 ACCEPT-PRODUCT 自动测试记录各有范围，不能相互替代。

能力及验证限制独立保留：打开标签注册表仍是单 API 进程内状态，尚未成为分布式 presence 服务。服务端结构树仍整体构造，Feature 贡献索引与子树分页仍未交付。没有逐项记录的真实排序交互、跨窗口及 Product 更新失败视觉场景仍缺专项证据；全量测试、大模型容量、并发、多副本部署和所有故障场景均不在此次反馈的可确认范围内。本次未运行浏览器/WebGL、全量单测或容量基准。这些边界不使本轮整改重新处于未完成状态。

这里的 `EditSession` 是工作台编辑上下文，不是 M4 约束求解 Session，也不证明最近可行连续拖拽已交付。会话工作 CAD Body 是 Part 历史/求值单元；装配 solver body 或未来运动学刚体是 occurrence 位姿对象，不能混作同一身份。
