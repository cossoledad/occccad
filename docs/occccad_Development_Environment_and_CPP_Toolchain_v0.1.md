# occccad 开发环境与 C++ 工具链规范

> **状态：** v0.1，按当前仓库校准
> **主平台：** Linux / WSL2 Linux
> **CAD 内核：** Open CASCADE Technology 7.9.1
> **最后更新：** 2026-08-09

## 1. 文档目的

本文档说明 **当前仓库真实可用** 的开发环境、构建入口和依赖边界。架构设计允许
工具链在未来升级，但任何升级都应先修改代码和 CI、完成验证，再更新这里；文档不把
尚未验证的 C++23、OCCT 8 或未来服务依赖描述成当前事实。

本文使用三个状态：

- **当前约束**：由仓库文件直接规定，构建必须满足；
- **已验证环境**：维护者实际编译测试通过的组合；
- **规划项**：目标架构需要、但当前尚未接入的能力。

## 2. 当前工程基线

### 2.1 已验证组合

2026-08-09 在 WSL2 Ubuntu 26.04 LTS 上验证：

| 组件 | 版本 | 依据或用途 |
|---|---:|---|
| Linux kernel | 6.6.114.1 WSL2 x86_64 | 已验证宿主环境 |
| GCC / G++ | 15.2.0 | 默认 C++ 编译器 |
| Clang | 21.1.8 | 已安装，尚未作为本次主构建验证对象 |
| C++ 标准 | C++17 | 根 CMake 与各 target 明确要求 |
| CMake | 4.2.3 | 已验证版本；项目最低 3.30 |
| Ninja | 1.13.2 | 当前生成器 |
| Conan | 2.31.2 | C++ 依赖管理 |
| Python | 3.14.7 | Conan / Invoke 运行环境 |
| Invoke | 3.0.3 | 统一开发入口 |
| OCCT | 7.9.1 | Conan 包 `opencascade/7.9.1` |
| Go | 1.26.5 | `services/go.mod` 声明版本 |
| Node.js | 26.7.0 | 前端占位工程已验证环境 |
| pnpm | 11.20.0 | `web/package.json` 固定版本 |

验证结果：`occccad_smoke_test` 通过，`occccad_geometry_worker` 能创建 10×20×30
的 Box，并返回 6 个面、12 条边、8 个顶点和 1 个实体。

### 2.2 仓库硬约束

真正的约束以代码为准：

| 文件 | 当前约束 |
|---|---|
| `CMakeLists.txt` | CMake >= 3.30，C++17，无编译器扩展 |
| `conanfile.py` | Conan 2；OCCT 7.9.1；GoogleTest `[>=1.14 <3]` |
| `build-support/conan/profiles/*` | Linux x86_64、libstdc++11、C++17 |
| `requirements-build.txt` | Conan 2、Invoke 3、cmake-format |
| `services/go.mod` | Go 1.26.5 |
| `web/package.json` | pnpm 11.20.0 |

“已验证版本”不是普遍最低版本。例如 CMake 4.2.3 已通过验证，但项目声明的最低版本
仍是 3.30。反过来，如果 Profile 写明 `gcc-15`，仅仅安装一个支持 C++17 的旧 GCC
并不足以直接运行默认命令。

### 2.3 当前与未来基线

| 领域 | 当前 v0.1 | 未来候选，尚未承诺 |
|---|---|---|
| C++ | C++17 | 需要明确收益和兼容验证后再评估 C++20/23 |
| OCCT | 7.9.1 | 通过回归测试后再升级 7.9.x/8.x |
| RPC | 未接入 | gRPC + Protobuf |
| 日志 | 标准输出 | spdlog 或其他结构化日志方案 |
| 数学/求解 | 未接入 | Eigen、Ceres 等 |
| 控制面 | Go module 占位 | 文档、产品、命令、调度服务 |
| Web | pnpm workspace 占位 | TypeScript/React/Three.js |
| 数据设施 | 地址占位 | PostgreSQL、Redis、S3-compatible storage |

升级路线不是安装说明。规划依赖只有在 `conanfile.py`、源代码和测试真正启用后，才列入
当前环境。

## 3. 平台约定

