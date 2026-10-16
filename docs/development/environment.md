# 开发环境准备

人工运行、Agent 验证与真实集成测试使用根 `.env` 的同一套数据库、ArtifactStore、计算暂存目录、日志和 Worker 构建。开发数据均可丢弃；资源使用与重置授权见[根指南](../../AGENTS.md#当前开发数据边界)。不要复制一份测试连接、另建测试库或测试桶。

## 构建准备

复用本机已有 Linux/WSL、CMake、Ninja、Conan、Python、Go、Node.js、pnpm 和 Chromium。准确版本以依赖文件、CMake cache 和 `invoke info` 为准。配置使用 Conan 生成的 toolchain，不能用缺少 toolchain 的裸 CMake 配置替代：

```bash
python -m pip install -r requirements-build.txt
invoke configure --build-type=Release
invoke build --build-type=Release
invoke doctor
```

默认 Release；显式 `--build-type` 优先于 `OCCCCAD_BUILD_TYPE`，并同时选择构建目录和测试 Worker。原生调试选择 Debug；不自动把旧 Release Worker 用于 Debug 验证。显式 `OCCCCAD_GEOMETRY_WORKER_BIN` 是应用与验证共用的覆盖项。

`invoke configure` 沿用仓库 Conan profile，使用生成的 `build/cmake/<type>/build/<Type>/generators/conan_toolchain.cmake` 和 CMake `--fresh`；不清理 Conan 缓存。`invoke build --jobs=N` 限制并行数。ASAN/UBSAN 由现有 `OCCCCAD_ENABLE_ASAN` / `OCCCCAD_ENABLE_UBSAN` 控制。`CMakeUserPresets.json` 与编译数据库是生成输出。

## 现有开发资源与配置

- 维护者已授权使用当前 `.env` 的数据库、S3 和开发账号，无需重复确认。已有文件保留连接与凭据；首次配置才从 `.env.example` 复制。
- 优先级为已导出变量 > `OCCCCAD_ENV_FILE` 指定文件 > 默认根 `.env`。显式文件缺失直接失败，不回退。加载器接受简单 `KEY=VALUE`、可选 `export` 和成对引号，不执行 shell 表达式。
- 配置文件的位置不改变仓库根目录。SQLite 相对路径、Worker 覆盖路径和生成测试输入按仓库根目录解析；`./data`、`./logs` 保持相对 `services/` 的含义，并在启动前转成绝对路径。
- Invoke、Go 与 Node 开发/Live 测试分别由 [Python 加载器](../../tools/development_environment.py)、[Go 加载器](../../services/internal/config/environment.go) 和 [Node 加载器](../../tools/development_environment.mjs)读取同一份字面值配置。应用进程、直接 Go 测试和 Invoke 使用同一组运行变量；不再设置测试专用数据库、Worker 地址或制品目录。
- `OCCCCAD_DATABASE_URL` 优先于 `OCCCCAD_POSTGRES_*`。本机选择 SQLite 时，API、Jobs 与真实集成读取同一文件；选择 PostgreSQL 时共同读取同一数据库的 `occccad` schema。不要在验证时自动切换后端。
- `OCCCCAD_ARTIFACT_BACKEND=LOCAL|S3`、`OCCCCAD_DATA_DIR` 和 `OCCCCAD_S3_*` 共同决定制品与暂存。S3 不消除本机计算暂存；API、Jobs、Worker 使用同一目录。文档、报告和诊断不复制密码。

真实集成测试可以为 RPC 断言启动有独立端口的临时 Worker/Router，但使用同一构建、数据库和制品存储，生成的文档可由正常应用读取。单元测试的纯模型、SQLite 方言、事务回滚、故障注入及上传缓冲可以拥有短期测试对象；这些对象不构成另一套开发配置。

## PostgreSQL

使用当前配置的资源，不因本机没有 PostgreSQL 进程就另起临时数据库。需要排障时先检查配置覆盖、目标地址、端口、认证和 `SELECT 1`，不要回显完整连接串。

正常真实集成通过 `internal/testsupport.OpenDevelopmentDatabase` 打开、迁移同一开发数据库，不自动清库。PostgreSQL pool、迁移与 schema parity 测试在当前配置选择 PostgreSQL 时执行；SQLite 下明确记录不适用。SQLite 通过不能宣称 PostgreSQL 已验证。数据库合同见[数据库 README](../../services/internal/database/README.md)。

## 启动与数据重置

```bash
invoke run
# 另一个终端；默认连接真实 API
invoke run.web
invoke doctor --live --browser
```

`invoke run` 等同 `invoke run.app`：构建后启动 Control、API、Jobs 和本机 Router/Worker；前端保持独立进程。`pnpm dev` 也默认连接真实 API，代理目标跟随根 `OCCCCAD_APP_LISTEN`。`dev:mock` / `invoke run.web --mode=mock` 用于前端隔离场景。

每次 `invoke run` 记录源码、制品、资源身份和本次进程标识到 `build/runtime/identity.json`。`doctor --live` 在 Linux/WSL 核对实际进程，发现源码、配置、构建类型或加载的二进制变化时要求重启。监听可达与业务/RPC 行为验收分别报告。

所有人工和 Agent 产生的开发数据都可删除，无需再次确认。先停止占用进程，再统一执行：

```bash
invoke data.reset --yes
# 或重置后启动同一套应用
invoke run --reset-data
```

仅清理命令报告的当前 PostgreSQL `occccad` schema 或专用应用 SQLite 的全部用户表、视图与迁移记录、本地 ArtifactStore/暂存目录，以及配置 S3 桶的对象、历史版本、删除标记和未完成分片；保留桶。不扩大到其他数据库、schema、目录或桶。跨存储删除不具备原子回滚，失败后停止写入、修复并重跑。

## 测试资源与默认目录

测试输入与导出统一位于 `build/test-resources/`；它不是第二套数据库或 ArtifactStore。路径可按具体 corpus 需求覆盖，应用连接仍来自同一 `.env`。

| 变量 | 默认文件/目录 | 用途 |
|---|---|---|
| `OCCCCAD_TEST_RESOURCES_DIR` | `build/test-resources` | 生成输入与诊断导出 |
| `OCCCCAD_ASSEMBLY_FIXTURE_DIR` | 根目录下 `analytic-fixtures` | 解析球、圆柱、圆锥 BREP/STEP |
| `OCCCCAD_TEST_IMPORT_BREP` | 根目录下 `import-repair.brep` | 反向面修复输入 |
| `OCCCCAD_TEST_NAMING_FIXTURE` | 根目录下 `naming-v2.pb` | 真实 C++ Naming → Go round trip |
| `OCCCCAD_TEST_CURVED_GLB` / `OCCCCAD_TEST_GLB_FIXTURE` | 根目录下 `curved.glb` / `geometry.glb` | 显示场景制品 |
| `OCCCCAD_TEST_ASSEMBLY_REPLAY_OUTPUT` | 根目录下 `assembly-replay.3dreplay` | 数值重放导出 |

解析 fixture 缺失或生产者更新时，Go 调用当前构建的 `AssemblyExactSupport.ExportAnalyticRouterFixtures`；Naming fixture 由 `occccad_naming_artifact_tests` 生成，生产者更新后重新生成。几何测试入口生成缺失的修复输入。缺少生产者或 Worker 不改用 Mock。

大 STEP corpus 由 `OCCCCAD_TEST_EXCHANGE_STEP` 显式选择，不自动用小合成样本冒充。S3 流式与 Worker round trip 随配置的 S3 后端执行；LOCAL 下报告不适用，不另建桶。重置合同使用 HTTP/SDK 故障场景验证，真实资源清理统一通过上述重置命令。

`invoke clean` 删除 `build/`、生成的 User Presets 和编译数据库链接，包含生成输入、验证报告及运行制品身份；不等同业务数据重置。数据库、制品或暂存若显式配置在 `build/`，也受该目录删除范围影响。

## Chromium 与浏览器验证

先复用已经安装的 Playwright Chromium 和系统库：

```bash
invoke doctor --scope web --browser
```

缺少浏览器才在 `web/apps/cad` 执行 `pnpm exec playwright install chromium`。缺动态库时对实际浏览器运行 `ldd`，按 `not found` 补齐系统依赖；不要默认下载临时库或重设 `LD_LIBRARY_PATH`。本机 Ubuntu 曾需要 `libnspr4`、`libnss3`、`libasound2t64`，这不是所有系统的完整清单。

Mock Playwright 在 5174 自动启动隔离前端，覆盖前端交互。Live 配置在 5175 连接真实 API，使用同一开发账号、数据库和制品；后端先用 `invoke run` 启动。具体入口见[Web README](../../web/apps/cad/README.md)。Mock、SSR、真实 Router 和 Live 浏览器各自证明对应范围，不能互相代替。
