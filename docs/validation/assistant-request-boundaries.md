# Assistant 当前请求职责边界回归

2026-10-22。Agent 负责理解和处理当前请求；应用负责正式 CAD 执行、几何求值、回执与一致性。现行合同见[自动化架构](../architecture/current/automation.md#当前请求与查询结果绑定)。本轮移除历史自然语言要求编译和覆盖完成门禁，复用已有 Run/Stage、紧凑拓扑与账本结果引用，没有 DSL、新分类模型或通用澄清状态机。

## 原因与实现

开发会话已存在两个孔口的 R2 圆角，却把“适当的倒圆角”记为 UNSCOPED_FILLET，又把“保留”“保留即可”累加成 UNRESOLVED_INTENT。AddMessage 重编要求、阶段/后台自动检查和两个完成入口共用旧硬门禁，使模型重复查询和请求用户重述。它是应用职责边界造成的循环，不是缺少一个新的语言特例。

- 删除 Assistant 的 GoalRequirement 编译、历史目标输出绑定、覆盖检查及自动验收调度；几何和尺寸检查继续由 Automation 提供，通过 cad_verify_part / cad_verify_assembly 显式按需调用。
- 无工具文字回复和 cad_finish 共用当前执行边界，允许无改动、部分完成和明确失败后结束。检查当前代际、活动/退休操作、当前调用及响应 Revision；保留真实失败诊断。工具完成反馈、游标、回复与结束状态原子保存。
- 新消息保留当前模型、最近回执角色和必要历史，重置本次应用计数。连续输入不会丢失已经等待中的退休 Task；旧阶段回执归档到原来源，不计为新请求已应用，也不能结束新请求。
- 提示词明确委托参数自行选择并报告、保留直接回应、修改最近圆角优先编辑参数。UI 显示“本次已结束”和本次真实“已应用”，取消历史目标验收标签；原请求、公开回复、参数和工具反馈仍可查看。

主要实现入口：`services/internal/assistant/run_store.go`、`run_completion.go`、`run_planner.go`、`run_stages.go`、`run_stage_repair.go`、`part_check_tool.go`；Web 的 `conversation-model.ts`、`assistant-panel.tsx`、`assistant-trace.tsx`。

## 原生连续回归

使用当前 `.env` 开发 SQLite、配置制品与原生 Geometry Worker，新建独立开发文档，没有重置数据或修改原问题文档。CreateRun/AddMessage 是真实消息服务入口；公开模型响应受控写入调用账本，再由生产 Step 调度、工具处理、Preview/evaluator/Commit/CAS 执行。测试 Provider 的 Complete 一旦被调用就失败，实际调用数为 0。

`TestAssistantCurrentRequestNativeFilletEditAndNoChange` 对自然回复、cad_finish 分别执行：100 mm 立方体 → 全边 R10 → 底面中心 Ø40 通孔 → 两个孔口圆边 R2 → 保留 → 改成 R3 → 已有结果无需修改。

| 项目 | 实际证据 |
|---|---|
| “适当的倒圆角” | 真实 AddMessage 后使用受控选择的 R2，不需额外确认；两条正式选边的圆角提交成功，回复报告 2 mm。 |
| 紧凑查询与绑定 | 当前圆柱内壁、半径和关联面筛选；limit=1 返回一条，selectionSet 覆盖两条，经原 resultBindings 解析和正式执行链应用。 |
| “可以，保留” | CAD Transaction 增量 0、Revision 增量 0、阶段增量 0，本次应用计数 0；重复同一消息 requestId 返回已保存回复。 |
| “不喜欢，改成3mm” | 从最近 FILLET 回执角色出发，经真实 cad_query_model 的 PARAMETER/ownerId 查询唯一半径参数，再用 parameter.set_literal。仅增加一个 Revision；目标圆角 R2→R3，其两条选择不变，其他五个 Feature 定义逐项完全一致，原全边 R10 和通孔保留。 |
| 已有结果无需修改 | 可直接结束；额外显式检查采用不符的 R4 期望会如实失败，但不强制修改或阻止保留现有 R3。 |
| 提交后恢复 | 原参数修改提交后、Run 检查点前重新创建 Service；按原请求恢复，一次正式 Transaction、一次计数，不重复执行。 |

孔壁、目标边和 R2 决策由测试提供的显式条件选择，**不算 LLM 自动语义定位或自主设计成功**。按需孔口检查仍为既有有限单圆贯穿孔场景，不是新孔识别算法。参数编辑路径已实际验证；正式删除/重建仍沿现有能力发现和命令链，本轮没有新增替代实现或验收复杂依赖删除。

## 边界与受影响检查

| 检查 | 结果 |
|---|---|
| 几何失败后结束 | 两个入口均保留已应用辅助草图、真实几何失败诊断及原实体，结束当前工作；不自行续跑。 |
| 活动操作和迟到结果 | 活动/退休 Task 阻止完成；连续两条输入保留退休边界，旧响应不能激活或完成新请求，迟到真实回执只归档一次。 |
| 版本变化 | 两个入口均拒绝旧 Revision 的完成回复；新快照可正常结束。 |
| 完成账本原子性 | 在工具反馈入库前注入中断，结束状态、公开回复和调用游标一起回滚；原持久响应可恢复，反馈重放返回同一原值。 |
| 按需检查 | 不修改 Run、不调用模型；显式固定 Revision，缺失或越界失败；检查结果不携带完整模型。 |
| 原有链路 | 查询引用损坏/过期/权限、工具在线/持久/恢复/trace 一致性、预算恢复、独立只读问答、重复诊断、权限拒绝、提交前缀恢复及原有倒角/通孔紧凑拓扑原生回归通过。 |
| 长任务 | 原有合成百万用量、多阶段和超时钟跨度回归通过，未增加任务总轮数、总步骤或总 Token 硬上限。 |
| Product | 受控装配回归改为读取现有紧凑阶段摘要，按需 cad_verify_assembly 后结束；原生求解收敛并保留一个旋转自由度。 |

Go 定向结果合并为 **34 个顶层测试通过**（含子场景）。初次受影响检查暴露装配测试仍按旧完整 Task 形状读取紧凑上下文；修正测试 fixture 后单独复验通过。其余已通过检查不因该测试失配失效。原始记录在 `build/agent-check/assistant-request-boundaries/affected.log`、`product.log`、`native-final.log` 及两份 `native-*.json`。

可重建的最小新回归入口（在 `services/`）：

```sh
go test ./internal/assistant -run '^TestAssistantCurrentRequest|^TestAssistantPartChecks' -count=1 -v
```

Web 的 `pnpm typecheck` 和 `pnpm test -- assistant`（6 个场景）通过；文档审计的 8 个测试、`invoke context-audit` 和 `git diff --check` 通过。

未执行全量单元测试、浏览器/WebGL 模拟或人工实机、真实 LLM/付费 Provider、PostgreSQL、完整进程故障接管。下一步应按上述三句话实机验证真实模型的工具选择、半径决策、公开解释与 UI；应用门禁不再替代这些能力，也不宣称自然语言需求均已验收。
