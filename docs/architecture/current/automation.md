# CAD 自动化与持续 AI 建模

下边栏“建模助手”通过账号 Key、模型及思考度设置发起持久 Run。Agent 理解当前请求、读取事实并规划有界片段；应用执行原命令、核对回执与新 Revision；关闭页面不暂停自动模式。累计预算默认 `null`，用户可主动设置并调整，默认无总调用数、工具数、步骤数、阶段数或整轮 deadline。

入口：[Assistant](../../../services/internal/assistant/README.md)、[Automation](../../../services/internal/automation/README.md)、[Web](../../../web/apps/cad/src/features/assistant/assistant-panel.tsx)、[线上评测](../../../services/cmd/occccad-ai-eval/README.md)。本文说明当前实现合同；当前限制和定向证据边界见文末，任务优先级只见 [CAD 核心主线](../../../plans/cad-core-roadmap.md)。

## 所有权与执行链

```text
用户输入/冻结目标 → Run + 输入日志 → 服务端租约调度
  → 有界事实工作集 → Provider 调用意图 → 已发送 attempt → 响应/usage 落账
  → 查询或下一片段 Task → 原 Preview → evaluator/Worker → Command/CAS
  → 精确 CommandReceipt → 新 Revision / 实际几何 → Agent 决定继续、按需检查或结束当前请求
真实 Viewport frame → 对应文档/Revision 的呈现回报（独立于执行）
```

Run 是保留当前模型和对话指代的持续会话；每条新的建模输入有独立执行边界。多条 GOAL/STEER/QUERY 输入具有稳定序号和来源。STEER 增加 goalVersion/epoch，保留已提交事实，退休旧片段并等待在途边界；旧响应只记账，不能激活旧步骤。只读问题独立租约与冻结快照，不等待整个 writer 结束，不能创建/推进 Task。服务器重新检查 actor/document/source ACL，历史或 VIEWER 上下文只读。应用不依据关键词或问号判断查询/修改及工具权限。可写上下文中的普通输入交给同一 Agent；仅真实 VIEWER 权限或历史快照等只读上下文使用 QUERY 控制通道。

单文档只有一个未结束 writer。Run 使用短事务 CAS、45 秒可续租，调度并发有界并为交互读预留位置；等待 Provider/Worker 不持数据库事务。租约到期允许另一进程接续，不是任务时限。轮询查询仅取待执行片段；浏览器不拥有推进职责。显式暂停/停止在边界生效，不撤销已提交历史。未知 CAD 提交必须先查原 actor/document/requestId 的回执，不能换身份重做；人工 Head 变化使旧候选失效，重读事实并重规划，不能仅替换 ExpectedHead。已知的绑定/参数/几何失败先持久 FAILED 步骤、退休旧片段并精确归档已提交前缀，再把诊断交回 Agent，由其决定修复或如实结束；不会重放已应用前缀。相同语义失败的摘要独立于新Task/step/request身份，重复仅记为 REPEATED_DIAGNOSTIC，供模型和用户核对，不因重复自动拦截或 BLOCKED。权限与未知提交仍保留正式校验及等待边界。

## 日志、检查点与上下文

双数据库 `0006_assistant_runs.sql` 保存 Run、输入、阶段、事件、逐调用及工具结果、已读合同和设置来源。旧预算列删除，旧 low/medium/high 不映射为累计封顶；已有加密凭证保留。旧对话表仅遗留历史数据，无旧同步运行时或路由。

Run JSON 只保留当前请求原文、有限回执角色提示、摘要及计数；无限增长的输入/事件/阶段/调用独立分页。调用先保存意图，再标记发送，响应及 usage 先落账后才安排副作用。响应已收到但工具尚未处理时从原公开响应和工具游标恢复；工具结果按 call/tool ID 幂等。预算或恢复边界消费已落盘响应时丢弃旧在线会话的未回答工具尾部，下一请求从事实重建，避免发送不完整的思考工具 transcript。CAD 片段的精确回执与 Run applied 计数通过同一事务检查点归档；requestAppliedCount 只计属于本次请求的非 NoChange 回执，新消息清零，退休阶段的迟到提交仅进入累计计数和历史来源。停止或旧 epoch 的不明 Provider 请求在失效租约后补记 UNKNOWN。

