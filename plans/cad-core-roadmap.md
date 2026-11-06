# CAD 核心主线

当前长任务：统一 Part/Product 参数与公式、分析依赖更新、测量输出与标注、结构树定位、参考元素及几何图形集。以下五阶段实现已接入正式调用链，范围与限制归所属 current 主题：

- [x] 共同参数所有权/来源/能力、Product 参数、几何量单位与编辑历史。
- [x] 共同依赖端口和输入证据、测量/空间分析失效及更新、公式时效与循环门禁。
- [x] 测量输出参数、持久标注布局、过期标注与统一显隐。
- [x] 空应用分组裁剪、基于操作意图的定位展开。
- [x] 几何图形集、参考点线面、参数子节点及标准平面直属显示。

验收沿正式 Command/Revision/Worker/历史链执行；标签布局不写建模历史，刚体移动不得使内在面积过期，过期输出不得伪装为当前公式输入。当前合同见[模型/参数](../docs/architecture/current/model-history.md)、[测量](../docs/architecture/current/geometric-measurement.md)、[语义树](../docs/architecture/current/semantic-tree.md)及[参考元素](../docs/architecture/current/reference-elements.md)。定向测试与浏览器验收覆盖核心流程，具体执行结果以当次 build 报告为准。

几何测量：Part/Product 共用的临时计算、保留/更新/历史及视区标注已接入，合同和未解决范围见[几何测量](../docs/architecture/current/geometric-measurement.md)。继续依据真实使用反馈完善专用上下文来源、标签排布和选择语义；不把这条基础闭环扩写成 CATIA 全部测量模式或物理测量已交付。

当前重点：装配空间分析与实时干涉的基础性能收口。复用完成的精确图形、最终数值检查点和显示轮廓，保留失效／取消及只读边界；持续变化构型的精确交集成本与大装配规模仍待专项优化。切割已接入局部移动／旋转手柄、平面与边缘拖动、数值同步、精确几何定位和简洁截面视图；拖动临时裁剪、松开计算，显式保存才写入历史。当前支持碰撞、切割和距离／距离区域，使用 Applications 下的稳定分析对象、已接受装配快照和派生结果制品；来源变化标为待更新，显式更新成功才替换结果。能力与边界见[空间分析](../docs/architecture/current/product-assembly.md#空间分析对象与手动更新)；机构试动保留[离散采样提示／停止](../docs/architecture/current/kinematics-dmu.md#试动和录制会话)。后续依据实机反馈完善复杂实体、显示精度与规模；全局自动／冻结更新和连续防穿透尚未实现。

历史创建阶段性收口：独立 Part 与直接、可编辑、READY 的 Product ContextVariant 支持历史补草图及同 Body、单方向 FINITE 的 LINEAR_EXTRUDE ADD/REMOVE；沿用四种修饰插入、受约束移动、完整候选检查及历史。能力和边界见唯一 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)，新增上下文交互仍待手工验收；默认构型发布失效时预览与提交共用发布解析门禁，历史输入为空不显示占位实体。

保留后续扩展：其他生成器的合法历史创建、更多受约束移动、任意深层上下文编辑、跨层更新编排、柔性场景及交换验收；具体限制归所属 current 主题，本阶段不继续扩展。

骨架引用的后续交互见[骨架与跨零件引用流程设计](skeleton-reference-workflow.md)：多个普通 Part/Product 作为设计意图来源，从消费参数或几何输入建立引用，在所属对象内编辑和修复。本轮仅移除独立“关联更新与修复”命令，保留内部 UpdatePlan；新引用编辑器、带链接复制和深层编排仍为设计，未实施。


沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
