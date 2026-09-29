# TREE-03：Product 编辑会话、标签与菜单

> 2026-09-29 代码基线。结构树领域身份与显隐见 [TREE-02](tree02-model-display.md)，长期 Product 上下文合同见[目标架构](../target/product-context.md)。

## 编辑会话

一个 Workbench 路由只拥有一个宿主文档。Product 路由的宿主始终是根 Product；编辑子 Part/Product 不改变 URL、活动标签或根场景。前端 `EditSession` 一次保存宿主 DocumentId、唯一编辑目标（DocumentId、类型和完整 InstancePath）、宿主/目标 Revision 基线、root snapshot digest、会话工作 Body 及 activation generation。工作台、属性、历史、Undo/Redo 和普通 Part/Product 命令读取编辑目标；Instance 引用策略、移动和替换仍按 InstancePath 最后一段的 owner Product 路由。

Product 内不再存在脱离 occurrence 的定义编辑模式。双击 Part Instance 与其下的 Part 根解析为同一上下文目标；双击 Product Instance 与其下的 Product 根解析为同一嵌套 Product 目标；双击根 Product 返回根装配编辑。Instance 只是快捷入口，绿色文档编辑态只标记类型、DocumentId 和完整 occurrence 路径均匹配的 Part/Product 根；同一共享 Part 的其他 occurrence、Instance 行和工作 Body 不复用该标记。Body 激活只更新会话工作 Body。

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

定向验证入口：

- Go `TestOpenDocumentRegistryPreservesOrderAndWorkspaceScope`：窗口隔离、原位摘要更新、关闭范围；
- `edit-session.scenario.mjs`：Instance/定义根同目标、唯一编辑态、重复 occurrence、嵌套 Product、空 Part、PINNED 和迟到激活；
- `open-document-tab.scenario.mjs` 与 `document-tab-order.scenario.mjs`：显式打开、后台摘要不新增、Product 宿主首位、稳定排序和关闭后不复活；
- `context-menu-icon.scenario.mjs`：图标/空槽及可访问名称；
- 既有 Product Update、TREE-02 多 Body/显隐/参数/Publication 场景用于回归。

打开标签注册表仍是单 API 进程内状态，尚未成为分布式 presence 服务。服务端结构树仍整体构造，Feature 贡献索引与子树分页仍未交付。本轮按维护者要求未执行浏览器/WebGL 验收和全量测试；真实拖动排序、跨窗口以及 Product 更新失败后的视觉状态由后续实机验收确认。
