# occccad Demo 03：分布式 CAD 开发框架

> 状态：已实现核心垂直切片并通过真实 PostgreSQL、gRPC 和 OCCT 验证  
> 最后验证：2026-08-09

Demo 03 不再增加孤立的演示按钮，而是建立后续 CAD 功能都要遵守的框架：Main Workspace
单线历史、不可变 Version、顺序 Feature 重生成、显式 Sketch Tool、STEP 中性交换、增量迁移和
跨 HTTP/gRPC 的可观测性。

## 1. Onshape 对标边界

设计依据是 Onshape 当前公开行为：Workspace 自动记录每次完成的编辑，Version 是只读且不可变
的历史里程碑；Part Studio 由 Feature Toolbar、Feature List 和 Graphics Area 组成；Sketch
Toolbar 中的矩形是显式工具；Extrude 是 Feature 并按顺序保存在 Feature List 中；STEP 属于
Part/Part Studio 支持的中性交换格式。

Demo 03 刻意只开放一个 `Main` Workspace 和一条时间线，暂不开放 Branch/Merge。这样先把用户
要求的单线变更语义做稳定，同时数据模型保留以后从不可变 Version 创建 Workspace 分支的空间。

## 2. Main Workspace、History 与 Version

```text
Main Workspace
  Change 0  CREATE_DOCUMENT
  Change 1  CREATE_RECTANGLE_SKETCH
  Change 2  PAD_SKETCH
  Change 3  CREATE_VERSION "V1"     <- immutable marker
  Change 4  CREATE_RECTANGLE_SKETCH
  Change 5  PAD_SKETCH
  Change 6  RESTORE V1              <- new latest change, not destructive rewind
```

- `document_changes` 是追加式审计线，每个已完成编辑、Undo、Redo、命名 Version 和 Restore 都追加；
- `document_versions` 保存不可变模型快照和 Geometry Reference；
- `documents.head_version_id` 是 Main 当前状态；
- Undo/Redo 可以移动当前编辑游标，但不会删除 Command、Version 或追加式 Change 审计；
- Undo 后产生新编辑只截断可重做游标，不删除审计历史；
- “创建版本”给当前不可变快照增加名称和说明，不复制几何；
- Restore 复制目标快照形成新的 Version，并在时间线末尾追加 `RESTORE`，不会覆盖中间历史；
- 当前没有自动保存按钮：一次成功 Command 的事务提交就是一次 Workspace 保存。

Product 的 `FOLLOW_HEAD` / `PINNED` 更新边界继续沿用 Demo 02。命名 Version 与 PINNED 是不同
概念：前者是人可读里程碑，后者是某条 Product 引用边的解析策略。

## 3. Part Feature Tree 与顺序重生成

Part 左侧结构固定分为：

```text
Part document
├─ Origin
│  ├─ XY Plane
│  ├─ XZ Plane
│  └─ YZ Plane
├─ Import model.step       optional base feature
├─ Sketch 1
├─ Extrude 1
├─ Sketch 2
├─ Extrude 2
└─ Parts
   └─ Part 1
```

Feature ID 永远稳定，显示名按类型自动编号。`PAD_SKETCH` 不再限制每个 Part 只有一个拉伸。
Go 按数组顺序解释 Feature：Sketch 必须先于引用它的 Extrude；完整链被规范化并计算
GeometryKey；C++ Worker 在一次粗粒度 `EvaluatePart` RPC 中创建所有棱柱并依次 Boolean Fuse。
相交拉伸形成合并实体，不相交拉伸保留多个 Solid；每次重生成仍只产生一份最终 B-Rep/GLB。

当前 Sketch 仍依附三个基准面。选择实体 Face 作为新 Sketch Support 需要 Persistent Topology
Naming，留给后续 Demo，不能用临时三角形或 Face 数组下标冒充稳定引用。

## 4. Sketch 与矩形命令

进入 Sketch 后默认是 `SELECT`，不会再把任意左键拖动解释成矩形。工具栏“矩形”按钮或快捷键
`R` 进入 `RECTANGLE` 状态，Esc 回到选择状态；矩形工具保持激活，可以连续创建多个 Sketch
Feature。矩形绘制使用 Pointer Capture，完成一次绘制才提交 `CREATE_RECTANGLE_SKETCH`。

当前一个 Sketch Feature 仍只有一个边角矩形，未引入约束求解。后续应把一个 Sketch 内的多个
Entity 与 Constraint 放进同一 Sketch Edit Session，而不是继续把每条线做成 Part Feature。

## 5. CAD 相机

```text
右键拖动              旋转
中键 / Ctrl+右键拖动  平移
滚轮                  以光标为中心缩放
F / 双击中键          Fit
TOP/FRONT/RIGHT/ISO    标准视图
```

