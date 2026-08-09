# occccad

> 基于 Open CASCADE Technology（OCCT）的开源云分布式 CAD 软件。

`occccad` 是项目的唯一名称。名称中的 `occ` 表示几何内核 OCCT，`c` 取自
`could`，`cad` 表示项目所服务的 CAD 领域。

项目希望把精确几何计算、文档与版本管理、分布式计算和浏览器交互组织成一套
云原生 CAD 基础设施，而不只是给桌面 CAD 内核增加一层 Web 接口。

## 当前状态

仓库目前处于 **v0.0.8 / 服务控制与调试路由阶段**。已经能够完成：

- 通过 Conan 2 获取 OCCT 7.9.1、gRPC/Protobuf 和 GoogleTest；
- 显示并选择 XY/XZ/YZ 三个基准面，在任意基准面进入草图模式；
- 在视图区拖动绘制矩形草图，从结构树或视图区选择并高亮草图；
- 对选择的草图设置长度并执行真实的 Rectangle Sketch → Face → Pad；
- 生成稳定的 SHA-256 GeometryId、B-Rep、GLB、网格、包围盒和拓扑摘要；
- 通过粗粒度 gRPC 在 Go API 与 C++ Geometry Worker 之间求值；
- 在 PostgreSQL 中保存 Document、Version、Command、Product Instance 和几何制品；
- 使用多标签 CAD 工作台创建 Part/Product，插入 Part 或嵌套 Product；
- Product 实例默认递归跟随被引用文档 Head，也可按实例固定到不可变 Version；
- 使用三轴手柄移动实例，并对草图、拉伸、插入、移动和引用策略执行持久化 Undo/Redo；
- Main Workspace 使用追加式 Change History、不可变命名 Version 和非破坏性 Restore；
- Part Feature Tree 支持 Sketch 1 → Extrude 1 → Sketch 2 → Extrude 2 顺序重生成；
- 矩形是显式 Sketch Toolbar 命令，支持 `R` 激活和 Esc 退出；
- Part 支持 STEP 导入/导出，并可在导入实体后继续草图与拉伸；
- 使用无惯性右键旋转、中键或 Ctrl+右键平移、光标中心缩放和动态相机裁剪；
- SQL 文件自动迁移具备 Advisory Lock、事务、Checksum；HTTP/gRPC 使用 JSON 日志和 Trace Context。
- 首页文档中心支持创建、搜索、修改、软删除和恢复 Part/Product；
- 双击文档进入路由式 CAD 工作台，多文档 Tab、分组 Toolbar、Feature Tree 和 Inspector 独立组织。
- 文档中心支持层级 Folder、Breadcrumb、最近打开、移动、Main Workspace 复制和服务端分页。
- User/Team、Owner/Editor/Viewer ACL、Folder 权限继承、文档/文件夹共享与“与我共享”视图；
- 所有 API 请求绑定 Principal，成功写操作记录 Actor、Resource、Request ID 与 Trace ID 审计。
- 使用真实账号登录和数据库会话；注册账号由管理员审批并分配 `ADMIN/MEMBER` 平台角色；
- 管理后台支持账号查询、添加、审批、禁用和密码重置，旧 Demo 身份切换已移除；
- STEP 导入/导出通过 PostgreSQL 持久任务和独立 `occccad-jobs` Worker 异步执行；
- 大文件制品按 SHA-256 原子写入服务启动目录的 `data/`，并通过存储接口预留未来 S3 后端。
- 新几何双写 B-Rep/GLB 本地制品，Part 缩略图由后台任务生成并显示在文档中心。
- 使用单个 `occccad-control` 启动 API、Jobs、Geometry Router 和最小 Worker 集合；
- Geometry Worker 按 Resident Geometry 容量自动扩缩容，并支持 API/Jobs/C++ Worker 调试切流。

当前矩形草图尚不包含尺寸/几何约束求解，装配也暂不包含旋转、配合约束、S3、Redis、完整任务
中心、跨主机调度或 XCAF Assembly STEP；这些属于后续目标。当前进度详见
[文档索引](docs/README.md)。

## 已验证的开发环境

以下组合已于 2026-08-09 在本仓库完成编译和测试：

| 组件 | 已验证版本 | 仓库中的实际约束 |
|---|---:|---|
| OS | Ubuntu 26.04 LTS（WSL2） | Linux 为第一开发平台 |
| C++ | C++17 | `CMAKE_CXX_STANDARD=17` |
| GCC | 15.2.0 | 默认 Conan Profile 当前配置为 GCC 15 |
| CMake | 4.2.3 | `cmake_minimum_required(VERSION 3.30)` |
| Ninja | 1.13.2 | CMake/Conan 默认生成器 |
| Conan | 2.31.2 | `conan>=2,<3` |
| Python | 3.14.7 | 用于 Conan 和 Invoke，不进入运行时 |
| OCCT | 7.9.1 | `opencascade/7.9.1` |
| GoogleTest | 1.17.0 | `gtest/[>=1.14 <3]`，当前解析结果 1.17.0 |
| Go | 1.26.5 | API、迁移、任务 Worker 和 gRPC Client |
| Node.js / pnpm | 26.7.0 / 11.20.0 | pnpm 版本由 `web/package.json` 固定 |
| PostgreSQL Client | 18 | 数据迁移和人工检查 |

这张表记录的是已跑通组合，不表示所有工具都必须逐字匹配该补丁版本。开始开发前，
请同时查看[开发环境与 C++ 工具链](docs/occccad_Development_Environment_and_CPP_Toolchain_v0.1.md)。

## 快速开始

Linux 或 WSL2 中安装 GCC 15、CMake 3.30+、Ninja、Python 3 和 `venv` 后：

