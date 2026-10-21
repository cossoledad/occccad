# 持续 AI Run 定向验收

2026-10-21，当前开发配置的 SQLite、ArtifactStore、Release Geometry Worker，独立测试账号及文档。未清库、未更改个人 Key。下表依据已实际运行的定向测试和真实浏览器；当前行为见[架构合同](../architecture/current/automation.md)，可重复命令见[测试路由](../../tests/README.md)及[小规模线上入口](../../services/cmd/occccad-ai-eval/README.md)。

## 证据边界

受控 Provider 驱动真实应用链路，不能证明真实模型规划质量。容量测试中的 2,050,000 Token 是合成 usage，未支付或消耗两百万线上 Token。原生几何验证与 Chromium/SwiftShader 呈现分别有证据。当前没有 PostgreSQL 实例运行证据，双迁移目录及合同已同步，实际 PostgreSQL 验收不能写成通过。

原始记录在 `build/acceptance/assistant-browser.json`、`assistant-concurrency.log`、`product-observation-native.json`，全范围检查记录在 `build/agent-check/`。这些为可清理构建制品；下列源码入口提供重建依据。

## 任务矩阵

| 项 | 结果 | 实际范围及重建入口 |
|---|---|---|
| T01 无固定调用上限 | PASS | `TestAssistantRunUnboundedCallsSyntheticMillionsAndRecovery`：同一Run，42次已发送（含1次结果未知），41次已报告响应，120次不同参数的实际查询；正常完成。 |
| T02 百万累计用量 | PASS，合成 | 同一Run 2,050,000 total＝1,640,000 prompt＋410,000 completion；cache1,230,000/reasoning328,000为子集。响应保存后重启、重复归档不重计；另有1次未知，未伪造为0。 |
| T03 总步骤/整轮时长 | PASS，受控执行 | `TestAssistantRunStagesHaveNoTotalStepOrRunDeadlineLimit`：3段共48步、可控时钟跨26分钟；容量测试跨3小时，单片段/请求仍有界。 |
| T04 同需求新拓扑 | PASS，原生+浏览器 | 一次需求、同一Run：空白Part→80×50×12→新Revision全部12条正式边→R2，真实实体有效；`TestAssistantRunStagesNativeObservationVerificationAndReceiptReplay`及browser首项。 |
| T05 冻结选择 | PASS，原生+浏览器 | 发送选定边后换选，实际R1仍使用原PersistentSelection；完整输入不截断。旧Revision正式重定位、重复/nested occurrence另有原生断言。 |
| T06 运行中修订 | PASS，受控+浏览器 | provider迟到create_task不能越过epoch；同Run厚度20/no-fillet更新goalVersion，旧已应用历史保留，最终按新目标检查。 |
| T07 只读问答 | PASS，HTTP+调度+浏览器 | VIEWER/历史只读，恶意工具写入拒绝；writer请求等待时独立QUERY能回答，重复拒绝不阻塞writer；修改与查询句分别路由。 |
| T08 后台/显示分层 | PASS，浏览器+原生 | 收起后服务端完成，reload同Run无新调用；GLB503不撤销COMPLETED、不伪造呈现，显示重试不改Revision/调用数；精确候选确认和过期再预览另测。 |
| T09 故障恢复 | PASS，边界注入 | 容量测试注入SENT未知、RECEIVED尚未处理；stage测试注入CAD提交后Run检查点未写、旧epoch退休回执。真实请求身份去重，未知计费如实保留。另有思考模式HTTP协议测试覆盖预算等待后的完整工具transcript重建。不是操作系统崩溃的全面混沌测试。 |
| T10 人工修改 | PASS，原生 | `TestAssistantRunManualHeadChangeDiscardsStageWithoutRebasingArguments`及旧选择测试：人工Head改变拒绝旧候选、重读/重定位，不替换旧集合Revision冒充解析。 |
| T11 上下文重建 | PASS，受控 | 24KiB测试工作集，40次重建，原目标尺寸/单位/保护措辞/方向/DOF来源保留；事件尾页/反向/增量及调用time+ID游标无重复。未声称任意自然语言约束均能结构化。 |
| T12 无进展/等待 | PASS，受控 | 相同事实/参数/诊断才BLOCKED；429由可控时钟验证退避；问题缺条件保存等待，用户新输入可修订，无累计失败次数终止。真实绑定失败在未checkpoint矩形之后归档一次，仅重规划拉伸；新请求身份重复同语义错误仍BLOCKED。 |
| T13 occurrence | PASS，原生 | 同名、重复Part、嵌套、PINNED/历史、逐层源ACL、转发Publication坐标和游标绑定；ContextVariant几何明确拒绝默认BRep替代；根/选区路径冒用源新Head或无关文档均拒绝。 |
| T14 最小装配 | PASS，受控Provider+原生 | `TestAssistantProductSameDurableRunInsertObserveMateAndVerify`：同根Run插入→Service重建→新occurrence/候选→原FIX/CONCENTRIC/端面命令→当前目标验收，2段/5应用/仅1插入。初始偏移(47,-31,19)mm+Y37°；真实solver CONVERGED，同轴/0mm端面，平移DOF0/旋转DOF1。候选用0mm底端面语义明确筛选，不选数组第一项；用户SAME/SAME/offset0/DOF1为独立来源合同，模型造反方向/偏置10不能自行验收。 |
| T15 UI接管/取消 | PASS，浏览器+原生 | 服务端已处理UI_OPENED后主动丢响应；reload不重放，取消不完成。原fillet工具同一预注册requestId完成preview/commit，仅精确receipt与原选边目标一致后完成；无关提交拒绝。 |
| T16 可选预算/状态 | PASS，受控+浏览器 | 默认null；主动预算用尽保存响应，提高后原Run续接无重复付费请求；问题预算不暂停writer。调用未知/重放/不同请求、缓存与reasoning子集均单独检查。 |

