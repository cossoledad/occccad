# Product 关联设计、装配与发布

> 本轮六族代码核对基线：`main / c1becfd` 加 CONSTRAINT-COMPOSITION 工作区变更（2026-10-01）。返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。各旧阶段保留自己的执行基线，人工与自动化证据分别记录。

- 当前 Product UI 中的新实例固定采用 `FOLLOW_HEAD`；被引用文档变化后先使 root-snapshot `ProductUpdatePlan` 失效并投影 `UPDATE_AVAILABLE`，打开 Product 的可编辑客户端随后按叶到根自动接受每一级带 digest 的计划，提交普通 `UPDATE_REFERENCES` Revision 并重建可重放快照。Instance 右键可把当前 resolved Revision 切换为 `PINNED`，也可恢复 `FOLLOW_HEAD`；任意历史版本 picker 尚未开放。Instance 右键“在新标签页中打开”调用显式 open-document 生命周期并进入独立 Reference Document 工作空间；普通 GET、依赖读取和更新广播只更新缓存或已打开摘要，不创建、置顶或激活标签。具体边界见 [TREE-03](tree03-product-edit-tabs.md)。

Product 工作台维护浏览器窗口级 `EditSession`，明确区分宿主 Product、唯一 Edit Target、root/target 快照基线、工作 Body 和激活代次。打开 Product 时根 Product 默认激活；双击结构树中的 Product、Part 或 Instance 节点解析并校验完整 occurrence，准备成功后一次切换 Toolbar、属性、历史、Undo/Redo 和 Domain Command 的 Reference Workspace。激活不是模型命令，不写 Revision；同一 Part Reference 的其他 occurrence 不显示为激活。普通建模命令修改共享 Part 定义，不隐式复制文档或生成 occurrence 专属几何；FOLLOW_HEAD 引用通过 UpdatePlan 更新，PINNED 路径进入修改前必须显式恢复。视口始终保留根装配；活动 Part 的草图、基准和命令预览施加该 occurrence 的世界 Placement 后就地编辑，其他部件继续显示。

根 Product 场景与 Active Part 交互视图由 `ViewportEditContext` 分离，命令/草图选择只读取激活 Part。Publication 与基准几何的显示/拾取规则见[Web 交互](web.md)。

`INSERT_INSTANCE` 继续把既有 Part/Product Reference 插入当前激活且具有 Editor 权限的 Product；InstanceName 由服务端在 owner Product 当前候选模型中按 `ReferenceName.N` 分配首个同级可用名称，显示名不参与身份，重命名属于独立属性命令。

`INSERT_INSTANCES` 可一次插入最多 128 个不同文档，或以一个已有直接子 Instance 为源沿当前 Product 的世界 X/Y/Z 轴复制。阵列数量含原实例（2–128），间距以毫米指定，反向开关改变轴向；原实例不变，新实例保留其引用版本、引用模式与旋转，各自获得新的稳定 ID。新实例名称以被引用文档名而非源 InstanceName 为基底，从同级首个可用序号分配，例如 `Part1.1`、`Part1.2` 后得到 `Part1.3`。API 对批量文档引用逐一验证读取权限并阻止 Product 循环引用；整个批次作为一个 Domain Command/ChangeSet 写入一个 Revision，可整体 Undo。Web 插入浏览器多选跨搜索和分页保留选择，“当前选择”页进入时冻结展示列表，在该页取消选择不会移除卡片，离开再进入才刷新。`product.pattern` 是独立 Toolbar 命令，使用非模态浮动面板；可先选或在面板打开后选择源组件，输入新增数量、间距、方向和反向，并在视口显示不可拾取的半透明副本，提交后由权威装配模型重建。交互参考本机 CATIA V5 [Defining a Multi-Instantiation](/mnt/s/tools/DS/Catia/B33doc/English/online/asmug_C2/asmugbt0312.htm) 的选组件、定义参数、预览与 Apply 流程；当前只实现三个坐标轴，不支持选择任意线/边为方向，也没有 Fast Multi-Instantiation。

Product 结构树的 Product 节点另提供“新建零件”。`POST /api/documents/{rootProductId}/part-components` 接收可选的 typed target Product InstancePath；服务端在事务外建立空 Part 初始 Revision 和目标/祖先 Product 候选，再以一个 `ProductDesignTransaction` 原子创建 Part 文档、插入 `FOLLOW_HEAD` occurrence，并从目标 Product 自底向上推进到 root snapshot。任一目标路径、权限或 Workspace Head 失效都会整体拒绝，不产生孤立 Part 或半更新装配。空名称按全局首个可用 `PartN` 分配，InstanceName 仍按目标 Product 同级的 `PartN.N` 分配；第一版只支持位于目标 Product 原点的 identity placement。Undo/Redo 补偿 Product 成员 Revision，因此 Undo 会移除 occurrence 并恢复祖先引用，但不会删除已经创建的 Part 文档；该文档保留在 Document Center，可再次插入或显式删除。结构树 capability 描述领域动作，实际入口仍由服务端对 root、目标和全部祖先 Product 再次执行 Editor ACL 校验。

第一版 typed `InstancePath` 保存 RootDocumentId 和有序 segment；每段包含 owner Document/Revision、稳定 InstanceId、显示 InstanceName、ReferencedDocumentId 和 resolved Revision。`canonical` 由 InstanceId 链构成并用于选择、激活和 occurrence 资源索引，`display` 才由 InstanceName 拼接。名称修改不得改变 canonical identity，也不得作为持久引用解析键。

DocumentView 另外投影由 Product 领域引用边确定的 `followedDocumentIds/followedProductIds`，跳过 PINNED 边，供 Web 实时订阅和叶到根 UpdatePlan 接受。该依赖集合与树是否展开、过滤、加载无关。Product 中的 Part 复用独立 Part 的结构投影，occurrence 追加完整 InstancePath、已解析 Revision 和 ContextVariant；相同 Part 在不同 occurrence 或固定版本中拥有不同选择与几何范围。模型刷新不会无条件复制选择；打开来源或激活实例是显式交互。未来机制仿真刚体引用 occurrence，临时位姿不写入 Product Revision；装配体原位建模的 Part 定义修改与上下文结果生成仍须使用不同命令边界。

## Publication 与受控外部引用

Publication 保存在 Part/Product Revision 中。Datum POINT/AXIS/PLANE/FRAME、PersistentSelection CURVE/SURFACE、Feature output BODY 与 PARAMETER 共用稳定 PublicationId、兼容版本、typed CRUD、ChangeSet、Undo/Redo 和 dependency graph；拓扑 target 随 Part 求值按 naming 重解，失效后保存 `BROKEN_PUBLICATION`。Parameter Publication 固定 value type、dimension、SI unit policy、可选 bounds 与 source ParameterId；消费 Part 的 `ExternalParameterRef` 保存来源 Document、FOLLOW/PINNED selector、PublicationId、期望合同与冻结 `ReferenceResolutionSnapshot`。DocumentView 对来源 Head 只投影 `UPDATE_AVAILABLE/BROKEN`，显式 Update References 在事务外解析完整候选并以 Workspace sequence CAS 提交，普通重算不读取来源 Head。Assembly endpoint 优先保存 PublicationRef 与 PersistentSelection deep link；兼容 Replace/Update 重连新 target，Product Publication 可转发一个相对 occurrence 的子 Publication。

