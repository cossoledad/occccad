# Product 关联设计、装配与发布

> 2026-09-21 文档核对基线。返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。ACCEPT-PRODUCT 已完成，人工与自动化证据及限制见本文末尾。

- 当前 Product UI 中的新实例固定采用 `FOLLOW_HEAD`；被引用文档变化后先使 root-snapshot `ProductUpdatePlan` 失效并投影 `UPDATE_AVAILABLE`，打开 Product 的可编辑客户端随后按叶到根自动接受每一级带 digest 的计划，提交普通 `UPDATE_REFERENCES` Revision 并重建可重放快照。Instance 右键可把当前 resolved Revision 切换为 `PINNED`，也可恢复 `FOLLOW_HEAD`；任意历史版本 picker 尚未开放。Instance 右键“在新标签页中打开”显式读取所引用文档以登记服务端 open document，更新并刷新 Tab 列表后通过工作台内部文档 Tab 打开所引用的 Part/Product 文档（当前 HEAD），已打开的文档直接切换到对应 Tab；嵌套实例同样打开其 Reference Document，不改变原装配的固定版本。

Product 工作台维护浏览器会话级的 Active Occurrence 编辑上下文，由 `Active InstancePath + Reference Document` 共同表达。打开 Product 时根 Product 默认激活；双击结构树中的 Product、Part 或 Instance 节点只激活该 occurrence，随后 Toolbar、属性、历史、Undo/Redo 和 Domain Command 绑定其 Reference Document 的 `main` Workspace。激活不是模型命令，不写 Revision；同一 Part Reference 的其他 occurrence 不显示为激活，但编辑 Reference 后全部 FOLLOW_HEAD occurrence 都会解析到新结果。视口始终保留根装配；活动 Part 的草图、基准和命令预览施加该 occurrence 的世界 Placement 后就地编辑，其他部件继续显示。

根 Product 场景与 Active Part 交互视图由 `ViewportEditContext` 分离，命令/草图选择只读取激活 Part。Publication 与基准几何的显示/拾取规则见[Web 交互](web.md)。

`INSERT_INSTANCE` 继续把既有 Part/Product Reference 插入当前激活且具有 Editor 权限的 Product；InstanceName 由服务端在 owner Product 当前候选模型中按 `ReferenceName.N` 分配首个同级可用名称，显示名不参与身份，重命名属于独立属性命令。

`INSERT_INSTANCES` 可一次插入最多 128 个不同文档，或以一个已有直接子 Instance 为源沿当前 Product 的世界 X/Y/Z 轴复制。阵列数量含原实例（2–128），间距以毫米指定，反向开关改变轴向；原实例不变，新实例保留其引用版本、引用模式与旋转，各自获得新的稳定 ID。新实例名称以被引用文档名而非源 InstanceName 为基底，从同级首个可用序号分配，例如 `Part1.1`、`Part1.2` 后得到 `Part1.3`。API 对批量文档引用逐一验证读取权限并阻止 Product 循环引用；整个批次作为一个 Domain Command/ChangeSet 写入一个 Revision，可整体 Undo。Web 插入浏览器多选跨搜索和分页保留选择，“当前选择”页进入时冻结展示列表，在该页取消选择不会移除卡片，离开再进入才刷新。`product.pattern` 是独立 Toolbar 命令，使用非模态浮动面板；可先选或在面板打开后选择源组件，输入新增数量、间距、方向和反向，并在视口显示不可拾取的半透明副本，提交后由权威装配模型重建。交互参考本机 CATIA V5 [Defining a Multi-Instantiation](/mnt/s/tools/DS/Catia/B33doc/English/online/asmug_C2/asmugbt0312.htm) 的选组件、定义参数、预览与 Apply 流程；当前只实现三个坐标轴，不支持选择任意线/边为方向，也没有 Fast Multi-Instantiation。

