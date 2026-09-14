# Assembly Solver Agent Guide

## Scope

本目录只拥有与 OCCT、Product、RPC 和持久化无关的三维刚体约束算法。调用方应先把稳定 `InstancePath`/PersistentSelection 解析为 body-local Point/Axis/Plane/Cylinder；本模块不解析业务文档。

## Invariants

- Pose 是 local → world 的 `SE(3)`；长度与角度尺度、容差和 branch 均显式。
- `initial_pose` 固定 nominal/branch，`initial_guess` 只作数值 warm start。
- 求解顺序是硬约束可行性 → reference 最小运动 → total nominal 最小运动；不得临时 Fix reference 再重试。
- 几何收敛、偏好收敛、rank/DOF 和冲突证据分别报告。稳定 constraint/equation identity 必须贯穿诊断。
- 解析 Jacobian 是正式路径，中央差分只作 conformance oracle；变换、body/constraint 顺序应保持语义等价。

## Context route

先在 `include/occccad/assembly/solver.hpp` 搜类型或入口，再在测试与 `src/solver.cpp` 搜具体符号。`src/solver.cpp` 超过 150 KB，不要整文件读取；按 constraint residual、Jacobian、rank/freedom、hierarchy/motion 或 diagnostics 命中范围展开。

当前行为看 `README.md`；数值算法看 `SOLVER_ALGORITHMS.md`，其中第 13 节记录 `solver.cpp` 的职责拆分计划；只有改变阶段边界、公共能力或长期 solver 设计时读 `SOLVER_ARCHITECTURE.md` 及目标架构 5.6。不要默认读全部三份。

## Tests and validation

- 场景：`tests/assembly_solver_scenarios.cpp`、`tests/motion_scenarios.cpp`
- corpus：根 `tests/assembly-corpus/`
- 局部验证：`invoke check --scope assembly`
- 精确回归：`invoke check --scope assembly --match '<test regex>'`，随后按语义风险升级到完整 assembly scope
- 单个回归：构建 `occcad_assembly_solver_scenarios` 后用 `ctest --test-dir build/cmake/debug -R '<exact regex>' --output-on-failure`
- 数值/性能变化另运行 `invoke performance-baseline`；它不替代正确性测试。
