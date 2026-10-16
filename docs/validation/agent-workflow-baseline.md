# Agent 工作流本机基线

测量日期：2026-10-16。源码与资源身份、原始任务包和验证报告见[机器可读数据](agent-workflow-baseline.json)。这是统一开发环境后的起点，不与旧环境比较。

复测：先构建当前 Worker，再执行 `invoke workflow-baseline`。五类任务串行各采集一个定位包，并执行配方中对应的真实定向验证。

| 任务 | 搜索调用 | 返回源码 B | 定位 ms | 验证 s | 原始测试输出 B | 结果 |
|---|---|---|---|---|---|---|
| property | 3 | 2776 | 5.52 | 23.20 | 66258 | PASS |
| rpc | 5 | 3073 | 8.54 | 6.47 | 27152 | PASS |
| selection | 3 | 4142 | 5.54 | 1.01 | 106 | PASS |
| feature | 3 | 2620 | 5.76 | 15.44 | 92857 | PASS |
| diagnostic | 4 | 1992 | 6.12 | 0.72 | 2271 | PASS |

只测量定位包内的搜索与返回片段、定向验证 wall time 和完整原始输出。终端默认只展示验证摘要，输出字节列包括保存在磁盘的日志。未统计实现、思考、后续读取、返工、环境准备或人工介入；模型 token 为 null。每类 n=1，不声称稳定分位数、节省比例或端到端开发提速。

任务义务与升级验证范围归[Agent 工作流](../development/agent-workflow.md)。完整测试、Mock 浏览器、真实 Router 与 Live 浏览器各自记录范围；本表不代表全仓通过。


## 集成验证与边界

本轮在当前开发 SQLite/LOCAL 上执行全量 C++/Go/Web，并在不清库的情况下重复真实 fixture。共享环境下的空队列假设、固定 request ID 和 PostgreSQL 专用测试 SQL 已按实际合同修正；Jobs 和 Loft 各连续重复 3 次通过。工具负例验证 skip、零匹配、进程失败和源码变化不能变为 PASS，装配合同仍保留原有语义与覆盖下限。

全仓尚不能报告通过：当前未修改的 `GeometryExchange.ShellAfterLargeModifiersAndMixedLoft` 和 `ShellOffsetRectangleToLargerCircle` 在 CTest 中失败；后一几何也在真实 Worker 的 Shell 场景中失败。错误为偏移算法/容差门禁拒绝。本轮未放宽精确几何、容差、拓扑或原有测试预期。

本轮选择 SQLite/LOCAL，PostgreSQL/S3 专用行为未执行；可选大 STEP corpus 和缩略图 gallery 不代表默认回归能力。基线表只记录配方中的定向范围，不能覆盖这些限制。


Live 验收使用正式 `invoke run --reset-data` 启动的 Control/API/Jobs/Router/Worker 与同一 `.env`。已清理当前 `services/data/occccad-local.db` 全部业务表/迁移及 `services/data` 制品/暂存，重新迁移；本机选择 LOCAL，没有清理 S3。此次重置发生在性能测量之后，性能报告仍描述其测量时已有开发数据的规模。

- 跨模块 conformance 4 项通过：2 项文件名合同，以及真实双客户端幂等提交、后台 STEP 导入及 WebSocket 终态通知这 2 项链路；Queued/Running 不再误当作终态。
- Live 浏览器 3 场景通过：Part 返回 Product、固定约束创建/未修改 Enter/编辑的权威提交、Boolean 拾取/自动预览/提交/重开。固定位置 `7.654` 和 `9.876` 持久值保留，显示仍为两位小数。
- Mock 浏览器 5 场景通过：固定表单/取消、固联组成员选择、基准轴预览，以及数字输入的失焦/Enter/真实输入与原有范围校验；不替代真实精确几何。
- 真实 Jobs 循环接收损坏 STEP，失败归档的源对象与 attempt 等于实际认领值，输入摘要覆盖源/文档/Revision/actor/payload；明确没有可重放数值请求，未伪造完整领域离线重放。

原始日志归 `build/acceptance/final-live.log`、`build/acceptance/final-mock.log`；跨模块结果归 `build/agent-check/20261016T101104-9d1f210e/report.json`。运行制品身份由 `invoke doctor --live --browser` 实际核对，资源可达与上述行为验收分开。

最终 Web 领域检查为 110 个场景与生产构建通过；开发工具 53 项、装配合同工具 22 项通过，58 个能力/677 个案例的锁校验未降低语义或覆盖下限。日志摘要与报告路径见同页机器可读数据。
