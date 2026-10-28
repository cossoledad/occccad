# 开发环境准备

人工、Agent 与真实集成使用所选 `.env` 的同一数据库、ArtifactStore、暂存与日志。[根指南](../../AGENTS.md#当前开发数据边界)定义授权范围。构建配置独立于这些资源；构建不读取业务凭据。

## 构建准备

Linux x86_64 / WSL2 使用 mise 2026.8.3 或更新版本。本仓库验证的工具版本在根 [mise.toml](../../mise.toml)，下载来源由实际生成的 [mise.lock](../../mise.lock) 固定；Web 依赖由 pnpm lock、C++ recipe revision 由 [conan.lock](../../conan.lock) 固定。Conan 通过 mise 的隔离 uv tool 环境运行 Python；项目没有 Python 开发运行器，不需要项目编排 venv。

```bash
mise trust
mise install --locked
mise run setup --component all --profile release --jobs 4
mise run build --component all --profile release --jobs 4
mise run doctor
```

setup 不安装系统包、不 sudo、不改全局 pip、不覆盖 `.env`。首次自行从 `.env.example` 复制并选择资源。系统须有真实 GCC/G++ 15、pkg-config、flock、X11/OpenGL 开发头与库；Debian/Ubuntu 可按本机源准备 `gcc-15 g++-15 pkg-config libgl-dev libx11-dev libxext-dev libxrender-dev libxfixes-dev libxi-dev libxrandr-dev libxcursor-dev libxinerama-dev libfontenc-dev libice-dev libsm-dev libxaw7-dev libxmu-dev libxpm-dev libxt-dev libxtst-dev libxv-dev libxxf86vm-dev`。缺项按 Conan/system 报错补齐。系统架构、真实编译器与 profile 必须一致，不能伪造版本。

仅准备 Go 开发工具：`mise run setup --component tooling --jobs 4`。这一步不需要 C++、Worker、数据库或 Web。后续帮助、plan 使用准备好的 `build/tools/occccad-dev`，不会隐式构建工具或应用；工具源码变化后重新 setup tooling。`mise tasks ls` 发现任务，`mise run <task> --help` 查 Usage 参数。

## 依赖、配置与增量构建

- setup / `cpp:deps`：显式同步依赖，可联网。Conan 使用 Release build context，host context 跟随 profile；安装由共享 Conan cache 锁保护。recipe、lock、profiles、编译器、工具及编译环境和生成输出共同构成准备身份，同时检查缓存 manifest 引用的文件是否仍存在。
- build：不升级依赖、不安装浏览器、不执行 `--fresh`。准备身份缺失/变化或缓存缺项给出 `cpp:deps`；准备完成后使用 CMake/Ninja 原生增量图。普通 CMake 自动 regeneration 由 Ninja 处理；Presets 改动触发一次普通 configure。真实编译器或编译环境变更需要显式 clean 该 profile 再 deps，避免继续使用旧编译器缓存；普通编译错误不需要 clean。
- `CMakePresets.json` 拥有团队配置与 Conan toolchain 路径。产物隔离在 `build/cmake/linux-gcc15-x86_64/{release,debug,sanitize}`；sanitize 是独立 ASan/UBSan Debug 配置。编译数据库在各自目录，不生成全局可变链接或 User Presets。
- `mise run cpp:build --target occccad_geometry_worker --profile debug --jobs 8` 使用真实 target。原生 `cmake --preset dev-release`、`cmake --build <目录> --target <target> --parallel 4` 仍可直接使用；与 mise 同时写同目录时也须遵守 `build/locks/cpp-<profile>.lock`。
- Go 构建由 `go build -p N` 拥有，Web 由 `pnpm build` 拥有；单组件不触发 C++。不因普通编译错误 clean，不默认全库测试。

`--jobs` > 导出的 `OCCCCAD_BUILD_JOBS` > 4，必须正整数。mise 调度默认 1，聚合大内存构建串行；CMake/Conan 各自接收 N，Go 先用 `go test -c -p N` 准备测试二进制，再用 `-p min(jobs,test-jobs)` / `-parallel test-jobs` 执行，避免包测试并发扩大。它们不是系统线程总数。Vite/tsc 没有同义原生 jobs 参数。`--test-jobs` 默认 1，集成/浏览器/full 要求 1；运行时 Worker 池仍由 `.env` 单独控制。改变 jobs 不改变编译语义或产物目录。

Proto 是真相：C++ 在 CMake 中生成，protoc/grpc_cpp_plugin 明确来自 Conan 的 Release build context，依赖包含导入 Proto 与生成器；Go 生成物集中提交到 `services/gen`。`mise run codegen` 使用固定 protoc/plugins；`mise run codegen --check` 对临时输出检查新增、删除和内容漂移，不改写工作区、不构建 OCCT。

## 现有开发资源与配置

Go [加载器](../../services/internal/config/environment.go) 与 Node [加载器](../../tools/development_environment.mjs)读取字面值文件；mise/Shell 不 source 或 eval `.env`。优先级是导出变量 > 显式 `OCCCCAD_ENV_FILE` > 默认根 `.env`。显式相对文件按仓库根解析，缺失失败。接受可选 export、成对引号，不展开变量或命令；首条同名值保留，导出变量始终优先。

SQLite、Worker 覆盖路径与测试输入相对仓库根；data/log 保留相对 `services/` 的含义。环境文件的位置、cwd 和 profile 不改变数据库/存储。`OCCCCAD_GEOMETRY_WORKER_BIN` 显式覆盖仍受尊重；默认 Worker 跟随当前 profile。

`OCCCCAD_DATABASE_URL` 优先于 PostgreSQL 字段；SQLite 连接同一专用文件，PostgreSQL 使用同一数据库的 `occccad` schema。`OCCCCAD_ARTIFACT_BACKEND=LOCAL|S3` 与 data/S3 变量共同决定制品和暂存。不添加 TEST_DATABASE_URL 或测试桶，不自动迁移现有业务数据。

## PostgreSQL

真实集成通过 `testsupport.OpenDevelopmentDatabase` 打开并迁移应用资源，不重置。PostgreSQL pool、迁移、schema parity 由 integration tag 选择；SQLite 配置下明确报告 NOT_APPLICABLE，不能宣称 PostgreSQL 已验证。单元测试可自己拥有短期 SQLite、故障注入对象和本机 RPC stub。详见[数据库](../../services/internal/database/README.md)。

## 启动与数据重置

```bash
mise run dev --profile release --jobs 4
mise run dev --no-web --profile release --jobs 4
mise run web:dev
mise run doctor --live
mise run monitor
```

dev 完成增量构建，由现有 Control 管理 API、Jobs、Router/Worker 和可选前端。默认包含 Web，`--no-web` 仅后端。前端默认监听 `0.0.0.0:5173`，应用 API 默认监听 `0.0.0.0:8080`；可用 `OCCCCAD_WEB_LISTEN` / `OCCCCAD_APP_LISTEN` 覆盖。Control 管理、内部 API 和调试端口仍只允许环回地址。前端运行 Vite development server，支持 React Fast Refresh、CSS 热更新和必要时整页重载；release/debug 只选择 Go/C++ 构建配置。重复启动报告已有所有者；未知端口占用不杀进程。Ctrl-C 请求停止本次进程；顶层 mise 的 SIGINT 退出码实测为 130。验证清理需要等待所拥有的 TCP 监听关闭，不能只等 mise 父进程退出。启动只在复制期间持有 C++/Go 共享锁，将 Control/API/Jobs/Worker 复制到 `build/runtime/instances/<runtimeId>`，由 Control 从该快照启动及重启子进程；随后释放构建锁。因此同 profile 可以继续增量 build/check，运行进程仍使用启动时的快照。退出后删除本次快照，日志和身份回执保留。

后端运行身份包含 workspace、profile、后端 Go/C++/Proto 编译输入、有效配置、原产物与加载快照 hash；doctor --live 另检查 `/proc` 的进程实例和实际可执行文件。README、测试文件、前端源码热更新及 `.env` 注释不会让后端失效；后端源码、相关配置或磁盘产物更新后，doctor 提示增量构建并重启 dev。前端 Vite/HMR 单独记录，不代表后端更新。

推荐循环：启动 dev → 修改代码 → 在另一终端增量 build/check → 按需 Ctrl-C 后重新 dev。真实集成和 CTest 在执行期间保留 C++ 共享锁，防止所用 Worker/测试二进制被重链接；构建、配置、依赖同步和 clean 仍需单写入者锁。冲突先显示锁路径及解除方式，15 秒内未获得锁则明确阻塞并退出，可在对应验证完成后重试。`OCCCCAD_BUILD_TIMING` 和其 JSON 日志分别记录锁等待、依赖检查、配置、原生构建与总耗时。

Control 管理端口、内部 API 与调试目标仅接受数字环回地址。VS Code 使用同一 Worker Debug 产物，Go/Delve 使用原生服务路径与相同加载器。调试切流用 `mise run debug:route --service api --target 127.0.0.1:18081`，退出用 `--off`；Jobs 调试先暂停托管消费者，结束恢复；编辑器 postDebugTask 负责还原路由。

`mise run clean --component cpp --profile debug` 只删除指定可重建输出，不删除数据库、制品、报告、诊断、日志或全局缓存。拒绝符号链接、跨 checkout 路径及与配置资源重叠。旧构建目录不会自动搬迁或清理。

独立破坏性入口为 `mise run data:reset --yes`。先停止占用进程，命令检查本 checkout 运行锁与配置端口并展示脱敏目标；沿用 migrate 的保护：只清理配置的 PostgreSQL schema 或专用 SQLite 全部表/视图/迁移记录、本地制品与暂存、专用 S3 桶的对象/版本/删除标记/分片，保留桶。跨存储删除不原子，失败修复后重跑。本轮工程验证不需要清空真实资源。

## 测试资源与默认目录

生成输入与导出统一在 `build/test-resources`，不是第二套应用环境。解析 fixture 缺失或生产者更新时调用当前 `AssemblyExactSupport.ExportAnalyticRouterFixtures`；Naming fixture 由 `occccad_naming_artifact_tests` 生成。缺真实生产者不能改用 Mock。

| 变量 | 相对默认资源根的路径 |
|---|---|
| `OCCCCAD_ASSEMBLY_FIXTURE_DIR` | `analytic-fixtures` |
| `OCCCCAD_TEST_IMPORT_BREP` | `import-repair.brep` |
| `OCCCCAD_TEST_NAMING_FIXTURE` | `naming-v2.pb` |
| `OCCCCAD_TEST_CURVED_GLB` / `OCCCCAD_TEST_GLB_FIXTURE` | `curved.glb` / `geometry.glb` |
| `OCCCCAD_TEST_ASSEMBLY_REPLAY_OUTPUT` | `assembly-replay.3dreplay` |

大 STEP 输入由 `OCCCCAD_TEST_EXCHANGE_STEP` 显式选择；不自动下载 corpus。S3 round trip 随所选后端执行，LOCAL 不另建桶。

## Chromium 与浏览器验证

`mise run doctor --scope web --browser` 检查已有 Chromium。setup/build/check 默认不下载浏览器；仅在明确需要时人工执行 `pnpm --dir web/apps/cad exec playwright install chromium`。缺系统库时对实际浏览器运行 ldd 并补缺项，不下载临时库。

browser-mock 在 5174 启动隔离前端；browser-live 使用 5175 与同一真实 API/账号/存储，先启动 dev。入口见 [Web README](../../web/apps/cad/README.md)。Node、Mock、真实 RPC 与 Live WebGL 证据不能互相替代。