第一阶段将 Linux x86_64 作为一等开发平台。Windows 开发推荐 WSL2，代码建议位于
Linux 文件系统，例如：

```text
/home/<user>/project/occccad
```

不建议把工作区放在 `/mnt/c/...`：Conan cache、CMake、Ninja 和前端依赖包含大量小文件，
跨文件系统访问会影响性能和文件监听行为。

macOS、原生 Windows 和 Linux aarch64 尚无仓库 Profile 与 CI 证明，当前不声明支持。

## 4. 系统工具安装

在 Ubuntu 上先安装基础工具。不同 Ubuntu 版本的软件包版本不同，因此以下命令负责
安装“工具种类”，实际版本仍需用第 5 节检查：

```bash
sudo apt update
sudo apt install -y \
    build-essential \
    gcc-15 \
    g++-15 \
    cmake \
    ninja-build \
    git \
    pkg-config \
    python3 \
    python3-pip \
    python3-venv \
    ccache \
    gdb
```

如果发行版仓库没有 GCC 15 或 CMake 3.30+，应使用可信的软件源或工具版本管理器。
不要为了满足文档而全局替换系统编译器软链接；Profile 应明确工具路径。

Go、Node.js 和 pnpm 暂不参与当前 C++ 冒烟构建，可在开始对应子项目时安装。

## 5. 环境检查

```bash
gcc-15 --version
g++-15 --version
cmake --version
ninja --version
python3 --version
git --version
```

创建 Python 虚拟环境后，还可以执行：

```bash
invoke info
```

该命令显示实际被发现的 `g++`、CMake、Ninja、Conan、Python、Go、Node、pnpm、
构建类型、Conan Profile 和构建目录。注意：它显示的 `g++` 命令不一定等于 Profile
最终调用的 `g++-15`，排障时两者都要检查。

## 6. Python 构建环境

Conan 与 Invoke 是构建工具，不是 occccad 运行时依赖。推荐在仓库内使用 `.venv`：

```bash
python3 -m venv .venv
source .venv/bin/activate
python -m pip install --upgrade pip
python -m pip install -r requirements-build.txt
```

不要使用 `sudo pip install`。`.venv/` 和 Conan 的用户缓存不应提交到 Git。

`invoke bootstrap` 也会安装 `requirements-build.txt`，但它使用当前 Python 环境；先激活
虚拟环境再运行，避免污染系统 Python：

```bash
source .venv/bin/activate
invoke bootstrap
```

## 7. Conan 依赖策略

### 7.1 当前依赖

当前普通构建只有一个第三方运行依赖：

```text
opencascade/7.9.1
```

当前测试依赖：

```text
gtest/[>=1.14 <3]
```

`spdlog`、Protobuf、gRPC、Eigen、Ceres 和 Benchmark 在 `conanfile.py` 中仍是注释或规划项，
不应出现在“当前必须安装”的列表中。

### 7.2 系统包与 Conan 的边界

APT 负责编译器、CMake、Ninja、Python、调试器等基础工具；OCCT 等 C++ 库由 Conan
提供。构建不应偶然使用 `/usr/include/opencascade` 或系统 OCCT 动态库。

Conan 负责：

- 依赖解析和二进制缓存；
- OS、架构、编译器、ABI、C++ 标准和构建类型设置；
- 生成 CMake toolchain 与依赖配置。

CMake 负责项目 target、编译、链接和测试图。

### 7.3 Profile 现状

仓库当前有：

```text
build-support/conan/profiles/
├── linux-gcc14-debug
├── linux-gcc14-release
└── linux-clang-debug
```

前两个文件名保留了早期 `gcc14` 命名，但**文件内容已配置为 GCC 15、C++17**：

```ini
[settings]
compiler=gcc
compiler.version=15
compiler.cppstd=17

[buildenv]
CC=gcc-15
CXX=g++-15
```

因此排障和修改时必须查看 Profile 内容，不能从文件名推断编译器版本。后续可以单独
重命名 Profile 并同步 `tasks.py`，但本次文档校准不改变已跑通的构建入口。

`linux-clang-debug` 当前声明 Clang 19；本次主验证环境中的 Clang 21 不等于该 Profile。
在修订 Profile 并完成测试前，不把 Clang 21 标记为已支持构建。