为了消除追手延迟，OrbitControls 不再启用惯性 Damping；Pixel Ratio 上限由 2 降为 1.5，关闭当前
场景中没有收益的 Shadow Map，并请求高性能 WebGL Context。相机不再固定使用 `near=0.1`、
`far=5000`，而是依据模型 Bounding Sphere、相机到 Orbit Target 的距离动态计算裁剪范围。
这能同时避免小零件近距离裁剪和大装配远端消失，并尽量控制 depth ratio 以减少 Z-fighting。

## 6. STEP 导入与导出

```text
Browser multipart upload
  -> Go validates Part / extension / 64 MiB limit
  -> gRPC ImportStep(bytes)
  -> OCCT STEPControl_Reader
  -> B-Rep + Mesh + GLB artifact
  -> IMPORT_STEP Feature + immutable Version

Part head B-Rep
  -> gRPC ExportStep
  -> OCCT STEPControl_Writer
  -> application/step download
```

导入目前要求空 Part，生成一个 `IMPORT_STEP` 基础 Feature；之后允许继续创建 Sketch 和 Extrude。
导出的是当前精确几何，不包含 occccad 参数化 Feature 历史，这与中性 STEP 的职责一致。当前仅
接受包含 Solid 且体积大于零的 STEP；Assembly/XCAF 名称、颜色和层级属于下一阶段。

## 7. 数据库迁移

迁移唯一入口是 `services/internal/database/migrations/NNNN_description.sql`：

1. 新功能只添加下一个递增 SQL 文件，不修改已经发布的迁移；
2. Server 启动和 `go run ./cmd/occccad-migrate` 使用同一个嵌入式 Runner；
3. Runner 获取 PostgreSQL Advisory Lock，多个实例只能有一个执行迁移；
4. 每个文件在独立事务中执行；失败整体回滚且不登记版本；
5. `schema_migrations` 保存 SHA-256 checksum 和执行耗时；已应用文件被修改时启动失败；
6. Demo 03 的 `0003_demo03_platform.sql` 增加 Main、命名 Version、Change Log 和 Trace 字段。

因此部署新版本不需要人工拼接 SQL；只需让一个 Server/Migrate Job 连接数据库。生产环境仍建议
用单独 Migration Job 先执行，再滚动启动 API 实例。

## 8. 日志与分布式追踪

- Go 使用 `slog.JSONHandler` 输出结构化 JSON；`OCCCCAD_LOG_LEVEL=debug` 可打开源码位置；
- 每个 HTTP 请求返回 `X-Request-ID` 和 `Trace-ID`，并记录 method、route、status、bytes、duration；
- OpenTelemetry HTTP instrumentation 接收/生成 W3C `traceparent`；
- gRPC Client instrumentation 把上下文传播到 Geometry Worker；
- Worker JSON 日志记录 operation、request_id、traceparent、status 和 duration；
- Command 保存 `trace_id` / `span_id`，可以从业务审计跳转到链路；
- 配置 `OTEL_EXPORTER_OTLP_ENDPOINT` 后使用 OTLP/HTTP Batch Exporter；留空时保留本地关联但不导出；
- `deploy/observability/otel-collector.yaml` 提供最小 Collector 接收与 Debug Export 配置。

当前 C++ Worker 已完成 Trace Context 关联日志，尚未链接 OpenTelemetry C++ SDK 产生 Worker 内部
Span；调度器、多 Worker 路由加入时再把 Worker Server Span、Queue Span 与 Geometry Cache
Metrics 一并接入，避免只为了形式增加不完整依赖。

## 9. API 增量

```text
GET  /api/documents/{id}/history
POST /api/documents/{id}/versions
POST /api/documents/{id}/commands       RESTORE / feature commands
POST /api/documents/{id}/import-step    multipart/form-data
GET  /api/documents/{id}/export-step
```

Worker 协议新增：

```text
EvaluatePart(repeated RectangularPadSpec, optional base_brep_data)
ImportStep(step_data)
ExportStep(brep_data)
```

## 10. 已验证结果与下一边界

真实端到端验收覆盖：两个 Sketch/Extrude 得到两个 Solid；创建 V1；第三次拉伸；Restore V1；
导出 STEP；导入新 Part；在导入体上继续 Sketch/Extrude；HTTP Trace 传播至 Worker；Command Trace
落库。C++、Go 和 TypeScript 构建测试均通过。

Demo 03 已把 Control Plane、不可变模型、几何 Worker、Artifact Cache、迁移和 Trace 框架连通，
但“分布式”当前仍是一台 API 加一台独立 Worker 的可扩展形态。下一阶段应优先增加 Worker
Registry/Lease、队列与重试、对象存储、Face Support/Persistent Naming、Extrude Remove 和真正的
Sketch Constraint Solver，而不是继续堆叠无领域模型的 UI 命令。
