# CAD 自动化与 AI 对话建模

DeepSeek/OpenAI 格式对话已接入既有 CAD 命令体系。用户在工作台下边栏打开“建模助手”，填写自己的 API Key，选择模型、思考度和低/中/高调用预算。Part/Product 定义使用相同的权限、Preview、领域命令与 CAS；需要拾取或表单输入的操作交给原工作台工具。MCP Server、CAD CLI、任意长动态计划和跨文档原子任务尚未提供。

入口：[Automation](../../../services/internal/automation/README.md)、[Assistant](../../../services/internal/assistant/README.md)、[Web 面板](../../../web/apps/cad/src/features/assistant/assistant-panel.tsx)、[后续计划](../../../plans/ai-native-modeling.md)。

## 调用与命令覆盖

```text
对话 → DeepSeek Chat Completions → 有界 Tool Host → 持久计划
  模型操作 → PreviewCommand → 同一意图 ExecuteCommand → evaluator/Worker
           → CAS / Revision / 精确回执 → 浏览器实际渲染确认
  界面操作 → 记录接管 → CommandRegistry 可见性/可用性 → 原工具/表单
           → 界面打开结果；不生成虚假的模型提交回执
```

操作目录包含三类入口：

| 入口 | 当前行为 |
|---|---|
| `command.*` | 覆盖 Workspace 公开适配器的 65 个领域命令合同：草图几何/尺寸/约束、实体、特征、参数、基准、发布、装配与机构等。真实执行仍交原 handler；文件导入、产品导入的验证图、引用更新计划必须由原应用流程产生，合同明确要求界面接管。 |
| `ui.*` | 从同一 Workbench Catalog 投影全部 134 个工作台命令。通过原 CommandRegistry 再检查运行时可见性和可用性；输入依赖保持原工具行为。撤销、视图、表单等也可发现，不走强制几何预览。 |
| 简短操作 | 保留矩形、草图、拉伸、参数操作，增加 `part.fillet` / `part.chamfer`。尺寸明确为 mm；复杂参数通过 `command.*` 按需读取合同。 |

[覆盖测试](../../../services/internal/automation/commands_test.go) 对照实际适配器 switch 和工作台目录，新增/遗漏命令会失败。该检查证明接入合同覆盖，不代表每个 CAD 参数组合都已通过原生或浏览器验收。

搜索分页返回匹配数与完整性，游标绑定目录 digest、文档类型和查询。describe 才读取一个操作合同；嵌套 DTO 通过 `cad_describe_type` 逐层加载，直接投影现有 Go 类型与实际 JSON 嵌入字段。校验拒绝未知/重复字段、错误类型、固定数组维度与预算超额；领域 handler 继续检查 CAD 语义、量纲、引用和几何。用户、文档、requestId、Head 和 Preview 由 Host 绑定，不接受模型覆盖。普通命令与 Automation 共用 `RequireCommandReferences`，新引用文档在求值前检查读取权限。

每个计划最多 32 步；使用的操作须先读取合同。局部步骤标签由应用转换为确定 UUID，前序输出只能绑定声明的稳定实体输入。无业务改动的机构调用恢复账号隔离的审计回执，明确 noChange，不虚构事务或新 Revision。创建结果取自精确 ChangeSet 回执；多创建结果明确列出，不猜测“最后一个特征”。创建草图、特征、实例及外部引用等沿请求生成确定身份，保证预览、提交与重试意图一致。

昂贵求值在数据库事务外，有用户/全局并发额度和 110 秒操作 deadline。数值错误保留 code、phase、retryable、diagnosticId。装配 `DEFINITION_ONLY` 预览不进入自动提交，须在原工具中检查；无装配方程时不把未调用求解器误判为未收敛。

## 定义查询与精确拓扑集合

`ReadModelSnapshot` 读取不可变定义，不建立完整引用树、几何和标签。`cad_query_model` 投影 Body、Feature/Sketch、Parameter、Datum、Publication、Context、Instance、装配约束及机构；草图子元素用 kind 和 ownerId 分页。字段 `definition` 按需读取 DTO，其余时候只取名称、类型、参数或状态等小字段。参数 `value` 为 SI quantity，`displayValue` 是 `unit` 中未舍入的值。