Product 结构树的 Product 节点另提供“新建零件”。`POST /api/documents/{rootProductId}/part-components` 接收可选的 typed target Product InstancePath；服务端在事务外建立空 Part 初始 Revision 和目标/祖先 Product 候选，再以一个 `ProductDesignTransaction` 原子创建 Part 文档、插入 `FOLLOW_HEAD` occurrence，并从目标 Product 自底向上推进到 root snapshot。任一目标路径、权限或 Workspace Head 失效都会整体拒绝，不产生孤立 Part 或半更新装配。空名称按全局首个可用 `PartN` 分配，InstanceName 仍按目标 Product 同级的 `PartN.N` 分配；第一版只支持位于目标 Product 原点的 identity placement。Undo/Redo 补偿 Product 成员 Revision，因此 Undo 会移除 occurrence 并恢复祖先引用，但不会删除已经创建的 Part 文档；该文档保留在 Document Center，可再次插入或显式删除。结构树 capability 描述领域动作，实际入口仍由服务端对 root、目标和全部祖先 Product 再次执行 Editor ACL 校验。

第一版 typed `InstancePath` 保存 RootDocumentId 和有序 segment；每段包含 owner Document/Revision、稳定 InstanceId、显示 InstanceName、ReferencedDocumentId 和 resolved Revision。`canonical` 由 InstanceId 链构成并用于选择、激活和 occurrence 资源索引，`display` 才由 InstanceName 拼接。名称修改不得改变 canonical identity，也不得作为持久引用解析键。

## Publication 与受控外部引用

Publication 保存在 Part/Product Revision 中。Datum POINT/AXIS/PLANE/FRAME、PersistentSelection CURVE/SURFACE、Feature output BODY 与 PARAMETER 共用稳定 PublicationId、兼容版本、typed CRUD、ChangeSet、Undo/Redo 和 dependency graph；拓扑 target 随 Part 求值按 naming 重解，失效后保存 `BROKEN_PUBLICATION`。Parameter Publication 固定 value type、dimension、SI unit policy、可选 bounds 与 source ParameterId；消费 Part 的 `ExternalParameterRef` 保存来源 Document、FOLLOW/PINNED selector、PublicationId、期望合同与冻结 `ReferenceResolutionSnapshot`。DocumentView 对来源 Head 只投影 `UPDATE_AVAILABLE/BROKEN`，显式 Update References 在事务外解析完整候选并以 Workspace sequence CAS 提交，普通重算不读取来源 Head。Assembly endpoint 优先保存 PublicationRef 与 PersistentSelection deep link；兼容 Replace/Update 重连新 target，Product Publication 可转发一个相对 occurrence 的子 Publication。

## Product Design Session 与上下文事务

Product 内关联由 Product Design Session 组织。`InstancePath` 由 root 与有序 typed segment 构成，canonical identity 只连接稳定 InstanceId，显示名只形成 breadcrumb；递归展开设有 cycle、深度 32 和成员 10000 的 gate。Instance、Part/Product Publication 与 ContextInput 使用版本化 `nfkc-casefold-v1` 作用域名称规则，rename 只改变显示投影。Product Publication 可沿嵌套 rigid Product path 转发。

Part 现在声明不含具体来源的 typed `ContextInput`，root Product Revision 持有 source occurrence Publication 到 owning occurrence input 的 `ContextBinding`、相对 Transform 和 accepted resolution snapshot。创建绑定会在事务外解析当前 root snapshot、合同、权限与依赖环，再为消费 Part、全部嵌套 owning Product 和 root Product 预生成 Revision/ChangeSet/EvaluationManifest；数据库事务按 Workspace 稳定顺序锁定并统一 CAS，一次提交所有 Revision、Outbox 和 Head。该事务组从任一成员文档触发 Undo/Redo 时也统一补偿，不会只留下 Part input 或 Product binding。`ContextReference` 仅保留为旧试点模型与独立 Part/受控外部通道能力，新 Product 内 picker 不再向用户暴露全局 DocumentId/PublicationId。

Web 以 root Product、active occurrence 和 definition/context 模式维护非持久设计会话；`GET design-session` 与 root-snapshot-scoped `GET context-catalog` 只返回当前 Product 可达且调用者可读的 Publication，并按 expected type、连接状态和已知 ContextBinding DAG 过滤。参数与 Context binding 面板按 occurrence breadcrumb/可编辑名称选择来源；跨 Workspace 提交仍由服务端重新验证 typed path、Head 和合同。

## 更新计划与派生 Variant

