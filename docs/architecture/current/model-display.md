# 建模归属、显示与节点操作

> 当前建模归属和显示合同；树的稳定身份见[语义投影](../semantic-tree-interaction.md)。

## 调用链与判定

`CommandRequest` 经 WebSocket/Workspace 的 legacy adapter 映射到版本化 Domain Command；Part/Product 的不可变 Revision 保存模型，ChangeSet 用于 Undo/Redo。实体命令先把 Sketch/Profile、显式目标 Body 和 BodyOperation 固定在请求中；Preview 与 Commit 共用 `applyCreateFeature`、per-Body evaluation 和 OCCT `apply_body_operation`。Part 结构由同一个 `partStructureChildren` 投影到独立文档与 Product occurrence；`VisibilityResolver` 使用结构中的稳定主体、BodyId、完整 InstancePath 和本地显示属性求有效状态。Viewport 以语义地址绑定渲染对象，正常拾取、悬浮和草图捕捉按有效显示过滤。参数和 Publication 的树入口、管理器及属性调用正式命令。

## Body 与几何

Body 是稳定的历史和独立求值单元。`ADD/REMOVE/INTERSECT` 在指定 Body 上执行真实 Fuse/Cut/Common，允许其合法结果含多个 Solid，拒绝空结果、无材料变化、无效 BREP 和游离拓扑。`NEW_BODY` 创建恰好一个 Body；两个不相交 Sketch 分别 ADD 可得到一个 Body、两个 Pad、两个 Solid。后续桥接或切断材料不改变 BodyId。首次新 Body 的 Feature 在求值链中记为 ADD。Preview 返回既有目标或显式新建的归属。Feature 编辑、Undo/Redo 和旧 Revision 加载不做身份迁移。Evaluator 与 topology policy 已升版，旧缓存不会当作新规则结果复用。

选中 Sketch 创建实体时，未选其他目标默认使用 Sketch 所属 Body；表单固定显示目标 Body 并把同一值送入 Preview/Commit。使用已有 Sketch 不改变其 `BodyID`；树下收纳和输入引用只改变入口，不复制定义。

## 显示模型

定义级 Body、Sketch、SketchEntity，以及 Origin、基准面、基准轴、轴系、原点和 XYZ 子轴的本地显隐随 Part Revision 保存；Product occurrence 保存 `InstancePath + EntityKind + EntityId` 的 `SHOW/HIDE/INHERIT` 覆盖。有效状态遵循 `parentEffectiveVisible AND localVisible AND displayEligible`，实体/草图层级为 occurrence → Part → Body → Sketch → SketchEntity；参考元素按 Part → Origin → 基准/轴系 → 原点/XYZ 子轴继承。XYZ 共用轴系稳定 ID，以显式 X/Y/Z 子槽区分显示地址。树收纳、输入引用和 Publication 不参与显示继承。隐藏祖先不会覆盖子对象的本地值；树给出本地设置、有效状态及阻断来源。工作台菜单分别声明“零件定义”和“当前实例”，恢复继承删除覆盖。编辑隐藏草图的临时显示仅属于会话，退出编辑后恢复持久状态。隐藏元素不参与普通拾取、悬浮或捕捉，但仍参与求解、Profile 和实体求值。装配约束的对象显示属性随所属 Product Revision 保存，隐藏标记及引线不会抑制约束、改变已接受状态或组件位姿。无独立几何结果的历史 Feature 不提供显隐命令。

显示隐藏命令使用已有 Domain Command、Revision、CAS 和 Undo/Redo；没有几何影响种子时，ChangeSet 保存空 JSON 数组 `[]`，满足 `change_sets.impact_seeds` 的非空约束。视觉投影更新复用已加载对象，并以语义地址更新可见性和交互候选。几何依赖输入剔除纯显示字段，显示改动不要求重算 BREP、naming.pb 或完整 GLB。辅助基准/临时隔离仍是独立会话显示状态，尚未统一为定义级显示属性。

