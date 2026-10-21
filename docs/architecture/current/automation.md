# CAD 自动化与持续 AI 建模

下边栏“建模助手”通过账号 Key、模型及思考度设置发起持久 Run。服务端持续读取事实、规划有界片段、执行原命令、观察新 Revision 并验证目标；关闭页面不暂停自动模式。累计预算默认 `null`，用户可主动设置并调整，默认无总调用数、工具数、步骤数、阶段数或整轮 deadline。

入口：[Assistant](../../../services/internal/assistant/README.md)、[Automation](../../../services/internal/automation/README.md)、[Web](../../../web/apps/cad/src/features/assistant/assistant-panel.tsx)、[线上评测](../../../services/cmd/occccad-ai-eval/README.md)、[剩余计划](../../../plans/ai-native-modeling.md)。本文说明当前实现合同，实际验收结果另见[验证目录](../../validation/README.md)。

## 所有权与执行链

```text
用户输入/冻结目标 → Run + 输入日志 → 服务端租约调度
  → 有界事实工作集 → Provider 调用意图 → 已发送 attempt → 响应/usage 落账
  → 查询或下一片段 Task → 原 Preview → evaluator/Worker → Command/CAS
  → 精确 CommandReceipt → 新 Revision / 实际几何 → 继续观察或目标验收
真实 Viewport frame → 对应文档/Revision 的呈现回报（独立于执行）
```

Run 是同一持续目标；多条 GOAL/STEER/QUERY 输入具有稳定序号和来源。STEER 增加 goalVersion/epoch，保留已提交事实，退休旧片段并等待在途边界；旧响应只记账，不能激活旧步骤。只读问题独立租约与冻结快照，不等待整个 writer 结束，不能创建/推进 Task。服务器重新检查 actor/document/source ACL，历史或 VIEWER 上下文只读。自动问答分类采用应用启发式，仍有语言覆盖边界；授权与写入校验不依赖分类结果。

单文档只有一个未结束 writer。Run 使用短事务 CAS、45 秒可续租，调度并发有界并为交互读预留位置；等待 Provider/Worker 不持数据库事务。租约到期允许另一进程接续，不是任务时限。轮询查询仅取待执行片段；浏览器不拥有推进职责。显式暂停/停止在边界生效，不撤销已提交历史。未知 CAD 提交必须先查原 actor/document/requestId 的回执，不能换身份重做；人工 Head 变化使旧候选失效，重读事实并重规划，不能仅替换 ExpectedHead。确定的绑定/参数错误先持久FAILED步骤、退休旧片段并精确归档已提交前缀，再带诊断重规划剩余目标；不会重放已应用前缀。相同语义失败的摘要独立于新Task/step/request身份，改名不能绕过无进展检测。权限、未知提交及UI取消保留各自等待边界。

## 日志、检查点与上下文

双数据库 `0006_assistant_runs.sql` 保存 Run、输入、阶段、事件、逐调用及工具结果、已读合同和设置来源。旧预算列删除，旧 low/medium/high 不映射为累计封顶；已有加密凭证保留。旧对话表仅遗留历史数据，无旧同步运行时或路由。

Run JSON 只保留有限角色目标、当前要求、摘要及计数；无限增长的输入/事件/阶段/调用独立分页。调用先保存意图，再标记发送，响应及 usage 先落账后才安排副作用。响应已收到但工具尚未处理时从原公开响应和工具游标恢复；工具结果按 call/tool ID 幂等。预算或恢复边界消费已落盘响应时丢弃旧在线会话的未回答工具尾部，下一请求从事实重建，避免发送不完整的思考工具 transcript。CAD 片段的精确回执与 Run applied 计数通过同一事务检查点归档。停止或旧 epoch 的不明 Provider 请求在失效租约后补记 UNKNOWN。

活动请求只加载目标及来源要求、冻结 scope 摘要、当前不可变上下文、有限稳定目标角色、最近少量片段/公开工具事实和语言。超出单次工作集时淘汰可重查内容并重建，不随完整历史线性膨胀。冻结选择的完整集合按 `cad_get_selection` 分页，预览子集明确不完整。必要信息本身超过配置时准确等待调整，不能丢弃尺寸、单位、保护对象或装配要求来缩短提示。

应用对矩形尺寸/平面、基体拉伸长度、全部/选定边圆角、取消圆角、圆形贯穿孔、受保护几何和最小轴孔目标提取带 sourceTurn/quote 的必需检查。验收从版本化用户输入分页重建要求；`GOAL:<requirementId>` 绑定该要求对应的正式操作输出，矩形/基体拉伸与圆形草图/REMOVE 切除分开。求值引起的其他草图 UPDATE 不能替换目标。缺少绑定时只按本 Run 的精确回执分页重建，不从名称、树位置或“最近创建的同类型特征”猜测。阶段结束先更新应用检查，已通过要求的同尺寸矩形重复计划在创建空草图前被拒绝；完成仍需当前目标、实际 Revision 与 Head 再验。

