# 开发环境准备

构建工具与应用启动见[根 README](../README.md)，各进程的接口与运行配置见所属 README。本页集中说明开发资源的使用约定和验证环境。

## 现有开发资源与配置

维护者已授权使用仓库根目录 `.env` 中实际配置的开发环境，包括任务所需的数据库连接、开发验证，以及后续配置的 S3 等资源。Agent 可直接读取并使用这些配置，无需重复请求连接或凭据使用许可。优先复用现有资源；只有实际故障或测试隔离要求需要时才准备替代环境。

- 已有 `.env` 时保留原配置；首次配置才从 `.env.example` 复制。文档与脚本引用变量名，不另存一份实际密码。
- `invoke` 在加载根目录 `tasks.py` 时读取根 `.env`，已导出的环境变量优先。加载器接受简单的 `KEY=VALUE`、可选 `export` 前缀及成对引号，不执行 shell 表达式。排障时先检查是否有旧的导出变量覆盖配置。
- 根 `.env.example` 按数据库、账号、制品存储、独立进程、控制进程、原生构建、日志和测试资源分组；实际 `.env` 使用同一分组并保留本机连接与凭据。Web 的 `.env.development` / `.env.mock` 由 Vite 加载，只包含前端变量，与根配置分开维护。
- 原生构建默认 `Release`：Invoke 的 `--build-type` 优先于 `OCCCCAD_BUILD_TYPE`，未配置或为空时回退到 `Release`；控制进程直接启动也默认寻找 `build/cmake/release` 中的 Worker。直接 CMake 单配置构建未指定类型时也使用 `Release`；Preset 使用 `dev-release`，原生调试显式使用 `dev-debug` 或 `--build-type=Debug`。装配合同 runner 未指定 `--build-type` 时使用 `Release`。
- `CMakeUserPresets.json` 是 Conan 自动生成并忽略提交的文件，不维护对旧 Debug 制品的固定引用；`invoke configure` 会重新生成它。`OCCCCAD_ENABLE_ASAN` / `OCCCCAD_ENABLE_UBSAN` 在 Invoke 配置时传给 CMake，默认均为 `OFF`。
- `invoke configure` 使用 Conan `cmake_layout` 的 `build/cmake/<type>/build/<Type>/generators/conan_toolchain.cmake`；文件缺失时直接失败。CMake 配置使用 `--fresh` 重建配置缓存，避免先前未加载 toolchain 的失败配置继续影响依赖查找；不会清理 Conan 依赖缓存，也不会执行项目编译。
- 默认 Conan profile 为仓库内的 `linux-gcc15-release` / `linux-gcc15-debug`；`--profile` 可显式传入 Conan profile 名称或路径。`invoke bootstrap` 仅安装 `requirements-build.txt` 中的 Python 工具，不生成或覆盖 profile。`invoke build --jobs=N` 限制并行数，默认使用全部核心；启动入口复用同一 C++ 构建命令。`invoke clean` 删除 `build/`、根编译数据库链接及 Conan 生成的 User Presets，不清理业务数据或 Conan 依赖缓存。
- 服务端优先使用 `OCCCCAD_DATABASE_URL`；未设置时使用 `OCCCCAD_POSTGRES_HOST`、`PORT`、`USER`、`PASSWORD`、`DB`（均带 `OCCCCAD_POSTGRES_` 前缀）。PostgreSQL 正常业务使用数据库中的 `occccad` schema。也可设置 `OCCCCAD_DATABASE_URL=sqlite:/absolute/path/local.db` 使用 SQLite Local Mode；API 与 Jobs 必须指向同一个绝对本机路径，数据库并发设为 1 或保持未配置。SQLite 运行与限制见[数据库层](../services/internal/database/README.md)。
- 直接执行 `psql`、`go test` 等命令时，不要假定它们会自动读取项目 `.env`；显式加载所需配置或使用已有项目入口。诊断输出只需连接是否成功和错误原因，不必回显完整连接串。
- ArtifactStore 支持 LOCAL/S3，使用根 `.env` 的 `OCCCCAD_ARTIFACT_BACKEND`、`OCCCCAD_S3_ENDPOINT/BUCKET/REGION/ACCESS_KEY/SECRET_KEY/SECURE`。HTTP MinIO 设置 `SECURE=false`，endpoint 不含协议前缀。`OCCCCAD_DATA_DIR` 在 S3 模式保留为计算暂存目录，API/Jobs/本机 Worker 共享；配置和迁移见[存储运维](../services/cmd/occccad-artifacts/README.md)。原始密码只保留在未跟踪的 `.env`。

## PostgreSQL

使用已有 `.env` 指向的数据库，不因本机缺少服务进程就另起一个临时数据库。需要本地 PostgreSQL 时，在 Ubuntu 上安装：

```bash
sudo apt install postgresql
```

安装软件包不会自动创建项目所需的角色、密码和数据库；这些必须与 `.env` 一致。连接排障先检查配置覆盖、目标地址与端口、服务可达性及认证；只读 `SELECT 1` 可以确认连接，不需要重置数据。

部分集成测试使用显式的 `OCCCCAD_TEST_DATABASE_URL`，不会自动使用应用的数据库配置。运行前读对应测试对迁移、空库和清理的要求：可兼容现有数据的测试可复用已授权开发数据库；要求可丢弃数据库的测试使用隔离测试库，并按测试入口准备迁移。不要为使测试执行而把所有测试无差别指向开发库。

