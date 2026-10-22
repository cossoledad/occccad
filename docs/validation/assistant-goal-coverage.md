# 目标覆盖与查询结果绑定回归（历史）

> 本页保留上一轮执行证据。自然语言目标编译、覆盖门禁及其测试已在后续职责调整中移除，不代表当前完成合同；现行行为与测试见[当前请求边界回归](assistant-request-boundaries.md)。查询引用与紧凑拓扑能力继续保留。

2026-10-22，基于紧凑拓扑观察与筛选基线扩展，未修改 Naming 身份、拓扑筛选/集合解析/分页预算、Worker/Proto 或数据库迁移。现行合同见[自动化架构](../architecture/current/automation.md#当前请求与查询结果绑定)，前置数据能力见[查询调试基线](cad-query-debug-baseline.md)。原生 fixture 使用当前 `.env` 的开发 SQLite、配置制品与 Geometry Worker；单元故障注入使用测试拥有的临时对象。未重置数据或配置模型，原生生成独立开发文档。

## 场景与证据

`TestAssistantGoalCoverageNativeCubeHoleAndLocalFillet` 分别通过自然回复和 cad_finish 两个完成入口，执行“XY 上 100×100 mm 草图 → 拉伸 100 mm → 全部 12 边 R10 → 底面中心 Ø40 THROUGH_ALL 切除 → 孔口两圆边 R2”。R2 是该测试给出的委托设计选择，用户要求只写“适当倒圆角”。每步经过模型侧 createStage 适配、既有 Automation Prepare/Preview/Commit/CAS 与生产几何求值，无模型 Provider 请求。

| 检查 | 实际结果 |
|---|---|
| 新目标漏验 | 增加“通孔 + 孔口局部圆角”的新来源与 GoalVersion 后，旧基体尺寸及 R10 检查不能完成新目标；通孔做完、局部圆角未做时仍 NOT_VERIFIED。 |
| 错误目标 | 将旧 R10 特征故障绑定为新孔口目标，即使局部半径委托，仍被输入快照/目标边验收拒绝。 |
| 多要求并存 | 原 R10 来源与目标保留；孔口圆角独立引用通孔要求。两项圆角在最终检查中均有独立目标。另一单元用例检查相同 Kind 不同 Body 的要求不会合并。 |
| 委托参数 | expected 仅声明 DELEGATED，不写 radius=2；observed/designChoice 记录实际 2 mm。无冻结选边且未指明全部边/指定孔口的圆角保持 UNRESOLVED，不默认为全边。 |
| 查询到执行 | 当前 Naming 的 CYLINDER/R20 内壁 → CIRCLE/R20/关联面筛选，limit=1 返回一条，但集合覆盖两条。命令通过账本引用取出原令牌，提交特征确有两个正式选择。 |
| 两个完成入口 | 缺通孔、缺局部圆角、错误目标三个阶段均分别尝试自然回复及 cad_finish；均不能完成。两种入口各自最终通过完整覆盖门禁。 |
| 恢复 | 用同一数据库和 Automation 创建新的 Assistant 服务实例，原查询引用仍取回相同值；清空缓存目标/集合后，来源与正式回执能重建通过的验收。非完整进程故障/租约接管验收。 |
| 错误分类 | 不存在/损坏引用、错误槽位/操作、冲突传值、跨 Run/Body、过期 Revision 和来源权限分别定向拒绝。有效引用传入 R1000 触发原 `MODIFIER_ALGORITHM_FAILED:FILLET:SEQUENTIAL`，未冒充引用损坏；失败预览不改变 Head。 |
| 原始记录 | 模型原 create_task 参数仍为 resultBindings，实际 Task 参数保存解析后的原 selectionSet；原工具反馈与 trace 不被摘要替换。公开工具 transcript 先保留原结果，再给单独的确定性引用说明。 |

测试中的调用/工具账本是受控注入的公开响应，服务于恢复和执行链验证；没有实际模型推理。两份原生报告的 Run.modelCalls 均为 0，SENT 调用记录为 0。不能把它当成 LLM 会正确选择工具或设计半径的证明。

## 数据量

同次原生回归、同一运行状态、相同最近八条工具记录（同样包含来源 epoch）的 UTF-8 JSON 对比：

| 内容 | 字节 |
|---|---:|
| 原集合 Base64 字符串 | 687 B |
| 查询引用对象 callId/toolId/slot | 119 B |
| 完整 resultBindings 单条绑定对象（含 target/source） | 154 B |
| 近期事实直接展开原参数与原反馈 | 14,865..14,869 B |
| 近期事实确定性投影 | 6,646..6,650 B |

原始工具参数和结果仍完整保存。两种完成入口各比较自身相同的八条记录，时间戳长度造成少量字节差异；近期事实约减少 55.3% 字节，不是整个模型请求或模型 Token 的节省。随机 ID 排序、查询范围、检查内容和请求封套会改变实际大小，不作费用、延迟或上下文容量承诺。原生报告另存完整重建上下文字节数。

## 验证入口与边界

原始日志与 `native-natural.json`、`native-cad_finish.json` 在 `build/agent-check/goal-result-bindings/`，可重新生成，不依赖固定开发文档或 Revision ID。

```sh
# services/
go test -p 1 ./internal/workspace ./internal/automation ./internal/assistant ./internal/api -run '^TestTopology(Observation|Filter|Unknown)|^TestHoleRimAcceptance|^TestAutomationDebug|^TestAssistant(GoalCoverage|CompletionCoverage|QueryResultReferences|ToolResult|Debug|AssemblyRequirements|RunStagesNative|RunStageRecovers|RunRepeatedStage|RunStageFailure|BudgetToolResponse)' -count=1 -v
```

覆盖测试、引用故障/权限/恢复、工具在线/持久/恢复/trace 一致性、既有阶段回执恢复与重复诊断、无模型 HTTP 调试、原有紧凑拓扑筛选及 20 mm 倒角方体/通孔/同半径干扰原生回归已通过。单元故障注入还验证 130 个必需检查以同一 Revision 分页验收，保留全部结果；原有每请求 64 项/96 KiB 边界不会变成目标要求总数上限。新增检查继续复用已有选择集合匹配；未新增总轮数、总步骤数或总 Token 硬上限。

仍为有限的源语句合同：未映射的语句/目标、局部取消等未支持编辑、缺失参数保持未解决，需要明确要求；不支持任意自然语言规格消歧。孔口验收只针对正式绑定单圆 THROUGH_ALL 切除的唯一圆柱内壁与两条完整圆边；有限深度/复杂布尔策略、盲孔、相交孔、多壁或分段边界不能据此推广。委托半径只证明执行及几何有效，未做设计优选。

未执行全量单元测试、浏览器/WebGL 实机、真实 LLM/付费 Provider、完整进程重启与 PostgreSQL 验收。建议先实机确认覆盖反馈、账本引用和原始调用详情，再增加有明确来源的复杂验收样例；本轮没有 DSL、通用孔识别、全局短 ID 或 LLM 摘要系统。