Product 普通视图从每个已解析 Body 的现有 GLB 读取草图图元，不要求进入 Part 原位编辑。GLB 扩展解码后是 schema 2 图元，视口也接受内存中的 schema 1 图元。`ResolvedInstance.ownedSketchIds` 由该 Part Revision 的 Feature 所属 Body 投影，只包含轻量 ID；跨 Body 使用 Sketch 时，消费 Body 的 GLB 也可能携带该输入，但视口仅在所属 Body 绘制它。每个 occurrence 各有草图渲染对象和选择身份，显示解析器按该实例的定义状态与覆盖更新对象及拾取；原位编辑的草图覆盖层替代当前实例的普通图元。Part 引用 Revision 或 ContextVariant 改变时重新绑定该实例场景，复用已下载的 GLB；单纯切换 occurrence 显隐只更新现有对象。

## 参数与 Publication

Feature 必需参数带稳定 ParameterId、OwnerFeatureId、PropertySlot 和生命周期；默认 UI 展示由 Body/Feature 名称与属性描述组成的路径，内部生成 key 仅在技术详情中出现。`EDIT_PARAMETER` 一次验证并提交别名与数值/表达式，表达式依赖绑定 ParameterId；`DELETE_PARAMETER` 拒绝必需参数、Sketch 尺寸及仍被表达式、Publication 或 ContextInput 使用的参数。参数管理器可创建受限的用户数值参数（长度、角度或无量纲），经依赖检查后可删除，并支持 Undo/Redo。输入限定显示路径目前只支持完整路径解析，混合公式仍使用别名。

Body Publication 目标用 `BODY_RESULT + BodyId` 表示该 Body 的当前结果；Feature 输出用 `FEATURE_OUTPUT + FeatureId + OutputSlot` 表示其历史输出。旧 `FEATURE_OUTPUT` Revision 仍按原语义读取，不在加载时转换。默认名称在所属 Part/Product 的 Publication 命名空间分配 `Body.1`、`Face.1` 等，与拓扑 localId 无关。树、管理器和属性面板共用编辑/删除入口；Part 与 Product 转发 Publication 均可重定向，服务端检查目标连通、所属路径和合同兼容，保留 PublicationId 与名称。删除 Body/Feature 时先检查同一 Part 的 Publication 目标；删除 Publication 时服务端扫描当前主工作区的 ContextReference、外部参数、ContextBinding 和转发 Publication。发现依赖则拒绝；删除 Publication 本体保留目标几何。该检查当前是主工作区快照预检，尚无跨工作区并发的锁定索引；完整引用查看仍未闭环。

## 数据与边界

所选关系数据库（PostgreSQL/SQLite）保存模型和轻量显示元数据；ArtifactStore 保存 per-Body BREP/GLB/Naming；WebSocket 承载命令、Preview 与状态；HTTP 承载制品下载。没有 Part 级持久聚合几何，也没有改变 XDE Definition/Occurrence。历史 Revision 不自动合并 Body、不重写 Naming。现有开发数据若依赖旧自动分 Body 的建模意图，应显式重建或按维护者批准的数据边界重置；本次没有执行数据删除。

验证入口为邻近 Body/显示/参数/Publication 场景。浏览器、多标签页、Product 更新及跨 Workspace 并发需要明确的独立证据，不能由模型测试代签。

## Feature 定义、当前贡献与阶段显示

同 Body 的实体 Feature 按模型历史顺序同级显示，阶段 S0/S1/S2 不生成普通树节点。`featureInputs` 从定义枚举轮廓、按定义顺序的截面/点、轴、支撑、中性面、工具、阵列种子与拓扑来源阶段，依赖检查和树投影共用该枚举。同 Body 单一消费者的专用草图可以收纳；共享/跨 Body 输入显示引用，原定义和所有权不变。阵列种子保持原树位置与编辑能力，依赖删除仍受保护。

`storeEvaluation` 验证同一 BREP/Naming 快照后，从完整 transition 与 tip locator 派生 `featureAssociations`，与面/边映射一起写入 GLB 的既有 Visualization 扩展。显示摘要中的 `featureContributions` 区分当前贡献、完整历史下无当前贡献和关联缺失。索引不成为持久 Naming，不改变 SemanticTopologyRef 或 PersistentSelection，也不保存每步历史网格；显示元数据变体保留原配套索引。