## Product Design Session 与上下文事务

Product 内关联由 Product Design Session 组织。`InstancePath` 由 root 与有序 typed segment 构成，canonical identity 只连接稳定 InstanceId，显示名只形成 breadcrumb；递归展开设有 cycle、深度 32 和成员 10000 的 gate。Instance、Part/Product Publication 与 ContextInput 使用版本化 `nfkc-casefold-v1` 作用域名称规则，rename 只改变显示投影。Product Publication 可沿嵌套 rigid Product path 转发。

Part 现在声明不含具体来源的 typed `ContextInput`，root Product Revision 持有 source occurrence Publication 到 owning occurrence input 的 `ContextBinding`、相对 Transform 和 accepted resolution snapshot。创建绑定会在事务外解析当前 root snapshot、合同、权限与依赖环，再为消费 Part、全部嵌套 owning Product 和 root Product 预生成 Revision/ChangeSet/EvaluationManifest；数据库事务按 Workspace 稳定顺序锁定并统一 CAS，一次提交所有 Revision、Outbox 和 Head。该事务组从任一成员文档触发 Undo/Redo 时也统一补偿，不会只留下 Part input 或 Product binding。`ContextReference` 仅保留为旧试点模型与独立 Part/受控外部通道能力，新 Product 内 picker 不再向用户暴露全局 DocumentId/PublicationId。

Web 以宿主 root Product 和唯一 occurrence 编辑目标维护非持久设计会话，无 definition/context 双模式切换；TREE-03 编辑会话与 M4 约束求解 Session 分属不同职责。`GET design-session` 与 root-snapshot-scoped `GET context-catalog` 只返回当前 Product 可达且调用者可读的 Publication，并按 expected type、连接状态和已知 ContextBinding DAG 过滤。参数与 Context binding 面板按 occurrence breadcrumb/可编辑名称选择来源；跨 Workspace 提交仍由服务端重新验证 typed path、Head 和合同。共享 Part 定义、定义显隐、实例覆盖及 ContextVariant 语义仍保留。

## 更新计划与派生 Variant

`ProductUpdatePlan` 从不可变 root snapshot 投影 occurrence/reference 与 ContextBinding 影响项，分别报告 connection、currency、evaluation，候选 Context Variant 会把已接受/候选 Publication 描述转换到 owning Part local frame，再复用 Part 参数、Sketch 与几何 evaluator 生成独立 GeometryKey、Publication resolution 和 EvaluationManifest。Variant identity 只包含 base Part Revision、规范化输入快照及 evaluator/policy；不包含 binding 显示名、BindingId、occurrence path、WorkerId，PARAMETER 输入也不包含无意义的 occurrence transform，因此同一定义和输入的四个 Wheel occurrence 可共享持久 variant cache；不同几何/参数输入不污染共享 Part。accepted variant 会覆盖 Product `ResolvedInstances` 的 base GeometryKey，并沿嵌套 rigid Product Publication 转发进入装配 descriptor。Update Plan 的 occurrence 更新项只属于当前 Product 的直属引用边；子 Product 通过自己的计划提交新 Revision，再由父 Product 接受。候选直属版本变化后，计划先按候选子版本重新展开路径，避免用旧子树阻塞父级接受。Web 对 FOLLOW_HEAD 变化按 Product 依赖顺序从叶到根自动接受，先读取待跟随子 Product 的最新定义，再处理新定义中引入的待更新后代；共享子 Product 按依赖排序，PINNED 边阻断遍历。每轮后重新读取当前 Product，深度和更新轮数均有界；每次仍以 digest 防止接受过期计划。Release 独立检查整棵非 PINNED 跟随树的版本状态，不因更新计划只处理直属引用而放松发布要求；Context Variant 等上游候选求值失败仍会阻止当前计划；装配约束 Broken/Impossible 仅作为诊断，不阻止引用接受。`HasUpdates` 由引用/上下文变化决定，失效约束本身不触发自动更新循环；显式 Refresh 仍可重新解析求解。接受新 HEAD 后支持元素解析失败的约束保留 Broken 并隔离，其余约束继续求解；Release 仍独立要求活动约束全部 Verified。

## M3 SolveManifest 与 Release

每次正式 assembly preview/commit 冻结 `AssemblySolveManifest`：root/candidate Revision、完整 body pose、局部几何描述符、Publication/PersistentSelection resolution evidence、约束、branch/intent、affected scope、schema 2 solver profile 与 build policy 共同形成确定 digest。构造器按持久化 JSON 表示冻结独立快照，位姿、角度分支及 Publication/PersistentSelection 证据不再共享调用方指针；后续调用方修改不改变已冻结内容与 digest。Worker 只消费 manifest 中的纯值；Publication endpoint 直接使用已解析 descriptor，不再让 solver 查询 Product/B-Rep。manifest 与 request-specific result 持久化，重试复用同一结果，digest replay、request lookup、deadline/cancel 和既有 `.3dreplay` 数值证据并存。

当前新输入使用 `assembly-six-families-composition-v10` policy/build，manifest schema 1、solver profile schema 2 不变，新增公共 definition v2、Quantity、八类 descriptor、独立角度轴与 `groupStages`。新 manifest 的 `digestPolicy=CANONICAL_JSON_V1` 针对 JSONB 的对象顺序与负零表示规范化；它只版本化本 manifest 身份，不改变单位、参数或引用语义。[摘要兼容入口](../../../services/internal/workspace/assembly_manifest_digest.go)读取旧空 policy 记录时按原冻结字段形状验证旧摘要，不能因新 Go struct 增加默认字段而使旧记录失效，也不能忽略被篡改的旧字段。v7/v8/v9 冻结记录继续校验其旧语义；新六族定义/解析 descriptor 不能伪装旧 policy。replay 记录当前真实 build，保证语义重放而非旧二进制逐位复现，不改写原 Revision、SolveManifest 或 Release。

当前保存独立 `ProductRelease`：Release Manifest 冻结完整 occurrence typed path/Revision/pose、ContextBinding、ContextVariant GeometryKey/EvaluationManifest、Product Publication、命名/evaluator policy、成功 SolveManifest 与 gate 结果。Gate 要求引用 current、全部 occurrence/variant READY、活动约束 Verified 且有可重放求解证据；停用定义保留在 SolveManifest 中，不伪造 Verified，也不参与此 gate。Release 可在 Workspace Head 移动后按 manifest replay，并从冻结 GeometryKey 提交 STEP/BREP 导出；Exchange placement 现已贯通 translation 与 quaternion rotation。Web 的“产品版本中心”只负责创建、列出和 replay 不可变里程碑，不再把 STEP/BREP 按钮混入发布流程；Exchange 保留为独立后续 UX。当前不把 Configuration/Design Table、partial update、flexible subassembly 或 Derive Part from Context 列为已实现能力。

