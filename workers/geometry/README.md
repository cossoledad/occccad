# Geometry Worker

Geometry Worker 是当前唯一的 C++ 网络计算服务。它通过粗粒度 gRPC 封装 OCCT，不向 Go 或浏览器暴露 OCCT 类型。

## 当前能力

- `Ping`：返回 Worker ID、OCCT 版本和 resident geometry 数；
- `SolveSketch`：求解版本化 Point/Line/Constraint SketchModel，返回坐标、状态、DoF 和冲突/冗余约束 ID；
- `EvaluatePart`：求值一个矩形草图/拉伸链或在基础 B-Rep 上追加拉伸；
- `InspectExchange` / `ImportExchange` / `ExportExchange`：通过 ArtifactReference 检查、导入和导出 STEP/BREP；
- `GetTopology`：返回面、边、点及诊断属性；
- 生成 SHA-256 GeometryId、B-Rep、三角网格、边折线、包围盒、体积和 GLB。
- 内置项目自有 `SketchSolver`/PlaneGCS 适配层；`GCS::*` 不进入公共头或 Proto。当前实体覆盖 Point/Line，约束覆盖 Coincident/Parallel/FixedPoint。

Proto 中已经声明但当前服务类没有覆盖的 `LoadGeometry`、`UnloadGeometry`、`Tessellate`、`CreateChamfer` 和 `CreateFillet` 会得到 gRPC `UNIMPLEMENTED`；协议声明不等于已交付能力。

## 内部结构

```mermaid
flowchart LR
    Client["Go gRPC client / Router"] --> Worker["Geometry Worker"]
    Worker --> API["kernel/api<br/>OCCT-free public types"]
    API --> Adapter["kernel/occt"]
    Adapter --> OCCT["Open CASCADE 7.9.1"]
    Worker --> SketchAPI["SolveSketch RPC / SketchSolver"]
    SketchAPI --> PlaneGCS["PlaneGCS shared library"]
    Worker --> Cache["in-memory resident geometry"]
```

`kernel/api` 定义稳定值类型和操作接口，`kernel/occt` 是唯一允许暴露 OCCT 头文件的适配层。Worker 是易失计算容器，不是文档真相来源。

## 协议与数据

- 契约：`proto/occccad/worker/v1/geometry_worker.proto`
- 包名：`occcad.worker.v1`
- 传输：gRPC/Protobuf
- 默认监听：`127.0.0.1:51001`
- 配置：`OCCCCAD_GEOMETRY_WORKER_LISTEN`

调用应是“求值完整 Part”“导入一个交换根”“合成一次导出”一类粗粒度操作，不能把每个 OCCT 函数映射为远程 RPC。请求携带 `request_id` 与 `geometry_key`；Trace Context 通过 gRPC metadata 传播。B-Rep、GLB、STEP 和 BREP 交换文件通过 ArtifactReference 读写，不进入 unary gRPC bytes；当前 `LOCAL` backend 要求 Worker 与 API/Jobs 共享 `OCCCCAD_DATA_DIR`，object key 必须是根目录内的 opaque 相对键。

## 构建与运行

```bash
invoke configure --build-type=Debug
invoke build --build-type=Debug --target=occccad_geometry_worker
invoke run.worker --build-type=Debug
```

PlaneGCS 适配器基础测试：

```bash
invoke build --build-type=Debug --target=occccad_plane_gcs_adapter_test
ctest --test-dir build/cmake/debug --output-on-failure -R PlaneGcsSketchSolver
```

几何冒烟程序：

```bash
invoke run.geometry --build-type=Debug
```

依赖版本以根 `conanfile.py` 为准，当前为 C++17、OCCT 7.9.1、gRPC 1.71.0、Eigen 3.4.0 和 header-only Boost 1.86.0。PlaneGCS 锁定 FreeCAD 1.0.2 的不可变 commit，CMake 仅下载带逐文件 SHA-256 的官方核心源码清单，并构建为独立 shared library；不链接完整 FreeCAD。构建产物旁的 `LICENSE.FreeCAD-PlaneGCS` 必须随该库分发。

## 资源与故障语义

- resident geometry 只存在于进程内；Worker 退出后必须能从持久 B-Rep 或参数模型重建；
- GeometryId 基于内容，不得包含 Worker 地址；
- 同一个 Worker 内的 OCCT 操作当前由互斥边界保护，扩展吞吐优先增加进程而不是假设所有 OCCT 路径线程安全；
- 输入 STEP/BREP 是不可信复杂文件；当前 Worker 拒绝空对象、越界 object key 和超过 512 MiB 的制品，生产环境仍需要更严格的时间、内存、实体数量和进程级隔离；
- 计算请求必须可重试，调用方不能把 Worker 局部状态当成唯一副本。

## 目标边界

二维草图约束推荐作为 Part Evaluation 内的独立模块与本 Worker 同进程部署，以避免草图求解和特征重生成之间的高频网络往返；三维装配约束不进入本 Worker，应使用独立 Assembly Solver Worker。原因与拆分触发条件见项目[目标架构](../../docs/TARGET_ARCHITECTURE.md)。