`ProductUpdatePlan` 从不可变 root snapshot 投影 occurrence/reference 与 ContextBinding 影响项，分别报告 connection、currency、evaluation，候选 Context Variant 会把已接受/候选 Publication 描述转换到 owning Part local frame，再复用 Part 参数、Sketch 与几何 evaluator 生成独立 GeometryKey、Publication resolution 和 EvaluationManifest。Variant identity 只包含 base Part Revision、规范化输入快照及 evaluator/policy；不包含 binding 显示名、BindingId、occurrence path、WorkerId，PARAMETER 输入也不包含无意义的 occurrence transform，因此同一定义和输入的四个 Wheel occurrence 可共享持久 variant cache；不同几何/参数输入不污染共享 Part。accepted variant 会覆盖 Product `ResolvedInstances` 的 base GeometryKey，并沿嵌套 rigid Product Publication 转发进入装配 descriptor。Update Plan 的 occurrence 更新项只属于当前 Product 的直属引用边；子 Product 通过自己的计划提交新 Revision，再由父 Product 接受。候选直属版本变化后，计划先按候选子版本重新展开路径，避免用旧子树阻塞父级接受。Web 对 FOLLOW_HEAD 变化按 Product 依赖顺序从叶到根自动接受，先读取待跟随子 Product 的最新定义，再处理新定义中引入的待更新后代；共享子 Product 按依赖排序，PINNED 边阻断遍历。每轮后重新读取当前 Product，深度和更新轮数均有界；每次仍以 digest 防止接受过期计划。Release 独立检查整棵非 PINNED 跟随树的版本状态，不因更新计划只处理直属引用而放松发布要求；Context Variant 等上游候选求值失败仍会阻止当前计划；装配约束 Broken/Impossible 仅作为诊断，不阻止引用接受。`HasUpdates` 由引用/上下文变化决定，失效约束本身不触发自动更新循环；显式 Refresh 仍可重新解析求解。接受新 HEAD 后支持元素解析失败的约束保留 Broken 并隔离，其余约束继续求解；Release 仍独立要求活动约束全部 Verified。

## M3 SolveManifest 与 Release

每次正式 assembly preview/commit 冻结 `AssemblySolveManifest`：root/candidate Revision、完整 body pose、局部几何描述符、Publication/PersistentSelection resolution evidence、约束、branch/intent、affected scope、schema 2 solver profile 与 build policy 共同形成确定 digest。构造器按持久化 JSON 表示冻结独立快照，位姿、角度分支及 Publication/PersistentSelection 证据不再共享调用方指针；后续调用方修改不改变已冻结内容与 digest。Worker 只消费 manifest 中的纯值；Publication endpoint 直接使用已解析 descriptor，不再让 solver 查询 Product/B-Rep。manifest 与 request-specific result 持久化，重试复用同一结果，digest replay、request lookup、deadline/cancel 和既有 `.3dreplay` 数值证据并存。

当前保存独立 `ProductRelease`：Release Manifest 冻结完整 occurrence typed path/Revision/pose、ContextBinding、ContextVariant GeometryKey/EvaluationManifest、Product Publication、命名/evaluator policy、成功 SolveManifest 与 gate 结果。Gate 要求引用 current、全部 occurrence/variant READY、活动约束 Verified 且有可重放求解证据；停用定义保留在 SolveManifest 中，不伪造 Verified，也不参与此 gate。Release 可在 Workspace Head 移动后按 manifest replay，并从冻结 GeometryKey 提交 STEP/BREP 导出；Exchange placement 现已贯通 translation 与 quaternion rotation。Web 的“产品版本中心”只负责创建、列出和 replay 不可变里程碑，不再把 STEP/BREP 按钮混入发布流程；Exchange 保留为独立后续 UX。当前不把 Configuration/Design Table、partial update、flexible subassembly 或 Derive Part from Context 列为已实现能力。

## 求解与交互的当前边界

一次 `solveAssembly`（含 accepted/candidate 准入试算与最终求解）共享操作内读取缓存：按 document/revision 复用拓扑清单与嵌套 Product 模型，按 GeometryKey 复用制品元数据/B-Rep 引用，按 GeometryKey/type/local ID 复用精确属性。local ID 仅是已验证不可变制品内的查询键，不成为持久身份。命名解析仍检查源证据、目标 manifest 与 policy；已解析的目标直接查询精确制品，不再重复执行交互拾取的授权/绑定/重解析流程。文档存活性每次求解检查，用户权限仍由请求边界验证；缓存不跨请求、不保存 HEAD、根 occurrence pose、失败读取或求解结果，不改变准入隔离、名义姿态和结果持久化。

