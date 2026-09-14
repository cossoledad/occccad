# OCCT Adapter Agent Guide

## Scope and invariants

`kernel/occt` 是精确 B-Rep、交换、布尔、拓扑分析与 persistent naming 的 OCCT 适配层。公共领域值类型留在 `kernel/api`；OCCT headers/types、local topology ID 和遍历顺序不得越过适配边界成为持久身份。

- 参数模型是源，B-Rep/GLB/TopologyInfo 是可重建制品。
- 算法成功标志之后仍需 Shape、容差、退化与 topology-history 完整性门禁。
- stable semantic output/lineage 来自领域 identity 与实际 OCCT history，不来自坐标猜测或数组下标。
- STEP/BREP corpus 是不可信输入；保持大小、路径、资源和失败诊断边界。

## Context route

先查 `kernel/api/include/occccad/kernel/` 的 OCCT-free 契约，再在 `src/occt_kernel.cpp` 和 `tests/geometry_exchange_scenarios.cpp` 搜操作/测试名。两者都是大文件，禁止默认整读；按 Import/Export、Profile/Feature、Boolean、Topology/Naming 或 Tessellation 责任展开。

协议/RPC 问题转到 `workers/geometry/AGENTS.md`；Product 选择解析转到 `services/AGENTS.md`。只有改变持久拓扑语义时读目标架构 5.7。

## Validation

- 领域验证：`invoke check --scope geometry`
- 单个测试：`ctest --test-dir build/cmake/debug -R '^geometry/<name>$' --output-on-failure`
- 只有测试明确需要真实交换输入时才读 `models/`；通常从测试中的 fixture 名和断言开始。
