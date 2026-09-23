# occccad-jobs

occccad-jobs 是当前 PostgreSQL 持久任务的消费者进程，适合脱离 HTTP 请求执行可重试工作。

## 当前职责

| 任务类型 | 输入 | 输出/副作用 |
|---|---|---|
| `EXCHANGE_IMPORT` | ArtifactStore 中的 STEP/BREP 对象 | 检查清单，并行导入根组件，创建带私有基准特征的 Part；多根再创建 Product |
| `EXCHANGE_EXPORT` | Part 或 Product 当前 Head 的 B-Rep 引用 | 生成 STEP/BREP，并把结果对象 ID 写回任务 |
| `THUMBNAIL_RENDER` | 文档与版本 | 使用 `png-v4` 生成 `640×400` 正交等轴测 PNG 并更新 `document_previews` |

Worker 不提供网络 API，不接受用户认证请求，也不是通用分布式工作流引擎。
导入的 Part/Product 文档名统一使用清理路径后的完整上传文件名，包含 `.step`/`.brep` 后缀；客户端不再另行传入可分叉的文档名。Product STEP 导出按 occurrence 生成独立 transferable root，以保持当前展平 Product 再导入时的类型和 placement。

## 执行模型

单个进程默认同时运行 2 个独立 Job 领取循环，`OCCCCAD_JOB_CONCURRENCY` 可设为 1–8。每个循环持有独立 lease owner 和 Workspace 缓存；共享 PostgreSQL 池、ArtifactStore 与 Geometry Router，多个导入文件可由不同 Geometry Worker 进程处理。每个文件仍是独立可重试任务，批次中的一个失败不会撤销其他文件。

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
- 用户取消排队任务时立即进入 `CANCELED`；运行任务每秒观察取消请求、取消正在进行的 Geometry 调用并由当前 lease owner 确认终态；
- Worker 按持久阶段单调写入 0–100 进度；导入进入正式文档提交阶段后关闭取消能力，避免形成半提交的组件集合；
- 最终失败或取消任务可在同一 Job identity 上手动重试，继续递增 attempt，不覆盖尝试历史；
- 领取语义是至少一次，任务处理器必须依赖幂等键和条件写入，不能假设“恰好一次”；
- 最终成功、最终失败或取消与 `JOB` Outbox 在同一数据库 statement 中写入；API 通过 `job.state.changed.v1` 通知任务发起用户，重试等待状态不制造失败通知；
- 轮询为空时等待 1 秒。
- 缩略图生成默认有 `5s` deadline，可通过 `OCCCCAD_THUMBNAIL_RENDER_TIMEOUT` 调整；超时或场景预算耗尽会持久化固定尺寸默认 PNG。API 在预览尚未生成或制品不可用时也返回默认 PNG；父 Job 取消直接结束渲染，不遗留后台渲染 goroutine。
- `png-v4` 与视口共用 `(1,-1,1)` / Z-up 的正交 ISO 约定，按实际投影几何统一 Fit，应用 occurrence 完整 quaternion/translation。
- CPU 扫描线光栅化使用逐像素深度、面内平滑法线、固定中性材质和 2× 超采样；优先绘制 B-Rep 边并做深度测试，缺少边时提取轮廓/折线。实体缩略图不绘制草图编辑覆盖层；无实体时保留 Visualization 曲线/点/面。
- 不依赖浏览器、GPU 或新外部服务。每次渲染预算为 100 万展开三角形、300 万顶点/曲线点、1 万 occurrence；同场景相同 GeometryKey 复用法线准备结果。该边界只影响预览，不删除/简化权威模型。
- RendererVersion 是缓存身份的一部分；迁移 `0025` 将旧图标记 STALE 并为当前 Head 排队重建，不修改文档 Revision。旧版本渲染任务会被跳过。
- 就绪图像通过 `ETag` 和 `private, no-cache` 复用响应字节，每次重验证仍校验权限与当前 Head，包含 FOLLOW_HEAD 引用变化。
- 渲染器、数值/遮挡测试与基准位于 `internal/thumbnail`；设置 `OCCCCAD_THUMBNAIL_GALLERY=/tmp/occccad-thumbnail-v4` 后运行 `go test ./internal/thumbnail -run TestThumbnailGallery` 可导出视觉样例。

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

交换导入提交保留原始输入对象、摘要、格式和组件索引，由 Workspace 冻结 ImportDefinition/拓扑身份并完成根命名求值，再提交普通 Part Revision。重试复用已采用的精确几何快照与身份分配，避免再次读取已移走的暂存制品。当前可编辑命名范围为有效单 Solid；大文件传输与分块显示仍以独立计划为准。
