# occccad

开放的云原生参数化 CAD，以 CATIA 机械设计工作流为长期参照，采用可替换计算后端和开放格式。当前是早期可运行垂直切片：React/Three.js 工作台、Go 控制面、PostgreSQL/SQLite、C++ Geometry Worker、LOCAL/S3 制品存储已贯通。

项目支持 Part/Product、Sketch/Profile、Multi-Body 参数化实体特征、持久命名、装配约束、机构试动与录制、版本历史、STEP/BREP 文档交换、账号与 ACL。能力范围与限制由[当前架构](docs/architecture/current/README.md)拥有；长期约定见[架构约定](docs/architecture/target/README.md)，下一步只见 [CAD 核心主线](plans/cad-core-roadmap.md)。定向实现不等于工业规模或完整浏览器验收。

## 从这里进入

- [项目地图](docs/README.md)：系统调用链、核心领域、源码归属、事实与意图入口。
- [Agent 路由](AGENTS.md)：全局规则与各领域指南；局部任务从这里搜符号及测试。
- [开发指南](docs/development/README.md)：环境、资源、Agent 工作流与文档维护。
- [操作指南](docs/guides/README.md)、[测试路由](tests/README.md)、[验证记录](docs/validation/README.md)：操作、可执行检查和测量证据分开。

## 快速开始

Linux x86_64 / WSL2 使用 mise 2026.8.3+ 与系统 GCC/G++ 15。系统依赖、资源选择与调试见[环境准备](docs/development/environment.md)。已有 .env 保留；首次从 .env.example 复制并配置数据库/存储和管理员。

```bash
mise trust
mise install --locked
mise run setup --component all --profile release --jobs 4
mise run dev --profile release --jobs 4
```

默认启动全栈；仅后端加 --no-web，单独前端用 `mise run web:dev`。`mise run doctor` 检查准备与资源。构建默认 release，debug/sanitize 显式选择；setup 不覆盖配置。无需安装 Invoke 或项目 Python 编排环境。

## 模块与运行单元

| 目录/单元 | 职责与运行说明 |
|---|---|
| `kernel/api/`、`kernel/occt/`、`kernel/assembly/` | 无第三方类型的领域 API、精确几何适配、[装配算法库](kernel/assembly/README.md) |
| `proto/`、[Geometry Worker](workers/geometry/README.md) | Go/C++ 契约、粗粒度计算；[Sketch solver](workers/geometry/sketch/README.md) 同进程承载 |
| [server](services/cmd/occccad-server/README.md) | HTTP/Realtime、账号/ACL、文档/命令、求值编排 |
| [jobs](services/cmd/occccad-jobs/README.md) | 持久交换、缩略图、机构结果任务 |
| [control](services/cmd/occccad-control/README.md)、[monitor](services/cmd/occccad-monitor/README.md) | 本机进程、代理与 Worker 池 / 只读 TUI |
| [migrate](services/cmd/occccad-migrate/README.md)、[artifacts](services/cmd/occccad-artifacts/README.md) | 数据库迁移 / 制品存储管理 |
| [3dreplay](services/cmd/occccad-3dreplay/README.md) | 不可变输入经生产 Worker/Router 的离线装配重放 |
| `services/internal/` | 同一部署族的领域、存储与控制包；[数据库](services/internal/database/README.md)、[装配合同](services/internal/assemblycontract/README.md) |
| [CAD Web](web/apps/cad/README.md) | 独立 React 应用，交互/近似预览/显示 |
| `tests/`、`deploy/`、`docs/`、`plans/` | 跨模块验证 / 部署配置 / 项目知识 / 未完成工作 |

逻辑模块不自动对应微服务。Worker 不拥有业务数据库真相，浏览器不执行权威 B-Rep 求值。

人工运行和真实集成共用根 `.env` 的数据库、制品、暂存与日志。所有开发数据可丢弃；不另设测试连接。一次性资源检查见 [occccad-dev](services/cmd/occccad-dev/README.md)。

## 验证与维护

```bash
mise run --quiet plan --changed --level unit --json
mise run check --scope assembly --level unit --match DirectedAngle --jobs 4
mise run check --scope workspace --level integration --match '^TestHistoryCapabilitiesAcrossTwoUndoAndRedoSteps$'
mise run docs:check
```

任务发现用 `mise tasks ls`，参数用 `mise run <task> --help`；[Agent 工作流](docs/development/agent-workflow.md)说明选择、证据层和结构化报告。优先最小 target/scope，不默认全库测试；浏览器只在显式选择相应 level 时运行，不自动下载。

`mise run clean --component cpp --profile debug` 只清可重建输出；`mise run data:reset --yes` 独立清理已授权的配置开发资源，先停止相关进程。报告、诊断、日志和全局缓存不属于 clean。

## License

[MIT License](LICENSE)。第三方库遵循各自许可证；目标设计中的候选不代表已引入依赖。
