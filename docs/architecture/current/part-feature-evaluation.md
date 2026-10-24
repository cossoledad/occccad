# Part Feature 求值与草图上下文

本文拥有实体求值阶段顺序及 Sketcher 的显示边界。Part/Body/Sketch 模型见[Part/Sketch](part-sketch.md)，各特征范围及失败 Revision 合同见[实体 Feature](solid-features.md)，命名历史与解析规则见[Naming](persistent-naming.md)。

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

[调整特征顺序](solid-features.md#调整特征顺序)按候选列表重建 Body 前驱和全 Part 依赖，使用候选阶段输入与缓存；不沿用旧图的隐式前驱边，不自动搬移或改绑输入。移动必须通过全 Part 引用与几何检查。

## Sketcher 的显示上下文

进入草图改变交互，不创建第二份 BREP 或改写 Part 真相。视口叠加三个层次：当前 Revision 的只读 Body 制品、活动草图/约束/暂态预览、世界及支撑平面网格等参考元素。

显示与可选择性分离。Body 参与导航与遮挡，普通草图工具只选活动实体和约束；只有显式 ExternalGeometry 工具可捕获上游 Face/Edge/Vertex，并将原始命中转为正式持久引用。草图曲线、点和尺寸应在重合支撑面上方可见；显示状态不进入求值输入。

## 源码、验证与限制

- `services/internal/workspace/solid_feature.go`、`evaluation_projection.go`：正式定义、参数及求值编排。
- `kernel/occt/src/occt_kernel.cpp`：精确构造、历史与门禁；先按操作符号搜索。
- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：多区域/孔岛、ADD/REMOVE、boss/pocket、merge/split/delete、完整拓扑覆盖与冷重建。
- `services/internal/control/solid_feature_integration_test.go`：真实 Router/Worker、预览、提交、历史及重开。
- Web 的 `product-sketch-visibility`、`multi-body` 场景验证上下文过滤；真实深度、遮挡与相机行为仍须浏览器/WebGL 验收。

测试入口存在不是当次执行通过；几何、持久身份、历史和浏览器操作分别举证。执行方法见[测试路由](../../../tests/README.md)。
