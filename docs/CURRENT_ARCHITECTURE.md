# occccad 当前架构

> 2026-09-21 文档核对基线。代码、迁移和测试决定实现事实；本文是分层入口。目标契约见[目标架构](TARGET_ARCHITECTURE.md)，未完成工作见[统一路线](../plans/README.md)。Product 验收已完成；人工确认和本轮标准测试见[完成记录](architecture/current/product-assembly.md#accept-product-完成记录)。

## 系统结论

当前是模块化 Go 控制面、PostgreSQL 持久任务、C++ Geometry Worker 与 React/Three.js Web 组成的可运行 CAD 垂直切片。参数模型与不可变 Revision 是业务真相；精确 B-Rep、显示制品、Variant 和求解证据带 provenance，可从权威输入重建。Geometry Router 只管理本机 Worker，ArtifactStore 支持 LOCAL/S3，Worker 使用共享本机暂存目录。

## 事实分册

| 主题 | 内容 |
|---|---|
| [运行与边界](architecture/current/runtime.md) | 进程、启动拓扑、认证、ACL、Control/Monitor |
| [模型与历史](architecture/current/model-history.md) | Workspace、Transaction/ChangeSet、Undo/Redo、参数、依赖、实时提交同步 |
| [Part 与 Sketch](architecture/current/part-sketch.md) | PlaneGCS、Profile、BodyOperation、面支撑、ExternalGeometry、Worker RPC |
| [几何结果与显示制品](architecture/current/geometry-representations.md) | 数据库摘要/角色索引、BREP/GLB/Naming、轻量 DocumentView、临时预览 |
| [持久命名](architecture/current/persistent-naming.md) | Extrude/Boolean Face/Edge/Vertex history、resolver、状态与 Reconnect |
| [Product 与 Assembly](architecture/current/product-assembly.md) | Publication、typed path、ContextBinding/Variant、多 Workspace 事务、M3、Release |
| [Jobs 与制品](architecture/current/jobs-artifacts.md) | Router、租约、取消、交换、缩略图、LOCAL/S3 ArtifactStore |
| [CAD realtime 控制面](architecture/current/realtime.md) | 命令、Preview 生命周期、幂等恢复、快照与文件通道边界 |
| [Web 与交互](architecture/current/web.md) | 根场景/编辑上下文、选择、preview、偏好、Toolbar、工作台 |
| [验证与可观测性](architecture/current/validation.md) | 日志/trace、性能、scoped checks、Mock 浏览器与真实验收边界 |

## 能力与限制

| 能力 | 当前事实 | 未交付/未验收边界 |
|---|---|---|
| 模型与历史 | Part/Product、Workspace、追加 Revision、CAS、补偿式 Undo/Redo | semantic rebase、通用协作合并 |
| Sketch/Part | 通用草图、基础约束、参数表达式、Profile、线性拉伸/旋转和单 Body 布尔 | 完整专业 Sketcher、多 Body、Hole/Fillet 等完整 Feature 链 |
| 持久命名 | Extrude/Boolean 完整 Face/Edge/Vertex history 与 resolver；当前 policy 为 v3 | Revolve/Import 等未达到同等覆盖；歧义需要 Reconnect |
| Part 关联 | 驱动尺寸 ParameterBinding、面支撑、Edge/Vertex 投影、更新/重连/Detach | 部分圆弧 snapshot、Face group、Section/Silhouette |
| 产品关联 | Publication、typed InstancePath、Product Design Session、ContextInput/Binding、Variant、原子事务、Update Plan | Configuration/Design Table、partial update、flexible subassembly、Derive Part from Context |
| 装配 | SE(3)、基础约束、平行/垂直、点线/线面偏移、激活与模式分离、M3/历史/Release 证据、0–6 阶组合测试 | 六类几何完整矩阵与多成员固联尚未完成；M4 最近可行拖拽、M5 冲突解释、M6 Engineering Connection；当前 MOVE 仍用临时 Fix |
| 产品发布 | 冻结 ProductRelease、gate、replay、按冻结几何导出 | ACCEPT-PRODUCT 已通过；后续新增能力仍需独立验收 |
| 交换 | STEP/XDE 共享 Definition、嵌套 Product、名称与局部 placement round-trip；BREP Solid/Compound | AP242 颜色、材质、层、PMI；大模型容量验收 |
| 平台 | 本机扩缩容、持久 Jobs、LOCAL/S3 制品、realtime Command/Preview 与状态同步 | CDN、跨主机 Scheduler、多 API 扇出、presence/preview 协作 |
| 工程扩展 | 尚无完整领域实现 | Surface/3D Wire、DMU、Kinematics、钣金、工程图、CAM/CAE |

## 旧计划归并结果

已完成的特征编辑/naming/装配状态、Part 内关联、Publication、Product 上下文/M3/Release 实现进入上表分册，旧 P0–P10 计划删除。ACCEPT-PRODUCT 已完成，证据归入[Product 分册](architecture/current/product-assembly.md#accept-product-完成记录)，对应待办已删除。旧 UX 计划中的圆环树图标、24 px grip 等已被当前侧栏/分隔条设计替代，不作为待实现功能恢复。

## 持续风险

持久命名覆盖有限；复杂 Part 同步求值尚未全部任务化；本地存储和开发 Control 不能直接承担跨主机生产部署。Mock/单元测试不能证明真实 WebGL、权限、数据库与 Worker 组合路径。无发布数据兼容承诺期间按唯一 schema 演进，受保护开发重置的准确范围以 CLI 和迁移为准。
