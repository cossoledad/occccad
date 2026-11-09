# 复杂导入件的切除性能

2026-11-09 UTC，本机 Release / OCCT 7.9.1，AMD Ryzen 9 9950X / WSL2，正常配置的 SQLite、LOCAL ArtifactStore 和生产 Router。完整数值、冻结输入摘要、二进制/源码指纹及计时边界见[机器样本](part-feature-performance.json)。这是指定输入的串行小样本，不代表 P95、整车容量或与 CATIA 的统一基准。

## 输入与操作

一个有效导入 Solid：827 Face、2169 Edge、1390 Vertex，包含 256 个三次 BSpline 面；冻结 BREP 约 4.94 MB，体积约 620928.47 mm³。圆草图半径 15.596120225694847 mm，原点零，X 方向 +X、支撑法向 −Y；反向切除 40 mm，再改为 41、42 mm。精确 Profile、默认 0.02 mm / 0.12 rad 显示策略保持一致。

原始模型不随仓库分发，也不以可清理的开发文档 ID 作为复现前提。输入 SHA-256 和完整草图平面记录在 JSON；没有同一 BREP 时，以下性能数字不能直接类比其他模型。

## 观察结果

| 边界 | 优化前 | 优化后 |
|---|---:|---:|
| 首次新切除：正式 API 到预览响应 | 89.125 s | 4.751 s |
| 深度 40 → 41：重新求值 | 85.631 s | 4.211 s |
| 深度 41 → 42：重新求值 | 未测 | 4.122 s |
| 完全相同输入 | 约 35 ms | 约 36 ms |
| 响应后的 GLB 预览呈现 | 6.170 s | 0.750 s |
| 呈现期间最长主线程任务 | 6.023 s | 0.575 s |

“首次”指新启动运行时中，已接纳导入件上的第一个新特征意图：实际执行生成器和材料布尔，恢复的是既有导入根证据，不是缓存的切除结果。它不包含最初文件导入，也不是 `ForceCold` 的根证据重建。后两种不同参数也实际执行，不以完全相同请求命中作为优化结论。

首次 Worker 分阶段为 base 0.521 s、exact 0.561 s、Naming 1.393 s、display 1.641 s；mesh 1.398 s 和 encoding 0.034 s 已包含在 display 中。一次局部材料积分、8 个新面证据、822 次面证据复用，邻接约 3.4 ms。Worker 阶段与整个 API 的计时边界不同，不用相减推断网络开销。

本次切除前后的 BREP SHA-256 完全相同，最终均为 828/2171/1392 个 Face/Edge/Vertex，740450 个三角形、395065 个显示顶点。质量由局部积分加减得出，与原整体积分的体积相对差约 8.33×10⁻¹¹；不是所有几何输入的误差上界。有效性、完整 Naming 和原积分精度策略继续生效，未降低显示精度。

原取消样本在 18 s 时中止客户端，原生计算随后仍消耗约 43.98 CPU s。优化后分别在 0.75 s、3 s 中止的新操作样本，额外 Worker CPU 约 0.31 s、0.38 s；含连续空闲确认的观察尾段约 1.20 s、1.40 s。取消时点不同，因此不能据此给出统一中断延迟。所有原模型测量均未提交或改变 Head。

## 验证与复建入口

- `mise run check --scope all --level unit --jobs 4`：共享 Proto/Worker 的单元覆盖；报告中的实际 case、失败和 skip 才是当次证据。
- `mise run check --scope services --level integration --match 'TestPartFeatureChainIncrementalAndColdThroughRouter|TestSolidBooleanLifecycleThroughRouter' --test-jobs 1`：正常数据库及 Router，八次切除、修改中间步骤、前缀执行计数、冷 BREP/Naming 摘要对照、预览只读、补偿历史。
- `TestEdgeVertexPersistentSelectionThroughRealRouter/import naming diagnostics`：冻结导入根恢复、首次操作与冷重建 BREP/Naming 等价、导入引用及真实 STEP/BREP 归一化。
- 在 `web/apps/cad` 运行 `pnpm exec playwright test --config playwright.live.config.ts browser/solid-feature-live.spec.ts --grep 'Hole preview keeps one shaded result'`：真实预览、拾取、取消、提交、Undo/Redo 及重开。

内核邻接与独立两两 oracle 对照、冻结根拒绝错误身份/摘要、多个 Solid（分离/重叠）的整体积分回退、惰性拓扑属性、取消不返回部分网格的回归，位于 `kernel/occt/tests/geometry_exchange_scenarios.cpp`。原始运行报告在 `build/agent-check/`；本机原模型测量及 CPU profile 在 `build/performance/import-cut-analysis/`。这些可清理文件不是功能的持久输入。

当前仍有约 4–5 s 的首次响应成本，Naming、完整剖分及数据装配仍占明显比例。当前缓存预算也不覆盖 OCCT 全进程内存；单次积分及部分构造器仍不能中途取消。未对复杂导入件的所有局部修改器、INTERSECT 或长链规模给出性能保证，需要整体积分的分支仍可能较慢。继续扩大真实复杂 STEP 和长链样本后再确定下一轮优化，不从本样本推断任意模型都能立即完成。算法边界归[Part 求值](../architecture/current/part-feature-evaluation.md#零件增量求值)，后续入口归[CAD 核心主线](../../plans/cad-core-roadmap.md)。
