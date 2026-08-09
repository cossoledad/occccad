# occccad Demo 02：最小 CAD 工作台

> 状态：已实现并通过端到端验证  
> 最后验证：2026-08-09

Demo 02 把 Demo 01 的固定模型展示升级为可编辑的 Part/Product 工作台。目标不是模拟 UI，
而是确保浏览器操作最终形成持久化 Command、不可变 Document Version 和真实 OCCT Geometry。

## 功能闭环

| 功能 | 实现 |
|---|---|
| 三个默认基准面 | Part 响应提供稳定的 `datum-xy/xz/yz`，视图区显示半透明 XY/XZ/YZ Plane |
| 进入/退出草图 | 选择基准面后进入正视草图模式；退出只切换编辑上下文，不修改模型 |
| 矩形草图 | 在基准面上按住鼠标拖动；保存局部二维原点、宽、高和 Plane，不引入约束求解器 |
| 草图选择与高亮 | 结构树和视图区共享 Selection，草图线框关闭深度测试以保持可见 |
| 拉伸 | 选择草图后输入长度，Go 生成 GeometryKey，通过 gRPC 请求 C++ Worker |
| 精确实体 | OCCT 按基准面局部坐标构造 Wire → Face → Prism，并生成 B-Rep、GLB 和 Mesh |
| Part Undo/Redo | 草图和拉伸分别生成版本；Undo 拉伸恢复只有草图的版本，Redo 恢复实体 |
| 多文档标签页 | 文档库与标签页可同时打开、切换 Part 和 Product |
| Product 插入 | 可引用 Part 或 Product 的特定版本，后端递归展开且拒绝引用环 |
| 实例拖动 | Three.js TransformControls 移动顶层 Part/Product 实例，释放手柄时提交 Command |
| Product Undo/Redo | 插入和移动都生成新版本，Undo/Redo 恢复完整 Product Snapshot |

## 状态与几何边界

```text
Browser Workbench
  └─ POST Document Command
       └─ Go Workspace Service
            ├─ immutable model_json snapshot
            ├─ document_history cursor
            ├─ Product reference cycle check
            └─ Part evaluation when a Pad is active
                 └─ gRPC EvaluatePart
                      └─ C++ OCCT Worker
                           ├─ B-Rep GeometryId (SHA-256)
                           ├─ tessellation / GLB
                           └─ PostgreSQL artifact cache
```

基准面不是普通 Feature，不占用版本，也不会被删除或重新编号。草图只保存二维几何和所依附
的基准面；拉伸引用稳定的 Sketch ID。Product 保存 Document/Version Reference 和 Instance
Transform，不保存 `TopoDS_Compound` 或复制后的 Part Geometry。

## 命令与历史语义

当前支持的可撤销命令：

```text
CREATE_RECTANGLE_SKETCH
PAD_SKETCH
INSERT_INSTANCE
MOVE_INSTANCE
```

每条命令创建新的 `document_versions` 记录，并在 `document_history` 当前游标之后建立时间线。
Undo/Redo 自身也写入 Command 审计，但不复制模型；它们只把 `documents.head_version_id` 和
历史游标移动到已有版本。Undo 后执行新编辑会截断当前 redo 时间线，旧 Version/Command
仍作为审计记录保留。

创建文档属于文档生命周期操作，不纳入文档内部 Undo；删除/恢复文档将在后续生命周期
切片中单独设计。

## HTTP API

```text
GET  /api/documents
POST /api/documents
GET  /api/documents/{documentId}
POST /api/documents/{documentId}/commands
```

所有修改请求都包含 `requestId`。前端使用 `crypto.randomUUID()`；数据库对 request ID 建立
唯一约束，避免同一命令重复落库。

## 当前边界

- 草图只有一个矩形图元类型，没有尺寸、重合、水平/垂直等约束；
- 每个 Part 当前只允许一个 Pad，尚未实现多 Feature Boolean 历史；
- 拉伸为单方向正长度，不含对称、反向、到面等终止条件；
- Product 手柄当前只提交平移，旋转、缩放和装配配合尚未实现；
- Product Reference 固定到插入时的 Document Version，尚未提供“跟随最新版本”策略；
- B-Rep/GLB 暂存在 PostgreSQL，后续迁移到 S3-compatible Artifact Store。

这些边界不会改变 Document/Command/Geometry 的分层，可在后续切片中增量扩展。