定义查询默认 20 项、最多 100 项，单页对象预算 32 KiB；游标固定 Revision 与过滤条件，complete 仅表示一次返回整个匹配集。定义文本、实体数和投影均有预算。当前仍解析并排序受限定义，尚无持久查询索引、全树懒加载或 occurrence 展开；5 万节点合成测试不是大型装配容量保证。工作台内对其他定义的在位编辑仍需显式切换目标，不隐式跨定义写入。

`cad_query_topology` 通过 Workspace 的 Naming manifest 读取指定文档/Revision/Body 的 EDGE、FACE 或 VERTEX。模型得到小页稳定 ID、匹配数与紧凑 `selectionSet`，可按已查询 ID 筛选；需要详细引用时才读取正式 selection。集合绑定文档、Revision、Body、拓扑种类、manifest digest 和完整匹配数量；执行前重新验证，编译成原 `PersistentSelection`。不把 OCCT local ID、mesh 序号或显示名暴露为持久身份。

“全部边线”使用整个 selectionSet，不能拿第一页当作全集。拓扑页最多 20 项；一次修改最多 4096 项，超额明确失败。几何修改后必须重新查询新 Revision 的拓扑，旧集合不会静默套用。当前句柄是不可变快照中的确定性选择，不是跨 Revision 自动重定位的通用集合服务。

## DeepSeek、模型与凭证

默认地址 `https://api.deepseek.com`，默认模型 `deepseek-flash`，Bearer Key、非流式原生 function tools。模型列表通过当前账号的 `/models` 获取，展示供应商名称和支持的思考等级；选择保存到账号。思考度为关闭、低、高、最高，对应显式关闭 thinking 或启用并传 `reasoning_effort=low/high/max`。每轮保存并冻结实际 model/thinking，设置变化只影响新请求。