发送前在既有事件日志中独立保存一次公开请求快照（MODEL_REQUEST_CAPTURED）：实际 messages、可用 tools/schema、冻结 generation，以及执行引擎标记、构建 VCS 版本、catalog/prompt digest。调用账本只保存快照序号；常规事件页排除大快照，工具游标更新不会重写完整提示。`GET /api/documents/{documentID}/assistant/runs/{runID}/trace` 返回有界近期摘要和向前游标；同一 Run 下的 `calls/{callID}` 按需读取原请求、完整公开回复、原工具参数和实际工具反馈。Task 的后续执行结果另列，不能冒充工具当时返回的内容。旧调用没有保存的请求原文明确不可还原，不用当前提示词补写历史。快照不包含 Key、HTTP 鉴权或私有 reasoning_content，也不作为额外上下文回灌模型。

### 查询数据与模型输出

| 层 | 职责与来源 |
|---|---|
| CAD 真相 | Workspace `ReadModelSnapshot` 读取不可变定义；`ReadBodyTopology` 从指定 Revision/Body 的当前 Naming tip 投影测量与有类型的面邻接，正式选择的哈希算法不变；属性查询读取指定制品的精确 B-Rep 属性。Mesh/local ID 不能代替稳定引用。 |
| Automation 查询 | `query.go` 按字段、对象、Body 和游标投影定义；`topology.go` 默认返回 typed 紧凑观察，完整固定范围先筛选、再生成 selectionSet、最后按项数及完整响应字节分页。服务页不等于整份模型。 |
| 工具处理 | `tools.go` 调用上述服务；`tool_result.go` 对完整 UTF-8 JSON 内容执行统一 32 KiB 边界，超限以完整资源错误替换，不切断 JSON。 |
| 持久与恢复 | `recordTool` 返回实际落账的规范结果；普通规划和独立问答均用返回值追加在线消息。幂等冲突返回原记录，恢复读取同一反馈，不能重新附上原始大结果。 |
| 上下文 | `buildContext` 从精确快照、有限近期工具事实/阶段的确定性摘要与查询引用及冻结目标组装；完整账本、工具重放与实际 trace 保留原值，摘要不写回工具结果。单次上下文仍有界。 |
| trace / 属性 | trace 读取实际公开请求和规范化反馈；属性面板的精确 B-Rep 数据仅供人工查看，不能冒充已经发给模型的信息。 |

已修正的偏差：原 `recordTool` 按值替换超限结果，在线消息仍使用原值，而数据库和恢复只看到错误；原 Host 的 64 KiB 与账本的 32 KiB 也不一致。现在出口统一，原始服务页只在明确的人工调试适配器中保留。定义查询的项预算不含整个响应封套，大 Feature 定义又包含完整选择引用；因此服务成功仍可能被工具边界替换，不能声称模型已看到该定义。

属性检查器的“查询调试”经 `POST /api/documents/{documentID}/automation/debug-query` 调用同一 Host 与规范化函数，无需 Key、Assistant Run 或 Provider。仅开放模型、拓扑及固定版本的文档上下文查询，沿用 Viewer 权限；明确 Revision 不存在即失败，不回退 Head。Body/选区在显式捕获时冻结，分页沿用该查询，查看历史结果只改变本地展开内容。服务原页、实际模型可见内容和人工补充分开显示；集合引用完整与单页是否包含匹配全集分别展示。请求沿用 64 KiB JSON 边界，调试选区最多 20 项，人工原页超过 1 MiB 明确省略；这些都不是持续任务的总轮数/Token 上限。操作和限制见[只读查询调试](../../development/cad-query-debugging.md)，本轮实测及语义缺口见[基础回归](automation.md)。

