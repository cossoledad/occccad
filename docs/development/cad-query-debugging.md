# CAD 只读查询调试

目的：无需配置模型或创建 Agent Run，检查现有 CAD 查询是否给出了足够的建模信息。链路所有权与输出合同见[自动化架构](../architecture/current/automation.md#查询数据与模型输出)；入口在工作台属性检查器的“查询调试”。本工具不执行建模、自动识别孔口或估算模型 Token。

## 固定范围与执行

1. 明确填写文档、Revision 和 Body。模型查询可留空 Body，拓扑查询必须指定 Body。Revision 必填；无效或无权限的历史版本直接报错。
2. “采用当前文档与选区”显式复制来源版本及完整选区，再读取其正式 PersistentSelection。随后换选或 Head 变化不会改输入。跨文档/版本、多个 Body 或不支持的 ContextVariant 不能混用。最多 20 个固定选择。
3. 选择现有工具并填写 JSON 参数。拓扑默认是紧凑观察，可用控件填写几何类型、半径等值/范围、显式 mm 容差与关联面稳定 ID；控件同步同一份 JSON，修改筛选会清除旧游标，不会立即查询。文档和 actor 绑定在应用边界，Revision/Body 参数必须与固定范围相符。“仅查询已固定的选区”支持一个模型对象，或同类型拓扑集合。
4. 执行后默认查看紧凑几何摘要、匹配/本页/集合数量及未知数量，完整 JSON、固定输入和实际参数可展开。查看记录不会选择场景或重新捕获输入。“读取此快照的完整证据”作为新的显式查询记录，只读该条稳定 ID，保留原快照及筛选，不改编辑框；“下一服务页”同样只沿用该记录的固定输入。历史查询不会改成最新 Head。

读取紧凑圆柱面，可人工根据位置、半径和材料侧确定明确目标的面 ID：

```json
{"kind":"FACE","limit":20,"filter":{"geometryType":"CYLINDER","radius":{"equalMM":3,"toleranceMM":0.000001}}}
```

随后筛出与该指定面关联的 R3 圆边；`adjacentFaceId` 必须换成上述查询返回的稳定面 ID，不能用名字、local ID 或跨 Revision 的 ID：

```json
{"kind":"EDGE","limit":1,"filter":{"geometryType":"CIRCLE","radius":{"equalMM":3,"toleranceMM":0.000001},"adjacentFaceId":"<queried-stable-face-id>"}}
```

半径范围使用 `radius:{"minMM":2.9,"maxMM":3.1,"toleranceMM":0.000001}`，不能与 equalMM 同时使用。等值判断为绝对误差 ≤ toleranceMM；范围包含容差扩张后的上下界。不设置半径条件时，不要求半径可用。

完整正式引用由服务端按需返回；用 `{"kind":"EDGE","ids":["<queried-stable-edge-id>"],"limit":1,"includeSelections":true}`，或点击该行的证据按钮。默认响应中没有 PersistentSelection/sourceIds/creationEvidence，不能靠前端隐藏来减小模型数据。

scope 明确 PART_LOCAL 与 mm/rad；lengthMM、areaMM2 已从 Naming SI 测量转换，未做显示舍入。圆的 originMM 是圆心，圆柱为解析轴原点，质心是 centroidMM。参数范围含 LINE_MM/ANGLE_RADIANS/NATIVE_CURVE_PARAMETER 约定；不支持的解析框架不返回零向量占位。unavailable 表示缺失/不支持的适用字段；省略的属性没有可用值，不等于零。方向不能直接判定孔入口/出口。

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

字节数指实际完整 JSON 内容的 UTF-8 字节（含 scope、集合和游标），不是格式化 `<pre>`、HTTP 总大小或 Token。筛选先作用于完整固定范围，再生成集合，最后分页。scopeCount 是 Body 同类拓扑数，candidateCount 是 ids 限定后的数；matchedCount/unknownCount 是已知匹配/无法判断数，returnedCount 是当前页数，selectionSetCount 是集合数。

“集合引用覆盖全部匹配”来自 selectionSetComplete；“本页包含匹配全集”来自 pageComplete/complete。未知成员会令 matchComplete/selectionSetComplete 为 false，该集合不能执行；零匹配集合也不能执行。最后一页即使没有后续页，也未必包含整个集合。浅层邻接 ids 最多展示 16 个，knownCount 是完整内部已知面关系数；complete 为 false 表示邻接证据缺失、存在未解析关系或列表未列全。筛选使用完整关系，不只检查所显示的 16 个 ID。工具资源错误时，人工页中的计数不代表模型已收到它们。

服务端只读入口沿用当前登录态、CSRF 和 Viewer 权限，不依赖个人 Key。UI 本地保留最近 10 条查询，离开该页不形成持久 Run。单次工具内容 32 KiB、人工原页 1 MiB、请求 JSON 64 KiB，模型查询最多 100 项、拓扑最多 20 项；可缩小字段或分页。未增加持续任务总调用数、总 Token 或总阶段上限。

## 人工场景

使用“20 mm 立方体 → 所有 12 条边倒角 1 mm → XY 中心 Ø6 mm 贯穿孔 → 检查两个孔口边”。用紧凑圆柱面信息人工确定指定面，再按 CIRCLE、R3 与关联面筛选；limit=1 时本页只有一条，集合仍须有两条。查看另页或展开一条正式证据，原范围应不变。原生回归还覆盖同半径第二孔/凸台干扰与筛选集合的真实倒角。人工确定圆柱面不是自动理解“孔口”；不根据命名字符串识别，不按显示名、local ID 或第一页位置保存身份。

当前查询范围、真实 Worker 证据边界和缺口见[自动化合同](../architecture/current/automation.md#当前限制)。轻量重建入口：

```sh
# services/
go test -tags=integration -p 1 ./internal/workspace ./internal/automation ./internal/assistant ./internal/api -run '^TestTopology(ObservationCurrentMeasurementsAndIdentity|Filter|Unknown)|^TestAssistant(ToolResult|Debug)|^TestAutomationDebug' -count=1 -v
# web/apps/cad/
pnpm test -- automation-debug
pnpm typecheck
```

原生用例使用当前 `.env` 的数据库、制品和生产 Worker，保留独立开发文档便于人工查看；不会创建 Run、配置 Key 或调用模型。原始报告在 `build/agent-check/automation-debug/`。这些测试不能代替真实界面操作与模型规划验收。