圆形贯穿孔检查绑定 REMOVE/THROUGH_ALL、轮廓直径、基准平面法向及当前实体中该特征的正式圆柱孔壁；矩形基体的中心孔通过权威草图支撑坐标系检查孔轴与原始矩形中心。孔后若有可能封堵孔口的实体操作，保留 NOT_VERIFIED，不能只凭原切除定义通过。通用复杂面中心、多孔和任意下游贯穿性不在此小合同保证内。模型不能移除失败项；混合自然语言要求仍有覆盖边界，不把这些局部检查当作完备规格编译器，也不因检查子集通过自动结束尚未明确完成的用户意图。

## 资源与 Provider 用量

单片段 1..32 步，Run 可继续追加任意多个片段。模型无执行器和呈现工具。搜索/查询/工具结果、单次输入与并发仍有边界；分页或重建之后可继续，不能把局部边界当整个目标的上限。不可分割的大操作须说明真实限制，例如全边一次圆角不能偷偷拆成语义不同的多次圆角。

默认单请求消息工作集 256 KiB、输出 8192 Token（思考开启时 max_tokens 32768）、网络 180 秒；工具执行复用既有操作 deadline。它们是当前配置保护，并非通用供应商能力或 Run deadline。每次调用保存 Provider origin、model、thinking、设置版本、实际 max_tokens、单次限制、逻辑调用/attempt、时间、Provider request ID（提供时）、请求摘要、状态及 usage。

用户可设置账号默认累计预算和活动 Run 的可空预算；达到后保留已收响应并等待，提高或清空后续接同一 Run，不重发已收响应。预算调整有版本及 USER 来源。输入 Token 不能提前精确获得，检查基于已报告 usage 和可知输出边界，最后一次可能超额。未知用量不记作确定零，不承诺金额硬限额。

输入/输出/总量按 Provider 字段记录；缓存是输入子集、reasoning 是输出子集，不再加入 total。原始数值细项经白名单保留。相同响应重复归档不重复累计；不同已发请求分别记账。超时/取消/进程丢失且没有 usage 时记 UNKNOWN，重试可能再次收费，不能承诺供应商无重复计费。相同事实、目标、操作参数和诊断没有新信息时 BLOCKED；限流按时间退避，无累计失败次数封顶。

