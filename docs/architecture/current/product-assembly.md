# Product 关联设计、装配与发布

> 2026-09-21 文档核对基线。返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。ACCEPT-PRODUCT 已完成，人工与自动化证据及限制见本文末尾。

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

每次正式 assembly preview/commit 冻结 `AssemblySolveManifest`：root/candidate Revision、完整 body pose、局部几何描述符、Publication/PersistentSelection resolution evidence、约束、branch/intent、affected scope、schema 2 solver profile 与 build policy 共同形成确定 digest。构造器按持久化 JSON 表示冻结独立快照，位姿、角度分支及 Publication/PersistentSelection 证据不再共享调用方指针；后续调用方修改不改变已冻结内容与 digest。Worker 只消费 manifest 中的纯值；Publication endpoint 直接使用已解析 descriptor，不再让 solver 查询 Product/B-Rep。manifest 与 request-specific result 持久化，重试复用同一结果，digest replay、request lookup、deadline/cancel 和既有 `.3dreplay` 数值证据并存。

当前保存独立 `ProductRelease`：Release Manifest 冻结完整 occurrence typed path/Revision/pose、ContextBinding、ContextVariant GeometryKey/EvaluationManifest、Product Publication、命名/evaluator policy、成功 SolveManifest 与 gate 结果。Gate 要求引用 current、全部 occurrence/variant READY、活动约束 Verified 且有可重放求解证据；停用定义保留在 SolveManifest 中，不伪造 Verified，也不参与此 gate。Release 可在 Workspace Head 移动后按 manifest replay，并从冻结 GeometryKey 提交 STEP/BREP 导出；Exchange placement 现已贯通 translation 与 quaternion rotation。Web 的“产品版本中心”只负责创建、列出和 replay 不可变里程碑，不再把 STEP/BREP 按钮混入发布流程；Exchange 保留为独立后续 UX。当前不把 Configuration/Design Table、partial update、flexible subassembly 或 Derive Part from Context 列为已实现能力。

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

SolveManifest 同时冻结完整 `definitions` 与实际编译的约束。全部停用时仍记录空活动集合的求解与可重放结果；全部活动支持断裂时不伪造空集合求解证据。断链定义保持 Broken，其余可解析约束仍可求解。Update Plan/Release 排除停用项的 Verified 要求，旧 Release 的停用状态不随新 Head 激活而改变。Product Undo/Redo 为新 Revision 生成新的求解证据，并从最终模型重建位姿和约束实体写集。

新增约束 identity 按 request ID 确定，浏览器保存 preview→request 对应关系；可复用的预览提交会冻结指向新 Revision 的 COMMIT manifest 与已验证结果，不重复求解，也不让 Release 依赖 PREVIEW 记录。当前 manifest policy 为 `assembly-m3-lifecycle-v2`，Worker solver build 为 `assembly-m2.5-hierarchy-v3`。

Angle 定义新增 `angleRelation`：Directed、Parallel、Perpendicular；后两者分别使用独立的方向对齐/点积方程。有向角的控制面支持 Plane/Axis/Cylinder 方向对；界面要求显式选择 `angleAxis` 并可用 `reverseAngleAxis` 反向，轴来自第二支持组件的 Datum/Publication/PersistentSelection，按接受的 Revision 冷解析并随该组件运动。其 descriptor digest 以 `ANGLE_AXIS` 保存到 manifest；不接受其他组件的轴并将它静默冻结。360°在求解提交时规范为 0°，旧无轴实验定义仍可读，但编辑需补齐稳定轴。Undefined 不再永久改写为 Same。Offset 数值路径新增 Point–Axis 与 Axis–Plane，均有解析 Jacobian；零点点/点线偏移编译为重合方程，避免零范数梯度丢失其实际秩。Measured 输出独立 `measuredValue`，不改驱动值；两非平行平面等无有效常量距离的构型不显示旧数值。

Fix 新增 SPACE/RELATIVE 基准：相对固定接受显式移动后的名义位姿，空间固定保留捕获位姿。编辑命令支持显式 `fixedPose`，前端提供模式切换、三个位置与三个角度编辑。角度显示采用依次绕 X/Y/Z 的外禀旋转，持久化仍用 quaternion；相对固定的显式 pose 编辑同步更新 nominal placement，补偿历史同时恢复基准与 placement。

验证入口：[生命周期单元测试](../../../services/internal/workspace/assembly_lifecycle_test.go)、[0–6 阶、3+2+1 及常见关节有限运动测试](../../../kernel/assembly/tests/assembly_solver_scenarios.cpp)、[真实 Router/Worker、历史与 Release 集成](../../../services/internal/control/assembly_motion_integration_test.go)、[浏览器生命周期场景](../../../web/apps/cad/browser/assembly-lifecycle.spec.ts)。浏览器场景使用 Mock adapter 验证交互；权威计算另由真实 Router/Worker 与独立测试数据库验证。单项/批量激活、Measured 恢复、停用后移动、连续 Undo/Redo、空活动集重放、Release 后再激活、相对/空间 Fix 位姿行为已有真实链路回归。

这些实现尚不代表“六类约束与生命周期补齐”整体完成：Contact、Circle/Sphere/Cone/Frame 等精确 descriptor、Point–Curve/Surface 完整组合、多成员且组内先解的 Fix Together、统一六类入口及新增几何族的参数/有限运动验收仍未完成。剩余任务见[装配计划](../../../plans/assembly-evolution.md)，目标语义见[六类约束合同](../target/assembly-constraints.md)。ACCEPT-PRODUCT 的已完成基线验收范围保持独立。
