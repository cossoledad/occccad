# Product 关联设计、装配与发布

> 2026-09-21 文档核对基线。返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。这里只记录实现事实；测试存在不等于本轮已经运行，验证缺口见[统一路线](../../../plans/README.md)。

- 当前 Product UI 中的新实例固定采用 `FOLLOW_HEAD`；被引用文档变化后先使 root-snapshot `ProductUpdatePlan` 失效并投影 `UPDATE_AVAILABLE`，打开 Product 的可编辑客户端随后按叶到根自动接受每一级带 digest 的计划，提交普通 `UPDATE_REFERENCES` Revision 并重建可重放快照。Instance 右键可把当前 resolved Revision 切换为 `PINNED`，也可恢复 `FOLLOW_HEAD`；任意历史版本 picker 尚未开放。

Product 工作台维护浏览器会话级的 Active Occurrence 编辑上下文，由 `Active InstancePath + Reference Document` 共同表达。打开 Product 时根 Product 默认激活；双击结构树中的 Product、Part 或 Instance 节点只激活该 occurrence，随后 Toolbar、属性、历史、Undo/Redo 和 Domain Command 绑定其 Reference Document 的 `main` Workspace。激活不是模型命令，不写 Revision；同一 Part Reference 的其他 occurrence 不显示为激活，但编辑 Reference 后全部 FOLLOW_HEAD occurrence 都会解析到新结果。视口始终保留根装配；活动 Part 的草图、基准和命令预览施加该 occurrence 的世界 Placement 后就地编辑，其他部件继续显示。

根 Product 场景与 Active Part 交互视图由 `ViewportEditContext` 分离，命令/草图选择只读取激活 Part。Publication 与基准几何的显示/拾取规则见[Web 交互](web.md)。

`INSERT_INSTANCE` 继续把既有 Part/Product Reference 插入当前激活且具有 Editor 权限的 Product；InstanceName 由服务端在 owner Product 当前候选模型中按 `ReferenceName.N` 分配首个同级可用名称，显示名不参与身份，重命名属于独立属性命令。

Product 结构树的 Product 节点另提供“新建零件”。`POST /api/documents/{rootProductId}/part-components` 接收可选的 typed target Product InstancePath；服务端在事务外建立空 Part 初始 Revision 和目标/祖先 Product 候选，再以一个 `ProductDesignTransaction` 原子创建 Part 文档、插入 `FOLLOW_HEAD` occurrence，并从目标 Product 自底向上推进到 root snapshot。任一目标路径、权限或 Workspace Head 失效都会整体拒绝，不产生孤立 Part 或半更新装配。空名称按全局首个可用 `PartN` 分配，InstanceName 仍按目标 Product 同级的 `PartN.N` 分配；第一版只支持位于目标 Product 原点的 identity placement。Undo/Redo 补偿 Product 成员 Revision，因此 Undo 会移除 occurrence 并恢复祖先引用，但不会删除已经创建的 Part 文档；该文档保留在 Document Center，可再次插入或显式删除。结构树 capability 描述领域动作，实际入口仍由服务端对 root、目标和全部祖先 Product 再次执行 Editor ACL 校验。

第一版 typed `InstancePath` 保存 RootDocumentId 和有序 segment；每段包含 owner Document/Revision、稳定 InstanceId、显示 InstanceName、ReferencedDocumentId 和 resolved Revision。`canonical` 由 InstanceId 链构成并用于选择、激活和 occurrence 资源索引，`display` 才由 InstanceName 拼接。名称修改不得改变 canonical identity，也不得作为持久引用解析键。

## Publication 与受控外部引用

