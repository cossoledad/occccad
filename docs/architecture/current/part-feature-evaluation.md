# Part Feature 求值与草图上下文

本文拥有实体求值阶段顺序及 Sketcher 的显示边界。Part/Body/Sketch 模型见[Part/Sketch](part-sketch.md)，各特征范围及失败 Revision 合同见[实体 Feature](solid-features.md)，命名历史与解析规则见[Naming](persistent-naming.md)。

独立曲线/曲面共用 Part DAG、参数、候选与制品流程，但不执行材料布尔及 Solid 体积门禁；其域、归属与限制归[线框与曲面](wireframe-surfaces.md)。`THICKEN` 将有界 Sheet 显式实体化后进入下述原 Body 管线，后续修改继续使用同一门禁。

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

导入件的首次新操作可从该 Body 已接纳的不可变 Naming 制品恢复 ImportBody 根 transition。输入绑定冻结 BREP SHA-256、Feature/Body、导入身份分配、OCCT/策略版本、完整历史及正体积；体积来自原始导入制品，不读取已切除后的 Body Tip。Worker 验证制品摘要和归属，内核逐项验证 typed locator 与随机身份覆盖。没有可恢复的根时执行完整初始化；`ForceCold` 显式绕过恢复和阶段缓存。此优化不需要曾经计算过同一个切除结果。

输入与工具均为单 Solid 时，ADD/REMOVE 复用本次 Boolean 的交集分区，在新增或移除的材料区域执行原精度体积积分；以输入体积加减变化量得到结果体积。无材料变化仍按原阈值拒绝，边界接触不作为材料，空结果仍拒绝。差值接近整体数值抵消时，回退为结果整体积分。多个 Solid（包括重叠分量）的补集可能改变有向体积的分量重数，保留原输入/结果整体积分；INTERSECT 也保留输入/结果体积检查。结果仍执行有效 Solid 集、无游离拓扑、同域合并及完整 Naming 门禁；已验证的同一结果不重复检查。

一次请求内按精确 OCCT occurrence（包含朝向）复用质量与几何证据；同一语义引用且拓扑未变时可沿用已接纳证据，邻接按本次结果重建。最终邻接使用包含/共享边界索引，不做全部拓扑两两扫描。上述索引不是持久身份，规则见 [Naming](persistent-naming.md)。

前缀检查点带输入与策略 digest、历史、质量、命名及构造上下文。缓存超预算时可仅保留经过验证的 Tip 检查点；缺少成员重执行上下文的检查点不得服务 `SOLID_PATTERN`，须重新建立完整上下文。取消后不接纳检查点；淘汰与冷重建不改变结果语义。

Boolean 和剖分通过 OCCT progress 协作取消，transport 状态轮询节流；制品发布前仍检查取消和过期资格。单次几何积分及部分构造器仍可能在返回后才响应取消，不承诺所有原生算法立即中断。

`PartRuntimeStats` 分开记录 base、exact、Naming、display、mesh、encoding、邻接、积分次数及证据复用；mesh/encoding 属于 display 的子阶段，不能相加。指定复杂导入件的测量与边界见[零件性能测量](../../validation/part-feature-performance.md)。

## Sketcher 的显示上下文

进入草图改变交互，不创建第二份 BREP 或改写 Part 真相。视口叠加三个层次：当前 Revision 的只读 Body 制品、活动草图/约束/暂态预览、世界及支撑平面网格等参考元素。

显示与可选择性分离。Body 参与导航与遮挡，普通草图工具只选活动实体和约束；只有显式 ExternalGeometry 工具可捕获上游 Face/Edge/Vertex，并将原始命中转为正式持久引用。草图曲线、点和尺寸应在重合支撑面上方可见；显示状态不进入求值输入。

## 源码、验证与限制

- `services/internal/workspace/solid_feature.go`、`evaluation_projection.go`：正式定义、参数及求值编排。
- `kernel/occt/src/occt_kernel.cpp`：精确构造、历史与门禁；先按操作符号搜索。
- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：多区域/孔岛、ADD/REMOVE、boss/pocket、merge/split/delete、完整拓扑覆盖与冷重建。
- `services/internal/control/solid_feature_integration_test.go`：真实 Router/Worker、预览、提交、历史及重开。
- `services/internal/control/part_feature_performance_integration_test.go`：多步切除、前缀复用、上游编辑、冷 BREP/Naming 等价和 Undo/Redo。
- Web 的 `product-sketch-visibility`、`multi-body` 场景验证上下文过滤；真实深度、遮挡与相机行为仍须浏览器/WebGL 验收。

测试入口存在不是当次执行通过；几何、持久身份、历史和浏览器操作分别举证。执行方法见[测试路由](../../../tests/README.md)。
