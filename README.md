# occccad

> 基于 Open CASCADE Technology（OCCT）的开源云分布式 CAD 软件。

`occccad` 是项目的唯一名称。名称中的 `occ` 表示几何内核 OCCT，`c` 取自
`could`，`cad` 表示项目所服务的 CAD 领域。

项目希望把精确几何计算、文档与版本管理、分布式计算和浏览器交互组织成一套
云原生 CAD 基础设施，而不只是给桌面 CAD 内核增加一层 Web 接口。

## 当前状态

仓库目前处于 **v0.2 / Demo 02 最小 CAD 工作台阶段**。已经能够完成：

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
- 使用右键旋转、中键或 Ctrl+右键平移、滚轮缩放以及 F 适配的 CAD 视图导航。

当前矩形草图尚不包含尺寸/几何约束求解，装配也暂不包含旋转、配合约束、Redis、对象
存储或分布式调度；这些属于后续目标。当前进度详见[文档索引](docs/README.md)。

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
| Go | 1.26.5 | Demo API、迁移和 gRPC Client |
| Node.js / pnpm | 26.7.0 / 11.20.0 | pnpm 版本由 `web/package.json` 固定 |
| PostgreSQL Client | 18 | Demo 数据迁移和人工检查 |

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

完整 Demo 还需要一个 PostgreSQL 数据库。复制 `.env.example` 为 `.env` 并设置连接信息后，
Invoke 会自动加载该文件。按照 [Demo 01 运行手册](docs/occccad_Demo01_Runbook.md) 分别启动
Worker 和 Server：

```bash
invoke web.build
invoke run.worker --build-type=Debug
invoke run.server
```

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
invoke run.worker                        启动 Geometry Worker gRPC Server
invoke run.server                        启动 Go API 和静态 Web Server
invoke web.build                         类型检查并构建 Demo Web
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
├── web/apps/demo/                 TypeScript + Three.js Demo 页面
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
PostgreSQL / Redis / S3-compatible storage
```

Demo 02 使用 PostgreSQL；为保持垂直切片紧凑，B-Rep 和 GLB 暂存于 PostgreSQL `bytea`。
Redis、MinIO/S3 与多 Worker 调度尚未进入本 Demo。

## 文档

- [文档索引与实现状态](docs/README.md)
- [架构规格](docs/occccad_Architecture_Specification_v0.1.md)
- [开发环境与 C++ 工具链](docs/occccad_Development_Environment_and_CPP_Toolchain_v0.1.md)
- [首个垂直切片 Demo](docs/occccad_demo_v0.1.md)
- [Demo 01 运行手册](docs/occccad_Demo01_Runbook.md)
- [Demo 02 最小 CAD 工作台设计与实现](docs/occccad_Demo02_Minimal_CAD_Workbench.md)

## License

许可证尚未确定。在正式选择并添加 `LICENSE` 文件前，请不要假定仓库代码已按某一
开源许可证授权。
