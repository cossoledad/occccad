# 建模归属、显示与节点操作

> 当前建模归属和显示合同；树的稳定身份见[语义投影](../semantic-tree-interaction.md)。

## 调用链与判定

`CommandRequest` 经 WebSocket/Workspace 的 legacy adapter 映射到版本化 Domain Command；Part/Product 的不可变 Revision 保存模型，ChangeSet 用于 Undo/Redo。实体命令先把 Sketch/Profile、显式目标 Body 和 BodyOperation 固定在请求中；Preview 与 Commit 共用 `applyCreateFeature`、per-Body evaluation 和 OCCT `apply_body_operation`。Part 结构由同一个 `partStructureChildren` 投影到独立文档与 Product occurrence；`VisibilityResolver` 使用结构中的稳定主体、BodyId、完整 InstancePath 和本地显示属性求有效状态。Viewport 以语义地址绑定渲染对象，正常拾取、悬浮和草图捕捉按有效显示过滤。参数和 Publication 的树入口、管理器及属性调用正式命令。

## Body 与几何

Body 是稳定的历史和独立求值单元。`ADD/REMOVE/INTERSECT` 在指定 Body 上执行真实 Fuse/Cut/Common，允许其合法结果含多个 Solid，拒绝空结果、无材料变化、无效 BREP 和游离拓扑。`NEW_BODY` 创建恰好一个 Body；两个不相交 Sketch 分别 ADD 可得到一个 Body、两个 Pad、两个 Solid。后续桥接或切断材料不改变 BodyId。首次新 Body 的 Feature 在求值链中记为 ADD。Preview 返回既有目标或显式新建的归属。Feature 编辑、Undo/Redo 和旧 Revision 加载不做身份迁移。Evaluator 与 topology policy 已升版，旧缓存不会当作新规则结果复用。

选中 Sketch 创建实体时，未选其他目标默认使用 Sketch 所属 Body；表单固定显示目标 Body 并把同一值送入 Preview/Commit。使用已有 Sketch 不改变其 `BodyID`；树下收纳和输入引用只改变入口，不复制定义。

## 显示模型

定义级 Body、Sketch、SketchEntity 本地显隐随 Part Revision 保存；Product occurrence 保存 `InstancePath + EntityKind + EntityId` 的 `SHOW/HIDE/INHERIT` 覆盖。有效状态遵循 `parentEffectiveVisible AND localVisible AND displayEligible`，层级为 occurrence → Part → Body → Sketch → SketchEntity。树收纳、输入引用和 Publication 不参与显示继承。隐藏祖先不会覆盖子对象的本地值；树给出本地设置、有效状态及阻断来源。工作台菜单分别声明“零件定义”和“当前实例”，恢复继承删除覆盖。编辑隐藏草图的临时显示仅属于会话，退出编辑后恢复持久状态。隐藏元素不参与普通拾取、悬浮或捕捉，但仍参与求解、Profile 和实体求值。无独立几何结果的历史 Feature 不提供显隐命令。

显示隐藏命令使用已有 Domain Command、Revision、CAS 和 Undo/Redo；没有几何影响种子时，ChangeSet 保存空 JSON 数组 `[]`，满足 `change_sets.impact_seeds` 的非空约束。视觉投影更新复用已加载对象，并以语义地址更新可见性和交互候选。几何依赖输入剔除纯显示字段，显示改动不要求重算 BREP、naming.pb 或完整 GLB。辅助基准/临时隔离仍是独立会话显示状态，尚未统一为定义级显示属性。

Product 普通视图从每个已解析 Body 的现有 GLB 读取草图图元，不要求进入 Part 原位编辑。GLB 扩展解码后是 schema 2 图元，视口也接受内存中的 schema 1 图元。`ResolvedInstance.ownedSketchIds` 由该 Part Revision 的 Feature 所属 Body 投影，只包含轻量 ID；跨 Body 使用 Sketch 时，消费 Body 的 GLB 也可能携带该输入，但视口仅在所属 Body 绘制它。每个 occurrence 各有草图渲染对象和选择身份，显示解析器按该实例的定义状态与覆盖更新对象及拾取；原位编辑的草图覆盖层替代当前实例的普通图元。Part 引用 Revision 或 ContextVariant 改变时重新绑定该实例场景，复用已下载的 GLB；单纯切换 occurrence 显隐只更新现有对象。

## 参数与 Publication

Feature 必需参数带稳定 ParameterId、OwnerFeatureId、PropertySlot 和生命周期；默认 UI 展示由 Body/Feature 名称与属性描述组成的路径，内部生成 key 仅在技术详情中出现。`EDIT_PARAMETER` 一次验证并提交别名与数值/表达式，表达式依赖绑定 ParameterId；`DELETE_PARAMETER` 拒绝必需参数、Sketch 尺寸及仍被表达式、Publication 或 ContextInput 使用的参数。参数管理器可创建受限的用户数值参数（长度、角度或无量纲），经依赖检查后可删除，并支持 Undo/Redo。输入限定显示路径目前只支持完整路径解析，混合公式仍使用别名。

Body Publication 目标用 `BODY_RESULT + BodyId` 表示该 Body 的当前结果；Feature 输出用 `FEATURE_OUTPUT + FeatureId + OutputSlot` 表示其历史输出。旧 `FEATURE_OUTPUT` Revision 仍按原语义读取，不在加载时转换。默认名称在所属 Part/Product 的 Publication 命名空间分配 `Body.1`、`Face.1` 等，与拓扑 localId 无关。树、管理器和属性面板共用编辑/删除入口；Part 与 Product 转发 Publication 均可重定向，服务端检查目标连通、所属路径和合同兼容，保留 PublicationId 与名称。删除 Body/Feature 时先检查同一 Part 的 Publication 目标；删除 Publication 时服务端扫描当前主工作区的 ContextReference、外部参数、ContextBinding 和转发 Publication。发现依赖则拒绝；删除 Publication 本体保留目标几何。该检查当前是主工作区快照预检，尚无跨工作区并发的锁定索引；完整引用查看仍未闭环。

## 数据与边界

所选关系数据库（PostgreSQL/SQLite）保存模型和轻量显示元数据；ArtifactStore 保存 per-Body BREP/GLB/Naming；WebSocket 承载命令、Preview 与状态；HTTP 承载制品下载。没有 Part 级持久聚合几何，也没有改变 XDE Definition/Occurrence。历史 Revision 不自动合并 Body、不重写 Naming。现有开发数据若依赖旧自动分 Body 的建模意图，应显式重建或按维护者批准的数据边界重置；本次没有执行数据删除。

验证入口为邻近 Body/显示/参数/Publication 场景。浏览器、多标签页、Product 更新及跨 Workspace 并发需要明确的独立证据，不能由模型测试代签。