## 求解与交互的当前边界

一次 `solveAssembly`（含 accepted/candidate 准入试算与最终求解）共享操作内读取缓存：按 document/revision 复用拓扑清单与嵌套 Product 模型，按 GeometryKey 复用制品元数据/B-Rep 引用，按 GeometryKey/type/local ID 复用精确属性。local ID 仅是已验证不可变制品内的查询键，不成为持久身份。命名解析仍检查源证据、目标 manifest 与 policy；已解析的目标直接查询精确制品，不再重复执行交互拾取的授权/绑定/重解析流程。文档存活性每次求解检查，用户权限仍由请求边界验证；缓存不跨请求、不保存 HEAD、根 occurrence pose、失败读取或求解结果，不改变准入隔离、名义姿态和结果持久化。

HTTP `phases_ms` 增加 `assembly-total`、`assembly-prepare`、`assembly-worker`、`assembly-manifest-write`、`assembly-result-read/write`，以及 `topology-manifest-read`、`topology-properties`、`topology-rpc`。同名阶段累加所有试算；阶段存在包含关系，不能相加当作总耗时。`assembly-worker` 包含 RPC/排队/求解，`topology-worker` 保留制品准备及 RPC 耗时，`topology-rpc` 单独计量 RPC；Worker 自身日志才是几何查询内部耗时。高延迟数据库下优先比较清单读取、准备和持久化与 Worker 阶段，而不是依据 `cache_hit` 推断端到端成本。拖拽前端仅保留一个在途权威预览并合并待处理姿态，因此每次 HTTP 延迟直接限制权威预览更新频率。跨请求缓存、批量预取和大系统数值优化尚未实现，应由这些阶段及 `.3dreplay` 测量决定。

`kernel/assembly` 只消费纯值 Point/Axis/Plane/Cylinder/Circle/Sphere/Cone/Frame、内部约束原语和完整 SE(3) pose。多 Body Part occurrence 仍是一个装配运动单元；支持可定位任意 CAD Body，不能据此拆分 solver body。Rigid cluster、Fix/Ground 消元与 connected-component 求解先于数值迭代；生产路径使用解析 Jacobian、augmented QR 和 SVD rank/null-space。M2.5 依次满足硬约束、最小化 reference motion、最小化总 nominal motion，独立报告偏好收敛与瞬时自由度。创建/编辑以第一选择为 moving、第二选择为 reference，Constraint 与全部 solved pose 在同一 ChangeSet 提交。Rigid 捕获当前相对位姿后允许 moving/reference 两个 occurrence 属于同一刚性组，reference 最小运动作用于共享组；既有约束仍逐项验证，真实不一致不会被固连掩盖。算法细节与 corpus 由[Solver Algorithms](../../../kernel/assembly/SOLVER_ALGORITHMS.md)维护。

当前 MOVE 仍注入临时 `interaction-driver` Fix；不可达预览恢复权威 pose，尚不能返回 M4 的最近可行拖拽结果。有效 preview candidate 可经完整身份与 CAS 校验提升为提交；未提供 PreviewID 的命令走权威求解，明确提供但已过期或不匹配的 candidate 返回 `PREVIEW_CANDIDATE_STALE_OR_MISMATCHED`，不能静默重新求解后采用另一结果。M3 已覆盖嵌套 rigid Product 与持久引用解析；flexible expansion、稀疏后端和最小冲突集尚未实现。

三维求解支持独立的 `occccad.3dreplay.v1` 下载：每次实际 SolveAssembly（包括 preview、成功、模型失败和已知的 RPC 失败）结束后，将精确数学请求、有效求解参数及紧凑结果在响应关键路径之外原子写入 `OCCCCAD_LOG_DIR/debug/assembly-replays/<documentID>/`。它不进入 PostgreSQL，不改变 Workspace Head/Revision，也不包含 B-Rep、网格或完整命令历史。每个文档最多保留 50 条且最长保留 7 天；文档读权限仍控制列表与下载，Web 可按真正发生求解的 request ID 下载 `.3dreplay`。文件可不依赖数据库通过 Worker/Router 重放。几何解析前失败尚未形成数值求解输入，不生成文件；本地存档失败只记录独立错误，不伪造求解状态。

装配约束创建和编辑现在都从最终候选模型提取相同的 `moving first / reference second` solve intent，不再因命令类型改变规约锚点。显式 Same/Opposite 的平面重合若从精确反向端点开始，Solver 会绕被约束平面锚点生成确定性的半周 branch seed，再执行普通 component solve，避免依赖有限差分噪声逃离零梯度鞍点。约束数值输入期间只更新本地表单；失焦或 Enter 才产生一次可取消、带 sequence 防迟到覆盖的权威预览，支持元素和方向等离散变更仍立即预览。

装配约束预览具有显式的两层工作流状态。Web 使用 XState actor 管理 `idle / pending / succeeded / failed`、请求 sequence、取消和迟到响应过滤；传输、基础设施或非法命令失败会显示错误并阻止提交；可解析的约束定义即使数值求解失败，也会返回带 NotUpdated/Impossible 诊断的预览并允许保存。失败预览不作为已收敛候选提升，提交重新求解且不接受失败候选姿态。Go Workspace 使用 Stateless 管理 `RESOLVING_GEOMETRY -> SOLVING -> APPLYING_RESULT -> COMPLETED`，任一活动阶段可进入 `FAILED`；API 对求解失败返回结构化 `code / phase / retryable`，而取消与 deadline 保持传输层语义。该工作流状态不持久化，也不取代 Product Revision、Command/ChangeSet 或 Solver 数值状态。

## 实现与验证入口

- [Product context](../../../services/internal/workspace/product_context.go)
- [原子事务](../../../services/internal/workspace/product_design_transaction.go)
- [事务历史](../../../services/internal/workspace/product_design_history.go)
- [Variant/Update/Release](../../../services/internal/workspace/product_update_release.go)
- [M3 manifest/replay](../../../services/internal/workspace/assembly_solve_manifest.go)
- [当前 MOVE Fix](../../../services/internal/workspace/assembly_solve.go)
- [typed path/命名 corpus](../../../services/internal/workspace/p10_product_context_test.go)
- [ToyCar/manifest corpus](../../../services/internal/workspace/p10_remaining_test.go)
- [Publication corpus](../../../services/internal/workspace/publication_test.go)
- [持久化迁移](../../../services/internal/database/migrations/0022_p10_product_manifests.sql)

## ACCEPT-PRODUCT 完成记录

状态：**已完成（2026-09-21）**。验收代码基线为 `a095288`，叠加本次 SolveManifest 快照隔离修复及其回归测试。

