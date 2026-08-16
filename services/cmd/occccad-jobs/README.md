# occccad-jobs

occccad-jobs 是当前 PostgreSQL 持久任务的消费者进程，适合脱离 HTTP 请求执行可重试工作。

## 当前职责

| 任务类型 | 输入 | 输出/副作用 |
|---|---|---|
| `EXCHANGE_IMPORT` | ArtifactStore 中的 STEP/BREP 对象 | 检查清单，并行导入根组件，创建带私有基准特征的 Part；多根再创建 Product |
| `EXCHANGE_EXPORT` | Part 或 Product 当前 Head 的 B-Rep 引用 | 生成 STEP/BREP，并把结果对象 ID 写回任务 |
| `THUMBNAIL_RENDER` | 文档与版本 | 生成 SVG 缩略图并更新 `document_previews` |

Worker 不提供网络 API，不接受用户认证请求，也不是通用分布式工作流引擎。
导入的 Part/Product 文档名统一使用清理路径后的完整上传文件名，包含 `.step`/`.brep` 后缀；客户端不再另行传入可分叉的文档名。Product STEP 导出按 occurrence 生成独立 transferable root，以保持当前展平 Product 再导入时的类型和 placement。

## 执行模型

```mermaid
flowchart LR
    API["occccad-server"] -->|"INSERT with idempotency key"| Queue[(PostgreSQL jobs)]
    Jobs["occccad-jobs"] -->|"FOR UPDATE SKIP LOCKED"| Queue
    Jobs --> DB[(Domain tables)]
    Jobs --> Store["Local ArtifactStore"]
    Jobs --> Geometry["Geometry gRPC"]
```

- 任务使用租约领取，默认租约 2 分钟，每 30 秒续约；
- 进程崩溃后，租约过期的 `RUNNING` 任务可被其他 Worker 重新领取；
- 失败任务在达到最大次数前进入 `RETRY_WAIT`；
- 领取语义是至少一次，任务处理器必须依赖幂等键和条件写入，不能假设“恰好一次”；
- 最终成功或最终失败与 `JOB` Outbox 在同一数据库 statement 中写入；API 通过 `job.state.changed.v1` 通知任务发起用户，重试等待状态不制造失败通知；
- 轮询为空时等待 1 秒。

## 依赖和配置

依赖 PostgreSQL、与 API 相同的 ArtifactStore 根目录，以及可用的 Geometry gRPC 地址。使用与 [occccad-server](../occccad-server/README.md) 相同的数据库、`OCCCCAD_DATA_DIR`、`OCCCCAD_GEOMETRY_WORKER_ADDRESS` 和 OTLP 配置。

在多进程或多主机部署中，所有实例必须看到同一套对象存储。当前本地目录后端不满足跨主机共享要求，因此当前 Jobs 只适合与 API 共享文件系统的部署。

## 运行

```bash
invoke run.jobs
```

完整本地拓扑使用：

```bash
invoke run.app --build-type=Debug
```

## 失败处理

- Exchange 导出提交后若文档 Head 已变化，任务失败，避免混合不同版本；
- 过时的缩略图任务直接成功结束，不覆盖新版本预览；
- 不支持的任务类型会失败并按队列策略重试；
- 制品写入成功但数据库提交失败时可能留下未引用对象，未来对象存储 GC 必须按引用扫描清理。

## 扩缩容与验证

多个实例可以并行消费，`SKIP LOCKED` 防止同时领取同一行。单个 Product 导入内部最多并行处理 8 个独立 root，正式 Geometry Router 可把它们分配给不同 Worker；最终 Product 组装与交换文件写出仍是确定性的 reduce 阶段。扩容前要确认 Geometry Worker 与共享制品存储容量；当前没有按任务类型隔离队列，耗时交换工作可能影响缩略图延迟。

```bash
cd services
go test ./...
```

当任务出现多阶段补偿、跨天计时、人工审批或复杂扇出时，再按[目标架构](../../../docs/TARGET_ARCHITECTURE.md)引入工作流引擎；不要因任务数量增加就立即替换当前简单队列。