装配候选、历史、诊断下载及精确 BREP/Router 验证共用 `services/internal/testsupport.OpenTestDatabase`，按 `OCCCCAD_TEST_DATABASE_URL` 显式选择 SQLite 或 PostgreSQL。默认使用独立的 `build/test-resources/occccad_test.db`；SQLite 目标必须是绝对路径的 `*_test.db`（相对配置先按仓库根目录解析），并拒绝应用数据库及其符号链接别名。PostgreSQL 仍要求专用 `occccad_*_test` 库，并检查 `current_database()` 身份。入口仅迁移、创建各测试自己的文档与临时制品，不重置或修补旧迁移记录。SQLite 通过不代表 PostgreSQL 专用行为或双后端一致性验收。

数据重置仍遵循[根 AGENTS 的开发数据边界](../AGENTS.md#当前开发数据边界)：先停止占用进程，仅通过 `invoke data.reset --yes` 或 `invoke run.app --reset-data` 删除命令报告的 PostgreSQL `occccad` schema 或当前配置的专用 SQLite 数据库全部表（含迁移记录），以及本地 ArtifactStore/暂存目录，以及 S3 模式下当前配置的专用桶全部对象（含版本和未完成分片，保留桶），并在交付中说明。资源使用授权不包括清空其他数据库、schema 或 S3 bucket。S3、数据库与本地制品不构成跨存储事务：清理失败直接报错，可能已部分删除，停止写入后可重新执行。

## 测试资源与默认目录

Invoke、几何 scenario 的测试 main，以及 API/Control/Geometry/Workspace/Jobs/数据库的 Go TestMain 都读取根 `.env`；已导出变量优先，`OCCCCAD_ENV_FILE` 显式文件缺失直接失败。测试路径按仓库根目录解析，不受 `go test` 包目录或 CTest 工作目录影响。Python/Go 会把相对 SQLite 测试 URL 转成绝对路径；应用数据库 URL 的既有规则不变。

| 变量 | 默认值（相对于仓库根目录） | 用途 |
| --- | --- | --- |
| `OCCCCAD_TEST_RESOURCES_DIR` | `build/test-resources` | 生成资源的默认根目录 |
| `OCCCCAD_ASSEMBLY_FIXTURE_DIR` | 根目录下 `analytic-fixtures` | 七种球/圆柱/圆锥的 BREP 与 STEP |
| `OCCCCAD_TEST_IMPORT_BREP` | 根目录下 `import-repair.brep` | 闭合盒体中反向一张面的修复样本；缺失时自动生成 |
| `OCCCCAD_TEST_GEOMETRY_WORKER` | `build/cmake/<type>/workers/geometry/occccad_geometry_worker` | 使用已有构建，不在测试中编译 |
| `OCCCCAD_TEST_DATABASE_URL` | `sqlite:build/test-resources/occccad_test.db` | 与应用数据隔离的测试库 |
| `OCCCCAD_TEST_CURVED_GLB` / `OCCCCAD_TEST_GLB_FIXTURE` | 根目录下 `curved.glb` / `geometry.glb` | 对应测试的诊断导出 |
| `OCCCCAD_TEST_ASSEMBLY_REPLAY_OUTPUT` | 根目录下 `assembly-replay.3dreplay` | 对应诊断测试的重放导出 |

各具体路径可独立覆盖；修改根目录后，显式配置的子路径仍按其配置解析。大文件输入 `OCCCCAD_TEST_EXCHANGE_STEP` 不自动替换为小型合成样本，需要按测试要求提供真实 corpus。

几何测试 main 在测试运行前生成缺失的修复样本，已有文件保持不变；仅列出测试时不生成。`AssemblyExactSupport.ExportAnalyticRouterFixtures` 使用默认输出目录，不再仅验证而不落盘。Go 装配 fixture 若发现任何一项 BREP/STEP 缺失，会调用当前构建中的该生成测试，成功后再执行真实导入。缺少 Worker 或生成器仍属于环境缺失，不会改用 Mock。生成修复文件后，`ImportedSolidRepairCorpus` 的导入、输入不可变、修复和幂等断言照常执行。

`invoke clean` 删除 `build/`，也会删除这些测试制品和默认 SQLite 测试库；下一次测试按需生成资源并从当前 SQLite 迁移建立测试库。应用数据库、S3 与业务制品不在该测试准备流程的清理范围内。PostgreSQL 专用 pool 测试在 SQLite 配置下明确跳过；切换到专用 PostgreSQL 测试 URL 才验证该后端。

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

当前 Playwright 配置自动启动端口 5174 的 Mock 前端，使用 Chromium 与软件 WebGL，测试后关闭进程。此入口验证真实浏览器中的前端交互；数据库、真实 API 和 Geometry Worker 的完整链路需另行连接真实后端验证。真实环境启动使用 `invoke run.app --build-type=Release` 与 `invoke run.web --mode=api`。

Invoke 与 Go 使用相同环境优先级：已导出变量 > `OCCCCAD_ENV_FILE` 指定文件（未指定时读取根 `.env`）。显式文件不存在时直接失败，不回退到默认资源。Invoke 将指定文件路径转为绝对路径，确保切换工作目录后仍读取同一文件。
