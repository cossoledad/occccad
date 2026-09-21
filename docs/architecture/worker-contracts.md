# Worker contracts focused reference

本文是 Proto、Geometry Worker、Router、Artifact 与 Job 路径的短投影，用于避免为一次 RPC 或任务修改整读两份架构总览。领域边界以[计算部署](target/compute-boundaries.md)和[分布式平台](target/distributed-platform.md)为准；已交付能力以[运行边界](current/runtime.md)、[Part RPC](current/part-sketch.md)、[Jobs/Artifact](current/jobs-artifacts.md)和代码为准。

## 权威边界

- Domain Command 与不可变 Revision 是业务事实；Worker 只消费不可变 manifest，不持有业务数据库凭据。
- 精确 B-Rep、mesh、缩略图和分析结果是带 provenance 的可重建制品，不是参数模型的替代品。
- 大制品经 ArtifactStore 交换；Proto 传稳定 ID、摘要、单位、容差、版本和对象引用，不传 OCCT 内部类型。
- Router 负责能力与实例路由，不得改变领域请求或用缓存结果绕过 Revision/CAS。
- Job 使用至少一次投递与效果幂等；attempt、lease、deadline、cancel 和迟到结果门禁都必须显式。

## 完整 RPC 路径

```text
Go command/evaluation
  -> generated GeometryWorker client
  -> GeometryPool / Router client
  -> Router proxy server
  -> selected Worker server
  -> evaluator / OCCT adapter
  -> immutable artifact
  -> short CAS commit against expected workspace head
```

新增或修改 RPC 时逐层核对：

1. Proto service、message 与 capability；
2. 生成的 Go/C++ 代码；
3. Worker server 实现；
4. 语言适配 client；
5. GeometryPool/Router 代理方法；
6. 应用正式 Router 路径的集成测试；
7. 结果 provenance、Artifact 写入和最终 CAS。

直连 Worker 成功不能证明正式调用链可用。任何代理层漏转发都可能在运行时表现为 `Unimplemented`。

## Job 与提交门禁

Job key 应覆盖规范输入、依赖 Revision、evaluator/kernel/solver 版本和策略 digest。Worker 可以重复计算和重复上传同一不可变制品，但最终业务效果必须按 transaction/job identity 幂等提交。

提交前至少验证：

- workspace head 仍等于请求中的 expected revision；
- job attempt/lease 仍有效且未取消；
- artifact digest、类型和 provenance 与请求匹配；
- 当前 Revision 的模型求值状态不会被旧成功结果覆盖。

对象已上传但 CAS 失败时，制品只能成为可回收孤儿；不能反向修改历史或静默覆盖新 Head。

## 失败分类

| 类别 | 示例 | 处理原则 |
|---|---|---|
| 用户模型 | 开轮廓、量纲错误、无解约束 | 稳定领域 code、可定位诊断，可形成失败 Revision |
| 数值/拓扑 | OCCT 失败、歧义、多解、容差退化 | 保留算法上下文与 provenance，不以旧几何冒充成功 |
| 冲突 | expected head 不匹配、lease 失效 | 拒绝迟到提交，由调用方重基或重算 |
| 基础设施 | timeout、Worker crash、对象存储不可用 | 在幂等边界内重试，区分 cancel 与 retryable failure |

## 源码路由

- 协议事实：先搜 `proto/` 中 RPC/message，再检查生成代码和锁定工具链；
- Worker：`workers/geometry/` 的 server、adapter 与 capability；
- Go 路由：`services/` 中 GeometryPool、Router client/server 和 Workspace evaluation；
- Artifact/Job：对应 store、job state machine、lease/cancel/commit 测试；
- 端到端入口：应用实际使用的 Router 地址，而非测试专用直连端口。

用 `rg -n 'RpcName|MessageName|CapabilityName' proto workers services` 定位同一契约的所有层，命中生成文件时回到 Proto 源定义，不直接修改生成代码。

## 完成门

- 通过正式 Router 路径验证 RPC 与 capability，而不只验证 Worker 直连；
- 覆盖重复投递、timeout/crash、cancel、lease 过期、CAS 冲突和对象上传后提交失败；
- 相同规范输入得到语义等价结果，缓存键含完整 provenance；
- Worker 无业务数据库凭据，持久协议无 OCCT/临时 topology/mesh primitive identity；
- 当前开发期的契约统一后从空数据重建；发布边界后才执行追加演进与旧协议兼容测试。

相关验证从 `invoke check --scope geometry`、`invoke check --scope services` 或精确 `--match` 开始；共享 Proto/Router 改动按 `invoke check --plan` 给出的升级路径执行。
