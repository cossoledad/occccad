# occccad Demo 02：最小 CAD 工作台

> 状态：已实现并通过端到端验证  
> 最后验证：2026-08-09

> Demo 02 是最小工作台基线；多 Feature、命名 Version、STEP、迁移校验和可观测性的当前行为
> 以 [Demo 03](occccad_Demo03_Distributed_CAD_Framework.md) 为准。

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
| Product 插入 | 默认递归跟随 Part/Product 的 Head，也可把单个实例固定到特定 Version；后端拒绝引用环 |
| 实例拖动 | Three.js TransformControls 移动顶层 Part/Product 实例，释放手柄时提交 Command |
| Product Undo/Redo | 插入、移动和引用策略变更都生成新版本，Undo/Redo 恢复完整 Product Snapshot |
| CAD 视图导航 | 右键旋转，中键或 Ctrl+右键平移，滚轮以光标为中心缩放，F/双击中键适配视图 |

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
的基准面；拉伸引用稳定的 Sketch ID。Product 保存 Document Reference、插入基线 Version、
引用策略和 Instance Transform，不保存 `TopoDS_Compound` 或复制后的 Part Geometry。

## Product 引用与更新边界

`Document` 是全局唯一的可编辑对象，`Version` 是该文档的不可变快照，`Head` 是文档当前
指向的 Version。Product 中的每个实例引用一份 Document，不复制 Part/Product 模型；每一条
父子引用边独立选择以下策略：

- `FOLLOW_HEAD`（默认）：读取 Product 时解析被引用文档当前 Head。Part 新拉伸、Part Undo/Redo、
  子 Product 插入或移动实例，都会在下一次打开、切换到或刷新父 Product 时自动出现；嵌套
  Product 按每一层引用策略递归解析。
- `PINNED`：固定到切换该策略时被引用文档的当前 Version。之后子文档 Head 如何变化，都不会
  改变该实例；适合发布、评审、导出和需要可复现结果的场景。

插入时仍记录 `versionId` 作为插入基线；`resolvedVersionId` 表示本次读取实际解析的版本。
旧模型没有 `referenceMode` 时按 `FOLLOW_HEAD` 处理，因此已有 Product 会自动获得修正后的更新
行为。结构树以 `LIVE` / `PINNED` 标识策略，选择实例后可用“固定版本/跟随最新”命令切换。

| 发生的操作 | FOLLOW_HEAD 实例 | PINNED 实例 | 哪个文档产生新版本 |
|---|---|---|---|
| 修改或 Undo/Redo Part | 下一次解析显示 Part 新 Head | 保持固定几何 | 仅 Part |
| 修改或 Undo/Redo 子 Product | 下一次解析显示其新结构与位置 | 保持固定子结构 | 仅子 Product |
| 移动/插入父 Product 实例 | 使用更新后的父结构 | 使用更新后的父结构 | 父 Product |
| 切换引用策略 | 按新策略解析 | 按新策略解析 | 父 Product |
| Undo/Redo 父 Product | 恢复父结构、位置和引用策略 | 同左 | 仅父 Product Head 移动 |

这里刻意不在子文档变化时给所有父 Product 生成版本。父 Product 的不可变快照保存的是引用
关系、策略和变换；FOLLOW_HEAD 的递归解析结果属于派生状态。否则一次常用 Part 修改会向整个
引用图传播并制造大量无用户命令的父版本。需要冻结完整装配时，应沿所需引用边切换为
`PINNED`；父 Product 的 Undo 也不会跨文档撤销子 Part 的建模命令。

## 视图区导航

当前鼠标映射对齐 Onshape 默认 CAD 导航习惯：

| 操作 | 输入 |
|---|---|
| 选择、草图绘制、操作三轴手柄 | 鼠标左键 |
| 旋转视图 | 按住鼠标右键拖动 |
| 平移视图 | 按住鼠标中键拖动，或 Ctrl+右键拖动 |
| 以光标为中心缩放 | 鼠标滚轮 |
| 适配全部内容 | `F`、双击中键或工具栏“适配” |
| 标准视图 | 点击右上角 TOP / FRONT / RIGHT / 等轴测按钮 |

进入草图后锁定视图旋转，但继续允许平移和缩放，避免草图平面意外倾斜；退出草图后恢复三维
旋转。视图区拦截右键菜单，启用惯性阻尼与光标中心缩放，矩形绘制使用 Pointer Capture，
因此拖出画布边界再释放也能正确结束操作。

## 命令与历史语义

当前支持的可撤销命令：

```text
CREATE_RECTANGLE_SKETCH
PAD_SKETCH
INSERT_INSTANCE
MOVE_INSTANCE
SET_REFERENCE_MODE
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
- FOLLOW_HEAD 是读取时解析，不使用 WebSocket 主动推送；已打开的父 Product 需要切换标签页或刷新后更新；
- 固定整个深层装配目前需要逐条引用边设置 PINNED，尚未提供“一键发布基线”命令；
- B-Rep/GLB 暂存在 PostgreSQL，后续迁移到 S3-compatible Artifact Store。

这些边界不会改变 Document/Command/Geometry 的分层，可在后续切片中增量扩展。