```bash
# 1. 创建隔离的 Python 构建环境
python3 -m venv .venv
source .venv/bin/activate
python -m pip install -r requirements-build.txt

# 2. 查看实际工具版本
invoke info

# 3. 获取依赖并生成 Debug 构建目录
invoke configure --build-type=Debug

# 4. 编译并测试
invoke build --build-type=Debug
invoke test --build-type=Debug

# 5. 运行 Rectangle Sketch -> Pad 冒烟程序
invoke run.geometry --build-type=Debug
```

首次执行 `invoke configure` 时，Conan 可能需要从 ConanCenter 下载或本地编译依赖。
当前 Profile 明确调用 `gcc-15` / `g++-15`；若本机命令名称不同，应新增或调整本地
Profile，而不是假设名为 `linux-gcc14-*` 的历史文件使用 GCC 14。

Release 构建：

```bash
invoke configure --build-type=Release
invoke build --build-type=Release
invoke test --build-type=Release
```

完整应用还需要一个 PostgreSQL 数据库。复制 `.env.example` 为 `.env` 并设置连接信息后，
Invoke 会自动加载该文件。`OCCCCAD_ADMIN_PASSWORD` 在首次启动时必须设置。按照
[v0.0.8 设计与运行说明](docs/occccad_v0.0.8_Service_Control_and_Debug_Routing.md) 使用一个入口启动：

```bash
invoke run.app --build-type=Debug
```

`.env` 中的密码用于管理员账号首次初始化，开发阶段登录后不强制修改密码。账号已初始化后，
修改 `.env` 不会覆盖数据库中的密码。

然后从 Windows 访问 `http://localhost:8080`。Server 默认绑定
`0.0.0.0:8080`；若 WSL 没有转发 localhost，可改用 `hostname -I` 显示的 WSL 地址。

## 常用命令

```text
invoke bootstrap                         安装 Python 构建依赖
invoke info                              显示工具链、Profile 和构建目录
invoke configure --build-type=Debug      Conan install + CMake configure
invoke build --build-type=Debug          编译全部 C++ 目标
invoke build --target=<target>           编译指定目标
invoke test --filter=<regex>             运行匹配的 CTest
invoke run.geometry                      运行 Geometry Worker 冒烟程序
invoke run.app                           启动完整应用、路由器和自动扩缩 Worker
invoke run.worker                        启动 Geometry Worker gRPC Server
invoke run.jobs                          启动 PostgreSQL 持久任务 Worker
invoke run.server                        启动 Go API 和静态 Web Server
invoke web.build                         类型检查并构建 Web 应用
invoke clean                             删除仓库内 build 产物
```

`tasks.py` 是当前推荐入口。`CMakePresets.json` 描述 CMake 构建目录，但干净环境仍需先由
Conan 生成 toolchain；不要跳过 `invoke configure`。

## 当前目录结构

```text
occccad/
├── CMakeLists.txt                 C++ 根构建
├── CMakePresets.json              CMake Debug/Release/CI presets
├── conanfile.py                   当前 C++ 依赖图
├── requirements-build.txt         Python 构建工具
├── tasks.py                       统一开发命令
├── build-support/conan/profiles/  Conan 编译器 profiles
├── cmake/                         warnings / sanitizers 配置
├── kernel/
│   ├── api/                       不暴露 OCCT 类型的公共 C++ API
│   └── occt/                      OCCT 适配实现
├── proto/                         C++ / Go 共用的 Worker 协议
├── workers/geometry/              C++ Geometry Worker gRPC Server
├── tests/geometry/                C++ 几何与制品测试
├── services/                      Go API、数据迁移和领域服务
├── web/apps/cad/                  TypeScript + Three.js 文档中心与 CAD 工作台
└── docs/                          架构、环境和 Demo 文档
```

## 架构方向

```text
Browser
   |
Gateway / Control Plane
   |
Geometry Router / Scheduler
   |
C++ Geometry Workers (OCCT)
   |
PostgreSQL / ArtifactStore
```

当前使用 PostgreSQL 保存业务/任务状态，B-Rep 和 GLB 仍保留于 `bytea`，STEP 制品使用本地
`data/` ArtifactStore。Redis 和 S3 暂不引入；未来按已预留接口替换或优化。

## 文档

- [文档索引与实现状态](docs/README.md)
- [架构规格](docs/occccad_Architecture_Specification_v0.1.md)
- [开发环境与 C++ 工具链](docs/occccad_Development_Environment_and_CPP_Toolchain_v0.1.md)
- [首个垂直切片 Demo](docs/occccad_demo_v0.1.md)
- [Demo 01 运行手册](docs/occccad_Demo01_Runbook.md)
- [Demo 02 最小 CAD 工作台设计与实现](docs/occccad_Demo02_Minimal_CAD_Workbench.md)
- [Demo 03 分布式 CAD 开发框架](docs/occccad_Demo03_Distributed_CAD_Framework.md)
- [v0.0.4 文档中心与专业 CAD 工作台](docs/occccad_v0.0.4_Document_Center_and_Workbench.md)
- [v0.0.5 文档组织与设计复用](docs/occccad_v0.0.5_Document_Organization.md)
- [v0.0.6 协作与访问控制基础](docs/occccad_v0.0.6_Collaboration_and_Access_Control.md)
- [v0.0.7 账号管理、本地制品与持久任务](docs/occccad_v0.0.7_Distributed_Artifact_Pipeline.md)
- [v0.0.8 服务控制、自动扩缩容与调试路由](docs/occccad_v0.0.8_Service_Control_and_Debug_Routing.md)

## License

本项目采用 [MIT License](LICENSE)，允许开源或商业使用、复制、修改、再发布和销售；
再分发时需保留原版权声明和许可文本。