默认 DeepSeek `https://api.deepseek.com` / `deepseek-flash`，模型通过当前账号 `/models` 获取；思考度 off/low/high/max。模型设置只影响后续调用，当前 attempt 不变。协议依据官方[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)和[思考/工具调用](https://api-docs.deepseek.com/guides/thinking_mode/)（2026-10-21 核对）。私有 reasoning_content 仅在在线内存 transcript 往返，不能进入持久响应、公开事件、日志或重建的历史。

AES-256-GCM、账号 AAD 和本地主密钥延续原凭证方案；主密钥与数据库共同恢复。API 只返回 configured 和无秘密设置。模型修改通过供应商能力校验；仅改预算不依赖 Provider 在线。HTTP 重定向不转发 Bearer，错误不透出上游原文。

## 命令、选择与动态绑定

目录复用所有公开 `command.*` 适配器与 Workbench `ui.*` 投影，覆盖测试对照真实 switch/catalog；合同覆盖不代表所有参数组合都通过原生验收。`part.fillet/chamfer` 等小操作减小 DTO 成本。先搜索再 describe，嵌套类型按需读取；未知字段、类型、数组维度、权限、几何和量纲仍由原校验负责。Host 固定 actor/document、requestId、baseline、preview 和 epoch，模型不能覆盖。

发送前完整复制选择，再读取正式 PersistentSelection；实例使用稳定 InstancePath，定义/根/源文档 Revision 分别保存；服务端逐路径核对接受的源文档与版本，不能冒用同源新Head或可读的无关定义。当前单输入最多 256 项及 512 KiB，超额拒绝整个输入，不静默截断。发送后换选不影响原指令。过期拓扑只经正式 Naming/selection resolver 的唯一 RESOLVED 结果重定位；失败或歧义要求重新选择，不依靠名称、mesh 序号、local ID 或改写 Revision。

`cad_query_model` 按不可变定义投影稳定实体、参数、草图子元素等，默认20/最多100项、32KiB单页，游标绑定 Revision/过滤条件。参数 value 是 SI quantity，displayValue 是未舍入的显示单位值。当前仍在受限定义上解析/排序，未提供持久查询索引或任意大树容量保证。

`cad_query_topology` 读取 Naming manifest，返回小页与完整集合 selectionSet；绑定 document/revision/body/kind/manifest/count。全部边使用完整集，新几何必须读新 Revision。输入已有完整选择不等于第一页内容完整；集合执行前再次校验。

`typedBindings` 仅声明允许的目标字段，绑定前序明确 outputId/role，可投影 ENTITY_ID、INSTANCE_PATH、ASSEMBLY_REF；复杂输出不猜“最后一个特征”或数组位置。实例输出使用提交模型已接受的 source Revision，不能另读 source 最新 Head。UUID 由原请求/步骤生成，恢复沿同一意图与精确回执。

## Product occurrence 与最小装配

`cad_query_occurrence` 按根 Revision + 稳定实例路径解析嵌套、重复和 PINNED 引用，逐层检查源 ACL，复用定义几何而分别返回 source snapshot、world pose、Body/Instance摘要或精确 FACE/EDGE/DATUM/PUBLICATION 小页。游标绑定根/路径/过滤条件，定义坐标与世界坐标明确区分；转发 Publication 按 accepted snapshot 和正式刚体变换派生。

`cad_mate_candidates` 使用精确圆柱/平面支持语义，区分材料内外侧、轴线/半径、长度、平面方向与偏置。默认紧凑候选，只在按 candidateId 选取时读取完整 recipe；多候选不静默选第一项。recipe 沿原 FIX/CONCENTRIC/端面关系命令与 solver，无第二套装配实体或几何脚本。

轴线/端面方向、偏置和目标DOF由用户来源合同逐字段保存，后续修改保留各自sourceTurn；缺失时澄清，不能默认猜测。正式验收检查真实持久位姿、同轴距离/角度、明确端面方向与 `SELECTED_PLANE_NORMAL_V1` 偏置、实际 solver 状态以及相对平移/旋转 DOF。求解出的假设修正姿态不能代替真实提交。支持普通轴/孔及端面定位、保留一个旋转自由度；不代表任意零件识别/复杂接触。带 ContextBinding 的 occurrence 目前明确返回 CONTEXT_VARIANT_NOT_AVAILABLE，不能替用普通定义 BRep 验收。跨文档原子写与整任务撤销仍未提供。

## 执行、接管与真实呈现

AUTOMATIC 持续在服务端推进；CONFIRM 每次等待精确 previewId、taskVersion、baseline 和 planEpoch 的批准，过期候选重新预览，旧批准不能用于新代际。

领域提交产生 APPLIED/精确 receipt；目标检查针对当前 goalVersion + 实际 Revision，全部必需检查成功后 VERIFIED/COMPLETED；真实 Viewport frame 才产生对应文档/Revision 的 PRESENTED。三者独立，无浏览器不会凭空显示。GLB加载失败只重试显示，不重做 CAD 或模型调用；重连不补造旧中间版本的显示。

`ui.*` 单步先持久 handoff/baseline/预期命令/DomainRequestID，再经原 CommandRegistry 检查可用性。打开结果丢失不自动重放。UI_OPENED 仅打开；取消明确记录并暂停；完成必须恢复同一预期请求、类型及基线的领域回执，无关提交不算完成。当前 SolidFeatureEditor 提供原 preview/commit request 身份桥，其他未提供安全完成桥的交互继续等待人工处理，不能伪造成功。

Web 继续由 TanStack Query 保存服务状态，原工作台选择/CommandRegistry/Viewport 各自拥有交互、命令与帧证据。下边栏使用单图标入口；面板以对话、15px 正文/输入及简短进展为主，用量、检查和原始事件折叠到详情。模型带工具调用的公开 content 按调用幂等保存为 ASSISTANT_MESSAGE；工具观察、实际 CAD 回执、阶段诊断及目标检查给出可读进度。公开简述在模型响应落账后显示，不承诺逐 Token 流式输出；私有 reasoning_content 不进入事件、账本响应或 UI。单独公开短答未执行操作时继续规划，同一事实下重复缺少下一步才等待补充；必要澄清使用 cad_wait_for_input。消息/步骤/事件按页增量读取，进展仅保留有界近期窗口，旧代际事件不能覆盖新状态。暂停/停止、精确确认、接管及显示重试保留原边界。

## 验证入口

- 服务：`go test -p 1 ./internal/assistant ./internal/automation ./internal/api -run '^TestAssistant|^TestAutomation'`（services目录）；容量/故障注入使用受控 Provider，不付费压测。
- Web：`invoke check --scope web --match assistant`；真实工作台先用本地19091 Provider启动服务，再运行 `pnpm exec playwright test --config playwright.live.config.ts automation-live.spec.ts`。独立测试账号不改个人Key，结束恢复默认 DeepSeek 配置。
- 原生几何、迁移与共享链：按 `invoke check --scope all`，数据库当前配置之外的 Provider 验证必须单独标明未执行。
- 线上小样：`services/cmd/occccad-ai-eval` 使用已有合法 Key 和显式小预算。模型表现、原生几何、浏览器、合成百万 usage 必须分别报告。
