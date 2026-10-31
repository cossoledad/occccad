# CAD 核心主线

当前重点：干涉与间隙检查。已接通四种范围、精确分类／按需局部图形、临时隔离和剖切，以及机构会话采样提示／停止；当前能力与限制见[装配检查](../docs/architecture/current/product-assembly.md#静态干涉与最小间隙)和[机构会话](../docs/architecture/current/kinematics-dmu.md#试动和录制会话)。后续依据实际反馈完善复杂实体、规模与运动区间覆盖；不将离散采样表述为连续防穿透，不设计结果保存或结构树分析对象。

历史创建阶段性收口：独立 Part 与直接、可编辑、READY 的 Product ContextVariant 支持历史补草图及同 Body、单方向 FINITE 的 LINEAR_EXTRUDE ADD/REMOVE；沿用四种修饰插入、受约束移动、完整候选检查及历史。能力和边界见唯一 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)，新增上下文交互仍待手工验收；默认构型发布失效时预览与提交共用发布解析门禁，历史输入为空不显示占位实体。

保留后续扩展：其他生成器的合法历史创建、更多受约束移动、任意深层上下文编辑、跨层更新编排、柔性场景及交换验收；具体限制归所属 current 主题，本阶段不继续扩展。


沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