活动请求只加载当前请求原文、冻结 scope 摘要、当前不可变上下文、有限稳定目标角色、最近少量片段/公开工具事实和语言。超出单次工作集时淘汰可重查内容并重建，不随完整历史线性膨胀。冻结选择按 `cad_get_selection` 分页，默认只投影类型、归属、正式标识和来源；显式 `includeEvidence:true` 才读取完整正式引用，预览子集明确不完整。必要信息本身超过配置时准确等待调整，不能丢弃尺寸、单位、保护对象或装配要求来缩短提示。

### 当前请求与查询结果绑定

Agent 负责理解并处理当前请求，应用不再从历史自然语言编译 GoalRequirement，不维护未解决语句覆盖、必需检查或按来源合并的目标合同。新增消息仅更新当前请求文本及执行代际，保留模型、最近回执角色、必要历史和正式选择。“适当”是委托设计选择，提示词要求 Agent 自主选择合理值并报告实际采用值；“可以，保留”可直接回复；“不喜欢，改成3mm”优先查询最近圆角所属参数并使用 parameter.set_literal，能力不足时发现正式删除/重建命令并保留其他内容和依赖。应用不新增意图分类模型或通用澄清流程。

自然文字回复与 cad_finish 通过同一执行边界结束当前工作。检查当前 epoch/requestVersion、活动/退休 Task、pending call 以及写入会话响应对应的锁定 Head；有未收敛操作返回 AI_EXECUTION_PENDING，版本已变返回 AI_COMPLETION_STALE。COMPLETED 仅表示本次请求已结束，允许无修改、部分完成或明确失败，不是应用对全部历史目标的验收声明。历史失败或未解析文字不能触发后台验收、强制继续或反复澄清。已知几何失败可交回 Agent 决定修复或结束；未知提交仍先恢复原请求回执。

cad_finish 的规范工具结果、调用游标、最终回复与结束状态在同一事务保存，重放沿原 call/tool ID 返回原结果。普通回复同样原子保存公开回复、调用状态和 REQUEST_FINISHED；两条路径均不写 CAD、不虚构 applied 或呈现，也不会因结束而清除真实几何诊断。新请求仍复用现有 Run/Stage；goalVersion 字段作为请求版本继续使用，不再对应自然语言规格。

几何、尺寸与正式选边验证继续保留为只读按需工具 cad_verify_part / cad_verify_assembly。Part 检查显式指定 Revision、对象、期望与单位，每次 1..64 项并遵守原请求/输出边界；可分批继续，没有会话检查总数限制。结果及 observed 来源保存在工具账本和 trace，不存为 Run 的强制完成门禁。委托半径可使用 radiusPolicy:DELEGATED，只检查实际正有限值而不反填用户硬期望；应用不证明设计优选。

孔口检查仍仅针对显式指定的单圆 THROUGH_ALL 切除和圆角：在圆角的输入 Revision 检查切除与唯一同轴同半径内壁，复用紧凑筛选匹配两条完整圆边并核对持久选择。它是指定对象的检查证据，不是自然语言或通用孔识别系统；相交、分段、多壁歧义及其他未支持路径保持 NOT_VERIFIED。

模型可在 `cad_create_task.steps[].resultBindings` 中引用 `{target:"selectionSet",source:{callId,toolId,slot:"selectionSet"}}`。callId 是调用账本 UUID，toolId 是其中的工具调用 ID；二者唯一定位已记录结果，不是 CAD 短 ID。仅开放 `cad_query_topology.selectionSet → part.fillet/part.chamfer.selectionSet`，禁止任意 JSON 路径及同槽位传值/重复绑定。应用从原工具账本取值，检查当前 Run、来源权限、文档/Body/类型、查询封套与集合元数据、命令基线 Revision；不重写令牌或追随最新 Head，再经原集合解析、预览与提交。损坏/不可用为 QUERY_RESULT_REF_INVALID，范围错误为 QUERY_RESULT_REF_SCOPE，版本过期为 QUERY_RESULT_REF_STALE；实际几何失败沿用原领域诊断。