实现依据官方[模型发现](https://api-docs.deepseek.com/api/list-models/)、[思考与工具调用](https://api-docs.deepseek.com/guides/thinking_mode/)及[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)合同，2026-10-20 核对。当前规划循环完整回传供应商 `reasoning_content`，包括空字段；它只在该轮内存工作集存在，不进入公开对话、任务、下一轮历史或日志。新轮将近期语言历史作为引用数据重新构造，避免回放缺失私有思考的旧 assistant/tool 消息。

保存 Key 时验证可用模型，再用 AES-256-GCM、账号 AAD 加密入库。HTTP 返回 configured/provider/model/thinking/budget 和服务端预算预设，不回显 Key。主密钥自动生成于 `<OCCCCAD_DATA_DIR>/assistant/credentials.key`，权限 0600；数据库与文件共同恢复，多 API 进程共用。服务端可选 `OCCCCAD_AI_BASE_URL/MODEL/KEY_FILE`。错误映射不泄露上游原文或凭证，重定向不转发 Bearer。`0003_assistant.sql` 保存凭证/对话；双数据库 `0004_assistant_preferences.sql` 增加模型和思考度，`0005_assistant_budget.sql` 增加预算并默认中档，保留已有加密凭证。只修改预算不依赖供应商在线，修改模型/思考度及更换 Key 保留所选预算。

## 规划预算与恢复

模型只查询、发现、读取合同/检查点并 `cad_create_task`，没有 advance 或显示确认权限。计划成功即结束模型循环；执行每个预览/提交不额外调用模型。请求幂等，记录需求、回复、实际 generation、预算快照、模型请求次数、taskId 和供应商 usage；同用户/文档最多一轮规划。deadline 按预算，租约比 deadline 多 15 秒，取消 CAS 防止迟到结果复活；过期为 INTERRUPTED。计划已生成而关联未保存时，按原请求恢复并停止取消/过期的孤立计划。

预算由 [budget.go](../../../services/internal/assistant/budget.go) 唯一维护，设置按账号保存，默认中档；每次发送独立计算，思考度与预算独立。请求开始时冻结全部实际边界，进行中的请求不受设置变化影响。

| 预算 | 累计 Token | 模型/工具调用 | 单次输出：关闭/开启思考 | 消息上下文 | 整轮/单次等待 |
|---|---:|---:|---:|---:|---:|
| 低 | 80,000 | 10 / 24 | 4,096 / 16,384 | 128 KiB | 180 / 90 秒 |
| 中（默认） | 200,000 | 20 / 48 | 8,192 / 32,768 | 256 KiB | 360 / 180 秒 |
| 高 | 500,000 | 32 / 96 | 16,384 / 65,536 | 512 KiB | 720 / 240 秒 |

累计 Token 包含该次需求每次工具往返的重复输入、输出和思考，以供应商 usage 为准，不是整段对话共享额度。后续请求的 `max_tokens` 不超过剩余累计额度；输入 Token 不能提前精确计算，检查发生于供应商返回后，最后一次调用可能超额且仍计费。输出截断或无效响应也保留可解析的已报告 usage。usage 缺失不伪造估计，字节/次数/时间限制继续生效；这些不是金额硬限额。

语言历史最多 8 轮/16 KiB，完整检查点至多 3 项/16 KiB，额外历史保留引用；工具结果最多 16 KiB，仍按需分页。完整供应商请求比消息上下文多 64 KiB envelope 额度；私有思考上限分别为 64/128/256 KiB，完整响应按预设限制 JSON/UTF-8 开销。不为迁就预算裁剪工具回执或当前轮私有思考；超限明确结束。用户可提高预算后重新发送，生成新的 requestId；旧请求重试仍只恢复原记录，不重复付费调用。未提供金额预算或长任务动态追加。

## 步骤、接管与界面

任务账本按版本 CAS 更新，保存有界事件和精确回执；Workspace 短锁/租约协调推进，普通人工编辑保留原 CAS 保护。

```text
建模：PLANNED → PREPARING → PREVIEW_READY → COMMITTING → COMMITTED → DISPLAYED
                         失败 → FAILED / PAUSED
界面：PLANNED → HANDED_OFF → UI_OPENED / UI_FAILED
```

每次 advance 只预览或提交一个阶段。对应 Revision 真正渲染后才能显示确认并推进；恢复先查原领域回执，候选丢失则重新预览。暂停/停止在边界生效，继续可显式接受当前 Head；被人工新版本取代的提交保留回执并标 SUPERSEDED。停止保留已提交步骤，整任务撤销与跨文档原子操作尚未提供。

界面命令使用单步任务：先持久记录接管，再检查并调用当前注册工具，最后记录打开成功/失败。丢失打开结果时显示待确认，刷新不重发界面命令；接管不是几何完成证明。

助手入口位于下边栏，模型、思考度和预算位于对话输入框上方；预算选项展示 Token 和调用次数，每轮回复显示供应商已报告用量与冻结的预算。预算失败提供“按当前预算重新发送”。宽视口下与实际画布并排，窄视口悬浮；沿用 ResizeObserver 更新渲染与拾取范围。对话展示澄清、回复、参数及实际步骤，支持逐步确认、暂停接管、停止、定位与恢复。只有成功发送获得计划才默认开始；重载不重发模型调用、不自动建模。关闭/隐藏页面暂停后续推进，没有脱离 UI 的后台执行。

## 验证入口与边界

- `invoke check --scope services --match '^TestAssistant|^TestAutomation'`：目录完整性、嵌套合同、模型/思考协议、凭证隔离、预算、恢复，以及原生 Worker 的矩形、全部边圆角、通用圆草图/拉伸和 Product 实例命令。
- `invoke check --scope web --match assistant`：绑定、精确显示确认、对话恢复、运行时命令可用性与接管分层。
- 以 `OCCCCAD_AI_BASE_URL=http://127.0.0.1:19091 invoke run` 启动 API/Worker，在 Web 目录执行 `pnpm exec playwright test --config playwright.live.config.ts automation-live.spec.ts`。受控模型服务搭配真实 Vite/API/Worker 验证连接、模型/思考度/预算保存、预算耗尽后提高预算重发、矩形→全部边 R2、显示、界面命令、取消与重载。使用独立账号，不覆盖个人 Key；结束后恢复默认 DeepSeek 配置。
- 迁移/共享命令边界变化运行 `invoke check --scope all`，知识入口修改运行 `invoke context-audit`。

以上为验证入口，当次结果以报告为准。用户已反馈线上矩形建模成功；受控模型测试证明应用调用链，不能替代复杂命令的线上推理质量、真实 token 成本或工业容量评测。