- 人工验收：维护者在本次会话明确确认 ACCEPT-PRODUCT 通过，覆盖原验收计划的 Product 内创建/激活、Publication/ContextBinding、Variant、Follow/Pin、约束恢复、Release/replay/export 与工作台偏好路径。此结论来自维护者确认，不是 Agent 本轮重新执行浏览器操作，也未补造截图或 trace。
- 标准测试：`GOFLAGS=-count=1 invoke check --scope all` 的 7 个步骤全部通过：验证路由、C++ 构建、116 项 CTest、Go 全包测试、Go 跨包 conformance、Web 场景、TypeScript/生产构建。Go 禁用结果缓存，实际重新执行。
- 补充回归：[SolveManifest 输入隔离](../../../services/internal/workspace/assembly_solve_manifest_test.go) 覆盖 initial guess、fixed pose、角度轴/分支、Publication 引用/Quantity、持久解析证据、intent 和 affected bodies 共 9 个子场景。修复前 7 项失败，修复后全部通过；原 manifest 确定性测试同样通过。
- 修复：原构造器只复制外层 slice，调用方修改嵌套指针可改变 manifest 内容而不改变 digest；现在构造器冻结独立持久化表示，保持现有 schema、字段与重放合同。
- 限制：本轮未设置 `OCCCCAD_TEST_DATABASE_URL` 和 `OCCCCAD_TEST_GEOMETRY_WORKER`，要求专用数据库/真实 Worker 的可选 Go 集成测试按既有规则跳过；标准单元测试通过不表述为这些集成测试已重新通过。本轮未清理或重置开发数据。

验收待办已移出计划；M4 稳定拖拽、M5 冲突解释和 M6 工程连接仍是后续工作，不因本次验收完成而视作交付。

## 六类公共定义、精确支持与生命周期

新建和显式编辑写入唯一公共 `definitionVersion=2` 的 `family/subtype`：Coincidence、Contact、Offset、Angle、Fix、FixTogether。Concentric/Distance/Parallel/Perpendicular/Rigid 是编译原语或快捷入口；Parallel/Perpendicular 归 Angle，双体 Rigid 不冒充领域固联组。生产能力声明来自 [assemblycontract](../../../services/internal/assemblycontract/catalog.json)，服务端查询/精确验证与 Web 能力适配消费该来源；[测试目录](../../../tests/assembly-contract/catalog.json)关联声明及证据，不以报告 PASS 数量开放功能。FACE/EDGE/VERTEX 仅是拾取类别，未精确解析不能默认当作 Plane/Line。服务端校验仍是权威，完整 occurrence、已接受 Revision、稳定源及显式派生角色参与解析。

精确描述符现有八类，长度/半径/坐标为 mm，角度/锥半角为 rad，旋转是右手正交帧；Quantity 源值保持 SI，只在公共数量编译边界转换。Point–Curve 明确覆盖无限 Line/underlying Circle；Point–Surface 为 Plane/Cylinder/Sphere/选定叶 Cone，不声明任意曲线或曲面。修剪 Arc 使用 underlying-circle 须显式选择并保留参数域/provenance。球心、圆心、圆柱轴、锥顶/锥轴以及 Frame 原点/轴/平面使用稳定源加派生角色，不保存 mesh 推测或拓扑数组身份。Frame–Frame 是完整相对位姿六秩；其他 Frame 组合显式使用子元素。嵌套刚性 Product 在 owning Product 边界应用一次变换，共享 Part occurrence 不共用放置或支持身份。

只读支持 inspection 分离 `status=RESOLVED` 与 `constraintEligible`/适用性诊断。修剪圆弧仍保持 CIRCLE 类型和原参数域，但未显式选择 underlying-circle 时不能用于完整圆数学关系；[公共编译与 inspection 共用判定](../../../services/internal/workspace/assembly_public.go)，Web 只对实际派生目标检查显式适用性，未知响应禁止 Preview/确认，不复制 2π/容差算法。来源半段的修剪诊断不阻塞合法圆心/轴/平面或 underlying-circle 派生，也不把不适用支持伪装为 Broken。真实导入球面 seam 的[完整链路回归](../../../services/internal/control/assembly_arc_eligibility_integration_test.go)覆盖非法提交无 Revision/solve、显式整圆的实际运动与独立几何、参数域冻结、Undo/Redo、冷 replay 和 Release 后激活状态隔离。

