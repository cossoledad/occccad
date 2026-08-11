# occccad 文档索引

> 最后更新：2026-08-11

`occccad` 是项目的唯一名称：`occ` 表示 OCCT 几何内核，`c` 取自 `could`，`cad`
表示 CAD 领域。文档中不再使用 CloudCAD、cloudcad 等临时名称。

## 阅读顺序

1. [仓库 README](../README.md)：当前状态、已验证环境和最快上手路径；
2. [开发环境与 C++ 工具链](occccad_Development_Environment_and_CPP_Toolchain_v0.1.md)：
   当前真实构建基线、依赖和排障；
3. [架构规格](occccad_Architecture_Specification_v0.1.md)：长期系统边界与关键原则；
4. [首个垂直切片 Demo](occccad_demo_v0.1.md)：已交付的首个端到端闭环；
5. [Demo 01 运行手册](occccad_Demo01_Runbook.md)：配置、启动和验收命令；
6. [Demo 02 最小 CAD 工作台](occccad_Demo02_Minimal_CAD_Workbench.md)：交互建模与命令历史；
7. [Demo 03 分布式 CAD 开发框架](occccad_Demo03_Distributed_CAD_Framework.md)：Main 历史线、
   Feature 重生成、STEP、迁移和可观测性。
8. [v0.0.4 文档中心与专业 CAD 工作台](occccad_v0.0.4_Document_Center_and_Workbench.md)：
   文档 CRUD、Trash、页面路由、Toolbar、Tab、Feature Tree 和清理边界。
9. [v0.0.5 文档组织与设计复用](occccad_v0.0.5_Document_Organization.md)：
   Folder、Breadcrumb、Recent、Move、Copy 和服务端分页。
10. [v0.0.6 协作与访问控制基础](occccad_v0.0.6_Collaboration_and_Access_Control.md)：
    User/Team、ACL、Folder 继承、Share、Request Principal 和访问审计。
11. [v0.0.7 账号管理、本地制品与持久任务](occccad_v0.0.7_Distributed_Artifact_Pipeline.md)：
    注册审批、管理后台、数据库会话、本地 ArtifactStore、Job Queue 和异步 STEP。
12. [v0.0.8 服务控制与调试路由](occccad_v0.0.8_Service_Control_and_Debug_Routing.md)：
    单命令启动、Geometry 自动扩缩容、进程管理和 VS Code 调试切流。
13. [目标架构与当前实现对照](occccad_Target_Architecture_and_Current_Implementation.md)：
    原始目标图、当前服务拓扑、Worker 依赖差异和实现进度。
14. [v0.0.9 前端应用架构](occccad_v0.0.9_Frontend_Application_Architecture.md)：
    React/Ant Design 技术决策、CAD Viewport 边界、前后端分离和无后端调试。
15. [v0.0.10 CAD 前端交互基础框架](occccad_v0.0.10_CAD_Interaction_Framework.md)：
    CadInput、Tool/Selection/Navigation 路由、Command、Shortcut Context、Floating Toolbar 和测试边界。

## 文档状态

| 文档 | 性质 | 如何理解 |
|---|---|---|
| `README.md` | 当前事实 | 新开发者首先执行的命令 |
| 开发环境与工具链 | 当前事实 + 升级规则 | 与仓库构建文件保持一致 |
| 架构规格 | 目标架构 | 描述方向，不表示所有模块已实现 |
| Demo v0.1 | 已实现基线 + 后续设计 | Demo 01 核心验收项已实现，扩展项仍按正文边界理解 |
| Demo 01 运行手册 | 历史归档 | 首个固定 Seed 垂直切片；Seed API 已在 v0.0.4 退役 |
| Demo 02 最小 CAD 工作台 | 当前事实 + 设计边界 | Part/Product 编辑、Undo/Redo 与验收结果 |
| Demo 03 分布式 CAD 开发框架 | 当前事实 + 下一阶段边界 | Onshape 对标交互与平台框架的当前实现 |
| v0.0.4 文档中心与工作台 | 已实现基线 | 首页、文档生命周期和工作台组件架构 |
| v0.0.5 文档组织与复用 | 当前事实 + 迭代边界 | 当前 Folder、Recent、Copy 和分页架构 |
| v0.0.6 协作与访问控制 | 当前事实 + 安全边界 | 当前 User/Team、ACL、Share 和审计架构 |
| v0.0.7 账号、制品与任务 | 已实现基线 | 管理后台、可信会话、本地 Artifact、Job Worker、异步 STEP 和 Thumbnail |
| v0.0.8 服务控制与调试 | 已实现基线 | Control Plane、Geometry Router、自动扩缩容和调试 Override |
| 目标架构与当前实现对照 | 当前事实 + 差距分析 | 目标与当前服务/Worker 关系的 Mermaid 对照 |
| v0.0.9 前端应用架构 | 已实现基线 | React 应用、统一 UI、Three.js Viewport 封装、独立启动和 Mock Adapter |
| v0.0.10 CAD 前端交互框架 | 已实现基线 | 统一输入、可替换导航、Tool/Command/Shortcut、悬浮 Overlay 和输入生命周期处理 |

## 当前实现快照

```text
已实现并验证
  C++17 kernel API
        |
  OCCT 7.9.1 adapter
        |
  coarse-grained gRPC Geometry Worker
        |
  XY/XZ/YZ Datum -> interactive Rectangle Sketch -> Pad
        |
  Go API -> PostgreSQL documents, versions, commands, artifacts
        |
  versioned Go commands -> persistent Undo / Redo
        |
  multi-tab React CAD workbench -> live/pinned Product references
        |
  Main change line -> immutable named Version -> non-destructive Restore
        |
  ordered Feature regeneration -> STEP import/export
        |
  OpenTelemetry HTTP/gRPC trace -> structured logs / Command correlation
        |
  Document Center CRUD / Trash -> routed multi-tab CAD workbench
        |
  Folder hierarchy / Recent / Copy -> paginated document organization
        |
  Request Principal -> User/Team ACL -> inherited Share / access audit
        |
  password session / admin console -> PostgreSQL jobs -> local ArtifactStore
        |
  React / Ant Design application -> isolated Three.js CAD Viewport -> REST or Mock adapter
        |
  CadInput -> Tool / Selection / Navigation router -> UI Command / Floating Overlay

尚未实现
  sketch constraint solving and dimensions
  assembly rotation and mating constraints
  Redis / S3 integration (reserved, not currently required)
  historical artifact backfill jobs
```

架构文档中的“系统应当”“目标”“V0.1 设计”均属于目标态；只有被源代码、构建配置和
自动化测试共同证明的能力，才属于当前实现。

## 文档维护规则

- 项目名统一写作 `occccad`，包括示例二进制、模块和目录前缀；
- 当前依赖版本以 `conanfile.py`、`go.mod`、`package.json` 等机器可读文件为准；
- 当前 C++ 标准以根 `CMakeLists.txt` 和 target compile features 为准；
- 已验证环境应写明验证日期，不把它等同于最低支持范围；
- 尚未接入的库、服务或功能明确标注为“规划”；
- 架构发生变化时同步更新规格与 Demo；实现状态变化时优先更新 README 和本索引；
- 引用、缓存和版本更新必须分别说明“源文档边界、派生解析边界、触发时机和是否产生版本”；
- 默认引用策略变更时必须说明旧数据兼容语义，避免历史 Product 静默采用不确定行为。
