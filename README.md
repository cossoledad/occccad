# occccad

开放的云原生参数化 CAD，以 CATIA 机械设计工作流为长期参照，采用可替换计算后端和开放格式。当前是早期可运行垂直切片：React/Three.js 工作台、Go 控制面、PostgreSQL/SQLite、C++ Geometry Worker、LOCAL/S3 制品存储已贯通。

项目支持 Part/Product、Sketch/Profile、Multi-Body 参数化实体特征、持久命名、装配约束、机构试动与录制、版本历史、STEP/BREP 文档交换、账号与 ACL。能力范围与限制由[当前架构](docs/architecture/current/README.md)拥有；长期约定见[架构约定](docs/architecture/target/README.md)，下一步只见 [CAD 核心主线](plans/cad-core-roadmap.md)。定向实现不等于工业规模或完整浏览器验收。

## 从这里进入

- [项目地图](docs/README.md)：系统调用链、核心领域、源码归属、事实与意图入口。
- [Agent 路由](AGENTS.md)：全局规则与各领域指南；局部任务从这里搜符号及测试。
- [开发指南](docs/development/README.md)：环境、资源、Agent 工作流与文档维护。
- [操作指南](docs/guides/README.md)、[测试路由](tests/README.md)、[验证记录](docs/validation/README.md)：操作、可执行检查和测量证据分开。

## 快速开始

Linux/WSL2 构建依赖与数据库配置见[环境准备](docs/development/environment.md)。准确版本看 Conan、Go、Web 的清单与锁文件。已有根 `.env` 时保留；首次才从 `.env.example` 复制并配置数据库与 `OCCCCAD_ADMIN_PASSWORD`。

```bash
python -m pip install -r requirements-build.txt
invoke configure --build-type=Release
invoke build --build-type=Release
invoke run --build-type=Release
```

另开终端启动前端，默认连接同一套真实 API：

```bash
invoke run.web
invoke doctor --live --browser
```

只读监控用 `invoke run.monitor`。原生构建默认 Release，显式 `--build-type` 优先于 `OCCCCAD_BUILD_TYPE`；调试可选 Debug。

开发数据重置用 `invoke data.reset --yes` 或 `invoke run --reset-data`，先停止占用进程。清理配置所指数据库业务域、本地制品/暂存和专用 S3 桶，完整授权与范围见[开发数据边界](AGENTS.md#当前开发数据边界)。

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
invoke check --plan
invoke check --scope <assembly|geometry|sketch|workspace|services|web>
invoke context-audit
```

精确回归用 `--match`；`invoke test` 为全量入口。性能入口为 `invoke performance.assembly` 与 `invoke performance-baseline`，最新本机数据与复现见[性能基线](docs/validation/performance-baseline.md)，计时和证据边界见所属 README。文档归属、命名与链接检查见[文档维护](docs/development/documentation.md)。

## License

[MIT License](LICENSE)。第三方库遵循各自许可证；目标设计中的候选不代表已引入依赖。
