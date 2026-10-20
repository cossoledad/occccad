# CAD 自动化与 AI 对话建模

当前提供 DeepSeek 对话接入、模型无关的操作目录、有界定义查询、持久建模任务和右下角对话助手。用户填写自己的 API Key 即可连接；支持独立 Part 的矩形草图、拉伸和参数修改。MCP Server、CAD CLI、任意长动态计划和装配自动化尚未提供。

## 入口与调用链

```text
Web 对话 → DeepSeek Chat Completions → 受控 Tool Host → 持久建模计划
  → Web 逐步驱动 Automation：预览、授权、执行
  → 现有 Workspace PreviewCommand / ExecuteCommand
  → evaluator / Worker → CAS → Revision / 回执
  → 浏览器实际渲染 → 步骤显示确认
```

所有模块留在现有控制面中：[automation](../../../services/internal/automation/README.md) 拥有操作、查询和任务；[assistant](../../../services/internal/assistant/README.md) 拥有凭证、对话、供应商客户端和工具适配；[API](../../../services/internal/api/assistant.go) 提供 HTTP 对话/连接设置，操作执行沿[鉴权 WebSocket](../../../services/internal/api/automation.go)；[Web](../../../web/apps/cad/src/features/assistant/assistant-panel.tsx) 拥有对话及步骤卡片，服务端状态归 TanStack Query。

## DeepSeek 对话与凭证