### 7.4 可复现性边界

仓库目前没有已提交的 Conan lockfile，因此依赖版本范围内的传递依赖仍可能随解析时间
变化。`conanfile.py` 固定 OCCT 7.9.1，但 GoogleTest 使用版本范围。

要达到严格可复现构建，后续需要：

1. 为 Debug/Release 生成并提交 lockfile；
2. 在 CI 使用 lockfile；
3. 记录 Conan remote；
4. 对升级执行几何回归和性能验证。

在这些工作完成前，文档不会声称构建已经完全可复现。

## 8. 标准构建流程

`tasks.py` 是当前统一入口。

### 8.1 Debug

```bash
source .venv/bin/activate
invoke configure --build-type=Debug
invoke build --build-type=Debug
invoke test --build-type=Debug
invoke run.geometry --build-type=Debug
```

生成目录：

```text
build/cmake/debug/
```

### 8.2 Release

```bash
invoke configure --build-type=Release
invoke build --build-type=Release
invoke test --build-type=Release
```

生成目录：

```text
build/cmake/release/
```

### 8.3 命令语法

Invoke 的可选参数使用具名形式：

```bash
invoke configure --build-type=Debug
invoke build --build-type=Debug --target=occccad_geometry_worker --jobs=8
invoke test --build-type=Debug --filter=occccad_smoke_test
```

不要使用文档早期版本中的 `invoke configure debug`。可随时用下面的命令查看真实接口：

```bash
invoke --list
invoke configure --help
```

### 8.4 CMake Presets

仓库保留 `dev-debug`、`dev-release` 和 `ci-debug` presets。它们本身不负责下载 OCCT；
干净环境需要先运行 Conan。日常开发优先使用 `invoke configure`，因为它会依次完成
Conan install、定位 toolchain 和 CMake configure。

`CMakeUserPresets.json` 包含 Conan 生成文件的路径，该文件只有在相应构建目录已经生成后
才有效，不应当被理解为全新 clone 可以直接配置。

## 9. 构建产物与目标

当前主要 CMake targets：

| Target | 类型 | 作用 |
|---|---|---|
| `occccad_kernel_api` | Interface library | 与 OCCT 类型隔离的公共 API |
| `occccad_occt_kernel` | Static library | OCCT 适配实现 |
| `occccad_geometry_worker` | Executable | Phase 0 本地冒烟程序，尚非 gRPC server |
| `occccad_smoke_test` | Executable / CTest | Box、包围盒、拓扑和 unload 验证 |

成功的 Geometry Worker 输出应包含：

```text
[SMOKE] Created box: ...
faces:  6
edges:  12
vertices: 8
solids: 1
[PASS] OCCT MakeBox smoke test passed.
```

## 10. 测试与质量工具

运行全部当前测试：

```bash
invoke test --build-type=Debug
```

或直接使用 CTest：

```bash
ctest --test-dir build/cmake/debug --output-on-failure
```

根 CMake 已提供以下选项：

```text
OCCCCAD_BUILD_TESTS
OCCCCAD_ENABLE_ASAN
OCCCCAD_ENABLE_UBSAN
OCCCCAD_ENABLE_TSAN
OCCCCAD_ENABLE_LTO
OCCCCAD_ENABLE_CCACHE
```

`dev-debug` 当前默认关闭 ASan/UBSan，`ci-debug` preset 默认打开 ASan/UBSan。不要只依据
preset 的显示名称判断 sanitizer 是否启用，应检查它的 `cacheVariables`。

## 11. 当前 API 与实现边界

公共头文件位于：

```text
kernel/api/include/occccad/kernel/
```

业务层不应暴露或传递 `TopoDS_Shape` 等 OCCT 类型。OCCT 头文件和实现应尽量收敛在：

```text
kernel/occt/
```

当前已经实现并被测试的能力：

- `createBox`；
- 内存内 GeometryId 占位标识；
- `getBoundingBox`；
- 面、边、点、实体的拓扑计数与基础分类；
- `unload` 和 resident count。

当前接口中存在但尚未完整实现的能力：

