# 实体特征支线

> 状态：待实施。返回[统一路线](README.md)。目标契约见[Part](../docs/architecture/target/part-model.md)与[Feature 质量门](../docs/architecture/target/part-evaluation.md)。

基线已具备参数绑定、Publication 与 Extrude/Boolean naming。每个 Feature 批次都必须同时完成 typed schema、编辑、表达式参数、validator、canonical hash、evaluator version、OCCT Shape gate、topology history、PersistentSelection、UI、Undo/Redo、资源诊断和 corpus；单独增加工具栏按钮或 OCCT 调用不算完成。

### FEATURE-REVOLVE-HISTORY：Revolve semantic naming 与 history

补齐旋转生成面、cap、seam、axis degeneracy、full/partial angle 和 Boolean 后续历史；覆盖 Face/Edge/Vertex resolver。

### FEATURE-REVOLVE-EDIT：Revolve 编辑和关联闭环

贯通方向、角度、轴引用、New/Add/Remove/Intersect、预览、属性编辑、面上草图和 Product 引用；以完整 E2E 冻结 Revolve 基线。

### FEATURE-HOLE：Hole 工程 Feature

实现独立 Hole schema，第一批支持 Blind、Through All、方向、直径和深度；支撑面/轴使用 PersistentSelection/Publication，不能长期只把 Hole 表达为匿名 Remove。

### FEATURE-FILLET：Fillet

先支持等半径常规 Edge 集；重点验证 Edge split/merge、半径失败、切线传播策略、naming 歧义和下游引用。

### FEATURE-CHAMFER：Chamfer

支持距离/距离和距离/角度的受控子集；复用 Fillet 的 Edge selection/history gate，但保持独立 typed semantics。

### FEATURE-PATTERN：Part Pattern

先实现线性/圆周 Feature Pattern；每个成员使用稳定 member key，数量变化产生可解释的保留和 tombstone，不能按数组下标顶替身份。

### FEATURE-MIRROR：Mirror

实现 Feature/Body 镜像的明确策略、镜像坐标框架和 member identity；不使用负 scale 伪造刚体或拓扑身份。

### FEATURE-SHELL：Shell

先用代表性 corpus 评估 OCCT history 完整度和 naming 风险，再完成等厚、向内/向外及移除面子集的首个全链路切片；无法可靠传播 history 的 case 必须明确诊断。

### FEATURE-DRAFT：Draft

基于稳定 neutral plane、pull direction 和 Face selection 实现常角度 Draft 子集；覆盖方向翻转、零/极限角、面删除和下游引用。Loft/Sweep 在 section 对应、seam 和 guide identity 设计完成后另立计划。


## 依赖与完成门

先完成 Revolve history，再完成 Revolve 编辑与关联闭环，之后按 Hole → Fillet → Chamfer → Pattern → Mirror → Shell → Draft 的推荐顺序推进。这是单人串行优先级，不表示所有后续 Feature 数学上依赖前一个 Feature；提前启动必须有自己的 naming/corpus 和明确范围。

每项覆盖正常生成、上游修改、真实删除/歧义、Undo/Redo、冷重建及 Product 引用。完整类型链和结果门没有完成前，不把已有 Revolve 几何生成等同于稳定持久命名。Loft/Sweep 进入条件见[候选方向](candidates.md)。