默认以 `https://api.deepseek.com/chat/completions`、`deepseek-flash`、Bearer Key、非流式/非思考模式和原生 function tools 调用。实现依据 [DeepSeek 首次调用](https://api-docs.deepseek.com/)、[工具调用](https://api-docs.deepseek.com/guides/tool_calls/)与[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)官方合同（2026-10-20 核对）。供应商输出按格式、大小、操作 schema、当前权限及 Head 校验，不执行任意模型脚本。

用户只填 API Key。保存时调用 `/models` 验证权限与已配置模型，再以 AES-256-GCM 和账号 AAD 加密写入数据库；HTTP 只返回 configured/provider/model，不回显 Key。主密钥自动生成于 `<OCCCCAD_DATA_DIR>/assistant/credentials.key`，文件权限 0600。服务端可选 `OCCCCAD_AI_BASE_URL`、`OCCCCAD_AI_MODEL`、`OCCCCAD_AI_KEY_FILE`，多实例共用主密钥。上游错误按状态映射为无密钥的错误信息，不输出原始响应或重定向转发凭证。

`0003_assistant.sql` 在两种数据库中增加账号凭证及用户/文档隔离的对话记录。请求幂等，记录原始需求、回复、taskId 和供应商 usage；同用户/文档最多一轮规划在进行。150 秒 deadline、165 秒租约和取消状态防止迟到结果复活；过期请求变为 INTERRUPTED，刷新不重发付费请求、不自动开始新写入。停止回复与停止建模分别控制模型规划和已生成任务。若计划已写入而对话尚未保存 taskId，恢复时按原请求幂等键找回并停止取消/过期的计划，不留下阻塞后续请求的孤立任务。

规划阶段仅向模型暴露查询、发现、describe、检查点和 `cad_create_task`，不暴露 advance。应用绑定冻结 Head/用户/文档/requestId，将局部步骤标签转换为确定 UUID，并校验前序输出绑定。每轮最多生成一个计划；每个使用的操作必须先读取合同。计划成功后结束模型循环，执行阶段复用确定性 CAD 通道，不为每个预览/提交再调用模型。当前状态和精确回执由后续对话重新查询，模型语言不充当完成证据。

默认携带最多 8 轮、16 KiB 的近期语言上下文，至多 3 项/16 KiB 的完整任务检查点，额外历史保留引用并按需查询。当前工具结果最多 16 KiB、消息上下文 72 KiB、完整供应商请求 96 KiB；超限明确失败。规划最多 10 次模型请求、24 次工具调用、每次输出 4096 tokens；累计供应商报告 usage 超过 40,000 tokens 后停止下一步，本次请求仍计费。usage 缺失不伪造 token 估计，字节/轮数/时间限制仍生效；未提供按货币金额的预算。

## 操作与一致性

操作目录包含 `sketch.create`、`sketch.rectangle`、`part.extrude`、`parameter.set_literal`、`parameter.set_expression`。字段描述生成输入 schema，与编译校验共用；CAD 语义仍由现有 handler 拥有。搜索返回短描述，describe 才读取完整合同。矩形复用 ADD_RECTANGLE 宏和约束，不承诺完全约束；拉伸长度明确为毫米，参数继续权威检查量纲、表达式和依赖。

写入面向独立 Part 的 main Workspace。稳定 requestId 和 `expectedHead={workspaceId,revisionId,sequence}` 必填，领域命令准备阶段先核对调用方基线，再沿用求值后 CAS。先成功预览，后以同一意图提交；失效候选明确拒绝。创建草图按请求生成确定身份，保证预览、提交和重试一致。

读取/执行检查当前权限；工具参数不能替换 Host 绑定的用户和文档。当前操作不接受跨文档写入或任意 shell/SQL。求值按用户和服务总量限流，并有单次 deadline；昂贵计算不持有数据库事务。 任务错误保留原领域 code、phase、retryable 和 diagnosticId，数值未收敛不会被改写为不可行结论。

## 查询与上下文

`ReadModelSnapshot` 读取不可变定义，不构建 DocumentView、引用树、几何或标签。Part 投影 Body、Feature/Sketch、Parameter、DatumPlane；Product 只投影本级 Instance 定义，尚不展开 occurrence。参数的 Body 范围由所属 Feature 的稳定身份派生；`value` 保留 SI quantity，`displayValue` 为对应 `unit` 的未舍入数值，两者不混用。历史读取校验文档归属，游标固定 Revision 与过滤条件。

查询支持 kind、稳定 ID、Body、名称和字段投影；默认 20 项、最多 100 项，单页对象预算 32 KiB。返回 matchedCount、returnedCount、hasMore 和 nextCursor；complete 仅在一次返回完整匹配集时为真。定义读取/投影有预算，超额明确失败。当前每次解析受限定义并排序分页，未实现持久索引、完整集合句柄或懒加载树；合成 5 万节点查询不构成大型装配容量验收。

工具入口为 `cad_context`、`cad_search_operations`、`cad_describe_operation`、`cad_query_model`、`cad_create_task`、`cad_get_task`、`cad_advance_task`。Host 写权限来自应用，模型不能发出浏览器显示确认。上下文加载简短规则、当前快照、任务检查点与至多 6 项相关操作合同，按字节预算明确拒绝超额；不把字节冒充模型 token，不内联完整模型/几何/聊天历史。

## 任务、接管与显示

两种数据库的 `0002_automation_tasks.sql` 增加任务账本，不改写旧 Revision。每任务最多 32 步，保存稳定步骤 ID、版本、输入、前序输出绑定、基线、阶段、回执和有界事件。定义创建幂等，更新按 task version CAS；Workspace 短锁和执行租约协调自动化，普通人工编辑仍受原 CAS 保护。

```text
PLANNED → PREPARING → PREVIEW_READY → COMMITTING → COMMITTED → DISPLAYED
                              失败 → FAILED / PAUSED
```

每次 advance 推进预览或提交一个阶段；提交后须确认对应步骤/Revision 已显示，再推进下一步。服务重建或结果未知时先查原 requestId 的领域回执，避免重复创建；候选丢失则重新预览。

暂停/停止在操作边界生效。继续任务可显式接受当前 Head，后续重新校验/预览；不会静默套用旧意图。被人工新版本取代的已提交步骤标记 SUPERSEDED，保留原回执，不冒充已显示。停止保留已提交步骤，显示确认不撤销停止。整任务撤销与跨文档原子操作尚未提供。

真实 API 模式的独立 Part 右下角提供对话入口；视口宽度充足时与画布并排，否则悬浮，沿用视口 ResizeObserver 更新绘制和拾取范围。对话含一次 Key 连接、需求输入、澄清/回复、尺寸和实际步骤卡片。默认在成功发送并获得计划后逐步执行，可切换逐步确认，支持暂停接管、停止、定位与重载恢复。恢复出的计划须用户主动继续；正式结果加载并执行渲染后才确认显示，显示失败不能推进。面板按需轮询，关闭/页面隐藏时暂停后续推进；没有脱离 UI 的后台建模。

## 验证入口与限制

- `invoke check --scope services --match '^TestAssistant|^TestAutomation'`：协议/密钥/取消/预算、幂等对话、查询及真实 Worker 建模、历史、回执恢复、接管和 HTTP/WebSocket 边界。
- `invoke check --scope web --match assistant`：步骤绑定、精确显示确认、对话排序和迟到响应门禁。
- 以 `OCCCCAD_AI_BASE_URL=http://127.0.0.1:19091 invoke run` 启动 API/Worker 后，在 Web 目录执行 `pnpm exec playwright test --config playwright.live.config.ts automation-live.spec.ts`。该测试自行启动受控 OpenAI 格式服务并创建独立账号，验证真实 Vite Upgrade、Key 设置、对话→计划→原生建模、逐步显示、暂停与重载；结束后以默认配置重启应用。它不测线上模型推理效果，不覆盖个人保存的 Key。
- 迁移及共享命令边界变化升级 `invoke check --scope all`；知识入口运行 `invoke context-audit`。

上述为入口，实际结果见当次报告。模型效果、真实 token 成本、工业容量和其他领域自动化待独立验收，见[后续计划](../../../plans/ai-native-modeling.md)。
