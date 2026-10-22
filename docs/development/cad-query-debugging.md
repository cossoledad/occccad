# CAD 只读查询调试

目的：无需配置模型或创建 Agent Run，检查现有 CAD 查询是否给出了足够的建模信息。链路所有权与输出合同见[自动化架构](../architecture/current/automation.md#查询数据与模型输出)；入口在工作台属性检查器的“查询调试”。本工具不执行建模、自动识别孔口或估算模型 Token。

## 固定范围与执行

1. 明确填写文档、Revision 和 Body。模型查询可留空 Body，拓扑查询必须指定 Body。Revision 必填；无效或无权限的历史版本直接报错。
2. “采用当前文档与选区”显式复制来源版本及完整选区，再读取其正式 PersistentSelection。随后换选或 Head 变化不会改输入。跨文档/版本、多个 Body 或不支持的 ContextVariant 不能混用。最多 20 个固定选择。
3. 选择现有工具并填写 JSON 参数。文档和 actor 绑定在应用边界，Revision/Body 参数必须与固定范围相符。“仅查询已固定的选区”支持一个模型对象，或同类型拓扑集合；不猜测目标，不将部分选择宣称为完整。
4. 执行后查看固定输入和实际绑定参数。查看某条本地记录或本页对象详情不会选中场景对象，也不会重新捕获输入。“下一服务页”使用该记录的原始固定输入和游标，不能套用当前编辑框或新选择。

读取详细边证据的起点：

```json
{"kind":"EDGE","limit":1,"includeSelections":true}
```

读取精简特征定义摘要：

```json
{"kind":"FEATURE","fields":["name","type","bodyId","profileId","length","unit"],"limit":20}
```

读取完整定义使用 `fields:["definition"]`，但单个大特征也可能超过模型输出边界。不能把设置 `limit:1` 当作必然解决大对象问题。`cad_context` 始终为固定 Revision 的文档级摘要，不按 Body 裁剪计数。

## 三类可见信息

| 显示 | 含义 |
|---|---|
| 模型可见输出 | 经过与真实 Host/账本相同的规范化处理；普通规划、问答、恢复重放和 trace 使用它。超限显示资源错误，不附带原页。 |
| 服务原页（人工对照） | 工具处理之前的完整服务响应页，仍受 Automation 自身字段/分页约束；并不代表整个模型或整个拓扑集。超过人工响应边界会明确省略。 |
| 人工补充信息 | 固定选区时读取的属性、来源快照、正式选择解析证据，以及分页诊断。没有自动添加到模型输出。若手工修改范围，须核对补充数据标注的原来源。 |

字节数指实际 JSON 内容的 UTF-8 字节，不是格式化 `<pre>`、HTTP 总大小或 Token。“集合引用覆盖全部匹配”来自 selectionSetComplete；“本页包含匹配全集”来自 pageComplete/complete。最后一页即使没有后续页，也未必包含整个集合；它不是损坏的半页。工具资源错误时，人工页中的游标和计数也不代表模型已收到它们。

服务端只读入口沿用当前登录态、CSRF 和 Viewer 权限，不依赖个人 Key。UI 本地保留最近 10 条查询，离开该页不形成持久 Run。单次工具内容 32 KiB、人工原页 1 MiB、请求 JSON 64 KiB，模型查询最多 100 项、拓扑最多 20 项；可缩小字段或分页。未增加持续任务总调用数、总 Token 或总阶段上限。

## 人工场景

使用“20 mm 立方体 → 所有 12 条边倒角 1 mm → XY 中心 Ø6 mm 贯穿孔 → 检查两个孔口边”。先查完整集合，再用小页读取详细证据。可以在视口人工选两个边并显式捕获，检查正式引用与结果是否一致；这只验证选区传递，不能算“两个孔口”的自动语义定位。不要按显示名、local ID 或第一页位置保存身份。

本轮真实 Worker 数据和已确认缺口见[基础回归](../validation/cad-query-debug-baseline.md)。轻量重建入口：

```sh
# services/
go test -p 1 ./internal/assistant ./internal/api ./internal/automation -run '^TestAssistant(ToolResult|Debug)|^TestAutomationDebug' -count=1 -v
# web/apps/cad/
pnpm test -- automation-debug
pnpm typecheck
```

原生用例使用当前 `.env` 的数据库、制品和生产 Worker，保留独立开发文档便于人工查看；不会创建 Run、配置 Key 或调用模型。原始报告在 `build/agent-check/automation-debug/`。这些测试不能代替真实界面操作与模型规划验收。