Publication 保存在 Part/Product Revision 中。Datum POINT/AXIS/PLANE/FRAME、PersistentSelection CURVE/SURFACE、Feature output BODY 与 PARAMETER 共用稳定 PublicationId、兼容版本、typed CRUD、ChangeSet、Undo/Redo 和 dependency graph；拓扑 target 随 Part 求值按 naming 重解，失效后保存 `BROKEN_PUBLICATION`。Parameter Publication 固定 value type、dimension、SI unit policy、可选 bounds 与 source ParameterId；消费 Part 的 `ExternalParameterRef` 保存来源 Document、FOLLOW/PINNED selector、PublicationId、期望合同与冻结 `ReferenceResolutionSnapshot`。DocumentView 对来源 Head 只投影 `UPDATE_AVAILABLE/BROKEN`，显式 Update References 在事务外解析完整候选并以 Workspace sequence CAS 提交，普通重算不读取来源 Head。Assembly endpoint 优先保存 PublicationRef 与 PersistentSelection deep link；兼容 Replace/Update 重连新 target，Product Publication 可转发一个相对 occurrence 的子 Publication。

## Product Design Session 与上下文事务

Product 内关联由 Product Design Session 组织。`InstancePath` 由 root 与有序 typed segment 构成，canonical identity 只连接稳定 InstanceId，显示名只形成 breadcrumb；递归展开设有 cycle、深度 32 和成员 10000 的 gate。Instance、Part/Product Publication 与 ContextInput 使用版本化 `nfkc-casefold-v1` 作用域名称规则，rename 只改变显示投影。Product Publication 可沿嵌套 rigid Product path 转发。

Part 现在声明不含具体来源的 typed `ContextInput`，root Product Revision 持有 source occurrence Publication 到 owning occurrence input 的 `ContextBinding`、相对 Transform 和 accepted resolution snapshot。创建绑定会在事务外解析当前 root snapshot、合同、权限与依赖环，再为消费 Part、全部嵌套 owning Product 和 root Product 预生成 Revision/ChangeSet/EvaluationManifest；数据库事务按 Workspace 稳定顺序锁定并统一 CAS，一次提交所有 Revision、Outbox 和 Head。该事务组从任一成员文档触发 Undo/Redo 时也统一补偿，不会只留下 Part input 或 Product binding。`ContextReference` 仅保留为旧试点模型与独立 Part/受控外部通道能力，新 Product 内 picker 不再向用户暴露全局 DocumentId/PublicationId。

Web 以 root Product、active occurrence 和 definition/context 模式维护非持久设计会话；`GET design-session` 与 root-snapshot-scoped `GET context-catalog` 只返回当前 Product 可达且调用者可读的 Publication，并按 expected type、连接状态和已知 ContextBinding DAG 过滤。参数与 Context binding 面板按 occurrence breadcrumb/可编辑名称选择来源；跨 Workspace 提交仍由服务端重新验证 typed path、Head 和合同。

## 更新计划与派生 Variant

`ProductUpdatePlan` 从不可变 root snapshot 投影 occurrence/reference 与 ContextBinding 影响项，分别报告 connection、currency、evaluation，候选 Context Variant 会把已接受/候选 Publication 描述转换到 owning Part local frame，再复用 Part 参数、Sketch 与几何 evaluator 生成独立 GeometryKey、Publication resolution 和 EvaluationManifest。Variant identity 只包含 base Part Revision、规范化输入快照及 evaluator/policy；不包含 binding 显示名、BindingId、occurrence path、WorkerId，PARAMETER 输入也不包含无意义的 occurrence transform，因此同一定义和输入的四个 Wheel occurrence 可共享持久 variant cache；不同几何/参数输入不污染共享 Part。accepted variant 会覆盖 Product `ResolvedInstances` 的 base GeometryKey，并沿嵌套 rigid Product Publication 转发进入装配 descriptor。Web 对 FOLLOW_HEAD 变化按叶到根自动接受 Update Plan，但每次仍以 digest 防止接受过期计划；任一候选失败都会阻止当前计划并保留诊断。

## M3 SolveManifest 与 Release

每次正式 assembly preview/commit 冻结 `AssemblySolveManifest`：root/candidate Revision、完整 body pose、局部几何描述符、Publication/PersistentSelection resolution evidence、约束、branch/intent、affected scope、schema 2 solver profile 与 build policy 共同形成确定 digest。Worker 只消费 manifest 中的纯值；Publication endpoint 直接使用已解析 descriptor，不再让 solver 查询 Product/B-Rep。manifest 与 request-specific result 持久化，重试复用同一结果，digest replay、request lookup、deadline/cancel 和既有 `.3dreplay` 数值证据并存。