源码：[Assistant测试](../../services/internal/assistant/)、[Automation测试](../../services/internal/automation/)、[occurrence原生测试](../../services/internal/workspace/automation_occurrence_integration_test.go)、[真实工作台用例](../../web/apps/cad/browser/automation-live.spec.ts)。

## 小规模真实 DeepSeek

使用当前账号已保存的合法Key，保持模型/思考度/账号预算不变。没有更换或回显个人凭证，评测文档与Run保留，实际服务已恢复 `https://api.deepseek.com`。

最终新样例采用 `deepseek-flash / high`、主动120,000累计预算：一条新Run完成矩形→拉伸→新拓扑全部12边R2，应用及独立几何验收均通过。7次实际调用、17.31秒，报告prompt84,533 + completion1,738 = total86,271；cache45,952为prompt子集、reasoning490为completion子集，unknown0。没有人工修改参数、提高预算或修复该新Run；这只是1个新样例，不是稳定成功率。

之前两条调试样例在原Run内提高预算/部署恢复修复后也通过，分别记录197,784（1次usage未知）与108,989已报告Token；不能冒充无干预的新任务成功。真实样例触发并修复了预算后不完整思考工具transcript、早期无效绑定未回规划，以及评测器遗漏正式目标scope的问题。原始初次失败、续接和最终样例分别保留于 `build/acceptance/ai-eval-live*.json`，未覆盖失败历史。三条样例合计已报告393,044 Token，另有1次用量/计费未知；没有进行真实百万Token付费压测，也未执行线上装配模型质量评测。

可重复入口：`go run ./cmd/occccad-ai-eval --case box-fillet --token-budget 120000 --timeout 3m`（services目录）。独立检查使用回执角色的稳定目标IDs及固定预期值读取真实模型/Worker，不能仅依据Run的成功标签。费用未做货币估算。

## 全范围门槛

最终执行 `invoke check --scope all`：PASS_WITH_SKIPS，657.6秒。原生CTest403/403；Go1100通过、9项后端/可选条件明确不适用；跨模块conformance4/4；Web全部125场景及生产构建通过，源码在检查期间未变化。报告 `build/agent-check/20261021T063403-1d3a455b/report.json`。SQLite静态SQL审计发现的分页模板问题已改成表名白名单和固定SQL片段，未禁用审计器。文档审计8项、context-audit及git diff检查通过。PostgreSQL未运行；当前SQLite的校验结果不能替代它。
