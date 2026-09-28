# 语义结构树与交互上下文

## 当前调用链与问题

`PartModel` 保存 Body、Feature、Sketch 输入、Publication 与 ContextReference；`ProductModel` 保存 occurrence、版本策略、Publication、Binding 和约束。`workspace.Service.buildDocumentStructure` 从已解析 Revision 生成 `DocumentStructureNode`；`DocumentView` 同时提供模型摘要、per-Body Artifact 索引、resolved occurrence 和引用更新。前端 `treeData` 适配为虚拟行，`structureSelection` 转换树入口；视口从 Body 几何和 GLB 拾取，`Workbench` 将树/视口选择交给工具、属性和命令。实时事件更新 Query 缓存，Product 的接受更新仍使用服务端 `UpdatePlan`。

现有断点：树路径被解析为 BodyId；Part 根被映射为 Body；同一 Feature 在多个 occurrence 中缺少完整选择范围；Publication 树入口冒充目标几何，删除/命令可能误用目标；折叠祖先被当作选中；已消费 Sketch 只按布尔集合收纳，多用户无法表达；Product 更新遍历 UI 树和诊断文字；旧单 Body 前端 fallback 与服务端投影并存；`ActiveBodyID` 和 Body `Visible` 是持久默认值，尚无完整的会话覆盖合同。

## 领域与投影合同

领域对象身份、所有权、依赖和求值顺序只来自 `PartModel` / `ProductModel`、命令和已解析 Revision。树是可操作投影；父子入口表达阅读关系，不改变 Sketch 所有权或 Feature 顺序。`Part → Body → Feature` 保留三层；Solid 是 Body 在给定快照下的几何结果，不进入建模历史。Origin、Parameters、外部引用和 Publication 仅在有实际内容时出现。BREP、mesh.glb、naming.pb 留在 Inspector 的文件区域。

Sketch 保留一个业务对象。未使用的 Sketch 位于其所属 Body；同 Body 单次使用可收纳到使用它的 Feature；跨 Body 或多次使用时，原 Sketch 保留所属 Body 的主入口，使用方显示只读输入引用。引用入口不能获得删除 Sketch 的能力。删除 Feature 输入关系、删除 Sketch 本体和删除引用入口是三种领域意图；未有命令的意图明确禁用。Feature 名称与图标可反映 ADD/REMOVE，但 `BodyOperation` 和生成器保持统一。`NEW_BODY` 创建 Body；当前不连通 ADD 的自动分配是服务端显式策略，应在 Preview 给出最终 BodyId 和操作结果；Feature 编辑不得重新分配 Body。

树节点携带 `subject = {documentId, entityKind, entityId}`、入口 `id`、`presentationRole`、`ownerDocumentId`、`bodyId`、`snapshot = {revisionId, contextVariantKey?, geometryKey?}`、`childrenState`、解析/求值状态和服务端 capability。`OccurrenceRef` 使用 rootDocumentId 与完整 InstancePath；同一 Part 定义可在不同 occurrence 与不同已解析 Revision 中投影。`TreeNodeId` 只定位入口。显示名、数组位置和路径字符串不参与业务身份。拓扑本地 ID 必须连同 Body geometryKey 和 Revision 使用；长期引用继续用 PersistentSelection。

Part 独立打开与 Product 内 Part 使用同一个投影函数。Product 追加 occurrence、解析 Revision、ContextVariant；PINNED 保留已接受版本。树按需展开和行虚拟化只影响渲染，不影响 Product 更新。结构树不持有 Mesh/Naming 大数据。独立视图和 Product occurrence 的树、属性和视口必须使用同一已解析快照；迟到异步结果以请求代次和快照范围丢弃。

## 交互合同

精确选择是命令目标；祖先只提示存在选中后代。折叠、过滤、滚动定位、自动展开不改选择集合。树入口选择返回语义对象，Publication 选择仍是 Publication；其解析目标可作关联高亮和定位。选择 Body 高亮 Body；选择 Feature 优先查询当前结果中的命名/provenance 贡献，无可靠定位时仅突出树入口并提示贡献不可定位，不能伪装整个 Body 为精确 Feature 高亮。历史结果和设为历史工作位置须显式命令。

单击选择，双击/菜单编辑或激活。Selection/Hover、编辑文档及 occurrence、会话工作 Body、In Work Object、临时显隐、持久 suppression 是不同状态。当前持久 `ActiveBodyID` / `Visible` 继续作为旧 Revision 可读的默认值；会话覆盖只能留在前端交互状态，不能将普通选择或显示切换写入建模 Revision。删除、抑制、重排由服务端领域命令验证；未实现的 Feature 顺序编辑和 occurrence 顺序编辑禁用。

Product 更新依赖必须由 `ProductModel.instances` 和服务端 UpdatePlan 提供，不得由树是否加载或诊断文字决定。普通模型刷新不传播 selection；“定位 occurrence”或“新标签打开”才传选择意图。ContextReference、ContextInput、ContextBinding 和 Publication 是明确的可检查入口，断裂后仍保留；连接、时效、求值和加载状态分别表达。

## 本阶段与后续

本阶段落实现有 Part/Product 类型的投影、身份、选择、更新和可用命令闭环；不声称任意历史插入、完整 Feature 贡献查询、任意拖拽重排、断裂外部引用原位重连、机制仿真或装配体原位建模已交付。当前结构快照仍由服务端整体构造，前端按需展开和虚拟渲染；服务端子树分页/复用 Definition 缓存待大树基准后实施。未来 Feature 类型只扩展描述器与领域命令；Mechanism/Joint/Driver 引用 occurrence，位姿是临时覆盖；跨 Body Boolean 记录消耗关系，不以树节点搬家代替；Product 上下文结果与 Part 定义编辑使用不同命令边界。

## 参照与有意差异

CATIA V5 本机 B33 文档 `online/ccvug_C2/ccvugbt0500.htm` 与 `ccvugbt2500.htm` 展示 Define in Work Object，`ccvugbt2300.htm` 展示 Parameters/Relations 的树显示设置；Dassault 的 [3DEXPERIENCE 多 Body 建模示例](https://3dswym.3dexperience.3ds.com/idea/catia-user-community/catia-methodology-simple-mechanical-part-design_I4pE_Ey5QA-E00XrgvS4CA)同样区分 Body 组织与工作对象。这里借鉴设计历史、工作对象和 Product occurrence 的交互语义，但保留 occccad 的 per-Body 求值、Revision/Workspace、PINNED/FOLLOW、显式命令和可重建制品规则。CATIA 文档不是当前实现证据。
