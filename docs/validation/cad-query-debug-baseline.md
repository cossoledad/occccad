# CAD 查询与工具输出基础回归

2026-10-22，原生回归使用当前 `.env` 的 SQLite、LOCAL ArtifactStore 与生产 Release Geometry Worker；输出一致性和 HTTP 单元用例使用测试拥有的短期 SQLite 对象。`invoke doctor` 确认原生构建类型和输入新鲜度。原生测试创建独立开发文档，未重置数据、修改既有模型或个人 Key。调试查询无外部模型请求，也不生成 Assistant Run；输出恢复单元测试中的 Run/调用记录为受控夹具。当前合同见[自动化架构](../architecture/current/automation.md#查询数据与模型输出)，操作见[查询调试](../development/cad-query-debugging.md)。

## 实际验证

| 项 | 结果与证据边界 |
|---|---|
| 单次 JSON 边界 | `TestAssistantToolResultByteBoundary`：完整边界值保持，超限/无效 JSON 返回完整错误，UTF-8 不被截断，重复规范化不变。 |
| 在线 / 持久 / 恢复 / trace | `TestAssistantToolResultOnlineLedgerRecoveryAndTraceAgree`：普通规划和独立问答均覆盖，含超过 32 KiB 的冻结选择页；在线工具消息、数据库、重放、近期上下文事实和 trace 一致。幂等冲突返回原记录且不重复计数。未调用模型。 |
| 固定快照与数据分层 | `TestAssistantDebugQueryFixedSnapshotAndActualOutput`、`TestAssistantDebugTopologySetAndPageCompleteness`：Head 改变仍读取指定版本；无效版本、Body、选区计数、权限和工具拒绝；服务原页与实际规范化输出分开，最后一页不等于全集，游标不能换过滤集合。 |
| 无模型 HTTP | `TestAutomationDebugHTTPWithoutAssistantOrCredentials`：服务器未构造 Assistant，OWNER/VIEWER 可读；未登录/无权限及不存在版本分别拒绝，写工具不开放，Head/Run/调用/Task 均未改变。测试直接注册路由，不是完整浏览器登录/CSRF 验收。 |
| 原生建模与查询 | `TestAutomationDebugNativeCubeChamferHole`：原 Preview/evaluator/Command/CAS → 新 Revision → 实际 Naming 查询；任意显式预选边经正式 Resolver 唯一恢复；旧立方体版本仍为 12 条边。没有孔口识别器。 |
| 前端范围状态 | `automation-debug.scenario.mjs`：实时选择、Head 和编辑框变化不改已执行输入；下一页沿用旧快照；跨 Body/版本、未知历史版本和 ContextVariant 明确拒绝。不涉及浏览器模拟。 |

6 个定向 Go 测试（包括规划/问答两个子用例）和 1 个轻量前端场景通过。另有 TypeScript、Go 构建与文档检查；未运行全量单测、浏览器/WebGL、完整 Agent、付费 Provider 或 PostgreSQL 验收。原始 Go 日志及 `native-case.json` 在 `build/agent-check/automation-debug/`，可由上述用例重建；本文不依赖可清理的文档/Revision ID。

## 原生场景的数据人工核查

按明确参数创建 XY 上的 20×20 mm 草图，拉伸 20 mm；以完整 selectionSet 对全部 12 条边做 1 mm 倒角；在 XY 草图中心 (10,10) 创建 Ø6 mm 圆，再做 THROUGH_ALL 切除。查询全程固定切除后的文档、Revision 与 Body。

人工检查完整分页导出得到：51 条边（49 LINE、2 CIRCLE），27 个面（26 PLANE、1 CYLINDER）。两个圆边分别位于 (10,10,0) 与 (10,10,20)，半径均 3 mm，参数范围为 0..2π；引用邻接关系分别包含原始基体起始/结束端面以及同一个圆柱侧面。圆柱面的半径为 3 mm、材料侧为 -1，轴线经过 (10,10)，可作为本例孔内壁证据。其解析面原点沿轴略超出实体范围来自 THROUGH_ALL 的余量，不能把这个原点当作孔口位置。

这些数据足以**人工确认本例两个孔口边**并保存其正式稳定引用。测试中的预选只验证既知引用传递；导出后的人工核查也不能证明系统会从“两个孔口边”自动定位目标。没有执行浏览器视口点选，后续实机仍须验证界面输入、显式捕获选区、分页及对象详情。

本次样本中的实际 UTF-8 内容大小如下，不是固定规模或容量承诺；稳定 ID 排序会改变新文档第一页的对象组合。

| 查询 | 服务内容 | 模型实际获得 |
|---|---|---|
| EDGE，默认详情，limit=5 | 1,286 B | 相同内容；只有稳定 ID/几何类型及集合/分页信息，没有半径、圆心或邻接。 |
| EDGE，includeSelections=true，limit=5 | 36,638 B | 149 B 资源错误；服务原页中的对象和游标没有发给模型。 |
| EDGE，详细信息，limit=1，全部 51 页 | 每页 3,348..8,908 B | 每页均原样通过；不表示这些页能同时留在模型上下文。 |
| FACE，详细信息，limit=1，全部 27 页 | 每页 4,909..11,688 B | 每页均原样通过。 |
| FEATURE，definition，limit=5 | 33,378 B，仅返回一个大倒角定义且还有后页 | 149 B 资源错误；完整 12 边选择引用使单对象本身已很大。 |
| SKETCH，definition，limit=2 | 6,320 B | 原样通过，含矩形与中心圆定义。 |

因此，“完整服务数据存在”“本页数据完整”“模型获得数据”和“语义目标已定位”是四个不同结论。尤其不能因为属性面板能展示半径/圆心，或因为人工预选了两个边，就宣称模型已经获得并识别这些目标。

## 当前语义缺口与下一步

- 孔仍由圆形轮廓和普通 REMOVE 特征表达；查询没有“某孔的两个孔口边”的直接角色/关系接口。默认拓扑项不含几何数值，详细引用又携带大量重复 lineage/邻接信息。
- 已有邻接是稳定语义引用，能用于人工沿图核查，但还没有面环/边用途等面向意图的有界投影，也没有通用多孔、相交孔、倒角孔口的定位保证。两个圆边方向相同，不能靠方向正负直接标注入口/出口。
- 大 Feature 的完整 definition 可能在 `limit=1` 时仍超过工具边界。服务的投影项预算不含整个响应封套，不能据此保证模型输出可容纳。当前调试入口如实暴露限制，未新增切片规则来改变特征定义。

建议先讨论紧凑的精确拓扑属性投影与按需正式引用读取：明确尺寸/单位、圆心/轴线、材料侧及有类型的稳定邻接，按完整响应核算单次字节预算。用本基线比较精简前后的实际可见数据，再设计 DSL 或做 LLM 规划测试。本轮未实现这些候选能力，也未新增任务总轮数或总 Token 硬上限。