Contact 的 11 个解析分支通过 [contact helper](../../../kernel/assembly/src/contact.cpp)进入同一 native 求解/解析 Jacobian，而非零距离近似或显示网格碰撞：Plane–Plane face、Plane–Cylinder line、Plane–Sphere point、Cylinder–Cylinder line/face、Sphere–Sphere face、Sphere–Cone ring、Sphere–Circle ring、Cone–Cone line/face、Cone–Circle ring。一般独立秩依次为 3/2/1/3/4/3/3/3/3/5/5；常量半径/锥角不兼容、材料侧不兼容和可恢复初态退化分别诊断。Plane 法向已经是材料外法向，不重复乘符号；其他曲面规范径向法向乘材料符号，External 相反、Internal 同向。整圆支撑不额外虚构材料相切门；Sphere–Sphere face 不是球球外切。精确公式/退化见[目标合同](../target/assembly-constraints.md#contact)，实现公式与微分见[求解算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md#51-解析-contact)。

多成员 Fix Together 保存稳定组 ID、名称、成员/嵌套组及捕获关系，不是无管理的 N 条 pair Rigid。[组编排](../../../services/internal/workspace/assembly_groups.go)冻结确定性的 `GroupStages`，重叠成员进入共同内部阶段、循环明确拒绝；先用成员内部有效约束求解，再从成功内部结果构造组关系，最后参与外部求解。关系变更/成员变更显式触发重新捕获，普通读取不重新捕获；组停用不删除或停用内部独立约束。内部或外部数值/偏好失败不晋升组捕获证据和候选姿态；完整阶段、成员、内部约束、关系与 solver build/result digest 进入 manifest/result，冷重放不查询新 Head。

数量参数统一为可选 `quantityParameter`，复用同一 checked AST、维度/循环检查与稳定参数引用：Offset 为长度，FREE/DIRECTED Angle 为角度，稳定 ID 分别沿用 `offset:<id>`、`angle:<id>`。Worker 投影是 mm/rad，源 Quantity 是 SI；旧 `offsetParameter` 只在旧记录读取时保留，显式编辑迁入同一 v2 定义，不双写两套活跃模型。参数、关系、模式和支持替换在同一命令原子提交；无数量的关系拒绝表达式参数，Measured 保留 Driving 源值/表达式而不改位姿或 Fix 基准。

产品约束具有独立 `suppressed`，与 `mode` 正交。`SET_ASSEMBLY_CONSTRAINT_STATE` 支持单个/批量停用、恢复及适用角度/距离量的 Driving/Measured 切换；Contact/Fix/组的不可用模式在能力查询中禁用并由服务端拒绝。实体 PropertySlot 记录完整状态，CAS、幂等与补偿历史复用正式命令路径。原模式和诊断在停用时保留；重新激活按已接受的 Part Revision 解析，不自动接受新 Head。树菜单、约束编辑面板、Inspector 与视口灰色标识可查看/切换状态。

SolveManifest 同时冻结完整 `definitions` 与实际编译的约束。全部停用时仍记录空活动集合的求解与可重放结果；全部活动定义被隔离时可记录已接纳空集合的结果，但完整定义仍保留失败状态，不表示全部约束满足。断链定义保持 Broken，其余可解析约束仍可求解。Update Plan/Release 排除停用项的 Verified 要求，旧 Release 的停用状态不随新 Head 激活而改变。Product Undo/Redo 恢复事务的原始位姿、定义与评价状态。已 Verified 的快照在副本上验证并生成新求解证据，禁止借验证移动组件或重写历史字段；包含活动未解决定义的历史快照直接保留原失败结果，重新计算是独立 Domain Command。

新增约束 identity 按 request ID 确定，浏览器保存 preview→request 对应关系；可复用的预览提交会冻结指向新 Revision 的 COMMIT manifest 与已验证结果，不重复求解，也不让 Release 依赖 PREVIEW 记录。支持顺序、精确来源、方向、模式、数量定义、快照及 CAS 任一变化都会使旧 candidate 不可晋升；当前 policy/build 见上文 v10，旧阶段记录保留自己的版本。

Angle 的 `angleRelation` 提供 FREE（默认无轴空间角）、DIRECTED（指定轴投影角）、PARALLEL、PERPENDICULAR 四种模式。两种数量角均接受0–360°；FREE 使用真实叉积范数/点积，不冻结旋转轴，正常构型控制一个转动自由度，0°/180°/360°驱动端点按方向对齐控制两个。无轴角保存第二组件局部坐标中的 `spatialAngleBranchDirection`，用叉积相对于该分支的符号区分正反解；分支只选择夹角扇区，不投影法向或添加对齐方程。首次从名义姿态建立，成功求解后沿已接受姿态运输，冻结到 manifest 并随 Undo/Redo 恢复；大于180°选择相反解，重复更新不翻回。DIRECTED 要求显式独立来源的稳定 `angleAxis`（可以属于第三 occurrence），可用 `reverseAngleAxis` 反向，沿既有 Datum/Publication/PersistentSelection 精确解析，冻结 `angle_reference_geometry` 的 owning body；该 body 进入 connected component 和解析微分变量。旧 `angle_reference_direction` 仍按第二 body-local 历史语义读取，不因选择交换悄悄换参考轴；反轴或交换按目标角规则显式转换，并将 `ANGLE_AXIS` 证据写入 manifest；仅约束投影方位角。切换到其他模式会清除旧轴及缓存方向，Undo 恢复完整定义。平行、垂直有独立工具栏入口；垂直以 `directionRelation` SAME/OPPOSITE 保存90°/270°意图，二者编译为带分支的空间角方程，实际选择相反姿态且不锁定轴。360°在求解提交时规范为0°。Undefined 不再永久改写为 Same。Offset 数值路径新增 Point–Axis 与 Axis–Plane，均有解析 Jacobian；零点点/点线偏移编译为重合方程，避免零范数梯度丢失其实际秩。Measured 输出独立 `measuredValue`，不改驱动值；两非平行平面等无有效常量距离的构型不显示旧数值。

Fix 新增 SPACE/RELATIVE 基准：相对固定接受显式移动后的名义位姿，空间固定保留捕获位姿。编辑命令支持显式 `fixedPose`，前端提供模式切换、三个位置与三个角度编辑。角度显示采用依次绕 X/Y/Z 的外禀旋转，持久化仍用 quaternion；相对固定的显式 pose 编辑同步更新 nominal placement，补偿历史同时恢复基准与 placement。

验证入口：[生命周期单元测试](../../../services/internal/workspace/assembly_lifecycle_test.go)、[0–6 阶、3+2+1 及常见关节有限运动测试](../../../kernel/assembly/tests/assembly_solver_scenarios.cpp)、[真实 Router/Worker、历史与 Release 集成](../../../services/internal/control/assembly_motion_integration_test.go)、[浏览器生命周期场景](../../../web/apps/cad/browser/assembly-lifecycle.spec.ts)。浏览器场景使用 Mock adapter 验证交互；权威计算测试走真实 Router/Worker 与独立测试数据库。单项/批量激活、Measured 恢复、停用后移动、连续 Undo/Redo、空活动集重放、Release 后再激活、相对/空间 Fix 位姿行为已有真实链路回归用例。2026-09-30 文档核对确认代码和测试入口存在，未重跑这些测试；此处未列出这些新增用例的逐项执行记录，不能借 ACCEPT-PRODUCT 的较早记录推断它们已执行通过。

上述新增实现不以旧阶段测试入口或 ACCEPT-PRODUCT 验收代替本轮 CONSTRAINT-COMPOSITION 的实际结果。目录逐 capability 区分实现层、专用数学/领域/UI/真实数据库证据及未运行/阻塞；本轮整体自动化收口以派生报告为准，不在此永久维护另一张状态矩阵。维护者只明确反馈本轮之前 OFFSET 改造人工使用验证通过，此反馈不能扩大到新 Contact、固联、全部来源/历史组合；本轮新增交互仍待维护者实机验收，未运行浏览器测试。M4 最近可行拖拽、M5/M6 均未实施；TREE-03 编辑会话不是约束求解 Session。

### 嵌套支持与可修复的约束状态

装配几何引用携带既有 typed `InstancePath`，按当前已接受的逐层 Product Revision 查找叶 Part 的 PersistentSelection；精确点/轴/面转换到直接子 Product 的 body-local frame。根装配求解仍将子 Product 视为刚体，不改变子 Product 内部自由度。几何键、显示名与网格编号不作为持久 occurrence 身份；路径消失产生 Broken。

状态采用 CATIA 手册 `cfyugasm_C2/cfyugasmut1500.htm`（Analyzing Constraints）与 `cfyugasmut0316.htm`（Inconsistent or Over-constrained Assemblies）的区分：Verified 表示满足；Broken 表示支持引用失效；Impossible 表示单个约束与支持几何不兼容（例如不同半径圆柱表面重合）；NotUpdated 表示待更新或当前组合尚未解出，包含冲突、过约束与不收敛。数值失败不能证明几何不可能。重复但满足的冗余约束仍可 Verified，并保留 rank 冗余证据。

添加、编辑、删除、抑制/激活允许保留可修复定义与失败诊断；抑制项、Broken、Impossible 及未获接纳的 NotUpdated 定义不进入已解方程集合。定义修改、激活/抑制、删除与显式重算会重新检查并尝试接纳；拖动只求解已接纳集合，不重试隔离项。失败试算不提交候选姿态；Undo/Redo 可恢复未满足定义；Release 仍要求所有活动约束 Verified。网络/Worker 不可用等可重试故障不伪装成模型结果。

无向对齐保留同向与反向两个解，残差/Jacobian 在同一当前分支求值。如果方向残差在错误半球相互抵消，最多尝试16个替代半周初值，保留原 nominal、Fix、Rigid 和所有约束；收敛才接受。该有界数值策略不宣称证明所有非线性系统可解或不可解。

偏好优化的普通 BFGS 方向若不能下降，会在参考目标为零的层级上使用约束流形的 Lagrangian 曲率作为第二搜索方向；保留几何恢复、参考优先级、能量回溯及原始收敛容差，解决长力臂圆柱同心约束的微小曲率停滞。无向关系切换边界的差分 oracle 固定基点局部分支，避免跨不连续点的中央差分被误判为解析 Jacobian 错误。

Product 与 Part 补偿冲突检查使用实际最近 REVERT/REAPPLY 结果 Revision 的字段作为预期值，恢复目标仍来自原命令 before/after；Part Publication Redirect 的 Undo 会重新评价来源，因此不能用原始 before 中的旧评价 Revision 作为 Redo 前提。后续第三方或用户业务字段、评价字段修改仍产生 CHANGESET_CONFLICT。不修改历史记录或清空 Undo/Redo 栈。

### 已解集合与待接纳定义

装配求值先验证既有 Verified 集合，再按持久约束顺序逐个试加入新增、修改或待恢复定义。某项与已解集合冲突或不收敛时，仅该项记为 NotUpdated 并隔离；此前已解项保持 Verified，之后独立项仍可接纳。如果上游几何变化使原集合整体失效，则按持久顺序重建已解集合。该策略是本系统的确定性接纳顺序，不宣称 NotUpdated 等价于证明“无解”；可满足的冗余约束仍由数值求解器判断。

鼠标拖动的 MOVE_INSTANCE（含 preview/commit）只使用已接纳方程和交互 driver。隔离定义不参与支持解析、方程、自由度计算或测量更新；拖动不可为了成功而丢弃已有 Verified 约束。抑制/激活、删除、修改或显式重算会再次尝试隔离项，恢复后重新参与运动。

每次接纳试算保留命令入口的 nominal pose；失败不污染姿态、warm start 或其他约束状态。网络、取消与基础设施故障使整次操作失败，不记为约束冲突。试算使用独立请求及 `PROBE` SolveManifest，最终 COMMIT/PREVIEW manifest 同时记录全部定义和实际接纳的方程集合；Release 不使用探测试算证据，活动隔离项仍阻止 Release。

嵌套 Part 的基准面、轴系原点/方向和自定义基准轴在视口拾取时与实体拓扑一样携带完整 InstancePath；直属 InstanceId 仍标识参与运动的子 Product，路径末端标识基准所属 Part 的已接受版本。支持检查和求解描述符读取统一规范化 Part 默认基准面/轴系，隐式默认基准可解析，删除的自定义基准仍返回 Broken。定向回归入口为 `nested_product_update_test.go`、`product-edit-context.scenario.mjs` 和 `nested-datum-selection.scenario.mjs`。

## STEP Definition / Occurrence 交换

STEP/XDE 导入复用既有 ProductInstance 与 typed InstancePath：每个源 Part/Product Definition 对应一个文档，多个 occurrence 通过 PINNED Revision 和独立 local placement 引用；BREP 不包含实例变换。图仅为导入/导出中间表示。同级源 Instance 名称冲突时按确定性后缀维持内部唯一性，`importedName` 记录源名与分配名；没有用户重命名时，STEP 导出恢复源名。命令历史保存完整 typed instance entity，Undo/Redo 保留名称来源及 pose。具体边界与验证见[交换架构](jobs-artifacts.md)。

## 六类约束可执行合同目录

CONSTRAINT-CONTRACT 交付共享 [catalog.json](../../../tests/assembly-contract/catalog.json)（schema 1、当前 `assembly-six-families-v2`；初始交付为 v1）、[执行器](../../../tests/assembly-contract/runner.py)和[使用说明](../../../tests/assembly-contract/README.md)。稳定 capability 与具体 case 按稳定 ID 组织，目标语义、各层实现声明、测试映射和实际 verdict 分离。Coincidence/Contact/Offset/Angle/Fix/Fix Together 是用户族；Concentric/Distance/Parallel/Perpendicular/pair Rigid 仅作内部映射，不增加新的公共用户族。Contact 解析秩及有限 Curve/Surface 子类已按目标公式冻结；测试不得用当前输出反写目标预期。

执行器按 capability/family/layer/case 选择，读取唯一目录，调用现有 C++ GTest、Go 包测试与 TypeScript 规则；Go/TS 新 adapter 也直接读取它。C++ 新断言补充六种非零 Offset 的解析距离、秩和选择交换，空间角端点的独立几何检查，以及指定轴0°/90°不等于空间平行/垂直、交换支持/反转轴的角度变换。既有 3+2+1、0–6 秩、六种关节有限运动、冗余和子空间 projector corpus 通过目录映射复用，不复制求解器。

`baseline` 只检查已纳入既有验证基线的 case；`gaps` 另执行目标断言。回归锁固定目标/案例、实现声明下限与既有断言源摘要；目录/fixture/测试引用不完整、空选择、零执行或断言放宽会失败。报告自动生成 JSON 与简明摘要，记录 commit/工作区、输入摘要、命令/环境、预期/观察、测试证据层级与剩余任务。源码/UI 检查、纯模型/编排 test double、真实 kernel 数学测试和 Router/Worker 集成各有明确证据种类，不能互相代替。

2026-10-01 实际执行基线：`main / b8b07fcdf16e366079b14f4d5bf38f43dc0fa6b5` 加本次未提交的目录/adapter/测试改动。目录/回归锁校验通过，10 个工具完整性测试通过。指定当前 Debug Worker 后，既有能力 `baseline` 为 **85 PASS、1 ENVIRONMENT_BLOCKED、1 NOT_RUN**，去重后 83 个通过测试映射；NOT_RUN 是下述目标专用断言，非跳过后计为通过。C++、Go（`-count=1`）、TypeScript 均实际执行，正式 Router→Worker 测试通过。全目录 `gaps` 为 **85 PASS、1 FAIL、1 ENVIRONMENT_BLOCKED**；每个通过 case 的具体范围、期望、观察与日志由执行器报告，不在 Markdown 手工复制全矩阵。

上述 CONSTRAINT-CONTRACT 执行时的差异是 `offset.plane-plane.first-normal-editor`：第一选择法向目标与第二法向编辑入口不一致。CONSTRAINT-OFFSET 本批已保持其目标与断言不变地修复，并补充下述真实数学/领域/数据库链路；旧执行结果不是当前仍失败的声明。

CONSTRAINT-CONTRACT 当时未配置专用数据库，历史/Release case 是 ENVIRONMENT_BLOCKED，不能借本批结果倒填当时的验收。未实现项的明确拒绝测试通过不等于产品能力通过。该阶段曾缺少 Contact、Frame/派生几何、明确 Point–Curve/Surface 与组内先解；本轮新增实现见上文，不能倒填当时结果。共享基础测试不等于具体组合验收。完成 CONSTRAINT-CONTRACT 不等于 CONSTRAINT-COMPOSITION 或 M4 完成，MOVE 仍用 `interaction-driver` Fix。

### Offset 有符号纵向切片

下述切片与平行 EDGE 小节保留各自实施时的版本和实际测试记录；最新公共 v2/Quantity/v10 实现及本轮原生验证见上文和末段，不把旧能力统计当作当前覆盖状态。

实现基线 `main / 4ceda79` 加本批工作区（2026-10-01）。新增符号意图 `SELECTED_PLANE_NORMAL_V1`，精确支持经 [Workspace 编译边界](../../../services/internal/workspace/assembly_offset.go)验证后保留端点、值和 moving/reference 顺序，经既有 Worker string 字段映射到 native `SelectedPlaneNormal`。公式是 `d = n·(p_first-p_second)`，双平面选第一法向，否则选唯一平面；Datum 是持久法向，FACE 是解析后的材料外法向，按完整 occurrence 帧变换。双平面 Same/Opposite 与符号正交，Undefined 不被回填为 Same。无平面保持非负无限支撑距离；原六对方程、零距离特殊秩和解析 Jacobian 复用。真实 Same→Opposite 用例发现错误半球驻点，新增符号双平面 Driving 复用既有 cluster seed 探测，不改 nominal、运动偏好、容差或求解层级。

`AssemblyConstraint.offsetParameter` 是约束自有 ParameterDefinition，稳定 ID 为 `offset:<constraintID>`。Literal 使用明确 mm→Quantity SI 转换，表达式保留 checked AST/稳定引用，只消费同一 Product 的其他 Offset 自有参数；投影 `value` 是 Worker/placement 使用的 mm，不用名字猜单位。复用已有数量维度、循环及原子命令检查，最小补上解析器一元正负号（仍为既有 AST）。重命名只更新表达式展示，依赖值与 ChangeSet 补偿同步；删除被引用定义会明确失败，不默默断开。Preview/创建/编辑共用 [Web 意图适配](../../../web/apps/cad/src/cad/assembly/assembly-offset.ts)，表达式、方向、模式与定义原子提交；Inspector 分开显示驱动值/表达式与测量值。

Measured 不发驱动方程；双平面测量仅要求实际法向平行（任一朝向），不改保存的 Driving 方向/Quantity。独立 `measuredValue` 使用同一符号定义，无有效常量偏移或被隔离时先清旧值；恢复 Driving 或解除 Suppressed 保留原模式与驱动源。实际非平行测量测试确认无旧值、无伪零、Fix 不变。目标 Offset 被隔离为 NotUpdated/Broken 时，即使已接纳子集 Converged 也不生成可晋升 PreviewID；允许保存失败定义的既有提交合同不变。Debug 数值 replay 是最终已接纳子集，失败定义和试算证据在 SolveManifest/结果中分别保留，不把两层状态混为一谈。

持久兼容边界：新字段为可选加法，旧 Revision 的空/UNSIGNED/ALONG_SECOND_NORMAL/OPPOSITE_SECOND_NORMAL 不换端点、不取负、不重标版本；旧编辑器回填保留旧约定。有符号切片使用 v8，平行线恢复后新 manifest 使用 `assembly-offset-parallel-line-v9`，schema 1/profile 2 不变；v7/v8 的冻结输入仍可读取和数值重放，v7 拒绝伪装新符号/新参数定义。重放由当前 Worker 执行并记录真实 solver build，不保证旧可执行程序的逐位结果，不修改原 Revision/manifest/Release。Release 冻结完整定义/激活/支持与数值输入。没有数据库迁移、重置、批量历史重写或应用制品重建。

执行记录与入口：`runner.py baseline/gaps --family Offset`，源/预期/观察/每次真实 verdict 在派生 `build/offset-after-{baseline,gaps}/report.json`；定向 Router fixture 为 `TestOffsetSignedProductHistoryThroughRouter`。本批显式创建并使用可丢弃数据库 `occccad_offset_contract_test` 与当前 Debug Worker，数据库保留，应用开发库和制品未清理。真实覆盖旋转/非零平移、负/零/正、Same/Opposite/Undefined、法向反转/选择交换、单平面两位置；数据库覆盖共享多 Body Part 的两个 occurrence、第二 CAD Body 材料面、嵌套 Product 双层旋转、Preview/提交、跨零表达式、稳定引用重命名/UndoRedo、Measured/抑制恢复、冷读、manifest 与 Head 变化隔离的 Release replay。三个 router case 共用一个 fixture，不冒充三次独立验收；模型/源码/UI 与真实几何、数据库证据保持分层。

有符号首切片执行结果：目录/锁通过，12 个执行设施测试通过；Offset baseline 与 gaps 均 **47 PASS**（44 个去重映射），受影响的六族合同 baseline **98 PASS**（94 个去重映射），没有环境跳过冒充通过。报告的能力覆盖仍为 44 PARTIAL、14 TARGET_NOT_IMPLEMENTED，Offset 六项均 PARTIAL。`invoke check --scope assembly` 四个步骤通过（C++ 构建/CTest、Go、Web 场景），TypeScript 无输出编译与共享 modelcore 包测试通过，`invoke context-audit` 通过。旧共享数据库历史测试的错误“抛错/数值子集失败”断言已替换为上述完整失败定义/试算隔离与无候选/无位姿/无 Head 变化断言，并真实通过；锁变更理由见测试 README。后续平行 EDGE 修复记录见下节，前述数字不是新增 case 的执行证据。

报告按必需层与适用专用测试推导实现覆盖、执行完整性和验收状态；共享基础/拒绝测试不能认证组合，缺测/未运行/阻塞仍显示缺口。当时双平面 UI/domain 仅按已交付的窄链提升，resolution/lifecycle/history 完整组合仍 partial，不把该首切片当作整个 Offset 已完成。该阶段列出的六组合精确 UI、来源恢复和生命周期缺口已进入本轮实现/目录验收，当前收口记录见下节，实机交互仍待验证，人工出口见 [CONSTRAINT-OFFSET](../../../plans/assembly-evolution.md#constraint-offset完整偏移与测量模式)。未运行浏览器、无差别全量单测、性能/容量基准；此旧阶段未实现 Contact/Fix Together；本轮已新增前两者，M4/M5 仍未实施，TREE-03 与数据面边界不变。

### 平行 EDGE 距离恢复

维护者报告两个 Part 插入 Product 后选两个边线做 Distance 被隔离为 NotUpdated。只读核对应用库试算输入，确认 EDGE 已精确解析为 AXIS，典型目标 30 mm，初始无限线距离约 28.284271 mm；两个偏离原点的平行线支撑，reference 已旋转且非零平移。这不是 UI 文案、符号转换或解析失败。旧 native 试算复现 NON_CONVERGENT；修复初值后又确认几何可行不等于偏好收敛，没有将二者合并或强行 Verified。

[solver](../../../kernel/assembly/src/solver.cpp) 增加径向初值恢复；孤立非零无符号平行线 Distance、一个自由 cluster 的偏好迭代/可行性校正使用局部平行 chart，避免跨入不光滑的异面线距离分支。chart 不进入物理方程、秩/DOF 或持久 Constraint，也不是隐藏 Parallel/Fix，不修改 nominal、容差或 moving/reference。证据是局部选定分支上的偏好收敛，不是跨分支全局最短运动。此阶段使用 v9；本轮在其基础上加入零目标/显式平行耦合与尺度修复，新 policy/build 为 v10，旧记录不重标版本。

新增 native 12 个目标 10/30/50、交换/Fix 构型，独立最终无限线距离、reference 不动、偏好收敛及物理秩 1/相对 DOF 5 均断言。新增 [真实两 Part 集成](../../../services/internal/control/assembly_edge_offset_integration_test.go) 使用各自生成的 B-Rep 和持久 EDGE 引用，经 Router/Worker/专用数据库完成 Preview、提交 30、编辑 35、UndoRedo、冷读与独立几何验证。两个专用 case 进入同一目录和基线；没有把数学层当作完整 UI 验收。

实际执行（同一 `4ceda79` 加本轮工作区）：目录/锁与 12 个设施测试通过；Offset baseline/gaps 各 **49 PASS**（46 个去重映射），受影响六族 baseline **100 PASS**（96 个去重映射），能力声明仍为 44 PARTIAL/14 TARGET_NOT_IMPLEMENTED。记录在 `build/edge-offset-verified-{baseline,gaps}/report.json` 与 `build/edge-offset-affected-baseline/report.json`，环境为显式 `occccad_offset_contract_test` 和当前 v9 Debug Worker，没有集成 skip。首次并发重链接 Worker 导致 6 个映射启动失败，原记录保留在 `build/edge-offset-baseline/`；完成构建后重跑上述正式检查，不把启动失败计为产品通过。`invoke check --scope assembly` 四步骤、定向 Workspace Offset、`invoke performance-baseline --count=1`、`invoke context-audit` 和 diff 空白检查通过；性能采样不作容量或跨版本无退化保证。未执行浏览器与无差别全量单测。

该阶段探索性零目标偏好停滞已在本轮原生回归处理：非平行零目标保持精确交线方程；平行零目标使用限定局部 chart；同支持的显式 Parallel 使用等价位置残差，未隐藏附加平行/Fix。`ZeroIntersectionIsExactAndInvariantUnderSupportOriginChanges`、`ZeroDistanceAllowsFiniteIntersectionRotationWithoutHiddenParallel`、`ZeroDistanceAndExplicitParallelKeepCoupledRankAndPreference` 和 `ZeroDistanceCannotHidePhysicalFixConflict` 独立检查原点改变、有限转动、耦合秩/偏好及物理 Fix 冲突。177 项 native 与 21 项 corpus 本轮实际通过，正式输出 `build/constraint-composition/native-composition-final.xml`、`native-corpus-final.xml`；其中六关节有限运动为每类 13 个解析有限姿态和一次受阻恢复，共 84 次真实求解，不仅检查瞬时零空间。此证据不替代数据库/来源/Release 或人工验收，整体状态由本轮合同报告给出。应用库无清理，未运行浏览器。

### CONSTRAINT-COMPOSITION 本轮收口

核对/执行基线是 `main / c1becfdc2a569f14468f29ca2b643b303f005213` 加本轮工作区，公共 definition v2、contract `assembly-six-families-v2`、schema 1、统一 Debug Worker v10。专用可丢弃数据库为 `occccad_offset_contract_test`，真实 Router→Worker 与数据库事务执行，不使用应用开发库或 Mock 替代。报告记录执行输入/断言摘要、准确 Worker 可执行文件 SHA256、命令与证据类型，且不记录数据库凭据。

初次完整 `gaps-verified/report.json` 已实际 **610 PASS，332 个去重映射，58 个 capability 实现层及必需专用证据齐全**，无环境阻塞或 skip。新增案例据此进入同一 baseline 锁，原 100 个 case ID 保留；动态组秩仍为 `6·(N−1)`，DAG、重叠阶段及全 incident bodies 的内外边界已冻结。最终交叉审查补齐圆弧适用性门禁，另 7 个 case 在 `arc-targeted/<caseId>/report.json` 实际通过后纳入基线；完整目录为 617 项。最终独立重跑结果使用 `build/constraint-composition/final-{baseline,gaps,composition}/report.json`，未生成的报告不推定通过，不另维护手工全矩阵。

最终实际结果：上述完整 **baseline/gaps/composition 各 617 PASS、335 个去重映射、58 个 capability 自动 ACCEPTED**，无 FAIL/NOT_RUN/MISSING_TEST/ENVIRONMENT_BLOCKED，执行输入摘要与收口源码一致。`GOFLAGS=-count=1 invoke check --scope assembly --verbose` 四步骤通过，日志 `build/constraint-composition/final-assembly-domain.log`：177 scenarios + 21 corpus 共 198 CTest、Go geometry/workspace/control、Web assembly 10 场景。另实际执行 20 项目录/runner 完整性测试、全 Go 包 `-run '^$'` 编译检查、api/modelcore/assemblycontract 包、TypeScript/生产构建、TREE-03 相关四场景、context-audit 与 diff 空白检查。`invoke performance-baseline --count=1` 只保存当前样本，不认证容量或跨版本性能无退化。

共享几何回归为 37 PASS / 1 SKIP，`GeometryExchange.ImportedSolidRepairCorpus` 缺少 `OCCCCAD_TEST_IMPORT_BREP`，原日志保留于 `geometry-shared-final.log`。Go 装配领域检查的两个通用大型 STEP/传输健康用例也因未配置 `OCCCCAD_TEST_EXCHANGE_STEP` 未执行；不能由包级 PASS 推断大模型容量通过。它们均不属于这 617 个合同 case；本轮合同内真实数据库/Worker/历史没有降级或跳过。

真实链路覆盖四族 47 个明确几何子组合、逐 Contact 分支创建/编辑/来源恢复和历史、Datum 参数更新、共享多 Body Part 的不同 occurrence 与嵌套 Product、独立第三角度轴、SPACE/RELATIVE owning-frame、2/3/N 组及内外求解、单项/批量激活、Measured/全停用空集合、Broken 重解析、Preview 身份/CAS、Undo/Redo、冷重放和 Release 冻结。具体 case 与执行范围以派生报告为准，不把代表性来源场景解释为任意模型、任意来源变化的笛卡尔积验证。

首轮真实失败报告 `gaps/`、`gaps-final/` 及定向复现日志保留：显式 Contact branch=0 曾被默认成合法分支；Part Publication Redirect 的 Redo 前提使用了旧评价字段；第三轴位于组外时曾错误进入内部阶段；DIRECTED 结果曾污染切回 FREE 的扇区。修复真实生产路径后重新验证，保留非法输入拒绝、严格历史冲突检查、原测量期望和物理容差。旧 Offset 回归测试适配唯一 Quantity v2 字段，Preview 保留同一 request ID；旧 Multi-Body 回归按既有 NEW_BODY 命令测试新建独立 Body，未借此改变 CAD Body 生命周期。

维护者反馈此前 OFFSET 使用验证通过，是维护者反馈而非本次 Agent 浏览器执行；未提供人工准确版本、日期或逐项清单。本轮新六族/Contact/固联交互仍为 `PENDING_MAINTAINER`，自动报告明确限定 `AUTOMATED_CONTRACT_ONLY`，人工场景和检查步骤见[合同设施 README](../../../tests/assembly-contract/README.md#维护者实机验收)。未运行浏览器、无差别全仓单测、容量验收或 M4/M5/M6；没有清理应用数据库或 S3、迁移重写历史或批量重建制品。
