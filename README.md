# occccad

> 基于开源技术构建的分布式云端 CAD 平台。

occccad 的目标不是把桌面 CAD 远程化，而是把参数化建模、产品装配、版本协作、精确几何计算和工程数据管理设计为可水平扩展的云端系统。项目以 CATIA 覆盖的机械设计能力为长期参照，但坚持开放格式、可替换计算后端和开源部署。

## 项目状态

仓库当前是可运行的早期垂直切片，而不是完整 CAD 产品。已经贯通浏览器工作台、Go API、PostgreSQL、持久任务、C++/OCCT Geometry Worker 和本地制品存储；当前支持矩形草图到拉伸、Part/Product 文档、版本历史、Document Center 中的 STEP/BREP 文档交换、账号与 ACL 等基础能力。

尚未实现通用二维约束求解、三维装配配合、稳定拓扑命名、曲面/钣金/工程图/CAM/CAE、跨主机调度和对象存储。这些能力的边界与演进顺序见[目标架构](docs/TARGET_ARCHITECTURE.md)。

## 文档

- [现有架构](docs/CURRENT_ARCHITECTURE.md)：只描述当前仓库中可以从代码、配置和数据库迁移验证的事实。
- [目标架构](docs/TARGET_ARCHITECTURE.md)：面向开源分布式 CAD 的长期设计、Worker 划分、调用关系、技术选型和演进路线。
- [AI Agent 指南](AGENTS.md)：面向仓库级 AI 开发的项目上下文、平台不变量、自主判断、验证与文档责任。

除根 README 与各可运行单元的 README 外，`docs/` 只维护上述两份核心文档。历史 Demo 和版本说明已经合并，不再作为有效架构依据。

## 可运行单元

| 单元 | 类型 | 说明 |
|---|---|---|
| [occccad-server](services/cmd/occccad-server/README.md) | Go HTTP 服务 | 账号、文档、版本、ACL、任务与几何编排 |
| [occccad-jobs](services/cmd/occccad-jobs/README.md) | Go 后台 Worker | STEP/BREP 文档交换和缩略图持久任务 |
| [occccad-control](services/cmd/occccad-control/README.md) | Go 本地控制进程 | 启动、代理、调试切流和本机 Geometry Worker 池 |
| [occccad-migrate](services/cmd/occccad-migrate/README.md) | Go 一次性任务 | 执行带校验的 PostgreSQL 迁移 |
| [Geometry Worker](workers/geometry/README.md) | C++ gRPC Worker | OCCT 精确几何、拓扑、网格与 STEP/BREP 交换 |
| [CAD Web](web/apps/cad/README.md) | React Web 应用 | 文档中心、CAD 工作台、交互和 Three.js 视口 |

`kernel/api` 与 `kernel/occt` 是 Geometry Worker 内部链接的 C++ 库，并非网络服务；`services/internal/*` 是 Go 进程共享的内部包，也不是微服务。

## 快速开始

推荐在 Linux 或 WSL2 中开发。需要 CMake 3.30+、C++17 编译器、Ninja、Conan 2、Python 3、Go、Node.js/pnpm 和 PostgreSQL。仓库当前固定 OCCT 7.9.1；精确版本以 `conanfile.py`、`services/go.mod` 和 `web/package.json` 为准。

```bash
sudo apt install cmake clang ninja-build build-essential
# WSLg
sudo apt install libgl1-mesa-dev libgl-dev libx11-xcb-dev libfontenc-dev libice-dev libsm-dev libxaw7-dev libxcomposite-dev libxcursor-dev libxdamage-dev libxext-dev libxfixes-dev libxi-dev libxinerama-dev libxkbfile-dev libxmu-dev libxmuu-dev libxpm-dev libxrandr-dev libxrender-dev libxres-dev libxss-dev libxt-dev libxtst-dev libxv-dev libxxf86vm-dev libxcb-glx0-dev libxcb-render0-dev libxcb-render-util0-dev libxcb-xkb-dev libxcb-icccm4-dev libxcb-image0-dev libxcb-keysyms1-dev libxcb-randr0-dev libxcb-shape0-dev libxcb-sync-dev libxcb-xfixes0-dev libxcb-xinerama0-dev libxcb-dri3-dev uuid-dev libxcb-cursor-dev libxcb-dri2-0-dev libxcb-dri3-dev libxcb-present-dev libxcb-composite0-dev libxcb-ewmh-dev libxcb-res0-dev libxcb-util-dev pkg-config

curl https://mise.run | sh
mise use --global python@3.14 node@lts go@1.26

python -m pip install -r requirements-build.txt
invoke configure --build-type=Debug
invoke build --build-type=Debug
invoke test --build-type=Debug
```

复制 `.env.example` 为 `.env`，配置 PostgreSQL，并设置首次启动所需的 `OCCCCAD_ADMIN_PASSWORD`：

```bash
invoke run.app --build-type=Debug
```

当前未发布开发阶段允许直接丢弃本地业务数据。需要从完全干净的服务端状态启动时，先停止已有 occccad 进程，再执行：

```bash
invoke run.app --reset-data --build-type=Debug
```

该选项只删除配置数据库中的 `occccad` schema 和 `OCCCCAD_DATA_DIR` 指向的本地 ArtifactStore，随后从当前迁移重建 schema；进程内 Router/Worker 状态随新进程自然清空。若只清理和迁移而不启动，使用 `invoke data.reset --yes`。命令不可用于已经承诺保留数据的发布环境。

另开终端启动连接真实 API 的前端：

```bash
invoke run.web --mode=api
```

只开发界面时可以使用浏览器内 Mock Adapter，不需要数据库或后端：

```bash
invoke run.web
```

常用命令及每个进程的配置、端口和故障语义请查看对应 README。

## 仓库结构

```text
occccad/
├── kernel/                  不暴露 OCCT 类型的内核 API 与 OCCT 适配
├── proto/                   Go/C++ 共用的 gRPC 契约
├── workers/geometry/        C++ Geometry Worker
├── services/cmd/            Go 可执行进程
├── services/internal/       Go 领域、存储和控制实现
├── services/internal/database/migrations/
├── web/apps/cad/            React CAD Web 应用
├── deploy/                  部署辅助配置
└── docs/                    现有架构与目标架构
```

## 文档维护规则

1. 已实现能力只写入现有架构；计划能力只写入目标架构。
2. 服务接口、环境变量或运行方式变化时，同一变更必须更新所属服务 README。
3. 架构图统一使用 Mermaid，避免提交难以审阅的二进制图源。
4. 目标技术栈不是永久承诺；升级必须经过许可证、兼容性、确定性和基准测试评审。
5. 不再新增按版本号或 Demo 编号命名的架构文档；重大决策应在目标架构中更新，并在提交历史中追踪。

## License

项目采用 [MIT License](LICENSE)。引入第三方库时仍需分别遵守其许可证；目标架构列出的候选库不代表已经成为项目依赖。
