# occccad 文档索引

> 最后更新：2026-08-09

`occccad` 是项目的唯一名称：`occ` 表示 OCCT 几何内核，`c` 取自 `could`，`cad`
表示 CAD 领域。文档中不再使用 CloudCAD、cloudcad 等临时名称。

## 阅读顺序

1. [仓库 README](../README.md)：当前状态、已验证环境和最快上手路径；
2. [开发环境与 C++ 工具链](occccad_Development_Environment_and_CPP_Toolchain_v0.1.md)：
   当前真实构建基线、依赖和排障；
3. [架构规格](occccad_Architecture_Specification_v0.1.md)：长期系统边界与关键原则；
4. [首个垂直切片 Demo](occccad_demo_v0.1.md)：已交付的首个端到端闭环；
5. [Demo 01 运行手册](occccad_Demo01_Runbook.md)：配置、启动和验收命令；
6. [Demo 02 最小 CAD 工作台](occccad_Demo02_Minimal_CAD_Workbench.md)：交互建模与命令历史。

## 文档状态

| 文档 | 性质 | 如何理解 |
|---|---|---|
| `README.md` | 当前事实 | 新开发者首先执行的命令 |
| 开发环境与工具链 | 当前事实 + 升级规则 | 与仓库构建文件保持一致 |
| 架构规格 | 目标架构 | 描述方向，不表示所有模块已实现 |
| Demo v0.1 | 已实现基线 + 后续设计 | Demo 01 核心验收项已实现，扩展项仍按正文边界理解 |
| Demo 01 运行手册 | 当前事实 | 本地启动、接口与验收结果 |
| Demo 02 最小 CAD 工作台 | 当前事实 + 设计边界 | Part/Product 编辑、Undo/Redo 与验收结果 |

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
  multi-tab TypeScript CAD workbench -> live/pinned Product references
        |
  Onshape-style viewport navigation -> Product hierarchy / transform gizmo

尚未实现
  sketch constraint solving and dimensions
  assembly rotation and mating constraints
  Redis / S3 integration
  distributed scheduling and recovery
```

架构文档中的“系统应当”“目标”“V0.1 设计”均属于目标态；只有被源代码、构建配置和
自动化测试共同证明的能力，才属于当前实现。

## 文档维护规则

- 项目名统一写作 `occccad`，包括示例二进制、模块和目录前缀；
- 当前依赖版本以 `conanfile.py`、`go.mod`、`package.json` 等机器可读文件为准；
- 当前 C++ 标准以根 `CMakeLists.txt` 和 target compile features 为准；
- 已验证环境应写明验证日期，不把它等同于最低支持范围；
- 尚未接入的库、服务或功能明确标注为“规划”；
- 架构发生变化时同步更新规格与 Demo；实现状态变化时优先更新 README 和本索引。
- 引用、缓存和版本更新必须分别说明“源文档边界、派生解析边界、触发时机和是否产生版本”；
- 默认引用策略变更时必须说明旧数据兼容语义，避免历史 Product 静默采用不确定行为。