工具回复仍是原规范化内容；完整工具组回复后另外给出确定性的账本引用说明。重建上下文的 recentFacts 默认保留来源 epoch、固定范围/历史标记、计数、少量紧凑观察、摘要 digest 和 selectionSetRef，省略长集合及重复定义。阶段默认保留参数 digest、状态/输出和原创建调用中的 resultBindings，失败步骤另保留紧凑输入；绑定来源沿已有 Task 幂等 requestId 和唯一创建调用恢复，不按相同集合值猜来源。Run 的集合缓存仅投影范围/计数/digest。原始模型参数、规范工具结果、待处理工具重放和实际请求 trace 均不被这些摘要替换；没有 LLM 摘要或新注册表。查询绑定与当前请求的重建测试见文末“验证入口”。

按需检查返回稳定身份和 observed；期望保留在原工具输入，trace 同时展示输入与实测。FAILED 表示检查所见不一致；对象角色不符、未解析的建模路径或证据不足返回 NOT_VERIFIED，不能据此声称几何错误或要求删除结果。系统提示要求模型先核对检查范围、当前对象和实测值，并用公开简述说明修改依据。检查不会强制完成请求，也不能把未知证据声称为通过。

圆形贯穿孔识别线性 REMOVE，或 BOOLEAN REMOVE 正式引用的单个圆柱切割特征；后者不强迫改成特定配方。检查轮廓直径、基准平面法向和目标实体当前权威拓扑中的圆柱孔壁（直径、轴线及孔内材料侧），保留实际稳定拓扑身份。有限长度切割通过权威 Body 几何范围沿孔轴的投影证明覆盖；THROUGH_ALL 使用正式范围定义。矩形中心孔通过权威草图支撑坐标系检查孔轴与原始矩形中心。孔后若有可能封堵孔口的实体操作，保留 NOT_VERIFIED。通用复杂面中心、多孔、复杂切割工具体及任意下游贯穿性不在此小合同保证内。这些局部检查不提供自然语言要求覆盖；Agent 根据当前请求与实际事实决定是否继续。

## 轮次用量与会话导出

Run.currentTurnId 指向当前用户输入；每个调用在 prepareCall 的创建事务中持久化独立 turnId。InputID 仍只承担 QUERY 的租约和控制语义，普通模型调用保持为空，不用 Epoch、最近时间或累计差值猜轮次。发送、未知回执及已报告 usage 与对应 Turn/Run 统计在同一事务更新；失败/重试按实际 call attempt 分别记账，同一调用第一次报告的 usage 不重复累计，UNKNOWN 后补报转换为已知。未发送不计实际调用，待返回和未知分别显示；缓存和思考量仍为各自输入/输出的子集。

会话顶部显示累计 Token，每个用户输入下显示整轮已报告用量，展开查看输入/输出、调用数、待返回及未知；调用详情保留每次实际状态。历史没有 turnId/轮次统计的记录明确标记缺失，不反推归属。历史活动 Run 没有当前输入指针时，需要一条新消息建立明确调用来源，不回填旧语句或创建兼容归属。

`GET /api/documents/{documentID}/assistant/runs/{runID}/export?format=json|markdown&turnId=<optional>` 共用会话/本轮导出。沿原 Run owner 和文档权限检查，一条 SQL 读取完整一致快照：所有相关输入/冻结选择、调用、含实际请求快照的事件、原工具参数/规范结果、阶段及完整持久 Task（含预览、回执和后续执行）。单条记录逐项写入 `build/assistant-exports/` 的 0600 临时文件，关闭数据库游标后再流式下载；不依赖 UI 缓存，不因页面条数、模型输出上限截断会话。结束/失败均清理本次暂存；进程崩溃可能留下暂存文件。

导出明确截至时间、运行状态、未完成工作、待返回/未知用量、无法归属的历史调用和缺少请求快照等来源缺口。`complete:true` 只表示当前快照的持久记录已经完整输出，不表示 CAD 请求成功或所有历史数据存在。整会话保留未归属调用；本轮仅导出明确关联记录，提示无法归属数量。实际请求仍包含它当时确实发送的历史上下文，不用当前提示词重建缺失历史。Markdown/JSON 共用记录迭代；均不导出 Key、私有 reasoning_content、未保存信息或 BRep 二进制。