HTTP `phases_ms` 增加 `assembly-total`、`assembly-prepare`、`assembly-worker`、`assembly-manifest-write`、`assembly-result-read/write`，以及 `topology-manifest-read`、`topology-properties`、`topology-rpc`。同名阶段累加所有试算；阶段存在包含关系，不能相加当作总耗时。`assembly-worker` 包含 RPC/排队/求解，`topology-worker` 保留制品准备及 RPC 耗时，`topology-rpc` 单独计量 RPC；Worker 自身日志才是几何查询内部耗时。高延迟数据库下优先比较清单读取、准备和持久化与 Worker 阶段，而不是依据 `cache_hit` 推断端到端成本。拖拽前端仅保留一个在途权威预览并合并待处理姿态，因此每次 HTTP 延迟直接限制权威预览更新频率。跨请求缓存、批量预取和大系统数值优化尚未实现，应由这些阶段及 `.3dreplay` 测量决定。

`kernel/assembly` 只消费纯值 Point/Axis/Plane/Cylinder、约束和完整 SE(3) pose。Rigid cluster、Fix/Ground 消元与 connected-component 求解先于数值迭代；生产路径使用解析 Jacobian、augmented QR 和 SVD rank/null-space。M2.5 依次满足硬约束、最小化 reference motion、最小化总 nominal motion，独立报告偏好收敛与瞬时自由度。创建/编辑以第一选择为 moving、第二选择为 reference，Constraint 与全部 solved pose 在同一 ChangeSet 提交。Rigid 捕获当前相对位姿后允许 moving/reference 两个 occurrence 属于同一刚性组，reference 最小运动作用于共享组；既有约束仍逐项验证，真实不一致不会被固连掩盖。算法细节与 corpus 由[Solver Algorithms](../../../kernel/assembly/SOLVER_ALGORITHMS.md)维护。

当前 MOVE 仍注入临时 `interaction-driver` Fix；不可达预览恢复权威 pose，尚不能返回 M4 的最近可行拖拽结果。有效 preview candidate 可经 CAS 提升为提交，缺失/过期时重新权威求解。M3 已覆盖嵌套 rigid Product 与持久引用解析；flexible expansion、稀疏后端和最小冲突集尚未实现。

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

## 六类约束与生命周期的首批实现

产品约束新增独立 `suppressed`，与 `mode` 正交。`SET_ASSEMBLY_CONSTRAINT_STATE` 支持单个/批量停用、恢复及角度/距离量的 Driving/Measured 切换；实体 PropertySlot 记录完整状态，CAS、幂等与补偿历史复用正式命令路径。原模式和诊断在停用时保留；重新激活按已接受的 Part Revision 解析，不自动接受新 Head。树菜单、约束编辑面板、Inspector 与视口灰色标识可查看/切换状态。

SolveManifest 同时冻结完整 `definitions` 与实际编译的约束。全部停用时仍记录空活动集合的求解与可重放结果；全部活动定义被隔离时可记录已接纳空集合的结果，但完整定义仍保留失败状态，不表示全部约束满足。断链定义保持 Broken，其余可解析约束仍可求解。Update Plan/Release 排除停用项的 Verified 要求，旧 Release 的停用状态不随新 Head 激活而改变。Product Undo/Redo 恢复事务的原始位姿、定义与评价状态。已 Verified 的快照在副本上验证并生成新求解证据，禁止借验证移动组件或重写历史字段；包含活动未解决定义的历史快照直接保留原失败结果，重新计算是独立 Domain Command。

新增约束 identity 按 request ID 确定，浏览器保存 preview→request 对应关系；可复用的预览提交会冻结指向新 Revision 的 COMMIT manifest 与已验证结果，不重复求解，也不让 Release 依赖 PREVIEW 记录。当前 manifest policy 为 `assembly-m3-lifecycle-v7`，Worker solver build 为 `assembly-m2.5-hierarchy-v8`。

