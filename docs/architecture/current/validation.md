# 可观测性与验证

> 返回[当前架构目录](README.md)。日志与诊断在本文维护；检查执行和状态判定见[Agent 工作流](../../development/agent-workflow.md)。

## 请求与计时

Go HTTP/gRPC 传播 OpenTelemetry Trace Context；配置 OTLP 时导出 Trace。C++ Worker 按地址写滚动日志，保留 RPC、request ID 和 traceparent。一次请求的低基数 Recorder 将 prepare、apply、求解、几何、候选提升和 commit 等阶段写入日志的 `phases_ms` 和响应的 `Server-Timing`。同一个 request ID 可以关联服务与 Worker。浏览器下载、绘制和交互延迟需独立采集。

[测试路由](../../../tests/README.md) · [工程验证场景](../../validation/README.md) · [执行报告与状态](../../development/agent-workflow.md)

## 操作失败诊断快照

API 初始化 `debug/cad-diagnostics` 的现有 `debugartifact.Store`，按文档最多 50 份、7 天保存操作失败；读取时也执行过期清理。`PreviewCommand` 和普通 Domain Command 应用失败记录原请求、可用的规范命令、失败阶段、基准 Revision/模型、已构造的未提交候选及基准 Body 的 BREP/Naming/GLB 引用、evaluator/Naming policy 与制品内的 kernel/Worker 版本。适配前失败可能没有候选；用户取消不归档。Undo/Redo/Restore 的失败使用 HISTORY 模式，保留实际基准 Revision、模型和已构造候选；已提交 FAILED Revision 以 `ACCEPTED_FAILED_REVISION` 及结果 Revision ID 归档，成功提交仍返回正常结果。单份上限 8 MiB，归档失败记录日志并保留原业务错误，不改变提交语义。

错误通过 HTTP/WebSocket 携带 `diagnosticId`（`documentId/recordId`）。`GET /api/documents/{documentID}/diagnostics/{diagnosticID}` 经当前文档 Viewer 权限返回失败时的不可变 JSON；不拼入查询时的新 Head，缺失/过期返回 404。服务重开仍可读取本机日志目录的记录，不创建业务 Revision或提交候选，不收集凭据或无关日志。历史手动诊断包下载入口仍可用。

Web 操作诊断和实体/阵列/布尔预览提供“复制诊断”，复制 `CAD_DIAGNOSTIC <documentId>/<recordId> <code>`；剪贴板不可用或拒绝时显示可选择的文本，不虚报成功。没有服务端记录时复制已有阶段、操作请求 ID、错误及可用文档/Revision 上下文。普通命令与预览共享结构化错误，预览保留原错误对象并清除旧成功候选。后台 Job 普通执行失败另在同一日志根的 `debug/job-diagnostics` 保存认领时 payload、源对象/文档/Revision/actor 引用、完整输入摘要、类型、attempt、失败阶段及关联操作诊断；失败消息携带 `CAD_JOB_DIAGNOSTIC jobId/recordId`。取消不作为普通执行失败留档，归档不改变 Job 的 lease/取消/CAS 语义。归档失败不伪造可下载引用。

`mise run diagnose` 统一定位操作与 Job 归档，列表与摘要不读取新 Head；已有 `assemblyAttempts` 可导出并通过生产数值重放执行，输出与源文件分开。没有已记录数值请求时明确说明范围；通用领域事务离线重放未交付。任务配方的源码路径和符号由审计检查，配方只路由工程任务，不替代生产合同。