## 资源与 Provider 用量

单片段 1..32 步，Run 可继续追加任意多个片段。模型无执行器和呈现工具。搜索/查询/工具结果、单次输入与并发仍有边界；分页或重建之后可继续，不能把局部边界当整个目标的上限。不可分割的大操作须说明真实限制，例如全边一次圆角不能偷偷拆成语义不同的多次圆角。

默认单请求消息工作集 256 KiB、输出 8192 Token（思考开启时 max_tokens 32768）、网络 180 秒；工具执行复用既有操作 deadline。它们是当前配置保护，并非通用供应商能力或 Run deadline。每次调用保存 Provider origin、model、thinking、设置版本、实际 max_tokens、单次限制、逻辑调用/attempt、时间、Provider request ID（提供时）、请求摘要、状态及 usage。

用户可设置账号默认累计预算和活动 Run 的可空预算；达到后保留已收响应并等待，提高或清空后续接同一 Run，不重发已收响应。预算调整有版本及 USER 来源。输入 Token 不能提前精确获得，检查基于已报告 usage 和可知输出边界，最后一次可能超额。未知用量不记作确定零，不承诺金额硬限额。

输入/输出/总量按 Provider 字段记录；缓存是输入子集、reasoning 是输出子集，不再加入 total。原始数值细项经白名单保留。相同响应重复归档不重复累计；不同已发请求分别记账。超时/取消/进程丢失且没有 usage 时记 UNKNOWN，重试可能再次收费，不能承诺供应商无重复计费。重复诊断保留为证据，不代替模型决策；限流按时间退避，无累计失败次数封顶。用户预算、暂停和停止仍控制执行。

