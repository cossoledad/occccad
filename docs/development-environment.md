# 开发环境准备

构建工具与应用启动见[根 README](../README.md)，各进程的接口与运行配置见所属 README。本页集中说明开发资源的使用约定和验证环境。

## 现有开发资源与配置

维护者已授权使用仓库根目录 `.env` 中实际配置的开发环境，包括任务所需的数据库连接、开发验证，以及后续配置的 S3 等资源。Agent 可直接读取并使用这些配置，无需重复请求连接或凭据使用许可。优先复用现有资源；只有实际故障或测试隔离要求需要时才准备替代环境。

- 已有 `.env` 时保留原配置；首次配置才从 `.env.example` 复制。文档与脚本引用变量名，不另存一份实际密码。
- `invoke` 在加载根目录 `tasks.py` 时读取根 `.env`，已导出的环境变量优先。加载器接受简单的 `KEY=VALUE`、可选 `export` 前缀及成对引号，不执行 shell 表达式。排障时先检查是否有旧的导出变量覆盖配置。
- 服务端优先使用 `OCCCCAD_DATABASE_URL`；未设置时使用 `OCCCCAD_POSTGRES_HOST`、`PORT`、`USER`、`PASSWORD`、`DB`（均带 `OCCCCAD_POSTGRES_` 前缀）。正常业务使用数据库中的 `occccad` schema。
- 直接执行 `psql`、`go test` 等命令时，不要假定它们会自动读取项目 `.env`；显式加载所需配置或使用已有项目入口。诊断输出只需连接是否成功和错误原因，不必回显完整连接串。
- 当前 ArtifactStore 只实现 LOCAL，以 `OCCCCAD_DATA_DIR` 配置；API 与 Jobs 必须访问同一物理目录。后续 S3 的资源使用授权已明确，但配置变量、适配器和验证方式应随实际实现补充，不能把授权视为 S3 已交付。

## PostgreSQL

使用已有 `.env` 指向的数据库，不因本机缺少服务进程就另起一个临时数据库。需要本地 PostgreSQL 时，在 Ubuntu 上安装：

```bash
sudo apt install postgresql
```

安装软件包不会自动创建项目所需的角色、密码和数据库；这些必须与 `.env` 一致。连接排障先检查配置覆盖、目标地址与端口、服务可达性及认证；只读 `SELECT 1` 可以确认连接，不需要重置数据。

部分集成测试使用显式的 `OCCCCAD_TEST_DATABASE_URL`，不会自动使用应用的数据库配置。运行前读对应测试对迁移、空库和清理的要求：可兼容现有数据的测试可复用已授权开发数据库；要求可丢弃数据库的测试使用隔离测试库，并按测试入口准备迁移。不要为使测试执行而把所有测试无差别指向开发库。

数据重置仍遵循[根 AGENTS 的开发数据边界](../AGENTS.md#当前开发数据边界)：先停止占用进程，仅通过 `invoke data.reset --yes` 或 `invoke run.app --reset-data` 删除命令报告的 `occccad` schema 与本地 ArtifactStore，并在交付中说明。资源使用授权不包括任意清空其他数据库、schema 或 S3 bucket。

## Chromium 与浏览器验证

Ubuntu 使用以下软件包提供曾缺失的 Chromium 动态库；采用其他发行版时按实际包名对应处理：

```bash
sudo apt install libnspr4 libnss3 libasound2t64
```

这些是本环境曾缺少的依赖，不是 Chromium 在所有系统上的完整依赖清单。安装到系统后，正常使用系统库，不再默认设置 `/tmp/occccad-browser/libs/usr/lib/x86_64-linux-gnu` 的 `LD_LIBRARY_PATH`。若仍无法启动，对实际浏览器可执行文件执行 `ldd`，根据 `not found` 项补齐依赖；不要反复下载相同的临时库。

先按 [CAD Web README](../web/apps/cad/README.md) 安装前端依赖。尚未安装项目 Playwright 对应的 Chromium 时执行：

```bash
pnpm --dir web/apps/cad exec playwright install chromium
```

例如验证法线视图，并确认不再依赖临时动态库：

```bash
env -u LD_LIBRARY_PATH pnpm --dir web/apps/cad exec playwright test browser/normal-view.spec.ts
```

当前 Playwright 配置自动启动端口 5174 的 Mock 前端，使用 Chromium 与软件 WebGL，测试后关闭进程。此入口验证真实浏览器中的前端交互；数据库、真实 API 和 Geometry Worker 的完整链路需另行连接真实后端验证。真实环境启动使用 `invoke run.app --build-type=Debug` 与 `invoke run.web --mode=api`。