生成、修改和支撑分别记录。组合历史中的 Removed 标记不能覆盖最终 Shape 中仍存在的同一 TShape；这种明确存活的面/边保留上游来源，不能被邻接补齐命名误标为局部修改生成。支撑域裁剪的限制面通过同类型 Modified 传播原面身份。圆角扇区的实际新增面经布尔、同域融合和容差副本的真实 History 传播，并补回根边到最终面的 Generated 关系，避免中间删除令组合 History 丢失新增封口来源。封口融合原面时，保留原拉伸和圆角两个来源；共用边另有真实邻接来源时保留额外候选，不要求面边的全部来源集合相同。Modified/Split/Merged 继承实际来源，多个生成来源保留候选；Generated 新过渡面主要定位其生成 Feature，读取整个 Body 不产生整 Body 贡献。阵列 MEMBER 语义槽位与 Naming 中精确 seed SemanticTopologyRef 摘要配对，成员及其下游过渡面保留 pattern/slot，不按位置匹配种子；过渡生成别名经精确源引用摘要恢复支撑及成员上下文。

视图命中保持真实拓扑主选择；树关联/预选提示不进入命令目标。Feature 选择按当前 Face/Edge ID 集合渲染融合网格高亮，Body 选择才高亮整体。匹配必须同时满足文档、Revision、Body、GeometryKey、occurrence、variant 和显示阶段。历史结果通过 `feature-input{resultStage:true}` 按需重建，下载仍验证 Feature、Revision 和精确对象范围；历史预览不参与建模拾取。恢复按钮、编辑结束或上下文变化清除临时阶段显示。

生成工具选源与整体 Body 阶段分开；视图生成来源有歧义时提示候选，定义列表给出明确选择。特征组合以明确起止步骤复用阶段差集；树多选同 Body 的生成特征及后续圆角/倒角时按历史顺序初始化范围，文档、Revision 和 occurrence 不一致则拒绝。已有实体阵列允许显式切换选源模式。该范围支持添加型拉伸/旋转及随后的圆角/倒角；此前 Body 材料不参与复制。当前生成工具阵列仍限已有 PAD/LINEAR_EXTRUDE/REVOLVE 能力，不以最终 Body、白名单扩展或全 Body 复制替代任意特征组重执行。阵列阶段可以作为下游局部加工输入，种子编辑与成员上的下游圆角分别提交定义并通过真实 Naming 重算。

标准轴的显示与树选择携带轴系 ID＋X/Y/Z、文档、Revision 和 occurrence；hover 定位轴的子节点，主选择不升级为整个轴系。轴系原点使用单独 `datum-point` 选择角色。投影方向的显示在所属 occurrence 坐标中按屏幕长度延伸，单位方向快照不冒充有限几何端点。

视区设置分别保存场景地面网格、草图网格和参考元素的前端显示偏好，网格关闭不关闭捕获。投影工具可临时揭示被显示偏好关闭的参考元素，不能揭示定义级隐藏对象。持久显示属性优先于旧的会话显示覆盖，且不进入 Datum 几何依赖摘要。Datum 编辑保留显示属性。

显示制品复用时，SelectionIndex 从当前权威快照投影选择身份：根 Part 同步 Revision，拓扑仅在原 GeometryKey 仍存在时同步；Product 继续按完整 occurrence 解析各自 Revision。基准几何位置/方向/增删变化进入显示几何签名，显示属性不进入。旧对象不因复用 GPU 资源而继续携带旧投影版本。

Origin/基准的纯 `display.*` 补偿与重放沿用冻结的 Body 制品，不进入参数、Publication 或实体求值。显示命令不追加基准求值槽，避免同一次显隐同时写 `display.PLANE` 与整份 `datum.plane`，使 Undo/Redo 保持与普通显示命令相同的几何复用规则。

## 完整属性检查

属性面板保留对象摘要，并默认展开“完整对象数据与上下文”。`inspectorObjectData` 从已加载定义与权威拓扑响应解析所选对象，原样展示参数来源/AST/依赖、量纲、SI 值、特征输入、基准变换、约束、Publication、求值诊断及关联制品字段；不按显示精度截断原始数据。数据区只读，可选择复制文本，长内容在区域内滚动。选中参数另显示别名、稳定 ID、所有者、属性槽、生命周期、值类型、单位和角色。

所有者文档与 Revision 必须匹配选择上下文。跨文档检查复用现有只读文档查询，不激活或打开标签。当前查询返回的 Head 不匹配历史/固定 Revision 时，明确提示完整定义尚未加载，不能以其他快照替代；已有精确拓扑响应和 occurrence 仍按各自上下文展示。读取失败保留选择数据和错误信息。菜单继续使用公共 `ContextMenuIcon`，没有图标的动作也保留固定空槽。