当前保存独立 `ProductRelease`：Release Manifest 冻结完整 occurrence typed path/Revision/pose、ContextBinding、ContextVariant GeometryKey/EvaluationManifest、Product Publication、命名/evaluator policy、成功 SolveManifest 与 gate 结果。Gate 要求引用 current、全部 occurrence/variant READY、约束 Verified 且有可重放求解证据。Release 可在 Workspace Head 移动后按 manifest replay，并从冻结 GeometryKey 提交 STEP/BREP 导出；Exchange placement 现已贯通 translation 与 quaternion rotation。Web 的“产品版本中心”只负责创建、列出和 replay 不可变里程碑，不再把 STEP/BREP 按钮混入发布流程；Exchange 保留为独立后续 UX。当前不把 Configuration/Design Table、partial update、flexible subassembly 或 Derive Part from Context 列为已实现能力。

## 求解与交互的当前边界

`kernel/assembly` 只消费纯值 Point/Axis/Plane/Cylinder、约束和完整 SE(3) pose。Rigid cluster、Fix/Ground 消元与 connected-component 求解先于数值迭代；生产路径使用解析 Jacobian、augmented QR 和 SVD rank/null-space。M2.5 依次满足硬约束、最小化 reference motion、最小化总 nominal motion，独立报告偏好收敛与瞬时自由度。创建/编辑以第一选择为 moving、第二选择为 reference，Constraint 与全部 solved pose 在同一 ChangeSet 提交。算法细节与 corpus 由[Solver Algorithms](../../../kernel/assembly/SOLVER_ALGORITHMS.md)维护。

当前 MOVE 仍注入临时 `interaction-driver` Fix；不可达预览恢复权威 pose，尚不能返回 M4 的最近可行拖拽结果。有效 preview candidate 可经 CAS 提升为提交，缺失/过期时重新权威求解。M3 已覆盖嵌套 rigid Product 与持久引用解析；flexible expansion、稀疏后端和最小冲突集尚未实现。

三维求解支持独立的 `occccad.3dreplay.v1` 下载：每次实际 SolveAssembly（包括 preview、成功、模型失败和已知的 RPC 失败）结束后，将精确数学请求、有效求解参数及紧凑结果在响应关键路径之外原子写入 `OCCCCAD_LOG_DIR/debug/assembly-replays/<documentID>/`。它不进入 PostgreSQL，不改变 Workspace Head/Revision，也不包含 B-Rep、网格或完整命令历史。每个文档最多保留 50 条且最长保留 7 天；文档读权限仍控制列表与下载，Web 可按真正发生求解的 request ID 下载 `.3dreplay`。文件可不依赖数据库通过 Worker/Router 重放。几何解析前失败尚未形成数值求解输入，不生成文件；本地存档失败只记录独立错误，不伪造求解状态。

装配约束创建和编辑现在都从最终候选模型提取相同的 `moving first / reference second` solve intent，不再因命令类型改变规约锚点。显式 Same/Opposite 的平面重合若从精确反向端点开始，Solver 会绕被约束平面锚点生成确定性的半周 branch seed，再执行普通 component solve，避免依赖有限差分噪声逃离零梯度鞍点。约束数值输入期间只更新本地表单；失焦或 Enter 才产生一次可取消、带 sequence 防迟到覆盖的权威预览，支持元素和方向等离散变更仍立即预览。

装配约束预览具有显式的两层工作流状态。Web 使用 XState actor 管理 `idle / pending / succeeded / failed`、请求 sequence、取消和迟到响应过滤；失败会在非模态约束面板中立即显示稳定错误码、服务端阶段和诊断，并阻止提交未经成功预览的 draft。Go Workspace 使用 Stateless 管理 `RESOLVING_GEOMETRY -> SOLVING -> APPLYING_RESULT -> COMPLETED`，任一活动阶段可进入 `FAILED`；API 对求解失败返回结构化 `code / phase / retryable`，而取消与 deadline 保持传输层语义。该工作流状态不持久化，也不取代 Product Revision、Command/ChangeSet 或 Solver 数值状态。

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