Angle 的 `angleRelation` 提供 FREE（默认无轴空间角）、DIRECTED（指定轴投影角）、PARALLEL、PERPENDICULAR 四种模式。两种数量角均接受0–360°；FREE 使用真实叉积范数/点积，不冻结旋转轴，正常构型控制一个转动自由度，0°/180°/360°驱动端点按方向对齐控制两个。无轴角保存第二组件局部坐标中的 `spatialAngleBranchDirection`，用叉积相对于该分支的符号区分正反解；分支只选择夹角扇区，不投影法向或添加对齐方程。首次从名义姿态建立，成功求解后沿已接受姿态运输，冻结到 manifest 并随 Undo/Redo 恢复；大于180°选择相反解，重复更新不翻回。DIRECTED 要求第二支持组件的稳定 `angleAxis`，可用 `reverseAngleAxis` 反向，沿既有 Datum/Publication/PersistentSelection 精确解析并将 `ANGLE_AXIS` 证据写入 manifest；仅约束投影方位角。切换到其他模式会清除旧轴及缓存方向，Undo 恢复完整定义。平行、垂直有独立工具栏入口；垂直以 `directionRelation` SAME/OPPOSITE 保存90°/270°意图，二者编译为带分支的空间角方程，实际选择相反姿态且不锁定轴。360°在求解提交时规范为0°。Undefined 不再永久改写为 Same。Offset 数值路径新增 Point–Axis 与 Axis–Plane，均有解析 Jacobian；零点点/点线偏移编译为重合方程，避免零范数梯度丢失其实际秩。Measured 输出独立 `measuredValue`，不改驱动值；两非平行平面等无有效常量距离的构型不显示旧数值。

Fix 新增 SPACE/RELATIVE 基准：相对固定接受显式移动后的名义位姿，空间固定保留捕获位姿。编辑命令支持显式 `fixedPose`，前端提供模式切换、三个位置与三个角度编辑。角度显示采用依次绕 X/Y/Z 的外禀旋转，持久化仍用 quaternion；相对固定的显式 pose 编辑同步更新 nominal placement，补偿历史同时恢复基准与 placement。

验证入口：[生命周期单元测试](../../../services/internal/workspace/assembly_lifecycle_test.go)、[0–6 阶、3+2+1 及常见关节有限运动测试](../../../kernel/assembly/tests/assembly_solver_scenarios.cpp)、[真实 Router/Worker、历史与 Release 集成](../../../services/internal/control/assembly_motion_integration_test.go)、[浏览器生命周期场景](../../../web/apps/cad/browser/assembly-lifecycle.spec.ts)。浏览器场景使用 Mock adapter 验证交互；权威计算另由真实 Router/Worker 与独立测试数据库验证。单项/批量激活、Measured 恢复、停用后移动、连续 Undo/Redo、空活动集重放、Release 后再激活、相对/空间 Fix 位姿行为已有真实链路回归。

这些实现尚不代表“六类约束与生命周期补齐”整体完成：Contact、Circle/Sphere/Cone/Frame 等精确 descriptor、Point–Curve/Surface 完整组合、多成员且组内先解的 Fix Together、统一六类入口及新增几何族的参数/有限运动验收仍未完成。剩余任务见[装配计划](../../../plans/assembly-evolution.md)，目标语义见[六类约束合同](../target/assembly-constraints.md)。ACCEPT-PRODUCT 的已完成基线验收范围保持独立。

### 嵌套支持与可修复的约束状态

装配几何引用携带既有 typed `InstancePath`，按当前已接受的逐层 Product Revision 查找叶 Part 的 PersistentSelection；精确点/轴/面转换到直接子 Product 的 body-local frame。根装配求解仍将子 Product 视为刚体，不改变子 Product 内部自由度。几何键、显示名与网格编号不作为持久 occurrence 身份；路径消失产生 Broken。

状态采用 CATIA 手册 `cfyugasm_C2/cfyugasmut1500.htm`（Analyzing Constraints）与 `cfyugasmut0316.htm`（Inconsistent or Over-constrained Assemblies）的区分：Verified 表示满足；Broken 表示支持引用失效；Impossible 表示单个约束与支持几何不兼容（例如不同半径圆柱表面重合）；NotUpdated 表示待更新或当前组合尚未解出，包含冲突、过约束与不收敛。数值失败不能证明几何不可能。重复但满足的冗余约束仍可 Verified，并保留 rank 冗余证据。

