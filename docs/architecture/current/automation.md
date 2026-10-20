# CAD 自动化与 AI 接入准备

当前提供模型无关的操作目录、有界定义查询、持久建模任务和右下角建模助手；尚未连接 LLM，DeepSeek 仅为后续 API 测试候选。没有模型 SDK、供应商凭证、自然语言执行循环、MCP Server 或 CAD CLI。

## 入口与调用链

```text
Web 建模助手 / 未来模型适配器
  → Automation：发现、查询、任务、授权
  → 现有 Workspace PreviewCommand / ExecuteCommand
  → evaluator / Worker → CAS → Revision / 回执
  → 浏览器实际渲染 → 步骤显示确认
```

所有模块留在现有控制面中：[automation](../../../services/internal/automation/README.md) 拥有操作、查询和任务；[assistant](../../../services/internal/assistant/README.md) 提供工具与上下文适配；[API](../../../services/internal/api/automation.go) 提供 HTTP 查询/任务定义和鉴权 WebSocket 控制；[Web](../../../web/apps/cad/src/features/assistant/assistant-panel.tsx) 拥有流程面板，服务端任务归 TanStack Query。

## 操作与一致性

操作目录包含 `sketch.create`、`sketch.rectangle`、`part.extrude`、`parameter.set_literal`、`parameter.set_expression`。字段描述生成输入 schema，与编译校验共用；CAD 语义仍由现有 handler 拥有。搜索返回短描述，describe 才读取完整合同。矩形复用 ADD_RECTANGLE 宏和约束，不承诺完全约束；拉伸长度明确为毫米，参数继续权威检查量纲、表达式和依赖。

写入面向独立 Part 的 main Workspace。稳定 requestId 和 `expectedHead={workspaceId,revisionId,sequence}` 必填，领域命令准备阶段先核对调用方基线，再沿用求值后 CAS。先成功预览，后以同一意图提交；失效候选明确拒绝。创建草图按请求生成确定身份，保证预览、提交和重试一致。

读取/执行检查当前权限；工具参数不能替换 Host 绑定的用户和文档。当前操作不接受跨文档写入或任意 shell/SQL。求值按用户和服务总量限流，并有单次 deadline；昂贵计算不持有数据库事务。 任务错误保留原领域 code、phase、retryable 和 diagnosticId，数值未收敛不会被改写为不可行结论。

## 查询与上下文

`ReadModelSnapshot` 读取不可变定义，不构建 DocumentView、引用树、几何或标签。Part 投影 Body、Feature/Sketch、Parameter、DatumPlane；Product 只投影本级 Instance 定义，尚不展开 occurrence。参数的 Body 范围由所属 Feature 的稳定身份派生。历史读取校验文档归属，游标固定 Revision 与过滤条件。

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

真实 API 模式的独立 Part 右下角提供矩形拉伸流程，可逐步确认、自动逐步执行、暂停接管、定位及重载恢复。正式结果加载并执行渲染后才确认显示；显示失败不能推进。活动面板按需轮询服务端状态，关闭/失联后不会自行提交下一步；没有脱离 UI 的模型执行循环。

## 验证入口与限制

- `invoke check --scope services --match '^TestAutomation'`：目录、合成分页/预算、真实 Worker 建模/参数/历史、回执恢复、接管/基线和 HTTP/WebSocket 边界。
- `invoke check --scope web --match automation-task`：步骤绑定、精确显示确认和迟到响应门禁。
- 当前 API/Worker 运行时，在 Web 目录执行 `pnpm exec playwright test --config playwright.live.config.ts automation-live.spec.ts`，验证真实 Vite Upgrade、预览、每步显示、暂停与重载。
- 迁移及共享命令边界变化升级 `invoke check --scope all`；知识入口运行 `invoke context-audit`。

上述为入口，实际结果见当次报告。模型效果、真实 token 成本、工业容量和其他领域自动化待独立验收，见[后续计划](../../../plans/ai-native-modeling.md)。