默认 DeepSeek `https://api.deepseek.com` / `deepseek-flash`，模型通过当前账号 `/models` 获取；思考度 off/low/high/max。模型设置只影响后续调用，当前 attempt 不变。协议依据官方[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)和[思考/工具调用](https://api-docs.deepseek.com/guides/thinking_mode/)（2026-10-21 核对）。私有 reasoning_content 仅在在线内存 transcript 往返，不能进入持久响应、公开事件、日志或重建的历史。

AES-256-GCM、账号 AAD 和本地主密钥延续原凭证方案；主密钥与数据库共同恢复。API 只返回 configured 和无秘密设置。模型修改通过供应商能力校验；仅改预算不依赖 Provider 在线。HTTP 重定向不转发 Bearer，错误不透出上游原文。

## 命令、选择与动态绑定

目录复用所有公开 `command.*` 适配器与 Workbench `ui.*` 投影，覆盖测试对照真实 switch/catalog；合同覆盖不代表所有参数组合都通过原生验收。 搜索可同时返回便捷操作、原领域命令与工作台入口，调用方须按 operation ID 选择并按游标分页，不能假定关键词只有一项结果。目录同步现有草图支撑重指定、特征移动/局部插入、轮廓及布尔区域输入和工程孔类型；基准面支撑可经原命令执行并固定来源 Revision，实体面支撑仍需原编辑器的合法阶段拾取，不接受模型生成的 local ID。`part.fillet/chamfer` 等小操作减小 DTO 成本。先搜索再 describe，嵌套类型按需读取；未知字段、类型、数组维度、权限、几何和量纲仍由原校验负责。Host 固定 actor/document、requestId、baseline、preview 和 epoch，模型不能覆盖。

发送前完整复制选择，再读取正式 PersistentSelection；工作台去重复用 document/Revision/InstancePath/Body/几何及草图子元素的正式选择键，不按 kind:id 合并不同 occurrence。树的 pad/visual 仅是展示分类，冻结时按该 Revision 的定义补齐真实 Feature 类型、草图实体/约束及所属关系；草图端点/控制点保留 SketchGeometryRef。实例使用路径接受的源文档/Revision，不能以父 Product 的快照替代；实例使用稳定 InstancePath，定义/根/源文档 Revision 分别保存；服务端逐路径核对接受的源文档与版本，不能冒用同源新Head或可读的无关定义。LLM 初始上下文只有紧凑预览，完整 Scope 仍用于执行和导出，不把同份拓扑证据同时注入 frozenScope 和 selectionPreview。发送前后显示简短类型/数量摘要。当前单输入最多 256 项及 512 KiB，超额拒绝整个输入，不静默截断。发送后换选不影响原指令。过期拓扑只经正式 Naming/selection resolver 的唯一 RESOLVED 结果重定位；失败或歧义要求重新选择，不依靠名称、mesh 序号、local ID 或改写 Revision。

`cad_query_model` 按不可变定义投影稳定实体、参数、草图子元素等，默认20/最多100项、32KiB单页，游标绑定 Revision/过滤条件。参数 value 是 SI quantity，displayValue 是未舍入的显示单位值。当前仍在受限定义上解析/排序，未提供持久查询索引或任意大树容量保证。

`cad_query_topology` 默认返回稳定 ID、geometryType、`geometry` 和 `adjacentFaces`。scope 在外层，明确 document/revision/body/kind、geometryKey、manifestDigest、PART_LOCAL/mm/rad 与 CURRENT_NAMING_BODY_TIP 来源。几何值直接来自该快照当前 tip，不把调用方旧 creationEvidence 当成测量。完整 PersistentSelection、sourceIds、creationEvidence 只在 `includeSelections:true` 时读取；按一个稳定 `ids` 展开即可，debug 与真实工具走同一查询。

`filter.geometryType`、`filter.radius`（equalMM 或 minMM/maxMM，必须显式给正的 toleranceMM）及 `filter.adjacentFaceId` 做合取。指定面必须属于同一固定快照；关联使用完整当前语义引用的精确映射，不按命名字符串或旧 lineage 猜孔口。`ids` 先固定候选范围。未知属性或未解析的关系采用三值判断：已知不匹配排除，无法判断计入 unknownCount；不冒充完整匹配。

selectionSet 仍是自包含集合，无新注册表；绑定 document/revision/body/kind/manifest、候选 IDs、筛选条件、匹配数、完整性和匹配 ID 摘要。查询、集合解析和既有 fillet 验证复用相同匹配函数，预览/提交重验相同集合。`selectionSetComplete:false` 或零匹配不能执行；零匹配不会退化成全选。游标绑定集合和投影（limit/includeSelections），新几何必须读新 Revision。`scopeCount/candidateCount/matchedCount/unknownCount`、`returnedCount` 与 `selectionSetCount` 分别描述范围、候选、匹配、未知、本页和集合；`pageComplete` 仅指本页是否包含全部已知匹配，不能代替 matchComplete/selectionSetComplete。

紧凑关系最多列 16 个关联面 ID，knownCount 计完整内部关系数，complete 要求 Naming 邻接证据存在、当前引用解析完整且本行未裁剪；没有邻接证据时保守标记未知，不声称不存在关联面。筛选使用未裁剪关系。几何字段带单位，缺失/不支持的适用字段列在 unavailable，不能当零值。圆/圆柱 direction 是轴向，平面是有向法线；圆柱 materialSide -1/+1 区分内壁/外壁，不构成通用孔角色。radiusMM 在椭圆表示长半径，minorRadiusMM 为短半径；缺少可靠解析框架的曲线不输出占位原点/方向。

拓扑每页最多 20 项，同时核算含 scope、集合令牌、游标和证据的完整 UTF-8 JSON 内容，边界与 Host 统一为 32 KiB。必要时返回更少整项并给出下一游标；单项连封套仍超限则返回 TOPOLOGY_PAGE_RESOURCE，不切片嵌套引用。这没有增加任务总调用数或总 Token 限制；大 Feature definition 的边界仍由既有工具规范化负责。

`typedBindings` 仅声明允许的目标字段，绑定前序明确 outputId/role，可投影 ENTITY_ID、INSTANCE_PATH、ASSEMBLY_REF；复杂输出不猜“最后一个特征”或数组位置。实例输出使用提交模型已接受的 source Revision，不能另读 source 最新 Head。UUID 由原请求/步骤生成，恢复沿同一意图与精确回执。

## Product occurrence 与最小装配

`cad_query_occurrence` 按根 Revision + 稳定实例路径解析嵌套、重复和 PINNED 引用，逐层检查源 ACL，复用定义几何而分别返回 source snapshot、world pose、Body/Instance摘要或精确 FACE/EDGE/DATUM/PUBLICATION 小页。游标绑定根/路径/过滤条件，定义坐标与世界坐标明确区分；转发 Publication 按 accepted snapshot 和正式刚体变换派生。

`cad_mate_candidates` 使用精确圆柱/平面支持语义，区分材料内外侧、轴线/半径、长度、平面方向与偏置。默认紧凑候选，只在按 candidateId 选取时读取完整 recipe；多候选不静默选第一项。recipe 沿原 FIX/CONCENTRIC/端面关系命令与 solver，无第二套装配实体或几何脚本。

轴线/端面方向、偏置和目标 DOF 由 Agent 依据用户请求与当前事实填写到正式命令及显式检查；按需检查真实持久位姿、同轴距离/角度、明确端面方向与 `SELECTED_PLANE_NORMAL_V1` 偏置、实际 solver 状态以及相对平移/旋转 DOF。求解出的假设修正姿态不能代替真实提交。支持普通轴/孔及端面定位、保留一个旋转自由度；不代表任意零件识别/复杂接触。带 ContextBinding 的 occurrence 目前明确返回 CONTEXT_VARIANT_NOT_AVAILABLE，不能替用普通定义 BRep 验收。跨文档原子写与整任务撤销仍未提供。

## 执行、接管与真实呈现

AUTOMATIC 持续在服务端推进；CONFIRM 每次等待精确 previewId、taskVersion、baseline 和 planEpoch 的批准，过期候选重新预览，旧批准不能用于新代际。

领域提交产生 APPLIED/精确 receipt；Agent 结束本次请求产生 COMPLETED，按需检查返回自身结果；真实 Viewport frame 才产生对应文档/Revision 的 PRESENTED。这些状态独立，无浏览器不会凭空显示。GLB加载失败只重试显示，不重做 CAD 或模型调用；重连不补造旧中间版本的显示。

`ui.*` 单步先持久 handoff/baseline/预期命令/DomainRequestID，再经原 CommandRegistry 检查可用性。打开结果丢失不自动重放。UI_OPENED 仅打开；取消明确记录并暂停；完成必须恢复同一预期请求、类型及基线的领域回执，无关提交不算完成。当前 SolidFeatureEditor 提供原 preview/commit request 身份桥，其他未提供安全完成桥的交互继续等待人工处理，不能伪造成功。

Web 继续由 TanStack Query 保存服务状态，原工作台选择/CommandRegistry/Viewport 各自拥有交互、命令与帧证据。下边栏使用单图标入口；面板以对话、15px 正文/输入及简短进展为主。“调用详情”以实际调用为序，展开查看公开请求 → 模型原文 → 工具/参数 → 原反馈及 CAD 后续执行；默认展开最近一条，较早记录分页读取。检查同时展示期望值和实测对象，代替原始事件代码列表。模型带工具调用的公开 content 按调用幂等保存为 ASSISTANT_MESSAGE；工具观察、实际 CAD 回执、阶段诊断及按需检查给出可读进度。公开简述在模型响应落账后显示，不承诺逐 Token 流式输出；私有 reasoning_content 不进入事件、账本响应或 UI。无工具的文字回复结束本次请求，必要澄清使用 cad_wait_for_input；带工具的公开 content 是进展。界面以“本次已结束”表达完成状态，仅按本次真实回执显示“已应用”，不显示强制目标验收标签。消息/步骤/事件按页增量读取，进展仅保留有界近期窗口，旧代际事件不能覆盖新状态。暂停/停止、精确确认、接管及显示重试保留原边界。

## 当前限制

- 查询仍在受限定义上解析/排序，无持久大树索引、按需子树或任意超大集合容量保证。单个大 Feature definition 或正式引用连同封套超限时仍会失败，limit=1 不保证可读取。
- 紧凑几何与邻接不提供通用孔、入口/出口、面环或边用途语义；斜孔、盲孔、相交孔、多壁、分段边界及任意下游贯穿性没有通用定位保证。不能将人工/测试给定目标当作 LLM 自主识别。
- ContextVariant occurrence 不可替用普通 Part BREP；复杂 Publication/接触、跨文档原子写和带人工冲突的整任务撤销未交付。只有提供精确完成桥的 UI 工具可自动核对接管结果。
- 多 API 的主密钥轮换、集中凭证管理、外部 MCP/CLI、受约束脚本运行时未交付；现有持久 Run 不代表这些运维和执行扩展已具备。
- 导出依赖可写暂存磁盘，进程崩溃可能残留文件；历史来源缺口明确保留，不补猜轮次或缺失请求。超大生产会话的容量/吞吐与运行中人工编辑的完整恢复仍缺独立验收。
- 受控 Provider、合成 usage 与静态字节对比不证明线上模型质量、Token 节省、费用或生产吞吐。更复杂几何、交互体验及 PostgreSQL 的实际结果须按运行环境单独确认。

## 验证入口

- 服务：`go test -tags=integration -count=1 -p 1 ./internal/assistant ./internal/automation ./internal/api -run '^TestAssistant|^TestAutomation'`（services目录）；容量/故障注入使用受控 Provider，不付费压测。
- Web：`mise run check --scope web --match assistant`；真实工作台先用本地19091 Provider启动服务，再运行 `pnpm exec playwright test --config playwright.live.config.ts automation-live.spec.ts`。独立测试账号不改个人Key，结束恢复默认 DeepSeek 配置。
- 原生几何、迁移与共享链：按 `mise run check --scope all`，数据库当前配置之外的 Provider 验证必须单独标明未执行。
- 线上小样：`services/cmd/occccad-ai-eval` 使用已有合法 Key 和显式小预算。模型表现、原生几何、浏览器、合成百万 usage 必须分别报告。

保留的历史证据范围（本次文档整理未重跑业务验收）：

| 范围 | 已有证据与可重建入口 | 不能外推的结论 |
|---|---|---|
| Run/选择/恢复/装配 | 2026-10-21：SQLite、Release Worker、受控 Provider 与 Chromium/SwiftShader 定向通过；`run_capacity_test.go`、`run_product_integration_test.go` 和 `browser/automation-live.spec.ts`。当日全范围为 PASS_WITH_SKIPS，非 PostgreSQL 验收。 | 合成 2,050,000 usage 不是实际付费消耗；原生 solver 与浏览器呈现分别举证。 |
| 当前请求与查询绑定 | 2026-10-22：`run_request_native_test.go`、`run_query_bindings_test.go` 及 `automation/query_debug_integration_test.go` 定向通过，使用正式 Preview/Commit/CAS 与原生 Worker，无 LLM。 | 测试指定孔壁、目标边和半径，不证明语言理解或通用孔识别；未执行对应新行为的浏览器验收。 |
| 用量/导出/冻结选择 | 2026-10-23：`turn_usage_test.go`、`session_export_test.go`、`selection_context_test.go`、`run_scope_test.go`、受影响 API/Web 场景定向通过，Provider 受控。 | 未执行完整浏览器、真实 LLM、PostgreSQL、生产超大会话或完整进程故障接管验收。 |
| 线上模型 | 2026-10-21 有 1 个无干预的新矩形→拉伸→全边圆角样例通过；复现入口为 ai-eval 的 box-fillet。 | 单样例不构成稳定成功率；曾干预恢复的调试样例不计为无干预成功，线上装配质量未验收。 |

测试文件相对 `services/internal/assistant/`，另列 Automation 与 Web 时按其模块路径；旧自然语言目标覆盖门禁已移除，其历史测试结论不能充当当前保证。原始过程由 Git 与 build/ 保留，不在主线累计日志。