添加、编辑、删除、抑制/激活允许保留可修复定义与失败诊断；抑制项、Broken、Impossible 及未获接纳的 NotUpdated 定义不进入已解方程集合。定义修改、激活/抑制、删除与显式重算会重新检查并尝试接纳；拖动只求解已接纳集合，不重试隔离项。失败试算不提交候选姿态；Undo/Redo 可恢复未满足定义；Release 仍要求所有活动约束 Verified。网络/Worker 不可用等可重试故障不伪装成模型结果。

无向对齐保留同向与反向两个解，残差/Jacobian 在同一当前分支求值。如果方向残差在错误半球相互抵消，最多尝试16个替代半周初值，保留原 nominal、Fix、Rigid 和所有约束；收敛才接受。该有界数值策略不宣称证明所有非线性系统可解或不可解。

偏好优化的普通 BFGS 方向若不能下降，会在参考目标为零的层级上使用约束流形的 Lagrangian 曲率作为第二搜索方向；保留几何恢复、参考优先级、能量回溯及原始收敛容差，解决长力臂圆柱同心约束的微小曲率停滞。无向关系切换边界的差分 oracle 固定基点局部分支，避免跨不连续点的中央差分被误判为解析 Jacobian 错误。

Product 补偿冲突检查使用实际最近 REVERT/REAPPLY 结果 Revision 的字段作为预期值，恢复目标仍来自原命令 before/after；后续第三方或用户字段修改仍产生 CHANGESET_CONFLICT。不修改历史记录或清空 Undo/Redo 栈。

### 已解集合与待接纳定义

装配求值先验证既有 Verified 集合，再按持久约束顺序逐个试加入新增、修改或待恢复定义。某项与已解集合冲突或不收敛时，仅该项记为 NotUpdated 并隔离；此前已解项保持 Verified，之后独立项仍可接纳。如果上游几何变化使原集合整体失效，则按持久顺序重建已解集合。该策略是本系统的确定性接纳顺序，不宣称 NotUpdated 等价于证明“无解”；可满足的冗余约束仍由数值求解器判断。

鼠标拖动的 MOVE_INSTANCE（含 preview/commit）只使用已接纳方程和交互 driver。隔离定义不参与支持解析、方程、自由度计算或测量更新；拖动不可为了成功而丢弃已有 Verified 约束。抑制/激活、删除、修改或显式重算会再次尝试隔离项，恢复后重新参与运动。

每次接纳试算保留命令入口的 nominal pose；失败不污染姿态、warm start 或其他约束状态。网络、取消与基础设施故障使整次操作失败，不记为约束冲突。试算使用独立请求及 `PROBE` SolveManifest，最终 COMMIT/PREVIEW manifest 同时记录全部定义和实际接纳的方程集合；Release 不使用探测试算证据，活动隔离项仍阻止 Release。

嵌套 Part 的基准面、轴系原点/方向和自定义基准轴在视口拾取时与实体拓扑一样携带完整 InstancePath；直属 InstanceId 仍标识参与运动的子 Product，路径末端标识基准所属 Part 的已接受版本。支持检查和求解描述符读取统一规范化 Part 默认基准面/轴系，隐式默认基准可解析，删除的自定义基准仍返回 Broken。定向回归入口为 `nested_product_update_test.go`、`product-edit-context.scenario.mjs` 和 `nested-datum-selection.scenario.mjs`。

## STEP Definition / Occurrence 交换

STEP/XDE 导入复用既有 ProductInstance 与 typed InstancePath：每个源 Part/Product Definition 对应一个文档，多个 occurrence 通过 PINNED Revision 和独立 local placement 引用；BREP 不包含实例变换。图仅为导入/导出中间表示。同级源 Instance 名称冲突时按确定性后缀维持内部唯一性，`importedName` 记录源名与分配名；没有用户重命名时，STEP 导出恢复源名。命令历史保存完整 typed instance entity，Undo/Redo 保留名称来源及 pose。具体边界与验证见[交换架构](jobs-artifacts.md)。
