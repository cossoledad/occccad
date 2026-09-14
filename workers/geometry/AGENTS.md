# Geometry Worker Agent Guide

## Scope

本目录拥有粗粒度 gRPC Geometry Worker、Proto C++ codegen 接线和同进程 PlaneGCS adapter。Worker 是易失计算容器，不拥有 Document、Revision、ACL 或业务数据库。

## Invariants and route

- 先读 `proto/occccad/worker/v1/geometry_worker.proto`，再读 server/client adapter；生成代码是 output，不是设计源。
- 新增/修改 RPC 必须检查 Worker server、C++/Go 生成、Go geometry client、GeometryPool/Router 代理和 capability，并通过正式 Router 路径测试。
- ArtifactReference 传大制品；Proto 不暴露操作系统路径、OCCT 或 PlaneGCS 类型。
- `src/main.cpp` 超过 75 KB：先按 RPC 名定位 service method，再读邻近 helper，不默认整读。
- Sketch solver 先看 `sketch/include` 的接口，再按 entity/constraint 在 `sketch/src` 与邻近 tests 搜索；backend 状态必须适配成稳定领域诊断。

当前 RPC、日志与运行事实看 `README.md`。精确 B-Rep 转到 `kernel/occt/AGENTS.md`；Assembly 数学转到 `kernel/assembly/AGENTS.md`；路由/控制面转到 `services/AGENTS.md`。

## Validation

- Sketch：`invoke check --scope sketch`
- OCCT 几何：`invoke check --scope geometry`
- 公共 Proto、通用 Worker server/CMake 或 Router contract：`invoke check --scope all`
- 精确单测可用 CTest 前缀 `^sketch/`、`^geometry/`、`^assembly/`；直连 Worker 成功不能证明 Router 完整。
- 稳定入口支持 `invoke check --scope <assembly|geometry|sketch> --match '<test regex>'`；精确回归后仍按契约风险升级。
