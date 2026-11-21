# Part Feature 求值与草图上下文

本文拥有实体求值阶段顺序及 Sketcher 的显示边界。Part/Body/Sketch 模型见[Part/Sketch](part-sketch.md)，各特征范围及失败 Revision 合同见[实体 Feature](solid-features.md)，命名历史与解析规则见[Naming](persistent-naming.md)。

独立曲线/曲面共用 Part DAG、参数、候选与制品流程，但不执行材料布尔及 Solid 材料门禁；其域、归属与限制归[线框与曲面](wireframe-surfaces.md)。`THICKEN` 将有界 Sheet 显式实体化后进入下述原 Body 管线，后续修改继续使用同一门禁。

## 权威求值链

```text
typed Feature definition
  → 解析参数、上游阶段与稳定引用
  → 精确 Profile / Tool Shape
  → BodyOperation + generator / Boolean history
  → same-domain normalization + history
  → BREP / Body policy validation
  → semantic adjacency closure
  → complete-shape Naming gate
  → 不可变 BREP / Visual / Naming 制品
  → evaluator 最终模型的 ChangeSet / CAS
```

OCCT IsDone 不是接纳条件。结果须非空、有效、满足多 Solid Body 规则，并让每个最终 Face/Edge/Vertex 都有唯一语义输出及 lineage。生成、Boolean 和同域合并先保留同类型真实历史；仅未覆盖的最终交线/点等可进入有版本的语义邻接闭包。完整规则统一归 [History 生成与制品门](persistent-naming.md#history-生成与制品门)。

成功闭包可以报告 `TOPOLOGY_HISTORY_DERIVED_CLOSURE:<count>`；缺失、重复、悬空、类型不明及 live/tombstone 冲突仍拒绝。Worker 保留 Feature、求值阶段与稳定错误码，不能通过丢弃历史或接纳无名实体恢复。

创建与编辑的失败接纳规则、FAILED/BLOCKED、displayFallback 及预览资格统一见[失败和预览](solid-features.md#失败和预览)。局部特征的逐项输入状态、显式替换/移除、必需输入门禁及定向证据统一见[输入修复边界](solid-features.md#输入修复边界)。

[调整特征顺序](solid-features.md#调整特征顺序)和[历史位置创建](solid-features.md#在历史位置创建)按候选列表重建 Body 前驱和全 Part 依赖，使用候选阶段输入与缓存；不沿用旧图的隐式前驱边，不自动搬移或改绑输入。两者都必须通过全 Part 引用与几何检查；插入直接创建完整候选，不拆成创建后移动两笔事务。

## 零件增量求值

1. **确定重算范围。** 服务以规范输入、参数、依赖图、已接纳 manifest、单位和 evaluator 版本计算脏节点。每个 Body 按自己的特征顺序执行，独立几何按依赖求值。重排、插入和删除会改变前驱关系。
2. **恢复前驱。** 合格的持久检查点以 Body、Feature 与输入摘要定位。Worker 从匹配的 BREP 与 Naming tip 恢复形体和拓扑，沿用已接纳历史块。缺失制品或缺少阵列所需构造上下文时重建前缀。冷算从冻结输入重新执行。
3. **验证精确结果。** 检查有效边界、有界实体、材料变化与覆盖完整的命名。已验证前驱上的布尔可用工具内部点对比操作前后材料归属，证据不足时执行完整分区检查。未变拓扑仅在身份、位置、方向与容差符合条件时继承局部检查。精确体积、面积、质心等按测量或属性查询计算。[实体边界](../../../kernel/occt/src/body_validation.cpp) · [抽壳的适用范围](shell-algorithm.md#性能与限制)
4. **生成显示与命名。** 网格和边采样仅复用具有相同几何、参数和显示策略的部分。完整冷算仍可验证 BREP 与 Naming。根及历史块的引用、局部定位和证据合同见[制品格式](visual-naming-artifacts.md)。
5. **接纳并发布。** BREP、Naming 根和本次历史块在事务外读写并校验长度与摘要，在同一短事务登记。已接纳前缀沿用，未返回的后缀按摘要查询。命名闭包与显示关联验证后登记几何表示，预览保持 Head，确认再走 Revision/CAS。

### 通用特征性能与增长边界

构造上下文、历史块和网格缓存受预算约束。原生阶段可共享冻结的形体、工具与历史前缀，淘汰后由正式输入恢复。取消在原生运算及接纳边界检查。部分原生调用可能到返回时才响应取消。模型冻结、确有变化的拓扑及完整显示仍产生成本。持久制品的自动物理 GC 尚需覆盖 Revision、跨文档引用、候选与活动 Job。

## Sketcher 的显示上下文

进入草图时保留现有 Body 显示、GPU 缓冲和拾取资源。草图工具按活动草图过滤输入，投影或支撑选择可以显式读取模型。只有真实几何消费者受影响时才安排 Body 求值。确认复用当前输入对应的候选，迟到响应不能替换新会话。

## 源码、验证与限制

- [执行计划](../../../services/internal/workspace/part_execution_plan.go)、[前驱恢复](../../../services/internal/workspace/part_checkpoint.go)、[几何接纳](../../../services/internal/workspace/geometry_artifacts.go)、[命名接纳](../../../services/internal/workspace/naming_history_artifacts.go)。
- 跨 Body 分区布尔后连续倒角、圆角，提交与冷重建的部分 Naming 证据摘要可能不同。撤销删除 Body 后，恢复制品的摘要可能变化。
- 贯穿孔与它的阵列之间插入其他特征时，BREP/Naming 前驱可能缺少原孔构造上下文，导致额外重算。`TestDirectionalThroughEngineeringHoleReplicationThroughRouter` 检查此路径的几何与阶段复用。

[工程验证场景](../../validation/part-feature-performance.md)