- GeometryId 的真实 SHA-256 内容寻址；
- B-Rep 序列化/反序列化；
- 完整 tessellation 数据；
- 真正执行的 chamfer / fillet；
- gRPC server 和远程 Worker 生命周期；
- 对象存储、调度与故障恢复。

文档、Issue 和提交说明应区分“接口已定义”与“功能已实现”。

## 12. 外部服务与跨语言工程

`.env.example` 预留 PostgreSQL、Redis、S3-compatible storage 和各服务端口，但当前 C++
构建、测试和 Geometry Worker 冒烟程序不连接这些服务。因此完成 Phase 0 不需要预先启动
数据库、缓存或 MinIO。

`services/` 当前只有 Go module 声明；`web/` 当前只有 pnpm workspace 声明。开始相关开发时：

- Go 依赖以 `go.mod` / `go.sum` 为准；
- Node 与 pnpm 版本以 `packageManager` 和未来的 lockfile 为准；
- Proto 工具链应固定版本，并让 C++、Go、TypeScript 从同一份 schema 生成代码；
- 外部服务应通过 `.env` 或部署配置注入，不硬编码到源代码。

## 13. 常见问题

### 找不到 `gcc-15` 或 `g++-15`

默认 GCC Profile 明确调用这两个命令。安装 GCC 15，或复制 Profile 创建适合本机的版本，
然后显式传入：

```bash
invoke configure --build-type=Debug --profile=<profile-name>
```

不要只修改 Profile 中的 `compiler.version`；`CC`、`CXX`、ABI 和缓存包 ID 必须一致。

### Conan 找不到包或下载失败

检查：

```bash
conan remote list
conan profile show -pr:h build-support/conan/profiles/linux-gcc14-debug \
                   -pr:b build-support/conan/profiles/linux-gcc14-debug
```

首次构建可能下载或编译较多传递依赖。网络、remote 和本地 Conan cache 都会影响耗时。

### CMake 找不到 OpenCASCADE

不要安装系统 OCCT 作为绕过方案。重新执行：

```bash
invoke configure --build-type=Debug
```

并确认 `build/cmake/debug` 下生成了 `conan_toolchain.cmake` 和 Conan 的依赖配置。

### 改了 Profile 后仍复用旧配置

不同编译器或 ABI 不应共用同一构建目录。使用新的 build type/profile 组合，或在确认
无需保留产物后执行 `invoke clean` 再配置。`invoke clean` 会删除仓库内整个 `build/`。

### `invoke info` 与实际 Profile 编译器不同

`invoke info` 的 `g++` 行读取 PATH 中的通用命令；Conan Profile 可以调用带版本后缀的
`g++-15`。以 CMake configure 输出、Profile 和 `CMakeCache.txt` 三者共同判断实际编译器。

## 14. 工具链升级规则

升级编译器、C++ 标准、CMake 或 OCCT 时，至少完成：

1. 修改对应 Profile、CMake 或 Conan 声明；
2. 从干净构建目录重新解析依赖；
3. 编译 Debug 和 Release；
4. 运行全部 CTest；
5. 运行 Geometry Worker 冒烟程序；
6. 对 OCCT 升级增加 STEP、B-Rep、布尔、倒角、圆角、离散化等回归数据；
7. 同步 README、本规范和 CI；
8. 单独提交依赖锁文件变化，便于审查。

在完成这些步骤前，“计划使用 C++23”或“希望升级 OCCT 8”都只是设计方向，不能替代
仓库事实。

## 15. Phase 0 完成标准

当前工程骨架的可重复验收命令为：

```bash
source .venv/bin/activate
invoke info
invoke configure --build-type=Debug
invoke build --build-type=Debug
invoke test --build-type=Debug
invoke run.geometry --build-type=Debug
```

验收标准：

- Conan 使用仓库 Profile 并解析 OCCT 7.9.1；
- CMake 以 C++17 配置成功；
- 所有当前 targets 编译成功；
- CTest 报告 100% 通过；
- Worker 输出正确 Box 包围盒与拓扑数量；
- 公共 API 不泄漏 OCCT 类型。

这套基线是后续加入 gRPC、持久化和分布式调度的起点。新增能力不应破坏这条最小验证链。
